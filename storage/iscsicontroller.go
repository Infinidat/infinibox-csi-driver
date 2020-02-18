package storage

import (
	"context"
	"errors"
	"fmt"
	"infinibox-csi-driver/api"
	"strconv"
	"strings"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (iscsi *iscsistorage) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	cr := req.GetCapacityRange()
	sizeBytes, err := verifyVolumeSize(cr)
	if err != nil {
		return &csi.CreateVolumeResponse{}, err
	}
	log.Infof("---------------------------------- requested size in bytes is %d ", sizeBytes)
	params := req.GetParameters()
	log.Infof(" csi request parameters %v", params)

	// Get Volume Provision Type
	volType := "THIN"
	if prosiontype, ok := params[KeyVolumeProvisionType]; ok {
		volType = prosiontype
	}

	// Volume name to be created
	name := req.GetName()
	log.Infof("csi voume name from request is %s", name)
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

	networkSpace := req.GetParameters()["nfs_networkspace"]
	nspace, err := iscsi.cs.api.GetNetworkSpaceByName(networkSpace)
	if err != nil {
		return nil, fmt.Errorf("Error getting network space")
	}
	log.Info("create volume nspace --------------------------->", nspace)
	log.Info("create volume  nspace Properties.IscsiIqn --------------------------->", nspace.Properties.IscsiIqn)
	log.Info("create volume nspace Portals --------------------------->", nspace.Portals)

	portals := ""
	for _, p := range nspace.Portals {
		portals = portals + "," + p.IpAdress
	}
	portals = portals[0:]
	req.GetParameters()["iqn"] = nspace.Properties.IscsiIqn
	req.GetParameters()["portals"] = portals

	// We require the storagePool name for creation
	sp, ok := req.GetParameters()["pool_name"]
	if !ok {
		return &csi.CreateVolumeResponse{}, errors.New("pool_name is a required parameter")
	}

	// Volume content source support Snapshots only
	contentSource := req.GetVolumeContentSource()
	if contentSource != nil {
		log.Info("request is to create volume from snapshot")
		return iscsi.createVolumeFromVolumeContent(req, name, sizeBytes, sp)

	}
	volumeParam := &api.VolumeParam{
		Name:          name,
		VolumeSize:    sizeBytes,
		ProvisionType: volType,
	}
	volumeResp, err := iscsi.cs.api.CreateVolume(volumeParam, sp)
	if err != nil {
		log.Errorf("error creating volume: %s pool %s error: %s", name, sp, err.Error())
		return &csi.CreateVolumeResponse{}, status.Errorf(codes.Internal,
			"error when creating volume %s storagepool %s: %s", name, sp, err.Error())

	}
	vi := iscsi.cs.getCSIVolume(volumeResp)

	luninfo, err := iscsi.cs.mapVolumeTohost(volumeResp.ID)
	if err != nil {
		return &csi.CreateVolumeResponse{}, status.Error(codes.Internal, err.Error())
	}
	vi.VolumeContext["lun"] = strconv.Itoa(luninfo.Lun)
	copyRequestParameters(req.GetParameters(), vi.VolumeContext)
	log.Info("create volume response vi-------------------->", vi)
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
	return csiResp, err
}

func (iscsi *iscsistorage) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	log.Debug("Called DeleteVolume")
	if req.GetVolumeId() == "" {
		return nil, status.Errorf(codes.Internal,
			"error parsing volume id : %s", errors.New("Volume id not found"))
	}
	id, err := strconv.Atoi(req.GetVolumeId())
	if err != nil {
		return nil, status.Errorf(codes.Internal,
			"error parsing volume id : %s", err.Error())
	}
	vol, err := iscsi.cs.getVolumeByID(id)
	if err != nil {
		if strings.Contains(err.Error(), "VOLUME_NOT_FOUND") {
			log.WithFields(log.Fields{"id": id}).Debug("volume is already deleted", id)
			return &csi.DeleteVolumeResponse{}, nil
		}

		return nil, status.Errorf(codes.Internal,
			"error while validating volume status : %s",
			err.Error())
	}
	if vol.Mapped {
		// Volume is mapped
		err = iscsi.cs.unMapVolumeFromhost(vol.ID)
		if err != nil {
			return nil, status.Errorf(codes.FailedPrecondition,
				"error while unmaping volume %s", err)
		}
	}

	log.WithFields(log.Fields{"name": vol.Name, "id": id}).Info("Deleting volume")
	err = iscsi.cs.deleteVolume(vol.ID)
	if err != nil {
		return nil, status.Errorf(codes.Internal,
			"error removing volume: %s", err.Error())
	}

	vol, err = iscsi.cs.getVolumeByID(id)
	if err != nil && !strings.Contains(err.Error(), "VOLUME_NOT_FOUND") {
		return nil, err
	}
	return &csi.DeleteVolumeResponse{}, nil
}

