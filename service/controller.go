package service

import (
	"context"
	"errors"
	"fmt"
	"infinibox-csi-driver/storage"

	log "github.com/sirupsen/logrus"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

//CreateVolume method create the volumne
func (s *service) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (csiResp *csi.CreateVolumeResponse, err error) {
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("Recoved from CSI CreateVolume  " + fmt.Sprint(res))
		}
	}()
	//TODO: validate the required parameter
	configparams := make(map[string]string)
	configparams["nodeid"] = s.nodeID
	configparams["nodeIPAddress"] = s.nodeIPAddress
	storageprotocol := req.GetParameters()["storage_protocol"]

	log.Infof("In CreateVolume method nodeid: %s, nodeIPAddress: %s, storageprotocols %s", s.nodeID, s.nodeIPAddress, storageprotocol)
	if storageprotocol == "" {
		return &csi.CreateVolumeResponse{}, status.Error(codes.Internal, "storage protocol is not found, 'storage_protocol' is required field")
	}
	storageController, err := storage.NewStorageController(storageprotocol, configparams, req.GetSecrets())
	if err != nil || storageController == nil {
		log.Errorf("In CreateVolume method : %v", err)
		err = errors.New("fail to initialise storage controller while create volume " + storageprotocol)
		return
	}
	csiResp, err = storageController.CreateVolume(ctx, req)
	log.Infof("CreateVolume return  %v", err)
	if err != nil {
		err = errors.New("fail to create volume of storage protocol " + storageprotocol)
		return
	}
	if csiResp != nil && csiResp.Volume != nil && csiResp.Volume.VolumeId != "" {
		csiResp.Volume.VolumeId = csiResp.Volume.VolumeId + "$$" + storageprotocol
		log.Infof("CreateVolume updated volumeId %s", csiResp.Volume.VolumeId)
		return
	}
	err = errors.New("CreateVolume error: failed to create volume")
	return
}

func (s *service) createVolumeFromSnapshot(req *csi.CreateVolumeRequest,
	snapshotSource *csi.VolumeContentSource_SnapshotSource,
	name string, sizeInKbytes int64, storagePool string) (*csi.CreateVolumeResponse, error) {
	return &csi.CreateVolumeResponse{}, nil
}

//DeleteVolume method delete the volumne
func (s *service) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (deleteResponce *csi.DeleteVolumeResponse, err error) {

	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("Recoved from CSI DeleteVolume  " + fmt.Sprint(res))
		}
	}()

	voltype := req.GetVolumeId()
	log.Infof("DeleteVolume method called with volume name", voltype)
	volproto, err := s.validateStorageType(req.GetVolumeId())
	if err != nil {
		log.Errorf("fail to validate storage type %v", err)
		return
	}
	config := make(map[string]string)
	config["nodeid"] = s.nodeID
	storageController, err := storage.NewStorageController(volproto.storageType, config, req.GetSecrets())
	if err != nil || storageController == nil {
		err = errors.New("fail to initialise storage controller while delete volume " + volproto.storageType)
		return
	}

	req.VolumeId = volproto.volumeID
	deleteResponce, err = storageController.DeleteVolume(ctx, req)
	if err != nil {
		log.Errorf("fail to delete volume %v", err)
		err = errors.New("fail to delete volume of type " + volproto.storageType)
		return
	}
	req.VolumeId = voltype
	return
}

//ControllerPublishVolume method
func (s *service) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (controlePublishResponce *csi.ControllerPublishVolumeResponse, err error) {

	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("Recoved from CSI ControllerPublishVolume  " + fmt.Sprint(res))
		}
	}()

	volproto, err := s.validateStorageType(req.GetVolumeId())
	if err != nil {
		log.Errorf("fail to validate StorageType Publish Volume %v", err)
		err = errors.New("fail to validate StorageType")
		return
	}
	config := make(map[string]string)
	config["nodeid"] = s.nodeID
	config["nodeIPAddress"] = req.GetNodeId()

	storageController, err := storage.NewStorageController(volproto.storageType, config, req.GetSecrets())
	if err != nil || storageController == nil {
		err = errors.New("fail to initialise storage controller while ControllerPublishVolume " + volproto.storageType)
		return
	}
	controlePublishResponce, err = storageController.ControllerPublishVolume(ctx, req)
	if err != nil {
		log.Errorf("ControllerPublishVolume %v", err)
	}
	return
}

