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
package nfs

import (
	"context"
	"fmt"
	"infinibox-csi-driver/common"
	"infinibox-csi-driver/helper"
	"infinibox-csi-driver/iboxapi"
	"infinibox-csi-driver/log"
	storagecommon "infinibox-csi-driver/storage/common"
	"strconv"
	"strings"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"k8s.io/mount-utils"
)

var zlog = log.Get() // grab the logger for package use

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

type NFSstorage struct {
	UniqueID               int
	StorageClassParameters map[string]string
	PVName                 string
	Capacity               int64
	FileSystemID           int
	ExportPath             string
	UsePrivilegedPorts     bool
	SnapdirVisible         bool
	ExportID               int
	ExportBlock            string
	IPAddress              string
	CS                     storagecommon.Commonservice
	Mounter                mount.Interface
	OSHelper               helper.OsHelper
	StorageHelper          storagecommon.StorageHelper
	csi.UnimplementedControllerServer
	csi.UnimplementedNodeServer
}

type accessType int

const (
	// InfiniBox default values
	NfsExportPermissions = "RW"
	NoRootSquash         = true
	NfsUnixPermissions   = "777"
)

func NewNFSstorage(capacity int64, cs storagecommon.Commonservice) (nfs *NFSstorage) {
	nfs = &NFSstorage{
		Capacity:      capacity,
		CS:            cs,
		StorageHelper: storagecommon.StorageService{},
		OSHelper:      helper.Service{},
		Mounter:       mount.NewWithoutSystemd(""),
	}
	return nfs
}

func (nfs *NFSstorage) ValidateStorageClass(params map[string]string) error {
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
	err := storagecommon.ValidateRequiredOptionalSCParameters(requiredParams, optionalParams, suppliedParams)
	if err != nil {
		e := fmt.Errorf("%s (nfs) - error %s", function, err.Error())
		zlog.Error().Msg(e.Error())
		return status.Error(codes.InvalidArgument, e.Error())
	}

	useChap := suppliedParams[common.SC_USE_CHAP]
	if useChap != "" {
		zlog.Warn().Msgf("%s (nfs) - useCHAP is not a valid storage class parameter for nfs or nfs-treeq", function)
	}

	err = ValidateNFSExportPermissions(suppliedParams)
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
	nfs.SnapdirVisible = snapdirVisible

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
	nfs.UsePrivilegedPorts = usePrivilegedPorts

	return nil
}

