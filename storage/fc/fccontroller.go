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
package fc

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/infinidat/infinibox-csi-driver/api"
	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/infinidat/infinibox-csi-driver/iboxapi"
	"github.com/infinidat/infinibox-csi-driver/log"
	storagecommon "github.com/infinidat/infinibox-csi-driver/storage/common"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type FCstorage struct {
	Capacity      int64
	CS            storagecommon.Commonservice
	ConfigMap     map[string]string
	StorageHelper storagecommon.StorageHelper
	csi.UnimplementedControllerServer
	csi.UnimplementedNodeServer
}

func NewFCstorage(capacity int64, cs storagecommon.Commonservice) (fc *FCstorage) {
	fc = &FCstorage{
		Capacity:      capacity,
		CS:            cs,
		StorageHelper: storagecommon.StorageService{},
	}
	return fc
}

var zlog = log.Get() // grab the logger for package use

func (fc *FCstorage) ValidateStorageClass(params map[string]string) error {
	const functionName = "ValidateStorageClass"

	requiredFCParams := map[string]string{
		common.StorageClassPoolName: `[a-zA-Z]+`, // match all strings except empty string or blank string
	}
	optionalFCParams := map[string]string{
		common.StorageClassProvisionType: `(?i)\A(THICK|THIN)\z`,
		common.StorageClassUID:           `^\d+$`,
		common.StorageClassGID:           `^\d+$`,
	}

	// validate required parameters
	err := storagecommon.ValidateRequiredOptionalSCParameters(requiredFCParams, optionalFCParams, params)
	if err != nil {
		e := fmt.Errorf("%s (fc) - error %s", functionName, err.Error())
		zlog.Error().Msg(e.Error())
		return status.Error(codes.InvalidArgument, e.Error())
	}
	return nil
}

