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
		e := fmt.Errorf("ValidateStorageClass (fc) - error %s", err.Error())
		zlog.Error().Msg(e.Error())
		return status.Error(codes.InvalidArgument, e.Error())
	}
	return nil
}

func (fc *fcstorage) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	params := req.GetParameters()
	fc.configmap = params
	zlog.Debug().Msgf("CreateVolume (fc) - requested volume parameters are %v", params)

	gid := params[common.SC_GID]
	uid := params[common.SC_UID]
	unix_permissions := params[common.SC_UNIX_PERMISSIONS]
	zlog.Debug().Msgf("CreateVolume (fc) - storageClass request parameters uid %s gid %s unix_permissions %s", gid, uid, unix_permissions)

	zlog.Debug().Msgf("CreateVolume (fc) - requested size in bytes is %d ", fc.capacity)

	// Volume name to be created - already verified in controller.go
	name := req.GetName()

	poolName := params[common.SC_POOL_NAME]

	targetVol, err := fc.cs.IboxApi.GetVolumeByName(name)
	if err != nil {
		re, ok := err.(*iboxapi.IboxAPIError)
		if ok && re.Code == iboxapi.IBOXAPI_RESOURCE_NOT_FOUND_ERROR {
			zlog.Debug().Msgf("CreateVolume (fc) - volume with name %s not found, proceeding to create", name)
		} else {
			e := fmt.Errorf("CreateVolume (fc) - GetVolumeByName %s - error: %s", name, err.Error())
			zlog.Error().Msg(e.Error())
			return nil, status.Error(codes.Internal, e.Error())
		}
	}

	if targetVol != nil {
		zlog.Debug().Msgf("CreateVolume (fc) - volume: %s found, size: %d requested: %d", name, targetVol.Size, fc.capacity)
		if targetVol.Size == fc.capacity {
			existingVolumeInfo := fc.cs.getCSIResponse(targetVol, req)
			copyRequestParameters(params, existingVolumeInfo.VolumeContext)
			return &csi.CreateVolumeResponse{
				Volume: existingVolumeInfo,
			}, nil
		}
		err = status.Errorf(codes.AlreadyExists, "CreateVolume (fc) - volume: %s already exists with a different size, %v", name, err)
		zlog.Error().Msg(err.Error())
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
		e := fmt.Sprintf("CreateVolume (fc) - determineSSDValue - error when creating volume %s storagepool %s, err: %s", name, poolName, err.Error())
		zlog.Error().Msg(e)
		return nil, status.Error(codes.Internal, e)
	}

	pool, err := fc.cs.IboxApi.GetPoolByName(poolName)
	if err != nil {
		e := fmt.Sprintf("CreateVolume (fc) - GetPoolByName volume name: %s pool name: %s error: %s", name, poolName, err.Error())
		zlog.Error().Msg(e)
		return nil, status.Error(codes.Internal, e)
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
		e := fmt.Sprintf("CreateVolume (fc) - CreateVolume - error when creating volume %s storagepool %s, err: %s", name, poolName, err.Error())
		zlog.Error().Msg(e)
		return nil, status.Error(codes.Internal, e)
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
		zlog.Error().Msgf("CreateVolume (fc) - GetVolume - error: %s", err.Error())
	}

	// a single test just in case there is a race condition on createVolume (doubtful)
	if vol == nil {
		time.Sleep(3 * time.Second)
		_, err = fc.cs.IboxApi.GetVolume(volumeResp.ID)
		if err != nil {
			e := fmt.Sprintf("fCreateVolume (fc) - GetVolume - failed to create volume name: %s volume not retrieved for id: %d", name, volumeResp.ID)
			zlog.Error().Msg(e)
			return nil, status.Error(codes.Internal, e)
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
		e := fmt.Sprintf("CreateVolume (fc) - PutMetadata - failed to attach metadata - volume %s- error: %s", name, err.Error())
		zlog.Error().Msg(e)
		return nil, status.Error(codes.Internal, e)
	}

	zlog.Debug().Msgf("CreateVolume (fc) - created volume: %s id: %d", name, volumeResp.ID)
	return csiResp, err
}

