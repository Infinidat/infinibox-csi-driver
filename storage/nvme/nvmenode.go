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
package nvme

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/infinidat/infinibox-csi-driver/helper"
	storagecommon "github.com/infinidat/infinibox-csi-driver/storage/common"

	"os"
	"path"
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
	nvmeDiskInfo *nvmeDisk
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

func (nvme *NVMEstorage) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	slog.Debug("NodeStageVolume (nvme)", "publish context", req.GetPublishContext(),
		"iboxInfo", storagecommon.GetHostInfo(ctx, req.GetSecrets(), nvme.CS.IboxAPI))

	hostID, ports, err := storagecommon.ValidatePublishContext(req.GetPublishContext())
	if err != nil {
		e := fmt.Errorf("NodeStageVolume (nvme) - - validatePublishContext - error: %s", err.Error())
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	hostNQN, err := getHostNQN()
	if err != nil {
		e := fmt.Errorf("NodeStageVolume (nvme) - - getHostNQN - error: %s", err.Error())
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	if !strings.Contains(ports, hostNQN) {
		slog.Debug("NodeStageVolume (nvme) - host nqn is not created, creating one")
		err = nvme.CS.AddPortForHost(ctx, hostID, "NVME", hostNQN)
		if err != nil {
			e := fmt.Errorf("NodeStageVolume (nvme) - AddPortForHost - error: %s", err.Error())
			slog.Error(e.Error())
			return nil, status.Error(codes.Internal, e.Error())
		}
	}

	return &csi.NodeStageVolumeResponse{}, nil
}

func (nvme *NVMEstorage) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	slog.Debug("NodePublishVolume (nvme)", "volume id", req.GetVolumeId(), "network space", req.GetVolumeContext()[common.StorageClassNetworkSpace], "access mode", req.GetVolumeCapability().GetAccessMode().Mode, "readonly", req.Readonly,
		"iboxInfo", storagecommon.GetHostInfo(ctx, req.GetSecrets(), nvme.CS.IboxAPI))

	targets, err := nvme.getNVMETargets(ctx, req)
	if err != nil {
		e := fmt.Errorf("NodePublishVolume (nvme) - - getNVMETargets - error: %s", err.Error())
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	slog.Debug("NodePublishVolume (nvme)", "nvme targets", len(targets), "targets", targets)

	nvmeDisk, err := nvme.getNVMEDisk(req)
	if err != nil {
		e := fmt.Errorf("NodePublishVolume (nvme) - getNVMEDisk - error: %s", err.Error())
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	nvmeDisk.Targets = targets
	slog.Debug("NodePublishVolume (nvme) - nvmeDisk", "volume id", nvmeDisk.VolumeID, "lun", nvmeDisk.lun)

	diskMounter, err := nvme.getNVMEDiskMounter(nvmeDisk, req)
	if err != nil {
		e := fmt.Errorf("NodePublishVolume (nvme) - getNVMEDiskMounter - error: %s", err.Error())
		slog.Error(e.Error())
		return nil, e
	}

	_, err = nvme.AttachDisk(*diskMounter, targets)
	if err != nil {
		e := fmt.Errorf("NodePublishVolume (nvme) - AttachDisk - error: %s", err.Error())
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	slog.Debug("NodePublishVolume (nvme) - nvme attachDisk succeeded")

	if diskMounter.readOnly {
		slog.Debug("NodePublishVolume (nvme) - skipping chown-chmod since this is readOnly volume")
	} else {
		err = nvme.StorageHelper.SetVolumePermissions(req)
		if err != nil {
			e := fmt.Errorf("NodePublishVolume (nvme) - SetVolumePermissions - error: %s", err.Error())
			slog.Error(e.Error())
			return nil, status.Error(codes.Internal, e.Error())
		}
	}

	helper.PrettyKlogDebug("NodePublishVolume (nvme) - returning csi.NodePublishVolumeResponse:", csi.NodePublishVolumeResponse{})
	return &csi.NodePublishVolumeResponse{}, nil
}

func (nvme *NVMEstorage) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	volumeID := req.GetVolumeId()
	targetPath := req.GetTargetPath()

	slog.Debug("NodeUnpublishVolume (nvme)", "volume id", volumeID, "targetpath", targetPath)

	err := storagecommon.UnmountAndCleanUp(targetPath)
	if err != nil {
		e := fmt.Errorf("NodeUnpublishVolume (nvme) - unmountAndCleanup - error: %s", err.Error())
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (nvme *NVMEstorage) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (res *csi.NodeUnstageVolumeResponse, err error) {

	stagePath := req.GetStagingTargetPath()

	removePath := path.Join("/host", stagePath)
	slog.Debug("NodeUnstageVolume (nvme)", "volume id", req.GetVolumeId(), "stagePath", stagePath, "removePath", removePath)

	_ = storagecommon.DebugWalkDir(removePath)

	// Remove directory contents
	slog.Debug("NodeUnstageVolume (nvme) - removePath is a directory", "removePath", removePath)
	jsonPath := fmt.Sprintf("%s/%d.json", removePath, nvme.CS.VolProto.VolumeID)
	if err := os.Remove(jsonPath); err != nil {
		e := fmt.Errorf("NodeUnstageVolume (nvme) - Remove - failed to remove json file '%s': %v", jsonPath, err)
		slog.Error(e.Error())
		return nil, e
	}

	// Remove directory or file
	slog.Debug("NodeUnstageVolume (nvme) - removing removePath", "removePath", removePath)
	if err := os.Remove(removePath); err != nil {
		e := fmt.Errorf("NodeUnstageVolume (nvme) - Remove - failed to remove path '%s': %v", removePath, err)
		slog.Error(e.Error())
		return nil, e
	}

	// logout all nvme connections if there are zero devices
	// currently no real way to know if you have zero devices, there is too
	// much latency and no real reason to disconnect on a real system
	/**
	devices, err := getNVMENamespaces()
	if err != nil {
		slog.Error("NodeUnstageVolume (nvme) - getNVMENamespaces - error getting nvme devices %s", err.Error())
	} else {
		slog.Debug("NodeUnstageVolume (nvme) nvme device count %d", len(devices.Devices))
		if len(devices.Devices) == 0 {
			slog.Debug().Msg("NodeUnstageVolume (nvme) zero devices - performing nvme disconnect-all")
			err = disconnectNVMEConnections()
			if err != nil {
				slog.Error("NodeUnstageVolume (nvme) - disconnectNVME - nvme logoutall error %s", err.Error())
			}
		}
	}
	*/

	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (nvme *NVMEstorage) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	return nil, status.Error(codes.Unimplemented, "NodeGetCapabilities (nvme) should never be called, called in node.go instead")
}

func (nvme *NVMEstorage) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	return &csi.NodeGetInfoResponse{}, nil
}

func (nvme *NVMEstorage) NodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	return nil, status.Error(codes.Unimplemented, time.Now().String())
}

func (nvme *NVMEstorage) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (response *csi.NodeExpandVolumeResponse, err error) {
	slog.Info("NodeExpandVolume (nvme)", "volume id", req.GetVolumeId(), "volume path", req.GetVolumePath())

	if req.GetVolumeCapability().GetBlock() != nil {
		slog.Debug("NodeExpandVolume (nvme) - block volume resize on node")
		return response, nil
	}

	// run find the multipath device name (e.g. /dev/nvme0n2) in the list of mounts
	multipathDevice, err := storagecommon.FindMultipathDeviceFromVolumePath(req.GetVolumePath())
	if err != nil {
		e := fmt.Errorf("NodeExpandVolume (nvme) - findMultipathDevice - error: %s", err.Error())
		slog.Error(e.Error())
		return nil, e
	}

	// run resize2fs or xfs_growfs on /dev/nvme0n2
	fsType := req.GetVolumeCapability().GetMount().FsType
	err = storagecommon.ExpandFileSystem(multipathDevice, fsType)
	if err != nil {
		e := fmt.Errorf("NodeExpandVolume (nvme) - error: %s", err.Error())
		slog.Error(e.Error())
		return nil, e
	}

	return response, nil
}

func (nvme *NVMEstorage) AttachDisk(diskMounter nvmeDiskMounter, targets []nvmeTarget) (nvmeDevicePath string, err error) {
	//slog.Debug("AttachDisk (nvme) - volName: %d mpathDevice: %s lun: %s fsType: %s readOnly: %v mountOpts: %v targetPath: %s stagePath: %s", diskMounter.nvmeDiskInfo.VolumeID, diskMounter.nvmeDiskInfo.MpathDevice,
	//diskMounter.nvmeDiskInfo.lun, diskMounter.fsType, diskMounter.readOnly, diskMounter.mountOptions, diskMounter.targetPath, diskMounter.stagePath)
	slog.Debug("AttachDisk (nvme)", "diskMounter", diskMounter)

	if len(targets) == 0 {
		return "", fmt.Errorf("AttachDisk (nvme) - error no targets")
	}
	for _, target := range targets {
		if len(target.Portals) == 0 {
			return "", fmt.Errorf("AttachDisk (nvme) - error target has no portals %v", target)
		}

		ipAddressOnly := strings.Split(target.Portals[0], ":")
		err = nvmeDiscover(ipAddressOnly[0])
		if err != nil {
			slog.Error("AttachDisk (nvme) - error nvme discover", "error", err.Error())
			return "", err
		}

		err = nvmeConnectAll(ipAddressOnly[0])
		if err != nil {
			return "", err
		}
	}

	devices, err := getNVMENamespacesByNormalOutput()
	if err != nil {
		slog.Error("AttachDisk (nvme) - error getting NVME device list", "error", err.Error())
		return "", err
	}

	// find the device path based on the lun/nsid

	for _, device := range devices {
		lunInt, err := strconv.Atoi(diskMounter.nvmeDiskInfo.lun)
		if err != nil {
			return "", fmt.Errorf("AttachDisk (nvme) - could not convert lun %s to integer - error %s", diskMounter.nvmeDiskInfo.lun, err.Error())
		}
		if device.Namespace == lunInt {
			nvmeDevicePath = device.Node
			slog.Debug("AttachDisk (nvme) - found nvme device path using lun", "node", device.Node, "lun", diskMounter.nvmeDiskInfo.lun)
			break
		}
	}
	if nvmeDevicePath == "" {
		return "", fmt.Errorf("AttachDisk (nvme) - could not find nvme device path using lun %s", diskMounter.nvmeDiskInfo.lun)
	}

	diskinf := storagecommon.DiskInfo{
		MpathDevice: nvmeDevicePath,
		VolumeID:    diskMounter.nvmeDiskInfo.VolumeID,
		IsBlock:     diskMounter.nvmeDiskInfo.isBlock,
		RootDir:     common.NodeRootDir,
	}

	slog.Debug("AttachDisk (nvme)", "diskinf", diskinf)

	err = storagecommon.MountLogic(diskinf, diskMounter.targetPath, nvmeDevicePath, diskMounter.stagePath, diskMounter.fsType, diskMounter.mountOptions, diskMounter.nvmeDiskInfo.isBlock, diskMounter.readOnly)
	if err != nil {
		slog.Error("AttachDisk (nvme) - mountLogic()", "error", err.Error())
		return "", err
	}

	slog.Debug("AttachDisk (nvme) - mounted volume", "device path", nvmeDevicePath)
	return nvmeDevicePath, nil
}

func (nvme *NVMEstorage) getNVMEDisk(req *csi.NodePublishVolumeRequest) (*nvmeDisk, error) {
	hostNQN, err := getHostNQN()
	if err != nil {
		return nil, err
	}

	volProto := nvme.CS.VolProto

	volContext := req.GetVolumeContext()
	publishContext := req.GetPublishContext()
	slog.Debug("getNVMEDisk (nvme)", "volume id", volProto.VolumeID, "volume context", volContext, "publishcontext", publishContext)

	lun := publishContext[storagecommon.LunPublishContext]
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

func (nvme *NVMEstorage) getNVMEDiskMounter(nvmeDisk *nvmeDisk, req *csi.NodePublishVolumeRequest) (*nvmeDiskMounter, error) {
	diskMounter := &nvmeDiskMounter{
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
		diskMounter.readOnly = true
	}
	// handle file (mount) and block parameters
	mountVolCapability := reqVolCapability.GetMount()
	blockVolCapability := reqVolCapability.GetBlock()

	// protocol-specific paths below
	if mountVolCapability != nil && blockVolCapability == nil {
		// option A. user wants file access to their nvme device
		nvmeDisk.isBlock = false

		diskMounter.fsType = mountVolCapability.GetFsType()

		// mountOptions - could be nothing
		diskMounter.mountOptions = mountVolCapability.GetMountFlags()

		// TODO: other validations needed for file?
		// - something about read-only access?
		// - check that fstype is supported?
		// - check that mount options are valid for fstype provided
	} else if mountVolCapability == nil && blockVolCapability != nil {
		// option B. user wants block access to their nvme device
		nvmeDisk.isBlock = true

		if accessMode == csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER {
			slog.Warn("getNVMEDiskMounter (nvme) - MULTI_NODE_MULTI_WRITER AccessMode requested for raw block volume, could be dangerous")
		}
		// TODO: something about SINGLE_NODE_MULTI_WRITER (alpha feature) as well?

		// don't need to look at FsType or MountFlags here, only relevant for mountVol
		// TODO: other validations needed for block?
		// - something about read-only access?
	} else {
		errMsg := "getNVMEDiskMounter (nvme) - Bad VolumeCapability parameters: both block and mount modes, for volume: " + req.GetVolumeId()
		slog.Error(errMsg)
		return nil, status.Error(codes.InvalidArgument, errMsg)
	}

	diskMounter.nvmeDiskInfo = nvmeDisk

	return diskMounter, nil
}

func (nvme *NVMEstorage) getNVMETargets(ctx context.Context, req *csi.NodePublishVolumeRequest) (targets []nvmeTarget, err error) {
	networkSpaces := strings.Split(req.GetVolumeContext()[common.StorageClassNetworkSpace], ",")
	if len(networkSpaces) == 0 {
		return targets, fmt.Errorf("getNVMETargets (nvme) - no network spaces found")
	}
	slog.Debug("info", "networkSpaces", networkSpaces)
	if nvme.CS.API == nil {
		return targets, fmt.Errorf("getNVMETargets (nvme) - no api found")
	}

	var portalsExist bool
	targets = make([]nvmeTarget, len(networkSpaces))

	for index, networkSpace := range networkSpaces {
		slog.Debug("getNVMETargets (nvme) - getting nspace by name", "networkSpace", networkSpace)
		nspace, err := nvme.CS.IboxAPI.GetNetworkSpaceByName(ctx, networkSpace)
		if err != nil {
			e := fmt.Errorf("getNVMETargets (nvme) - error getting network space: %s error: %v", networkSpace, err)
			slog.Error(e.Error())
			return targets, status.Error(codes.InvalidArgument, e.Error())
		}
		slog.Debug("getNVMETargets (nvme) - got nspace by name", "name", nspace.Name)

		targets[index] = nvmeTarget{
			Portals: []string{},
		}
		for _, portal := range nspace.Portals {
			if !portal.Enabled {
				slog.Error("getNVMETargets (nvme) - network space ip address is disabled, not adding to list of available ip addresses", "network space", nspace.Name, "ip address", portal.IPAddress)
				continue
			}

			err := nvme.StorageHelper.ValidateIPAddress(portal.IPAddress, NVMEDiscoveryPort)
			if err != nil {
				slog.Error("getNVMETargets (nvme) - error getting nvme network space ip connection error", "networkspace", networkSpace, "ip address", portal.IPAddress, "port", NVMEDiscoveryPort, "error", err)
				continue
			}

			slog.Debug("getNVMETargets (nvme) - adding nvme network space ip connection to list", "network space", networkSpace, "ip address", portal.IPAddress, "port", NVMEDiscoveryPort)
			targets[index].Portals = append(targets[index].Portals, storagecommon.PortalMounter(portal.IPAddress))
			portalsExist = true
		}
	}

	if !portalsExist {
		return targets, fmt.Errorf("getNVMETargets (nvme) - there are zero network space ip addresses available")
	}
	return targets, nil
}
