/*
Copyright 2022 Infinidat
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
package fc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"strconv"

	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/infinidat/infinibox-csi-driver/helper"
	"github.com/infinidat/infinibox-csi-driver/iboxapi"
	storagecommon "github.com/infinidat/infinibox-csi-driver/storage/common"

	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fcDevice struct {
	VolumeID   int
	TargetWWNs []string
	Lun        string
	WWIDs      []string
}

const FCPortOnline = "Online"

func (fc *FCstorage) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	defer helper.TimeTrack(time.Now())
	var err error
	slog.Debug("start", "PublishContext", req.GetPublishContext(), "volume ID", req.GetVolumeId(), "iboxInfo", storagecommon.GetHostInfo(ctx, req.GetSecrets(), fc.CS.IboxAPI))

	hostID, _, err := storagecommon.ValidatePublishContext(req.GetPublishContext())
	if err != nil {
		_, file, line, _ := runtime.Caller(0)
		e := storagecommon.ImplementationError{
			Code: int(codes.Internal),
			Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
		}
		return nil, e
	}

	portInfo := storagecommon.GetPortInfo()
	fcPorts := []string{}
	for _, p := range portInfo {
		fcPorts = append(fcPorts, p.PortName)
	}

	if len(fcPorts) == 0 {
		_, file, line, _ := runtime.Caller(0)
		e := storagecommon.ImplementationError{
			Code: int(codes.Internal),
			Msg:  fmt.Sprintf("%s:%d: %s", file, line, "port name not found on worker"),
		}
		return nil, e
	}
	slog.Debug("getPortName", "output", fcPorts)

	var fcOnline bool
	for _, p := range portInfo {
		if p.PortState == FCPortOnline {
			fcOnline = true
		}
	}

	if !fcOnline {
		_, file, line, _ := runtime.Caller(0)
		e := storagecommon.ImplementationError{
			Code: int(codes.Internal),
			Msg:  fmt.Sprintf("%s:%d: %s", file, line, "all FC ports on worker are offline"),
		}
		return nil, e
	}

	slog.Debug("publishing volume to host ", "host ID", hostID, "port state", fcOnline)

	for _, fcp := range fcPorts {
		slog.Debug("GetHostPort", "hostID", hostID, "with", fcp)
		_, err = fc.CS.IboxAPI.GetHostPort(ctx, hostID, fcp)
		if err != nil {
			if errors.Is(err, iboxapi.ErrNotFound) {
				//if !strings.Contains(ports, fcp) {
				slog.Debug("host port is not created, creating it", "host port", fcp)
				err = fc.CS.AddPortForHost(ctx, hostID, "FC", fcp)
				if err != nil {
					_, file, line, _ := runtime.Caller(0)
					e := storagecommon.ImplementationError{
						Code: int(codes.Internal),
						Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
					}
					return nil, e
				}
			} else {
				_, file, line, _ := runtime.Caller(0)
				e := storagecommon.ImplementationError{
					Code: int(codes.Internal),
					Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
				}
				return nil, e
			}
		} else {
			slog.Debug("host port already created", "hostID", hostID, "host port", fcp)
		}
	}

	return &csi.NodeStageVolumeResponse{}, nil
}

func (fc *FCstorage) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	defer helper.TimeTrack(time.Now())
	var err error
	defer func() {
		if err == nil {
			slog.Debug("succeeded", "volume ID", req.GetVolumeId())
		} else {
			slog.Debug("failed", "volume ID", req.GetVolumeId(), "error", err)
		}
	}()

	slog.Debug("start", "volume ID", req.GetVolumeId(), "volumecontext", req.GetVolumeContext(),
		"iboxInfo", storagecommon.GetHostInfo(ctx, req.GetSecrets(), fc.CS.IboxAPI))
	slog.Debug("info", "uid", req.GetVolumeContext()[common.StorageClassUID], "gid", req.GetVolumeContext()[common.StorageClassGID], "unix perm", req.GetVolumeContext()[common.StorageClassUNIXPermissions])

	fcDetails, err := fc.getFCDiskDetails(ctx, req)
	if err != nil {
		_, file, line, _ := runtime.Caller(0)
		e := storagecommon.ImplementationError{
			Code: int(codes.Internal),
			Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
		}
		return nil, e
	}

	devicePath, err := fc.searchDisk(*fcDetails)
	if err != nil {
		_, file, line, _ := runtime.Caller(0)
		e := storagecommon.ImplementationError{
			Code: int(codes.Internal),
			Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
		}
		return nil, e
	}

	// remove the leading "/host" to reveal the path on the actual node host
	devicePath = strings.Replace(devicePath, "/host", "", 1)
	slog.Debug("fc device path found", "path", devicePath)

	//diskMounter, err := storagecommon.GetDiskMounter(req, *fcDetails)
	diskMounter, err := storagecommon.GetDiskMounter(req)
	if err != nil {
		_, file, line, _ := runtime.Caller(0)
		e := storagecommon.ImplementationError{
			Code: int(codes.Internal),
			Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
		}
		return nil, e
	}

	err = fc.mountFCDisk(*diskMounter, devicePath, fcDetails.VolumeID)
	if err != nil {
		_, file, line, _ := runtime.Caller(0)
		e := storagecommon.ImplementationError{
			Code: int(codes.Internal),
			Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
		}
		return nil, e
	}

	// set volume permissions based on uid/uid/unix_permissions
	slog.Debug("info", "volume ID:", req.GetVolumeId(), "after mount targetPath", diskMounter.TargetPath, "devicePath", devicePath)
	// print out the target permissions
	storagecommon.LogPermissions("after mount targetPath ", filepath.Dir("/host"+diskMounter.TargetPath))
	storagecommon.LogPermissions("after mount devicePath ", "/host"+devicePath)
	if diskMounter.ReadOnly {
		slog.Debug("skipping chown-chmod since this is readOnly volume")
	} else {
		err = fc.StorageHelper.SetVolumePermissions(req)
		if err != nil {
			_, file, line, _ := runtime.Caller(0)
			e := storagecommon.ImplementationError{
				Code: int(codes.Internal),
				Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
			}
			return nil, e
		}
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func (fc *FCstorage) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	var err error
	defer helper.TimeTrack(time.Now())

	targetPath := req.GetTargetPath()
	slog.Debug("start", "volume ID", fc.CS.VolProto.VolumeID, "targetPath", targetPath)

	err = storagecommon.UnmountAndCleanUp(targetPath)
	if err != nil {
		_, file, line, _ := runtime.Caller(0)
		e := storagecommon.ImplementationError{
			Code: int(codes.Internal),
			Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
		}
		return nil, e
	}
	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (fc *FCstorage) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	defer helper.TimeTrack(time.Now())
	stagePath := req.GetStagingTargetPath()
	slog.Debug("start", "stagePath", stagePath)
	cmd := exec.Command("ls", "/host/"+stagePath)
	out, err2 := cmd.Output()

	if err2 != nil {
		slog.Debug("ls error", "error", err2)
	} else {
		slog.Debug("ls output", "output", out)
	}
	var mpathDevice string

	dskInfo := storagecommon.DiskInfo{
		VolumeID: fc.CS.VolProto.VolumeID,
		RootDir:  common.NodeRootDir,
	}

	// load fc disk config from json file
	slog.Debug("read fc config from staging path", "volume ID", req.GetVolumeId())
	if err := storagecommon.LoadDiskInfoFromFile(&dskInfo, stagePath); err == nil {
		mpathDevice = dskInfo.MpathDevice
		slog.Debug("fc config", "mpathDevice", mpathDevice)
	} else {
		slog.Debug("fc config not existing at staging path")
		confFile := path.Join("/host", stagePath, strconv.Itoa(fc.CS.VolProto.VolumeID)+".json")
		slog.Debug("check if fc config file exists", "path", confFile)
		pathExist, pathErr := fc.CS.PathExists(confFile)
		if pathErr == nil {
			if !pathExist {
				slog.Debug("config file not found", "path", confFile)
				if err := os.RemoveAll(stagePath); err != nil {
					_, file, line, _ := runtime.Caller(0)
					e := storagecommon.ImplementationError{
						Code: int(codes.Internal),
						Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
					}
					return nil, e
				}
				slog.Debug("removed stage path", "path", stagePath)
				return &csi.NodeUnstageVolumeResponse{}, nil
			}
		}
		slog.Warn("fc detach disk: failed to get fc config", "path", stagePath, "error", err)
	}

	// remove multipath device
	err := storagecommon.DetachMpathDevice(mpathDevice, common.ProtocolFC)
	if err != nil {
		slog.Warn("cannot detach volume", "volumeID", req.GetVolumeId(), "error", err)
	}

	if err := os.RemoveAll("/host" + stagePath); err != nil {
		_, file, line, _ := runtime.Caller(0)
		e := storagecommon.ImplementationError{
			Code: int(codes.Internal),
			Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
		}
		return nil, e
	}
	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (fc *FCstorage) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	return nil, status.Error(codes.Unimplemented, "NodeGetCapabilities (fc) - should never be called, called in node.go instead")
}

func (fc *FCstorage) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	return &csi.NodeGetInfoResponse{}, nil
}

func (fc *FCstorage) NodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	return nil, status.Error(codes.Unimplemented, time.Now().String())
}

func (fc *FCstorage) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	defer helper.TimeTrack(time.Now())
	slog.Debug("start", "volumeID", req.GetVolumeId())

	response := csi.NodeExpandVolumeResponse{}

	if req.GetVolumeCapability().GetBlock() != nil {
		err := storagecommon.BlockExpandVolume(req.GetVolumePath())
		if err != nil {
			_, file, line, _ := runtime.Caller(0)
			e := storagecommon.ImplementationError{
				Code: int(codes.Internal),
				Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
			}
			return nil, e
		}
		return &response, nil
	}

	// 1 - find the multipath device name (e.g. /dev/mapper/mpathwi) from the list of mounts
	multipathDevice, err := storagecommon.FindMultipathDeviceFromVolumePath(req.GetVolumePath())
	if err != nil {
		_, file, line, _ := runtime.Caller(0)
		e := storagecommon.ImplementationError{
			Code: int(codes.Internal),
			Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
		}
		return nil, e
	}
	slog.Debug("info", "multipathDevice", multipathDevice)

	// 2 - run multipath -l multipathDevice  to look up the particular device names (sda, sdb, sdx, ....)
	multipathDeviceBase := filepath.Base(multipathDevice)
	commandWildcards := "%m_%d_"
	command := fmt.Sprintf("multipathd show paths raw format \"%s\" | grep %s", commandWildcards, multipathDeviceBase+"_")
	slog.Debug("executing", "command", command)
	out, _, err := storagecommon.ExecCommand.Command(command, "")
	if err != nil {
		_, file, line, _ := runtime.Caller(0)
		e := storagecommon.ImplementationError{
			Code: int(codes.Internal),
			Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
		}
		return nil, e
	}

	if out == "" {
		return nil, common.Errorf("error getting multipathDevice: %s, command output was empty", multipathDevice)
	}

	output := strings.TrimSpace(out)
	slog.Debug("output", "output", output)
	outputParts := strings.Split(output, "\n")
	slog.Log(ctx, common.LevelTrace, "count", "lines", len(outputParts))

	// 3 - echo 1 > /sys/block/path_device/device/rescan  .... run those commands on each device from the previous step
	for i := range outputParts {
		if outputParts[i] != "" {
			line := strings.Split(outputParts[i], "_")
			if len(line) < 2 {
				slog.Error("error getting multipath blockDevice", "output", line)
				continue
			}
			blockDevice := line[1]
			slog.Debug("info", "device", blockDevice)
			rescanPath := fmt.Sprintf("/sys/block/%s/device/rescan", blockDevice)
			command = fmt.Sprintf("echo 1 > %s", rescanPath)
			out, _, err := storagecommon.ExecCommand.Command(command, "")
			if err != nil {
				_, file, line, _ := runtime.Caller(0)
				e := storagecommon.ImplementationError{
					Code: int(codes.Internal),
					Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
				}
				return nil, e
			}
			if out != "" {
				slog.Debug("rescan", "output", out)
			}
		}
	}

	// 4 - run multipathd resize map multipath_device - where multipath_device is like /dev/mapper/mpathwi from previous step,
	// we need to strip off the /dev/mapper/ path prefix
	mpathPart := strings.SplitAfter(multipathDevice, "/dev/mapper/")
	if len(mpathPart) < 2 {
		_, file, line, _ := runtime.Caller(0)
		e := storagecommon.ImplementationError{
			Code: int(codes.Internal),
			Msg:  fmt.Sprintf("%s:%d: %s", file, line, fmt.Sprintf("error getting mpathPart: %+v", mpathPart)),
		}
		return nil, e
	}
	command = fmt.Sprintf("multipathd resize map %s", mpathPart[1])
	out, _, err = storagecommon.ExecCommand.Command(command, "")
	if err != nil {
		_, file, line, _ := runtime.Caller(0)
		e := storagecommon.ImplementationError{
			Code: int(codes.Internal),
			Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
		}
		return nil, e
	}
	slog.Debug("multipathd resize map", "output", strings.TrimSpace(out))

	// 5 - run resize2fs or xfs_growfs on /dev/mapper/mpathwi
	fsType := req.GetVolumeCapability().GetMount().FsType
	err = storagecommon.ExpandFileSystem(multipathDevice, fsType)
	if err != nil {
		_, file, line, _ := runtime.Caller(0)
		e := storagecommon.ImplementationError{
			Code: int(codes.Internal),
			Msg:  fmt.Sprintf("%s:%d: %s", file, line, err.Error()),
		}
		return nil, e
	}

	return &response, nil
}

func (fc *FCstorage) mountFCDisk(mounter storagecommon.Mounter, devicePath string, volumeID int) error {
	defer helper.TimeTrack(time.Now())
	slog.Debug("info", "mounter", mounter, "devicePath", devicePath)

	diskInfo := storagecommon.DiskInfo{
		MpathDevice: devicePath,
		IsBlock:     mounter.IsBlock,
		VolumeID:    volumeID,
		RootDir:     common.NodeRootDir,
	}
	err := storagecommon.MountLogic(diskInfo, mounter.TargetPath, devicePath, mounter.StagePath, mounter.FsType, mounter.MountOptions, mounter.IsBlock, mounter.ReadOnly)
	if err != nil {
		return common.Errorf("%w", err)
	}

	if strings.HasPrefix(devicePath, "/dev/dm-") && !mounter.ReadOnly {
		dskinfo := storagecommon.DiskInfo{
			RootDir:     common.NodeRootDir,
			MpathDevice: devicePath,
			IsBlock:     mounter.IsBlock,
			VolumeID:    volumeID,
		}
		slog.Debug("attempting to create FC config file", "dskinfo", dskinfo, "stagePath", mounter.StagePath, "targetPath", mounter.TargetPath)
		if err := storagecommon.CreateConfigFile(dskinfo, mounter.StagePath); err != nil {
			return common.Errorf("%w", err)
		}
		slog.Debug("created FC config file", "path", mounter.StagePath)
	}
	slog.Debug("formatAndMount succeeded", "devicePath", devicePath, "targetPath", mounter.TargetPath, "fsType", mounter.FsType)
	return nil
}

func (fc *FCstorage) getFCDiskDetails(ctx context.Context, req *csi.NodePublishVolumeRequest) (*fcDevice, error) {
	lun := req.GetPublishContext()["lun"]
	wwids := req.GetVolumeContext()["WWIDs"]
	wwidList := strings.Split(wwids, ",")
	targetList := []string{}
	fcNodes, err := fc.CS.IboxAPI.GetFCPorts(ctx)
	if err != nil {
		return nil, common.Errorf("%w", err)
	}
	if len(fcNodes) == 0 {
		return nil, common.Errorf("error getting FC details, zero FC ports found")
	}
	for _, fcnode := range fcNodes {
		for _, fcport := range fcnode.Ports {
			if fcport.WWPn != "" {
				targetList = append(targetList, strings.ReplaceAll(fcport.WWPn, ":", ""))
			}
		}
	}
	slog.Debug("info", "lun", lun, "targetList", targetList, "wwidList", wwidList)
	if lun == "" || (len(targetList) == 0 && len(wwidList) == 0) {
		return nil, common.Errorf("FC target information is missing")
	}

	return &fcDevice{
		VolumeID:   fc.CS.VolProto.VolumeID,
		TargetWWNs: targetList,
		WWIDs:      wwidList,
		Lun:        lun,
	}, nil
}

func (fc *FCstorage) searchDisk(connector fcDevice) (string, error) {
	defer helper.TimeTrack(time.Now())
	slog.Debug("info", "targetWWNs", connector.TargetWWNs, "wwids", connector.WWIDs)
	var diskIDs []string // target wwns
	var disk string
	var dmDevice string

	if len(connector.TargetWWNs) != 0 {
		diskIDs = connector.TargetWWNs
	} else {
		diskIDs = connector.WWIDs
	}

	fcHosts := []string{}
	portInfo := storagecommon.GetPortInfo()
	for _, p := range portInfo {
		if p.PortState == FCPortOnline {
			fcHosts = append(fcHosts, p.HostID)
		}
	}

	slog.Debug("rescan hosts", "fcHosts", fcHosts)
	wwid, err := storagecommon.RescanDeviceMap(fcHosts, diskIDs[0], connector.Lun)
	if err != nil {
		return "", common.Errorf("%w", err)
	}
	slog.Debug("rescan scsi host", "wwid", wwid)

	const defaultTries = 10
	tries := defaultTries // currently this means a max of 10 seconds which is ample almost always
	tmp := os.Getenv(storagecommon.FCSearchDiskDelay)
	if tmp != "" {
		userSelectedValue, err := strconv.Atoi(tmp)
		if err != nil {
			slog.Error("conversion failed", "env var", storagecommon.FCSearchDiskDelay, "using default value instead", defaultTries)
		} else {
			tries = userSelectedValue
			slog.Warn("using non-default value", "env var", storagecommon.FCSearchDiskDelay, "user has specified", userSelectedValue, "default is", defaultTries)
		}
	}

	slog.Debug("sleeping to allow devmapper time to work", "seconds", tries)
	// during testing, I found that devmapper would not create the dm-X device quick enough
	// after the rescan above for the code below to work, instead of seeing a dm-X device
	// the path would be /dev/sdaX which is not what we want, sleeping a bit gives devmapper
	// time to construct the dm-X device path
	// ideally this sleep time would be configurable

	for sleepIteration := range tries {
		for _, diskID := range diskIDs {
			if len(connector.TargetWWNs) != 0 {
				dmDevice = storagecommon.GetDMDevicePath(wwid)
			}
			if dmDevice != "" {
				slog.Debug("found disk", "disk", disk, "dm", dmDevice, "diskID", diskID)
				break
			}
		}
		if dmDevice != "" && strings.Contains(dmDevice, "dm-") {
			slog.Debug("found a valid dm device", "device", dmDevice, "iteration", sleepIteration)
			break
		}
		time.Sleep(time.Second * 1)
	}

	// if no disk matches input wwn and lun, exit
	if disk == "" && dmDevice == "" {
		return "", common.Errorf("no FC disk found")
	}

	// if multipath devicemapper device is found, use it; otherwise use raw disk
	if dmDevice != "" {
		slog.Debug("dm device found", "disk", disk, "dm", dmDevice)
		return dmDevice, nil
	}
	slog.Debug("dm device not found, using raw disk", "disk", disk)
	return disk, nil
}
