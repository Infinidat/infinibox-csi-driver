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
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
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

func (nfs *nfsstorage) ValidateStorageClass(params map[string]string) error {
	requiredParams := map[string]string{
		common.SC_NETWORK_SPACE: `\A.*\z`, // TODO: could make this enforce IBOX network_space requirements, but probably not necessary
	}

	optionalParams := map[string]string{}

	suppliedParams := params
	err := ValidateRequiredOptionalSCParameters(requiredParams, optionalParams, suppliedParams)
	if err != nil {
		zlog.Err(err)
		return status.Error(codes.InvalidArgument, err.Error())
	}

	useChap := suppliedParams[common.SC_USE_CHAP]
	if useChap != "" {
		zlog.Warn().Msgf("useCHAP is not a valid storage class parameter for nfs or nfs-treeq")
	}

	err = validateNFSExportPermissions(suppliedParams)
	if err != nil {
		zlog.Err(err)
		return status.Error(codes.InvalidArgument, err.Error())
	}

	snapdirVisible := false
	snapdirVisibleString := params[common.SC_SNAPDIR_VISIBLE]
	if snapdirVisibleString != "" {
		snapdirVisible, err = strconv.ParseBool(snapdirVisibleString)
		if err != nil {
			e := fmt.Errorf("invalid NFS snapdir_visible value: %s, error: %v", snapdirVisibleString, err)
			zlog.Err(e)
			return e
		}
	}
	nfs.snapdirVisible = snapdirVisible

	usePrivilegedPorts := false
	usePrivilegedPortsString := params[common.SC_PRIV_PORTS]
	if usePrivilegedPortsString != "" {
		usePrivilegedPorts, err = strconv.ParseBool(usePrivilegedPortsString)
		if err != nil {
			e := fmt.Errorf("invalid NFS privileged_ports_only value: %s, error: %v", usePrivilegedPortsString, err)
			zlog.Err(e)
			return status.Error(codes.InvalidArgument, e.Error())
		}
	}
	nfs.usePrivilegedPorts = usePrivilegedPorts

	return nil
}