func (nfs *NFSstorage) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	const function = "CreateVolume"
	zlog.Trace().Msgf("%s (nfs) called", function)
	var err error
	// Adding the the request parameter into Map params
	params := req.GetParameters()
	pvName := req.GetName()

	zlog.Debug().Msgf("%s (nfs) - csi request name %s, parameters %v, caps %+v, privport %t snapdir %t %s",
		function, req.Name, params, req.VolumeCapabilities, nfs.UsePrivilegedPorts, nfs.SnapdirVisible,
		storagecommon.GetHostInfo(req.GetSecrets(), nfs.CS.IboxApi))

	// basic sanity-checking to ensure the user is not requesting block access to a NFS filesystem
	for _, cap := range req.GetVolumeCapabilities() {
		if block := cap.GetBlock(); block != nil {
			e := fmt.Errorf("%s (nfs) - GetBlock - block access requested for %s PV %s", function, params[common.SC_STORAGE_PROTOCOL], req.GetName())
			zlog.Error().Msg(e.Error())
			return nil, status.Error(codes.InvalidArgument, e.Error())
		}
	}

	nfs.PVName = pvName
	nfs.StorageClassParameters = params
	nfs.ExportPath = "/" + pvName
	ipAddress, err := nfs.CS.GetNetworkSpaceIP(strings.Trim(params[common.SC_NETWORK_SPACE], " "))
	if err != nil {
		e := fmt.Errorf("%s (nfs) - getNetworkSpaceIP - error: %s", function, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}
	nfs.IPAddress = ipAddress
	zlog.Debug().Msgf("%s (nfs) - getNetworkSpaceIP ipAddress %s", function, nfs.IPAddress)

	// check if volume with given name already exists
	volume, err := nfs.CS.IboxApi.GetFileSystemByName(pvName)
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
		nfs.FileSystemID = volume.ID
		exportArray, err := nfs.CS.IboxApi.GetExportsByFileSystemID(nfs.FileSystemID)
		if err != nil {
			e := fmt.Errorf("%s (nfs) - GetExportByFileSystem fs ID %d- error: %s", function, nfs.FileSystemID, err.Error())
			zlog.Error().Msg(e.Error())
			return nil, status.Error(codes.Internal, e.Error())
		}
		if nfs.Capacity != volume.Size {
			e := fmt.Errorf("%s (nfs) - nfs.capacity not equal volume.Size capacity %d volume: %+v", function, nfs.Capacity, volume)
			zlog.Error().Msg(e.Error())
			return nil, status.Error(codes.AlreadyExists, e.Error())
		}
		for _, export := range exportArray {
			nfs.ExportBlock = export.ExportPath
			nfs.ExportID = export.ID
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
			csiResp, err = nfs.createVolumeFromPVCSource(req, nfs.Capacity, params[common.SC_POOL_NAME], snapshot.GetSnapshotId())
			if err != nil {
				e := fmt.Errorf("%s (nfs) - createVolumeFromPVCSource - failed to create volume from snapshot with error: %v", function, err)
				zlog.Error().Msg(e.Error())
				return nil, e
			}
		} else if contentSource.GetVolume() != nil {
			volume := req.GetVolumeContentSource().GetVolume()
			csiResp, err = nfs.createVolumeFromPVCSource(req, nfs.Capacity, params[common.SC_POOL_NAME], volume.GetVolumeId())
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

func (nfs *NFSstorage) createVolumeFromPVCSource(req *csi.CreateVolumeRequest, size int64, storagePool string, srcVolumeID string) (csiResp *csi.CreateVolumeResponse, err error) {
	const function = "createVolumeFromPVCSource"
	zlog.Debug().Msgf("%s (nfs)", function)

	volproto, err := storagecommon.ValidateVolumeID(srcVolumeID)
	if err != nil {
		e := fmt.Errorf("%s (nfs) - failed to validate volume id: %s, err: %v", function, srcVolumeID, err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.NotFound, e.Error())
	}
	sourceVolumeID := volproto.VolumeID

	// Look up the source volume
	srcfsys, err := nfs.CS.IboxApi.GetFileSystemByID(sourceVolumeID)
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
	pool, err := nfs.CS.IboxApi.GetPoolByName(storagePool)
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
	newSnapshot, err := nfs.CS.IboxApi.CreateFileSystemSnapshot(newSnapshotParams)
	if err != nil {
		e := fmt.Errorf("%s (nfs) - failed to create snapshot: %s error: %v", function, newSnapshotParams.SnapshotName, err)
		zlog.Err(e)
		return nil, status.Error(codes.Internal, e.Error())
	}
	zlog.Debug().Msgf("%s (nfs) - successfully created volume from clone with name: %s", function, newSnapshotName)
	nfs.FileSystemID = newSnapshot.SnapshotID

	err = nfs.CreateExportPathAndAddMetadata()
	if err != nil {
		e := fmt.Errorf("%s (nfs) - failed to create export and metadata, %v", function, err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	return nfs.getNfsCsiResponse(req), nil
}

// CreateNFSVolume create volume method
func (nfs *NFSstorage) CreateNFSVolume(req *csi.CreateVolumeRequest) (csiResp *csi.CreateVolumeResponse, err error) {
	const function = "CreateNFSVolume"
	err = nfs.CreateFileSystem(nfs.PVName)
	if err != nil {
		e := fmt.Errorf("%s (nfs) - failed to create file system, %v", function, err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	err = nfs.CreateExportPathAndAddMetadata()
	if err != nil {
		e := fmt.Errorf("%s (nfs) - failed to create export and metadata, %v", function, err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	return nfs.getNfsCsiResponse(req), nil
}

func (nfs *NFSstorage) CreateExportPathAndAddMetadata() (err error) {
	const function = "createExportPathAndAddMetadata"
	defer func() {
		if err != nil && nfs.FileSystemID != 0 {
			zlog.Debug().Msgf("%s (nfs) - seems to be some problem reverting filesystem: %s", function, nfs.PVName)
			if errDelFS := nfs.CS.IboxApi.DeleteFileSystem(nfs.FileSystemID); errDelFS != nil {
				zlog.Error().Msgf("%s (nfs) - failed to delete file system id: %d %v", function, nfs.FileSystemID, errDelFS)
			}
		}
	}()

	if nfs.StorageClassParameters[common.SC_NFS_EXPORT_PERMISSIONS] == "" {
		zlog.Debug().Msgf("%s (nfs) - nfs_export_permissions parameter is not set in the StorageClass, will use default export", function)
	} else {
		err = nfs.createExportPath()
		if err != nil {
			e := fmt.Errorf("%s (nfs) - failed to export path %v", function, err)
			zlog.Error().Msg(e.Error())
			return e
		}
		zlog.Debug().Msgf("%s (nfs) - export path created for filesytem: %s", function, nfs.PVName)
	}

	defer func() {
		if err != nil && nfs.ExportID != 0 {
			zlog.Debug().Msgf("%s (nfs) - seems to be some problem reverting created export id: %d", function, nfs.ExportID)
			if _, errDelExport := nfs.CS.IboxApi.DeleteExport(nfs.ExportID); errDelExport != nil {
				zlog.Error().Msgf("%s (nfs) - failed to delete export path for file system id: %d %v", function, nfs.FileSystemID, errDelExport)
			}
		}
	}()

	metadata := map[string]interface{}{
		"host.k8s.pvname": nfs.PVName,
		"host.created_by": nfs.CS.GetCreatedBy(),
	}

	_, err = nfs.CS.IboxApi.PutMetadata(nfs.FileSystemID, metadata)
	if err != nil {
		e := fmt.Errorf("%s (nfs) - failed to attach metadata for file system %s, %v", function, nfs.PVName, err)
		zlog.Error().Msg(e.Error())
		return e
	}
	zlog.Debug().Msgf("%s (nfs) - metadata attached successfully for file system %s", function, nfs.PVName)
	return nil
}

func (nfs *NFSstorage) createExportPath() (err error) {
	const function = "createExportPath"
	permissionsMapArray, err := getPermissionMaps(nfs.StorageClassParameters[common.SC_NFS_EXPORT_PERMISSIONS])
	if err != nil {
		e := fmt.Errorf("%s (nfs) - failed to parse permission map string %s %v", function, nfs.StorageClassParameters[common.SC_NFS_EXPORT_PERMISSIONS], err)
		zlog.Error().Msg(e.Error())
		return e
	}

	exportFileSystem := iboxapi.CreateExportRequest{
		FilesystemID:        nfs.FileSystemID,
		Transport_protocols: "TCP",
		Privileged_port:     nfs.UsePrivilegedPorts,
		SnapdirVisible:      nfs.SnapdirVisible,
		Export_path:         nfs.ExportPath,
	}
	exportFileSystem.Permissionsput = append(exportFileSystem.Permissionsput, permissionsMapArray...)
	var exportResp *iboxapi.Export
	exportResp, err = nfs.CS.IboxApi.CreateExport(exportFileSystem)
	if err != nil {
		e := fmt.Errorf("%s (nfs) - failed to create export path of filesystem %s %v", function, nfs.PVName, err)
		zlog.Error().Msg(e.Error())
		return e
	}
	nfs.ExportID = exportResp.ID
	nfs.ExportBlock = exportResp.ExportPath
	zlog.Debug().Msgf("%s (nfs) - created nfs export for PV '%s', snapdirVisible: %t", function, nfs.PVName, nfs.SnapdirVisible)
	return nil
}

func (nfs *NFSstorage) CreateFileSystem(fileSystemName string) (err error) {
	const function = "createFileSystem"
	poolName := nfs.StorageClassParameters[common.SC_POOL_NAME]
	pool, err := nfs.CS.IboxApi.GetPoolByName(poolName)
	if err != nil {
		e := fmt.Errorf("%s (nfs) - failed to get GetPoolID by pool_name %s %v", function, poolName, err)
		zlog.Error().Msg(e.Error())
		return e
	}
	provtype := strings.ToUpper(nfs.StorageClassParameters[common.SC_PROVISION_TYPE])
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
		Size:     nfs.Capacity,
		Provtype: provtype,
	}

	fsRequest.SsdEnabled, err = storagecommon.DetermineSSDValue(nfs.StorageClassParameters[common.SC_SSD_ENABLED], poolName, nfs.CS.IboxApi)
	if err != nil {
		e := status.Errorf(codes.Internal, "%s (nfs) - error when creating filesystem %s storagepool %s, err: %s", function, fileSystemName, poolName, err.Error())
		zlog.Error().Msg(e.Error())
		return e
	}

	fileSystem, err := nfs.CS.IboxApi.CreateFileSystem(fsRequest)
	if err != nil {
		e := fmt.Errorf("%s (nfs) - failed to create filesystem %s %v", function, fileSystemName, err)
		zlog.Error().Msg(e.Error())
		return e
	}
	nfs.FileSystemID = fileSystem.ID
	zlog.Debug().Msgf("%s (nfs) - filesystem Created %s", function, fileSystemName)
	return nil
}

func (nfs *NFSstorage) getNfsCsiResponse(req *csi.CreateVolumeRequest) *csi.CreateVolumeResponse {
	infinidatVol := &infinidatVolume{
		VolID:     fmt.Sprint(nfs.FileSystemID),
		VolPath:   nfs.ExportPath,
		IpAddress: nfs.IPAddress,
		ExportID:  nfs.ExportID,
	}
	nfs.StorageClassParameters["ipAddress"] = (*infinidatVol).IpAddress
	nfs.StorageClassParameters["exportID"] = strconv.Itoa(int((*infinidatVol).ExportID))
	nfs.StorageClassParameters["volPathd"] = (*infinidatVol).VolPath

	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      (*infinidatVol).VolID,
			CapacityBytes: nfs.Capacity,
			VolumeContext: nfs.StorageClassParameters,
			ContentSource: req.GetVolumeContentSource(),
		},
	}
}

func (nfs *NFSstorage) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	const function = "DeleteVolume"
	volproto := nfs.CS.VolProto

	nfs.UniqueID = volproto.VolumeID
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
func (nfs *NFSstorage) DeleteNFSVolume() (err error) {
	const function = "DeleteNFSVolume"
	fs, fileSystemErr := nfs.CS.IboxApi.GetFileSystemByID(nfs.UniqueID)
	if fileSystemErr != nil {
		zlog.Error().Msgf("%s (nfs) - failed to get file system by ID %d %v", function, nfs.UniqueID, fileSystemErr)
		err = fileSystemErr
		return
	}

	if fs.LockState == common.LOCKED_STATE {
		return status.Errorf(codes.Aborted, "%s (nfs) - snapshot %d is locked and can't be deleted till it expires at %s", function, nfs.UniqueID, time.UnixMilli(fs.LockExpiresAt))
	}

	fileSystems, err := nfs.CS.IboxApi.GetFileSystemsByParentID(nfs.UniqueID)
	if err != nil {
		e := fmt.Errorf("%s (nfs) - failed to get file systems by parentID %d %v", function, nfs.UniqueID, err)
		zlog.Error().Msg(e.Error())
		return e
	}
	if len(fileSystems) > 0 {
		metadata := map[string]interface{}{
			storagecommon.TOBEDELETED: true,
		}
		_, err = nfs.CS.IboxApi.PutMetadata(nfs.UniqueID, metadata)
		if err != nil {
			e := fmt.Errorf("%s (nfs) - failed to update host.k8s.to_be_deleted for filesystem %s error: %v", function, nfs.PVName, err)
			zlog.Error().Msg(e.Error())
			return e
		}
		return nil
	}

	err = nfs.CS.Api.DeleteFileSystemComplete(nfs.UniqueID)
	if err != nil {
		e := fmt.Errorf("%s (nfs) - failed to delete filesystem %s error: %v id: %d parentID: %d", function, nfs.PVName, err, nfs.UniqueID, fs.ParentID)
		zlog.Error().Msg(e.Error())
		return e
	}

	if fs.ParentID != 0 {
		err = nfs.CS.Api.DeleteParentFileSystem(fs.ParentID)
		if err != nil {
			e := fmt.Errorf("%s (nfs) - failed to delete filesystem's %s parent filesystems error: %v", function, nfs.PVName, err)
			zlog.Error().Msg(e.Error())
			return e
		}

	}
	return nil
}

func (nfs *NFSstorage) ControllerModifyVolume(ctx context.Context, req *csi.ControllerModifyVolumeRequest) (*csi.ControllerModifyVolumeResponse, error) {
	return nil, nil
}

func (nfs *NFSstorage) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	const function = "ControllerPublishVolume"
	var err error
	volumeID := req.GetVolumeId()
	exportID := req.GetVolumeContext()["exportID"]

	zlog.Debug().Msgf("%s (nfs) - nodeId %s volumeID %s exportID %s nfs_export_permissions %s %s",
		function, req.GetNodeId(), volumeID, exportID, req.GetVolumeContext()[common.SC_NFS_EXPORT_PERMISSIONS],
		storagecommon.GetHostInfo(req.GetSecrets(), nfs.CS.IboxApi))

	kubeNodeID := req.GetNodeId()
	if kubeNodeID == "" {
		e := fmt.Errorf("%s (nfs) - node ID is required", function)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	_, err = nfs.CS.AccessModesHelper.IsValidAccessModeNfs(req)
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
	_, err = nfs.CS.Api.AddNodeInExport(exportid, access, noRootSquash, nodeIP)
	if err != nil {
		e := fmt.Errorf("%s (nfs) - AddNodeInExport - failed to add export rule, %v", function, err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	return &csi.ControllerPublishVolumeResponse{}, nil
}

func (nfs *NFSstorage) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {

	zlog.Debug().Msgf("ControllerUnpublishVolume (nfs) - volproto %+v", nfs.CS.VolProto)

	err := nfs.CS.Api.DeleteExportRule(nfs.CS.VolProto.VolumeID, nfs.CS.VolProto.NodeID)
	if err != nil {
		e := fmt.Errorf("ControllerUnpublishVolume (nfs) - DeleteExportRule - fileystemID %d error %v", nfs.CS.VolProto.VolumeID, err)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

func (nfs *NFSstorage) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (resp *csi.ValidateVolumeCapabilitiesResponse, err error) {
	zlog.Error().Msgf("ValidateVolumeCapabilities (nfs) - should not be called, implemented in controller.go")
	return
}

func (nfs *NFSstorage) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	return &csi.ListVolumesResponse{}, nil
}

func (nfs *NFSstorage) ListSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	return &csi.ListSnapshotsResponse{}, nil
}

func (nfs *NFSstorage) GetCapacity(ctx context.Context, req *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error) {
	return &csi.GetCapacityResponse{}, nil
}

func (nfs *NFSstorage) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	return &csi.ControllerGetCapabilitiesResponse{}, nil
}

func (nfs *NFSstorage) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (createSnapshot *csi.CreateSnapshotResponse, err error) {
	const function = "CreateSnapshot"
	zlog.Debug().Msgf("%s (nfs) - parameters %+v snapshotName %s source volume ID %s", function, req.Parameters, req.GetName(), req.GetSourceVolumeId())
	var snapshotID string
	snapshotName := req.GetName()
	srcVolumeId := req.GetSourceVolumeId()

	sourceFilesystemID := nfs.CS.VolProto.VolumeID
	snap, err := nfs.CS.IboxApi.GetFileSystemByName(snapshotName)
	if err != nil {
		re, ok := err.(*iboxapi.IboxAPIError)
		if ok && re.Code == iboxapi.IBOXAPI_RESOURCE_NOT_FOUND_ERROR {
		} else {
			e := fmt.Errorf("%s (nfs) - GetSnapshotByName %d - error: %v", function, nfs.CS.VolProto.VolumeID, err)
			zlog.Error().Msg(e.Error())
			return nil, e
		}
	}

	if snap != nil {
		if snap.ParentID == sourceFilesystemID {
			snapshotID = strconv.Itoa(snap.ID) + "$$" + nfs.CS.VolProto.StorageType
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

	parentFilesystem, err := nfs.CS.IboxApi.GetFileSystemByID(sourceFilesystemID)
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
		ntpStatus, err := nfs.CS.IboxApi.GetNtpStatus()
		if err != nil {
			e := fmt.Errorf("%s (nfs) - GetNtpStatus - failed to get ntp status error %v", function, err)
			zlog.Error().Msg(e.Error())
			return nil, e
		}
		lockExpiresAt, err = storagecommon.ValidateSnapshotLockingParameter(ntpStatus[0].LastProbeTimestamp, lockExpiresAtParameter)
		if err != nil {
			e := fmt.Errorf("%s (nfs) - validateSnapshotLockingParameter - failed to create snapshot %s error %v, invalid lock_expires_at parameter ", function, snapshotName, err)
			zlog.Error().Msg(e.Error())
			return nil, e
		}
		zlog.Debug().Msgf("%s (nfs) - snapshot param has a lock_expires_at of %s", function, lockExpiresAtParameter)
		fileSystemSnapshot.LockExpiresAt = lockExpiresAt
	}
	resp, err := nfs.CS.IboxApi.CreateFileSystemSnapshot(fileSystemSnapshot)
	if err != nil {
		e := fmt.Errorf("%s (nfs) - CreateFileSystemSnapshot - failed to create snapshot %s error %v", function, snapshotName, err)
		zlog.Error().Msg(e.Error())
		return nil, e
	}

	snapshotID = strconv.Itoa(resp.SnapshotID) + "$$" + nfs.CS.VolProto.StorageType
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

func (nfs *NFSstorage) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (deleteSnapshot *csi.DeleteSnapshotResponse, err error) {
	const function = "DeleteSnapshot"
	zlog.Debug().Msgf("%s (nfs) - snapshotID %s", function, req.GetSnapshotId())

	snapshotID, err := strconv.Atoi(req.GetSnapshotId())
	if err != nil {
		e := fmt.Errorf("%s (nfs) - failed to parse int from snapshotID, %s, error %s", function, req.GetSnapshotId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Aborted, e.Error())
	}
	nfs.UniqueID = snapshotID

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

func (nfs *NFSstorage) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (expandVolume *csi.ControllerExpandVolumeResponse, err error) {
	ID := nfs.CS.VolProto.VolumeID
	zlog.Debug().Msgf("ControllerExpandVolume (nfs) - fs ID %d", ID)

	capacity := int64(req.GetCapacityRange().GetRequiredBytes())
	if capacity < storagecommon.GIB {
		capacity = storagecommon.GIB
		zlog.Warn().Msgf("ControllerExpandVolume (nfs) - volume Minimum capacity should be greater than %d", storagecommon.GIB)
	}

	// Expand file system size
	var fileSys iboxapi.FileSystem
	fileSys.Size = capacity
	_, err = nfs.CS.IboxApi.UpdateFileSystem(ID, fileSys)
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

func (st *NFSstorage) ControllerGetVolume(
	_ context.Context, _ *csi.ControllerGetVolumeRequest,
) (*csi.ControllerGetVolumeResponse, error) {
	// Infinidat does not support ControllerGetVolume
	return nil, status.Error(codes.Unimplemented, "")
}
