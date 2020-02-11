package storage

import (
	"context"
	"errors"
	"fmt"
	"infinibox-csi-driver/api"
	"path"
	"strconv"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/protobuf/ptypes"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
	dataRoot               = "/fs"
	mountAccess accessType = iota
	blockAccess

	//Infinibox default values
	//Ibox max allowed filesystem
	MaxFileSystemAllowed = 4000
	MountOptions         = "hard,rsize=1024,wsize=1024"
	NfsExportPermissions = "RW"
	NoRootSquash         = true

	// for size conversion
	kib    int64 = 1024
	mib    int64 = kib * 1024
	gib    int64 = mib * 1024
	gib100 int64 = gib * 100
	tib    int64 = gib * 1024
	tib100 int64 = tib * 100
)

func validateParameter(config map[string]string) (bool, map[string]string) {
	compulsaryFields := []string{"pool_name", "nfs_networkspace"} //TODO: add remaining paramters
	validationStatus := true
	validationStatusMap := make(map[string]string)
	for _, param := range compulsaryFields {
		if config[param] == "" {
			validationStatusMap[param] = param + " valume missing"
			validationStatus = false
		}
	}
	log.Debug("parameter Validation completed")
	return validationStatus, validationStatusMap
}

func (nfs *nfsstorage) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (csiResp *csi.CreateVolumeResponse, err error) {
	log.Debug("Creating Volume of nfs protocol")
	//Adding the the request parameter into Map config
	config := req.GetParameters()

	pvName := req.GetName()
	log.Debug("Creating fileystem %s of nfs protocol ", pvName)
	validationStatus, validationStatusMap := validateParameter(config)
	if !validationStatus {
		log.Errorf("Fail to validate parameter for nfs protocol %v ", validationStatusMap)
		return nil, status.Error(codes.InvalidArgument, "Fail to validate parameter for nfs protocol")
	}
	log.Debugf("fileystem %s ,parameter validation success", pvName)

	capacity := int64(req.GetCapacityRange().GetRequiredBytes())
	if capacity < gib { //INF90
		capacity = gib
		log.Warn("Volume Minimum capacity should be greater %d", gib)
	}

	nfs.pVName = pvName
	nfs.configmap = config
	nfs.capacity = capacity
	nfs.exportpath = path.Join(dataRoot, pvName) //TODO: export path prefix need to add here
	ipAddress, err := nfs.cs.getNetworkSpaceIP(strings.Trim(config["nfs_networkspace"], " "))
	if err != nil {
		log.Errorf("fail to get networkspace ipaddress %v", err)
		return nil, err
	}
	nfs.ipAddress = ipAddress
	log.Debugf("getNetworkSpaceIP ipAddress", nfs.ipAddress)

	// check if volume with given name already exists
	volume, err := nfs.cs.api.GetFileSystemByName(pvName)
	log.Debug("CreateVolume - GetFileSystemByName error : ", err)
	if err != nil && !strings.EqualFold(err.Error(), "filesystem with given name not found") {
		return &csi.CreateVolumeResponse{}, err
	}
	if volume != nil {
		// return exiting volume
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
			break
		}
		return nfs.getNfsCsiResponse(req), nil
	}

	// Volume content source support Volumes and Snapshots
	contentSource := req.GetVolumeContentSource()
	log.Debug("content volume source is : ", contentSource)
	if contentSource != nil {
		log.Debug("content volume source type is ", contentSource.GetType)
		if contentSource.GetSnapshot() != nil {
			csiResp, err = nfs.createVolumeFrmSnapshot(req, capacity, config["pool_name"])
			if err != nil {
				log.Errorf("failed to create volume from snapshot with error %v", err)
				return &csi.CreateVolumeResponse{}, err
			}
		} else if contentSource.GetVolume() != nil {
			csiResp, err = nfs.createVolumeFrmPVCSource(req, capacity, config["pool_name"])
			if err != nil {
				log.Errorf("failed to create volume from pvc with error %v", err)
				return &csi.CreateVolumeResponse{}, err
			}
		}
	} else {
		csiResp, err = nfs.CreateNFSVolume(req)
		if err != nil {
			log.Errorf("failt to create volume %v", err)
			return &csi.CreateVolumeResponse{}, err
		}
	}
	return csiResp, nil
}

