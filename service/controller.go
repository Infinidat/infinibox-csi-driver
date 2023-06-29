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
	"infinibox-csi-driver/storage"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	tspb "google.golang.org/protobuf/types/known/timestamppb"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"
)

// ControllerServer controller server setting
type ControllerServer struct {
	Driver *Driver
}

// CreateVolume method create the volume
func (s *ControllerServer) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (createVolResp *csi.CreateVolumeResponse, err error) {

	volName := req.GetName()
	reqParameters := req.GetParameters()
	if len(reqParameters) == 0 {
		return nil, status.Error(codes.InvalidArgument, "no Parameters provided to CreateVolume")
	}

	networkSpace := reqParameters[common.SC_NETWORK_SPACE]
	storageprotocol := reqParameters[common.SC_STORAGE_PROTOCOL]
	reqCapabilities := req.GetVolumeCapabilities()

	klog.V(2).Infof("CreateVolume called, capacity-range: %v ",
		req.GetCapacityRange())
	klog.V(2).Infof("CreateVolume called %v params: %v",
		reqParameters)
	klog.V(2).Infof("CreateVolume called, name: '%s' controller nodeid: '%s' storage_protocol: '%s' capacity-range: %v params: %v",
		volName, s.Driver.nodeID, storageprotocol, req.GetCapacityRange(), reqParameters)

	// Basic CSI parameter checking across protocols

	if len(storageprotocol) == 0 {
		return nil, status.Error(codes.InvalidArgument, "no 'storage_protocol' provided to CreateVolume")
	}
	if storageprotocol != common.PROTOCOL_FC && len(networkSpace) == 0 {
		return nil, status.Error(codes.InvalidArgument, "no 'network_space' provided to CreateVolume")
	}
	if len(volName) == 0 {
		return nil, status.Error(codes.InvalidArgument, "no name provided to CreateVolume")
	}
	if len(reqCapabilities) == 0 {
		return nil, status.Error(codes.InvalidArgument, "no VolumeCapabilities provided to CreateVolume")
	}
	error := validateCapabilities(reqCapabilities)
	if error != nil {
		return nil, status.Errorf(codes.InvalidArgument, "VolumeCapabilities invalid: %v", error)
	}
	// TODO: move non-protocol-specific capacity request validation here too, verifyVolumeSize function etc

	configparams := map[string]string{
		"nodeid":                         s.Driver.nodeID,
		"driverversion":                  s.Driver.version,
		common.SC_NFS_EXPORT_PERMISSIONS: reqParameters[common.SC_NFS_EXPORT_PERMISSIONS],
	}

	storageController, err := storage.NewStorageController(storageprotocol, configparams, req.GetSecrets())
	if err != nil || storageController == nil {
		klog.Errorf("CreateVolume error: %v", err)
		err = status.Errorf(codes.Internal, "failed to initialize storage controller while creating volume '%s'", volName)
		return nil, err
	}

	createVolResp, err = storageController.CreateVolume(ctx, req)
	if err != nil {
		klog.Errorf("CreateVolume error: %v", err)
		// it's important to return the original error, because it matches K8s expectations
		return nil, err
	} else if createVolResp == nil {
		err = status.Errorf(codes.Internal, "failed to create volume '%s', empty response", volName)
		return nil, err
	} else if createVolResp.Volume == nil {
		err = status.Errorf(codes.Internal, "failed to create volume '%s', resp: %v, no volume struct", volName, createVolResp)
		return nil, err
	} else if createVolResp.Volume.VolumeId == "" {
		err = status.Errorf(codes.Internal, "failed to create volume '%s', resp: %v, no volumeID", volName, createVolResp)
		return nil, err
	}
	createVolResp.Volume.VolumeId = createVolResp.Volume.VolumeId + "$$" + storageprotocol
	klog.V(2).Infof("CreateVolume success, resp: %v", createVolResp)
	return
}

