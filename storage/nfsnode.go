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
	"infinibox-csi-driver/api"
	"infinibox-csi-driver/common"
	"os"
	"strconv"
	"strings"

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
	targetPath := req.GetTargetPath() // this is the path on the host node
	// instead of hard-coding, we get he '/host' mount prefix via configuration, this lets us unit test with '/tmp' easier
	containerHostMountPoint := req.PublishContext["csiContainerHostMountPoint"]
	if containerHostMountPoint == "" {
		containerHostMountPoint = DEFAULT_HOST_MOUNT_POINT
	}
	hostTargetPath := containerHostMountPoint + targetPath // this is the path inside the csi container

	zlog.Debug().Msgf("NodePublishVolume targetPath=%s", hostTargetPath)

	tmp := strings.Split(req.GetVolumeId(), "$$")[0]
	fileSystemId, err := strconv.ParseInt(tmp, 10, 64)
	if err != nil {
		zlog.Err(err)
		return nil, err
	}

	if req.GetVolumeContext()[common.SC_NFS_EXPORT_PERMISSIONS] == "" {
		nfs.snapdirVisible = false
		nfs.usePrivilegedPorts = false
		snapDir := req.GetVolumeContext()[common.SC_SNAPDIR_VISIBLE]
		if snapDir != "" {
			nfs.snapdirVisible, err = strconv.ParseBool(snapDir)
			if err != nil {
				zlog.Err(err)
				return nil, err
			}
		}
		privPorts := req.GetVolumeContext()[common.SC_PRIV_PORTS]
		if privPorts != "" {
			nfs.usePrivilegedPorts, err = strconv.ParseBool(privPorts)
			if err != nil {
				zlog.Err(err)
				return nil, err
			}
		}
		err = nfs.updateExport(fileSystemId, req.GetVolumeContext()["nodeID"])
		if err != nil {
			zlog.Err(err)
			return nil, err
		}
	} else {
		zlog.Trace().Msgf("nfs_export_permissions was specified %s, will not create default export rule", req.GetVolumeContext()[common.SC_NFS_EXPORT_PERMISSIONS])
	}

	_, err = os.Stat(hostTargetPath)
	if os.IsNotExist(err) {
		zlog.Debug().Msgf("targetPath %s does not exist, will create", targetPath)
		if err := os.MkdirAll(hostTargetPath, 0750); err != nil {
			zlog.Err(err)
			return nil, err
		}
	} else {
		zlog.Debug().Msgf("targetPath %s already exists, will not do anything", targetPath)
		// TODO do I need or care about checking for existing Mount Refs?  k8s.io/utils/GetMountRefs
		// dont' return, this may be a second call after a mount timeout
	}

	mountOptions, err := nfs.storageHelper.GetNFSMountOptions(req)
	if err != nil {
		zlog.Err(err)
		return nil, status.Errorf(codes.Internal, "failed to get mount options for targetPath '%s': %s", hostTargetPath, err.Error())
	}

	zlog.Debug().Msgf("nfs mount options are [%v]", mountOptions)

	sourceIP := req.GetVolumeContext()["ipAddress"]
	dnsName := req.GetVolumeContext()["dnsname"]
	if dnsName != "" {
		sourceIP = dnsName
		zlog.Debug().Msgf("storageclass has dnsname specified, using it for mount instead of ipAddress %s", dnsName)
	}
	ep := req.GetVolumeContext()["volPathd"]
	source := fmt.Sprintf("%s:%s", sourceIP, ep)
	zlog.Debug().Msgf("Mount sourcePath %v, targetPath %v", source, targetPath)
	err = nfs.mounter.Mount(source, targetPath, "nfs", mountOptions)
	if err != nil {
		e := fmt.Errorf("failed to mount source '%s ' target %s: %v", source, targetPath, err)
		zlog.Err(e)
		return nil, status.Errorf(codes.Internal, e.Error())
	}
	zlog.Debug().Msgf("successfully mounted nfs volume '%s' to mount point '%s' with options %s", source, targetPath, mountOptions)

	if req.GetReadonly() {
		zlog.Debug().Msg("this is a readonly volume, skipping setting volume permissions")
		return &csi.NodePublishVolumeResponse{}, nil
	}

	err = nfs.storageHelper.SetVolumePermissions(req)
	if err != nil {
		zlog.Err(err)
		return nil, status.Errorf(codes.Internal, err.Error())
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func (nfs *nfsstorage) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	targetPath := req.GetTargetPath()
	zlog.Debug().Msgf("NodeUnpublishVolume targetPath %s", targetPath)
	err := unmountAndCleanUp(targetPath)
	if err != nil {
		zlog.Err(err)
		return nil, err
	}

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (nfs *nfsstorage) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	nodeCaps := []csi.NodeServiceCapability_RPC_Type{}
	var caps []*csi.NodeServiceCapability
	for _, cap := range nodeCaps {
		c := &csi.NodeServiceCapability{
			Type: &csi.NodeServiceCapability_Rpc{
				Rpc: &csi.NodeServiceCapability_RPC{
					Type: cap,
				},
			},
		}
		caps = append(caps, c)
	}
	return &csi.NodeGetCapabilitiesResponse{Capabilities: caps}, nil

}