func (nfs *nfsstorage) createVolumeFrmPVCSource(req *csi.CreateVolumeRequest, size int64, storagePool string) (csiResp *csi.CreateVolumeResponse, err error) {
	log.Info("Called createVolumeFrmPVCSource")
	defer func() {
		if res := recover(); res != nil {
			err = errors.New("error while creating volume from clone (PVC) " + fmt.Sprint(res))
		}
	}()
	volume := req.GetVolumeContentSource().GetVolume()
	name := req.GetName()

	volproto, err := validateStorageType(volume.GetVolumeId())
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
		return nil, status.Errorf(codes.NotFound, "volume not found: %s", sourceVolumeID)
	}

	// Validate the size is the same.
	if srcfsys.Size != size {
		return nil, status.Errorf(codes.InvalidArgument,
			"volume %s has not valid size %d with requested %d ",
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

	snapParam := &api.FileSystemSnapshot{ParentID: sourceVolumeID, SnapshotName: name, WriteProtected: false}
	log.Info("createVolumeFrmPVCSource creating filesystem with params : ", snapParam)
	// Create snapshot
	snapResponse, err := nfs.cs.api.CreateFileSystemSnapshot(snapParam)
	if err != nil {
		log.Errorf("Failed to create snapshot: %s error: %v", snapParam.SnapshotName, err.Error())
		return nil, status.Errorf(codes.Internal, "Failed to create snapshot: %s", err.Error())
	}
	log.Info("createVolumeFrmPVCSource successfully created volume from clone with name: ", snapParam.SnapshotName)
	nfs.fileSystemID = snapResponse.SnapshotID

	err = nfs.createExportPathAndAddMetadata()
	if err != nil {
		log.Errorf("fail to create export and metadata %v", err)
		return nil, err
	}
	return nfs.getNfsCsiResponse(req), nil
}

func (nfs *nfsstorage) createVolumeFrmSnapshot(req *csi.CreateVolumeRequest, size int64, storagePool string) (csiResp *csi.CreateVolumeResponse, err error) {
	log.Info("Called createVolumeFrmSnapshot")
	defer func() {
		if res := recover(); res != nil {
			err = errors.New("error while creating volume from clone (PVC) " + fmt.Sprint(res))
		}
	}()
	snapshot := req.GetVolumeContentSource().GetSnapshot()
	name := req.GetName()

	// Lookup the Snapshot source.
	volproto, err := validateStorageType(snapshot.GetSnapshotId())
	if err != nil {
		return nil, errors.New("error getting volume id")
	}
	sourceVolumeID, err := strconv.ParseInt(volproto.VolumeID, 10, 64)
	if err != nil {
		return nil, errors.New("invalid volume id " + volproto.VolumeID)
	}

	sourceFileSysVolume, err := nfs.cs.api.GetFileSystemByID(sourceVolumeID)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "volume not found: %s", sourceVolumeID)
	}

	if sourceFileSysVolume.Size != size {
		return nil, status.Errorf(codes.InvalidArgument,
			"volume %s has not valid size %d with requested %d ",
			sourceVolumeID, sourceFileSysVolume.Size, size)
	}

	storagePoolID, err := nfs.cs.api.GetStoragePoolIDByName(storagePool)
	if err != nil {
		return nil, status.Errorf(codes.Internal,
			"error while getting storagepoolid with name %s ", storagePool)
	}
	if storagePoolID != sourceFileSysVolume.PoolID {
		return nil, status.Errorf(codes.InvalidArgument,
			"volume storage pool is different than the requested storage pool %s", storagePool)
	}

	snapParam := &api.FileSystemSnapshot{ParentID: sourceFileSysVolume.ParentID, SnapshotName: name, WriteProtected: false}
	log.Info("createVolumeFrmPVCSource creating filesystem with params : ", snapParam)

	// Create snapshot
	snapResponse, err := nfs.cs.api.CreateFileSystemSnapshot(snapParam)
	if err != nil {
		log.Errorf("Failed to create snapshot: %s error: %v", snapParam.SnapshotName, err.Error())
		return nil, status.Errorf(codes.Internal, "Failed to create snapshot: %s", err.Error())
	}
	isSuccess, err := nfs.cs.api.RestoreFileSystemFromSnapShot(sourceFileSysVolume.ParentID, snapResponse.SnapshotID)
	if err != nil {
		log.Errorf("Error while restoring snapshot %v", err)
		nfs.cs.api.DeleteFileSystem(snapResponse.SnapshotID)
		return nil, status.Errorf(codes.Internal,
			"error restoring snapshot from snapshot id  %d ", sourceFileSysVolume.ID)
	}
	if !isSuccess {
		return nil, status.Errorf(codes.Internal, "restore volume from snapshot failed")
	}
	fileSysVol, err := nfs.cs.api.GetFileSystemByID(sourceFileSysVolume.ParentID)
	if err != nil {
		log.Errorf("Unable to retrive restored volume %v", err)
		return nil, status.Errorf(codes.Internal, "Unable to retrive restored volume with id  %d ", sourceFileSysVolume.ParentID)
	}
	log.Info("createVolumeFrmPVCSource successfully created volume from snapshot with name: ", snapParam.SnapshotName)
	nfs.fileSystemID = fileSysVol.ID

	err = nfs.createExportPathAndAddMetadata()
	if err != nil {
		log.Errorf("fail to create export and metadata %v", err)
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
	validnwlist, err := nfs.cs.api.OneTimeValidation(nfs.configmap["pool_name"], nfs.configmap["nfs_networkspace"])
	if err != nil {
		log.Errorf(err.Error())
		return nil, err
	}
	nfs.configmap["nfs_networkspace"] = validnwlist
	log.Debug("networkspace validation success")

	err = nfs.createFileSystem()
	if err != nil {
		log.Errorf("fail to create fileSystem %v", err)
		return nil, err
	}
	err = nfs.createExportPathAndAddMetadata()
	if err != nil {
		log.Errorf("fail to create export and metadata %v", err)
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
			log.Infoln("Seemes to be some problem reverting filesystem: %s", nfs.pVName)
			nfs.cs.api.DeleteFileSystem(nfs.fileSystemID)
		}
	}()

	err = nfs.createExportPath()
	if err != nil {
		log.Errorf("fail to export path %v", err)
		return
	}
	log.Debugf("export path created for filesytem: %s", nfs.pVName)

	defer func() {
		if res := recover(); res != nil {
			err = errors.New("error while AttachMetadata directory" + fmt.Sprint(res))
		}
		if err != nil && nfs.exportID != 0 {
			log.Infoln("Seemes to be some problem reverting created export id:", nfs.exportID)
			nfs.cs.api.DeleteExportPath(nfs.exportID)
		}
	}()
	metadata := make(map[string]interface{})
	metadata["host.k8s.pvname"] = nfs.pVName
	metadata["filesystem_type"] = ""

	_, err = nfs.cs.api.AttachMetadataToObject(nfs.fileSystemID, metadata)
	if err != nil {
		log.Errorf("fail to attach metadata for fileSystem : %s", nfs.pVName)
		log.Errorf("error to attach metadata %v", err)
		return
	}
	log.Debug("metadata attached successfully for filesystem %s", nfs.pVName)
	return
}

