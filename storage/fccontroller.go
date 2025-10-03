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
	const FN = "ValidateStorageClass"

	requiredFCParams := map[string]string{
		common.SC_POOL_NAME: `[a-zA-Z]+`, //match all strings except empty string or blank string
	}
	optionalFCParams := map[string]string{
		common.SC_PROVISION_TYPE: `(?i)\A(THICK|THIN)\z`,
		common.SC_UID:            `^\d+$`,
		common.SC_GID:            `^\d+$`,
	}

	// validate required parameters
	err := ValidateRequiredOptionalSCParameters(requiredFCParams, optionalFCParams, params)
	if err != nil {
		e := fmt.Errorf("%s (fc) - error %s", FN, err.Error())
		zlog.Error().Msg(e.Error())
		return status.Error(codes.InvalidArgument, e.Error())
	}
	return nil
}

func (fc *fcstorage) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	const FN = "CreateVolume"
	params := req.GetParameters()
	fc.configmap = params
	zlog.Debug().Msgf("%s (fc) - requested volume parameters are %v - requested size %d %s", FN, params, fc.capacity,
		GetHostInfo(req.GetSecrets(), fc.cs.IboxApi))

	// Volume name to be created - already verified in controller.go
	name := req.GetName()

	poolName := params[common.SC_POOL_NAME]

	targetVol, err := fc.cs.IboxApi.GetVolumeByName(name)
	if err != nil {
		re, ok := err.(*iboxapi.IboxAPIError)
		if ok && re.Code == iboxapi.IBOXAPI_RESOURCE_NOT_FOUND_ERROR {
			zlog.Debug().Msgf("%s (fc) - volume with name %s not found, proceeding to create", FN, name)
		} else {
			e := fmt.Errorf("%s (fc) - GetVolumeByName %s - error: %s", FN, name, err.Error())
			zlog.Error().Msg(e.Error())
			return nil, status.Error(codes.Internal, e.Error())
		}
	}

	if targetVol != nil {
		zlog.Debug().Msgf("%s (fc) - volume: %s found, size: %d requested: %d", FN, name, targetVol.Size, fc.capacity)
		if targetVol.Size == fc.capacity {
			existingVolumeInfo := fc.cs.getCSIResponse(targetVol, req)
			copyRequestParameters(params, existingVolumeInfo.VolumeContext)
			return &csi.CreateVolumeResponse{
				Volume: existingVolumeInfo,
			}, nil
		}
		err = status.Errorf(codes.AlreadyExists, "%s (fc) - volume: %s already exists with a different size, %v", FN, name, err)
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
		e := fmt.Sprintf("%s (fc) - determineSSDValue - error when creating volume %s storagepool %s, err: %s", FN, name, poolName, err.Error())
		zlog.Error().Msg(e)
		return nil, status.Error(codes.Internal, e)
	}

	pool, err := fc.cs.IboxApi.GetPoolByName(poolName)
	if err != nil {
		e := fmt.Sprintf("%s (fc) - GetPoolByName volume name: %s pool name: %s error: %s", FN, name, poolName, err.Error())
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
		e := fmt.Sprintf("%s (fc) - CreateVolume - error when creating volume %s storagepool %s, err: %s", FN, name, poolName, err.Error())
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
		zlog.Error().Msgf("%s (fc) - GetVolume - error: %s", FN, err.Error())
	}

	// a single test just in case there is a race condition on createVolume (doubtful)
	if vol == nil {
		time.Sleep(3 * time.Second)
		_, err = fc.cs.IboxApi.GetVolume(volumeResp.ID)
		if err != nil {
			e := fmt.Sprintf("%s (fc) - GetVolume - failed to create volume name: %s volume not retrieved for id: %d", FN, name, volumeResp.ID)
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
		e := fmt.Sprintf("%s (fc) - PutMetadata - failed to attach metadata - volume %s- error: %s", FN, name, err.Error())
		zlog.Error().Msg(e)
		return nil, status.Error(codes.Internal, e)
	}

	zlog.Debug().Msgf("%s (fc) - created volume: %s id: %d", FN, name, volumeResp.ID)
	return csiResp, err
}

func (fc *fcstorage) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (csiResp *csi.DeleteVolumeResponse, err error) {
	const FN = "DeleteVolume"
	zlog.Debug().Msgf("%s (fc) called", FN)
	err = fc.ValidateDeleteVolume(fc.cs.VolProto.VolumeID)
	if err != nil {
		e := fmt.Sprintf("%s (fc) - ValidateDeleteVolume volume ID %d- error: %s", FN, fc.cs.VolProto.VolumeID, err.Error())
		zlog.Error().Msg(e)
		return nil, status.Error(codes.Internal, e)
	}
	return &csi.DeleteVolumeResponse{}, nil
}

