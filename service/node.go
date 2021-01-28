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
package service

import (
	"context"
	"errors"
	"fmt"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"infinibox-csi-driver/storage"
	"k8s.io/klog"
	"time"
)

func (s *service) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("Recovered from NodePublishVolume " + fmt.Sprint(res))
		}
	}()
	volumeId := req.GetVolumeId()
	klog.V(2).Infof("NodePublishVolume called with volume ID '%s'", volumeId)
	storageProtocol := req.GetVolumeContext()["storage_protocol"]
	config := make(map[string]string)
	config["nodeIPAddress"] = s.nodeIPAddress
	klog.V(4).Infof("NodePublishVolume nodeIPAddress '%s'", s.nodeIPAddress)

	// get operator
	storageNode, err := storage.NewStorageNode(storageProtocol, config, req.GetSecrets())
	if storageNode != nil {
		return storageNode.NodePublishVolume(ctx, req)
	}
	klog.Errorf("Error occurred: ", err)
	return &csi.NodePublishVolumeResponse{}, status.Error(codes.Internal, err.Error())
}

func (s *service) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("Recovered from  NodeUnpublishVolume " + fmt.Sprint(res))
		}
	}()
	klog.V(2).Infof("NodeUnpublishVolume called with volume name %s", req.GetVolumeId())
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
	klog.V(2).Infof("Setting NodeId %s", s.nodeID)
	nodeFQDN := s.getNodeFQDN()
	return &csi.NodeGetInfoResponse{
		NodeId: nodeFQDN + "$$" + s.nodeID,
	}, nil
}

func (s service) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("Recovered from NodeStageVolume " + fmt.Sprint(res))
		}
	}()
	volumeId := req.GetVolumeId()
	klog.V(2).Infof("NodeStageVolume called with volume ID '%s'", volumeId)
	storageProtocol := req.GetVolumeContext()["storage_protocol"]
	config := make(map[string]string)
	config["nodeIPAddress"] = s.nodeIPAddress
	// get operator
	storageNode, err := storage.NewStorageNode(storageProtocol, config, req.GetSecrets())
	if storageNode != nil {
		return storageNode.NodeStageVolume(ctx, req)
	}
	klog.Errorf("Error Occurred: ", err)
	return &csi.NodeStageVolumeResponse{}, status.Error(codes.Internal, err.Error())
}

func (s *service) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("Recovered from NodeUnstageVolume " + fmt.Sprint(res))
		}
	}()
	klog.V(2).Infof("NodeUnstageVolume called with volume name %s", req.GetVolumeId())
	volproto, err := s.validateStorageType(req.GetVolumeId())
	if err != nil {
		return &csi.NodeUnstageVolumeResponse{}, status.Error(codes.Internal, err.Error())
	}
	protocolOperation, err := storage.NewStorageNode(volproto.StorageType, nil, nil)
	if err != nil {
		return &csi.NodeUnstageVolumeResponse{}, status.Error(codes.Internal, err.Error())
	}
	resp, err := protocolOperation.NodeUnstageVolume(ctx, req)
	return resp, err
}
func (s *service) NodeGetVolumeStats(
	ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	return &csi.NodeGetVolumeStatsResponse{}, status.Error(codes.Unimplemented, time.Now().String())

}

func (s *service) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("Recovered from NodePublishVolume " + fmt.Sprint(res))
		}
	}()
	volID := req.GetVolumeId()
	if len(volID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID not provided")
	}

	return &csi.NodeExpandVolumeResponse{}, nil
}
