/*Copyright 2022 Infinidat
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.*/
package storage

import (
	"context"
	"errors"
	"fmt"
	"infinibox-csi-driver/api"
	"strconv"
	"strings"
	"time"

	"k8s.io/klog"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (fc *fcstorage) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("Recovered from FC CreateVolume " + fmt.Sprint(res))
		}
	}()
	cr := req.GetCapacityRange()
	sizeBytes, err := verifyVolumeSize(cr)
	if err != nil {
		return nil, err
	}
	klog.V(2).Infof("requested size in bytes is %d ", sizeBytes)

	params := req.GetParameters()
	fc.configmap = params
	klog.V(2).Infof(" csi request parameters %v", params)

	// validate required parameters
	err = validateStorageClassParameters(map[string]string{
		"pool_name":         `\A.*\z`, // TODO: could make this enforce IBOX pool_name requirements, but probably not necessary
		"max_vols_per_host": `(?i)\A\d+\z`,
	}, params)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	// validate optional parameters
	volType, provided := params["provision_type"]
	if !provided {
		volType = "THIN" // TODO: add support for leaving this unspecified, CSIC-340
	}
	ssdEnabled := true // TODO: add support for leaving this unspecified, CSIC-340
	ssdEnabledString, provided := params["ssd_enabled"]
	if provided {
		ssdEnabled, _ = strconv.ParseBool(ssdEnabledString)
	}

	gid := params["gid"]
	uid := params["uid"]
	unix_permissions := params["unix_permissions"]
	klog.V(2).Infof("storageClass request parameters uid %s gid %s unix_permissions %s", gid, uid, unix_permissions)

	// Volume name to be created - already verified in controller.go
	name := req.GetName()

	// Pool name - already verified earlier
	poolName := params["pool_name"]

	targetVol, err := fc.cs.api.GetVolumeByName(name)
	if err != nil {
		if !strings.Contains(err.Error(), "volume with given name not found") {
			return nil, status.Errorf(codes.NotFound, "CreateVolume failed: %v", err)
		}
	}
	if targetVol != nil {
		klog.V(2).Infof("volume: %s found, size: %d requested: %d", name, targetVol.Size, sizeBytes)
		if targetVol.Size == sizeBytes {
			existingVolumeInfo := fc.cs.getCSIResponse(targetVol, req)
			copyRequestParameters(params, existingVolumeInfo.VolumeContext)
			return &csi.CreateVolumeResponse{
				Volume: existingVolumeInfo,
			}, nil
		}
		err = status.Errorf(codes.AlreadyExists, "CreateVolume failed: volume exists but has different size")
		klog.Errorf("Volume: %s already exists with a different size, %v", name, err)
		return nil, err
	}

	// Volume content source support volume and snapshots
	contentSource := req.GetVolumeContentSource()
	if contentSource != nil {
		return fc.createVolumeFromVolumeContent(req, name, sizeBytes, poolName)
	}
	volumeParam := &api.VolumeParam{
		Name:          name,
		VolumeSize:    sizeBytes,
		ProvisionType: volType,
		SsdEnabled:    ssdEnabled,
	}
	volumeResp, err := fc.cs.api.CreateVolume(volumeParam, poolName)
	if err != nil {
		klog.Errorf("error creating volume: %s pool %s error: %s", name, poolName, err.Error())
		return nil, status.Errorf(codes.Internal, "error when creating volume %s storagepool %s, err: %s", name, poolName, err.Error())
	}
	vi := fc.cs.getCSIResponse(volumeResp, req)

	// check volume id format
	volID, err := strconv.Atoi(vi.VolumeId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "error getting volume id")
	}

	// confirm volume creation
	var vol *api.Volume
	var counter int
	vol, err = fc.cs.api.GetVolume(volID)
	for vol == nil && counter < 100 {
		time.Sleep(3 * time.Millisecond)
		vol, err = fc.cs.api.GetVolume(volID)
		counter = counter + 1
	}
	if vol == nil {
		return nil, status.Errorf(codes.Internal, "failed to create volume name: %s volume not retrieved for id: %d", name, volID)
	}

	// Prepare response struct
	copyRequestParameters(params, vi.VolumeContext)
	csiResp := &csi.CreateVolumeResponse{
		Volume: vi,
	}

	// attach metadata to volume object
	metadata := make(map[string]interface{})
	metadata["host.k8s.pvname"] = volumeResp.Name
	// metadata["host.filesystem_type"] = req.GetParameters()["fstype"] // TODO: set this correctly according to what fcnode.go does, not the fstype parameter originally captured in this function ... which is likely overwritten by the VolumeCapability
	_, err = fc.cs.api.AttachMetadataToObject(int64(volumeResp.ID), metadata)
	if err != nil {
		klog.Errorf("failed to attach metadata for volume: %s, err: %v", name, err)
		return nil, status.Errorf(codes.Internal, "failed to attach metadata")
	}

	klog.V(2).Infof("CreateVolume resp: %v", *csiResp)
	klog.Infof("created volume: %s id: %d", name, volID)
	return csiResp, err
}