func (fc *FCstorage) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	const functionName = "CreateVolume"
	params := req.GetParameters()
	fc.ConfigMap = params
	zlog.Debug().Msgf("%s (fc) - requested volume parameters are %v - requested size %d %s", functionName, params, fc.Capacity,
		storagecommon.GetHostInfo(req.GetSecrets(), fc.CS.IboxAPI))

	// Volume name to be created - already verified in controller.go
	name := req.GetName()

	poolName := params[common.StorageClassPoolName]

	targetVol, err := fc.CS.IboxAPI.GetVolumeByName(name)
	if err != nil {
		re, ok := err.(*iboxapi.APIError)
		if ok && re.Code == iboxapi.IBOXAPI_RESOURCE_NOT_FOUND_ERROR {
			zlog.Debug().Msgf("%s (fc) - volume with name %s not found, proceeding to create", functionName, name)
		} else {
			e := fmt.Errorf("%s (fc) - GetVolumeByName %s - error: %s", functionName, name, err.Error())
			zlog.Error().Msg(e.Error())
			return nil, status.Error(codes.Internal, e.Error())
		}
	}

	if targetVol != nil {
		zlog.Debug().Msgf("%s (fc) - volume: %s found, size: %d requested: %d", functionName, name, targetVol.Size, fc.Capacity)
		if targetVol.Size == fc.Capacity {
			existingVolumeInfo := fc.CS.GetCSIResponse(targetVol, req)
			storagecommon.CopyRequestParameters(params, existingVolumeInfo.VolumeContext)
			return &csi.CreateVolumeResponse{
				Volume: existingVolumeInfo,
			}, nil
		}
		err = status.Errorf(codes.AlreadyExists, "%s (fc) - volume: %s already exists with a different size, %v", functionName, name, err)
		zlog.Error().Msg(err.Error())
		return nil, err
	}

	// Volume content source support volume and snapshots
	contentSource := req.GetVolumeContentSource()
	if contentSource != nil {
		return fc.createVolumeFromVolumeContent(req, name, fc.Capacity, poolName)
	}

	volType, provided := params[common.StorageClassProvisionType]
	if !provided {
		volType = common.StorageClassThinProvision
	}

	volumeParam := &api.VolumeParam{
		Name:          name,
		VolumeSize:    fc.Capacity,
		ProvisionType: volType,
	}

	volumeParam.SSDEnabled, err = storagecommon.DetermineSSDValue(params[common.StorageClassSSDEnabled], poolName, fc.CS.IboxAPI)
	if err != nil {
		e := fmt.Sprintf("%s (fc) - determineSSDValue - error when creating volume %s storagepool %s, err: %s", functionName, name, poolName, err.Error())
		zlog.Error().Msg(e)
		return nil, status.Error(codes.Internal, e)
	}

	pool, err := fc.CS.IboxAPI.GetPoolByName(poolName)
	if err != nil {
		e := fmt.Sprintf("%s (fc) - GetPoolByName volume name: %s pool name: %s error: %s", functionName, name, poolName, err.Error())
		zlog.Error().Msg(e)
		return nil, status.Error(codes.Internal, e)
	}

	createVolumeRequest := iboxapi.CreateVolumeRequest{
		Name:          volumeParam.Name,
		SSDEnabled:    volumeParam.SSDEnabled,
		ProvisionType: volumeParam.ProvisionType,
		PoolID:        pool.ID,
		VolumeSize:    volumeParam.VolumeSize,
	}

	volumeResp, err := fc.CS.IboxAPI.CreateVolume(createVolumeRequest)
	if err != nil {
		e := fmt.Sprintf("%s (fc) - CreateVolume - error when creating volume %s storagepool %s, err: %s", functionName, name, poolName, err.Error())
		zlog.Error().Msg(e)
		return nil, status.Error(codes.Internal, e)
	}

	attributes := map[string]string{
		"ID":              strconv.Itoa(volumeResp.ID),
		"Name":            volumeResp.Name,
		"StoragePoolID":   strconv.Itoa(volumeResp.PoolID),
		"StoragePoolName": volumeResp.PoolName,
		"CreationTime":    time.Unix(volumeResp.CreatedAt, 0).String(),
		"targetWWNs":      req.GetParameters()["targetWWNs"],
	}
	newVolume := &csi.Volume{
		VolumeId:      strconv.Itoa(volumeResp.ID),
		CapacityBytes: volumeResp.Size,
		VolumeContext: attributes,
		ContentSource: req.GetVolumeContentSource(),
	}

	// confirm volume creation
	var vol *iboxapi.Volume
	vol, err = fc.CS.IboxAPI.GetVolume(volumeResp.ID)
	if err != nil {
		zlog.Error().Msgf("%s (fc) - GetVolume - error: %s", functionName, err.Error())
	}

	// a single test just in case there is a race condition on createVolume (doubtful)
	if vol == nil {
		time.Sleep(3 * time.Second)
		_, err = fc.CS.IboxAPI.GetVolume(volumeResp.ID)
		if err != nil {
			e := fmt.Sprintf("%s (fc) - GetVolume - failed to create volume name: %s volume not retrieved for id: %d", functionName, name, volumeResp.ID)
			zlog.Error().Msg(e)
			return nil, status.Error(codes.Internal, e)
		}
	}

	// Prepare response struct
	storagecommon.CopyRequestParameters(params, newVolume.VolumeContext)
	csiResp := &csi.CreateVolumeResponse{
		Volume: newVolume,
	}

	// attach metadata to volume object
	metadata := map[string]interface{}{
		"host.k8s.pvname": volumeResp.Name,
	}
	_, err = fc.CS.IboxAPI.PutMetadata(volumeResp.ID, metadata)
	if err != nil {
		e := fmt.Sprintf("%s (fc) - PutMetadata - failed to attach metadata - volume %s- error: %s", functionName, name, err.Error())
		zlog.Error().Msg(e)
		return nil, status.Error(codes.Internal, e)
	}

	zlog.Debug().Msgf("%s (fc) - created volume: %s id: %d", functionName, name, volumeResp.ID)
	return csiResp, err
}

