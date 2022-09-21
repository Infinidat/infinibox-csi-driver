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
	"os/exec"
	"strings"

	log "infinibox-csi-driver/helper/logger"

	"k8s.io/klog"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (treeq *treeqstorage) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	log.Debug("treeq NodePublishVolume")
	targetPath := req.GetTargetPath()
	klog.V(4).Infof("NodePublishVolume with targetPath %s\n", targetPath)
	notMnt, err := treeq.mounter.IsLikelyNotMountPoint(targetPath)
	klog.V(4).Infof("after IsLike with notMnt %t", notMnt)
	if err != nil {
		if treeq.osHelper.IsNotExist(err) {
			klog.V(4).Infof("targetPath %s does not exist, will create", targetPath)
			if err := treeq.osHelper.MkdirAll(targetPath, 0o750); err != nil {
				log.Errorf("Error in MkdirAll %s", err.Error())
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

	// get mount options from VolumeCapability - the standard way
	mountOptions := req.GetVolumeCapability().GetMount().GetMountFlags()
	// accommodate legacy nfs_mount_options parameter and legacy default in storageservice.go - remove in future releases, CSIC-346
	mountOptionsString, oldNfsMountOptionsParamProvided := req.GetVolumeContext()["nfs_mount_options"]
	if oldNfsMountOptionsParamProvided {
		klog.Warningf("Deprecated 'nfs_mount_options' parameter %s provided, will NOT be supported in future releases - please move to standard 'mountOptions' parameter", mountOptionsString)
	} else {
		mountOptionsString = StandardMountOptions // defined in nfscontroller.go!
	}
	for _, option := range strings.Split(mountOptionsString, ",") {
		if option != "" {
			mountOptions = append(mountOptions, option)
		}
	}
	if req.GetReadonly() {
		// TODO: ensure ro / rw behavior is correct, CSIC-343. eg what if user specifies "rw" as a mountOption?
		mountOptions = append(mountOptions, "ro")
	}
	// TODO: remove duplicates from this list

	sourceIP := req.GetVolumeContext()["ipAddress"]
	ep := req.GetVolumeContext()["volumePath"]
	source := fmt.Sprintf("%s:%s", sourceIP, ep)
	log.Debugf("Mount sourcePath %v, tagetPath %v", source, targetPath)

	// Do not use os.MkdirAll(). This ignores the mount chroot defined in the Dockerfile.
	// MkdirAll() will cause hard-to-grok mount errors.
	klog.V(4).Infof("Mount point does not exist. Creating mount point.")
	klog.V(4).Infof("Run: mkdir --parents --mode 0750 '%s' ", targetPath)
	cmd := exec.Command("mkdir", "--parents", "--mode", "0750", targetPath)
	err = cmd.Run()
	if err != nil {
		klog.Errorf("failed to mkdir '%s': %s", targetPath, err)
		return nil, err
	}

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
