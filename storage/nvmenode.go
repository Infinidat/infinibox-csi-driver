/*
Copyright 2024 Infinidat
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
	"encoding/json"
	"fmt"
	"infinibox-csi-driver/common"
	"infinibox-csi-driver/helper"

	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/kubernetes/pkg/volume/util"
	"k8s.io/mount-utils"
	utilexec "k8s.io/utils/exec"
)

type nvmeDiskMounter struct {
	*nvmeDisk
	readOnly     bool
	fsType       string
	mountOptions []string
	mounter      *mount.SafeFormatAndMount
	exec         utilexec.Interface
	deviceUtil   util.DeviceUtil
	targetPath   string
	stagePath    string
}

type nvmeTarget struct {
	Portals []string
	Iqn     string
}

type nvmeDisk struct {
	lun         string
	secret      map[string]string
	HostNQN     string
	VolumeID    int
	isBlock     bool
	MpathDevice string
	Targets     []nvmeTarget
}

// for NVMEDevices, note that UsedBytes, MaximumLBA, and PhysicalSize are different types (either int/int64 or string) depending
// on the version of nvme used, that is why they specify 'any' as the type.
// Ubuntu and RHEL return int/int64 for those whereas Suse returns strings.
// Currently these fields are unused so there is no need to check the type that was set in the JSON.
type NVMEDevices struct {
	Devices []struct {
		NameSpace    int    `json:"NameSpace"`
		DevicePath   string `json:"DevicePath"`
		Firmware     string `json:"Firmware"`
		Index        int    `json:"Index"`
		ModelNumber  string `json:"ModelNumber"`
		SerialNumber string `json:"SerialNumber"`
		UsedBytes    any    `json:"UsedBytes"`
		MaximumLBA   any    `json:"MaximumLBA"`
		PhysicalSize any    `json:"PhysicalSize"`
		SectorSize   int    `json:"SectorSize"`
	} `json:"Devices"`
}

const NVME_DISCOVERY_PORT = 8009

func (nvme *nvmestorage) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	zlog.Debug().Msgf("NodeStageVolume (nvme) - called with publish context: %s", req.GetPublishContext())

	hostID, ports, err := validatePublishContext(req.GetPublishContext())
	if err != nil {
		e := fmt.Errorf("NodeStageVolume (nvme) - - validatePublishContext - error: %s", err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	hostNQN, err := getHostNQN()
	if err != nil {
		e := fmt.Errorf("NodeStageVolume (nvme) - - getHostNQN - error: %s", err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	if !strings.Contains(ports, hostNQN) {
		zlog.Debug().Msgf("NodeStageVolume (nvme) - host nqn is not created, creating one")
		err = nvme.cs.AddPortForHost(hostID, "NVME", hostNQN)
		if err != nil {
			e := fmt.Errorf("NodeStageVolume (nvme) - AddPortForHost - error: %s", err.Error())
			zlog.Error().Msg(e.Error())
			return nil, status.Error(codes.Internal, e.Error())
		}
	}

	return &csi.NodeStageVolumeResponse{}, nil
}

func (nvme *nvmestorage) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {

	zlog.Debug().Msgf("NodePublishVolume (nvme) - volume ID %s, network_space %s mode %s readOnly %t", req.GetVolumeId(), req.GetVolumeContext()[common.SC_NETWORK_SPACE], req.GetVolumeCapability().GetAccessMode().Mode, req.Readonly)

	targets, err := nvme.getNVMETargets(req)
	if err != nil {
		e := fmt.Errorf("NodePublishVolume (nvme) - - getNVMETargets - error: %s", err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	zlog.Debug().Msgf("NodePublishVolume (nvme) - nvme %d targets  %v", len(targets), targets)

	nvmeDisk, err := nvme.getNVMEDisk(req)
	if err != nil {
		e := fmt.Errorf("NodePublishVolume (nvme) - getNVMEDisk - error: %s", err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	nvmeDisk.Targets = targets
	zlog.Debug().Msgf("NodePublishVolume (nvme) - nvmeDisk: vol %d lun %s", nvmeDisk.VolumeID, nvmeDisk.lun)

	diskMounter, err := nvme.getNVMEDiskMounter(nvmeDisk, req)
	if err != nil {
		e := fmt.Errorf("NodePublishVolume (nvme) - getNVMEDiskMounter - error: %s", err.Error())
		zlog.Error().Msg(e.Error())
		return nil, e
	}

	_, err = nvme.AttachDisk(*diskMounter, targets)
	if err != nil {
		e := fmt.Errorf("NodePublishVolume (nvme) - AttachDisk - error: %s", err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	zlog.Debug().Msgf("NodePublishVolume (nvme) - nvme attachDisk succeeded")

	if diskMounter.readOnly {
		zlog.Debug().Msgf("NodePublishVolume (nvme) - skipping chown-chmod since this is readOnly volume")
	} else {
		err = nvme.storageHelper.SetVolumePermissions(req)
		if err != nil {
			e := fmt.Errorf("NodePublishVolume (nvme) - SetVolumePermissions - error: %s", err.Error())
			zlog.Error().Msg(e.Error())
			return nil, status.Error(codes.Internal, e.Error())
		}
	}

	helper.PrettyKlogDebug("NodePublishVolume (nvme) - returning csi.NodePublishVolumeResponse:", csi.NodePublishVolumeResponse{})
	return &csi.NodePublishVolumeResponse{}, nil
}

func (nvme *nvmestorage) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	volumeId := req.GetVolumeId()
	targetPath := req.GetTargetPath()

	zlog.Debug().Msgf("NodeUnpublishVolume (nvme) - volume ID %s and targetPath '%s'", volumeId, targetPath)

	err := unmountAndCleanUp(targetPath)
	if err != nil {
		e := fmt.Errorf("NodeUnpublishVolume (nvme) - unmountAndCleanup - error: %s", err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (nvme *nvmestorage) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (res *csi.NodeUnstageVolumeResponse, err error) {

	zlog.Debug().Msgf("NodeUnstageVolume (nvme) - volume ID %s", req.GetVolumeId())

	stagePath := req.GetStagingTargetPath()
	zlog.Debug().Msgf("NodeUnstageVolume (nvme) - staging target path: %s", stagePath)

	removePath := path.Join("/host", stagePath)
	zlog.Debug().Msgf("NodeUnstageVolume (nvme) - calling RemoveAll with removePath '%s'", removePath)

	_ = debugWalkDir(removePath)

	// Remove directory contents
	zlog.Debug().Msgf("NodeUnstageVolume (nvme) - removePath '%s' is a directory", removePath)
	jsonPath := fmt.Sprintf("%s/%d.json", removePath, nvme.cs.VolProto.VolumeID)
	if err := os.Remove(jsonPath); err != nil {
		e := fmt.Errorf("NodeUnstageVolume (nvme) - Remove - failed to remove json file '%s': %v", jsonPath, err)
		zlog.Error().Msg(e.Error())
		return nil, e
	}

	// Remove directory or file
	zlog.Debug().Msgf("NodeUnstageVolume (nvme) - removing removePath '%s'", removePath)
	if err := os.Remove(removePath); err != nil {
		e := fmt.Errorf("NodeUnstageVolume (nvme) - Remove - failed to remove path '%s': %v", removePath, err)
		zlog.Error().Msg(e.Error())
		return nil, e
	}

	// logout all nvme connections if there are zero devices
	devices, err := getNVMENamespaces()
	if err != nil {
		zlog.Error().Msgf("NodeUnstageVolume (nvme) - getNVMENamespaces - error getting nvme devices %s", err.Error())
	} else {
		zlog.Debug().Msgf("NodeUnstageVolume (nvme) nvme device count %d", len(devices.Devices))
		if len(devices.Devices) == 0 {
			zlog.Debug().Msg("NodeUnstageVolume (nvme) zero devices - performing nvme disconnect-all")
			err = disconnectNVMEConnections()
			if err != nil {
				zlog.Error().Msgf("NodeUnstageVolume (nvme) - disconnectNVME - nvme logoutall error %s", err.Error())
			}
		}
	}

	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (nvme *nvmestorage) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	return nil, status.Error(codes.Unimplemented, "NodeGetCapabilities (nvme) should never be called, called in node.go instead")
}

func (nvme *nvmestorage) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	return &csi.NodeGetInfoResponse{}, nil
}

func (nvme *nvmestorage) NodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	return nil, status.Error(codes.Unimplemented, time.Now().String())
}

func (nvme *nvmestorage) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (response *csi.NodeExpandVolumeResponse, err error) {
	zlog.Info().Msgf("NodeExpandVolume (nvme) - called request volume ID %s path %s\n", req.GetVolumeId(), req.GetVolumePath())

	if req.GetVolumeCapability().GetBlock() != nil {
		zlog.Debug().Msg("NodeExpandVolume (nvme) - block volume resize on node")
		return response, nil
	}

	// run find the multipath device name (e.g. /dev/nvme0n2) in the list of mounts
	multipathDevice, err := findMultipathDeviceFromVolumePath(req.GetVolumePath())
	if err != nil {
		e := fmt.Errorf("NodeExpandVolume (nvme) - findMultipathDevice - error: %s", err.Error())
		zlog.Error().Msg(e.Error())
		return nil, e
	}

	// run resize2fs /dev/nvme0n2
	command := fmt.Sprintf("resize2fs %s", multipathDevice)
	out, err := execCommand.Command(command, "")
	if err != nil {
		e := fmt.Errorf("NodeExpandVolume (nvme) - Command - command: %s  error: %s", command, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, e
	}
	zlog.Debug().Msgf("NodeExpandVolume (nvme) - %s output is [%s]\n", command, strings.TrimSpace(string(out)))

	return response, nil
}

func (nvme *nvmestorage) AttachDisk(b nvmeDiskMounter, targets []nvmeTarget) (nvmeDevicePath string, err error) {

	zlog.Debug().Msgf("AttachDisk (nvme) - volName: %d mpathDevice: %s lun: %s fsType: %s readOnly: %v mountOpts: %v targetPath: %s stagePath: %s", b.nvmeDisk.VolumeID, b.nvmeDisk.MpathDevice,
		b.nvmeDisk.lun, b.fsType, b.readOnly, b.mountOptions, b.targetPath, b.stagePath)

	if len(targets) == 0 {
		return "", fmt.Errorf("AttachDisk (nvme) - error no targets")
	}
	if len(targets[0].Portals) == 0 {
		return "", fmt.Errorf("AttachDisk (nvme) - error target has no portals %v", targets)
	}

	ipAddressOnly := strings.Split(targets[0].Portals[0], ":")
	err = nvmeDiscover(ipAddressOnly[0])
	if err != nil {
		zlog.Error().Msgf("AttachDisk (nvme) - error nvme discover %s", err.Error())
		return "", err
	}

	err = nvmeConnectAll(ipAddressOnly[0])
	if err != nil {
		return "", err
	}

	devices, err := getNVMENamespaces()
	if err != nil {
		zlog.Error().Msgf("AttachDisk (nvme) - error getting NVME device list %s", err.Error())
		return "", err
	}

	// find the device path based on the lun/nsid

	for i := 0; i < len(devices.Devices); i++ {
		dev := devices.Devices[i]
		lunInt, err := strconv.Atoi(b.lun)
		if err != nil {
			return "", fmt.Errorf("AttachDisk (nvme) - could not convert lun %s to integer - error %s", b.lun, err.Error())
		}
		if dev.NameSpace == lunInt {
			nvmeDevicePath = dev.DevicePath
			zlog.Debug().Msgf("AttachDisk (nvme) - found nvme device path %s using lun %s", dev.DevicePath, b.lun)
			break
		}
	}
	if nvmeDevicePath == "" {
		return "", fmt.Errorf("AttachDisk (nvme) - could not find nvme device path using lun %s", b.lun)
	}

	// Mount device
	notMnt, err := b.mounter.IsLikelyNotMountPoint(b.targetPath)
	if err == nil {
		if !notMnt {
			zlog.Debug().Msgf("AttachDisk (nvme) - %s already mounted", b.targetPath)
			return "", nil
		}
	} else if !os.IsNotExist(err) {
		zlog.Error().Msgf("%s", err.Error())
		return "", status.Errorf(codes.Internal, "AttachDisk (nvme) - %s exists but IsLikelyNotMountPoint failed: %v", b.targetPath, err)
	}

	b.nvmeDisk.MpathDevice = nvmeDevicePath

	mountOptionMode := "rw"
	mode := "0750" //rwx
	var options []string
	if b.readOnly {
		mountOptionMode = "ro"
		mode = "0550" //read only
		zlog.Debug().Msgf("AttachDisk (nvme) - readOnly so setting mountPoint to %s", mode)
	}

	if b.isBlock {
		zlog.Debug().Msgf("AttachDisk (nvme) - mounting raw block volume at given path %s", b.targetPath)

		zlog.Debug().Msgf("AttachDisk (nvme) - run: mkdir --parents --mode %s '%s' ", mode, filepath.Dir(b.targetPath))
		// Do not use os.MkdirAll(). This ignores the mount chroot defined in the Dockerfile.
		// MkdirAll() will cause hard-to-grok mount errors.
		cmd := exec.Command("mkdir", "--parents", "--mode", mode, filepath.Dir(b.targetPath))
		err = cmd.Run()
		if err != nil {
			zlog.Error().Msgf("AttachDisk (nvme) - failed to mkdir '%s': %s", b.targetPath, err)
			return "", err
		}

		_, err = os.Create("/host/" + b.targetPath)
		if err != nil {
			e := fmt.Errorf("AttachDisk (nvme) - failed to create target file %q: %v", b.targetPath, err)
			zlog.Error().Msgf("%s", e.Error())
			return "", e
		}

		nvmeDevicePath = strings.Replace(nvmeDevicePath, "/host", "", 1)

		options = append(options, "bind")
		options = append(options, mountOptionMode)

		if err := b.mounter.Mount(nvmeDevicePath, b.targetPath, "", options); err != nil {
			zlog.Error().Msgf("AttachDisk (nvme) - failed to bind mount nvme block volume %s [%s] to %s, error %v", nvmeDevicePath, b.fsType, b.targetPath, err)
			return "", err
		}
		if err := nvme.createNVMEConfigFile(*(b.nvmeDisk), b.stagePath); err != nil {
			zlog.Error().Msgf("AttachDisk (nvme) - failed to save nvme config with error: %v", err)
			return "", err
		}
		zlog.Debug().Msgf("AttachDisk (nvme) - block volume bind mounted successfully to %s", b.targetPath)
		return nvmeDevicePath, nil
	} else {
		mountPoint := b.targetPath

		zlog.Debug().Msgf("AttachDisk (nvme) - mounting volume %s with filesystem at given path %s", nvmeDevicePath, mountPoint)

		// Create mountPoint if it does not exist.
		_, err := os.Stat(mountPoint)
		if os.IsNotExist(err) {
			zlog.Debug().Msgf("AttachDisk (nvme) - mount point does not exist. creating mount point.")
			// Do not use os.MkdirAll(). This ignores the mount chroot defined in the Dockerfile.
			// MkdirAll() will cause hard-to-grok mount errors.
			_, err := execCommand.Command("mkdir", fmt.Sprintf("--parents --mode %s '%s'", mode, mountPoint))
			if err != nil {
				zlog.Error().Msgf("AttachDisk (nvme) - failed to mkdir '%s': %v", mountPoint, err)
				return "", err
			}
		} else {
			zlog.Debug().Msgf("AttachDisk (nvme) - mkdir of mountPoint not required. '%s' already exists", mountPoint)
		}

		options = append(options, mountOptionMode) // BUG: what if user separately specified "rw" option?
		options = append(options, b.mountOptions...)

		// Persist here so that even if mount fails, the globalmount metadata json
		// file will contain an mpath to use during clean up.
		zlog.Debug().Msgf("AttachDisk (nvme) - persist nvme disk config to json file for later use, when detaching the disk")
		if err = nvme.createNVMEConfigFile(*(b.nvmeDisk), b.stagePath); err != nil {
			zlog.Error().Msgf("AttachDisk (nvme) - failed to save nvme config with error: %v", err)
			return "", err
		}

		if b.fsType == "xfs" {
			zlog.Debug().Msgf("AttachDisk (nvme) - device %s is of type XFS. Mounting without regard to its XFS UUID.", nvmeDevicePath)
			options = append(options, "nouuid")
		}

		zlog.Debug().Msgf("AttachDisk (nvme) - format '%s' (if needed) and mount volume", nvmeDevicePath)
		err = b.mounter.FormatAndMount(nvmeDevicePath, mountPoint, b.fsType, options)
		zlog.Debug().Msgf("AttachDisk (nvme) - formatAndMount returned: %+v", err)
		if err != nil {
			searchAlreadyMounted := fmt.Sprintf("already mounted on %s", mountPoint)
			zlog.Debug().Msgf("AttachDisk (nvme) - search error for matches to handle: %+v", err)

			if isAlreadyMounted := strings.Contains(err.Error(), searchAlreadyMounted); isAlreadyMounted {
				zlog.Error().Msgf("AttachDisk (nvme) - device %s is already mounted on %s", nvmeDevicePath, mountPoint)
			} else {
				zlog.Error().Msgf("AttachDisk (nvme) - failed to mount nvme volume %s [%s] to %s, error %+v", nvmeDevicePath, b.fsType, mountPoint, err)
				_, _ = mountPathExists(mountPoint)
				return "", err
			}
		}
	}
	zlog.Debug().Msgf("AttachDisk (nvme) - mounted volume with device path %s", nvmeDevicePath)
	return nvmeDevicePath, nil
}

func (nvme *nvmestorage) getNVMEDisk(req *csi.NodePublishVolumeRequest) (*nvmeDisk, error) {
	hostNQN, err := getHostNQN()
	if err != nil {
		return nil, err
	}

	volProto := nvme.cs.VolProto

	volContext := req.GetVolumeContext()
	publishContext := req.GetPublishContext()
	zlog.Debug().Msgf("getNVMEDisk (nvme) - volume: %d context: %v publish context: %v", volProto.VolumeID, volContext, publishContext)

	lun := publishContext[LUN_PUBLISH_CONTEXT]
	if lun == "" {
		return nil, fmt.Errorf("getNVMEDisk (nvme): LUN is missing")
	}

	secret := req.GetSecrets()

	return &nvmeDisk{
		VolumeID: volProto.VolumeID,
		lun:      lun,
		secret:   secret,
		HostNQN:  hostNQN,
	}, nil
}

func (nvme *nvmestorage) getNVMEDiskMounter(nvmeDisk *nvmeDisk, req *csi.NodePublishVolumeRequest) (*nvmeDiskMounter, error) {
	m := &nvmeDiskMounter{
		targetPath:   req.GetTargetPath(),
		stagePath:    req.GetStagingTargetPath(),
		mountOptions: []string{},
		mounter:      &mount.SafeFormatAndMount{Interface: mount.NewWithoutSystemd(""), Exec: utilexec.New()},
		exec:         utilexec.New(),
		deviceUtil:   util.NewDeviceHandler(util.NewIOHandler()),
	}

	// handle volumeCapabilities, the standard place to define block/file etc
	reqVolCapability := req.GetVolumeCapability()

	// check accessMode - where we will eventually police R/W etc (CSIC-343)
	accessMode := reqVolCapability.GetAccessMode().GetMode() // GetAccessMode() guaranteed not nil from controller.go

	if req.Readonly || accessMode == csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY {
		m.readOnly = true
	}
	// handle file (mount) and block parameters
	mountVolCapability := reqVolCapability.GetMount()
	blockVolCapability := reqVolCapability.GetBlock()

	// protocol-specific paths below
	if mountVolCapability != nil && blockVolCapability == nil {
		// option A. user wants file access to their nvme device
		nvmeDisk.isBlock = false

		m.fsType = mountVolCapability.GetFsType()

		// mountOptions - could be nothing
		m.mountOptions = mountVolCapability.GetMountFlags()

		// TODO: other validations needed for file?
		// - something about read-only access?
		// - check that fstype is supported?
		// - check that mount options are valid for fstype provided

	} else if mountVolCapability == nil && blockVolCapability != nil {
		// option B. user wants block access to their nvme device
		nvmeDisk.isBlock = true

		if accessMode == csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER {
			zlog.Warn().Msg("getNVMEDiskMounter (nvme) - MULTI_NODE_MULTI_WRITER AccessMode requested for raw block volume, could be dangerous")
		}
		// TODO: something about SINGLE_NODE_MULTI_WRITER (alpha feature) as well?

		// don't need to look at FsType or MountFlags here, only relevant for mountVol
		// TODO: other validations needed for block?
		// - something about read-only access?
	} else {
		errMsg := "getNVMEDiskMounter (nvme) - Bad VolumeCapability parameters: both block and mount modes, for volume: " + req.GetVolumeId()
		zlog.Error().Msg(errMsg)
		return nil, status.Error(codes.InvalidArgument, errMsg)
	}

	m.nvmeDisk = nvmeDisk

	return m, nil
}

func (nvme *nvmestorage) createNVMEConfigFile(conf nvmeDisk, mnt string) error {
	file := path.Join("/host", mnt, strconv.Itoa(conf.VolumeID)+".json")
	zlog.Debug().Msgf("createNVMEConfigFile (nvme) - creating nvme config file at path %s", file)
	fp, err := os.Create(file)
	if err != nil {
		zlog.Error().Msgf("%s", err.Error())
		return err
	}
	defer fp.Close()
	encoder := json.NewEncoder(fp)
	if err = encoder.Encode(conf); err != nil {
		zlog.Error().Msgf("%s", err.Error())
		return err
	}
	return nil
}

func (nvme *nvmestorage) getNVMETargets(req *csi.NodePublishVolumeRequest) (targets []nvmeTarget, err error) {
	networkSpaces := strings.Split(req.GetVolumeContext()[common.SC_NETWORK_SPACE], ",")
	if len(networkSpaces) == 0 {
		return targets, fmt.Errorf("getNVMETargets (nvme) - no network spaces found")
	}
	zlog.Debug().Msgf("networkSpaces %v", networkSpaces)
	if nvme.cs.Api == nil {
		return targets, fmt.Errorf("getNVMETargets (nvme) - no api found")
	}

	var portalsExist bool
	targets = make([]nvmeTarget, len(networkSpaces))

	for i := 0; i < len(networkSpaces); i++ {
		zlog.Debug().Msgf("getNVMETargets (nvme) - getting nspace by name: %v", networkSpaces[i])
		nspace, err := nvme.cs.IboxApi.GetNetworkSpaceByName(networkSpaces[i])
		if err != nil {
			e := fmt.Errorf("getNVMETargets (nvme) - error getting network space: %s error: %v", networkSpaces[i], err)
			zlog.Error().Msgf("%s", e.Error())
			return targets, status.Error(codes.InvalidArgument, e.Error())
		}
		zlog.Debug().Msgf("getNVMETargets (nvme) - got nspace by name: %s", nspace.Name)

		targets[i] = nvmeTarget{
			Portals: []string{},
		}
		for _, p := range nspace.Portals {
			if !p.Enabled {
				zlog.Error().Msgf("getNVMETargets (nvme) - network space %s ip address %s is disabled, not adding to list of available ip addresses", nspace.Name, p.IpAdress)
				continue
			}

			err := nvme.storageHelper.ValidateIPAddress(p.IpAdress, NVME_DISCOVERY_PORT)
			if err != nil {
				zlog.Error().Msgf("getNVMETargets (nvme) - error getting nvme network space %s ip connection to %s %d error: %v", networkSpaces[i], p.IpAdress, NVME_DISCOVERY_PORT, err)
				continue
			}

			zlog.Debug().Msgf("getNVMETargets (nvme) - adding nvme network space %s ip connection to %s %d list", networkSpaces[i], p.IpAdress, NVME_DISCOVERY_PORT)
			targets[i].Portals = append(targets[i].Portals, portalMounter(p.IpAdress))
			portalsExist = true
		}
	}

	if !portalsExist {
		return targets, fmt.Errorf("getNVMETargets (nvme) - there are zero network space ip addresses available")
	}
	return targets, nil
}

func getHostNQN() (string, error) {

	fileContent, err := os.ReadFile("/host/etc/nvme/hostnqn")
	if err != nil {
		zlog.Error().Msgf("getHostNQN (nvme) - failed to read hostnqn file %s", err.Error())
		return "", err
	}
	hostnqn := string(fileContent)
	hostnqn = strings.TrimSuffix(hostnqn, "\n")
	zlog.Debug().Msgf("getHostNQN (nvme) - host nqn %s ", hostnqn)
	return hostnqn, nil
}

func getNVMENamespaces() (devices NVMEDevices, err error) {
	cmd := "nvme list -o json"
	rawOutput, err := execCommand.Command(cmd, "")
	if err != nil {
		zlog.Error().Msgf("getHostNQN (nvme) - %s failed, err: %v, %s", cmd, err, rawOutput)
		return devices, err
	}
	zlog.Trace().Msgf("getHostNQN (nvme) - %s raw output %s", cmd, rawOutput)

	err = json.Unmarshal([]byte(rawOutput), &devices)
	if err != nil {
		zlog.Error().Msgf("getHostNQN (nvme) - error unmarshalling %s output - error %s", cmd, err.Error())
		return devices, err
	}
	return devices, nil
}

// nvme connect-all -t tcp -a 172.20.51.170
func nvmeConnectAll(ipAddress string) (err error) {
	cmd := fmt.Sprintf("nvme connect-all -t tcp -a %s", ipAddress)
	rawOutput, err := execCommand.Command(cmd, "")
	if err != nil {
		zlog.Error().Msgf("nvmeConnectAll (nvme) - %s failed, ip: %s err: %v, %s", cmd, ipAddress, err, rawOutput)
		return err
	}
	zlog.Debug().Msgf("nvmeConnectAll (nvme) - %s raw output %s", cmd, rawOutput)

	return nil
}

// nvme discover -t tcp -a 172.20.51.170 -s 8009
func nvmeDiscover(ipAddress string) (err error) {
	cmd := fmt.Sprintf("nvme discover -t tcp -a %s -s %d", ipAddress, NVME_DISCOVERY_PORT)
	rawOutput, err := execCommand.Command(cmd, "")
	if err != nil {
		zlog.Error().Msgf("nvmeDiscover (nvme) - %s failed, err: %v, %s", cmd, err, rawOutput)
		return err
	}
	zlog.Trace().Msgf("nvmeDiscover (nvme) - %s - raw output %s", cmd, rawOutput)

	return nil
}

func disconnectNVMEConnections() error {
	cmd := "nvme disconnect-all"
	rawOutput, err := execCommand.Command(cmd, "")
	if err != nil {
		zlog.Error().Msgf("disconnectNVMEConnections (nvme) - %s failed, err: %v, %s", cmd, err, rawOutput)
		return err
	}
	zlog.Debug().Msg(cmd)
	return nil
}

/**
// not used for now, but useful for debugging
func getConnectionDetails() (results string, err error) {
	cmd := "nvme list-subsys"
	results, err = execScsi.Command(cmd, "")
	if err != nil {
		zlog.Error().Msgf("%s failed, err: %v", cmd, err)
		return results, err
	}
	return results, nil
}
*/
