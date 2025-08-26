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
	"fmt"
	"infinibox-csi-driver/common"
	"infinibox-csi-driver/iboxapi"
	"strconv"
	"strings"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
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
	ExportID      int        `json:"exportID"`
	FileSystemID  int        `json:"fileSystemID"`
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
	const function = "ValidateStorageClass"
	requiredParams := map[string]string{
		common.SC_NETWORK_SPACE: `\A.*\z`,    // TODO: could make this enforce IBOX network_space requirements, but probably not necessary
		common.SC_POOL_NAME:     `[a-zA-Z]+`, //match all strings except empty string or blank string
	}

	optionalParams := map[string]string{
		common.SC_UID: `^\d+$`,
		common.SC_GID: `^\d+$`,
	}

	suppliedParams := params
	err := ValidateRequiredOptionalSCParameters(requiredParams, optionalParams, suppliedParams)
	if err != nil {
		e := fmt.Errorf("%s (nfs) - error %s", function, err.Error())
		zlog.Error().Msg(e.Error())
		return status.Error(codes.InvalidArgument, e.Error())
	}

	useChap := suppliedParams[common.SC_USE_CHAP]
	if useChap != "" {
		zlog.Warn().Msgf("%s (nfs) - useCHAP is not a valid storage class parameter for nfs or nfs-treeq", function)
	}

	err = validateNFSExportPermissions(suppliedParams)
	if err != nil {
		e := fmt.Errorf("%s (nfs) - error %s", function, err.Error())
		zlog.Error().Msg(e.Error())
		return status.Error(codes.InvalidArgument, e.Error())
	}

	snapdirVisible := false
	snapdirVisibleString := params[common.SC_SNAPDIR_VISIBLE]
	if snapdirVisibleString != "" {
		snapdirVisible, err = strconv.ParseBool(snapdirVisibleString)
		if err != nil {
			e := fmt.Errorf("%s (nfs) - invalid NFS snapdir_visible value: %s, error: %v", function, snapdirVisibleString, err)
			zlog.Error().Msg(e.Error())
			return status.Error(codes.InvalidArgument, e.Error())
		}
	}
	nfs.snapdirVisible = snapdirVisible

	usePrivilegedPorts := false
	usePrivilegedPortsString := params[common.SC_PRIV_PORTS]
	if usePrivilegedPortsString != "" {
		usePrivilegedPorts, err = strconv.ParseBool(usePrivilegedPortsString)
		if err != nil {
			e := fmt.Errorf("%s (nfs) - invalid NFS privileged_ports_only value: %s, error: %v", function, usePrivilegedPortsString, err)
			zlog.Err(e)
			return status.Error(codes.InvalidArgument, e.Error())
		}
	}
	nfs.usePrivilegedPorts = usePrivilegedPorts

	return nil
}

