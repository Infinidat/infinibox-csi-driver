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
package storage

import (
	"context"
	"errors"
	"fmt"
	"infinibox-csi-driver/api"
	"infinibox-csi-driver/common"
	"strconv"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"k8s.io/klog/v2"
)

const (
	// TOBEDELETED status
	TOBEDELETED          = "host.k8s.to_be_deleted"
	StandardMountOptions = "vers=3,tcp,rsize=262144,wsize=262144"
)

// NFSVolumeServiceType servier type
type NfsVolumeServiceType interface {
	CreateNFSVolume() (*infinidatVolume, error)
	DeleteNFSVolume() error
}

type infinidatVolume struct {
	VolName       string     `json:"volName"`
	VolID         string     `json:"volID"`
	VolSize       int64      `json:"volSize"`
	VolPath       string     `json:"volPath"`
	IpAddress     string     `json:"ipAddress"`
	VolAccessType accessType `json:"volAccessType"`
	Ephemeral     bool       `json:"ephemeral"`
	ExportID      int64      `json:"exportID"`
	FileSystemID  int64      `json:"fileSystemID"`
	ExportBlock   string     `json:"exportBlock"`
}

type accessType int

const (
	// InfiniBox default values
	NfsExportPermissions = "RW"
	NoRootSquash         = true
	NfsUnixPermissions   = "777"
)

func (nfs *nfsstorage) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	klog.V(2).Infof("Creating Volume of nfs protocol")
	var err error
	// Adding the the request parameter into Map config
	config := req.GetParameters()
	pvName := req.GetName()

	klog.V(4).Infof(" csi request parameters %v", config)

	capacity, err := nfsSanityCheck(req, map[string]string{
		common.SC_POOL_NAME:     `\A.*\z`, // TODO: could make this enforce IBOX pool_name requirements, but probably not necessary
		common.SC_NETWORK_SPACE: `\A.*\z`, // TODO: could make this enforce IBOX network_space requirements, but probably not necessary
	}, nil, nfs.cs.Api)
	if err != nil {
		klog.Error(err)
		return nil, err
	}

	usePrivilegedPorts := false
	usePrivilegedPortsString := config[common.SC_PRIV_PORTS]
	if usePrivilegedPortsString != "" {
		usePrivilegedPorts, err = strconv.ParseBool(usePrivilegedPortsString)
		if err != nil {
			e := fmt.Errorf("invalid NFS privileged_ports_only value: %s, error: %v", usePrivilegedPortsString, err)
			klog.Error(e)
			return nil, status.Error(codes.InvalidArgument, e.Error())
		}
	}
	klog.V(4).Infof("Using privileged ports only: %t", usePrivilegedPorts)

	snapdirVisible := false
	snapdirVisibleString := config[common.SC_SNAPDIR_VISIBLE]
	if snapdirVisibleString != "" {
		snapdirVisible, err = strconv.ParseBool(snapdirVisibleString)
		if err != nil {
			e := fmt.Errorf("invalid NFS snapdir_visible value: %s, error: %v", snapdirVisibleString, err)
			klog.Error(e)
			return nil, e
		}
	}
	klog.V(4).Infof("Snapshot directory is visible: %t", snapdirVisible)

	nfs.pVName = pvName
	nfs.storageClassParameters = config
	nfs.capacity = capacity
	nfs.usePrivilegedPorts = usePrivilegedPorts
	nfs.snapdirVisible = snapdirVisible
	nfs.exportPath = "/" + pvName
	ipAddress, err := nfs.cs.getNetworkSpaceIP(strings.Trim(config[common.SC_NETWORK_SPACE], " "))
	if err != nil {
		klog.Error(err)
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	nfs.ipAddress = ipAddress
	klog.V(4).Infof("getNetworkSpaceIP ipAddress %s", nfs.ipAddress)

	// check if volume with given name already exists
	volume, err := nfs.cs.Api.GetFileSystemByName(pvName)
	if err != nil {
		klog.Error(err)
	}
	if err != nil && !strings.EqualFold(err.Error(), "filesystem with given name not found") {
		klog.V(4).Infof("CreateVolume - GetFileSystemByName error: %v", err)
		return nil, status.Errorf(codes.NotFound, "error CreateVolume failed: %v", err)
	}
	if volume != nil {
		// return existing volume
		nfs.fileSystemID = volume.ID
		exportArray, err := nfs.cs.Api.GetExportByFileSystem(nfs.fileSystemID)
		if err != nil {
			klog.Error(err)
			return nil, status.Errorf(codes.Internal, "error CreateVolume failed: %v", err)
		}
		if exportArray == nil {
			return nil, status.Errorf(codes.NotFound, "error CreateVolume failed: %v", err)
		}
		if capacity != volume.Size {
			err = status.Errorf(codes.AlreadyExists, "error CreateVolume failed: volume exists but has different size")
			klog.Errorf("capacity %d volume: %+v", capacity, volume)
			return nil, err
		}
		for _, export := range *exportArray {
			nfs.exportBlock = export.ExportPath
			nfs.exportID = export.ID
			break
		}
		return nfs.getNfsCsiResponse(req), nil
	}

	// Volume content source support Volumes and Snapshots
	contentSource := req.GetVolumeContentSource()
	klog.V(4).Infof("content volume source: %v", contentSource)
	var csiResp *csi.CreateVolumeResponse
	if contentSource != nil {
		if contentSource.GetSnapshot() != nil {
			snapshot := req.GetVolumeContentSource().GetSnapshot()
			csiResp, err = nfs.createVolumeFromPVCSource(req, capacity, config[common.SC_POOL_NAME], snapshot.GetSnapshotId())
			if err != nil {
				klog.Errorf("failed to create volume from snapshot with error: %v", err)
				return nil, err
			}
		} else if contentSource.GetVolume() != nil {
			volume := req.GetVolumeContentSource().GetVolume()
			csiResp, err = nfs.createVolumeFromPVCSource(req, capacity, config[common.SC_POOL_NAME], volume.GetVolumeId())
			if err != nil {
				klog.Errorf("failed to create volume from pvc with error: %v", err)
				return nil, err
			}
		}
	} else {
		csiResp, err = nfs.CreateNFSVolume(req)
		if err != nil {
			klog.Errorf("failed to create volume, %v", err)
			return nil, err
		}
	}
	return csiResp, nil
}

