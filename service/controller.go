package service

import (
	"context"
	"infinibox-csi-driver/storage"
	"strings"

	log "github.com/sirupsen/logrus"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *service) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	configparams := make(map[string]interface{})
	configparams["nodeid"] = s.nodeID
	configparams["nodeIPAddress"] = s.nodeIPAddress
	nameSpace := req.GetParameters()["namespace"]
	configparams["namespace"] = nameSpace
	secretName := req.GetParameters()["secretname"]
	configparams["secretname"] = secretName
	storageprotocol := req.GetParameters()["storage_protocol"]

	log.Infof("Main CreateVolume nodeid, nodeIPAddress, storageprotocols", s.nodeID, s.nodeIPAddress, storageprotocol)
	if storageprotocol == "" {
		return nil, status.Error(codes.Internal, "storage protocol is not found, 'storage_protocol' is required field")
	}
	storageController, err := storage.NewStorageController(storageprotocol, configparams)
	if storageController != nil {
		csiResp, err := storageController.CreateVolume(ctx, req)
		log.Infof("CreateVolume return err %v", err)
		if csiResp != nil && csiResp.Volume.VolumeId != "" {
			if storageprotocol != "" {
				csiResp.Volume.VolumeId = csiResp.Volume.VolumeId + "$$" + storageprotocol + "$$" + nameSpace + "$$" + secretName
				log.Infof("CreateVolume updated volumeId %s", csiResp.Volume.VolumeId)
			}
		}
		return csiResp, err
	}
	log.Error("CreateVolume Error Occured: ", err)
	return &csi.CreateVolumeResponse{}, status.Error(codes.Internal, err.Error())
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
	volproto := strings.Split(voltype, "$$")
	log.Infof("DeleteVolume volproto", volproto)
	if len(volproto) != 4 {
		return nil, status.Error(codes.Internal, "volume Id and other details not found")
	}
	config := make(map[string]interface{})
	config["nodeid"] = s.nodeID
	config["namespace"] = volproto[2]
	config["secretname"] = volproto[3]
	storageController, err := storage.NewStorageController(volproto[1], config)
	if storageController != nil {
		req.VolumeId = volproto[0]
		deleteResponce, err := storageController.DeleteVolume(ctx, req)
		if err != nil {
			log.Error("Error Occured: ", err)
		}
		req.VolumeId = voltype
		return deleteResponce, err
	}

	return nil, status.Error(codes.Internal, err.Error())
}

func (s *service) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	log.Infof("------------IN s.req.GetNodeId() %v ", req.GetNodeId())

	voltype := req.GetVolumeId()
	log.Infof("ControllerPublishVolume called with volume name", voltype)
	volproto := strings.Split(voltype, "$$")
	log.Infof("ControllerPublishVolume volproto", volproto)
	if len(volproto) != 4 {
		return nil, status.Error(codes.Internal, "volume Id and other details not found")
	}
	config := make(map[string]interface{})
	config["nodeid"] = s.nodeID
	config["namespace"] = volproto[2]
	config["secretname"] = volproto[3]
	config["nodeIPAddress"] = req.GetNodeId()

	log.Infof("------------IN config ctx %v", config)
	storageController, err := storage.NewStorageController(volproto[1], config)
	if err != nil {
		log.Infof("-----------err ctx %v", err)
	}
	if storageController != nil {
		return storageController.ControllerPublishVolume(ctx, req)
	}

	return &csi.ControllerPublishVolumeResponse{}, nil
}

func (s *service) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	log.Infof("------------IN ControllerUnpublishVolume req.GetNodeId() %v", req.GetNodeId())
	log.Infof("------------IN ControllerUnpublishVolume ctx %v", ctx)
	voltype := req.GetVolumeId()
	log.Infof("ControllerUnpublishVolume called with volume name", voltype)
	volproto := strings.Split(voltype, "$$")
	log.Infof("ControllerUnpublishVolume volproto", volproto)
	if len(volproto) != 4 {
		return nil, status.Error(codes.Internal, "volume Id and other details not found")
	}
	config := make(map[string]interface{})
	config["nodeid"] = s.nodeID
	config["namespace"] = volproto[2]
	config["secretname"] = volproto[3]
	config["nodeIPAddress"] = req.GetNodeId()

	log.Infof("------------IN config ctx %v", config)
	storageController, err := storage.NewStorageController(volproto[1], config)
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
	return nil, status.Error(codes.Unimplemented, "")
}

func (s *service) ListSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}
func (s *service) GetCapacity(ctx context.Context, req *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
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
		},
	}, nil
}
func (s *service) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	storageController, _ := storage.NewStorageController(req.String(), nil)
	if storageController != nil {
		return storageController.CreateSnapshot(ctx, req)
	}
	return nil, nil
}
func (s *service) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	storageController, _ := storage.NewStorageController(req.String(), nil)
	if storageController != nil {
		return storageController.DeleteSnapshot(ctx, req)
	}
	return nil, nil
}

func (s *service) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	storageController, _ := storage.NewStorageController(req.String(), nil)
	if storageController != nil {
		return storageController.ControllerExpandVolume(ctx, req)
	}
	return nil, nil
}
