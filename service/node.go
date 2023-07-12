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
	"fmt"
	"infinibox-csi-driver/common"
	"infinibox-csi-driver/helper"
	"infinibox-csi-driver/storage"
	"os/exec"
	"strings"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/mount-utils"
)

// NodeServer driver
type NodeServer struct {
	Driver  *Driver
	mounter mount.Interface
}

func (s *NodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	if req.GetVolumeId() == "" {
		err := fmt.Errorf("NodePublishVolume error volumeId parameter was empty")
		zlog.Err(err)
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if req.GetStagingTargetPath() == "" {
		err := fmt.Errorf("NodeUnstageVolume error stagingTargetPath parameter was empty")
		zlog.Err(err)
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if req.VolumeCapability == nil {
		err := fmt.Errorf("NodeUnstageVolume error volumeCapability parameter was nil")
		zlog.Err(err)
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	defer func() {
		isLocking := false
		_ = helper.ManageNodeVolumeMutex(isLocking, "NodePublishVolume", req.GetVolumeId())
	}()

	isLocking := true
	_ = helper.ManageNodeVolumeMutex(isLocking, "NodePublishVolume", req.GetVolumeId())

	volumeId := req.GetVolumeId()
	zlog.Info().Msgf("NodePublishVolume - volume ID '%s'", volumeId)

	storageProtocol := req.GetVolumeContext()[common.SC_STORAGE_PROTOCOL]
	config := make(map[string]string)

	// get operator
	storageNode, err := storage.NewStorageNode(storageProtocol, config, req.GetSecrets())
	if storageNode != nil {
		zlog.Info().Msgf("NodePublishVolume - NewStorageNode succeeded with volume ID %s", volumeId)
		req.VolumeContext["nodeID"] = s.Driver.nodeID
		return storageNode.NodePublishVolume(ctx, req)
	}
	zlog.Error().Msgf("NodePublishVolume - NewStorageNode error: %s", err)
	return nil, status.Error(codes.Internal, err.Error())
}

func (s *NodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	if req.GetTargetPath() == "" {
		err := fmt.Errorf("NodeUnpublishVolume error targetPath parameter was empty")
		zlog.Err(err)
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if req.GetVolumeId() == "" {
		err := fmt.Errorf("NodeUnpublishVolume error volumeId parameter was empty")
		zlog.Err(err)
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	defer func() {
		isLocking := false
		_ = helper.ManageNodeVolumeMutex(isLocking, "NodeUnpublishVolume", req.GetVolumeId())
	}()

	isLocking := true
	_ = helper.ManageNodeVolumeMutex(isLocking, "NodeUnpublishVolume", req.GetVolumeId())

	zlog.Info().Msgf("NodeUnpublishVolume called with volume ID %s", req.GetVolumeId())
	zlog.Info().Msgf("NodeUnpublishVolume called with req %+v", req)
	volproto, err := validateVolumeID(req.GetVolumeId())
	if err != nil {
		zlog.Info().Msgf("NodeUnpublishVolume failed with volume ID %s: %s", req.GetVolumeId(), err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	protocolOperation, err := storage.NewStorageNode(volproto.StorageType, nil, nil)
	if err != nil {
		zlog.Info().Msgf("NodeUnpublishVolume failed with volume ID %s: %s", req.GetVolumeId(), err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	resp, err := protocolOperation.NodeUnpublishVolume(ctx, req)
	if err != nil {
		zlog.Info().Msgf("NodeUnpublishVolume failed with volume ID %s: %s", req.GetVolumeId(), err)
		return nil, err
	}
	zlog.Info().Msgf("NodeUnpublishVolume succeeded with volume ID %s", req.GetVolumeId())
	return resp, err
}

func (s *NodeServer) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
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

func (s *NodeServer) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	nodeFQDN := getNodeFQDN()
	k8sNodeID := nodeFQDN + "$$" + s.Driver.nodeID
	zlog.Info().Msgf("NodeGetInfo NodeId: %s", k8sNodeID)
	return &csi.NodeGetInfoResponse{
		NodeId: k8sNodeID,
	}, nil
}

func (s NodeServer) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	volumeId := req.GetVolumeId()
	zlog.Info().Msgf("NodeStageVolume called with volume ID '%s'", volumeId)
	if volumeId == "" {
		err := fmt.Errorf("NodeStageVolume error volumeId parameter was empty")
		zlog.Err(err)
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if req.VolumeCapability == nil {
		err := fmt.Errorf("NodeStageVolume error volumeCapability parameter was nil")
		zlog.Err(err)
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if req.StagingTargetPath == "" {
		err := fmt.Errorf("NodeStageVolume error stagingTargetPath parameter was empty")
		zlog.Err(err)
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	defer func() {
		isLocking := false
		_ = helper.ManageNodeVolumeMutex(isLocking, "NodeStageVolume", req.GetVolumeId())
	}()

	isLocking := true
	_ = helper.ManageNodeVolumeMutex(isLocking, "NodeStageVolume", req.GetVolumeId())

	storageProtocol := req.GetVolumeContext()[common.SC_STORAGE_PROTOCOL]
	config := make(map[string]string)
	// get operator
	zlog.Info().Msgf("NodeStageVolume volumeContext %+v storageProtocol is %s", req.GetVolumeContext(), storageProtocol)
	storageNode, err := storage.NewStorageNode(storageProtocol, config, req.GetSecrets())
	if storageNode != nil {
		zlog.Info().Msgf("NodeStageVolume succeeded with volume ID '%s'", volumeId)
		return storageNode.NodeStageVolume(ctx, req)
	}
	zlog.Error().Msgf("NodeStageVolume failed with volume ID %s: %s", volumeId, err)
	return nil, status.Error(codes.Internal, err.Error())
}

func (s *NodeServer) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	volumeId := req.GetVolumeId()
	if volumeId == "" {
		err := fmt.Errorf("NodeUnstageVolume error volumeId parameter was empty")
		zlog.Err(err)
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if req.StagingTargetPath == "" {
		err := fmt.Errorf("NodeUnstageVolume error stagingTargetPath parameter was empty")
		zlog.Err(err)
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	defer func() {
		isLocking := false
		_ = helper.ManageNodeVolumeMutex(isLocking, "NodeUnstageVolume", req.GetVolumeId())
	}()

	isLocking := true
	_ = helper.ManageNodeVolumeMutex(isLocking, "NodeUnstageVolume", volumeId)

	zlog.Info().Msgf("NodeUnstageVolume called with volume name %s", volumeId)
	volproto, err := validateVolumeID(volumeId)
	if err != nil {
		zlog.Error().Msgf("NodeUnstageVolume failed with volume ID %s: %s", volumeId, err)
		return nil, status.Error(codes.Internal, err.Error())
	}
	protocolOperation, err := storage.NewStorageNode(volproto.StorageType, nil, nil)
	if err != nil {
		zlog.Error().Msgf("NodeUnstageVolume failed with volume ID %s: %s", volumeId, err)
		return nil, status.Error(codes.Internal, err.Error())
	}
	resp, err := protocolOperation.NodeUnstageVolume(ctx, req)
	if err != nil {
		zlog.Error().Msgf("NodeUnstageVolume failed with volume ID %s: %s", volumeId, err)
		return nil, status.Error(codes.Internal, err.Error())
	}
	zlog.Info().Msgf("NodeUnstageVolume succeeded with volume ID '%s'", volumeId)
	return resp, err
}

func (s *NodeServer) NodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	return nil, status.Error(codes.Unimplemented, time.Now().String())
}

func (s *NodeServer) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, time.Now().String())
}

func getNodeFQDN() string {
	cmd := "hostname -f"
	out, err := exec.Command("bash", "-c", cmd).Output()
	if err != nil {
		zlog.Warn().Msgf("could not get fqdn with cmd : 'hostname -f', get hostname with 'echo $HOSTNAME'")
		cmd = "echo $HOSTNAME"
		out, err = exec.Command("bash", "-c", cmd).Output()
		if err != nil {
			zlog.Error().Msgf("Failed to execute command: %s", cmd)
			return "unknown"
		}
	}
	nodeFQDN := string(out)
	if nodeFQDN == "" {
		zlog.Warn().Msgf("node fqnd not found, setting node name as node fqdn instead")
		nodeFQDN = "unknown"
	}
	nodeFQDN = strings.TrimSuffix(nodeFQDN, "\n")
	return nodeFQDN
}