// DeleteVolume method delete the volumne
func (s *ControllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (deleteVolResp *csi.DeleteVolumeResponse, err error) {
	volumeId := req.GetVolumeId()
	klog.V(2).Infof("DeleteVolume called with volume ID %s", volumeId)
	if volumeId == "" {
		err := fmt.Errorf("DeleteVolume error volumeId parameter was empty")
		klog.Error(err)
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	volproto, err := validateVolumeID(volumeId)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			klog.Warningf("DeleteVolume was successful. However, no volume with ID %s was not found", volumeId)
		} else {
			klog.Warningf("DeleteVolume was successful. However, validateVolumeID, using ID %s, returned an error: %v", volumeId, err)
		}
		return &csi.DeleteVolumeResponse{}, nil
	}
	config := map[string]string{
		"nodeid": s.Driver.nodeID,
	}

	storageController, err := storage.NewStorageController(volproto.StorageType, config, req.GetSecrets())
	if err != nil || storageController == nil {
		err = status.Error(codes.Internal, "failed to initialise storage controller while delete volume "+volproto.StorageType)
		return
	}
	req.VolumeId = volproto.VolumeID
	deleteVolResp, err = storageController.DeleteVolume(ctx, req)
	if err != nil {
		klog.Errorf("failed to delete volume with ID %s: %v", volumeId, err)
		return
	}
	req.VolumeId = volumeId
	return
}

// ControllerPublishVolume method
func (s *ControllerServer) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (publishVolResp *csi.ControllerPublishVolumeResponse, err error) {
	klog.V(2).Infof("ControllerPublishVolume called with request volumeID %s and nodeID %s",
		req.GetVolumeId(), req.GetNodeId())

	if req.VolumeCapability == nil {
		err = fmt.Errorf("ControllerPublishVolume request VolumeCapability was nil")
		klog.Error(err)
		err = status.Errorf(codes.InvalidArgument, err.Error())
		return
	}
	if req.GetVolumeId() == "" {
		err = fmt.Errorf("ControllerPublishVolume request volumeId was empty")
		klog.Error(err)
		err = status.Errorf(codes.InvalidArgument, err.Error())
		return
	}

	volproto, err := validateVolumeID(req.GetVolumeId())
	if err != nil {
		klog.Errorf("ControllerPublishVolume failed to validate request: %v", err)
		err = status.Errorf(codes.NotFound, "ControllerPublishVolume failed: %v", err)
		return
	}

	if req.GetNodeId() == "" {
		err = fmt.Errorf("ControllerPublishVolume request nodeId was empty")
		klog.Error(err)
		err = status.Errorf(codes.InvalidArgument, err.Error())
		return
	}

	err = validateNodeID(req.GetNodeId())
	if err != nil {
		klog.Errorf("ControllerPublishVolume failed to validate request: %v", err)
		return nil, err
	}

	config := make(map[string]string)

	storageController, err := storage.NewStorageController(volproto.StorageType, config, req.GetSecrets())
	if err != nil || storageController == nil {
		klog.Errorf("failed to create storage controller: %s", volproto.StorageType)
		err = status.Errorf(codes.Internal, "ControllerPublishVolume failed to initialise storage controller: %s", volproto.StorageType)
		return
	}
	publishVolResp, err = storageController.ControllerPublishVolume(ctx, req)
	if err != nil {
		klog.Errorf("ControllerPublishVolume failed with volume ID %s and node ID %s: %v", req.GetVolumeId(), req.GetNodeId(), err)
	}
	return
}