func (nfs *nfsstorage) createExportPath() (err error) {
	access := nfs.configmap["nfs_export_permissions"]
	if access == "" {
		access = NfsExportPermissions
	}
	rootsquash := nfs.configmap["no_root_squash"]
	if rootsquash == "" {
		rootsquash = fmt.Sprint(NoRootSquash)
	}
	rootsq, _ := strconv.ParseBool(rootsquash) //TODO
	var permissionsput []map[string]interface{}

	permissionsput = append(permissionsput, map[string]interface{}{"access": access, "no_root_squash": rootsq, "client": "*"})

	var exportFileSystem api.ExportFileSys
	exportFileSystem.FilesystemID = nfs.fileSystemID
	exportFileSystem.Transport_protocols = "TCP"
	exportFileSystem.Privileged_port = true
	exportFileSystem.Export_path = nfs.exportpath
	exportFileSystem.Permissionsput = append(exportFileSystem.Permissionsput, permissionsput...)
	exportResp, err := nfs.cs.api.ExportFileSystem(exportFileSystem)
	if err != nil {
		log.Errorf("fail to create export path of filesystem %s", nfs.pVName)
		return
	}
	nfs.exportID = exportResp.ID
	nfs.exportBlock = exportResp.ExportPath
	return
}

func (nfs *nfsstorage) createFileSystem() (err error) {
	fileSystemCnt, err := nfs.cs.api.GetFileSystemCount()
	if err != nil {
		log.Errorf("fail to get the filesystem count from Ibox %v", err)
		return
	}
	if fileSystemCnt >= MaxFileSystemAllowed {
		log.Debugf("Max filesystem allowed on Ibox %v", MaxFileSystemAllowed)
		log.Debugf("Current filesystem count on Ibox %v", fileSystemCnt)
		log.Errorf("Ibox not allowed to create new file system")
		err = errors.New("Ibox not allowed to create new file system")
		return
	}
	var namepool = nfs.configmap["pool_name"]
	//TODO:
	poolID, err := nfs.cs.api.GetStoragePoolIDByName(namepool)
	if err != nil {
		log.Errorf("fail to get GetPoolID by pool_name %s", namepool)
		return
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
		log.Errorf("fail to create filesystem %s", nfs.pVName)
		return
	}
	nfs.fileSystemID = fileSystem.ID
	log.Debugf("filesystem Created %s", nfs.pVName)
	return
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
	nfs.configmap["volID"] = (*infinidatVol).VolID
	nfs.configmap["volSize"] = strconv.Itoa(int((*infinidatVol).VolSize))
	nfs.configmap["exportID"] = strconv.Itoa(int((*infinidatVol).ExportID))
	nfs.configmap["fileSystemID"] = strconv.Itoa(int((*infinidatVol).FileSystemID))
	nfs.configmap["volPathd"] = (*infinidatVol).VolPath
	nfs.configmap["exportBlock"] = (*infinidatVol).ExportBlock

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
		log.Errorf("Invalid Volume ID %v", err)
		return &csi.DeleteVolumeResponse{}, nil
	}

	nfs.uniqueID = volID
	nfsDeleteErr := nfs.DeleteNFSVolume()
	if nfsDeleteErr != nil {
		if strings.Contains(nfsDeleteErr.Error(), "FILESYSTEM_NOT_FOUND") {
			log.Error("file system already delete from infinibox")
			return &csi.DeleteVolumeResponse{}, nil
		}
		log.Errorf("fail to delete NFS Volume %v", nfsDeleteErr)
		return &csi.DeleteVolumeResponse{}, nfsDeleteErr
	}
	log.Infof("volume %d successfully deleted", volumeID)
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
		log.Errorf("fail to check file system exist or not")
		return
	}
	hasChild := nfs.cs.api.FileSystemHasChild(nfs.uniqueID)
	if hasChild {
		metadata := make(map[string]interface{})
		metadata[TOBEDELETED] = true
		_, err = nfs.cs.api.AttachMetadataToObject(nfs.uniqueID, metadata)
		if err != nil {
			log.Errorf("fail to update host.k8s.to_be_deleted for filesystem %s error: %v", nfs.pVName, err)
			err = errors.New("error while Set metadata host.k8s.to_be_deleted")
		}
		return
	}

	parentID := nfs.cs.api.GetParentID(nfs.uniqueID)
	err = nfs.cs.api.DeleteFileSystemComplete(nfs.uniqueID)
	if err != nil {
		log.Errorf("fail to delete filesystem %s error: %v", nfs.pVName, err)
		err = errors.New("error while delete file system")
	}
	if parentID != 0 {
		err = nfs.cs.api.DeleteParentFileSystem(parentID)
		if err != nil {
			log.Errorf("fail to delete filesystem's %s parent filesystems error: %v", nfs.pVName, err)
		}

	}
	return
}

