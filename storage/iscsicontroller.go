package storage

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"infinibox-csi-driver/api"

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
	log.Infof(" requested size in bytes is %d ", sizeBytes)
	params := req.GetParameters()
	log.Infof(" csi request parameters %v", params)

	volType := iscsi.cs.getVolType(params)

	name := req.GetName()
	log.Infof("csi voume name from request is %s", name)
	if name == "" {
		return &csi.CreateVolumeResponse{}, errors.New("Name cannot be empty")
	}

	params = mergeStringMaps(params, req.GetSecrets())
	log.Infof("params after mearge are %v", params)
	// We require the storagePool name for creation
	sp, ok := params[StoragePoolKey]
	if !ok {
		return &csi.CreateVolumeResponse{}, errors.New("StoragePoolKey is a required parameter")
	}

	// Volume content source support Snapshots only
	contentSource := req.GetVolumeContentSource()
	var snapshotSource *csi.VolumeContentSource_SnapshotSource
	if contentSource != nil {
		log.Info("request is to create volume from snapshot")
		volumeSource := contentSource.GetVolume()
		if volumeSource != nil {
			return &csi.CreateVolumeResponse{}, status.Error(codes.InvalidArgument, "Volume as a VolumeContentSource is not supported (i.e. clone)")
		}
		snapshotSource = contentSource.GetSnapshot()
		if snapshotSource != nil {
			return iscsi.createVolumeFromSnapshot(req, snapshotSource, name, sizeBytes, sp)
		}
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
	if volumeResp == nil {
		// if volume already exists, look it up by name
		log.Info("checking if volume is already present")
		volumeResp, err = iscsi.cs.api.GetVolumeByName(name)
		if err != nil {
			return &csi.CreateVolumeResponse{}, status.Errorf(codes.Internal, err.Error())
		}
	}
	vi := iscsi.cs.getCSIVolume(volumeResp)

	// since the volume could have already exists, double check that the
	// volume has the expected parameters
	spID, err := iscsi.cs.api.GetStoragePoolIDByName(sp)

	if err != nil {
		return &csi.CreateVolumeResponse{}, status.Errorf(codes.Unavailable,
			"volume exists, but could not verify parameters: %s",
			err.Error())
	}
	if volumeResp.PoolId != spID {
		return &csi.CreateVolumeResponse{}, status.Errorf(codes.AlreadyExists,
			"volume exists, but in different storage pool than requested")
	}

	if vi.CapacityBytes != sizeBytes {
		return &csi.CreateVolumeResponse{}, status.Errorf(codes.AlreadyExists,
			"volume exists, but at different size than requested")
	}

	copyRequestParameters(req.GetParameters(), vi.VolumeContext)
	luninfo, err := iscsi.cs.mapVolumeTohost(volumeResp.ID)
	if err != nil {
		return &csi.CreateVolumeResponse{}, status.Error(codes.Internal, err.Error())
	}
	vi.VolumeContext["lun"] = strconv.Itoa(luninfo.Lun)
	log.Errorf("volume %s (%s) created %s\n", vi.VolumeContext["Name"], vi.VolumeId, vi.VolumeContext["CreationTime"])

	csiResp := &csi.CreateVolumeResponse{
		Volume: vi,
	}
	volID, err := strconv.Atoi(vi.VolumeId)
	if err != nil {
		return &csi.CreateVolumeResponse{}, errors.New("error getting volume id")
	}

	// confirm volume creation
	vol, err := iscsi.cs.getVolumeByID(volID)
	counter := 0
	if vol == nil {
		for err != nil && counter < 100 {
			time.Sleep(3 * time.Millisecond)
			vol, err = iscsi.cs.getVolumeByID(volID)
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

func (iscsi *iscsistorage) createVolumeFromSnapshot(req *csi.CreateVolumeRequest,
	snapshotSource *csi.VolumeContentSource_SnapshotSource,
	name string, sizeInKbytes int64, storagePool string) (*csi.CreateVolumeResponse, error) {

	// Lookup the snapshot source volume.
	snapshotID, err := strconv.Atoi(snapshotSource.SnapshotId)
	if err != nil {
		return nil, errors.New("error getting volume id")
	}
	srcVol, err := iscsi.cs.getVolumeByID(snapshotID)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "Snapshot not found: %s", snapshotSource.SnapshotId)
	}

	// Validate the size is the same.
	if int64(srcVol.Size) != sizeInKbytes {
		return nil, status.Errorf(codes.InvalidArgument,
			"Snapshot %s has incompatible size %d kbytes with requested %d kbytes",
			snapshotSource.SnapshotId, srcVol.Size, sizeInKbytes)
	}

	// Validate the storagePool is the same.
	snapStoragePool := iscsi.cs.getStoragePoolNameFromID(srcVol.PoolId)
	if snapStoragePool != storagePool {
		return nil, status.Errorf(codes.InvalidArgument,
			"Snapshot storage pool %s is different than the requested storage pool %s", snapStoragePool, storagePool)
	}

	vol, err := iscsi.cs.api.GetVolumeByName(name)
	if vol.PoolId == srcVol.PoolId {
		log.Errorf("Requested volume %s already exists", name)
		csiVolume := iscsi.cs.getCSIVolume(vol)
		log.Errorf("Requested volume (from snap) already exists %s (%s) storage pool %s",
			csiVolume.VolumeContext["Name"], csiVolume.VolumeId, csiVolume.VolumeContext["StoragePoolName"])
		return &csi.CreateVolumeResponse{Volume: csiVolume}, nil
	}

	// Volume the source volume
	snapshotParam := &api.SnapshotDef{ParentID: snapshotID, SnapshotName: name}
	//snapshotDefs = append(snapshotDefs, snapDef)
	//snapParam := &api.SnapshotVolumesParam{SnapshotDefs: snapshotDefs}

	// Create snapshot
	snapResponse, err := iscsi.cs.api.CreateSnapshotVolume(snapshotParam)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to create snapshot: %s", err.Error())
	}

	// Retrieve created destination volumevolume
	volID := snapResponse.SnapShotID
	dstVol, err := iscsi.cs.getVolumeByID(volID)
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

func (iscsi *iscsistorage) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	return nil, nil
}
