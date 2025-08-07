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
	"github.com/infinidat/infinibox-csi-driver/common"
	"os"
	"strconv"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const DEFAULT_HOST_MOUNT_POINT = "/host/"

func (treeq *treeqstorage) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	zlog.Debug().Msg("NodePublishVolume (treeq) - started")

	targetPath := req.GetTargetPath() // this is the path on the host node
	containerHostMountPoint := req.PublishContext["csiContainerHostMountPoint"]
	if containerHostMountPoint == "" {
		containerHostMountPoint = DEFAULT_HOST_MOUNT_POINT
	}
	hostTargetPath := containerHostMountPoint + targetPath // this is the path inside the csi container

	zlog.Debug().Msgf("NodePublishVolume (treeq) - with targetPath %s volumeId %s\n", hostTargetPath, req.GetVolumeId())

	fileSystemId, treeqId, err := getVolumeIDs(req.GetVolumeId())
	if err != nil {
		e := fmt.Errorf("NodePublishVolume (treeq) - getVolumeIDs - error parsing fileSystemId %v from %s", err, req.GetVolumeId())
		zlog.Err(e)
		return nil, e
	}
	zlog.Debug().Msgf("NodePublishVolume (treeq) - fileSystemId %d treeqId %d", fileSystemId, treeqId)
	zlog.Debug().Msgf("NodePublishVolume (treeq) - volumeContext=%+v", req.GetVolumeContext())
	zlog.Debug().Msgf("NodePublishVolume (treeq) - treeq.nfsstorage.configmap=%+v", treeq.nfsstorage.storageClassParameters)

	treeq.nfsstorage.snapdirVisible = false
	treeq.nfsstorage.usePrivilegedPorts = false

	snapDirVisible := req.GetVolumeContext()[common.SC_SNAPDIR_VISIBLE]
	if snapDirVisible != "" {
		treeq.nfsstorage.snapdirVisible, err = strconv.ParseBool(snapDirVisible)
		if err != nil {
			e := fmt.Errorf("NodePublishVolume (treeq) - parse snapdir visible - error: %s", err.Error())
			zlog.Error().Msg(e.Error())
			return nil, e
		}
	}
	privPorts := req.GetVolumeContext()[common.SC_PRIV_PORTS]
	if privPorts != "" {
		treeq.nfsstorage.usePrivilegedPorts, err = strconv.ParseBool(privPorts)
		if err != nil {
			e := fmt.Errorf("NodePublishVolume (treeq) - parse priv ports - error: %s", err.Error())
			zlog.Error().Msg(e.Error())
			return nil, e
		}
	}

	// only update the export if this is the only treeq since treeq's share a single export
	exports, err := treeq.nfsstorage.cs.IboxApi.GetExportsByFileSystemID(fileSystemId)
	if err != nil {
		e := fmt.Errorf("NodePublishVolume (treeq) - GetExportByFileSystem - error: %s", err.Error())
		zlog.Error().Msg(e.Error())
		return nil, e
	}
	zlog.Debug().Msgf("NodePublishVolume (treeq) - exports count %d on filesystemId %d", len(exports), fileSystemId)

	if len(exports) == 0 {
		exportAccess := "RW"
		if req.GetReadonly() || req.VolumeCapability.GetAccessMode().GetMode() == csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY {
			zlog.Debug().Msgf("NodePublishVolume (treeq) - detected read-only, setting export to RO")
			exportAccess = "RO"
		}
		exportPerms := fmt.Sprintf("[{'access':'%s','client':'"+req.GetVolumeContext()["nodeID"]+"','no_root_squash':true}]", exportAccess)
		if req.GetVolumeContext()[common.SC_NFS_EXPORT_PERMISSIONS] != "" {
			exportPerms = req.GetVolumeContext()[common.SC_NFS_EXPORT_PERMISSIONS]
			zlog.Debug().Msgf("NodePublishVolume (treeq) - %s was specified %s, will not create default export rule, will create this rule instead", common.SC_NFS_EXPORT_PERMISSIONS, exportPerms)
		}
		err = treeq.nfsstorage.updateExport(fileSystemId, exportPerms)
		if err != nil {
			e := fmt.Errorf("NodePublishVolume (treeq) - updateExport - error: %s", err.Error())
			zlog.Error().Msg(e.Error())
			return nil, e
		}
	} else {
		zlog.Debug().Msg("NodePublishVolume (treeq) - skipping updateExport because other exports exist")
	}

	_, err = os.Stat(hostTargetPath)
	if os.IsNotExist(err) {
		zlog.Debug().Msgf("NodePublishVolume (treeq) - targetPath %s does not exist, will create", targetPath)
		if err := os.MkdirAll(hostTargetPath, 0750); err != nil {
			e := fmt.Errorf("NodePublishVolume (treeq) - mkdirAll - error: %s", err.Error())
			zlog.Error().Msg(e.Error())
			return nil, e
		}
	} else {
		if err != nil {
			zlog.Error().Msgf("NodePublishVolume (treeq) - host target path exists - error: %s", err.Error())
		}
		zlog.Debug().Msgf("NodePublishVolume (treeq) - targetPath %s already exists, will not do anything", targetPath)
		// TODO do I need or care about checking for existing Mount Refs?  k8s.io/utils/GetMountRefs
		// don't return, this may be a second call after a mount timeout
	}

	mountOptions, err := treeq.nfsstorage.storageHelper.GetNFSMountOptions(req)
	if err != nil {
		e := fmt.Errorf("NodePublishVolume (treeq) - GetNFSMountOptions - targetPath: %s error: %s", hostTargetPath, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	sourceIP := req.GetVolumeContext()["ipAddress"]
	dnsName := req.GetVolumeContext()["dnsname"]
	if dnsName != "" {
		sourceIP = dnsName
		zlog.Debug().Msgf("NodePublishVolume (treeq) - storageclass has dnsname specified, using it for mount instead of ipAddress %s", dnsName)
	}
	ep := req.GetVolumeContext()["volumePath"]
	source := fmt.Sprintf("%s:%s", sourceIP, ep)

	nfsVersion, nfsPort := GetNFSVersionPort(mountOptions)
	zlog.Debug().Msgf("NodePublishVolume (treeq) - GetNFSVersionPort - vers %s port %s", nfsVersion, nfsPort)

	port, err := strconv.Atoi(nfsPort)
	if err != nil {
		e := fmt.Errorf("NodePublishVolume (treeq) - ValidateNFSPortalIPAddress - port parsing error: %s", err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	err = treeq.nfsstorage.storageHelper.ValidateIPAddress(sourceIP, port)
	if err != nil {
		e := fmt.Errorf("NodePublishVolume (treeq) - ValidateIPAddress - error: %s", err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	zlog.Debug().Msgf("NodePUblishVolume (treeq) - mount sourcePath %v, targetPath %v", source, targetPath)
	err = treeq.nfsstorage.mounter.Mount(source, targetPath, "nfs", mountOptions)
	if err != nil {
		e := fmt.Errorf("NodePublishVolume (treeq) - Mount - failed to mount targetPath %s sourcePath '%s' : %v", targetPath, source, err)
		zlog.Err(e)
		return nil, status.Error(codes.Internal, e.Error())
	}
	zlog.Debug().Msgf("NodePublishVolume (treeq) - mounted treeq volume: '%s' volumeID: %s to mount point: '%s' with options %s", source, req.GetVolumeId(), targetPath, mountOptions)

	if req.GetReadonly() || req.VolumeCapability.GetAccessMode().GetMode() == csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY {
		zlog.Debug().Msg("NodePublishVolume (treeq) - this is a readonly volume, skipping setting volume permissions")
		return &csi.NodePublishVolumeResponse{}, nil
	}

	err = treeq.nfsstorage.storageHelper.SetVolumePermissions(req)
	if err != nil {
		e := fmt.Errorf("NodePublishVolume (treeq) - SetVolumePermissions - failed to set volume permissions '%v'", err)
		zlog.Err(e)
		return nil, status.Error(codes.Internal, e.Error())
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func (treeq *treeqstorage) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	zlog.Debug().Msg("NodeUnpublishVolume (treeq) starts")
	targetPath := req.GetTargetPath()
	err := unmountAndCleanUp(targetPath)
	if err != nil {
		e := fmt.Errorf("NodeUnpublishVolume  (treeq) - unmountAndCleanup - error: %s", err.Error())
		zlog.Error().Msg(e.Error())
		return nil, e
	}
	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (treeq *treeqstorage) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	return &csi.NodeStageVolumeResponse{}, nil
}

func (treeq *treeqstorage) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	return &csi.NodeUnstageVolumeResponse{}, nil
}
