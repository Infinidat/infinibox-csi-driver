/*Copyright 2020 Infinidat
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
	log "infinibox-csi-driver/helper/logger"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/protobuf/ptypes"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (iscsi *iscsistorage) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("Recovered from ISCSI CreateVolume " + fmt.Sprint(res))
		}
	}()
	cr := req.GetCapacityRange()
	sizeBytes, err := verifyVolumeSize(cr)
	if err != nil {
		return &csi.CreateVolumeResponse{}, err
	}
	klog.V(2).Infof("requested size in bytes is %d ", sizeBytes)
	params := req.GetParameters()
	klog.V(2).Infof(" csi request parameters %v", params)
	err = validateParametersiSCSI(params)
	if err != nil {
		return &csi.CreateVolumeResponse{}, status.Error(codes.InvalidArgument, err.Error())
	}
	// Get Volume Provision Type
	volType := "THIN"
	if prosiontype, ok := params[KeyVolumeProvisionType]; ok {
		volType = prosiontype
	}

	// Access Mode check
	volCaps := req.GetVolumeCapabilities()
	if volCaps == nil {
		return nil, status.Error(codes.InvalidArgument, "Volume capability not provided")
	}
	for _, volCap := range volCaps {
		if volCap.GetAccessMode().GetMode() != csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER {
			klog.Errorf("volume cpability %s for ISCSI is not supported", volCap.GetAccessMode().GetMode().String())
			return &csi.CreateVolumeResponse{}, fmt.Errorf("volume cpability %s for ISCSI is not supported", volCap.GetAccessMode().GetMode().String())
		}
	}

	// Volume name to be created
	name := req.GetName()
	klog.V(2).Infof("csi voume name from request is %s", name)
	if name == "" {
		return &csi.CreateVolumeResponse{}, errors.New("Name cannot be empty")
	}

	targetVol, err := iscsi.cs.api.GetVolumeByName(name)
	if err != nil {
		if !strings.Contains(err.Error(), "volume with given name not found") {
			return &csi.CreateVolumeResponse{}, status.Error(codes.Internal, err.Error())
		}
	}
	if targetVol != nil {
		return &csi.CreateVolumeResponse{}, nil
	}

	networkSpace := req.GetParameters()["network_space"]
	nspace, err := iscsi.cs.api.GetNetworkSpaceByName(networkSpace)
	if err != nil {
		return nil, fmt.Errorf("Error getting network space")
	}
	portals := ""
	for _, p := range nspace.Portals {
		portals = portals + "," + p.IpAdress
	}
	portals = portals[1:]
	req.GetParameters()["iqn"] = nspace.Properties.IscsiIqn
	req.GetParameters()["portals"] = portals

	// We require the storagePool name for creation
	poolName, ok := req.GetParameters()["pool_name"]
	if !ok {
		return &csi.CreateVolumeResponse{}, errors.New("pool_name is a required parameter")
	}
	fstype := req.GetParameters()["fstype"]
	// Volume content source support volume and snapshots
	contentSource := req.GetVolumeContentSource()
	if contentSource != nil {
		return iscsi.createVolumeFromVolumeContent(req, name, sizeBytes, poolName)

	}
	ssd := req.GetParameters()["ssd_enabled"]
	if ssd == "" {
		ssd = fmt.Sprint(false)
	}
	ssdEnabled, _ := strconv.ParseBool(ssd)
	volumeParam := &api.VolumeParam{
		Name:          name,
		VolumeSize:    sizeBytes,
		ProvisionType: volType,
		SsdEnabled:    ssdEnabled,
	}
	volumeResp, err := iscsi.cs.api.CreateVolume(volumeParam, poolName)
	if err != nil {
		klog.Errorf("error creating volume: %s pool %s error: %s", name, poolName, err.Error())
		return &csi.CreateVolumeResponse{}, status.Errorf(codes.Internal,
			"error when creating volume %s storagepool %s: %s", name, poolName, err.Error())

	}
	vi := iscsi.cs.getCSIResponse(volumeResp, req)

	copyRequestParameters(req.GetParameters(), vi.VolumeContext)
	csiResp := &csi.CreateVolumeResponse{
		Volume: vi,
	}
	volID, err := strconv.Atoi(vi.VolumeId)
	if err != nil {
		return &csi.CreateVolumeResponse{}, errors.New("error getting volume id")
	}

	// confirm volume creation
	vol, err := iscsi.cs.api.GetVolume(volID)
	counter := 0
	if vol == nil {
		for err != nil && counter < 100 {
			time.Sleep(3 * time.Millisecond)
			vol, err = iscsi.cs.api.GetVolume(volID)
			counter = counter + 1
		}
	}
	metadata := make(map[string]interface{})
	metadata["host.k8s.pvname"] = vol.Name
	metadata["host.filesystem_type"] = fstype
	_, err = iscsi.cs.api.AttachMetadataToObject(int64(vol.ID), metadata)
	if err != nil {
		klog.Errorf("fail to attach metadata for volume : %s", vol.Name)
		klog.Errorf("error to attach metadata %v", err)
		return &csi.CreateVolumeResponse{}, errors.New("error attach metadata")
	}
	return csiResp, err
}

func (iscsi *iscsistorage) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (csiResp *csi.DeleteVolumeResponse, err error) {
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("Recovered from ISCSI DeleteSnapshot  " + fmt.Sprint(res))
		}
	}()
	klog.V(4).Infof("Called DeleteVolume")
	if req.GetVolumeId() == "" {
		return &csi.DeleteVolumeResponse{}, status.Errorf(codes.Internal,
			"error parsing volume id : %s", errors.New("Volume id not found"))
	}
	id, err := strconv.Atoi(req.GetVolumeId())
	if err != nil {
		return &csi.DeleteVolumeResponse{}, status.Errorf(codes.Internal,
			"error parsing volume id : %s", err.Error())
	}
	err = iscsi.ValidateDeleteVolume(id)
	if err != nil {
		return &csi.DeleteVolumeResponse{}, status.Errorf(codes.Internal,
			"error deleting volume : %s", err.Error())
	}
	return &csi.DeleteVolumeResponse{}, nil
}

func (iscsi *iscsistorage) createVolumeFromVolumeContent(req *csi.CreateVolumeRequest, name string, sizeInKbytes int64, storagePool string) (*csi.CreateVolumeResponse, error) {
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("Recovered from ISCSI createVolumeFromVolumeContent " + fmt.Sprint(res))
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

	// Lookup the snapshot source volume.
	volproto, err := validateStorageType(volumeContentID)
	if err != nil {
		klog.Errorf("Failed to validate storage type %v", err)
		return nil, errors.New("error getting volume id")
	}

	ID, err := strconv.Atoi(volproto.VolumeID)
	if err != nil {
		return nil, errors.New("error getting volume id")
	}
	srcVol, err := iscsi.cs.api.GetVolume(ID)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, restoreType+" not found: %s", volumeContentID)
	}

	// Validate the size is the same.
	if int64(srcVol.Size) != sizeInKbytes {
		return nil, status.Errorf(codes.InvalidArgument,
			restoreType+" %s has incompatible size %d kbytes with requested %d kbytes",
			volumeContentID, srcVol.Size, sizeInKbytes)
	}

	// Validate the storagePool is the same.
	storagePoolID, err := iscsi.cs.api.GetStoragePoolIDByName(storagePool)
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
	snapshotParam := &api.VolumeSnapshot{ParentID: ID, SnapshotName: name, WriteProtected: false, SsdEnabled: ssdEnabled}

	// Create snapshot
	snapResponse, err := iscsi.cs.api.CreateSnapshotVolume(snapshotParam)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to create snapshot: %s", err.Error())
	}

	// Retrieve created destination volume
	volID := snapResponse.SnapShotID
	dstVol, err := iscsi.cs.api.GetVolume(volID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not retrieve created volume: %d", volID)
	}

	// Create a volume response and return it
	csiVolume := iscsi.cs.getCSIResponse(dstVol, req)
	copyRequestParameters(req.GetParameters(), csiVolume.VolumeContext)

	metadata := make(map[string]interface{})
	metadata["host.k8s.pvname"] = dstVol.Name
	metadata["host.filesystem_type"] = req.GetParameters()["fstype"]
	_, err = iscsi.cs.api.AttachMetadataToObject(int64(dstVol.ID), metadata)
	if err != nil {
		klog.Errorf("fail to attach metadata for volume : %s", dstVol.Name)
		klog.Errorf("error to attach metadata %v", err)
		return &csi.CreateVolumeResponse{}, errors.New("error attach metadata")
	}
	klog.Errorf("Volume (from snap) %s (%s) storage pool %s",
		csiVolume.VolumeContext["Name"], csiVolume.VolumeId, csiVolume.VolumeContext["StoragePoolName"])
	return &csi.CreateVolumeResponse{Volume: csiVolume}, nil
}

func (iscsi *iscsistorage) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (resp *csi.ControllerPublishVolumeResponse, err error) {
	klog.V(2).Infof("ControllerPublishVolume called with nodeID %s and volumeId %s", req.GetNodeId(), req.GetVolumeId())
	volproto, err := validateStorageType(req.GetVolumeId())
	if err != nil {
		klog.Errorf("Failed to validate storage type %v", err)
		return &csi.ControllerPublishVolumeResponse{}, errors.New("error getting volume id")
	}
	volID, _ := strconv.Atoi(volproto.VolumeID)

	nodeNameIP := strings.Split(req.GetNodeId(), "$$")
	if len(nodeNameIP) != 2 {
		return &csi.ControllerPublishVolumeResponse{}, errors.New("Node ID not found")
	}
	hostName := nodeNameIP[0]

	host, err := iscsi.cs.validateHost(hostName)
	if err != nil {
		return &csi.ControllerPublishVolumeResponse{}, status.Error(codes.Internal, err.Error())
	}

	ports := ""
	if len(host.Ports) > 0 {
		for _, port := range host.Ports {
			if port.PortType == "ISCSI" {
				ports = ports + "," + port.PortAddress
			}
		}
	}
	if ports != "" {
		ports = ports[1:]
	}

	lunList, err := iscsi.cs.api.GetAllLunByHost(host.ID)
	if err != nil {
		return &csi.ControllerPublishVolumeResponse{}, err
	}
	for _, lun := range lunList {
		if lun.VolumeID == volID {
			volCtx := make(map[string]string)
			volCtx["lun"] = strconv.Itoa(lun.Lun)
			volCtx["hostID"] = strconv.Itoa(host.ID)
			volCtx["hostPorts"] = ports
			return &csi.ControllerPublishVolumeResponse{
				PublishContext: volCtx,
			}, nil
		}
	}
	maxAllowedVol, err := strconv.Atoi(req.GetVolumeContext()["max_vols_per_host"])
	if err != nil {
		klog.Errorf("Invalid parameter max_vols_per_host error:  %v", err)
		return &csi.ControllerPublishVolumeResponse{}, err
	}
	klog.V(4).Infof("host can have maximum %d volume mapped", maxAllowedVol)
	klog.V(4).Infof("host %s has %d volume mapped", host.Name, len(lunList))
	if len(lunList) >= maxAllowedVol {
		klog.Errorf("unable to publish volume on host %s, as maximum allowed volume per host is (%d), limit reached", host.Name, maxAllowedVol)
		return &csi.ControllerPublishVolumeResponse{}, status.Error(codes.Internal, "Unable to publish volume as max allowed volume (per host) limit reached")
	}
	// map volume to host
	klog.V(4).Infof("mapping volume %d to host %s", volID, host.Name)
	luninfo, err := iscsi.cs.mapVolumeTohost(volID, host.ID)
	if err != nil {
		klog.Errorf("Failed to map volume to host with error %v", err)
		return &csi.ControllerPublishVolumeResponse{}, status.Error(codes.Internal, err.Error())
	}

	volCtx := make(map[string]string)
	volCtx["lun"] = strconv.Itoa(luninfo.Lun)
	volCtx["hostID"] = strconv.Itoa(host.ID)
	volCtx["hostPorts"] = ports
	volCtx["securityMethod"] = host.SecurityMethod
	return &csi.ControllerPublishVolumeResponse{
		PublishContext: volCtx,
	}, nil
}

func (iscsi *iscsistorage) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (resp *csi.ControllerUnpublishVolumeResponse, err error) {
	klog.V(2).Infof("ControllerUnpublishVolume called with nodeID %s and volumeId %s", req.GetNodeId(), req.GetVolumeId())
	volproto, err := validateStorageType(req.GetVolumeId())
	if err != nil {
		klog.Errorf("failed to validate storage type %v", err)
		return nil, errors.New("error getting volume id")
	}
	nodeNameIP := strings.Split(req.GetNodeId(), "$$")
	if len(nodeNameIP) != 2 {
		return &csi.ControllerUnpublishVolumeResponse{}, errors.New("Node ID not found")
	}
	hostName := nodeNameIP[0]
	host, err := iscsi.cs.api.GetHostByName(hostName)
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
		err = iscsi.cs.unmapVolumeFromHost(host.ID, volID)
		if err != nil {
			klog.Errorf("failed to unmap volume %d from host %d with error %v", volID, host.ID, err)
			return &csi.ControllerUnpublishVolumeResponse{}, status.Error(codes.Internal, err.Error())
		}
	}
	if len(host.Luns) < 2 {
		luns, err := iscsi.cs.api.GetAllLunByHost(host.ID)
		if err != nil {
			klog.Errorf("failed to retrive luns for host %d with error %v", host.ID, err)
		}
		if len(luns) == 0 {
			err = iscsi.cs.api.DeleteHost(host.ID)
			if err != nil && !strings.Contains(err.Error(), "HOST_NOT_FOUND") {
				klog.Errorf("failed to delete host with error %v", err)
				return &csi.ControllerUnpublishVolumeResponse{}, status.Error(codes.Internal, err.Error())
			}
		}
	}
	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

func (iscsi *iscsistorage) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (resp *csi.ValidateVolumeCapabilitiesResponse, err error) {
	return &csi.ValidateVolumeCapabilitiesResponse{}, nil
}

func (iscsi *iscsistorage) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (resp *csi.ListVolumesResponse, err error) {
	return &csi.ListVolumesResponse{}, nil
}

func (iscsi *iscsistorage) ListSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (resp *csi.ListSnapshotsResponse, err error) {
	return &csi.ListSnapshotsResponse{}, nil
}
func (iscsi *iscsistorage) GetCapacity(ctx context.Context, req *csi.GetCapacityRequest) (resp *csi.GetCapacityResponse, err error) {
	return &csi.GetCapacityResponse{}, nil
}
func (iscsi *iscsistorage) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (resp *csi.ControllerGetCapabilitiesResponse, err error) {
	return &csi.ControllerGetCapabilitiesResponse{}, nil
}
func (iscsi *iscsistorage) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (resp *csi.CreateSnapshotResponse, err error) {
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("Recovered from ISCSI CreateSnapshot  " + fmt.Sprint(res))
		}
	}()
	var snapshotID string
	snapshotName := req.GetName()
	klog.V(4).Infof("Create Snapshot of name %s", snapshotName)
	klog.V(4).Infof("Create Snapshot called with volume Id %s", req.GetSourceVolumeId())
	volproto, err := validateStorageType(req.GetSourceVolumeId())
	if err != nil {
		klog.Errorf("fail to validate storage type %v", err)
		return
	}

	sourceVolumeID, _ := strconv.Atoi(volproto.VolumeID)
	volumeSnapshot, err := iscsi.cs.api.GetVolumeByName(snapshotName)
	if err != nil {
		klog.V(4).Infof("Snapshot with given name not found : ", snapshotName)
	} else if volumeSnapshot.ParentId == sourceVolumeID {
		snapshotID = strconv.Itoa(volumeSnapshot.ID) + "$$" + volproto.StorageType
		return &csi.CreateSnapshotResponse{
			Snapshot: &csi.Snapshot{
				SizeBytes:      volumeSnapshot.Size,
				SnapshotId:     snapshotID,
				SourceVolumeId: req.GetSourceVolumeId(),
				CreationTime:   ptypes.TimestampNow(),
				ReadyToUse:     true,
			},
		}, nil
	}

	snapshotParam := &api.VolumeSnapshot{
		ParentID:       sourceVolumeID,
		SnapshotName:   snapshotName,
		WriteProtected: true,
	}

	snapshot, err := iscsi.cs.api.CreateSnapshotVolume(snapshotParam)
	if err != nil {
		klog.Errorf("Failed to create snapshot %s error %v", snapshotName, err)
		return
	}

	snapshotID = strconv.Itoa(snapshot.SnapShotID) + "$$" + volproto.StorageType
	csiSnapshot := &csi.Snapshot{
		SnapshotId:     snapshotID,
		SourceVolumeId: req.GetSourceVolumeId(),
		ReadyToUse:     true,
		CreationTime:   ptypes.TimestampNow(),
		SizeBytes:      snapshot.Size,
	}
	klog.V(4).Infof("CreateFileSystemSnapshot resp() ", csiSnapshot)
	snapshotResp := &csi.CreateSnapshotResponse{Snapshot: csiSnapshot}
	return snapshotResp, nil
}

func (iscsi *iscsistorage) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (resp *csi.DeleteSnapshotResponse, err error) {
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("Recovered from ISCSI DeleteSnapshot  " + fmt.Sprint(res))
		}
	}()

	snapshotID, _ := strconv.Atoi(req.GetSnapshotId())
	err = iscsi.ValidateDeleteVolume(snapshotID)
	if err != nil {
		klog.Errorf("fail to delete snapshot %v", err)
		return &csi.DeleteSnapshotResponse{}, err
	}
	return &csi.DeleteSnapshotResponse{}, nil
}

func (iscsi *iscsistorage) ValidateDeleteVolume(volumeID int) (err error) {
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("Recovered from ISCSI DeleteSnapshot  " + fmt.Sprint(res))
		}
	}()
	vol, err := iscsi.cs.api.GetVolume(volumeID)
	if err != nil {
		if strings.Contains(err.Error(), "VOLUME_NOT_FOUND") {
			log.WithFields(log.Fields{"id": volumeID}).Debug("volume is already deleted", volumeID)
			return nil
		}
		return status.Errorf(codes.Internal,
			"error while validating volume status : %s",
			err.Error())
	}
	childVolumes, err := iscsi.cs.api.GetVolumeSnapshotByParentID(vol.ID)
	if len(*childVolumes) > 0 {
		metadata := make(map[string]interface{})
		metadata[TOBEDELETED] = true
		_, err = iscsi.cs.api.AttachMetadataToObject(int64(vol.ID), metadata)
		if err != nil {
			klog.Errorf("fail to update host.k8s.to_be_deleted for volume %s error: %v", vol.Name, err)
			err = errors.New("error while Set metadata host.k8s.to_be_deleted")
		}
		return
	}
	log.WithFields(log.Fields{"name": vol.Name, "id": vol.ID}).Info("Deleting volume")
	err = iscsi.cs.api.DeleteVolume(vol.ID)
	if err != nil {
		return status.Errorf(codes.Internal,
			"error removing volume: %s", err.Error())
	}
	if vol.ParentId != 0 {
		log.WithFields(log.Fields{"name": vol.Name, "id": vol.ID}).Info("Checking if Parent volume can be")
		tobedel := iscsi.cs.api.GetMetadataStatus(int64(vol.ParentId))
		if tobedel {
			err = iscsi.ValidateDeleteVolume(vol.ParentId)
			if err != nil {
				return
			}
		}
	}
	return
}

func (iscsi *iscsistorage) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (resp *csi.ControllerExpandVolumeResponse, err error) {
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("Recovered from ISCSI ControllerExpandVolume " + fmt.Sprint(res))
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
	_, err = iscsi.cs.api.UpdateVolume(volumeID, volume)
	if err != nil {
		klog.Errorf("Failed to update file system %v", err)
		return
	}
	log.Infoln("Volume size updated successfully")
	return &csi.ControllerExpandVolumeResponse{
		CapacityBytes:         capacity,
		NodeExpansionRequired: false,
	}, nil
}
