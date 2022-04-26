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
	log "infinibox-csi-driver/helper/logger"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"syscall"

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
			if err := nfs.osHelper.MkdirAll(targetPath, 0o750); err != nil {
				klog.Errorf("Error while mkdir %v", err)
				return nil, err
			}
			notMnt = true
		} else {
			klog.Errorf("IsLikelyNotMountPoint error  %v", err)
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
		mountOptionsString = StandardMountOptions // defined in nfscontroller.go
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
	log.Infof("Successfully mounted nfs volume '%s' to mount point '%s' with options %s", source, targetPath, mountOptions)

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

func (nfs *nfsstorage) isCorruptedMnt(err error) bool {
	if err == nil {
		return false
	}
	var underlyingError error
	switch pe := err.(type) {
	case nil:
		return false
	case *os.PathError:
		underlyingError = pe.Err
	case *os.LinkError:
		underlyingError = pe.Err
	case *os.SyscallError:
		underlyingError = pe.Err
	}

	return underlyingError == syscall.ENOTCONN || underlyingError == syscall.ESTALE || underlyingError == syscall.EIO
}

func (nfs *nfsstorage) pathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		klog.V(4).Infof("Path exists: %s", path)
		return true, nil
	} else if os.IsNotExist(err) {
		klog.V(4).Infof("Path does not exist: %s", path)
		return false, nil
	} else if nfs.isCorruptedMnt(err) {
		klog.V(4).Infof("Path is corrupted: %s", path)
		return true, err
	} else {
		klog.V(4).Infof("Path cannot be validated: %s", path)
		return false, err
	}
}

func (nfs *nfsstorage) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	klog.V(4).Infof("NodeUnpublishVolume")

	targetPath := req.GetTargetPath()

	targetHostPath := path.Join("/host", targetPath)
	pathToUnmount := ""

	if targetPathExist, targetPathErr := nfs.pathExists(targetPath); targetPathErr != nil {
		return nil, fmt.Errorf("failed to check if target path exists: %s, err: %v", targetPath, targetPathErr)
	} else if !targetPathExist { // Successfully checked targetPath
		// No targetPath so try targetHostPath
		if targetHostPathExist, targetHostPathErr := nfs.pathExists(targetHostPath); targetHostPathErr != nil {
			return nil, fmt.Errorf("Failed to check if mount path exists: %s, err: %v", targetHostPath, targetHostPathErr)
		} else { // Successfully checked targetHostPath
			if !targetHostPathExist {
				// No targetHostPath either
				klog.Warningf("Unmount not performed because host mount path exists in neither %s nor %s", targetPath, targetHostPath)
				return &csi.NodeUnpublishVolumeResponse{}, nil
			} else { // targetHostPath exists
				pathToUnmount = targetHostPath
			}
		}
	} else { // targetPath exists
		pathToUnmount = targetPath
	}

	if pathToUnmount != targetPath && pathToUnmount != targetHostPath {
		err := fmt.Errorf("Failed to unmount, path '%s' is not correct", pathToUnmount)
		klog.Error(err.Error())
		return nil, err
	}

	klog.V(4).Infof("Unmounting path: %s", pathToUnmount)
	if err := nfs.mounter.Unmount(pathToUnmount); err != nil {
		if strings.Contains(err.Error(), "not mounted") {
			klog.V(4).Infof("Path not mounted while trying to unmount %s", pathToUnmount)
		} else {
			klog.Errorf("Failed to unmount path: %s, err: %v", pathToUnmount, err)
			return nil, err
		}
	}

	if isEmpty, err := IsDirEmpty(pathToUnmount); err != nil {
		checkErr := fmt.Errorf("Failed to check if directory %s is empty: %s", pathToUnmount, err)
		klog.Error(checkErr.Error())
		return nil, checkErr
	} else if !isEmpty {
		err := fmt.Errorf("Cannot remove path %s. Not empty", pathToUnmount)
		klog.Error(err.Error())
		return nil, err
	}

	if err := os.Remove(pathToUnmount); err != nil {
		klog.Errorf("After unmounting, failed to remove mount path %s: %v", pathToUnmount, err)
		return nil, err
	}

	pathToUnmountParent := filepath.Dir(pathToUnmount)
	if isEmpty, err := IsDirEmpty(pathToUnmountParent); err != nil {
		checkErr := fmt.Errorf("Failed to check if parent directory %s is empty: %s", pathToUnmountParent, err)
		klog.Error(checkErr.Error())
		return nil, checkErr
	} else if !isEmpty {
		err := fmt.Errorf("Cannot remove path %s. Not empty", pathToUnmountParent)
		klog.Error(err.Error())
		return nil, err
	}
	if err := os.Remove(pathToUnmountParent); err != nil {
		klog.Errorf("After unmounting, failed to remove parent path %s: %v", pathToUnmountParent, err)
		return nil, err
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
