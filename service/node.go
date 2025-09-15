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
	"infinibox-csi-driver/iboxapi"
	"infinibox-csi-driver/storage"
	"os"
	"os/exec"
	"strings"

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

func (s *NodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {

	const FN = "NodePublishVolume"
	zlog.Info().Msgf("%s Started - volume ID: '%s'", FN, req.GetVolumeId())

	if req.GetVolumeId() == "" {
		e := fmt.Errorf("%s - error volumeId parameter was empty", FN)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	if req.GetStagingTargetPath() == "" {
		e := fmt.Errorf("%s - error stagingTargetPath parameter was empty", FN)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	if req.VolumeCapability == nil {
		e := fmt.Errorf("%s - error volumeCapability parameter was nil", FN)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	caps := []*csi.VolumeCapability{
		req.VolumeCapability,
	}

	_, err := validateCapabilities(caps)
	if err != nil {
		e := fmt.Errorf("%s - validateCapabilities - error %s, volume cap %v", FN, err.Error(), req.VolumeCapability)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.FailedPrecondition, e.Error())
	}
	defer func() {
		isLocking := false
		_ = helper.ManageNodeVolumeMutex(isLocking, "NodePublishVolume", req.GetVolumeId())
	}()

	isLocking := true
	_ = helper.ManageNodeVolumeMutex(isLocking, "NodePublishVolume", req.GetVolumeId())

	storageProtocol := req.GetVolumeContext()[common.SC_STORAGE_PROTOCOL]

	fsGroup := req.VolumeCapability.GetMount().GetVolumeMountGroup()

	zlog.Debug().Msgf("VolumeMountGroup: %s", fsGroup)

	config := make(map[string]string)

	volProto, err := storage.ValidateVolumeID(req.GetVolumeId())
	if err != nil {
		e := fmt.Errorf("%s - ValidateVolumeID volume ID: %s error: %s", FN, req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	if volProto.StorageType == common.PROTOCOL_AUTO {
		sp, protocolSecret, err := storage.DetermineProtocol()
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		if sp == common.PROTOCOL_ISCSI {
			req.VolumeContext[common.SC_NETWORK_SPACE] = protocolSecret["iscsi.network_space"]
		}
		if sp == common.PROTOCOL_NVME {
			req.VolumeContext[common.SC_NETWORK_SPACE] = protocolSecret["nvme.network_space"]
		}
		// need to determine the protocol based on user defined protocol order
		// need to look up the network_space for this protocol as defined in the protocol secret
		volProto.StorageType = sp
	}

	// the storageclass is required to specify node-publish secrets as a parameter,this will cause
	// the secret values (hostname, password, username) to be passed down to the NodePublishVolume function
	err = validateSecret("NodePublishVolume", req.GetVolumeId(), common.SC_NODE_PUBLISH_SECRET_NAME, common.SC_NODE_PUBLISH_SECRET_NAMESPACE, req.GetSecrets())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	comnserv, err := storage.BuildCommonService(config, req.GetSecrets(), &volProto)
	if err != nil {
		e := fmt.Errorf("%s - BuildCommonService volume ID: %s error: %s", FN, req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	storageNode, err := storage.NewStorageNode(comnserv, config, req.GetSecrets())
	if err != nil {
		e := fmt.Errorf("%s - NewStorageNode - volume ID: %s error: %s", FN, req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	zlog.Info().Msgf("%s Finished - ID: '%s'", FN, req.GetVolumeId())
	req.VolumeContext["nodeID"] = s.Driver.nodeID
	response, err := storageNode.NodePublishVolume(ctx, req)
	if err != nil {
		e := fmt.Errorf("%s - sn.NodePublishVolume - volume ID: %s error: %s", FN, req.GetVolumeId(), err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	eventData := make([]iboxapi.EventRequestData, 0)
	protocolData := iboxapi.EventRequestData{
		Name:  "protocol",
		Type:  "String",
		Value: volProto.StorageType,
	}
	eventData = append(eventData, protocolData)

	volumeIDData := iboxapi.EventRequestData{
		Name:  common.CUSTOM_EVENT_VOLUME_ID,
		Type:  "String",
		Value: req.GetVolumeId(),
	}
	eventData = append(eventData, volumeIDData)

	actionData := iboxapi.EventRequestData{
		Name:  common.CUSTOM_EVENT_ACTION,
		Type:  "String",
		Value: "Mounted Volume",
	}
	eventData = append(eventData, actionData)

	if storageProtocol == common.PROTOCOL_NFS {
		mountOptions := req.GetVolumeCapability().GetMount().GetMountFlags()
		nfsVersion, nfsPort := storage.GetNFSVersionPort(mountOptions)
		zlog.Debug().Msgf("%s - nfs mount options are [%v], nfs version [%s] port [%s]", FN, mountOptions, nfsVersion, nfsPort)
		actionData := iboxapi.EventRequestData{
			Name:  common.CUSTOM_EVENT_NFS_VERSION,
			Type:  "String",
			Value: nfsVersion,
		}
		eventData = append(eventData, actionData)

	}

	eventErr := helper.CreateEvent(comnserv.Api, comnserv.IboxApi, fmt.Sprintf("CSI - Mounted Volume: volume ID %s", req.GetVolumeId()), eventData)
	if eventErr != nil {
		zlog.Error().Msgf("%s - CreateEvent - error %s", FN, eventErr.Error())
		// only log errors since older ibox versions don't support this event code
	} else {
		zlog.Debug().Msgf("%s - created external event %+v", FN, eventData)
	}

	return response, nil
}

func (s *NodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {

	const FN = "NodeUnpublishVolume"

	zlog.Info().Msgf("%s Started - ID: %s", FN, req.GetVolumeId())

	if req.GetTargetPath() == "" {
		e := fmt.Errorf("%s - error targetPath parameter was empty", FN)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}
	if req.GetVolumeId() == "" {
		e := fmt.Errorf("%s - error volumeId parameter was empty", FN)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	defer func() {
		isLocking := false
		_ = helper.ManageNodeVolumeMutex(isLocking, "NodeUnpublishVolume", req.GetVolumeId())
	}()

	isLocking := true
	_ = helper.ManageNodeVolumeMutex(isLocking, "NodeUnpublishVolume", req.GetVolumeId())

	zlog.Debug().Msgf("%s called with volume ID %s", FN, req.GetVolumeId())
	zlog.Trace().Msgf("%s called with req %+v", FN, req)
	volProto, err := storage.ValidateVolumeID(req.GetVolumeId())
	if err != nil {
		e := fmt.Errorf("%s - ValidateVolumeID volume ID %s - error: %s", FN, req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	if volProto.StorageType == common.PROTOCOL_AUTO {
		sp, protocolSecret, err := storage.DetermineProtocol()
		if err != nil {
			e := fmt.Errorf("%s - DetermineProtocol -  volume ID %s - error: %s", FN, req.GetVolumeId(), err.Error())
			zlog.Error().Msg(e.Error())
			return nil, status.Error(codes.Internal, e.Error())
		}
		volProto.StorageType = sp
		zlog.Debug().Msgf("%s setting auto to %s storageProtocol  protocolSecret %v", FN, volProto.StorageType, protocolSecret)
	}

	protocolOperation, err := storage.NewStorageNode(storage.Commonservice{VolProto: &volProto}, nil, nil)
	if err != nil {
		e := fmt.Errorf("%s - NewStorageNode volume ID %s - error: %s", FN, req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	resp, err := protocolOperation.NodeUnpublishVolume(ctx, req)
	if err != nil {
		// TODO do we trust the error being correctly set with a valid gRPC status code?
		zlog.Error().Msgf("%s NodeUnpublishVolume volume ID %s - error: %s", FN, req.GetVolumeId(), err.Error())
		return nil, err
	}

	zlog.Info().Msgf("%s Finished - ID: %s", FN, req.GetVolumeId())
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

	const FN = "NodeStageVolume"
	volumeId := req.GetVolumeId()
	zlog.Info().Msgf("%s Started - ID: '%s'", FN, volumeId)

	if volumeId == "" {
		e := fmt.Errorf("%s -  error volumeId parameter was empty", FN)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}
	if req.VolumeCapability == nil {
		e := fmt.Errorf("%s - error volumeCapability parameter was nil", FN)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	caps := []*csi.VolumeCapability{
		req.VolumeCapability,
	}

	_, err := validateCapabilities(caps)
	if err != nil {
		e := fmt.Errorf("%s - validateCapabilities - error %s", FN, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.FailedPrecondition, e.Error())
	}

	if req.StagingTargetPath == "" {
		e := fmt.Errorf("%s  - error stagingTargetPath parameter was empty", FN)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	defer func() {
		isLocking := false
		_ = helper.ManageNodeVolumeMutex(isLocking, "NodeStageVolume", req.GetVolumeId())
	}()

	isLocking := true
	_ = helper.ManageNodeVolumeMutex(isLocking, "NodeStageVolume", req.GetVolumeId())

	//storageProtocol := req.GetVolumeContext()[common.SC_STORAGE_PROTOCOL]

	fsGroup := req.VolumeCapability.GetMount().GetVolumeMountGroup()

	zlog.Debug().Msgf("VolumeMountGroup: %s", fsGroup)
	config := make(map[string]string)

	volProto, err := storage.ValidateVolumeID(req.GetVolumeId())
	if err != nil {
		e := fmt.Errorf("%s - ValidateVolumeID -  volume ID %s - error: %s", FN, req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	if volProto.StorageType == common.PROTOCOL_AUTO {
		sp, protocolSecret, err := storage.DetermineProtocol()
		if err != nil {
			e := fmt.Errorf("%s - DetermineProtocol -  volume ID %s - error: %s", FN, req.GetVolumeId(), err.Error())
			zlog.Error().Msg(e.Error())
			return nil, status.Error(codes.Internal, e.Error())
		}
		volProto.StorageType = sp
		zlog.Debug().Msgf("%s setting auto to %s storageProtocol  protocolSecret %v", FN, volProto.StorageType, protocolSecret)
	}

	zlog.Debug().Msgf("%s volumeContext %+v storageProtocol is %s", FN, req.GetVolumeContext(), volProto.StorageType)

	err = validateSecret("NodeStageVolume", req.GetVolumeId(), common.SC_NODE_STAGE_SECRET_NAME, common.SC_NODE_STAGE_SECRET_NAMESPACE, req.GetSecrets())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	comnserv, err := storage.BuildCommonService(config, req.GetSecrets(), &volProto)
	if err != nil {
		e := fmt.Errorf("%s - BuildCommonService volume ID %s - error: %s", FN, volumeId, err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	storageNode, err := storage.NewStorageNode(comnserv, config, req.GetSecrets())
	if err != nil {
		e := fmt.Errorf("%s - NewStorageNode volume ID %s - error: %s", FN, volumeId, err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	resp, err := storageNode.NodeStageVolume(ctx, req)
	if err != nil {
		e := fmt.Errorf("%s - sn.NodeStageVolume volume ID %s - error: %s", FN, volumeId, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	zlog.Info().Msgf("%s Finished - ID: '%s'", FN, volumeId)
	return resp, nil

}

func (s *NodeServer) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	const FN = "NodeUnstageVolume"
	volumeId := req.GetVolumeId()

	zlog.Info().Msgf("%s Started - ID: %s", FN, volumeId)

	if volumeId == "" {
		e := fmt.Errorf("%s - error volumeId parameter was empty", FN)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}
	if req.StagingTargetPath == "" {
		e := fmt.Errorf("%s - error stagingTargetPath parameter was empty", FN)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	defer func() {
		isLocking := false
		_ = helper.ManageNodeVolumeMutex(isLocking, "NodeUnstageVolume", req.GetVolumeId())
	}()

	isLocking := true
	_ = helper.ManageNodeVolumeMutex(isLocking, "NodeUnstageVolume", volumeId)

	volProto, err := storage.ValidateVolumeID(volumeId)
	if err != nil {
		e := fmt.Errorf("%s - ValidateVolumeID volume ID: %s - error: %s", FN, volumeId, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	if volProto.StorageType == common.PROTOCOL_AUTO {
		sp, protocolSecret, err := storage.DetermineProtocol()
		if err != nil {
			e := fmt.Errorf("%s - DetermineProtocol -  volume ID %s - error: %s", FN, req.GetVolumeId(), err.Error())
			zlog.Error().Msg(e.Error())
			return nil, status.Error(codes.Internal, e.Error())
		}
		volProto.StorageType = sp
		zlog.Debug().Msgf("%s setting auto to %s storageProtocol  protocolSecret %v", FN, volProto.StorageType, protocolSecret)
	}
	protocolOperation, err := storage.NewStorageNode(storage.Commonservice{VolProto: &volProto}, nil, nil)
	if err != nil {
		e := fmt.Errorf("%s - NewStorageNode volume ID: %s - error: %s", FN, volumeId, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	resp, err := protocolOperation.NodeUnstageVolume(ctx, req)
	if err != nil {
		e := fmt.Errorf("%s - po.NodeUnstageVolume volume ID: %s - error: %s", FN, volumeId, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	zlog.Info().Msgf("%s Finished - volume ID: '%s'", FN, volumeId)

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

	available, ok := volumeMetrics.Available.AsInt64()
	if !ok {
		return nil, status.Errorf(codes.Internal, "failed to transform volume available size(%v)", volumeMetrics.Available)
	}
	capacity, ok := volumeMetrics.Capacity.AsInt64()
	if !ok {
		return nil, status.Errorf(codes.Internal, "failed to transform volume capacity size(%v)", volumeMetrics.Capacity)
	}
	used, ok := volumeMetrics.Used.AsInt64()
	if !ok {
		return nil, status.Errorf(codes.Internal, "failed to transform volume used size(%v)", volumeMetrics.Used)
	}

	inodesFree, ok := volumeMetrics.InodesFree.AsInt64()
	if !ok {
		return nil, status.Errorf(codes.Internal, "failed to transform disk inodes free(%v)", volumeMetrics.InodesFree)
	}
	inodes, ok := volumeMetrics.Inodes.AsInt64()
	if !ok {
		return nil, status.Errorf(codes.Internal, "failed to transform disk inodes(%v)", volumeMetrics.Inodes)
	}
	inodesUsed, ok := volumeMetrics.InodesUsed.AsInt64()
	if !ok {
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
	const FN = "NodeExpandVolume"
	volumeId := req.GetVolumeId()
	zlog.Info().Msgf("%s Started - volume ID: '%s'", FN, volumeId)

	if volumeId == "" {
		e := fmt.Errorf("%s - error volumeId parameter was empty", FN)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	defer func() {
		isLocking := false
		_ = helper.ManageNodeVolumeMutex(isLocking, "NodeExpandVolume", volumeId)
		zlog.Debug().Msgf("%s unlocking - volume ID: '%s'", FN, volumeId)
	}()

	zlog.Debug().Msgf("%s locking - volume ID: '%s'", FN, volumeId)
	isLocking := true
	_ = helper.ManageNodeVolumeMutex(isLocking, "NodeExpandVolume", volumeId)

	/**
	volproto := strings.Split(req.GetVolumeId(), "$$")
	if len(volproto) != 2 {
		e := fmt.Errorf("%s - error volume ID error %v", function, volproto)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.NotFound, e.Error())
	}
	*/

	config := make(map[string]string)

	volProto, err := storage.ValidateVolumeID(req.GetVolumeId())
	if err != nil {
		zlog.Error().Msgf("%s  - ValidateVolumeID -  volume ID: %s - error: %s", FN, req.GetVolumeId(), err.Error())
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	// the storageclass is required to specify node-expand secrets as a parameter,this will cause
	// the secret values (hostname, password, username) to be passed down to the NodeExpandVolume function
	err = validateSecret("NodeExpandVolume", req.GetVolumeId(), common.SC_NODE_EXPAND_SECRET_NAME, common.SC_NODE_EXPAND_SECRET_NAMESPACE, req.GetSecrets())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	comnserv, err := storage.BuildCommonService(config, req.GetSecrets(), &volProto)
	if err != nil {
		e := fmt.Errorf("%s  - BuildCommonService volume ID: %s - error: %s", FN, volumeId, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	storageNode, err := storage.NewStorageNode(comnserv, config, req.GetSecrets())
	if err != nil {
		e := fmt.Errorf("%s - NewStorageNode volume ID: %s - error: %s", FN, volumeId, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	resp, err := storageNode.NodeExpandVolume(context.Background(), req)
	if err != nil {
		e := fmt.Errorf("%s - sn.NodeExpandVolume volume ID: %s - error: %s", FN, volumeId, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	zlog.Info().Msgf("%s Finished - volume ID: '%s'", FN, volumeId)
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
