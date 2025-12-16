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

	slog.Debug("start", "volume id", nfs.CS.VolProto.VolumeID, "host target path", hostTargetPath, "iboxInfo", storagecommon.GetHostInfo(ctx, req.GetSecrets(), nfs.CS.IboxAPI))
	fileSystemID := nfs.CS.VolProto.VolumeID

	nfs.SnapdirVisible = false
	nfs.UsePrivilegedPorts = false
	var err error
	// see if user is setting snapDirVisible in the StorageClass
	snapDir := req.GetVolumeContext()[common.StorageClassSnapDirVisible]
	if snapDir != "" {
		nfs.SnapdirVisible, err = strconv.ParseBool(snapDir)
		if err != nil {
			return nil, common.Errorf("error parsing snapsdir visible error: %w", err)
		}
	}
	privPorts := req.GetVolumeContext()[common.StorageClassPrivPorts]
	if privPorts != "" {
		nfs.UsePrivilegedPorts, err = strconv.ParseBool(privPorts)
		if err != nil {
			return nil, common.Errorf("error parsing priv ports error: %w", err)
		}
	}

	if req.GetVolumeContext()[common.StorageClassNFSExportPermissions] == "" {
		exportAccess := "RW"
		if req.GetReadonly() || req.VolumeCapability.GetAccessMode().GetMode() == csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY {
			slog.Debug("detected read-only, setting export to RO")
			exportAccess = "RO"
		}
		exportPerms := fmt.Sprintf("[{'access':'%s','client':'"+req.GetVolumeContext()["nodeID"]+"','no_root_squash':true}]", exportAccess)
		err = nfs.UpdateExport(ctx, fileSystemID, exportPerms)
		if err != nil {
			return nil, common.Errorf("from updateExport - error: %w", err)
		}
	} else {
		slog.Log(ctx, common.LevelTrace, "nfs_export_permissions was specified, will not create default export rule", "perms", req.GetVolumeContext()[common.StorageClassNFSExportPermissions])
	}

	_, err = os.Stat(hostTargetPath)
	if os.IsNotExist(err) {
		slog.Debug("targetPath does not exist, will create", "targetPath", targetPath)
		if err := os.MkdirAll(hostTargetPath, 0750); err != nil {
			return nil, common.Errorf("from MkdirAll - error: %w", err)
		}
	} else {
		slog.Debug("targetPath already exists, will not do anything", "targetPath", targetPath)
	}

	mountOptions, err := nfs.StorageHelper.GetNFSMountOptions(req)
	if err != nil {
		return nil, status.Error(codes.Internal, common.Errorf("from GetNFSMountOptions - targetPath: %s error: %w", hostTargetPath, err).Error())
	}

	nfsVersion, nfsPort := GetNFSVersionPort(mountOptions)
	slog.Debug("info", "mountoptions", mountOptions, "nfs version", nfsVersion, "nfs port", nfsPort)

	sourceIP := req.GetVolumeContext()["ipAddress"]
	dnsName := req.GetVolumeContext()["dnsname"]
	if dnsName != "" {
		sourceIP = dnsName
		slog.Debug("storageclass has dnsname specified, using it for mount instead of ipAddress", "dnsName", dnsName)
	}

	port, err := strconv.Atoi(nfsPort)
	if err != nil {
		return nil, status.Error(codes.Internal, common.Errorf("parsing error nfs port error: %w", err).Error())
	}

	err = nfs.StorageHelper.ValidateIPAddress(sourceIP, port)
	if err != nil {
		return nil, status.Error(codes.Internal, common.Errorf("from ValidateIPAddress - error: %w", err).Error())
	}

	ep := req.GetVolumeContext()["volPathd"]
	source := fmt.Sprintf("%s:%s", sourceIP, ep)
	slog.Debug("mounting", "sourceIP", source, "targetPath", targetPath)
	err = nfs.Mounter.Mount(source, targetPath, "nfs", mountOptions)
	if err != nil {
		return nil, status.Error(codes.Internal, common.Errorf("from Mount - failed to mount source: %s targetPath: %s error: %w", source, targetPath, err).Error())
	}
	slog.Debug("successfully mounted nfs volume to mount point with options", "sourceIP", source, "target", targetPath, "mountoptions", mountOptions)

	if req.GetReadonly() || req.VolumeCapability.GetAccessMode().GetMode() == csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY {
		slog.Debug("this is a readonly volume, skipping setting volume permissions")
		return &csi.NodePublishVolumeResponse{}, nil
	}

	err = nfs.StorageHelper.SetVolumePermissions(req)
	if err != nil {
		return nil, status.Error(codes.Internal, common.Errorf("from SetVolumePermissions - error: %w", err).Error())
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func (nfs *NFSstorage) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	targetPath := req.GetTargetPath()
	slog.Debug("start", "target", targetPath, "vol id", req.GetVolumeId())
	err := storagecommon.UnmountAndCleanUp(targetPath)
	if err != nil {
		return nil, common.Errorf("from UnmountAndCleanup - error: %w", err)
	}

	if isCleanupNFSPermsSet() {
		cleanupNFSPerms(ctx, nfs.CS.VolProto.VolumeID)
	}
	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (nfs *NFSstorage) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	return nil, status.Error(codes.Unimplemented, "NodeGetCapabilities should never be called, called in node.go instead")
}

