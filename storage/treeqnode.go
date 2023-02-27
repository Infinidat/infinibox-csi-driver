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

	"k8s.io/klog/v2"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const DEFAULT_HOST_MOUNT_POINT = "/host/"

func (treeq *treeqstorage) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	klog.V(2).Info("treeq NodePublishVolume")

	targetPath := req.GetTargetPath() // this is the path on the host node
	containerHostMountPoint := req.PublishContext["csiContainerHostMountPoint"]
	if containerHostMountPoint == "" {
		containerHostMountPoint = DEFAULT_HOST_MOUNT_POINT
	}
	hostTargetPath := containerHostMountPoint + targetPath // this is the path inside the csi container

	klog.V(4).Infof("NodePublishVolume with targetPath %s volumeId %s\n", hostTargetPath, req.GetVolumeId())

	fileSystemId, treeqId, err := getVolumeIDs(req.GetVolumeId())
	if err != nil {
		klog.V(4).Infof("error parsing fileSystemId %s from %s", err.Error(), req.GetVolumeId())
		return nil, err
	}
	klog.V(4).Infof("fileSystemId %d treeqId %d\n", fileSystemId, treeqId)
	klog.V(4).Infof("volumeContext=%+v", req.GetVolumeContext())
	klog.V(4).Infof("treeq.nfsstorage.configmap=%+v", treeq.nfsstorage.storageClassParameters)

	if req.GetVolumeContext()[common.SC_NFS_EXPORT_PERMISSIONS] == "" {
		treeq.nfsstorage.snapdirVisible = false
		treeq.nfsstorage.usePrivilegedPorts = false

		snapDirVisible := req.GetVolumeContext()[common.SC_SNAPDIR_VISIBLE]
		if snapDirVisible != "" {
			treeq.nfsstorage.snapdirVisible, err = strconv.ParseBool(snapDirVisible)
			if err != nil {
				klog.Errorf("error %s", err.Error())
				return nil, err
			}
		}
		privPorts := req.GetVolumeContext()[common.SC_PRIV_PORTS]
		if privPorts != "" {
			treeq.nfsstorage.usePrivilegedPorts, err = strconv.ParseBool(privPorts)
			if err != nil {
				klog.Errorf("error %s", err.Error())
				return nil, err
			}
		}
		err = treeq.nfsstorage.updateExport(fileSystemId, req.GetVolumeContext()["nodeID"])
		if err != nil {
			return nil, err
		}
	} else {
		klog.V(4).Infof("%s was specified %s, will not create default export rule", common.SC_NFS_EXPORT_PERMISSIONS, req.GetVolumeContext()[common.SC_NFS_EXPORT_PERMISSIONS])
	}

	_, err = os.Stat(hostTargetPath)
	if os.IsNotExist(err) {
		klog.V(4).Infof("targetPath %s does not exist, will create", targetPath)
		if err := os.MkdirAll(hostTargetPath, 0750); err != nil {
			klog.Errorf("error in MkdirAll %s", err.Error())
			return nil, err
		}
	} else {
		klog.V(4).Infof("targetPath %s already exists, will not do anything", targetPath)
		// TODO do I need or care about checking for existing Mount Refs?  k8s.io/utils/GetMountRefs
		// don't return, this may be a second call after a mount timeout
	}

	mountOptions, err := treeq.nfsstorage.storageHelper.GetNFSMountOptions(req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to get mount options for targetPath '%s': %s", hostTargetPath, err.Error())
	}

	sourceIP := req.GetVolumeContext()["ipAddress"]
	ep := req.GetVolumeContext()["volumePath"]
	source := fmt.Sprintf("%s:%s", sourceIP, ep)
	klog.V(4).Infof("Mount sourcePath %v, targetPath %v", source, targetPath)
	err = treeq.nfsstorage.mounter.Mount(source, targetPath, "nfs", mountOptions)
	if err != nil {
		klog.Errorf("failed to mount source path '%s' : %s", source, err)
		return nil, status.Errorf(codes.Internal, "Failed to mount target path '%s': %s", targetPath, err)
	}
	klog.V(2).Infof("mounted treeq volume: '%s' volumeID: %s to mount point: '%s' with options %s", source, req.GetVolumeId(), targetPath, mountOptions)

	if req.GetReadonly() {
		klog.V(2).Info("this is a readonly volume, skipping setting volume permissions")
		return &csi.NodePublishVolumeResponse{}, nil
	}

	err = treeq.nfsstorage.storageHelper.SetVolumePermissions(req)
	if err != nil {
		msg := fmt.Sprintf("Failed to set volume permissions '%s'", err.Error())
		klog.Errorf(msg)
		return nil, status.Errorf(codes.Internal, msg)
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func (treeq *treeqstorage) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	klog.V(2).Info("treeq NodeUnpublishVolume")
	targetPath := req.GetTargetPath()
	notMnt, err := treeq.nfsstorage.mounter.IsLikelyNotMountPoint(targetPath)
	if err != nil {
		if treeq.nfsstorage.osHelper.IsNotExist(err) {
			klog.V(2).Infof("mount point '%s' already doesn't exist: '%s', return OK", targetPath, err)
			return &csi.NodeUnpublishVolumeResponse{}, nil
		}
		return nil, err
	}
	if notMnt {
		if err := treeq.nfsstorage.mounter.Unmount(targetPath); err != nil {
			return nil, status.Errorf(codes.Internal, "Failed to unmount target path '%s': %s", targetPath, err)
		}
	}
	if err := treeq.nfsstorage.osHelper.Remove(targetPath); err != nil && !treeq.nfsstorage.osHelper.IsNotExist(err) {
		return nil, status.Errorf(codes.Internal, "Cannot remove unmounted target path '%s': %s", targetPath, err)
	}
	klog.V(2).Infof("pod successfully unmounted from volumeID %s", req.GetVolumeId())
	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (treeq *treeqstorage) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	return &csi.NodeStageVolumeResponse{}, nil
}

func (treeq *treeqstorage) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	return &csi.NodeUnstageVolumeResponse{}, nil
}
