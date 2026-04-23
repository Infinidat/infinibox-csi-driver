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
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/infinidat/infinibox-csi-driver/helper"
	"github.com/infinidat/infinibox-csi-driver/storage"
	storagecommon "github.com/infinidat/infinibox-csi-driver/storage/common"
	"github.com/infinidat/infinibox-csi-driver/storage/nfs"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/kubernetes/pkg/volume"
	"k8s.io/mount-utils"
)

// NodeServer driver
type NodeServer struct {
	Driver  *Driver
	mounter mount.Interface
	csi.UnimplementedNodeServer
}

const UNKNOWN = "unknown"

func (s *NodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	slog.Info("Started", "volume ID", req.GetVolumeId())

	if req.GetVolumeId() == "" {
		e := common.Errorf("error volumeId parameter was empty")
		slog.Error(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	if req.GetStagingTargetPath() == "" {
		e := common.Errorf("error stagingTargetPath parameter was empty")
		slog.Error(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	if req.VolumeCapability == nil {
		e := common.Errorf("error volumeCapability parameter was nil")
		slog.Error(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	caps := []*csi.VolumeCapability{
		req.VolumeCapability,
	}

	_, err := validateCapabilities(caps)
	if err != nil {
		e := common.Errorf("validateCapabilities - error %w, volume cap %v", err, req.VolumeCapability)
		slog.Error(e.Error())
		return nil, status.Error(codes.FailedPrecondition, e.Error())
	}
	defer func() {
		isLocking := false
		_ = helper.ManageNodeVolumeMutex(isLocking, "NodePublishVolume", req.GetVolumeId())
	}()

	isLocking := true
	_ = helper.ManageNodeVolumeMutex(isLocking, "NodePublishVolume", req.GetVolumeId())

	storageProtocol := req.GetVolumeContext()[common.StorageClassStorageProtocol]

	fsGroup := req.VolumeCapability.GetMount().GetVolumeMountGroup()

	slog.Debug("VolumeMountGroup", "fsGroup", fsGroup)

	config := make(map[string]string)

	volumeInfo, err := storagecommon.ValidateVolumeID(req.GetVolumeId())
	if err != nil {
		e := common.Errorf("ValidateVolumeID volume ID: %s error: %w", req.GetVolumeId(), err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	nodeProtocol, err := getNodeProtocol(ctx, os.Getenv(common.EnvVarKubeNodeName))
	if err != nil {
		e := common.Errorf("getNodeProtocol volume ID %s - error: %w", req.GetVolumeId(), err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	if nodeProtocol != "" {
		slog.Debug("nodeProtocol found on node, using instead of", "nodeProtocol", nodeProtocol, "node name", os.Getenv(common.EnvVarKubeNodeName), "storage type", volumeInfo.StorageType)
		volumeInfo.StorageType = nodeProtocol
		protocolSecret, protocolSecretInUse, err := GetProtocolSecret(ctx)
		if err != nil {
			e := common.Errorf("error: could not get protocol secret %w", err)
			return nil, status.Error(codes.InvalidArgument, e.Error())
		}
		if !protocolSecretInUse {
			e := common.Errorf("error: protocol secret not in use, but is required when nodeProtocol label is set on node")
			return nil, status.Error(codes.InvalidArgument, e.Error())
		}
		switch nodeProtocol {
		case common.ProtocolISCSI:
			networkSpace := protocolSecret[ProtocolSecretISCSINetworkSpace]
			if networkSpace == "" {
				e := common.Errorf("error: protocol secret in use, but ISCSI network space is empty")
				return nil, status.Error(codes.InvalidArgument, e.Error())
			}
			req.VolumeContext[common.StorageClassNetworkSpace] = networkSpace
		case common.ProtocolNVME:
			networkSpace := protocolSecret[ProtocolSecretNVMENetworkSpace]
			if networkSpace == "" {
				e := common.Errorf("error: protocol secret in use, but NVMEe network space is empty")
				return nil, status.Error(codes.InvalidArgument, e.Error())
			}
			req.VolumeContext[common.StorageClassNetworkSpace] = networkSpace
		}
	}

	// the storageclass is required to specify node-publish secrets as a parameter,this will cause
	// the secret values (hostname, password, username) to be passed down to the NodePublishVolume function
	err = validateSecret(req.GetVolumeId(), common.CSINodePublishSecretName, common.CSINodePublishSecretNamespace, req.GetSecrets())
	if err != nil {
		e := common.Errorf("validateSecret %w", err)
		slog.Error(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	storageNode, commonService, err := storage.NewStorageNodeAndCommonService(0, config, req.GetSecrets(), &volumeInfo)
	if err != nil {
		e := common.Errorf("NewStorageNode - volume ID: %s error: %w", req.GetVolumeId(), err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	slog.Info("Finished", "volume id", req.GetVolumeId())
	req.VolumeContext["nodeID"] = s.Driver.nodeID
	response, err := storageNode.NodePublishVolume(ctx, req)
	if err != nil {
		e := common.Errorf("sn.NodePublishVolume - volume ID: %s error: %w", req.GetVolumeId(), err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	if storageProtocol == common.ProtocolNFS {
		mountOptions := req.GetVolumeCapability().GetMount().GetMountFlags()
		nfsVersion, nfsPort := nfs.GetNFSVersionPort(mountOptions)
		slog.Debug("nfs mount options", "mountoptions", mountOptions, "nfsversion", nfsVersion, "port", nfsPort)
		helper.EventNFSVersions[nfsVersion]++
	}

	helper.EventAPIClient = commonService.API
	helper.EventIboxAPIClient = commonService.IboxAPI
	helper.EventPublishedVolumes[volumeInfo.StorageType]++

	return response, nil
}

func (s *NodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	slog.Info("Started", "volume id", req.GetVolumeId())

	if req.GetTargetPath() == "" {
		e := common.Errorf("error targetPath parameter was empty")
		slog.Error(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}
	if req.GetVolumeId() == "" {
		e := common.Errorf("error volumeId parameter was empty")
		slog.Error(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	defer func() {
		isLocking := false
		_ = helper.ManageNodeVolumeMutex(isLocking, "NodeUnpublishVolume", req.GetVolumeId())
	}()

	isLocking := true
	_ = helper.ManageNodeVolumeMutex(isLocking, "NodeUnpublishVolume", req.GetVolumeId())

	slog.Debug("called", "volume id", req.GetVolumeId())
	slog.Log(ctx, common.LevelTrace, "called", "req", req)
	volumeInfo, err := storagecommon.ValidateVolumeID(req.GetVolumeId())
	if err != nil {
		e := common.Errorf("ValidateVolumeID volume ID %s - error: %w", req.GetVolumeId(), err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	nodeProtocol, err := getNodeProtocol(ctx, os.Getenv(common.EnvVarKubeNodeName))
	if err != nil {
		e := common.Errorf("getNodeProtocol volume ID %s - error: %w", req.GetVolumeId(), err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	if nodeProtocol != "" {
		slog.Debug("nodeProtocol found on node, using instead of", "nnodeProtocol", nodeProtocol, "node name", os.Getenv(common.EnvVarKubeNodeName), "storage type", volumeInfo.StorageType)
		volumeInfo.StorageType = nodeProtocol
	}

	protocolOperation, err := storage.NewStorageNode(storagecommon.Commonservice{VolProto: &volumeInfo}, 0)
	if err != nil {
		e := common.Errorf("NewStorageNode volume ID %s - error: %w", req.GetVolumeId(), err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	resp, err := protocolOperation.NodeUnpublishVolume(ctx, req)
	if err != nil {
		e := common.Errorf("NodeUnpublishVolume error %w", err)
		slog.Error("NodeUnpublishVolume", "volume ID", req.GetVolumeId(), "error", e.Error())
		return nil, e
	}

	slog.Info("Finished", "volume id", req.GetVolumeId())
	return resp, nil
}

func (s *NodeServer) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	// set as trace because it happens frequently
	slog.Log(ctx, common.LevelTrace, "NodeGetCapabilities Requested", "node id", s.Driver.nodeID, "nscap", s.Driver.nscap)

	return &csi.NodeGetCapabilitiesResponse{
		Capabilities: s.Driver.nscap,
	}, nil
}

func (s *NodeServer) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	slog.Debug("NodeGetInfo Requested", "node id", s.Driver.nodeID)

	nodeFQDN := getNodeFQDN()
	topo := &csi.Topology{
		Segments: map[string]string{
			"topology.csi.infinidat.com/zone": "true",
		},
	}
	k8sNodeID := nodeFQDN + "$$" + s.Driver.nodeID
	return &csi.NodeGetInfoResponse{
		NodeId:             k8sNodeID,
		AccessibleTopology: topo,
	}, nil
}

func (s NodeServer) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	volumeId := req.GetVolumeId()
	slog.Info("Started", "volume id", volumeId)

	if volumeId == "" {
		e := common.Errorf("error volumeId parameter was empty")
		slog.Error(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}
	if req.VolumeCapability == nil {
		e := common.Errorf("error volumeCapability parameter was nil")
		slog.Error(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	caps := []*csi.VolumeCapability{
		req.VolumeCapability,
	}

	_, err := validateCapabilities(caps)
	if err != nil {
		e := common.Errorf("validateCapabilities - error %w", err)
		slog.Error(e.Error())
		return nil, status.Error(codes.FailedPrecondition, e.Error())
	}

	if req.StagingTargetPath == "" {
		e := common.Errorf("error stagingTargetPath parameter was empty")
		slog.Error(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	defer func() {
		isLocking := false
		_ = helper.ManageNodeVolumeMutex(isLocking, "NodeStageVolume", req.GetVolumeId())
	}()

	isLocking := true
	_ = helper.ManageNodeVolumeMutex(isLocking, "NodeStageVolume", req.GetVolumeId())

	// storageProtocol := req.GetVolumeContext()[common.SC_STORAGE_PROTOCOL]

	fsGroup := req.VolumeCapability.GetMount().GetVolumeMountGroup()

	slog.Debug("VolumeMountGroup", "fsgroup", fsGroup)
	config := make(map[string]string)

	volumeInfo, err := storagecommon.ValidateVolumeID(req.GetVolumeId())
	if err != nil {
		e := common.Errorf("ValidateVolumeID -  volume ID %s - error: %w", req.GetVolumeId(), err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	slog.Debug("info", "volumeContext", req.GetVolumeContext(), "storagetype", volumeInfo.StorageType)

	err = validateSecret(req.GetVolumeId(), common.CSINodeStageSecretName, common.CSINodeStageSecretNamespace, req.GetSecrets())
	if err != nil {
		e := common.Errorf("validateSecret error %w", err)
		slog.Error(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	nodeProtocol, err := getNodeProtocol(ctx, os.Getenv(common.EnvVarKubeNodeName))
	if err != nil {
		e := common.Errorf("getNodeProtocol volume ID %s - error: %w", req.GetVolumeId(), err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	if nodeProtocol != "" {
		slog.Debug("nodeProtocol found on node, instead of", "node protocol", nodeProtocol, "kube node name", os.Getenv(common.EnvVarKubeNodeName), "storage type", volumeInfo.StorageType)
		volumeInfo.StorageType = nodeProtocol
	}

	storageNode, _, err := storage.NewStorageNodeAndCommonService(0, config, req.GetSecrets(), &volumeInfo)
	if err != nil {
		e := common.Errorf("NewStorageNode volume ID %s - error: %w", volumeId, err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	resp, err := storageNode.NodeStageVolume(ctx, req)
	if err != nil {
		e := common.Errorf("sn.NodeStageVolume volume ID %s - error: %w", volumeId, err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	slog.Info("Finished", "volumeid", volumeId)
	return resp, nil
}

func (s *NodeServer) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	volumeId := req.GetVolumeId()

	slog.Info("Started", "volumeid", volumeId)

	if volumeId == "" {
		e := common.Errorf("error volumeId parameter was empty")
		slog.Error(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}
	if req.StagingTargetPath == "" {
		e := common.Errorf("error stagingTargetPath parameter was empty")
		slog.Error(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	defer func() {
		isLocking := false
		_ = helper.ManageNodeVolumeMutex(isLocking, "NodeUnstageVolume", req.GetVolumeId())
	}()

	isLocking := true
	_ = helper.ManageNodeVolumeMutex(isLocking, "NodeUnstageVolume", volumeId)

	volumeInfo, err := storagecommon.ValidateVolumeID(volumeId)
	if err != nil {
		e := common.Errorf("ValidateVolumeID volume ID: %s - error: %w", volumeId, err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	nodeProtocol, err := getNodeProtocol(ctx, os.Getenv(common.EnvVarKubeNodeName))
	if err != nil {
		e := common.Errorf("getNodeProtocol volume ID %s - error: %w", req.GetVolumeId(), err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	if nodeProtocol != "" {
		slog.Debug("nodeProtocol found on node, instead of", "nodeprotocol", nodeProtocol, "node name", os.Getenv(common.EnvVarKubeNodeName), "storagetype", volumeInfo.StorageType)
		volumeInfo.StorageType = nodeProtocol
	}

	protocolOperation, err := storage.NewStorageNode(storagecommon.Commonservice{VolProto: &volumeInfo}, 0)
	if err != nil {
		e := common.Errorf("NewStorageNode volume ID: %s - error: %w", volumeId, err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	resp, err := protocolOperation.NodeUnstageVolume(ctx, req)
	if err != nil {
		e := common.Errorf("po.NodeUnstageVolume volume ID: %s - error: %w", volumeId, err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	slog.Info("Finished", "volumeid", volumeId)

	return resp, nil
}

func (s *NodeServer) NodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	volumeID := req.GetVolumeId()
	volumePath := "/host" + req.GetVolumePath()

	slog.Log(ctx, common.LevelTrace, "NodeGetVolumeStats", "volumeID", volumeID, "volume path", volumePath)

	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "NodeGetVolumeStatus - volumeID empty")
	}

	if _, err := os.Lstat(volumePath); err != nil {
		if os.IsNotExist(err) {
			e := common.Errorf("path does not exist %s %w", volumePath, err)
			slog.Error(e.Error())
			return nil, status.Error(codes.NotFound, e.Error())
		}
		e := common.Errorf("failed to stat file %s %w", volumePath, err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	volumeMetrics, err := volume.NewMetricsStatFS(volumePath).GetMetrics()
	if err != nil {
		e := common.Errorf("failed to get metrics %w", err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	available, parseOK := volumeMetrics.Available.AsInt64()
	if !parseOK {
		e := fmt.Errorf("failed to transform volume available size %v", volumeMetrics.Available)
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	capacity, parseOK := volumeMetrics.Capacity.AsInt64()
	if !parseOK {
		e := fmt.Errorf("failed to transform volume capacity size %v", volumeMetrics.Capacity)
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	used, parseOK := volumeMetrics.Used.AsInt64()
	if !parseOK {
		e := fmt.Errorf("failed to transform volume used size %v", volumeMetrics.Used)
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	inodesFree, parseOK := volumeMetrics.InodesFree.AsInt64()
	if !parseOK {
		e := fmt.Errorf("failed to transform disk inodes free(%v)", volumeMetrics.InodesFree)
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	inodes, parseOK := volumeMetrics.Inodes.AsInt64()
	if !parseOK {
		e := fmt.Errorf("failed to transform disk inodes(%v)", volumeMetrics.Inodes)
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	inodesUsed, parseOK := volumeMetrics.InodesUsed.AsInt64()
	if !parseOK {
		e := fmt.Errorf("failed to transform disk inodes used(%v)", volumeMetrics.InodesUsed)
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	resp := &csi.NodeGetVolumeStatsResponse{
		Usage: []*csi.VolumeUsage{
			{
				Unit:      csi.VolumeUsage_BYTES,
				Available: available,
				Total:     capacity,
				Used:      used,
			},
			{
				Unit:      csi.VolumeUsage_INODES,
				Available: inodesFree,
				Total:     inodes,
				Used:      inodesUsed,
			},
		},
	}
	return resp, nil
}

func (s *NodeServer) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	volumeId := req.GetVolumeId()
	slog.Info("Started", "volumeid", volumeId)

	if volumeId == "" {
		e := common.Errorf("error volumeId parameter was empty")
		slog.Error(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	defer func() {
		isLocking := false
		_ = helper.ManageNodeVolumeMutex(isLocking, "NodeExpandVolume", volumeId)
		slog.Debug("unlocking", "volume ID", volumeId)
	}()

	slog.Debug("locking", "volume ID", volumeId)
	isLocking := true
	_ = helper.ManageNodeVolumeMutex(isLocking, "NodeExpandVolume", volumeId)

	config := make(map[string]string)

	volumeInfo, err := storagecommon.ValidateVolumeID(req.GetVolumeId())
	if err != nil {
		e := fmt.Errorf("ValidateVolumeID volume ID %s error %w", req.GetVolumeId(), err)
		slog.Error(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	nodeProtocol, err := getNodeProtocol(ctx, os.Getenv(common.EnvVarKubeNodeName))
	if err != nil {
		e := common.Errorf("getNodeProtocol volume ID %s - error: %w", req.GetVolumeId(), err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	if nodeProtocol != "" {
		slog.Debug("nodeProtocol found on node, using instead of", "node protocol", nodeProtocol, "kube node name", os.Getenv(common.EnvVarKubeNodeName), "storage type", volumeInfo.StorageType)
		volumeInfo.StorageType = nodeProtocol
	}

	// the storageclass is required to specify node-expand secrets as a parameter,this will cause
	// the secret values (hostname, password, username) to be passed down to the NodeExpandVolume function
	err = validateSecret(req.GetVolumeId(), common.CSINodeExpandSecretName, common.CSINodeExpandSecretNamespace, req.GetSecrets())
	if err != nil {
		e := common.Errorf("validateSecret error %w", err)
		slog.Error(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	storageNode, _, err := storage.NewStorageNodeAndCommonService(0, config, req.GetSecrets(), &volumeInfo)
	if err != nil {
		e := common.Errorf("NewStorageNode volume ID: %s - error: %w", volumeId, err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	resp, err := storageNode.NodeExpandVolume(ctx, req)
	if err != nil {
		e := common.Errorf("sn.NodeExpandVolume volume ID: %s - error: %w", volumeId, err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	slog.Info("Finished", "volume ID", volumeId)
	return resp, nil
}

func getNodeFQDN() string {
	iboxHostNamingConvention := os.Getenv("IBOX_HOST_NAMING_CONVENTION")
	if iboxHostNamingConvention == "nodename" {
		nodeName := os.Getenv(common.EnvVarKubeNodeName)
		slog.Debug("using nodename for ibox host naming convention", "node name", nodeName)
		return nodeName
	}
	cmd := "hostname -f"
	out, err := exec.Command("bash", "-c", cmd).Output()
	if err != nil {
		slog.Warn("could not get fqdn with cmd : 'hostname -f', get hostname with 'echo $HOSTNAME'")
		cmd = "echo $HOSTNAME"
		out, err = exec.Command("bash", "-c", cmd).Output()
		if err != nil {
			slog.Error("Failed to execute command", "command", cmd, "error", err)
			return UNKNOWN
		}
	}
	nodeFQDN := string(out)
	if nodeFQDN == "" {
		slog.Warn("node fqnd not found, setting node name as node fqdn instead")
		nodeFQDN = UNKNOWN
	}
	nodeFQDN = strings.TrimSuffix(nodeFQDN, "\n")
	return nodeFQDN
}
