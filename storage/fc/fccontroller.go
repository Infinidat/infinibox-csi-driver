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
	"runtime"
	"strconv"
	"time"

	"github.com/infinidat/infinibox-csi-driver/api"
	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/infinidat/infinibox-csi-driver/iboxapi"
	storagecommon "github.com/infinidat/infinibox-csi-driver/storage/common"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
		_, file, line, _ := runtime.Caller(0)
		e := storagecommon.ImplementationError{
			Code: int(codes.InvalidArgument),
			Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
		}
		return e
	}
	return nil
}

func (fc *FCstorage) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	params := req.GetParameters()
	fc.ConfigMap = params
	slog.Debug("start", "requested volume parameters", params, "requested size", fc.Capacity,
		"iboxInfo", storagecommon.GetHostInfo(ctx, req.GetSecrets(), fc.CS.IboxAPI))

	// Volume name to be created - already verified in controller.go
	name := req.GetName()

	poolName := params[common.StorageClassPoolName]

	targetVol, err := fc.CS.IboxAPI.GetVolumeByName(ctx, name)
	if err != nil {
		if errors.Is(err, iboxapi.ErrNotFound) {
			slog.Debug("volume with name not found, proceeding to create", "name", name)
		} else {
			_, file, line, _ := runtime.Caller(0)
			e := storagecommon.ImplementationError{
				Code: int(codes.Internal),
				Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
			}
			return nil, e
		}
	}

	if targetVol != nil {
		slog.Debug("volume: found", "volume", name, "size", targetVol.Size, "requested:", fc.Capacity)
		if targetVol.Size == fc.Capacity {
			existingVolumeInfo := fc.CS.GetCSIResponse(ctx, targetVol, req)
			storagecommon.CopyRequestParameters(params, existingVolumeInfo.VolumeContext)
			return &csi.CreateVolumeResponse{
				Volume: existingVolumeInfo,
			}, nil
		}
		_, file, line, _ := runtime.Caller(0)
		e := storagecommon.ImplementationError{
			Code: int(codes.AlreadyExists),
			Msg:  fmt.Sprintf("%s:%d: %s", file, line, fmt.Sprintf("volume: %s already exists with a different size", name)),
		}
		return nil, e
	}

	// Volume content source support volume and snapshots
	contentSource := req.GetVolumeContentSource()
	if contentSource != nil {
		return storagecommon.CreateVolumeFromVolumeContent(ctx, fc.CS, req, name, fc.Capacity, poolName)
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

	pool, err := fc.CS.IboxAPI.GetPoolByName(ctx, poolName)
	if err != nil {
		_, file, line, _ := runtime.Caller(0)
		e := storagecommon.ImplementationError{
			Code: int(codes.Internal),
			Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
		}
		return nil, e
	}

	volumeParam.SSDEnabled, err = storagecommon.DetermineSSDValue(ctx, params[common.StorageClassSSDEnabled], *pool)
	if err != nil {
		_, file, line, _ := runtime.Caller(0)
		e := storagecommon.ImplementationError{
			Code: int(codes.Internal),
			Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
		}
		return nil, e
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
		_, file, line, _ := runtime.Caller(0)
		e := storagecommon.ImplementationError{
			Code: int(codes.Internal),
			Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
		}
		return nil, e
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
		slog.Error("fc GetVolume", "error", err.Error())
	}

	// a single test just in case there is a race condition on createVolume (doubtful)
	if vol == nil {
		time.Sleep(3 * time.Second)
		_, err = fc.CS.IboxAPI.GetVolume(ctx, volumeResp.ID)
		if err != nil {
			_, file, line, _ := runtime.Caller(0)
			e := storagecommon.ImplementationError{
				Code: int(codes.Internal),
				Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
			}
			return nil, e
		}
	}

	// Prepare response struct
	storagecommon.CopyRequestParameters(params, newVolume.VolumeContext)
	csiResp := &csi.CreateVolumeResponse{
		Volume: newVolume,
	}

	// attach metadata to volume object
	metadata := map[string]any{
		"host.k8s.pvname": volumeResp.Name,
	}
	_, err = fc.CS.IboxAPI.PutMetadata(ctx, volumeResp.ID, metadata)
	if err != nil {
		_, file, line, _ := runtime.Caller(0)
		e := storagecommon.ImplementationError{
			Code: int(codes.Internal),
			Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
		}
		return nil, e
	}

	slog.Debug("created volume", "name", name, "id", volumeResp.ID)
	return csiResp, err
}

func (fc *FCstorage) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (csiResp *csi.DeleteVolumeResponse, err error) {
	slog.Debug("start", "id", req.GetVolumeId())
	csiResp, err = storagecommon.DeleteVolume(ctx, fc.CS, fc.CS.VolProto.VolumeID)
	if err != nil {
		_, file, line, _ := runtime.Caller(0)
		e := storagecommon.ImplementationError{
			Code: int(codes.Internal),
			Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
		}
		return nil, e
	}
	slog.Debug("deleted volume", "volume id", req.GetVolumeId())
	return csiResp, nil
}

func (fc *FCstorage) ControllerModifyVolume(ctx context.Context, req *csi.ControllerModifyVolumeRequest) (resp *csi.ControllerModifyVolumeResponse, err error) {
	return resp, nil
}

