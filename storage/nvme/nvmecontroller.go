/*
Copyright 2024 Infinidat
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
package nvme

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/infinidat/infinibox-csi-driver/api"
	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/infinidat/infinibox-csi-driver/helper"
	"github.com/infinidat/infinibox-csi-driver/iboxapi"
	storagecommon "github.com/infinidat/infinibox-csi-driver/storage/common"

	"strconv"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const NVMEHostSuffix = "-nvme"

type NVMEstorage struct {
	Capacity      int64
	CS            storagecommon.Commonservice
	OSHelper      helper.OsHelper
	StorageHelper storagecommon.StorageHelper
	csi.UnimplementedControllerServer
	csi.UnimplementedNodeServer
}

func NewNVMEstorage(capacity int64, cs storagecommon.Commonservice) (nvme *NVMEstorage) {
	nvme = &NVMEstorage{
		Capacity:      capacity,
		CS:            cs,
		StorageHelper: storagecommon.StorageService{},
		OSHelper:      helper.Service{},
	}
	return nvme
}

func (nvme *NVMEstorage) ValidateStorageClass(params map[string]string) error {
	requiredNVMEParams := map[string]string{
		common.StorageClassPoolName:     `[a-zA-Z]+`, // match all strings except empty string or blank string
		common.StorageClassNetworkSpace: `\A.*\z`,    // TODO: could make this enforce IBOX network_space requirements, but probably not necessary
	}
	optionalNVMEParams := map[string]string{}

	// validate required parameters
	err := storagecommon.ValidateRequiredOptionalSCParameters(requiredNVMEParams, optionalNVMEParams, params)
	if err != nil {
		e := fmt.Errorf("from Validate - error: %s", err.Error())
		slog.Error(e.Error())
		return status.Error(codes.InvalidArgument, e.Error())
	}
	return nil
}

func (nvme *NVMEstorage) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	params := req.GetParameters()

	slog.Debug("info", "volume", req.GetName(), "size", nvme.Capacity, "params", params, "iboxInfo", storagecommon.GetHostInfo(ctx, req.GetSecrets(), nvme.CS.IboxAPI))

	// Volume name to be created - already verified earlier
	name := req.GetName()

	poolName := params[common.StorageClassPoolName]

	targetVolume, err := nvme.CS.IboxAPI.GetVolumeByName(ctx, name)
	if err != nil {
		re, ok := err.(*iboxapi.APIError)
		if ok && re.Code == iboxapi.RESOURCE_NOT_FOUND {
			slog.Debug("volume not found, will proceed to create it", "volume", req.GetName())
		} else {
			e := fmt.Errorf("from GetVolumeByName - error: %s", err.Error())
			slog.Error(e.Error())
			return nil, status.Error(codes.NotFound, e.Error())
		}
	}
	if targetVolume != nil {
		slog.Debug("volume found", "volume", name, "size", targetVolume.Size, "requested", nvme.Capacity)
		if targetVolume.Size == nvme.Capacity {
			existingVolumeInfo := nvme.CS.GetCSIResponse(ctx, targetVolume, req)
			storagecommon.CopyRequestParameters(params, existingVolumeInfo.VolumeContext)
			return &csi.CreateVolumeResponse{
				Volume: existingVolumeInfo,
			}, nil
		}
		msg := fmt.Sprintf("CreateVolume (nvme) - failed: volume %s exists but has different size", name)
		slog.Error(msg)
		return nil, status.Error(codes.AlreadyExists, msg)
	}

	// Volume content source support volume and snapshots
	contentSource := req.GetVolumeContentSource()
	if contentSource != nil {
		return nvme.createVolumeFromContentSource(ctx, req, name, nvme.Capacity, poolName)
	}

	volumeType, provided := params[common.StorageClassProvisionType]
	if !provided {
		volumeType = common.StorageClassThinProvision
	}

	volumeParam := &api.VolumeParam{
		Name:          name,
		VolumeSize:    nvme.Capacity,
		ProvisionType: volumeType,
	}

	volumeParam.SSDEnabled, err = storagecommon.DetermineSSDValue(ctx, params[common.StorageClassSSDEnabled], poolName, nvme.CS.IboxAPI)
	if err != nil {
		e := status.Errorf(codes.Internal, "CreateVolume (nvme) - determineSSDValue - error when creating volume %s storagepool %s, err: %s", name, poolName, err.Error())
		slog.Error(e.Error())
		return nil, e
	}

	pool, err := nvme.CS.IboxAPI.GetPoolByName(ctx, poolName)
	if err != nil {
		e := fmt.Errorf("from GetPoolByName name: %s error: %s", poolName, err.Error())
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	createVolumeRequest := iboxapi.CreateVolumeRequest{
		Name:          volumeParam.Name,
		PoolID:        pool.ID,
		SSDEnabled:    volumeParam.SSDEnabled,
		ProvisionType: volumeParam.ProvisionType,
		VolumeSize:    volumeParam.VolumeSize,
	}
	createVolumeResponse, err := nvme.CS.IboxAPI.CreateVolume(ctx, createVolumeRequest)
	if err != nil {
		e := fmt.Errorf("api CreateVolume - creating volume: %s pool: %s error: %s", name, poolName, err.Error())
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	csiResponse := nvme.CS.GetCSIResponse(ctx, createVolumeResponse, req)

	// check volume id format
	volumeID, err := strconv.Atoi(csiResponse.VolumeId)
	if err != nil {
		e := fmt.Errorf("parsing volumeID: %s - error: %s", csiResponse.VolumeId, err.Error())
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	MAX_TRIES := 10
	var volume *iboxapi.Volume
	for range MAX_TRIES {
		volume, err = nvme.CS.IboxAPI.GetVolume(ctx, volumeID)
		if err == nil {
			slog.Debug("volume found", "volume", volume.Name)
			break
		}
		slog.Debug("volume not found, trying again after 1 second", "volumeID", volumeID)
		time.Sleep(1 * time.Second)
	}
	if volume == nil {
		e := fmt.Errorf("from GetVolume - name: %s volumeID: %d", name, volumeID)
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	// Prepare response struct
	storagecommon.CopyRequestParameters(params, csiResponse.VolumeContext)
	csiResp := &csi.CreateVolumeResponse{
		Volume: csiResponse,
	}

	// attach metadata to volume object
	metadata := map[string]interface{}{
		"host.k8s.pvname": volume.Name,
	}
	_, err = nvme.CS.IboxAPI.PutMetadata(ctx, volume.ID, metadata)
	if err != nil {
		e := fmt.Errorf("from PutMetadata - volume: %s, error: %s", name, err.Error())
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	slog.Debug("successfully created volume", "volume", name, "volumeID", volumeID)
	return csiResp, err
}

func (nvme *NVMEstorage) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (csiResp *csi.DeleteVolumeResponse, err error) {
	slog.Debug("info", "volume id", req.GetVolumeId())
	volproto := nvme.CS.VolProto
	csiResp, err = storagecommon.DeleteVolume(ctx, nvme.CS, volproto.VolumeID)
	if err != nil {
		return nil, err
	}
	slog.Debug("successfully deleted volume", "volume id", req.GetVolumeId())
	return csiResp, nil
}

func (nvme *NVMEstorage) ControllerModifyVolume(ctx context.Context, req *csi.ControllerModifyVolumeRequest) (resp *csi.ControllerModifyVolumeResponse, err error) {
	return resp, nil
}

func (nvme *NVMEstorage) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (resp *csi.ControllerPublishVolumeResponse, err error) {
	slog.Debug("start", "node id", req.GetNodeId(), "volume id", req.GetVolumeId(),
		"iboxInfo", storagecommon.GetHostInfo(ctx, req.GetSecrets(), nvme.CS.IboxAPI))

	volumeIDString := req.GetVolumeId()
	volproto, err := storagecommon.ValidateVolumeID(volumeIDString)
	if err != nil {
		e := fmt.Errorf("from ValidateVolumeID - volumeID: %s, error: %s", volumeIDString, err.Error())
		slog.Error(e.Error())
		return nil, status.Error(codes.NotFound, e.Error())
	}

	slog.Debug("info", "volume id", volproto.VolumeID)
	volume, err := nvme.CS.IboxAPI.GetVolume(ctx, volproto.VolumeID)
	if err != nil {
		e := fmt.Errorf("from GetVolume - volumeID: %d error: %s", volproto.VolumeID, err.Error())
		slog.Error(e.Error())
		return nil, status.Error(codes.NotFound, e.Error())
	}

	_, err = nvme.CS.AccessModesHelper.IsValidAccessMode(volume, req)
	if err != nil {
		e := fmt.Errorf("from IsValidAccessMode - error: %s", err.Error())
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	hostName, err := storagecommon.DetermineHostName(req.GetNodeId())
	if err != nil {
		e := fmt.Errorf("from DetermineHostName - error: %s", err.Error())
		slog.Error(e.Error())
		return nil, e
	}

	// only nvme protocol uses a hostname suffix like this
	hostName += NVMEHostSuffix
	host, err := nvme.CS.ValidateHost(ctx, hostName)
	if err != nil {
		e := fmt.Errorf("from ValidateHost - error: %s", err.Error())
		slog.Error(e.Error())
		return nil, e
	}
	slog.Debug("found host name", "host name", host.Name, "hostID", host.ID, "ports", host.Ports, "luns", host.Luns)

	var ports string
	if len(host.Ports) > 0 {
		for _, port := range host.Ports {
			if port.Type == "NVME" {
				ports = ports + "," + port.Address
			}
		}
	}
	if ports != "" {
		ports = ports[1:]
	}

	lunList, err := nvme.CS.IboxAPI.GetAllLunByHost(ctx, host.ID)
	if err != nil {
		e := fmt.Errorf("from GetAllLunByHost - host: %s, error: %s", hostName, err.Error())
		slog.Error(e.Error())
		return nil, e
	}
	slog.Debug("got LUNs", "host name", host.Name, "luns", lunList)
	for _, lun := range lunList {
		if lun.VolumeID == volproto.VolumeID {
			publishVolCtxt := map[string]string{
				storagecommon.LunPublishContext:       strconv.Itoa(lun.Lun),
				storagecommon.HostIDPublishContext:    strconv.Itoa(host.ID),
				storagecommon.HostPortsPublishContext: ports,
			}
			slog.Debug("volume already mapped to host", "volume id", volproto.VolumeID, "host name", host.Name, "hostID", host.ID, "lun", lun.Lun, "ports", ports)
			return &csi.ControllerPublishVolumeResponse{
				PublishContext: publishVolCtxt,
			}, nil
		}
	}

	maxVolsPerHostStr := req.GetVolumeContext()[common.StorageClassMaxVolsPerHost]
	if maxVolsPerHostStr != "" {
		maxAllowedVol, err := strconv.Atoi(maxVolsPerHostStr)
		if err != nil {
			e := fmt.Errorf("parse max vols per host - invalid parameter: %s error: %s", common.StorageClassMaxVolsPerHost, err.Error())
			slog.Error(e.Error())
			return nil, e
		}
		if maxAllowedVol < 1 {
			e := fmt.Errorf("parse  max allowed - invalid parameter: %s required to be greater than 0", common.StorageClassMaxVolsPerHost)
			slog.Error(e.Error())
			return nil, e
		}
		slog.Debug("host has volumes mapped", "host name", host.Name, "hostID", host.ID, "luns", len(lunList), "max allowed", maxAllowedVol)
		if len(lunList) >= maxAllowedVol {
			e := fmt.Errorf("max allowed error - unable to publish volume on host: %s, as maximum allowed volume per host: %d, limit reached", host.Name, maxAllowedVol)
			slog.Error(e.Error())
			return nil, status.Error(codes.ResourceExhausted, e.Error())
		}
	}

	// map volume to host
	slog.Debug("mapping volume to host", "volumeID", volproto.VolumeID, "host name", host.Name)
	luninfo, err := nvme.CS.MapVolumeTohost(ctx, volproto.VolumeID, host.ID)
	if err != nil {
		e := fmt.Errorf("failed to map volume to host  - error: %s", err.Error())
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	publishVolCtxt := map[string]string{
		storagecommon.LunPublishContext:       strconv.Itoa(luninfo.Lun),
		storagecommon.HostIDPublishContext:    strconv.Itoa(host.ID),
		storagecommon.HostPortsPublishContext: ports,
	}
	slog.Debug("mapped volume", "volumeID", volproto.VolumeID, "publish context", publishVolCtxt, "nodeID", req.GetNodeId())
	return &csi.ControllerPublishVolumeResponse{
		PublishContext: publishVolCtxt,
	}, nil
}

func (nvme *NVMEstorage) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (resp *csi.ControllerUnpublishVolumeResponse, err error) {
	slog.Debug("start", "nodeID", req.GetNodeId(), "volumeID", req.GetVolumeId())
	host := nvme.CS.VolProto.Host
	slog.Debug("unmapping host's luns", "hostID", host.ID, "host name", host.Name, "luns", len(host.Luns))
	if len(host.Luns) > 0 {
		slog.Debug("unmap volume from host", "volumeID", nvme.CS.VolProto.VolumeID, "hostID", host.ID)
		err = nvme.CS.UnmapVolumeFromHost(ctx, host.ID, nvme.CS.VolProto.VolumeID)
		if err != nil {
			e := fmt.Errorf("from UnmapVolumeFromHost - volumeID: %d hostID: %d - error: %s", nvme.CS.VolProto.VolumeID, host.ID, err.Error())
			slog.Error(e.Error())
			return nil, status.Error(codes.Internal, e.Error())
		}
	}

	// avoid a race condition when there is a single LUN that you just unmapped
	if len(host.Luns) == 1 {
		time.Sleep(2 * time.Second)
	}

	luns, err := nvme.CS.IboxAPI.GetAllLunByHost(ctx, host.ID)
	if err != nil {
		slog.Error("failed to get LUNs for host", "hostID", host.ID, "error", err)
	}
	if len(luns) == 0 {
		err = storagecommon.HostCleanup(ctx, nvme.CS.IboxAPI, host.ID, host.Name+NVMEHostSuffix)
		if err != nil {
			e := fmt.Errorf("from HostCleanup - hostID: %d - error: %s", host.ID, err.Error())
			slog.Error(e.Error())
			return nil, status.Error(codes.Internal, e.Error())
		}
	}

	slog.Debug("completed", "node id", req.GetNodeId(), "volume id", req.GetVolumeId())
	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

func (nvme *NVMEstorage) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (resp *csi.ValidateVolumeCapabilitiesResponse, err error) {
	slog.Error("should not be called, implemented in controller.go instead")
	return
}

func (nvme *NVMEstorage) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (resp *csi.ListVolumesResponse, err error) {
	return &csi.ListVolumesResponse{}, nil
}

func (nvme *NVMEstorage) ListSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (resp *csi.ListSnapshotsResponse, err error) {
	return &csi.ListSnapshotsResponse{}, nil
}

func (nvme *NVMEstorage) GetCapacity(ctx context.Context, req *csi.GetCapacityRequest) (resp *csi.GetCapacityResponse, err error) {
	return &csi.GetCapacityResponse{}, nil
}

func (nvme *NVMEstorage) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (resp *csi.ControllerGetCapabilitiesResponse, err error) {
	return &csi.ControllerGetCapabilitiesResponse{}, nil
}

func (nvme *NVMEstorage) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (resp *csi.CreateSnapshotResponse, err error) {
	var snapshotID string
	snapshotName := req.GetName()
	slog.Debug("called to create snapshot", "snapshot", snapshotName, "source volume id", req.GetSourceVolumeId())

	volumeSnapshot, err := nvme.CS.IboxAPI.GetVolumeByName(ctx, snapshotName)
	if err != nil {
		re, ok := err.(*iboxapi.APIError)
		if ok && re.Code == iboxapi.RESOURCE_NOT_FOUND {
			slog.Debug("snapshot not found", "name", snapshotName)
		} else {
			slog.Error("GetVolumeByName error", "snapshot", snapshotName, "error", err.Error())
			return nil, status.Error(codes.Internal, err.Error())
		}
	} else if volumeSnapshot.ParentID == nvme.CS.VolProto.VolumeID {
		snapshotID = strconv.Itoa(volumeSnapshot.ID) + "$$" + nvme.CS.VolProto.StorageType
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
		e := fmt.Errorf("snapshot: %s ID: %d exists. Different source volume with ID %d: requested",
			snapshotName, volumeSnapshot.ParentID, nvme.CS.VolProto.VolumeID)
		slog.Error(e.Error())
		return nil, status.Error(codes.AlreadyExists, e.Error())
	}

	snapshotParam := iboxapi.CreateSnapshotVolumeRequest{
		ParentID:       nvme.CS.VolProto.VolumeID,
		SnapshotName:   snapshotName,
		WriteProtected: true,
	}

	lockExpiresAtParameter := req.Parameters[common.LockExpiresAtParameter]
	var lockExpiresAt int64
	if lockExpiresAtParameter != "" {
		ntpStatus, err := nvme.CS.IboxAPI.GetNtpStatus(ctx)
		if err != nil {
			e := fmt.Errorf("from GetNtpStatus - error %s", err.Error())
			slog.Error(e.Error())
			return nil, e
		}
		lockExpiresAt, err = storagecommon.ValidateSnapshotLockingParameter(ntpStatus[0].LastProbeTimestamp, lockExpiresAtParameter)
		if err != nil {
			e := fmt.Errorf("from ValidateSnapshotLocking - failed to create snapshot: %s error: %s, invalid lock_expires_at parameter ", snapshotName, err.Error())
			slog.Error(e.Error())
			return nil, e
		}
		slog.Debug("snapshot param", "lockExpiresAtParam", lockExpiresAtParameter, "lockExpiresAt", lockExpiresAt, "timestamp", ntpStatus[0].LastProbeTimestamp)
	}

	snapshotParam.LockExpiresAt = lockExpiresAt

	snapshot, err := nvme.CS.IboxAPI.CreateSnapshotVolume(ctx, snapshotParam)
	if err != nil {
		e := fmt.Errorf("from CreateSnapshotVolume - snapshot: %s error: %s", snapshotName, err.Error())
		slog.Error(e.Error())
		return nil, e
	}

	snapshotID = strconv.Itoa(snapshot.SnapShotID) + "$$" + nvme.CS.VolProto.StorageType
	csiSnapshot := &csi.Snapshot{
		SnapshotId:     snapshotID,
		SourceVolumeId: req.GetSourceVolumeId(),
		ReadyToUse:     true,
		CreationTime:   timestamppb.Now(),
		SizeBytes:      snapshot.Size,
	}
	slog.Debug("CreateFileSystemSnapshot", "response", csiSnapshot)
	snapshotResp := &csi.CreateSnapshotResponse{Snapshot: csiSnapshot}

	slog.Debug("successfully created snapshot", "snapshot name", snapshotName, "source volume id", req.GetSourceVolumeId())
	return snapshotResp, nil
}

func (nvme *NVMEstorage) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (resp *csi.DeleteSnapshotResponse, err error) {
	snapshotID, _ := strconv.Atoi(req.GetSnapshotId())
	slog.Debug("to delete snapshot", "snapshotID", snapshotID)

	_, err = storagecommon.DeleteVolume(ctx, nvme.CS, snapshotID)
	if err != nil {
		return nil, err
	}
	slog.Debug("successfully deleted snapshot", "snapshotID", snapshotID)
	return &csi.DeleteSnapshotResponse{}, nil
}

func (nvme *NVMEstorage) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (resp *csi.ControllerExpandVolumeResponse, err error) {
	volumeID := nvme.CS.VolProto.VolumeID
	slog.Debug("called", "volumeID", volumeID)

	capacity := nvme.Capacity

	// Expand volume size
	volume := iboxapi.Volume{
		Size: capacity,
	}
	_, err = nvme.CS.IboxAPI.UpdateVolume(ctx, volumeID, volume)
	if err != nil {
		slog.Error("UpdateVolume - failed to update file system", "error", err)
		return nil, err
	}
	slog.Debug("volume size updated successfully", "volumeID", volumeID)

	nodeExpansionRequired := true
	if req.GetVolumeCapability().GetBlock() != nil {
		slog.Debug("volume is block so nodeExpansionRequired is false")
		nodeExpansionRequired = false
	}

	return &csi.ControllerExpandVolumeResponse{
		CapacityBytes:         capacity,
		NodeExpansionRequired: nodeExpansionRequired,
	}, nil
}

func (nvme *NVMEstorage) ControllerGetVolume(
	_ context.Context, _ *csi.ControllerGetVolumeRequest,
) (*csi.ControllerGetVolumeResponse, error) {
	// Infinidat does not support ControllerGetVolume
	return nil, status.Error(codes.Unimplemented, "")
}

func (nvme *NVMEstorage) createVolumeFromContentSource(ctx context.Context, req *csi.CreateVolumeRequest, name string, sizeInBytes int64, storagePool string) (*csi.CreateVolumeResponse, error) {
	var msg string

	volumecontent := req.GetVolumeContentSource()
	var volumeContentID string
	var restoreType string
	if volumecontent.GetSnapshot() != nil {
		restoreType = storagecommon.RestoryTypeSnapshot
		volumeContentID = volumecontent.GetSnapshot().GetSnapshotId()
	} else if volumecontent.GetVolume() != nil {
		restoreType = storagecommon.RestoreTypeVolume
		volumeContentID = volumecontent.GetVolume().GetVolumeId()
	}

	slog.Debug("info", "volume content id", volumeContentID, "restore type", restoreType, "size", sizeInBytes)

	// Lookup the snapshot source volume.
	volproto, err := storagecommon.ValidateVolumeID(volumeContentID)
	if err != nil {
		e := fmt.Errorf("failed to validate storage type restoreType: %s source id: %s, error: %s", restoreType, volumeContentID, err.Error())
		slog.Error(e.Error())
		return nil, status.Error(codes.NotFound, e.Error())
	}

	srcVol, err := nvme.CS.IboxAPI.GetVolume(ctx, volproto.VolumeID)
	if err != nil {
		e := fmt.Errorf("from GetVolume id: %d restoreType: %s error: %s", volproto.VolumeID, restoreType, err.Error())
		slog.Error(e.Error())
		return nil, status.Error(codes.NotFound, e.Error())
	}

	// Validate the size is the same.
	if srcVol.Size != sizeInBytes {
		msg := fmt.Sprintf("createVolumeFromContentSource (nvme) - %s %s has incompatible size. size is %d bytes with requested size %d bytes", restoreType, volumeContentID, srcVol.Size, sizeInBytes)
		slog.Error(msg)
		return nil, status.Error(codes.InvalidArgument, msg)
	}

	params := req.GetParameters()

	// Check the storagePool is the same.
	pool, err := nvme.CS.IboxAPI.GetPoolByName(ctx, storagePool)
	if err != nil {
		e := fmt.Errorf("from GetPoolByName name: %s error: %s", storagePool, err.Error())
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	if pool.ID != srcVol.PoolID {
		msg = fmt.Sprintf("createVolumeFromContentSource (nvme) - volume storage pool is different than the requested storage pool %s %d %d", storagePool, pool.ID, srcVol.PoolID)
		slog.Error(msg)
		return nil, status.Error(codes.InvalidArgument, msg)
	}

	// Create snapshot descriptor
	snapshotParam := iboxapi.CreateSnapshotVolumeRequest{
		ParentID:       volproto.VolumeID,
		SnapshotName:   name,
		WriteProtected: false,
		LockExpiresAt:  0,
	}

	// Create snapshot
	snapResponse, err := nvme.CS.IboxAPI.CreateSnapshotVolume(ctx, snapshotParam)
	if err != nil {
		e := fmt.Errorf("from CreateSnapshotVolume - error: %s", err.Error())
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	// Retrieve created destination volume
	volID := snapResponse.SnapShotID
	dstVol, err := nvme.CS.IboxAPI.GetVolume(ctx, volID)
	if err != nil {
		e := fmt.Errorf("from GetVolume - error: %s", err.Error())
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	// Create a volume response and return it
	csiVolume := nvme.CS.GetCSIResponse(ctx, dstVol, req)
	storagecommon.CopyRequestParameters(params, csiVolume.VolumeContext)

	metadata := map[string]interface{}{
		"host.k8s.pvname": dstVol.Name,
	}
	_, err = nvme.CS.IboxAPI.PutMetadata(ctx, dstVol.ID, metadata)
	if err != nil {
		e := fmt.Errorf("from PutMetadata volume: %s, error: %s", dstVol.Name, err.Error())
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	slog.Debug("from", "restore type", restoreType, "volume id", volproto.VolumeID, "name", csiVolume.VolumeContext["Name"], "volume id2", csiVolume.VolumeId, "storage pool", csiVolume.VolumeContext["StoragePoolName"])
	return &csi.CreateVolumeResponse{Volume: csiVolume}, nil
}