func (nfs *nfsstorage) createVolumeFromPVCSource(req *csi.CreateVolumeRequest, size int64, storagePool string, srcVolumeID string) (csiResp *csi.CreateVolumeResponse, err error) {
	klog.V(4).Infof("createVolumeFromPVCSource")

	volproto, err := validateVolumeID(srcVolumeID)
	if err != nil || volproto.VolumeID == "" {
		klog.Errorf("failed to validate volume id: %s, err: %v", srcVolumeID, err)
		return nil, status.Errorf(codes.NotFound, "invalid source volume id format: %s", srcVolumeID)
	}
	sourceVolumeID, err := strconv.ParseInt(volproto.VolumeID, 10, 64)
	if err != nil {
		klog.Error(err)
		return nil, status.Errorf(codes.InvalidArgument, "invalid source volume volume id (non-numeric): %s", volproto.VolumeID)
	}

	// Look up the source volume
	srcfsys, err := nfs.cs.Api.GetFileSystemByID(sourceVolumeID)
	if err != nil {
		klog.Error(err)
		return nil, status.Errorf(codes.NotFound, "volume not found: %d", sourceVolumeID)
	}

	// Check that the requested volume size matches the size of source volume
	if srcfsys.Size != size {
		return nil, status.Errorf(codes.InvalidArgument,
			"volume %d, invalid size %d, requested %d ", sourceVolumeID, srcfsys.Size, size)
	}

	// Check that the requested storagePool matches the source
	storagePoolID, err := nfs.cs.Api.GetStoragePoolIDByName(storagePool)
	if err != nil {
		klog.Error(err)
		return nil, status.Errorf(codes.InvalidArgument, "error GetStoragePoolIDByName: %s", storagePool)
	}
	if storagePoolID != srcfsys.PoolID {
		return nil, status.Errorf(codes.InvalidArgument,
			"source storagepool id differs from requested: %s", storagePool)
	}

	newSnapshotName := req.GetName() // create snapshot using the original CreateVolumeRequest
	newSnapshotParams := &api.FileSystemSnapshot{ParentID: sourceVolumeID, SnapshotName: newSnapshotName, WriteProtected: false}
	klog.V(4).Infof("CreateFileSystemSnapshot: %v", newSnapshotParams)
	// Create snapshot
	newSnapshot, err := nfs.cs.Api.CreateFileSystemSnapshot(newSnapshotParams)
	if err != nil {
		e := fmt.Errorf("failed to create snapshot: %s error: %v", newSnapshotParams.SnapshotName, err)
		klog.Error(e)
		return nil, status.Errorf(codes.Internal, e.Error())
	}
	klog.V(4).Infof("createVolumeFrmPVCSource successfully created volume from clone with name: %s", newSnapshotName)
	nfs.fileSystemID = newSnapshot.SnapshotID

	err = nfs.createExportPathAndAddMetadata()
	if err != nil {
		klog.Errorf("failed to create export and metadata, %v", err)
		return nil, err
	}
	return nfs.getNfsCsiResponse(req), nil
}