func (nfs *nfsstorage) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	return nil, status.Error(codes.Unimplemented, "nfs NodeGetInfo not implemented")
}

func (nfs *nfsstorage) NodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "NodeGetVolumeStats not implemented")
}

func (nfs *nfsstorage) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	zlog.Info().Msgf("nfs NodeExpandVolume called req %+v\n", req)
	response := csi.NodeExpandVolumeResponse{}
	return &response, nil
}

func (nfs *nfsstorage) updateExport(filesystemId int64, ipAddress string) (err error) {
	//lookup file system information
	fs, err := nfs.cs.Api.GetFileSystemByID(filesystemId)
	if err != nil {
		e := fmt.Errorf("failed to get filesystem by id %d %v", filesystemId, err)
		zlog.Err(e)
		return status.Error(codes.Internal, e.Error())
	}

	// use the volumeId to get the filesystem information,
	//example export {'access':'RW','client':'192.168.0.110', 'no_root_squash':true}
	exportFileSystem := api.ExportFileSys{
		FilesystemID:        filesystemId,
		Transport_protocols: "TCP",
		Privileged_port:     nfs.usePrivilegedPorts,
		SnapdirVisible:      nfs.snapdirVisible,
		Export_path:         "/" + fs.Name, // convention is /csi-xxxxxxxx  where xxxx is the filesystem name/pvname
	}
	exportPerms := "[{'access':'RW','client':'" + ipAddress + "','no_root_squash':true}]"
	permissionsMapArray, err := getPermissionMaps(exportPerms)
	if err != nil {
		zlog.Error().Msgf("failed to parse permission map string %s %v", exportPerms, err)
		return err
	}

	// remove an existing export if it exists, this occurs when a pod restarts
	resp, err := nfs.cs.Api.GetExportByFileSystem(filesystemId)
	if err != nil {
		zlog.Error().Msgf("error from GetExportByFileSystem filesystemId %d %v", filesystemId, err)
		return err
	}
	zlog.Trace().Msgf("GetExportByFileSystem response =%+v", resp)
	if resp != nil {
		responses := *resp
		for i := 0; i < len(responses); i++ {
			r := responses[i]
			if r.ExportPath == exportFileSystem.Export_path {
				zlog.Debug().Msgf("export path was found to already exist %s", r.ExportPath)
				// here is where we would delete the existing export
				deleteResp, err := nfs.cs.Api.DeleteExportPath(r.ID)
				if err != nil {
					zlog.Error().Msgf("error from DeleteExportPath ID %d filesystemId %d %v", r.ID, filesystemId, err)
					return err
				}
				zlog.Trace().Msgf("delete export path response %+v\n", deleteResp)
			}
		}
	}

	// create the export rule
	exportFileSystem.Permissionsput = append(exportFileSystem.Permissionsput, permissionsMapArray...)
	zlog.Debug().Msgf("exportFileSystem =%+v", exportFileSystem)
	exportResp, err := nfs.cs.Api.ExportFileSystem(exportFileSystem)
	if err != nil {
		zlog.Error().Msgf("failed to create export path of filesystem %s %v", fs.Name, err)
		return err
	}
	nfs.exportID = exportResp.ID
	nfs.exportBlock = exportResp.ExportPath
	zlog.Debug().Msgf("created nfs export for PV '%s', snapdirVisible: %t", fs.Name, exportFileSystem.SnapdirVisible)

	return nil
}