func (fc *fcstorage) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (csiResp *csi.DeleteVolumeResponse, err error) {
	zlog.Debug().Msgf("DeleteVolume (fc) called")
	err = fc.ValidateDeleteVolume(fc.cs.VolProto.VolumeID)
	if err != nil {
		e := fmt.Sprintf("DeleteVolume (fc) - ValidateDeleteVolume volume ID %d- error: %s", fc.cs.VolProto.VolumeID, err.Error())
		zlog.Error().Msg(e)
		return nil, status.Error(codes.Internal, e)
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
		e := fmt.Sprintf("createVolumeFromVolumeContent (fc) - failed to validate storage type for restore type: %s source id: %s, err: %v", restoreType, volumeContentID, err)
		zlog.Error().Msg(e)
		return nil, status.Error(codes.NotFound, e)
	}

	srcVol, err := fc.cs.IboxApi.GetVolume(volproto.VolumeID)
	if err != nil {
		e := fmt.Sprintf("createVolumeFromVolumeContent (fc) - GetVolume - restoreType: %s volume ID: %d error: %s", restoreType, volproto.VolumeID, err.Error())
		zlog.Error().Msg(e)
		return nil, status.Error(codes.NotFound, e)
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
		e := fmt.Sprintf("createVolumeFromVolumeContent (fc) - GetPoolByName - pool name: %s  error %s", storagePool, err.Error())
		zlog.Error().Msg(e)
		return nil, status.Error(codes.Internal, e)
	}
	if pool.ID != srcVol.PoolId {
		e := fmt.Sprintf("createVolumeFromVolumeContent (fc) - volume storage pool is different than requested storage pool %s", storagePool)
		zlog.Error().Msg(e)
		return nil, status.Error(codes.InvalidArgument, e)
	}
	ssd := req.GetParameters()[common.SC_SSD_ENABLED]
	if ssd == "" {
		ssd = fmt.Sprint(false)
	}
	ssdEnabled, _ := strconv.ParseBool(ssd)
	snapshotParam := iboxapi.CreateSnapshotVolumeRequest{
		ParentID:       volproto.VolumeID,
		SnapshotName:   name,
		WriteProtected: false,
		SsdEnabled:     ssdEnabled,
	}
	// Create snapshot
	snapResponse, err := fc.cs.IboxApi.CreateSnapshotVolume(0, snapshotParam)
	if err != nil {
		e := fmt.Sprintf("createVolumeFromVolumeContent (fc) - CreateSnapshotVolume - error %s", err.Error())
		zlog.Error().Msg(e)
		return nil, status.Error(codes.Internal, e)
	}

	// Retrieve created destination volume
	volID := snapResponse.SnapShotID
	dstVol, err := fc.cs.IboxApi.GetVolume(volID)
	if err != nil {
		e := fmt.Sprintf("createVolumeFromVolumeContent (fc) - GetVolume - volume ID: %d error: %s", volID, err.Error())
		zlog.Error().Msg(e)
		return nil, status.Error(codes.Internal, e)
	}

	// Create a volume response and return it
	csiVolume := fc.cs.getCSIResponse(dstVol, req)
	copyRequestParameters(req.GetParameters(), csiVolume.VolumeContext)

	metadata := map[string]interface{}{
		"host.k8s.pvname": dstVol.Name,
	}
	_, err = fc.cs.IboxApi.PutMetadata(dstVol.ID, metadata)
	if err != nil {
		e := fmt.Sprintf("createVolumeFromVolumeContent - PutMetadata - failed to attach metadata for volume: %s, err: %v", dstVol.Name, err)
		zlog.Error().Msg(e)
		return nil, status.Error(codes.Internal, e)
	}
	zlog.Debug().Msgf("createVolumeFromVolumeContent - Volume (from snap) %s (%s) storage pool %s",
		csiVolume.VolumeContext["Name"], csiVolume.VolumeId, csiVolume.VolumeContext["StoragePoolName"])
	return &csi.CreateVolumeResponse{Volume: csiVolume}, nil
}

func (fc *fcstorage) ControllerModifyVolume(ctx context.Context, req *csi.ControllerModifyVolumeRequest) (resp *csi.ControllerModifyVolumeResponse, err error) {
	return nil, nil
}