func (fc *FCstorage) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (csiResp *csi.DeleteVolumeResponse, err error) {
	const functionName = "DeleteVolume"
	zlog.Debug().Msgf("%s (fc) called", functionName)
	err = fc.ValidateDeleteVolume(fc.CS.VolProto.VolumeID)
	if err != nil {
		e := fmt.Sprintf("%s (fc) - ValidateDeleteVolume volume ID %d- error: %s", functionName, fc.CS.VolProto.VolumeID, err.Error())
		zlog.Error().Msg(e)
		return nil, status.Error(codes.Internal, e)
	}
	return &csi.DeleteVolumeResponse{}, nil
}

func (fc *FCstorage) ControllerModifyVolume(ctx context.Context, req *csi.ControllerModifyVolumeRequest) (resp *csi.ControllerModifyVolumeResponse, err error) {
	return resp, nil
}

func (fc *FCstorage) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (resp *csi.ControllerPublishVolumeResponse, err error) {
	const functionName = "ControllerPublishVolume"
	zlog.Debug().Msgf("%s (fc) nodeID: %s volumeId: %s %s", functionName, req.GetNodeId(), req.GetVolumeId(), storagecommon.GetHostInfo(req.GetSecrets(), fc.CS.IboxAPI))
	volproto, err := storagecommon.ValidateVolumeID(req.GetVolumeId())
	if err != nil {
		e := fmt.Sprintf("%s (fc) - ValidateVolumeID - error: %s", functionName, err.Error())
		zlog.Error().Msg(e)
		return nil, status.Error(codes.Internal, e)
	}

	hostName, err := storagecommon.DetermineHostName(req.GetNodeId())
	if err != nil {
		e := fmt.Sprintf("%s (fc) - DetermineHostName - error: %s", functionName, err.Error())
		zlog.Error().Msg(e)
		return nil, status.Error(codes.Internal, e)
	}

	host, err := fc.CS.ValidateHost(hostName)
	if err != nil {
		e := fmt.Sprintf("%s (fc) - validateHost hostname %s- error: %s", functionName, hostName, err.Error())
		zlog.Error().Msg(e)
		return nil, status.Error(codes.Internal, e)
	}

	volume, err := fc.CS.IboxAPI.GetVolume(volproto.VolumeID)
	if err != nil {
		e := fmt.Sprintf("%s (fc) - GetVolume volume ID '%s' - error: %v", functionName, req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e)
		return nil, status.Error(codes.Internal, e)
	}

	_, err = fc.CS.AccessModesHelper.IsValidAccessMode(volume, req)
	if err != nil {
		e := fmt.Sprintf("%s (fc) - IsValidAccessMode - error: %s", functionName, err.Error())
		zlog.Error().Msg(e)
		return nil, status.Error(codes.Internal, e)
	}

	lunList, err := fc.CS.IboxAPI.GetAllLunByHost(host.ID)
	if err != nil {
		e := fmt.Sprintf("%s (fc) - GetAllLunByHost volume Name: %s host ID: %d- error: %s", functionName, volume.Name, host.ID, err.Error())
		zlog.Error().Msg(e)
		return nil, status.Error(codes.Internal, e)
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
				storagecommon.LUN_PUBLISH_CONTEXT:        strconv.Itoa(lun.Lun),
				storagecommon.HOST_ID_PUBLISH_CONTEXT:    strconv.Itoa(host.ID),
				storagecommon.HOST_PORTS_PUBLISH_CONTEXT: ports,
			}
			zlog.Debug().Msgf("%s (fc) - volume Name: %s volumeID: %d already mapped to host: %s", functionName, volume.Name, lun.VolumeID, host.Name)
			return &csi.ControllerPublishVolumeResponse{
				PublishContext: volCtx,
			}, nil
		}
	}

	// the max_vols_per_host storageclass parameter is not mandatory
	maxAllowedVolString := req.GetVolumeContext()[common.StorageClassMaxVolsPerHost]
	if maxAllowedVolString != "" {
		maxAllowedVol, err := strconv.Atoi(maxAllowedVolString)
		if err != nil {
			e := fmt.Sprintf("%s (fc) - invalid parameter %s error:  %v", functionName, common.StorageClassMaxVolsPerHost, err)
			zlog.Error().Msg(e)
			return nil, status.Error(codes.Internal, e)
		}
		if maxAllowedVol < 1 {
			e := fmt.Sprintf("%s (fc) - invalid parameter %s error:  required to be greater than 0", functionName, common.StorageClassMaxVolsPerHost)
			zlog.Error().Msg(e)
			return nil, status.Error(codes.Internal, e)
		}
		zlog.Debug().Msgf("host can have maximum %d volume mapped", maxAllowedVol)
		zlog.Debug().Msgf("host %s has %d volume mapped", host.Name, len(lunList))
		if len(lunList) >= maxAllowedVol {
			e := fmt.Sprintf("%s (fc) - unable to publish volume on host %s, maximum allowed volume per host is (%d), limit reached", functionName, host.Name, maxAllowedVol)
			zlog.Error().Msg(e)
			return nil, status.Error(codes.ResourceExhausted, e)
		}
	}
	// map volume to host
	zlog.Debug().Msgf("%s (fc) - mapping volume Name: %s volume ID: %d to host: %s", functionName, volume.Name, volproto.VolumeID, host.Name)
	luninfo, err := fc.CS.MapVolumeTohost(volproto.VolumeID, host.ID)
	if err != nil {
		e := fmt.Sprintf("%s (fc) - mapVolumeToHost - error: %s", functionName, err.Error())
		zlog.Error().Msg(e)
		return nil, status.Error(codes.Internal, e)
	}

	volCtx := map[string]string{
		storagecommon.LUN_PUBLISH_CONTEXT:        strconv.Itoa(luninfo.Lun),
		storagecommon.HOST_ID_PUBLISH_CONTEXT:    strconv.Itoa(host.ID),
		storagecommon.HOST_PORTS_PUBLISH_CONTEXT: ports,
	}
	return &csi.ControllerPublishVolumeResponse{
		PublishContext: volCtx,
	}, nil
}

