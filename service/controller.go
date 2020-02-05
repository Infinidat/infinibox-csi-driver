package service

import (
	"context"
	"infinibox-csi-driver/storage"

	log "github.com/sirupsen/logrus"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *service) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	configparams := make(map[string]string)
	configparams["nodeid"] = s.nodeID
	configparams["nodeIPAddress"] = s.nodeIPAddress
	storageprotocol := req.GetParameters()["storage_protocol"]
	log.Debugf("secretes secretMap %v: ", req.GetSecrets())
	log.Infof("Main CreateVolume nodeid, nodeIPAddress, storageprotocols", s.nodeID, s.nodeIPAddress, storageprotocol)
	if storageprotocol == "" {
		return &csi.CreateVolumeResponse{}, status.Error(codes.Internal, "storage protocol is not found, 'storage_protocol' is required field")
	}
	storageController, err := storage.NewStorageController(storageprotocol, configparams, req.GetSecrets())
	if err != nil {
		log.Error("CreateVolume Error Occured: ", err)
		return &csi.CreateVolumeResponse{}, status.Error(codes.Internal, err.Error())
	}
	if storageController != nil {
		csiResp, err := storageController.CreateVolume(ctx, req)
		log.Infof("CreateVolume return err %v", err)
		if err != nil {
			return &csi.CreateVolumeResponse{}, status.Error(codes.Internal, err.Error())
		}
		if csiResp != nil && csiResp.Volume.VolumeId != "" {
			csiResp.Volume.VolumeId = csiResp.Volume.VolumeId + "$$" + storageprotocol
			log.Infof("CreateVolume updated volumeId %s", csiResp.Volume.VolumeId)
			return csiResp, nil
		}
	}
	log.Error("CreateVolume error: failed to create volume")
	return &csi.CreateVolumeResponse{}, status.Error(codes.Internal, "Create volume failed")
}

func (s *service) createVolumeFromSnapshot(req *csi.CreateVolumeRequest,
	snapshotSource *csi.VolumeContentSource_SnapshotSource,
	name string, sizeInKbytes int64, storagePool string) (*csi.CreateVolumeResponse, error) {
	return &csi.CreateVolumeResponse{}, nil
}

func (s *service) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	log.Info("IN DeleteVolume req")
	voltype := req.GetVolumeId()
	log.Infof("DeleteVolume called with volume name", voltype)
	volproto, err := s.validateStorageType(req.GetVolumeId())
	if err != nil {
		return &csi.DeleteVolumeResponse{}, status.Error(codes.Internal, err.Error())
	}
	config := make(map[string]string)
	config["nodeid"] = s.nodeID
	storageController, err := storage.NewStorageController(volproto.storageType, config, req.GetSecrets())
	if storageController != nil {
		req.VolumeId = volproto.volumeID
		deleteResponce, err := storageController.DeleteVolume(ctx, req)
		if err != nil {
			log.Error("Error Occured: ", err)
			return &csi.DeleteVolumeResponse{}, status.Error(codes.Internal, err.Error())
		}
		req.VolumeId = voltype
		return deleteResponce, nil
	}

	return &csi.DeleteVolumeResponse{}, status.Error(codes.Internal, err.Error())
}

func (s *service) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	log.Infof("IN ControllerPublishVolume called with volume name", req.GetVolumeId())
	volproto, err := s.validateStorageType(req.GetVolumeId())
	if err != nil {
		return &csi.ControllerPublishVolumeResponse{}, status.Error(codes.Internal, err.Error())
	}
	config := make(map[string]string)
	config["nodeid"] = s.nodeID
	config["nodeIPAddress"] = req.GetNodeId()

	storageController, err := storage.NewStorageController(volproto.storageType, config, req.GetSecrets())
	if err != nil {
		return &csi.ControllerPublishVolumeResponse{}, status.Error(codes.Internal, err.Error())
	}
	if storageController != nil {
		return storageController.ControllerPublishVolume(ctx, req)
	}

	return &csi.ControllerPublishVolumeResponse{}, nil
}

func (s *service) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	log.Infof("ControllerUnpublishVolume called with volume name", req.GetVolumeId())
	volproto, err := s.validateStorageType(req.GetVolumeId())
	if err != nil {
		return &csi.ControllerUnpublishVolumeResponse{}, status.Error(codes.Internal, err.Error())
	}

	config := make(map[string]string)
	config["nodeid"] = s.nodeID
	config["nodeIPAddress"] = req.GetNodeId()

	storageController, err := storage.NewStorageController(volproto.storageType, config, req.GetSecrets())
	if err != nil {
		log.Error("Error Occured: ", err)
		return &csi.ControllerUnpublishVolumeResponse{}, status.Error(codes.Internal, err.Error())
	}
	if storageController != nil {
		return storageController.ControllerUnpublishVolume(ctx, req)
	}
	return &csi.ControllerUnpublishVolumeResponse{}, nil
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