// CreateNFSVolume create volume method
func (nfs *nfsstorage) CreateNFSVolume(req *csi.CreateVolumeRequest) (csiResp *csi.CreateVolumeResponse, err error) {
	validnwlist, err := nfs.cs.Api.OneTimeValidation(nfs.storageClassParameters[common.SC_POOL_NAME], nfs.storageClassParameters[common.SC_NETWORK_SPACE])
	if err != nil {
		klog.Error(err)
		return nil, err
	}
	nfs.storageClassParameters[common.SC_NETWORK_SPACE] = validnwlist
	klog.V(4).Infof("networkspace validation success")

	err = nfs.createFileSystem(nfs.pVName)
	if err != nil {
		klog.Errorf("failed to create file system, %v", err)
		return nil, err
	}
	err = nfs.createExportPathAndAddMetadata()
	if err != nil {
		klog.Errorf("failed to create export and metadata, %v", err)
		return nil, err
	}
	return nfs.getNfsCsiResponse(req), nil
}

func (nfs *nfsstorage) createExportPathAndAddMetadata() (err error) {
	defer func() {
		if err != nil && nfs.fileSystemID != 0 {
			klog.V(4).Infof("seems to be some problem reverting filesystem: %s", nfs.pVName)
			if _, errDelFS := nfs.cs.Api.DeleteFileSystem(nfs.fileSystemID); errDelFS != nil {
				klog.Errorf("failed to delete file system id: %d %v", nfs.fileSystemID, errDelFS)
			}
		}
	}()

	if nfs.storageClassParameters[common.SC_NFS_EXPORT_PERMISSIONS] == "" {
		klog.V(4).Info("nfs_export_permissions parameter is not set in the StorageClass, will use default export")
	} else {
		err = nfs.createExportPath()
		if err != nil {
			klog.Errorf("failed to export path %v", err)
			return
		}
		klog.V(4).Infof("export path created for filesytem: %s", nfs.pVName)
	}

	defer func() {
		if err != nil && nfs.exportID != 0 {
			klog.V(4).Infof("seems to be some problem reverting created export id: %d", nfs.exportID)
			if _, errDelExport := nfs.cs.Api.DeleteExportPath(nfs.exportID); errDelExport != nil {
				klog.Errorf("failed to delete export path for file system id: %d %v", nfs.fileSystemID, errDelExport)
			}
		}
	}()

	metadata := map[string]interface{}{
		"host.k8s.pvname": nfs.pVName,
		"host.created_by": nfs.cs.GetCreatedBy(),
	}

	_, err = nfs.cs.Api.AttachMetadataToObject(nfs.fileSystemID, metadata)
	if err != nil {
		klog.Errorf("failed to attach metadata for file system %s, %v", nfs.pVName, err)
		return
	}
	klog.V(4).Infof("metadata attached successfully for file system %s", nfs.pVName)
	return
}

