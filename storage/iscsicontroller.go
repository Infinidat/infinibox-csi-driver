/*
Copyright 2022 Infinidat
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package storage

import (
	"context"
	"fmt"
	"infinibox-csi-driver/api"
	"infinibox-csi-driver/common"
	"infinibox-csi-driver/iboxapi"

	"strconv"
	"strings"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (iscsi *iscsistorage) ValidateStorageClass(params map[string]string) error {
	requiredISCSIParams := map[string]string{
		common.SC_USE_CHAP:      `(?i)\A(none|chap|mutual_chap)\z`,
		common.SC_NETWORK_SPACE: `\A.*\z`, // TODO: could make this enforce IBOX network_space requirements, but probably not necessary
	}
	optionalISCSIParams := map[string]string{
		common.SC_PROVISION_TYPE: `(?i)\A(THICK|THIN)\z`,
	}

	// validate required parameters
	err := ValidateRequiredOptionalSCParameters(requiredISCSIParams, optionalISCSIParams, params)
	if err != nil {
		zlog.Err(err)
		return status.Error(codes.InvalidArgument, err.Error())
	}
	return nil
}

func (iscsi *iscsistorage) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {

	params := req.GetParameters()
	zlog.Debug().Msgf("requested volume parameters are %v", params)

	zlog.Debug().Msgf("CreateVolume volume: %s of size: %d bytes", req.GetName(), iscsi.capacity)

	// Volume name to be created - already verified earlier
	name := req.GetName()

	poolName := params[common.SC_POOL_NAME]

	targetVol, err := iscsi.cs.Api.GetVolumeByName(name)
	if err != nil {
		zlog.Err(err)
		if !strings.Contains(err.Error(), "volume with given name not found") {
			return nil, status.Errorf(codes.NotFound, "%s", fmt.Sprintf("CreateVolume failed: %v", err))
		}
	}
	if targetVol != nil {
		zlog.Debug().Msgf("volume: %s found, size: %d requested: %d", name, targetVol.Size, iscsi.capacity)
		if targetVol.Size == iscsi.capacity {
			existingVolumeInfo := iscsi.cs.getCSIResponse(targetVol, req)
			copyRequestParameters(params, existingVolumeInfo.VolumeContext)
			return &csi.CreateVolumeResponse{
				Volume: existingVolumeInfo,
			}, nil
		}
		msg := fmt.Sprintf("CreateVolume failed: volume %s exists but has different size", name)
		zlog.Error().Msg(msg)
		return nil, status.Errorf(codes.AlreadyExists, "%s", msg)
	}

	// Volume content source support volume and snapshots
	contentSource := req.GetVolumeContentSource()
	if contentSource != nil {
		return iscsi.createVolumeFromContentSource(req, name, iscsi.capacity, poolName)
	}

	volType, provided := params[common.SC_PROVISION_TYPE]
	if !provided {
		volType = common.SC_THIN_PROVISION_TYPE
	}

	volumeParam := &api.VolumeParam{
		Name:          name,
		VolumeSize:    iscsi.capacity,
		ProvisionType: volType,
	}

	volumeParam.SsdEnabled, err = determineSSDValue(params[common.SC_SSD_ENABLED], poolName, iscsi.cs.IboxApi)
	if err != nil {
		e := status.Errorf(codes.Internal, "error when creating volume %s storagepool %s, err: %s", name, poolName, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, e
	}

	pool, err := iscsi.cs.IboxApi.GetPoolByName(poolName)
	if err != nil {
		e := status.Errorf(codes.Internal, "error when getting pool %s , err: %s", poolName, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, e
	}
	volumeResp, err := iscsi.cs.Api.CreateVolume(volumeParam, pool.ID)
	if err != nil {
		e := fmt.Errorf("error creating volume: %s pool %s error: %v", name, poolName, err)
		zlog.Error().Msg(e.Error())
		return nil, status.Errorf(codes.Internal, "%s", e.Error())
	}
	vi := iscsi.cs.getCSIResponse(volumeResp, req)

	// check volume id format
	volID, err := strconv.Atoi(vi.VolumeId)
	if err != nil {
		zlog.Error().Msg(err.Error())
		return nil, status.Errorf(codes.Internal, "error getting volume id")
	}

	// confirm volume creation
	var vol *api.Volume
	var counter int
	vol, err = iscsi.cs.Api.GetVolume(volID)
	if err != nil {
		zlog.Error().Msg(err.Error())
		return nil, err
	}
	for vol == nil && counter < 100 {
		time.Sleep(3 * time.Millisecond)
		vol, err = iscsi.cs.Api.GetVolume(volID)
		if err != nil {
			zlog.Error().Msg(err.Error())
			return nil, err
		}
		counter = counter + 1
	}
	if vol == nil {
		return nil, status.Errorf(codes.Internal, "failed to create volume name: %s volume not retrieved for id: %d", name, volID)
	}

	// Prepare response struct
	copyRequestParameters(params, vi.VolumeContext)
	csiResp := &csi.CreateVolumeResponse{
		Volume: vi,
	}

	// attach metadata to volume object
	metadata := map[string]interface{}{
		"host.k8s.pvname": vol.Name,
	}
	_, err = iscsi.cs.IboxApi.PutMetadata(vol.ID, metadata)
	if err != nil {
		e := fmt.Errorf("failed to attach metadata for volume : %s, err: %v", name, err)
		zlog.Error().Msg(e.Error())
		return nil, status.Errorf(codes.Internal, "%s", e.Error())
	}

	zlog.Debug().Msgf("successfully created volume with name %s and ID %d", name, volID)
	return csiResp, err
}

func (iscsi *iscsistorage) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (csiResp *csi.DeleteVolumeResponse, err error) {
	volproto := iscsi.cs.VolProto
	zlog.Debug().Msgf("DeleteVolume volumeID %s volproto %+v", req.GetVolumeId(), volproto)
	err = iscsi.ValidateDeleteVolume(volproto.VolumeID)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return &csi.DeleteVolumeResponse{}, nil
		} else {
			zlog.Error().Msg(err.Error())
			return nil, status.Errorf(codes.Internal, "failed to delete volume: %s", err.Error())
		}
	}
	zlog.Debug().Msgf("successfully deleted volume with ID %s", req.GetVolumeId())
	return &csi.DeleteVolumeResponse{}, nil
}

func (iscsi *iscsistorage) createVolumeFromContentSource(req *csi.CreateVolumeRequest, name string, sizeInBytes int64, storagePool string) (*csi.CreateVolumeResponse, error) {
	var msg string

	volumecontent := req.GetVolumeContentSource()
	var volumeContentID string
	var restoreType string
	if volumecontent.GetSnapshot() != nil {
		restoreType = "Snapshot"
		volumeContentID = volumecontent.GetSnapshot().GetSnapshotId()
	} else if volumecontent.GetVolume() != nil {
		restoreType = "Volume"
		volumeContentID = volumecontent.GetVolume().GetVolumeId()
	}

	zlog.Debug().Msgf("createVolumeFromContentSource source ID: %s type: %s size: %d B", volumeContentID, restoreType, sizeInBytes)

	// Lookup the snapshot source volume.
	volproto, err := ValidateVolumeID(volumeContentID)
	if err != nil {
		e := fmt.Errorf("failed to validate storage type restoreType: %s source id: %s, err: %v", restoreType, volumeContentID, err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.NotFound, e.Error())
	}

	srcVol, err := iscsi.cs.Api.GetVolume(volproto.VolumeID)
	if err != nil {
		e := fmt.Errorf("error GetVolume id: %d restoreType: %s error: %v", volproto.VolumeID, restoreType, err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.NotFound, e.Error())
	}

	// Validate the size is the same.
	if int64(srcVol.Size) != sizeInBytes {
		msg := fmt.Sprintf("%s %s has incompatible size. size is %d bytes with requested size %d bytes", restoreType, volumeContentID, srcVol.Size, sizeInBytes)
		zlog.Error().Msg(msg)
		return nil, status.Errorf(codes.InvalidArgument, "%s", msg)
	}

	params := req.GetParameters()

	// Check the storagePool is the same.
	pool, err := iscsi.cs.IboxApi.GetPoolByName(storagePool)
	if err != nil {
		e := fmt.Errorf("error GetStoragePoolIDByName name: %s error: %v", storagePool, err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	if pool.ID != srcVol.PoolId {
		msg = fmt.Sprintf("volume storage pool is different than the requested storage pool %s", storagePool)
		zlog.Error().Msg(msg)
		return nil, status.Error(codes.InvalidArgument, msg)
	}

	// Parse ssd enabled flag
	ssd := params[common.SC_SSD_ENABLED]
	if ssd == "" {
		ssd = fmt.Sprint(false)
	}
	ssdEnabled, _ := strconv.ParseBool(ssd)

	// Create snapshot descriptor
	snapshotParam := &api.VolumeSnapshot{
		ParentID:       volproto.VolumeID,
		SnapshotName:   name,
		WriteProtected: false,
		SsdEnabled:     ssdEnabled,
	}

	// Create snapshot
	snapResponse, err := iscsi.cs.Api.CreateSnapshotVolume(0, snapshotParam)
	if err != nil {
		zlog.Error().Msg(err.Error())
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Retrieve created destination volume
	volID := snapResponse.SnapShotID
	dstVol, err := iscsi.cs.Api.GetVolume(volID)
	if err != nil {
		zlog.Error().Msg(err.Error())
		return nil, status.Error(codes.Internal, msg)
	}

	// Create a volume response and return it
	csiVolume := iscsi.cs.getCSIResponse(dstVol, req)
	copyRequestParameters(params, csiVolume.VolumeContext)

	metadata := map[string]interface{}{
		"host.k8s.pvname": dstVol.Name,
	}
	_, err = iscsi.cs.IboxApi.PutMetadata(dstVol.ID, metadata)
	if err != nil {
		e := fmt.Errorf("error attach metadata for volume : %s, err: %v", dstVol.Name, err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	zlog.Debug().Msgf("from source %s with ID %d, created volume %s with ID %s in storage pool %s",
		restoreType, volproto.VolumeID, csiVolume.VolumeContext["Name"], csiVolume.VolumeId, csiVolume.VolumeContext["StoragePoolName"])
	return &csi.CreateVolumeResponse{Volume: csiVolume}, nil
}

func (iscsi *iscsistorage) ControllerModifyVolume(ctx context.Context, req *csi.ControllerModifyVolumeRequest) (resp *csi.ControllerModifyVolumeResponse, err error) {
	return nil, nil
}

func (iscsi *iscsistorage) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (resp *csi.ControllerPublishVolumeResponse, err error) {
	zlog.Debug().Msgf("ControllerPublishVolume node ID: %s volume ID: %s", req.GetNodeId(), req.GetVolumeId())

	volIdStr := req.GetVolumeId()
	volproto, err := ValidateVolumeID(volIdStr)
	if err != nil {
		e := fmt.Errorf("failed to validate storage type for volume ID: %s, err: %v", volIdStr, err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.NotFound, e.Error())
	}

	zlog.Debug().Msgf("volID: %d", volproto.VolumeID)
	v, err := iscsi.cs.Api.GetVolume(volproto.VolumeID)
	if err != nil {
		e := fmt.Errorf("failed to find volume by volume ID '%d': %v", volproto.VolumeID, err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.NotFound, e.Error())
	}

	// TODO: revisit this as part of CSIC-343
	_, err = iscsi.cs.AccessModesHelper.IsValidAccessMode(v, req)
	if err != nil {
		zlog.Error().Msg(err.Error())
		return nil, status.Error(codes.Internal, err.Error())
	}

	hostName, err := DetermineHostName(req.GetNodeId())
	if err != nil {
		return nil, err
	}

	host, err := iscsi.cs.validateHost(hostName)
	if err != nil {
		zlog.Error().Msg(err.Error())
		return nil, err
	}
	zlog.Debug().Msgf("found host name: %s id: %d ports: %v LUNs: %v", host.Name, host.ID, host.Ports, host.Luns)

	var ports string
	if len(host.Ports) > 0 {
		for _, port := range host.Ports {
			if port.PortType == "ISCSI" {
				ports = ports + "," + port.PortAddress
			}
		}
	}
	if ports != "" {
		ports = ports[1:]
	}

	lunList, err := iscsi.cs.IboxApi.GetAllLunByHost(host.ID)
	if err != nil {
		e := fmt.Errorf("failed to GetAllLunByHost() for host: %s, error: %v", hostName, err)
		zlog.Error().Msg(e.Error())
		return nil, e
	}
	zlog.Debug().Msgf("got LUNs for host: %s, LUNs: %+v", host.Name, lunList)
	for _, lun := range lunList {
		if lun.VolumeID == volproto.VolumeID {
			publishVolCtxt := map[string]string{
				LUN_PUBLISH_CONTEXT:        strconv.Itoa(lun.Lun),
				HOST_ID_PUBLISH_CONTEXT:    strconv.Itoa(host.ID),
				HOST_PORTS_PUBLISH_CONTEXT: ports,
			}
			zlog.Debug().Msgf("vol: %d already mapped to host:%s id:%d as LUN: %d at ports: %s", volproto.VolumeID, host.Name, host.ID, lun.Lun, ports)
			return &csi.ControllerPublishVolumeResponse{
				PublishContext: publishVolCtxt,
			}, nil
		}
	}

	maxVolsPerHostStr := req.GetVolumeContext()[common.SC_MAX_VOLS_PER_HOST]
	if maxVolsPerHostStr != "" {
		maxAllowedVol, err := strconv.Atoi(maxVolsPerHostStr)
		if err != nil {
			e := fmt.Errorf("invalid parameter %s error:  %v", common.SC_MAX_VOLS_PER_HOST, err)
			zlog.Error().Msg(e.Error())
			return nil, e
		}
		if maxAllowedVol < 1 {
			e := fmt.Errorf("invalid parameter %s error:  required to be greater than 0", common.SC_MAX_VOLS_PER_HOST)
			zlog.Error().Msg(e.Error())
			return nil, e
		}
		zlog.Debug().Msgf("host can have maximum %d volume mapped", maxAllowedVol)
		zlog.Debug().Msgf("host %s id: %d has %d volumes mapped", host.Name, host.ID, len(lunList))
		if len(lunList) >= maxAllowedVol {
			e := fmt.Errorf("unable to publish volume on host %s, as maximum allowed volume per host is (%d), limit reached", host.Name, maxAllowedVol)
			zlog.Error().Msg(e.Error())
			return nil, status.Error(codes.ResourceExhausted, e.Error())
		}
	}

	// map volume to host
	zlog.Debug().Msgf("mapping volume %d to host %s", volproto.VolumeID, host.Name)
	luninfo, err := iscsi.cs.mapVolumeTohost(volproto.VolumeID, host.ID)
	if err != nil {
		e := fmt.Errorf("failed to map volume to host with error %v", err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	publishVolCtxt := map[string]string{
		LUN_PUBLISH_CONTEXT:             strconv.Itoa(luninfo.Lun),
		HOST_ID_PUBLISH_CONTEXT:         strconv.Itoa(host.ID),
		HOST_PORTS_PUBLISH_CONTEXT:      ports,
		SECURITY_METHOD_PUBLISH_CONTEXT: host.SecurityMethod,
	}
	zlog.Debug().Msgf("mapped volume %d, publish context: %v", volproto.VolumeID, publishVolCtxt)

	zlog.Debug().Msgf("ControllerPublishVolume completed node ID: %s volume ID: %s", req.GetNodeId(), req.GetVolumeId())
	return &csi.ControllerPublishVolumeResponse{
		PublishContext: publishVolCtxt,
	}, nil
}

func (iscsi *iscsistorage) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (resp *csi.ControllerUnpublishVolumeResponse, err error) {
	zlog.Debug().Msgf("ControllerUnpublishVolume volproto %+v node ID: %s volume ID: %s", iscsi.cs.VolProto, req.GetNodeId(), req.GetVolumeId())

	host := iscsi.cs.VolProto.Host
	zlog.Debug().Msgf("unmapping host's luns: host id: %d, name: %s lun count %d", host.ID, host.Name, len(host.Luns))
	if len(host.Luns) > 0 {
		zlog.Debug().Msgf("unmap volume %d from host %d", iscsi.cs.VolProto.VolumeID, host.ID)
		err = iscsi.cs.unmapVolumeFromHost(host.ID, int(iscsi.cs.VolProto.VolumeID))
		if err != nil {
			e := fmt.Errorf("failed to unmap volume with ID %d from host with ID %d. Error: %v", iscsi.cs.VolProto.VolumeID, host.ID, err)
			zlog.Err(e)
			return nil, status.Error(codes.Internal, e.Error())
		}
	}
	if len(host.Luns) < 2 {
		err = hostCleanup(iscsi.cs.IboxApi, host.ID, host.Name)
		if err != nil {
			e := fmt.Errorf("ControllerUnpublishVolume: failed to perform hostCleanup for host ID %d. Error: %s", host.ID, err.Error())
			zlog.Err(e)
			return nil, status.Error(codes.Internal, e.Error())
		}
	}

	zlog.Debug().Msgf("ControllerUnpublishVolume completed with node ID %s and volume ID %s", req.GetNodeId(), req.GetVolumeId())
	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

func (iscsi *iscsistorage) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (resp *csi.ValidateVolumeCapabilitiesResponse, err error) {
	zlog.Error().Msgf("should not be called, implemented in controller.go")
	return
}

func (iscsi *iscsistorage) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (resp *csi.ListVolumesResponse, err error) {
	return &csi.ListVolumesResponse{}, nil
}

func (iscsi *iscsistorage) ListSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (resp *csi.ListSnapshotsResponse, err error) {
	return &csi.ListSnapshotsResponse{}, nil
}

func (iscsi *iscsistorage) GetCapacity(ctx context.Context, req *csi.GetCapacityRequest) (resp *csi.GetCapacityResponse, err error) {
	return &csi.GetCapacityResponse{}, nil
}

func (iscsi *iscsistorage) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (resp *csi.ControllerGetCapabilitiesResponse, err error) {
	return &csi.ControllerGetCapabilitiesResponse{}, nil
}

func (iscsi *iscsistorage) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (resp *csi.CreateSnapshotResponse, err error) {
	var snapshotID string
	snapshotName := req.GetName()
	zlog.Debug().Msgf("CreateSnapshot called to create snapshot named %s from source volume ID %s", snapshotName, req.GetSourceVolumeId())

	volumeSnapshot, err := iscsi.cs.Api.GetVolumeByName(snapshotName)
	if err != nil {
		zlog.Debug().Msgf("Snapshot with name %s not found", snapshotName)
	} else if volumeSnapshot.ParentId == int(iscsi.cs.VolProto.VolumeID) {
		snapshotID = strconv.Itoa(volumeSnapshot.ID) + "$$" + iscsi.cs.VolProto.StorageType
		return &csi.CreateSnapshotResponse{
			Snapshot: &csi.Snapshot{
				SizeBytes:      volumeSnapshot.Size,
				SnapshotId:     snapshotID,
				SourceVolumeId: req.GetSourceVolumeId(),
				CreationTime:   timestamppb.Now(),
				ReadyToUse:     true,
			},
		}, nil
	} else {
		e := fmt.Errorf("snapshot named %s with ID %d exists. Different source volume with ID %d requested",
			snapshotName, volumeSnapshot.ParentId, iscsi.cs.VolProto.VolumeID)
		zlog.Err(e)
		return nil, status.Error(codes.AlreadyExists, e.Error())
	}

	parentVolume, err := iscsi.cs.Api.GetVolume(int(iscsi.cs.VolProto.VolumeID))
	if err != nil {
		e := fmt.Errorf("failed to get parent volume for snapshot - volume id %d, err: %v", iscsi.cs.VolProto.VolumeID, err)
		zlog.Err(e)
		return nil, status.Error(codes.NotFound, e.Error())
	}

	snapshotParam := &api.VolumeSnapshot{
		ParentID:       int(iscsi.cs.VolProto.VolumeID),
		SnapshotName:   snapshotName,
		WriteProtected: true,
		SsdEnabled:     parentVolume.SsdEnabled,
	}

	lockExpiresAtParameter := req.Parameters[common.LOCK_EXPIRES_AT_PARAMETER]
	var lockExpiresAt int64
	if lockExpiresAtParameter != "" {
		ntpStatus, err := iscsi.cs.Api.GetNtpStatus()
		if err != nil {
			zlog.Error().Msgf("failed to get ntp status error %v", err)
			return nil, err
		}
		lockExpiresAt, err = validateSnapshotLockingParameter(ntpStatus[0].LastProbeTimestamp, lockExpiresAtParameter)
		if err != nil {
			zlog.Error().Msgf("failed to create snapshot %s error %v, invalid lock_expires_at parameter ", snapshotName, err)
			return nil, err
		}
		zlog.Debug().Msgf("snapshot param has a lock_expires_at of %s int value %d, start time on ibox is %d", lockExpiresAtParameter, lockExpiresAt, ntpStatus[0].LastProbeTimestamp)
	}

	snapshot, err := iscsi.cs.Api.CreateSnapshotVolume(lockExpiresAt, snapshotParam)
	if err != nil {
		zlog.Error().Msgf("Failed to create snapshot %s error %v", snapshotName, err)
		return nil, err
	}

	snapshotID = strconv.Itoa(snapshot.SnapShotID) + "$$" + iscsi.cs.VolProto.StorageType
	csiSnapshot := &csi.Snapshot{
		SnapshotId:     snapshotID,
		SourceVolumeId: req.GetSourceVolumeId(),
		ReadyToUse:     true,
		CreationTime:   timestamppb.Now(),
		SizeBytes:      snapshot.Size,
	}
	zlog.Debug().Msgf("CreateFileSystemSnapshot resp: %v", csiSnapshot)
	snapshotResp := &csi.CreateSnapshotResponse{Snapshot: csiSnapshot}

	zlog.Debug().Msgf("CreateSnapshot successfully created snapshot named %s from source volume ID %s", snapshotName, req.GetSourceVolumeId())
	return snapshotResp, nil
}

func (iscsi *iscsistorage) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (resp *csi.DeleteSnapshotResponse, err error) {

	snapshotID, _ := strconv.Atoi(req.GetSnapshotId())
	zlog.Debug().Msgf("DeleteSnapshot to delete snapshot with ID %d", snapshotID)

	err = iscsi.ValidateDeleteVolume(snapshotID)
	if err != nil {
		if status.Code(err) == codes.Aborted {
			return nil, err
		}

		if status.Code(err) == codes.NotFound {
			zlog.Debug().Msgf("snapshot with ID %d not found", snapshotID)
			return &csi.DeleteSnapshotResponse{}, nil
		}

		e := fmt.Errorf("failed to delete snapshot with ID %d", snapshotID)
		zlog.Err(e)
		return nil, status.Error(codes.Internal, e.Error())
	}
	zlog.Debug().Msgf("DeleteSnapshot successfully deleted snapshot with ID %d", snapshotID)
	return &csi.DeleteSnapshotResponse{}, nil
}

func (iscsi *iscsistorage) ValidateDeleteVolume(volumeID int) (err error) {

	zlog.Debug().Msgf("ValidateDeleteVolume called (also deletes volume) with ID %d", volumeID)

	vol, err := iscsi.cs.Api.GetVolume(volumeID)
	if err != nil {
		if strings.Contains(err.Error(), "VOLUME_NOT_FOUND") {
			zlog.Debug().Msgf("volume: %d is already deleted", volumeID)
			return status.Errorf(codes.NotFound, "volume not found")
		}
		msg := fmt.Sprintf("failed to get volume: %d, err: %s", volumeID, err.Error())
		zlog.Error().Msg(msg)
		return status.Error(codes.Internal, msg)
	}

	// this applies for when we are evaluating a snapshot volume
	if vol.LockState == common.LOCKED_STATE {
		return status.Errorf(codes.Aborted, "volume %d was locked, can not delete till expire date is reached at %s", volumeID, time.UnixMilli(vol.LockExpiresAt))
	}

	childVolumes, err := iscsi.cs.Api.GetVolumeSnapshotByParentID(vol.ID)
	if err != nil {
		zlog.Err(err)
		return err
	}
	if len(*childVolumes) > 0 {
		metadata := map[string]interface{}{
			TOBEDELETED: true,
		}
		_, err = iscsi.cs.IboxApi.PutMetadata(vol.ID, metadata)
		if err != nil {
			e := fmt.Errorf("failed to update host.k8s.to_be_deleted for volume %s error: %v", vol.Name, err)
			zlog.Err(e)
			return e
		}
		zlog.Debug().Msgf("ValidateDeleteVolume found volume with ID %d has children volumes. Set metadata TOBEDELETED to 'true'. Deferring deletion.", volumeID)
		return
	}
	zlog.Debug().Msgf("deleting volume named %s with ID %d", vol.Name, vol.ID)
	_, err = iscsi.cs.IboxApi.DeleteMetadata(vol.ID)
	if err != nil {
		msg := fmt.Sprintf("Error deleting metadata for volume named %s with ID %d: %s", vol.Name, vol.ID, err.Error())
		zlog.Error().Msg(msg)
		return status.Error(codes.Internal, msg)
	}

	if err = iscsi.cs.Api.DeleteVolume(vol.ID); err != nil {
		msg := fmt.Sprintf("Error deleting volume named %s with ID %d: %s", vol.Name, vol.ID, err.Error())
		zlog.Error().Msg(msg)
		return status.Error(codes.Internal, msg)
	}
	zlog.Debug().Msgf("deleted volume named %s with ID %d", vol.Name, vol.ID)

	if vol.ParentId != 0 {
		zlog.Debug().Msgf("checking if parent volume with ID %d of volume named %s, with ID %d, can be deleted", vol.ParentId, vol.Name, vol.ID)
		var metadata []iboxapi.GetMetadataResult
		metadata, err = iscsi.cs.IboxApi.GetMetadata(vol.ParentId)
		if err != nil {
			zlog.Err(err)
			return err
		}
		var toBeDeleted bool
		for _, m := range metadata {
			if m.Key == api.TOBEDELETED {
				toBeDeleted = true
			}
		}
		if toBeDeleted {
			zlog.Debug().Msgf("ValidateDeleteVolume recursively called for parent. Volume ID: %d. Parent volume ID: %d", vol.ID, vol.ParentId)
			// Recursion
			err = iscsi.ValidateDeleteVolume(vol.ParentId)
			if err != nil {
				zlog.Err(err)
				return err
			}
		}
	}
	return nil
}

func (iscsi *iscsistorage) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (resp *csi.ControllerExpandVolumeResponse, err error) {

	volumeID, err := strconv.Atoi(req.GetVolumeId())
	if err != nil {
		zlog.Error().Msgf("invalid volume ID %v", err)
		return nil, err
	}

	capacity := int64(req.GetCapacityRange().GetRequiredBytes())
	if capacity < gib {
		capacity = gib
		zlog.Warn().Msgf("volume minimum capacity should be greater 1 GB")
	}

	// Expand volume size
	var volume api.Volume
	volume.Size = capacity
	_, err = iscsi.cs.Api.UpdateVolume(volumeID, volume)
	if err != nil {
		zlog.Error().Msgf("failed to update file system %v", err)
		return nil, err
	}
	zlog.Debug().Msgf("volume with ID %d size updated successfully", volumeID)
	return &csi.ControllerExpandVolumeResponse{
		CapacityBytes:         capacity,
		NodeExpansionRequired: true,
	}, nil
}

func (st *iscsistorage) ControllerGetVolume(
	_ context.Context, _ *csi.ControllerGetVolumeRequest,
) (*csi.ControllerGetVolumeResponse, error) {
	// Infinidat does not support ControllerGetVolume
	return nil, status.Error(codes.Unimplemented, "")
}