func (nfs *NFSstorage) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	return nil, status.Error(codes.Unimplemented, "NodeGetInfo (nfs) not implemented")
}

func (nfs *NFSstorage) NodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "NodeGetVolumeStats (nfs) not implemented")
}

func (nfs *NFSstorage) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	slog.Info("start", "volumePath", req.GetVolumePath())
	// there is no implementation here for NFS expand since nfs handles resizing automatically
	// this function does get called only because of the CSI driver design
	response := csi.NodeExpandVolumeResponse{}
	return &response, nil
}

func (nfs *NFSstorage) UpdateExport(ctx context.Context, fileSystemID int, exportPerms string) (err error) {
	fileSystem, err := nfs.CS.IboxAPI.GetFileSystemByID(ctx, fileSystemID)
	if err != nil {
		return status.Error(codes.Internal, common.Errorf("from GetFileSystemByID filesystemID: %d error: %w", fileSystemID, err).Error())
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
		return common.Errorf("from getPermissionMaps exportPerms: %s error: %w", exportPerms, err)
	}
	updatePerms := convertToExportRulePermissions(permissionsMapArray)
	slog.Debug("updatePermissions", "len", len(updatePerms), "perms", updatePerms)

	existingExports, err := nfs.CS.IboxAPI.GetExportsByFileSystemID(ctx, fileSystemID)
	if err != nil {
		return common.Errorf("error from GetExportByFileSystem fileSystemID: %d error: %w", fileSystemID, err)
	}
	slog.Debug("from GetExportByFileSystem", "existingExports", existingExports)
	for _, existingExport := range existingExports {
		if existingExport.ExportPath == exportFileSystem.ExportPath {
			slog.Debug("export path was found to already exist with snapDirVisible", "export path", existingExport.ExportPath, "snapdir visible", existingExport.SnapdirVisible)

			// look at all existing permissions, see if the client IP already is used, do nothing if that is the case
			for _, p := range existingExport.Permissions {
				for _, newP := range updatePerms {
					if newP.Client == p.Client {
						slog.Debug("client IP was found to already exist, skipping adding it or updating existing perms", "client", newP.Client)
						return nil
					}
				}
			}

			// update the existing filesystem export with the new permissions
			slog.Debug("updating  export ID old perms plus new perms", "export id", existingExport.ID, "old perms", existingExport.Permissions, "updated perms", updatePerms)
			exportPathRef := iboxapi.ExportPathRef{
				Permissions: append(existingExport.Permissions, updatePerms...),
			}
			_, err = nfs.CS.IboxAPI.UpdateExportPermissions(ctx, existingExport, exportPathRef)
			if err != nil {
				return common.Errorf("error from UpdateExportPermissions exportID: %d filesystemID: %d error: %w", existingExport.ID, fileSystemID, err)
			}
			nfs.ExportID = existingExport.ID
			nfs.ExportBlock = existingExport.ExportPath
			nfs.SnapdirVisible = existingExport.SnapdirVisible
			return nil
		}
	}

	// create the export rule if it didn't already exist
	exportFileSystem.Permissionsput = append(exportFileSystem.Permissionsput, permissionsMapArray...)
	slog.Debug("info", "exportFileSystem", exportFileSystem)
	exportResp, err := nfs.CS.IboxAPI.CreateExport(ctx, exportFileSystem)
	if err != nil {
		return common.Errorf("from CreateExport filesystem: %s error: %w", fileSystem.Name, err)
	}
	nfs.ExportID = exportResp.ID
	nfs.ExportBlock = exportResp.ExportPath
	nfs.SnapdirVisible = exportFileSystem.SnapdirVisible
	slog.Debug("created nfs export for PV", "fs name", fileSystem.Name, "snapdirvis", exportFileSystem.SnapdirVisible)

	return nil
}
