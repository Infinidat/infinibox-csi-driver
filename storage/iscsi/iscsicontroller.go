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
package iscsi

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

const (
	SecurityMethodPublishContext = "securityMethod"
)

type ISCSIstorage struct {
	Capacity      int64
	CS            storagecommon.Commonservice
	OSHelper      helper.OsHelper
	StorageHelper storagecommon.StorageHelper
	csi.UnimplementedControllerServer
	csi.UnimplementedNodeServer
}

func NewISCSIstorage(capacity int64, cs storagecommon.Commonservice) (iscsi *ISCSIstorage) {
	iscsi = &ISCSIstorage{
		Capacity:      capacity,
		CS:            cs,
		StorageHelper: storagecommon.StorageService{},
		OSHelper:      helper.Service{},
	}
	return iscsi
}

func (iscsi *ISCSIstorage) ValidateStorageClass(params map[string]string) error {
	requiredISCSIParams := map[string]string{
		common.StorageClassPoolName:     `[a-zA-Z]+`, // match all strings except empty string or blank string
		common.StorageClassUseCHAP:      `(?i)\A(none|chap|mutual_chap)\z`,
		common.StorageClassNetworkSpace: `\A.*\z`,
	}
	optionalISCSIParams := map[string]string{
		common.StorageClassProvisionType: `(?i)\A(THICK|THIN)\z`,
		common.StorageClassUID:           `^\d+$`,
		common.StorageClassGID:           `^\d+$`,
	}

	// validate required parameters
	err := storagecommon.ValidateRequiredOptionalSCParameters(requiredISCSIParams, optionalISCSIParams, params)
	if err != nil {
		e := fmt.Errorf("error from Validate - error: %s", err.Error())
		slog.Error(e.Error())
		return status.Error(codes.InvalidArgument, e.Error())
	}
	return nil
}

