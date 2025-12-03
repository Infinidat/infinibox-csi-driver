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
	"log/slog"
	"strconv"
	"time"

	"github.com/infinidat/infinibox-csi-driver/api"
	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/infinidat/infinibox-csi-driver/iboxapi"
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

func (fc *FCstorage) ValidateStorageClass(params map[string]string) error {

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
		e := fmt.Errorf("(fc) - error %s", err.Error())
		slog.Error(e.Error())
		return status.Error(codes.InvalidArgument, e.Error())
	}
	return nil
}

func (fc *FCstorage) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	params := req.GetParameters()
	fc.ConfigMap = params
	slog.Debug("(fc)", "requested volume parameters", params, "requested size", fc.Capacity,
		"iboxInfo", storagecommon.GetHostInfo(ctx, req.GetSecrets(), fc.CS.IboxAPI))

	// Volume name to be created - already verified in controller.go
	name := req.GetName()

	poolName := params[common.StorageClassPoolName]

	targetVol, err := fc.CS.IboxAPI.GetVolumeByName(ctx, name)
	if err != nil {
		re, ok := err.(*iboxapi.APIError)
		if ok && re.Code == iboxapi.RESOURCE_NOT_FOUND {
			slog.Debug("fc) volume with name not found, proceeding to create", "name", name)
		} else {
			e := fmt.Errorf("(fc) - GetVolumeByName %s - error: %s", name, err.Error())
			slog.Error(e.Error())
			return nil, status.Error(codes.Internal, e.Error())
		}
	}

	if targetVol != nil {
		slog.Debug("(fc) volume: found", "volume", name, "size", targetVol.Size, "requested:", fc.Capacity)
		if targetVol.Size == fc.Capacity {
			existingVolumeInfo := fc.CS.GetCSIResponse(ctx, targetVol, req)
			storagecommon.CopyRequestParameters(params, existingVolumeInfo.VolumeContext)
			return &csi.CreateVolumeResponse{
				Volume: existingVolumeInfo,
			}, nil
		}
		err = status.Errorf(codes.AlreadyExists, "(fc) - volume: %s already exists with a different size, %v", name, err)
		slog.Error(err.Error())
		return nil, err
	}

	// Volume content source support volume and snapshots
	contentSource := req.GetVolumeContentSource()
	if contentSource != nil {
		return fc.createVolumeFromVolumeContent(ctx, req, name, fc.Capacity, poolName)
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

	volumeParam.SSDEnabled, err = storagecommon.DetermineSSDValue(ctx, params[common.StorageClassSSDEnabled], poolName, fc.CS.IboxAPI)
	if err != nil {
		e := fmt.Sprintf("(fc) - determineSSDValue - error when creating volume %s storagepool %s, err: %s", name, poolName, err.Error())
		slog.Error(e)
		return nil, status.Error(codes.Internal, e)
	}

	pool, err := fc.CS.IboxAPI.GetPoolByName(ctx, poolName)
	if err != nil {
		e := fmt.Sprintf("(fc) - GetPoolByName volume name: %s pool name: %s error: %s", name, poolName, err.Error())
		slog.Error(e)
		return nil, status.Error(codes.Internal, e)
	}

	createVolumeRequest := iboxapi.CreateVolumeRequest{
		Name:          volumeParam.Name,
		SSDEnabled:    volumeParam.SSDEnabled,
		ProvisionType: volumeParam.ProvisionType,
		PoolID:        pool.ID,
		VolumeSize:    volumeParam.VolumeSize,
	}

	volumeResp, err := fc.CS.IboxAPI.CreateVolume(ctx, createVolumeRequest)
	if err != nil {
		e := fmt.Sprintf("(fc) - CreateVolume - error when creating volume %s storagepool %s, err: %s", name, poolName, err.Error())
		slog.Error(e)
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
	vol, err = fc.CS.IboxAPI.GetVolume(ctx, volumeResp.ID)
	if err != nil {
		slog.Error("(fc) - GetVolume", "error", err.Error())
	}

	// a single test just in case there is a race condition on createVolume (doubtful)
	if vol == nil {
		time.Sleep(3 * time.Second)
		_, err = fc.CS.IboxAPI.GetVolume(ctx, volumeResp.ID)
		if err != nil {
			e := fmt.Sprintf("(fc) - GetVolume - failed to create volume name: %s volume not retrieved for id: %d", name, volumeResp.ID)
			slog.Error(e)
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
	_, err = fc.CS.IboxAPI.PutMetadata(ctx, volumeResp.ID, metadata)
	if err != nil {
		e := fmt.Sprintf("(fc) - PutMetadata - failed to attach metadata - volume %s- error: %s", name, err.Error())
		slog.Error(e)
		return nil, status.Error(codes.Internal, e)
	}

	slog.Debug("(fc) - created volume", "name", name, "id", volumeResp.ID)
	return csiResp, err
}

func (fc *FCstorage) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (csiResp *csi.DeleteVolumeResponse, err error) {
	slog.Debug("(fc) called")
	err = fc.ValidateDeleteVolume(ctx, fc.CS.VolProto.VolumeID)
	if err != nil {
		e := fmt.Sprintf("(fc) - ValidateDeleteVolume volume ID %d- error: %s", fc.CS.VolProto.VolumeID, err.Error())
		slog.Error(e)
		return nil, status.Error(codes.Internal, e)
	}
	return &csi.DeleteVolumeResponse{}, nil
}

func (fc *FCstorage) ControllerModifyVolume(ctx context.Context, req *csi.ControllerModifyVolumeRequest) (resp *csi.ControllerModifyVolumeResponse, err error) {
	return resp, nil
}

func (fc *FCstorage) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (resp *csi.ControllerPublishVolumeResponse, err error) {
	slog.Debug("(fc)", "nodeID", req.GetNodeId(), "volumeId", req.GetVolumeId(), "iboxInfo", storagecommon.GetHostInfo(ctx, req.GetSecrets(), fc.CS.IboxAPI))
	volproto, err := storagecommon.ValidateVolumeID(req.GetVolumeId())
	if err != nil {
		e := fmt.Sprintf("(fc) - ValidateVolumeID - error: %s", err.Error())
		slog.Error(e)
		return nil, status.Error(codes.Internal, e)
	}

	hostName, err := storagecommon.DetermineHostName(req.GetNodeId())
	if err != nil {
		e := fmt.Sprintf("(fc) - DetermineHostName - error: %s", err.Error())
		slog.Error(e)
		return nil, status.Error(codes.Internal, e)
	}

	host, err := fc.CS.ValidateHost(ctx, hostName)
	if err != nil {
		e := fmt.Sprintf("(fc) - validateHost hostname %s- error: %s", hostName, err.Error())
		slog.Error(e)
		return nil, status.Error(codes.Internal, e)
	}

	volume, err := fc.CS.IboxAPI.GetVolume(ctx, volproto.VolumeID)
	if err != nil {
		e := fmt.Sprintf("(fc) - GetVolume volume ID '%s' - error: %v", req.GetVolumeId(), err.Error())
		slog.Error(e)
		return nil, status.Error(codes.Internal, e)
	}

	_, err = fc.CS.AccessModesHelper.IsValidAccessMode(volume, req)
	if err != nil {
		e := fmt.Sprintf("(fc) - IsValidAccessMode - error: %s", err.Error())
		slog.Error(e)
		return nil, status.Error(codes.Internal, e)
	}

	lunList, err := fc.CS.IboxAPI.GetAllLunByHost(ctx, host.ID)
	if err != nil {
		e := fmt.Sprintf("(fc) - GetAllLunByHost volume Name: %s host ID: %d- error: %s", volume.Name, host.ID, err.Error())
		slog.Error(e)
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
	slog.Debug("info", "ports", ports)
	for _, lun := range lunList {
		if lun.VolumeID == volproto.VolumeID {
			volCtx := map[string]string{
				storagecommon.LunPublishContext:       strconv.Itoa(lun.Lun),
				storagecommon.HostIDPublishContext:    strconv.Itoa(host.ID),
				storagecommon.HostPortsPublishContext: ports,
			}
			slog.Debug("(fc)", "volume Name", volume.Name, "volumeID", lun.VolumeID, "already mapped to host", host.Name)
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
			e := fmt.Sprintf("(fc) - invalid parameter %s error:  %v", common.StorageClassMaxVolsPerHost, err)
			slog.Error(e)
			return nil, status.Error(codes.Internal, e)
		}
		if maxAllowedVol < 1 {
			e := fmt.Sprintf("(fc) - invalid parameter %s error:  required to be greater than 0", common.StorageClassMaxVolsPerHost)
			slog.Error(e)
			return nil, status.Error(codes.Internal, e)
		}
		slog.Debug("host can have maximum volume mapped", "value", maxAllowedVol)
		slog.Debug("volume mapped", "host", host.Name, "mapped", len(lunList))
		if len(lunList) >= maxAllowedVol {
			e := fmt.Sprintf("(fc) - unable to publish volume on host %s, maximum allowed volume per host is (%d), limit reached", host.Name, maxAllowedVol)
			slog.Error(e)
			return nil, status.Error(codes.ResourceExhausted, e)
		}
	}
	// map volume to host
	slog.Debug("(fc) - mapping volume", "Name", volume.Name, "volume ID", volproto.VolumeID, "to host", host.Name)
	luninfo, err := fc.CS.MapVolumeTohost(ctx, volproto.VolumeID, host.ID)
	if err != nil {
		e := fmt.Sprintf("(fc) - mapVolumeToHost - error: %s", err.Error())
		slog.Error(e)
		return nil, status.Error(codes.Internal, e)
	}

	volCtx := map[string]string{
		storagecommon.LunPublishContext:       strconv.Itoa(luninfo.Lun),
		storagecommon.HostIDPublishContext:    strconv.Itoa(host.ID),
		storagecommon.HostPortsPublishContext: ports,
	}
	return &csi.ControllerPublishVolumeResponse{
		PublishContext: volCtx,
	}, nil
}

func (fc *FCstorage) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (resp *csi.ControllerUnpublishVolumeResponse, err error) {
	slog.Debug("(fc)", "volProto", fc.CS.VolProto, "nodeID", req.GetNodeId(), "volumeId", req.GetVolumeId())

	host := fc.CS.VolProto.Host
	if len(host.Luns) > 0 {
		slog.Debug("(fc) - unmap volume", "ID", fc.CS.VolProto.VolumeID, "from host", host.ID)
		err = fc.CS.UnmapVolumeFromHost(ctx, host.ID, fc.CS.VolProto.VolumeID)
		if err != nil {
			e := fmt.Sprintf("(fc) - unmapVolumeFromHost - error unmapping volume %d from host %d error %v", fc.CS.VolProto.VolumeID, host.ID, err)
			slog.Error(e)
			return nil, status.Error(codes.Internal, e)
		}
	}
	if len(host.Luns) < 2 {
		err = storagecommon.HostCleanup(ctx, fc.CS.IboxAPI, host.ID, host.Name)
		if err != nil {
			e := fmt.Sprintf("(fc) - hostCleanup - error host ID: %d. Error: %s", host.ID, err.Error())
			slog.Error(e)
			return nil, status.Error(codes.Internal, e)
		}
	}
	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

func (fc *FCstorage) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (resp *csi.ValidateVolumeCapabilitiesResponse, err error) {
	slog.Error("ValidateVolumeCapabilities (fc) - should not be called, implemented in controller.go instead")
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
	snapshotName := req.GetName()
	slog.Debug("(fc)", "name", snapshotName, "source volume ID", req.GetSourceVolumeId(), "volproto", fc.CS.VolProto)

	volumeSnapshot, err := fc.CS.IboxAPI.GetVolumeByName(ctx, snapshotName)
	if err != nil {
		re, ok := err.(*iboxapi.APIError)
		if ok && re.Code == iboxapi.RESOURCE_NOT_FOUND {
			slog.Debug("(fc) - snapshot with given name not found", "name", snapshotName)
		} else {
			e := fmt.Sprintf("(fc) - GetVolumeByName - name: %s error: %s", snapshotName, err.Error())
			slog.Error(e)
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
	parentVolume, err := fc.CS.IboxAPI.GetVolume(ctx, fc.CS.VolProto.VolumeID)
	if err != nil {
		e := fmt.Sprintf("(fc) - GetVolume - error get parent volume when creating snapshot - volume id %d, err: %v", fc.CS.VolProto.VolumeID, err)
		slog.Error(e)
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
		ntpStatus, err := fc.CS.IboxAPI.GetNtpStatus(ctx)
		if err != nil {
			e := fmt.Sprintf("(fc) - GetNtpStatus - error: %s", err.Error())
			slog.Error(e)
			return nil, status.Error(codes.Internal, e)
		}
		lockExpiresAt, err = storagecommon.ValidateSnapshotLockingParameter(ntpStatus[0].LastProbeTimestamp, lockExpiresAtParameter)
		if err != nil {
			e := fmt.Sprintf("(fc) - failed to create snapshot %s error %v, invalid lock_expires_at parameter ", snapshotName, err)
			slog.Error(e)
			return nil, status.Error(codes.Internal, e)
		}
		slog.Info("(fc) - snapshot", "Name", snapshotName, "snapshot param has a lock_expires_at", lockExpiresAtParameter)
	}

	snapshotParam.LockExpiresAt = lockExpiresAt
	snapshot, err := fc.CS.IboxAPI.CreateSnapshotVolume(ctx, snapshotParam)
	if err != nil {
		e := fmt.Sprintf("CreateSnapshot (fc) - CreateSnapshotVolume  snapshot name: %s error: %s", snapshotName, err.Error())
		slog.Error(e)
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
	slog.Debug("CreateFileSystemSnapshot (fc)", "response", csiSnapshot)
	snapshotResp := &csi.CreateSnapshotResponse{Snapshot: csiSnapshot}
	return snapshotResp, nil
}

func (fc *FCstorage) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (resp *csi.DeleteSnapshotResponse, err error) {
	snapshotID, _ := strconv.Atoi(req.GetSnapshotId())

	err = fc.ValidateDeleteVolume(ctx, snapshotID)
	if err != nil {
		e := fmt.Sprintf("DeleteSnapshot (fc) - ValidateDeleteVolume - snapshot ID: %s error: %s", req.GetSnapshotId(), err.Error())
		slog.Error(e)
		return nil, status.Error(codes.Internal, e)
	}
	return &csi.DeleteSnapshotResponse{}, nil
}

func (fc *FCstorage) ValidateDeleteVolume(ctx context.Context, volumeID int) (err error) {
	vol, err := fc.CS.IboxAPI.GetVolume(ctx, volumeID)
	if err != nil {
		re, ok := err.(*iboxapi.APIError)
		if ok && re.Code == iboxapi.RESOURCE_NOT_FOUND {
			slog.Debug("(fc)", "volume ID is already deleted", volumeID)
			return nil
		}
		e := fmt.Sprintf("(fc) - GetVolume - error %s", err.Error())
		slog.Error(e)
		return status.Error(codes.Internal, e)
	}

	if vol.LockState == common.LockedState {
		e := fmt.Sprintf("(fc) - volume ID: %d was locked, can not delete till expire date %s is reached", volumeID, time.UnixMilli(vol.LockExpiresAt))
		slog.Error(e)
		return status.Error(codes.Aborted, e)
	}

	childVolumes, err := fc.CS.IboxAPI.GetVolumesByParentID(ctx, vol.ID)
	if err != nil {
		slog.Error("(fc)", "error", err.Error())
	}
	if len(childVolumes) > 0 {
		metadata := map[string]interface{}{
			storagecommon.ToBeDeleted: true,
		}
		_, err = fc.CS.IboxAPI.PutMetadata(ctx, vol.ID, metadata)
		if err != nil {
			e := fmt.Sprintf("(fc) - failed to update host.k8s.to_be_deleted for volume %s error: %v", vol.Name, err)
			slog.Error(e)
			err = errors.New(e)
		}
		return
	}
	slog.Debug("(fc) - deleting volume", "name", vol.Name, "ID", vol.ID)
	_, err = fc.CS.IboxAPI.DeleteMetadata(ctx, vol.ID)
	if err != nil {
		e := fmt.Sprintf("(fc) - DeleteMetadata - error %s", err.Error())
		slog.Error(e)
		return status.Error(codes.Internal, e)
	}
	_, err = fc.CS.IboxAPI.DeleteVolume(ctx, vol.ID)
	if err != nil {
		e := fmt.Sprintf("(fc) -  DeleteVolume - error %s", err.Error())
		slog.Error(e)
		return status.Error(codes.Internal, e)
	}
	if vol.ParentID != 0 {
		slog.Debug("(fc) - checking if parent volume can be", "name", vol.Name, "ID", vol.ID)
		var metadata []iboxapi.GetMetadataResult
		metadata, err = fc.CS.IboxAPI.GetMetadata(ctx, vol.ParentID)
		if err != nil {
			e := fmt.Sprintf("(fc) - error %s", err.Error())
			slog.Error(e)
			return status.Error(codes.Internal, e)
		}
		var toBeDeleted bool
		for _, m := range metadata {
			if m.Key == api.TOBEDELETED {
				toBeDeleted = true
			}
		}
		if toBeDeleted {
			err = fc.ValidateDeleteVolume(ctx, vol.ParentID)
			if err != nil {
				e := fmt.Sprintf("(fc) - error %s", err.Error())
				slog.Error(e)
				return status.Error(codes.Internal, e)
			}
		}
	}
	return
}

func (fc *FCstorage) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (resp *csi.ControllerExpandVolumeResponse, err error) {
	volumeID := fc.CS.VolProto.VolumeID
	slog.Debug("(fc)", "volume ID", volumeID)

	capacity := fc.Capacity

	// Expand volume size
	volume := iboxapi.Volume{
		Size: capacity,
	}
	_, err = fc.CS.IboxAPI.UpdateVolume(ctx, volumeID, volume)
	if err != nil {
		e := fmt.Errorf("(fc) - UpdateVolume - error: %s", err.Error())
		slog.Error(e.Error())
		return nil, e
	}
	slog.Debug("(fc)- volume size updated successfully", "volume ID", volumeID)
	nodeExpansionRequired := true
	if req.GetVolumeCapability().GetBlock() != nil {
		slog.Debug("ControllerExpandVolume (iscsi) - volume is block so nodeExpansionRequired is false")
		nodeExpansionRequired = false
	}
	return &csi.ControllerExpandVolumeResponse{
		CapacityBytes:         capacity,
		NodeExpansionRequired: nodeExpansionRequired,
	}, nil
}

func (fc *FCstorage) ControllerGetVolume(
	_ context.Context, _ *csi.ControllerGetVolumeRequest,
) (*csi.ControllerGetVolumeResponse, error) {
	// Infinidat does not support ControllerGetVolume
	return nil, status.Error(codes.Unimplemented, "")
}

func (fc *FCstorage) createVolumeFromVolumeContent(ctx context.Context, req *csi.CreateVolumeRequest, name string, sizeInKbytes int64, storagePool string) (*csi.CreateVolumeResponse, error) {
	var err error

	volumecontent := req.GetVolumeContentSource()
	var volumeContentID string
	var restoreType string
	if volumecontent.GetSnapshot() != nil {
		restoreType = storagecommon.RestoryTypeSnapshot
		volumeContentID = volumecontent.GetSnapshot().GetSnapshotId()
	} else if volumecontent.GetVolume() != nil {
		volumeContentID = volumecontent.GetVolume().GetVolumeId()
		restoreType = storagecommon.RestoreTypeVolume
	}

	// Validate the source content id
	volproto, err := storagecommon.ValidateVolumeID(volumeContentID)
	if err != nil {
		e := fmt.Sprintf("(fc) - failed to validate storage type for restore type: %s source id: %s, err: %v", restoreType, volumeContentID, err)
		slog.Error(e)
		return nil, status.Error(codes.NotFound, e)
	}

	srcVol, err := fc.CS.IboxAPI.GetVolume(ctx, volproto.VolumeID)
	if err != nil {
		e := fmt.Sprintf("(fc) - GetVolume - restoreType: %s volume ID: %d error: %s", restoreType, volproto.VolumeID, err.Error())
		slog.Error(e)
		return nil, status.Error(codes.NotFound, e)
	}

	// Validate the size is the same.
	if srcVol.Size != sizeInKbytes {
		return nil, status.Errorf(codes.InvalidArgument,
			restoreType+" %s has incompatible size %d kbytes with requested %d kbytes",
			volumeContentID, srcVol.Size, sizeInKbytes)
	}

	// Validate the storagePool is the same.
	pool, err := fc.CS.IboxAPI.GetPoolByName(ctx, storagePool)
	if err != nil {
		e := fmt.Sprintf("(fc) - GetPoolByName - pool name: %s  error %s", storagePool, err.Error())
		slog.Error(e)
		return nil, status.Error(codes.Internal, e)
	}
	if pool.ID != srcVol.PoolID {
		e := fmt.Sprintf("(fc) - volume storage pool is different than requested storage pool %s", storagePool)
		slog.Error(e)
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
	snapResponse, err := fc.CS.IboxAPI.CreateSnapshotVolume(ctx, snapshotParam)
	if err != nil {
		e := fmt.Sprintf("(fc) - CreateSnapshotVolume - error %s", err.Error())
		slog.Error(e)
		return nil, status.Error(codes.Internal, e)
	}

	// Retrieve created destination volume
	volID := snapResponse.SnapShotID
	dstVol, err := fc.CS.IboxAPI.GetVolume(ctx, volID)
	if err != nil {
		e := fmt.Sprintf("(fc) - GetVolume - volume ID: %d error: %s", volID, err.Error())
		slog.Error(e)
		return nil, status.Error(codes.Internal, e)
	}

	// Create a volume response and return it
	csiVolume := fc.CS.GetCSIResponse(ctx, dstVol, req)
	storagecommon.CopyRequestParameters(req.GetParameters(), csiVolume.VolumeContext)

	metadata := map[string]interface{}{
		"host.k8s.pvname": dstVol.Name,
	}
	_, err = fc.CS.IboxAPI.PutMetadata(ctx, dstVol.ID, metadata)
	if err != nil {
		e := fmt.Sprintf("PutMetadata - failed to attach metadata for volume: %s, err: %v", dstVol.Name, err)
		slog.Error(e)
		return nil, status.Error(codes.Internal, e)
	}
	slog.Debug("info", "Volume (from snap)", csiVolume.VolumeContext["Name"], "volumeID", csiVolume.VolumeId, "storage pool", csiVolume.VolumeContext["StoragePoolName"])
	return &csi.CreateVolumeResponse{Volume: csiVolume}, nil
}
