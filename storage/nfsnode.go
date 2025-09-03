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
	"os"
	"strconv"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (nfs *nfsstorage) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	return &csi.NodeStageVolumeResponse{}, nil
}

func (nfs *nfsstorage) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (nfs *nfsstorage) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	const function = "NodePublishVolume"
	targetPath := req.GetTargetPath() // this is the path on the host node
	// instead of hard-coding, we get he '/host' mount prefix via configuration, this lets us unit test with '/tmp' easier
	containerHostMountPoint := req.PublishContext["csiContainerHostMountPoint"]
	if containerHostMountPoint == "" {
		containerHostMountPoint = DEFAULT_HOST_MOUNT_POINT
	}
	hostTargetPath := containerHostMountPoint + targetPath // this is the path inside the csi container

	zlog.Debug().Msgf("%s (nfs) - fs ID: %d targetPath=%s ", function, nfs.cs.VolProto.VolumeID, hostTargetPath)
	fileSystemId := nfs.cs.VolProto.VolumeID

	nfs.snapdirVisible = false
	nfs.usePrivilegedPorts = false
	var err error
	// see if user is setting snapDirVisible in the StorageClass
	snapDir := req.GetVolumeContext()[common.SC_SNAPDIR_VISIBLE]
	if snapDir != "" {
		nfs.snapdirVisible, err = strconv.ParseBool(snapDir)
		if err != nil {
			e := fmt.Errorf("%s (nfs) - snapsdir visible format error - error: %s", function, err.Error())
			zlog.Error().Msg(e.Error())
			return nil, e
		}
	}
	privPorts := req.GetVolumeContext()[common.SC_PRIV_PORTS]
	if privPorts != "" {
		nfs.usePrivilegedPorts, err = strconv.ParseBool(privPorts)
		if err != nil {
			e := fmt.Errorf("%s (nfs) - priv ports format error - error: %s", function, err.Error())
			zlog.Error().Msg(e.Error())
			return nil, e
		}
	}

	if req.GetVolumeContext()[common.SC_NFS_EXPORT_PERMISSIONS] == "" {
		exportAccess := "RW"
		if req.GetReadonly() || req.VolumeCapability.GetAccessMode().GetMode() == csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY {
			zlog.Debug().Msgf("%s (nfs) - detected read-only, setting export to RO", function)
			exportAccess = "RO"
		}
		exportPerms := fmt.Sprintf("[{'access':'%s','client':'"+req.GetVolumeContext()["nodeID"]+"','no_root_squash':true}]", exportAccess)
		err = nfs.updateExport(fileSystemId, exportPerms)
		if err != nil {
			e := fmt.Errorf("%s (nfs) - updateExport - error: %s", function, err.Error())
			zlog.Error().Msg(e.Error())
			return nil, e
		}
	} else {
		zlog.Trace().Msgf("%s (nfs) - nfs_export_permissions was specified %s, will not create default export rule", function, req.GetVolumeContext()[common.SC_NFS_EXPORT_PERMISSIONS])
	}

	_, err = os.Stat(hostTargetPath)
	if os.IsNotExist(err) {
		zlog.Debug().Msgf("%s (nfs) - targetPath %s does not exist, will create", function, targetPath)
		if err := os.MkdirAll(hostTargetPath, 0750); err != nil {
			e := fmt.Errorf("%s (nfs) - MkdirAll - error: %s", function, err.Error())
			zlog.Error().Msg(e.Error())
			return nil, e
		}
	} else {
		zlog.Debug().Msgf("%s (nfs) - targetPath %s already exists, will not do anything", function, targetPath)
		// TODO do I need or care about checking for existing Mount Refs?  k8s.io/utils/GetMountRefs
		// dont' return, this may be a second call after a mount timeout
	}

	mountOptions, err := nfs.storageHelper.GetNFSMountOptions(req)
	if err != nil {
		e := fmt.Errorf("%s (nfs) - GetNFSMountOptions - targetPath: %s error: %s", function, hostTargetPath, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	nfsVersion, nfsPort := GetNFSVersionPort(mountOptions)
	zlog.Debug().Msgf("%s (nfs) -  mount options are [%v], nfs version [%s] port [%s]", function, mountOptions, nfsVersion, nfsPort)

	sourceIP := req.GetVolumeContext()["ipAddress"]
	dnsName := req.GetVolumeContext()["dnsname"]
	if dnsName != "" {
		sourceIP = dnsName
		zlog.Debug().Msgf("%s (nfs) - storageclass has dnsname specified, using it for mount instead of ipAddress %s", function, dnsName)
	}

	port, err := strconv.Atoi(nfsPort)
	if err != nil {
		e := fmt.Errorf("%s (nfs) - ValidateIPAddress - port parsing error: %s", function, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	err = nfs.storageHelper.ValidateIPAddress(sourceIP, port)
	if err != nil {
		e := fmt.Errorf("%s (nfs) - ValidateIPAddress - error: %s", function, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	ep := req.GetVolumeContext()["volPathd"]
	source := fmt.Sprintf("%s:%s", sourceIP, ep)
	zlog.Debug().Msgf("%s (nfs) - Mount sourcePath %v, targetPath %v", function, source, targetPath)
	err = nfs.mounter.Mount(source, targetPath, "nfs", mountOptions)
	if err != nil {
		e := fmt.Errorf("%s (nfs) - Mount - failed to mount source '%s ' target %s: %v", function, source, targetPath, err)
		zlog.Err(e)
		return nil, status.Error(codes.Internal, e.Error())
	}
	zlog.Debug().Msgf("%s (nfs) - successfully mounted nfs volume '%s' to mount point '%s' with options %s", function, source, targetPath, mountOptions)

	if req.GetReadonly() || req.VolumeCapability.GetAccessMode().GetMode() == csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY {
		zlog.Debug().Msgf("%s (nfs) - this is a readonly volume, skipping setting volume permissions", function)
		return &csi.NodePublishVolumeResponse{}, nil
	}

	err = nfs.storageHelper.SetVolumePermissions(req)
	if err != nil {
		e := fmt.Errorf("%s (nfs) - SetVolumePermissions - error: %s", function, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func (nfs *nfsstorage) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	targetPath := req.GetTargetPath()
	zlog.Debug().Msgf("NodeUnpublishVolume (nfs) - targetPath %s volume ID %s", targetPath, req.GetVolumeId())
	err := unmountAndCleanUp(targetPath)
	if err != nil {
		e := fmt.Errorf("NodeUnpublishVolume (nfs) - unmountAndCleanup - error: %s", err.Error())
		zlog.Error().Msg(e.Error())
		return nil, e
	}

	if isCleanupNFSPermsSet() {
		cleanupNFSPerms(nfs.cs.VolProto.VolumeID)
	}
	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (nfs *nfsstorage) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	return nil, status.Error(codes.Unimplemented, "NodeGetCapabilities (nfs) - should never be called, called in node.go instead")
}

func (nfs *nfsstorage) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	return nil, status.Error(codes.Unimplemented, "NodeGetInfo (nfs) not implemented")
}

func (nfs *nfsstorage) NodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "NodeGetVolumeStats (nfs) not implemented")
}

func (nfs *nfsstorage) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	zlog.Info().Msgf("NodeExpandVolume (nfs) - called req volume path %s", req.GetVolumePath())
	// there is no implementation here for NFS expand since nfs handles resizing automatically
	// this function does get called only because of the CSI driver design
	response := csi.NodeExpandVolumeResponse{}
	return &response, nil
}

func (nfs *nfsstorage) updateExport(filesystemId int, exportPerms string) (err error) {
	const function = "updateExport"
	//lookup file system information
	fs, err := nfs.cs.IboxApi.GetFileSystemByID(filesystemId)
	if err != nil {
		e := fmt.Errorf("%s (nfs) - failed to get filesystem by id %d %v", function, filesystemId, err)
		zlog.Err(e)
		return status.Error(codes.Internal, e.Error())
	}

	// use the volumeId to get the filesystem information,
	//example export {'access':'RW','client':'192.168.0.110', 'no_root_squash':true}
	exportFileSystem := iboxapi.CreateExportRequest{
		FilesystemID:        filesystemId,
		Transport_protocols: "TCP",
		Privileged_port:     nfs.usePrivilegedPorts,
		SnapdirVisible:      nfs.snapdirVisible,
		Export_path:         "/" + fs.Name, // convention is /csi-xxxxxxxx  where xxxx is the filesystem name/pvname
	}

	permissionsMapArray, err := getPermissionMaps(exportPerms)
	if err != nil {
		e := fmt.Errorf("%s (nfs) - failed to parse permission map string %s %v", function, exportPerms, err)
		zlog.Error().Msg(e.Error())
		return e
	}
	updatePerms := convertToExportRulePermissions(permissionsMapArray)
	zlog.Debug().Msgf("%s (nfs) updatePermissions len(%d) %+v", function, len(updatePerms), updatePerms)

	existingExports, err := nfs.cs.IboxApi.GetExportsByFileSystemID(filesystemId)
	if err != nil {
		e := fmt.Errorf("%s (nfs) - error from GetExportByFileSystem filesystemId %d %v", function, filesystemId, err)
		zlog.Error().Msg(e.Error())
		return e
	}
	zlog.Debug().Msgf("%s (nfs) - GetExportByFileSystem response =%+v", function, existingExports)
	for _, existingExport := range existingExports {
		if existingExport.ExportPath == exportFileSystem.Export_path {
			zlog.Debug().Msgf("%s (nfs) - export path was found to already exist %s with snapDirVisible %t", function, existingExport.ExportPath, existingExport.SnapdirVisible)

			// look at all existing permissions, see if the client IP already is used, do nothing if that is the case
			for _, p := range existingExport.Permissions {
				for _, newP := range updatePerms {
					if newP.Client == p.Client {
						zlog.Debug().Msgf("%s (nfs) - client IP was found to already exist %s, skipping adding it or updating existing perms", function, newP.Client)
						return nil
					}
				}
			}

			// update the existing filesystem export with the new permissions
			zlog.Debug().Msgf("%s (nfs) - updating  export ID %d old perms %+v plus new perms %+v", function, existingExport.ID, existingExport.Permissions, updatePerms)
			exportPathRef := iboxapi.ExportPathRef{
				Permissions: append(existingExport.Permissions, updatePerms...),
			}
			_, err = nfs.cs.IboxApi.UpdateExportPermissions(existingExport, exportPathRef)
			if err != nil {
				e := fmt.Errorf("%s (nfs) - error from UpdateExport ID %d filesystemId %d %v", function, existingExport.ID, filesystemId, err)
				zlog.Error().Msg(e.Error())
				return e
			}
			nfs.exportID = existingExport.ID
			nfs.exportBlock = existingExport.ExportPath
			nfs.snapdirVisible = existingExport.SnapdirVisible
			return nil
		}
	}

	// create the export rule if it didn't already exist
	exportFileSystem.Permissionsput = append(exportFileSystem.Permissionsput, permissionsMapArray...)
	zlog.Debug().Msgf("%s (nfs) - exportFileSystem =%+v", function, exportFileSystem)
	exportResp, err := nfs.cs.IboxApi.CreateExport(exportFileSystem)
	if err != nil {
		e := fmt.Errorf("%s (nfs) - failed to create export path of filesystem %s %v", function, fs.Name, err)
		zlog.Error().Msg(e.Error())
		return e
	}
	nfs.exportID = exportResp.ID
	nfs.exportBlock = exportResp.ExportPath
	nfs.snapdirVisible = exportFileSystem.SnapdirVisible
	zlog.Debug().Msgf("%s (nfs) - created nfs export for PV '%s', snapdirVisible: %t", function, fs.Name, exportFileSystem.SnapdirVisible)

	return nil
}