func (iscsi *iscsistorage) createVolumeFromVolumeContent(req *csi.CreateVolumeRequest, name string, sizeInKbytes int64, storagePool string) (*csi.CreateVolumeResponse, error) {
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
	snapshotID, err := strconv.Atoi(volumeContentID)
	if err != nil {
		return nil, errors.New("error getting volume id")
	}
	srcVol, err := iscsi.cs.api.GetVolume(snapshotID)
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
	snapStoragePool := iscsi.cs.getStoragePoolNameFromID(srcVol.PoolId)
	if snapStoragePool != storagePool {
		return nil, status.Errorf(codes.InvalidArgument,
			restoreType+" storage pool %s is different than the requested storage pool %s", snapStoragePool, storagePool)
	}

	vol, err := iscsi.cs.api.GetVolumeByName(name)
	if vol.PoolId == srcVol.PoolId {
		log.Errorf("Requested volume %s already exists", name)
		csiVolume := iscsi.cs.getCSIVolume(vol)
		log.Errorf("Requested volume already exists %s (%s) storage pool %s",
			csiVolume.VolumeContext["Name"], csiVolume.VolumeId, csiVolume.VolumeContext[StoragePoolKey])
		return &csi.CreateVolumeResponse{Volume: csiVolume}, nil
	}

	snapshotParam := &api.VolumeSnapshot{ParentID: snapshotID, SnapshotName: name, WriteProtected: false}

	// Create snapshot
	snapResponse, err := iscsi.cs.api.CreateSnapshotVolume(snapshotParam)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to create snapshot: %s", err.Error())
	}

	// Retrieve created destination volumevolume
	volID := snapResponse.SnapShotID
	dstVol, err := iscsi.cs.api.GetVolume(volID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not retrieve created volume: %d", volID)
	}
	// Create a volume response and return it
	csiVolume := iscsi.cs.getCSIVolume(dstVol)
	copyRequestParameters(req.GetParameters(), csiVolume.VolumeContext)

	log.Errorf("Volume (from snap) %s (%s) storage pool %s",
		csiVolume.VolumeContext["Name"], csiVolume.VolumeId, csiVolume.VolumeContext["StoragePoolName"])
	return &csi.CreateVolumeResponse{Volume: csiVolume}, nil
}

func (iscsi *iscsistorage) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	return nil, nil
}

func (iscsi *iscsistorage) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	return nil, nil
}
func (iscsi *iscsistorage) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	return nil, nil
}

func (iscsi *iscsistorage) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	return nil, nil
}

func (iscsi *iscsistorage) ListSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	return nil, nil
}
func (iscsi *iscsistorage) GetCapacity(ctx context.Context, req *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error) {
	return nil, nil
}
func (iscsi *iscsistorage) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	return nil, nil
}
func (iscsi *iscsistorage) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	return nil, nil
}
func (iscsi *iscsistorage) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	return nil, nil
}

func (iscsi *iscsistorage) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (resp *csi.ControllerExpandVolumeResponse, err error) {
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("Recovered from ISCSI CreateSnapshot  " + fmt.Sprint(res))
		}
	}()

	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "Volume ID missing in request")
	}

	if req.GetCapacityRange() == nil {
		return nil, status.Error(codes.InvalidArgument, "CapacityRange cannot be empty")
	}

	volproto, err := validateStorageType(req.GetVolumeId())
	if err != nil {
		log.Errorf("fail to validate storage type %v", err)
		return
	}

	volumeID, err := strconv.Atoi(volproto.VolumeID)
	if err != nil {
		log.Errorf("Invalid Volume ID %v", err)
		return
	}

	capacity := int64(req.GetCapacityRange().GetRequiredBytes())
	if capacity < gib {
		capacity = gib
		log.Warn("Volume Minimum capacity should be greater 1 GB")
	}

	var volume api.Volume
	volume.Size = capacity
	// Expand volume size
	_, err = iscsi.cs.api.UpdateVolume(volumeID, volume)
	if err != nil {
		log.Errorf("Failed to update file system %v", err)
		return
	}
	log.Infoln("Filesystem updated successfully")
	return &csi.ControllerExpandVolumeResponse{
		CapacityBytes:         capacity,
		NodeExpansionRequired: false,
	}, nil
}
