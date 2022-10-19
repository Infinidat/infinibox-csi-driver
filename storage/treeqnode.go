/*Copyright 2022 Infinidat
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.*/
package storage

import (
	"context"
	"fmt"
	"os"

	"k8s.io/klog"

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

	klog.V(4).Infof("NodePublishVolume with targetPath %s\n", hostTargetPath)

	_, err := os.Stat(hostTargetPath)
	if os.IsNotExist(err) {
		klog.V(4).Infof("targetPath %s does not exist, will create", targetPath)
		if err := os.MkdirAll(hostTargetPath, 0750); err != nil {
			klog.Errorf("Error in MkdirAll %s", err.Error())
			return nil, err
		}
	} else {
		klog.V(4).Infof("targetPath %s already exists, will not do anything", targetPath)
		// TODO do I need or care about checking for existing Mount Refs?  k8s.io/utils/GetMountRefs
		return &csi.NodePublishVolumeResponse{}, nil
	}

	mountOptions, err := treeq.storageHelper.GetNFSMountOptions(req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to get mount options for targetPath '%s': %s", hostTargetPath, err.Error())
	}

	sourceIP := req.GetVolumeContext()["ipAddress"]
	ep := req.GetVolumeContext()["volumePath"]
	source := fmt.Sprintf("%s:%s", sourceIP, ep)
	klog.V(4).Infof("Mount sourcePath %v, targetPath %v", source, targetPath)
	err = treeq.mounter.Mount(source, targetPath, "nfs", mountOptions)
	if err != nil {
		klog.Errorf("failed to mount source path '%s' : %s", source, err)
		return nil, status.Errorf(codes.Internal, "Failed to mount target path '%s': %s", targetPath, err)
	}
	klog.V(2).Infof("Successfully mounted treeq nfs volume '%s' to mount point '%s' with options %s", source, targetPath, mountOptions)

	klog.V(2).Infof("pod successfully mounted to volumeID %s", req.GetVolumeId())

	svc := Service{}
	err = svc.SetVolumePermissions(req)
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
	notMnt, err := treeq.mounter.IsLikelyNotMountPoint(targetPath)
	if err != nil {
		if treeq.osHelper.IsNotExist(err) {
			klog.V(2).Infof("mount point '%s' already doesn't exist: '%s', return OK", targetPath, err)
			return &csi.NodeUnpublishVolumeResponse{}, nil
		}
		return nil, err
	}
	if notMnt {
		if err := treeq.mounter.Unmount(targetPath); err != nil {
			return nil, status.Errorf(codes.Internal, "Failed to unmount target path '%s': %s", targetPath, err)
		}
	}
	if err := treeq.osHelper.Remove(targetPath); err != nil && !treeq.osHelper.IsNotExist(err) {
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