func (nfs *nfsstorage) createExportPath() (err error) {
	permissionsMapArray, err := getPermissionMaps(nfs.storageClassParameters[common.SC_NFS_EXPORT_PERMISSIONS])
	if err != nil {
		klog.Errorf("failed to parse permission map string %s %v", nfs.storageClassParameters[common.SC_NFS_EXPORT_PERMISSIONS], err)
		return err
	}

	exportFileSystem := api.ExportFileSys{
		FilesystemID:        nfs.fileSystemID,
		Transport_protocols: "TCP",
		Privileged_port:     nfs.usePrivilegedPorts,
		SnapdirVisible:      nfs.snapdirVisible,
		Export_path:         nfs.exportPath,
	}
	exportFileSystem.Permissionsput = append(exportFileSystem.Permissionsput, permissionsMapArray...)
	var exportResp *api.ExportResponse
	exportResp, err = nfs.cs.Api.ExportFileSystem(exportFileSystem)
	if err != nil {
		klog.Errorf("failed to create export path of filesystem %s %v", nfs.pVName, err)
		return err
	}
	nfs.exportID = exportResp.ID
	nfs.exportBlock = exportResp.ExportPath
	klog.V(4).Infof("created nfs export for PV '%s', snapdirVisible: %t", nfs.pVName, nfs.snapdirVisible)
	return err
}

func (nfs *nfsstorage) createFileSystem(fileSystemName string) (err error) {
	namepool := nfs.storageClassParameters[common.SC_POOL_NAME]
	poolID, err := nfs.cs.Api.GetStoragePoolIDByName(namepool)
	if err != nil {
		klog.Errorf("failed to get GetPoolID by pool_name %s %v", namepool, err)
		return err
	}
	ssdEnabled := nfs.storageClassParameters[common.SC_SSD_ENABLED]
	if ssdEnabled == "" {
		ssdEnabled = fmt.Sprint(false)
	}
	ssd, _ := strconv.ParseBool(ssdEnabled)
	mapRequest := map[string]interface{}{
		"pool_id":             poolID,
		"name":                fileSystemName,
		common.SC_SSD_ENABLED: ssd,
		"provtype":            strings.ToUpper(nfs.storageClassParameters[common.SC_PROVISION_TYPE]),
		"size":                nfs.capacity,
	}
	fileSystem, err := nfs.cs.Api.CreateFilesystem(mapRequest)
	if err != nil {
		klog.Errorf("failed to create filesystem %s %v", fileSystemName, err)
		return err
	}
	nfs.fileSystemID = fileSystem.ID
	klog.V(4).Infof("filesystem Created %s", fileSystemName)
	return err
}

func (nfs *nfsstorage) getNfsCsiResponse(req *csi.CreateVolumeRequest) *csi.CreateVolumeResponse {
	infinidatVol := &infinidatVolume{
		VolID:        fmt.Sprint(nfs.fileSystemID),
		VolName:      nfs.pVName,
		VolSize:      nfs.capacity,
		VolPath:      nfs.exportPath,
		IpAddress:    nfs.ipAddress,
		ExportID:     nfs.exportID,
		ExportBlock:  nfs.exportBlock,
		FileSystemID: nfs.fileSystemID,
	}
	nfs.storageClassParameters["ipAddress"] = (*infinidatVol).IpAddress
	nfs.storageClassParameters["exportID"] = strconv.Itoa(int((*infinidatVol).ExportID))
	nfs.storageClassParameters["volPathd"] = (*infinidatVol).VolPath

	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      (*infinidatVol).VolID,
			CapacityBytes: nfs.capacity,
			VolumeContext: nfs.storageClassParameters,
			ContentSource: req.GetVolumeContentSource(),
		},
	}
}