func (iscsi *ISCSIstorage) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	params := req.GetParameters()
	slog.Debug("start", "volume", req.GetName(), "size", iscsi.Capacity, "params", params,
		"iboxInfo", storagecommon.GetHostInfo(ctx, req.GetSecrets(), iscsi.CS.IboxAPI))

	// Volume name to be created - already verified earlier
	name := req.GetName()

	poolName := params[common.StorageClassPoolName]

	targetVol, err := iscsi.CS.IboxAPI.GetVolumeByName(ctx, name)
	if err != nil {
		if errors.Is(err, iboxapi.ErrNotFound) {
			slog.Debug("volume not found, going to create", "volume", name)
		} else {
			e := fmt.Errorf("error from GetVolumeByName name: %s error: %s", name, err.Error())
			slog.Error(e.Error())
			return nil, status.Error(codes.NotFound, e.Error())
		}
	}
	if targetVol != nil {
		slog.Debug("volume found", "volume", name, "size", targetVol.Size, "requested", iscsi.Capacity)
		if targetVol.Size == iscsi.Capacity {
			existingVolumeInfo := iscsi.CS.GetCSIResponse(ctx, targetVol, req)
			storagecommon.CopyRequestParameters(params, existingVolumeInfo.VolumeContext)
			return &csi.CreateVolumeResponse{
				Volume: existingVolumeInfo,
			}, nil
		}
		msg := fmt.Sprintf("(iscsi) - failed: volume %s exists but has different size", name)
		slog.Error(msg)
		return nil, status.Errorf(codes.AlreadyExists, "%s", msg)
	}

	// Volume content source support volume and snapshots
	if req.GetVolumeContentSource() != nil {
		return storagecommon.CreateVolumeFromVolumeContent(ctx, iscsi.CS, req, name, iscsi.Capacity, poolName)
	}

	volType, provided := params[common.StorageClassProvisionType]
	if !provided {
		volType = common.StorageClassThinProvision
	}

	volumeParam := &api.VolumeParam{
		VolumeSize:    iscsi.Capacity,
		ProvisionType: volType,
	}

	volumeParam.SSDEnabled, err = storagecommon.DetermineSSDValue(ctx, params[common.StorageClassSSDEnabled], poolName, iscsi.CS.IboxAPI)
	if err != nil {
		e := status.Errorf(codes.Internal, "(iscsi) - determineSSDValue - error when creating volume %s storagepool %s, err: %s", name, poolName, err.Error())
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	pool, err := iscsi.CS.IboxAPI.GetPoolByName(ctx, poolName)
	if err != nil {
		e := status.Errorf(codes.Internal, "(iscsi) - GetPoolByName - error when getting pool %s , err: %s", poolName, err.Error())
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	request := iboxapi.CreateVolumeRequest{
		Name:          name,
		PoolID:        pool.ID,
		VolumeSize:    volumeParam.VolumeSize,
		ProvisionType: volumeParam.ProvisionType,
		SSDEnabled:    volumeParam.SSDEnabled,
	}
	volumeResp, err := iscsi.CS.IboxAPI.CreateVolume(ctx, request)
	if err != nil {
		e := fmt.Errorf("error from CreateVolume - name: %s poolName: %s error: %s", name, poolName, err.Error())
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	csiResponse := iscsi.CS.GetCSIResponse(ctx, volumeResp, req)

	// check volume id format
	volID, err := strconv.Atoi(csiResponse.VolumeId)
	if err != nil {
		e := fmt.Errorf("volumeID conversion error: %s", err.Error())
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	// confirm volume creation
	var vol *iboxapi.Volume
	var counter int
	vol, err = iscsi.CS.IboxAPI.GetVolume(ctx, volID)
	if err != nil {
		e := fmt.Errorf("error from GetVolume - error: %s", err.Error())
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	for vol == nil && counter < 100 {
		time.Sleep(3 * time.Millisecond)
		vol, err = iscsi.CS.IboxAPI.GetVolume(ctx, volID)
		if err != nil {
			e := fmt.Errorf("error from GetVolume - error: %s", err.Error())
			slog.Error(e.Error())
			return nil, status.Error(codes.Internal, e.Error())
		}
		counter++
	}
	if vol == nil {
		e := fmt.Errorf("failed to create volume name: %s ID: %d", name, volID)
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
		"host.k8s.pvname": vol.Name,
	}
	_, err = iscsi.CS.IboxAPI.PutMetadata(ctx, vol.ID, metadata)
	if err != nil {
		e := fmt.Errorf("error from PutMetadata volume: %s, error: %s", name, err.Error())
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	slog.Debug("successfully created volume", "name", name, "ID", volID)
	return csiResp, err
}

func (iscsi *ISCSIstorage) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (csiResp *csi.DeleteVolumeResponse, err error) {
	volproto := iscsi.CS.VolProto
	slog.Debug("start", "volumeID", req.GetVolumeId(), "volproto", volproto)
	csiResp, err = storagecommon.DeleteVolume(ctx, iscsi.CS, volproto.VolumeID)
	if err != nil {
		return nil, err
	}
	slog.Debug("successfully deleted volume", "ID", req.GetVolumeId())
	return csiResp, nil
}

func (iscsi *ISCSIstorage) ControllerModifyVolume(ctx context.Context, req *csi.ControllerModifyVolumeRequest) (resp *csi.ControllerModifyVolumeResponse, err error) {
	return resp, nil
}

func (iscsi *ISCSIstorage) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (resp *csi.ControllerPublishVolumeResponse, err error) {
	slog.Debug("start", "node ID", req.GetNodeId(), "volume ID", req.GetVolumeId(),
		"iboxInfo", storagecommon.GetHostInfo(ctx, req.GetSecrets(), iscsi.CS.IboxAPI))

	volumeIDString := req.GetVolumeId()
	volumePrototype, err := storagecommon.ValidateVolumeID(volumeIDString)
	if err != nil {
		e := fmt.Errorf("error from ValidateVolumeID - volumeID: %s, error: %s", volumeIDString, err.Error())
		slog.Error(e.Error())
		return nil, status.Error(codes.NotFound, e.Error())
	}

	slog.Debug("info", "volID", volumePrototype.VolumeID)
	volume, err := iscsi.CS.IboxAPI.GetVolume(ctx, volumePrototype.VolumeID)
	if err != nil {
		e := fmt.Errorf("error from GetVolume volumeID: %d error: %s", volumePrototype.VolumeID, err.Error())
		slog.Error(e.Error())
		return nil, status.Error(codes.NotFound, e.Error())
	}

	_, err = iscsi.CS.AccessModesHelper.IsValidAccessMode(volume, req)
	if err != nil {
		e := fmt.Errorf("error from IsValidAccessMode volumeID: %d error: %s", volumePrototype.VolumeID, err.Error())
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	hostName, err := storagecommon.DetermineHostName(req.GetNodeId())
	if err != nil {
		e := fmt.Errorf("error from DetermineHostName volumeID: %d error: %s", volumePrototype.VolumeID, err.Error())
		slog.Error(e.Error())
		return nil, e
	}

	host, err := iscsi.CS.ValidateHost(ctx, hostName)
	if err != nil {
		e := fmt.Errorf("error from ValidateHost hostName: %s error: %s", hostName, err.Error())
		slog.Error(e.Error())
		return nil, e
	}
	slog.Debug("found", "host name", host.Name, "id", host.ID, "ports", host.Ports, "LUNs", host.Luns)

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

	lunList, err := iscsi.CS.IboxAPI.GetAllLunByHost(ctx, host.ID)
	if err != nil {
		e := fmt.Errorf("error from GetAllLunByHost hostID: %d error: %s", host.ID, err.Error())
		slog.Error(e.Error())
		return nil, e
	}
	slog.Debug("got LUNs", "host", host.Name, "luns", lunList)
	for _, lun := range lunList {
		if lun.VolumeID == volumePrototype.VolumeID {
			publishVolCtxt := map[string]string{
				storagecommon.LunPublishContext:       strconv.Itoa(lun.Lun),
				storagecommon.HostIDPublishContext:    strconv.Itoa(host.ID),
				storagecommon.HostPortsPublishContext: ports,
			}
			slog.Debug("volume already mapped to host", "volumeID", volumePrototype.VolumeID, "host name", host.Name, "host ID", host.ID, "lun", lun.Lun, "ports", ports)
			return &csi.ControllerPublishVolumeResponse{
				PublishContext: publishVolCtxt,
			}, nil
		}
	}

	maxVolsPerHostStr := req.GetVolumeContext()[common.StorageClassMaxVolsPerHost]
	if maxVolsPerHostStr != "" {
		maxAllowedVol, err := strconv.Atoi(maxVolsPerHostStr)
		if err != nil {
			e := fmt.Errorf("invalid conversion of parameter: %s error: %s", common.StorageClassMaxVolsPerHost, err.Error())
			slog.Error(e.Error())
			return nil, e
		}
		if maxAllowedVol < 1 {
			e := fmt.Errorf("invalid parameter: %s error: required to be greater than 0", common.StorageClassMaxVolsPerHost)
			slog.Error(e.Error())
			return nil, e
		}
		slog.Debug("host has volumes mapped", "host name", host.Name, "host ID", host.ID, "number of luns", len(lunList), "max allowed", maxAllowedVol)
		if len(lunList) >= maxAllowedVol {
			e := fmt.Errorf("unable to publish volume on hostName: %s, as maximum allowed volume per host is: %d, limit reached", host.Name, maxAllowedVol)
			slog.Error(e.Error())
			return nil, status.Error(codes.ResourceExhausted, e.Error())
		}
	}

	// map volume to host
	slog.Debug("mapping volume to host", "volume ID", volumePrototype.VolumeID, "host name", host.Name)
	luninfo, err := iscsi.CS.MapVolumeTohost(ctx, volumePrototype.VolumeID, host.ID)
	if err != nil {
		e := fmt.Errorf("error from mapVolumeToHost hostID: %d error: %s", host.ID, err.Error())
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	publishVolCtxt := map[string]string{
		storagecommon.LunPublishContext:       strconv.Itoa(luninfo.Lun),
		storagecommon.HostIDPublishContext:    strconv.Itoa(host.ID),
		storagecommon.HostPortsPublishContext: ports,
		SecurityMethodPublishContext:          host.SecurityMethod,
	}
	slog.Debug("mapped volume", "volume ID", volumePrototype.VolumeID, "publish context", publishVolCtxt)

	slog.Debug("completed", "node id", req.GetNodeId(), "volume id", req.GetVolumeId())
	return &csi.ControllerPublishVolumeResponse{
		PublishContext: publishVolCtxt,
	}, nil
}

func (iscsi *ISCSIstorage) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (resp *csi.ControllerUnpublishVolumeResponse, err error) {
	return storagecommon.CommonUnpublishVolume(ctx, req, iscsi.CS, "")
}

func (iscsi *ISCSIstorage) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (resp *csi.ValidateVolumeCapabilitiesResponse, err error) {
	slog.Error("should not be called, implemented in controller.go")
	return
}

func (iscsi *ISCSIstorage) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (resp *csi.ListVolumesResponse, err error) {
	return &csi.ListVolumesResponse{}, nil
}

func (iscsi *ISCSIstorage) ListSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (resp *csi.ListSnapshotsResponse, err error) {
	return &csi.ListSnapshotsResponse{}, nil
}

func (iscsi *ISCSIstorage) GetCapacity(ctx context.Context, req *csi.GetCapacityRequest) (resp *csi.GetCapacityResponse, err error) {
	return &csi.GetCapacityResponse{}, nil
}

func (iscsi *ISCSIstorage) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (resp *csi.ControllerGetCapabilitiesResponse, err error) {
	return &csi.ControllerGetCapabilitiesResponse{}, nil
}

func (iscsi *ISCSIstorage) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (resp *csi.CreateSnapshotResponse, err error) {
	return storagecommon.CommonCreateSnapshot(ctx, req, iscsi.CS)
}

func (iscsi *ISCSIstorage) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (resp *csi.DeleteSnapshotResponse, err error) {
	snapshotID, _ := strconv.Atoi(req.GetSnapshotId())
	slog.Debug("start", "ID", snapshotID)

	_, err = storagecommon.DeleteVolume(ctx, iscsi.CS, snapshotID)
	if err != nil {
		e := fmt.Sprintf("error from DeleteVolume - snapshotID: %s error: %s", req.GetSnapshotId(), err.Error())
		slog.Error(e)
		return nil, status.Error(codes.Internal, e)
	}
	slog.Debug("successfully deleted snapshot", "ID", snapshotID)
	return &csi.DeleteSnapshotResponse{}, nil
}

func (iscsi *ISCSIstorage) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (resp *csi.ControllerExpandVolumeResponse, err error) {
	volumeID := iscsi.CS.VolProto.VolumeID
	slog.Debug("start", "volume ID", volumeID)

	capacity := iscsi.Capacity

	// Expand volume size
	volume := iboxapi.Volume{
		Size: capacity,
	}
	_, err = iscsi.CS.IboxAPI.UpdateVolume(ctx, volumeID, volume)
	if err != nil {
		e := fmt.Errorf("error from UpdateVolume error: %s", err.Error())
		slog.Error(e.Error())
		return nil, e
	}
	slog.Debug("volume size updated successfully", "ID", volumeID)
	nodeExpansionRequired := true
	if req.GetVolumeCapability().GetBlock() != nil {
		slog.Debug("volume is block so nodeExpansionRequired is true because multipath resize is required")
	}
	return &csi.ControllerExpandVolumeResponse{
		CapacityBytes:         capacity,
		NodeExpansionRequired: nodeExpansionRequired,
	}, nil
}

func (iscsi *ISCSIstorage) ControllerGetVolume(_ context.Context, _ *csi.ControllerGetVolumeRequest) (*csi.ControllerGetVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}
