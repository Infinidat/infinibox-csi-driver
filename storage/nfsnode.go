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
	"path/filepath"
	"strconv"

	"regexp"
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
	targetPath := req.GetTargetPath()      // this is the path on the host node
	hostTargetPath := "/host" + targetPath // this is the path inside the csi container

	klog.V(4).Infof("NodePublishVolume targetPath=%s", hostTargetPath)

	notMnt, err := nfs.mounter.IsLikelyNotMountPoint(targetPath)
	if err != nil {
		if nfs.osHelper.IsNotExist(err) {
			if err := nfs.osHelper.MkdirAll(hostTargetPath, 0750); err != nil {
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

	// Get mount options from VolumeCapability - the standard way
	mountOptions := req.GetVolumeCapability().GetMount().GetMountFlags()
	if len(mountOptions) == 0 {
		// Not using standard way. Try legacy way.
		mountOptionsString, oldNfsMountOptionsParamProvided := req.GetVolumeContext()["nfs_mount_options"]
		if oldNfsMountOptionsParamProvided {
			// Use legacy nfs_mount_options parameter. TODO: Remove in future releases: CSIC-346
			klog.Warningf("Deprecated 'nfs_mount_options' parameter %s provided, will NOT be supported in future releases - please move to standard 'mountOptions' parameter", mountOptionsString)
		} else {
			// No mountOptions nor nfs_mount_options. Use default defined in storageservice.go.
			mountOptionsString = StandardMountOptions // defined in nfscontroller.go
		}
		// Split legacy or default string into slice
		for _, option := range strings.Split(mountOptionsString, ",") {
			if option != "" {
				mountOptions = append(mountOptions, option)
			}
		}
	}

	mountOptions, err = updateNfsMountOptions(mountOptions, req)
	if err != nil {
		klog.Errorf("Failed updateNfsMountOptions(): %s", err)
		return nil, status.Errorf(codes.Internal, "Failed to update mount options for targetPath '%s': %s", hostTargetPath, err)
	}

	klog.V(4).Infof("nfs mount options are [%v]", mountOptions)

	sourceIP := req.GetVolumeContext()["ipAddress"]
	ep := req.GetVolumeContext()["volPathd"]
	source := fmt.Sprintf("%s:%s", sourceIP, ep)
	klog.V(4).Infof("Mount sourcePath %v, targetPath %v", source, targetPath)
	err = nfs.mounter.Mount(source, targetPath, "nfs", mountOptions)
	if err != nil {
		klog.Errorf("Failed to mount source path '%s' : %s", source, err)
		return nil, status.Errorf(codes.Internal, "Failed to mount target path '%s': %s", targetPath, err)
	}
	log.Infof("Successfully mounted nfs volume '%s' to mount point '%s' with options %s", source, targetPath, mountOptions)

	// Chown
	uid := req.GetVolumeContext()["uid"] // Returns an empty string if key not found
	if uid == "" {
		uid = "-1" // -1 means to not change the value
	}
	gid := req.GetVolumeContext()["gid"]
	if gid == "" {
		gid = "-1" // -1 means to not change the value
	}

	if uid != "-1" || gid != "-1" {
		var uidInt, gidInt int
		uidInt, err = strconv.Atoi(uid)
		if err != nil {
			msg := fmt.Sprintf("Failed to convert uid to integer '%s'", err.Error())
			klog.Errorf(msg)
			return nil, status.Errorf(codes.Internal, msg)
		}
		gidInt, err = strconv.Atoi(gid)
		if err != nil {
			msg := fmt.Sprintf("Failed to convert gid to integer '%s'", err.Error())
			klog.Errorf(msg)
			return nil, status.Errorf(codes.Internal, msg)
		}

		err = os.Chown(hostTargetPath, uidInt, gidInt)
		if err != nil {
			msg := fmt.Sprintf("Failed to chown path '%s': %s", hostTargetPath, err)
			klog.Errorf(msg)
			return nil, status.Errorf(codes.Internal, msg)
		}
		log.Infof("chown mount %s uid=%d gid=%d", hostTargetPath, uidInt, gidInt)
	}

	// Chmod
	unixPermissions := req.GetVolumeContext()["unix_permissions"] // Returns an empty string if key not found
	if unixPermissions != "" {
		tempVal, err := strconv.ParseUint(unixPermissions, 8, 32)
		//err = os.Chmod("/host"+targetPath, 0770)
		if err != nil {
			msg := fmt.Sprintf("Failed to convert unix_permissions '%s' error: %s", unixPermissions, err.Error())
			klog.Errorf(msg)
			return nil, status.Errorf(codes.Internal, msg)
		}
		mode := uint(tempVal)
		err = os.Chmod(hostTargetPath, os.FileMode(mode))
		if err != nil {
			msg := fmt.Sprintf("Failed to chmod path '%s' with perms %s: error: %s", hostTargetPath, unixPermissions, err.Error())
			klog.Errorf(msg)
			return nil, status.Errorf(codes.Internal, msg)
		}
		log.Infof("chmod mount %s perms=%s", hostTargetPath, unixPermissions)
	}

	// print out the target permissions
	cmd := exec.Command("ls", "-l", filepath.Dir(hostTargetPath))
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("error in doing ls command  %s\n", err.Error())
	}
	klog.V(4).Infof("mount point permissions %s\n", string(output))

	return &csi.NodePublishVolumeResponse{}, nil
}

func updateNfsMountOptions(mountOptions []string, req *csi.NodePublishVolumeRequest) ([]string, error) {
	// If vers set to anything but 3, fail.
	re := regexp.MustCompile(`(nfs){0,1}vers=([0-9]*)`)
	for _, opt := range mountOptions {
		matches := re.FindStringSubmatch(opt)
		if len(matches) > 0 {
			version := matches[2]
			if version != "3" {
				err := fmt.Errorf("NFS version mount option '%s' encountered, but only NFS version 3 is supported", opt)
				klog.Error(err.Error())
				return nil, err
			}
		}
	}

	// Force vers=3 to be in the mountOptions slice. IBoxes require NFS version 3.
	vers3InMountOptions := false
	for _, opt := range mountOptions {
		if opt == "vers=3" || opt == "nfsvers=3" {
			vers3InMountOptions = true
			break
		}
	}
	if !vers3InMountOptions {
		mountOptions = append(mountOptions, "vers=3")
	}

	// Add option hard if 'soft' not set explicitly.
	hardInMountOptions := false
	softInMountOptions := false
	for _, opt := range mountOptions {
		if opt == "hard" {
			hardInMountOptions = true
		}
		if opt == "soft" {
			softInMountOptions = true
		}
	}
	if !hardInMountOptions && !softInMountOptions {
		mountOptions = append(mountOptions, "hard")
	}

	// Support readonly mount option.
	if req.GetReadonly() {
		// TODO: ensure ro / rw behavior is correct, CSIC-343. eg what if user specifies "rw" as a mountOption?
		mountOptions = append(mountOptions, "ro")
	}

	// TODO: remove duplicates from this list

	return mountOptions, nil
}

// func (nfs *nfsstorage) isCorruptedMnt(err error) bool {
// 	if err == nil {
// 		return false
// 	}
// 	var underlyingError error
// 	switch pe := err.(type) {
// 	case nil:
// 		return false
// 	case *os.PathError:
// 		underlyingError = pe.Err
// 	case *os.LinkError:
// 		underlyingError = pe.Err
// 	case *os.SyscallError:
// 		underlyingError = pe.Err
// 	}
//
// 	return underlyingError == syscall.ENOTCONN || underlyingError == syscall.ESTALE || underlyingError == syscall.EIO
// }

// func (nfs *nfsstorage) pathExists(path string) (bool, error) {
// 	_, err := os.Stat(path)
// 	if err == nil {
// 		klog.V(4).Infof("Path exists: %s", path)
// 		return true, nil
// 	} else if os.IsNotExist(err) {
// 		klog.V(4).Infof("Path does not exist: %s", path)
// 		return false, nil
// 	} else if nfs.isCorruptedMnt(err) {
// 		klog.V(4).Infof("Path is corrupted: %s", path)
// 		return true, err
// 	} else {
// 		klog.V(4).Infof("Path cannot be validated: %s", path)
// 		return false, err
// 	}
// }

func (nfs *nfsstorage) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	klog.V(4).Infof("NodeUnpublishVolume")
	targetPath := req.GetTargetPath()
	klog.V(4).Infof("Unmounting path '%s'", targetPath)
	err := unmountAndCleanUp(targetPath)
	if err != nil {
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
