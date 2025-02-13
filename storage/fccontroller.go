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
	"errors"
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

func (fc *fcstorage) ValidateStorageClass(params map[string]string) error {
	requiredFCParams := map[string]string{
		common.SC_POOL_NAME: `\A.*\z`,
	}
	optionalFCParams := map[string]string{
		common.SC_PROVISION_TYPE: `(?i)\A(THICK|THIN)\z`,
	}

	// validate required parameters
	err := ValidateRequiredOptionalSCParameters(requiredFCParams, optionalFCParams, params)
	if err != nil {
		zlog.Error().Msgf("error %s", err.Error())
		return status.Error(codes.InvalidArgument, err.Error())
	}
	return nil
}

func (fc *fcstorage) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	params := req.GetParameters()
	fc.configmap = params
	zlog.Debug().Msgf("requested volume parameters are %v", params)

	gid := params[common.SC_GID]
	uid := params[common.SC_UID]
	unix_permissions := params[common.SC_UNIX_PERMISSIONS]
	zlog.Debug().Msgf("storageClass request parameters uid %s gid %s unix_permissions %s", gid, uid, unix_permissions)

	zlog.Debug().Msgf("requested size in bytes is %d ", fc.capacity)

	// Volume name to be created - already verified in controller.go
	name := req.GetName()

	poolName := params[common.SC_POOL_NAME]

	targetVol, err := fc.cs.IboxApi.GetVolumeByName(name)
	if err != nil {
		re, ok := err.(*iboxapi.IboxAPIError)
		if ok && re.Code == iboxapi.IBOXAPI_NOT_FOUND_ERROR {
			zlog.Debug().Msgf("volume with name %s not found, proceeding to create", name)
		} else {
			zlog.Error().Msgf("CreateVolume - GetVolumeByName %s - error: %s", name, err.Error())
			return nil, status.Errorf(codes.Internal, "%s", fmt.Sprintf("CreateVolume failed: %v", err))
		}
	}

	if targetVol != nil {
		zlog.Debug().Msgf("volume: %s found, size: %d requested: %d", name, targetVol.Size, fc.capacity)
		if targetVol.Size == fc.capacity {
			existingVolumeInfo := fc.cs.getCSIResponse(targetVol, req)
			copyRequestParameters(params, existingVolumeInfo.VolumeContext)
			return &csi.CreateVolumeResponse{
				Volume: existingVolumeInfo,
			}, nil
		}
		err = status.Errorf(codes.AlreadyExists, "error CreateVolume: volume exists but has different size")
		zlog.Error().Msgf("CreateVolume - volume: %s already exists with a different size, %v", name, err)
		return nil, err
	}

	// Volume content source support volume and snapshots
	contentSource := req.GetVolumeContentSource()
	if contentSource != nil {
		return fc.createVolumeFromVolumeContent(req, name, fc.capacity, poolName)
	}

	volType, provided := params[common.SC_PROVISION_TYPE]
	if !provided {
		volType = common.SC_THIN_PROVISION_TYPE
	}

	volumeParam := &api.VolumeParam{
		Name:          name,
		VolumeSize:    fc.capacity,
		ProvisionType: volType,
	}

	volumeParam.SsdEnabled, err = determineSSDValue(params[common.SC_SSD_ENABLED], poolName, fc.cs.IboxApi)
	if err != nil {
		zlog.Error().Msgf("CreateVolume - determineSSDValue - error: %s", err.Error())
		return nil, status.Errorf(codes.Internal, "error when creating volume %s storagepool %s, err: %s", name, poolName, err.Error())
	}

	pool, err := fc.cs.IboxApi.GetPoolByName(poolName)
	if err != nil {
		zlog.Error().Msgf("CreateVolume - GetPoolByName name %s- error: %s", poolName, err.Error())
		return nil, status.Errorf(codes.Internal, "error when creating volume %s storagepool %s, err: %s", name, poolName, err.Error())
	}

	createVolumeRequest := iboxapi.CreateVolumeRequest{
		Name:          volumeParam.Name,
		SsdEnabled:    volumeParam.SsdEnabled,
		ProvisionType: volumeParam.ProvisionType,
		PoolId:        pool.ID,
		VolumeSize:    volumeParam.VolumeSize,
	}

	volumeResp, err := fc.cs.IboxApi.CreateVolume(createVolumeRequest)
	if err != nil {
		zlog.Error().Msgf("CreateVolume - CreateVolume - error: %s", err.Error())
		return nil, status.Errorf(codes.Internal, "error when creating volume %s storagepool %s, err: %s", name, poolName, err.Error())
	}

	attributes := map[string]string{
		"ID":              strconv.Itoa(volumeResp.ID),
		"Name":            volumeResp.Name,
		"StoragePoolID":   strconv.Itoa(volumeResp.PoolId),
		"StoragePoolName": volumeResp.PoolName,
		"CreationTime":    time.Unix(int64(volumeResp.CreatedAt), 0).String(),
		"targetWWNs":      req.GetParameters()["targetWWNs"],
	}
	vi := &csi.Volume{
		VolumeId:      strconv.Itoa(volumeResp.ID),
		CapacityBytes: volumeResp.Size,
		VolumeContext: attributes,
		ContentSource: req.GetVolumeContentSource(),
	}

	// confirm volume creation
	var vol *iboxapi.Volume
	vol, err = fc.cs.IboxApi.GetVolume(volumeResp.ID)
	if err != nil {
		zlog.Error().Msgf("CreateVolume - GetVolume - error: %s", err.Error())
	}

	// a single test just in case there is a race condition on createVolume (doubtful)
	if vol == nil {
		time.Sleep(3 * time.Second)
		_, err = fc.cs.IboxApi.GetVolume(volumeResp.ID)
		if err != nil {
			zlog.Error().Msgf("CreateVolume - GetVolume - error: %s", err.Error())
			return nil, status.Errorf(codes.Internal, "failed to create volume name: %s volume not retrieved for id: %d", name, volumeResp.ID)
		}
	}

	// Prepare response struct
	copyRequestParameters(params, vi.VolumeContext)
	csiResp := &csi.CreateVolumeResponse{
		Volume: vi,
	}

	// attach metadata to volume object
	metadata := map[string]interface{}{
		"host.k8s.pvname": volumeResp.Name,
	}
	_, err = fc.cs.IboxApi.PutMetadata(volumeResp.ID, metadata)
	if err != nil {
		zlog.Error().Msgf("CreateVolume - PutMetadata volume %s- error: %s", name, err.Error())
		return nil, status.Errorf(codes.Internal, "failed to attach metadata")
	}

	zlog.Debug().Msgf("created volume: %s id: %d", name, volumeResp.ID)
	return csiResp, err
}

