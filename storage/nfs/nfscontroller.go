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
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/infinidat/infinibox-csi-driver/helper"
	"github.com/infinidat/infinibox-csi-driver/iboxapi"
	storagecommon "github.com/infinidat/infinibox-csi-driver/storage/common"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"k8s.io/mount-utils"
)

// NFSVolumeServiceType servier type
type VolumeServiceType interface {
	CreateNFSVolume() (*infinidatVolume, error)
	DeleteNFSVolume() error
}

type infinidatVolume struct {
	VolName       string     `json:"volName"`
	VolID         string     `json:"volID"`
	VolSize       int64      `json:"volSize"`
	VolPath       string     `json:"volPath"`
	IPAddress     string     `json:"ipAddress"`
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
	NFSExportPermissions = "RW"
	NoRootSquash         = true
	NFSUnixPermissions   = "777"
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
	requiredParams := map[string]string{
		common.StorageClassNetworkSpace: `\A.*\z`,    // TODO: could make this enforce IBOX network_space requirements, but probably not necessary
		common.StorageClassPoolName:     `[a-zA-Z]+`, // match all strings except empty string or blank string
	}

	optionalParams := map[string]string{
		common.StorageClassUID: `^\d+$`,
		common.StorageClassGID: `^\d+$`,
	}

	suppliedParams := params
	err := storagecommon.ValidateRequiredOptionalSCParameters(requiredParams, optionalParams, suppliedParams)
	if err != nil {
		e := fmt.Errorf("error: %s", err.Error())
		slog.Error(e.Error())
		return status.Error(codes.InvalidArgument, e.Error())
	}

	useChap := suppliedParams[common.StorageClassUseCHAP]
	if useChap != "" {
		slog.Warn("useCHAP is not a valid storage class parameter for nfs or nfs-treeq")
	}

	err = ValidateNFSExportPermissions(suppliedParams)
	if err != nil {
		e := fmt.Errorf("error: %s", err.Error())
		slog.Error(e.Error())
		return status.Error(codes.InvalidArgument, e.Error())
	}

	snapdirVisible := false
	snapdirVisibleString := params[common.StorageClassSnapDirVisible]
	if snapdirVisibleString != "" {
		snapdirVisible, err = strconv.ParseBool(snapdirVisibleString)
		if err != nil {
			e := fmt.Errorf("parsing error NFS snapdir_visible: %s, error: %s", snapdirVisibleString, err.Error())
			slog.Error(e.Error())
			return status.Error(codes.InvalidArgument, e.Error())
		}
	}
	nfs.SnapdirVisible = snapdirVisible

	usePrivilegedPorts := false
	usePrivilegedPortsString := params[common.StorageClassPrivPorts]
	if usePrivilegedPortsString != "" {
		usePrivilegedPorts, err = strconv.ParseBool(usePrivilegedPortsString)
		if err != nil {
			e := fmt.Errorf("parsing error NFS privileged_ports_only: %s error: %s", usePrivilegedPortsString, err.Error())
			slog.Error(e.Error())
			return status.Error(codes.InvalidArgument, e.Error())
		}
	}
	nfs.UsePrivilegedPorts = usePrivilegedPorts

	return nil
}