//ControllerUnpublishVolume method
func (s *service) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (controleUnPublishResponce *csi.ControllerUnpublishVolumeResponse, err error) {

	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("Recoved from CSI ControllerUnpublishVolume  " + fmt.Sprint(res))
		}
	}()

	volproto, err := s.validateStorageType(req.GetVolumeId())
	if err != nil {
		log.Errorf("fail to validate StorageType while Unpublish Volume %v", err)
		err = errors.New("fail to validate StorageType while Unpublish Volume")
		return
	}
	config := make(map[string]string)
	config["nodeid"] = s.nodeID
	config["nodeIPAddress"] = req.GetNodeId()
	storageController, err := storage.NewStorageController(volproto.storageType, config, req.GetSecrets())
	if err != nil || storageController == nil {
		err = errors.New("fail to initialise storage controller while ControllerUnpublishVolume " + volproto.storageType)
		return
	}
	controleUnPublishResponce, err = storageController.ControllerUnpublishVolume(ctx, req)
	if err != nil {
		log.Errorf("ControllerUnpublishVolume %v", err)
	}
	return
}

func (s *service) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	return &csi.ValidateVolumeCapabilitiesResponse{}, nil
}

func (s *service) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	return &csi.ListVolumesResponse{}, status.Error(codes.Unimplemented, "")
}

func (s *service) ListSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	return &csi.ListSnapshotsResponse{}, status.Error(codes.Unimplemented, "")
}
func (s *service) GetCapacity(ctx context.Context, req *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error) {
	return &csi.GetCapacityResponse{}, status.Error(codes.Unimplemented, "")
}

func (s *service) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	return &csi.ControllerGetCapabilitiesResponse{
		Capabilities: []*csi.ControllerServiceCapability{
			&csi.ControllerServiceCapability{
				Type: &csi.ControllerServiceCapability_Rpc{
					Rpc: &csi.ControllerServiceCapability_RPC{
						Type: csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
					},
				},
			},
			&csi.ControllerServiceCapability{
				Type: &csi.ControllerServiceCapability_Rpc{
					Rpc: &csi.ControllerServiceCapability_RPC{
						Type: csi.ControllerServiceCapability_RPC_LIST_VOLUMES,
					},
				},
			},
			&csi.ControllerServiceCapability{
				Type: &csi.ControllerServiceCapability_Rpc{
					Rpc: &csi.ControllerServiceCapability_RPC{
						Type: csi.ControllerServiceCapability_RPC_GET_CAPACITY,
					},
				},
			},
			&csi.ControllerServiceCapability{
				Type: &csi.ControllerServiceCapability_Rpc{
					Rpc: &csi.ControllerServiceCapability_RPC{
						Type: csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT,
					},
				},
			},
			&csi.ControllerServiceCapability{
				Type: &csi.ControllerServiceCapability_Rpc{
					Rpc: &csi.ControllerServiceCapability_RPC{
						Type: csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME,
					},
				},
			},
			&csi.ControllerServiceCapability{
				Type: &csi.ControllerServiceCapability_Rpc{
					Rpc: &csi.ControllerServiceCapability_RPC{
						Type: csi.ControllerServiceCapability_RPC_CLONE_VOLUME,
					},
				},
			},
			&csi.ControllerServiceCapability{
				Type: &csi.ControllerServiceCapability_Rpc{
					Rpc: &csi.ControllerServiceCapability_RPC{
						Type: csi.ControllerServiceCapability_RPC_LIST_SNAPSHOTS,
					},
				},
			},
			&csi.ControllerServiceCapability{
				Type: &csi.ControllerServiceCapability_Rpc{
					Rpc: &csi.ControllerServiceCapability_RPC{
						Type: csi.ControllerServiceCapability_RPC_EXPAND_VOLUME,
					},
				},
			},
		},
	}, nil
}

func (s *service) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	storageController, _ := storage.NewStorageController(req.String(), nil)
	if storageController != nil {
		return storageController.CreateSnapshot(ctx, req)
	}
	return &csi.CreateSnapshotResponse{}, nil
}
func (s *service) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	storageController, _ := storage.NewStorageController(req.String(), nil)
	if storageController != nil {
		return storageController.DeleteSnapshot(ctx, req)
	}
	return &csi.DeleteSnapshotResponse{}, nil
}

func (s *service) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	configparams := make(map[string]string)
	configparams["nodeid"] = s.nodeID
	configparams["nodeIPAddress"] = s.nodeIPAddress
	log.Infof("Main ExpandVolume nodeid, nodeIPAddress %s %s", s.nodeID, s.nodeIPAddress)

	volproto, err := s.validateStorageType(req.GetVolumeId())
	if err != nil {
		return &csi.ControllerExpandVolumeResponse{}, status.Error(codes.Internal, err.Error())
	}
	storageController, err := storage.NewStorageController(volproto.storageType, configparams)
	if storageController != nil {
		csiResp, err := storageController.ControllerExpandVolume(ctx, req)
		return csiResp, err
	}
	log.Error("UpdateVolume Error Occured: ", err)
	return &csi.ControllerExpandVolumeResponse{}, status.Error(codes.Internal, err.Error())
}