// ControllerUnpublishVolume method
func (s *ControllerServer) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (unpublishVolResp *csi.ControllerUnpublishVolumeResponse, err error) {
	klog.V(2).Infof("ControllerUnpublishVolume called with req volume ID %s and node ID %s", req.GetVolumeId(), req.GetNodeId())

	if req.GetVolumeId() == "" {
		err = fmt.Errorf("ControllerUnpublishVolume request volumeId parameter was empty")
		klog.Error(err)
		err = status.Errorf(codes.InvalidArgument, err.Error())
		return
	}
	volproto, err := validateVolumeID(req.GetVolumeId())
	if err != nil {
		klog.Errorf("ControllerUnpublishVolume failed to validate request: %v", err)
		err = status.Errorf(codes.NotFound, "ControllerUnpublishVolume failed: %v", err)
		return
	}

	nodeID := req.GetNodeId()
	if nodeID != "" { // NodeId is optional, when empty we should unpublish the volume from any nodes it is published to
		err = validateNodeID(nodeID)
		if err != nil {
			klog.Errorf("ControllerUnpublishVolume failed to validate request: %v", err)
			return nil, err
		}
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

func validateCapabilities(capabilities []*csi.VolumeCapability) error {
	isBlock := false
	isFile := false

	if capabilities == nil {
		return errors.New("no volume capabilities specified")
	}

	for _, capability := range capabilities {
		// validate accessMode
		accessMode := capability.GetAccessMode()
		if accessMode == nil {
			return errors.New("no accessmode specified in volume capability")
		}
		mode := accessMode.GetMode()
		// TODO: do something to actually reject invalid access modes, if any
		// there aren't any that we don't support yet, but some combinations are dumb?

		// check block and file behavior
		if block := capability.GetBlock(); block != nil {
			isBlock = true
			if mode == csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER {
				klog.Warning("MULTI_NODE_MULTI_WRITER AccessMode requested for block volume, could be dangerous")
			}
			// TODO: something about SINGLE_NODE_MULTI_WRITER (alpha feature) as well?
		}
		if file := capability.GetMount(); file != nil {
			isFile = true
			// We should validate fs_type and []mount_flags parts of MountVolume message in NFS/TreeQ controllers - CSIC-339
		}
	}

	if isBlock && isFile {
		return errors.New("both file and block volume capabilities specified")
	}

	return nil
}

func (s *ControllerServer) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (validateVolCapsResponse *csi.ValidateVolumeCapabilitiesResponse, err error) {
	klog.V(2).Infof("ValidateVolumeCapabilities called with req volumeID %s", req.GetVolumeId())

	if req.GetVolumeId() == "" {
		err := fmt.Errorf("ValidateVolumeCapabilities error volumeId parameter was empty")
		klog.Error(err)
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if req.VolumeCapabilities == nil {
		err := fmt.Errorf("ValidateVolumeCapabilities error volumeCapabilities parameter was nil")
		klog.Error(err)
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if len(req.VolumeCapabilities) == 0 {
		err := fmt.Errorf("ValidateVolumeCapabilities error volumeCapabilities parameter was empty")
		klog.Error(err)
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	volproto, err := validateVolumeID(req.GetVolumeId())
	if err != nil {
		klog.Errorf("ValidateVolumeCapabilities failed to validate request: %v", err)
		err = status.Errorf(codes.NotFound, "ValidateVolumeCapabilities failed: %v", err)
		return
	}

	config := make(map[string]string)
	storageController, err := storage.NewStorageController(volproto.StorageType, config, req.GetSecrets())
	if err != nil || storageController == nil {
		err = errors.New("error ValidateVolumeCapabilities failed to initialize storage controller: " + volproto.StorageType)
		return
	}
	validateVolCapsResponse, err = storageController.ValidateVolumeCapabilities(ctx, req)
	if err != nil {
		klog.Errorf("error ValidateVolumeCapabilities %v", err)
	}
	return
}

func (s *ControllerServer) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	klog.V(4).Infof("controller ListVolumes() called")
	res := &csi.ListVolumesResponse{
		Entries: make([]*csi.ListVolumesResponse_Entry, 0),
	}

	if req.StartingToken == "" || req.StartingToken == "next-token" {
	} else {
		err := fmt.Errorf("ListVolumes error startingToken parameter was incorrect [%s]", req.StartingToken)
		klog.Error(err)
		return nil, status.Error(codes.Aborted, err.Error())
	}

	// Get a k8s go client for in-cluster use
	cl, err := clientgo.BuildClient()
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "cannot list volumes: %v", err)
	}

	// Find PVs managed by this CSI driver
	pvList, err := cl.GetAllPersistentVolumes()
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "cannot list volumes: %v", err)
	}
	klog.V(4).Infof("pvList count: %d", len(pvList.Items))

	for _, pv := range pvList.Items {
		klog.V(4).Infof("pv capacity : %#v", pv.Spec.Capacity)
		klog.V(4).Infof("pv name: %#v", pv.ObjectMeta.GetName())
		klog.V(4).Infof("pv anno: %#v", pv.ObjectMeta.GetAnnotations()["pv.kubernetes.io/provisioned-by"])
		if pv.ObjectMeta.GetAnnotations()["pv.kubernetes.io/provisioned-by"] == common.SERVICE_NAME {
			var status csi.ListVolumesResponse_VolumeStatus
			status.PublishedNodeIds = append(status.PublishedNodeIds, pv.ObjectMeta.GetName())
			// TODO Handle csi.ListVolumesResponse_VolumeStatus.VolumeCondition?
			klog.V(4).Infof("status: %#v", status)

			var volume csi.Volume

			volume.CapacityBytes = pv.Spec.Capacity.Storage().AsDec().UnscaledBig().Int64()
			volume.VolumeId = pv.ObjectMeta.GetName()
			volume.VolumeContext = map[string]string{
				"network_space":    pv.Spec.CSI.VolumeAttributes["network_space"],
				"pool_name":        pv.Spec.CSI.VolumeAttributes["pool_name"],
				"storage_protocol": pv.Spec.CSI.VolumeAttributes["storage_protocol"],
			}
			volume.ContentSource = nil
			volume.AccessibleTopology = nil

			var entry csi.ListVolumesResponse_Entry
			entry.Volume = &volume
			entry.Status = &status
			klog.V(4).Infof("entry: %#v", entry)

			res.Entries = append(res.Entries, &entry)
		}
	}

	return res, nil

}