func (nfs *NFSstorage) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	slog.Log(ctx, common.LevelTrace, "starts")
	var err error
	// Adding the request parameter into Map params
	params := req.GetParameters()
	pvName := req.GetName()

	slog.Debug("starts", "name", req.Name, "params", params, "volcaps", req.VolumeCapabilities, "useprivports", nfs.UsePrivilegedPorts, "snapdirvisible", nfs.SnapdirVisible, "iboxInfo", storagecommon.GetHostInfo(ctx, req.GetSecrets(), nfs.CS.IboxAPI))

	// basic sanity-checking to ensure the user is not requesting block access to a NFS filesystem
	for _, cap := range req.GetVolumeCapabilities() {
		if block := cap.GetBlock(); block != nil {
			e := fmt.Errorf("from GetBlock - protocol: %s name: %s", params[common.StorageClassStorageProtocol], req.GetName())
			slog.Error(e.Error())
			return nil, status.Error(codes.InvalidArgument, e.Error())
		}
	}

	nfs.PVName = pvName
	nfs.StorageClassParameters = params
	nfs.ExportPath = "/" + pvName
	ipAddress, err := nfs.CS.GetNetworkSpaceIP(ctx, strings.Trim(params[common.StorageClassNetworkSpace], " "))
	if err != nil {
		e := fmt.Errorf("from GetNetworkSpaceIP - error: %s", err.Error())
		slog.Error(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}
	nfs.IPAddress = ipAddress
	slog.Debug("getNetworkSpaceIP", "ipAddress", nfs.IPAddress)

	// check if volume with given name already exists
	volume, err := nfs.CS.IboxAPI.GetFileSystemByName(ctx, pvName)
	if err != nil {
		e := fmt.Errorf("from GetFileSystemByName pvName: %s error: %s", pvName, err.Error())
		slog.Error(e.Error())
		re, ok := err.(*iboxapi.APIError)
		if ok && re.Code == iboxapi.RESOURCE_NOT_FOUND {
			slog.Debug("not found from GetFileSystemByName, will proceed to create it", "error", err)
			// return nil, status.Errorf(codes.NotFound, "error CreateVolume failed: %v", err)
		} else {
			return nil, status.Errorf(codes.Internal, "(nfs) error: %v", err)
		}
	}
	if volume != nil {
		// return existing volume
		nfs.FileSystemID = volume.ID
		exportArray, err := nfs.CS.IboxAPI.GetExportsByFileSystemID(ctx, nfs.FileSystemID)
		if err != nil {
			e := fmt.Errorf("from GetExportByFileSystem fsID: %d error: %s", nfs.FileSystemID, err.Error())
			slog.Error(e.Error())
			return nil, status.Error(codes.Internal, e.Error())
		}
		if nfs.Capacity != volume.Size {
			e := fmt.Errorf("error nfs.capacity not equal volume.Size capacity: %d volume: %+v", nfs.Capacity, volume)
			slog.Error(e.Error())
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
			csiResp, err = nfs.createVolumeFromPVCSource(ctx, req, nfs.Capacity, params[common.StorageClassPoolName], snapshot.GetSnapshotId())
			if err != nil {
				e := fmt.Errorf("from createVolumeFromPVCSource - failed to create volume from snapshot with error: %s", err.Error())
				slog.Error(e.Error())
				return nil, e
			}
		} else if contentSource.GetVolume() != nil {
			volume := req.GetVolumeContentSource().GetVolume()
			csiResp, err = nfs.createVolumeFromPVCSource(ctx, req, nfs.Capacity, params[common.StorageClassPoolName], volume.GetVolumeId())
			if err != nil {
				e := fmt.Errorf("from createVolumeFromPVCSource - failed to create volume from pvc with error: %s", err.Error())
				slog.Error(e.Error())
				return nil, e
			}
		}
	} else {
		csiResp, err = nfs.CreateNFSVolume(ctx, req)
		if err != nil {
			e := fmt.Errorf("from CreateNFSVolume - error: %s", err.Error())
			slog.Error(e.Error())
			return nil, e
		}
	}
	return csiResp, nil
}

