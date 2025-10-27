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
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/infinidat/infinibox-csi-driver/api"
	"github.com/infinidat/infinibox-csi-driver/api/clientgo"
	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/infinidat/infinibox-csi-driver/helper"
	"github.com/infinidat/infinibox-csi-driver/iboxapi"
	"github.com/infinidat/infinibox-csi-driver/storage"
	storagecommon "github.com/infinidat/infinibox-csi-driver/storage/common"
	"github.com/infinidat/infinibox-csi-driver/storage/nvme"

	"github.com/container-storage-interface/spec/lib/go/csi"
	tspb "google.golang.org/protobuf/types/known/timestamppb"

	"github.com/infinidat/infinibox-csi-driver/log"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ControllerServer controller server setting
type ControllerServer struct {
	Driver *Driver
	csi.UnimplementedControllerServer
	csi.UnimplementedGroupControllerServer
}

var zlog = log.Get() // grab the logger for package use

// CreateVolume method create the volume
func (s *ControllerServer) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (createVolResp *csi.CreateVolumeResponse, err error) {
	const functionName = "CreateVolume"
	zlog.Info().Msgf("%s Start - Name: %s", functionName, req.GetName())

	volName := req.GetName()

	reqParameters := req.GetParameters()
	if len(reqParameters) == 0 {
		e := fmt.Errorf("%s - GetParameters empty ", functionName)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	kubernetesClient, err := clientgo.BuildClient()
	if err != nil {
		e := fmt.Errorf("%s - BuildClient - error %s", functionName, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	storageProtocol := reqParameters[common.StorageClassStorageProtocol]
	networkSpace := reqParameters[common.StorageClassNetworkSpace]

	// if the user supplies a storage_protocol in the StorageClass, then use that instead of the protocol secret

	if storageProtocol == "" {
		protocolSecretMap, protocolSecretInUse, err := helper.GetProtocolSecret()
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}

		if protocolSecretInUse {
			storageProtocol = protocolSecretMap[common.StorageClassStorageProtocol]
			if storageProtocol == common.ProtocolAuto {
				calculatedProtocol, _, err := DetermineProtocol()
				if err != nil {
					return nil, status.Error(codes.Internal, err.Error())
				}
				storageProtocol = calculatedProtocol
			}

			var useCHAP, nfsExportPerms string
			switch storageProtocol {
			case common.ProtocolISCSI:
				useCHAP = protocolSecretMap[common.ProtocolISCSI+"."+common.StorageClassUseCHAP]
				networkSpace = protocolSecretMap[common.ProtocolISCSI+"."+common.StorageClassNetworkSpace]
			case common.ProtocolNVME:
				networkSpace = protocolSecretMap[common.ProtocolNVME+"."+common.StorageClassNetworkSpace]
			case common.ProtocolNFS, common.ProtocolTreeq:
				networkSpace = protocolSecretMap[common.ProtocolNFS+"."+common.StorageClassNetworkSpace]
				nfsExportPerms = protocolSecretMap[common.ProtocolNFS+"."+common.StorageClassNFSExportPermissions]
			default:
			}

			zlog.Debug().Msgf("%s - protocol secret %s:%s %s:%s %s:%s %s:%s", functionName, common.StorageClassStorageProtocol, storageProtocol, common.StorageClassNetworkSpace, networkSpace, common.StorageClassUseCHAP, useCHAP, common.StorageClassNFSExportPermissions, nfsExportPerms)
			reqParameters[common.StorageClassNetworkSpace] = networkSpace
			reqParameters[common.StorageClassUseCHAP] = useCHAP
			reqParameters[common.StorageClassNFSExportPermissions] = nfsExportPerms
		}
	}

	reqCapabilities := req.GetVolumeCapabilities()

	zlog.Debug().Msgf("%s - capacity-range: %v ", functionName, req.GetCapacityRange())
	zlog.Debug().Msgf("%s - params: %v", functionName, reqParameters)
	zlog.Debug().Msgf("%s - name: '%s' controller nodeid: '%s' storage_protocol: '%s' capacity-range: %v params: %v",
		functionName, volName, s.Driver.nodeID, storageProtocol, req.GetCapacityRange(), reqParameters)

	// Basic CSI parameter checking across protocols

	if len(storageProtocol) == 0 {
		e := fmt.Errorf("%s - storage protocol empty ", functionName)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}
	if storageProtocol != common.ProtocolFC && len(networkSpace) == 0 {
		e := fmt.Errorf("%s - network space empty ", functionName)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}
	if len(volName) == 0 {
		e := fmt.Errorf("%s - volume name empty ", functionName)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}
	if len(reqCapabilities) == 0 {
		e := fmt.Errorf("%s - volume capabilities empty ", functionName)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	var summary string
	summary, err = validateCapabilities(reqCapabilities)
	if err != nil {
		e := fmt.Errorf("%s - validateCapabilities - error %s summary %s", functionName, err.Error(), summary)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}
	if reqParameters[common.StorageClassPoolName] == "" {
		e := fmt.Errorf("%s - %s empty", functionName, common.StorageClassPoolName)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}
	// TODO: move non-protocol-specific capacity request validation here too, verifyVolumeSize function etc

	configparams := map[string]string{
		"nodeid":                                s.Driver.nodeID,
		"driverversion":                         s.Driver.version,
		common.StorageClassNFSExportPermissions: reqParameters[common.StorageClassNFSExportPermissions],
	}

	pvcAnnotations := make(map[string]string)
	extraMetadataPVCName := req.Parameters["csi.storage.k8s.io/pvc/name"]
	extraMetadataPVCNamespace := req.Parameters["csi.storage.k8s.io/pvc/namespace"]
	if extraMetadataPVCName != "" && extraMetadataPVCNamespace != "" {
		pvcAnnotations, err = kubernetesClient.GetPVCAnnotations(extraMetadataPVCName, extraMetadataPVCNamespace)
		if err != nil {
			e := fmt.Errorf("%s - GetPVCAnnotations - name %s error %s", functionName, req.GetName(), err.Error())
			zlog.Error().Msg(e.Error())
			return nil, status.Error(codes.InvalidArgument, e.Error())
		}
	}
	secretsToUse := req.GetSecrets()

	pvcAnnoSecret := pvcAnnotations[common.PVCAnnotationIBOXSecret]
	if pvcAnnoSecret != "" {
		secretsToUse, err = kubernetesClient.GetSecret(pvcAnnoSecret, os.Getenv("POD_NAMESPACE"))
		if err != nil {
			e := fmt.Errorf("%s - GetSecret - %s error %s", functionName, pvcAnnoSecret, err.Error())
			zlog.Error().Msg(e.Error())
			return nil, status.Error(codes.InvalidArgument, e.Error())
		}
	}

	capacity := req.GetCapacityRange().RequiredBytes

	roundUp := true // default to always rounding up, users can set the StorageClass parameter to false if for some reason they want
	roundUpParameter := req.Parameters[common.StorageClassRoundup]
	if roundUpParameter != "" {
		roundUp, err = strconv.ParseBool(roundUpParameter)
		if err != nil {
			e := fmt.Errorf("%s - name %s param %s parse %s - error %s", functionName, volName, roundUpParameter, common.StorageClassRoundup, err.Error())
			zlog.Error().Msg(e.Error())
			return nil, status.Error(codes.Internal, e.Error())
		}
	}

	if roundUp {
		roundUpBytes := helper.RoundUp(capacity)
		if capacity == roundUpBytes {
			zlog.Debug().Msgf("%s requested bytes %d equals calculated rounded up %d bytes", functionName, capacity, roundUpBytes)
		} else {
			zlog.Debug().Msgf("%s requested bytes %d will be rounded up to %d bytes", functionName, capacity, roundUpBytes)
			capacity = roundUpBytes
		}
	}

	err = validateSecret("CreateVolume", "", common.CSIProvisionerSecretName, common.CSIProvisionerSecretNamespace, req.GetSecrets())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	commonService, err := storagecommon.BuildCommonService(configparams, secretsToUse, nil)
	if err != nil {
		e := fmt.Errorf("%s - BuildCommonService - name %s error %s", functionName, volName, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	storageController, err := storage.NewStorageController(commonService, capacity, storageProtocol)
	if err != nil || storageController == nil {
		e := fmt.Errorf("%s - NewStorageController - name %s error %s", functionName, volName, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	err = validateCommonStorageClassParameters(commonService, reqParameters, storageProtocol)
	if err != nil {
		e := fmt.Errorf("%s - validateCommonStorageClassParameters - name %s error %s", functionName, volName, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	req.Parameters[common.PVCAnnotationNetworkSpace] = pvcAnnotations[common.PVCAnnotationNetworkSpace]
	req.Parameters[common.PVCAnnotationPoolName] = pvcAnnotations[common.PVCAnnotationPoolName]

	if pvcAnnotations[common.PVCAnnotationPoolName] != "" {
		zlog.Debug().Msgf("%s is specified in the PVC, this will be used instead of the pool_name in the StorageClass", pvcAnnotations[common.PVCAnnotationPoolName])
		req.Parameters[common.StorageClassPoolName] = pvcAnnotations[common.PVCAnnotationPoolName] // overwrite what was in the storageclass if any
	}

	if pvcAnnotations[common.PVCAnnotationNetworkSpace] != "" {
		zlog.Debug().Msgf("network_space %s is specified in the PVC, this will be used instead of the network_space in the StorageClass", pvcAnnotations[common.PVCAnnotationNetworkSpace])
		reqParameters[common.StorageClassNetworkSpace] = pvcAnnotations[common.PVCAnnotationNetworkSpace] // overwrite what was in the storageclass if any
	}

	// perform protocol specific StorageClass validations
	err = storageController.ValidateStorageClass(reqParameters)
	if err != nil {
		e := fmt.Errorf("%s - ValidateStorageClass - name %s error %s", functionName, volName, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	createVolResp, err = storageController.CreateVolume(ctx, req)
	if err != nil {
		e := fmt.Errorf("%s - sc.CreateVolume - error %s", functionName, err.Error())
		zlog.Error().Msg(e.Error())
		// it's important to return the original error, because it matches K8s expectations
		return nil, err
	} else if createVolResp == nil {
		e := fmt.Errorf("%s - sc.CreateVolume resp nil - %s", functionName, volName)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	} else if createVolResp.Volume == nil {
		e := fmt.Errorf("%s - sc.CreateVolume Volume is nil - name %s - resp %v", functionName, volName, createVolResp)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	} else if createVolResp.Volume.VolumeId == "" {
		e := fmt.Errorf("%s - sc.CreateVolume Volume ID is empty - name %s - resp %v", functionName, volName, createVolResp)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	createVolResp.Volume.VolumeId = createVolResp.Volume.VolumeId + "$$" + storageProtocol

	helper.EventAPIClient = commonService.API
	helper.EventIboxAPIClient = commonService.IboxAPI
	helper.EventCreatedVolumes++

	zlog.Info().Msgf("%s Finish - Name: %s volume ID: %s", functionName, volName, createVolResp.Volume.VolumeId)
	return createVolResp, nil
}

// DeleteVolume method delete the volume
func (s *ControllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (deleteVolResp *csi.DeleteVolumeResponse, err error) {
	const functionName = "DeleteVolume"
	volumeID := req.GetVolumeId()

	zlog.Info().Msgf("%s Start - volume ID: %s", functionName, volumeID)

	volproto, err := storagecommon.ValidateVolumeID(volumeID)
	if err != nil {
		e := fmt.Errorf("%s - ValidateVolumeID - volume ID: %s error: %s", functionName, volumeID, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	secretsToUse := req.GetSecrets()

	// see if the pvc annotation was specified in the original PVC
	kubernetesClient, err := clientgo.BuildClient()
	if err != nil {
		e := fmt.Errorf("%s - BuildClient - error %s", functionName, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	pvList, err := kubernetesClient.GetAllPersistentVolumes()
	if err != nil {
		e := fmt.Errorf("%s - GetAllPersistentVolumes - error %s", functionName, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}
	for _, persistentVolume := range pvList.Items {
		// we match the PV using the volumeHandle (aka volumeId from above)
		if persistentVolume.Spec.CSI.VolumeHandle == volumeID {
			zlog.Debug().Msgf("%s - pv found for volume ID: %s ", functionName, volumeID)
			annoPVCSecretName := persistentVolume.Spec.CSI.ControllerPublishSecretRef.Name
			annoPVCSecret, err := kubernetesClient.GetSecret(annoPVCSecretName, os.Getenv("POD_NAMESPACE"))
			if err != nil {
				e := fmt.Errorf("%s - GetSecret - volume ID: %s anno %s error %s", functionName, volumeID, annoPVCSecretName, err.Error())
				zlog.Error().Msg(e.Error())
				return nil, status.Error(codes.InvalidArgument, e.Error())
			}
			zlog.Debug().Msgf("%s - volume ID: %s using secret: %s", functionName, volumeID, annoPVCSecretName)
			secretsToUse = annoPVCSecret
		}
	}

	config := map[string]string{
		"nodeid": s.Driver.nodeID,
	}

	commonService, err := storagecommon.BuildCommonService(config, secretsToUse, &volproto)
	if err != nil {
		e := fmt.Errorf("%s - BuildCommonService - volume ID: %s error: %s", functionName, volumeID, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	storageController, err := storage.NewStorageController(commonService, 0, volproto.StorageType)
	if err != nil || storageController == nil {
		e := fmt.Errorf("%s - NewStorageController - volume ID: %s error: %s", functionName, volumeID, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	deleteVolResp, err = storageController.DeleteVolume(ctx, req)
	if err != nil {
		e := fmt.Errorf("%s - sc.DeleteVolume volume ID: %s error: %s", functionName, volumeID, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	zlog.Info().Msgf("%s Finish - volume ID: %s", functionName, volumeID)
	return deleteVolResp, nil
}

// ControllerModifyVolume method
func (s *ControllerServer) ControllerModifyVolume(ctx context.Context, req *csi.ControllerModifyVolumeRequest) (publishVolResp *csi.ControllerModifyVolumeResponse, err error) {
	zlog.Info().Msg("ControllerModifyVolume is not implemented")
	return publishVolResp, nil
}

// ControllerPublishVolume method
func (s *ControllerServer) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (publishVolResp *csi.ControllerPublishVolumeResponse, err error) {
	const functionName = "ControllerPublishVolume"
	zlog.Info().Msgf("%s volume ID: %s, node ID: %s", functionName, req.GetVolumeId(), req.GetNodeId())

	if req.VolumeCapability == nil {
		e := fmt.Errorf("%s - request VolumeCapability was nil", functionName)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	caps := []*csi.VolumeCapability{
		req.VolumeCapability,
	}

	_, err = validateCapabilities(caps)
	if err != nil {
		e := fmt.Errorf("%s - validateCapabilities - error %s, node ID %s, volume cap %v", functionName, err.Error(), req.GetNodeId(), req.VolumeCapability)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.FailedPrecondition, e.Error())
	}

	if req.GetVolumeId() == "" {
		e := fmt.Errorf("%s  - request volumeId was empty", functionName)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	volproto, err := storagecommon.ValidateVolumeID(req.GetVolumeId())
	if err != nil {
		e := fmt.Errorf("%s - ValidateVolumeID - volume ID: %s error: %s", functionName, req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	if volproto.StorageType == common.ProtocolAuto {
		zlog.Debug().Msgf("%s protocol auto detected", functionName)
		calculatedProtocol, protocolSecret, err := DetermineProtocol()
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		if calculatedProtocol == common.ProtocolISCSI {
			req.VolumeContext[common.StorageClassNetworkSpace] = protocolSecret["iscsi.network_space"]
		}
		if calculatedProtocol == common.ProtocolNVME {
			req.VolumeContext[common.StorageClassNetworkSpace] = protocolSecret["nvme.network_space"]
		}
		// need to determine the protocol based on user defined protocol order
		// need to look up the network_space for this protocol as defined in the protocol secret
		volproto.StorageType = calculatedProtocol
	}

	if req.GetNodeId() == "" {
		e := fmt.Errorf("%s - volume ID: %s request nodeId was empty", functionName, req.VolumeId)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	err = validateNodeID(req.GetNodeId())
	if err != nil {
		e := fmt.Errorf("%s - validateNodeID - volume ID: %s error: %s", functionName, req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, err.Error())
	}

	config := make(map[string]string)

	err = validateSecret("ControllerPublishVolume", req.GetVolumeId(), common.CSIControllerPublishSecretName, common.CSIControllerPublishSecretNamespace, req.GetSecrets())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	commonService, err := storagecommon.BuildCommonService(config, req.GetSecrets(), &volproto)
	if err != nil {
		e := fmt.Errorf("%s - BuildCommonService - volume ID: %s error: %s", functionName, req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	storageController, err := storage.NewStorageController(commonService, 0, volproto.StorageType)
	if err != nil || storageController == nil {
		e := fmt.Errorf("%s - NewStorageController - volume ID: %s type %v error: %s", functionName, req.GetVolumeId(), volproto, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	publishVolResp, err = storageController.ControllerPublishVolume(ctx, req)
	if err != nil {
		e := fmt.Errorf("%s - ControllerPublishVolume - failed proto: %v volume ID: %s node ID: %s error: %v", functionName, volproto, req.GetVolumeId(), req.GetNodeId(), err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	zlog.Info().Msgf("%s Finish - volume ID: %s", functionName, req.GetVolumeId())

	return publishVolResp, nil
}

// ControllerUnpublishVolume method
func (s *ControllerServer) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (unpublishVolResp *csi.ControllerUnpublishVolumeResponse, err error) {
	const functionName = "ControllerUnpublishVolume"
	zlog.Info().Msgf("%s Start - volume ID: %s node ID: %s", functionName, req.GetVolumeId(), req.GetNodeId())

	if req.GetVolumeId() == "" {
		e := fmt.Errorf("%s - request volumeId parameter was empty", functionName)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}
	volproto, err := storagecommon.ValidateVolumeID(req.GetVolumeId())
	if err != nil {
		e := fmt.Errorf("%s - ValidateVolumeID -  volume ID: %s error: %s", functionName, req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	nodeID := req.GetNodeId()
	if nodeID != "" { // NodeId is optional, when empty we should unpublish the volume from any nodes it is published to
		err = validateNodeID(nodeID)
		if err != nil {
			e := fmt.Errorf("%s - validateNodeID - node ID: %s error: %s", functionName, nodeID, err.Error())
			zlog.Error().Msg(e.Error())
			return nil, status.Error(codes.InvalidArgument, e.Error())
		}
	}

	config := make(map[string]string)

	commonService, err := storagecommon.BuildCommonService(config, req.GetSecrets(), &volproto)
	if err != nil {
		e := fmt.Errorf("%s - BuildCommonService - volume ID: %s error: %s", functionName, req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	hostName, err := storagecommon.DetermineHostName(req.GetNodeId())
	if err != nil {
		e := fmt.Errorf("%s - DetermineHostName - volume ID: %s error: %s", functionName, req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	volproto.NodeID = req.GetNodeId()

	if volproto.StorageType != common.ProtocolNFS && volproto.StorageType != common.ProtocolTreeq {
		if volproto.StorageType == common.ProtocolNVME {
			hostName += nvme.NVMEHostSuffix
		}
		volproto.Host, err = commonService.IboxAPI.GetHostByName(hostName)
		if err != nil {
			re, ok := err.(*iboxapi.APIError)
			if ok && re.Code == iboxapi.RESOURCE_NOT_FOUND {
				return &csi.ControllerUnpublishVolumeResponse{}, nil
			}
			e := fmt.Errorf("%s - GetHostByName - volume ID: %s error: %s", functionName, req.GetVolumeId(), err.Error())
			zlog.Error().Msg(e.Error())
			return nil, status.Error(codes.Internal, e.Error())
		}
	}

	storageController, err := storage.NewStorageController(commonService, 0, volproto.StorageType)
	if err != nil {
		e := fmt.Errorf("%s - NewStorageController - volume ID: %s error: %s", functionName, req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	unpublishVolResp, err = storageController.ControllerUnpublishVolume(ctx, req)
	if err != nil {
		e := fmt.Errorf("%s - sc.ControllerUnpublishVolume - volume ID: %s error: %s", functionName, req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	zlog.Info().Msgf("%s Finish - volume ID: %s", functionName, req.GetVolumeId())

	return unpublishVolResp, nil
}

func validateCapabilities(capabilities []*csi.VolumeCapability) (summary string, err error) {
	isBlock := false
	isFile := false

	if capabilities == nil {
		return "", errors.New("no volume capabilities specified")
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
	const functionName = "ValidateVolumeCapabilities"
	zlog.Info().Msgf("%s Started - volume ID: %s", functionName, req.GetVolumeId())

	if req.GetVolumeId() == "" {
		e := fmt.Errorf("%s - error volumeId parameter was empty", functionName)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}
	if req.VolumeCapabilities == nil {
		e := fmt.Errorf("%s - volume ID: %s error volumeCapabilities parameter was nil", functionName, req.GetVolumeId())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}
	if len(req.VolumeCapabilities) == 0 {
		e := fmt.Errorf("%s - volume ID: %s error volumeCapabilities parameter was empty", functionName, req.GetVolumeId())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	volproto, err := storagecommon.ValidateVolumeID(req.GetVolumeId())
	if err != nil {
		e := fmt.Errorf("%s - ValidateVolumeID - volume ID: %s error: %s", functionName, req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	config := make(map[string]string)
	commonService, err := storagecommon.BuildCommonService(config, req.GetSecrets(), &volproto)
	if err != nil {
		e := fmt.Errorf("%s - BuildCommonService - volume ID: %s error: %s", functionName, req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	scParameters := req.Parameters
	protocol := scParameters[common.StorageClassStorageProtocol]

	//	if protocol != common.PROTOCOL_NFS && protocol != common.PROTOCOL_TREEQ {
	protocolSecretMap, protocolSecretInUse, err := helper.GetProtocolSecret()
	if err != nil {
		e := fmt.Errorf("%s - BuildCommonService - error getting protocol secret: %s", functionName, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	if protocolSecretInUse {
		protocol = protocolSecretMap[common.StorageClassStorageProtocol]
	}
	//}

	if protocol == common.ProtocolNFS || protocol == common.ProtocolTreeq {
		var fileSystem *iboxapi.FileSystem
		fileSystem, err = commonService.IboxAPI.GetFileSystemByID(volproto.VolumeID)
		if err != nil {
			e := fmt.Errorf("%s - GetFileSystemByID volume ID: %d error: %s", functionName, volproto.VolumeID, err.Error())
			zlog.Error().Msg(e.Error())
			return nil, status.Error(codes.NotFound, e.Error())
		}
		zlog.Debug().Msgf("filesystem volume ID: %d file system details: %v", volproto.VolumeID, fileSystem)
	} else {
		var volume *iboxapi.Volume
		volume, err = commonService.IboxAPI.GetVolume(volproto.VolumeID)
		if err != nil {
			e := fmt.Errorf("%s - GetVolume - failed to find volume ID: %d Error: %v", functionName, volproto.VolumeID, err)
			zlog.Error().Msg(e.Error())
			return nil, status.Error(codes.NotFound, e.Error())
		}
		zlog.Debug().Msgf("volume ID: %d volume details: %v", volproto.VolumeID, volume)
	}
	validateVolCapsResponse = &csi.ValidateVolumeCapabilitiesResponse{
		Confirmed: &csi.ValidateVolumeCapabilitiesResponse_Confirmed{
			VolumeCapabilities: req.GetVolumeCapabilities(),
		},
	}

	zlog.Info().Msgf("%s Finished - volume ID: %s", functionName, req.GetVolumeId())

	return validateVolCapsResponse, nil
}

func (s *ControllerServer) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	const functionName = "ListVolumes"
	zlog.Info().Msgf("%s Started", functionName)

	res := &csi.ListVolumesResponse{
		Entries: make([]*csi.ListVolumesResponse_Entry, 0),
	}

	if req.StartingToken == "" || req.StartingToken == "next-token" {
	} else {
		e := fmt.Errorf("%s - error startingToken parameter was incorrect [%s]", functionName, req.StartingToken)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Aborted, e.Error())
	}

	// Get a k8s go client for in-cluster use
	client, err := clientgo.BuildClient()
	if err != nil {
		e := fmt.Errorf("%s  - BuildClient - error: %s", functionName, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Unavailable, e.Error())
	}

	// Find PVs managed by this CSI driver
	pvList, err := client.GetAllPersistentVolumes()
	if err != nil {
		e := fmt.Errorf("%s  - GetAllPersistentVolumes - error: %s", functionName, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Unavailable, e.Error())
	}
	zlog.Info().Msgf("pvList count: %d", len(pvList.Items))

	for _, persistentVolume := range pvList.Items {
		zlog.Info().Msgf("pv capacity : %#v", persistentVolume.Spec.Capacity)
		zlog.Info().Msgf("pv name: %#v", persistentVolume.GetName())
		zlog.Info().Msgf("pv anno: %#v", persistentVolume.GetAnnotations()["pv.kubernetes.io/provisioned-by"])
		if persistentVolume.GetAnnotations()["pv.kubernetes.io/provisioned-by"] == common.ServiceName {
			var status csi.ListVolumesResponse_VolumeStatus
			status.PublishedNodeIds = append(status.PublishedNodeIds, persistentVolume.GetName())
			// TODO Handle csi.ListVolumesResponse_VolumeStatus.VolumeCondition?
			zlog.Info().Msgf("status: %s", status.String())

			var volume csi.Volume

			volume.CapacityBytes = persistentVolume.Spec.Capacity.Storage().AsDec().UnscaledBig().Int64()
			volume.VolumeId = persistentVolume.GetName()
			volume.VolumeContext = map[string]string{
				common.StorageClassNetworkSpace:    persistentVolume.Spec.CSI.VolumeAttributes[common.StorageClassNetworkSpace],
				common.StorageClassPoolName:        persistentVolume.Spec.CSI.VolumeAttributes[common.StorageClassPoolName],
				common.StorageClassStorageProtocol: persistentVolume.Spec.CSI.VolumeAttributes[common.StorageClassStorageProtocol],
			}
			volume.ContentSource = nil
			volume.AccessibleTopology = nil

			var entry csi.ListVolumesResponse_Entry
			entry.Volume = &volume
			entry.Status = &status
			zlog.Info().Msgf("entry: %s", entry.String())

			res.Entries = append(res.Entries, &entry)
		}
	}

	zlog.Info().Msgf("%s Finished", functionName)

	return res, nil
}

func (s *ControllerServer) ListSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	const functionName = "ListSnapshots"
	zlog.Info().Msgf("%s Started, MaxEntries=%d", functionName, req.MaxEntries)

	res := &csi.ListSnapshotsResponse{
		Entries: make([]*csi.ListSnapshotsResponse_Entry, 0),
	}

	// Get a k8s go client for in-cluster use
	client, err := clientgo.BuildClient()
	if err != nil {
		e := fmt.Errorf("%s  - BuildClient - error: %s", functionName, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Unavailable, e.Error())
	}

	namespace := os.Getenv("POD_NAMESPACE")
	zlog.Debug().Msgf("POD_NAMESPACE=%s", namespace)
	if namespace == "" {
		e := fmt.Errorf("%s - env var POD_NAMESPACE was not set, this is a required env var", functionName)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Unavailable, e.Error())
	}

	secrets, err := client.GetSecrets(namespace)
	if err != nil {
		e := fmt.Errorf("%s - GetSecrets - error: %s", functionName, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Unavailable, e.Error())
	}

	for _, secret := range secrets {
		client := api.ClientService{
			ConfigMap:  make(map[string]string),
			SecretsMap: secret,
		}

		clientService, err := client.NewClient()
		if err != nil {
			e := fmt.Errorf("%s - NewClient - error: %s", functionName, err.Error())
			zlog.Error().Msg(e.Error())
			return nil, status.Error(codes.Unavailable, e.Error())
		}

		snapshots, err := clientService.IboxAPI.GetAllSnapshots()
		if err != nil {
			e := fmt.Errorf("%s - GetAllSnapshots - error: %s", functionName, err.Error())
			zlog.Error().Msg(e.Error())
			return nil, status.Error(codes.Unavailable, e.Error())
		}
		zlog.Debug().Msgf("got back %d snapshots", len(snapshots))

		// handle the optional case where a SnapshotId is passed in the ListSnapshots request
		var volProto api.VolumeProtocolConfig
		var volumeID int
		if req.SnapshotId != "" {
			volProto, err = storagecommon.ValidateVolumeID(req.SnapshotId)
			if err != nil {
				e := fmt.Errorf("%s - ValidateVolumeID - error: %s", functionName, err.Error())
				zlog.Error().Msg(e.Error())
				return nil, status.Error(codes.InvalidArgument, e.Error())
			}
			volumeID = volProto.VolumeID
		}

		for _, snapshot := range snapshots {
			createdAtValue := snapshot.CreatedAt / 1000
			timeValue := time.Unix(createdAtValue, 0)
			timestampValue := tspb.New(timeValue)

			var parentName string
			zlog.Trace().Msgf("snapshot datasettype %s", snapshot.DatasetType)
			switch snapshot.DatasetType {
			case "VOLUME":
				_, err := clientService.IboxAPI.GetVolume(snapshot.ParentID)
				if err != nil {
					zlog.Error().Msgf("%s - GetVolume - snapshot %s VOLUME parentId %d error %s", functionName, snapshot.Name, snapshot.ParentID, err.Error())
					parentName = UNKNOWN
				} else {
					parentName = strconv.Itoa(snapshot.ParentID)
				}
			case "FILESYSTEM":
				_, err := clientService.IboxAPI.GetFileSystemByID(snapshot.ParentID)
				if err != nil {
					zlog.Error().Msgf("%s - GetFileSystemByID - snapshot %s FILESYSTEM parentId %d error %s", functionName, snapshot.Name, snapshot.ParentID, err.Error())
					parentName = UNKNOWN
				} else {
					parentName = strconv.Itoa(snapshot.ParentID)
				}
			default:
				zlog.Error().Msgf("%s - snapshot %s unknown dataset type %s parentId %d ", functionName, snapshot.Name, snapshot.DatasetType, snapshot.ParentID)
				parentName = UNKNOWN
			}

			newSnapshot := &csi.Snapshot{
				SnapshotId:     snapshot.Name,
				SourceVolumeId: parentName,
				SizeBytes:      snapshot.Size,
				CreationTime:   timestampValue,
				ReadyToUse:     true, // always true on the ibox according to Jason.
			}
			entry := csi.ListSnapshotsResponse_Entry{
				Snapshot: newSnapshot,
			}

			zlog.Trace().Msgf("SourceVolumeId = %s SnapshotId = %s", req.SourceVolumeId, req.SnapshotId)

			if req.SourceVolumeId != "" {
				volProto, err := storagecommon.ValidateVolumeID(req.SourceVolumeId)
				if err != nil {
					e := fmt.Errorf("%s - ValidateVolumeID - error validating sourceVolumeId %s %s", functionName, req.SourceVolumeId, err.Error())
					zlog.Error().Msg(e.Error())
					return nil, status.Error(codes.InvalidArgument, e.Error())
				} else {
					zlog.Trace().Msgf("comparing %d to %s %+v\n", volProto.VolumeID, entry.Snapshot.SourceVolumeId, entry.Snapshot)
					if strconv.Itoa(volProto.VolumeID) == entry.Snapshot.SourceVolumeId {
						zlog.Trace().Msgf("matches!")
						entry.Snapshot.SourceVolumeId = req.SourceVolumeId // set the SourceVolumeId sent back to the incoming format xxxx$$nfs
						res.Entries = append(res.Entries, &entry)
					}
				}
			} else if req.SnapshotId != "" {
				zlog.Trace().Msgf("comparing %d to %d", volumeID, snapshot.ID)
				if volumeID == snapshot.ID {
					zlog.Trace().Msgf("req.SnapshotID contains %s found matching snapshot with ID %d name %s\n", req.SnapshotId, snapshot.ID, snapshot.Name)
					entry.Snapshot.SnapshotId = req.SnapshotId
					res.Entries = append(res.Entries, &entry)
				}
			} else {
				res.Entries = append(res.Entries, &entry)
			}
		}
	}

	zlog.Info().Msgf("%s Finished with returned entries count %d", functionName, len(res.Entries))

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
	const functionName = "CreateSnapshot"
	zlog.Info().Msgf("%s Started - Snapshot Name: %s source volume ID: %s", functionName, req.GetName(), req.GetSourceVolumeId())

	volproto, err := storagecommon.ValidateVolumeID(req.GetSourceVolumeId())
	if err != nil {
		e := fmt.Errorf("%s - ValidateVolumeID - snapshot Name: %s source volume ID: %s failed to validate storage type %v", functionName, req.GetName(), req.GetSourceVolumeId(), err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}
	config := map[string]string{
		"nodeid": s.Driver.nodeID,
	}

	comnserv, err := storagecommon.BuildCommonService(config, req.GetSecrets(), &volproto)
	if err != nil {
		e := fmt.Errorf("%s - BuildCommonService - snapshot name: %s source volume ID: %s failed to get ibox api error: %v", functionName, req.GetName(), req.GetSourceVolumeId(), err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}
	storageController, err := storage.NewStorageController(comnserv, 0, volproto.StorageType)
	if err != nil {
		e := fmt.Errorf("%s - NewStorageController - snapshot name: %s source volume ID: %s error: %s", functionName, req.GetName(), req.GetSourceVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	createSnapshotResp, err = storageController.CreateSnapshot(ctx, req)
	if err != nil {
		e := fmt.Errorf("%s - sc.CreateSnapshot - snapshot name: %s source volume ID: %s error: %s", functionName, req.GetName(), req.GetSourceVolumeId(), err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	helper.EventAPIClient = comnserv.API
	helper.EventIboxAPIClient = comnserv.IboxAPI
	helper.EventCreatedSnapshots++

	return createSnapshotResp, nil
}

func (s *ControllerServer) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (deleteSnapshotResp *csi.DeleteSnapshotResponse, err error) {
	const functionName = "DeleteSnapshot"
	snapshotID := req.GetSnapshotId()
	zlog.Info().Msgf("%s Start - snapshot ID:  %s", functionName, snapshotID)
	volproto, err := storagecommon.ValidateVolumeID(snapshotID)
	if err != nil {
		e := fmt.Errorf("%s - ValidateVolumeID - snapshot ID: %s invalid, error: %v", functionName, snapshotID, err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	config := map[string]string{
		"nodeid": s.Driver.nodeID,
	}
	comnserv, err := storagecommon.BuildCommonService(config, req.GetSecrets(), &volproto)
	if err != nil {
		e := fmt.Errorf("%s - BuildCommonService - snapshot ID: %s error: %v", functionName, req.GetSnapshotId(), err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	storageController, err := storage.NewStorageController(comnserv, 0, volproto.StorageType)
	if err != nil {
		e := fmt.Errorf("%s - NewStorageController - snapshot ID: %s error %s", functionName, req.GetSnapshotId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	req.SnapshotId = strconv.Itoa(volproto.VolumeID)

	deleteSnapshotResp, err = storageController.DeleteSnapshot(ctx, req)
	if err != nil {
		e := fmt.Errorf("%s - sc.DeleteSnapshot - snapshot ID: %s error: %s", functionName, req.GetSnapshotId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	zlog.Info().Msgf("%s Finished - snapshot ID:  %s", functionName, snapshotID)
	return deleteSnapshotResp, err
}

func (s *ControllerServer) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (expandVolResp *csi.ControllerExpandVolumeResponse, err error) {
	const functionName = "ControllerExpandVolume"
	zlog.Info().Msgf("%s Started - volume ID: %s", functionName, req.GetVolumeId())

	err = validateExpandVolumeRequest(req)
	if err != nil {
		e := fmt.Errorf("%s - validate - volume ID: %s error: %s", functionName, req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	configparams := map[string]string{
		"nodeid": s.Driver.nodeID,
	}
	volproto, err := storagecommon.ValidateVolumeID(req.GetVolumeId())
	if err != nil {
		e := fmt.Errorf("%s - ValidateVolumeID - volume ID: %s error: %s", functionName, req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	capacity := req.GetCapacityRange().GetRequiredBytes()

	err = validateSecret("ControllerExpandVolume", req.GetVolumeId(), common.CSIControllerExpandSecretName, common.CSIControllerExpandSecretNamespace, req.GetSecrets())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	comnserv, err := storagecommon.BuildCommonService(configparams, req.GetSecrets(), &volproto)
	if err != nil {
		e := fmt.Errorf("%s - BuildCommonService - volume ID: %s error: %v", functionName, req.GetVolumeId(), err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	storageController, err := storage.NewStorageController(comnserv, capacity, volproto.StorageType)
	if err != nil {
		e := fmt.Errorf("%s - NewStorageController - volume ID: %s error: %s", functionName, req.GetVolumeId(), err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	if storageController != nil {
		req.VolumeId = strconv.Itoa(volproto.VolumeID)
		expandVolResp, err = storageController.ControllerExpandVolume(ctx, req)
		if err != nil {
			e := fmt.Errorf("%s - sc.ControllerExpandVolume - volume ID: %s error: %s", functionName, req.GetVolumeId(), err)
			zlog.Error().Msg(e.Error())
			return nil, status.Error(codes.Internal, e.Error())
		}
	}

	zlog.Info().Msgf("%s Finished - volume ID: %s", functionName, req.GetVolumeId())

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
		return status.Error(codes.NotFound, fmt.Sprintf("node ID: %s does not follow '<fqdn>$$<id>' pattern", nodeID))
	}
	return nil
}

// Controller expand volume request validation
func validateExpandVolumeRequest(req *csi.ControllerExpandVolumeRequest) error {
	if req.GetVolumeId() == "" {
		return status.Error(codes.InvalidArgument, "Volume ID cannot be empty")
	}
	capRange := req.GetCapacityRange()
	if capRange == nil {
		return status.Error(codes.InvalidArgument, "CapacityRange cannot be empty")
	}
	return nil
}

func validateCommonStorageClassParameters(comnserv storagecommon.Commonservice, scParameters map[string]string, protocol string) error {
	poolName := scParameters[common.StorageClassPoolName]
	_, err := comnserv.IboxAPI.GetPoolByName(poolName)
	if err != nil {
		return err
	}

	// skip validation of network space when FC
	if protocol != common.ProtocolFC {
		networkspace := scParameters[common.StorageClassNetworkSpace]
		arrayofNetworkSpaces := strings.Split(networkspace, ",")

		for _, name := range arrayofNetworkSpaces {
			_, err := comnserv.IboxAPI.GetNetworkSpaceByName(name)
			if err != nil {
				zlog.Error().Msgf("network space: %s is not found on the ibox", name)
				return err
			}
		}
		// validate network protocol / networkspace compatibility
		if err := storagecommon.ValidateProtocolToNetworkSpace(protocol, arrayofNetworkSpaces, comnserv.IboxAPI); err != nil {
			zlog.Err(err)
			return err
		}
	}

	// validate optional uid and gid parameters
	gidProvided := scParameters[common.StorageClassGID]
	if gidProvided != "" {
		gid_int, err := strconv.Atoi(gidProvided)
		if err != nil || gid_int < -1 {
			return fmt.Errorf("format error in StorageClass, storage class parameter [%s] appears to not be a valid integer, value entered was %s", common.StorageClassGID, gidProvided)
		}
	}

	uidProvided := scParameters[common.StorageClassUID]
	if uidProvided != "" {
		uid_int, err := strconv.Atoi(uidProvided)
		if err != nil || uid_int < -1 {
			return fmt.Errorf("format error in StorageClass, storage class parameter [%s] appears to not be a valid integer, value entered was %s", common.StorageClassUID, uidProvided)
		}
	}

	unixPermissionsProvided := scParameters[common.StorageClassUNIXPermissions]
	if unixPermissionsProvided != "" {
		_, err := strconv.ParseUint(unixPermissionsProvided, 8, 32)
		if err != nil {
			return fmt.Errorf("format error in StorageClass, storage class parameter [%s] appears to not be a valid integer, value entered was %s", common.StorageClassUNIXPermissions, unixPermissionsProvided)
		}
	}

	maxVolsProvided := scParameters[common.StorageClassMaxVolsPerHost]
	if maxVolsProvided != "" {
		maxVols_int, err := strconv.Atoi(maxVolsProvided)
		if err != nil || maxVols_int < -1 {
			return fmt.Errorf("format error in StorageClass, [%s] appears to not be a valid integer, value entered was %s", common.StorageClassMaxVolsPerHost, maxVolsProvided)
		}
	}

	provTypeProvided := scParameters[common.StorageClassProvisionType]
	if provTypeProvided != "" {
		p := strings.ToUpper(provTypeProvided)
		if p != common.StorageClassThickProvision && p != common.StorageClassThinProvision {
			return fmt.Errorf("format error in StorageClass, [%s] appears to not be a valid value, value entered was %s", common.StorageClassProvisionType, provTypeProvided)
		}
	}

	ssdEnabledProvided := scParameters[common.StorageClassSSDEnabled]
	if ssdEnabledProvided != "" {
		_, err := strconv.ParseBool(ssdEnabledProvided)
		if err != nil {
			return fmt.Errorf("format error in StorageClass, [%s] appears to not be a valid boolean, value entered was %s", common.StorageClassSSDEnabled, ssdEnabledProvided)
		}
	}

	return nil
}

func validateSecret(functionName, volumeID, secretName, secretNamespace string, secrets map[string]string) error {
	// the storageclass is required to specify various secrets as CSI parameters,this will cause
	// the secret values (hostname, password, username) to be passed down to the various CSI workflow functions
	u := secrets[common.CredentialUsername]
	p := secrets[common.CredentialPassword]
	h := secrets[common.CredentialHostname]
	if u == "" || p == "" || h == "" {
		e := fmt.Errorf("%s - volumeID - %s - hostname/username/password secrets are not found and are required - verify your StorageClass has the %s and %s parameters", functionName, volumeID, secretName, secretNamespace)
		zlog.Error().Msg(e.Error())
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
func DetermineProtocol() (protocol string, protocolSecret map[string]string, err error) {
	const functionName = "DetermineProtocol"
	protocolSecret, protocolSecretInUse, err := helper.GetProtocolSecret()
	if err != nil {
		return "", protocolSecret, fmt.Errorf("%s error: could not get protocol secret %s", functionName, err.Error())
	}
	if !protocolSecretInUse {
		return "", protocolSecret, fmt.Errorf("%s error: protocol secret not in use", functionName)
	}
	preferredOrder := []string{common.ProtocolFC, common.ProtocolNVME, common.ProtocolISCSI}
	userPreferredOrder := protocolSecret[common.StorageClassProtocolSecretAutoOrder]
	if userPreferredOrder != "" {
		preferredOrder = strings.Split(userPreferredOrder, ",")
		zlog.Debug().Msgf("%s user preferred auto order %v", functionName, preferredOrder)
	}

	client, err := clientgo.BuildClient()
	if err != nil {
		e := fmt.Errorf("%s  - BuildClient - error: %s", functionName, err.Error())
		zlog.Error().Msg(e.Error())
		return "", protocolSecret, e
	}

	namespace := os.Getenv("POD_NAMESPACE")
	zlog.Debug().Msgf("POD_NAMESPACE=%s", namespace)
	if namespace == "" {
		e := fmt.Errorf("%s - env var POD_NAMESPACE was not set, this is a required env var", functionName)
		zlog.Error().Msg(e.Error())
		return "", protocolSecret, e
	}

	pods, err := client.GetRunningDriverNodePods(namespace)
	if err != nil {
		e := fmt.Errorf("%s - GetRunningDriverNodePods - error: %s", functionName, err.Error())
		zlog.Error().Msg(e.Error())
		return "", protocolSecret, e
	}

	zlog.Debug().Msgf("%s found %d driver node pods", functionName, len(pods))

	// we only need to test with a single driver node pod since they are required
	// to be configured the same wrt protocol configurations
	podToTest := pods[0].Name

	command := "cat /sys/class/fc_host/ho*/port_state"
	containerName := "driver"
	fcOutput, fcStderr, err := client.ExecCmdInPod(podToTest, namespace, command, containerName)
	zlog.Debug().Msgf("%s fc command stdout [%s] stderr [%s]", functionName, fcOutput, fcStderr)
	var fcEnabled bool
	if err != nil {
		zlog.Debug().Msgf("%s fcEnabled set to false due to error %s - stderr %s", functionName, err.Error(), fcStderr)
	} else {
		if fcStderr != "" {
			zlog.Debug().Msgf("%s fcEnabled stderr %s , setting to fcEnabled to false", functionName, fcStderr)
		} else {
			fcEnabled = isFC(fcOutput)
		}
	}

	var nvmeEnabled bool
	command = "nvme list"
	nvmeOutput, nvmeStderr, err := client.ExecCmdInPod(podToTest, namespace, command, containerName)
	zlog.Debug().Msgf("%s nvme command stdout [%s] stderr [%s]", functionName, nvmeOutput, nvmeStderr)
	if err != nil {
		zlog.Debug().Msgf("%s nvmeEnabled set to false due to error %s - stderr %s", functionName, err.Error(), nvmeStderr)
	} else {
		if nvmeStderr != "" {
			zlog.Debug().Msgf("%s nvmeEnabled stderr %s , setting to nvmeEnabled to false", functionName, nvmeStderr)
		} else {
			nvmeEnabled = isNVME(nvmeOutput)
		}
	}

	// command = "cat /etc/iscsi/initiatorname.iscsi"
	command = "pgrep iscsid"
	var iscsiEnabled bool
	iscsiOutput, iscsiStderr, err := client.ExecCmdInPod(podToTest, namespace, command, containerName)
	zlog.Debug().Msgf("%s iscsi command stdout [%s] stderr [%s]", functionName, iscsiOutput, iscsiStderr)
	if err != nil {
		zlog.Debug().Msgf("%s iscsiEnabled set to false due to error %s - stderr %s", functionName, err.Error(), iscsiStderr)
	} else {
		if iscsiStderr != "" {
			zlog.Debug().Msgf("%s iscsiEnabled stderr %s , setting to iscsiEnabled to false", functionName, iscsiStderr)
		} else {
			iscsiEnabled = isISCSI(iscsiOutput)
		}
	}
	zlog.Debug().Msgf("%s protocol test results [%s=%t] [%s=%t] [%s=%t]", functionName, common.ProtocolFC, fcEnabled, common.ProtocolNVME, nvmeEnabled, common.ProtocolISCSI, iscsiEnabled)

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
	zlog.Warn().Msgf("%s could not determine protocol based on heuristics, defaulting to FC", functionName)
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