func (fc *FCstorage) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (resp *csi.ControllerUnpublishVolumeResponse, err error) {
	const functionName = "ControllerUnpublishVolume"
	zlog.Debug().Msgf("%s (fc) - volProto %+v nodeID %s and volumeId %s", functionName, fc.CS.VolProto, req.GetNodeId(), req.GetVolumeId())

	host := fc.CS.VolProto.Host
	if len(host.Luns) > 0 {
		zlog.Debug().Msgf("%s (fc) - unmap volume ID: %d from host: %d", functionName, fc.CS.VolProto.VolumeID, host.ID)
		err = fc.CS.UnmapVolumeFromHost(host.ID, fc.CS.VolProto.VolumeID)
		if err != nil {
			e := fmt.Sprintf("%s (fc) - unmapVolumeFromHost - error unmapping volume %d from host %d error %v", functionName, fc.CS.VolProto.VolumeID, host.ID, err)
			zlog.Error().Msg(e)
			return nil, status.Error(codes.Internal, e)
		}
	}
	if len(host.Luns) < 2 {
		err = storagecommon.HostCleanup(fc.CS.IboxAPI, host.ID, host.Name)
		if err != nil {
			e := fmt.Sprintf("%s (fc) - hostCleanup - error host ID: %d. Error: %s", functionName, host.ID, err.Error())
			zlog.Error().Msg(e)
			return nil, status.Error(codes.Internal, e)
		}
	}
	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

func (fc *FCstorage) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (resp *csi.ValidateVolumeCapabilitiesResponse, err error) {
	zlog.Error().Msgf("ValidateVolumeCapabilities (fc) - should not be called, implemented in controller.go instead")
	return
}

func (fc *FCstorage) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (resp *csi.ListVolumesResponse, err error) {
	return &csi.ListVolumesResponse{}, nil
}

func (fc *FCstorage) ListSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (resp *csi.ListSnapshotsResponse, err error) {
	return &csi.ListSnapshotsResponse{}, nil
}

func (fc *FCstorage) GetCapacity(ctx context.Context, req *csi.GetCapacityRequest) (resp *csi.GetCapacityResponse, err error) {
	return &csi.GetCapacityResponse{}, nil
}

func (fc *FCstorage) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (resp *csi.ControllerGetCapabilitiesResponse, err error) {
	return &csi.ControllerGetCapabilitiesResponse{}, nil
}

func (fc *FCstorage) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (resp *csi.CreateSnapshotResponse, err error) {
	var snapshotID string
	const functionName = "CreateSnapshot"
	snapshotName := req.GetName()
	zlog.Debug().Msgf("%s (fc) - name: %s source volume ID: %s volproto: %+v", functionName, snapshotName, req.GetSourceVolumeId(), fc.CS.VolProto)

	volumeSnapshot, err := fc.CS.IboxAPI.GetVolumeByName(snapshotName)
	if err != nil {
		re, ok := err.(*iboxapi.APIError)
		if ok && re.Code == iboxapi.IBOXAPI_RESOURCE_NOT_FOUND_ERROR {
			zlog.Debug().Msgf("%s (fc) - snapshot with given name not found : %s", functionName, snapshotName)
		} else {
			e := fmt.Sprintf("%s (fc) - GetVolumeByName - name: %s error: %s", functionName, snapshotName, err.Error())
			zlog.Error().Msg(e)
			return nil, status.Error(codes.Internal, e)
		}
	} else if volumeSnapshot.ParentID == fc.CS.VolProto.VolumeID {
		snapshotID = strconv.Itoa(volumeSnapshot.ID) + "$$" + fc.CS.VolProto.StorageType
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
		return nil, status.Error(codes.AlreadyExists, fmt.Sprintf("snapshot Name: %s with already existing name and different source volume ID", volumeSnapshot.Name))
	}

	// look up the parent volume so we can get the ssd_enabled value and use that for
	// the snapshot being created next
	parentVolume, err := fc.CS.IboxAPI.GetVolume(fc.CS.VolProto.VolumeID)
	if err != nil {
		e := fmt.Sprintf("%s (fc) - GetVolume - error get parent volume when creating snapshot - volume id %d, err: %v", functionName, fc.CS.VolProto.VolumeID, err)
		zlog.Error().Msg(e)
		return nil, status.Error(codes.NotFound, e)
	}

	snapshotParam := iboxapi.CreateSnapshotVolumeRequest{
		ParentID:       fc.CS.VolProto.VolumeID,
		SnapshotName:   snapshotName,
		WriteProtected: true,
		SSDEnabled:     parentVolume.SsdEnabled,
	}

	lockExpiresAtParameter := req.Parameters[common.LockExpiresAtParameter]
	var lockExpiresAt int64
	if lockExpiresAtParameter != "" {
		ntpStatus, err := fc.CS.IboxAPI.GetNtpStatus()
		if err != nil {
			e := fmt.Sprintf("%s (fc) - GetNtpStatus - error: %s", functionName, err.Error())
			zlog.Error().Msg(e)
			return nil, status.Error(codes.Internal, e)
		}
		lockExpiresAt, err = storagecommon.ValidateSnapshotLockingParameter(ntpStatus[0].LastProbeTimestamp, lockExpiresAtParameter)
		if err != nil {
			e := fmt.Sprintf("%s (fc) - failed to create snapshot %s error %v, invalid lock_expires_at parameter ", functionName, snapshotName, err)
			zlog.Error().Msg(e)
			return nil, status.Error(codes.Internal, e)
		}
		zlog.Info().Msgf("%s (fc) - snapshot Name: %s snapshot param has a lock_expires_at: %s", functionName, snapshotName, lockExpiresAtParameter)
	}

	snapshotParam.LockExpiresAt = lockExpiresAt
	snapshot, err := fc.CS.IboxAPI.CreateSnapshotVolume(snapshotParam)
	if err != nil {
		e := fmt.Sprintf("CreateSnapshot (fc) - CreateSnapshotVolume  snapshot name: %s error: %s", snapshotName, err.Error())
		zlog.Error().Msg(e)
		return nil, status.Error(codes.Internal, e)
	}

	snapshotID = strconv.Itoa(snapshot.SnapShotID) + "$$" + fc.CS.VolProto.StorageType
	csiSnapshot := &csi.Snapshot{
		SnapshotId:     snapshotID,
		SourceVolumeId: req.GetSourceVolumeId(),
		ReadyToUse:     true,
		CreationTime:   timestamppb.Now(),
		SizeBytes:      snapshot.Size,
	}
	zlog.Debug().Msgf("CreateFileSystemSnapshot (fc) resp: %v", csiSnapshot)
	snapshotResp := &csi.CreateSnapshotResponse{Snapshot: csiSnapshot}
	return snapshotResp, nil
}

func (fc *FCstorage) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (resp *csi.DeleteSnapshotResponse, err error) {
	snapshotID, _ := strconv.Atoi(req.GetSnapshotId())

	err = fc.ValidateDeleteVolume(snapshotID)
	if err != nil {
		e := fmt.Sprintf("DeleteSnapshot (fc) - ValidateDeleteVolume - snapshot ID: %s error: %s", req.GetSnapshotId(), err.Error())
		zlog.Error().Msg(e)
		return nil, status.Error(codes.Internal, e)
	}
	return &csi.DeleteSnapshotResponse{}, nil
}

func (fc *FCstorage) ValidateDeleteVolume(volumeID int) (err error) {
	const functionName = "ValidateDeleteVolume"
	vol, err := fc.CS.IboxAPI.GetVolume(volumeID)
	if err != nil {
		re, ok := err.(*iboxapi.APIError)
		if ok && re.Code == iboxapi.IBOXAPI_RESOURCE_NOT_FOUND_ERROR {
			zlog.Debug().Msgf("%s (fc) - volume ID: %d is already deleted", functionName, volumeID)
			return nil
		}
		e := fmt.Sprintf("%s (fc) - GetVolume - error %s", functionName, err.Error())
		zlog.Error().Msg(e)
		return status.Error(codes.Internal, e)
	}

	if vol.LockState == common.LockedState {
		e := fmt.Sprintf("%s (fc) - volume ID: %d was locked, can not delete till expire date %s is reached", functionName, volumeID, time.UnixMilli(vol.LockExpiresAt))
		zlog.Error().Msg(e)
		return status.Error(codes.Aborted, e)
	}

	childVolumes, err := fc.CS.IboxAPI.GetVolumesByParentID(vol.ID)
	if err != nil {
		zlog.Error().Msgf("%s (fc) - error %s", functionName, err.Error())
	}
	if len(childVolumes) > 0 {
		metadata := map[string]interface{}{
			storagecommon.TOBEDELETED: true,
		}
		_, err = fc.CS.IboxAPI.PutMetadata(vol.ID, metadata)
		if err != nil {
			e := fmt.Sprintf("%s (fc) - failed to update host.k8s.to_be_deleted for volume %s error: %v", functionName, vol.Name, err)
			zlog.Error().Msg(e)
			err = errors.New(e)
		}
		return
	}
	zlog.Debug().Msgf("%s (fc) - deleting volume name: %s ID: %d", functionName, vol.Name, vol.ID)
	_, err = fc.CS.IboxAPI.DeleteMetadata(vol.ID)
	if err != nil {
		e := fmt.Sprintf("%s (fc) - DeleteMetadata - error %s", functionName, err.Error())
		zlog.Error().Msg(e)
		return status.Error(codes.Internal, e)
	}
	_, err = fc.CS.IboxAPI.DeleteVolume(vol.ID)
	if err != nil {
		e := fmt.Sprintf("%s (fc) -  DeleteVolume - error %s", functionName, err.Error())
		zlog.Error().Msg(e)
		return status.Error(codes.Internal, e)
	}
	if vol.ParentID != 0 {
		zlog.Debug().Msgf("%s (fc) - checking if parent volume can be name: %s ID: %d", functionName, vol.Name, vol.ID)
		var metadata []iboxapi.GetMetadataResult
		metadata, err = fc.CS.IboxAPI.GetMetadata(vol.ParentID)
		if err != nil {
			e := fmt.Sprintf("%s (fc) - error %s", functionName, err.Error())
			zlog.Error().Msg(e)
			return status.Error(codes.Internal, e)
		}
		var toBeDeleted bool
		for _, m := range metadata {
			if m.Key == api.TOBEDELETED {
				toBeDeleted = true
			}
		}
		if toBeDeleted {
			err = fc.ValidateDeleteVolume(vol.ParentID)
			if err != nil {
				e := fmt.Sprintf("%s (fc) - error %s", functionName, err.Error())
				zlog.Error().Msg(e)
				return status.Error(codes.Internal, e)
			}
		}
	}
	return
}

func (fc *FCstorage) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (resp *csi.ControllerExpandVolumeResponse, err error) {
	const functionName = "ControllerExpandVolume"
	volumeID := fc.CS.VolProto.VolumeID
	zlog.Debug().Msgf("%s (fc) - volume ID: %d", functionName, volumeID)

	capacity := req.GetCapacityRange().GetRequiredBytes()
	if capacity < storagecommon.GIB {
		capacity = storagecommon.GIB
		zlog.Warn().Msgf("%s (fc) - Volume Minimum capacity should be greater 1 GB", functionName)
	}

	// Expand volume size
	volume := iboxapi.Volume{
		Size: capacity,
	}
	_, err = fc.CS.IboxAPI.UpdateVolume(volumeID, volume)
	if err != nil {
		e := fmt.Errorf("%s (fc) - UpdateVolume - error: %s", functionName, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, e
	}
	zlog.Debug().Msgf("%s (fc) - volume size updated successfully volume ID: %d", functionName, volumeID)
	return &csi.ControllerExpandVolumeResponse{
		CapacityBytes:         capacity,
		NodeExpansionRequired: true,
	}, nil
}

func (fc *FCstorage) ControllerGetVolume(
	_ context.Context, _ *csi.ControllerGetVolumeRequest,
) (*csi.ControllerGetVolumeResponse, error) {
	// Infinidat does not support ControllerGetVolume
	return nil, status.Error(codes.Unimplemented, "")
}

func (fc *FCstorage) createVolumeFromVolumeContent(req *csi.CreateVolumeRequest, name string, sizeInKbytes int64, storagePool string) (*csi.CreateVolumeResponse, error) {
	var err error
	const functionName = "createVolumeFromVolumeContent"

	volumecontent := req.GetVolumeContentSource()
	var volumeContentID string
	var restoreType string
	if volumecontent.GetSnapshot() != nil {
		restoreType = storagecommon.RESTORE_TYPE_SNAPSHOT
		volumeContentID = volumecontent.GetSnapshot().GetSnapshotId()
	} else if volumecontent.GetVolume() != nil {
		volumeContentID = volumecontent.GetVolume().GetVolumeId()
		restoreType = storagecommon.RESTORE_TYPE_VOLUME
	}

	// Validate the source content id
	volproto, err := storagecommon.ValidateVolumeID(volumeContentID)
	if err != nil {
		e := fmt.Sprintf("%s (fc) - failed to validate storage type for restore type: %s source id: %s, err: %v", functionName, restoreType, volumeContentID, err)
		zlog.Error().Msg(e)
		return nil, status.Error(codes.NotFound, e)
	}

	srcVol, err := fc.CS.IboxAPI.GetVolume(volproto.VolumeID)
	if err != nil {
		e := fmt.Sprintf("%s (fc) - GetVolume - restoreType: %s volume ID: %d error: %s", functionName, restoreType, volproto.VolumeID, err.Error())
		zlog.Error().Msg(e)
		return nil, status.Error(codes.NotFound, e)
	}

	// Validate the size is the same.
	if srcVol.Size != sizeInKbytes {
		return nil, status.Errorf(codes.InvalidArgument,
			restoreType+" %s has incompatible size %d kbytes with requested %d kbytes",
			volumeContentID, srcVol.Size, sizeInKbytes)
	}

	// Validate the storagePool is the same.
	pool, err := fc.CS.IboxAPI.GetPoolByName(storagePool)
	if err != nil {
		e := fmt.Sprintf("%s (fc) - GetPoolByName - pool name: %s  error %s", functionName, storagePool, err.Error())
		zlog.Error().Msg(e)
		return nil, status.Error(codes.Internal, e)
	}
	if pool.ID != srcVol.PoolID {
		e := fmt.Sprintf("%s (fc) - volume storage pool is different than requested storage pool %s", functionName, storagePool)
		zlog.Error().Msg(e)
		return nil, status.Error(codes.InvalidArgument, e)
	}
	ssd := req.GetParameters()[common.StorageClassSSDEnabled]
	if ssd == "" {
		ssd = strconv.FormatBool(false)
	}
	ssdEnabled, _ := strconv.ParseBool(ssd)
	snapshotParam := iboxapi.CreateSnapshotVolumeRequest{
		ParentID:       volproto.VolumeID,
		SnapshotName:   name,
		WriteProtected: false,
		SSDEnabled:     ssdEnabled,
		LockExpiresAt:  0,
	}
	// Create snapshot
	snapResponse, err := fc.CS.IboxAPI.CreateSnapshotVolume(snapshotParam)
	if err != nil {
		e := fmt.Sprintf("%s (fc) - CreateSnapshotVolume - error %s", functionName, err.Error())
		zlog.Error().Msg(e)
		return nil, status.Error(codes.Internal, e)
	}

	// Retrieve created destination volume
	volID := snapResponse.SnapShotID
	dstVol, err := fc.CS.IboxAPI.GetVolume(volID)
	if err != nil {
		e := fmt.Sprintf("%s (fc) - GetVolume - volume ID: %d error: %s", functionName, volID, err.Error())
		zlog.Error().Msg(e)
		return nil, status.Error(codes.Internal, e)
	}

	// Create a volume response and return it
	csiVolume := fc.CS.GetCSIResponse(dstVol, req)
	storagecommon.CopyRequestParameters(req.GetParameters(), csiVolume.VolumeContext)

	metadata := map[string]interface{}{
		"host.k8s.pvname": dstVol.Name,
	}
	_, err = fc.CS.IboxAPI.PutMetadata(dstVol.ID, metadata)
	if err != nil {
		e := fmt.Sprintf("%s - PutMetadata - failed to attach metadata for volume: %s, err: %v", functionName, dstVol.Name, err)
		zlog.Error().Msg(e)
		return nil, status.Error(codes.Internal, e)
	}
	zlog.Debug().Msgf("%s - Volume (from snap) %s (%s) storage pool %s", functionName,
		csiVolume.VolumeContext["Name"], csiVolume.VolumeId, csiVolume.VolumeContext["StoragePoolName"])
	return &csi.CreateVolumeResponse{Volume: csiVolume}, nil
}
