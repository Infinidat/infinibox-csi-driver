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
package storage

import (
	"context"
	"errors"
	"fmt"
	"infinibox-csi-driver/api"
	"strconv"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/protobuf/ptypes"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog"
)

const (
	//TOBEDELETED status
	TOBEDELETED = "host.k8s.to_be_deleted"
)

// NFSVolumeServiceType servier type
type NfsVolumeServiceType interface {
	CreateNFSVolume() (*infinidatVolume, error)
	DeleteNFSVolume() error
}

type infinidat struct {
	name              string
	nodeID            string
	version           string
	endpoint          string
	ephemeral         bool
	maxVolumesPerNode int64
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
type MetaData struct {
	pVName    string
	k8sVer    string
	namespace string
	pvcId     string
	pvcName   string
	pvname    string
}

type accessType int

const (
	mountAccess accessType = iota
	blockAccess

	//Infinibox default values
	//Ibox max allowed filesystem
	MaxFileSystemAllowed = 4000
	MountOptions         = "hard,rsize=1024,wsize=1024"
	NfsExportPermissions = "RW"
	NoRootSquash         = true
	NfsUnixPermissions   = "777"

	// for size conversion
	kib    int64 = 1024
	mib    int64 = kib * 1024
	gib    int64 = mib * 1024
	gib100 int64 = gib * 100
	tib    int64 = gib * 1024
	tib100 int64 = tib * 100
)

func validateParameter(config map[string]string) (bool, map[string]string) {
	compulsaryFields := []string{"pool_name", "network_space", "nfs_export_permissions"} //TODO: add remaining paramters
	validationStatus := true
	validationStatusMap := make(map[string]string)
	for _, param := range compulsaryFields {
		if config[param] == "" {
			validationStatusMap[param] = param + " value missing"
			validationStatus = false
		}
	}
	klog.V(4).Infof("parameter Validation completed")
	return validationStatus, validationStatusMap
}

func (nfs *nfsstorage) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (csiResp *csi.CreateVolumeResponse, err error) {
	klog.V(4).Infof("Creating Volume of nfs protocol")
	//Adding the the request parameter into Map config
	config := req.GetParameters()
	pvName := req.GetName()

	klog.V(4).Infof("Creating fileystem %s of nfs protocol ", pvName)
	validationStatus, validationStatusMap := validateParameter(config)
	if !validationStatus {
		klog.Errorf("failed to validate parameter for nfs protocol, %v", validationStatusMap)
		return nil, status.Error(codes.InvalidArgument, "failed to validate parameter for nfs protocol")
	}
	klog.V(4).Infof("fileystem %s ,parameter validation success", pvName)

	capacity := int64(req.GetCapacityRange().GetRequiredBytes())
	if capacity < gib { //INF90
		capacity = gib
		klog.Warningf("Volume Minimum capacity should be greater than %d", gib)
	}

	// Privileged ports only
	usePrivilegedPortsString := config["privileged_ports_only"]
	if usePrivilegedPortsString == "" {
		usePrivilegedPortsString = "false"
	}
	usePrivilegedPorts, err := strconv.ParseBool(usePrivilegedPortsString)
	if err != nil {
		msg := fmt.Sprintf("Invalid NFS privileged_ports_only value: %s, error: %s", usePrivilegedPortsString, err)
		klog.Errorf(msg)
		return nil, errors.New(msg)
	}
	klog.V(2).Infof("Using priviledged ports only: %t", usePrivilegedPorts)

	// Snapshot dir visible
	snapdirVisibleString := config["snapdir_visible"]
	if snapdirVisibleString == "" {
		snapdirVisibleString = "true"
	}
	snapdirVisible, err := strconv.ParseBool(snapdirVisibleString)
	if err != nil {
		msg := fmt.Sprintf("Invalid NFS snapdir_visible value: %s, error: %s", snapdirVisibleString, err)
		klog.Errorf(msg)
		return nil, errors.New(msg)
	}
	klog.V(2).Infof("Snapshot directory is visible: %t", snapdirVisible)

	nfs.pVName = pvName
	nfs.configmap = config
	nfs.capacity = capacity
	nfs.usePrivilegedPorts = usePrivilegedPorts
	nfs.snapdirVisible = snapdirVisible
	nfs.exportpath = "/" + pvName
	ipAddress, err := nfs.cs.getNetworkSpaceIP(strings.Trim(config["network_space"], " "))
	if err != nil {
		klog.Errorf("failed to get networkspace ipaddress, %v", err)
		return nil, err
	}
	nfs.ipAddress = ipAddress
	klog.V(4).Infof("getNetworkSpaceIP ipAddress %s", nfs.ipAddress)

	// check if volume with given name already exists
	volume, err := nfs.cs.api.GetFileSystemByName(pvName)
	klog.V(4).Infof("CreateVolume - GetFileSystemByName error: %v", err)
	if err != nil && !strings.EqualFold(err.Error(), "filesystem with given name not found") {
		return &csi.CreateVolumeResponse{}, err
	}
	if volume != nil {
		// return existing volume
		nfs.fileSystemID = volume.ID
		exportArray, err := nfs.cs.api.GetExportByFileSystem(nfs.fileSystemID)
		if err != nil {
			return &csi.CreateVolumeResponse{}, err
		}
		if exportArray == nil {
			return &csi.CreateVolumeResponse{}, errors.New("exports not found")
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
	if contentSource != nil {
		if contentSource.GetSnapshot() != nil {
			snapshot := req.GetVolumeContentSource().GetSnapshot()
			csiResp, err = nfs.createVolumeFrmPVCSource(req, capacity, config["pool_name"], snapshot.GetSnapshotId())
			if err != nil {
				klog.Errorf("failed to create volume from snapshot with error: %v", err)
				return &csi.CreateVolumeResponse{}, err
			}
		} else if contentSource.GetVolume() != nil {
			volume := req.GetVolumeContentSource().GetVolume()
			csiResp, err = nfs.createVolumeFrmPVCSource(req, capacity, config["pool_name"], volume.GetVolumeId())
			if err != nil {
				klog.Errorf("failed to create volume from pvc with error: %v", err)
				return &csi.CreateVolumeResponse{}, err
			}
		}
	} else {
		csiResp, err = nfs.CreateNFSVolume(req)
		if err != nil {
			klog.Errorf("failed to create volume, %v", err)
			return &csi.CreateVolumeResponse{}, err
		}
	}
	return csiResp, nil
}

func (nfs *nfsstorage) createVolumeFrmPVCSource(req *csi.CreateVolumeRequest, size int64, storagePool string, srcVolumeID string) (csiResp *csi.CreateVolumeResponse, err error) {
	klog.V(2).Infof("Called createVolumeFrmPVCSource")
	defer func() {
		if res := recover(); res != nil {
			err = errors.New("error while creating volume from clone (PVC) " + fmt.Sprint(res))
		}
	}()

	volproto, err := validateStorageType(srcVolumeID)
	if err != nil || volproto.VolumeID == "" {
		return nil, errors.New("error getting volume id")
	}
	sourceVolumeID, err := strconv.ParseInt(volproto.VolumeID, 10, 64)
	if err != nil {
		return nil, errors.New("invalid volume id " + volproto.VolumeID)
	}
	// Lookup the VolumeSource source.
	srcfsys, err := nfs.cs.api.GetFileSystemByID(sourceVolumeID)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "volume not found: %d", sourceVolumeID)
	}

	// Validate the size is the same.
	if srcfsys.Size != size {
		return nil, status.Errorf(codes.InvalidArgument,
			"volume %d has not valid size %d with requested %d ",
			sourceVolumeID, srcfsys.Size, size)
	}
	// Validate the storagePool is the same.
	storagePoolID, err := nfs.cs.api.GetStoragePoolIDByName(storagePool)
	if err != nil {
		return nil, status.Errorf(codes.Internal,
			"error while getting storagepoolid with name %s ", storagePool)
	}
	if storagePoolID != srcfsys.PoolID {
		return nil, status.Errorf(codes.InvalidArgument,
			"volume storage pool is different than the requested storage pool %s", storagePool)
	}

	newSnapshotName := req.GetName() // create snapshot using the original CreateVolumeRequest
	newSnapshotParams := &api.FileSystemSnapshot{ParentID: sourceVolumeID, SnapshotName: newSnapshotName, WriteProtected: false}
	klog.V(2).Infof("createVolumeFrmPVCSource creating filesystem with params: %v", newSnapshotParams)
	// Create snapshot
	newSnapshot, err := nfs.cs.api.CreateFileSystemSnapshot(newSnapshotParams)
	if err != nil {
		klog.Errorf("failed to create snapshot: %s error: %v", newSnapshotParams.SnapshotName, err.Error())
		return nil, status.Errorf(codes.Internal, "failed to create snapshot, %v", err.Error())
	}
	klog.V(2).Infof("createVolumeFrmPVCSource successfully created volume from clone with name: %s", newSnapshotName)
	nfs.fileSystemID = newSnapshot.SnapshotID

	err = nfs.createExportPathAndAddMetadata()
	if err != nil {
		klog.Errorf("failed to create export and metadata, %v", err)
		return nil, err
	}
	return nfs.getNfsCsiResponse(req), nil
}

//CreateNFSVolume create volumne method
func (nfs *nfsstorage) CreateNFSVolume(req *csi.CreateVolumeRequest) (csiResp *csi.CreateVolumeResponse, err error) {
	defer func() {
		if res := recover(); res != nil {
			err = errors.New("error while creating CreateNFSVolume method " + fmt.Sprint(res))
		}
	}()
	validnwlist, err := nfs.cs.api.OneTimeValidation(nfs.configmap["pool_name"], nfs.configmap["network_space"])
	if err != nil {
		klog.Errorf(err.Error())
		return nil, err
	}
	nfs.configmap["network_space"] = validnwlist
	klog.V(4).Infof("networkspace validation success")

	err = nfs.createFileSystem()
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
		if res := recover(); res != nil {
			err = errors.New("error while export directory" + fmt.Sprint(res))
		}
		if err != nil && nfs.fileSystemID != 0 {
			klog.V(2).Infof("Seemes to be some problem reverting filesystem: %s", nfs.pVName)
			nfs.cs.api.DeleteFileSystem(nfs.fileSystemID)
		}
	}()

	err = nfs.createExportPath()
	if err != nil {
		klog.Errorf("failed to export path %v", err)
		return
	}
	klog.V(4).Infof("export path created for filesytem: %s", nfs.pVName)

	defer func() {
		if res := recover(); res != nil {
			err = errors.New("error while AttachMetadata directory" + fmt.Sprint(res))
		}
		if err != nil && nfs.exportID != 0 {
			klog.V(2).Infof("Seemes to be some problem reverting created export id: %d", nfs.exportID)
			nfs.cs.api.DeleteExportPath(nfs.exportID)
		}
	}()
	metadata := make(map[string]interface{})
	metadata["host.k8s.pvname"] = nfs.pVName
	metadata["host.created_by"] = nfs.cs.GetCreatedBy()

	_, err = nfs.cs.api.AttachMetadataToObject(nfs.fileSystemID, metadata)
	if err != nil {
		klog.Errorf("failed to attach metadata for file system %s, %v", nfs.pVName, err)
		return
	}
	klog.V(4).Infof("metadata attached successfully for file system %s", nfs.pVName)
	return
}

