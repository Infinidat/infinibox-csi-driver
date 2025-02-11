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

	zlog.Info().Msgf("CreateVolume Start - ID: %s", req.GetName())

	volName := req.GetName()

	reqParameters := req.GetParameters()
	if len(reqParameters) == 0 {
		zlog.Error().Msgf("CreateVolume - GetParameters empty ")
		return nil, status.Error(codes.InvalidArgument, "no Parameters provided to CreateVolume")
	}

	networkSpace := reqParameters[common.SC_NETWORK_SPACE]
	storageprotocol := reqParameters[common.SC_STORAGE_PROTOCOL]
	reqCapabilities := req.GetVolumeCapabilities()

	zlog.Debug().Msgf("CreateVolume - capacity-range: %v ", req.GetCapacityRange())
	zlog.Debug().Msgf("CreateVolume - params: %v", reqParameters)
	zlog.Debug().Msgf("CreateVolume  - name: '%s' controller nodeid: '%s' storage_protocol: '%s' capacity-range: %v params: %v",
		volName, s.Driver.nodeID, storageprotocol, req.GetCapacityRange(), reqParameters)

	// Basic CSI parameter checking across protocols

	if len(storageprotocol) == 0 {
		zlog.Error().Msgf("CreateVolume - storage protocol empty ")
		return nil, status.Error(codes.InvalidArgument, "no 'storage_protocol' provided to CreateVolume")
	}
	if storageprotocol != common.PROTOCOL_FC && len(networkSpace) == 0 {
		zlog.Error().Msgf("CreateVolume - network space empty ")
		return nil, status.Error(codes.InvalidArgument, "no 'network_space' provided to CreateVolume")
	}
	if len(volName) == 0 {
		zlog.Error().Msgf("CreateVolume - volume name empty ")
		return nil, status.Error(codes.InvalidArgument, "no name provided to CreateVolume")
	}
	if len(reqCapabilities) == 0 {
		zlog.Error().Msgf("CreateVolume - volume capabilitiles empty ")
		return nil, status.Error(codes.InvalidArgument, "no VolumeCapabilities provided to CreateVolume")
	}

	var summary string
	summary, err = validateCapabilities(reqCapabilities)
	if err != nil {
		zlog.Error().Msgf("CreateVolume - validateCapabilities - error %s", err.Error())
		return nil, status.Errorf(codes.InvalidArgument, "VolumeCapabilities invalid: %v", err)
	}
	if reqParameters[common.SC_POOL_NAME] == "" {
		zlog.Error().Msgf("CreateVolume - %s empty", common.SC_POOL_NAME)
		return nil, status.Errorf(codes.InvalidArgument, "no 'pool_name' provided to CreateVolume, verify pool_name is specified in StorageClass")
	}
	// TODO: move non-protocol-specific capacity request validation here too, verifyVolumeSize function etc

	configparams := map[string]string{
		"nodeid":                         s.Driver.nodeID,
		"driverversion":                  s.Driver.version,
		common.SC_NFS_EXPORT_PERMISSIONS: reqParameters[common.SC_NFS_EXPORT_PERMISSIONS],
	}

	kc, err := clientgo.BuildClient()
	if err != nil {
		zlog.Error().Msgf("CreateVolume - BuildClient - error %s", err.Error())
		err = status.Errorf(codes.Internal, "failed to initialize kube client while creating volume '%v'", err)
		return nil, err
	}

	pvcAnnotations := make(map[string]string)
	extraMetadataPVCName := req.Parameters["csi.storage.k8s.io/pvc/name"]
	extraMetadataPVCNamespace := req.Parameters["csi.storage.k8s.io/pvc/namespace"]
	if extraMetadataPVCName != "" && extraMetadataPVCNamespace != "" {
		pvcAnnotations, err = kc.GetPVCAnnotations(extraMetadataPVCName, extraMetadataPVCNamespace)
		if err != nil {
			zlog.Error().Msgf("CreateVolume - GetPVCAnnotations - error %s", err.Error())
			return nil, status.Errorf(codes.InvalidArgument, "PVC Annotation for %s invalid: %v", req.GetName(), err)
		}
	}
	secretsToUse := req.GetSecrets()

	pvcAnnoSecret := pvcAnnotations[common.PVC_ANNOTATION_IBOX_SECRET]
	if pvcAnnoSecret != "" {
		secretsToUse, err = kc.GetSecret(pvcAnnoSecret, os.Getenv("POD_NAMESPACE"))
		if err != nil {
			zlog.Error().Msgf("CreateVolume - GetSecret - error %s", err.Error())
			return nil, status.Errorf(codes.InvalidArgument, "PVC Annotation for %s invalid: %v", pvcAnnoSecret, err)
		}
	}

	capacity := req.GetCapacityRange().RequiredBytes

	roundUp := true // default to always rounding up, users can set the StorageClass parameter to false if for some reason they want
	roundUpParameter := req.Parameters[common.SC_ROUND_UP]
	if roundUpParameter != "" {
		roundUp, err = strconv.ParseBool(roundUpParameter)
		if err != nil {
			zlog.Error().Msgf("CreateVolume - parse %s - error %s", common.SC_ROUND_UP, err.Error())
			err = status.Errorf(codes.Internal, "error converting %s StorageClass parameter volume '%s' - %s", roundUpParameter, volName, err.Error())
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

	comnserv, err := storage.BuildCommonService(configparams, secretsToUse, nil)
	if err != nil {
		zlog.Error().Msgf("CreateVolume - BuildCommonService - error %s", err.Error())
		err = status.Errorf(codes.Internal, "error getting api,  volume '%s' - %s", volName, err.Error())
		return nil, err
	}

	storageController, err := storage.NewStorageController(comnserv, capacity, storageprotocol, configparams, secretsToUse)
	if err != nil || storageController == nil {
		zlog.Error().Msgf("CreateVolume - NewStorageController - error %s", err.Error())
		err = status.Errorf(codes.Internal, "error while creating volume '%s' - %s", volName, err.Error())
		return nil, err
	}

	err = validateCommonStorageClassParameters(comnserv, req.Parameters)
	if err != nil {
		zlog.Error().Msgf("CreateVolume - validateCommonStorageClassParameters - error %s", err.Error())
		err = status.Errorf(codes.Internal, "error validating StorageClass parameters volume '%s' - %s", volName, err.Error())
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
		zlog.Error().Msgf("CreateVolume - ValidateStorageClass - error %s", err.Error())
		err = status.Errorf(codes.Internal, "error validating StorageClass parameters volume '%s' - %s", volName, err.Error())
		return nil, err
	}

	createVolResp, err = storageController.CreateVolume(ctx, req)
	if err != nil {
		zlog.Error().Msgf("CreateVolume - sc.CreateVolume - error %s", err.Error())
		// it's important to return the original error, because it matches K8s expectations
		return nil, err
	} else if createVolResp == nil {
		zlog.Error().Msgf("CreateVolume - sc.CreateVolume resp nil")
		err = status.Errorf(codes.Internal, "failed to create volume '%s', empty response", volName)
		return nil, err
	} else if createVolResp.Volume == nil {
		zlog.Error().Msgf("CreateVolume - sc.CreateVolume Volume is nil")
		err = status.Errorf(codes.Internal, "failed to create volume '%s', resp: %v, no volume struct", volName, createVolResp)
		return nil, err
	} else if createVolResp.Volume.VolumeId == "" {
		zlog.Error().Msgf("CreateVolume - sc.CreateVolume Volume ID is empty")
		err = status.Errorf(codes.Internal, "failed to create volume '%s', resp: %v, no volumeID", volName, createVolResp)
		return nil, err
	}
	createVolResp.Volume.VolumeId = createVolResp.Volume.VolumeId + "$$" + storageprotocol

	eventData := make([]iboxapi.EventRequestData, 0)
	protocolData := iboxapi.EventRequestData{
		Name:  common.SC_STORAGE_PROTOCOL,
		Type:  "String",
		Value: storageprotocol,
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
	}

	zlog.Info().Msgf("CreateVolume Finish - Name: %s ID: %s", volName, createVolResp.Volume.VolumeId)
	return
}

// DeleteVolume method delete the volumne
func (s *ControllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (deleteVolResp *csi.DeleteVolumeResponse, err error) {

	volumeId := req.GetVolumeId()

	zlog.Info().Msgf("DeleteVolume Start - ID: %s", volumeId)

	volproto, err := storage.ValidateVolumeID(volumeId)
	if err != nil {
		zlog.Error().Msgf("DeleteVolume - ValidateVolumeID - error %s", err.Error())
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	secretsToUse := req.GetSecrets()

	// see if the pvc annotation was specified in the original PVC
	kc, err := clientgo.BuildClient()
	if err != nil {
		zlog.Error().Msgf("DeleteVolume - BuildClient - error %s", err.Error())
		return nil, err
	}
	pvList, err := kc.GetAllPersistentVolumes()
	if err != nil {
		zlog.Error().Msgf("DeleteVolume - GetAllPersistentVolumes - error %s", err.Error())
		return nil, status.Error(codes.InvalidArgument, err.Error())

	}
	for i := 0; i < len(pvList.Items); i++ {
		pv := pvList.Items[i]
		// we match the PV using the volumeHandle (aka volumeId from above)
		if pv.Spec.CSI.VolumeHandle == volumeId {
			zlog.Debug().Msgf("DeleteVolume - pv found for volume ID %s ", volumeId)
			annoPVCSecretName := pv.Spec.CSI.ControllerPublishSecretRef.Name
			annoPVCSecret, err := kc.GetSecret(annoPVCSecretName, os.Getenv("POD_NAMESPACE"))
			if err != nil {
				zlog.Error().Msgf("DeleteVolume - GetSecret - error %s", err.Error())
				err := fmt.Errorf("pvc annotation %s get error %v", annoPVCSecretName, err)
				return nil, status.Error(codes.InvalidArgument, err.Error())
			}
			zlog.Debug().Msgf("DeleteVolume - using secret %s", annoPVCSecretName)
			secretsToUse = annoPVCSecret
		}
	}

	config := map[string]string{
		"nodeid": s.Driver.nodeID,
	}

	comnserv, err := storage.BuildCommonService(config, secretsToUse, &volproto)
	if err != nil {
		zlog.Error().Msgf("DeleteVolume - BuildCommonService - volume ID %s - error %s", volumeId, err.Error())
		return
	}

	storageController, err := storage.NewStorageController(comnserv, 0, volproto.StorageType, config, secretsToUse)
	if err != nil || storageController == nil {
		zlog.Error().Msgf("DeleteVolume - NewStorageController - error %s", err.Error())
		err = status.Error(codes.Internal, "failed to initialise storage controller while delete volume "+volproto.StorageType)
		return
	}

	deleteVolResp, err = storageController.DeleteVolume(ctx, req)
	if err != nil {
		zlog.Error().Msgf("DeleteVolume - sc.DeleteVolume volume ID %s - error %s", volumeId, err.Error())
		return
	}

	zlog.Info().Msgf("DeleteVolume Finish - ID: %s", volumeId)
	return
}

// ControllerModifyVolume method
func (s *ControllerServer) ControllerModifyVolume(ctx context.Context, req *csi.ControllerModifyVolumeRequest) (publishVolResp *csi.ControllerModifyVolumeResponse, err error) {
	zlog.Info().Msg("ControllerModifyVolume is not implemented")
	return nil, nil
}

// ControllerPublishVolume method
func (s *ControllerServer) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (publishVolResp *csi.ControllerPublishVolumeResponse, err error) {

	zlog.Info().Msgf("ControllerPublishVolume Start - ID: %s", req.GetVolumeId())

	zlog.Debug().Msgf("ControllerPublishVolume ID: %s, nodeID: %s", req.GetVolumeId(), req.GetNodeId())

	if req.VolumeCapability == nil {
		err = fmt.Errorf("ControllerPublishVolume - request VolumeCapability was nil")
		zlog.Error().Msg(err.Error())
		err = status.Error(codes.InvalidArgument, err.Error())
		return
	}
	if req.GetVolumeId() == "" {
		err = fmt.Errorf("ControllerPublishVolume  - request volumeId was empty")
		zlog.Error().Msg(err.Error())
		err = status.Error(codes.InvalidArgument, err.Error())
		return
	}

	volproto, err := storage.ValidateVolumeID(req.GetVolumeId())
	if err != nil {
		zlog.Error().Msgf("ControllerPublishVolume - ValidateVolumeID - error : %s", err.Error())
		err = status.Errorf(codes.NotFound, "ControllerPublishVolume failed: %v", err)
		return
	}

	if req.GetNodeId() == "" {
		err = fmt.Errorf("ControllerPublishVolume - request nodeId was empty")
		zlog.Error().Msg(err.Error())
		err = status.Error(codes.InvalidArgument, err.Error())
		return
	}

	err = validateNodeID(req.GetNodeId())
	if err != nil {
		zlog.Error().Msgf("ControllerPublishVolume - validateNodeID - error: %s", err.Error())
		return nil, err
	}

	config := make(map[string]string)

	comnserv, err := storage.BuildCommonService(config, req.GetSecrets(), &volproto)
	if err != nil {
		zlog.Error().Msgf("ControllerPublishVolume - BuildCommonService - error: %s", err.Error())
		return
	}

	storageController, err := storage.NewStorageController(comnserv, 0, volproto.StorageType, config, req.GetSecrets())
	if err != nil || storageController == nil {
		zlog.Error().Msgf("ControllerPublishVolume - NewStorageController - error: %s", err.Error())
		err = status.Errorf(codes.Internal, "ControllerPublishVolume failed to initialise storage controller: %s", volproto.StorageType)
		return
	}
	publishVolResp, err = storageController.ControllerPublishVolume(ctx, req)
	if err != nil {
		zlog.Error().Msgf("ControllerPublishVolume - ControllerPublishVolume - failed with volume ID %s and node ID %s: %v", req.GetVolumeId(), req.GetNodeId(), err)
	}

	zlog.Info().Msgf("ControllerPublishVolume Finish - ID: %s", req.GetVolumeId())

	return
}

// ControllerUnpublishVolume method
func (s *ControllerServer) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (unpublishVolResp *csi.ControllerUnpublishVolumeResponse, err error) {
	zlog.Info().Msgf("ControllerUnpublishVolume Start - ID: %s, nodeID: %s", req.GetVolumeId(), req.GetNodeId())

	if req.GetVolumeId() == "" {
		err = fmt.Errorf("ControllerUnpublishVolume - request volumeId parameter was empty")
		zlog.Error().Msg(err.Error())
		err = status.Error(codes.InvalidArgument, err.Error())
		return
	}
	volproto, err := storage.ValidateVolumeID(req.GetVolumeId())
	if err != nil {
		zlog.Error().Msgf("ControllerUnpublishVolume - ValidateVolumeID -  error: %s", err.Error())
		err = status.Errorf(codes.NotFound, "ControllerUnpublishVolume failed: %v", err)
		return
	}

	nodeID := req.GetNodeId()
	if nodeID != "" { // NodeId is optional, when empty we should unpublish the volume from any nodes it is published to
		err = validateNodeID(nodeID)
		if err != nil {
			zlog.Error().Msgf("ControllerUnpublishVolume - validateNodeID - error: %s", err.Error())
			return nil, err
		}
	}

	config := make(map[string]string)

	comnserv, err := storage.BuildCommonService(config, req.GetSecrets(), &volproto)
	if err != nil {
		zlog.Error().Msgf("ControllerUnpublishVolume - BuildCommonService - error %s", err.Error())
		return
	}

	hostName, err := storage.DetermineHostName(req.GetNodeId())
	if err != nil {
		zlog.Error().Msgf("ControllerUnpublishVolume - DetermineHostName - error %s", err.Error())
		return nil, err
	}

	volproto.NodeID = req.GetNodeId()

	if volproto.StorageType != common.PROTOCOL_NFS && volproto.StorageType != common.PROTOCOL_TREEQ {
		volproto.Host, err = comnserv.IboxApi.GetHostByName(hostName)
		if err != nil {
			if errors.Is(err, iboxapi.ErrNotFound) {
				return &csi.ControllerUnpublishVolumeResponse{}, nil
			}
			zlog.Error().Msgf("ControllerUnpublishVolume - GetHostByName - error %s", err.Error())
			return nil, err
		}
	}

	storageController, err := storage.NewStorageController(comnserv, 0, volproto.StorageType, config, req.GetSecrets())
	if err != nil || storageController == nil {
		err = errors.New("ControllerUnpublishVolume - NewStorageController - error: " + err.Error())
		return
	}

	unpublishVolResp, err = storageController.ControllerUnpublishVolume(ctx, req)
	if err != nil {
		zlog.Error().Msgf("ControllerUnpublishVolume - sc.ControllerUnpublishVolume - error: %s", err.Error())
	}

	zlog.Info().Msgf("ControllerUnPublishVolume Finish - ID: %s", req.GetVolumeId())

	return
}

func validateCapabilities(capabilities []*csi.VolumeCapability) (summary string, err error) {
	isBlock := false
	isFile := false

	if capabilities == nil {
		return "", errors.New("no volume capabilities specified")
	}

	var modes string
	for _, capability := range capabilities {
		// validate accessMode
		accessMode := capability.GetAccessMode()
		if accessMode == nil {
			return "", errors.New("no accessmode specified in volume capability")
		}
		mode := accessMode.GetMode()
		// TODO: do something to actually reject invalid access modes, if any
		// there aren't any that we don't support yet, but some combinations are dumb?
		if modes != "" {
			modes = modes + ", "
		}
		modes = modes + fmt.Sprintf("mode: %s", mode.String())

		// check block and file behavior
		if block := capability.GetBlock(); block != nil {
			isBlock = true
			if mode == csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER {
				zlog.Warn().Msg("MULTI_NODE_MULTI_WRITER AccessMode requested for block volume, could be dangerous")
			}
			// TODO: something about SINGLE_NODE_MULTI_WRITER (alpha feature) as well?
		}
		if file := capability.GetMount(); file != nil {
			isFile = true
			// We should validate fs_type and []mount_flags parts of MountVolume message in NFS/TreeQ controllers - CSIC-339
		}
	}

	if isBlock && isFile {
		return "", errors.New("both file and block volume capabilities specified")
	}

	summary = summary + fmt.Sprintf("modes: %s isBlock: %t isFile: %t", modes, isBlock, isFile)

	return summary, nil
}

func (s *ControllerServer) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (validateVolCapsResponse *csi.ValidateVolumeCapabilitiesResponse, err error) {
	zlog.Info().Msgf("ValidateVolumeCapabilities Started - ID: %s", req.GetVolumeId())

	if req.GetVolumeId() == "" {
		err := fmt.Errorf("ValidateVolumeCapabilities - error volumeId parameter was empty")
		zlog.Error().Msg(err.Error())
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if req.VolumeCapabilities == nil {
		err := fmt.Errorf("ValidateVolumeCapabilities - error volumeCapabilities parameter was nil")
		zlog.Error().Msg(err.Error())
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if len(req.VolumeCapabilities) == 0 {
		err := fmt.Errorf("ValidateVolumeCapabilities - error volumeCapabilities parameter was empty")
		zlog.Error().Msg(err.Error())
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	volproto, err := storage.ValidateVolumeID(req.GetVolumeId())
	if err != nil {
		zlog.Error().Msgf("ValidateVolumeCapabilities - ValidateVolumeID - error: %s", err.Error())
		err = status.Errorf(codes.NotFound, "ValidateVolumeCapabilities failed: %v", err)
		return
	}

	config := make(map[string]string)
	comnserv, err := storage.BuildCommonService(config, req.GetSecrets(), &volproto)
	if err != nil {
		zlog.Error().Msgf("ValidateVolumeCapabilities - BuildCommonService - error: %s", err.Error())
		return
	}

	scParameters := req.Parameters
	protocol := scParameters[common.SC_STORAGE_PROTOCOL]

	if protocol == common.PROTOCOL_NFS || protocol == common.PROTOCOL_TREEQ {
		var fs *iboxapi.FileSystem
		fs, err = comnserv.IboxApi.GetFileSystemByID(volproto.VolumeID)
		if err != nil {
			zlog.Error().Msgf("ValidateVolumeCapabilities - GetFileSystemByID volume ID %d - error: %s", volproto.VolumeID, err.Error())
			err = status.Errorf(codes.NotFound, "ValidateVolumeCapabilities failed to find filesystem ID: %d, %v", volproto.VolumeID, err)
			return
		}
		zlog.Debug().Msgf("filesystem volID: %d volume: %v", volproto.VolumeID, fs)
	} else {
		var vol *iboxapi.Volume
		vol, err = comnserv.IboxApi.GetVolume(volproto.VolumeID)
		if err != nil {
			e := fmt.Errorf("ValidateVolumeCapabilities - GetVolume - failed to find volume with ID %d. Error: %v", volproto.VolumeID, err)
			zlog.Err(e)
			err = status.Error(codes.NotFound, e.Error())
			return
		}
		zlog.Debug().Msgf("volume volID: %d volume: %v", volproto.VolumeID, vol)
	}
	validateVolCapsResponse = &csi.ValidateVolumeCapabilitiesResponse{
		Confirmed: &csi.ValidateVolumeCapabilitiesResponse_Confirmed{
			VolumeCapabilities: req.GetVolumeCapabilities(),
		},
	}

	zlog.Info().Msgf("ValidateVolumeCapabilities Finished - ID: %s", req.GetVolumeId())

	return
}

func (s *ControllerServer) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	zlog.Info().Msgf("ControllerListVolumes Started")

	res := &csi.ListVolumesResponse{
		Entries: make([]*csi.ListVolumesResponse_Entry, 0),
	}

	if req.StartingToken == "" || req.StartingToken == "next-token" {
	} else {
		err := fmt.Errorf("ListVolumes - error startingToken parameter was incorrect [%s]", req.StartingToken)
		zlog.Error().Msg(err.Error())
		return nil, status.Error(codes.Aborted, err.Error())
	}

	// Get a k8s go client for in-cluster use
	cl, err := clientgo.BuildClient()
	if err != nil {
		zlog.Error().Msgf("ListVolumes  - BuildClient - error: %s", err.Error())
		return nil, status.Errorf(codes.Unavailable, "ListVolumes - BuildClient - cannot list volumes: %v", err)
	}

	// Find PVs managed by this CSI driver
	pvList, err := cl.GetAllPersistentVolumes()
	if err != nil {
		zlog.Error().Msgf("ListVolumes  - GetAllPersistentVolumes - error: %s", err.Error())
		return nil, status.Errorf(codes.Unavailable, "cannot list volumes: %v", err)
	}
	zlog.Info().Msgf("pvList count: %d", len(pvList.Items))

	for _, pv := range pvList.Items {
		zlog.Info().Msgf("pv capacity : %#v", pv.Spec.Capacity)
		zlog.Info().Msgf("pv name: %#v", pv.ObjectMeta.GetName())
		zlog.Info().Msgf("pv anno: %#v", pv.ObjectMeta.GetAnnotations()["pv.kubernetes.io/provisioned-by"])
		if pv.ObjectMeta.GetAnnotations()["pv.kubernetes.io/provisioned-by"] == common.SERVICE_NAME {
			var status csi.ListVolumesResponse_VolumeStatus
			status.PublishedNodeIds = append(status.PublishedNodeIds, pv.ObjectMeta.GetName())
			// TODO Handle csi.ListVolumesResponse_VolumeStatus.VolumeCondition?
			zlog.Info().Msgf("status: %s", status.String())

			var volume csi.Volume

			volume.CapacityBytes = pv.Spec.Capacity.Storage().AsDec().UnscaledBig().Int64()
			volume.VolumeId = pv.ObjectMeta.GetName()
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
		zlog.Error().Msgf("ListSnapshots  - BuildClient - error: %s", err.Error())
		return nil, status.Errorf(codes.Unavailable, "cannot list snapshots: %v", err)
	}

	ns := os.Getenv("POD_NAMESPACE")
	zlog.Debug().Msgf("POD_NAMESPACE=%s", ns)
	if ns == "" {
		zlog.Error().Msg("ListSnapshots - env var POD_NAMESPACE was not set, this is a required env var")
		return nil, status.Errorf(codes.Unavailable, "cannot list snapshots, POD_NAMESPACE required to be set: %v", err)
	}

	secrets, err := cl.GetSecrets(ns)
	if err != nil {
		zlog.Error().Msgf("ListSnapshots - GetSecrets - error: %s", err.Error())
		return nil, status.Errorf(codes.Unavailable, "cannot list snapshots, error getting secrets: %v", err)
	}

	for i := 0; i < len(secrets); i++ {
		x := api.ClientService{
			ConfigMap:  make(map[string]string),
			SecretsMap: secrets[i],
		}

		clientsvc, err := x.NewClient()
		if err != nil {
			zlog.Error().Msgf("ListSnapshots - NewClient - error: %s", err.Error())
			return nil, status.Errorf(codes.Unavailable, "cannot get api client: %v", err)
		}

		snapshots, err := clientsvc.GetAllSnapshots()
		if err != nil {
			zlog.Error().Msgf("ListSnapshots - GetAllSnapshots - error: %s", err.Error())
			return nil, status.Errorf(codes.Unavailable, "cannot list snapshots: %v", err)
		}
		zlog.Debug().Msgf("got back %d snapshots", len(snapshots))

		// handle the optional case where a SnapshotId is passed in the ListSnapshots request
		var volProto api.VolumeProtocolConfig
		var iValue int
		if req.SnapshotId != "" {
			volProto, err = storage.ValidateVolumeID(req.SnapshotId)
			if err != nil {
				zlog.Error().Msgf("ListSnapshots - ValidateVolumeID - error: %s", err.Error())
				return res, nil
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

			zlog.Info().Msgf("SourceVolumeId = %s", req.SourceVolumeId)
			zlog.Info().Msgf("SnapshotId = %s", req.SnapshotId)

			if req.SourceVolumeId != "" {
				volProto, err := storage.ValidateVolumeID(req.SourceVolumeId)
				if err != nil {
					zlog.Error().Msgf("ListSnapshots - ValidateVolumeID - error validating sourceVolumeId %s %s", req.SourceVolumeId, err.Error())
				} else {
					zlog.Debug().Msgf("comparing %d to %s whole thing %+v\n", volProto.VolumeID, entry.Snapshot.SourceVolumeId, entry.Snapshot)
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

	zlog.Info().Msgf("CreateSnapshot Started - ID: %s", req.GetSourceVolumeId())

	volproto, err := storage.ValidateVolumeID(req.GetSourceVolumeId())
	if err != nil {
		zlog.Error().Msgf("CreateSnapshot - ValidateVolumeID - failed to validate storage type %v", err)
		return nil, status.Errorf(codes.InvalidArgument, "Failed to validate source Vol Id: %s", err.Error())
	}
	config := map[string]string{
		"nodeid": s.Driver.nodeID,
	}

	comnserv, err := storage.BuildCommonService(config, req.GetSecrets(), &volproto)
	if err != nil {
		zlog.Error().Msgf("CreateSnapshot - BuildCommonService - failed to get ibox api %v", err)
		return
	}
	storageController, err := storage.NewStorageController(comnserv, 0, volproto.StorageType, config, req.GetSecrets())
	if err != nil {
		zlog.Error().Msgf("CreateSnapshot - NewStorageController - error: %s", err.Error())
		return nil, err
	}
	createSnapshotResp, err = storageController.CreateSnapshot(ctx, req)
	if err != nil {
		zlog.Error().Msgf("CreateSnapshot - sc.CreateSnapshot - error: %s", err)
		return nil, err
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

	eventDesc := fmt.Sprintf("CSI - Create Snapshot- name: %s volume id: %s", req.GetName(), req.GetSourceVolumeId())
	eventErr := helper.CreateEvent(comnserv.Api, comnserv.IboxApi, eventDesc, eventData)
	if eventErr != nil {
		zlog.Error().Msg(eventErr.Error())
		// only log errors if custom event fails
	}

	return createSnapshotResp, nil
}

func (s *ControllerServer) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (deleteSnapshotResp *csi.DeleteSnapshotResponse, err error) {

	snapshotID := req.GetSnapshotId()
	zlog.Info().Msgf("DeleteSnapshot Start - ID:  %s", snapshotID)
	volproto, err := storage.ValidateVolumeID(snapshotID)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			zlog.Error().Msgf("DeleteSnapshot - ValidateVolumeID - snapshot ID: '%s' not found, err: %v - return success", snapshotID, err)
			return &csi.DeleteSnapshotResponse{}, nil
		} else {
			zlog.Error().Msgf("DeleteSnapshot - ValidateVolumeID - snapshot ID: '%s' invalid, err: %v", snapshotID, err)
			return nil, err
		}
	}

	config := map[string]string{
		"nodeid": s.Driver.nodeID,
	}
	comnserv, err := storage.BuildCommonService(config, req.GetSecrets(), &volproto)
	if err != nil {
		zlog.Error().Msgf("DeleteSnapshot - BuildCommonService - error %v", err)
		return
	}
	storageController, err := storage.NewStorageController(comnserv, 0, volproto.StorageType, config, req.GetSecrets())
	if err != nil {
		zlog.Error().Msgf("DeleteSnapshot - NewStorageController - error: %s", err)
		return nil, err
	}
	if storageController == nil {
		return nil, errors.New("DeleteSnapshot - failed to create storageController for " + volproto.StorageType)
	}

	req.SnapshotId = strconv.Itoa(volproto.VolumeID)

	deleteSnapshotResp, err = storageController.DeleteSnapshot(ctx, req)
	if err != nil {
		zlog.Error().Msgf("DeleteSnapshot - sc.DeleteSnapshot - error: %s", err.Error())
	}
	zlog.Info().Msgf("DeleteSnapshot Finished - ID:  %s", snapshotID)
	return deleteSnapshotResp, err

}

func (s *ControllerServer) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (expandVolResp *csi.ControllerExpandVolumeResponse, err error) {

	zlog.Info().Msgf("ControllerExpandVolume Started - ID:  %s", req.GetVolumeId())

	err = validateExpandVolumeRequest(req)
	if err != nil {
		zlog.Error().Msgf("ControllerExpandVolume - validate - error:  %s", err.Error())
		return
	}

	configparams := map[string]string{
		"nodeid": s.Driver.nodeID,
	}
	volproto, err := storage.ValidateVolumeID(req.GetVolumeId())
	if err != nil {
		zlog.Error().Msgf("ControllerExpandVolume - ValidateVolumeID volume ID %s - error:  %s", req.GetVolumeId(), err.Error())
		return
	}

	capacity := int64(req.GetCapacityRange().GetRequiredBytes())

	comnserv, err := storage.BuildCommonService(configparams, req.GetSecrets(), &volproto)
	if err != nil {
		zlog.Error().Msgf("ControllerExpandVolume - BuildCommonService - error %v", err)
		return
	}
	storageController, err := storage.NewStorageController(comnserv, capacity, volproto.StorageType, configparams, req.GetSecrets())
	if err != nil {
		zlog.Error().Msgf("ControllerExpandVolume - NewStorageController - error: %s", err)
		return
	}
	if storageController != nil {
		req.VolumeId = strconv.Itoa(volproto.VolumeID)
		expandVolResp, err = storageController.ControllerExpandVolume(ctx, req)
		return expandVolResp, err
	}

	zlog.Info().Msgf("ControllerExpandVolume Finished - ID:  %s", req.GetVolumeId())

	return
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
		return status.Error(codes.NotFound, "node Id does not follow '<fqdn>$$<id>' pattern")
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
			_, err := comnserv.Api.GetNetworkSpaceByName(name)
			if err != nil {
				zlog.Error().Msgf("network space %s is not found on the ibox", name)
				return err
			}
		}
		// validate network protocol / networkspace compatability
		if err := storage.ValidateProtocolToNetworkSpace(protocol, arrayofNetworkSpaces, comnserv.Api); err != nil {
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
			return fmt.Errorf("format error in StorageClass, %s appears to not be a valid integer, value entered was %s", common.SC_MAX_VOLS_PER_HOST, maxVolsProvided)
		}
	}

	provTypeProvided := scParameters[common.SC_PROVISION_TYPE]
	if provTypeProvided != "" {
		p := strings.ToUpper(provTypeProvided)
		if p != common.SC_THICK_PROVISION_TYPE && p != common.SC_THIN_PROVISION_TYPE {
			return fmt.Errorf("format error in StorageClass, %s appears to not be a valid value, value entered was %s", common.SC_PROVISION_TYPE, provTypeProvided)
		}
	}

	ssdEnabledProvided := scParameters[common.SC_SSD_ENABLED]
	if ssdEnabledProvided != "" {
		_, err := strconv.ParseBool(ssdEnabledProvided)
		if err != nil {
			return fmt.Errorf("format error in StorageClass, %s appears to not be a valid boolean, value entered was %s", common.SC_SSD_ENABLED, ssdEnabledProvided)
		}
	}

	return nil
}