func (fc *fcstorage) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (csiResp *csi.DeleteVolumeResponse, err error) {
	zlog.Debug().Msgf("DeleteVolume")
	err = fc.ValidateDeleteVolume(fc.cs.VolProto.VolumeID)
	if err != nil {
		zlog.Error().Msgf("DeleteVolume - ValidateDeleteVolume volume ID %d- error: %s", fc.cs.VolProto.VolumeID, err.Error())
		return nil, status.Errorf(codes.Internal, "error deleting volume : %s", err.Error())
	}
	return &csi.DeleteVolumeResponse{}, nil
}

func (fc *fcstorage) createVolumeFromVolumeContent(req *csi.CreateVolumeRequest, name string, sizeInKbytes int64, storagePool string) (*csi.CreateVolumeResponse, error) {
	var err error

	volumecontent := req.GetVolumeContentSource()
	var volumeContentID string
	var restoreType string
	if volumecontent.GetSnapshot() != nil {
		restoreType = RESTORE_TYPE_SNAPSHOT
		volumeContentID = volumecontent.GetSnapshot().GetSnapshotId()
	} else if volumecontent.GetVolume() != nil {
		volumeContentID = volumecontent.GetVolume().GetVolumeId()
		restoreType = RESTORE_TYPE_VOLUME
	}

	// Validate the source content id
	volproto, err := ValidateVolumeID(volumeContentID)
	if err != nil {
		zlog.Error().Msgf("failed to validate storage type for source id: %s, err: %v", volumeContentID, err)
		return nil, status.Errorf(codes.NotFound, restoreType+" not found: %s", volumeContentID)
	}

	srcVol, err := fc.cs.IboxApi.GetVolume(volproto.VolumeID)
	if err != nil {
		zlog.Error().Msgf("error %s", err.Error())
		return nil, status.Errorf(codes.NotFound, restoreType+" not found: %d", volproto.VolumeID)
	}

	// Validate the size is the same.
	if int64(srcVol.Size) != sizeInKbytes {
		return nil, status.Errorf(codes.InvalidArgument,
			restoreType+" %s has incompatible size %d kbytes with requested %d kbytes",
			volumeContentID, srcVol.Size, sizeInKbytes)
	}

	// Validate the storagePool is the same.
	pool, err := fc.cs.IboxApi.GetPoolByName(storagePool)
	if err != nil {
		zlog.Error().Msgf("error %s", err.Error())
		return nil, status.Errorf(codes.Internal, "error getting pool [%s]", storagePool)
	}
	if pool.ID != srcVol.PoolId {
		return nil, status.Errorf(codes.InvalidArgument,
			"volume storage pool is different than requested storage pool %s", storagePool)
	}
	ssd := req.GetParameters()[common.SC_SSD_ENABLED]
	if ssd == "" {
		ssd = fmt.Sprint(false)
	}
	ssdEnabled, _ := strconv.ParseBool(ssd)
	snapshotParam := &api.VolumeSnapshot{
		ParentID:       volproto.VolumeID,
		SnapshotName:   name,
		WriteProtected: false,
		SsdEnabled:     ssdEnabled,
	}
	// Create snapshot
	snapResponse, err := fc.cs.Api.CreateSnapshotVolume(0, snapshotParam)
	if err != nil {
		zlog.Error().Msgf("error %s", err.Error())
		return nil, status.Errorf(codes.Internal, "error create snapshot: %s", err.Error())
	}

	// Retrieve created destination volume
	volID := snapResponse.SnapShotID
	dstVol, err := fc.cs.IboxApi.GetVolume(volID)
	if err != nil {
		zlog.Error().Msgf("error %s", err.Error())
		return nil, status.Errorf(codes.Internal, "error get volume: %d", volID)
	}

	// Create a volume response and return it
	csiVolume := fc.cs.getCSIResponse(dstVol, req)
	copyRequestParameters(req.GetParameters(), csiVolume.VolumeContext)

	metadata := map[string]interface{}{
		"host.k8s.pvname": dstVol.Name,
	}
	_, err = fc.cs.IboxApi.PutMetadata(dstVol.ID, metadata)
	if err != nil {
		zlog.Error().Msgf("failed to attach metadata for volume: %s, err: %v", dstVol.Name, err)
		return nil, status.Errorf(codes.Internal, "error attaching metadata to volume: %s, err: %v", dstVol.Name, err)
	}
	zlog.Error().Msgf("Volume (from snap) %s (%s) storage pool %s",
		csiVolume.VolumeContext["Name"], csiVolume.VolumeId, csiVolume.VolumeContext["StoragePoolName"])
	return &csi.CreateVolumeResponse{Volume: csiVolume}, nil
}