func (nfs *nfsstorage) createExportPath() (err error) {
	permissionsMapArray, err := getPermissionMaps(nfs.configmap["nfs_export_permissions"])
	if err != nil {
		klog.Errorf("failed to parse permission map string %s", nfs.configmap["nfs_export_permissions"])
		return err
	}

	var exportFileSystem api.ExportFileSys
	exportFileSystem.FilesystemID = nfs.fileSystemID
	exportFileSystem.Transport_protocols = "TCP"
	exportFileSystem.Privileged_port = nfs.usePrivilegedPorts
	exportFileSystem.SnapdirVisible = nfs.snapdirVisible
	exportFileSystem.Export_path = nfs.exportpath
	exportFileSystem.Permissionsput = append(exportFileSystem.Permissionsput, permissionsMapArray...)
	exportResp, err := nfs.cs.api.ExportFileSystem(exportFileSystem)
	if err != nil {
		klog.Errorf("failed to create export path of filesystem %s", nfs.pVName)
		return err
	}
	nfs.exportID = exportResp.ID
	nfs.exportBlock = exportResp.ExportPath
	klog.V(4).Infof("Created nfs export for PV '%s', snapdirVisible: %t", nfs.pVName, nfs.snapdirVisible)
	return err
}

func (nfs *nfsstorage) createFileSystem() (err error) {
	fileSystemCnt, err := nfs.cs.api.GetFileSystemCount()
	if err != nil {
		klog.Errorf("failed to get the filesystem count from Ibox %v", err)
		return err
	}
	if fileSystemCnt >= MaxFileSystemAllowed {
		klog.V(4).Infof("Max filesystem allowed on Ibox %v", MaxFileSystemAllowed)
		klog.V(4).Infof("Current filesystem count on Ibox %v", fileSystemCnt)
		klog.Errorf("Ibox not allowed to create new file system")
		err = errors.New("Ibox not allowed to create new file system")
		return err
	}
	var namepool = nfs.configmap["pool_name"]
	poolID, err := nfs.cs.api.GetStoragePoolIDByName(namepool)
	if err != nil {
		klog.Errorf("failed to get GetPoolID by pool_name %s", namepool)
		return err
	}
	ssdEnabled := nfs.configmap["ssd_enabled"]
	if ssdEnabled == "" {
		ssdEnabled = fmt.Sprint(false)
	}
	ssd, _ := strconv.ParseBool(ssdEnabled)
	mapRequest := make(map[string]interface{})
	mapRequest["pool_id"] = poolID
	mapRequest["name"] = nfs.pVName
	mapRequest["ssd_enabled"] = ssd
	mapRequest["provtype"] = strings.ToUpper(nfs.configmap["provision_type"])
	mapRequest["size"] = nfs.capacity
	fileSystem, err := nfs.cs.api.CreateFilesystem(mapRequest)
	if err != nil {
		klog.Errorf("failed to create filesystem %s", nfs.pVName)
		return err
	}
	nfs.fileSystemID = fileSystem.ID
	klog.V(4).Infof("filesystem Created %s", nfs.pVName)
	return err
}

