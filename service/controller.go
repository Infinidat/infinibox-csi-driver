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
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/infinidat/infinibox-csi-driver/api"
	"github.com/infinidat/infinibox-csi-driver/api/clientgo"
	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/infinidat/infinibox-csi-driver/helper"
	"github.com/infinidat/infinibox-csi-driver/iboxapi"
	"github.com/infinidat/infinibox-csi-driver/storage"
	storagecommon "github.com/infinidat/infinibox-csi-driver/storage/common"
	"github.com/infinidat/infinibox-csi-driver/storage/nvme"

	"github.com/container-storage-interface/spec/lib/go/csi"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ControllerServer struct {
	Driver *Driver
	csi.UnimplementedControllerServer
	csi.UnimplementedGroupControllerServer
}

func (s *ControllerServer) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (createVolResp *csi.CreateVolumeResponse, err error) {
	slog.Info("Start", "volume", req.GetName())

	volName := req.GetName()
	if volName == "" {
		msg := "volume name empty"
		slog.Error(msg)
		return nil, status.Error(codes.InvalidArgument, msg)
	}

	reqParameters := req.GetParameters()
	if len(reqParameters) == 0 {
		msg := "GetParameters empty"
		slog.Error(msg)
		return nil, status.Error(codes.InvalidArgument, msg)
	}

	storageProtocol, err := determineStorageProtocol(ctx, reqParameters)
	if err != nil {
		e := common.Errorf("determineStorageProtocol error %w", err)
		slog.Error(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	slog.Debug("info", "capacity-range", req.GetCapacityRange(), "params", reqParameters)
	slog.Debug("info", "volume name", volName, "node id", s.Driver.nodeID, "protocol", storageProtocol)

	// Basic CSI parameter checking across protocols
	var summary string
	summary, err = validateCapabilities(req.GetVolumeCapabilities())
	if err != nil {
		e := common.Errorf("validateCapabilities - error %w summary %s", err, summary)
		slog.Error(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	configparams := map[string]string{
		"nodeid":                                s.Driver.nodeID,
		"driverversion":                         s.Driver.version,
		common.StorageClassNFSExportPermissions: reqParameters[common.StorageClassNFSExportPermissions],
	}

	secretsToUse, err := handlePVCAnnotations(ctx, req)
	if err != nil {
		e := common.Errorf("handlePVCAnnotation error %w", err)
		slog.Error(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	capacity, err := determineCapacity(req)
	if err != nil {
		e := common.Errorf("%w", err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	err = validateSecret("", common.CSIProvisionerSecretName, common.CSIProvisionerSecretNamespace, req.GetSecrets())
	if err != nil {
		e := common.Errorf("%w", err)
		slog.Error(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	err = validateStorageClassSecretParameters(ctx, reqParameters)
	if err != nil {
		e := common.Errorf("validateStorageClassSecretParameters error %w", err)
		slog.Error(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	volumeInfo := api.VolumeProtocolConfig{
		StorageType: storageProtocol,
	}
	storageController, commonService, err := storage.NewStorageController(configparams, secretsToUse, &volumeInfo, capacity)
	if err != nil || storageController == nil {
		e := common.Errorf("NewStorageController - name %s error %w", volName, err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	err = validateCommonStorageClassParameters(ctx, commonService, reqParameters, storageProtocol)
	if err != nil {
		e := common.Errorf("validateCommonStorageClassParameters - name %s error %w", volName, err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	// perform protocol specific StorageClass validations
	err = storageController.ValidateStorageClass(reqParameters)
	if err != nil {
		e := common.Errorf("ValidateStorageClass - name %s error %w", volName, err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	createVolResp, err = storageController.CreateVolume(ctx, req)
	if err != nil {
		e := common.Errorf("sc.CreateVolume - error %w", err)
		slog.Error(e.Error())
		// it's important to return the original error, because it matches K8s expectations
		return nil, err
	}
	if createVolResp == nil || createVolResp.Volume == nil || createVolResp.Volume.VolumeId == "" {
		e := common.Errorf("sc.CreateVolume response is nil")
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	volumeIDString := createVolResp.Volume.VolumeId
	createVolResp.Volume.VolumeId = createVolResp.Volume.VolumeId + "$$" + storageProtocol

	handlePVCAnnotationForMetadata(ctx, commonService, req.Parameters[common.PVCAnnotationVolumeMetadata], volumeIDString)

	err = handleSCReplica(ctx, commonService, volName, storageProtocol, req.Parameters)
	if err != nil {
		// default for now is to leave with an error if the replica logic fails
		// this will leave the PVC in pending state
		e := common.Errorf("%w", err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	helper.EventCreatedVolumes++

	return createVolResp, nil
}

// DeleteVolume method delete the volume
func (s *ControllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (deleteVolResp *csi.DeleteVolumeResponse, err error) {
	volumeID := req.GetVolumeId()

	slog.Info("Start", "volume id", volumeID)

	volumeInfo, err := storagecommon.ValidateVolumeID(volumeID)
	if err != nil {
		e := common.Errorf("ValidateVolumeID - volume ID: %s error: %w", volumeID, err)
		slog.Error(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	secretsToUse := req.GetSecrets()

	// see if the pvc annotation was specified in the original PVC
	kubernetesClient, err := clientgo.BuildClient()
	if err != nil {
		e := common.Errorf("BuildClient - error %w", err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	pvList, err := kubernetesClient.GetAllPersistentVolumes(ctx)
	if err != nil {
		e := common.Errorf("GetAllPersistentVolumes - error %w", err)
		slog.Error(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}
	for _, persistentVolume := range pvList.Items {
		// we match the PV using the volumeHandle (aka volumeId from above)
		if persistentVolume.Spec.CSI.VolumeHandle == volumeID {
			secretRef := persistentVolume.Spec.CSI.ControllerPublishSecretRef
			if secretRef == nil || secretRef.Name == "" {
				// No per-volume secret set on this PV. Fall back to request secrets.
				break
			}
			annoPVCSecretName := secretRef.Name
			annoPVCSecret, err := kubernetesClient.GetSecret(ctx, annoPVCSecretName, os.Getenv("POD_NAMESPACE"))
			if err != nil {
				e := common.Errorf("GetSecret - volume ID: %s anno %s error %w", volumeID, annoPVCSecretName, err)
				slog.Error(e.Error())
				return nil, status.Error(codes.InvalidArgument, e.Error())
			}
			slog.Debug("volume found, using secret", "volume id", volumeID, "pvc anno name", annoPVCSecretName)
			secretsToUse = annoPVCSecret
			break
		}
	}

	config := map[string]string{
		"nodeid": s.Driver.nodeID,
	}

	storageController, _, err := storage.NewStorageController(config, secretsToUse, &volumeInfo, 0)
	if err != nil || storageController == nil {
		e := common.Errorf("NewStorageController - volume ID: %s error: %w", volumeID, err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	deleteVolResp, err = storageController.DeleteVolume(ctx, req)
	if err != nil {
		e := common.Errorf("sc.DeleteVolume volume ID: %s error: %w", volumeID, err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	return deleteVolResp, nil
}

// ControllerModifyVolume method
func (s *ControllerServer) ControllerModifyVolume(ctx context.Context, req *csi.ControllerModifyVolumeRequest) (publishVolResp *csi.ControllerModifyVolumeResponse, err error) {
	slog.Info("ControllerModifyVolume is not implemented")
	return publishVolResp, nil
}

// ControllerPublishVolume method
func (s *ControllerServer) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (publishVolResp *csi.ControllerPublishVolumeResponse, err error) {
	slog.Info("info", "volume ID", req.GetVolumeId(), "node id", req.GetNodeId())

	if req.VolumeCapability == nil {
		msg := "request VolumeCapability was nil"
		slog.Error(msg)
		return nil, status.Error(codes.InvalidArgument, msg)
	}

	caps := []*csi.VolumeCapability{
		req.VolumeCapability,
	}

	_, err = validateCapabilities(caps)
	if err != nil {
		e := common.Errorf("validateCapabilities - error %w, node ID %s, volume cap %v", err, req.GetNodeId(), req.VolumeCapability)
		slog.Error(e.Error())
		return nil, status.Error(codes.FailedPrecondition, e.Error())
	}

	if req.GetVolumeId() == "" {
		msg := "request volumeId was empty"
		slog.Error(msg)
		return nil, status.Error(codes.InvalidArgument, msg)
	}

	volumeInfo, err := storagecommon.ValidateVolumeID(req.GetVolumeId())
	if err != nil {
		e := common.Errorf("ValidateVolumeID - volume ID: %s error: %w", req.GetVolumeId(), err)
		slog.Error(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	nodeProtocol, err := getNodeProtocol(ctx, req.GetNodeId())
	if err != nil {
		e := common.Errorf("ValidateVolumeID - volume ID: %s error: %w", req.GetVolumeId(), err)
		slog.Error(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	if nodeProtocol != "" {
		slog.Debug("node protocol found on node, using it instead of", "node protocol", nodeProtocol, "node id", req.GetNodeId(), "storage type", volumeInfo.StorageType)
		volumeInfo.StorageType = nodeProtocol
		protocolSecret, protocolSecretInUse, err := GetProtocolSecret(ctx)
		if err != nil {
			e := common.Errorf("error: could not get protocol secret %w", err)
			slog.Error(e.Error())
			return nil, status.Error(codes.InvalidArgument, e.Error())
		}
		if !protocolSecretInUse {
			msg := "error: protocol secret not in use, but is required when nodeProtocol label is set on node"
			slog.Error(msg)
			return nil, status.Error(codes.InvalidArgument, msg)
		}

		switch nodeProtocol {
		case common.ProtocolISCSI:
			networkSpace := protocolSecret[ProtocolSecretISCSINetworkSpace]
			if networkSpace == "" {
				msg := "error: protocol secret specified ISCSI but network_space is empty"
				slog.Error(msg)
				return nil, status.Error(codes.InvalidArgument, msg)
			}
			req.VolumeContext[common.StorageClassNetworkSpace] = networkSpace
		case common.ProtocolNVME:
			networkSpace := protocolSecret[ProtocolSecretNVMENetworkSpace]
			if networkSpace == "" {
				msg := "error: protocol secret specified NVMe but network_space is empty"
				slog.Error(msg)
				return nil, status.Error(codes.InvalidArgument, msg)
			}
			req.VolumeContext[common.StorageClassNetworkSpace] = networkSpace
		}
	}

	if req.GetNodeId() == "" {
		msg := fmt.Sprintf("volume ID: %s request nodeId was empty", req.VolumeId)
		slog.Error(msg)
		return nil, status.Error(codes.InvalidArgument, msg)
	}

	err = validateNodeID(req.GetNodeId())
	if err != nil {
		e := common.Errorf("validateNodeID - volume ID: %s error: %w", req.GetVolumeId(), err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, err.Error())
	}

	config := make(map[string]string)

	err = validateSecret(req.GetVolumeId(), common.CSIControllerPublishSecretName, common.CSIControllerPublishSecretNamespace, req.GetSecrets())
	if err != nil {
		e := common.Errorf("%w", err)
		slog.Error(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	storageController, _, err := storage.NewStorageController(config, req.GetSecrets(), &volumeInfo, 0)
	if err != nil || storageController == nil {
		e := common.Errorf("NewStorageController - volume ID: %s type %v error: %w", req.GetVolumeId(), volumeInfo, err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	defer func() {
		isLocking := false
		_ = helper.ManageNodeVolumeMutex(isLocking, "ControllerPublishVolume", req.GetVolumeId())
	}()

	isLocking := true
	_ = helper.ManageNodeVolumeMutex(isLocking, "ControllerPublishVolume", req.GetVolumeId())

	publishVolResp, err = storageController.ControllerPublishVolume(ctx, req)
	if err != nil {
		e := common.Errorf("ControllerPublishVolume - failed proto: %v volume ID: %s node ID: %s error: %w", volumeInfo, req.GetVolumeId(), req.GetNodeId(), err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	return publishVolResp, nil
}

// ControllerUnpublishVolume method
func (s *ControllerServer) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (unpublishVolResp *csi.ControllerUnpublishVolumeResponse, err error) {
	slog.Info("Start", "volume ID", req.GetVolumeId(), "node id", req.GetNodeId())

	if req.GetVolumeId() == "" {
		msg := "request volumeId parameter was empty"
		slog.Error(msg)
		return nil, status.Error(codes.InvalidArgument, msg)
	}
	volumeInfo, err := storagecommon.ValidateVolumeID(req.GetVolumeId())
	if err != nil {
		e := common.Errorf("ValidateVolumeID -  volume ID: %s error: %w", req.GetVolumeId(), err)
		slog.Error(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	nodeID := req.GetNodeId()
	if nodeID != "" { // NodeId is optional, when empty we should unpublish the volume from any nodes it is published to
		err = validateNodeID(nodeID)
		if err != nil {
			e := common.Errorf("validateNodeID - node ID: %s error: %w", nodeID, err)
			slog.Error(e.Error())
			return nil, status.Error(codes.InvalidArgument, e.Error())
		}
	}

	config := make(map[string]string)

	nodeProtocol, err := getNodeProtocol(ctx, req.GetNodeId())
	if err != nil {
		e := common.Errorf("volume ID: %s node ID: %s error: %w", req.GetVolumeId(), req.GetNodeId(), err)
		slog.Error(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	if nodeProtocol != "" {
		slog.Debug("node protocol found on node, using it instead of", "node protocol", nodeProtocol, "node id", req.GetNodeId(), "storage type", volumeInfo.StorageType)
		volumeInfo.StorageType = nodeProtocol
	}

	hostName, err := storagecommon.DetermineHostName(req.GetNodeId())
	if err != nil {
		e := common.Errorf("DetermineHostName - volume ID: %s error: %w", req.GetVolumeId(), err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	volumeInfo.NodeID = req.GetNodeId()

	storageController, commonService, err := storage.NewStorageController(config, req.GetSecrets(), &volumeInfo, 0)
	if err != nil {
		e := common.Errorf("NewStorageController - volume ID: %s error: %w", req.GetVolumeId(), err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	if volumeInfo.StorageType != common.ProtocolNFS && volumeInfo.StorageType != common.ProtocolTreeq {
		if volumeInfo.StorageType == common.ProtocolNVME {
			hostName += nvme.NVMEHostSuffix
		}
		volumeInfo.Host, err = commonService.IboxAPI.GetHostByName(ctx, hostName)
		if err != nil {
			if errors.Is(err, iboxapi.ErrNotFound) {
				return &csi.ControllerUnpublishVolumeResponse{}, nil
			}
			e := common.Errorf("GetHostByName - volume ID: %s error: %w", req.GetVolumeId(), err)
			slog.Error(e.Error())
			return nil, status.Error(codes.Internal, e.Error())
		}
	}

	unpublishVolResp, err = storageController.ControllerUnpublishVolume(ctx, req)
	if err != nil {
		e := common.Errorf("sc.ControllerUnpublishVolume - volume ID: %s error: %w", req.GetVolumeId(), err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	return unpublishVolResp, nil
}

func validateCapabilities(capabilities []*csi.VolumeCapability) (summary string, err error) {
	isBlock := false
	isFile := false

	if capabilities == nil {
		return "", errors.New("no volume capabilities specified")
	}
	if len(capabilities) == 0 {
		e := fmt.Errorf("volume capabilities empty")
		slog.Error(e.Error())
		return "", status.Error(codes.InvalidArgument, e.Error())
	}

	var modes string
	for _, capability := range capabilities {
		accessMode := capability.GetAccessMode()
		if accessMode == nil {
			return "", errors.New("no accessmode specified in volume capability")
		}
		mode := accessMode.GetMode()

		if block := capability.GetBlock(); block != nil {
			isBlock = true
			switch mode {
			case csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY,
				csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
				csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER:
			default:
				return "", fmt.Errorf("access mode [%s] is not a supported block access mode", mode)
			}
		}
		if file := capability.GetMount(); file != nil {
			isFile = true
			switch mode {
			case csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY,
				csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
				csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER:
			default:
				return "", fmt.Errorf("access mode [%s] is not a supported file access mode", mode)
			}

			switch file.FsType {
			case "", common.FSTypeExt3, common.FSTypeExt4, common.FSTypeXFS:
			default:
				return "", fmt.Errorf("fstype [%s] is not supported", file.FsType)
			}
		}

		if modes != "" {
			modes += ", "
		}
		modes += fmt.Sprintf("mode: %s", mode.String())
	}

	if isBlock && isFile {
		return "", errors.New("both file and block volume capabilities specified")
	}

	summary += fmt.Sprintf("modes: %s isBlock: %t isFile: %t", modes, isBlock, isFile)

	return summary, nil
}

func (s *ControllerServer) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (validateVolCapsResponse *csi.ValidateVolumeCapabilitiesResponse, err error) {
	slog.Info("Started", "volume ID", req.GetVolumeId())

	if req.GetVolumeId() == "" {
		msg := "error volumeId parameter was empty"
		slog.Error(msg)
		return nil, status.Error(codes.InvalidArgument, msg)
	}
	if req.VolumeCapabilities == nil {
		msg := fmt.Sprintf("volume ID: %s error volumeCapabilities parameter was nil", req.GetVolumeId())
		slog.Error(msg)
		return nil, status.Error(codes.InvalidArgument, msg)
	}
	if len(req.VolumeCapabilities) == 0 {
		msg := fmt.Sprintf("volume ID: %s error volumeCapabilities parameter was empty", req.GetVolumeId())
		slog.Error(msg)
		return nil, status.Error(codes.InvalidArgument, msg)
	}

	volumeInfo, err := storagecommon.ValidateVolumeID(req.GetVolumeId())
	if err != nil {
		e := common.Errorf("ValidateVolumeID - volume ID: %s error: %w", req.GetVolumeId(), err)
		slog.Error(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	config := make(map[string]string)
	commonService, err := storagecommon.BuildCommonService(config, req.GetSecrets(), &volumeInfo)
	if err != nil {
		e := common.Errorf("BuildCommonService - volume ID: %s error: %w", req.GetVolumeId(), err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	scParameters := req.Parameters
	protocol := scParameters[common.StorageClassStorageProtocol]

	//	if protocol != common.PROTOCOL_NFS && protocol != common.PROTOCOL_TREEQ {
	protocolSecretMap, protocolSecretInUse, err := GetProtocolSecret(ctx)
	if err != nil {
		e := common.Errorf("BuildCommonService - error getting protocol secret: %w", err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	if protocolSecretInUse {
		protocol = protocolSecretMap[common.StorageClassStorageProtocol]
	}
	//}

	if protocol == common.ProtocolNFS || protocol == common.ProtocolTreeq {
		var fileSystem *iboxapi.FileSystem
		fileSystem, err = commonService.IboxAPI.GetFileSystemByID(ctx, volumeInfo.VolumeID)
		if err != nil {
			e := common.Errorf("GetFileSystemByID volume ID: %d error: %w", volumeInfo.VolumeID, err)
			slog.Error(e.Error())
			return nil, status.Error(codes.NotFound, e.Error())
		}
		slog.Debug("filesystem", "volume ID", volumeInfo.VolumeID, "fs", fileSystem)
	} else {
		var volume *iboxapi.Volume
		volume, err = commonService.IboxAPI.GetVolume(ctx, volumeInfo.VolumeID)
		if err != nil {
			e := common.Errorf("GetVolume - failed to find volume ID: %d Error: %w", volumeInfo.VolumeID, err)
			slog.Error(e.Error())
			return nil, status.Error(codes.NotFound, e.Error())
		}
		slog.Debug("volume", "ID", volumeInfo.VolumeID, "vol", volume)
	}
	validateVolCapsResponse = &csi.ValidateVolumeCapabilitiesResponse{
		Confirmed: &csi.ValidateVolumeCapabilitiesResponse_Confirmed{
			VolumeCapabilities: req.GetVolumeCapabilities(),
		},
	}

	return validateVolCapsResponse, nil
}

func (s *ControllerServer) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	slog.Info("Started")

	if req.StartingToken == "" || req.StartingToken == "next-token" {
	} else {
		e := fmt.Errorf("error startingToken parameter was incorrect [%s]", req.StartingToken)
		slog.Error(e.Error())
		return nil, status.Error(codes.Aborted, e.Error())
	}

	// Get a k8s go client for in-cluster use
	client, err := clientgo.BuildClient()
	if err != nil {
		e := common.Errorf("BuildClient - error: %w", err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Unavailable, e.Error())
	}

	res, err := ListVolumesImplementation(ctx, client)
	if err != nil {
		e := common.Errorf("ListVolumesImpl - error: %w", err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Unavailable, e.Error())
	}

	return res, nil
}

func (s *ControllerServer) ListSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	slog.Info("Started", "MaxEntries", req.MaxEntries)

	// Get a k8s go client for in-cluster use
	client, err := clientgo.BuildClient()
	if err != nil {
		e := common.Errorf("BuildClient - error: %w", err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Unavailable, e.Error())
	}

	namespace := os.Getenv("POD_NAMESPACE")
	slog.Debug("info", "POD_NAMESPACE", namespace)
	if namespace == "" {
		msg := "env var POD_NAMESPACE was not set, this is a required env var"
		slog.Error(msg)
		return nil, status.Error(codes.Unavailable, msg)
	}

	res, err := ListSnapshotsImplementation(ctx, req, client, namespace)
	if err != nil {
		e := common.Errorf("ListSnapshotsImpl - error: %w", err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Unavailable, e.Error())
	}

	return res, nil
}

func (s *ControllerServer) GetCapacity(ctx context.Context, req *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (s *ControllerServer) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	return &csi.ControllerGetCapabilitiesResponse{
		Capabilities: s.Driver.cscap,
	}, nil
}

func (s *ControllerServer) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (createSnapshotResp *csi.CreateSnapshotResponse, err error) {
	slog.Info("Started", "snapshot name", req.GetName(), "source volume id", req.GetSourceVolumeId())

	volumeInfo, err := storagecommon.ValidateVolumeID(req.GetSourceVolumeId())
	if err != nil {
		e := common.Errorf("ValidateVolumeID - snapshot Name: %s source volume ID: %s failed to validate storage type error %w", req.GetName(), req.GetSourceVolumeId(), err)
		slog.Error(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}
	config := map[string]string{
		"nodeid": s.Driver.nodeID,
	}

	storageController, _, err := storage.NewStorageController(config, req.GetSecrets(), &volumeInfo, 0)
	if err != nil {
		e := common.Errorf("NewStorageController - snapshot name: %s source volume ID: %s error: %w", req.GetName(), req.GetSourceVolumeId(), err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	createSnapshotResp, err = storageController.CreateSnapshot(ctx, req)
	if err != nil {
		e := common.Errorf("sc.CreateSnapshot - snapshot name: %s source volume ID: %s error: %w", req.GetName(), req.GetSourceVolumeId(), err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	helper.EventCreatedSnapshots++

	return createSnapshotResp, nil
}

func (s *ControllerServer) GetSnapshot(ctx context.Context, req *csi.GetSnapshotRequest) (getSnapshotResp *csi.GetSnapshotResponse, err error) {
	slog.Info("GetSnapshot is not implemented")
	return getSnapshotResp, nil
}

func (s *ControllerServer) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (deleteSnapshotResp *csi.DeleteSnapshotResponse, err error) {
	snapshotID := req.GetSnapshotId()
	slog.Info("Start", "snapshot ID", snapshotID)
	volumeInfo, err := storagecommon.ValidateVolumeID(snapshotID)
	if err != nil {
		e := common.Errorf("ValidateVolumeID - snapshot ID: %s invalid, error: %w", snapshotID, err)
		slog.Error(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	config := map[string]string{
		"nodeid": s.Driver.nodeID,
	}

	storageController, _, err := storage.NewStorageController(config, req.GetSecrets(), &volumeInfo, 0)
	if err != nil {
		e := common.Errorf("NewStorageController - snapshot ID: %s error %w", req.GetSnapshotId(), err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	req.SnapshotId = strconv.Itoa(volumeInfo.VolumeID)

	deleteSnapshotResp, err = storageController.DeleteSnapshot(ctx, req)
	if err != nil {
		c := codes.Internal
		re, ok := err.(storagecommon.ImplementationError)
		if ok {
			c = codes.Code(re.Code)
		}
		e := common.Errorf("sc.DeleteSnapshot - snapshot ID: %s error: %w", req.GetSnapshotId(), err)
		slog.Error(e.Error())
		return nil, status.Error(c, e.Error())
	}
	return deleteSnapshotResp, err
}

func (s *ControllerServer) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (expandVolResp *csi.ControllerExpandVolumeResponse, err error) {
	slog.Info("Started", "volume ID", req.GetVolumeId())

	err = validateExpandVolumeRequest(req)
	if err != nil {
		e := common.Errorf("validate - volume ID: %s error: %w", req.GetVolumeId(), err)
		slog.Error(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	configparams := map[string]string{
		"nodeid": s.Driver.nodeID,
	}
	volumeInfo, err := storagecommon.ValidateVolumeID(req.GetVolumeId())
	if err != nil {
		e := common.Errorf("ValidateVolumeID - volume ID: %s error: %w", req.GetVolumeId(), err)
		slog.Error(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	capacity := req.GetCapacityRange().GetRequiredBytes()
	if capacity < storagecommon.GIB {
		capacity = storagecommon.GIB
		slog.Warn("ControllerExpandVolume (nvme) - volume minimum capacity should be greater 1 GB")
	}
	roundUp := true // default to always rounding up
	if roundUp {
		roundUpBytes := helper.RoundUp(capacity)
		if capacity == roundUpBytes {
			slog.Debug("requested bytes equals calculated rounded up bytes", "capacity", capacity, "rounded", roundUpBytes)
		} else {
			slog.Debug("requested bytes will be rounded up to bytes", "capacity", capacity, "roundedup", roundUpBytes)
			capacity = roundUpBytes
		}
	}

	err = validateSecret(req.GetVolumeId(), common.CSIControllerExpandSecretName, common.CSIControllerExpandSecretNamespace, req.GetSecrets())
	if err != nil {
		e := common.Errorf("%w", err)
		slog.Error(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	storageController, _, err := storage.NewStorageController(configparams, req.GetSecrets(), &volumeInfo, capacity)
	if err != nil {
		e := common.Errorf("NewStorageController - volume ID: %s error: %w", req.GetVolumeId(), err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	if storageController != nil {
		req.VolumeId = strconv.Itoa(volumeInfo.VolumeID)
		expandVolResp, err = storageController.ControllerExpandVolume(ctx, req)
		if err != nil {
			e := common.Errorf("sc.ControllerExpandVolume - volume ID: %s error: %w", req.GetVolumeId(), err)
			slog.Error(e.Error())
			return nil, status.Error(codes.Internal, e.Error())
		}
	}

	return expandVolResp, nil
}

func (s *ControllerServer) ControllerGetVolume(_ context.Context, _ *csi.ControllerGetVolumeRequest) (*csi.ControllerGetVolumeResponse, error) {
	// Infinidat does not support ControllerGetVolume
	return nil, status.Error(codes.Unimplemented, "")
}

func validateNodeID(nodeID string) error {
	if nodeID == "" {
		return status.Error(codes.InvalidArgument, "node ID empty")
	}
	nodeSplit := strings.Split(nodeID, "$$")
	if len(nodeSplit) != 2 {
		e := common.Errorf("node ID: %s does not follow '<fqdn>$$<id>' pattern", nodeID)
		slog.Error(e.Error())
		return status.Error(codes.NotFound, e.Error())
	}
	return nil
}

// Controller expand volume request validation
func validateExpandVolumeRequest(req *csi.ControllerExpandVolumeRequest) error {
	if req.GetVolumeId() == "" {
		e := common.Errorf("volume ID cannot be empty")
		slog.Error(e.Error())
		return status.Error(codes.InvalidArgument, e.Error())
	}
	capRange := req.GetCapacityRange()
	if capRange == nil {
		e := common.Errorf("capacityRange cannot be empty")
		slog.Error(e.Error())
		return status.Error(codes.InvalidArgument, e.Error())
	}
	return nil
}

func validateStorageClassSecretParameters(ctx context.Context, reqParameters map[string]string) error {
	pvcName := reqParameters[common.CSIPVCName]
	pvcNamespace := reqParameters[common.CSIPVCNamespace]

	kubernetesClient, err := clientgo.BuildClient()
	if err != nil {
		return fmt.Errorf("BuildClient error %w", err)
	}

	pvc, err := kubernetesClient.GetPVC(ctx, pvcNamespace, pvcName)
	if err != nil {
		return fmt.Errorf("GetPVC - name %s error %w", pvcName, err)
	}

	if pvc.Spec.StorageClassName == nil || *pvc.Spec.StorageClassName == "" {
		return fmt.Errorf("PVC %s has no StorageClassName", pvcName)
	}
	storageClass, err := kubernetesClient.GetStorageClass(ctx, *pvc.Spec.StorageClassName)

	if err != nil {
		return fmt.Errorf("GetStorageClass - name %s error %w", *pvc.Spec.StorageClassName, err)
	}

	requiredParams := []string{
		common.CSIProvisionerSecretName,
		common.CSIProvisionerSecretNamespace,
		common.CSIControllerPublishSecretName,
		common.CSIControllerPublishSecretNamespace,
		common.CSINodeStageSecretName,
		common.CSINodeStageSecretNamespace,
		common.CSINodePublishSecretName,
		common.CSINodePublishSecretNamespace,
		common.CSIControllerExpandSecretName,
		common.CSIControllerExpandSecretNamespace,
		common.CSINodeExpandSecretName,
		common.CSINodeExpandSecretNamespace,
	}
	for _, param := range requiredParams {
		if storageClass.Parameters[param] == "" {
			return fmt.Errorf("required CSI parameter %q is missing - verify your StorageClass "+
				"includes all csi.storage.k8s.io secret parameters", param)
		}
	}
	return nil
}

func validateCommonStorageClassParameters(ctx context.Context, commonService storagecommon.Commonservice, scParameters map[string]string, protocol string) error {
	poolName := scParameters[common.StorageClassPoolName]
	_, err := commonService.IboxAPI.GetPoolByName(ctx, poolName)
	if err != nil {
		return fmt.Errorf("pool %s - %w", poolName, err)
	}

	// skip validation of network space when FC
	if protocol != common.ProtocolFC {
		networkspace := scParameters[common.StorageClassNetworkSpace]
		arrayofNetworkSpaces := strings.Split(networkspace, ",")

		for _, name := range arrayofNetworkSpaces {
			_, err := commonService.IboxAPI.GetNetworkSpaceByName(ctx, name)
			if err != nil {
				return fmt.Errorf("network space %s - %w", name, err)
			}
		}
		// validate network protocol / networkspace compatibility
		if err := storagecommon.ValidateProtocolToNetworkSpace(ctx, protocol, arrayofNetworkSpaces, commonService.IboxAPI); err != nil {
			return err
		}
	}

	// validate optional uid and gid parameters
	if scParameters[common.StorageClassGID] != "" {
		gid_int, err := strconv.Atoi(scParameters[common.StorageClassGID])
		if err != nil || gid_int < -1 {
			return invalidFormatError(common.StorageClassGID, scParameters[common.StorageClassGID])
		}
	}

	if scParameters[common.StorageClassUID] != "" {
		uid_int, err := strconv.Atoi(scParameters[common.StorageClassUID])
		if err != nil || uid_int < -1 {
			return invalidFormatError(common.StorageClassUID, scParameters[common.StorageClassUID])
		}
	}

	if scParameters[common.StorageClassUNIXPermissions] != "" {
		_, err := strconv.ParseUint(scParameters[common.StorageClassUNIXPermissions], 8, 32)
		if err != nil {
			return invalidFormatError(common.StorageClassUNIXPermissions, scParameters[common.StorageClassUNIXPermissions])
		}
	}

	if scParameters[common.StorageClassMaxVolsPerHost] != "" {
		maxVols_int, err := strconv.Atoi(scParameters[common.StorageClassMaxVolsPerHost])
		if err != nil || maxVols_int < -1 {
			return invalidFormatError(common.StorageClassMaxVolsPerHost, scParameters[common.StorageClassMaxVolsPerHost])
		}
	}

	if scParameters[common.StorageClassProvisionType] != "" {
		p := strings.ToUpper(scParameters[common.StorageClassProvisionType])
		if p != common.StorageClassThickProvision && p != common.StorageClassThinProvision {
			return invalidFormatError(common.StorageClassProvisionType, scParameters[common.StorageClassProvisionType])
		}
	}

	if scParameters[common.StorageClassSSDEnabled] != "" {
		_, err := strconv.ParseBool(scParameters[common.StorageClassSSDEnabled])
		if err != nil {
			return invalidFormatError(common.StorageClassSSDEnabled, scParameters[common.StorageClassSSDEnabled])
		}
	}

	return nil
}

func validateSecret(volumeID, secretName, secretNamespace string, secrets map[string]string) error {
	// the storageclass is required to specify various secrets as CSI parameters,this will cause
	// the secret values (hostname, password, username) to be passed down to the various CSI workflow functions
	u := secrets[common.CredentialUsername]
	p := secrets[common.CredentialPassword]
	h := secrets[common.CredentialHostname]
	if u == "" || p == "" || h == "" {
		e := fmt.Errorf("volumeID %s - hostname/username/password secrets are not found and are required - verify your StorageClass has the %s and %s parameters", volumeID, secretName, secretNamespace)
		slog.Error(e.Error())
		return status.Error(codes.InvalidArgument, e.Error())
	}
	return nil
}

// determine on this node the protocol based on node
// configuration, this is used when the user specifies
// a protocol service and sets the protocol to 'auto'
// 'auto' is used to pick at runtime the block storage
// that we might support that being (fc, iscsi, or nvme)
// auto is not for nfs or nfs_treeq
func DetermineProtocol(ctx context.Context) (protocol string, protocolSecret map[string]string, err error) {
	protocolSecret, protocolSecretInUse, err := GetProtocolSecret(ctx)
	if err != nil {
		return "", protocolSecret, fmt.Errorf("error: could not get protocol secret %s", err.Error())
	}
	if !protocolSecretInUse {
		return "", protocolSecret, fmt.Errorf("error: protocol secret not in use")
	}
	preferredOrder := []string{common.ProtocolFC, common.ProtocolNVME, common.ProtocolISCSI}
	userPreferredOrder := protocolSecret[common.StorageClassProtocolSecretAutoOrder]
	if userPreferredOrder != "" {
		preferredOrder = strings.Split(userPreferredOrder, ",")
		slog.Debug("user preferred", "auto order", preferredOrder)
	}

	client, err := clientgo.BuildClient()
	if err != nil {
		e := common.Errorf("BuildClient - error: %w", err)
		slog.Error(e.Error())
		return "", protocolSecret, e
	}

	namespace := os.Getenv("POD_NAMESPACE")
	slog.Debug("env var", "POD_NAMESPACE", namespace)
	if namespace == "" {
		e := common.Errorf("env var POD_NAMESPACE was not set, this is a required env var")
		slog.Error(e.Error())
		return "", protocolSecret, e
	}

	pods, err := client.GetRunningDriverNodePods(ctx, namespace)
	if err != nil {
		e := common.Errorf("GetRunningDriverNodePods - error: %w", err)
		slog.Error(e.Error())
		return "", protocolSecret, e
	}

	slog.Debug("found driver node pods", "node pods", len(pods))

	// we only need to test with a single driver node pod since they are required
	// to be configured the same wrt protocol configurations
	podToTest := pods[0].Name

	command := "cat /sys/class/fc_host/ho*/port_state"
	containerName := "driver"
	fcOutput, fcStderr, err := client.ExecCmdInPod(ctx, podToTest, namespace, command, containerName)
	slog.Debug("fc command", "stdout", fcOutput, "stderr", fcStderr)
	var fcEnabled bool
	if err != nil {
		slog.Debug("fcEnabled set to false due to error", "error", err.Error(), "stderr", fcStderr)
	} else {
		if fcStderr != "" {
			slog.Debug("fcEnabled", "stderr", fcStderr)
		} else {
			fcEnabled = isFC(fcOutput)
		}
	}

	var nvmeEnabled bool
	command = "nvme list"
	nvmeOutput, nvmeStderr, err := client.ExecCmdInPod(ctx, podToTest, namespace, command, containerName)
	slog.Debug("nvme command", "stdout", nvmeOutput, "stderr", nvmeStderr)
	if err != nil {
		slog.Debug("nvmeEnabled set to false due to error", "error", err.Error(), "stderr", nvmeStderr)
	} else {
		if nvmeStderr != "" {
			slog.Debug("nvmeEnabled", "stderr", nvmeStderr)
		} else {
			nvmeEnabled = isNVME(nvmeOutput)
		}
	}

	// command = "cat /etc/iscsi/initiatorname.iscsi"
	command = "pgrep iscsid"
	var iscsiEnabled bool
	iscsiOutput, iscsiStderr, err := client.ExecCmdInPod(ctx, podToTest, namespace, command, containerName)
	slog.Debug("iscsi command", "stdout", iscsiOutput, "stderr", iscsiStderr)
	if err != nil {
		slog.Debug("iscsiEnabled set to false due to error", "error", err.Error(), "stderr", iscsiStderr)
	} else {
		if iscsiStderr != "" {
			slog.Debug("iscsiEnabled", "stderr", iscsiStderr)
		} else {
			iscsiEnabled = isISCSI(iscsiOutput)
		}
	}
	slog.Debug("fc protocol test results", "value", common.ProtocolFC, "enabled", fcEnabled)
	slog.Debug("nvme protocol test results", "value", common.ProtocolNVME, "enabled", nvmeEnabled)
	slog.Debug("iscsi protocol test results", "value", common.ProtocolISCSI, "enabled", iscsiEnabled)

	for _, orderValue := range preferredOrder {
		switch orderValue {
		case common.ProtocolFC:
			if fcEnabled {
				return orderValue, protocolSecret, nil
			}
		case common.ProtocolISCSI:
			if iscsiEnabled {
				return orderValue, protocolSecret, nil
			}
		case common.ProtocolNVME:
			if nvmeEnabled {
				return orderValue, protocolSecret, nil
			}
		}
	}

	// out of ideas? pick FC and cross fingers
	slog.Warn("could not determine protocol based on heuristics, defaulting to FC")
	return common.ProtocolFC, protocolSecret, nil
}

func isFC(output string) bool {
	// read /sys/class/fc_host/host*/port_state and treat Online as usable
	// kubectl exec -it infinidat-csi-driver-node-z5hb6 -c driver -- sh -c "cat /sys/class/fc_host/ho*/port_state"
	// if the word Online is in the output then we assume fc is enabled
	return strings.Contains(output, "Online")
}

func isISCSI(output string) bool {
	// read /etc/iscsi/initiatorname.iscsi
	// pgrep iscsid should return a PID if iscsid is running
	trimmed := strings.TrimSpace(output)
	return trimmed != ""
}

func isNVME(output string) bool {
	return output != ""
}

func getNodeProtocol(ctx context.Context, nodeID string) (string, error) {
	const NODE_PROTOCOL_LABEL = "infinidat.com/node-protocol"

	// in some cases we get a node ID of just the hostname and other times
	// we can get hostname$$12345, we only want to compare nodes based on the hostname value
	nodeSplit := strings.Split(nodeID, "$$")
	fullNodeName := nodeSplit[0]

	// the hostname can be fully qualified, we want to compare only on the first part of the hostname in the openshift case
	nodeSplit = strings.Split(fullNodeName, ".")
	shortNodeName := nodeSplit[0]
	slog.Debug("getNodeProtocol", "nodeID", nodeID, "shortNodeName", shortNodeName, "fullNodeName", fullNodeName)

	kc, err := clientgo.BuildClient()
	if err != nil {
		return "", err
	}
	nodes, err := kc.GetNodes(ctx)
	if err != nil {
		return "", err
	}
	if len(nodes) == 0 {
		return "", fmt.Errorf("zero nodes found, problem getting a node")
	}
	for _, node := range nodes {
		if shortNodeName == node.Name || fullNodeName == node.Name {
			nodeProtocol := node.Labels[NODE_PROTOCOL_LABEL]
			slog.Debug("node was found", "nodeid", nodeID, "short", shortNodeName, "full", fullNodeName, "node name", node.Name, "protocol", nodeProtocol)
			return nodeProtocol, nil
		}
	}

	// no node protocol label was set on this node
	return "", nil
}

func determineStorageProtocol(ctx context.Context, reqParameters map[string]string) (storageProtocol string, err error) {
	storageProtocol = reqParameters[common.StorageClassStorageProtocol]
	networkSpace := reqParameters[common.StorageClassNetworkSpace]
	// if the user doesn't supply a storage_protocol in the StorageClass, then use the protocol secret
	if storageProtocol == "" {
		protocolSecretMap, protocolSecretInUse, err := GetProtocolSecret(ctx)
		if err != nil {
			return "", status.Error(codes.InvalidArgument, err.Error())
		}

		if protocolSecretInUse {
			storageProtocol = protocolSecretMap[common.StorageClassStorageProtocol]
			switch storageProtocol {
			case common.ProtocolAuto:
				calculatedProtocol, _, err := DetermineProtocol(ctx)
				if err != nil {
					return "", status.Error(codes.Internal, err.Error())
				}
				storageProtocol = calculatedProtocol
			case "":
				// assume FC during CreateVolume if protocol is not set
				// this means node labels will be used when mounting this volume
				storageProtocol = common.ProtocolFC
			case common.ProtocolISCSI:
				reqParameters[common.StorageClassUseCHAP] = protocolSecretMap[ProtocolSecretISCSIUseCHAP]
				reqParameters[common.StorageClassNetworkSpace] = protocolSecretMap[ProtocolSecretISCSINetworkSpace]
			case common.ProtocolNVME:
				networkSpace = protocolSecretMap[ProtocolSecretNVMENetworkSpace]
			case common.ProtocolNFS, common.ProtocolTreeq:
				networkSpace = protocolSecretMap[ProtocolSecretNFSNetworkSpace]
				reqParameters[common.StorageClassNFSExportPermissions] = protocolSecretMap[ProtocolSecretNFSExportPermissions]
			default:
			}

			slog.Debug("protocol secrets", "map", protocolSecretMap)
		}
	}
	if storageProtocol == "" {
		e := fmt.Errorf("storage protocol empty")
		slog.Error(e.Error())
		return storageProtocol, e
	}
	if storageProtocol != common.ProtocolFC && len(networkSpace) == 0 {
		e := fmt.Errorf("network space empty")
		slog.Error(e.Error())
		return storageProtocol, e
	}
	return storageProtocol, nil
}

func determineCapacity(req *csi.CreateVolumeRequest) (capacity int64, err error) {
	capacity = req.GetCapacityRange().RequiredBytes

	roundUp := true // default to always rounding up, users can set the StorageClass parameter to false if for some reason they want
	roundUpParameter := req.Parameters[common.StorageClassRoundup]
	if roundUpParameter != "" {
		roundUp, err = strconv.ParseBool(roundUpParameter)
		if err != nil {
			e := fmt.Errorf("parse error %s - error %s", roundUpParameter, err.Error())
			slog.Error(e.Error())
			return 0, e
		}
	}

	if roundUp {
		roundUpBytes := helper.RoundUp(capacity)
		if capacity == roundUpBytes {
			slog.Debug("requested bytes equals calculated rounded up bytes", "capacity", capacity, "rounded", roundUpBytes)
		} else {
			slog.Debug("requested bytes will be rounded up", "capacity", capacity, "rounded", roundUpBytes)
			capacity = roundUpBytes
		}
	}
	return capacity, nil
}

func handlePVCAnnotations(ctx context.Context, req *csi.CreateVolumeRequest) (secretsToUse map[string]string, err error) {
	secretsToUse = req.GetSecrets()
	kubernetesClient, err := clientgo.BuildClient()
	if err != nil {
		e := common.Errorf("BuildClient - error %w", err)
		slog.Error(e.Error())
		return secretsToUse, status.Error(codes.Internal, e.Error())
	}
	pvcAnnotations := make(map[string]string)
	extraMetadataPVCName := req.Parameters["csi.storage.k8s.io/pvc/name"]
	extraMetadataPVCNamespace := req.Parameters["csi.storage.k8s.io/pvc/namespace"]
	if extraMetadataPVCName != "" && extraMetadataPVCNamespace != "" {
		pvcAnnotations, err = kubernetesClient.GetPVCAnnotations(ctx, extraMetadataPVCName, extraMetadataPVCNamespace)
		if err != nil {
			e := common.Errorf("GetPVCAnnotations - name %s error %w", req.GetName(), err)
			slog.Error(e.Error())
			return secretsToUse, status.Error(codes.InvalidArgument, e.Error())
		}
	}

	pvcAnnoSecret := pvcAnnotations[common.PVCAnnotationIBOXSecret]
	if pvcAnnoSecret != "" {
		//override the normal secrets with the ones from the annotation
		secretsToUse, err = kubernetesClient.GetSecret(ctx, pvcAnnoSecret, os.Getenv(common.EnvVarPodNamespace))
		if err != nil {
			e := common.Errorf("GetSecrets - %s error %w", pvcAnnoSecret, err)
			slog.Error(e.Error())
			return secretsToUse, status.Error(codes.InvalidArgument, e.Error())
		}
	}
	req.Parameters[common.PVCAnnotationNetworkSpace] = pvcAnnotations[common.PVCAnnotationNetworkSpace]
	req.Parameters[common.PVCAnnotationPoolName] = pvcAnnotations[common.PVCAnnotationPoolName]

	if pvcAnnotations[common.PVCAnnotationPoolName] != "" {
		slog.Debug("pool name is specified in the PVC, this will be used instead of the pool_name in the StorageClass", "pool", pvcAnnotations[common.PVCAnnotationPoolName])
		req.Parameters[common.StorageClassPoolName] = pvcAnnotations[common.PVCAnnotationPoolName] // overwrite what was in the storageclass if any
	}

	if pvcAnnotations[common.PVCAnnotationNetworkSpace] != "" {
		slog.Debug("network_space is specified in the PVC, this will be used instead of the network_space in the StorageClass", "network space", pvcAnnotations[common.PVCAnnotationNetworkSpace])
		req.Parameters[common.StorageClassNetworkSpace] = pvcAnnotations[common.PVCAnnotationNetworkSpace] // overwrite what was in the storageclass if any
	}

	volumeMetadataAnnotation := pvcAnnotations[common.PVCAnnotationVolumeMetadata]
	if volumeMetadataAnnotation != "" {
		slog.Debug("volume_metadata annotation is specified in the PVC", "volume_metadata", volumeMetadataAnnotation)
		req.Parameters[common.PVCAnnotationVolumeMetadata] = volumeMetadataAnnotation
	}

	return secretsToUse, nil
}

func handlePVCAnnotationForMetadata(ctx context.Context, cs storagecommon.Commonservice, metadataValue string, volumeIDString string) {
	if metadataValue == "" {
		return
	}
	metadata := make(map[string]any)
	metadata[common.PVCAnnotationVolumeMetadata] = metadataValue
	volumeID, err := strconv.Atoi(volumeIDString)
	if err != nil {
		e := common.Errorf("create metadata for PVC annotation error converting volumeID to int %w", err)
		slog.Error(e.Error())
		return
	}
	_, err = cs.IboxAPI.PutMetadata(ctx, volumeID, metadata)
	if err != nil {
		e := common.Errorf("create metadata for PVC annotation response error %w", err)
		slog.Error(e.Error())
		return
	}
	slog.Debug("info", "created pvc annotation for metadata", metadataValue, "volume id", volumeIDString)
}

func invalidFormatError(parameterName string, enteredValue string) error {
	return fmt.Errorf("format error in StorageClass, storage class parameter [%s] appears to not be a valid integer, value entered was %s", parameterName, enteredValue)
}
