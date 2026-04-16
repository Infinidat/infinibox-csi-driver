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
)

type nvmeTarget struct {
	Portals []string
	Iqn     string
}

type nvmeDevice struct {
	lun         string
	secret      map[string]string
	HostNQN     string
	VolumeID    int
	MpathDevice string
	Targets     []nvmeTarget
}

func (nvme *NVMEstorage) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	slog.Debug("start", "publish context", req.GetPublishContext(),
		"iboxInfo", storagecommon.GetHostInfo(ctx, req.GetSecrets(), nvme.CS.IboxAPI))

	hostID, ports, err := storagecommon.ValidatePublishContext(req.GetPublishContext())
	if err != nil {
		return nil, status.Error(codes.Internal, common.Errorf("from ValidatePublishContext - error: %w", err).Error())
	}

	hostNQN, err := getHostNQN()
	if err != nil {
		return nil, status.Error(codes.Internal, common.Errorf("from getHostNQN - error: %w", err).Error())
	}

	if !strings.Contains(ports, hostNQN) {
		slog.Debug("host nqn is not created, creating one")
		err = nvme.CS.AddPortForHost(ctx, hostID, "NVME", hostNQN)
		if err != nil {
			return nil, status.Error(codes.Internal, common.Errorf("from AddPortForHost - error: %w", err).Error())
		}
	}

	return &csi.NodeStageVolumeResponse{}, nil
}

func (nvme *NVMEstorage) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	slog.Debug("start", "volume id", req.GetVolumeId(), "network space", req.GetVolumeContext()[common.StorageClassNetworkSpace], "access mode", req.GetVolumeCapability().GetAccessMode().Mode, "readonly", req.Readonly,
		"iboxInfo", storagecommon.GetHostInfo(ctx, req.GetSecrets(), nvme.CS.IboxAPI))

	targets, err := nvme.getNVMETargets(ctx, req)
	if err != nil {
		return nil, status.Error(codes.Internal, common.Errorf("from getNVMETargets - error: %w", err).Error())
	}
	slog.Debug("info", "nvme targets", len(targets), "targets", targets)

	nvmeDisk, err := nvme.getNVMEDisk(req)
	if err != nil {
		return nil, status.Error(codes.Internal, common.Errorf("from getNVMEDisk - error: %w", err).Error())
	}
	nvmeDisk.Targets = targets
	slog.Debug("nvmeDisk", "volume id", nvmeDisk.VolumeID, "lun", nvmeDisk.lun)

	diskMounter, err := storagecommon.GetDiskMounter(req)
	if err != nil {
		return nil, status.Error(codes.Internal, common.Errorf("from getNVMEDiskMounter - error: %w", err).Error())
	}

	_, err = nvme.AttachDisk(*diskMounter, targets, *nvmeDisk)
	if err != nil {
		return nil, status.Error(codes.Internal, common.Errorf("from AttachDisk - error: %w", err).Error())
	}
	slog.Debug("nvme attachDisk succeeded")

	if diskMounter.ReadOnly {
		slog.Debug("skipping chown-chmod since this is readOnly volume")
	} else {
		err = nvme.StorageHelper.SetVolumePermissions(req)
		if err != nil {
			return nil, status.Error(codes.Internal, common.Errorf("from SetVolumePermissions - error: %w", err).Error())
		}
	}

	helper.PrettyKlogDebug("NodePublishVolume (nvme) - returning csi.NodePublishVolumeResponse:", csi.NodePublishVolumeResponse{})
	return &csi.NodePublishVolumeResponse{}, nil
}

