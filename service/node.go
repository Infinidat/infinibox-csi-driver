package service

import (
	"context"
	"infinibox-csi-driver/storage"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *service) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	voltype := req.GetVolumeId()
	log.Infof("NodePublishVolume called with volume name", voltype)
	storagePorotcol := req.GetVolumeContext()["storage_protocol"]
	config := make(map[string]string)
	config["nodeIPAddress"] = s.nodeIPAddress
	log.Debug("NodePublishVolume nodeIPAddress ", s.nodeIPAddress)

	// get operator
	storageNode, err := storage.NewStorageNode(storagePorotcol, config, req.GetSecrets())
	if storageNode != nil {
		return storageNode.NodePublishVolume(ctx, req)
	}
	log.Error("Error Occured: ", err)
	return &csi.NodePublishVolumeResponse{}, status.Error(codes.Internal, err.Error())
}

func (s *service) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	log.Infof("NodeUnpublishVolume called with volume name", req.GetVolumeId())
	volproto, err := s.validateStorageType(req.GetVolumeId())
	if err != nil {
		return &csi.NodeUnpublishVolumeResponse{}, status.Error(codes.Internal, err.Error())
	}
	protocolOperation, err := storage.NewStorageNode(volproto.StorageType, nil, nil)
	if err != nil {
		return &csi.NodeUnpublishVolumeResponse{}, status.Error(codes.Internal, err.Error())
	}
	resp, err := protocolOperation.NodeUnpublishVolume(ctx, req)
	return resp, err
}

func (s *service) NodeGetCapabilities(
	ctx context.Context,
	req *csi.NodeGetCapabilitiesRequest) (
	*csi.NodeGetCapabilitiesResponse, error) {
	return &csi.NodeGetCapabilitiesResponse{
		Capabilities: []*csi.NodeServiceCapability{
			{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_UNKNOWN,
					},
				},
			},
			{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_EXPAND_VOLUME,
					},
				},
			},
			{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
					},
				},
			},
		},
	}, nil
}

func (s *service) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	log.Infof("Setting NodeId %s", s.nodeID)
	return &csi.NodeGetInfoResponse{
		NodeId: s.nodeID,
	}, nil
}
func (s *service) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	return &csi.NodeStageVolumeResponse{}, nil
}

func (s *service) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	return &csi.NodeUnstageVolumeResponse{}, nil
}
func (s *service) NodeGetVolumeStats(
	ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	return &csi.NodeGetVolumeStatsResponse{}, status.Error(codes.Unimplemented, time.Now().String())

}

func (s *service) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	volID := req.GetVolumeId()
	log.Info("----------------------> NodeExpandVolume called ", volID)
	if len(volID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID not provided")
	}

	return &csi.NodeExpandVolumeResponse{}, nil
}
