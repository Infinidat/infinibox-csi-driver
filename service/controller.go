/*Copyright 2020 Infinidat
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.*/
package service

import (
	"context"
	"errors"
	"fmt"

	"infinibox-csi-driver/storage"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog"
)

//CreateVolume method create the volume
func (s *service) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (createVolResp *csi.CreateVolumeResponse, err error) {
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("Recovered from CSI CreateVolume  " + fmt.Sprint(res))
		}
	}()

	//TODO: validate the required parameter
	configparams := make(map[string]string)
	configparams["nodeid"] = s.nodeID
	configparams["driverversion"] = s.driverVersion

	volName := req.GetName()
	storageprotocol := req.GetParameters()["storage_protocol"]

	klog.V(2).Infof("CreateVolume called, vol-name: %s controller nodeid: %s storage_protocol: %s capacity-range: %v params: %v",
		volName, s.nodeID, storageprotocol, req.GetCapacityRange(), req.GetParameters())
	if storageprotocol == "" {
		return nil, status.Error(codes.Internal, "'storage_protocol' is a required field, not found")
	}
	storageController, err := storage.NewStorageController(storageprotocol, configparams, req.GetSecrets())
	if err != nil || storageController == nil {
		klog.Errorf("CreateVolume error: %v", err)
		err = errors.New("failed to initialise storage controller while create volume " + storageprotocol)
		return nil, err
	}
	createVolResp, err = storageController.CreateVolume(ctx, req)
	if err == nil && createVolResp != nil && createVolResp.Volume != nil && createVolResp.Volume.VolumeId != "" {
		createVolResp.Volume.VolumeId = createVolResp.Volume.VolumeId + "$$" + storageprotocol
		klog.V(2).Infof("CreateVolume success, volume name: %s id: %s", volName, createVolResp.Volume.VolumeId)
	}
	return
}

func (s *service) createVolumeFromSnapshot(req *csi.CreateVolumeRequest,
	snapshotSource *csi.VolumeContentSource_SnapshotSource,
	name string, sizeInKbytes int64, storagePool string) (*csi.CreateVolumeResponse, error) {
	return &csi.CreateVolumeResponse{}, nil
}

//DeleteVolume method delete the volumne
func (s *service) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (deleteVolResp *csi.DeleteVolumeResponse, err error) {

	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("Recovered from CSI DeleteVolume  " + fmt.Sprint(res))
		}
	}()

	voltype := req.GetVolumeId()
	klog.V(2).Infof("DeleteVolume method called with volume name %s", voltype)
	volproto, err := s.validateStorageType(req.GetVolumeId())
	if err != nil {
		if status.Code(err) == codes.NotFound {
			klog.Errorf("DeleteVolume success, volume not found")
		} else {
			klog.Errorf("DeleteVolume success, no such volume - invalid storage type")
		}
		return &csi.DeleteVolumeResponse{}, nil
	}
	config := make(map[string]string)
	config["nodeid"] = s.nodeID
	storageController, err := storage.NewStorageController(volproto.StorageType, config, req.GetSecrets())
	if err != nil || storageController == nil {
		err = errors.New("fail to initialise storage controller while delete volume " + volproto.StorageType)
		return
	}
	req.VolumeId = volproto.VolumeID
	deleteVolResp, err = storageController.DeleteVolume(ctx, req)
	if err != nil {
		klog.Errorf("fail to delete volume %v", err)
		err = errors.New("fail to delete volume of type " + volproto.StorageType)
		return
	}
	req.VolumeId = voltype
	return
}

//ControllerPublishVolume method
func (s *service) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (publishVolResp *csi.ControllerPublishVolumeResponse, err error) {
	klog.V(2).Infof("Main ControllerPublishVolume called with req volumeID %s, nodeID %s", req.GetVolumeId(), req.GetNodeId())
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("Recovered from CSI ControllerPublishVolume  " + fmt.Sprint(res))
		}
	}()

	volproto, err := s.validateStorageType(req.GetVolumeId())
	if err != nil {
		klog.Errorf("ControllerPublishVolume failed to validate request: %v", err)
		err = status.Errorf(codes.NotFound, "ControllerPublishVolume failed: %v", err)
		return
	}
	config := make(map[string]string)

	storageController, err := storage.NewStorageController(volproto.StorageType, config, req.GetSecrets())
	if err != nil || storageController == nil {
		err = errors.New("ControllerPublishVolume failed to initialise storage controller: " + volproto.StorageType)
		return
	}
	publishVolResp, err = storageController.ControllerPublishVolume(ctx, req)
	if err != nil {
		klog.Errorf("ControllerPublishVolume, request: '%v', error: '%v'", req, err)
	}
	return
}

