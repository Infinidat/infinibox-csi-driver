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
	"os"
	"strconv"

	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/infinidat/infinibox-csi-driver/iboxapi"
	storagecommon "github.com/infinidat/infinibox-csi-driver/storage/common"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const DefaultHostMountPoint = "/host/"

func (nfs *NFSstorage) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	return &csi.NodeStageVolumeResponse{}, nil
}

func (nfs *NFSstorage) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (nfs *NFSstorage) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	targetPath := req.GetTargetPath() // this is the path on the host node
	// instead of hard-coding, we get he '/host' mount prefix via configuration, this lets us unit test with '/tmp' easier
	containerHostMountPoint := req.PublishContext["csiContainerHostMountPoint"]
	if containerHostMountPoint == "" {
		containerHostMountPoint = DefaultHostMountPoint
	}
	hostTargetPath := containerHostMountPoint + targetPath // this is the path inside the csi container

	slog.Debug("(nfs)", "volume id", nfs.CS.VolProto.VolumeID, "host target path", hostTargetPath, "iboxInfo", storagecommon.GetHostInfo(ctx, req.GetSecrets(), nfs.CS.IboxAPI))
	fileSystemID := nfs.CS.VolProto.VolumeID

	nfs.SnapdirVisible = false
	nfs.UsePrivilegedPorts = false
	var err error
	// see if user is setting snapDirVisible in the StorageClass
	snapDir := req.GetVolumeContext()[common.StorageClassSnapDirVisible]
	if snapDir != "" {
		nfs.SnapdirVisible, err = strconv.ParseBool(snapDir)
		if err != nil {
			e := fmt.Errorf("(nfs) - snapsdir visible format error - error: %s", err.Error())
			slog.Error(e.Error())
			return nil, e
		}
	}
	privPorts := req.GetVolumeContext()[common.StorageClassPrivPorts]
	if privPorts != "" {
		nfs.UsePrivilegedPorts, err = strconv.ParseBool(privPorts)
		if err != nil {
			e := fmt.Errorf("(nfs) - priv ports format error - error: %s", err.Error())
			slog.Error(e.Error())
			return nil, e
		}
	}

	if req.GetVolumeContext()[common.StorageClassNFSExportPermissions] == "" {
		exportAccess := "RW"
		if req.GetReadonly() || req.VolumeCapability.GetAccessMode().GetMode() == csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY {
			slog.Debug("(nfs) - detected read-only, setting export to RO")
			exportAccess = "RO"
		}
		exportPerms := fmt.Sprintf("[{'access':'%s','client':'"+req.GetVolumeContext()["nodeID"]+"','no_root_squash':true}]", exportAccess)
		err = nfs.UpdateExport(ctx, fileSystemID, exportPerms)
		if err != nil {
			e := fmt.Errorf("(nfs) - updateExport - error: %s", err.Error())
			slog.Error(e.Error())
			return nil, e
		}
	} else {
		slog.Log(ctx, common.LevelTrace, "(nfs) - nfs_export_permissions was specified, will not create default export rule", "perms", req.GetVolumeContext()[common.StorageClassNFSExportPermissions])
	}

	_, err = os.Stat(hostTargetPath)
	if os.IsNotExist(err) {
		slog.Debug("(nfs) - targetPath does not exist, will create", "targetPath", targetPath)
		if err := os.MkdirAll(hostTargetPath, 0750); err != nil {
			e := fmt.Errorf("(nfs) - MkdirAll - error: %s", err.Error())
			slog.Error(e.Error())
			return nil, e
		}
	} else {
		slog.Debug("(nfs) - targetPath already exists, will not do anything", "targetPath", targetPath)
		// TODO do I need or care about checking for existing Mount Refs?  k8s.io/utils/GetMountRefs
		// dont' return, this may be a second call after a mount timeout
	}

	mountOptions, err := nfs.StorageHelper.GetNFSMountOptions(req)
	if err != nil {
		e := fmt.Errorf("(nfs) - GetNFSMountOptions - targetPath: %s error: %s", hostTargetPath, err.Error())
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	nfsVersion, nfsPort := GetNFSVersionPort(mountOptions)
	slog.Debug("(nfs)", "mountoptions", mountOptions, "nfs version", nfsVersion, "nfs port", nfsPort)

	sourceIP := req.GetVolumeContext()["ipAddress"]
	dnsName := req.GetVolumeContext()["dnsname"]
	if dnsName != "" {
		sourceIP = dnsName
		slog.Debug("(nfs) - storageclass has dnsname specified, using it for mount instead of ipAddress", "dnsName", dnsName)
	}

	port, err := strconv.Atoi(nfsPort)
	if err != nil {
		e := fmt.Errorf("(nfs) - ValidateIPAddress - port parsing error: %s", err.Error())
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	err = nfs.StorageHelper.ValidateIPAddress(sourceIP, port)
	if err != nil {
		e := fmt.Errorf("(nfs) - ValidateIPAddress - error: %s", err.Error())
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	ep := req.GetVolumeContext()["volPathd"]
	source := fmt.Sprintf("%s:%s", sourceIP, ep)
	slog.Debug("(nfs) - Mount", "source", source, "targetPath", targetPath)
	err = nfs.Mounter.Mount(source, targetPath, "nfs", mountOptions)
	if err != nil {
		e := fmt.Errorf("(nfs) - Mount - failed to mount source '%s ' target %s: %v", source, targetPath, err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	slog.Debug("(nfs) - successfully mounted nfs volume to mount point with options", "source", source, "target", targetPath, "mountoptions", mountOptions)

	if req.GetReadonly() || req.VolumeCapability.GetAccessMode().GetMode() == csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY {
		slog.Debug("(nfs) - this is a readonly volume, skipping setting volume permissions")
		return &csi.NodePublishVolumeResponse{}, nil
	}

	err = nfs.StorageHelper.SetVolumePermissions(req)
	if err != nil {
		e := fmt.Errorf("(nfs) - SetVolumePermissions - error: %s", err.Error())
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func (nfs *NFSstorage) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	targetPath := req.GetTargetPath()
	slog.Debug("NodeUnpublishVolume (nfs)", "target", targetPath, "vol id", req.GetVolumeId())
	err := storagecommon.UnmountAndCleanUp(targetPath)
	if err != nil {
		e := fmt.Errorf("NodeUnpublishVolume (nfs) - unmountAndCleanup - error: %s", err.Error())
		slog.Error(e.Error())
		return nil, e
	}

	if isCleanupNFSPermsSet() {
		cleanupNFSPerms(ctx, nfs.CS.VolProto.VolumeID)
	}
	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (nfs *NFSstorage) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	return nil, status.Error(codes.Unimplemented, "NodeGetCapabilities (nfs) - should never be called, called in node.go instead")
}

func (nfs *NFSstorage) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	return nil, status.Error(codes.Unimplemented, "NodeGetInfo (nfs) not implemented")
}

func (nfs *NFSstorage) NodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "NodeGetVolumeStats (nfs) not implemented")
}

func (nfs *NFSstorage) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	slog.Info("NodeExpandVolume (nfs) - called req volume path", "vol path", req.GetVolumePath())
	// there is no implementation here for NFS expand since nfs handles resizing automatically
	// this function does get called only because of the CSI driver design
	response := csi.NodeExpandVolumeResponse{}
	return &response, nil
}

func (nfs *NFSstorage) UpdateExport(ctx context.Context, fileSystemID int, exportPerms string) (err error) {
	// lookup file system information
	// TODO pass in context
	fileSystem, err := nfs.CS.IboxAPI.GetFileSystemByID(ctx, fileSystemID)
	if err != nil {
		e := fmt.Errorf("(nfs) - failed to get filesystem by id %d %v", fileSystemID, err)
		slog.Error(e.Error())
		return status.Error(codes.Internal, e.Error())
	}

	// use the volumeID to get the filesystem information,
	// example export {'access':'RW','client':'192.168.0.110', 'no_root_squash':true}
	exportFileSystem := iboxapi.CreateExportRequest{
		FilesystemID:       fileSystemID,
		TransportProtocols: "TCP",
		PrivilegedPort:     nfs.UsePrivilegedPorts,
		SnapdirVisible:     nfs.SnapdirVisible,
		ExportPath:         "/" + fileSystem.Name, // convention is /csi-xxxxxxxx  where xxxx is the filesystem name/pvname
	}

	permissionsMapArray, err := getPermissionMaps(exportPerms)
	if err != nil {
		e := fmt.Errorf("(nfs) - failed to parse permission map string %s %v", exportPerms, err)
		slog.Error(e.Error())
		return e
	}
	updatePerms := convertToExportRulePermissions(permissionsMapArray)
	slog.Debug("(nfs) updatePermissions", "len", len(updatePerms), "perms", updatePerms)

	existingExports, err := nfs.CS.IboxAPI.GetExportsByFileSystemID(ctx, fileSystemID)
	if err != nil {
		e := fmt.Errorf("(nfs) - error from GetExportByFileSystem fileSystemID %d %v", fileSystemID, err)
		slog.Error(e.Error())
		return e
	}
	slog.Debug("(nfs) - GetExportByFileSystem", "existingExports", existingExports)
	for _, existingExport := range existingExports {
		if existingExport.ExportPath == exportFileSystem.ExportPath {
			slog.Debug("(nfs) - export path was found to already exist with snapDirVisible", "export path", existingExport.ExportPath, "snapdir visible", existingExport.SnapdirVisible)

			// look at all existing permissions, see if the client IP already is used, do nothing if that is the case
			for _, p := range existingExport.Permissions {
				for _, newP := range updatePerms {
					if newP.Client == p.Client {
						slog.Debug("(nfs) - client IP was found to already exist, skipping adding it or updating existing perms", "client", newP.Client)
						return nil
					}
				}
			}

			// update the existing filesystem export with the new permissions
			slog.Debug("(nfs) - updating  export ID old perms plus new perms", "export id", existingExport.ID, "old perms", existingExport.Permissions, "updated perms", updatePerms)
			exportPathRef := iboxapi.ExportPathRef{
				Permissions: append(existingExport.Permissions, updatePerms...),
			}
			_, err = nfs.CS.IboxAPI.UpdateExportPermissions(ctx, existingExport, exportPathRef)
			if err != nil {
				e := fmt.Errorf("(nfs) - error from UpdateExport ID %d filesystemID %d %v", existingExport.ID, fileSystemID, err)
				slog.Error(e.Error())
				return e
			}
			nfs.ExportID = existingExport.ID
			nfs.ExportBlock = existingExport.ExportPath
			nfs.SnapdirVisible = existingExport.SnapdirVisible
			return nil
		}
	}

	// create the export rule if it didn't already exist
	exportFileSystem.Permissionsput = append(exportFileSystem.Permissionsput, permissionsMapArray...)
	slog.Debug("(nfs)", "exportFileSystem", exportFileSystem)
	exportResp, err := nfs.CS.IboxAPI.CreateExport(ctx, exportFileSystem)
	if err != nil {
		e := fmt.Errorf("(nfs) - failed to create export path of filesystem %s %v", fileSystem.Name, err)
		slog.Error(e.Error())
		return e
	}
	nfs.ExportID = exportResp.ID
	nfs.ExportBlock = exportResp.ExportPath
	nfs.SnapdirVisible = exportFileSystem.SnapdirVisible
	slog.Debug("(nfs) - created nfs export for PV", "fs name", fileSystem.Name, "snapdirvis", exportFileSystem.SnapdirVisible)

	return nil
}