func (fc *FCstorage) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (resp *csi.ControllerPublishVolumeResponse, err error) {
	slog.Debug("start", "nodeID", req.GetNodeId(), "volumeId", req.GetVolumeId(), "iboxInfo", storagecommon.GetHostInfo(ctx, req.GetSecrets(), fc.CS.IboxAPI))
	volproto, err := storagecommon.ValidateVolumeID(req.GetVolumeId())
	if err != nil {
		_, file, line, _ := runtime.Caller(0)
		e := storagecommon.ImplementationError{
			Code: int(codes.Internal),
			Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
		}
		return nil, e
	}

	hostName, err := storagecommon.DetermineHostName(req.GetNodeId())
	if err != nil {
		_, file, line, _ := runtime.Caller(0)
		e := storagecommon.ImplementationError{
			Code: int(codes.Internal),
			Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
		}
		return nil, e
	}

	host, err := fc.CS.ValidateHost(ctx, hostName)
	if err != nil {
		_, file, line, _ := runtime.Caller(0)
		e := storagecommon.ImplementationError{
			Code: int(codes.Internal),
			Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
		}
		return nil, e
	}

	volume, err := fc.CS.IboxAPI.GetVolume(ctx, volproto.VolumeID)
	if err != nil {
		_, file, line, _ := runtime.Caller(0)
		e := storagecommon.ImplementationError{
			Code: int(codes.Internal),
			Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
		}
		return nil, e
	}

	_, err = fc.CS.AccessModesHelper.IsValidAccessMode(volume, req)
	if err != nil {
		_, file, line, _ := runtime.Caller(0)
		e := storagecommon.ImplementationError{
			Code: int(codes.Internal),
			Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
		}
		return nil, e
	}

	lunList, err := fc.CS.IboxAPI.GetAllLunByHost(ctx, host.ID)
	if err != nil {
		_, file, line, _ := runtime.Caller(0)
		e := storagecommon.ImplementationError{
			Code: int(codes.Internal),
			Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
		}
		return nil, e
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
			slog.Debug("info", "volume Name", volume.Name, "volumeID", lun.VolumeID, "already mapped to host", host.Name)
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
			_, file, line, _ := runtime.Caller(0)
			e := storagecommon.ImplementationError{
				Code: int(codes.Internal),
				Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
			}
			return nil, e
		}
		if maxAllowedVol < 1 {
			_, file, line, _ := runtime.Caller(0)
			e := storagecommon.ImplementationError{
				Code: int(codes.Internal),
				Msg:  fmt.Sprintf("%s:%d: %s", file, line, fmt.Sprintf("invalid parameter: %s error: required to be greater than 0", common.StorageClassMaxVolsPerHost)),
			}
			return nil, e
		}
		slog.Debug("host can have maximum volume mapped", "value", maxAllowedVol)
		slog.Debug("volume mapped", "host", host.Name, "mapped", len(lunList))
		if len(lunList) >= maxAllowedVol {
			_, file, line, _ := runtime.Caller(0)
			e := storagecommon.ImplementationError{
				Code: int(codes.ResourceExhausted),
				Msg:  fmt.Sprintf("%s:%d: %s", file, line, fmt.Sprintf("unable to publish volume on hostName: %s, maximum allowed volume per host: %d, limit reached", host.Name, maxAllowedVol)),
			}
			return nil, e
		}
	}
	// map volume to host
	slog.Debug("mapping volume", "Name", volume.Name, "volume ID", volproto.VolumeID, "to host", host.Name)
	luninfo, err := fc.CS.MapVolumeTohost(ctx, volproto.VolumeID, host.ID)
	if err != nil {
		_, file, line, _ := runtime.Caller(0)
		e := storagecommon.ImplementationError{
			Code: int(codes.Internal),
			Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
		}
		return nil, e
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
	return storagecommon.CommonUnpublishVolume(ctx, req, fc.CS, "")
}

func (fc *FCstorage) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (resp *csi.ValidateVolumeCapabilitiesResponse, err error) {
	slog.Error("should not be called, implemented in controller.go instead")
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
	return storagecommon.CommonCreateSnapshot(ctx, req, fc.CS)
}

func (fc *FCstorage) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (resp *csi.DeleteSnapshotResponse, err error) {
	snapshotID, _ := strconv.Atoi(req.GetSnapshotId())

	_, err = storagecommon.DeleteVolume(ctx, fc.CS, snapshotID)
	if err != nil {
		_, file, line, _ := runtime.Caller(0)
		e := storagecommon.ImplementationError{
			Code: int(codes.Internal),
			Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
		}
		return nil, e
	}
	return &csi.DeleteSnapshotResponse{}, nil
}

func (fc *FCstorage) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (resp *csi.ControllerExpandVolumeResponse, err error) {
	volumeID := fc.CS.VolProto.VolumeID
	slog.Debug("start", "volume ID", volumeID)

	capacity := fc.Capacity

	// Expand volume size
	volume := iboxapi.Volume{
		Size: capacity,
	}
	_, err = fc.CS.IboxAPI.UpdateVolume(ctx, volumeID, volume)
	if err != nil {
		_, file, line, _ := runtime.Caller(0)
		e := storagecommon.ImplementationError{
			Code: int(codes.Internal),
			Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
		}
		return nil, e
	}
	slog.Debug("volume size updated successfully", "volume ID", volumeID)
	nodeExpansionRequired := true
	if req.GetVolumeCapability().GetBlock() != nil {
		slog.Debug("volume is block so nodeExpansionRequired is true due to multipath resize is required")
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
