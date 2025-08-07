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
package storage

import (
	"context"
	"fmt"
	"infinibox-csi-driver/common"
	"infinibox-csi-driver/helper"
	"io/fs"
	"strconv"

	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/kubernetes/pkg/volume/util"
	"k8s.io/mount-utils"
	utilexec "k8s.io/utils/exec"
)

type fcDevice struct {
	connector *Connector
	isBlock   bool
}

type FCMounter struct {
	ReadOnly     bool
	FsType       string
	MountOptions []string
	Mounter      *mount.SafeFormatAndMount
	Exec         utilexec.Interface
	DeviceUtil   util.DeviceUtil
	TargetPath   string
	StagePath    string
	fcDisk       fcDevice
}

// Global resouce contains a sync.Mutex. Used to serialize FC resource accesses.

func (fc *fcstorage) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	defer helper.TimeTrack(zlog, time.Now())
	var err error
	zlog.Debug().Msgf("NodeStageVolume (fc) called with PublishContext: volume ID: %s details: %+v", req.GetVolumeId(), req.GetPublishContext())

	hostID, ports, err := validatePublishContext(req.GetPublishContext())
	if err != nil {
		e := fmt.Errorf("NodeStageVolume (fc) - validatePublishContext - volume ID: %s error: %s", req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	fcPorts := getPortName()
	if len(fcPorts) == 0 {
		e := fmt.Errorf("NodeStageVolume (fc) - port name not found on worker")
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	fcOnline := validateFCIsOnline()
	if !fcOnline {
		e := fmt.Errorf("error - all FC ports on worker are offline")
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	zlog.Debug().Msgf("NodeStagetVolume (fc) - Publishing volume to host with host ID: %d port state: %t", hostID, fcOnline)

	for _, fcp := range fcPorts {
		zlog.Debug().Msgf("NodeStageVolume (fc) - comparing %s with %s", ports, fcp)
		if !strings.Contains(ports, fcp) {
			zlog.Debug().Msgf("NodeStageVolume (fc) - host port %s is not created, creating it", fcp)
			err = fc.cs.AddPortForHost(hostID, "FC", fcp)
			if err != nil {
				e := fmt.Errorf("NodeStageVolume (fc) - AddPortForHost - volume ID: %s error: %s", req.GetVolumeId(), err.Error())
				zlog.Error().Msg(e.Error())
				return nil, status.Error(codes.Internal, e.Error())
			}
			_, err := fc.cs.IboxApi.GetHostPort(hostID, fcp)
			if err != nil {
				e := fmt.Errorf("NodeStageVolume (fc) - GetHostPort host port %s - volume ID: %s error: %s", fcp, req.GetVolumeId(), err.Error())
				zlog.Error().Msg(e.Error())
				return nil, status.Error(codes.Internal, e.Error())
			}
		}
	}

	return &csi.NodeStageVolumeResponse{}, nil
}

func (fc *fcstorage) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	defer helper.TimeTrack(zlog, time.Now())
	var err error
	defer func() {
		if err == nil {
			zlog.Debug().Msgf("NodePublishVolume (fc) succeeded - volume ID: %s", req.GetVolumeId())
		} else {
			zlog.Debug().Msgf("NodePublishVolume (fc) failed - volume ID: %s: error: %+v", req.GetVolumeId(), err)
		}
	}()

	zlog.Debug().Msgf("NodePublishVolume (fc) volume ID: %s volumecontext %v", req.GetVolumeId(), req.GetVolumeContext())
	zlog.Debug().Msgf("NodePublishVolume (fc) uid: %s gid: %s unix_perm: %s", req.GetVolumeContext()[common.SC_UID], req.GetVolumeContext()[common.SC_GID], req.GetVolumeContext()[common.SC_UNIX_PERMISSIONS])

	fcDetails, err := fc.getFCDiskDetails(req)
	if err != nil {
		e := fmt.Errorf("NodePublishVolume (fc) - getFCDiskDetails - volume ID: %s error: %s", req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	devicePath, err := fc.searchDisk(*fcDetails.connector)
	if err != nil {
		e := fmt.Errorf("NodePublishVolume (fc) - searchDisk -  volume ID: %s error: Unable to find disk given WWNN or WWIDs: %s", req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	// remove the leading "/host" to reveal the path on the actual node host
	devicePath = strings.Replace(devicePath, "/host", "", 1)
	zlog.Debug().Msgf("FC device path %s found", devicePath)

	diskMounter, err := fc.getFCDiskMounter(req, *fcDetails)
	if err != nil {
		e := fmt.Errorf("NodePublishVolume (fc) - getFCDiskMounter - volume ID: %s error: %s", req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	err = fc.MountFCDisk(*diskMounter, devicePath)
	if err != nil {
		e := fmt.Errorf("NodePublishVolume (fc) - MountFCDisk - volume ID: %s error: %s", req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	// set volume permissions based on uid/uid/unix_permissions
	zlog.Debug().Msgf("NodePublishVolume (fc) - volume ID: %s after mount targetPath %s, devicePath %s ", req.GetVolumeId(), diskMounter.TargetPath, devicePath)
	// print out the target permissions
	logPermissions("after mount targetPath ", filepath.Dir("/host"+diskMounter.TargetPath))
	logPermissions("after mount devicePath ", "/host"+devicePath)
	if diskMounter.ReadOnly {
		zlog.Debug().Msgf("NodePublishVolume (fc) - skipping chown-chmod since this is readOnly volume")
	} else {
		err = fc.storageHelper.SetVolumePermissions(req)
		if err != nil {
			e := fmt.Errorf("NodePublishVolume (fc) - SetVolumePermissions - volume ID: %s error: %s", req.GetVolumeId(), err.Error())
			zlog.Error().Msg(e.Error())
			return nil, status.Error(codes.Internal, e.Error())
		}
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func (fc *fcstorage) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	var err error
	defer helper.TimeTrack(zlog, time.Now())

	targetPath := req.GetTargetPath()
	zlog.Debug().Msgf("NodeUnpublishVolume (fc) called - volume ID: %d targetPath: %s", fc.cs.VolProto.VolumeID, targetPath)

	err = unmountAndCleanUp(targetPath)
	if err != nil {
		e := fmt.Errorf("NodeUnpublishVolume (fc) - unmountAndCleanup  volume ID: %s target path: %s error: %s", req.GetVolumeId(), targetPath, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, e
	}
	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (fc *fcstorage) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	defer helper.TimeTrack(zlog, time.Now())
	stagePath := req.GetStagingTargetPath()
	zlog.Debug().Msgf("NodeUnstageVolume (fc) - FC called - stagePath: %s", stagePath)
	cmd := exec.Command("ls", "/host/"+stagePath)
	out, err2 := cmd.Output()

	if err2 != nil {
		zlog.Debug().Msgf("ls error: %s", err2)
	} else {
		zlog.Debug().Msgf("ls output: %s", string(out))
	}
	var mpathDevice string

	dskInfo := diskInfo{
		VolumeID: fc.cs.VolProto.VolumeID,
		RootDir:  common.NODE_ROOT_DIR,
	}

	// load fc disk config from json file
	zlog.Debug().Msgf("NodeUnstageVolume (fc) - read fc config from staging path - volume ID: %s", req.GetVolumeId())
	if err := loadDiskInfoFromFile(&dskInfo, stagePath); err == nil {
		mpathDevice = dskInfo.MpathDevice
		zlog.Debug().Msgf("NodeUnstageVolume (fc) - fc config: mpathDevice %s", mpathDevice)
	} else {
		zlog.Debug().Msgf("NodeUnstageVolume (fc) - fc config not existing at staging path")
		confFile := path.Join("/host", stagePath, strconv.Itoa(fc.cs.VolProto.VolumeID)+".json")
		zlog.Debug().Msgf("NodeUnstageVolume (fc) - check if fc config file exists")
		pathExist, pathErr := fc.cs.pathExists(confFile)
		if pathErr == nil {
			if !pathExist {
				zlog.Debug().Msgf("NodeUnstageVolume (fc) - config file not found at %s", confFile)
				if err := os.RemoveAll(stagePath); err != nil {
					e := fmt.Errorf("NodeUnstageVolume (fc) - RemoveAll - Failed to remove mount path Error: %v", err)
					zlog.Error().Msg(e.Error())
					return nil, e
				}
				zlog.Debug().Msgf("NodeUnstageVolume (fc) - Removed stage path at %s", stagePath)
				return &csi.NodeUnstageVolumeResponse{}, nil
			}
		}
		zlog.Warn().Msgf("NodeUnstageVolume (fc) - fc detach disk: failed to get fc config from path %s Error: %v", stagePath, err)
	}

	// remove multipath device
	err := detachMpathDevice(mpathDevice, common.PROTOCOL_FC)
	if err != nil {
		zlog.Error().Msgf("NodeUnstageVolume (fc) - detachMpathDevice - error: %s", err.Error())
		zlog.Warn().Msgf("NodeUnstageVolume (fc) -  cannot detach volume with ID %s: %+v", req.GetVolumeId(), err)
	}

	if err := os.RemoveAll("/host" + stagePath); err != nil {
		e := fmt.Errorf("NodeUnstageVolume (fc) - RemoveAll - failed to remove mount path error: %s", err.Error())
		zlog.Error().Msg(e.Error())
		return nil, e
	}
	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (fc *fcstorage) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	return nil, status.Error(codes.Unimplemented, "NodeGetCapabilities (fc) - should never be called, called in node.go instead")
}

func (fc *fcstorage) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	return &csi.NodeGetInfoResponse{}, nil
}

func (fc *fcstorage) NodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	return nil, status.Error(codes.Unimplemented, time.Now().String())
}

func (fc *fcstorage) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	defer helper.TimeTrack(zlog, time.Now())
	zlog.Debug().Msgf("NodeExpandVolume (fc) - called - request %s", req.GetVolumeId())

	response := csi.NodeExpandVolumeResponse{}

	if req.GetVolumeCapability().GetBlock() != nil {
		err := blockExpandVolume(req.GetVolumePath())
		if err != nil {
			e := fmt.Errorf("NodeExpandVolume (fc) - blockExpandVolume - volume path: %s error: %s", req.GetVolumePath(), err.Error())
			zlog.Error().Msg(e.Error())
			return nil, e
		}
		return &response, nil
	}

	// 1 - find the multipath device name (e.g. /dev/mapper/mpathwi) from the list of mounts
	multipathDevice, err := findMultipathDeviceFromVolumePath(req.GetVolumePath())
	if err != nil {
		e := fmt.Errorf("NodeExpandVolume (fc) - findMultipathDeviceFromVolumePath - error: %s", err.Error())
		zlog.Error().Msg(e.Error())
		return nil, e
	}
	zlog.Debug().Msgf("NodeExpandVolume (fc) - multipathDevice=[%s]", multipathDevice)

	// 2 - run multipath -l multipathDevice  to look up the particular device names (sda, sdb, sdx, ....)
	multipathDeviceBase := filepath.Base(multipathDevice)
	commandWildcards := "%m_%d_"
	command := fmt.Sprintf("multipathd show paths raw format \"%s\" | grep %s", commandWildcards, multipathDeviceBase+"_")
	zlog.Debug().Msgf("command is [%s]", command)
	out, _, err := execCommand.Command(command, "")
	if err != nil {
		e := fmt.Errorf("NodeExpandVolume (fc) - command: %s error: %s", command, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, e
	}

	if out == "" {
		e := fmt.Errorf("NodeExpandVolume (fc) - error getting multipath device name %s, command output was empty", multipathDevice)
		zlog.Error().Msg(e.Error())
		return nil, e
	}

	output := strings.TrimSpace(out)
	zlog.Debug().Msgf("output is [%s]\n", output)
	outputParts := strings.Split(output, "\n")
	zlog.Trace().Msgf("lines %d\n", len(outputParts))

	// 3 - echo 1 > /sys/block/path_device/device/rescan  .... run those commands on each device from the previous step
	for i := 0; i < len(outputParts); i++ {
		if outputParts[i] != "" {
			line := strings.Split(outputParts[i], "_")
			if len(line) < 2 {
				zlog.Error().Msgf("NodeExpandVolume (fc) - error getting multipath blockDevice from output %v", line)
				continue
			}
			blockDevice := line[1]
			zlog.Debug().Msgf("device is [%s]\n", blockDevice)
			rescanPath := fmt.Sprintf("/sys/block/%s/device/rescan", blockDevice)
			command = fmt.Sprintf("echo 1 > %s", rescanPath)
			out, _, err := execCommand.Command(command, "")
			if err != nil {
				e := fmt.Errorf("NodeExpandVolume (fc) - Command %s - error writing rescan on multipath devices %s", command, err.Error())
				zlog.Error().Msg(e.Error())
				return nil, e
			}
			if out != "" {
				zlog.Debug().Msgf("rescan output is [%s]\n", out)
			}
		}
	}

	// 4 - run multipathd resize map multipath_device - where multipath_device is like /dev/mapper/mpathwi from previous step,
	// we need to strip off the /dev/mapper/ path prefix
	mpathPart := strings.SplitAfter(multipathDevice, "/dev/mapper/")
	if len(mpathPart) < 2 {
		e := fmt.Errorf("NodeExpandVolume (fc) - error getting mpathPart from %+v", mpathPart)
		zlog.Error().Msg(e.Error())
		return nil, e
	}
	command = fmt.Sprintf("multipathd resize map %s", mpathPart[1])
	out, _, err = execCommand.Command(command, "")
	if err != nil {
		e := fmt.Errorf("NodeExpandVolume (fc) - command: %s -  error multipathd resize map multipath devices %s", command, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, e
	}
	zlog.Debug().Msgf("multipathd resize map output is [%s]", strings.TrimSpace(string(out)))

	// 5 - run resize2fs or xfs_growfs on /dev/mapper/mpathwi
	fsType := req.GetVolumeCapability().GetMount().FsType
	err = expandFileSystem(multipathDevice, fsType)
	if err != nil {
		e := fmt.Errorf("NodeExpandVolume (fc)- error: %s", err.Error())
		zlog.Error().Msg(e.Error())
		return nil, e
	}

	return &response, nil
}

func (fc *fcstorage) MountFCDisk(fm FCMounter, devicePath string) error {
	defer helper.TimeTrack(zlog, time.Now())
	zlog.Debug().Msgf("MountFCDisk - called - request %+v devicePath %s", fm, devicePath)

	dskinfo := diskInfo{
		MpathDevice: devicePath,
		IsBlock:     fm.fcDisk.isBlock,
		VolumeID:    fm.fcDisk.connector.VolumeID,
		RootDir:     common.NODE_ROOT_DIR,
	}
	err := mountLogic(dskinfo, fm.TargetPath, devicePath, fm.StagePath, fm.FsType, fm.MountOptions, fm.fcDisk.isBlock, fm.ReadOnly)
	if err != nil {
		e := fmt.Errorf("MountFCDisk - mountLogic() error %s", err.Error())
		zlog.Error().Msg(e.Error())
		return status.Error(codes.Internal, e.Error())
	}

	if strings.HasPrefix(devicePath, "/dev/dm-") && !fm.ReadOnly {
		dskinfo := diskInfo{
			RootDir:     common.NODE_ROOT_DIR,
			MpathDevice: devicePath,
			IsBlock:     fm.fcDisk.isBlock,
			VolumeID:    fm.fcDisk.connector.VolumeID,
		}
		zlog.Debug().Msgf("MountFCDisk - attempting to create FC config file dskinfo [%+v] stagePath [%s] targetPath [%s]", dskinfo, fm.StagePath, fm.TargetPath)
		if err := createConfigFile(dskinfo, fm.StagePath); err != nil {
			e := fmt.Errorf("MountFCDisk - failed to save fc config with error: %v", err)
			zlog.Error().Msg(e.Error())
			return e
		}
		zlog.Debug().Msgf("MountFCDisk - created FC config file at [%s]", fm.StagePath)
	}
	zlog.Debug().Msgf("MountFCDisk - FormatAndMount succeeded. devicePath: %s, targetPath: %s, fsType: %s", devicePath, fm.TargetPath, fm.FsType)
	return nil
}

func getPortName() []string {
	ports := []string{}
	cmd := "cat /sys/class/fc_host/host*/port_name"
	out, err := exec.Command("bash", "-c", cmd).Output()
	if err != nil {
		zlog.Error().Msgf("Failed to get port name using command '%s': %v", cmd, err)
		return ports
	}
	portName := string(out)
	if portName != "" {
		for _, port := range strings.Split(strings.TrimSuffix(portName, "\n"), "\n") {
			ports = append(ports, strings.Replace(port, "0x", "", 1))
		}
	}
	zlog.Debug().Msgf("fc ports found %v ", ports)
	return ports
}

// validateFCIsOnline returns true if a FC port is found to be Online
//
// the check is performed by looking for FC port_state files that might exist
// and returns true if it finds one that is Online
//
// NOTE:  multipath should work even with a single Online port,
// other ports (if any) can be down
func validateFCIsOnline() bool {

	const FC_ONLINE = "Online"

	// we append /host because this code works against the mounted host directly
	// instead of via a chroot command
	const fcHostPath = "/host/sys/class/fc_host"

	// example for a FC port_state path is:
	// /host/sys/class/fc_host/host33/port_state

	entries, err := os.ReadDir(fcHostPath)
	if err != nil {
		zlog.Error().Msgf("error reading fc directory: %s", err)
		return false
	}

	zlog.Debug().Msgf("searching for FC port_state in %s:", fcHostPath)
	for _, entry := range entries {
		filePath := fcHostPath + "/" + entry.Name() + "/port_state"
		zlog.Debug().Msgf("reading - %s ", filePath)
		portStateBytes, err := os.ReadFile(filePath)
		if err != nil {
			if os.IsNotExist(err) {
				zlog.Debug().Msgf("file not exist %s", err.Error())
			} else {
				zlog.Error().Msgf("error reading port_state file %s", err.Error())
			}
		} else {
			portState := strings.TrimSpace(string(portStateBytes))
			if portState == FC_ONLINE {
				zlog.Debug().Msgf("%s/port_state %s is %s", entry.Name(), portState, FC_ONLINE)
				return true
			} else {
				zlog.Debug().Msgf("%s/port_state %s is NOT %s", entry.Name(), portState, FC_ONLINE)
			}
		}
	}
	return false
}

func (fc *fcstorage) getFCDiskDetails(req *csi.NodePublishVolumeRequest) (*fcDevice, error) {
	lun := req.GetPublishContext()["lun"]
	wwids := req.GetVolumeContext()["WWIDs"]
	wwidList := strings.Split(wwids, ",")
	targetList := []string{}
	fcNodes, err := fc.cs.IboxApi.GetFCPorts()
	if err != nil {
		e := fmt.Errorf("getFCDiskDetails - error getting fc details - error %s", err.Error())
		zlog.Error().Msg(e.Error())
		return nil, e
	}
	if len(fcNodes) == 0 {
		return nil, fmt.Errorf("getFCDiskDetails - error getting fiber channel details, zero fc ports found")
	}
	for _, fcnode := range fcNodes {
		for _, fcport := range fcnode.Ports {
			if fcport.WWPn != "" {
				targetList = append(targetList, strings.ReplaceAll(fcport.WWPn, ":", ""))
			}
		}
	}
	zlog.Debug().Msgf("lun %s , targetList %v , wwidList %v", lun, targetList, wwidList)
	if lun == "" || (len(targetList) == 0 && len(wwidList) == 0) {
		e := fmt.Errorf("getFCDiskDetails - FC target information is missing")
		zlog.Error().Msg(e.Error())
		return nil, e
	}
	fcConnector := &Connector{
		VolumeID:   fc.cs.VolProto.VolumeID,
		TargetWWNs: targetList,
		WWIDs:      wwidList,
		Lun:        lun,
	}

	return &fcDevice{
		connector: fcConnector,
	}, nil
}

func (fc *fcstorage) getFCDiskMounter(req *csi.NodePublishVolumeRequest, fcDetails fcDevice) (*FCMounter, error) {
	reqVolCapability := req.GetVolumeCapability()

	// check accessMode - where we will eventually police R/W etc (CSIC-343)
	accessMode := reqVolCapability.GetAccessMode().GetMode() // GetAccessMode() guaranteed not nil from controller.go
	// TODO: set readonly flag for RO accessmodes, any other validations needed?

	// handle file (mount) and block parameters
	mountVolCapability := reqVolCapability.GetMount()
	var fstype string
	mountOptions := []string{}
	blockVolCapability := reqVolCapability.GetBlock()

	readOnly := false
	if req.Readonly || accessMode == csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY {
		readOnly = true
		zlog.Debug().Msg("MULTI_NODE_READER_ONLY AccessMode requested")
	}

	// protocol-specific paths below
	if mountVolCapability != nil && blockVolCapability == nil {
		// option A. user wants file access to their FC device
		fcDetails.isBlock = false

		fstype = mountVolCapability.GetFsType()

		// mountOptions - could be nil
		mountOptions = mountVolCapability.GetMountFlags()

		// TODO: other validations needed for file?
		// - something about read-only access?
		// - check that fstype is supported?
		// - check that mount options are valid for fstype provided

	} else if mountVolCapability == nil && blockVolCapability != nil {
		// option B. user wants block access to their FC device
		fcDetails.isBlock = true

		if accessMode == csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER {
			zlog.Warn().Msg("MULTI_NODE_MULTI_WRITER AccessMode requested for raw block volume, could be dangerous")
		}
		// TODO: something about SINGLE_NODE_MULTI_WRITER (alpha feature) as well?

		// don't need to look at FsType or MountFlags here, only relevant for mountVol
		// TODO: other validations needed for block?
		// - something about read-only access?
	} else {
		errMsg := "Bad VolumeCapability parameters: both block and mount modes, for volume: " + req.GetVolumeId()
		zlog.Error().Msg(errMsg)
		return nil, status.Error(codes.InvalidArgument, errMsg)
	}

	return &FCMounter{
		fcDisk:       fcDetails,
		ReadOnly:     readOnly,
		FsType:       fstype,
		MountOptions: mountOptions,
		Mounter:      &mount.SafeFormatAndMount{Interface: mount.NewWithoutSystemd(""), Exec: utilexec.New()},
		Exec:         utilexec.New(),
		DeviceUtil:   util.NewDeviceHandler(util.NewIOHandler()),
		TargetPath:   req.GetTargetPath(),
		StagePath:    req.GetStagingTargetPath(),
	}, nil
}

// Connector provides a struct to hold all of the needed parameters to make our Fibre Channel connection
type Connector struct {
	VolumeID   int
	TargetWWNs []string
	Lun        string
	WWIDs      []string
}

// OSioHandler is a wrapper that includes all the necessary io functions used for (Should be used as default io handler)
type OSioHandler struct{}

// ReadDir calls the ReadDir function from ioutil package
func (handler *OSioHandler) ReadDir(dirname string) (infos []os.FileInfo, err error) {
	entries, err := os.ReadDir(dirname)
	if err != nil {
		e := fmt.Errorf("ReadDir error %s", err.Error())
		zlog.Error().Msg(e.Error())
		return infos, e
	}
	infos = make([]fs.FileInfo, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			e := fmt.Errorf("ReadDir error %s", err.Error())
			zlog.Error().Msg(e.Error())
			return infos, e

		}
		infos = append(infos, info)
	}
	return infos, nil
}

// Lstat calls the Lstat function from os package
func (handler *OSioHandler) Lstat(name string) (os.FileInfo, error) {
	return os.Lstat(name)
}

// EvalSymlinks calls EvalSymlinks from filepath package
func (handler *OSioHandler) EvalSymlinks(path string) (string, error) {
	return filepath.EvalSymlinks(path)
}

// WriteFile calls WriteFile from ioutil package
func (handler *OSioHandler) WriteFile(filename string, data []byte, perm os.FileMode) error {
	return os.WriteFile(filename, data, perm)
}

func (fc *fcstorage) searchDisk(c Connector) (string, error) {
	defer helper.TimeTrack(zlog, time.Now())
	zlog.Debug().Msgf("searchDisk - targetWWNs=[%+v] wwids=[%v]", c.TargetWWNs, c.WWIDs)
	var diskIds []string // target wwns
	var disk string
	var dm string

	if len(c.TargetWWNs) != 0 {
		diskIds = c.TargetWWNs
	} else {
		diskIds = c.WWIDs
	}

	fcHosts, err := findHosts("fc")
	if err != nil {
		e := fmt.Errorf("searchDisk - findHosts - error %s", err.Error())
		zlog.Error().Msg(e.Error())
		return "", e
	}

	zlog.Debug().Msgf("Rescan hosts fcHosts [%v]", fcHosts)
	wwid, err := rescanDeviceMap(fcHosts, diskIds[0], c.Lun)
	if err != nil {
		e := fmt.Errorf("searchDisk rescan error %s", err.Error())
		zlog.Error().Msg(e.Error())
		return "", e
	}
	if wwid == "" {
		e := fmt.Errorf("searchDisk rescan error wwid not found")
		zlog.Error().Msg(e.Error())
		return "", e
	}
	zlog.Debug().Msgf("searchDisk rescan scsi host wwid is [%s]", wwid)

	const defaultTries = 10
	tries := defaultTries //currently this means a max of 10 seconds which is ample almost always
	tmp := os.Getenv(FC_SEARCH_DISK_DELAY)
	if tmp != "" {
		userSelectedValue, err := strconv.Atoi(tmp)
		if err != nil {
			zlog.Error().Msgf("conversion of %s env var failed, using default value of %d instead", FC_SEARCH_DISK_DELAY, defaultTries)
		} else {
			tries = userSelectedValue
			zlog.Warn().Msgf("using non-default value for %s env var, user has specified %d, default is %d", FC_SEARCH_DISK_DELAY, userSelectedValue, defaultTries)
		}
	}

	zlog.Debug().Msgf("searchDisk sleeping up to %d seconds to allow devmapper time to work", tries)
	// during testing, I found that devmapper would not create the dm-X device quick enough
	// after the rescan above for the code below to work, instead of seeing a dm-X device
	// the path would be /dev/sdaX whic is not what we want, sleeping a bit gives devmapper
	// time to construct the dm-X device path
	// ideally this sleep time would be configurable

	for i := 0; i < tries; i++ {
		for _, diskID := range diskIds {
			if len(c.TargetWWNs) != 0 {
				dm = getDMDevicePath(wwid)
			}
			if dm != "" {
				zlog.Debug().Msgf("searchDisk found disk '%s' and dm '%s' diskID '%s'", disk, dm, diskID)
				break
			}
		}
		if dm != "" && strings.Contains(dm, "dm-") {
			zlog.Debug().Msgf("searchDisk found a valid dm device [%s] iteration %d", dm, i)
			break
		}
		time.Sleep(time.Second * 1)
	}

	// if no disk matches input wwn and lun, exit
	if disk == "" && dm == "" {
		return "", fmt.Errorf("no fc disk found")
	}

	// if multipath devicemapper device is found, use it; otherwise use raw disk
	if dm != "" {
		zlog.Debug().Msgf("searchDisk dm device found disk [%s] dm [%s]", disk, dm)
		return dm, nil
	}
	zlog.Debug().Msgf("searchDisk dm device not found, using raw disk %s", disk)
	return disk, nil
}
