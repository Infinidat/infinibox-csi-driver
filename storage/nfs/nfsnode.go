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
	"os"
	"strconv"

	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/infinidat/infinibox-csi-driver/iboxapi"
	storagecommon "github.com/infinidat/infinibox-csi-driver/storage/common"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const DEFAULT_HOST_MOUNT_POINT = "/host/"

func (nfs *NFSstorage) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	return &csi.NodeStageVolumeResponse{}, nil
}

func (nfs *NFSstorage) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (nfs *NFSstorage) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	const functionName = "NodePublishVolume"
	targetPath := req.GetTargetPath() // this is the path on the host node
	// instead of hard-coding, we get he '/host' mount prefix via configuration, this lets us unit test with '/tmp' easier
	containerHostMountPoint := req.PublishContext["csiContainerHostMountPoint"]
	if containerHostMountPoint == "" {
		containerHostMountPoint = DEFAULT_HOST_MOUNT_POINT
	}
	hostTargetPath := containerHostMountPoint + targetPath // this is the path inside the csi container

	zlog.Debug().Msgf("%s (nfs) - fs ID: %d targetPath=%s %s", functionName, nfs.CS.VolProto.VolumeID, hostTargetPath, storagecommon.GetHostInfo(req.GetSecrets(), nfs.CS.IboxAPI))
	fileSystemID := nfs.CS.VolProto.VolumeID

	nfs.SnapdirVisible = false
	nfs.UsePrivilegedPorts = false
	var err error
	// see if user is setting snapDirVisible in the StorageClass
	snapDir := req.GetVolumeContext()[common.StorageClassSnapDirVisible]
	if snapDir != "" {
		nfs.SnapdirVisible, err = strconv.ParseBool(snapDir)
		if err != nil {
			e := fmt.Errorf("%s (nfs) - snapsdir visible format error - error: %s", functionName, err.Error())
			zlog.Error().Msg(e.Error())
			return nil, e
		}
	}
	privPorts := req.GetVolumeContext()[common.StorageClassPrivPorts]
	if privPorts != "" {
		nfs.UsePrivilegedPorts, err = strconv.ParseBool(privPorts)
		if err != nil {
			e := fmt.Errorf("%s (nfs) - priv ports format error - error: %s", functionName, err.Error())
			zlog.Error().Msg(e.Error())
			return nil, e
		}
	}

	if req.GetVolumeContext()[common.StorageClassNFSExportPermissions] == "" {
		exportAccess := "RW"
		if req.GetReadonly() || req.VolumeCapability.GetAccessMode().GetMode() == csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY {
			zlog.Debug().Msgf("%s (nfs) - detected read-only, setting export to RO", functionName)
			exportAccess = "RO"
		}
		exportPerms := fmt.Sprintf("[{'access':'%s','client':'"+req.GetVolumeContext()["nodeID"]+"','no_root_squash':true}]", exportAccess)
		err = nfs.UpdateExport(fileSystemID, exportPerms)
		if err != nil {
			e := fmt.Errorf("%s (nfs) - updateExport - error: %s", functionName, err.Error())
			zlog.Error().Msg(e.Error())
			return nil, e
		}
	} else {
		zlog.Trace().Msgf("%s (nfs) - nfs_export_permissions was specified %s, will not create default export rule", functionName, req.GetVolumeContext()[common.StorageClassNFSExportPermissions])
	}

	_, err = os.Stat(hostTargetPath)
	if os.IsNotExist(err) {
		zlog.Debug().Msgf("%s (nfs) - targetPath %s does not exist, will create", functionName, targetPath)
		if err := os.MkdirAll(hostTargetPath, 0750); err != nil {
			e := fmt.Errorf("%s (nfs) - MkdirAll - error: %s", functionName, err.Error())
			zlog.Error().Msg(e.Error())
			return nil, e
		}
	} else {
		zlog.Debug().Msgf("%s (nfs) - targetPath %s already exists, will not do anything", functionName, targetPath)
		// TODO do I need or care about checking for existing Mount Refs?  k8s.io/utils/GetMountRefs
		// dont' return, this may be a second call after a mount timeout
	}

	mountOptions, err := nfs.StorageHelper.GetNFSMountOptions(req)
	if err != nil {
		e := fmt.Errorf("%s (nfs) - GetNFSMountOptions - targetPath: %s error: %s", functionName, hostTargetPath, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	nfsVersion, nfsPort := GetNFSVersionPort(mountOptions)
	zlog.Debug().Msgf("%s (nfs) -  mount options are [%v], nfs version [%s] port [%s]", functionName, mountOptions, nfsVersion, nfsPort)

	sourceIP := req.GetVolumeContext()["ipAddress"]
	dnsName := req.GetVolumeContext()["dnsname"]
	if dnsName != "" {
		sourceIP = dnsName
		zlog.Debug().Msgf("%s (nfs) - storageclass has dnsname specified, using it for mount instead of ipAddress %s", functionName, dnsName)
	}

	port, err := strconv.Atoi(nfsPort)
	if err != nil {
		e := fmt.Errorf("%s (nfs) - ValidateIPAddress - port parsing error: %s", functionName, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	err = nfs.StorageHelper.ValidateIPAddress(sourceIP, port)
	if err != nil {
		e := fmt.Errorf("%s (nfs) - ValidateIPAddress - error: %s", functionName, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	ep := req.GetVolumeContext()["volPathd"]
	source := fmt.Sprintf("%s:%s", sourceIP, ep)
	zlog.Debug().Msgf("%s (nfs) - Mount sourcePath %v, targetPath %v", functionName, source, targetPath)
	err = nfs.Mounter.Mount(source, targetPath, "nfs", mountOptions)
	if err != nil {
		e := fmt.Errorf("%s (nfs) - Mount - failed to mount source '%s ' target %s: %v", functionName, source, targetPath, err)
		zlog.Err(e)
		return nil, status.Error(codes.Internal, e.Error())
	}
	zlog.Debug().Msgf("%s (nfs) - successfully mounted nfs volume '%s' to mount point '%s' with options %s", functionName, source, targetPath, mountOptions)

	if req.GetReadonly() || req.VolumeCapability.GetAccessMode().GetMode() == csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY {
		zlog.Debug().Msgf("%s (nfs) - this is a readonly volume, skipping setting volume permissions", functionName)
		return &csi.NodePublishVolumeResponse{}, nil
	}

	err = nfs.StorageHelper.SetVolumePermissions(req)
	if err != nil {
		e := fmt.Errorf("%s (nfs) - SetVolumePermissions - error: %s", functionName, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func (nfs *NFSstorage) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	targetPath := req.GetTargetPath()
	zlog.Debug().Msgf("NodeUnpublishVolume (nfs) - targetPath %s volume ID %s", targetPath, req.GetVolumeId())
	err := storagecommon.UnmountAndCleanUp(targetPath)
	if err != nil {
		e := fmt.Errorf("NodeUnpublishVolume (nfs) - unmountAndCleanup - error: %s", err.Error())
		zlog.Error().Msg(e.Error())
		return nil, e
	}

	if isCleanupNFSPermsSet() {
		cleanupNFSPerms(nfs.CS.VolProto.VolumeID)
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
	zlog.Info().Msgf("NodeExpandVolume (nfs) - called req volume path %s", req.GetVolumePath())
	// there is no implementation here for NFS expand since nfs handles resizing automatically
	// this function does get called only because of the CSI driver design
	response := csi.NodeExpandVolumeResponse{}
	return &response, nil
}

func (nfs *NFSstorage) UpdateExport(fileSystemID int, exportPerms string) (err error) {
	const functionName = "UpdateExport"
	// lookup file system information
	fileSystem, err := nfs.CS.IboxAPI.GetFileSystemByID(fileSystemID)
	if err != nil {
		e := fmt.Errorf("%s (nfs) - failed to get filesystem by id %d %v", functionName, fileSystemID, err)
		zlog.Err(e)
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
		e := fmt.Errorf("%s (nfs) - failed to parse permission map string %s %v", functionName, exportPerms, err)
		zlog.Error().Msg(e.Error())
		return e
	}
	updatePerms := convertToExportRulePermissions(permissionsMapArray)
	zlog.Debug().Msgf("%s (nfs) updatePermissions len(%d) %+v", functionName, len(updatePerms), updatePerms)

	existingExports, err := nfs.CS.IboxAPI.GetExportsByFileSystemID(fileSystemID)
	if err != nil {
		e := fmt.Errorf("%s (nfs) - error from GetExportByFileSystem fileSystemID %d %v", functionName, fileSystemID, err)
		zlog.Error().Msg(e.Error())
		return e
	}
	zlog.Debug().Msgf("%s (nfs) - GetExportByFileSystem response =%+v", functionName, existingExports)
	for _, existingExport := range existingExports {
		if existingExport.ExportPath == exportFileSystem.ExportPath {
			zlog.Debug().Msgf("%s (nfs) - export path was found to already exist %s with snapDirVisible %t", functionName, existingExport.ExportPath, existingExport.SnapdirVisible)

			// look at all existing permissions, see if the client IP already is used, do nothing if that is the case
			for _, p := range existingExport.Permissions {
				for _, newP := range updatePerms {
					if newP.Client == p.Client {
						zlog.Debug().Msgf("%s (nfs) - client IP was found to already exist %s, skipping adding it or updating existing perms", functionName, newP.Client)
						return nil
					}
				}
			}

			// update the existing filesystem export with the new permissions
			zlog.Debug().Msgf("%s (nfs) - updating  export ID %d old perms %+v plus new perms %+v", functionName, existingExport.ID, existingExport.Permissions, updatePerms)
			exportPathRef := iboxapi.ExportPathRef{
				Permissions: append(existingExport.Permissions, updatePerms...),
			}
			_, err = nfs.CS.IboxAPI.UpdateExportPermissions(existingExport, exportPathRef)
			if err != nil {
				e := fmt.Errorf("%s (nfs) - error from UpdateExport ID %d filesystemID %d %v", functionName, existingExport.ID, fileSystemID, err)
				zlog.Error().Msg(e.Error())
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
	zlog.Debug().Msgf("%s (nfs) - exportFileSystem =%+v", functionName, exportFileSystem)
	exportResp, err := nfs.CS.IboxAPI.CreateExport(exportFileSystem)
	if err != nil {
		e := fmt.Errorf("%s (nfs) - failed to create export path of filesystem %s %v", functionName, fileSystem.Name, err)
		zlog.Error().Msg(e.Error())
		return e
	}
	nfs.ExportID = exportResp.ID
	nfs.ExportBlock = exportResp.ExportPath
	nfs.SnapdirVisible = exportFileSystem.SnapdirVisible
	zlog.Debug().Msgf("%s (nfs) - created nfs export for PV '%s', snapdirVisible: %t", functionName, fileSystem.Name, exportFileSystem.SnapdirVisible)

	return nil
}
