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
	"os"
	"strconv"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const DEFAULT_HOST_MOUNT_POINT = "/host/"

func (treeq *treeqstorage) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	zlog.Debug().Msg("NodePublishVolume")

	targetPath := req.GetTargetPath() // this is the path on the host node
	containerHostMountPoint := req.PublishContext["csiContainerHostMountPoint"]
	if containerHostMountPoint == "" {
		containerHostMountPoint = DEFAULT_HOST_MOUNT_POINT
	}
	hostTargetPath := containerHostMountPoint + targetPath // this is the path inside the csi container

	zlog.Debug().Msgf("NodePublishVolume with targetPath %s volumeId %s\n", hostTargetPath, req.GetVolumeId())

	fileSystemId, treeqId, err := getVolumeIDs(req.GetVolumeId())
	if err != nil {
		e := fmt.Errorf("error parsing fileSystemId %v from %s", err, req.GetVolumeId())
		zlog.Err(e)
		return nil, e
	}
	zlog.Debug().Msgf("fileSystemId %d treeqId %d\n", fileSystemId, treeqId)
	zlog.Debug().Msgf("volumeContext=%+v", req.GetVolumeContext())
	zlog.Debug().Msgf("treeq.nfsstorage.configmap=%+v", treeq.nfsstorage.storageClassParameters)

	treeq.nfsstorage.snapdirVisible = false
	treeq.nfsstorage.usePrivilegedPorts = false

	snapDirVisible := req.GetVolumeContext()[common.SC_SNAPDIR_VISIBLE]
	if snapDirVisible != "" {
		treeq.nfsstorage.snapdirVisible, err = strconv.ParseBool(snapDirVisible)
		if err != nil {
			zlog.Err(err)
			return nil, err
		}
	}
	privPorts := req.GetVolumeContext()[common.SC_PRIV_PORTS]
	if privPorts != "" {
		treeq.nfsstorage.usePrivilegedPorts, err = strconv.ParseBool(privPorts)
		if err != nil {
			zlog.Err(err)
			return nil, err
		}
	}

	// only update the export if this is the only treeq since treeq's share a single export
	exports, err := treeq.nfsstorage.cs.Api.GetExportByFileSystem(fileSystemId)
	if err != nil {
		zlog.Err(err)
		return nil, err
	}
	zlog.Debug().Msgf("treeq exports count %d on filesystemId %d", len(*exports), fileSystemId)

	treeqCount, err := treeq.nfsstorage.cs.Api.GetFilesystemTreeqCount(fileSystemId)
	if err != nil {
		zlog.Err(err)
		return nil, err
	}
	zlog.Debug().Msgf("treeq count %d on filesystemId %d", treeqCount, fileSystemId)
	if len(*exports) == 0 {
		exportAccess := "RW"
		if req.GetReadonly() || req.VolumeCapability.GetAccessMode().GetMode() == csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY {
			zlog.Debug().Msgf("NodePublishVolume detected read-only, setting export to RO")
			exportAccess = "RO"
		}
		exportPerms := fmt.Sprintf("[{'access':'%s','client':'"+req.GetVolumeContext()["nodeID"]+"','no_root_squash':true}]", exportAccess)
		//exportPerms := "[{'access':'RW','client':'" + req.GetVolumeContext()["nodeID"] + "','no_root_squash':true}]"
		if req.GetVolumeContext()[common.SC_NFS_EXPORT_PERMISSIONS] != "" {
			exportPerms = req.GetVolumeContext()[common.SC_NFS_EXPORT_PERMISSIONS]
			zlog.Debug().Msgf("%s was specified %s, will not create default export rule, will create this rule instead", common.SC_NFS_EXPORT_PERMISSIONS, exportPerms)
		}
		err = treeq.nfsstorage.updateExport(fileSystemId, exportPerms)
		if err != nil {
			zlog.Err(err)
			return nil, err
		}
	} else {
		zlog.Debug().Msg("skipping updateExport because other exports exist")
	}

	_, err = os.Stat(hostTargetPath)
	if os.IsNotExist(err) {
		zlog.Debug().Msgf("targetPath %s does not exist, will create", targetPath)
		if err := os.MkdirAll(hostTargetPath, 0750); err != nil {
			zlog.Error().Msgf("error in MkdirAll %s", err.Error())
			return nil, err
		}
	} else {
		if err != nil {
			zlog.Err(err)
		}
		zlog.Debug().Msgf("targetPath %s already exists, will not do anything", targetPath)
		// TODO do I need or care about checking for existing Mount Refs?  k8s.io/utils/GetMountRefs
		// don't return, this may be a second call after a mount timeout
	}

	mountOptions, err := treeq.nfsstorage.storageHelper.GetNFSMountOptions(req)
	if err != nil {
		zlog.Err(err)
		return nil, status.Errorf(codes.Internal, "Failed to get mount options for targetPath '%s': %s", hostTargetPath, err.Error())
	}

	sourceIP := req.GetVolumeContext()["ipAddress"]
	dnsName := req.GetVolumeContext()["dnsname"]
	if dnsName != "" {
		sourceIP = dnsName
		zlog.Debug().Msgf("storageclass has dnsname specified, using it for mount instead of ipAddress %s", dnsName)
	}
	ep := req.GetVolumeContext()["volumePath"]
	source := fmt.Sprintf("%s:%s", sourceIP, ep)

	err = treeq.nfsstorage.storageHelper.ValidateNFSPortalIPAddress(sourceIP)
	if err != nil {
		zlog.Err(err)
		return nil, status.Errorf(codes.Internal, err.Error())
	}

	zlog.Debug().Msgf("mount sourcePath %v, targetPath %v", source, targetPath)
	err = treeq.nfsstorage.mounter.Mount(source, targetPath, "nfs", mountOptions)
	if err != nil {
		e := fmt.Errorf("failed to mount targetPath %s sourcePath '%s' : %v", targetPath, source, err)
		zlog.Err(e)
		return nil, status.Errorf(codes.Internal, e.Error())
	}
	zlog.Debug().Msgf("mounted treeq volume: '%s' volumeID: %s to mount point: '%s' with options %s", source, req.GetVolumeId(), targetPath, mountOptions)

	if req.GetReadonly() || req.VolumeCapability.GetAccessMode().GetMode() == csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY {
		zlog.Debug().Msg("this is a readonly volume, skipping setting volume permissions")
		return &csi.NodePublishVolumeResponse{}, nil
	}

	err = treeq.nfsstorage.storageHelper.SetVolumePermissions(req, treeq.nfsstorage.snapdirVisible)
	if err != nil {
		e := fmt.Errorf("failed to set volume permissions '%v'", err)
		zlog.Err(e)
		return nil, status.Errorf(codes.Internal, e.Error())
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func (treeq *treeqstorage) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	zlog.Debug().Msg("NodeUnpublishVolume")
	targetPath := req.GetTargetPath()
	err := unmountAndCleanUp(targetPath)
	if err != nil {
		zlog.Err(err)
		return nil, err
	}
	/**
	notMnt, err := treeq.nfsstorage.mounter.IsLikelyNotMountPoint(targetPath)
	if err != nil {
		if treeq.nfsstorage.osHelper.IsNotExist(err) {
			zlog.Debug().Msgf("mount point '%s' already doesn't exist: '%s', return OK", targetPath, err)
			return &csi.NodeUnpublishVolumeResponse{}, nil
		}
		if err != nil {
			zlog.Err(err)
		}
		return nil, err
	}
	if notMnt {
		if err := treeq.nfsstorage.mounter.Unmount(targetPath); err != nil {
			zlog.Err(err)
			return nil, status.Errorf(codes.Internal, "failed to unmount target path '%s': %v", targetPath, err)
		}
	}
	if err := treeq.nfsstorage.osHelper.Remove(targetPath); err != nil && !treeq.nfsstorage.osHelper.IsNotExist(err) {
		zlog.Err(err)
		return nil, status.Errorf(codes.Internal, "cannot remove unmounted target path '%s': %v", targetPath, err)
	}
	zlog.Debug().Msgf("pod successfully unmounted from volumeID %s", req.GetVolumeId())
	*/
	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (treeq *treeqstorage) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	return &csi.NodeStageVolumeResponse{}, nil
}

func (treeq *treeqstorage) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	return &csi.NodeUnstageVolumeResponse{}, nil
}
