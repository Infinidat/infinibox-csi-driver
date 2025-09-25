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
	"infinibox-csi-driver/api"
	"infinibox-csi-driver/api/clientgo"
	"infinibox-csi-driver/common"
	"infinibox-csi-driver/helper"
	"infinibox-csi-driver/iboxapi"
	"infinibox-csi-driver/storage"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	tspb "google.golang.org/protobuf/types/known/timestamppb"

	"infinibox-csi-driver/log"

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
	const FN = "CreateVolume"
	zlog.Info().Msgf("%s Start - Name: %s", FN, req.GetName())

	volName := req.GetName()

	reqParameters := req.GetParameters()
	if len(reqParameters) == 0 {
		e := fmt.Errorf("%s - GetParameters empty ", FN)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	kc, err := clientgo.BuildClient()
	if err != nil {
		e := fmt.Errorf("%s - BuildClient - error %s", FN, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	storageProtocol := reqParameters[common.SC_STORAGE_PROTOCOL]
	networkSpace := reqParameters[common.SC_NETWORK_SPACE]

	// protocol secret only applies to block volumes (fc, nvme, iscsi)
	//if storageProtocol != common.PROTOCOL_NFS && storageProtocol != common.PROTOCOL_TREEQ {
	protocolSecretMap, protocolSecretInUse, err := helper.GetProtocolSecret()
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if protocolSecretInUse {

		storageProtocol = protocolSecretMap[common.SC_STORAGE_PROTOCOL]

		var useChap, nfsExportPerms string
		switch protocolSecretMap[common.SC_STORAGE_PROTOCOL] {
		case common.PROTOCOL_AUTO:
			networkSpace = common.PROTOCOL_AUTO
		case common.PROTOCOL_ISCSI:
			useChap = protocolSecretMap[common.PROTOCOL_ISCSI+"."+common.SC_USE_CHAP]
			networkSpace = protocolSecretMap[common.PROTOCOL_ISCSI+"."+common.SC_NETWORK_SPACE]
		case common.PROTOCOL_NVME:
			networkSpace = protocolSecretMap[common.PROTOCOL_NVME+"."+common.SC_NETWORK_SPACE]
		case common.PROTOCOL_NFS, common.PROTOCOL_TREEQ:
			networkSpace = protocolSecretMap[common.PROTOCOL_NFS+"."+common.SC_NETWORK_SPACE]
			nfsExportPerms = protocolSecretMap[common.PROTOCOL_NFS+"."+common.SC_NFS_EXPORT_PERMISSIONS]
		default:
		}

		zlog.Debug().Msgf("%s - protocol secret %s:%s %s:%s %s:%s %s:%s", FN, common.SC_STORAGE_PROTOCOL, storageProtocol, common.SC_NETWORK_SPACE, networkSpace, common.SC_USE_CHAP, useChap, common.SC_NFS_EXPORT_PERMISSIONS, nfsExportPerms)
		reqParameters[common.SC_NETWORK_SPACE] = networkSpace
		reqParameters[common.SC_USE_CHAP] = useChap
		reqParameters[common.SC_NFS_EXPORT_PERMISSIONS] = nfsExportPerms
	}
	//}

	reqCapabilities := req.GetVolumeCapabilities()

	zlog.Debug().Msgf("%s - capacity-range: %v ", FN, req.GetCapacityRange())
	zlog.Debug().Msgf("%s - params: %v", FN, reqParameters)
	zlog.Debug().Msgf("%s - name: '%s' controller nodeid: '%s' storage_protocol: '%s' capacity-range: %v params: %v",
		FN, volName, s.Driver.nodeID, storageProtocol, req.GetCapacityRange(), reqParameters)

	// Basic CSI parameter checking across protocols

	if len(storageProtocol) == 0 {
		e := fmt.Errorf("%s - storage protocol empty ", FN)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}
	if storageProtocol != common.PROTOCOL_FC && len(networkSpace) == 0 {
		e := fmt.Errorf("%s - network space empty ", FN)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}
	if len(volName) == 0 {
		e := fmt.Errorf("%s - volume name empty ", FN)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}
	if len(reqCapabilities) == 0 {
		e := fmt.Errorf("%s - volume capabilities empty ", FN)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	var summary string
	summary, err = validateCapabilities(reqCapabilities)
	if err != nil {
		e := fmt.Errorf("%s - validateCapabilities - error %s summary %s", FN, err.Error(), summary)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}
	if reqParameters[common.SC_POOL_NAME] == "" {
		e := fmt.Errorf("%s - %s empty", FN, common.SC_POOL_NAME)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}
	// TODO: move non-protocol-specific capacity request validation here too, verifyVolumeSize function etc

	configparams := map[string]string{
		"nodeid":                         s.Driver.nodeID,
		"driverversion":                  s.Driver.version,
		common.SC_NFS_EXPORT_PERMISSIONS: reqParameters[common.SC_NFS_EXPORT_PERMISSIONS],
	}

	pvcAnnotations := make(map[string]string)
	extraMetadataPVCName := req.Parameters["csi.storage.k8s.io/pvc/name"]
	extraMetadataPVCNamespace := req.Parameters["csi.storage.k8s.io/pvc/namespace"]
	if extraMetadataPVCName != "" && extraMetadataPVCNamespace != "" {
		pvcAnnotations, err = kc.GetPVCAnnotations(extraMetadataPVCName, extraMetadataPVCNamespace)
		if err != nil {
			e := fmt.Errorf("%s - GetPVCAnnotations - name %s error %s", FN, req.GetName(), err.Error())
			zlog.Error().Msg(e.Error())
			return nil, status.Error(codes.InvalidArgument, e.Error())
		}
	}
	secretsToUse := req.GetSecrets()

	pvcAnnoSecret := pvcAnnotations[common.PVC_ANNOTATION_IBOX_SECRET]
	if pvcAnnoSecret != "" {
		secretsToUse, err = kc.GetSecret(pvcAnnoSecret, os.Getenv("POD_NAMESPACE"))
		if err != nil {
			e := fmt.Errorf("%s - GetSecret - %s error %s", FN, pvcAnnoSecret, err.Error())
			zlog.Error().Msg(e.Error())
			return nil, status.Error(codes.InvalidArgument, e.Error())
		}
	}

	capacity := req.GetCapacityRange().RequiredBytes

	roundUp := true // default to always rounding up, users can set the StorageClass parameter to false if for some reason they want
	roundUpParameter := req.Parameters[common.SC_ROUND_UP]
	if roundUpParameter != "" {
		roundUp, err = strconv.ParseBool(roundUpParameter)
		if err != nil {
			e := fmt.Errorf("%s - name %s param %s parse %s - error %s", FN, volName, roundUpParameter, common.SC_ROUND_UP, err.Error())
			zlog.Error().Msg(e.Error())
			return nil, status.Error(codes.Internal, e.Error())
		}
	}

	if roundUp {
		roundUpBytes := helper.RoundUp(capacity)
		if capacity == roundUpBytes {
			zlog.Debug().Msgf("%s requested bytes %d equals calculated rounded up %d bytes", FN, capacity, roundUpBytes)
		} else {
			zlog.Debug().Msgf("%s requested bytes %d will be rounded up to %d bytes", FN, capacity, roundUpBytes)
			capacity = roundUpBytes
		}
	}

	err = validateSecret("CreateVolume", "", common.SC_PROVISIONER_SECRET_NAME, common.SC_PROVISIONER_SECRET_NAMESPACE, req.GetSecrets())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	comnserv, err := storage.BuildCommonService(configparams, secretsToUse, nil)
	if err != nil {
		e := fmt.Errorf("%s - BuildCommonService - name %s error %s", FN, volName, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	storageController, err := storage.NewStorageController(comnserv, capacity, storageProtocol, configparams, secretsToUse)
	if err != nil || storageController == nil {
		e := fmt.Errorf("%s - NewStorageController - name %s error %s", FN, volName, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	err = validateCommonStorageClassParameters(comnserv, reqParameters, storageProtocol)
	if err != nil {
		e := fmt.Errorf("%s - validateCommonStorageClassParameters - name %s error %s", FN, volName, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	req.Parameters[common.PVC_ANNOTATION_NETWORK_SPACE] = pvcAnnotations[common.PVC_ANNOTATION_NETWORK_SPACE]
	req.Parameters[common.PVC_ANNOTATION_POOL_NAME] = pvcAnnotations[common.PVC_ANNOTATION_POOL_NAME]

	if pvcAnnotations[common.PVC_ANNOTATION_POOL_NAME] != "" {
		zlog.Debug().Msgf("%s is specified in the PVC, this will be used instead of the pool_name in the StorageClass", pvcAnnotations[common.PVC_ANNOTATION_POOL_NAME])
		req.Parameters[common.SC_POOL_NAME] = pvcAnnotations[common.PVC_ANNOTATION_POOL_NAME] //overwrite what was in the storageclass if any
	}

	if pvcAnnotations[common.PVC_ANNOTATION_NETWORK_SPACE] != "" {
		zlog.Debug().Msgf("network_space %s is specified in the PVC, this will be used instead of the network_space in the StorageClass", pvcAnnotations[common.PVC_ANNOTATION_NETWORK_SPACE])
		reqParameters[common.SC_NETWORK_SPACE] = pvcAnnotations[common.PVC_ANNOTATION_NETWORK_SPACE] //overwrite what was in the storageclass if any
	}

	// perform protocol specific StorageClass validations
	err = storageController.ValidateStorageClass(reqParameters)
	if err != nil {
		e := fmt.Errorf("%s - ValidateStorageClass - name %s error %s", FN, volName, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	createVolResp, err = storageController.CreateVolume(ctx, req)
	if err != nil {
		e := fmt.Errorf("%s - sc.CreateVolume - error %s", FN, err.Error())
		zlog.Error().Msg(e.Error())
		// it's important to return the original error, because it matches K8s expectations
		return nil, err
	} else if createVolResp == nil {
		e := fmt.Errorf("%s - sc.CreateVolume resp nil - %s", FN, volName)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	} else if createVolResp.Volume == nil {
		e := fmt.Errorf("%s - sc.CreateVolume Volume is nil - name %s - resp %v", FN, volName, createVolResp)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	} else if createVolResp.Volume.VolumeId == "" {
		e := fmt.Errorf("%s - sc.CreateVolume Volume ID is empty - name %s - resp %v", FN, volName, createVolResp)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	createVolResp.Volume.VolumeId = createVolResp.Volume.VolumeId + "$$" + storageProtocol

	helper.EventAPIClient = comnserv.Api
	helper.EventIboxAPIClient = comnserv.IboxApi
	helper.EventCreatedVolumes++

	zlog.Info().Msgf("%s Finish - Name: %s volume ID: %s", FN, volName, createVolResp.Volume.VolumeId)
	return createVolResp, nil
}

// DeleteVolume method delete the volumne
func (s *ControllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (deleteVolResp *csi.DeleteVolumeResponse, err error) {
	const FN = "DeleteVolume"
	volumeId := req.GetVolumeId()

	zlog.Info().Msgf("%s Start - volume ID: %s", FN, volumeId)

	volproto, err := storage.ValidateVolumeID(volumeId)
	if err != nil {
		e := fmt.Errorf("%s - ValidateVolumeID - volume ID: %s error: %s", FN, volumeId, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	secretsToUse := req.GetSecrets()

	// see if the pvc annotation was specified in the original PVC
	kc, err := clientgo.BuildClient()
	if err != nil {
		e := fmt.Errorf("%s - BuildClient - error %s", FN, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	pvList, err := kc.GetAllPersistentVolumes()
	if err != nil {
		e := fmt.Errorf("%s - GetAllPersistentVolumes - error %s", FN, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())

	}
	for i := 0; i < len(pvList.Items); i++ {
		pv := pvList.Items[i]
		// we match the PV using the volumeHandle (aka volumeId from above)
		if pv.Spec.CSI.VolumeHandle == volumeId {
			zlog.Debug().Msgf("%s - pv found for volume ID: %s ", FN, volumeId)
			annoPVCSecretName := pv.Spec.CSI.ControllerPublishSecretRef.Name
			annoPVCSecret, err := kc.GetSecret(annoPVCSecretName, os.Getenv("POD_NAMESPACE"))
			if err != nil {
				e := fmt.Errorf("%s - GetSecret - volume ID: %s anno %s error %s", FN, volumeId, annoPVCSecretName, err.Error())
				zlog.Error().Msg(e.Error())
				return nil, status.Error(codes.InvalidArgument, e.Error())
			}
			zlog.Debug().Msgf("%s - volume ID: %s using secret: %s", FN, volumeId, annoPVCSecretName)
			secretsToUse = annoPVCSecret
		}
	}

	config := map[string]string{
		"nodeid": s.Driver.nodeID,
	}

	comnserv, err := storage.BuildCommonService(config, secretsToUse, &volproto)
	if err != nil {
		e := fmt.Errorf("%s - BuildCommonService - volume ID: %s error: %s", FN, volumeId, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	storageController, err := storage.NewStorageController(comnserv, 0, volproto.StorageType, config, secretsToUse)
	if err != nil || storageController == nil {
		e := fmt.Errorf("%s - NewStorageController - volume ID: %s error: %s", FN, volumeId, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	deleteVolResp, err = storageController.DeleteVolume(ctx, req)
	if err != nil {
		e := fmt.Errorf("%s - sc.DeleteVolume volume ID: %s error: %s", FN, volumeId, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	zlog.Info().Msgf("%s Finish - volume ID: %s", FN, volumeId)
	return deleteVolResp, nil
}

// ControllerModifyVolume method
func (s *ControllerServer) ControllerModifyVolume(ctx context.Context, req *csi.ControllerModifyVolumeRequest) (publishVolResp *csi.ControllerModifyVolumeResponse, err error) {
	zlog.Info().Msg("ControllerModifyVolume is not implemented")
	return nil, nil
}

// ControllerPublishVolume method
func (s *ControllerServer) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (publishVolResp *csi.ControllerPublishVolumeResponse, err error) {
	const FN = "ControllerPublishVolume"
	zlog.Info().Msgf("%s volume ID: %s, node ID: %s", FN, req.GetVolumeId(), req.GetNodeId())

	if req.VolumeCapability == nil {
		e := fmt.Errorf("%s - request VolumeCapability was nil", FN)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	caps := []*csi.VolumeCapability{
		req.VolumeCapability,
	}

	_, err = validateCapabilities(caps)
	if err != nil {
		e := fmt.Errorf("%s - validateCapabilities - error %s, node ID %s, volume cap %v", FN, err.Error(), req.GetNodeId(), req.VolumeCapability)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.FailedPrecondition, e.Error())
	}

	if req.GetVolumeId() == "" {
		e := fmt.Errorf("%s  - request volumeId was empty", FN)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	volproto, err := storage.ValidateVolumeID(req.GetVolumeId())
	if err != nil {
		e := fmt.Errorf("%s - ValidateVolumeID - volume ID: %s error: %s", FN, req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	if req.GetNodeId() == "" {
		e := fmt.Errorf("%s - volume ID: %s request nodeId was empty", FN, req.VolumeId)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	err = validateNodeID(req.GetNodeId())
	if err != nil {
		e := fmt.Errorf("%s - validateNodeID - volume ID: %s error: %s", FN, req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, err.Error())
	}

	config := make(map[string]string)

	err = validateSecret("ControllerPublishVolume", req.GetVolumeId(), common.SC_CONTROLLER_PUBLISH_SECRET_NAME, common.SC_CONTROLLER_PUBLISH_SECRET_NAMESPACE, req.GetSecrets())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	comnserv, err := storage.BuildCommonService(config, req.GetSecrets(), &volproto)
	if err != nil {
		e := fmt.Errorf("%s - BuildCommonService - volume ID: %s error: %s", FN, req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	storageController, err := storage.NewStorageController(comnserv, 0, volproto.StorageType, config, req.GetSecrets())
	if err != nil || storageController == nil {
		e := fmt.Errorf("%s - NewStorageController - volume ID: %s type %v error: %s", FN, req.GetVolumeId(), volproto, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	publishVolResp, err = storageController.ControllerPublishVolume(ctx, req)
	if err != nil {
		e := fmt.Errorf("%s - ControllerPublishVolume - failed proto: %v volume ID: %s node ID: %s error: %v", FN, volproto, req.GetVolumeId(), req.GetNodeId(), err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	zlog.Info().Msgf("%s Finish - volume ID: %s", FN, req.GetVolumeId())

	return publishVolResp, nil
}

// ControllerUnpublishVolume method
func (s *ControllerServer) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (unpublishVolResp *csi.ControllerUnpublishVolumeResponse, err error) {
	const FN = "ControllerUnpublishVolume"
	zlog.Info().Msgf("%s Start - volume ID: %s node ID: %s", FN, req.GetVolumeId(), req.GetNodeId())

	if req.GetVolumeId() == "" {
		e := fmt.Errorf("%s - request volumeId parameter was empty", FN)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}
	volproto, err := storage.ValidateVolumeID(req.GetVolumeId())
	if err != nil {
		e := fmt.Errorf("%s - ValidateVolumeID -  volume ID: %s error: %s", FN, req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	nodeID := req.GetNodeId()
	if nodeID != "" { // NodeId is optional, when empty we should unpublish the volume from any nodes it is published to
		err = validateNodeID(nodeID)
		if err != nil {
			e := fmt.Errorf("%s - validateNodeID - node ID: %s error: %s", FN, nodeID, err.Error())
			zlog.Error().Msg(e.Error())
			return nil, status.Error(codes.InvalidArgument, e.Error())
		}
	}

	config := make(map[string]string)

	comnserv, err := storage.BuildCommonService(config, req.GetSecrets(), &volproto)
	if err != nil {
		e := fmt.Errorf("%s - BuildCommonService - volume ID: %s error: %s", FN, req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	hostName, err := storage.DetermineHostName(req.GetNodeId())
	if err != nil {
		e := fmt.Errorf("%s - DetermineHostName - volume ID: %s error: %s", FN, req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	volproto.NodeID = req.GetNodeId()

	if volproto.StorageType != common.PROTOCOL_NFS && volproto.StorageType != common.PROTOCOL_TREEQ {
		if volproto.StorageType == common.PROTOCOL_NVME {
			hostName = hostName + storage.NVME_HOST_SUFFIX
		}
		volproto.Host, err = comnserv.IboxApi.GetHostByName(hostName)
		if err != nil {
			re, ok := err.(*iboxapi.IboxAPIError)
			if ok && re.Code == iboxapi.IBOXAPI_RESOURCE_NOT_FOUND_ERROR {
				return &csi.ControllerUnpublishVolumeResponse{}, nil
			}
			e := fmt.Errorf("%s - GetHostByName - volume ID: %s error: %s", FN, req.GetVolumeId(), err.Error())
			zlog.Error().Msg(e.Error())
			return nil, status.Error(codes.Internal, e.Error())
		}
	}

	storageController, err := storage.NewStorageController(comnserv, 0, volproto.StorageType, config, req.GetSecrets())
	if err != nil {
		e := fmt.Errorf("%s - NewStorageController - volume ID: %s error: %s", FN, req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	unpublishVolResp, err = storageController.ControllerUnpublishVolume(ctx, req)
	if err != nil {
		e := fmt.Errorf("%s - sc.ControllerUnpublishVolume - volume ID: %s error: %s", FN, req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	zlog.Info().Msgf("%s Finish - volume ID: %s", FN, req.GetVolumeId())

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
			case "", common.FS_TYPE_EXT3, common.FS_TYPE_EXT4, common.FS_TYPE_XFS:
			default:
				return "", fmt.Errorf("fstype [%s] is not supported", file.FsType)
			}
		}

		if modes != "" {
			modes = modes + ", "
		}
		modes = modes + fmt.Sprintf("mode: %s", mode.String())

	}

	if isBlock && isFile {
		return "", errors.New("both file and block volume capabilities specified")
	}

	summary = summary + fmt.Sprintf("modes: %s isBlock: %t isFile: %t", modes, isBlock, isFile)

	return summary, nil
}

func (s *ControllerServer) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (validateVolCapsResponse *csi.ValidateVolumeCapabilitiesResponse, err error) {
	const FN = "ValidateVolumeCapabilities"
	zlog.Info().Msgf("%s Started - volume ID: %s", FN, req.GetVolumeId())

	if req.GetVolumeId() == "" {
		e := fmt.Errorf("%s - error volumeId parameter was empty", FN)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}
	if req.VolumeCapabilities == nil {
		e := fmt.Errorf("%s - volume ID: %s error volumeCapabilities parameter was nil", FN, req.GetVolumeId())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}
	if len(req.VolumeCapabilities) == 0 {
		e := fmt.Errorf("%s - volume ID: %s error volumeCapabilities parameter was empty", FN, req.GetVolumeId())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	volproto, err := storage.ValidateVolumeID(req.GetVolumeId())
	if err != nil {
		e := fmt.Errorf("%s - ValidateVolumeID - volume ID: %s error: %s", FN, req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	config := make(map[string]string)
	comnserv, err := storage.BuildCommonService(config, req.GetSecrets(), &volproto)
	if err != nil {
		e := fmt.Errorf("%s - BuildCommonService - volume ID: %s error: %s", FN, req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	scParameters := req.Parameters
	protocol := scParameters[common.SC_STORAGE_PROTOCOL]

	//	if protocol != common.PROTOCOL_NFS && protocol != common.PROTOCOL_TREEQ {
	protocolSecretMap, protocolSecretInUse, err := helper.GetProtocolSecret()
	if err != nil {
		e := fmt.Errorf("%s - BuildCommonService - error getting protocol secret: %s", FN, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	if protocolSecretInUse {
		protocol = protocolSecretMap[common.SC_STORAGE_PROTOCOL]
	}
	//}

	if protocol == common.PROTOCOL_NFS || protocol == common.PROTOCOL_TREEQ {
		var fs *iboxapi.FileSystem
		fs, err = comnserv.IboxApi.GetFileSystemByID(volproto.VolumeID)
		if err != nil {
			e := fmt.Errorf("%s - GetFileSystemByID volume ID: %d error: %s", FN, volproto.VolumeID, err.Error())
			zlog.Error().Msg(e.Error())
			return nil, status.Error(codes.NotFound, e.Error())
		}
		zlog.Debug().Msgf("filesystem volume ID: %d file system details: %v", volproto.VolumeID, fs)
	} else {
		var vol *iboxapi.Volume
		vol, err = comnserv.IboxApi.GetVolume(volproto.VolumeID)
		if err != nil {
			e := fmt.Errorf("%s - GetVolume - failed to find volume ID: %d Error: %v", FN, volproto.VolumeID, err)
			zlog.Error().Msg(e.Error())
			return nil, status.Error(codes.NotFound, e.Error())
		}
		zlog.Debug().Msgf("volume ID: %d volume details: %v", volproto.VolumeID, vol)
	}
	validateVolCapsResponse = &csi.ValidateVolumeCapabilitiesResponse{
		Confirmed: &csi.ValidateVolumeCapabilitiesResponse_Confirmed{
			VolumeCapabilities: req.GetVolumeCapabilities(),
		},
	}

	zlog.Info().Msgf("%s Finished - volume ID: %s", FN, req.GetVolumeId())

	return validateVolCapsResponse, nil
}

func (s *ControllerServer) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	const FN = "ListVolumes"
	zlog.Info().Msgf("%s Started", FN)

	res := &csi.ListVolumesResponse{
		Entries: make([]*csi.ListVolumesResponse_Entry, 0),
	}

	if req.StartingToken == "" || req.StartingToken == "next-token" {
	} else {
		e := fmt.Errorf("%s - error startingToken parameter was incorrect [%s]", FN, req.StartingToken)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Aborted, e.Error())
	}

	// Get a k8s go client for in-cluster use
	cl, err := clientgo.BuildClient()
	if err != nil {
		e := fmt.Errorf("%s  - BuildClient - error: %s", FN, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Unavailable, e.Error())
	}

	// Find PVs managed by this CSI driver
	pvList, err := cl.GetAllPersistentVolumes()
	if err != nil {
		e := fmt.Errorf("%s  - GetAllPersistentVolumes - error: %s", FN, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Unavailable, e.Error())
	}
	zlog.Info().Msgf("pvList count: %d", len(pvList.Items))

	for _, pv := range pvList.Items {
		zlog.Info().Msgf("pv capacity : %#v", pv.Spec.Capacity)
		zlog.Info().Msgf("pv name: %#v", pv.GetName())
		zlog.Info().Msgf("pv anno: %#v", pv.GetAnnotations()["pv.kubernetes.io/provisioned-by"])
		if pv.GetAnnotations()["pv.kubernetes.io/provisioned-by"] == common.SERVICE_NAME {
			var status csi.ListVolumesResponse_VolumeStatus
			status.PublishedNodeIds = append(status.PublishedNodeIds, pv.GetName())
			// TODO Handle csi.ListVolumesResponse_VolumeStatus.VolumeCondition?
			zlog.Info().Msgf("status: %s", status.String())

			var volume csi.Volume

			volume.CapacityBytes = pv.Spec.Capacity.Storage().AsDec().UnscaledBig().Int64()
			volume.VolumeId = pv.GetName()
			volume.VolumeContext = map[string]string{
				common.SC_NETWORK_SPACE:    pv.Spec.CSI.VolumeAttributes[common.SC_NETWORK_SPACE],
				common.SC_POOL_NAME:        pv.Spec.CSI.VolumeAttributes[common.SC_POOL_NAME],
				common.SC_STORAGE_PROTOCOL: pv.Spec.CSI.VolumeAttributes[common.SC_STORAGE_PROTOCOL],
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

	zlog.Info().Msgf("%s Finished", FN)

	return res, nil

}

func (s *ControllerServer) ListSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	const FN = "ListSnapshots"
	zlog.Info().Msgf("%s Started, MaxEntries=%d", FN, req.MaxEntries)

	res := &csi.ListSnapshotsResponse{
		Entries: make([]*csi.ListSnapshotsResponse_Entry, 0),
	}

	// Get a k8s go client for in-cluster use
	cl, err := clientgo.BuildClient()
	if err != nil {
		e := fmt.Errorf("%s  - BuildClient - error: %s", FN, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Unavailable, e.Error())
	}

	ns := os.Getenv("POD_NAMESPACE")
	zlog.Debug().Msgf("POD_NAMESPACE=%s", ns)
	if ns == "" {
		e := fmt.Errorf("%s - env var POD_NAMESPACE was not set, this is a required env var", FN)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Unavailable, e.Error())
	}

	secrets, err := cl.GetSecrets(ns)
	if err != nil {
		e := fmt.Errorf("%s - GetSecrets - error: %s", FN, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Unavailable, e.Error())
	}

	for i := 0; i < len(secrets); i++ {
		x := api.ClientService{
			ConfigMap:  make(map[string]string),
			SecretsMap: secrets[i],
		}

		clientsvc, err := x.NewClient()
		if err != nil {
			e := fmt.Errorf("%s - NewClient - error: %s", FN, err.Error())
			zlog.Error().Msg(e.Error())
			return nil, status.Error(codes.Unavailable, e.Error())
		}

		snapshots, err := clientsvc.Iboxapi.GetAllSnapshots()
		if err != nil {
			e := fmt.Errorf("%s - GetAllSnapshots - error: %s", FN, err.Error())
			zlog.Error().Msg(e.Error())
			return nil, status.Error(codes.Unavailable, e.Error())
		}
		zlog.Debug().Msgf("got back %d snapshots", len(snapshots))

		// handle the optional case where a SnapshotId is passed in the ListSnapshots request
		var volProto api.VolumeProtocolConfig
		var iValue int
		if req.SnapshotId != "" {
			volProto, err = storage.ValidateVolumeID(req.SnapshotId)
			if err != nil {
				e := fmt.Errorf("%s - ValidateVolumeID - error: %s", FN, err.Error())
				zlog.Error().Msg(e.Error())
				return nil, status.Error(codes.InvalidArgument, e.Error())
			}
			iValue = volProto.VolumeID
		}

		for i := 0; i < len(snapshots); i++ {
			cdt := snapshots[i].CreatedAt / 1000
			tt := time.Unix(cdt, 0)
			t := tspb.New(tt)

			var parentName string
			zlog.Trace().Msgf("snapshot datasettype %s", snapshots[i].DatasetType)
			switch snapshots[i].DatasetType {
			case "VOLUME":
				_, err := clientsvc.Iboxapi.GetVolume(snapshots[i].ParentId)
				if err != nil {
					zlog.Error().Msgf("%s - GetVolume - snapshot %s VOLUME parentId %d error %s", FN, snapshots[i].Name, snapshots[i].ParentId, err.Error())
					parentName = "unknown"
				} else {
					parentName = strconv.Itoa(snapshots[i].ParentId)
				}
			case "FILESYSTEM":
				_, err := clientsvc.Iboxapi.GetFileSystemByID(snapshots[i].ParentId)
				if err != nil {
					zlog.Error().Msgf("%s - GetFileSystemByID - snapshot %s FILESYSTEM parentId %d error %s", FN, snapshots[i].Name, snapshots[i].ParentId, err.Error())
					parentName = "unknown"
				} else {
					parentName = strconv.Itoa(snapshots[i].ParentId)
				}
			default:
				zlog.Error().Msgf("%s - snapshot %s unknown dataset type %s parentId %d ", FN, snapshots[i].Name, snapshots[i].DatasetType, snapshots[i].ParentId)
				parentName = "unknown"
			}

			snapshot := &csi.Snapshot{
				SnapshotId:     snapshots[i].Name,
				SourceVolumeId: parentName,
				SizeBytes:      snapshots[i].Size,
				CreationTime:   t,
				ReadyToUse:     true, //always true on the ibox according to Jason.
			}
			entry := csi.ListSnapshotsResponse_Entry{
				Snapshot: snapshot,
			}

			zlog.Trace().Msgf("SourceVolumeId = %s SnapshotId = %s", req.SourceVolumeId, req.SnapshotId)

			if req.SourceVolumeId != "" {
				volProto, err := storage.ValidateVolumeID(req.SourceVolumeId)
				if err != nil {
					e := fmt.Errorf("%s - ValidateVolumeID - error validating sourceVolumeId %s %s", FN, req.SourceVolumeId, err.Error())
					zlog.Error().Msg(e.Error())
					return nil, status.Error(codes.InvalidArgument, e.Error())
				} else {
					zlog.Trace().Msgf("comparing %d to %s %+v\n", volProto.VolumeID, entry.Snapshot.SourceVolumeId, entry.Snapshot)
					if strconv.Itoa(volProto.VolumeID) == entry.Snapshot.SourceVolumeId {
						zlog.Trace().Msgf("matches!")
						entry.Snapshot.SourceVolumeId = req.SourceVolumeId //set the SourceVolumeId sent back to the incoming format xxxx$$nfs
						res.Entries = append(res.Entries, &entry)
					}
				}
			} else if req.SnapshotId != "" {
				zlog.Trace().Msgf("comparing %d to %d", iValue, snapshots[i].ID)
				if iValue == snapshots[i].ID {
					zlog.Trace().Msgf("req.SnapshotID contains %s found matching snapshot with ID %d name %s\n", req.SnapshotId, snapshots[i].ID, snapshots[i].Name)
					entry.Snapshot.SnapshotId = req.SnapshotId
					res.Entries = append(res.Entries, &entry)
				}
			} else {
				res.Entries = append(res.Entries, &entry)
			}

		}
	}

	zlog.Info().Msgf("%s Finished with returned entries count %d", FN, len(res.Entries))

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
	const FN = "CreateSnapshot"
	zlog.Info().Msgf("%s Started - Snapshot Name: %s source volume ID: %s", FN, req.GetName(), req.GetSourceVolumeId())

	volproto, err := storage.ValidateVolumeID(req.GetSourceVolumeId())
	if err != nil {
		e := fmt.Errorf("%s - ValidateVolumeID - snapshot Name: %s source volume ID: %s failed to validate storage type %v", FN, req.GetName(), req.GetSourceVolumeId(), err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}
	config := map[string]string{
		"nodeid": s.Driver.nodeID,
	}

	comnserv, err := storage.BuildCommonService(config, req.GetSecrets(), &volproto)
	if err != nil {
		e := fmt.Errorf("%s - BuildCommonService - snapshot name: %s source volume ID: %s failed to get ibox api error: %v", FN, req.GetName(), req.GetSourceVolumeId(), err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}
	storageController, err := storage.NewStorageController(comnserv, 0, volproto.StorageType, config, req.GetSecrets())
	if err != nil {
		e := fmt.Errorf("%s - NewStorageController - snapshot name: %s source volume ID: %s error: %s", FN, req.GetName(), req.GetSourceVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	createSnapshotResp, err = storageController.CreateSnapshot(ctx, req)
	if err != nil {
		e := fmt.Errorf("%s - sc.CreateSnapshot - snapshot name: %s source volume ID: %s error: %s", FN, req.GetName(), req.GetSourceVolumeId(), err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	helper.EventAPIClient = comnserv.Api
	helper.EventIboxAPIClient = comnserv.IboxApi
	helper.EventCreatedSnapshots++

	return createSnapshotResp, nil
}

func (s *ControllerServer) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (deleteSnapshotResp *csi.DeleteSnapshotResponse, err error) {
	const FN = "DeleteSnapshot"
	snapshotID := req.GetSnapshotId()
	zlog.Info().Msgf("%s Start - snapshot ID:  %s", FN, snapshotID)
	volproto, err := storage.ValidateVolumeID(snapshotID)
	if err != nil {
		e := fmt.Errorf("%s - ValidateVolumeID - snapshot ID: %s invalid, error: %v", FN, snapshotID, err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	config := map[string]string{
		"nodeid": s.Driver.nodeID,
	}
	comnserv, err := storage.BuildCommonService(config, req.GetSecrets(), &volproto)
	if err != nil {
		e := fmt.Errorf("%s - BuildCommonService - snapshot ID: %s error: %v", FN, req.GetSnapshotId(), err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	storageController, err := storage.NewStorageController(comnserv, 0, volproto.StorageType, config, req.GetSecrets())
	if err != nil {
		e := fmt.Errorf("%s - NewStorageController - snapshot ID: %s error %s", FN, req.GetSnapshotId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	req.SnapshotId = strconv.Itoa(volproto.VolumeID)

	deleteSnapshotResp, err = storageController.DeleteSnapshot(ctx, req)
	if err != nil {
		e := fmt.Errorf("%s - sc.DeleteSnapshot - snapshot ID: %s error: %s", FN, req.GetSnapshotId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	zlog.Info().Msgf("%s Finished - snapshot ID:  %s", FN, snapshotID)
	return deleteSnapshotResp, err

}

func (s *ControllerServer) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (expandVolResp *csi.ControllerExpandVolumeResponse, err error) {

	const FN = "ControllerExpandVolume"
	zlog.Info().Msgf("%s Started - volume ID: %s", FN, req.GetVolumeId())

	err = validateExpandVolumeRequest(req)
	if err != nil {
		e := fmt.Errorf("%s - validate - volume ID: %s error: %s", FN, req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	configparams := map[string]string{
		"nodeid": s.Driver.nodeID,
	}
	volproto, err := storage.ValidateVolumeID(req.GetVolumeId())
	if err != nil {
		e := fmt.Errorf("%s - ValidateVolumeID - volume ID: %s error: %s", FN, req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	capacity := int64(req.GetCapacityRange().GetRequiredBytes())

	err = validateSecret("ControllerExpandVolume", req.GetVolumeId(), common.SC_CONTROLLER_EXPAND_SECRET_NAME, common.SC_CONTROLLER_EXPAND_SECRET_NAMESPACE, req.GetSecrets())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	comnserv, err := storage.BuildCommonService(configparams, req.GetSecrets(), &volproto)
	if err != nil {
		e := fmt.Errorf("%s - BuildCommonService - volume ID: %s error: %v", FN, req.GetVolumeId(), err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	storageController, err := storage.NewStorageController(comnserv, capacity, volproto.StorageType, configparams, req.GetSecrets())
	if err != nil {
		e := fmt.Errorf("%s - NewStorageController - volume ID: %s error: %s", FN, req.GetVolumeId(), err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	if storageController != nil {
		req.VolumeId = strconv.Itoa(volproto.VolumeID)
		expandVolResp, err = storageController.ControllerExpandVolume(ctx, req)
		if err != nil {
			e := fmt.Errorf("%s - sc.ControllerExpandVolume - volume ID: %s error: %s", FN, req.GetVolumeId(), err)
			zlog.Error().Msg(e.Error())
			return nil, status.Error(codes.Internal, e.Error())
		}
	}

	zlog.Info().Msgf("%s Finished - volume ID: %s", FN, req.GetVolumeId())

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

func validateCommonStorageClassParameters(comnserv storage.Commonservice, scParameters map[string]string, protocol string) error {
	poolName := scParameters[common.SC_POOL_NAME]
	_, err := comnserv.IboxApi.GetPoolByName(poolName)
	if err != nil {
		return err
	}

	// skip validation of network space when AUTO
	if protocol != common.PROTOCOL_AUTO {
		// skip validation of network space when FC
		if protocol != common.PROTOCOL_FC {
			networkspace := scParameters[common.SC_NETWORK_SPACE]
			arrayofNetworkSpaces := strings.Split(networkspace, ",")

			for _, name := range arrayofNetworkSpaces {
				_, err := comnserv.IboxApi.GetNetworkSpaceByName(name)
				if err != nil {
					zlog.Error().Msgf("network space: %s is not found on the ibox", name)
					return err
				}
			}
			// validate network protocol / networkspace compatability
			if err := storage.ValidateProtocolToNetworkSpace(protocol, arrayofNetworkSpaces, comnserv.IboxApi); err != nil {
				zlog.Err(err)
				return err
			}
		}
	}

	// validate optional uid and gid parameters
	gidProvided := scParameters[common.SC_GID]
	if gidProvided != "" {
		gid_int, err := strconv.Atoi(gidProvided)
		if err != nil || gid_int < -1 {
			return fmt.Errorf("format error in StorageClass, storage class parameter [%s] appears to not be a valid integer, value entered was %s", common.SC_GID, gidProvided)
		}
	}

	uidProvided := scParameters[common.SC_UID]
	if uidProvided != "" {
		uid_int, err := strconv.Atoi(uidProvided)
		if err != nil || uid_int < -1 {
			return fmt.Errorf("format error in StorageClass, storage class parameter [%s] appears to not be a valid integer, value entered was %s", common.SC_UID, uidProvided)
		}
	}

	unixPermissionsProvided := scParameters[common.SC_UNIX_PERMISSIONS]
	if unixPermissionsProvided != "" {
		_, err := strconv.ParseUint(unixPermissionsProvided, 8, 32)
		if err != nil {
			return fmt.Errorf("format error in StorageClass, storage class parameter [%s] appears to not be a valid integer, value entered was %s", common.SC_UNIX_PERMISSIONS, unixPermissionsProvided)
		}
	}

	maxVolsProvided := scParameters[common.SC_MAX_VOLS_PER_HOST]
	if maxVolsProvided != "" {
		maxVols_int, err := strconv.Atoi(maxVolsProvided)
		if err != nil || maxVols_int < -1 {
			return fmt.Errorf("format error in StorageClass, [%s] appears to not be a valid integer, value entered was %s", common.SC_MAX_VOLS_PER_HOST, maxVolsProvided)
		}
	}

	provTypeProvided := scParameters[common.SC_PROVISION_TYPE]
	if provTypeProvided != "" {
		p := strings.ToUpper(provTypeProvided)
		if p != common.SC_THICK_PROVISION_TYPE && p != common.SC_THIN_PROVISION_TYPE {
			return fmt.Errorf("format error in StorageClass, [%s] appears to not be a valid value, value entered was %s", common.SC_PROVISION_TYPE, provTypeProvided)
		}
	}

	ssdEnabledProvided := scParameters[common.SC_SSD_ENABLED]
	if ssdEnabledProvided != "" {
		_, err := strconv.ParseBool(ssdEnabledProvided)
		if err != nil {
			return fmt.Errorf("format error in StorageClass, [%s] appears to not be a valid boolean, value entered was %s", common.SC_SSD_ENABLED, ssdEnabledProvided)
		}
	}

	return nil
}

func validateSecret(functionName, volumeID, secretName, secretNamespace string, secrets map[string]string) error {
	// the storageclass is required to specify various secrets as CSI parameters,this will cause
	// the secret values (hostname, password, username) to be passed down to the various CSI workflow functions
	u := secrets[common.CRED_USERNAME]
	p := secrets[common.CRED_PASSWORD]
	h := secrets[common.CRED_HOSTNAME]
	if u == "" || p == "" || h == "" {
		e := fmt.Errorf("%s - volumeID - %s - hostname/username/password secrets are not found and are required - verify your StorageClass has the %s and %s parameters", functionName, volumeID, secretName, secretNamespace)
		zlog.Error().Msg(e.Error())
		return status.Error(codes.InvalidArgument, e.Error())
	}
	return nil
}
