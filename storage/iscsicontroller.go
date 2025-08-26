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
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (iscsi *iscsistorage) ValidateStorageClass(params map[string]string) error {
	requiredISCSIParams := map[string]string{
		common.SC_POOL_NAME:     `[a-zA-Z]+`, //match all strings except empty string or blank string
		common.SC_USE_CHAP:      `(?i)\A(none|chap|mutual_chap)\z`,
		common.SC_NETWORK_SPACE: `\A.*\z`, // TODO: could make this enforce IBOX network_space requirements, but probably not necessary
	}
	optionalISCSIParams := map[string]string{
		common.SC_PROVISION_TYPE: `(?i)\A(THICK|THIN)\z`,
		common.SC_UID:            `^\d+$`,
		common.SC_GID:            `^\d+$`,
	}

	// validate required parameters
	err := ValidateRequiredOptionalSCParameters(requiredISCSIParams, optionalISCSIParams, params)
	if err != nil {
		e := fmt.Errorf("ValidateStorageClass (iscsi) - Validate - error: %s", err.Error())
		zlog.Err(e)
		return status.Error(codes.InvalidArgument, e.Error())
	}
	return nil
}

func (iscsi *iscsistorage) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	const function = "CreateVolume"
	params := req.GetParameters()
	zlog.Debug().Msgf("%s (iscsi) volume: %s of size: %d bytes params: %v", function, req.GetName(), iscsi.capacity, params)

	// Volume name to be created - already verified earlier
	name := req.GetName()

	poolName := params[common.SC_POOL_NAME]

	targetVol, err := iscsi.cs.IboxApi.GetVolumeByName(name)
	if err != nil {
		re, ok := err.(*iboxapi.IboxAPIError)
		if ok && re.Code == iboxapi.IBOXAPI_RESOURCE_NOT_FOUND_ERROR {
			zlog.Debug().Msgf("%s (iscsi) volume with name %s not found, proceeding to create", function, name)
		} else {
			e := fmt.Errorf("%s (iscsi) - GetVolumeByName name %s - error: %s", function, name, err.Error())
			zlog.Error().Msg(e.Error())
			return nil, status.Error(codes.NotFound, e.Error())
		}
	}
	if targetVol != nil {
		zlog.Debug().Msgf("%s (iscsi) volume: %s found, size: %d requested: %d", function, name, targetVol.Size, iscsi.capacity)
		if targetVol.Size == iscsi.capacity {
			existingVolumeInfo := iscsi.cs.getCSIResponse(targetVol, req)
			copyRequestParameters(params, existingVolumeInfo.VolumeContext)
			return &csi.CreateVolumeResponse{
				Volume: existingVolumeInfo,
			}, nil
		}
		msg := fmt.Sprintf("%s (iscsi) - failed: volume %s exists but has different size", function, name)
		zlog.Error().Msg(msg)
		return nil, status.Errorf(codes.AlreadyExists, "%s", msg)
	}

	// Volume content source support volume and snapshots
	if req.GetVolumeContentSource() != nil {
		return iscsi.createVolumeFromContentSource(req, name, iscsi.capacity, poolName)
	}

	volType, provided := params[common.SC_PROVISION_TYPE]
	if !provided {
		volType = common.SC_THIN_PROVISION_TYPE
	}

	volumeParam := &api.VolumeParam{
		VolumeSize:    iscsi.capacity,
		ProvisionType: volType,
	}

	volumeParam.SsdEnabled, err = determineSSDValue(params[common.SC_SSD_ENABLED], poolName, iscsi.cs.IboxApi)
	if err != nil {
		e := status.Errorf(codes.Internal, "%s (iscsi) - determineSSDValue - error when creating volume %s storagepool %s, err: %s", function, name, poolName, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	pool, err := iscsi.cs.IboxApi.GetPoolByName(poolName)
	if err != nil {
		e := status.Errorf(codes.Internal, "%s (iscsi) - GetPoolByName - error when getting pool %s , err: %s", function, poolName, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	request := iboxapi.CreateVolumeRequest{
		Name:          name,
		PoolId:        pool.ID,
		VolumeSize:    volumeParam.VolumeSize,
		ProvisionType: volumeParam.ProvisionType,
		SsdEnabled:    volumeParam.SsdEnabled,
	}
	volumeResp, err := iscsi.cs.IboxApi.CreateVolume(request)
	if err != nil {
		e := fmt.Errorf("%s (iscsi) - api CreateVolume - error creating volume: %s pool %s error: %v", function, name, poolName, err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	vi := iscsi.cs.getCSIResponse(volumeResp, req)

	// check volume id format
	volID, err := strconv.Atoi(vi.VolumeId)
	if err != nil {
		e := fmt.Errorf("%s (iscsi) - volumeID conversion error - %s", function, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	// confirm volume creation
	var vol *iboxapi.Volume
	var counter int
	vol, err = iscsi.cs.IboxApi.GetVolume(volID)
	if err != nil {
		e := fmt.Errorf("%s (iscsi) - GetVolume - error: %s", function, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	for vol == nil && counter < 100 {
		time.Sleep(3 * time.Millisecond)
		vol, err = iscsi.cs.IboxApi.GetVolume(volID)
		if err != nil {
			e := fmt.Errorf("%s (iscsi) - GetVolume - error: %s", function, err.Error())
			zlog.Error().Msg(e.Error())
			return nil, status.Error(codes.Internal, e.Error())
		}
		counter = counter + 1
	}
	if vol == nil {
		e := fmt.Errorf("%s (iscsi) - failed to create volume name %s ID: %d", function, name, volID)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
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
		e := fmt.Errorf("%s (iscsi) - PutMetadata volume : %s, error: %s", function, name, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	zlog.Debug().Msgf("%s (iscsi) successfully created volume with name %s and ID %d", function, name, volID)
	return csiResp, err
}

func (iscsi *iscsistorage) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (csiResp *csi.DeleteVolumeResponse, err error) {
	const function = "DeleteVolume"
	volproto := iscsi.cs.VolProto
	zlog.Debug().Msgf("%s (iscsi) volumeID %s volproto %+v", function, req.GetVolumeId(), volproto)
	err = iscsi.ValidateDeleteVolume(volproto.VolumeID)
	if err != nil {
		re, ok := err.(*iboxapi.IboxAPIError)
		if ok && re.Code == iboxapi.IBOXAPI_RESOURCE_NOT_FOUND_ERROR {
			return &csi.DeleteVolumeResponse{}, nil
		} else {
			e := fmt.Errorf("%s (iscsi) - ValidateDeleteVolume - error: %s", function, err.Error())
			zlog.Error().Msg(e.Error())
			return nil, status.Error(codes.Internal, e.Error())
		}
	}
	zlog.Debug().Msgf("%s (iscsi) successfully deleted volume with ID %s", function, req.GetVolumeId())
	return &csi.DeleteVolumeResponse{}, nil
}

func (iscsi *iscsistorage) createVolumeFromContentSource(req *csi.CreateVolumeRequest, name string, sizeInBytes int64, storagePool string) (*csi.CreateVolumeResponse, error) {
	var msg, volumeContentID, restoreType string
	const function = "createVolumeFromContentSource"
	volumecontent := req.GetVolumeContentSource()
	if volumecontent.GetSnapshot() != nil {
		restoreType = RESTORE_TYPE_SNAPSHOT
		volumeContentID = volumecontent.GetSnapshot().GetSnapshotId()
	} else if volumecontent.GetVolume() != nil {
		restoreType = RESTORE_TYPE_VOLUME
		volumeContentID = volumecontent.GetVolume().GetVolumeId()
	}

	zlog.Debug().Msgf("%s (iscsi) source ID: %s type: %s size: %d B", function, volumeContentID, restoreType, sizeInBytes)

	// Lookup the snapshot source volume.
	volproto, err := ValidateVolumeID(volumeContentID)
	if err != nil {
		e := fmt.Errorf("%s (iscsi) failed to validate storage type restoreType: %s source id: %s, err: %v", function, restoreType, volumeContentID, err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.NotFound, e.Error())
	}

	srcVol, err := iscsi.cs.IboxApi.GetVolume(volproto.VolumeID)
	if err != nil {
		e := fmt.Errorf("%s (iscsi) error GetVolume id: %d restoreType: %s error: %v", function, volproto.VolumeID, restoreType, err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.NotFound, e.Error())
	}

	// Validate the size is the same.
	if int64(srcVol.Size) != sizeInBytes {
		msg := fmt.Sprintf("%s (iscsi) %s %s has incompatible size. size is %d bytes with requested size %d bytes", function, restoreType, volumeContentID, srcVol.Size, sizeInBytes)
		zlog.Error().Msg(msg)
		return nil, status.Errorf(codes.InvalidArgument, "%s", msg)
	}

	params := req.GetParameters()

	// Check the storagePool is the same.
	pool, err := iscsi.cs.IboxApi.GetPoolByName(storagePool)
	if err != nil {
		e := fmt.Errorf("%s (iscsi) error GetStoragePoolName name: %s error: %v", function, storagePool, err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	if pool.ID != srcVol.PoolId {
		msg = fmt.Sprintf("%s (iscsi) volume storage pool is different than the requested storage pool %s", function, storagePool)
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
	snapshotParam := iboxapi.CreateSnapshotVolumeRequest{
		ParentID:       volproto.VolumeID,
		SnapshotName:   name,
		WriteProtected: false,
		SsdEnabled:     ssdEnabled,
		LockExpiresAt:  0,
	}

	// Create snapshot
	snapResponse, err := iscsi.cs.IboxApi.CreateSnapshotVolume(snapshotParam)
	if err != nil {
		e := fmt.Errorf("%s (iscsi) - CreateSnapshotVolume - error: %s", function, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	// Retrieve created destination volume
	volID := snapResponse.SnapShotID
	dstVol, err := iscsi.cs.IboxApi.GetVolume(volID)
	if err != nil {
		e := fmt.Errorf("%s (iscsi) - GetVolume - error: %s", function, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	// Create a volume response and return it
	csiVolume := iscsi.cs.getCSIResponse(dstVol, req)
	copyRequestParameters(params, csiVolume.VolumeContext)

	metadata := map[string]interface{}{
		"host.k8s.pvname": dstVol.Name,
	}
	_, err = iscsi.cs.IboxApi.PutMetadata(dstVol.ID, metadata)
	if err != nil {
		e := fmt.Errorf("%s (iscsi) error attach metadata for volume : %s, err: %v", function, dstVol.Name, err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	zlog.Debug().Msgf("%s (iscsi) from source %s with ID %d, created volume %s with ID %s in storage pool %s",
		function, restoreType, volproto.VolumeID, csiVolume.VolumeContext["Name"], csiVolume.VolumeId, csiVolume.VolumeContext["StoragePoolName"])
	return &csi.CreateVolumeResponse{Volume: csiVolume}, nil
}

func (iscsi *iscsistorage) ControllerModifyVolume(ctx context.Context, req *csi.ControllerModifyVolumeRequest) (resp *csi.ControllerModifyVolumeResponse, err error) {
	return nil, nil
}

func (iscsi *iscsistorage) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (resp *csi.ControllerPublishVolumeResponse, err error) {
	const function = "ControllerPublishVolume"
	zlog.Debug().Msgf("%s (iscsi) node ID: %s volume ID: %s", function, req.GetNodeId(), req.GetVolumeId())

	volIdStr := req.GetVolumeId()
	volproto, err := ValidateVolumeID(volIdStr)
	if err != nil {
		e := fmt.Errorf("%s (iscsi) - ValidateVolumeID - failed to validate storage type for volume ID: %s, err: %v", function, volIdStr, err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.NotFound, e.Error())
	}

	zlog.Debug().Msgf("volID: %d", volproto.VolumeID)
	v, err := iscsi.cs.IboxApi.GetVolume(volproto.VolumeID)
	if err != nil {
		e := fmt.Errorf("%s (iscsi) - GetVolume volume ID '%d' - error: %s", function, volproto.VolumeID, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.NotFound, e.Error())
	}

	_, err = iscsi.cs.AccessModesHelper.IsValidAccessMode(v, req)
	if err != nil {
		e := fmt.Errorf("%s (iscsi) - IsValidAccessMode volume ID '%d' - error: %s", function, volproto.VolumeID, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	hostName, err := DetermineHostName(req.GetNodeId())
	if err != nil {
		e := fmt.Errorf("%s (iscsi) - DetermineHostName volume ID '%d' - error: %s", function, volproto.VolumeID, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, e
	}

	host, err := iscsi.cs.validateHost(hostName)
	if err != nil {
		e := fmt.Errorf("%s (iscsi) - validateHost host %s - error: %s", function, hostName, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, e
	}
	zlog.Debug().Msgf("%s (iscsi) - found host name: %s id: %d ports: %v LUNs: %v", function, host.Name, host.ID, host.Ports, host.Luns)

	var ports string
	if len(host.Ports) > 0 {
		for _, port := range host.Ports {
			if port.Type == "ISCSI" {
				ports = ports + "," + port.Address
			}
		}
	}
	if ports != "" {
		ports = ports[1:]
	}

	lunList, err := iscsi.cs.IboxApi.GetAllLunByHost(host.ID)
	if err != nil {
		e := fmt.Errorf("%s (iscsi) - GetAllLunByHost  host: %d, error: %s", function, host.ID, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, e
	}
	zlog.Debug().Msgf("%s (iscsi) got LUNs for host: %s, LUNs: %+v", function, host.Name, lunList)
	for _, lun := range lunList {
		if lun.VolumeID == volproto.VolumeID {
			publishVolCtxt := map[string]string{
				LUN_PUBLISH_CONTEXT:        strconv.Itoa(lun.Lun),
				HOST_ID_PUBLISH_CONTEXT:    strconv.Itoa(host.ID),
				HOST_PORTS_PUBLISH_CONTEXT: ports,
			}
			zlog.Debug().Msgf("%s (iscsi) vol: %d already mapped to host:%s id:%d as LUN: %d at ports: %s", function, volproto.VolumeID, host.Name, host.ID, lun.Lun, ports)
			return &csi.ControllerPublishVolumeResponse{
				PublishContext: publishVolCtxt,
			}, nil
		}
	}

	maxVolsPerHostStr := req.GetVolumeContext()[common.SC_MAX_VOLS_PER_HOST]
	if maxVolsPerHostStr != "" {
		maxAllowedVol, err := strconv.Atoi(maxVolsPerHostStr)
		if err != nil {
			e := fmt.Errorf("%s (iscsi) - invalid parameter %s error:  %v", function, common.SC_MAX_VOLS_PER_HOST, err)
			zlog.Error().Msg(e.Error())
			return nil, e
		}
		if maxAllowedVol < 1 {
			e := fmt.Errorf("%s (iscsi) - invalid parameter %s error:  required to be greater than 0", function, common.SC_MAX_VOLS_PER_HOST)
			zlog.Error().Msg(e.Error())
			return nil, e
		}
		zlog.Debug().Msgf("host %s id: %d has %d volumes mapped, maxAllowed %d", host.Name, host.ID, len(lunList), maxAllowedVol)
		if len(lunList) >= maxAllowedVol {
			e := fmt.Errorf("%s (iscsi) - unable to publish volume on host %s, as maximum allowed volume per host is (%d), limit reached", function, host.Name, maxAllowedVol)
			zlog.Error().Msg(e.Error())
			return nil, status.Error(codes.ResourceExhausted, e.Error())
		}
	}

	// map volume to host
	zlog.Debug().Msgf("%s (iscsi) - mapping volume %d to host %s", function, volproto.VolumeID, host.Name)
	luninfo, err := iscsi.cs.mapVolumeTohost(volproto.VolumeID, host.ID)
	if err != nil {
		e := fmt.Errorf("%s (iscsi) - mapVolumeToHost host ID %d - error: %s", function, host.ID, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	publishVolCtxt := map[string]string{
		LUN_PUBLISH_CONTEXT:             strconv.Itoa(luninfo.Lun),
		HOST_ID_PUBLISH_CONTEXT:         strconv.Itoa(host.ID),
		HOST_PORTS_PUBLISH_CONTEXT:      ports,
		SECURITY_METHOD_PUBLISH_CONTEXT: host.SecurityMethod,
	}
	zlog.Debug().Msgf("%s (iscsi) mapped volume %d, publish context: %v", function, volproto.VolumeID, publishVolCtxt)

	zlog.Debug().Msgf("%s (iscsi) completed node ID: %s volume ID: %s", function, req.GetNodeId(), req.GetVolumeId())
	return &csi.ControllerPublishVolumeResponse{
		PublishContext: publishVolCtxt,
	}, nil
}

func (iscsi *iscsistorage) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (resp *csi.ControllerUnpublishVolumeResponse, err error) {
	const function = "ControllerUnpublishVolume"
	zlog.Debug().Msgf("%s (iscsi) volproto %+v node ID: %s volume ID: %s", function, iscsi.cs.VolProto, req.GetNodeId(), req.GetVolumeId())

	host := iscsi.cs.VolProto.Host
	zlog.Debug().Msgf("%s (iscsi) unmapping host's luns: host id: %d, name: %s lun count %d", function, host.ID, host.Name, len(host.Luns))
	if len(host.Luns) > 0 {
		zlog.Debug().Msgf("%s (iscsi) unmap volume %d from host %d", function, iscsi.cs.VolProto.VolumeID, host.ID)
		err = iscsi.cs.unmapVolumeFromHost(host.ID, int(iscsi.cs.VolProto.VolumeID))
		if err != nil {
			e := fmt.Errorf("%s (iscsi) - unmapVolumeFromHost volume ID %d host %d- error: %s", function, iscsi.cs.VolProto.VolumeID, host.ID, err.Error())
			zlog.Err(e)
			return nil, status.Error(codes.Internal, e.Error())
		}
	}
	if len(host.Luns) < 2 {
		err = hostCleanup(iscsi.cs.IboxApi, host.ID, host.Name)
		if err != nil {
			e := fmt.Errorf("%s (iscsi) - hostCleanup host ID %d -  error: %s", function, host.ID, err.Error())
			zlog.Err(e)
			return nil, status.Error(codes.Internal, e.Error())
		}
	}

	zlog.Debug().Msgf("%s (iscsi) completed with node ID %s and volume ID %s", function, req.GetNodeId(), req.GetVolumeId())
	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

func (iscsi *iscsistorage) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (resp *csi.ValidateVolumeCapabilitiesResponse, err error) {
	zlog.Error().Msgf("ValidateVolumeCapabilities (iscsi) should not be called, implemented in controller.go")
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
	const function = "CreateSnapshot"
	snapshotName := req.GetName()
	zlog.Debug().Msgf("%s (iscsi) called to create snapshot named %s from source volume ID %s", function, snapshotName, req.GetSourceVolumeId())

	volumeSnapshot, err := iscsi.cs.IboxApi.GetVolumeByName(snapshotName)
	if err != nil {
		re, ok := err.(*iboxapi.IboxAPIError)
		if ok && re.Code == iboxapi.IBOXAPI_RESOURCE_NOT_FOUND_ERROR {
			zlog.Debug().Msgf("%s (iscsi) with name %s not found", function, snapshotName)
		} else {
			return nil, status.Error(codes.Internal, err.Error())
		}
	} else if volumeSnapshot.ParentId == iscsi.cs.VolProto.VolumeID {
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
		e := fmt.Errorf("%s (iscsi) - snapshot named %s with ID %d exists. Different source volume with ID %d requested",
			function, snapshotName, volumeSnapshot.ParentId, iscsi.cs.VolProto.VolumeID)
		zlog.Err(e)
		return nil, status.Error(codes.AlreadyExists, e.Error())
	}

	parentVolume, err := iscsi.cs.IboxApi.GetVolume(int(iscsi.cs.VolProto.VolumeID))
	if err != nil {
		e := fmt.Errorf("%s (iscsi) - GetVolume - volume id %d, error: %s", function, iscsi.cs.VolProto.VolumeID, err.Error())
		zlog.Err(e)
		return nil, status.Error(codes.NotFound, e.Error())
	}

	snapshotParam := iboxapi.CreateSnapshotVolumeRequest{
		ParentID:       iscsi.cs.VolProto.VolumeID,
		SnapshotName:   snapshotName,
		WriteProtected: true,
		SsdEnabled:     parentVolume.SsdEnabled,
	}

	lockExpiresAtParameter := req.Parameters[common.LOCK_EXPIRES_AT_PARAMETER]
	var lockExpiresAt int64
	if lockExpiresAtParameter != "" {
		ntpStatus, err := iscsi.cs.IboxApi.GetNtpStatus()
		if err != nil {
			e := fmt.Errorf("%s (iscsi) - GetNtpStatus - error: %s", function, err.Error())
			zlog.Error().Msg(e.Error())
			return nil, status.Error(codes.Internal, e.Error())
		}
		lockExpiresAt, err = validateSnapshotLockingParameter(ntpStatus[0].LastProbeTimestamp, lockExpiresAtParameter)
		if err != nil {
			e := fmt.Errorf("%s (iscsi) - validateSnapshotLockingParameter snapshot %s -  error: %s, invalid lock_expires_at parameter ", function, snapshotName, err.Error())
			zlog.Error().Msg(e.Error())
			return nil, status.Error(codes.Internal, e.Error())
		}
		zlog.Debug().Msgf("%s (iscsi) - snapshot param has a lock_expires_at of %s int value %d, start time on ibox is %d", function, lockExpiresAtParameter, lockExpiresAt, ntpStatus[0].LastProbeTimestamp)
	}

	snapshotParam.LockExpiresAt = lockExpiresAt

	snapshot, err := iscsi.cs.IboxApi.CreateSnapshotVolume(snapshotParam)
	if err != nil {
		e := fmt.Errorf("%s (iscsi) - CreateSnapshotVolume snapshot %s - error: %s", function, snapshotName, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	snapshotID = strconv.Itoa(snapshot.SnapShotID) + "$$" + iscsi.cs.VolProto.StorageType
	csiSnapshot := &csi.Snapshot{
		SnapshotId:     snapshotID,
		SourceVolumeId: req.GetSourceVolumeId(),
		ReadyToUse:     true,
		CreationTime:   timestamppb.Now(),
		SizeBytes:      snapshot.Size,
	}
	zlog.Debug().Msgf("%s (iscsi) resp: %v", function, csiSnapshot)
	snapshotResp := &csi.CreateSnapshotResponse{Snapshot: csiSnapshot}

	zlog.Debug().Msgf("%s (iscsi) successfully created snapshot named %s from source volume ID %s", function, snapshotName, req.GetSourceVolumeId())
	return snapshotResp, nil
}

func (iscsi *iscsistorage) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (resp *csi.DeleteSnapshotResponse, err error) {
	const function = "DeleteSnapshot"
	snapshotID, _ := strconv.Atoi(req.GetSnapshotId())
	zlog.Debug().Msgf("%s (iscsi) to delete snapshot with ID %d", function, snapshotID)

	err = iscsi.ValidateDeleteVolume(snapshotID)
	if err != nil {
		if status.Code(err) == codes.Aborted {
			e := fmt.Errorf("%s (iscsi) - ValidateDeleteVolume snapshot ID %d - error: %s", function, snapshotID, err.Error())
			zlog.Error().Msg(e.Error())
			return nil, status.Error(codes.Internal, e.Error())
		}

		re, ok := err.(*iboxapi.IboxAPIError)
		if ok && re.Code == iboxapi.IBOXAPI_RESOURCE_NOT_FOUND_ERROR {
			zlog.Debug().Msgf("%s (iscsi) - snapshot with ID %d not found", function, snapshotID)
			return &csi.DeleteSnapshotResponse{}, nil
		}

		e := fmt.Errorf("failed to delete snapshot with ID %d", snapshotID)
		zlog.Error().Msgf("%s (iscsi) - error deleting snapshot %s", function, e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	zlog.Debug().Msgf("%s (iscsi) successfully deleted snapshot with ID %d", function, snapshotID)
	return &csi.DeleteSnapshotResponse{}, nil
}

func (iscsi *iscsistorage) ValidateDeleteVolume(volumeID int) (err error) {
	const function = "ValidateDeleteVolume"
	zlog.Debug().Msgf("%s (iscsi) called (also deletes volume) with ID %d", function, volumeID)

	vol, err := iscsi.cs.IboxApi.GetVolume(volumeID)
	if err != nil {
		re, ok := err.(*iboxapi.IboxAPIError)
		if ok && re.Code == iboxapi.IBOXAPI_RESOURCE_NOT_FOUND_ERROR {
			return err
		}
		msg := fmt.Sprintf("%s (iscsi) - failed to get volume: %d, err: %s", function, volumeID, err.Error())
		zlog.Error().Msg(msg)
		return status.Error(codes.Internal, msg)
	}

	// this applies for when we are evaluating a snapshot volume
	if vol.LockState == common.LOCKED_STATE {
		return status.Errorf(codes.Aborted, "%s (iscsi) - volume %d was locked, can not delete till expire date is reached at %s", function, volumeID, time.UnixMilli(vol.LockExpiresAt))
	}

	childVolumes, err := iscsi.cs.IboxApi.GetVolumesByParentID(vol.ID)
	if err != nil {
		e := fmt.Errorf("%s (iscsi) - GetVolumesByParentID error :%s", function, err.Error())
		zlog.Err(e)
		return status.Error(codes.Internal, e.Error())
	}
	if len(childVolumes) > 0 {
		metadata := map[string]interface{}{
			TOBEDELETED: true,
		}
		_, err = iscsi.cs.IboxApi.PutMetadata(vol.ID, metadata)
		if err != nil {
			e := fmt.Errorf("%s (iscsi) - failed to update host.k8s.to_be_deleted for volume %s error: %v", function, vol.Name, err)
			zlog.Err(e)
			return status.Error(codes.Internal, e.Error())
		}
		zlog.Debug().Msgf("%s (iscsi) - found volume with ID %d has children volumes. Set metadata TOBEDELETED to 'true'. Deferring deletion.", function, volumeID)
		return nil
	}
	zlog.Debug().Msgf("%s (iscsi) - deleting volume named %s with ID %d", function, vol.Name, vol.ID)
	_, err = iscsi.cs.IboxApi.DeleteMetadata(vol.ID)
	if err != nil {
		msg := fmt.Sprintf("%s (iscsi) - Error deleting metadata for volume named %s with ID %d: %s", function, vol.Name, vol.ID, err.Error())
		zlog.Error().Msg(msg)
		return status.Error(codes.Internal, msg)
	}

	_, err = iscsi.cs.IboxApi.DeleteVolume(vol.ID)
	if err != nil {
		msg := fmt.Sprintf("%s (iscsi) - Error deleting volume named %s with ID %d: %s", function, vol.Name, vol.ID, err.Error())
		zlog.Error().Msg(msg)
		return status.Error(codes.Internal, msg)
	}
	zlog.Debug().Msgf("%s (iscsi) - deleted volume named %s with ID %d", function, vol.Name, vol.ID)

	if vol.ParentId != 0 {
		zlog.Debug().Msgf("%s (iscsi) - checking if parent volume with ID %d of volume named %s, with ID %d, can be deleted", function, vol.ParentId, vol.Name, vol.ID)
		var metadata []iboxapi.GetMetadataResult
		metadata, err = iscsi.cs.IboxApi.GetMetadata(vol.ParentId)
		if err != nil {
			e := fmt.Errorf("%s (iscsi) GetMetadata - error: %s", function, err.Error())
			zlog.Err(e)
			return status.Error(codes.Internal, e.Error())
		}
		var toBeDeleted bool
		for _, m := range metadata {
			if m.Key == api.TOBEDELETED {
				toBeDeleted = true
			}
		}
		if toBeDeleted {
			zlog.Debug().Msgf("%s (iscsi) recursively called for parent. Volume ID: %d. Parent volume ID: %d", function, vol.ID, vol.ParentId)
			// Recursion
			err = iscsi.ValidateDeleteVolume(vol.ParentId)
			if err != nil {
				e := fmt.Errorf("%s - recurse - error: %s", function, err.Error())
				zlog.Err(e)
				return status.Error(codes.Internal, e.Error())
			}
		}
	}
	return nil
}

func (iscsi *iscsistorage) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (resp *csi.ControllerExpandVolumeResponse, err error) {
	const function = "ControllerExpandVolume"
	volumeID := iscsi.cs.VolProto.VolumeID
	zlog.Debug().Msgf("%s (iscsi) volume ID %d", function, volumeID)

	capacity := int64(req.GetCapacityRange().GetRequiredBytes())
	if capacity < gib {
		capacity = gib
		zlog.Warn().Msgf("%s (iscsi) - volume minimum capacity should be greater 1 GB", function)
	}

	// Expand volume size
	volume := iboxapi.Volume{
		Size: capacity,
	}
	_, err = iscsi.cs.IboxApi.UpdateVolume(volumeID, volume)
	if err != nil {
		e := fmt.Errorf("%s (iscsi) - UpdateVolume - error: %s", function, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, e
	}
	zlog.Debug().Msgf("%s (iscsi) - volume with ID %d size updated successfully", function, volumeID)
	return &csi.ControllerExpandVolumeResponse{
		CapacityBytes:         capacity,
		NodeExpansionRequired: true,
	}, nil
}

func (st *iscsistorage) ControllerGetVolume(_ context.Context, _ *csi.ControllerGetVolumeRequest) (*csi.ControllerGetVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}