func (s *ControllerServer) ListSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	klog.V(4).Infof("controller ListSnapshots() called")
	res := &csi.ListSnapshotsResponse{
		Entries: make([]*csi.ListSnapshotsResponse_Entry, 0),
	}

	// Get a k8s go client for in-cluster use
	cl, err := clientgo.BuildClient()
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "cannot list snapshots: %v", err)
	}

	ns := os.Getenv("POD_NAMESPACE")
	klog.V(4).Infof("POD_NAMESPACE=%s", ns)
	if ns == "" {
		klog.Error("env var POD_NAMESPACE was not set, defaulting to openshift-operators namespace")
		ns = "openshift-operators"
	}
	secret, err := cl.GetSecret("infinibox-creds", ns)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "cannot list snapshots, error getting secret: %v", err)
	}

	x := api.ClientService{
		ConfigMap:  make(map[string]string),
		SecretsMap: secret,
	}

	clientsvc, err := x.NewClient()
	if err != nil {
		klog.Error(err)
		return nil, status.Errorf(codes.Unavailable, "cannot get api client: %v", err)
	}

	snapshots, err := clientsvc.GetAllSnapshots()
	if err != nil {
		klog.Error(err)
		return nil, status.Errorf(codes.Unavailable, "cannot list snapshots: %v", err)
	}
	klog.V(4).Infof("got back %d snapshots", len(snapshots))

	// handle the optional case where a SnapshotId is passed in the ListSnapshots request
	var volProto api.VolumeProtocolConfig
	var iValue int
	if req.SnapshotId != "" {
		volProto, err = validateVolumeID(req.SnapshotId)
		if err != nil {
			klog.Error(err)
			return nil, status.Errorf(codes.Unavailable, "cannot validate req.SnapshotId: %s error %v", req.SnapshotId, err)
		}
		iValue, err = strconv.Atoi(volProto.VolumeID)
		if err != nil {
			klog.V(4).Infof("error converting VolumeID %s", &volProto.VolumeID)
			klog.Error(err)
			return nil, status.Errorf(codes.Unavailable, "cannot convert VolumeID: %s error %v", volProto.VolumeID, err)
		}
	}

	for i := 0; i < len(snapshots); i++ {
		cdt := snapshots[i].CreatedAt / 1000
		tt := time.Unix(cdt, 0)
		t := tspb.New(tt)
		if err != nil {
			klog.Error(err)
		}

		var parentName string
		switch snapshots[i].DatasetType {
		case "VOLUME":
			v, err := clientsvc.GetVolume(snapshots[i].ParentId)
			if err != nil {
				klog.Errorf("snapshot %s VOLUME parentId %d error %s", snapshots[i].Name, snapshots[i].ParentId, err.Error())
				parentName = "unknown"
			} else {
				parentName = v.Name
			}
		case "FILESYSTEM":
			f, err := clientsvc.GetFileSystemByID(int64(snapshots[i].ParentId))
			if err != nil {
				klog.Errorf("snapshot %s FILESYSTEM parentId %d error %s", snapshots[i].Name, snapshots[i].ParentId, err.Error())
				parentName = "unknown"
			} else {
				parentName = f.Name
			}
		default:
			klog.Errorf("snapshot %s unknown dataset type %s parentId %d ", snapshots[i].Name, snapshots[i].DatasetType, snapshots[i].ParentId)
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

		if req.SourceVolumeId != "" {
			if req.SourceVolumeId == entry.Snapshot.SourceVolumeId {
				klog.V(4).Infof("comparing req.SourceVolumeId %s SourceVolumeId %s\n", req.SourceVolumeId, entry.Snapshot.SourceVolumeId)
				res.Entries = append(res.Entries, &entry)
			}
		} else if req.SnapshotId != "" {
			if iValue == snapshots[i].ID {
				klog.V(4).Infof("req.SnapshotID contains %s found matching snapshot with ID %d name %s\n", req.SnapshotId, snapshots[i].ID, snapshots[i].Name)
				entry.Snapshot.SnapshotId = req.SnapshotId
				res.Entries = append(res.Entries, &entry)
			}
		} else {
			res.Entries = append(res.Entries, &entry)
		}

	}
	return res, nil
}