func (nfs *nfsstorage) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	if len(req.GetVolumeId()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "volume ID missing in request")
	}

	volumeID := req.GetVolumeId()
	volID, err := strconv.ParseInt(volumeID, 10, 64)
	if err != nil {
		klog.Errorf("invalid Volume ID %v", err)
		return nil, err
	}

	nfs.uniqueID = volID
	nfsDeleteErr := nfs.DeleteNFSVolume()
	if nfsDeleteErr != nil {
		klog.Error(nfsDeleteErr)
		if strings.Contains(nfsDeleteErr.Error(), "FILESYSTEM_NOT_FOUND") {
			klog.Errorf("file system already delete from infinibox")
			return &csi.DeleteVolumeResponse{}, nil
		}
		klog.Errorf("failed to delete NFS Volume ID %s, %v", volumeID, nfsDeleteErr)
		return nil, nfsDeleteErr
	}
	klog.V(4).Infof("volume %s successfully deleted", volumeID)
	return &csi.DeleteVolumeResponse{}, nil
}

// DeleteNFSVolume delete volume method
func (nfs *nfsstorage) DeleteNFSVolume() (err error) {

	_, fileSystemErr := nfs.cs.Api.GetFileSystemByID(nfs.uniqueID)
	if fileSystemErr != nil {
		klog.Errorf("failed to get file system by ID %d %v", nfs.uniqueID, fileSystemErr)
		err = fileSystemErr
		return
	}
	hasChild := nfs.cs.Api.FileSystemHasChild(nfs.uniqueID)
	if hasChild {
		metadata := map[string]interface{}{
			TOBEDELETED: true,
		}
		_, err = nfs.cs.Api.AttachMetadataToObject(nfs.uniqueID, metadata)
		if err != nil {
			klog.Errorf("failed to update host.k8s.to_be_deleted for filesystem %s error: %v", nfs.pVName, err)
			err = errors.New("error while Set metadata host.k8s.to_be_deleted")
		}
		return
	}

	parentID := nfs.cs.Api.GetParentID(nfs.uniqueID)
	err = nfs.cs.Api.DeleteFileSystemComplete(nfs.uniqueID)
	if err != nil {
		klog.Errorf("failed to delete filesystem %s error: %v", nfs.pVName, err)
		err = errors.New("error while delete file system")
	}
	if parentID != 0 {
		err = nfs.cs.Api.DeleteParentFileSystem(parentID)
		if err != nil {
			klog.Errorf("failed to delete filesystem's %s parent filesystems error: %v", nfs.pVName, err)
		}

	}
	return
}

type ExportPermission struct {
	Access         string
	No_Root_Squash bool
	Client         string
}