func (fc *fcstorage) ControllerModifyVolume(ctx context.Context, req *csi.ControllerModifyVolumeRequest) (resp *csi.ControllerModifyVolumeResponse, err error) {
	return nil, nil
}

func (fc *fcstorage) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (resp *csi.ControllerPublishVolumeResponse, err error) {
	zlog.Debug().Msgf("ControllerPublishVolume nodeID %s and volumeId %s", req.GetNodeId(), req.GetVolumeId())
	volproto, err := ValidateVolumeID(req.GetVolumeId())
	if err != nil {
		zlog.Error().Msgf("ControllerPublishVolume - ValidateVolumeID - error: %s", err.Error())
		return nil, errors.New("error getting volume ID")
	}

	hostName, err := DetermineHostName(req.GetNodeId())
	if err != nil {
		zlog.Error().Msgf("ControllerPublishVolume - DetermineHostName - error: %s", err.Error())
		return nil, err
	}

	host, err := fc.cs.validateHost(hostName)
	if err != nil {
		zlog.Error().Msgf("ControllerPublishVolume - validateHost hostname %s- error: %s", hostName, err.Error())
		return nil, status.Error(codes.Internal, err.Error())
	}

	v, err := fc.cs.IboxApi.GetVolume(volproto.VolumeID)
	if err != nil {
		zlog.Error().Msgf("ControllerPublishVolume - GetVolume volume ID '%s' - error: %v", req.GetVolumeId(), err.Error())
		return nil, errors.New("error getting volume by id")
	}

	_, err = fc.cs.AccessModesHelper.IsValidAccessMode(v, req)
	if err != nil {
		zlog.Error().Msgf("ControllerPublishVolume - IsValidAccessMode - error: %s", err.Error())
		return nil, status.Error(codes.Internal, err.Error())
	}

	lunList, err := fc.cs.IboxApi.GetAllLunByHost(host.ID)
	if err != nil {
		zlog.Error().Msgf("ControllerPublishVolume - GetAllLunByHost host ID %d- error: %s", host.ID, err.Error())
		return nil, err
	}
	var ports string
	if len(host.Ports) > 0 {
		for _, port := range host.Ports {
			if port.Type == "FC" {
				ports = ports + "," + port.Address
			}
		}
	}
	if ports != "" {
		ports = ports[1:]
	}
	zlog.Debug().Msgf("ports=[%v]", ports)
	for _, lun := range lunList {
		if lun.VolumeID == volproto.VolumeID {
			volCtx := map[string]string{
				LUN_PUBLISH_CONTEXT:        strconv.Itoa(lun.Lun),
				HOST_ID_PUBLISH_CONTEXT:    strconv.Itoa(host.ID),
				HOST_PORTS_PUBLISH_CONTEXT: ports,
			}
			zlog.Debug().Msgf("volumeID %d already mapped to host %s", lun.VolumeID, host.Name)
			return &csi.ControllerPublishVolumeResponse{
				PublishContext: volCtx,
			}, nil
		}
	}

	// the max_vols_per_host storageclass parameter is not mandatory
	maxAllowedVolString := req.GetVolumeContext()[common.SC_MAX_VOLS_PER_HOST]
	if maxAllowedVolString != "" {
		maxAllowedVol, err := strconv.Atoi(maxAllowedVolString)
		if err != nil {
			zlog.Error().Msgf("ControllerPublishVolume - invalid parameter %s error:  %v", common.SC_MAX_VOLS_PER_HOST, err)
			return nil, err
		}
		if maxAllowedVol < 1 {
			e := fmt.Errorf("ControllerPublishVolume - invalid parameter %s error:  required to be greater than 0", common.SC_MAX_VOLS_PER_HOST)
			zlog.Error().Msgf("error %s", e.Error())
			return nil, e
		}
		zlog.Debug().Msgf("host can have maximum %d volume mapped", maxAllowedVol)
		zlog.Debug().Msgf("host %s has %d volume mapped", host.Name, len(lunList))
		if len(lunList) >= maxAllowedVol {
			zlog.Error().Msgf("ControllerPublishVolume - unable to publish volume on host %s, maximum allowed volume per host is (%d), limit reached", host.Name, maxAllowedVol)
			return nil, status.Error(codes.ResourceExhausted, "Unable to publish volume as max allowed volume (per host) limit reached")
		}
	}
	// map volume to host
	zlog.Debug().Msgf("mapping volume %d to host %s", volproto.VolumeID, host.Name)
	luninfo, err := fc.cs.mapVolumeTohost(volproto.VolumeID, host.ID)
	if err != nil {
		zlog.Error().Msgf("ControllerPublishVolume - mapVolumeToHost - error: %s", err.Error())
		return nil, status.Error(codes.Internal, err.Error())
	}

	volCtx := map[string]string{
		LUN_PUBLISH_CONTEXT:        strconv.Itoa(luninfo.Lun),
		HOST_ID_PUBLISH_CONTEXT:    strconv.Itoa(host.ID),
		HOST_PORTS_PUBLISH_CONTEXT: ports,
	}
	return &csi.ControllerPublishVolumeResponse{
		PublishContext: volCtx,
	}, nil
}