func (nfs *nfsstorage) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	zlog.Trace().Msgf("nfstorage.CreateVolume")
	var err error
	// Adding the the request parameter into Map params
	params := req.GetParameters()
	pvName := req.GetName()

	zlog.Debug().Msgf(" csi request %v", req)
	zlog.Debug().Msgf(" csi request parameters %v", params)
	zlog.Debug().Msgf(" csi volume caps %+v", req.VolumeCapabilities)
	zlog.Debug().Msgf(" csi request name %s", req.Name)

	// basic sanity-checking to ensure the user is not requesting block access to a NFS filesystem
	for _, cap := range req.GetVolumeCapabilities() {
		if block := cap.GetBlock(); block != nil {
			e := fmt.Errorf("block access requested for %s PV %s", params[common.SC_STORAGE_PROTOCOL], req.GetName())
			zlog.Err(e)
			return nil, status.Error(codes.InvalidArgument, e.Error())
		}
	}

	zlog.Debug().Msgf("Using privileged ports only: %t", nfs.usePrivilegedPorts)

	zlog.Debug().Msgf("Snapshot directory is visible: %t", nfs.snapdirVisible)

	nfs.pVName = pvName
	nfs.storageClassParameters = params
	nfs.exportPath = "/" + pvName
	ipAddress, err := nfs.cs.getNetworkSpaceIP(strings.Trim(params[common.SC_NETWORK_SPACE], " "))
	if err != nil {
		zlog.Err(err)
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	nfs.ipAddress = ipAddress
	zlog.Debug().Msgf("getNetworkSpaceIP ipAddress %s", nfs.ipAddress)

	// check if volume with given name already exists
	volume, err := nfs.cs.Api.GetFileSystemByName(pvName)
	if err != nil {
		zlog.Err(err)
	}
	if err != nil && !strings.EqualFold(err.Error(), "filesystem with given name not found") {
		zlog.Debug().Msgf("CreateVolume - GetFileSystemByName error: %v", err)
		return nil, status.Errorf(codes.NotFound, "error CreateVolume failed: %v", err)
	}
	if volume != nil {
		// return existing volume
		nfs.fileSystemID = volume.ID
		exportArray, err := nfs.cs.Api.GetExportByFileSystem(nfs.fileSystemID)
		if err != nil {
			zlog.Err(err)
			return nil, status.Errorf(codes.Internal, "error CreateVolume failed: %v", err)
		}
		if exportArray == nil {
			return nil, status.Errorf(codes.NotFound, "error CreateVolume failed: %v", err)
		}
		if nfs.capacity != volume.Size {
			err = status.Errorf(codes.AlreadyExists, "error CreateVolume failed: volume exists but has different size")
			zlog.Error().Msgf("capacity %d volume: %+v", nfs.capacity, volume)
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
	zlog.Debug().Msgf("content volume source: %v", contentSource)
	var csiResp *csi.CreateVolumeResponse
	if contentSource != nil {
		if contentSource.GetSnapshot() != nil {
			snapshot := req.GetVolumeContentSource().GetSnapshot()
			csiResp, err = nfs.createVolumeFromPVCSource(req, nfs.capacity, params[common.SC_POOL_NAME], snapshot.GetSnapshotId())
			if err != nil {
				zlog.Error().Msgf("failed to create volume from snapshot with error: %v", err)
				return nil, err
			}
		} else if contentSource.GetVolume() != nil {
			volume := req.GetVolumeContentSource().GetVolume()
			csiResp, err = nfs.createVolumeFromPVCSource(req, nfs.capacity, params[common.SC_POOL_NAME], volume.GetVolumeId())
			if err != nil {
				zlog.Error().Msgf("failed to create volume from pvc with error: %v", err)
				return nil, err
			}
		}
	} else {
		csiResp, err = nfs.CreateNFSVolume(req)
		if err != nil {
			zlog.Error().Msgf("failed to create volume, %v", err)
			return nil, err
		}
	}
	return csiResp, nil
}

func (nfs *nfsstorage) createVolumeFromPVCSource(req *csi.CreateVolumeRequest, size int64, storagePool string, srcVolumeID string) (csiResp *csi.CreateVolumeResponse, err error) {
	zlog.Debug().Msgf("createVolumeFromPVCSource")

	volproto, err := validateVolumeID(srcVolumeID)
	if err != nil || volproto.VolumeID == "" {
		zlog.Error().Msgf("failed to validate volume id: %s, err: %v", srcVolumeID, err)
		return nil, status.Errorf(codes.NotFound, "invalid source volume id format: %s", srcVolumeID)
	}
	sourceVolumeID, err := strconv.ParseInt(volproto.VolumeID, 10, 64)
	if err != nil {
		zlog.Err(err)
		return nil, status.Errorf(codes.InvalidArgument, "invalid source volume volume id (non-numeric): %s", volproto.VolumeID)
	}

	// Look up the source volume
	srcfsys, err := nfs.cs.Api.GetFileSystemByID(sourceVolumeID)
	if err != nil {
		zlog.Err(err)
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
		zlog.Err(err)
		return nil, status.Errorf(codes.InvalidArgument, "error GetStoragePoolIDByName: %s", storagePool)
	}
	if storagePoolID != srcfsys.PoolID {
		return nil, status.Errorf(codes.InvalidArgument,
			"source storagepool id differs from requested: %s", storagePool)
	}

	newSnapshotName := req.GetName() // create snapshot using the original CreateVolumeRequest
	newSnapshotParams := &api.FileSystemSnapshot{ParentID: sourceVolumeID, SnapshotName: newSnapshotName, WriteProtected: false}
	zlog.Debug().Msgf("CreateFileSystemSnapshot: %v", newSnapshotParams)
	// Create snapshot
	var lockExpiresAt int64
	newSnapshot, err := nfs.cs.Api.CreateFileSystemSnapshot(lockExpiresAt, newSnapshotParams)
	if err != nil {
		e := fmt.Errorf("failed to create snapshot: %s error: %v", newSnapshotParams.SnapshotName, err)
		zlog.Err(e)
		return nil, status.Errorf(codes.Internal, e.Error())
	}
	zlog.Debug().Msgf("createVolumeFrmPVCSource successfully created volume from clone with name: %s", newSnapshotName)
	nfs.fileSystemID = newSnapshot.SnapshotID

	err = nfs.createExportPathAndAddMetadata()
	if err != nil {
		zlog.Error().Msgf("failed to create export and metadata, %v", err)
		return nil, err
	}
	return nfs.getNfsCsiResponse(req), nil
}

// CreateNFSVolume create volume method
func (nfs *nfsstorage) CreateNFSVolume(req *csi.CreateVolumeRequest) (csiResp *csi.CreateVolumeResponse, err error) {
	err = nfs.createFileSystem(nfs.pVName)
	if err != nil {
		zlog.Error().Msgf("failed to create file system, %v", err)
		return nil, err
	}
	err = nfs.createExportPathAndAddMetadata()
	if err != nil {
		zlog.Error().Msgf("failed to create export and metadata, %v", err)
		return nil, err
	}
	return nfs.getNfsCsiResponse(req), nil
}

func (nfs *nfsstorage) createExportPathAndAddMetadata() (err error) {
	defer func() {
		if err != nil && nfs.fileSystemID != 0 {
			zlog.Debug().Msgf("seems to be some problem reverting filesystem: %s", nfs.pVName)
			if _, errDelFS := nfs.cs.Api.DeleteFileSystem(nfs.fileSystemID); errDelFS != nil {
				zlog.Error().Msgf("failed to delete file system id: %d %v", nfs.fileSystemID, errDelFS)
			}
		}
	}()

	if nfs.storageClassParameters[common.SC_NFS_EXPORT_PERMISSIONS] == "" {
		zlog.Debug().Msg("nfs_export_permissions parameter is not set in the StorageClass, will use default export")
	} else {
		err = nfs.createExportPath()
		if err != nil {
			zlog.Error().Msgf("failed to export path %v", err)
			return
		}
		zlog.Debug().Msgf("export path created for filesytem: %s", nfs.pVName)
	}

	defer func() {
		if err != nil && nfs.exportID != 0 {
			zlog.Debug().Msgf("seems to be some problem reverting created export id: %d", nfs.exportID)
			if _, errDelExport := nfs.cs.Api.DeleteExportPath(nfs.exportID); errDelExport != nil {
				zlog.Error().Msgf("failed to delete export path for file system id: %d %v", nfs.fileSystemID, errDelExport)
			}
		}
	}()

	metadata := map[string]interface{}{
		"host.k8s.pvname": nfs.pVName,
		"host.created_by": nfs.cs.GetCreatedBy(),
	}

	_, err = nfs.cs.Api.AttachMetadataToObject(nfs.fileSystemID, metadata)
	if err != nil {
		zlog.Error().Msgf("failed to attach metadata for file system %s, %v", nfs.pVName, err)
		return
	}
	zlog.Debug().Msgf("metadata attached successfully for file system %s", nfs.pVName)
	return
}

func (nfs *nfsstorage) createExportPath() (err error) {
	permissionsMapArray, err := getPermissionMaps(nfs.storageClassParameters[common.SC_NFS_EXPORT_PERMISSIONS])
	if err != nil {
		zlog.Error().Msgf("failed to parse permission map string %s %v", nfs.storageClassParameters[common.SC_NFS_EXPORT_PERMISSIONS], err)
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
		zlog.Error().Msgf("failed to create export path of filesystem %s %v", nfs.pVName, err)
		return err
	}
	nfs.exportID = exportResp.ID
	nfs.exportBlock = exportResp.ExportPath
	zlog.Debug().Msgf("created nfs export for PV '%s', snapdirVisible: %t", nfs.pVName, nfs.snapdirVisible)
	return err
}

func (nfs *nfsstorage) createFileSystem(fileSystemName string) (err error) {
	namepool := nfs.storageClassParameters[common.SC_POOL_NAME]
	poolID, err := nfs.cs.Api.GetStoragePoolIDByName(namepool)
	if err != nil {
		zlog.Error().Msgf("failed to get GetPoolID by pool_name %s %v", namepool, err)
		return err
	}
	provtype := strings.ToUpper(nfs.storageClassParameters[common.SC_PROVISION_TYPE])
	switch provtype {
	case "":
		provtype = common.SC_THIN_PROVISION_TYPE
	case common.SC_THIN_PROVISION_TYPE, common.SC_THICK_PROVISION_TYPE:
	default:
		errStr := fmt.Sprintf("%s valid values are THICK or THIN, THIN is the default when not specified, entered value was [%s]", common.SC_PROVISION_TYPE, provtype)
		zlog.Error().Msgf(errStr)
		return fmt.Errorf(errStr)
	}

	if provtype == "" {
		provtype = common.SC_THIN_PROVISION_TYPE
	}
	mapRequest := map[string]interface{}{
		"pool_id":  poolID,
		"name":     fileSystemName,
		"size":     nfs.capacity,
		"provtype": provtype,
	}

	ssdEnabled := nfs.storageClassParameters[common.SC_SSD_ENABLED]
	if ssdEnabled != "" {
		ssd, err := strconv.ParseBool(ssdEnabled)
		if err != nil {
			errStr := fmt.Sprintf("%s invalid format, needs to be true or false, %s was specified", common.SC_SSD_ENABLED, ssdEnabled)
			zlog.Error().Msgf(errStr)
			return fmt.Errorf(errStr)
			//return err
		}
		mapRequest[common.SC_SSD_ENABLED] = ssd
	}

	fileSystem, err := nfs.cs.Api.CreateFilesystem(mapRequest)
	if err != nil {
		zlog.Error().Msgf("failed to create filesystem %s %v", fileSystemName, err)
		return err
	}
	nfs.fileSystemID = fileSystem.ID
	zlog.Debug().Msgf("filesystem Created %s", fileSystemName)
	return err
}

func (nfs *nfsstorage) getNfsCsiResponse(req *csi.CreateVolumeRequest) *csi.CreateVolumeResponse {
	infinidatVol := &infinidatVolume{
		VolID:     fmt.Sprint(nfs.fileSystemID),
		VolPath:   nfs.exportPath,
		IpAddress: nfs.ipAddress,
		ExportID:  nfs.exportID,
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
	volumeID := req.GetVolumeId()
	volID, err := strconv.ParseInt(volumeID, 10, 64)
	if err != nil {
		zlog.Error().Msgf("invalid Volume ID %v", err)
		return nil, err
	}

	nfs.uniqueID = volID
	nfsDeleteErr := nfs.DeleteNFSVolume()
	if nfsDeleteErr != nil {
		zlog.Err(nfsDeleteErr)
		if strings.Contains(nfsDeleteErr.Error(), "FILESYSTEM_NOT_FOUND") {
			zlog.Error().Msgf("file system already delete from infinibox")
			return &csi.DeleteVolumeResponse{}, nil
		}
		zlog.Error().Msgf("failed to delete NFS Volume ID %s, %v", volumeID, nfsDeleteErr)
		return nil, nfsDeleteErr
	}
	zlog.Debug().Msgf("volume %s successfully deleted", volumeID)
	return &csi.DeleteVolumeResponse{}, nil
}

// DeleteNFSVolume delete volume method
func (nfs *nfsstorage) DeleteNFSVolume() (err error) {

	fs, fileSystemErr := nfs.cs.Api.GetFileSystemByID(nfs.uniqueID)
	if fileSystemErr != nil {
		zlog.Error().Msgf("failed to get file system by ID %d %v", nfs.uniqueID, fileSystemErr)
		err = fileSystemErr
		return
	}

	if fs.LockState == common.LOCKED_STATE {
		return status.Errorf(codes.Aborted, "snapshot %d is locked and can't be deleted till it expires at %s", nfs.uniqueID, time.UnixMilli(fs.LockExpiresAt))
	}

	hasChild := nfs.cs.Api.FileSystemHasChild(nfs.uniqueID)
	if hasChild {
		metadata := map[string]interface{}{
			TOBEDELETED: true,
		}
		_, err = nfs.cs.Api.AttachMetadataToObject(nfs.uniqueID, metadata)
		if err != nil {
			zlog.Error().Msgf("failed to update host.k8s.to_be_deleted for filesystem %s error: %v", nfs.pVName, err)
			err = errors.New("error while Set metadata host.k8s.to_be_deleted")
		}
		return
	}

	parentID := nfs.cs.Api.GetParentID(nfs.uniqueID)
	err = nfs.cs.Api.DeleteFileSystemComplete(nfs.uniqueID)
	if err != nil {
		zlog.Error().Msgf("failed to delete filesystem %s error: %v parentID: %d", nfs.pVName, err, parentID)
		err = errors.New("error while delete file system")
	}

	if parentID != 0 {
		err = nfs.cs.Api.DeleteParentFileSystem(parentID)
		if err != nil {
			zlog.Error().Msgf("failed to delete filesystem's %s parent filesystems error: %v", nfs.pVName, err)
		}

	}
	return
}

type ExportPermission struct {
	Access         string
	No_Root_Squash bool
	Client         string
}

func (nfs *nfsstorage) ControllerModifyVolume(ctx context.Context, req *csi.ControllerModifyVolumeRequest) (*csi.ControllerModifyVolumeResponse, error) {
	return nil, nil
}

func (nfs *nfsstorage) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	var err error
	volumeID := req.GetVolumeId()
	exportID := req.GetVolumeContext()["exportID"]

	zlog.Debug().Msgf("ControllerPublishVolume nodeId %s volumeID %s exportID %s nfs_export_permissions %s",
		req.GetNodeId(), volumeID, exportID, req.GetVolumeContext()[common.SC_NFS_EXPORT_PERMISSIONS])

	kubeNodeID := req.GetNodeId()
	if kubeNodeID == "" {
		return nil, status.Error(codes.InvalidArgument, "node ID is required")
	}

	// TODO: revisit this as part of CSIC-343
	_, err = nfs.cs.AccessModesHelper.IsValidAccessModeNfs(req)
	if err != nil {
		zlog.Err(err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	if req.GetVolumeContext()[common.SC_NFS_EXPORT_PERMISSIONS] == "" {
		zlog.Debug().Msgf("nfs_export_permissions parameter not set, volume ID %s export ID %s", volumeID, exportID)
		return &csi.ControllerPublishVolumeResponse{}, nil
	}

	// proceed to create a default export rule using the Node ip address

	exportPermissionMapArray, err := getPermissionMaps(req.GetVolumeContext()[common.SC_NFS_EXPORT_PERMISSIONS])
	if err != nil {
		zlog.Error().Msgf("failed to retrieve permission maps, %v", err)
		return nil, status.Error(codes.Internal, err.Error())
	}
	zlog.Debug().Msgf("nfs export permissions for volume ID %s and export ID %s: %v", volumeID, exportID, exportPermissionMapArray)

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
		zlog.Error().Msgf("failed to add export rule, %v", err)
		return nil, status.Errorf(codes.Internal, "failed to add export rule  %s", err)
	}

	return &csi.ControllerPublishVolumeResponse{}, nil
}

func (nfs *nfsstorage) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	zlog.Debug().Msgf("ControllerUnpublishVolume")
	kubeNodeID := req.GetNodeId()
	if kubeNodeID == "" {
		return nil, status.Error(codes.InvalidArgument, "node ID is required")
	}
	voltype := req.GetVolumeId()
	volproto := strings.Split(voltype, "$$")
	fileID, _ := strconv.ParseInt(volproto[0], 10, 64)
	err := nfs.cs.Api.DeleteExportRule(fileID, kubeNodeID)
	if err != nil {
		zlog.Error().Msgf("failed to delete Export Rule fileystemID %d error %v", fileID, err)
		return nil, status.Errorf(codes.Internal, "failed to delete Export Rule  %v", err)
	}
	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

func (nfs *nfsstorage) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (resp *csi.ValidateVolumeCapabilitiesResponse, err error) {
	zlog.Debug().Msgf("ValidateVolumeCapabilities called with volumeId %s", req.GetVolumeId())
	volproto, err := validateVolumeID(req.GetVolumeId())
	if err != nil {
		zlog.Error().Msgf("failed to validate storage type: %v", err)
		return nil, status.Errorf(codes.InvalidArgument, "invalid volume id format: %s", req.GetVolumeId())
	}
	volID, err := strconv.ParseInt(volproto.VolumeID, 10, 64)
	if err != nil {
		zlog.Error().Msgf("failed to validate volume id: %v", err)
		return nil, status.Errorf(codes.InvalidArgument, "invalid volume id (non-numeric): %s", req.GetVolumeId())
	}

	zlog.Debug().Msgf("volID: %d", volID)
	fs, err := nfs.cs.Api.GetFileSystemByID(volID)
	if err != nil {
		zlog.Error().Msgf("failed to find volume ID: %d, %v", volID, err)
		err = status.Errorf(codes.NotFound, "ValidateVolumeCapabilities failed to find volume ID: %d, %v", volID, err)
	}
	zlog.Debug().Msgf("volID: %d volume: %v", volID, fs)

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
	zlog.Debug().Msgf("CreateSnapshot parameters %+v\n", req.Parameters)
	var snapshotID string
	snapshotName := req.GetName()
	srcVolumeId := req.GetSourceVolumeId()
	zlog.Debug().Msgf("called CreateSnapshot source volume Id '%s' snapshot name %s", srcVolumeId, snapshotName)
	volproto, err := validateVolumeID(srcVolumeId)
	if err != nil {
		zlog.Error().Msgf("failed to validate storage type for volume %s, %v", srcVolumeId, err)
		return
	}

	sourceFilesystemID, _ := strconv.ParseInt(volproto.VolumeID, 10, 64)
	snapshotArray, err := nfs.cs.Api.GetSnapshotByName(snapshotName)
	if err != nil {
		zlog.Error().Msgf("error GetSnapshotByName %s, %v", volproto.VolumeID, err)
		return
	}
	if len(*snapshotArray) > 0 {
		for _, snap := range *snapshotArray {
			if snap.ParentId == sourceFilesystemID {
				snapshotID = strconv.FormatInt(snap.SnapshotID, 10) + "$$" + volproto.StorageType
				zlog.Debug().Msgf("snapshot: %s src fs id: %d exists, snapshot id: %d", snapshotName, snap.ParentId, snap.SnapshotID)
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
				zlog.Debug().Msgf("Snapshot: %s snapshot id: %d src fs id: %d (requested: %d)",
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

	var lockExpiresAt int64
	lockExpiresAtParameter := req.Parameters[common.LOCK_EXPIRES_AT_PARAMETER]
	if lockExpiresAtParameter != "" {
		ntpStatus, err := nfs.cs.Api.GetNtpStatus()
		if err != nil {
			zlog.Error().Msgf("failed to get ntp status error %v", err)
			return nil, err
		}
		lockExpiresAt, err = validateSnapshotLockingParameter(ntpStatus[0].LastProbeTimestamp, lockExpiresAtParameter)
		if err != nil {
			zlog.Error().Msgf("failed to create snapshot %s error %v, invalid lock_expires_at parameter ", snapshotName, err)
			return nil, err
		}
		zlog.Debug().Msgf("snapshot param has a lock_expires_at of %s", lockExpiresAtParameter)
	}
	resp, err := nfs.cs.Api.CreateFileSystemSnapshot(lockExpiresAt, fileSystemSnapshot)
	if err != nil {
		zlog.Error().Msgf("failed to create snapshot %s error %v", snapshotName, err)
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
	zlog.Debug().Msgf("CreateFileSystemSnapshot resp: %v", snapshot)
	snapshotResp := &csi.CreateSnapshotResponse{Snapshot: snapshot}
	return snapshotResp, nil
}

func (nfs *nfsstorage) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (deleteSnapshot *csi.DeleteSnapshotResponse, err error) {
	zlog.Debug().Msgf("DeleteSnapshot snapshotID %s", req.GetSnapshotId())

	snapshotID, err := strconv.ParseInt(req.GetSnapshotId(), 10, 64)
	if err != nil {
		zlog.Error().Msgf("failed to parse int from snapshotID, %s, error %s", req.GetSnapshotId(), err.Error())
		return nil, status.Error(codes.Aborted, err.Error())
	}
	nfs.uniqueID = snapshotID

	nfsSnapDeleteErr := nfs.DeleteNFSVolume()
	if nfsSnapDeleteErr != nil {
		zlog.Err(nfsSnapDeleteErr)
		if strings.Contains(nfsSnapDeleteErr.Error(), "FILESYSTEM_NOT_FOUND") {
			zlog.Error().Msgf("snapshot already delete from infinibox")
			deleteSnapshot = &csi.DeleteSnapshotResponse{}
			return
		}
		zlog.Error().Msgf("failed to delete snapshot, %v", nfsSnapDeleteErr)
		err = nfsSnapDeleteErr
		return
	}
	deleteSnapshot = &csi.DeleteSnapshotResponse{}
	return
}

func (nfs *nfsstorage) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (expandVolume *csi.ControllerExpandVolumeResponse, err error) {
	zlog.Debug().Msgf("ControllerExpandVolume")

	ID, err := strconv.ParseInt(req.GetVolumeId(), 10, 64)
	if err != nil {
		zlog.Error().Msgf("invalid Volume ID %v", err)
		return
	}

	capacity := int64(req.GetCapacityRange().GetRequiredBytes())
	if capacity < gib {
		capacity = gib
		zlog.Warn().Msgf("volume Minimum capacity should be greater than %d", gib)
	}

	// Expand file system size
	var fileSys api.FileSystem
	fileSys.Size = capacity
	_, err = nfs.cs.Api.UpdateFilesystem(ID, fileSys)
	if err != nil {
		zlog.Error().Msgf("failed to update file system %v", err)
		return
	}
	return &csi.ControllerExpandVolumeResponse{
		CapacityBytes:         capacity,
		NodeExpansionRequired: false,
	}, nil
}