func (nfs *nfsstorage) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	const function = "CreateVolume"
	zlog.Trace().Msgf("%s (nfs) called", function)
	var err error
	// Adding the the request parameter into Map params
	params := req.GetParameters()
	pvName := req.GetName()

	zlog.Debug().Msgf("%s (nfs) - csi request name %s, parameters %v, caps %+v, privport %t snapdir %t",
		function, req.Name, params, req.VolumeCapabilities, nfs.usePrivilegedPorts, nfs.snapdirVisible)

	// basic sanity-checking to ensure the user is not requesting block access to a NFS filesystem
	for _, cap := range req.GetVolumeCapabilities() {
		if block := cap.GetBlock(); block != nil {
			e := fmt.Errorf("%s (nfs) - GetBlock - block access requested for %s PV %s", function, params[common.SC_STORAGE_PROTOCOL], req.GetName())
			zlog.Error().Msg(e.Error())
			return nil, status.Error(codes.InvalidArgument, e.Error())
		}
	}

	nfs.pVName = pvName
	nfs.storageClassParameters = params
	nfs.exportPath = "/" + pvName
	ipAddress, err := nfs.cs.getNetworkSpaceIP(strings.Trim(params[common.SC_NETWORK_SPACE], " "))
	if err != nil {
		e := fmt.Errorf("%s (nfs) - getNetworkSpaceIP - error: %s", function, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}
	nfs.ipAddress = ipAddress
	zlog.Debug().Msgf("%s (nfs) - getNetworkSpaceIP ipAddress %s", function, nfs.ipAddress)

	// check if volume with given name already exists
	volume, err := nfs.cs.IboxApi.GetFileSystemByName(pvName)
	if err != nil {
		e := fmt.Errorf("%s (nfs) - GetFileSystemByName pvName %s- error: %s", function, pvName, err.Error())
		zlog.Error().Msg(e.Error())
		re, ok := err.(*iboxapi.IboxAPIError)
		if ok && re.Code == iboxapi.IBOXAPI_RESOURCE_NOT_FOUND_ERROR {
			zlog.Debug().Msgf("%s (nfs) - GetFileSystemByName error: %v, will proceed to create it", function, err)
			//return nil, status.Errorf(codes.NotFound, "error CreateVolume failed: %v", err)
		} else {
			return nil, status.Errorf(codes.Internal, "%s (nfs) error: %v", function, err)
		}
	}
	if volume != nil {
		// return existing volume
		nfs.fileSystemID = volume.ID
		exportArray, err := nfs.cs.IboxApi.GetExportsByFileSystemID(nfs.fileSystemID)
		if err != nil {
			e := fmt.Errorf("%s (nfs) - GetExportByFileSystem fs ID %d- error: %s", function, nfs.fileSystemID, err.Error())
			zlog.Error().Msg(e.Error())
			return nil, status.Error(codes.Internal, e.Error())
		}
		if nfs.capacity != volume.Size {
			e := fmt.Errorf("%s (nfs) - nfs.capacity not equal volume.Size capacity %d volume: %+v", function, nfs.capacity, volume)
			zlog.Error().Msg(e.Error())
			return nil, status.Error(codes.AlreadyExists, e.Error())
		}
		for _, export := range exportArray {
			nfs.exportBlock = export.ExportPath
			nfs.exportID = export.ID
			break
		}
		return nfs.getNfsCsiResponse(req), nil
	}

	// Volume content source support Volumes and Snapshots
	contentSource := req.GetVolumeContentSource()
	var csiResp *csi.CreateVolumeResponse
	if contentSource != nil {
		if contentSource.GetSnapshot() != nil {
			snapshot := req.GetVolumeContentSource().GetSnapshot()
			csiResp, err = nfs.createVolumeFromPVCSource(req, nfs.capacity, params[common.SC_POOL_NAME], snapshot.GetSnapshotId())
			if err != nil {
				e := fmt.Errorf("%s (nfs) - createVolumeFromPVCSource - failed to create volume from snapshot with error: %v", function, err)
				zlog.Error().Msg(e.Error())
				return nil, e
			}
		} else if contentSource.GetVolume() != nil {
			volume := req.GetVolumeContentSource().GetVolume()
			csiResp, err = nfs.createVolumeFromPVCSource(req, nfs.capacity, params[common.SC_POOL_NAME], volume.GetVolumeId())
			if err != nil {
				e := fmt.Errorf("%s (nfs) - createVolumeFromPVCSource - failed to create volume from pvc with error: %v", function, err)
				zlog.Error().Msg(e.Error())
				return nil, e
			}
		}
	} else {
		csiResp, err = nfs.CreateNFSVolume(req)
		if err != nil {
			e := fmt.Errorf("%s (nfs) - CreateNFSVolume - error: %s", function, err.Error())
			zlog.Error().Msg(e.Error())
			return nil, e
		}
	}
	return csiResp, nil
}