func (nfs *nfsstorage) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	var err error
	volumeID := req.GetVolumeId()
	exportID := req.GetVolumeContext()["exportID"]

	klog.V(2).Infof("ControllerPublishVolume nodeId %s volumeID %s exportID %s nfs_export_permissions %s",
		req.GetNodeId(), volumeID, exportID, req.GetVolumeContext()[common.SC_NFS_EXPORT_PERMISSIONS])

	// TODO: revisit this as part of CSIC-343
	_, err = nfs.cs.AccessModesHelper.IsValidAccessModeNfs(req)
	if err != nil {
		klog.Error(err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	if req.GetVolumeContext()[common.SC_NFS_EXPORT_PERMISSIONS] == "" {
		klog.V(4).Infof("nfs_export_permissions parameter not set, volume ID %s export ID %s", volumeID, exportID)
		return &csi.ControllerPublishVolumeResponse{}, nil
	}

	// proceed to create a default export rule using the Node ip address

	exportPermissionMapArray, err := getPermissionMaps(req.GetVolumeContext()[common.SC_NFS_EXPORT_PERMISSIONS])
	if err != nil {
		klog.Errorf("failed to retrieve permission maps, %v", err)
		return nil, status.Error(codes.Internal, err.Error())
	}
	klog.V(4).Infof("nfs export permissions for volume ID %s and export ID %s: %v", volumeID, exportID, exportPermissionMapArray)

	access := ""
	if len(exportPermissionMapArray) > 0 {
		access = exportPermissionMapArray[0]["access"].(string)
	}

	noRootSquash := true // default value
	nodeNameIP := strings.Split(req.GetNodeId(), "$$")
	if len(nodeNameIP) != 2 {
		return nil, errors.New("not found Node ID")
	}
	nodeIP := nodeNameIP[1]
	exportid, _ := strconv.Atoi(exportID)
	_, err = nfs.cs.Api.AddNodeInExport(exportid, access, noRootSquash, nodeIP)
	if err != nil {
		klog.Errorf("failed to add export rule, %v", err)
		return nil, status.Errorf(codes.Internal, "failed to add export rule  %s", err)
	}

	return &csi.ControllerPublishVolumeResponse{}, nil
}

func (nfs *nfsstorage) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	klog.V(2).Infof("ControllerUnpublishVolume")
	voltype := req.GetVolumeId()
	volproto := strings.Split(voltype, "$$")
	fileID, _ := strconv.ParseInt(volproto[0], 10, 64)
	err := nfs.cs.Api.DeleteExportRule(fileID, req.GetNodeId())
	if err != nil {
		klog.Errorf("failed to delete Export Rule fileystemID %d error %v", fileID, err)
		return nil, status.Errorf(codes.Internal, "failed to delete Export Rule  %v", err)
	}
	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

func (nfs *nfsstorage) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (resp *csi.ValidateVolumeCapabilitiesResponse, err error) {
	klog.V(2).Infof("ValidateVolumeCapabilities called with volumeId %s", req.GetVolumeId())
	volproto, err := validateVolumeID(req.GetVolumeId())
	if err != nil {
		klog.Errorf("failed to validate storage type: %v", err)
		return nil, status.Errorf(codes.InvalidArgument, "invalid volume id format: %s", req.GetVolumeId())
	}
	volID, err := strconv.ParseInt(volproto.VolumeID, 10, 64)
	if err != nil {
		klog.Errorf("failed to validate volume id: %v", err)
		return nil, status.Errorf(codes.InvalidArgument, "invalid volume id (non-numeric): %s", req.GetVolumeId())
	}

	klog.V(4).Infof("volID: %d", volID)
	fs, err := nfs.cs.Api.GetFileSystemByID(volID)
	if err != nil {
		klog.Errorf("failed to find volume ID: %d, %v", volID, err)
		err = status.Errorf(codes.NotFound, "ValidateVolumeCapabilities failed to find volume ID: %d, %v", volID, err)
	}
	klog.V(4).Infof("volID: %d volume: %v", volID, fs)

	// TODO: revisit this as part of CSIC-343
	// _, err = nfs.cs.accessModesHelper.IsValidAccessMode(fs, req)
	// if err != nil {
	//     return nil, status.Error(codes.InvalidArgument, err.Error())
	// }

	resp = &csi.ValidateVolumeCapabilitiesResponse{
		Confirmed: &csi.ValidateVolumeCapabilitiesResponse_Confirmed{
			VolumeCapabilities: req.GetVolumeCapabilities(),
		},
	}
	return
}

func (nfs *nfsstorage) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	return &csi.ListVolumesResponse{}, nil
}

func (nfs *nfsstorage) ListSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	return &csi.ListSnapshotsResponse{}, nil
}

func (nfs *nfsstorage) GetCapacity(ctx context.Context, req *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error) {
	return &csi.GetCapacityResponse{}, nil
}

func (nfs *nfsstorage) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	return &csi.ControllerGetCapabilitiesResponse{}, nil
}