//ControllerUnpublishVolume method
func (s *service) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (unpublishVolResp *csi.ControllerUnpublishVolumeResponse, err error) {
	klog.V(2).Infof("Main ControllerUnpublishVolume called with req volume ID %s and node ID %s", req.GetVolumeId(), req.GetNodeId())
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("Recovered from CSI ControllerUnpublishVolume  " + fmt.Sprint(res))
		}
	}()

	volproto, err := s.validateStorageType(req.GetVolumeId())
	if err != nil {
		klog.Errorf("ControllerUnpublishVolume failed to validate request: %v", err)
		err = status.Errorf(codes.NotFound, "ControllerUnpublishVolume failed: %v", err)
		return
	}
	config := make(map[string]string)
	storageController, err := storage.NewStorageController(volproto.StorageType, config, req.GetSecrets())
	if err != nil || storageController == nil {
		err = errors.New("ControllerUnpublishVolume failed to initialise storage controller: " + volproto.StorageType)
		return
	}
	unpublishVolResp, err = storageController.ControllerUnpublishVolume(ctx, req)
	if err != nil {
		klog.Errorf("ControllerUnpublishVolume %v", err)
	}
	return
}

func (s *service) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (validateVolCapsResp *csi.ValidateVolumeCapabilitiesResponse, err error) {
	klog.V(2).Infof("Main ValidateVolumeCapabilities called with req volumeID %s", req.GetVolumeId())
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("Recovered from CSI ValidateVolumeCapabilities  " + fmt.Sprint(res))
		}
	}()

	volproto, err := s.validateStorageType(req.GetVolumeId())
	if err != nil {
		klog.Errorf("ValidateVolumeCapabilities failed to validate request: %v", err)
		err = status.Errorf(codes.NotFound, "ValidateVolumeCapabilities failed: %v", err)
		return
	}

	config := make(map[string]string)
	storageController, err := storage.NewStorageController(volproto.StorageType, config, req.GetSecrets())
	if err != nil || storageController == nil {
		err = errors.New("ValidateVolumeCapabilities failed to initialise storage controller: " + volproto.StorageType)
		return
	}
	validateVolCapsResp, err = storageController.ValidateVolumeCapabilities(ctx, req)
	if err != nil {
		klog.Errorf("ValidateVolumeCapabilities %v", err)
	}
	return
}

func (s *service) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	return &csi.ListVolumesResponse{}, status.Error(codes.Unimplemented, "")
}

func (s *service) ListSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	return &csi.ListSnapshotsResponse{}, status.Error(codes.Unimplemented, "")
}

func (s *service) GetCapacity(ctx context.Context, req *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error) {
	return &csi.GetCapacityResponse{}, nil
}

func (s *service) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	return &csi.ControllerGetCapabilitiesResponse{
		Capabilities: []*csi.ControllerServiceCapability{
			&csi.ControllerServiceCapability{
				Type: &csi.ControllerServiceCapability_Rpc{
					Rpc: &csi.ControllerServiceCapability_RPC{
						Type: csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
					},
				},
			},
			// &csi.ControllerServiceCapability{
			// 	Type: &csi.ControllerServiceCapability_Rpc{
			// 		Rpc: &csi.ControllerServiceCapability_RPC{
			// 			Type: csi.ControllerServiceCapability_RPC_LIST_VOLUMES,
			// 		},
			// 	},
			// },
			&csi.ControllerServiceCapability{
				Type: &csi.ControllerServiceCapability_Rpc{
					Rpc: &csi.ControllerServiceCapability_RPC{
						Type: csi.ControllerServiceCapability_RPC_GET_CAPACITY,
					},
				},
			},
			&csi.ControllerServiceCapability{
				Type: &csi.ControllerServiceCapability_Rpc{
					Rpc: &csi.ControllerServiceCapability_RPC{
						Type: csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT,
					},
				},
			},
			&csi.ControllerServiceCapability{
				Type: &csi.ControllerServiceCapability_Rpc{
					Rpc: &csi.ControllerServiceCapability_RPC{
						Type: csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME,
					},
				},
			},
			&csi.ControllerServiceCapability{
				Type: &csi.ControllerServiceCapability_Rpc{
					Rpc: &csi.ControllerServiceCapability_RPC{
						Type: csi.ControllerServiceCapability_RPC_CLONE_VOLUME,
					},
				},
			},
			// &csi.ControllerServiceCapability{
			// 	Type: &csi.ControllerServiceCapability_Rpc{
			// 		Rpc: &csi.ControllerServiceCapability_RPC{
			// 			Type: csi.ControllerServiceCapability_RPC_LIST_SNAPSHOTS,
			// 		},
			// 	},
			// },
			&csi.ControllerServiceCapability{
				Type: &csi.ControllerServiceCapability_Rpc{
					Rpc: &csi.ControllerServiceCapability_RPC{
						Type: csi.ControllerServiceCapability_RPC_EXPAND_VOLUME,
					},
				},
			},
		},
	}, nil
}

