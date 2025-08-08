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

const NVME_HOST_SUFFIX = "-nvme"

func (nvme *nvmestorage) ValidateStorageClass(params map[string]string) error {
	requiredNVMEParams := map[string]string{
		common.SC_POOL_NAME:     `[a-zA-Z]+`, //match all strings except empty string or blank string
		common.SC_NETWORK_SPACE: `\A.*\z`,    // TODO: could make this enforce IBOX network_space requirements, but probably not necessary
	}
	optionalNVMEParams := map[string]string{}

	// validate required parameters
	err := ValidateRequiredOptionalSCParameters(requiredNVMEParams, optionalNVMEParams, params)
	if err != nil {
		e := fmt.Errorf("ValidateStorageClass (nvme) - Validate - error: %s", err.Error())
		zlog.Error().Msg(e.Error())
		return status.Error(codes.InvalidArgument, e.Error())
	}
	return nil
}

func (nvme *nvmestorage) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {

	params := req.GetParameters()

	zlog.Debug().Msgf("CreateVolume (nvme) - volume: %s of size: %d bytes, params: %v", req.GetName(), nvme.capacity, params)

	// Volume name to be created - already verified earlier
	name := req.GetName()

	poolName := params[common.SC_POOL_NAME]

	targetVol, err := nvme.cs.IboxApi.GetVolumeByName(name)
	if err != nil {
		re, ok := err.(*iboxapi.IboxAPIError)
		if ok && re.Code == iboxapi.IBOXAPI_RESOURCE_NOT_FOUND_ERROR {
			zlog.Debug().Msgf("CreateVolume (nvme) - volume: %s not found, will proceed to create it", req.GetName())
		} else {
			e := fmt.Errorf("CreateVolume (nvme) - GetVolumeByName - error %s", err.Error())
			zlog.Error().Msg(e.Error())
			return nil, status.Error(codes.NotFound, e.Error())
		}
	}
	if targetVol != nil {
		zlog.Debug().Msgf("CreateVolume (nvme) - volume: %s found, size: %d requested: %d", name, targetVol.Size, nvme.capacity)
		if targetVol.Size == nvme.capacity {
			existingVolumeInfo := nvme.cs.getCSIResponse(targetVol, req)
			copyRequestParameters(params, existingVolumeInfo.VolumeContext)
			return &csi.CreateVolumeResponse{
				Volume: existingVolumeInfo,
			}, nil
		}
		msg := fmt.Sprintf("CreateVolume (nvme) - failed: volume %s exists but has different size", name)
		zlog.Error().Msg(msg)
		return nil, status.Error(codes.AlreadyExists, msg)
	}

	// Volume content source support volume and snapshots
	contentSource := req.GetVolumeContentSource()
	if contentSource != nil {
		return nvme.createVolumeFromContentSource(req, name, nvme.capacity, poolName)
	}

	volType, provided := params[common.SC_PROVISION_TYPE]
	if !provided {
		volType = common.SC_THIN_PROVISION_TYPE
	}

	volumeParam := &api.VolumeParam{
		Name:          name,
		VolumeSize:    nvme.capacity,
		ProvisionType: volType,
	}

	volumeParam.SsdEnabled, err = determineSSDValue(params[common.SC_SSD_ENABLED], poolName, nvme.cs.IboxApi)
	if err != nil {
		e := status.Errorf(codes.Internal, "CreateVolume (nvme) - determineSSDValue - error when creating volume %s storagepool %s, err: %s", name, poolName, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, e
	}

	pool, err := nvme.cs.IboxApi.GetPoolByName(poolName)
	if err != nil {
		e := fmt.Errorf("CreateVolume (nvme) - GetPoolByName name: %s error: %v", poolName, err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	request := iboxapi.CreateVolumeRequest{
		Name:          volumeParam.Name,
		PoolId:        pool.ID,
		SsdEnabled:    volumeParam.SsdEnabled,
		ProvisionType: volumeParam.ProvisionType,
		VolumeSize:    volumeParam.VolumeSize,
	}
	volumeResp, err := nvme.cs.IboxApi.CreateVolume(request)
	if err != nil {
		e := fmt.Errorf("CreatVolume (nvme) - api CreateVolume - creating volume: %s pool %s error: %v", name, poolName, err)
		zlog.Err(e)
		return nil, status.Error(codes.Internal, e.Error())
	}
	vi := nvme.cs.getCSIResponse(volumeResp, req)

	// check volume id format
	volID, err := strconv.Atoi(vi.VolumeId)
	if err != nil {
		e := fmt.Errorf("CreateVolume (nvme) - parsing volume id %s - error: %s", vi.VolumeId, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	MAX_TRIES := 10
	var vol *iboxapi.Volume
	for i := 0; i < MAX_TRIES; i++ {
		vol, err = nvme.cs.IboxApi.GetVolume(volID)
		if err == nil {
			zlog.Debug().Msgf("CreateVolume (nvme) - volume: %s found", vol.Name)
			break
		}
		zlog.Debug().Msgf("CreateVolume (nvme) - volume: %d not found, trying again after 1 second", volID)
		time.Sleep(1 * time.Second)
	}
	if vol == nil {
		e := fmt.Errorf("CreateVolume (nvme) - GetVolume - failed to create volume name: %s volume not retrieved for id: %d", name, volID)
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
	_, err = nvme.cs.IboxApi.PutMetadata(vol.ID, metadata)
	if err != nil {
		e := fmt.Errorf("CreateVolume (nvme) - PutMetadata - failed to attach metadata for volume : %s, err: %v", name, err)
		zlog.Err(e)
		return nil, status.Error(codes.Internal, e.Error())
	}

	zlog.Debug().Msgf("CreateVolume (nvme) - successfully created volume with name %s and ID %d", name, volID)
	return csiResp, err
}

func (nvme *nvmestorage) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (csiResp *csi.DeleteVolumeResponse, err error) {
	zlog.Debug().Msgf("DeleteVolume (nvme) - volumeID %s", req.GetVolumeId())
	volproto := nvme.cs.VolProto
	err = nvme.ValidateDeleteVolume(volproto.VolumeID)
	if err != nil {
		re, ok := err.(*iboxapi.IboxAPIError)
		if ok && re.Code == iboxapi.IBOXAPI_RESOURCE_NOT_FOUND_ERROR {
			return &csi.DeleteVolumeResponse{}, nil
		} else {
			e := fmt.Errorf("DeleteVolume (nvme) - validateDeleteVolume - failed to delete volume: %s", err.Error())
			zlog.Error().Msg(e.Error())
			return nil, status.Error(codes.Internal, e.Error())
		}
	}
	zlog.Debug().Msgf("DeleteVolume (nvme) - successfully deleted volume with ID %s", req.GetVolumeId())
	return &csi.DeleteVolumeResponse{}, nil
}

func (nvme *nvmestorage) createVolumeFromContentSource(req *csi.CreateVolumeRequest, name string, sizeInBytes int64, storagePool string) (*csi.CreateVolumeResponse, error) {
	var msg string

	volumecontent := req.GetVolumeContentSource()
	var volumeContentID string
	var restoreType string
	if volumecontent.GetSnapshot() != nil {
		restoreType = RESTORE_TYPE_SNAPSHOT
		volumeContentID = volumecontent.GetSnapshot().GetSnapshotId()
	} else if volumecontent.GetVolume() != nil {
		restoreType = RESTORE_TYPE_VOLUME
		volumeContentID = volumecontent.GetVolume().GetVolumeId()
	}

	zlog.Debug().Msgf("createVolumeFromContentSource (nvme) source ID: %s type: %s size: %d B", volumeContentID, restoreType, sizeInBytes)

	// Lookup the snapshot source volume.
	volproto, err := ValidateVolumeID(volumeContentID)
	if err != nil {
		e := fmt.Errorf("createVolumeFromContentSource (nvme) - failed to validate storage type restoreType: %s source id: %s, err: %v", restoreType, volumeContentID, err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.NotFound, e.Error())
	}

	srcVol, err := nvme.cs.IboxApi.GetVolume(volproto.VolumeID)
	if err != nil {
		e := fmt.Errorf("createVolumeFromContentSource (nvme) - error GetVolume id: %d restoreType: %s error: %v", volproto.VolumeID, restoreType, err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.NotFound, e.Error())
	}

	// Validate the size is the same.
	if int64(srcVol.Size) != sizeInBytes {
		msg := fmt.Sprintf("createVolumeFromContentSource (nvme) - %s %s has incompatible size. size is %d bytes with requested size %d bytes", restoreType, volumeContentID, srcVol.Size, sizeInBytes)
		zlog.Error().Msg(msg)
		return nil, status.Error(codes.InvalidArgument, msg)
	}

	params := req.GetParameters()

	// Check the storagePool is the same.
	pool, err := nvme.cs.IboxApi.GetPoolByName(storagePool)
	if err != nil {
		e := fmt.Errorf("createVolumeFromContentSource (nvme) - error GetPoolByName name: %s error: %v", storagePool, err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	if pool.ID != srcVol.PoolId {
		msg = fmt.Sprintf("createVolumeFromContentSource (nvme) - volume storage pool is different than the requested storage pool %s %d %d", storagePool, pool.ID, srcVol.PoolId)
		zlog.Error().Msg(msg)
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
	snapResponse, err := nvme.cs.IboxApi.CreateSnapshotVolume(snapshotParam)
	if err != nil {
		e := fmt.Errorf("createVolumeFromContentSource (nvme) - CreateSnapshotVolume - error: %s", err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	// Retrieve created destination volume
	volID := snapResponse.SnapShotID
	dstVol, err := nvme.cs.IboxApi.GetVolume(volID)
	if err != nil {
		e := fmt.Errorf("createVolumeFromContentSource (nvme) - GetVolume - error: %s", err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	// Create a volume response and return it
	csiVolume := nvme.cs.getCSIResponse(dstVol, req)
	copyRequestParameters(params, csiVolume.VolumeContext)

	metadata := map[string]interface{}{
		"host.k8s.pvname": dstVol.Name,
	}
	_, err = nvme.cs.IboxApi.PutMetadata(dstVol.ID, metadata)
	if err != nil {
		e := fmt.Errorf("createVolumeFromContentSource (nvme) - error attach metadata for volume : %s, err: %v", dstVol.Name, err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	zlog.Debug().Msgf("createVolumeFromContentSource (nvme) - from source %s with ID %d, created volume %s with ID %s in storage pool %s",
		restoreType, volproto.VolumeID, csiVolume.VolumeContext["Name"], csiVolume.VolumeId, csiVolume.VolumeContext["StoragePoolName"])
	return &csi.CreateVolumeResponse{Volume: csiVolume}, nil
}

func (nvme *nvmestorage) ControllerModifyVolume(ctx context.Context, req *csi.ControllerModifyVolumeRequest) (resp *csi.ControllerModifyVolumeResponse, err error) {
	return nil, nil
}

func (nvme *nvmestorage) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (resp *csi.ControllerPublishVolumeResponse, err error) {
	zlog.Debug().Msgf("ControllerPublishVolume (nvme) - node ID: %s volume ID: %s", req.GetNodeId(), req.GetVolumeId())

	volIdStr := req.GetVolumeId()
	volproto, err := ValidateVolumeID(volIdStr)
	if err != nil {
		e := fmt.Errorf("ControllerPublishVolume (nvme) - - ValidateVolumeID - failed to validate storage type for volume ID: %s, err: %v", volIdStr, err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.NotFound, e.Error())
	}

	zlog.Debug().Msgf("volID: %d", volproto.VolumeID)
	v, err := nvme.cs.IboxApi.GetVolume(volproto.VolumeID)
	if err != nil {
		e := fmt.Errorf("ControllerPublishVolume (nvme) - - GetVolume - failed to find volume by volume ID '%d': %v", volproto.VolumeID, err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.NotFound, e.Error())
	}

	_, err = nvme.cs.AccessModesHelper.IsValidAccessMode(v, req)
	if err != nil {
		e := fmt.Errorf("ControllerPublishVolume (nvme) - - IsValidAccessMode - error: %s", err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	hostName, err := DetermineHostName(req.GetNodeId())
	if err != nil {
		e := fmt.Errorf("ControllerPublishVolume (nvme) - - DetermineHostName - error: %s", err.Error())
		zlog.Error().Msg(e.Error())
		return nil, e
	}

	// only nvme protocol uses a hostname suffix like this
	hostName = hostName + NVME_HOST_SUFFIX
	host, err := nvme.cs.validateHost(hostName)
	if err != nil {
		e := fmt.Errorf("ControllerPublishVolume (nvme) - - validateHost - error: %s", err.Error())
		zlog.Error().Msg(e.Error())
		return nil, e
	}
	zlog.Debug().Msgf("ControllerPublishVolume (nvme) - found host name: %s id: %d ports: %v LUNs: %v", host.Name, host.ID, host.Ports, host.Luns)

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

	lunList, err := nvme.cs.IboxApi.GetAllLunByHost(host.ID)
	if err != nil {
		e := fmt.Errorf("ControllerPublishVolume (nvme) - GetAllLunByHost - failed to GetAllLunByHost() for host: %s, error: %v", hostName, err)
		zlog.Err(e)
		return nil, e
	}
	zlog.Debug().Msgf("ControllerPublishVolume (nvme) - got LUNs for host: %s, LUNs: %+v", host.Name, lunList)
	for _, lun := range lunList {
		if lun.VolumeID == volproto.VolumeID {
			publishVolCtxt := map[string]string{
				LUN_PUBLISH_CONTEXT:        strconv.Itoa(lun.Lun),
				HOST_ID_PUBLISH_CONTEXT:    strconv.Itoa(host.ID),
				HOST_PORTS_PUBLISH_CONTEXT: ports,
			}
			zlog.Debug().Msgf("ControllerPublishVolume (nvme) - vol: %d already mapped to host:%s id:%d as LUN: %d at ports: %s", volproto.VolumeID, host.Name, host.ID, lun.Lun, ports)
			return &csi.ControllerPublishVolumeResponse{
				PublishContext: publishVolCtxt,
			}, nil
		}
	}

	maxVolsPerHostStr := req.GetVolumeContext()[common.SC_MAX_VOLS_PER_HOST]
	if maxVolsPerHostStr != "" {
		maxAllowedVol, err := strconv.Atoi(maxVolsPerHostStr)
		if err != nil {
			e := fmt.Errorf("ControllerPublishVolume (nvme) - parse max vols per host - invalid parameter %s error:  %v", common.SC_MAX_VOLS_PER_HOST, err)
			zlog.Err(e)
			return nil, e
		}
		if maxAllowedVol < 1 {
			e := fmt.Errorf("ControllerPublishVolume (nvme) - parse  max allowed - invalid parameter %s error:  required to be greater than 0", common.SC_MAX_VOLS_PER_HOST)
			zlog.Err(e)
			return nil, e
		}
		zlog.Debug().Msgf("ControllerPublishVolume (nvme) - host can have maximum %d volume mapped", maxAllowedVol)
		zlog.Debug().Msgf("ControllerPublishVolume (nvme) - host %s id: %d has %d volumes mapped", host.Name, host.ID, len(lunList))
		if len(lunList) >= maxAllowedVol {
			e := fmt.Errorf("ControllerPublishVolume (nvme) - max allowed error - unable to publish volume on host %s, as maximum allowed volume per host is (%d), limit reached", host.Name, maxAllowedVol)
			zlog.Err(e)
			return nil, status.Error(codes.ResourceExhausted, e.Error())
		}
	}

	// map volume to host
	zlog.Debug().Msgf("ControllerPublishVolume (nvme) - mapping volume %d to host %s", volproto.VolumeID, host.Name)
	luninfo, err := nvme.cs.mapVolumeTohost(volproto.VolumeID, host.ID)
	if err != nil {
		e := fmt.Errorf("ControllerPublishVolume (nvme) - mapVolumeToHost - failed to map volume to host with error %v", err)
		zlog.Err(e)
		return nil, status.Error(codes.Internal, e.Error())
	}

	publishVolCtxt := map[string]string{
		LUN_PUBLISH_CONTEXT:        strconv.Itoa(luninfo.Lun),
		HOST_ID_PUBLISH_CONTEXT:    strconv.Itoa(host.ID),
		HOST_PORTS_PUBLISH_CONTEXT: ports,
	}
	zlog.Debug().Msgf("ControllerPublishVolume (nvme) - mapped volume %d, publish context: %v", volproto.VolumeID, publishVolCtxt)

	zlog.Debug().Msgf("ControllerPublishVolume (nvme) - completed node ID: %s volume ID: %s", req.GetNodeId(), req.GetVolumeId())
	return &csi.ControllerPublishVolumeResponse{
		PublishContext: publishVolCtxt,
	}, nil
}

func (nvme *nvmestorage) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (resp *csi.ControllerUnpublishVolumeResponse, err error) {
	zlog.Debug().Msgf("ControllerUnpublishVolume (nvme) - volproto %+v node ID: %s volume ID: %s", nvme.cs.VolProto, req.GetNodeId(), req.GetVolumeId())
	host := nvme.cs.VolProto.Host
	zlog.Debug().Msgf("ControllerUnpublishVolume (nvme) - unmapping host's luns: host id: %d, name: %s lun count %d", host.ID, host.Name, len(host.Luns))
	if len(host.Luns) > 0 {
		zlog.Debug().Msgf("ControllerUnpublishVolume (nvme) - unmap volume %d from host %d", nvme.cs.VolProto.VolumeID, host.ID)
		err = nvme.cs.unmapVolumeFromHost(host.ID, int(nvme.cs.VolProto.VolumeID))
		if err != nil {
			e := fmt.Errorf("ControllerUnpublishVolume (nvme) - unmapVolumeFromHost - failed to unmap volume with ID %d from host with ID %d. Error: %v", nvme.cs.VolProto.VolumeID, host.ID, err)
			zlog.Err(e)
			return nil, status.Error(codes.Internal, e.Error())
		}
	}

	// avoid a race condition when there is a single LUN that you just unmapped
	if len(host.Luns) == 1 {
		time.Sleep(2 * time.Second)
	}

	luns, err := nvme.cs.IboxApi.GetAllLunByHost(host.ID)
	if err != nil {
		zlog.Error().Msgf("ControllerUnpublishVolume (nvme) - failed to get LUNs for host with ID %d. Error: %v", host.ID, err)
	}
	if len(luns) == 0 {
		err = hostCleanup(nvme.cs.IboxApi, host.ID, host.Name+NVME_HOST_SUFFIX)
		if err != nil {
			e := fmt.Errorf("ControllerUnpublishVolume (nvme) - hostCleanup - failed to perform hostCleanup for host ID %d. Error: %s", host.ID, err.Error())
			zlog.Err(e)
			return nil, status.Error(codes.Internal, e.Error())
		}
	}

	zlog.Debug().Msgf("ControllerUnpublishVolume (nvme) - completed with node ID %s and volume ID %s", req.GetNodeId(), req.GetVolumeId())
	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

func (nvme *nvmestorage) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (resp *csi.ValidateVolumeCapabilitiesResponse, err error) {
	zlog.Error().Msgf("ValidateVolumeCapabilities (nvme) - should not be called, implemented in controller.go instead")
	return
}

func (nvme *nvmestorage) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (resp *csi.ListVolumesResponse, err error) {
	return &csi.ListVolumesResponse{}, nil
}

func (nvme *nvmestorage) ListSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (resp *csi.ListSnapshotsResponse, err error) {
	return &csi.ListSnapshotsResponse{}, nil
}

func (nvme *nvmestorage) GetCapacity(ctx context.Context, req *csi.GetCapacityRequest) (resp *csi.GetCapacityResponse, err error) {
	return &csi.GetCapacityResponse{}, nil
}

func (nvme *nvmestorage) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (resp *csi.ControllerGetCapabilitiesResponse, err error) {
	return &csi.ControllerGetCapabilitiesResponse{}, nil
}

func (nvme *nvmestorage) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (resp *csi.CreateSnapshotResponse, err error) {
	var snapshotID string
	snapshotName := req.GetName()
	zlog.Debug().Msgf("CreateSnapshot (nvme) - called to create snapshot named %s from source volume ID %s", snapshotName, req.GetSourceVolumeId())

	volumeSnapshot, err := nvme.cs.IboxApi.GetVolumeByName(snapshotName)
	if err != nil {
		re, ok := err.(*iboxapi.IboxAPIError)
		if ok && re.Code == iboxapi.IBOXAPI_RESOURCE_NOT_FOUND_ERROR {
			zlog.Debug().Msgf("CreateSnapshot (nvme) - snapshot with name %s not found", snapshotName)
		} else {
			zlog.Error().Msgf("CreateSnapshot  (nvme) - GetVolumeByName - snapshot %s error: %s", snapshotName, err.Error())
			return nil, status.Error(codes.Internal, err.Error())
		}
	} else if volumeSnapshot.ParentId == nvme.cs.VolProto.VolumeID {
		snapshotID = strconv.Itoa(volumeSnapshot.ID) + "$$" + nvme.cs.VolProto.StorageType
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
		e := fmt.Errorf("CreateSnapshot (nvme) - snapshot named %s with ID %d exists. Different source volume with ID %d requested",
			snapshotName, volumeSnapshot.ParentId, nvme.cs.VolProto.VolumeID)
		zlog.Err(e)
		return nil, status.Error(codes.AlreadyExists, e.Error())
	}

	snapshotParam := iboxapi.CreateSnapshotVolumeRequest{
		ParentID:       nvme.cs.VolProto.VolumeID,
		SnapshotName:   snapshotName,
		WriteProtected: true,
	}

	lockExpiresAtParameter := req.Parameters[common.LOCK_EXPIRES_AT_PARAMETER]
	var lockExpiresAt int64
	if lockExpiresAtParameter != "" {
		ntpStatus, err := nvme.cs.IboxApi.GetNtpStatus()
		if err != nil {
			e := fmt.Errorf("CreateSnapshot (nvme) - GetNtpStatus - error %s", err.Error())
			zlog.Error().Msg(e.Error())
			return nil, e
		}
		lockExpiresAt, err = validateSnapshotLockingParameter(ntpStatus[0].LastProbeTimestamp, lockExpiresAtParameter)
		if err != nil {
			e := fmt.Errorf("CreateSnapshot (nvme) - validateSnapshotLocking - failed to create snapshot %s error %v, invalid lock_expires_at parameter ", snapshotName, err)
			zlog.Error().Msg(e.Error())
			return nil, e
		}
		zlog.Debug().Msgf("CreateSnapshot (nvme) - snapshot param has a lock_expires_at of %s int value %d, start time on ibox is %d", lockExpiresAtParameter, lockExpiresAt, ntpStatus[0].LastProbeTimestamp)
	}

	snapshotParam.LockExpiresAt = lockExpiresAt

	snapshot, err := nvme.cs.IboxApi.CreateSnapshotVolume(snapshotParam)
	if err != nil {
		e := fmt.Errorf("CreateSnapshot (nvme) - CreateSnapshotVolume - snapshot %s error %s", snapshotName, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, e
	}

	snapshotID = strconv.Itoa(snapshot.SnapShotID) + "$$" + nvme.cs.VolProto.StorageType
	csiSnapshot := &csi.Snapshot{
		SnapshotId:     snapshotID,
		SourceVolumeId: req.GetSourceVolumeId(),
		ReadyToUse:     true,
		CreationTime:   timestamppb.Now(),
		SizeBytes:      snapshot.Size,
	}
	zlog.Debug().Msgf("CreateSnapshot (nvme) - CreateFileSystemSnapshot resp: %v", csiSnapshot)
	snapshotResp := &csi.CreateSnapshotResponse{Snapshot: csiSnapshot}

	zlog.Debug().Msgf("CreateSnapshot (nvme) - successfully created snapshot named %s from source volume ID %s", snapshotName, req.GetSourceVolumeId())
	return snapshotResp, nil
}

func (nvme *nvmestorage) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (resp *csi.DeleteSnapshotResponse, err error) {

	snapshotID, _ := strconv.Atoi(req.GetSnapshotId())
	zlog.Debug().Msgf("DeleteSnapshot (nvme) - to delete snapshot with ID %d", snapshotID)

	err = nvme.ValidateDeleteVolume(snapshotID)
	if err != nil {
		if status.Code(err) == codes.Aborted {
			e := fmt.Errorf("DeleteSnapshot (nvme) - ValidateDeleteVolume - snapshot ID %d error: %s", snapshotID, err.Error())
			zlog.Error().Msg(e.Error())
			return nil, e
		}

		if status.Code(err) == codes.NotFound {
			zlog.Debug().Msgf("DeleteSnapshot (nvme) - snapshot with ID %d not found", snapshotID)
			return &csi.DeleteSnapshotResponse{}, nil
		}

		e := fmt.Errorf("DeleteSnaphsot (nvme) - failed to delete snapshot with ID %d", snapshotID)
		zlog.Err(e)
		return nil, status.Error(codes.Internal, e.Error())
	}
	zlog.Debug().Msgf("DeleteSnapshot (nvme) - successfully deleted snapshot with ID %d", snapshotID)
	return &csi.DeleteSnapshotResponse{}, nil
}

func (nvme *nvmestorage) ValidateDeleteVolume(volumeID int) (err error) {

	zlog.Debug().Msgf("ValidateDeleteVolume (nvme) - called (also deletes volume) with ID %d", volumeID)

	vol, err := nvme.cs.IboxApi.GetVolume(volumeID)
	if err != nil {
		re, ok := err.(*iboxapi.IboxAPIError)
		if ok && re.Code == iboxapi.IBOXAPI_RESOURCE_NOT_FOUND_ERROR {
			zlog.Debug().Msgf("ValidateDeleteVolume (nvme) - volume: %d is already deleted", volumeID)
			return err
		}
		msg := fmt.Sprintf("ValidateDeleteVolume (nvme) - failed to get volume: %d, err: %s", volumeID, err.Error())
		zlog.Error().Msg(msg)
		return status.Error(codes.Internal, msg)
	}

	// this applies for when we are evaluating a snapshot volume
	if vol.LockState == common.LOCKED_STATE {
		return status.Errorf(codes.Aborted, "ValidateDeleteVolume (nvme) - volume %d was locked, can not delete till expire date is reached at %s", volumeID, time.UnixMilli(vol.LockExpiresAt))
	}

	childVolumes, err := nvme.cs.IboxApi.GetVolumesByParentID(vol.ID)
	if err != nil {
		zlog.Err(err)
		return err
	}
	if len(childVolumes) > 0 {
		metadata := map[string]interface{}{
			TOBEDELETED: true,
		}
		_, err = nvme.cs.IboxApi.PutMetadata(vol.ID, metadata)
		if err != nil {
			e := fmt.Errorf("ValidateDeleteVolume (nvme) - failed to update host.k8s.to_be_deleted for volume %s error: %v", vol.Name, err)
			zlog.Err(e)
			return e
		}
		zlog.Debug().Msgf("ValidateDeleteVolume (nvme) - found volume with ID %d has children volumes. Set metadata TOBEDELETED to 'true'. Deferring deletion.", volumeID)
		return
	}
	zlog.Debug().Msgf("ValidateDeleteVolume (nvme) - deleting volume named %s with ID %d", vol.Name, vol.ID)
	_, err = nvme.cs.IboxApi.DeleteMetadata(vol.ID)
	if err != nil {
		msg := fmt.Sprintf("ValidateDeleteVolume (nvme) - error deleting metadata for volume named %s with ID %d: %s", vol.Name, vol.ID, err.Error())
		zlog.Error().Msg(msg)
		return status.Error(codes.Internal, msg)
	}
	_, err = nvme.cs.IboxApi.DeleteVolume(vol.ID)
	if err != nil {
		msg := fmt.Sprintf("ValidateDeleteVolume (nvme) - error deleting volume named %s with ID %d: %s", vol.Name, vol.ID, err.Error())
		zlog.Error().Msg(msg)
		return status.Error(codes.Internal, msg)
	}
	zlog.Debug().Msgf("ValidateDeleteVolume (nvme) - deleted volume named %s with ID %d", vol.Name, vol.ID)

	if vol.ParentId != 0 {
		zlog.Debug().Msgf("ValidateDeleteVolume (nvme) - checking if parent volume with ID %d of volume named %s, with ID %d, can be deleted", vol.ParentId, vol.Name, vol.ID)
		var metadata []iboxapi.GetMetadataResult
		metadata, err = nvme.cs.IboxApi.GetMetadata(vol.ParentId)
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
			zlog.Debug().Msgf("ValidateDeleteVolume (nvme) - recursively called for parent. Volume ID: %d. Parent volume ID: %d", vol.ID, vol.ParentId)
			// Recursion
			err = nvme.ValidateDeleteVolume(vol.ParentId)
			if err != nil {
				zlog.Err(err)
				return err
			}
		}
	}
	return nil
}

func (nvme *nvmestorage) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (resp *csi.ControllerExpandVolumeResponse, err error) {

	volumeID := nvme.cs.VolProto.VolumeID
	zlog.Debug().Msgf("ControllerExpandVolume (nvme) - called volume ID %d", volumeID)

	capacity := int64(req.GetCapacityRange().GetRequiredBytes())
	if capacity < gib {
		capacity = gib
		zlog.Warn().Msgf("ControllerExpandVolume (nvme) - volume minimum capacity should be greater 1 GB")
	}

	// Expand volume size
	volume := iboxapi.Volume{
		Size: capacity,
	}
	_, err = nvme.cs.IboxApi.UpdateVolume(volumeID, volume)
	if err != nil {
		zlog.Error().Msgf("ControllerExpandVolume (nvme) - UpdateVolume - failed to update file system %v", err)
		return nil, err
	}
	zlog.Debug().Msgf("ControllerExpandVolume (nvme) - volume with ID %d size updated successfully", volumeID)
	return &csi.ControllerExpandVolumeResponse{
		CapacityBytes:         capacity,
		NodeExpansionRequired: true,
	}, nil
}

func (st *nvmestorage) ControllerGetVolume(
	_ context.Context, _ *csi.ControllerGetVolumeRequest,
) (*csi.ControllerGetVolumeResponse, error) {
	// Infinidat does not support ControllerGetVolume
	return nil, status.Error(codes.Unimplemented, "")
}