func (nfs *nfsstorage) createVolumeFromPVCSource(req *csi.CreateVolumeRequest, size int64, storagePool string, srcVolumeID string) (csiResp *csi.CreateVolumeResponse, err error) {
	const function = "createVolumeFromPVCSource"
	zlog.Debug().Msgf("%s (nfs)", function)

	volproto, err := ValidateVolumeID(srcVolumeID)
	if err != nil {
		e := fmt.Errorf("%s (nfs) - failed to validate volume id: %s, err: %v", function, srcVolumeID, err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.NotFound, e.Error())
	}
	sourceVolumeID := volproto.VolumeID

	// Look up the source volume
	srcfsys, err := nfs.cs.IboxApi.GetFileSystemByID(sourceVolumeID)
	if err != nil {
		e := fmt.Errorf("%s (nfs) - volume not found: %d", function, sourceVolumeID)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.NotFound, e.Error())
	}

	// Check that the requested volume size matches the size of source volume
	if srcfsys.Size != size {
		e := fmt.Errorf("%s (nfs) - volume %d, invalid size %d, requested %d ", function, sourceVolumeID, srcfsys.Size, size)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	// Check that the requested storagePool matches the source
	pool, err := nfs.cs.IboxApi.GetPoolByName(storagePool)
	if err != nil {
		e := fmt.Errorf("%s (nfs) - error GetPoolByName: %s", function, storagePool)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}
	if pool.ID != srcfsys.PoolID {
		e := fmt.Errorf("%s (nfs) - source storagepool id differs from requested: %s", function, storagePool)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	newSnapshotName := req.GetName() // create snapshot using the original CreateVolumeRequest
	newSnapshotParams := iboxapi.FileSystemSnapshot{ParentID: sourceVolumeID, SnapshotName: newSnapshotName, WriteProtected: false}
	zlog.Debug().Msgf("%s (nfs) - CreateFileSystemSnapshot: %v", function, newSnapshotParams)
	// Create snapshot
	newSnapshot, err := nfs.cs.IboxApi.CreateFileSystemSnapshot(newSnapshotParams)
	if err != nil {
		e := fmt.Errorf("%s (nfs) - failed to create snapshot: %s error: %v", function, newSnapshotParams.SnapshotName, err)
		zlog.Err(e)
		return nil, status.Error(codes.Internal, e.Error())
	}
	zlog.Debug().Msgf("%s (nfs) - successfully created volume from clone with name: %s", function, newSnapshotName)
	nfs.fileSystemID = newSnapshot.SnapshotID

	err = nfs.createExportPathAndAddMetadata()
	if err != nil {
		e := fmt.Errorf("%s (nfs) - failed to create export and metadata, %v", function, err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	return nfs.getNfsCsiResponse(req), nil
}

// CreateNFSVolume create volume method
func (nfs *nfsstorage) CreateNFSVolume(req *csi.CreateVolumeRequest) (csiResp *csi.CreateVolumeResponse, err error) {
	const function = "CreateNFSVolume"
	err = nfs.createFileSystem(nfs.pVName)
	if err != nil {
		e := fmt.Errorf("%s (nfs) - failed to create file system, %v", function, err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	err = nfs.createExportPathAndAddMetadata()
	if err != nil {
		e := fmt.Errorf("%s (nfs) - failed to create export and metadata, %v", function, err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	return nfs.getNfsCsiResponse(req), nil
}

func (nfs *nfsstorage) createExportPathAndAddMetadata() (err error) {
	const function = "createExportPathAndAddMetadata"
	defer func() {
		if err != nil && nfs.fileSystemID != 0 {
			zlog.Debug().Msgf("%s (nfs) - seems to be some problem reverting filesystem: %s", function, nfs.pVName)
			if errDelFS := nfs.cs.IboxApi.DeleteFileSystem(nfs.fileSystemID); errDelFS != nil {
				zlog.Error().Msgf("%s (nfs) - failed to delete file system id: %d %v", function, nfs.fileSystemID, errDelFS)
			}
		}
	}()

	if nfs.storageClassParameters[common.SC_NFS_EXPORT_PERMISSIONS] == "" {
		zlog.Debug().Msgf("%s (nfs) - nfs_export_permissions parameter is not set in the StorageClass, will use default export", function)
	} else {
		err = nfs.createExportPath()
		if err != nil {
			e := fmt.Errorf("%s (nfs) - failed to export path %v", function, err)
			zlog.Error().Msg(e.Error())
			return e
		}
		zlog.Debug().Msgf("%s (nfs) - export path created for filesytem: %s", function, nfs.pVName)
	}

	defer func() {
		if err != nil && nfs.exportID != 0 {
			zlog.Debug().Msgf("%s (nfs) - seems to be some problem reverting created export id: %d", function, nfs.exportID)
			if _, errDelExport := nfs.cs.IboxApi.DeleteExport(nfs.exportID); errDelExport != nil {
				zlog.Error().Msgf("%s (nfs) - failed to delete export path for file system id: %d %v", function, nfs.fileSystemID, errDelExport)
			}
		}
	}()

	metadata := map[string]interface{}{
		"host.k8s.pvname": nfs.pVName,
		"host.created_by": nfs.cs.GetCreatedBy(),
	}

	_, err = nfs.cs.IboxApi.PutMetadata(nfs.fileSystemID, metadata)
	if err != nil {
		e := fmt.Errorf("%s (nfs) - failed to attach metadata for file system %s, %v", function, nfs.pVName, err)
		zlog.Error().Msg(e.Error())
		return e
	}
	zlog.Debug().Msgf("%s (nfs) - metadata attached successfully for file system %s", function, nfs.pVName)
	return nil
}

func (nfs *nfsstorage) createExportPath() (err error) {
	const function = "createExportPath"
	permissionsMapArray, err := getPermissionMaps(nfs.storageClassParameters[common.SC_NFS_EXPORT_PERMISSIONS])
	if err != nil {
		e := fmt.Errorf("%s (nfs) - failed to parse permission map string %s %v", function, nfs.storageClassParameters[common.SC_NFS_EXPORT_PERMISSIONS], err)
		zlog.Error().Msg(e.Error())
		return e
	}

	exportFileSystem := iboxapi.CreateExportRequest{
		FilesystemID:        nfs.fileSystemID,
		Transport_protocols: "TCP",
		Privileged_port:     nfs.usePrivilegedPorts,
		SnapdirVisible:      nfs.snapdirVisible,
		Export_path:         nfs.exportPath,
	}
	exportFileSystem.Permissionsput = append(exportFileSystem.Permissionsput, permissionsMapArray...)
	var exportResp *iboxapi.Export
	exportResp, err = nfs.cs.IboxApi.CreateExport(exportFileSystem)
	if err != nil {
		e := fmt.Errorf("%s (nfs) - failed to create export path of filesystem %s %v", function, nfs.pVName, err)
		zlog.Error().Msg(e.Error())
		return e
	}
	nfs.exportID = exportResp.ID
	nfs.exportBlock = exportResp.ExportPath
	zlog.Debug().Msgf("%s (nfs) - created nfs export for PV '%s', snapdirVisible: %t", function, nfs.pVName, nfs.snapdirVisible)
	return nil
}

func (nfs *nfsstorage) createFileSystem(fileSystemName string) (err error) {
	const function = "createFileSystem"
	poolName := nfs.storageClassParameters[common.SC_POOL_NAME]
	pool, err := nfs.cs.IboxApi.GetPoolByName(poolName)
	if err != nil {
		e := fmt.Errorf("%s (nfs) - failed to get GetPoolID by pool_name %s %v", function, poolName, err)
		zlog.Error().Msg(e.Error())
		return e
	}
	provtype := strings.ToUpper(nfs.storageClassParameters[common.SC_PROVISION_TYPE])
	switch provtype {
	case "":
		provtype = common.SC_THIN_PROVISION_TYPE
	case common.SC_THIN_PROVISION_TYPE, common.SC_THICK_PROVISION_TYPE:
	default:
		errStr := fmt.Sprintf("%s (nfs) - %s valid values are THICK or THIN, THIN is the default when not specified, entered value was [%s]", function, common.SC_PROVISION_TYPE, provtype)
		zlog.Error().Msg(errStr)
		return fmt.Errorf("%s", errStr)
	}

	if provtype == "" {
		provtype = common.SC_THIN_PROVISION_TYPE
	}
	fsRequest := iboxapi.CreateFileSystemRequest{
		PoolID:   pool.ID,
		Name:     fileSystemName,
		Size:     nfs.capacity,
		Provtype: provtype,
	}

	fsRequest.SsdEnabled, err = determineSSDValue(nfs.storageClassParameters[common.SC_SSD_ENABLED], poolName, nfs.cs.IboxApi)
	if err != nil {
		e := status.Errorf(codes.Internal, "%s (nfs) - error when creating filesystem %s storagepool %s, err: %s", function, fileSystemName, poolName, err.Error())
		zlog.Error().Msg(e.Error())
		return e
	}

	fileSystem, err := nfs.cs.IboxApi.CreateFileSystem(fsRequest)
	if err != nil {
		e := fmt.Errorf("%s (nfs) - failed to create filesystem %s %v", function, fileSystemName, err)
		zlog.Error().Msg(e.Error())
		return e
	}
	nfs.fileSystemID = fileSystem.ID
	zlog.Debug().Msgf("%s (nfs) - filesystem Created %s", function, fileSystemName)
	return nil
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
	const function = "DeleteVolume"
	volproto := nfs.cs.VolProto

	nfs.uniqueID = volproto.VolumeID
	nfsDeleteErr := nfs.DeleteNFSVolume()
	if nfsDeleteErr != nil {
		zlog.Err(nfsDeleteErr)
		if strings.Contains(nfsDeleteErr.Error(), "FILESYSTEM_NOT_FOUND") {
			zlog.Error().Msgf("%s (nfs) - file system already delete from infinibox", function)
			return &csi.DeleteVolumeResponse{}, nil
		}
		zlog.Error().Msgf("%s (nfs) - failed to delete NFS Volume ID %s, %v", function, req.GetVolumeId(), nfsDeleteErr)
		return nil, nfsDeleteErr
	}
	zlog.Debug().Msgf("%s (nfs) - volume %s successfully deleted", function, req.GetVolumeId())
	return &csi.DeleteVolumeResponse{}, nil
}

// DeleteNFSVolume delete volume method
func (nfs *nfsstorage) DeleteNFSVolume() (err error) {
	const function = "DeleteNFSVolume"
	fs, fileSystemErr := nfs.cs.IboxApi.GetFileSystemByID(nfs.uniqueID)
	if fileSystemErr != nil {
		zlog.Error().Msgf("%s (nfs) - failed to get file system by ID %d %v", function, nfs.uniqueID, fileSystemErr)
		err = fileSystemErr
		return
	}

	if fs.LockState == common.LOCKED_STATE {
		return status.Errorf(codes.Aborted, "%s (nfs) - snapshot %d is locked and can't be deleted till it expires at %s", function, nfs.uniqueID, time.UnixMilli(fs.LockExpiresAt))
	}

	fileSystems, err := nfs.cs.IboxApi.GetFileSystemsByParentID(nfs.uniqueID)
	if err != nil {
		e := fmt.Errorf("%s (nfs) - failed to get file systems by parentID %d %v", function, nfs.uniqueID, err)
		zlog.Error().Msg(e.Error())
		return e
	}
	if len(fileSystems) > 0 {
		metadata := map[string]interface{}{
			TOBEDELETED: true,
		}
		_, err = nfs.cs.IboxApi.PutMetadata(nfs.uniqueID, metadata)
		if err != nil {
			e := fmt.Errorf("%s (nfs) - failed to update host.k8s.to_be_deleted for filesystem %s error: %v", function, nfs.pVName, err)
			zlog.Error().Msg(e.Error())
			return e
		}
		return nil
	}

	err = nfs.cs.Api.DeleteFileSystemComplete(nfs.uniqueID)
	if err != nil {
		e := fmt.Errorf("%s (nfs) - failed to delete filesystem %s error: %v id: %d parentID: %d", function, nfs.pVName, err, nfs.uniqueID, fs.ParentID)
		zlog.Error().Msg(e.Error())
		return e
	}

	if fs.ParentID != 0 {
		err = nfs.cs.Api.DeleteParentFileSystem(fs.ParentID)
		if err != nil {
			e := fmt.Errorf("%s (nfs) - failed to delete filesystem's %s parent filesystems error: %v", function, nfs.pVName, err)
			zlog.Error().Msg(e.Error())
			return e
		}

	}
	return nil
}

func (nfs *nfsstorage) ControllerModifyVolume(ctx context.Context, req *csi.ControllerModifyVolumeRequest) (*csi.ControllerModifyVolumeResponse, error) {
	return nil, nil
}

func (nfs *nfsstorage) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	const function = "ControllerPublishVolume"
	var err error
	volumeID := req.GetVolumeId()
	exportID := req.GetVolumeContext()["exportID"]

	zlog.Debug().Msgf("%s (nfs) - nodeId %s volumeID %s exportID %s nfs_export_permissions %s",
		function, req.GetNodeId(), volumeID, exportID, req.GetVolumeContext()[common.SC_NFS_EXPORT_PERMISSIONS])

	kubeNodeID := req.GetNodeId()
	if kubeNodeID == "" {
		e := fmt.Errorf("%s (nfs) - node ID is required", function)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	_, err = nfs.cs.AccessModesHelper.IsValidAccessModeNfs(req)
	if err != nil {
		e := fmt.Errorf("%s (nfs) - IsValidAccessModeNfs - error: %s", function, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	if req.GetVolumeContext()[common.SC_NFS_EXPORT_PERMISSIONS] == "" {
		zlog.Debug().Msg(fmt.Sprintf("%s (nfs) - nfs_export_permissions parameter not set, volume ID %s export ID %s", function, volumeID, exportID))
		return &csi.ControllerPublishVolumeResponse{}, nil
	}

	// proceed to create a default export rule using the Node ip address

	exportPermissionMapArray, err := getPermissionMaps(req.GetVolumeContext()[common.SC_NFS_EXPORT_PERMISSIONS])
	if err != nil {
		e := fmt.Errorf("%s (nfs) - getPermissionsMaps - error: %s", function, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	zlog.Debug().Msgf("%s (nfs) - nfs export permissions for volume ID %s and export ID %s: %v", function, volumeID, exportID, exportPermissionMapArray)

	var access string
	if len(exportPermissionMapArray) > 0 {
		access = exportPermissionMapArray[0][NFS_EXPORT_PERM_ACCESS].(string)
	}

	noRootSquash := true // default value
	nodeNameIP := strings.Split(req.GetNodeId(), "$$")
	if len(nodeNameIP) != 2 {
		e := fmt.Errorf("%s (nfs) - node ID not found %v", function, nodeNameIP)
		zlog.Error().Msg(e.Error())
		return nil, e
	}
	nodeIP := nodeNameIP[1]
	exportid, _ := strconv.Atoi(exportID)
	_, err = nfs.cs.Api.AddNodeInExport(exportid, access, noRootSquash, nodeIP)
	if err != nil {
		e := fmt.Errorf("%s (nfs) - AddNodeInExport - failed to add export rule, %v", function, err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	return &csi.ControllerPublishVolumeResponse{}, nil
}

func (nfs *nfsstorage) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {

	zlog.Debug().Msgf("ControllerUnpublishVolume (nfs) - volproto %+v", nfs.cs.VolProto)

	err := nfs.cs.Api.DeleteExportRule(nfs.cs.VolProto.VolumeID, nfs.cs.VolProto.NodeID)
	if err != nil {
		e := fmt.Errorf("ControllerUnpublishVolume (nfs) - DeleteExportRule - fileystemID %d error %v", nfs.cs.VolProto.VolumeID, err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

func (nfs *nfsstorage) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (resp *csi.ValidateVolumeCapabilitiesResponse, err error) {
	zlog.Error().Msgf("ValidateVolumeCapabilities (nfs) - should not be called, implemented in controller.go")
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
	const function = "CreateSnapshot"
	zlog.Debug().Msgf("%s (nfs) - parameters %+v snapshotName %s source volume ID %s", function, req.Parameters, req.GetName(), req.GetSourceVolumeId())
	var snapshotID string
	snapshotName := req.GetName()
	srcVolumeId := req.GetSourceVolumeId()

	sourceFilesystemID := nfs.cs.VolProto.VolumeID
	snap, err := nfs.cs.IboxApi.GetFileSystemByName(snapshotName)
	if err != nil {
		re, ok := err.(*iboxapi.IboxAPIError)
		if ok && re.Code == iboxapi.IBOXAPI_RESOURCE_NOT_FOUND_ERROR {
		} else {
			e := fmt.Errorf("%s (nfs) - GetSnapshotByName %d - error: %v", function, nfs.cs.VolProto.VolumeID, err)
			zlog.Error().Msg(e.Error())
			return nil, e
		}
	}

	if snap != nil {
		if snap.ParentID == sourceFilesystemID {
			snapshotID = strconv.Itoa(snap.ID) + "$$" + nfs.cs.VolProto.StorageType
			zlog.Debug().Msgf("%s (nfs) - snapshot: %s src fs id: %d exists, snapshot id: %d", function, snapshotName, snap.ParentID, snap.ID)
			return &csi.CreateSnapshotResponse{
				Snapshot: &csi.Snapshot{
					SizeBytes:      snap.Size,
					SnapshotId:     snapshotID,
					SourceVolumeId: srcVolumeId,
					CreationTime:   timestamppb.Now(),
					ReadyToUse:     true,
				},
			}, nil
		}
		return nil, status.Error(codes.AlreadyExists, "CreateSnapshot (nfs) - snapshot with already existing name and different source volume ID")
	}

	parentFilesystem, err := nfs.cs.IboxApi.GetFileSystemByID(sourceFilesystemID)
	if err != nil {
		e := fmt.Errorf("%s (nfs) - GetFileSystemByID - error getting parent volume for snapshot - volume id %d - %s", function, sourceFilesystemID, err)
		zlog.Error().Msg(e.Error())
		return nil, e
	}

	fileSystemSnapshot := iboxapi.FileSystemSnapshot{
		ParentID:       sourceFilesystemID,
		SnapshotName:   snapshotName,
		WriteProtected: true,
		SsdEnabled:     parentFilesystem.SsdEnabled,
	}

	var lockExpiresAt int64
	lockExpiresAtParameter := req.Parameters[common.LOCK_EXPIRES_AT_PARAMETER]
	if lockExpiresAtParameter != "" {
		ntpStatus, err := nfs.cs.IboxApi.GetNtpStatus()
		if err != nil {
			e := fmt.Errorf("%s (nfs) - GetNtpStatus - failed to get ntp status error %v", function, err)
			zlog.Error().Msg(e.Error())
			return nil, e
		}
		lockExpiresAt, err = validateSnapshotLockingParameter(ntpStatus[0].LastProbeTimestamp, lockExpiresAtParameter)
		if err != nil {
			e := fmt.Errorf("%s (nfs) - validateSnapshotLockingParameter - failed to create snapshot %s error %v, invalid lock_expires_at parameter ", function, snapshotName, err)
			zlog.Error().Msg(e.Error())
			return nil, e
		}
		zlog.Debug().Msgf("%s (nfs) - snapshot param has a lock_expires_at of %s", function, lockExpiresAtParameter)
		fileSystemSnapshot.LockExpiresAt = lockExpiresAt
	}
	resp, err := nfs.cs.IboxApi.CreateFileSystemSnapshot(fileSystemSnapshot)
	if err != nil {
		e := fmt.Errorf("%s (nfs) - CreateFileSystemSnapshot - failed to create snapshot %s error %v", function, snapshotName, err)
		zlog.Error().Msg(e.Error())
		return nil, e
	}

	snapshotID = strconv.Itoa(resp.SnapshotID) + "$$" + nfs.cs.VolProto.StorageType
	snapshot := &csi.Snapshot{
		SnapshotId:     snapshotID,
		SourceVolumeId: srcVolumeId,
		ReadyToUse:     true,
		CreationTime:   timestamppb.Now(),
		SizeBytes:      resp.Size,
	}
	zlog.Debug().Msgf("%s (nfs) - response: %v", function, snapshot)
	snapshotResp := &csi.CreateSnapshotResponse{Snapshot: snapshot}
	return snapshotResp, nil
}

func (nfs *nfsstorage) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (deleteSnapshot *csi.DeleteSnapshotResponse, err error) {
	const function = "DeleteSnapshot"
	zlog.Debug().Msgf("%s (nfs) - snapshotID %s", function, req.GetSnapshotId())

	snapshotID, err := strconv.Atoi(req.GetSnapshotId())
	if err != nil {
		e := fmt.Errorf("%s (nfs) - failed to parse int from snapshotID, %s, error %s", function, req.GetSnapshotId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Aborted, e.Error())
	}
	nfs.uniqueID = snapshotID

	nfsSnapDeleteErr := nfs.DeleteNFSVolume()
	if nfsSnapDeleteErr != nil {
		zlog.Err(nfsSnapDeleteErr)
		if strings.Contains(nfsSnapDeleteErr.Error(), "FILESYSTEM_NOT_FOUND") {
			zlog.Error().Msgf("%s (nfs) - snapshot already delete from infinibox", function)
			deleteSnapshot = &csi.DeleteSnapshotResponse{}
			return
		}
		zlog.Error().Msgf("%s (nfs) - DeleteNFSVolume - error: %v", function, nfsSnapDeleteErr)
		err = nfsSnapDeleteErr
		return
	}
	deleteSnapshot = &csi.DeleteSnapshotResponse{}
	return
}

func (nfs *nfsstorage) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (expandVolume *csi.ControllerExpandVolumeResponse, err error) {
	ID := nfs.cs.VolProto.VolumeID
	zlog.Debug().Msgf("ControllerExpandVolume (nfs) - fs ID %d", ID)

	capacity := int64(req.GetCapacityRange().GetRequiredBytes())
	if capacity < gib {
		capacity = gib
		zlog.Warn().Msgf("ControllerExpandVolume (nfs) - volume Minimum capacity should be greater than %d", gib)
	}

	// Expand file system size
	var fileSys iboxapi.FileSystem
	fileSys.Size = capacity
	_, err = nfs.cs.IboxApi.UpdateFileSystem(ID, fileSys)
	if err != nil {
		e := fmt.Errorf("ControllerExpandVolume (nfs) - UpdateFileSystem - error: %v", err)
		zlog.Error().Msg(e.Error())
		return nil, e
	}
	return &csi.ControllerExpandVolumeResponse{
		CapacityBytes:         capacity,
		NodeExpansionRequired: false,
	}, nil
}

func (st *nfsstorage) ControllerGetVolume(
	_ context.Context, _ *csi.ControllerGetVolumeRequest,
) (*csi.ControllerGetVolumeResponse, error) {
	// Infinidat does not support ControllerGetVolume
	return nil, status.Error(codes.Unimplemented, "")
}
