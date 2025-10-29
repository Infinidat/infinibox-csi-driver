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
	"fmt"
	"strconv"

	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/infinidat/infinibox-csi-driver/helper"
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
	"k8s.io/kubernetes/pkg/volume/util"
	"k8s.io/mount-utils"
	utilexec "k8s.io/utils/exec"
)

type fcDevice struct {
	connector *Connector
	isBlock   bool
}

type Mounter struct {
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

const FCPortOnline = "Online"

func (fc *FCstorage) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	defer helper.TimeTrack(zlog, time.Now())
	var err error
	const functionName = "NodeStageVolume"
	zlog.Debug().Msgf("%s (fc) called with PublishContext: volume ID: %s details: %+v %s", functionName, req.GetVolumeId(), req.GetPublishContext(), storagecommon.GetHostInfo(ctx, req.GetSecrets(), fc.CS.IboxAPI))

	hostID, ports, err := storagecommon.ValidatePublishContext(req.GetPublishContext())
	if err != nil {
		e := fmt.Errorf("%s (fc) - validatePublishContext - volume ID: %s error: %s", functionName, req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	portInfo := storagecommon.GetPortInfo()
	fcPorts := []string{}
	for _, p := range portInfo {
		fcPorts = append(fcPorts, p.PortName)
	}

	if len(fcPorts) == 0 {
		e := fmt.Errorf("%s (fc) - port name not found on worker", functionName)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	zlog.Debug().Msgf("%s (fc) getPortName output %v", functionName, fcPorts)

	var fcOnline bool
	for _, p := range portInfo {
		if p.PortState == FCPortOnline {
			fcOnline = true
		}
	}

	if !fcOnline {
		e := fmt.Errorf("%s (fc) - error - all FC ports on worker are offline", functionName)
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	zlog.Debug().Msgf("%s (fc) - Publishing volume to host with host ID: %d port state: %t", functionName, hostID, fcOnline)

	for _, fcp := range fcPorts {
		zlog.Debug().Msgf("%s (fc) - comparing %s with %s", functionName, ports, fcp)
		if !strings.Contains(ports, fcp) {
			zlog.Debug().Msgf("%s (fc) - host port %s is not created, creating it", functionName, fcp)
			err = fc.CS.AddPortForHost(ctx, hostID, "FC", fcp)
			if err != nil {
				e := fmt.Errorf("%s (fc) - AddPortForHost - volume ID: %s error: %s", functionName, req.GetVolumeId(), err.Error())
				zlog.Error().Msg(e.Error())
				return nil, status.Error(codes.Internal, e.Error())
			}
			_, err := fc.CS.IboxAPI.GetHostPort(ctx, hostID, fcp)
			if err != nil {
				e := fmt.Errorf("%s (fc) - GetHostPort host port %s - volume ID: %s error: %s", functionName, fcp, req.GetVolumeId(), err.Error())
				zlog.Error().Msg(e.Error())
				return nil, status.Error(codes.Internal, e.Error())
			}
		}
	}

	return &csi.NodeStageVolumeResponse{}, nil
}

func (fc *FCstorage) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	defer helper.TimeTrack(zlog, time.Now())
	var err error
	const functionName = "NodePublishVolume"
	defer func() {
		if err == nil {
			zlog.Debug().Msgf("%s (fc) succeeded - volume ID: %s", functionName, req.GetVolumeId())
		} else {
			zlog.Debug().Msgf("%s (fc) failed - volume ID: %s: error: %+v", functionName, req.GetVolumeId(), err)
		}
	}()

	zlog.Debug().Msgf("%s (fc) volume ID: %s volumecontext %v %s", functionName, req.GetVolumeId(), req.GetVolumeContext(),
		storagecommon.GetHostInfo(ctx, req.GetSecrets(), fc.CS.IboxAPI))
	zlog.Debug().Msgf("%s (fc) uid: %s gid: %s unix_perm: %s", functionName, req.GetVolumeContext()[common.StorageClassUID], req.GetVolumeContext()[common.StorageClassGID], req.GetVolumeContext()[common.StorageClassUNIXPermissions])

	fcDetails, err := fc.getFCDiskDetails(ctx, req)
	if err != nil {
		e := fmt.Errorf("%s (fc) - getFCDiskDetails - volume ID: %s error: %s", functionName, req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	devicePath, err := fc.searchDisk(*fcDetails.connector)
	if err != nil {
		e := fmt.Errorf("%s (fc) - searchDisk -  volume ID: %s error: Unable to find disk given WWNN or WWIDs: %s", functionName, req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	// remove the leading "/host" to reveal the path on the actual node host
	devicePath = strings.Replace(devicePath, "/host", "", 1)
	zlog.Debug().Msgf("FC device path %s found", devicePath)

	diskMounter, err := fc.getFCDiskMounter(req, *fcDetails)
	if err != nil {
		e := fmt.Errorf("%s (fc) - getFCDiskMounter - volume ID: %s error: %s", functionName, req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	err = fc.MountFCDisk(*diskMounter, devicePath)
	if err != nil {
		e := fmt.Errorf("%s (fc) - MountFCDisk - volume ID: %s error: %s", functionName, req.GetVolumeId(), err.Error())
		zlog.Error().Msg(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	// set volume permissions based on uid/uid/unix_permissions
	zlog.Debug().Msgf("%s (fc) - volume ID: %s after mount targetPath %s, devicePath %s ", functionName, req.GetVolumeId(), diskMounter.TargetPath, devicePath)
	// print out the target permissions
	storagecommon.LogPermissions("after mount targetPath ", filepath.Dir("/host"+diskMounter.TargetPath))
	storagecommon.LogPermissions("after mount devicePath ", "/host"+devicePath)
	if diskMounter.ReadOnly {
		zlog.Debug().Msgf("%s (fc) - skipping chown-chmod since this is readOnly volume", functionName)
	} else {
		err = fc.StorageHelper.SetVolumePermissions(req)
		if err != nil {
			e := fmt.Errorf("%s (fc) - SetVolumePermissions - volume ID: %s error: %s", functionName, req.GetVolumeId(), err.Error())
			zlog.Error().Msg(e.Error())
			return nil, status.Error(codes.Internal, e.Error())
		}
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func (fc *FCstorage) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	var err error
	const functionName = "NodeUnpublishVolume"
	defer helper.TimeTrack(zlog, time.Now())

	targetPath := req.GetTargetPath()
	zlog.Debug().Msgf("%s (fc) called - volume ID: %d targetPath: %s", functionName, fc.CS.VolProto.VolumeID, targetPath)

	err = storagecommon.UnmountAndCleanUp(targetPath)
	if err != nil {
		e := fmt.Errorf("%s (fc) - unmountAndCleanup  volume ID: %s target path: %s error: %s", functionName, req.GetVolumeId(), targetPath, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, e
	}
	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (fc *FCstorage) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	const functionName = "NodeUnstageVolume"
	defer helper.TimeTrack(zlog, time.Now())
	stagePath := req.GetStagingTargetPath()
	zlog.Debug().Msgf("%s (fc) - FC called - stagePath: %s", functionName, stagePath)
	cmd := exec.Command("ls", "/host/"+stagePath)
	out, err2 := cmd.Output()

	if err2 != nil {
		zlog.Debug().Msgf("ls error: %s", err2)
	} else {
		zlog.Debug().Msgf("ls output: %s", out)
	}
	var mpathDevice string

	dskInfo := storagecommon.DiskInfo{
		VolumeID: fc.CS.VolProto.VolumeID,
		RootDir:  common.NodeRootDir,
	}

	// load fc disk config from json file
	zlog.Debug().Msgf("%s (fc) - read fc config from staging path - volume ID: %s", functionName, req.GetVolumeId())
	if err := storagecommon.LoadDiskInfoFromFile(&dskInfo, stagePath); err == nil {
		mpathDevice = dskInfo.MpathDevice
		zlog.Debug().Msgf("%s (fc) - fc config: mpathDevice %s", functionName, mpathDevice)
	} else {
		zlog.Debug().Msgf("%s (fc) - fc config not existing at staging path", functionName)
		confFile := path.Join("/host", stagePath, strconv.Itoa(fc.CS.VolProto.VolumeID)+".json")
		zlog.Debug().Msgf("%s (fc) - check if fc config file exists", functionName)
		pathExist, pathErr := fc.CS.PathExists(confFile)
		if pathErr == nil {
			if !pathExist {
				zlog.Debug().Msgf("%s (fc) - config file not found at %s", functionName, confFile)
				if err := os.RemoveAll(stagePath); err != nil {
					e := fmt.Errorf("%s (fc) - RemoveAll - Failed to remove mount path Error: %v", functionName, err)
					zlog.Error().Msg(e.Error())
					return nil, e
				}
				zlog.Debug().Msgf("%s (fc) - Removed stage path at %s", functionName, stagePath)
				return &csi.NodeUnstageVolumeResponse{}, nil
			}
		}
		zlog.Warn().Msgf("%s (fc) - fc detach disk: failed to get fc config from path %s Error: %v", functionName, stagePath, err)
	}

	// remove multipath device
	err := storagecommon.DetachMpathDevice(mpathDevice, common.ProtocolFC)
	if err != nil {
		zlog.Error().Msgf("%s (fc) - detachMpathDevice - error: %s", functionName, err.Error())
		zlog.Warn().Msgf("%s (fc) -  cannot detach volume with ID %s: %+v", functionName, req.GetVolumeId(), err)
	}

	if err := os.RemoveAll("/host" + stagePath); err != nil {
		e := fmt.Errorf("%s (fc) - RemoveAll - failed to remove mount path error: %s", functionName, err.Error())
		zlog.Error().Msg(e.Error())
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
	const functionName = "NodeExpandVolume"
	defer helper.TimeTrack(zlog, time.Now())
	zlog.Debug().Msgf("%s (fc) - called - request %s", functionName, req.GetVolumeId())

	response := csi.NodeExpandVolumeResponse{}

	if req.GetVolumeCapability().GetBlock() != nil {
		err := storagecommon.BlockExpandVolume(req.GetVolumePath())
		if err != nil {
			e := fmt.Errorf("%s (fc) - blockExpandVolume - volume path: %s error: %s", functionName, req.GetVolumePath(), err.Error())
			zlog.Error().Msg(e.Error())
			return nil, e
		}
		return &response, nil
	}

	// 1 - find the multipath device name (e.g. /dev/mapper/mpathwi) from the list of mounts
	multipathDevice, err := storagecommon.FindMultipathDeviceFromVolumePath(req.GetVolumePath())
	if err != nil {
		e := fmt.Errorf("%s (fc) - findMultipathDeviceFromVolumePath - error: %s", functionName, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, e
	}
	zlog.Debug().Msgf("%s (fc) - multipathDevice=[%s]", functionName, multipathDevice)

	// 2 - run multipath -l multipathDevice  to look up the particular device names (sda, sdb, sdx, ....)
	multipathDeviceBase := filepath.Base(multipathDevice)
	commandWildcards := "%m_%d_"
	command := fmt.Sprintf("multipathd show paths raw format \"%s\" | grep %s", commandWildcards, multipathDeviceBase+"_")
	zlog.Debug().Msgf("command is [%s]", command)
	out, _, err := storagecommon.ExecCommand.Command(command, "")
	if err != nil {
		e := fmt.Errorf("%s (fc) - command: %s error: %s", functionName, command, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, e
	}

	if out == "" {
		e := fmt.Errorf("%s (fc) - error getting multipath device name %s, command output was empty", functionName, multipathDevice)
		zlog.Error().Msg(e.Error())
		return nil, e
	}

	output := strings.TrimSpace(out)
	zlog.Debug().Msgf("output is [%s]\n", output)
	outputParts := strings.Split(output, "\n")
	zlog.Trace().Msgf("lines %d\n", len(outputParts))

	// 3 - echo 1 > /sys/block/path_device/device/rescan  .... run those commands on each device from the previous step
	for i := range outputParts {
		if outputParts[i] != "" {
			line := strings.Split(outputParts[i], "_")
			if len(line) < 2 {
				zlog.Error().Msgf("%s (fc) - error getting multipath blockDevice from output %v", functionName, line)
				continue
			}
			blockDevice := line[1]
			zlog.Debug().Msgf("device is [%s]\n", blockDevice)
			rescanPath := fmt.Sprintf("/sys/block/%s/device/rescan", blockDevice)
			command = fmt.Sprintf("echo 1 > %s", rescanPath)
			out, _, err := storagecommon.ExecCommand.Command(command, "")
			if err != nil {
				e := fmt.Errorf("%s (fc) - Command %s - error writing rescan on multipath devices %s", functionName, command, err.Error())
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
		e := fmt.Errorf("%s (fc) - error getting mpathPart from %+v", functionName, mpathPart)
		zlog.Error().Msg(e.Error())
		return nil, e
	}
	command = fmt.Sprintf("multipathd resize map %s", mpathPart[1])
	out, _, err = storagecommon.ExecCommand.Command(command, "")
	if err != nil {
		e := fmt.Errorf("%s (fc) - command: %s -  error multipathd resize map multipath devices %s", functionName, command, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, e
	}
	zlog.Debug().Msgf("multipathd resize map output is [%s]", strings.TrimSpace(out))

	// 5 - run resize2fs or xfs_growfs on /dev/mapper/mpathwi
	fsType := req.GetVolumeCapability().GetMount().FsType
	err = storagecommon.ExpandFileSystem(multipathDevice, fsType)
	if err != nil {
		e := fmt.Errorf("%s (fc)- error: %s", functionName, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, e
	}

	return &response, nil
}

func (fc *FCstorage) MountFCDisk(mounter Mounter, devicePath string) error {
	const functionName = "MountFCDisk"
	defer helper.TimeTrack(zlog, time.Now())
	zlog.Debug().Msgf("%s - called - request %+v devicePath %s", functionName, mounter, devicePath)

	diskInfo := storagecommon.DiskInfo{
		MpathDevice: devicePath,
		IsBlock:     mounter.fcDisk.isBlock,
		VolumeID:    mounter.fcDisk.connector.VolumeID,
		RootDir:     common.NodeRootDir,
	}
	err := storagecommon.MountLogic(diskInfo, mounter.TargetPath, devicePath, mounter.StagePath, mounter.FsType, mounter.MountOptions, mounter.fcDisk.isBlock, mounter.ReadOnly)
	if err != nil {
		e := fmt.Errorf("%s - mountLogic() error %s", functionName, err.Error())
		zlog.Error().Msg(e.Error())
		return status.Error(codes.Internal, e.Error())
	}

	if strings.HasPrefix(devicePath, "/dev/dm-") && !mounter.ReadOnly {
		dskinfo := storagecommon.DiskInfo{
			RootDir:     common.NodeRootDir,
			MpathDevice: devicePath,
			IsBlock:     mounter.fcDisk.isBlock,
			VolumeID:    mounter.fcDisk.connector.VolumeID,
		}
		zlog.Debug().Msgf("%s - attempting to create FC config file dskinfo [%+v] stagePath [%s] targetPath [%s]", functionName, dskinfo, mounter.StagePath, mounter.TargetPath)
		if err := storagecommon.CreateConfigFile(dskinfo, mounter.StagePath); err != nil {
			e := fmt.Errorf("%s - failed to save fc config with error: %v", functionName, err)
			zlog.Error().Msg(e.Error())
			return e
		}
		zlog.Debug().Msgf("%s - created FC config file at [%s]", functionName, mounter.StagePath)
	}
	zlog.Debug().Msgf("%s - FormatAndMount succeeded. devicePath: %s, targetPath: %s, fsType: %s", functionName, devicePath, mounter.TargetPath, mounter.FsType)
	return nil
}

func (fc *FCstorage) getFCDiskDetails(ctx context.Context, req *csi.NodePublishVolumeRequest) (*fcDevice, error) {
	const functionName = "getFCDiskDetails"
	lun := req.GetPublishContext()["lun"]
	wwids := req.GetVolumeContext()["WWIDs"]
	wwidList := strings.Split(wwids, ",")
	targetList := []string{}
	fcNodes, err := fc.CS.IboxAPI.GetFCPorts(ctx)
	if err != nil {
		e := fmt.Errorf("%s - error getting fc details - error %s", functionName, err.Error())
		zlog.Error().Msg(e.Error())
		return nil, e
	}
	if len(fcNodes) == 0 {
		return nil, fmt.Errorf("%s - error getting fiber channel details, zero fc ports found", functionName)
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
		e := fmt.Errorf("%s - FC target information is missing", functionName)
		zlog.Error().Msg(e.Error())
		return nil, e
	}
	fcConnector := &Connector{
		VolumeID:   fc.CS.VolProto.VolumeID,
		TargetWWNs: targetList,
		WWIDs:      wwidList,
		Lun:        lun,
	}

	return &fcDevice{
		connector: fcConnector,
	}, nil
}

func (fc *FCstorage) getFCDiskMounter(req *csi.NodePublishVolumeRequest, fcDetails fcDevice) (*Mounter, error) {
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

	return &Mounter{
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

func (fc *FCstorage) searchDisk(connector Connector) (string, error) {
	const functionName = "searchDisk"
	defer helper.TimeTrack(zlog, time.Now())
	zlog.Debug().Msgf("%s - targetWWNs=[%+v] wwids=[%v]", functionName, connector.TargetWWNs, connector.WWIDs)
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

	zlog.Debug().Msgf("Rescan hosts fcHosts [%v]", fcHosts)
	wwid, err := storagecommon.RescanDeviceMap(fcHosts, diskIDs[0], connector.Lun)
	if err != nil {
		e := fmt.Errorf("%s rescan error %s", functionName, err.Error())
		zlog.Error().Msg(e.Error())
		return "", e
	}
	if wwid == "" {
		e := fmt.Errorf("%s rescan error wwid not found", functionName)
		zlog.Error().Msg(e.Error())
		return "", e
	}
	zlog.Debug().Msgf("%s rescan scsi host wwid is [%s]", functionName, wwid)

	const defaultTries = 10
	tries := defaultTries // currently this means a max of 10 seconds which is ample almost always
	tmp := os.Getenv(storagecommon.FCSearchDiskDelay)
	if tmp != "" {
		userSelectedValue, err := strconv.Atoi(tmp)
		if err != nil {
			zlog.Error().Msgf("conversion of %s env var failed, using default value of %d instead", storagecommon.FCSearchDiskDelay, defaultTries)
		} else {
			tries = userSelectedValue
			zlog.Warn().Msgf("using non-default value for %s env var, user has specified %d, default is %d", storagecommon.FCSearchDiskDelay, userSelectedValue, defaultTries)
		}
	}

	zlog.Debug().Msgf("%s sleeping up to %d seconds to allow devmapper time to work", functionName, tries)
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
				zlog.Debug().Msgf("%s found disk '%s' and dm '%s' diskID '%s'", functionName, disk, dmDevice, diskID)
				break
			}
		}
		if dmDevice != "" && strings.Contains(dmDevice, "dm-") {
			zlog.Debug().Msgf("%s found a valid dm device [%s] iteration %d", functionName, dmDevice, sleepIteration)
			break
		}
		time.Sleep(time.Second * 1)
	}

	// if no disk matches input wwn and lun, exit
	if disk == "" && dmDevice == "" {
		return "", fmt.Errorf("no fc disk found")
	}

	// if multipath devicemapper device is found, use it; otherwise use raw disk
	if dmDevice != "" {
		zlog.Debug().Msgf("%s dm device found disk [%s] dm [%s]", functionName, disk, dmDevice)
		return dmDevice, nil
	}
	zlog.Debug().Msgf("%s dm device not found, using raw disk %s", functionName, disk)
	return disk, nil
}