func (nvme *NVMEstorage) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	volumeID := req.GetVolumeId()
	targetPath := req.GetTargetPath()

	slog.Debug("start", "volume id", volumeID, "targetpath", targetPath)

	err := storagecommon.UnmountAndCleanUp(targetPath)
	if err != nil {
		return nil, status.Error(codes.Internal, common.Errorf("from UnmountAndCleanup - error: %w", err).Error())
	}

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (nvme *NVMEstorage) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (res *csi.NodeUnstageVolumeResponse, err error) {

	stagePath := req.GetStagingTargetPath()

	removePath := path.Join("/host", stagePath)
	slog.Debug("start", "volume id", req.GetVolumeId(), "removePath", removePath)

	_ = storagecommon.DebugWalkDir(removePath)

	// Remove directory contents
	jsonPath := fmt.Sprintf("%s/%d.json", removePath, nvme.CS.VolProto.VolumeID)
	pathExists, pathErr := nvme.CS.PathExists(jsonPath)
	if pathErr != nil {
		slog.Error("path doesn't exist error", "path", jsonPath, "error", pathErr)
	} else {
		if pathExists {
			slog.Debug("removing json config file", "jsonPath", jsonPath)
			if err := os.Remove(jsonPath); err != nil {
				return nil, common.Errorf("from Remove - failed to remove json file: %s error: %w", jsonPath, err)
			}
		}
	}

	// Remove directory or file
	pathExists, pathErr = nvme.CS.PathExists(removePath)
	if pathErr != nil {
		slog.Error("path doesn't exist error", "path", removePath, "error", pathErr)
	} else {
		if pathExists {
			slog.Debug("removing removePath", "removePath", removePath)
			if err := os.Remove(removePath); err != nil {
				return nil, common.Errorf("from Remove - failed to remove path: %s error: %w", removePath, err)
			}
		}
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
	slog.Info("start", "volume id", req.GetVolumeId(), "volume path", req.GetVolumePath())

	if req.GetVolumeCapability().GetBlock() != nil {
		slog.Debug("block volume resize on node")
		return response, nil
	}

	// run find the multipath device name (e.g. /dev/nvme0n2) in the list of mounts
	multipathDevice, err := storagecommon.FindMultipathDeviceFromVolumePath(req.GetVolumePath())
	if err != nil {
		return nil, common.Errorf("from FindMultipathDevice - error: %w", err)
	}

	// run resize2fs or xfs_growfs on /dev/nvme0n2
	fsType := req.GetVolumeCapability().GetMount().FsType
	err = storagecommon.ExpandFileSystem(multipathDevice, fsType)
	if err != nil {
		return nil, common.Errorf("from ExpandFileSystem error: %w", err)
	}

	return response, nil
}

func (nvme *NVMEstorage) AttachDisk(diskMounter storagecommon.Mounter, targets []nvmeTarget, disk nvmeDevice) (nvmeDevicePath string, err error) {
	//slog.Debug("AttachDisk (nvme) - volName: %d mpathDevice: %s lun: %s fsType: %s readOnly: %v mountOpts: %v targetPath: %s stagePath: %s", diskMounter.nvmeDiskInfo.VolumeID, diskMounter.nvmeDiskInfo.MpathDevice,
	//diskMounter.nvmeDiskInfo.lun, diskMounter.fsType, diskMounter.readOnly, diskMounter.mountOptions, diskMounter.targetPath, diskMounter.stagePath)
	slog.Debug("start", "diskMounter", diskMounter)

	if len(targets) == 0 {
		return "", fmt.Errorf("error no targets")
	}
	for _, target := range targets {
		if len(target.Portals) == 0 {
			return "", fmt.Errorf("error target has no portals target: %v", target)
		}

		ipAddressOnly := strings.Split(target.Portals[0], ":")
		err = nvmeDiscover(ipAddressOnly[0])
		if err != nil {
			return "", common.Errorf("error nvme discover error: %w", err)
		}

		err = nvmeConnectAll(ipAddressOnly[0])
		if err != nil {
			return "", err
		}
	}

	devices, err := getNVMENamespacesByNormalOutput()
	if err != nil {
		slog.Error("error getting NVME device list", "error", err.Error())
		return "", err
	}

	// find the device path based on the lun/nsid

	for _, device := range devices {
		lunInt, err := strconv.Atoi(disk.lun)
		if err != nil {
			return "", common.Errorf("could not convert lun: %s to integer - error: %w", disk.lun, err)
		}
		if device.Namespace == lunInt {
			nvmeDevicePath = device.Node
			slog.Debug("found nvme device path using lun", "node", device.Node, "lun", disk.lun)
			break
		}
	}
	if nvmeDevicePath == "" {
		return "", common.Errorf("could not find nvme device path using lun: %s", disk.lun)
	}

	diskinf := storagecommon.DiskInfo{
		MpathDevice: nvmeDevicePath,
		VolumeID:    disk.VolumeID,
		IsBlock:     diskMounter.IsBlock,
		RootDir:     common.NodeRootDir,
	}

	slog.Debug("info", "diskinf", diskinf)

	err = storagecommon.MountLogic(diskinf, diskMounter.TargetPath, nvmeDevicePath, diskMounter.StagePath, diskMounter.FsType, diskMounter.MountOptions, diskMounter.IsBlock, diskMounter.ReadOnly)
	if err != nil {
		return "", common.Errorf("from MountLogic error: %w", err)
	}

	slog.Debug("mounted volume", "device path", nvmeDevicePath)
	return nvmeDevicePath, nil
}

func (nvme *NVMEstorage) getNVMEDisk(req *csi.NodePublishVolumeRequest) (*nvmeDevice, error) {
	hostNQN, err := getHostNQN()
	if err != nil {
		return nil, err
	}

	volProto := nvme.CS.VolProto

	volContext := req.GetVolumeContext()
	publishContext := req.GetPublishContext()
	slog.Debug("start", "volume id", volProto.VolumeID, "volume context", volContext, "publishcontext", publishContext)

	lun := publishContext[storagecommon.LunPublishContext]
	if lun == "" {
		return nil, fmt.Errorf("lun is missing")
	}

	secret := req.GetSecrets()

	return &nvmeDevice{
		VolumeID: volProto.VolumeID,
		lun:      lun,
		secret:   secret,
		HostNQN:  hostNQN,
	}, nil
}

func (nvme *NVMEstorage) getNVMETargets(ctx context.Context, req *csi.NodePublishVolumeRequest) (targets []nvmeTarget, err error) {
	networkSpaces := strings.Split(req.GetVolumeContext()[common.StorageClassNetworkSpace], ",")
	if len(networkSpaces) == 0 {
		return targets, fmt.Errorf("no network spaces found")
	}
	slog.Debug("info", "networkSpaces", networkSpaces)
	if nvme.CS.API == nil {
		return targets, fmt.Errorf("no api found")
	}

	var portalsExist bool
	targets = make([]nvmeTarget, len(networkSpaces))

	for index, networkSpace := range networkSpaces {
		slog.Debug("getting nspace by name", "networkSpace", networkSpace)
		nspace, err := nvme.CS.IboxAPI.GetNetworkSpaceByName(ctx, networkSpace)
		if err != nil {
			return targets, status.Error(codes.InvalidArgument, common.Errorf("error getting network space: %s error: %w", networkSpace, err).Error())
		}
		slog.Debug("got nspace by name", "name", nspace.Name)

		targets[index] = nvmeTarget{
			Portals: []string{},
		}
		for _, portal := range nspace.Portals {
			if !portal.Enabled {
				slog.Error("network space ip address is disabled, not adding to list of available ip addresses", "network space", nspace.Name, "ip address", portal.IPAddress)
				continue
			}

			err := nvme.StorageHelper.ValidateIPAddress(portal.IPAddress, NVMEDiscoveryPort)
			if err != nil {
				slog.Error("error getting nvme network space ip connection error", "networkspace", networkSpace, "ip address", portal.IPAddress, "port", NVMEDiscoveryPort, "error", err)
				continue
			}

			slog.Debug("adding nvme network space ip connection to list", "network space", networkSpace, "ip address", portal.IPAddress, "port", NVMEDiscoveryPort)
			targets[index].Portals = append(targets[index].Portals, storagecommon.PortalMounter(portal.IPAddress))
			portalsExist = true
		}
	}

	if !portalsExist {
		return targets, fmt.Errorf("there are zero network space ip addresses available")
	}
	return targets, nil
}