func (nfs *nfsstorage) getNfsCsiResponse(req *csi.CreateVolumeRequest) *csi.CreateVolumeResponse {
	infinidatVol := &infinidatVolume{
		VolID:        fmt.Sprint(nfs.fileSystemID),
		VolName:      nfs.pVName,
		VolSize:      nfs.capacity,
		VolPath:      nfs.exportpath,
		IpAddress:    nfs.ipAddress,
		ExportID:     nfs.exportID,
		ExportBlock:  nfs.exportBlock,
		FileSystemID: nfs.fileSystemID,
	}
	nfs.configmap["ipAddress"] = (*infinidatVol).IpAddress
	nfs.configmap["exportID"] = strconv.Itoa(int((*infinidatVol).ExportID))
	nfs.configmap["volPathd"] = (*infinidatVol).VolPath

	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      (*infinidatVol).VolID,
			CapacityBytes: nfs.capacity,
			VolumeContext: nfs.configmap,
			ContentSource: req.GetVolumeContentSource(),
		},
	}
}

func (nfs *nfsstorage) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {

	if len(req.GetVolumeId()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID missing in request")
	}

	volumeID := req.GetVolumeId()
	volID, err := strconv.ParseInt(volumeID, 10, 64)
	if err != nil {
		klog.Errorf("Invalid Volume ID %v", err)
		return nil, err
	}

	nfs.uniqueID = volID
	nfsDeleteErr := nfs.DeleteNFSVolume()
	if nfsDeleteErr != nil {
		if strings.Contains(nfsDeleteErr.Error(), "FILESYSTEM_NOT_FOUND") {
			klog.Errorf("file system already delete from infinibox")
			return &csi.DeleteVolumeResponse{}, nil
		}
		klog.Errorf("failed to delete NFS Volume ID %s, %v", volumeID, nfsDeleteErr)
		return &csi.DeleteVolumeResponse{}, nfsDeleteErr
	}
	klog.V(2).Infof("volume %s successfully deleted", volumeID)
	return &csi.DeleteVolumeResponse{}, nil
}