// CreateNFSVolume create volume method
func (nfs *NFSstorage) CreateNFSVolume(ctx context.Context, req *csi.CreateVolumeRequest) (csiResp *csi.CreateVolumeResponse, err error) {
	err = nfs.CreateFileSystem(ctx, nfs.PVName)
	if err != nil {
		e := fmt.Errorf("from CreateFileSystem error: %s", err.Error())
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	err = nfs.CreateExportPathAndAddMetadata(ctx)
	if err != nil {
		e := fmt.Errorf("from CreateExportPath error: %s", err.Error())
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	return nfs.getNfsCsiResponse(req), nil
}

func (nfs *NFSstorage) CreateExportPathAndAddMetadata(ctx context.Context) (err error) {
	defer func() {
		if err != nil && nfs.FileSystemID != 0 {
			slog.Debug("seems to be some problem reverting filesystem", "pvname", nfs.PVName, "error", err)
			if errDelFS := nfs.CS.IboxAPI.DeleteFileSystem(ctx, nfs.FileSystemID); errDelFS != nil {
				slog.Error("failed to delete file system", "fs id", nfs.FileSystemID, "error", errDelFS)
			}
		}
	}()

	if nfs.StorageClassParameters[common.StorageClassNFSExportPermissions] == "" {
		slog.Debug("nfs_export_permissions parameter is not set in the StorageClass, will use default export")
	} else {
		err = nfs.createExportPath(ctx)
		if err != nil {
			e := fmt.Errorf("from createExportPath error: %s", err.Error())
			slog.Error(e.Error())
			return e
		}
		slog.Debug("export path created", "pvname", nfs.PVName)
	}

	defer func() {
		if err != nil && nfs.ExportID != 0 {
			slog.Debug("seems to be some problem reverting created", "export id", nfs.ExportID)
			if _, errDelExport := nfs.CS.IboxAPI.DeleteExport(ctx, nfs.ExportID); errDelExport != nil {
				slog.Error("failed to delete export path for file system", "fs id", nfs.FileSystemID, "error", errDelExport)
			}
		}
	}()

	metadata := map[string]interface{}{
		"host.k8s.pvname": nfs.PVName,
		"host.created_by": nfs.CS.GetCreatedBy(),
	}

	_, err = nfs.CS.IboxAPI.PutMetadata(ctx, nfs.FileSystemID, metadata)
	if err != nil {
		e := fmt.Errorf("from PutMetadata failed to attach metadata for filesystem: %s, error: %s", nfs.PVName, err.Error())
		slog.Error(e.Error())
		return e
	}
	slog.Debug("metadata attached successfully for file system", "pvname", nfs.PVName)
	return nil
}

func (nfs *NFSstorage) CreateFileSystem(ctx context.Context, fileSystemName string) (err error) {
	poolName := nfs.StorageClassParameters[common.StorageClassPoolName]
	pool, err := nfs.CS.IboxAPI.GetPoolByName(ctx, poolName)
	if err != nil {
		e := fmt.Errorf("from GetPoolByName pool_name: %s error: %s", poolName, err.Error())
		slog.Error(e.Error())
		return e
	}
	provtype := strings.ToUpper(nfs.StorageClassParameters[common.StorageClassProvisionType])
	switch provtype {
	case "":
		provtype = common.StorageClassThinProvision
	case common.StorageClassThinProvision, common.StorageClassThickProvision:
	default:
		e := fmt.Errorf("%s valid values are THICK or THIN, THIN is the default when not specified, entered value was [%s]", common.StorageClassProvisionType, provtype)
		slog.Error(e.Error())
		return e
	}

	if provtype == "" {
		provtype = common.StorageClassThinProvision
	}
	fsRequest := iboxapi.CreateFileSystemRequest{
		PoolID:   pool.ID,
		Name:     fileSystemName,
		Size:     nfs.Capacity,
		Provtype: provtype,
	}

	fsRequest.SsdEnabled, err = storagecommon.DetermineSSDValue(ctx, nfs.StorageClassParameters[common.StorageClassSSDEnabled], poolName, nfs.CS.IboxAPI)
	if err != nil {
		e := status.Errorf(codes.Internal, "error when creating filesystem %s storagepool %s, err: %s", fileSystemName, poolName, err.Error())
		slog.Error(e.Error())
		return e
	}

	fileSystem, err := nfs.CS.IboxAPI.CreateFileSystem(ctx, fsRequest)
	if err != nil {
		e := fmt.Errorf("from CreateFileSystem fs: %s error: %s", fileSystemName, err.Error())
		slog.Error(e.Error())
		return e
	}
	nfs.FileSystemID = fileSystem.ID
	slog.Debug("filesystem Created", "fsname", fileSystemName)
	return nil
}

func (nfs *NFSstorage) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	volproto := nfs.CS.VolProto

	nfs.UniqueID = volproto.VolumeID
	nfsDeleteErr := nfs.DeleteNFSVolume(ctx)
	if nfsDeleteErr != nil {
		slog.Error(nfsDeleteErr.Error())
		if strings.Contains(nfsDeleteErr.Error(), "FILESYSTEM_NOT_FOUND") {
			slog.Error("file system already delete from infinibox")
			return &csi.DeleteVolumeResponse{}, nil
		}
		slog.Error("failed to delete NFS Volume", "vol id", req.GetVolumeId(), "error", nfsDeleteErr)
		return nil, nfsDeleteErr
	}
	slog.Debug("volume successfully deleted", "volume id", req.GetVolumeId())
	return &csi.DeleteVolumeResponse{}, nil
}

// DeleteNFSVolume delete volume method
func (nfs *NFSstorage) DeleteNFSVolume(ctx context.Context) (err error) {
	fileSystem, fileSystemErr := nfs.CS.IboxAPI.GetFileSystemByID(ctx, nfs.UniqueID)
	if fileSystemErr != nil {
		slog.Error("failed to get file system by ID", "nfs unique id", nfs.UniqueID, "error", fileSystemErr)
		err = fileSystemErr
		return
	}

	if fileSystem.LockState == common.LockedState {
		return status.Errorf(codes.Aborted, "snapshot %d is locked and can't be deleted till it expires at %s", nfs.UniqueID, time.UnixMilli(fileSystem.LockExpiresAt))
	}

	fileSystems, err := nfs.CS.IboxAPI.GetFileSystemsByParentID(ctx, nfs.UniqueID)
	if err != nil {
		e := fmt.Errorf("from GetFileSystemByParentID parentID: %d error: %s", nfs.UniqueID, err.Error())
		slog.Error(e.Error())
		return e
	}
	if len(fileSystems) > 0 {
		metadata := map[string]interface{}{
			storagecommon.ToBeDeleted: true,
		}
		_, err = nfs.CS.IboxAPI.PutMetadata(ctx, nfs.UniqueID, metadata)
		if err != nil {
			e := fmt.Errorf("from PutMetadata failed to update host.k8s.to_be_deleted filesystem: %s error: %s", nfs.PVName, err.Error())
			slog.Error(e.Error())
			return e
		}
		return nil
	}

	err = nfs.CS.API.DeleteFileSystemComplete(ctx, nfs.UniqueID)
	if err != nil {
		e := fmt.Errorf("from DeleteFileSystemComplete failed to delete filesystem: %s error: %s id: %d parentID: %d", nfs.PVName, err.Error(), nfs.UniqueID, fileSystem.ParentID)
		slog.Error(e.Error())
		return e
	}

	if fileSystem.ParentID != 0 {
		err = nfs.CS.API.DeleteParentFileSystem(ctx, fileSystem.ParentID)
		if err != nil {
			e := fmt.Errorf("from DeleteParentFileSystem failed to delete filesystem's %s parent filesystems error: %s", nfs.PVName, err.Error())
			slog.Error(e.Error())
			return e
		}
	}
	return nil
}

func (nfs *NFSstorage) ControllerModifyVolume(ctx context.Context, req *csi.ControllerModifyVolumeRequest) (resp *csi.ControllerModifyVolumeResponse, err error) {
	return resp, nil
}

func (nfs *NFSstorage) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	var err error
	volumeID := req.GetVolumeId()
	exportID := req.GetVolumeContext()["exportID"]

	slog.Debug("start", "node id", req.GetNodeId(), "vol id", volumeID, "export id", exportID, "nfs export perms", req.GetVolumeContext()[common.StorageClassNFSExportPermissions],
		"iboxInfo", storagecommon.GetHostInfo(ctx, req.GetSecrets(), nfs.CS.IboxAPI))

	kubeNodeID := req.GetNodeId()
	if kubeNodeID == "" {
		e := fmt.Errorf("node ID is required")
		slog.Error(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	_, err = nfs.CS.AccessModesHelper.IsValidAccessModeNfs(req)
	if err != nil {
		e := fmt.Errorf("from IsValidAccessModeNfs - error: %s", err.Error())
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	if req.GetVolumeContext()[common.StorageClassNFSExportPermissions] == "" {
		slog.Debug("nfs_export_permissions parameter not set", "volume ID", volumeID, "export ID", exportID)
		return &csi.ControllerPublishVolumeResponse{}, nil
	}

	// proceed to create a default export rule using the Node ip address

	exportPermissionMapArray, err := getPermissionMaps(req.GetVolumeContext()[common.StorageClassNFSExportPermissions])
	if err != nil {
		e := fmt.Errorf("from getPermissionsMaps - error: %s", err.Error())
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	slog.Debug("nfs export permissions", "volume id", volumeID, "export id", exportID, "export perms", exportPermissionMapArray)

	var access string
	if len(exportPermissionMapArray) > 0 {
		access = exportPermissionMapArray[0][NFSExportPermAccess].(string)
	}

	noRootSquash := true // default value
	nodeNameIP := strings.Split(req.GetNodeId(), "$$")
	if len(nodeNameIP) != 2 {
		e := fmt.Errorf("node ID not found %v", nodeNameIP)
		slog.Error(e.Error())
		return nil, e
	}
	nodeIP := nodeNameIP[1]
	exportid, _ := strconv.Atoi(exportID)
	_, err = nfs.CS.API.AddNodeInExport(ctx, exportid, access, noRootSquash, nodeIP)
	if err != nil {
		e := fmt.Errorf("from AddNodeInExport error: %s", err.Error())
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	return &csi.ControllerPublishVolumeResponse{}, nil
}

func (nfs *NFSstorage) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	slog.Debug("start", "volproto", nfs.CS.VolProto)

	err := nfs.CS.API.DeleteExportRule(ctx, nfs.CS.VolProto.VolumeID, nfs.CS.VolProto.NodeID)
	if err != nil {
		e := fmt.Errorf("from DeleteExportRule - fileystemID: %d error: %s", nfs.CS.VolProto.VolumeID, err.Error())
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

func (nfs *NFSstorage) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (resp *csi.ValidateVolumeCapabilitiesResponse, err error) {
	slog.Error("should not be called, implemented in controller.go")
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
	slog.Debug("start", "parameters", req.Parameters, "name", req.GetName(), "source vol id", req.GetSourceVolumeId())
	var snapshotID string
	snapshotName := req.GetName()
	sourceVolumeID := req.GetSourceVolumeId()

	sourceFilesystemID := nfs.CS.VolProto.VolumeID
	snap, err := nfs.CS.IboxAPI.GetFileSystemByName(ctx, snapshotName)
	if err != nil {
		re, ok := err.(*iboxapi.APIError)
		if ok && re.Code == iboxapi.RESOURCE_NOT_FOUND {
		} else {
			e := fmt.Errorf("from GetSnapshotByName volumeID: %d error: %s", nfs.CS.VolProto.VolumeID, err.Error())
			slog.Error(e.Error())
			return nil, e
		}
	}

	if snap != nil {
		if snap.ParentID == sourceFilesystemID {
			snapshotID = strconv.Itoa(snap.ID) + "$$" + nfs.CS.VolProto.StorageType
			slog.Debug("info", "snapshot", snapshotName, "snap parent id", snap.ParentID, "snap id", snap.ID)
			return &csi.CreateSnapshotResponse{
				Snapshot: &csi.Snapshot{
					SizeBytes:      snap.Size,
					SnapshotId:     snapshotID,
					SourceVolumeId: sourceVolumeID,
					CreationTime:   timestamppb.Now(),
					ReadyToUse:     true,
				},
			}, nil
		}
		return nil, status.Error(codes.AlreadyExists, "CreateSnapshot (nfs) - snapshot with already existing name and different source volume ID")
	}

	parentFilesystem, err := nfs.CS.IboxAPI.GetFileSystemByID(ctx, sourceFilesystemID)
	if err != nil {
		e := fmt.Errorf("from GetFileSystemByID - error getting parent volume for snapshot - volumeID: %d error: %s", sourceFilesystemID, err)
		slog.Error(e.Error())
		return nil, e
	}

	fileSystemSnapshot := iboxapi.FileSystemSnapshot{
		ParentID:       sourceFilesystemID,
		SnapshotName:   snapshotName,
		WriteProtected: true,
		SSDEnabled:     parentFilesystem.SSDEnabled,
	}

	var lockExpiresAt int64
	lockExpiresAtParameter := req.Parameters[common.LockExpiresAtParameter]
	if lockExpiresAtParameter != "" {
		ntpStatus, err := nfs.CS.IboxAPI.GetNtpStatus(ctx)
		if err != nil {
			e := fmt.Errorf("from GetNtpStatus error: %s", err.Error())
			slog.Error(e.Error())
			return nil, e
		}
		lockExpiresAt, err = storagecommon.ValidateSnapshotLockingParameter(ntpStatus[0].LastProbeTimestamp, lockExpiresAtParameter)
		if err != nil {
			e := fmt.Errorf("from ValidateSnapshotLockingParameter - failed to create snapshot: %s error: %s, invalid lock_expires_at parameter ", snapshotName, err.Error())
			slog.Error(e.Error())
			return nil, e
		}
		slog.Debug("snapshot param has a lock_expires_at of", "value", lockExpiresAtParameter)
		fileSystemSnapshot.LockExpiresAt = lockExpiresAt
	}
	resp, err := nfs.CS.IboxAPI.CreateFileSystemSnapshot(ctx, fileSystemSnapshot)
	if err != nil {
		e := fmt.Errorf("from CreateFileSystemSnapshot snapshot: %s error: %s", snapshotName, err.Error())
		slog.Error(e.Error())
		return nil, e
	}

	snapshotID = strconv.Itoa(resp.SnapshotID) + "$$" + nfs.CS.VolProto.StorageType
	snapshot := &csi.Snapshot{
		SnapshotId:     snapshotID,
		SourceVolumeId: sourceVolumeID,
		ReadyToUse:     true,
		CreationTime:   timestamppb.Now(),
		SizeBytes:      resp.Size,
	}
	slog.Debug("completes", "response", snapshot)
	snapshotResp := &csi.CreateSnapshotResponse{Snapshot: snapshot}
	return snapshotResp, nil
}

func (nfs *NFSstorage) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (deleteSnapshot *csi.DeleteSnapshotResponse, err error) {
	slog.Debug("start", "snapshotID", req.GetSnapshotId())

	snapshotID, err := strconv.Atoi(req.GetSnapshotId())
	if err != nil {
		e := fmt.Errorf("failed to parse snapshotID, value: %s, error: %s", req.GetSnapshotId(), err.Error())
		slog.Error(e.Error())
		return nil, status.Error(codes.Aborted, e.Error())
	}
	nfs.UniqueID = snapshotID

	err = nfs.DeleteNFSVolume(ctx)
	if err != nil {
		slog.Error(err.Error())
		if strings.Contains(err.Error(), "FILESYSTEM_NOT_FOUND") {
			slog.Error("snapshot already delete from infinibox")
			deleteSnapshot = &csi.DeleteSnapshotResponse{}
			return deleteSnapshot, nil
		}
		slog.Error("DeleteNFSVolume - error", "error", err)
		return
	}
	deleteSnapshot = &csi.DeleteSnapshotResponse{}
	return
}

func (nfs *NFSstorage) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (expandVolume *csi.ControllerExpandVolumeResponse, err error) {
	volumeID := nfs.CS.VolProto.VolumeID
	slog.Debug("start", "fs ID", volumeID)

	capacity := nfs.Capacity

	// Expand file system size
	var fileSys iboxapi.FileSystem
	fileSys.Size = capacity
	_, err = nfs.CS.IboxAPI.UpdateFileSystem(ctx, volumeID, fileSys)
	if err != nil {
		e := fmt.Errorf("from UpdateFileSystem - error: %s", err.Error())
		slog.Error(e.Error())
		return nil, e
	}
	return &csi.ControllerExpandVolumeResponse{
		CapacityBytes:         capacity,
		NodeExpansionRequired: false,
	}, nil
}

func (nfs *NFSstorage) ControllerGetVolume(_ context.Context, _ *csi.ControllerGetVolumeRequest) (*csi.ControllerGetVolumeResponse, error) {
	// Infinidat does not support ControllerGetVolume
	return nil, status.Error(codes.Unimplemented, "")
}

func (nfs *NFSstorage) getNfsCsiResponse(req *csi.CreateVolumeRequest) *csi.CreateVolumeResponse {
	infinidatVol := &infinidatVolume{
		VolID:     strconv.Itoa(nfs.FileSystemID),
		VolPath:   nfs.ExportPath,
		IPAddress: nfs.IPAddress,
		ExportID:  nfs.ExportID,
	}
	nfs.StorageClassParameters["ipAddress"] = infinidatVol.IPAddress
	nfs.StorageClassParameters["exportID"] = strconv.Itoa(infinidatVol.ExportID)
	nfs.StorageClassParameters["volPathd"] = infinidatVol.VolPath

	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      infinidatVol.VolID,
			CapacityBytes: nfs.Capacity,
			VolumeContext: nfs.StorageClassParameters,
			ContentSource: req.GetVolumeContentSource(),
		},
	}
}
func (nfs *NFSstorage) createExportPath(ctx context.Context) (err error) {
	permissionsMapArray, err := getPermissionMaps(nfs.StorageClassParameters[common.StorageClassNFSExportPermissions])
	if err != nil {
		e := fmt.Errorf("from getPermissionMaps parameter: %s error: %s", nfs.StorageClassParameters[common.StorageClassNFSExportPermissions], err.Error())
		slog.Error(e.Error())
		return e
	}

	exportFileSystem := iboxapi.CreateExportRequest{
		FilesystemID:       nfs.FileSystemID,
		TransportProtocols: "TCP",
		PrivilegedPort:     nfs.UsePrivilegedPorts,
		SnapdirVisible:     nfs.SnapdirVisible,
		ExportPath:         nfs.ExportPath,
	}
	exportFileSystem.Permissionsput = append(exportFileSystem.Permissionsput, permissionsMapArray...)
	var exportResp *iboxapi.Export
	exportResp, err = nfs.CS.IboxAPI.CreateExport(ctx, exportFileSystem)
	if err != nil {
		e := fmt.Errorf("from CreateExport filesystem: %s error: %s", nfs.PVName, err.Error())
		slog.Error(e.Error())
		return e
	}
	nfs.ExportID = exportResp.ID
	nfs.ExportBlock = exportResp.ExportPath
	slog.Debug("created nfs export", "for PV", nfs.PVName, "snapdirvisible", nfs.SnapdirVisible)
	return nil
}

func (nfs *NFSstorage) createVolumeFromPVCSource(ctx context.Context, req *csi.CreateVolumeRequest, size int64, storagePool string, srcVolumeID string) (csiResp *csi.CreateVolumeResponse, err error) {
	slog.Debug("start")

	volproto, err := storagecommon.ValidateVolumeID(srcVolumeID)
	if err != nil {
		e := fmt.Errorf("from ValidateVolumeID volumeID: %s error: %s", srcVolumeID, err.Error())
		slog.Error(e.Error())
		return nil, status.Error(codes.NotFound, e.Error())
	}
	sourceVolumeID := volproto.VolumeID

	// Look up the source volume
	srcfsys, err := nfs.CS.IboxAPI.GetFileSystemByID(ctx, sourceVolumeID)
	if err != nil {
		e := fmt.Errorf("from GetFileSystemByID volumeID: %d error: %s", sourceVolumeID, err.Error())
		slog.Error(e.Error())
		return nil, status.Error(codes.NotFound, e.Error())
	}

	// Check that the requested volume size matches the size of source volume
	if srcfsys.Size != size {
		e := fmt.Errorf("error volumeID: %d, invalid size: %d, requested size: %d ", sourceVolumeID, srcfsys.Size, size)
		slog.Error(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	// Check that the requested storagePool matches the source
	pool, err := nfs.CS.IboxAPI.GetPoolByName(ctx, storagePool)
	if err != nil {
		e := fmt.Errorf("from GetPoolByName: %s error: %s", storagePool, err.Error())
		slog.Error(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}
	if pool.ID != srcfsys.PoolID {
		e := fmt.Errorf("error source storagepool id differs from requested: %s", storagePool)
		slog.Error(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	newSnapshotName := req.GetName() // create snapshot using the original CreateVolumeRequest
	newSnapshotParams := iboxapi.FileSystemSnapshot{ParentID: sourceVolumeID, SnapshotName: newSnapshotName, WriteProtected: false}
	slog.Debug("info", "params", newSnapshotParams)
	// Create snapshot
	newSnapshot, err := nfs.CS.IboxAPI.CreateFileSystemSnapshot(ctx, newSnapshotParams)
	if err != nil {
		e := fmt.Errorf("from CreateFileSystemSnapshot snapshot: %s error: %s", newSnapshotParams.SnapshotName, err.Error())
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	slog.Debug("successfully created volume from clone", "name", newSnapshotName)
	nfs.FileSystemID = newSnapshot.SnapshotID

	err = nfs.CreateExportPathAndAddMetadata(ctx)
	if err != nil {
		e := fmt.Errorf("from CreateExportPath error: %s", err.Error())
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	return nfs.getNfsCsiResponse(req), nil
}
