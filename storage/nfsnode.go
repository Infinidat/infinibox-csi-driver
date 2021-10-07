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
	log "infinibox-csi-driver/helper/logger"
	"os/exec"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog"
)

func (nfs *nfsstorage) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	return &csi.NodeStageVolumeResponse{}, nil
}

func (nfs *nfsstorage) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (nfs *nfsstorage) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	klog.V(4).Infof("NodePublishVolume")
	targetPath := req.GetTargetPath()
	notMnt, err := nfs.mounter.IsLikelyNotMountPoint(targetPath)
	if err != nil {
		if nfs.osHelper.IsNotExist(err) {
			if err := nfs.osHelper.MkdirAll(targetPath, 0750); err != nil {
				klog.Errorf("Error while mkdir %v", err)
				return nil, err
			}
			notMnt = true
		} else {
			klog.Errorf("IsLikelyNotMountPoint method error  %v", err)
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
	ep := req.GetVolumeContext()["volPathd"]
	source := fmt.Sprintf("%s:%s", sourceIP, ep)
	klog.V(4).Infof("Mount sourcePath %v, targetPath %v", source, targetPath)

	// Create mount point
	klog.V(4).Infof("Mount point doesn't exist, create: mkdir --parents --mode 0750 '%s'", targetPath)
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
	err = nfs.mounter.Mount(source, targetPath, "nfs", mountOptions)
	if err != nil {
		klog.Errorf("Failed to mount source path '%s' : %s", source, err)
		return nil, status.Errorf(codes.Internal, "Failed to mount target path '%s': %s", targetPath, err)
	}
	log.Infof("Successfully mounted nfs volume '%s' to mount point '%s'", source, targetPath)

	// Chown
	uid := req.GetVolumeContext()["uid"] // Returns an empty string if key not found
	gid := req.GetVolumeContext()["gid"]
	err = nfs.osHelper.ChownVolume(uid, gid, targetPath)
	if err != nil {
		msg := fmt.Sprintf("Failed to chown path '%s': %s", targetPath, err)
		klog.Errorf(msg)
		return nil, status.Errorf(codes.Internal, msg)
	}

	// Chmod
	unixPermissions := req.GetVolumeContext()["unix_permissions"] // Returns an empty string if key not found
	err = nfs.osHelper.ChmodVolume(unixPermissions, targetPath)
	if err != nil {
		msg := fmt.Sprintf("Failed to chmod path '%s': %s", targetPath, err)
		klog.Errorf(msg)
		return nil, status.Errorf(codes.Internal, msg)
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func (nfs *nfsstorage) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	klog.V(4).Infof("NodeUnpublishVolume")
	targetPath := req.GetTargetPath()
	notMnt, err := nfs.mounter.IsLikelyNotMountPoint(targetPath)
	if err != nil {
		if nfs.osHelper.IsNotExist(err) {
			klog.Warningf("target path: %s doesn't exist, return OK", targetPath)
			return &csi.NodeUnpublishVolumeResponse{}, nil
		}
		klog.Errorf("failed to check if target path: %s is a mount point, err: %s", targetPath, err)
		return nil, err
	}
	if !notMnt {
		klog.V(4).Infof("unmount target path: %s", targetPath)
		if err := nfs.mounter.Unmount(targetPath); err != nil {
			return nil, status.Errorf(codes.Internal, "failed to unmount target path: %s, err: %s", targetPath, err)
		}
	} else {
		klog.V(4).Infof("target path: %s is not a mount point", targetPath)
	}

	klog.V(4).Infof("remove target path: %s", targetPath)
	if err := nfs.osHelper.Remove(targetPath); err != nil && !nfs.osHelper.IsNotExist(err) {
		return nil, status.Errorf(codes.Internal, "failed to remove target path: %s, err: %s", targetPath, err)
	}
	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (nfs *nfsstorage) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	// should never be called
	return nil, status.Error(codes.Unimplemented, "nfs NodeGetCapabilities not implemented")
}

func (nfs *nfsstorage) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	// should never be called
	return nil, status.Error(codes.Unimplemented, "nfs NodeGetInfo not implemented")
}

func (nfs *nfsstorage) NodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "NodeGetVolumeStats not implemented")
}

func (nfs *nfsstorage) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "NodeExpandVolume not implemented")
}