func (fc *fcstorage) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (resp *csi.ControllerPublishVolumeResponse, err error) {
	zlog.Debug().Msgf("ControllerPublishVolume (fc) nodeID: %s volumeId: %s", req.GetNodeId(), req.GetVolumeId())
	volproto, err := ValidateVolumeID(req.GetVolumeId())
	if err != nil {
		e := fmt.Sprintf("ControllerPublishVolume (fc) - ValidateVolumeID - error: %s", err.Error())
		zlog.Error().Msg(e)
		return nil, status.Error(codes.Internal, e)
	}

	hostName, err := DetermineHostName(req.GetNodeId())
	if err != nil {
		e := fmt.Sprintf("ControllerPublishVolume (fc) - DetermineHostName - error: %s", err.Error())
		zlog.Error().Msg(e)
		return nil, status.Error(codes.Internal, e)
	}

	host, err := fc.cs.validateHost(hostName)
	if err != nil {
		e := fmt.Sprintf("ControllerPublishVolume (fc) - validateHost hostname %s- error: %s", hostName, err.Error())
		zlog.Error().Msg(e)
		return nil, status.Error(codes.Internal, e)
	}

	v, err := fc.cs.IboxApi.GetVolume(volproto.VolumeID)
	if err != nil {
		e := fmt.Sprintf("ControllerPublishVolume (fc) - GetVolume volume ID '%s' - error: %v", req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e)
		return nil, status.Error(codes.Internal, e)
	}

	_, err = fc.cs.AccessModesHelper.IsValidAccessMode(v, req)
	if err != nil {
		e := fmt.Sprintf("ControllerPublishVolume (fc) - IsValidAccessMode - error: %s", err.Error())
		zlog.Error().Msg(e)
		return nil, status.Error(codes.Internal, e)
	}

	lunList, err := fc.cs.IboxApi.GetAllLunByHost(host.ID)
	if err != nil {
		e := fmt.Sprintf("ControllerPublishVolume (fc) - GetAllLunByHost volume Name: %s host ID: %d- error: %s", v.Name, host.ID, err.Error())
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
				LUN_PUBLISH_CONTEXT:        strconv.Itoa(lun.Lun),
				HOST_ID_PUBLISH_CONTEXT:    strconv.Itoa(host.ID),
				HOST_PORTS_PUBLISH_CONTEXT: ports,
			}
			zlog.Debug().Msgf("ControllerPublishVolume (fc) - volume Name: %s volumeID: %d already mapped to host: %s", v.Name, lun.VolumeID, host.Name)
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
			e := fmt.Sprintf("ControllerPublishVolume (fc) - invalid parameter %s error:  %v", common.SC_MAX_VOLS_PER_HOST, err)
			zlog.Error().Msg(e)
			return nil, status.Error(codes.Internal, e)
		}
		if maxAllowedVol < 1 {
			e := fmt.Sprintf("ControllerPublishVolume (fc) - invalid parameter %s error:  required to be greater than 0", common.SC_MAX_VOLS_PER_HOST)
			zlog.Error().Msg(e)
			return nil, status.Error(codes.Internal, e)
		}
		zlog.Debug().Msgf("host can have maximum %d volume mapped", maxAllowedVol)
		zlog.Debug().Msgf("host %s has %d volume mapped", host.Name, len(lunList))
		if len(lunList) >= maxAllowedVol {
			e := fmt.Sprintf("ControllerPublishVolume (fc) - unable to publish volume on host %s, maximum allowed volume per host is (%d), limit reached", host.Name, maxAllowedVol)
			zlog.Error().Msg(e)
			return nil, status.Error(codes.ResourceExhausted, e)
		}
	}
	// map volume to host
	zlog.Debug().Msgf("ControllerPublishVolume (fc) - mapping volume Name: %s volume ID: %d to host: %s", v.Name, volproto.VolumeID, host.Name)
	luninfo, err := fc.cs.mapVolumeTohost(volproto.VolumeID, host.ID)
	if err != nil {
		e := fmt.Sprintf("ControllerPublishVolume (fc) - mapVolumeToHost - error: %s", err.Error())
		zlog.Error().Msg(e)
		return nil, status.Error(codes.Internal, e)
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
	zlog.Debug().Msgf("ControllerUnpublishVolume (fc) - volProto %+v nodeID %s and volumeId %s", fc.cs.VolProto, req.GetNodeId(), req.GetVolumeId())

	host := fc.cs.VolProto.Host
	if len(host.Luns) > 0 {
		zlog.Debug().Msgf("ControllerUnpublishVolume (fc) - unmap volume ID: %d from host: %d", fc.cs.VolProto.VolumeID, host.ID)
		err = fc.cs.unmapVolumeFromHost(host.ID, int(fc.cs.VolProto.VolumeID))
		if err != nil {
			e := fmt.Sprintf("ControllerUnpublishVolume (fc) - unmapVolumeFromHost - error unmapping volume %d from host %d error %v", fc.cs.VolProto.VolumeID, host.ID, err)
			zlog.Error().Msg(e)
			return nil, status.Error(codes.Internal, e)
		}
	}
	if len(host.Luns) < 2 {
		err = hostCleanup(fc.cs.IboxApi, host.ID, host.Name)
		if err != nil {
			e := fmt.Sprintf("ControllerUnpublishVolume (fc) - hostCleanup - error host ID: %d. Error: %s", host.ID, err.Error())
			zlog.Error().Msg(e)
			return nil, status.Error(codes.Internal, e)
		}
	}
	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

func (fc *fcstorage) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (resp *csi.ValidateVolumeCapabilitiesResponse, err error) {
	zlog.Error().Msgf("ValidateVolumeCapabilities (fc) - should not be called, implemented in controller.go instead")
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
	zlog.Debug().Msgf("CreateSnapshot (fc) - name: %s source volume ID: %s volproto: %+v", snapshotName, req.GetSourceVolumeId(), fc.cs.VolProto)

	volumeSnapshot, err := fc.cs.IboxApi.GetVolumeByName(snapshotName)
	if err != nil {
		re, ok := err.(*iboxapi.IboxAPIError)
		if ok && re.Code == iboxapi.IBOXAPI_RESOURCE_NOT_FOUND_ERROR {
			zlog.Debug().Msgf("CreateSnapshot (fc) - snapshot with given name not found : %s", snapshotName)
		} else {
			e := fmt.Sprintf("CreateSnapshot (fc) - GetVolumeByName - name: %s error: %s", snapshotName, err.Error())
			zlog.Error().Msg(e)
			return nil, status.Error(codes.Internal, e)
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
		return nil, status.Error(codes.AlreadyExists, fmt.Sprintf("snapshot Name: %s with already existing name and different source volume ID", volumeSnapshot.Name))
	}

	// look up the parent volume so we can get the ssd_enabled value and use that for
	// the snapshot being created next
	parentVolume, err := fc.cs.IboxApi.GetVolume(fc.cs.VolProto.VolumeID)
	if err != nil {
		e := fmt.Sprintf("CreateSnapshot (fc) - GetVolume - error get parent volume when creating snapshot - volume id %d, err: %v", fc.cs.VolProto.VolumeID, err)
		zlog.Error().Msg(e)
		return nil, status.Error(codes.NotFound, e)
	}

	snapshotParam := iboxapi.CreateSnapshotVolumeRequest{
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
			e := fmt.Sprintf("CreateSnapshot (fc) - GetNtpStatus - error: %s", err.Error())
			zlog.Error().Msg(e)
			return nil, status.Error(codes.Internal, e)
		}
		lockExpiresAt, err = validateSnapshotLockingParameter(ntpStatus[0].LastProbeTimestamp, lockExpiresAtParameter)
		if err != nil {
			e := fmt.Sprintf("CreateSnapshot (fc) - failed to create snapshot %s error %v, invalid lock_expires_at parameter ", snapshotName, err)
			zlog.Error().Msg(e)
			return nil, status.Error(codes.Internal, e)
		}
		zlog.Info().Msgf("CreateSnapshot (fc) - snapshot Name: %s snapshot param has a lock_expires_at: %s", snapshotName, lockExpiresAtParameter)
	}

	snapshot, err := fc.cs.IboxApi.CreateSnapshotVolume(lockExpiresAt, snapshotParam)
	if err != nil {
		e := fmt.Sprintf("CreateSnapshot (fc) - CreateSnapshotVolume  snapshot name: %s error: %s", snapshotName, err.Error())
		zlog.Error().Msg(e)
		return nil, status.Error(codes.Internal, e)
	}

	snapshotID = strconv.Itoa(snapshot.SnapShotID) + "$$" + fc.cs.VolProto.StorageType
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

func (fc *fcstorage) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (resp *csi.DeleteSnapshotResponse, err error) {

	snapshotID, _ := strconv.Atoi(req.GetSnapshotId())

	err = fc.ValidateDeleteVolume(snapshotID)
	if err != nil {
		e := fmt.Sprintf("DeleteSnapshot (fc) - ValidateDeleteVolume - snapshot ID: %s error: %s", req.GetSnapshotId(), err.Error())
		zlog.Error().Msg(e)
		return nil, status.Error(codes.Internal, e)
	}
	return &csi.DeleteSnapshotResponse{}, nil
}

func (fc *fcstorage) ValidateDeleteVolume(volumeID int) (err error) {
	vol, err := fc.cs.IboxApi.GetVolume(volumeID)
	if err != nil {
		re, ok := err.(*iboxapi.IboxAPIError)
		if ok && re.Code == iboxapi.IBOXAPI_RESOURCE_NOT_FOUND_ERROR {
			zlog.Debug().Msgf("ValidateDeleteVolume (fc) - volume ID: %d is already deleted", volumeID)
			return nil
		}
		e := fmt.Sprintf("ValidateDeleteVolume (fc) - GetVolume - error %s", err.Error())
		zlog.Error().Msg(e)
		return status.Error(codes.Internal, e)
	}

	if vol.LockState == common.LOCKED_STATE {
		e := fmt.Sprintf("ValidateDeleteVolume (fc) - volume ID: %d was locked, can not delete till expire date %s is reached", volumeID, time.UnixMilli(vol.LockExpiresAt))
		zlog.Error().Msg(e)
		return status.Error(codes.Aborted, e)
	}

	childVolumes, err := fc.cs.IboxApi.GetVolumesByParentID(vol.ID)
	if err != nil {
		zlog.Error().Msgf("ValidateDeleteVolume (fc) - error %s", err.Error())
	}
	if len(childVolumes) > 0 {
		metadata := map[string]interface{}{
			TOBEDELETED: true,
		}
		_, err = fc.cs.IboxApi.PutMetadata(vol.ID, metadata)
		if err != nil {
			e := fmt.Sprintf("ValidateDeleteVolume (fc) - failed to update host.k8s.to_be_deleted for volume %s error: %v", vol.Name, err)
			zlog.Error().Msg(e)
			err = errors.New(e)
		}
		return
	}
	zlog.Debug().Msgf("ValidateDeleteVolume (fc) - deleting volume name: %s ID: %d", vol.Name, vol.ID)
	_, err = fc.cs.IboxApi.DeleteMetadata(vol.ID)
	if err != nil {
		e := fmt.Sprintf("ValidateDeleteVolume (fc) - DeleteMetadata - error %s", err.Error())
		zlog.Error().Msg(e)
		return status.Error(codes.Internal, e)
	}
	_, err = fc.cs.IboxApi.DeleteVolume(vol.ID)
	if err != nil {
		e := fmt.Sprintf("ValidateDeleteVolume (fc) -  DeleteVolume - error %s", err.Error())
		zlog.Error().Msg(e)
		return status.Error(codes.Internal, e)
	}
	if vol.ParentId != 0 {
		zlog.Debug().Msgf("ValidateDeleteVolume (fc) - checking if parent volume can be name: %s ID: %d", vol.Name, vol.ID)
		var metadata []iboxapi.GetMetadataResult
		metadata, err = fc.cs.IboxApi.GetMetadata(vol.ParentId)
		if err != nil {
			e := fmt.Sprintf("ValidateDeleteVolume (fc) - error %s", err.Error())
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
			err = fc.ValidateDeleteVolume(vol.ParentId)
			if err != nil {
				e := fmt.Sprintf("ValidateDeleteVolume (fc) - error %s", err.Error())
				zlog.Error().Msg(e)
				return status.Error(codes.Internal, e)
			}
		}
	}
	return
}

func (fc *fcstorage) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (resp *csi.ControllerExpandVolumeResponse, err error) {

	volumeID := fc.cs.VolProto.VolumeID
	zlog.Debug().Msgf("ControllerExpandVolume (fc) - volume ID: %d", volumeID)

	capacity := int64(req.GetCapacityRange().GetRequiredBytes())
	if capacity < gib {
		capacity = gib
		zlog.Warn().Msgf("ControllerExpandVolume (fc) - Volume Minimum capacity should be greater 1 GB")
	}

	// Expand volume size
	volume := iboxapi.Volume{
		Size: capacity,
	}
	_, err = fc.cs.IboxApi.UpdateVolume(volumeID, volume)
	if err != nil {
		e := fmt.Errorf("ControllerExpandVolume (fc) - UpdateVolume - error: %s", err.Error())
		zlog.Error().Msg(e.Error())
		return nil, e
	}
	zlog.Debug().Msgf("ControllerExpandVolume (fc) - volume size updated successfully volume ID: %d", volumeID)
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