func (fc *fcstorage) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (csiResp *csi.DeleteVolumeResponse, err error) {
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("Recovered from FC DeleteSnapshot  " + fmt.Sprint(res))
		}
	}()
	klog.V(4).Infof("Called DeleteVolume")
	if req.GetVolumeId() == "" {
		return nil, status.Errorf(codes.Internal,
			"error parsing volume id : %s", errors.New("Volume id not found"))
	}
	id, err := strconv.Atoi(req.GetVolumeId())
	if err != nil {
		return nil, status.Errorf(codes.Internal,
			"error parsing volume id : %s", err.Error())
	}
	err = fc.ValidateDeleteVolume(id)
	if err != nil {
		return nil, status.Errorf(codes.Internal,
			"error deleting volume : %s", err.Error())
	}
	return &csi.DeleteVolumeResponse{}, nil
}

func (fc *fcstorage) createVolumeFromVolumeContent(req *csi.CreateVolumeRequest, name string, sizeInKbytes int64, storagePool string) (*csi.CreateVolumeResponse, error) {
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("Recovered from FC createVolumeFromVolumeContent " + fmt.Sprint(res))
		}
	}()

	volumecontent := req.GetVolumeContentSource()
	volumeContentID := ""
	var restoreType string
	if volumecontent.GetSnapshot() != nil {
		restoreType = "Snapshot"
		volumeContentID = volumecontent.GetSnapshot().GetSnapshotId()
	} else if volumecontent.GetVolume() != nil {
		volumeContentID = volumecontent.GetVolume().GetVolumeId()
		restoreType = "Volume"
	}

	// Validate the source content id
	volproto, err := validateVolumeID(volumeContentID)
	if err != nil {
		klog.Errorf("Failed to validate storage type for source id: %s, err: %v", volumeContentID, err)
		return nil, status.Errorf(codes.NotFound, restoreType+" not found: %s", volumeContentID)
	}

	ID, err := strconv.Atoi(volproto.VolumeID)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, restoreType+" invalid: %s", volumeContentID)
	}
	srcVol, err := fc.cs.api.GetVolume(ID)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, restoreType+" not found: %d", ID)
	}

	// Validate the size is the same.
	if int64(srcVol.Size) != sizeInKbytes {
		return nil, status.Errorf(codes.InvalidArgument,
			restoreType+" %s has incompatible size %d kbytes with requested %d kbytes",
			volumeContentID, srcVol.Size, sizeInKbytes)
	}

	// Validate the storagePool is the same.
	storagePoolID, err := fc.cs.api.GetStoragePoolIDByName(storagePool)
	if err != nil {
		return nil, status.Errorf(codes.Internal,
			"error while getting storagepoolid with name %s ", storagePool)
	}
	if storagePoolID != srcVol.PoolId {
		return nil, status.Errorf(codes.InvalidArgument,
			"volume storage pool is different than the requested storage pool %s", storagePool)
	}
	ssd := req.GetParameters()["ssd_enabled"]
	if ssd == "" {
		ssd = fmt.Sprint(false)
	}
	ssdEnabled, _ := strconv.ParseBool(ssd)
	snapshotParam := &api.VolumeSnapshot{
		ParentID:       ID,
		SnapshotName:   name,
		WriteProtected: false,
		SsdEnabled:     ssdEnabled,
	}
	// Create snapshot
	snapResponse, err := fc.cs.api.CreateSnapshotVolume(snapshotParam)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to create snapshot: %s", err.Error())
	}

	// Retrieve created destination volume
	volID := snapResponse.SnapShotID
	dstVol, err := fc.cs.api.GetVolume(volID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not retrieve created volume: %d", volID)
	}

	// Create a volume response and return it
	csiVolume := fc.cs.getCSIResponse(dstVol, req)
	copyRequestParameters(req.GetParameters(), csiVolume.VolumeContext)

	metadata := make(map[string]interface{})
	metadata["host.k8s.pvname"] = dstVol.Name
	// metadata["host.filesystem_type"] = req.GetParameters()["fstype"] // TODO: set this correctly according to what fcnode.go does, not the fstype parameter originally captured in this function ... which is likely overwritten by the VolumeCapability
	_, err = fc.cs.api.AttachMetadataToObject(int64(dstVol.ID), metadata)
	if err != nil {
		klog.Errorf("failed to attach metadata for volume: %s, err: %v", dstVol.Name, err)
		return nil, status.Errorf(codes.Internal, "failed to attach metadata to volume: %s, err: %v", dstVol.Name, err)
	}
	klog.Errorf("Volume (from snap) %s (%s) storage pool %s",
		csiVolume.VolumeContext["Name"], csiVolume.VolumeId, csiVolume.VolumeContext["StoragePoolName"])
	return &csi.CreateVolumeResponse{Volume: csiVolume}, nil
}

