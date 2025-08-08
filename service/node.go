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
	csi.UnimplementedNodeServer
}

func (s *NodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {

	zlog.Info().Msgf("NodePublishVolume Started - volume ID: '%s'", req.GetVolumeId())

	if req.GetVolumeId() == "" {
		e := fmt.Errorf("NodePublishVolume - error volumeId parameter was empty")
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	if req.GetStagingTargetPath() == "" {
		e := fmt.Errorf("NodeUnstageVolume - error stagingTargetPath parameter was empty")
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	if req.VolumeCapability == nil {
		e := fmt.Errorf("NodeUnstageVolume - error volumeCapability parameter was nil")
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	caps := []*csi.VolumeCapability{
		req.VolumeCapability,
	}

	_, err := validateCapabilities(caps)
	if err != nil {
		e := fmt.Errorf("NodePublishVolume - validateCapabilities - error %s, volume cap %v", err.Error(), req.VolumeCapability)
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
		e := fmt.Errorf("NodePublishVolume - ValidateVolumeID volume ID: %s error: %s", req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	// the storageclass is required to specify node-publish secrets as a parameter,this will cause
	// the secret values (hostname, password, username) to be passed down to the NodePublishVolume function
	err = validateSecret("NodePublishVolume", req.GetVolumeId(), common.SC_NODE_PUBLISH_SECRET_NAME, common.SC_NODE_PUBLISH_SECRET_NAMESPACE, req.GetSecrets())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	comnserv, err := storage.BuildCommonService(config, req.GetSecrets(), &volProto)
	if err != nil {
		e := fmt.Errorf("NodePublishVolume - BuildCommonService volume ID: %s error: %s", req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	storageNode, err := storage.NewStorageNode(comnserv, config, req.GetSecrets())
	if err != nil {
		e := fmt.Errorf("NodePublishVolume - NewStorageNode - volume ID: %s error: %s", req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	zlog.Info().Msgf("NodePublishVolume Finished - ID: '%s'", req.GetVolumeId())
	req.VolumeContext["nodeID"] = s.Driver.nodeID
	response, err := storageNode.NodePublishVolume(ctx, req)
	if err != nil {
		e := fmt.Errorf("NodePublishVolume - sn.NodePublishVolume - volume ID: %s error: %s", req.GetVolumeId(), err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	eventData := make([]iboxapi.EventRequestData, 0)
	protocolData := iboxapi.EventRequestData{
		Name:  "protocol",
		Type:  "String",
		Value: storageProtocol,
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
		zlog.Debug().Msgf("NodePublishVolume - nfs mount options are [%v], nfs version [%s] port [%s]", mountOptions, nfsVersion, nfsPort)
		actionData := iboxapi.EventRequestData{
			Name:  common.CUSTOM_EVENT_NFS_VERSION,
			Type:  "String",
			Value: nfsVersion,
		}
		eventData = append(eventData, actionData)

	}

	eventErr := helper.CreateEvent(comnserv.Api, comnserv.IboxApi, fmt.Sprintf("CSI - Mounted Volume: volume ID %s", req.GetVolumeId()), eventData)
	if eventErr != nil {
		zlog.Error().Msgf("NodePublishVolume - CreateEvent - error %s", eventErr.Error())
		// only log errors since older ibox versions don't support this event code
	} else {
		zlog.Debug().Msgf("NodePublishVolume - created external event %+v", eventData)
	}

	return response, nil
}

func (s *NodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {

	zlog.Info().Msgf("NodeUnpublishVolume Started - ID: %s", req.GetVolumeId())

	if req.GetTargetPath() == "" {
		e := fmt.Errorf("NodeUnpublishVolume - error targetPath parameter was empty")
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}
	if req.GetVolumeId() == "" {
		e := fmt.Errorf("NodeUnpublishVolume - error volumeId parameter was empty")
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	defer func() {
		isLocking := false
		_ = helper.ManageNodeVolumeMutex(isLocking, "NodeUnpublishVolume", req.GetVolumeId())
	}()

	isLocking := true
	_ = helper.ManageNodeVolumeMutex(isLocking, "NodeUnpublishVolume", req.GetVolumeId())

	zlog.Debug().Msgf("NodeUnpublishVolume called with volume ID %s", req.GetVolumeId())
	zlog.Trace().Msgf("NodeUnpublishVolume called with req %+v", req)
	volProto, err := storage.ValidateVolumeID(req.GetVolumeId())
	if err != nil {
		e := fmt.Errorf("NodeUnpublishVolume - ValidateVolumeID volume ID %s - error: %s", req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	protocolOperation, err := storage.NewStorageNode(storage.Commonservice{VolProto: &volProto}, nil, nil)
	if err != nil {
		e := fmt.Errorf("NodeUnpublishVolume - NewStorageNode volume ID %s - error: %s", req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	resp, err := protocolOperation.NodeUnpublishVolume(ctx, req)
	if err != nil {
		// TODO do we trust the error being correctly set with a valid gRPC status code?
		zlog.Error().Msgf("NodeUnpublishVolume NodeUnpublishVolume volume ID %s - error: %s", req.GetVolumeId(), err.Error())
		return nil, err
	}

	zlog.Info().Msgf("NodeUnpublishVolume Finished - ID: %s", req.GetVolumeId())
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
	volumeId := req.GetVolumeId()
	zlog.Info().Msgf("NodeStageVolume Started - ID: '%s'", volumeId)
	zlog.Debug().Msgf("NodeStageVolume jeff secrets %v", req.GetSecrets())

	if volumeId == "" {
		e := fmt.Errorf("NodeStageVolume -  error volumeId parameter was empty")
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}
	if req.VolumeCapability == nil {
		e := fmt.Errorf("NodeStageVolume - error volumeCapability parameter was nil")
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	caps := []*csi.VolumeCapability{
		req.VolumeCapability,
	}

	_, err := validateCapabilities(caps)
	if err != nil {
		e := fmt.Errorf("NodeStageVolume - validateCapabilities - error %s", err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.FailedPrecondition, e.Error())
	}

	if req.StagingTargetPath == "" {
		e := fmt.Errorf("NodeStageVolume  - error stagingTargetPath parameter was empty")
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	defer func() {
		isLocking := false
		_ = helper.ManageNodeVolumeMutex(isLocking, "NodeStageVolume", req.GetVolumeId())
	}()

	isLocking := true
	_ = helper.ManageNodeVolumeMutex(isLocking, "NodeStageVolume", req.GetVolumeId())

	storageProtocol := req.GetVolumeContext()[common.SC_STORAGE_PROTOCOL]

	fsGroup := req.VolumeCapability.GetMount().GetVolumeMountGroup()

	zlog.Debug().Msgf("VolumeMountGroup: %s", fsGroup)
	config := make(map[string]string)

	volProto, err := storage.ValidateVolumeID(req.GetVolumeId())
	if err != nil {
		e := fmt.Errorf("NodeStageVolume - ValidateVolumeID -  volume ID %s - error: %s", req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	zlog.Debug().Msgf("NodeStageVolume volumeContext %+v storageProtocol is %s", req.GetVolumeContext(), storageProtocol)

	err = validateSecret("NodeStageVolume", req.GetVolumeId(), common.SC_NODE_STAGE_SECRET_NAME, common.SC_NODE_STAGE_SECRET_NAMESPACE, req.GetSecrets())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	comnserv, err := storage.BuildCommonService(config, req.GetSecrets(), &volProto)
	if err != nil {
		e := fmt.Errorf("NodeStageVolume - BuildCommonService volume ID %s - error: %s", volumeId, err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	storageNode, err := storage.NewStorageNode(comnserv, config, req.GetSecrets())
	if err != nil {
		e := fmt.Errorf("NodeStageVolume - NewStorageNode volume ID %s - error: %s", volumeId, err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	resp, err := storageNode.NodeStageVolume(ctx, req)
	if err != nil {
		e := fmt.Errorf("NodeStageVolume - sn.NodeStageVolume volume ID %s - error: %s", volumeId, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	zlog.Info().Msgf("NodeStageVolume Finished - ID: '%s'", volumeId)
	return resp, nil

}

func (s *NodeServer) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	volumeId := req.GetVolumeId()

	zlog.Info().Msgf("NodeUnstageVolume Started - ID: %s", volumeId)

	if volumeId == "" {
		e := fmt.Errorf("NodeUnstageVolume - error volumeId parameter was empty")
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}
	if req.StagingTargetPath == "" {
		e := fmt.Errorf("NodeUnstageVolume - error stagingTargetPath parameter was empty")
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
		e := fmt.Errorf("NodeUnstageVolume - ValidateVolumeID volume ID: %s - error: %s", volumeId, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	protocolOperation, err := storage.NewStorageNode(storage.Commonservice{VolProto: &volProto}, nil, nil)
	if err != nil {
		e := fmt.Errorf("NodeUnstageVolume - NewStorageNode volume ID: %s - error: %s", volumeId, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	resp, err := protocolOperation.NodeUnstageVolume(ctx, req)
	if err != nil {
		e := fmt.Errorf("NodeUnstageVolume - po.NodeUnstageVolume volume ID: %s - error: %s", volumeId, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	zlog.Info().Msgf("NodeUnstageVolume Finished - volume ID: '%s'", volumeId)

	return resp, nil
}

func (s *NodeServer) NodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	return nil, status.Error(codes.Unimplemented, time.Now().String())
}

func (s *NodeServer) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	volumeId := req.GetVolumeId()
	zlog.Info().Msgf("NodeExpandVolume Started - volume ID: '%s'", volumeId)
	zlog.Debug().Msgf("NodeExpandVolume jeff secrets %v", req.GetSecrets())

	if volumeId == "" {
		e := fmt.Errorf("NodeExpandVolume - error volumeId parameter was empty")
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	defer func() {
		isLocking := false
		_ = helper.ManageNodeVolumeMutex(isLocking, "NodeExpandVolume", volumeId)
		zlog.Debug().Msgf("NodeExpandVolume unlocking - volume ID: '%s'", volumeId)
	}()

	zlog.Debug().Msgf("NodeExpandVolume locking - volume ID: '%s'", volumeId)
	isLocking := true
	_ = helper.ManageNodeVolumeMutex(isLocking, "NodeExpandVolume", volumeId)

	volproto := strings.Split(req.GetVolumeId(), "$$")
	if len(volproto) != 2 {
		e := fmt.Errorf("NodeExpandVolume - error volume ID error %v", volproto)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.NotFound, e.Error())
	}

	config := make(map[string]string)

	volProto, err := storage.ValidateVolumeID(req.GetVolumeId())
	if err != nil {
		zlog.Error().Msgf("NodeExpandVolume  - ValidateVolumeID -  volume ID: %s - error: %s", req.GetVolumeId(), err.Error())
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
		e := fmt.Errorf("NodeExpandVolume  - BuildCommonService volume ID: %s - error: %s", volumeId, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	storageNode, err := storage.NewStorageNode(comnserv, config, req.GetSecrets())
	if err != nil {
		e := fmt.Errorf("NodeExpandVolume - NewStorageNode volume ID: %s - error: %s", volumeId, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	resp, err := storageNode.NodeExpandVolume(context.Background(), req)
	if err != nil {
		e := fmt.Errorf("NodeExpandVolume - sn.NodeExpandVolume volume ID: %s - error: %s", volumeId, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	zlog.Info().Msgf("NodeExpandVolume Finished - volume ID: '%s'", volumeId)
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
