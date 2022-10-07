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
	"errors"
	"fmt"
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

type NfsHelper interface {
	SetVolumePermissions(req *csi.NodePublishVolumeRequest) error
	GetNFSMountOptions(req *csi.NodePublishVolumeRequest) ([]string, error)
}

type Service struct{}

func (n Service) GetNFSMountOptions(req *csi.NodePublishVolumeRequest) (mountOptions []string, err error) {

	// Get mount options from VolumeCapability - the standard way
	mountOptions = req.GetVolumeCapability().GetMount().GetMountFlags()
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
		return mountOptions, err
	}

	if req.GetReadonly() {
		// TODO: ensure ro / rw behavior is correct, CSIC-343. eg what if user specifies "rw" as a mountOption?
		mountOptions = append(mountOptions, "ro")
	}

	klog.V(4).Infof("nfs mount options are [%v]", mountOptions)

	return mountOptions, nil
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

func (n Service) SetVolumePermissions(req *csi.NodePublishVolumeRequest) (err error) {

	targetPath := req.GetTargetPath()      // this is the path on the host node
	hostTargetPath := "/host" + targetPath // this is the path inside the csi container

	// Chown
	var uid_int, gid_int int
	tmp := req.GetVolumeContext()["uid"] // Returns an empty string if key not found
	if tmp == "" {
		uid_int = -1 // -1 means to not change the value
	} else {
		uid_int, err = strconv.Atoi(tmp)
		if err != nil || uid_int < -1 {
			msg := fmt.Sprintf("Storage class specifies an invalid volume UID with value [%d]: %s", uid_int, err)
			klog.Errorf(msg)
			return errors.New(msg)
		}
	}

	tmp = req.GetVolumeContext()["gid"]
	if tmp == "" {
		gid_int = -1 // -1 means to not change the value
	} else {
		gid_int, err = strconv.Atoi(tmp)
		if err != nil || gid_int < -1 {
			msg := fmt.Sprintf("Storage class specifies an invalid volume GID with value [%d]: %s", gid_int, err)
			klog.Errorf(msg)
			return errors.New(msg)
		}
	}

	err = os.Chown(hostTargetPath, uid_int, gid_int)
	if err != nil {
		msg := fmt.Sprintf("Failed to chown path '%s': %s", hostTargetPath, err)
		klog.Errorf(msg)
		return status.Errorf(codes.Internal, msg)
	}
	klog.V(4).Infof("chown mount %s uid=%d gid=%d", hostTargetPath, uid_int, gid_int)

	// Chmod
	unixPermissions := req.GetVolumeContext()["unix_permissions"] // Returns an empty string if key not found
	if unixPermissions != "" {
		tempVal, err := strconv.ParseUint(unixPermissions, 8, 32)
		if err != nil {
			msg := fmt.Sprintf("Failed to convert unix_permissions '%s' error: %s", unixPermissions, err.Error())
			klog.Errorf(msg)
			return status.Errorf(codes.Internal, msg)
		}
		mode := uint(tempVal)
		err = os.Chmod(hostTargetPath, os.FileMode(mode))
		if err != nil {
			msg := fmt.Sprintf("Failed to chmod path '%s' with perms %s: error: %s", hostTargetPath, unixPermissions, err.Error())
			klog.Errorf(msg)
			return status.Errorf(codes.Internal, msg)
		}
		klog.V(4).Infof("chmod mount %s perms=%s", hostTargetPath, unixPermissions)
	}

	// print out the target permissions
	cmd := exec.Command("ls", "-l", filepath.Dir(hostTargetPath))
	output, err := cmd.CombinedOutput()
	if err != nil {
		klog.Errorf("error in doing ls command  %s\n", err.Error())
	}
	klog.V(4).Infof("mount point permissions %s", string(output))

	return nil
}
