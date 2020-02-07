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
	ptypes "github.com/golang/protobuf/ptypes"
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

func (nfs *nfsstorage) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	log.Debug("Creating Volume of nfs protocol")
	//Adding the the request parameter into Map config
	config := make(map[string]string)
	for key, value := range req.GetParameters() {
		config[key] = value
	}
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

	contentSource := &csi.VolumeContentSource{}
	content := req.GetVolumeContentSource()

	//this is for volumne cloning and snapshot
	var sourceSnapshotID string
	var sourceVolumeID string
	if content != nil {
		if content.GetSnapshot() != nil {
			log.Infof("content.GetSnapshot %v", content.GetSnapshot())
			sourceSnapshotID = content.GetSnapshot().GetSnapshotId()
			contentSource = req.GetVolumeContentSource()
		} else if sourceVolume := content.GetVolume(); sourceVolume != nil {
			sourceVolumeID = content.GetVolume().GetVolumeId()
			log.Infof("content.GetVolume() %v", sourceVolumeID)
			contentSource = req.GetVolumeContentSource()
		}
	}
	if sourceSnapshotID != "" { //volume frm snapshot
		log.Debug("sourceSnapshotID %s", sourceSnapshotID)
	} else if sourceVolumeID != "" { //volume from PV
		log.Debug("sourceVolumeID %s", sourceVolumeID)
	}

	infinidatVol, createVolumeErr := nfs.CreateNFSVolume()
	if createVolumeErr != nil {
		log.Errorf("fail to create volume nfs file system %s error: %v", pvName, createVolumeErr)
		return &csi.CreateVolumeResponse{}, createVolumeErr
	}

	config["ipAddress"] = (*infinidatVol).IpAddress
	config["volID"] = (*infinidatVol).VolID
	config["volSize"] = strconv.Itoa(int((*infinidatVol).VolSize))
	config["exportID"] = strconv.Itoa(int((*infinidatVol).ExportID))
	config["fileSystemID"] = strconv.Itoa(int((*infinidatVol).FileSystemID))
	config["volPathd"] = (*infinidatVol).VolPath
	config["exportBlock"] = (*infinidatVol).ExportBlock
	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      (*infinidatVol).VolID,
			CapacityBytes: capacity,
			VolumeContext: config,
			ContentSource: contentSource,
		},
	}, nil
}

//CreateNFSVolume create volumne method
func (nfs *nfsstorage) CreateNFSVolume() (infinidatVol *infinidatVolume, err error) {
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
		return nil, err
	}
	log.Debugf("export path created for filesytem: %s", nfs.pVName)

	nfs.ipAddress, err = nfs.cs.getNetworkSpaceIP(nfs.configmap)
	if err != nil {
		log.Errorf("fail to get networkspace ipaddress %v", err)
		return nil, err
	}
	log.Debugf("Networkspace IP Address %s", nfs.ipAddress)

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
		return nil, err
	}
	log.Debug("metadata attached successfully for filesystem %s", nfs.pVName)
	infinidatVol = &infinidatVolume{
		VolID:        fmt.Sprint(nfs.fileSystemID),
		VolName:      nfs.pVName,
		VolSize:      nfs.capacity,
		IpAddress:    nfs.ipAddress,
		ExportID:     nfs.exportID,
		FileSystemID: nfs.fileSystemID,
		VolPath:      nfs.exportpath,
		ExportBlock:  nfs.exportBlock,
	}
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
func (nfs *nfsstorage) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	srcVolume := req.GetSourceVolumeId()
	log.Error("CreateSnapshot GetSourceVolumeId(): ", srcVolume)
	log.Error("CreateSnapshot GetName() ", req.GetName())
	volproto := strings.Split(srcVolume, "$$")
	n, _ := strconv.ParseInt(volproto[0], 10, 64)
	resp, err := nfs.cs.api.CreateFileSystemSnapshot(n, req.GetName())
	if err != nil {
		log.Errorf("fail to create snapshot %s error %v", req.GetName(), err)
		return nil, status.Error(codes.Internal, "internal server error")
	}
	log.Error("CreateFileSystemSnapshot resp() ", resp)
	snapshot := &csi.Snapshot{
		SnapshotId:     req.GetName(),
		SourceVolumeId: srcVolume,
		ReadyToUse:     true,
		CreationTime:   ptypes.TimestampNow(),
		SizeBytes:      1000,
	}
	snapshotResp := &csi.CreateSnapshotResponse{Snapshot: snapshot}
	return snapshotResp, nil
}

func (nfs *nfsstorage) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	return &csi.DeleteSnapshotResponse{}, nil
}

func (nfs *nfsstorage) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	log.Debug("ExpandVolume")
	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "Volume ID missing in request")
	}

	if req.GetCapacityRange() == nil {
		return nil, status.Error(codes.InvalidArgument, "CapacityRange cannot be empty")
	}

	volDetails := req.GetVolumeId()
	volDetail := strings.Split(volDetails, "$$")
	ID, err := strconv.ParseInt(volDetail[0], 10, 64)
	if err != nil {
		log.Errorf("Invalid Volume ID %v", err)
		return &csi.ControllerExpandVolumeResponse{}, nil
	}

	capacity := int64(req.GetCapacityRange().GetRequiredBytes())
	if capacity < gib {
		capacity = gib
		log.Warn("Volume Minimum capacity should be greater 1 GB")
	}
	log.Infof("volumen capacity %v", capacity)
	var fileSys api.FileSystem
	fileSys.Size = capacity
	// Expand file system size
	_, err = nfs.cs.api.UpdateFilesystem(ID, fileSys)
	if err != nil {
		log.Errorf("Failed to update file system %v", err)
		return &csi.ControllerExpandVolumeResponse{}, err
	}
	log.Infoln("Filesystem updated successfully")
	return &csi.ControllerExpandVolumeResponse{
		CapacityBytes:         capacity,
		NodeExpansionRequired: false,
	}, nil
}