//DeleteNFSVolume delete volumne method
func (nfs *nfsstorage) DeleteNFSVolume() (err error) {

	defer func() {
		if res := recover(); res != nil {
			err = errors.New("error while deleting filesystem " + fmt.Sprint(res))
			return
		}
	}()

	_, fileSystemErr := nfs.cs.api.GetFileSystemByID(nfs.uniqueID)
	if fileSystemErr != nil {
		klog.Errorf("failed to get file system by ID %d", nfs.uniqueID)
		err = fileSystemErr
		return
	}
	hasChild := nfs.cs.api.FileSystemHasChild(nfs.uniqueID)
	if hasChild {
		metadata := make(map[string]interface{})
		metadata[TOBEDELETED] = true
		_, err = nfs.cs.api.AttachMetadataToObject(nfs.uniqueID, metadata)
		if err != nil {
			klog.Errorf("failed to update host.k8s.to_be_deleted for filesystem %s error: %v", nfs.pVName, err)
			err = errors.New("error while Set metadata host.k8s.to_be_deleted")
		}
		return
	}

	parentID := nfs.cs.api.GetParentID(nfs.uniqueID)
	err = nfs.cs.api.DeleteFileSystemComplete(nfs.uniqueID)
	if err != nil {
		klog.Errorf("failed to delete filesystem %s error: %v", nfs.pVName, err)
		err = errors.New("error while delete file system")
	}
	if parentID != 0 {
		err = nfs.cs.api.DeleteParentFileSystem(parentID)
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

//ControllerPublishVolume
func (nfs *nfsstorage) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {

	var err error

	_, err = nfs.cs.accessModesHelper.IsValidAccessModeNfs(req)
	if err != nil {
		return &csi.ControllerPublishVolumeResponse{}, status.Error(codes.Internal, err.Error())
	}

	exportPermissionMapArray, err := getPermissionMaps(req.GetVolumeContext()["nfs_export_permissions"])
	if err != nil {
		klog.Errorf("failed to retrieve permission maps, %v", err)
		return &csi.ControllerPublishVolumeResponse{}, status.Error(codes.Internal, err.Error())
	}
	access := ""
	if len(exportPermissionMapArray) > 0 {
		access = exportPermissionMapArray[0]["access"].(string)
	}

	exportID := req.GetVolumeContext()["exportID"]
	noRootSquash := true //default value
	nodeNameIP := strings.Split(req.GetNodeId(), "$$")
	if len(nodeNameIP) != 2 {
		return &csi.ControllerPublishVolumeResponse{}, errors.New("not found Node ID")
	}
	nodeIP := nodeNameIP[1]
	eportid, _ := strconv.Atoi(exportID)
	_, err = nfs.cs.api.AddNodeInExport(eportid, access, noRootSquash, nodeIP)
	if err != nil {
		klog.Errorf("failed to add export rule, %v", err)
		return &csi.ControllerPublishVolumeResponse{}, status.Errorf(codes.Internal, "failed to add export rule  %s", err)
	}
	return &csi.ControllerPublishVolumeResponse{}, nil
}

func (nfs *nfsstorage) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	voltype := req.GetVolumeId()
	volproto := strings.Split(voltype, "$$")
	fileID, _ := strconv.ParseInt(volproto[0], 10, 64)
	err := nfs.cs.api.DeleteExportRule(fileID, req.GetNodeId())
	if err != nil {
		klog.Errorf("failed to delete Export Rule fileystemID %d error %v", fileID, err)
		return &csi.ControllerUnpublishVolumeResponse{}, status.Errorf(codes.Internal, "failed to delete Export Rule  %v", err)
	}
	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

func (nfs *nfsstorage) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	return nil, nil
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
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("Recoved from CSI CreateSnapshot  " + fmt.Sprint(res))
		}
	}()
	//var ts *timestamp.Timestamp
	var snapshotID string
	snapshotName := req.GetName()
	srcVolumeId := req.GetSourceVolumeId()
	klog.V(4).Infof("Create Snapshot name '%s'", snapshotName)
	klog.V(2).Infof("Create Snapshot called with source volume Id '%s'", srcVolumeId)
	volproto, err := validateStorageType(srcVolumeId)
	if err != nil {
		klog.Errorf("failed to validate storage type for volume %s, %v", srcVolumeId, err)
		return
	}

	sourceFilesystemID, _ := strconv.ParseInt(volproto.VolumeID, 10, 64)
	snapshotArray, err := nfs.cs.api.GetSnapshotByName(snapshotName)
	for _, snap := range *snapshotArray {
		if snap.ParentId == sourceFilesystemID {
			snapshotID = strconv.FormatInt(snap.SnapshotID, 10) + "$$" + volproto.StorageType
			klog.V(4).Infof("Got snapshot so returning nil")
			return &csi.CreateSnapshotResponse{
				Snapshot: &csi.Snapshot{
					SizeBytes:      snap.Size,
					SnapshotId:     snapshotID,
					SourceVolumeId: srcVolumeId,
					CreationTime:   ptypes.TimestampNow(),
					ReadyToUse:     true,
				},
			}, nil
		}
	}

	fileSystemSnapshot := &api.FileSystemSnapshot{
		ParentID:       sourceFilesystemID,
		SnapshotName:   snapshotName,
		WriteProtected: true,
	}

	resp, err := nfs.cs.api.CreateFileSystemSnapshot(fileSystemSnapshot)
	if err != nil {
		klog.Errorf("failed to create snapshot %s error %v", snapshotName, err)
		return
	}

	snapshotID = strconv.FormatInt(resp.SnapshotID, 10) + "$$" + volproto.StorageType
	snapshot := &csi.Snapshot{
		SnapshotId:     snapshotID,
		SourceVolumeId: srcVolumeId,
		ReadyToUse:     true,
		CreationTime:   ptypes.TimestampNow(),
		SizeBytes:      resp.Size,
	}
	klog.V(4).Infof("CreateFileSystemSnapshot resp: %v", snapshot)
	snapshotResp := &csi.CreateSnapshotResponse{Snapshot: snapshot}
	return snapshotResp, nil
}