func (fc *fcstorage) createVolumeFromVolumeContent(req *csi.CreateVolumeRequest, name string, sizeInKbytes int64, storagePool string) (*csi.CreateVolumeResponse, error) {
	var err error
	const FN = "createVolumeFromVolumeContent"

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
		e := fmt.Sprintf("%s (fc) - failed to validate storage type for restore type: %s source id: %s, err: %v", FN, restoreType, volumeContentID, err)
		zlog.Error().Msg(e)
		return nil, status.Error(codes.NotFound, e)
	}

	srcVol, err := fc.cs.IboxApi.GetVolume(volproto.VolumeID)
	if err != nil {
		e := fmt.Sprintf("%s (fc) - GetVolume - restoreType: %s volume ID: %d error: %s", FN, restoreType, volproto.VolumeID, err.Error())
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
		e := fmt.Sprintf("%s (fc) - GetPoolByName - pool name: %s  error %s", FN, storagePool, err.Error())
		zlog.Error().Msg(e)
		return nil, status.Error(codes.Internal, e)
	}
	if pool.ID != srcVol.PoolId {
		e := fmt.Sprintf("%s (fc) - volume storage pool is different than requested storage pool %s", FN, storagePool)
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
		LockExpiresAt:  0,
	}
	// Create snapshot
	snapResponse, err := fc.cs.IboxApi.CreateSnapshotVolume(snapshotParam)
	if err != nil {
		e := fmt.Sprintf("%s (fc) - CreateSnapshotVolume - error %s", FN, err.Error())
		zlog.Error().Msg(e)
		return nil, status.Error(codes.Internal, e)
	}

	// Retrieve created destination volume
	volID := snapResponse.SnapShotID
	dstVol, err := fc.cs.IboxApi.GetVolume(volID)
	if err != nil {
		e := fmt.Sprintf("%s (fc) - GetVolume - volume ID: %d error: %s", FN, volID, err.Error())
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
		e := fmt.Sprintf("%s - PutMetadata - failed to attach metadata for volume: %s, err: %v", FN, dstVol.Name, err)
		zlog.Error().Msg(e)
		return nil, status.Error(codes.Internal, e)
	}
	zlog.Debug().Msgf("%s - Volume (from snap) %s (%s) storage pool %s", FN,
		csiVolume.VolumeContext["Name"], csiVolume.VolumeId, csiVolume.VolumeContext["StoragePoolName"])
	return &csi.CreateVolumeResponse{Volume: csiVolume}, nil
}

func (fc *fcstorage) ControllerModifyVolume(ctx context.Context, req *csi.ControllerModifyVolumeRequest) (resp *csi.ControllerModifyVolumeResponse, err error) {
	return nil, nil
}

