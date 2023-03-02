/*
Copyright 2022 Infinidat
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package service

import (
	"context"
	"errors"
	"fmt"
	"infinibox-csi-driver/common"
	"infinibox-csi-driver/helper"
	"infinibox-csi-driver/storage"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"
)

func (s *service) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("Recovered from NodePublishVolume " + fmt.Sprint(res))
		}

		isLocking := false
		_ = helper.ManageNodeVolumeMutex(isLocking, "NodePublishVolume", req.GetVolumeId())
	}()

	isLocking := true
	_ = helper.ManageNodeVolumeMutex(isLocking, "NodePublishVolume", req.GetVolumeId())

	volumeId := req.GetVolumeId()
	klog.V(2).Infof("NodePublishVolume - volume ID '%s'", volumeId)
	storageProtocol := req.GetVolumeContext()[common.SC_STORAGE_PROTOCOL]
	config := make(map[string]string)

	// get operator
	storageNode, err := storage.NewStorageNode(storageProtocol, config, req.GetSecrets())
	if storageNode != nil {
		klog.V(2).Infof("NodePublishVolume - NewStorageNode succeeded with volume ID %s", volumeId)
		req.VolumeContext["nodeID"] = s.nodeID
		return storageNode.NodePublishVolume(ctx, req)
	}
	klog.Errorf("NodePublishVolume - NewStorageNode error: %s", err)
	return nil, status.Error(codes.Internal, err.Error())
}

func (s *service) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = fmt.Errorf("recovered from NodeUnpublishVolume with volume ID %s: %s", req.GetVolumeId(), res)
		}

		isLocking := false
		_ = helper.ManageNodeVolumeMutex(isLocking, "NodeUnpublishVolume", req.GetVolumeId())
	}()

	isLocking := true
	_ = helper.ManageNodeVolumeMutex(isLocking, "NodeUnpublishVolume", req.GetVolumeId())

	klog.V(2).Infof("NodeUnpublishVolume called with volume ID %s", req.GetVolumeId())
	klog.V(5).Infof("NodeUnpublishVolume called with req %+v", req)
	volproto, err := s.validateVolumeID(req.GetVolumeId())
	if err != nil {
		klog.V(2).Infof("NodeUnpublishVolume failed with volume ID %s: %s", req.GetVolumeId(), err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	protocolOperation, err := storage.NewStorageNode(volproto.StorageType, nil, nil)
	if err != nil {
		klog.V(2).Infof("NodeUnpublishVolume failed with volume ID %s: %s", req.GetVolumeId(), err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	resp, err := protocolOperation.NodeUnpublishVolume(ctx, req)
	if err != nil {
		klog.V(2).Infof("NodeUnpublishVolume failed with volume ID %s: %s", req.GetVolumeId(), err)
		return nil, err
	}
	klog.V(2).Infof("NodeUnpublishVolume succeeded with volume ID %s", req.GetVolumeId())
	return resp, err
}

func (s *service) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	return &csi.NodeGetCapabilitiesResponse{
		Capabilities: []*csi.NodeServiceCapability{
			{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_UNKNOWN,
					},
				},
			},
			// {
			// 	Type: &csi.NodeServiceCapability_Rpc{
			// 		Rpc: &csi.NodeServiceCapability_RPC{
			// 			Type: csi.NodeServiceCapability_RPC_EXPAND_VOLUME,
			// 		},
			// 	},
			// },
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
	nodeFQDN := s.getNodeFQDN()
	k8sNodeID := nodeFQDN + "$$" + s.nodeID
	klog.V(2).Infof("NodeGetInfo NodeId: %s", k8sNodeID)
	return &csi.NodeGetInfoResponse{
		NodeId: k8sNodeID,
	}, nil
}

func (s service) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = fmt.Errorf("recovered from NodeStageVolume with ID %s: %s", req.GetVolumeId(), res)
		}

		isLocking := false
		_ = helper.ManageNodeVolumeMutex(isLocking, "NodeStageVolume", req.GetVolumeId())
	}()

	isLocking := true
	_ = helper.ManageNodeVolumeMutex(isLocking, "NodeStageVolume", req.GetVolumeId())

	volumeId := req.GetVolumeId()
	klog.V(2).Infof("NodeStageVolume called with volume ID '%s'", volumeId)

	storageProtocol := req.GetVolumeContext()[common.SC_STORAGE_PROTOCOL]
	config := make(map[string]string)
	// get operator
	klog.V(2).Infof("NodeStageVolume volumeContext %+v storageProtocol is %s", req.GetVolumeContext(), storageProtocol)
	storageNode, err := storage.NewStorageNode(storageProtocol, config, req.GetSecrets())
	if storageNode != nil {
		klog.V(2).Infof("NodeStageVolume succeeded with volume ID '%s'", volumeId)
		return storageNode.NodeStageVolume(ctx, req)
	}
	klog.Errorf("NodeStageVolume failed with volume ID %s: %s", volumeId, err)
	return nil, status.Error(codes.Internal, err.Error())
}

func (s *service) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = fmt.Errorf("recovered from NodeUnstageVolume with volume ID %s: %s", req.GetVolumeId(), res)
		}

		isLocking := false
		_ = helper.ManageNodeVolumeMutex(isLocking, "NodeUnstageVolume", req.GetVolumeId())
	}()

	volumeId := req.GetVolumeId()

	isLocking := true
	_ = helper.ManageNodeVolumeMutex(isLocking, "NodeUnstageVolume", volumeId)

	klog.V(2).Infof("NodeUnstageVolume called with volume name %s", volumeId)
	volproto, err := s.validateVolumeID(volumeId)
	if err != nil {
		klog.Errorf("NodeUnstageVolume failed with volume ID %s: %s", volumeId, err)
		return nil, status.Error(codes.Internal, err.Error())
	}
	protocolOperation, err := storage.NewStorageNode(volproto.StorageType, nil, nil)
	if err != nil {
		klog.Errorf("NodeUnstageVolume failed with volume ID %s: %s", volumeId, err)
		return nil, status.Error(codes.Internal, err.Error())
	}
	resp, err := protocolOperation.NodeUnstageVolume(ctx, req)
	if err != nil {
		klog.Errorf("NodeUnstageVolume failed with volume ID %s: %s", volumeId, err)
		return nil, status.Error(codes.Internal, err.Error())
	}
	klog.V(2).Infof("NodeUnstageVolume succeeded with volume ID '%s'", volumeId)
	return resp, err
}

func (s *service) NodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	return nil, status.Error(codes.Unimplemented, time.Now().String())
}

func (s *service) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, time.Now().String())
}