func (fc *fcstorage) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (resp *csi.ControllerUnpublishVolumeResponse, err error) {
	zlog.Debug().Msgf("ControllerUnpublishVolume volProto %+v nodeID %s and volumeId %s", fc.cs.VolProto, req.GetNodeId(), req.GetVolumeId())

	host := fc.cs.VolProto.Host
	if len(host.Luns) > 0 {
		zlog.Debug().Msgf("unmap volume %d from host %d", fc.cs.VolProto.VolumeID, host.ID)
		err = fc.cs.unmapVolumeFromHost(host.ID, int(fc.cs.VolProto.VolumeID))
		if err != nil {
			zlog.Error().Msgf("ControllerUnpublishVolume - unmapVolumeFromHost - error unmapping volume %d from host %d error %v", fc.cs.VolProto.VolumeID, host.ID, err)
			return nil, status.Error(codes.Internal, err.Error())
		}
	}
	if len(host.Luns) < 2 {
		err = hostCleanup(fc.cs.IboxApi, host.ID, host.Name)
		if err != nil {
			e := fmt.Errorf("ControllerUnpublishVolume: hostCleanup - error host ID %d. Error: %s", host.ID, err.Error())
			zlog.Error().Msgf("%s", e.Error())
			return nil, status.Error(codes.Internal, e.Error())
		}
	}
	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

func (fc *fcstorage) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (resp *csi.ValidateVolumeCapabilitiesResponse, err error) {
	zlog.Error().Msgf("should not be called, implemented in controller.go instead")
	return
}

func (fc *fcstorage) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (resp *csi.ListVolumesResponse, err error) {
	return &csi.ListVolumesResponse{}, nil
}

func (fc *fcstorage) ListSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (resp *csi.ListSnapshotsResponse, err error) {
	return &csi.ListSnapshotsResponse{}, nil
}

func (fc *fcstorage) GetCapacity(ctx context.Context, req *csi.GetCapacityRequest) (resp *csi.GetCapacityResponse, err error) {
	return &csi.GetCapacityResponse{}, nil
}

func (fc *fcstorage) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (resp *csi.ControllerGetCapabilitiesResponse, err error) {
	return &csi.ControllerGetCapabilitiesResponse{}, nil
}

func (fc *fcstorage) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (resp *csi.CreateSnapshotResponse, err error) {
	var snapshotID string
	snapshotName := req.GetName()
	zlog.Debug().Msgf("CreateSnapshot name %s source volume ID %s volproto %+v", snapshotName, req.GetSourceVolumeId(), fc.cs.VolProto)

	volumeSnapshot, err := fc.cs.IboxApi.GetVolumeByName(snapshotName)
	if err != nil {
		re, ok := err.(*iboxapi.IboxAPIError)
		if ok && re.Code == iboxapi.IBOXAPI_NOT_FOUND_ERROR {
			zlog.Debug().Msgf("Snapshot with given name not found : %s", snapshotName)
		} else {
			zlog.Error().Msgf("CreateSnapshot - GetVolumeByName - error: %s", err.Error())
			return nil, status.Error(codes.Internal, fmt.Sprintf("error getting volume by name %s", snapshotName))
		}
	} else if volumeSnapshot.ParentId == fc.cs.VolProto.VolumeID {
		snapshotID = strconv.Itoa(volumeSnapshot.ID) + "$$" + fc.cs.VolProto.StorageType
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
		return nil, status.Error(codes.AlreadyExists, "snapshot with already existing name and different source volume ID")
	}

	// look up the parent volume so we can get the ssd_enabled value and use that for
	// the snapshot being created next
	parentVolume, err := fc.cs.IboxApi.GetVolume(fc.cs.VolProto.VolumeID)
	if err != nil {
		e := fmt.Errorf("CreateSnapshot - GetVolume - error get parent volume when creating snapshot - volume id %d, err: %v", fc.cs.VolProto.VolumeID, err)
		zlog.Error().Msgf("%s", e.Error())
		return nil, status.Error(codes.NotFound, e.Error())
	}

	snapshotParam := &api.VolumeSnapshot{
		ParentID:       fc.cs.VolProto.VolumeID,
		SnapshotName:   snapshotName,
		WriteProtected: true,
		SsdEnabled:     parentVolume.SsdEnabled,
	}

	lockExpiresAtParameter := req.Parameters[common.LOCK_EXPIRES_AT_PARAMETER]
	var lockExpiresAt int64
	if lockExpiresAtParameter != "" {
		ntpStatus, err := fc.cs.IboxApi.GetNtpStatus()
		if err != nil {
			zlog.Error().Msgf("CreateSnapshot - GetNtpStatus - error: %s", err.Error())
			return nil, err
		}
		lockExpiresAt, err = validateSnapshotLockingParameter(ntpStatus[0].LastProbeTimestamp, lockExpiresAtParameter)
		if err != nil {
			zlog.Error().Msgf("CreateSnapshot - failed to create snapshot %s error %v, invalid lock_expires_at parameter ", snapshotName, err)
			return nil, err
		}
		zlog.Info().Msgf("snapshot param has a lock_expires_at of %s", lockExpiresAtParameter)
	}

	snapshot, err := fc.cs.Api.CreateSnapshotVolume(lockExpiresAt, snapshotParam)
	if err != nil {
		zlog.Error().Msgf("CreateSnapshot - CreateSnapshotVolume  snapshot %s error: %s", snapshotName, err.Error())
		return
	}

	snapshotID = strconv.Itoa(snapshot.SnapShotID) + "$$" + fc.cs.VolProto.StorageType
	csiSnapshot := &csi.Snapshot{
		SnapshotId:     snapshotID,
		SourceVolumeId: req.GetSourceVolumeId(),
		ReadyToUse:     true,
		CreationTime:   timestamppb.Now(),
		SizeBytes:      snapshot.Size,
	}
	zlog.Debug().Msgf("CreateFileSystemSnapshot resp: %v", csiSnapshot)
	snapshotResp := &csi.CreateSnapshotResponse{Snapshot: csiSnapshot}
	return snapshotResp, nil
}

func (fc *fcstorage) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (resp *csi.DeleteSnapshotResponse, err error) {

	snapshotID, _ := strconv.Atoi(req.GetSnapshotId())

	err = fc.ValidateDeleteVolume(snapshotID)
	if err != nil {
		zlog.Error().Msgf("DeleteSnapshot - ValidateDeleteVolume - error: %s", err.Error())
		return nil, err
	}
	return &csi.DeleteSnapshotResponse{}, nil
}

func (fc *fcstorage) ValidateDeleteVolume(volumeID int) (err error) {
	vol, err := fc.cs.IboxApi.GetVolume(volumeID)
	if err != nil {
		re, ok := err.(*iboxapi.IboxAPIError)
		if ok && re.Code == iboxapi.IBOXAPI_NOT_FOUND_ERROR {
			zlog.Debug().Msgf("volume is already deleted %d", volumeID)
			return nil
		}
		zlog.Error().Msgf("error %s", err.Error())
		return status.Errorf(codes.Internal,
			"error while validating volume status : %s",
			err.Error())
	}

	if vol.LockState == common.LOCKED_STATE {
		return status.Errorf(codes.Aborted, "volume %d was locked, can not delete till expire date %s is reached", volumeID, time.UnixMilli(vol.LockExpiresAt))
	}

	childVolumes, err := fc.cs.Api.GetVolumeSnapshotByParentID(vol.ID)
	if err != nil {
		zlog.Error().Msgf("error %s", err.Error())
	}
	if len(*childVolumes) > 0 {
		metadata := map[string]interface{}{
			TOBEDELETED: true,
		}
		_, err = fc.cs.IboxApi.PutMetadata(vol.ID, metadata)
		if err != nil {
			zlog.Error().Msgf("failed to update host.k8s.to_be_deleted for volume %s error: %v", vol.Name, err)
			err = errors.New("error while Set metadata host.k8s.to_be_deleted")
		}
		return
	}
	zlog.Debug().Msgf("Deleting volume name: %s id: %d", vol.Name, vol.ID)
	_, err = fc.cs.IboxApi.DeleteMetadata(vol.ID)
	if err != nil {
		zlog.Error().Msgf("error %s", err.Error())
		return status.Errorf(codes.Internal,
			"error removing metadata for volume: %s", err.Error())
	}
	_, err = fc.cs.IboxApi.DeleteVolume(vol.ID)
	if err != nil {
		zlog.Error().Msgf("error %s", err.Error())
		return status.Errorf(codes.Internal, "error removing volume: %s", err.Error())
	}
	if vol.ParentId != 0 {
		zlog.Debug().Msgf("checking if parent volume can be name: %s id: %d", vol.Name, vol.ID)
		var metadata []iboxapi.GetMetadataResult
		metadata, err = fc.cs.IboxApi.GetMetadata(vol.ParentId)
		if err != nil {
			zlog.Error().Msgf("error %s", err.Error())
			return err
		}
		var toBeDeleted bool
		for _, m := range metadata {
			if m.Key == api.TOBEDELETED {
				toBeDeleted = true
			}
		}
		if toBeDeleted {
			err = fc.ValidateDeleteVolume(vol.ParentId)
			if err != nil {
				zlog.Error().Msgf("error %s", err.Error())
				return
			}
		}
	}
	return
}

func (fc *fcstorage) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (resp *csi.ControllerExpandVolumeResponse, err error) {

	volumeID := fc.cs.VolProto.VolumeID
	zlog.Debug().Msgf("ControllerExpandVolume volume ID %d", volumeID)

	capacity := int64(req.GetCapacityRange().GetRequiredBytes())
	if capacity < gib {
		capacity = gib
		zlog.Warn().Msgf("Volume Minimum capacity should be greater 1 GB")
	}

	// Expand volume size
	volume := iboxapi.Volume{
		Size: capacity,
	}
	_, err = fc.cs.IboxApi.UpdateVolume(volumeID, volume)
	if err != nil {
		zlog.Error().Msgf("ControllerExpandVolume - UpdateVolume - error: %s", err.Error())
		return
	}
	zlog.Debug().Msg("Volume size updated successfully")
	return &csi.ControllerExpandVolumeResponse{
		CapacityBytes:         capacity,
		NodeExpansionRequired: true,
	}, nil
}

func (fc *fcstorage) ControllerGetVolume(
	_ context.Context, _ *csi.ControllerGetVolumeRequest,
) (*csi.ControllerGetVolumeResponse, error) {
	// Infinidat does not support ControllerGetVolume
	return nil, status.Error(codes.Unimplemented, "")
}