//ControllerPublishVolume
func (nfs *nfsstorage) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	exportID := req.GetVolumeContext()["exportID"]
	access := req.GetVolumeContext()["nfs_export_permissions"]
	noRootSquash, castErr := strconv.ParseBool(req.GetVolumeContext()["no_root_squash"])
	if castErr != nil {
		log.Debug("fail to cast no_root_squash .set default =true")
		noRootSquash = true
	}
	eportid, _ := strconv.Atoi(exportID)
	_, err := nfs.cs.api.AddNodeInExport(eportid, access, noRootSquash, nfs.cs.nodeIPAddress)
	if err != nil {
		log.Errorf("fail to add export rule %v", err)
		return &csi.ControllerPublishVolumeResponse{}, status.Errorf(codes.Internal, "fail to add export rule  %s", err)
	}
	return &csi.ControllerPublishVolumeResponse{}, nil
}

func (nfs *nfsstorage) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	voltype := req.GetVolumeId()
	volproto := strings.Split(voltype, "$$")
	fileID, _ := strconv.ParseInt(volproto[0], 10, 64)
	err := nfs.cs.api.DeleteExportRule(fileID, req.GetNodeId())
	if err != nil {
		log.Errorf("fail to delete Export Rule fileystemID %s error %v", fileID, err)
		return &csi.ControllerUnpublishVolumeResponse{}, status.Errorf(codes.Internal, "fail to delete Export Rule  %v", err)
	}
	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

func (nfs *nfsstorage) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	return nil, nil
}

func (nfs *nfsstorage) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	log.Debugf("ListVolumes %v", ctx, req)
	return &csi.ListVolumesResponse{}, nil
}