func (s *service) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (createSnapshotResp *csi.CreateSnapshotResponse, err error) {
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("Recovered from CSI CreateSnapshot  " + fmt.Sprint(res))
		}
	}()

	klog.V(2).Infof("Create Snapshot called with volume Id %s", req.GetSourceVolumeId())
	volproto, err := s.validateStorageType(req.GetSourceVolumeId())
	if err != nil {
		klog.Errorf("fail to validate storage type %v", err)
		return nil, status.Errorf(codes.InvalidArgument, "Failed to validate source Vol Id: %s", err.Error())
	}
	config := make(map[string]string)
	config["nodeid"] = s.nodeID
	config["nodeIPAddress"] = s.nodeIPAddress
	storageController, err := storage.NewStorageController(volproto.StorageType, config, req.GetSecrets())
	if err != nil {
		klog.Errorf("Create snapshot failed: %s", err)
		return nil, err
	}
	if storageController != nil {
		createSnapshotResp, err = storageController.CreateSnapshot(ctx, req)
		return createSnapshotResp, err
	}
	return nil, errors.New("Failed to create storageController for " + volproto.StorageType)
}

func (s *service) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (deleteSnapshotResp *csi.DeleteSnapshotResponse, err error) {
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("Recovered from CSI DeleteSnapshot  " + fmt.Sprint(res))
		}
	}()

	snapshotID := req.GetSnapshotId()
	klog.V(2).Infof("Delete Snapshot called with snapshot Id %s", snapshotID)
	if snapshotID == "" {
		klog.Errorf("Delete Snapshot called with empty snapshot Id")
		return nil, errors.New("Delete Snapshot called with empty snapshot Id")
	}
	volproto, err := s.validateStorageType(snapshotID)
	if err != nil {
		klog.Errorf("fail to validate storage type %v", err)
		return nil, status.Errorf(codes.InvalidArgument, "Failed to validate snapshot Id: %s", err.Error())
	}

	config := make(map[string]string)
	config["nodeid"] = s.nodeID
	config["nodeIPAddress"] = s.nodeIPAddress
	storageController, err := storage.NewStorageController(volproto.StorageType, config, req.GetSecrets())
	if err != nil {
		klog.Errorf("Delete snapshot failed: %s", err)
		return nil, err
	}
	if storageController != nil {
		req.SnapshotId = volproto.VolumeID
		deleteSnapshotResp, err := storageController.DeleteSnapshot(ctx, req)
		return deleteSnapshotResp, err
	}
	return nil, errors.New("Failed to create storageController for " + volproto.StorageType)
}

func (s *service) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (expandVolResp *csi.ControllerExpandVolumeResponse, err error) {
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("Recovered from CSI ControllerExpandVolume  " + fmt.Sprint(res))
		}
	}()

	err = s.validateExpandVolumeRequest(req)
	if err != nil {
		return
	}

	configparams := make(map[string]string)
	configparams["nodeid"] = s.nodeID
	volproto, err := s.validateStorageType(req.GetVolumeId())
	if err != nil {
		return
	}

	storageController, err := storage.NewStorageController(volproto.StorageType, configparams, req.GetSecrets())
	if err != nil {
		klog.Errorf("Expand volume failed: %s", err)
		return
	}
	if storageController != nil {
		req.VolumeId = volproto.VolumeID
		expandVolResp, err = storageController.ControllerExpandVolume(ctx, req)
		return expandVolResp, err
	}
	return
}

func (s *service) ControllerGetVolume(
	_ context.Context, _ *csi.ControllerGetVolumeRequest,
) (*csi.ControllerGetVolumeResponse, error) {

	// Infinidat does not support ControllerGetVolume
	return nil, status.Error(codes.Unimplemented, "")
}
