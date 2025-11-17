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
	const functionName = "NodePublishVolume"
	zlog.Info().Msgf("%s Started - volume ID: '%s'", functionName, req.GetVolumeId())

	if req.GetVolumeId() == "" {
		e := fmt.Errorf("%s - error volumeId parameter was empty", functionName)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	if req.GetStagingTargetPath() == "" {
		e := fmt.Errorf("%s - error stagingTargetPath parameter was empty", functionName)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	if req.VolumeCapability == nil {
		e := fmt.Errorf("%s - error volumeCapability parameter was nil", functionName)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	caps := []*csi.VolumeCapability{
		req.VolumeCapability,
	}

	_, err := validateCapabilities(caps)
	if err != nil {
		e := fmt.Errorf("%s - validateCapabilities - error %s, volume cap %v", functionName, err.Error(), req.VolumeCapability)
		zlog.Error().Msg(e.Error())
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

	zlog.Debug().Msgf("VolumeMountGroup: %s", fsGroup)

	config := make(map[string]string)

	volumeInfo, err := storagecommon.ValidateVolumeID(req.GetVolumeId())
	if err != nil {
		e := fmt.Errorf("%s - ValidateVolumeID volume ID: %s error: %s", functionName, req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	nodeProtocol, err := getNodeProtocol(ctx, os.Getenv(common.EnvVarKubeNodeName))
	if err != nil {
		e := fmt.Errorf("%s - getNodeProtocol volume ID %s - error: %s", functionName, req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	if nodeProtocol != "" {
		zlog.Debug().Msgf("%s nodeProtocol [%s] found on node %s, using instead of %s", functionName, nodeProtocol, os.Getenv(common.EnvVarKubeNodeName), volumeInfo.StorageType)
		volumeInfo.StorageType = nodeProtocol
		protocolSecret, protocolSecretInUse, err := GetProtocolSecret(ctx)
		if err != nil {
			e := fmt.Errorf("%s error: could not get protocol secret %s", functionName, err.Error())
			return nil, status.Error(codes.InvalidArgument, e.Error())
		}
		if !protocolSecretInUse {
			e := fmt.Errorf("%s error: protocol secret not in use, but is required when nodeProtocol label is set on node", functionName)
			return nil, status.Error(codes.InvalidArgument, e.Error())
		}
		if nodeProtocol == common.ProtocolISCSI {
			networkSpace := protocolSecret[ProtocolSecretISCSINetworkSpace]
			if networkSpace == "" {
				e := fmt.Errorf("%s error: protocol secret in use, but ISCSI network space is empty", functionName)
				return nil, status.Error(codes.InvalidArgument, e.Error())
			}
			req.VolumeContext[common.StorageClassNetworkSpace] = networkSpace
		}
		if nodeProtocol == common.ProtocolNVME {
			networkSpace := protocolSecret[ProtocolSecretNVMENetworkSpace]
			if networkSpace == "" {
				e := fmt.Errorf("%s error: protocol secret in use, but NVMEe network space is empty", functionName)
				return nil, status.Error(codes.InvalidArgument, e.Error())
			}
			req.VolumeContext[common.StorageClassNetworkSpace] = networkSpace
		}
	}

	// the storageclass is required to specify node-publish secrets as a parameter,this will cause
	// the secret values (hostname, password, username) to be passed down to the NodePublishVolume function
	err = validateSecret("NodePublishVolume", req.GetVolumeId(), common.CSINodePublishSecretName, common.CSINodePublishSecretNamespace, req.GetSecrets())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	comnserv, err := storagecommon.BuildCommonService(config, req.GetSecrets(), &volumeInfo)
	if err != nil {
		e := fmt.Errorf("%s - BuildCommonService volume ID: %s error: %s", functionName, req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	storageNode, err := storage.NewStorageNode(comnserv)
	if err != nil {
		e := fmt.Errorf("%s - NewStorageNode - volume ID: %s error: %s", functionName, req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	zlog.Info().Msgf("%s Finished - ID: '%s'", functionName, req.GetVolumeId())
	req.VolumeContext["nodeID"] = s.Driver.nodeID
	response, err := storageNode.NodePublishVolume(ctx, req)
	if err != nil {
		e := fmt.Errorf("%s - sn.NodePublishVolume - volume ID: %s error: %s", functionName, req.GetVolumeId(), err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	if storageProtocol == common.ProtocolNFS {
		mountOptions := req.GetVolumeCapability().GetMount().GetMountFlags()
		nfsVersion, nfsPort := nfs.GetNFSVersionPort(mountOptions)
		zlog.Debug().Msgf("%s - nfs mount options are [%v], nfs version [%s] port [%s]", functionName, mountOptions, nfsVersion, nfsPort)
		helper.EventNFSVersions[nfsVersion]++
	}

	helper.EventAPIClient = comnserv.API
	helper.EventIboxAPIClient = comnserv.IboxAPI
	helper.EventPublishedVolumes[volumeInfo.StorageType]++

	return response, nil
}

func (s *NodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	const functionName = "NodeUnpublishVolume"

	zlog.Info().Msgf("%s Started - ID: %s", functionName, req.GetVolumeId())

	if req.GetTargetPath() == "" {
		e := fmt.Errorf("%s - error targetPath parameter was empty", functionName)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}
	if req.GetVolumeId() == "" {
		e := fmt.Errorf("%s - error volumeId parameter was empty", functionName)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	defer func() {
		isLocking := false
		_ = helper.ManageNodeVolumeMutex(isLocking, "NodeUnpublishVolume", req.GetVolumeId())
	}()

	isLocking := true
	_ = helper.ManageNodeVolumeMutex(isLocking, "NodeUnpublishVolume", req.GetVolumeId())

	zlog.Debug().Msgf("%s called with volume ID %s", functionName, req.GetVolumeId())
	zlog.Trace().Msgf("%s called with req %+v", functionName, req)
	volumeInfo, err := storagecommon.ValidateVolumeID(req.GetVolumeId())
	if err != nil {
		e := fmt.Errorf("%s - ValidateVolumeID volume ID %s - error: %s", functionName, req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	nodeProtocol, err := getNodeProtocol(ctx, os.Getenv(common.EnvVarKubeNodeName))
	if err != nil {
		e := fmt.Errorf("%s - getNodeProtocol volume ID %s - error: %s", functionName, req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	if nodeProtocol != "" {
		zlog.Debug().Msgf("%s nodeProtocol [%s] found on node %s, using instead of %s", functionName, nodeProtocol, os.Getenv(common.EnvVarKubeNodeName), volumeInfo.StorageType)
		volumeInfo.StorageType = nodeProtocol
	}

	protocolOperation, err := storage.NewStorageNode(storagecommon.Commonservice{VolProto: &volumeInfo})
	if err != nil {
		e := fmt.Errorf("%s - NewStorageNode volume ID %s - error: %s", functionName, req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	resp, err := protocolOperation.NodeUnpublishVolume(ctx, req)
	if err != nil {
		// TODO do we trust the error being correctly set with a valid gRPC status code?
		zlog.Error().Msgf("%s NodeUnpublishVolume volume ID %s - error: %s", functionName, req.GetVolumeId(), err.Error())
		return nil, err
	}

	zlog.Info().Msgf("%s Finished - ID: %s", functionName, req.GetVolumeId())
	return resp, nil
}

func (s *NodeServer) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	// set as trace because it happens frequently
	zlog.Trace().Msgf("NodeGetCapabilities Requested - Node: %s capabilities: %v", s.Driver.nodeID, s.Driver.nscap)

	return &csi.NodeGetCapabilitiesResponse{
		Capabilities: s.Driver.nscap,
	}, nil
}

func (s *NodeServer) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	zlog.Debug().Msgf("NodeGetInfo Requested - Node: %s", s.Driver.nodeID)

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
	const functionName = "NodeStageVolume"
	volumeId := req.GetVolumeId()
	zlog.Info().Msgf("%s Started - ID: '%s'", functionName, volumeId)

	if volumeId == "" {
		e := fmt.Errorf("%s -  error volumeId parameter was empty", functionName)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}
	if req.VolumeCapability == nil {
		e := fmt.Errorf("%s - error volumeCapability parameter was nil", functionName)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	caps := []*csi.VolumeCapability{
		req.VolumeCapability,
	}

	_, err := validateCapabilities(caps)
	if err != nil {
		e := fmt.Errorf("%s - validateCapabilities - error %s", functionName, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.FailedPrecondition, e.Error())
	}

	if req.StagingTargetPath == "" {
		e := fmt.Errorf("%s  - error stagingTargetPath parameter was empty", functionName)
		zlog.Error().Msg(e.Error())
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

	zlog.Debug().Msgf("VolumeMountGroup: %s", fsGroup)
	config := make(map[string]string)

	volumeInfo, err := storagecommon.ValidateVolumeID(req.GetVolumeId())
	if err != nil {
		e := fmt.Errorf("%s - ValidateVolumeID -  volume ID %s - error: %s", functionName, req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	zlog.Debug().Msgf("%s volumeContext %+v storageProtocol is %s", functionName, req.GetVolumeContext(), volumeInfo.StorageType)

	err = validateSecret("NodeStageVolume", req.GetVolumeId(), common.CSINodeStageSecretName, common.CSINodeStageSecretNamespace, req.GetSecrets())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	nodeProtocol, err := getNodeProtocol(ctx, os.Getenv(common.EnvVarKubeNodeName))
	if err != nil {
		e := fmt.Errorf("%s - getNodeProtocol volume ID %s - error: %s", functionName, req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	if nodeProtocol != "" {
		zlog.Debug().Msgf("%s nodeProtocol [%s] found on node %s, instead of %s", functionName, nodeProtocol, os.Getenv(common.EnvVarKubeNodeName), volumeInfo.StorageType)
		volumeInfo.StorageType = nodeProtocol
	}

	comnserv, err := storagecommon.BuildCommonService(config, req.GetSecrets(), &volumeInfo)
	if err != nil {
		e := fmt.Errorf("%s - BuildCommonService volume ID %s - error: %s", functionName, volumeId, err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	storageNode, err := storage.NewStorageNode(comnserv)
	if err != nil {
		e := fmt.Errorf("%s - NewStorageNode volume ID %s - error: %s", functionName, volumeId, err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	resp, err := storageNode.NodeStageVolume(ctx, req)
	if err != nil {
		e := fmt.Errorf("%s - sn.NodeStageVolume volume ID %s - error: %s", functionName, volumeId, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	zlog.Info().Msgf("%s Finished - ID: '%s'", functionName, volumeId)
	return resp, nil
}

func (s *NodeServer) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	const functionName = "NodeUnstageVolume"
	volumeId := req.GetVolumeId()

	zlog.Info().Msgf("%s Started - ID: %s", functionName, volumeId)

	if volumeId == "" {
		e := fmt.Errorf("%s - error volumeId parameter was empty", functionName)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}
	if req.StagingTargetPath == "" {
		e := fmt.Errorf("%s - error stagingTargetPath parameter was empty", functionName)
		zlog.Error().Msg(e.Error())
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
		e := fmt.Errorf("%s - ValidateVolumeID volume ID: %s - error: %s", functionName, volumeId, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	nodeProtocol, err := getNodeProtocol(ctx, os.Getenv(common.EnvVarKubeNodeName))
	if err != nil {
		e := fmt.Errorf("%s - getNodeProtocol volume ID %s - error: %s", functionName, req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	if nodeProtocol != "" {
		zlog.Debug().Msgf("%s nodeProtocol [%s] found on node %s, instead of %s", functionName, nodeProtocol, os.Getenv(common.EnvVarKubeNodeName), volumeInfo.StorageType)
		volumeInfo.StorageType = nodeProtocol
	}

	protocolOperation, err := storage.NewStorageNode(storagecommon.Commonservice{VolProto: &volumeInfo})
	if err != nil {
		e := fmt.Errorf("%s - NewStorageNode volume ID: %s - error: %s", functionName, volumeId, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	resp, err := protocolOperation.NodeUnstageVolume(ctx, req)
	if err != nil {
		e := fmt.Errorf("%s - po.NodeUnstageVolume volume ID: %s - error: %s", functionName, volumeId, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	zlog.Info().Msgf("%s Finished - volume ID: '%s'", functionName, volumeId)

	return resp, nil
}

func (s *NodeServer) NodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	volumeID := req.GetVolumeId()
	volumePath := "/host" + req.GetVolumePath()

	zlog.Trace().Msgf("NodeGetVolumeStats volumeID [%s] volume path [%s]", volumeID, volumePath)

	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "NodeGetVolumeStatus - volumeID empty")
	}

	if _, err := os.Lstat(volumePath); err != nil {
		if os.IsNotExist(err) {
			return nil, status.Errorf(codes.NotFound, "path %s does not exist", volumePath)
		}
		return nil, status.Errorf(codes.Internal, "failed to stat file %s: %v", volumePath, err)
	}

	volumeMetrics, err := volume.NewMetricsStatFS(volumePath).GetMetrics()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get metrics: %v", err)
	}

	available, parseOK := volumeMetrics.Available.AsInt64()
	if !parseOK {
		return nil, status.Errorf(codes.Internal, "failed to transform volume available size(%v)", volumeMetrics.Available)
	}
	capacity, parseOK := volumeMetrics.Capacity.AsInt64()
	if !parseOK {
		return nil, status.Errorf(codes.Internal, "failed to transform volume capacity size(%v)", volumeMetrics.Capacity)
	}
	used, parseOK := volumeMetrics.Used.AsInt64()
	if !parseOK {
		return nil, status.Errorf(codes.Internal, "failed to transform volume used size(%v)", volumeMetrics.Used)
	}

	inodesFree, parseOK := volumeMetrics.InodesFree.AsInt64()
	if !parseOK {
		return nil, status.Errorf(codes.Internal, "failed to transform disk inodes free(%v)", volumeMetrics.InodesFree)
	}
	inodes, parseOK := volumeMetrics.Inodes.AsInt64()
	if !parseOK {
		return nil, status.Errorf(codes.Internal, "failed to transform disk inodes(%v)", volumeMetrics.Inodes)
	}
	inodesUsed, parseOK := volumeMetrics.InodesUsed.AsInt64()
	if !parseOK {
		return nil, status.Errorf(codes.Internal, "failed to transform disk inodes used(%v)", volumeMetrics.InodesUsed)
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
	const functionName = "NodeExpandVolume"
	volumeId := req.GetVolumeId()
	zlog.Info().Msgf("%s Started - volume ID: '%s'", functionName, volumeId)

	if volumeId == "" {
		e := fmt.Errorf("%s - error volumeId parameter was empty", functionName)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	defer func() {
		isLocking := false
		_ = helper.ManageNodeVolumeMutex(isLocking, "NodeExpandVolume", volumeId)
		zlog.Debug().Msgf("%s unlocking - volume ID: '%s'", functionName, volumeId)
	}()

	zlog.Debug().Msgf("%s locking - volume ID: '%s'", functionName, volumeId)
	isLocking := true
	_ = helper.ManageNodeVolumeMutex(isLocking, "NodeExpandVolume", volumeId)

	config := make(map[string]string)

	volumeInfo, err := storagecommon.ValidateVolumeID(req.GetVolumeId())
	if err != nil {
		zlog.Error().Msgf("%s  - ValidateVolumeID -  volume ID: %s - error: %s", functionName, req.GetVolumeId(), err.Error())
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	nodeProtocol, err := getNodeProtocol(ctx, os.Getenv(common.EnvVarKubeNodeName))
	if err != nil {
		e := fmt.Errorf("%s - getNodeProtocol volume ID %s - error: %s", functionName, req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	if nodeProtocol != "" {
		zlog.Debug().Msgf("%s nodeProtocol [%s] found on node %s, using instead of %s", functionName, nodeProtocol, os.Getenv(common.EnvVarKubeNodeName), volumeInfo.StorageType)
		volumeInfo.StorageType = nodeProtocol
	}

	// the storageclass is required to specify node-expand secrets as a parameter,this will cause
	// the secret values (hostname, password, username) to be passed down to the NodeExpandVolume function
	err = validateSecret("NodeExpandVolume", req.GetVolumeId(), common.CSINodeExpandSecretName, common.CSINodeExpandSecretNamespace, req.GetSecrets())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	comnserv, err := storagecommon.BuildCommonService(config, req.GetSecrets(), &volumeInfo)
	if err != nil {
		e := fmt.Errorf("%s  - BuildCommonService volume ID: %s - error: %s", functionName, volumeId, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	storageNode, err := storage.NewStorageNode(comnserv)
	if err != nil {
		e := fmt.Errorf("%s - NewStorageNode volume ID: %s - error: %s", functionName, volumeId, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	resp, err := storageNode.NodeExpandVolume(ctx, req)
	if err != nil {
		e := fmt.Errorf("%s - sn.NodeExpandVolume volume ID: %s - error: %s", functionName, volumeId, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	zlog.Info().Msgf("%s Finished - volume ID: '%s'", functionName, volumeId)
	return resp, nil
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
			return UNKNOWN
		}
	}
	nodeFQDN := string(out)
	if nodeFQDN == "" {
		zlog.Warn().Msgf("node fqnd not found, setting node name as node fqdn instead")
		nodeFQDN = UNKNOWN
	}
	nodeFQDN = strings.TrimSuffix(nodeFQDN, "\n")
	return nodeFQDN
}