func (s *ControllerServer) GetCapacity(ctx context.Context, req *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (s *ControllerServer) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	return &csi.ControllerGetCapabilitiesResponse{
		Capabilities: []*csi.ControllerServiceCapability{
			{
				Type: &csi.ControllerServiceCapability_Rpc{
					Rpc: &csi.ControllerServiceCapability_RPC{
						Type: csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
					},
				},
			},
			{
				Type: &csi.ControllerServiceCapability_Rpc{
					Rpc: &csi.ControllerServiceCapability_RPC{
						Type: csi.ControllerServiceCapability_RPC_LIST_VOLUMES,
					},
				},
			},
			{
				Type: &csi.ControllerServiceCapability_Rpc{
					Rpc: &csi.ControllerServiceCapability_RPC{
						Type: csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT,
					},
				},
			},
			{
				Type: &csi.ControllerServiceCapability_Rpc{
					Rpc: &csi.ControllerServiceCapability_RPC{
						Type: csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME,
					},
				},
			},
			{
				Type: &csi.ControllerServiceCapability_Rpc{
					Rpc: &csi.ControllerServiceCapability_RPC{
						Type: csi.ControllerServiceCapability_RPC_CLONE_VOLUME,
					},
				},
			},
			{
				Type: &csi.ControllerServiceCapability_Rpc{
					Rpc: &csi.ControllerServiceCapability_RPC{
						Type: csi.ControllerServiceCapability_RPC_LIST_SNAPSHOTS,
					},
				},
			},
			{
				Type: &csi.ControllerServiceCapability_Rpc{
					Rpc: &csi.ControllerServiceCapability_RPC{
						Type: csi.ControllerServiceCapability_RPC_EXPAND_VOLUME,
					},
				},
			},
		},
	}, nil
}

