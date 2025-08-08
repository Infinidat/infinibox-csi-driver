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
	zlog.Info().Msgf("CreateVolume Start - Name: %s", req.GetName())

	volName := req.GetName()

	reqParameters := req.GetParameters()
	if len(reqParameters) == 0 {
		e := fmt.Errorf("CreateVolume - GetParameters empty ")
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	networkSpace := reqParameters[common.SC_NETWORK_SPACE]
	storageProtocol := reqParameters[common.SC_STORAGE_PROTOCOL]
	reqCapabilities := req.GetVolumeCapabilities()

	zlog.Debug().Msgf("CreateVolume - capacity-range: %v ", req.GetCapacityRange())
	zlog.Debug().Msgf("CreateVolume - params: %v", reqParameters)
	zlog.Debug().Msgf("CreateVolume  - name: '%s' controller nodeid: '%s' storage_protocol: '%s' capacity-range: %v params: %v",
		volName, s.Driver.nodeID, storageProtocol, req.GetCapacityRange(), reqParameters)

	// Basic CSI parameter checking across protocols

	if len(storageProtocol) == 0 {
		e := fmt.Errorf("CreateVolume - storage protocol empty ")
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}
	if storageProtocol != common.PROTOCOL_FC && len(networkSpace) == 0 {
		e := fmt.Errorf("CreateVolume - network space empty ")
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}
	if len(volName) == 0 {
		e := fmt.Errorf("CreateVolume - volume name empty ")
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}
	if len(reqCapabilities) == 0 {
		e := fmt.Errorf("CreateVolume - volume capabilities empty ")
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	var summary string
	summary, err = validateCapabilities(reqCapabilities)
	if err != nil {
		e := fmt.Errorf("CreateVolume - validateCapabilities - error %s", err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}
	if reqParameters[common.SC_POOL_NAME] == "" {
		e := fmt.Errorf("CreateVolume - %s empty", common.SC_POOL_NAME)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}
	// TODO: move non-protocol-specific capacity request validation here too, verifyVolumeSize function etc

	configparams := map[string]string{
		"nodeid":                         s.Driver.nodeID,
		"driverversion":                  s.Driver.version,
		common.SC_NFS_EXPORT_PERMISSIONS: reqParameters[common.SC_NFS_EXPORT_PERMISSIONS],
	}

	kc, err := clientgo.BuildClient()
	if err != nil {
		e := fmt.Errorf("CreateVolume - BuildClient - error %s", err.Error())
		zlog.Error().Msg(e.Error())
		err = status.Error(codes.Internal, e.Error())
		return nil, err
	}

	pvcAnnotations := make(map[string]string)
	extraMetadataPVCName := req.Parameters["csi.storage.k8s.io/pvc/name"]
	extraMetadataPVCNamespace := req.Parameters["csi.storage.k8s.io/pvc/namespace"]
	if extraMetadataPVCName != "" && extraMetadataPVCNamespace != "" {
		pvcAnnotations, err = kc.GetPVCAnnotations(extraMetadataPVCName, extraMetadataPVCNamespace)
		if err != nil {
			e := fmt.Errorf("CreateVolume - GetPVCAnnotations - name %s error %s", req.GetName(), err.Error())
			zlog.Error().Msg(e.Error())
			return nil, status.Error(codes.InvalidArgument, e.Error())
		}
	}
	secretsToUse := req.GetSecrets()

	pvcAnnoSecret := pvcAnnotations[common.PVC_ANNOTATION_IBOX_SECRET]
	if pvcAnnoSecret != "" {
		secretsToUse, err = kc.GetSecret(pvcAnnoSecret, os.Getenv("POD_NAMESPACE"))
		if err != nil {
			e := fmt.Errorf("CreateVolume - GetSecret - %s error %s", pvcAnnoSecret, err.Error())
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
			e := fmt.Errorf("CreateVolume - name %s param %s parse %s - error %s", volName, roundUpParameter, common.SC_ROUND_UP, err.Error())
			zlog.Error().Msg(e.Error())
			err = status.Error(codes.Internal, e.Error())
			return nil, err
		}
	}

	if roundUp {
		roundUpBytes := helper.RoundUp(capacity)
		if capacity == roundUpBytes {
			zlog.Debug().Msgf("CreateVolume requested bytes %d equals calculated rounded up %d bytes", capacity, roundUpBytes)
		} else {
			zlog.Debug().Msgf("CreateVolume requested bytes %d will be rounded up to %d bytes", capacity, roundUpBytes)
			capacity = roundUpBytes
		}
	}

	err = validateSecret("CreateVolume", "", common.SC_PROVISIONER_SECRET_NAME, common.SC_PROVISIONER_SECRET_NAMESPACE, req.GetSecrets())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	comnserv, err := storage.BuildCommonService(configparams, secretsToUse, nil)
	if err != nil {
		e := fmt.Errorf("CreateVolume - BuildCommonService - name %s error %s", volName, err.Error())
		zlog.Error().Msg(e.Error())
		err = status.Error(codes.Internal, e.Error())
		return nil, err
	}

	storageController, err := storage.NewStorageController(comnserv, capacity, storageProtocol, configparams, secretsToUse)
	if err != nil || storageController == nil {
		e := fmt.Errorf("CreateVolume - NewStorageController - name %s error %s", volName, err.Error())
		zlog.Error().Msg(e.Error())
		err = status.Error(codes.Internal, e.Error())
		return nil, err
	}

	err = validateCommonStorageClassParameters(comnserv, req.Parameters)
	if err != nil {
		e := fmt.Errorf("CreateVolume - validateCommonStorageClassParameters - name %s error %s", volName, err.Error())
		zlog.Error().Msg(e.Error())
		err = status.Error(codes.Internal, e.Error())
		return nil, err
	}

	req.Parameters[common.PVC_ANNOTATION_NETWORK_SPACE] = pvcAnnotations[common.PVC_ANNOTATION_NETWORK_SPACE]
	req.Parameters[common.PVC_ANNOTATION_POOL_NAME] = pvcAnnotations[common.PVC_ANNOTATION_POOL_NAME]

	if pvcAnnotations[common.PVC_ANNOTATION_POOL_NAME] != "" {
		zlog.Debug().Msgf("%s is specified in the PVC, this will be used instead of the pool_name in the StorageClass", pvcAnnotations[common.PVC_ANNOTATION_POOL_NAME])
		req.Parameters[common.SC_POOL_NAME] = pvcAnnotations[common.PVC_ANNOTATION_POOL_NAME] //overwrite what was in the storageclass if any
	}

	if pvcAnnotations[common.PVC_ANNOTATION_NETWORK_SPACE] != "" {
		zlog.Debug().Msgf("network_space %s is specified in the PVC, this will be used instead of the network_space in the StorageClass", pvcAnnotations[common.PVC_ANNOTATION_NETWORK_SPACE])
		req.Parameters[common.SC_NETWORK_SPACE] = pvcAnnotations[common.PVC_ANNOTATION_NETWORK_SPACE] //overwrite what was in the storageclass if any
	}

	// perform protocol specific StorageClass validations
	err = storageController.ValidateStorageClass(req.Parameters)
	if err != nil {
		e := fmt.Errorf("CreateVolume - ValidateStorageClass - name %s error %s", volName, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	createVolResp, err = storageController.CreateVolume(ctx, req)
	if err != nil {
		e := fmt.Errorf("CreateVolume - sc.CreateVolume - error %s", err.Error())
		zlog.Error().Msg(e.Error())
		// it's important to return the original error, because it matches K8s expectations
		return nil, err
	} else if createVolResp == nil {
		e := fmt.Errorf("CreateVolume - sc.CreateVolume resp nil - %s", volName)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	} else if createVolResp.Volume == nil {
		e := fmt.Errorf("CreateVolume - sc.CreateVolume Volume is nil - name %s - resp %v", volName, createVolResp)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	} else if createVolResp.Volume.VolumeId == "" {
		e := fmt.Errorf("CreateVolume - sc.CreateVolume Volume ID is empty - name %s - resp %v", volName, createVolResp)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	createVolResp.Volume.VolumeId = createVolResp.Volume.VolumeId + "$$" + storageProtocol

	eventData := make([]iboxapi.EventRequestData, 0)
	protocolData := iboxapi.EventRequestData{
		Name:  common.SC_STORAGE_PROTOCOL,
		Type:  "String",
		Value: storageProtocol,
	}
	eventData = append(eventData, protocolData)

	capacityData := iboxapi.EventRequestData{
		Name:  common.CUSTOM_EVENT_CAPACITY,
		Type:  "String",
		Value: strconv.FormatInt(capacity, 10),
	}
	eventData = append(eventData, capacityData)

	volumeCapsData := iboxapi.EventRequestData{
		Name:  common.CUSTOM_EVENT_VOLUME_CAPS,
		Type:  "String",
		Value: summary,
	}
	eventData = append(eventData, volumeCapsData)

	volumeIDData := iboxapi.EventRequestData{
		Name:  common.CUSTOM_EVENT_VOLUME_ID,
		Type:  "String",
		Value: createVolResp.Volume.VolumeId,
	}
	eventData = append(eventData, volumeIDData)

	volumeNameData := iboxapi.EventRequestData{
		Name:  common.CUSTOM_EVENT_VOLUME_NAME,
		Type:  "String",
		Value: volName,
	}
	eventData = append(eventData, volumeNameData)

	actionData := iboxapi.EventRequestData{
		Name:  common.CUSTOM_EVENT_ACTION,
		Type:  "String",
		Value: "Create Volume",
	}
	eventData = append(eventData, actionData)

	eventDesc := fmt.Sprintf("CSI - Create Volume: id %s name %s", createVolResp.Volume.VolumeId, volName)
	eventErr := helper.CreateEvent(comnserv.Api, comnserv.IboxApi, eventDesc, eventData)
	if eventErr != nil {
		zlog.Error().Msgf("CreateVolume - CreateEvent - error %s", eventErr.Error())
		// only log errors if custom event fails
	} else {
		zlog.Debug().Msgf("CreateEvent - created external event %+v", eventData)
	}

	zlog.Info().Msgf("CreateVolume Finish - Name: %s volume ID: %s", volName, createVolResp.Volume.VolumeId)
	return createVolResp, nil
}

// DeleteVolume method delete the volumne
func (s *ControllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (deleteVolResp *csi.DeleteVolumeResponse, err error) {

	volumeId := req.GetVolumeId()

	zlog.Info().Msgf("DeleteVolume Start - volume ID: %s", volumeId)

	volproto, err := storage.ValidateVolumeID(volumeId)
	if err != nil {
		e := fmt.Errorf("DeleteVolume - ValidateVolumeID - volume ID: %s error: %s", volumeId, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	secretsToUse := req.GetSecrets()

	// see if the pvc annotation was specified in the original PVC
	kc, err := clientgo.BuildClient()
	if err != nil {
		e := fmt.Errorf("DeleteVolume - BuildClient - error %s", err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	pvList, err := kc.GetAllPersistentVolumes()
	if err != nil {
		e := fmt.Errorf("DeleteVolume - GetAllPersistentVolumes - error %s", err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())

	}
	for i := 0; i < len(pvList.Items); i++ {
		pv := pvList.Items[i]
		// we match the PV using the volumeHandle (aka volumeId from above)
		if pv.Spec.CSI.VolumeHandle == volumeId {
			zlog.Debug().Msgf("DeleteVolume - pv found for volume ID: %s ", volumeId)
			annoPVCSecretName := pv.Spec.CSI.ControllerPublishSecretRef.Name
			annoPVCSecret, err := kc.GetSecret(annoPVCSecretName, os.Getenv("POD_NAMESPACE"))
			if err != nil {
				e := fmt.Errorf("DeleteVolume - GetSecret - volume ID: %s anno %s error %s", volumeId, annoPVCSecretName, err.Error())
				zlog.Error().Msg(e.Error())
				return nil, status.Error(codes.InvalidArgument, e.Error())
			}
			zlog.Debug().Msgf("DeleteVolume - volume ID: %s using secret: %s", volumeId, annoPVCSecretName)
			secretsToUse = annoPVCSecret
		}
	}

	config := map[string]string{
		"nodeid": s.Driver.nodeID,
	}

	comnserv, err := storage.BuildCommonService(config, secretsToUse, &volproto)
	if err != nil {
		e := fmt.Errorf("DeleteVolume - BuildCommonService - volume ID: %s error: %s", volumeId, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	storageController, err := storage.NewStorageController(comnserv, 0, volproto.StorageType, config, secretsToUse)
	if err != nil || storageController == nil {
		e := fmt.Errorf("DeleteVolume - NewStorageController - volume ID: %s error: %s", volumeId, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	deleteVolResp, err = storageController.DeleteVolume(ctx, req)
	if err != nil {
		e := fmt.Errorf("DeleteVolume - sc.DeleteVolume volume ID: %s error: %s", volumeId, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	zlog.Info().Msgf("DeleteVolume Finish - volume ID: %s", volumeId)
	return deleteVolResp, nil
}

// ControllerModifyVolume method
func (s *ControllerServer) ControllerModifyVolume(ctx context.Context, req *csi.ControllerModifyVolumeRequest) (publishVolResp *csi.ControllerModifyVolumeResponse, err error) {
	zlog.Info().Msg("ControllerModifyVolume is not implemented")
	return nil, nil
}

// ControllerPublishVolume method
func (s *ControllerServer) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (publishVolResp *csi.ControllerPublishVolumeResponse, err error) {

	zlog.Info().Msgf("ControllerPublishVolume volume ID: %s, node ID: %s", req.GetVolumeId(), req.GetNodeId())

	if req.VolumeCapability == nil {
		e := fmt.Errorf("ControllerPublishVolume - request VolumeCapability was nil")
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	caps := []*csi.VolumeCapability{
		req.VolumeCapability,
	}

	_, err = validateCapabilities(caps)
	if err != nil {
		e := fmt.Errorf("ControllerPublishVolume - validateCapabilities - error %s, node ID %s, volume cap %v", err.Error(), req.GetNodeId(), req.VolumeCapability)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.FailedPrecondition, e.Error())
	}

	if req.GetVolumeId() == "" {
		e := fmt.Errorf("ControllerPublishVolume  - request volumeId was empty")
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	volproto, err := storage.ValidateVolumeID(req.GetVolumeId())
	if err != nil {
		e := fmt.Errorf("ControllerPublishVolume - ValidateVolumeID - volume ID: %s error: %s", req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	if req.GetNodeId() == "" {
		e := fmt.Errorf("ControllerPublishVolume - volume ID: %s request nodeId was empty", req.VolumeId)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	err = validateNodeID(req.GetNodeId())
	if err != nil {
		e := fmt.Errorf("ControllerPublishVolume - validateNodeID - volume ID: %s error: %s", req.GetVolumeId(), err.Error())
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
		e := fmt.Errorf("ControllerPublishVolume - BuildCommonService - volume ID: %s error: %s", req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	storageController, err := storage.NewStorageController(comnserv, 0, volproto.StorageType, config, req.GetSecrets())
	if err != nil || storageController == nil {
		e := fmt.Errorf("ControllerPublishVolume - NewStorageController - volume ID: %s type %v error: %s", req.GetVolumeId(), volproto, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	publishVolResp, err = storageController.ControllerPublishVolume(ctx, req)
	if err != nil {
		e := fmt.Errorf("ControllerPublishVolume - ControllerPublishVolume - failed proto: %v volume ID: %s node ID: %s error: %v", volproto, req.GetVolumeId(), req.GetNodeId(), err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	zlog.Info().Msgf("ControllerPublishVolume Finish - volume ID: %s", req.GetVolumeId())

	return publishVolResp, nil
}

// ControllerUnpublishVolume method
func (s *ControllerServer) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (unpublishVolResp *csi.ControllerUnpublishVolumeResponse, err error) {
	zlog.Info().Msgf("ControllerUnpublishVolume Start - volume ID: %s node ID: %s", req.GetVolumeId(), req.GetNodeId())

	if req.GetVolumeId() == "" {
		e := fmt.Errorf("ControllerUnpublishVolume - request volumeId parameter was empty")
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}
	volproto, err := storage.ValidateVolumeID(req.GetVolumeId())
	if err != nil {
		e := fmt.Errorf("ControllerUnpublishVolume - ValidateVolumeID -  volume ID: %s error: %s", req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	nodeID := req.GetNodeId()
	if nodeID != "" { // NodeId is optional, when empty we should unpublish the volume from any nodes it is published to
		err = validateNodeID(nodeID)
		if err != nil {
			e := fmt.Errorf("ControllerUnpublishVolume - validateNodeID - node ID: %s error: %s", nodeID, err.Error())
			zlog.Error().Msg(e.Error())
			return nil, status.Error(codes.InvalidArgument, e.Error())
		}
	}

	config := make(map[string]string)

	comnserv, err := storage.BuildCommonService(config, req.GetSecrets(), &volproto)
	if err != nil {
		e := fmt.Errorf("ControllerUnpublishVolume - BuildCommonService - volume ID: %s error: %s", req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	hostName, err := storage.DetermineHostName(req.GetNodeId())
	if err != nil {
		e := fmt.Errorf("ControllerUnpublishVolume - DetermineHostName - volume ID: %s error: %s", req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	volproto.NodeID = req.GetNodeId()

	if volproto.StorageType != common.PROTOCOL_NFS && volproto.StorageType != common.PROTOCOL_TREEQ {
		volproto.Host, err = comnserv.IboxApi.GetHostByName(hostName)
		if err != nil {
			re, ok := err.(*iboxapi.IboxAPIError)
			if ok && re.Code == iboxapi.IBOXAPI_RESOURCE_NOT_FOUND_ERROR {
				return &csi.ControllerUnpublishVolumeResponse{}, nil
			}
			e := fmt.Errorf("ControllerUnpublishVolume - GetHostByName - volume ID: %s error: %s", req.GetVolumeId(), err.Error())
			zlog.Error().Msg(e.Error())
			return nil, status.Error(codes.Internal, e.Error())
		}
	}

	storageController, err := storage.NewStorageController(comnserv, 0, volproto.StorageType, config, req.GetSecrets())
	if err != nil {
		e := fmt.Errorf("ControllerUnpublishVolume - NewStorageController - volume ID: %s error: %s", req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	unpublishVolResp, err = storageController.ControllerUnpublishVolume(ctx, req)
	if err != nil {
		e := fmt.Errorf("ControllerUnpublishVolume - sc.ControllerUnpublishVolume - volume ID: %s error: %s", req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	zlog.Info().Msgf("ControllerUnPublishVolume Finish - volume ID: %s", req.GetVolumeId())

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
	zlog.Info().Msgf("ValidateVolumeCapabilities Started - volume ID: %s", req.GetVolumeId())

	if req.GetVolumeId() == "" {
		e := fmt.Errorf("ValidateVolumeCapabilities - error volumeId parameter was empty")
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}
	if req.VolumeCapabilities == nil {
		e := fmt.Errorf("ValidateVolumeCapabilities - volume ID: %s error volumeCapabilities parameter was nil", req.GetVolumeId())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}
	if len(req.VolumeCapabilities) == 0 {
		e := fmt.Errorf("ValidateVolumeCapabilities - volume ID: %s error volumeCapabilities parameter was empty", req.GetVolumeId())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	volproto, err := storage.ValidateVolumeID(req.GetVolumeId())
	if err != nil {
		e := fmt.Errorf("ValidateVolumeCapabilities - ValidateVolumeID - volume ID: %s error: %s", req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	config := make(map[string]string)
	comnserv, err := storage.BuildCommonService(config, req.GetSecrets(), &volproto)
	if err != nil {
		e := fmt.Errorf("ValidateVolumeCapabilities - BuildCommonService - volume ID: %s error: %s", req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	scParameters := req.Parameters
	protocol := scParameters[common.SC_STORAGE_PROTOCOL]

	if protocol == common.PROTOCOL_NFS || protocol == common.PROTOCOL_TREEQ {
		var fs *iboxapi.FileSystem
		fs, err = comnserv.IboxApi.GetFileSystemByID(volproto.VolumeID)
		if err != nil {
			e := fmt.Errorf("ValidateVolumeCapabilities - GetFileSystemByID volume ID: %d error: %s", volproto.VolumeID, err.Error())
			zlog.Error().Msg(e.Error())
			return nil, status.Error(codes.NotFound, e.Error())
		}
		zlog.Debug().Msgf("filesystem volume ID: %d file system details: %v", volproto.VolumeID, fs)
	} else {
		var vol *iboxapi.Volume
		vol, err = comnserv.IboxApi.GetVolume(volproto.VolumeID)
		if err != nil {
			e := fmt.Errorf("ValidateVolumeCapabilities - GetVolume - failed to find volume ID: %d Error: %v", volproto.VolumeID, err)
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

	zlog.Info().Msgf("ValidateVolumeCapabilities Finished - volume ID: %s", req.GetVolumeId())

	return validateVolCapsResponse, nil
}

func (s *ControllerServer) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	zlog.Info().Msgf("ControllerListVolumes Started")

	res := &csi.ListVolumesResponse{
		Entries: make([]*csi.ListVolumesResponse_Entry, 0),
	}

	if req.StartingToken == "" || req.StartingToken == "next-token" {
	} else {
		e := fmt.Errorf("ListVolumes - error startingToken parameter was incorrect [%s]", req.StartingToken)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Aborted, e.Error())
	}

	// Get a k8s go client for in-cluster use
	cl, err := clientgo.BuildClient()
	if err != nil {
		e := fmt.Errorf("ListVolumes  - BuildClient - error: %s", err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Unavailable, e.Error())
	}

	// Find PVs managed by this CSI driver
	pvList, err := cl.GetAllPersistentVolumes()
	if err != nil {
		e := fmt.Errorf("ListVolumes  - GetAllPersistentVolumes - error: %s", err.Error())
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

	zlog.Info().Msgf("ControllerListVolumes Finished")

	return res, nil

}

func (s *ControllerServer) ListSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	zlog.Info().Msgf("ControllerListSnapshots Started, MaxEntries=%d", req.MaxEntries)

	res := &csi.ListSnapshotsResponse{
		Entries: make([]*csi.ListSnapshotsResponse_Entry, 0),
	}

	// Get a k8s go client for in-cluster use
	cl, err := clientgo.BuildClient()
	if err != nil {
		e := fmt.Errorf("ListSnapshots  - BuildClient - error: %s", err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Unavailable, e.Error())
	}

	ns := os.Getenv("POD_NAMESPACE")
	zlog.Debug().Msgf("POD_NAMESPACE=%s", ns)
	if ns == "" {
		e := fmt.Errorf("ListSnapshots - env var POD_NAMESPACE was not set, this is a required env var")
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Unavailable, e.Error())
	}

	secrets, err := cl.GetSecrets(ns)
	if err != nil {
		e := fmt.Errorf("ListSnapshots - GetSecrets - error: %s", err.Error())
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
			e := fmt.Errorf("ListSnapshots - NewClient - error: %s", err.Error())
			zlog.Error().Msg(e.Error())
			return nil, status.Error(codes.Unavailable, e.Error())
		}

		snapshots, err := clientsvc.Iboxapi.GetAllSnapshots()
		if err != nil {
			e := fmt.Errorf("ListSnapshots - GetAllSnapshots - error: %s", err.Error())
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
				e := fmt.Errorf("ListSnapshots - ValidateVolumeID - error: %s", err.Error())
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
			zlog.Debug().Msgf("snapshot datasettype %s", snapshots[i].DatasetType)
			switch snapshots[i].DatasetType {
			case "VOLUME":
				_, err := clientsvc.Iboxapi.GetVolume(snapshots[i].ParentId)
				if err != nil {
					zlog.Error().Msgf("ListSnapshots - GetVolume - snapshot %s VOLUME parentId %d error %s", snapshots[i].Name, snapshots[i].ParentId, err.Error())
					parentName = "unknown"
				} else {
					parentName = strconv.Itoa(snapshots[i].ParentId)
				}
			case "FILESYSTEM":
				_, err := clientsvc.Iboxapi.GetFileSystemByID(snapshots[i].ParentId)
				if err != nil {
					zlog.Error().Msgf("ListSnapshots - GetFileSystemByID - snapshot %s FILESYSTEM parentId %d error %s", snapshots[i].Name, snapshots[i].ParentId, err.Error())
					parentName = "unknown"
				} else {
					parentName = strconv.Itoa(snapshots[i].ParentId)
				}
			default:
				zlog.Error().Msgf("ListSnapshots - snapshot %s unknown dataset type %s parentId %d ", snapshots[i].Name, snapshots[i].DatasetType, snapshots[i].ParentId)
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

			zlog.Info().Msgf("SourceVolumeId = %s SnapshotId = %s", req.SourceVolumeId, req.SnapshotId)

			if req.SourceVolumeId != "" {
				volProto, err := storage.ValidateVolumeID(req.SourceVolumeId)
				if err != nil {
					e := fmt.Errorf("ListSnapshots - ValidateVolumeID - error validating sourceVolumeId %s %s", req.SourceVolumeId, err.Error())
					zlog.Error().Msg(e.Error())
					return nil, status.Error(codes.InvalidArgument, e.Error())
				} else {
					zlog.Debug().Msgf("comparing %d to %s %+v\n", volProto.VolumeID, entry.Snapshot.SourceVolumeId, entry.Snapshot)
					if strconv.Itoa(volProto.VolumeID) == entry.Snapshot.SourceVolumeId {
						zlog.Debug().Msgf("matches!")
						entry.Snapshot.SourceVolumeId = req.SourceVolumeId //set the SourceVolumeId sent back to the incoming format xxxx$$nfs
						res.Entries = append(res.Entries, &entry)
					}
				}
			} else if req.SnapshotId != "" {
				zlog.Debug().Msgf("comparing %d to %d", iValue, snapshots[i].ID)
				if iValue == snapshots[i].ID {
					zlog.Debug().Msgf("req.SnapshotID contains %s found matching snapshot with ID %d name %s\n", req.SnapshotId, snapshots[i].ID, snapshots[i].Name)
					entry.Snapshot.SnapshotId = req.SnapshotId
					res.Entries = append(res.Entries, &entry)
				}
			} else {
				res.Entries = append(res.Entries, &entry)
			}

		}
	}

	zlog.Info().Msgf("ControllerListSnapshots Finished with returned entries count %d", len(res.Entries))

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

	zlog.Info().Msgf("CreateSnapshot Started - Snapshot Name: %s source volume ID: %s", req.GetName(), req.GetSourceVolumeId())

	volproto, err := storage.ValidateVolumeID(req.GetSourceVolumeId())
	if err != nil {
		e := fmt.Errorf("CreateSnapshot - ValidateVolumeID - snapshot Name: %s source volume ID: %s failed to validate storage type %v", req.GetName(), req.GetSourceVolumeId(), err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}
	config := map[string]string{
		"nodeid": s.Driver.nodeID,
	}

	comnserv, err := storage.BuildCommonService(config, req.GetSecrets(), &volproto)
	if err != nil {
		e := fmt.Errorf("CreateSnapshot - BuildCommonService - snapshot name: %s source volume ID: %s failed to get ibox api error: %v", req.GetName(), req.GetSourceVolumeId(), err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}
	storageController, err := storage.NewStorageController(comnserv, 0, volproto.StorageType, config, req.GetSecrets())
	if err != nil {
		e := fmt.Errorf("CreateSnapshot - NewStorageController - snapshot name: %s source volume ID: %s error: %s", req.GetName(), req.GetSourceVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	createSnapshotResp, err = storageController.CreateSnapshot(ctx, req)
	if err != nil {
		e := fmt.Errorf("CreateSnapshot - sc.CreateSnapshot - snapshot name: %s source volume ID: %s error: %s", req.GetName(), req.GetSourceVolumeId(), err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	eventData := make([]iboxapi.EventRequestData, 0)
	locking := req.Parameters[common.LOCK_EXPIRES_AT_PARAMETER]
	if locking != "" {
		lockingData := iboxapi.EventRequestData{
			Name:  common.LOCK_EXPIRES_AT_PARAMETER,
			Type:  "String",
			Value: locking,
		}
		eventData = append(eventData, lockingData)
	}
	protocolData := iboxapi.EventRequestData{
		Name:  common.SC_STORAGE_PROTOCOL,
		Type:  "String",
		Value: volproto.StorageType,
	}
	eventData = append(eventData, protocolData)

	volumeNameData := iboxapi.EventRequestData{
		Name:  common.CUSTOM_EVENT_VOLUME_NAME,
		Type:  "String",
		Value: req.GetName(),
	}
	eventData = append(eventData, volumeNameData)

	volumeIDData := iboxapi.EventRequestData{
		Name:  common.CUSTOM_EVENT_VOLUME_ID,
		Type:  "String",
		Value: req.GetSourceVolumeId(),
	}
	eventData = append(eventData, volumeIDData)

	actionData := iboxapi.EventRequestData{
		Name:  common.CUSTOM_EVENT_ACTION,
		Type:  "String",
		Value: "Create Snapshot",
	}
	eventData = append(eventData, actionData)

	eventDesc := fmt.Sprintf("CSI - Create Snapshot - snapshot name: %s source volume ID: %s", req.GetName(), req.GetSourceVolumeId())
	eventErr := helper.CreateEvent(comnserv.Api, comnserv.IboxApi, eventDesc, eventData)
	if eventErr != nil {
		zlog.Error().Msgf("CreateSnapshot - CreateEvent - snapshot name: %s source volume ID: %s error %s", req.GetName(), req.GetSourceVolumeId(), eventErr.Error())
		// only log errors if custom event fails
	} else {
		zlog.Debug().Msgf("CreateEvent - created external event %+v", eventData)
	}

	return createSnapshotResp, nil
}

func (s *ControllerServer) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (deleteSnapshotResp *csi.DeleteSnapshotResponse, err error) {

	snapshotID := req.GetSnapshotId()
	zlog.Info().Msgf("DeleteSnapshot Start - snapshot ID:  %s", snapshotID)
	volproto, err := storage.ValidateVolumeID(snapshotID)
	if err != nil {
		e := fmt.Errorf("DeleteSnapshot - ValidateVolumeID - snapshot ID: %s invalid, error: %v", snapshotID, err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	config := map[string]string{
		"nodeid": s.Driver.nodeID,
	}
	comnserv, err := storage.BuildCommonService(config, req.GetSecrets(), &volproto)
	if err != nil {
		e := fmt.Errorf("DeleteSnapshot - BuildCommonService - snapshot ID: %s error: %v", req.GetSnapshotId(), err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	storageController, err := storage.NewStorageController(comnserv, 0, volproto.StorageType, config, req.GetSecrets())
	if err != nil {
		e := fmt.Errorf("DeleteSnapshot - NewStorageController - snapshot ID: %s error %s", req.GetSnapshotId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	req.SnapshotId = strconv.Itoa(volproto.VolumeID)

	deleteSnapshotResp, err = storageController.DeleteSnapshot(ctx, req)
	if err != nil {
		e := fmt.Errorf("DeleteSnapshot - sc.DeleteSnapshot - snapshot ID: %s error: %s", req.GetSnapshotId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	zlog.Info().Msgf("DeleteSnapshot Finished - snapshot ID:  %s", snapshotID)
	return deleteSnapshotResp, err

}

func (s *ControllerServer) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (expandVolResp *csi.ControllerExpandVolumeResponse, err error) {

	zlog.Info().Msgf("ControllerExpandVolume Started - volume ID: %s", req.GetVolumeId())

	err = validateExpandVolumeRequest(req)
	if err != nil {
		e := fmt.Errorf("ControllerExpandVolume - validate - volume ID: %s error: %s", req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	configparams := map[string]string{
		"nodeid": s.Driver.nodeID,
	}
	volproto, err := storage.ValidateVolumeID(req.GetVolumeId())
	if err != nil {
		e := fmt.Errorf("ControllerExpandVolume - ValidateVolumeID - volume ID: %s error: %s", req.GetVolumeId(), err.Error())
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
		e := fmt.Errorf("ControllerExpandVolume - BuildCommonService - volume ID: %s error: %v", req.GetVolumeId(), err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	storageController, err := storage.NewStorageController(comnserv, capacity, volproto.StorageType, configparams, req.GetSecrets())
	if err != nil {
		e := fmt.Errorf("ControllerExpandVolume - NewStorageController - volume ID: %s error: %s", req.GetVolumeId(), err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	if storageController != nil {
		req.VolumeId = strconv.Itoa(volproto.VolumeID)
		expandVolResp, err = storageController.ControllerExpandVolume(ctx, req)
		if err != nil {
			e := fmt.Errorf("ControllerExpandVolume - sc.ControllerExpandVolume - volume ID: %s error: %s", req.GetVolumeId(), err)
			zlog.Error().Msg(e.Error())
			return nil, status.Error(codes.Internal, e.Error())
		}
	}

	zlog.Info().Msgf("ControllerExpandVolume Finished - volume ID: %s", req.GetVolumeId())

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

func validateCommonStorageClassParameters(comnserv storage.Commonservice, scParameters map[string]string) error {
	poolName := scParameters[common.SC_POOL_NAME]
	_, err := comnserv.IboxApi.GetPoolByName(poolName)
	if err != nil {
		return err
	}

	protocol := scParameters[common.SC_STORAGE_PROTOCOL]

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