func (nfs *nfsstorage) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (deleteSnapshot *csi.DeleteSnapshotResponse, err error) {
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("Recoved from CSI DeleteSnapshot  " + fmt.Sprint(res))
		}
	}()

	snapshotID, _ := strconv.ParseInt(req.GetSnapshotId(), 10, 64)
	nfs.uniqueID = snapshotID
	nfsSnapDeleteErr := nfs.DeleteNFSVolume()
	if nfsSnapDeleteErr != nil {
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
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("Recoved from CSI CreateSnapshot  " + fmt.Sprint(res))
		}
	}()

	ID, err := strconv.ParseInt(req.GetVolumeId(), 10, 64)
	if err != nil {
		klog.Errorf("Invalid Volume ID %v", err)
		return
	}

	capacity := int64(req.GetCapacityRange().GetRequiredBytes())
	if capacity < gib {
		capacity = gib
		klog.Warningf("Volume Minimum capacity should be greater than %d", gib)
	}

	// Expand file system size
	var fileSys api.FileSystem
	fileSys.Size = capacity
	_, err = nfs.cs.api.UpdateFilesystem(ID, fileSys)
	if err != nil {
		klog.Errorf("failed to update file system %v", err)
		return
	}
	klog.V(2).Infof("Filesystem size updated successfully")
	return &csi.ControllerExpandVolumeResponse{
		CapacityBytes:         capacity,
		NodeExpansionRequired: false,
	}, nil
}