func (nfs *nfsstorage) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (createSnapshot *csi.CreateSnapshotResponse, err error) {
	// var ts *timestamp.Timestamp
	var snapshotID string
	snapshotName := req.GetName()
	srcVolumeId := req.GetSourceVolumeId()
	klog.V(2).Infof("called CreateSnapshot source volume Id '%s' snapshot name %s", srcVolumeId, snapshotName)
	volproto, err := validateVolumeID(srcVolumeId)
	if err != nil {
		klog.Errorf("failed to validate storage type for volume %s, %v", srcVolumeId, err)
		return
	}

	sourceFilesystemID, _ := strconv.ParseInt(volproto.VolumeID, 10, 64)
	snapshotArray, err := nfs.cs.Api.GetSnapshotByName(snapshotName)
	if err != nil {
		klog.Errorf("error GetSnapshotByName %s, %v", volproto.VolumeID, err)
		return
	}
	if len(*snapshotArray) > 0 {
		for _, snap := range *snapshotArray {
			if snap.ParentId == sourceFilesystemID {
				snapshotID = strconv.FormatInt(snap.SnapshotID, 10) + "$$" + volproto.StorageType
				klog.V(4).Infof("snapshot: %s src fs id: %d exists, snapshot id: %d", snapshotName, snap.ParentId, snap.SnapshotID)
				return &csi.CreateSnapshotResponse{
					Snapshot: &csi.Snapshot{
						SizeBytes:      snap.Size,
						SnapshotId:     snapshotID,
						SourceVolumeId: srcVolumeId,
						CreationTime:   timestamppb.Now(),
						ReadyToUse:     true,
					},
				}, nil
			} else {
				klog.V(4).Infof("Snapshot: %s snapshot id: %d src fs id: %d (requested: %d)",
					snapshotName, snap.ParentId, snap.SnapshotID, sourceFilesystemID)
			}
		}
		return nil, status.Error(codes.AlreadyExists, "snapshot with already existing name and different source volume ID")
	}

	fileSystemSnapshot := &api.FileSystemSnapshot{
		ParentID:       sourceFilesystemID,
		SnapshotName:   snapshotName,
		WriteProtected: true,
	}

	resp, err := nfs.cs.Api.CreateFileSystemSnapshot(fileSystemSnapshot)
	if err != nil {
		klog.Errorf("failed to create snapshot %s error %v", snapshotName, err)
		return
	}

	snapshotID = strconv.FormatInt(resp.SnapshotID, 10) + "$$" + volproto.StorageType
	snapshot := &csi.Snapshot{
		SnapshotId:     snapshotID,
		SourceVolumeId: srcVolumeId,
		ReadyToUse:     true,
		CreationTime:   timestamppb.Now(),
		SizeBytes:      resp.Size,
	}
	klog.V(4).Infof("CreateFileSystemSnapshot resp: %v", snapshot)
	snapshotResp := &csi.CreateSnapshotResponse{Snapshot: snapshot}
	return snapshotResp, nil
}

func (nfs *nfsstorage) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (deleteSnapshot *csi.DeleteSnapshotResponse, err error) {

	snapshotID, _ := strconv.ParseInt(req.GetSnapshotId(), 10, 64)
	nfs.uniqueID = snapshotID
	nfsSnapDeleteErr := nfs.DeleteNFSVolume()
	if nfsSnapDeleteErr != nil {
		klog.Error(nfsSnapDeleteErr)
		if strings.Contains(nfsSnapDeleteErr.Error(), "FILESYSTEM_NOT_FOUND") {
			klog.Errorf("snapshot already delete from infinibox")
			deleteSnapshot = &csi.DeleteSnapshotResponse{}
			return
		}
		klog.Errorf("failed to delete snapshot, %v", nfsSnapDeleteErr)
		err = nfsSnapDeleteErr
		return
	}
	deleteSnapshot = &csi.DeleteSnapshotResponse{}
	return
}

func (nfs *nfsstorage) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (expandVolume *csi.ControllerExpandVolumeResponse, err error) {
	klog.V(2).Infof("ControllerExpandVolume")

	ID, err := strconv.ParseInt(req.GetVolumeId(), 10, 64)
	if err != nil {
		klog.Errorf("invalid Volume ID %v", err)
		return
	}

	capacity := int64(req.GetCapacityRange().GetRequiredBytes())
	if capacity < gib {
		capacity = gib
		klog.Warningf("volume Minimum capacity should be greater than %d", gib)
	}

	// Expand file system size
	var fileSys api.FileSystem
	fileSys.Size = capacity
	_, err = nfs.cs.Api.UpdateFilesystem(ID, fileSys)
	if err != nil {
		klog.Errorf("failed to update file system %v", err)
		return
	}
	return &csi.ControllerExpandVolumeResponse{
		CapacityBytes:         capacity,
		NodeExpansionRequired: false,
	}, nil
}
