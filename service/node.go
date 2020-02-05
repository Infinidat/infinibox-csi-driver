package service

import (
	"context"
	"infinibox-csi-driver/storage"
	"strings"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/kubernetes/pkg/util/mount"
)

func (s *service) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	log.Println("Node method Main NodePublishVolume----------------------------------->")
	log.Debug("Node method Main NodePublishVolume")
	voltype := req.GetVolumeId()
	log.Infof("NodePublishVolume called with volume name", voltype)
	nameSpace := req.GetVolumeContext()["namespace"]
	secretName := req.GetVolumeContext()["secretname"]
	storagePorotcol := req.GetVolumeContext()["storage_protocol"]
	config := make(map[string]interface{})
	config["nodeIPAddress"] = s.nodeIPAddress
	config["namespace"] = nameSpace
	config["secretname"] = secretName
	log.Debug("NodePublishVolume nodeIPAddress ", s.nodeIPAddress)

	// get operator
	storageNode, err := storage.NewStorageNode(storagePorotcol, config)
	if storageNode == nil || err != nil {
		log.Error("Error Occured: ", err)
		return &csi.NodePublishVolumeResponse{}, status.Error(codes.Internal, err.Error())
	}
	volproto := strings.Split(voltype, "$$")
	req.VolumeId = volproto[0]
	resp, err := storageNode.NodePublishVolume(ctx, req)
	req.VolumeId = voltype
	if err != nil {
		log.Error("Error Occured: ", err)
	}
	return resp, err
}

func (s *service) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	log.Println("Node method Main NodeUnPublishVolume----------------------------------->")
	voltype := req.GetVolumeId()
	log.Infof("NodeUnpublishVolume called with volume name", voltype)
	volproto := strings.Split(voltype, "$$")
	log.Infof("NodeUnpublishVolume volproto", volproto)
	if len(volproto) != 4 {
		return nil, status.Error(codes.Internal, "volume Id and storage protocol not found")
	}
	config := make(map[string]interface{})
	config["namespace"] = volproto[2]
	config["secretname"] = volproto[3]
	protocolOperation, err := storage.NewStorageNode(volproto[1], config)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	req.VolumeId = volproto[0]
	resp, err := protocolOperation.NodeUnpublishVolume(ctx, req)
	req.VolumeId = voltype
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
						Type: csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
					},
				},
			},
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
	log.Infof("------------IN s.NodeStageVolume req %v ", req)
	mounter := mount.New("")
	targetPath := req.GetStagingTargetPath()
	log.Infof("------------IN s.targetPath ctx %v ", targetPath)
	notMnt, err := mounter.IsLikelyNotMountPoint(targetPath)
	log.Infof("------------IN s.notMnt,  ctx %v ", notMnt)
	log.Infof("------------IN s.notMnt, err  %v ", err)
	log.Infof("------------------------------------------------------------------------------------------------")
	return &csi.NodeStageVolumeResponse{}, nil
}

func (s *service) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	log.Infof("------------IN s.NodeUnstageVolume req %v ", req)
	mounter := mount.New("")
	targetPath := req.GetStagingTargetPath()
	log.Infof("------------IN NodeUnstageVolume s.targetPath ctx %v ", targetPath)
	notMnt, err := mounter.IsLikelyNotMountPoint(targetPath)
	log.Infof("------------IN NodeUnstageVolume s.notMnt,  ctx %v ", notMnt)
	log.Infof("------------IN NodeUnstageVolume s.notMnt, err  %v ", err)

	log.Infof("------------------------------------------------------------------------------------------------")

	return &csi.NodeUnstageVolumeResponse{}, nil
}
func (s *service) NodeGetVolumeStats(
	ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	return nil, status.Error(codes.Unimplemented, time.Now().String())

}

func (s *service) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
        volID := req.GetVolumeId()
        if len(volID) == 0 {
                return nil, status.Error(codes.InvalidArgument, "Volume ID not provided")
        }

        return &csi.NodeExpandVolumeResponse{}, nil
}
