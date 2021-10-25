/*Copyright 2020 Infinidat
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
	"strings"

	log "infinibox-csi-driver/helper/logger"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (treeq *treeqstorage) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	log.Debug("treeq NodePublishVolume")
	targetPath := req.GetTargetPath()
	notMnt, err := treeq.mounter.IsLikelyNotMountPoint(targetPath)
	if err != nil {
		if treeq.osHelper.IsNotExist(err) {
			if err := treeq.osHelper.MkdirAll(targetPath, 0o750); err != nil {
				log.Errorf("Error while mkdir %v", err)
				return nil, err
			}
			notMnt = true
		} else {
			log.Errorf("IsLikelyNotMountPoint method error  %v", err)
			return nil, err
		}
	}
	if !notMnt {
		return &csi.NodePublishVolumeResponse{}, nil
	}
	mountOptions := []string{}
	configMountOptions := req.GetVolumeContext()["nfs_mount_options"]
	if configMountOptions == "" {
		configMountOptions = MountOptions
	}
	for _, option := range strings.Split(configMountOptions, ",") {
		if option != "" {
			mountOptions = append(mountOptions, option)
		}
	}
	if req.GetReadonly() {
		mountOptions = append(mountOptions, "ro")
	}
	sourceIP := req.GetVolumeContext()["ipAddress"]
	ep := req.GetVolumeContext()["volumePath"]
	source := fmt.Sprintf("%s:%s", sourceIP, ep)
	log.Debugf("Mount sourcePath %v, tagetPath %v", source, targetPath)
	err = treeq.mounter.Mount(source, targetPath, "nfs", mountOptions)
	if err != nil {
		log.Errorf("failed to mount source path '%s' : %s", source, err)
		return nil, status.Errorf(codes.Internal, "Failed to mount target path '%s': %s", targetPath, err)
	}
	log.Debugf("pod successfully mounted to volumeID %s", req.GetVolumeId())
	return &csi.NodePublishVolumeResponse{}, nil
}

func (treeq *treeqstorage) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	log.Debug("treeq NodeUnpublishVolume")
	targetPath := req.GetTargetPath()
	notMnt, err := treeq.mounter.IsLikelyNotMountPoint(targetPath)
	if err != nil {
		if treeq.osHelper.IsNotExist(err) {
			log.Warnf("mount point '%s' already doesn't exist: '%s', return OK", targetPath, err)
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
	log.Debugf("pod successfully unmounted from volumeID %s", req.GetVolumeId())
	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (treeq *treeqstorage) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	return &csi.NodeStageVolumeResponse{}, nil
}

func (treeq *treeqstorage) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	return &csi.NodeUnstageVolumeResponse{}, nil
}