func (nfs *nfsstorage) ListSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	log.Debugf("ListSnapshots context :%v  request: %v", ctx, req)
	return &csi.ListSnapshotsResponse{}, nil
}
func (nfs *nfsstorage) GetCapacity(ctx context.Context, req *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error) {
	log.Debugf("GetCapacity context :%v  request: %v", ctx, req)
	return &csi.GetCapacityResponse{}, nil
}
func (nfs *nfsstorage) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	log.Debugf("ControllerGetCapabilities context :%v  request: %v", ctx, req)
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
	log.Debug("Create Snapshot of name ", snapshotName)
	log.Infof("Create Snapshot called with volume Id", req.GetSourceVolumeId())
	volproto, err := validateStorageType(req.GetSourceVolumeId())
	if err != nil {
		log.Errorf("fail to validate storage type %v", err)
		return
	}

	sourceFilesystemID, _ := strconv.ParseInt(volproto.VolumeID, 10, 64)
	snapshotArray, err := nfs.cs.api.GetSnapshotByName(snapshotName)
	for _, snap := range *snapshotArray {
		if snap.ParentId == sourceFilesystemID {
			snapshotID = strconv.FormatInt(snap.SnapshotID, 10) + "$$" + volproto.StorageType
			log.Debug("Got snapshot so returning nil")
			return &csi.CreateSnapshotResponse{
				Snapshot: &csi.Snapshot{
					SizeBytes:      snap.Size,
					SnapshotId:     snapshotID,
					SourceVolumeId: req.GetSourceVolumeId(),
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
		log.Errorf("Failed to create snapshot %s error %v", snapshotName, err)
		return
	}

	snapshotID = strconv.FormatInt(resp.SnapshotID, 10) + "$$" + volproto.StorageType
	snapshot := &csi.Snapshot{
		SnapshotId:     snapshotID,
		SourceVolumeId: req.GetSourceVolumeId(),
		ReadyToUse:     true,
		CreationTime:   ptypes.TimestampNow(),
		SizeBytes:      resp.Size,
	}
	log.Debug("CreateFileSystemSnapshot resp() ", snapshot)
	snapshotResp := &csi.CreateSnapshotResponse{Snapshot: snapshot}
	return snapshotResp, nil
}

func (nfs *nfsstorage) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (deleteSnapshot *csi.DeleteSnapshotResponse, err error) {
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("Recoved from CSI CreateSnapshot  " + fmt.Sprint(res))
		}
	}()

	volproto, err := validateStorageType(req.GetSnapshotId())
	if err != nil {
		log.Errorf("fail to validate storage type %v", err)
		return
	}

	snapshotID, _ := strconv.ParseInt(volproto.VolumeID, 10, 64)
	_, err = nfs.cs.api.GetFileSystemByID(snapshotID)
	if err != nil {
		if strings.Contains(err.Error(), "FILESYSTEM_NOT_FOUND") {
			log.Debug("snapshot already deleted")
			err = nil
		}
		return
	}

	_, err = nfs.cs.api.DeleteFileSystem(snapshotID)
	if err != nil {
		log.Errorf("Failed to delete snapshot %s error %v", snapshotID, err)
		return nil, status.Error(codes.Internal, "internal server error")
	}
	return
}

func (nfs *nfsstorage) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (expandVolume *csi.ControllerExpandVolumeResponse, err error) {
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("Recoved from CSI CreateSnapshot  " + fmt.Sprint(res))
		}
	}()

	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "Volume ID missing in request")
	}

	if req.GetCapacityRange() == nil {
		return nil, status.Error(codes.InvalidArgument, "CapacityRange cannot be empty")
	}

	volproto, err := validateStorageType(req.GetVolumeId())
	if err != nil {
		log.Errorf("fail to validate storage type %v", err)
		return
	}

	ID, err := strconv.ParseInt(volproto.VolumeID, 10, 64)
	if err != nil {
		log.Errorf("Invalid Volume ID %v", err)
		return
	}

	capacity := int64(req.GetCapacityRange().GetRequiredBytes())
	if capacity < gib {
		capacity = gib
		log.Warn("Volume Minimum capacity should be greater 1 GB")
	}

	var fileSys api.FileSystem
	fileSys.Size = capacity
	// Expand file system size
	_, err = nfs.cs.api.UpdateFilesystem(ID, fileSys)
	if err != nil {
		log.Errorf("Failed to update file system %v", err)
		return
	}
	log.Infoln("Filesystem updated successfully")
	return &csi.ControllerExpandVolumeResponse{
		CapacityBytes:         capacity,
		NodeExpansionRequired: false,
	}, nil
}