func (fc *fcstorage) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (resp *csi.ControllerPublishVolumeResponse, err error) {
	klog.V(2).Infof("ControllerPublishVolume called with nodeID %s and volumeId %s", req.GetNodeId(), req.GetVolumeId())
	volproto, err := validateVolumeID(req.GetVolumeId())
	if err != nil {
		klog.Errorf("Failed to validate storage type %v", err)
		return nil, errors.New("error getting volume id")
	}
	volID, _ := strconv.Atoi(volproto.VolumeID)

	nodeNameIP := strings.Split(req.GetNodeId(), "$$")
	if len(nodeNameIP) != 2 {
		return nil, errors.New("not found Node ID")
	}
	hostName := nodeNameIP[0]

	host, err := fc.cs.validateHost(hostName)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	v, err := fc.cs.api.GetVolume(volID)
	if err != nil {
		klog.Errorf("Failed to find volume by volume ID '%s': %v", req.GetVolumeId(), err)
		return nil, errors.New("error getting volume by id")
	}

	// TODO: revisit this as part of CSIC-343
	_, err = fc.cs.accessModesHelper.IsValidAccessMode(v, req)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	lunList, err := fc.cs.api.GetAllLunByHost(host.ID)
	if err != nil {
		return nil, err
	}
	ports := ""
	if len(host.Ports) > 0 {
		for _, port := range host.Ports {
			if port.PortType == "FC" {
				ports = ports + "," + port.PortAddress
			}
		}
	}
	if ports != "" {
		ports = ports[1:]
	}
	for _, lun := range lunList {
		if lun.VolumeID == volID {
			volCtx := make(map[string]string)
			volCtx["lun"] = strconv.Itoa(lun.Lun)
			volCtx["hostID"] = strconv.Itoa(host.ID)
			volCtx["hostPorts"] = ports
			klog.V(4).Infof("volumeID %d already mapped to host %s", lun.VolumeID, host.Name)
			return &csi.ControllerPublishVolumeResponse{
				PublishContext: volCtx,
			}, nil
		}
	}

	maxAllowedVol, err := strconv.Atoi(req.GetVolumeContext()["max_vols_per_host"])
	if err != nil {
		klog.Errorf("Invalid parameter max_vols_per_host error:  %v", err)
		return nil, err
	}
	klog.V(4).Infof("host can have maximum %d volume mapped", maxAllowedVol)
	klog.V(4).Infof("host %s has %d volume mapped", host.Name, len(lunList))
	if len(lunList) >= maxAllowedVol {
		klog.Errorf("unable to publish volume on host %s, as maximum allowed volume per host is (%d), limit reached", host.Name, maxAllowedVol)
		return nil, status.Error(codes.Internal, "Unable to publish volume as max allowed volume (per host) limit reached")
	}
	// map volume to host
	klog.V(4).Infof("mapping volume %d to host %s", volID, host.Name)
	luninfo, err := fc.cs.mapVolumeTohost(volID, host.ID)
	if err != nil {
		klog.Errorf("Failed to map volume to host with error %v", err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	volCtx := make(map[string]string)
	volCtx["lun"] = strconv.Itoa(luninfo.Lun)
	volCtx["hostID"] = strconv.Itoa(host.ID)
	volCtx["hostPorts"] = ports
	return &csi.ControllerPublishVolumeResponse{
		PublishContext: volCtx,
	}, nil
}

func (fc *fcstorage) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (resp *csi.ControllerUnpublishVolumeResponse, err error) {
	klog.V(2).Infof("ControllerUnpublishVolume called with nodeID %s and volumeId %s", req.GetNodeId(), req.GetVolumeId())
	volproto, err := validateVolumeID(req.GetVolumeId())
	if err != nil {
		klog.Errorf("failed to validate storage type %v", err)
		return nil, errors.New("error getting volume id")
	}
	nodeNameIP := strings.Split(req.GetNodeId(), "$$")
	if len(nodeNameIP) != 2 {
		return nil, errors.New("Node ID not found")
	}
	hostName := nodeNameIP[0]
	host, err := fc.cs.api.GetHostByName(hostName)
	if err != nil {
		if strings.Contains(err.Error(), "HOST_NOT_FOUND") {
			return &csi.ControllerUnpublishVolumeResponse{}, nil
		}
		klog.Errorf("failed to get host details with error %v", err)
		return nil, err
	}
	if len(host.Luns) > 0 {
		volID, _ := strconv.Atoi(volproto.VolumeID)
		klog.V(4).Infof("unmap volume %d from host %d", volID, host.ID)
		err = fc.cs.unmapVolumeFromHost(host.ID, volID)
		if err != nil {
			klog.Errorf("failed to unmap volume %d from host %d with error %v", volID, host.ID, err)
			return nil, status.Error(codes.Internal, err.Error())
		}
	}
	if len(host.Luns) < 2 {
		luns, err := fc.cs.api.GetAllLunByHost(host.ID)
		if err != nil {
			klog.Errorf("failed to retrive luns for host %d with error %v", host.ID, err)
		}
		if len(luns) == 0 {
			err = fc.cs.api.DeleteHost(host.ID)
			if err != nil && !strings.Contains(err.Error(), "HOST_NOT_FOUND") {
				klog.Errorf("failed to delete host with error %v", err)
				return nil, status.Error(codes.Internal, err.Error())
			}
		}
	}
	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

func (fc *fcstorage) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (resp *csi.ValidateVolumeCapabilitiesResponse, err error) {
	klog.V(2).Infof("ValidateVolumeCapabilities called with volumeId %s", req.GetVolumeId())
	volproto, err := validateVolumeID(req.GetVolumeId())
	if err != nil {
		klog.Errorf("Failed to validate storage type %v", err)
		return nil, errors.New("error getting volume id")
	}
	volID, _ := strconv.Atoi(volproto.VolumeID)

	klog.V(4).Infof("volID: %d", volID)
	v, err := fc.cs.api.GetVolume(volID)
	if err != nil {
		klog.Errorf("Failed to find volume ID: %d, %v", volID, err)
		err = status.Errorf(codes.NotFound, "ValidateVolumeCapabilities failed to find volume ID: %d, %v", volID, err)
	}
	klog.V(4).Infof("volID: %d colume: %v", volID, v)

	// TODO: revisit this as part of CSIC-343
	// _, err = iscsi.cs.accessModesHelper.IsValidAccessMode(v, req)
	// if err != nil {
	// 	   return nil, status.Error(codes.Internal, err.Error())
	// }

	resp = &csi.ValidateVolumeCapabilitiesResponse{
		Confirmed: &csi.ValidateVolumeCapabilitiesResponse_Confirmed{
			VolumeCapabilities: req.GetVolumeCapabilities(),
		},
	}
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
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("Recovered from FC CreateSnapshot  " + fmt.Sprint(res))
		}
	}()
	var snapshotID string
	snapshotName := req.GetName()
	klog.V(4).Infof("Create Snapshot of name %s", snapshotName)
	klog.V(4).Infof("Create Snapshot called with volume Id %s", req.GetSourceVolumeId())
	volproto, err := validateVolumeID(req.GetSourceVolumeId())
	if err != nil {
		klog.Errorf("failed to validate storage type %v", err)
		return
	}

	sourceVolumeID, _ := strconv.Atoi(volproto.VolumeID)
	volumeSnapshot, err := fc.cs.api.GetVolumeByName(snapshotName)
	if err != nil {
		klog.V(4).Infof("Snapshot with given name not found : %s", snapshotName)
	} else if volumeSnapshot.ParentId == sourceVolumeID {
		snapshotID = strconv.Itoa(volumeSnapshot.ID) + "$$" + volproto.StorageType
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

	snapshotParam := &api.VolumeSnapshot{
		ParentID:       sourceVolumeID,
		SnapshotName:   snapshotName,
		WriteProtected: true,
	}

	snapshot, err := fc.cs.api.CreateSnapshotVolume(snapshotParam)
	if err != nil {
		klog.Errorf("Failed to create snapshot %s error %v", snapshotName, err)
		return
	}

	snapshotID = strconv.Itoa(snapshot.SnapShotID) + "$$" + volproto.StorageType
	csiSnapshot := &csi.Snapshot{
		SnapshotId:     snapshotID,
		SourceVolumeId: req.GetSourceVolumeId(),
		ReadyToUse:     true,
		CreationTime:   timestamppb.Now(),
		SizeBytes:      snapshot.Size,
	}
	klog.V(4).Infof("CreateFileSystemSnapshot resp: %v", csiSnapshot)
	snapshotResp := &csi.CreateSnapshotResponse{Snapshot: csiSnapshot}
	return snapshotResp, nil
}

func (fc *fcstorage) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (resp *csi.DeleteSnapshotResponse, err error) {
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("Recovered from FC DeleteSnapshot  " + fmt.Sprint(res))
		}
	}()

	snapshotID, _ := strconv.Atoi(req.GetSnapshotId())
	err = fc.ValidateDeleteVolume(snapshotID)
	if err != nil {
		klog.Errorf("failed to delete snapshot %v", err)
		return nil, err
	}
	return &csi.DeleteSnapshotResponse{}, nil
}

