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
	"errors"
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
		common.StorageClassNetworkSpace: `\A.*\z`,
	}
	optionalNVMEParams := map[string]string{}

	// validate required parameters
	err := storagecommon.ValidateRequiredOptionalSCParameters(requiredNVMEParams, optionalNVMEParams, params)
	if err != nil {
		return status.Error(codes.InvalidArgument, common.Errorf("from Validate - error: %w", err).Error())
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
		if errors.Is(err, iboxapi.ErrNotFound) {
			slog.Debug("volume not found, will proceed to create it", "volume", req.GetName())
		} else {
			return nil, status.Error(codes.NotFound, common.Errorf("from GetVolumeByName - error: %w", err).Error())
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
		return storagecommon.CreateVolumeFromVolumeContent(ctx, nvme.CS, req, name, nvme.Capacity, poolName)
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
		return nil, status.Error(codes.Internal, common.Errorf("from GetPoolByName name: %s error: %w", poolName, err).Error())
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
		return nil, status.Error(codes.Internal, common.Errorf("api CreateVolume - creating volume: %s pool: %s error: %w", name, poolName, err).Error())
	}
	csiResponse := nvme.CS.GetCSIResponse(ctx, createVolumeResponse, req)

	// check volume id format
	volumeID, err := strconv.Atoi(csiResponse.VolumeId)
	if err != nil {
		return nil, status.Error(codes.Internal, common.Errorf("parsing volumeID: %s - error: %w", csiResponse.VolumeId, err).Error())
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
		return nil, status.Error(codes.Internal, common.Errorf("from GetVolume - name: %s volumeID: %d", name, volumeID).Error())
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
		return nil, status.Error(codes.Internal, common.Errorf("from PutMetadata - volume: %s, error: %w", name, err).Error())
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
		return nil, status.Error(codes.NotFound, common.Errorf("from ValidateVolumeID - volumeID: %s, error: %w", volumeIDString, err).Error())
	}

	slog.Debug("info", "volume id", volproto.VolumeID)
	volume, err := nvme.CS.IboxAPI.GetVolume(ctx, volproto.VolumeID)
	if err != nil {
		return nil, status.Error(codes.NotFound, common.Errorf("from GetVolume - volumeID: %d error: %w", volproto.VolumeID, err).Error())
	}

	_, err = nvme.CS.AccessModesHelper.IsValidAccessMode(volume, req)
	if err != nil {
		return nil, status.Error(codes.Internal, common.Errorf("from IsValidAccessMode - error: %w", err).Error())
	}

	hostName, err := storagecommon.DetermineHostName(req.GetNodeId())
	if err != nil {
		return nil, common.Errorf("from DetermineHostName - error: %w", err)
	}

	// only nvme protocol uses a hostname suffix like this
	hostName += NVMEHostSuffix
	host, err := nvme.CS.ValidateHost(ctx, hostName)
	if err != nil {
		return nil, common.Errorf("from ValidateHost - error: %w", err)
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
		return nil, common.Errorf("from GetAllLunByHost - host: %s, error: %w", hostName, err)
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
			return nil, common.Errorf("parse max vols per host - invalid parameter: %s error: %w", common.StorageClassMaxVolsPerHost, err)
		}
		if maxAllowedVol < 1 {
			return nil, common.Errorf("parse  max allowed - invalid parameter: %s required to be greater than 0", common.StorageClassMaxVolsPerHost)
		}
		slog.Debug("host has volumes mapped", "host name", host.Name, "hostID", host.ID, "luns", len(lunList), "max allowed", maxAllowedVol)
		if len(lunList) >= maxAllowedVol {
			return nil, status.Error(codes.ResourceExhausted, common.Errorf("max allowed error - unable to publish volume on host: %s, as maximum allowed volume per host: %d, limit reached", host.Name, maxAllowedVol).Error())
		}
	}

	// map volume to host
	slog.Debug("mapping volume to host", "volumeID", volproto.VolumeID, "host name", host.Name)
	luninfo, err := nvme.CS.MapVolumeTohost(ctx, volproto.VolumeID, host.ID)
	if err != nil {
		return nil, status.Error(codes.Internal, common.Errorf("failed to map volume to host  - error: %w", err).Error())
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
	return storagecommon.CommonUnpublishVolume(ctx, req, nvme.CS, NVMEHostSuffix)
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
	return storagecommon.CommonCreateSnapshot(ctx, req, nvme.CS)
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
		slog.Debug("volume is block so nodeExpansionRequired is false when nvme uses native multipath")
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