func (fc *fcstorage) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (resp *csi.ControllerPublishVolumeResponse, err error) {
	const FN = "ControllerPublishVolume"
	zlog.Debug().Msgf("%s (fc) nodeID: %s volumeId: %s %s", FN, req.GetNodeId(), req.GetVolumeId(),
		GetHostInfo(req.GetSecrets(), fc.cs.IboxApi))
	volproto, err := ValidateVolumeID(req.GetVolumeId())
	if err != nil {
		e := fmt.Sprintf("%s (fc) - ValidateVolumeID - error: %s", FN, err.Error())
		zlog.Error().Msg(e)
		return nil, status.Error(codes.Internal, e)
	}

	hostName, err := DetermineHostName(req.GetNodeId())
	if err != nil {
		e := fmt.Sprintf("%s (fc) - DetermineHostName - error: %s", FN, err.Error())
		zlog.Error().Msg(e)
		return nil, status.Error(codes.Internal, e)
	}

	host, err := fc.cs.validateHost(hostName)
	if err != nil {
		e := fmt.Sprintf("%s (fc) - validateHost hostname %s- error: %s", FN, hostName, err.Error())
		zlog.Error().Msg(e)
		return nil, status.Error(codes.Internal, e)
	}

	v, err := fc.cs.IboxApi.GetVolume(volproto.VolumeID)
	if err != nil {
		e := fmt.Sprintf("%s (fc) - GetVolume volume ID '%s' - error: %v", FN, req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e)
		return nil, status.Error(codes.Internal, e)
	}

	_, err = fc.cs.AccessModesHelper.IsValidAccessMode(v, req)
	if err != nil {
		e := fmt.Sprintf("%s (fc) - IsValidAccessMode - error: %s", FN, err.Error())
		zlog.Error().Msg(e)
		return nil, status.Error(codes.Internal, e)
	}

	lunList, err := fc.cs.IboxApi.GetAllLunByHost(host.ID)
	if err != nil {
		e := fmt.Sprintf("%s (fc) - GetAllLunByHost volume Name: %s host ID: %d- error: %s", FN, v.Name, host.ID, err.Error())
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
			zlog.Debug().Msgf("%s (fc) - volume Name: %s volumeID: %d already mapped to host: %s", FN, v.Name, lun.VolumeID, host.Name)
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
			e := fmt.Sprintf("%s (fc) - invalid parameter %s error:  %v", FN, common.SC_MAX_VOLS_PER_HOST, err)
			zlog.Error().Msg(e)
			return nil, status.Error(codes.Internal, e)
		}
		if maxAllowedVol < 1 {
			e := fmt.Sprintf("%s (fc) - invalid parameter %s error:  required to be greater than 0", FN, common.SC_MAX_VOLS_PER_HOST)
			zlog.Error().Msg(e)
			return nil, status.Error(codes.Internal, e)
		}
		zlog.Debug().Msgf("host can have maximum %d volume mapped", maxAllowedVol)
		zlog.Debug().Msgf("host %s has %d volume mapped", host.Name, len(lunList))
		if len(lunList) >= maxAllowedVol {
			e := fmt.Sprintf("%s (fc) - unable to publish volume on host %s, maximum allowed volume per host is (%d), limit reached", FN, host.Name, maxAllowedVol)
			zlog.Error().Msg(e)
			return nil, status.Error(codes.ResourceExhausted, e)
		}
	}
	// map volume to host
	zlog.Debug().Msgf("%s (fc) - mapping volume Name: %s volume ID: %d to host: %s", FN, v.Name, volproto.VolumeID, host.Name)
	luninfo, err := fc.cs.mapVolumeTohost(volproto.VolumeID, host.ID)
	if err != nil {
		e := fmt.Sprintf("%s (fc) - mapVolumeToHost - error: %s", FN, err.Error())
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
	const FN = "ControllerUnpublishVolume"
	zlog.Debug().Msgf("%s (fc) - volProto %+v nodeID %s and volumeId %s", FN, fc.cs.VolProto, req.GetNodeId(), req.GetVolumeId())

	host := fc.cs.VolProto.Host
	if len(host.Luns) > 0 {
		zlog.Debug().Msgf("%s (fc) - unmap volume ID: %d from host: %d", FN, fc.cs.VolProto.VolumeID, host.ID)
		err = fc.cs.unmapVolumeFromHost(host.ID, int(fc.cs.VolProto.VolumeID))
		if err != nil {
			e := fmt.Sprintf("%s (fc) - unmapVolumeFromHost - error unmapping volume %d from host %d error %v", FN, fc.cs.VolProto.VolumeID, host.ID, err)
			zlog.Error().Msg(e)
			return nil, status.Error(codes.Internal, e)
		}
	}
	if len(host.Luns) < 2 {
		err = hostCleanup(fc.cs.IboxApi, host.ID, host.Name)
		if err != nil {
			e := fmt.Sprintf("%s (fc) - hostCleanup - error host ID: %d. Error: %s", FN, host.ID, err.Error())
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
	const FN = "CreateSnapshot"
	snapshotName := req.GetName()
	zlog.Debug().Msgf("%s (fc) - name: %s source volume ID: %s volproto: %+v", FN, snapshotName, req.GetSourceVolumeId(), fc.cs.VolProto)

	volumeSnapshot, err := fc.cs.IboxApi.GetVolumeByName(snapshotName)
	if err != nil {
		re, ok := err.(*iboxapi.IboxAPIError)
		if ok && re.Code == iboxapi.IBOXAPI_RESOURCE_NOT_FOUND_ERROR {
			zlog.Debug().Msgf("%s (fc) - snapshot with given name not found : %s", FN, snapshotName)
		} else {
			e := fmt.Sprintf("%s (fc) - GetVolumeByName - name: %s error: %s", FN, snapshotName, err.Error())
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
		e := fmt.Sprintf("%s (fc) - GetVolume - error get parent volume when creating snapshot - volume id %d, err: %v", FN, fc.cs.VolProto.VolumeID, err)
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
			e := fmt.Sprintf("%s (fc) - GetNtpStatus - error: %s", FN, err.Error())
			zlog.Error().Msg(e)
			return nil, status.Error(codes.Internal, e)
		}
		lockExpiresAt, err = validateSnapshotLockingParameter(ntpStatus[0].LastProbeTimestamp, lockExpiresAtParameter)
		if err != nil {
			e := fmt.Sprintf("%s (fc) - failed to create snapshot %s error %v, invalid lock_expires_at parameter ", FN, snapshotName, err)
			zlog.Error().Msg(e)
			return nil, status.Error(codes.Internal, e)
		}
		zlog.Info().Msgf("%s (fc) - snapshot Name: %s snapshot param has a lock_expires_at: %s", FN, snapshotName, lockExpiresAtParameter)
	}

	snapshotParam.LockExpiresAt = lockExpiresAt
	snapshot, err := fc.cs.IboxApi.CreateSnapshotVolume(snapshotParam)
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
	const FN = "ValidateDeleteVolume"
	vol, err := fc.cs.IboxApi.GetVolume(volumeID)
	if err != nil {
		re, ok := err.(*iboxapi.IboxAPIError)
		if ok && re.Code == iboxapi.IBOXAPI_RESOURCE_NOT_FOUND_ERROR {
			zlog.Debug().Msgf("%s (fc) - volume ID: %d is already deleted", FN, volumeID)
			return nil
		}
		e := fmt.Sprintf("%s (fc) - GetVolume - error %s", FN, err.Error())
		zlog.Error().Msg(e)
		return status.Error(codes.Internal, e)
	}

	if vol.LockState == common.LOCKED_STATE {
		e := fmt.Sprintf("%s (fc) - volume ID: %d was locked, can not delete till expire date %s is reached", FN, volumeID, time.UnixMilli(vol.LockExpiresAt))
		zlog.Error().Msg(e)
		return status.Error(codes.Aborted, e)
	}

	childVolumes, err := fc.cs.IboxApi.GetVolumesByParentID(vol.ID)
	if err != nil {
		zlog.Error().Msgf("%s (fc) - error %s", FN, err.Error())
	}
	if len(childVolumes) > 0 {
		metadata := map[string]interface{}{
			TOBEDELETED: true,
		}
		_, err = fc.cs.IboxApi.PutMetadata(vol.ID, metadata)
		if err != nil {
			e := fmt.Sprintf("%s (fc) - failed to update host.k8s.to_be_deleted for volume %s error: %v", FN, vol.Name, err)
			zlog.Error().Msg(e)
			err = errors.New(e)
		}
		return
	}
	zlog.Debug().Msgf("%s (fc) - deleting volume name: %s ID: %d", FN, vol.Name, vol.ID)
	_, err = fc.cs.IboxApi.DeleteMetadata(vol.ID)
	if err != nil {
		e := fmt.Sprintf("%s (fc) - DeleteMetadata - error %s", FN, err.Error())
		zlog.Error().Msg(e)
		return status.Error(codes.Internal, e)
	}
	_, err = fc.cs.IboxApi.DeleteVolume(vol.ID)
	if err != nil {
		e := fmt.Sprintf("%s (fc) -  DeleteVolume - error %s", FN, err.Error())
		zlog.Error().Msg(e)
		return status.Error(codes.Internal, e)
	}
	if vol.ParentId != 0 {
		zlog.Debug().Msgf("%s (fc) - checking if parent volume can be name: %s ID: %d", FN, vol.Name, vol.ID)
		var metadata []iboxapi.GetMetadataResult
		metadata, err = fc.cs.IboxApi.GetMetadata(vol.ParentId)
		if err != nil {
			e := fmt.Sprintf("%s (fc) - error %s", FN, err.Error())
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
				e := fmt.Sprintf("%s (fc) - error %s", FN, err.Error())
				zlog.Error().Msg(e)
				return status.Error(codes.Internal, e)
			}
		}
	}
	return
}

func (fc *fcstorage) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (resp *csi.ControllerExpandVolumeResponse, err error) {
	const FN = "ControllerExpandVolume"
	volumeID := fc.cs.VolProto.VolumeID
	zlog.Debug().Msgf("%s (fc) - volume ID: %d", FN, volumeID)

	capacity := int64(req.GetCapacityRange().GetRequiredBytes())
	if capacity < gib {
		capacity = gib
		zlog.Warn().Msgf("%s (fc) - Volume Minimum capacity should be greater 1 GB", FN)
	}

	// Expand volume size
	volume := iboxapi.Volume{
		Size: capacity,
	}
	_, err = fc.cs.IboxApi.UpdateVolume(volumeID, volume)
	if err != nil {
		e := fmt.Errorf("%s (fc) - UpdateVolume - error: %s", FN, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, e
	}
	zlog.Debug().Msgf("%s (fc) - volume size updated successfully volume ID: %d", FN, volumeID)
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