func (fc *fcstorage) ValidateDeleteVolume(volumeID int) (err error) {
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("Recovered from FC DeleteSnapshot  " + fmt.Sprint(res))
		}
	}()
	vol, err := fc.cs.api.GetVolume(volumeID)
	if err != nil {
		if strings.Contains(err.Error(), "VOLUME_NOT_FOUND") {
			klog.V(2).Infof("volume is already deleted %d", volumeID)
			return nil
		}
		return status.Errorf(codes.Internal,
			"error while validating volume status : %s",
			err.Error())
	}
	childVolumes, err := fc.cs.api.GetVolumeSnapshotByParentID(vol.ID)
	if len(*childVolumes) > 0 {
		metadata := make(map[string]interface{})
		metadata[TOBEDELETED] = true
		_, err = fc.cs.api.AttachMetadataToObject(int64(vol.ID), metadata)
		if err != nil {
			klog.Errorf("failed to update host.k8s.to_be_deleted for volume %s error: %v", vol.Name, err)
			err = errors.New("error while Set metadata host.k8s.to_be_deleted")
		}
		return
	}
	klog.V(2).Infof("Deleting volume name: %s id: %d", vol.Name, vol.ID)
	err = fc.cs.api.DeleteVolume(vol.ID)
	if err != nil {
		return status.Errorf(codes.Internal,
			"error removing volume: %s", err.Error())
	}
	if vol.ParentId != 0 {
		klog.V(2).Infof("checkingif parent volume can be name: %s id: %d", vol.Name, vol.ID)
		tobedel := fc.cs.api.GetMetadataStatus(int64(vol.ParentId))
		if tobedel {
			err = fc.ValidateDeleteVolume(vol.ParentId)
			if err != nil {
				return
			}
		}
	}
	return
}

func (fc *fcstorage) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (resp *csi.ControllerExpandVolumeResponse, err error) {
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("Recovered from FC ControllerExpandVolume " + fmt.Sprint(res))
		}
	}()

	volumeID, err := strconv.Atoi(req.GetVolumeId())
	if err != nil {
		klog.Errorf("Invalid Volume ID %v", err)
		return
	}

	capacity := int64(req.GetCapacityRange().GetRequiredBytes())
	if capacity < gib {
		capacity = gib
		klog.Warningf("Volume Minimum capacity should be greater 1 GB")
	}

	// Expand volume size
	var volume api.Volume
	volume.Size = capacity
	_, err = fc.cs.api.UpdateVolume(volumeID, volume)
	if err != nil {
		klog.Errorf("Failed to update file system %v", err)
		return
	}
	klog.V(2).Info("Volume size updated successfully")
	return &csi.ControllerExpandVolumeResponse{
		CapacityBytes:         capacity,
		NodeExpansionRequired: false,
	}, nil
}
