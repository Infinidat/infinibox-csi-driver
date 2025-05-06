/*
Copyright 2025 Infinidat
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
	"fmt"
	"regexp"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
)

const (
	NFSv3Port         = "2049"
	NFSv4Port         = "12049"
	NFS_VERSION_REGEX = `(nfs){0,1}vers=([0-9]*)`
)

func (n StorageService) GetNFSMountOptions(req *csi.NodePublishVolumeRequest) (mountOptions []string, err error) {
	// Get mount options from VolumeCapability - the standard way
	mountOptions = req.GetVolumeCapability().GetMount().GetMountFlags()
	if len(mountOptions) == 0 {
		for _, option := range strings.Split(StandardMountOptions, ",") {
			if option != "" {
				mountOptions = append(mountOptions, option)
			}
		}
	}

	mountOptions, err = updateNfsMountOptions(mountOptions, req)
	if err != nil {
		zlog.Error().Msgf("failed updateNfsMountOptions(): %s", err)
		return mountOptions, err
	}

	if req.GetReadonly() || req.VolumeCapability.GetAccessMode().GetMode() == csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY {
		mountOptions = append(mountOptions, "ro")
	}

	zlog.Debug().Msgf("nfs mount options are [%v]", mountOptions)

	return mountOptions, nil
}

func GetNFSVersionPort(mountOptions []string) (version, port string) {

	// we will default to nfs v3
	version = "3"
	port = NFSv3Port

	for _, opt := range mountOptions {
		if strings.Contains(opt, "vers") {
			parts := strings.Split(opt, "=")
			if len(parts) == 2 {
				version = parts[1]
			}
		}
		if strings.Contains(opt, "port") {
			parts := strings.Split(opt, "=")
			if len(parts) == 2 {
				port = parts[1]
			}
		}
	}
	if version == "4" || version == "4.1" {
		port = NFSv4Port
	}
	return version, port
}

func updateNfsMountOptions(mountOptions []string, req *csi.NodePublishVolumeRequest) ([]string, error) {
	// If vers set to anything but 3 or 4 or 4.1, fail.
	re := regexp.MustCompile(NFS_VERSION_REGEX)
	for _, opt := range mountOptions {
		matches := re.FindStringSubmatch(opt)
		if len(matches) > 0 {
			version := matches[2]
			if version != "3" && version != "4" && version != "4.1" {
				e := fmt.Errorf("nfs version mount option '%s' encountered, but only NFS versions 3 and 4 are supported", opt)
				zlog.Err(e)
				return nil, e
			}
		}
	}

	// Force vers=3 to be in the mountOptions slice if a vers is not explicitly set in the StorageClass. IBoxes require NFS version 3 or 4.
	versInMountOptions := false
	for _, opt := range mountOptions {
		if opt == "vers=3" || opt == "nfsvers=3" || opt == "vers=4" || opt == "nfsvers=4" || opt == "vers=4.1" || opt == "nfsvers=4.1" {
			versInMountOptions = true
			break
		}
	}
	if !versInMountOptions {
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