func (s *ControllerServer) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (createSnapshotResp *csi.CreateSnapshotResponse, err error) {

	klog.V(2).Infof("Create Snapshot called with volume Id %s", req.GetSourceVolumeId())
	volproto, err := validateVolumeID(req.GetSourceVolumeId())
	if err != nil {
		klog.Errorf("failed to validate storage type %v", err)
		return nil, status.Errorf(codes.InvalidArgument, "Failed to validate source Vol Id: %s", err.Error())
	}
	config := map[string]string{
		"nodeid": s.Driver.nodeID,
	}
	storageController, err := storage.NewStorageController(volproto.StorageType, config, req.GetSecrets())
	if err != nil {
		klog.Errorf("Create snapshot failed: %s", err)
		return nil, err
	}
	if storageController != nil {
		createSnapshotResp, err = storageController.CreateSnapshot(ctx, req)
		return createSnapshotResp, err
	}
	return nil, errors.New("failed to create storageController for " + volproto.StorageType)
}

func (s *ControllerServer) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (deleteSnapshotResp *csi.DeleteSnapshotResponse, err error) {

	snapshotID := req.GetSnapshotId()
	klog.V(2).Infof("DeleteSnapshot called with snapshot Id %s", snapshotID)
	volproto, err := validateVolumeID(snapshotID)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			klog.Errorf("snapshot ID: '%s' not found, err: %v - return success", snapshotID, err)
			return &csi.DeleteSnapshotResponse{}, nil
		} else {
			klog.Errorf("snapshot ID: '%s' invalid, err: %v", snapshotID, err)
			return nil, err
		}
	}

	config := map[string]string{
		"nodeid": s.Driver.nodeID,
	}
	storageController, err := storage.NewStorageController(volproto.StorageType, config, req.GetSecrets())
	if err != nil {
		klog.Errorf("delete snapshot failed: %s", err)
		return nil, err
	}
	if storageController != nil {
		req.SnapshotId = volproto.VolumeID
		deleteSnapshotResp, err := storageController.DeleteSnapshot(ctx, req)
		return deleteSnapshotResp, err
	}
	return nil, errors.New("failed to create storageController for " + volproto.StorageType)
}

func (s *ControllerServer) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (expandVolResp *csi.ControllerExpandVolumeResponse, err error) {

	err = validateExpandVolumeRequest(req)
	if err != nil {
		return
	}

	configparams := map[string]string{
		"nodeid": s.Driver.nodeID,
	}
	volproto, err := validateVolumeID(req.GetVolumeId())
	if err != nil {
		return
	}

	storageController, err := storage.NewStorageController(volproto.StorageType, configparams, req.GetSecrets())
	if err != nil {
		klog.Errorf("expand volume failed: %s", err)
		return
	}
	if storageController != nil {
		req.VolumeId = volproto.VolumeID
		expandVolResp, err = storageController.ControllerExpandVolume(ctx, req)
		return expandVolResp, err
	}
	return
}

func (s *ControllerServer) ControllerGetVolume(
	_ context.Context, _ *csi.ControllerGetVolumeRequest,
) (*csi.ControllerGetVolumeResponse, error) {

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
func validateVolumeID(str string) (volprotoconf api.VolumeProtocolConfig, err error) {
	if str == "" {
		return volprotoconf, status.Error(codes.InvalidArgument, "volume Id empty")
	}
	volproto := strings.Split(str, "$$")
	if len(volproto) != 2 {
		return volprotoconf, status.Error(codes.NotFound, "volume Id does not follow '<id>$$<proto>' pattern")
	}
	klog.V(2).Infof("volproto: %s", volproto)
	volprotoconf.VolumeID = volproto[0]
	volprotoconf.StorageType = volproto[1]
	return volprotoconf, nil
}
