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
	"encoding/json"
	"errors"
	"fmt"
	"infinibox-csi-driver/common"
	"infinibox-csi-driver/helper"
	"io/fs"

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

type fcDevice struct {
	connector *Connector
	isBlock   bool
}

type diskInfo struct {
	MpathDevice string
	IsBlock     bool
	VolName     string
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
var execFc helper.ExecScsi

func (fc *fcstorage) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	var err error
	zlog.Debug().Msgf("NodeStageVolume called with PublishContext: %+v", req.GetPublishContext())
	hostIdString := req.GetPublishContext()["hostID"]
	ports := req.GetPublishContext()["hostPorts"]

	hostId, err := strconv.Atoi(hostIdString)
	if err != nil {
		zlog.Err(err)
		err := fmt.Errorf("hostID string '%s' is not valid host ID: %s", hostIdString, err)
		zlog.Error().Msgf(err.Error())
		return nil, status.Error(codes.Internal, err.Error())
	}

	zlog.Debug().Msgf("Publishing volume to host with host ID %d", hostId)
	// validate host exists
	if hostId < 1 {
		err := fmt.Errorf("hostId %d is not valid host Id", hostId)
		zlog.Error().Msgf(err.Error())
		return nil, status.Error(codes.Internal, err.Error())
	}

	fcPorts := getPortName()
	if len(fcPorts) == 0 {
		zlog.Error().Msgf("port name not found on worker")
		return nil, status.Error(codes.Internal, "Port name not found")
	}
	for _, fcp := range fcPorts {
		if !strings.Contains(ports, fcp) {
			zlog.Debug().Msgf("host port %s is not created, creating it", fcp)
			err = fc.cs.AddPortForHost(hostId, "FC", fcp)
			if err != nil {
				zlog.Error().Msgf("error creating host port %v", err)
				return nil, status.Error(codes.Internal, err.Error())
			}
			_, err := fc.cs.Api.GetHostPort(hostId, fcp)
			if err != nil {
				zlog.Error().Msgf("failed to get host port %s with error %v", fcp, err)
				return nil, status.Error(codes.Internal, err.Error())
			}
		}
	}

	return &csi.NodeStageVolumeResponse{}, nil
}

func (fc *fcstorage) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	var err error
	defer func() {
		if err == nil {
			zlog.Debug().Msgf("NodePublishVolume succeeded with volume ID %s", req.GetVolumeId())
		} else {
			zlog.Debug().Msgf("NodePublishVolume failed with volume ID %s: %+v", req.GetVolumeId(), err)
		}
	}()

	zlog.Debug().Msgf("NodePublishVolume volumecontext %v", req.GetVolumeContext())
	zlog.Debug().Msgf("uid %s gid %s unix_perm %s", req.GetVolumeContext()[common.SC_UID], req.GetVolumeContext()[common.SC_GID], req.GetVolumeContext()[common.SC_UNIX_PERMISSIONS])
	zlog.Debug().Msgf("NodePublishVolume called with volume ID %s", req.GetVolumeId())

	fcDetails, err := fc.getFCDiskDetails(req)
	if err != nil {
		zlog.Err(err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	devicePath, err := fc.searchDisk(*fcDetails.connector, &OSioHandler{})
	if err != nil {
		zlog.Error().Msgf("fc.searchDisk() failed. Unable to find disk given WWNN or WWIDs: %+v", err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	// remove the leading "/host" to reveal the path on the actual node host
	devicePath = strings.Replace(devicePath, "/host", "", 1)
	zlog.Debug().Msgf("FC device path %s found", devicePath)

	diskMounter, err := fc.getFCDiskMounter(req, *fcDetails)
	if err != nil {
		zlog.Err(err)
		return nil, err
	}

	err = fc.MountFCDisk(*diskMounter, devicePath)
	if err != nil {
		zlog.Err(err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	// set volume permissions based on uid/uid/unix_permissions
	zlog.Debug().Msgf("after mount targetPath %s, devicePath %s ", diskMounter.TargetPath, devicePath)
	// print out the target permissions
	logPermissions("after mount targetPath ", filepath.Dir("/host"+diskMounter.TargetPath))
	logPermissions("after mount devicePath ", "/host"+devicePath)
	err = fc.storageHelper.SetVolumePermissions(req)
	if err != nil {
		zlog.Error().Msgf("error in setting volume permissions %s on volume %s\n", err.Error(), req.GetVolumeId())
		return nil, err
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func (fc *fcstorage) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	var err error
	/**
	defer func() {
		if err == nil {
			zlog.Debug().Msgf("NodeUnpublishVolume succeeded with volume ID %s", req.GetVolumeId())
		} else {
			zlog.Debug().Msgf("NodeUnpublishVolume failed with volume ID %s: %+v", req.GetVolumeId(), err)
		}
	}()
	*/

	zlog.Debug().Msgf("NodeUnpublishVolume called with volume ID %s", req.GetVolumeId())

	targetPath := req.GetTargetPath()
	err = unmountAndCleanUp(targetPath)
	if err != nil {
		zlog.Err(err)
		return nil, err
	}
	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (fc *fcstorage) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	zlog.Debug().Msgf("Called FC NodeUnstageVolume")
	var mpathDevice string
	stagePath := req.GetStagingTargetPath()

	volproto := strings.Split(req.GetVolumeId(), "$$")
	volName := volproto[0]

	dskInfo := diskInfo{}
	dskInfo.VolName = volName

	// load fc disk config from json file
	zlog.Debug().Msgf("read fc config from staging path")
	if err := fc.loadFcDiskInfoFromFile(&dskInfo, stagePath); err == nil {
		mpathDevice = dskInfo.MpathDevice
		zlog.Debug().Msgf("fc config: mpathDevice %s", mpathDevice)
	} else {
		zlog.Debug().Msgf("fc config not existing at staging path")
		confFile := path.Join("/host", stagePath, volName+".json")
		zlog.Debug().Msgf("check if fc config file exists")
		pathExist, pathErr := fc.cs.pathExists(confFile)
		if pathErr == nil {
			if !pathExist {
				zlog.Debug().Msgf("Config file not found at %s", confFile)
				if err := os.RemoveAll(stagePath); err != nil {
					zlog.Error().Msgf("Failed to remove mount path Error: %v", err)
					return nil, err
				}
				zlog.Debug().Msgf("Removed stage path at %s", stagePath)
				return &csi.NodeUnstageVolumeResponse{}, nil
			}
		}
		zlog.Warn().Msgf("fc detach disk: failed to get fc config from path %s Error: %v", stagePath, err)
	}

	// remove multipath device
	protocol := "fc"
	err := detachMpathDevice(mpathDevice, protocol)
	if err != nil {
		zlog.Err(err)
		zlog.Warn().Msgf("NodeUnstageVolume cannot detach volume with ID %s: %+v", req.GetVolumeId(), err)
	}

	if err := os.RemoveAll("/host" + stagePath); err != nil {
		zlog.Error().Msgf("fc: failed to remove mount path Error: %v", err)
		return nil, err
	}
	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (fc *fcstorage) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	return nil, status.Error(codes.Unimplemented, "fc NodeGetCapabilities should never be called, called in node.go instead")
}

func (fc *fcstorage) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	return &csi.NodeGetInfoResponse{}, nil
}

func (fc *fcstorage) NodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	return nil, status.Error(codes.Unimplemented, time.Now().String())
}

func (fc *fcstorage) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	zlog.Info().Msgf("fc NodeExpandVolume called request %+v\n", req)

	response := csi.NodeExpandVolumeResponse{}

	if req.GetVolumeCapability().GetBlock() != nil {
		err := blockExpandVolume(req.GetVolumePath())
		if err != nil {
			zlog.Error().Msgf("error expanding block volume name %s - from %s \n", err.Error(), req.GetVolumePath())
			return nil, err
		}
		return &response, nil
	}

	// 1 - run mount | grep <volume_path> to find the multipath device name (e.g. /dev/mapper/mpathwi)

	command := fmt.Sprintf("mount | grep %s", req.GetVolumePath())
	out, err := execScsi.Command(command, "")
	if err != nil {
		zlog.Error().Msgf("error getting multipath device name %s - from %s \n", err.Error(), req.GetVolumePath())
		return nil, err
	}

	if out == "" {
		err := fmt.Errorf("error getting multipath device name volume path is %s, command output was empty", req.GetVolumePath())
		zlog.Error().Msgf(err.Error())
		return nil, err
	}

	output := strings.TrimSpace(out)
	outputParts := strings.Split(output, " ")
	multipathDevice := outputParts[0]
	zlog.Debug().Msgf("output is [%v] multipathDevice=[%s]", output, multipathDevice)

	// 2 - run multipath -l multipathDevice  to look up the particular device names (sda, sdb, sdx, ....)
	command = fmt.Sprintf("multipath -l %s | tail -n +4", multipathDevice)
	out, err = execScsi.Command(command, "")
	if err != nil {
		zlog.Error().Msgf("error getting multipath devices from output %s \n", err.Error())
		return nil, err
	}

	if out == "" {
		err := fmt.Errorf("error getting multipath device name %s, command output was empty", multipathDevice)
		zlog.Error().Msgf(err.Error())
		return nil, err
	}

	output = strings.TrimSpace(out)
	zlog.Debug().Msgf("output is [%s]\n", output)
	outputParts = strings.Split(output, "\n")
	zlog.Debug().Msgf("lines %d\n", len(outputParts))

	// 3 - echo 1 > /sys/block/path_device/device/rescan  .... run those commands on each device from the previous step
	for i := 0; i < len(outputParts); i++ {
		line := strings.TrimSpace(outputParts[i])
		lineParts := strings.Split(line, " ")
		if len(lineParts) < 3 {
			zlog.Error().Msgf("error getting multipath blockDevice from output %v", lineParts)
			continue
		}
		blockDevice := lineParts[2]
		zlog.Debug().Msgf("device is [%s]\n", blockDevice)
		rescanPath := fmt.Sprintf("/sys/block/%s/device/rescan", blockDevice)
		command = fmt.Sprintf("echo 1 > %s", rescanPath)
		out, err := execScsi.Command(command, "")
		if err != nil {
			zlog.Error().Msgf("error writing rescan on multipath devices %s \n", err.Error())
			return nil, err
		}
		zlog.Debug().Msgf("rescan output is [%s]\n", strings.TrimSpace(string(out)))
	}

	// 4 - run multipathd resize map multipath_device - where multipath_device is like /dev/mapper/mpathwi from previous step,
	// we need to strip off the /dev/mapper/ path prefix
	mpathPart := strings.SplitAfter(multipathDevice, "/dev/mapper/")
	if len(mpathPart) < 2 {
		return nil, fmt.Errorf("error getting mpathPart from %+v", mpathPart)
	}
	command = fmt.Sprintf("multipathd resize map %s", mpathPart[1])
	out, err = execScsi.Command(command, "")
	if err != nil {
		zlog.Error().Msgf("error multipathd resize map multipath devices %s \n", err.Error())
		return nil, err
	}
	zlog.Debug().Msgf("multipathd resize map output is [%s]\n", strings.TrimSpace(string(out)))

	// 5 - run resize2fs /dev/mapper/mpathwi, this appears to work for both FC and iSCSI
	time.Sleep(time.Second * 5)
	command = fmt.Sprintf("resize2fs %s", multipathDevice)
	out, err = execScsi.Command(command, "")
	if err != nil {
		zlog.Error().Msgf("error resize2fs %s \n", err.Error())
		return nil, err
	}
	zlog.Debug().Msgf("resize2fs output is [%s]\n", strings.TrimSpace(string(out)))

	return &response, nil
}

func (fc *fcstorage) MountFCDisk(fm FCMounter, devicePath string) error {
	zlog.Debug().Msgf("fc MountFCDisk called request %+v devicePath %s", fm, devicePath)

	var mounted bool
	mntPoints, err := fm.Mounter.List()
	if err != nil {
		zlog.Err(err)
		return status.Errorf(codes.Internal, "fm.Mounter.List error: %s", err.Error())
	}

	var chrootPath = "/host" + fm.TargetPath
	zlog.Debug().Msgf("Mount List has %d, looking for %s", len(mntPoints), chrootPath)
	for i := 0; i < len(mntPoints); i++ {
		if mntPoints[i].Path == chrootPath {
			mounted = true
			break
		}
	}

	if mounted {
		zlog.Debug().Msgf("fc: MountFCDisk %s already Mounted", chrootPath)
		return nil
	}

	if fm.fcDisk.isBlock {
		// option A: raw block volume access
		zlog.Debug().Msgf("mounting raw block volume at given path %s", fm.TargetPath)
		if fm.ReadOnly {
			// TODO: actually implement this - CSIC-343
			return status.Error(codes.Internal, "Read only is not supported for Block Volume")
		}

		zlog.Debug().Msgf("Mount point does not exist. Creating mount point.")
		zlog.Debug().Msgf("Run: mkdir --parents --mode 0750 '%s' ", filepath.Dir(fm.TargetPath))
		// Do not use os.MkdirAll(). This ignores the mount chroot defined in the Dockerfile.
		// MkdirAll() will cause hard-to-grok mount errors.
		cmd := exec.Command("mkdir", "--parents", "--mode", "0750", filepath.Dir(fm.TargetPath))
		err = cmd.Run()
		if err != nil {
			zlog.Error().Msgf("failed to mkdir '%s': %s", fm.TargetPath, err)
			return err
		}

		zlog.Debug().Msgf("Creating file: %s", chrootPath)
		_, err = os.Create(chrootPath)
		if err != nil {
			zlog.Error().Msgf("failed to create target path for raw bind mount: %q, err: %v", fm.TargetPath, err)
			return status.Errorf(codes.Internal, "failed to create target path for raw block bind mount: %v", err)
		}
		devicePath = strings.Replace(devicePath, "/host", "", 1)

		// TODO: validate this further, see CSIC-341
		options := []string{"bind"}
		options = append(options, "rw") // TODO: address in CSIC-343
		if err := fm.Mounter.Mount(devicePath, fm.TargetPath, "", options); err != nil {
			zlog.Error().Msgf("fc: failed to mount fc volume %s to %s, error %v", devicePath, fm.TargetPath, err)
			return err
		}
		zlog.Debug().Msgf("Block volume mounted successfully")
	} else {
		// option B: local filesystem access
		zlog.Debug().Msgf("mounting volume with filesystem at given path %s", fm.TargetPath)

		// Create mountPoint, with prepended /host, if it does not exist.
		mountPoint := chrootPath
		_, err := os.Stat(mountPoint)
		if err != nil {
			zlog.Err(err)
		}
		if os.IsNotExist(err) {
			zlog.Debug().Msgf("Mount point does not exist. Creating mount point.")
			// Do not use os.MkdirAll(). This ignores the mount chroot defined in the Dockerfile.
			// MkdirAll() will cause hard-to-grok mount errors.
			_, err := execFc.Command("mkdir", fmt.Sprintf("--parents --mode 0750 '%s'", fm.TargetPath))
			if err != nil {
				zlog.Error().Msgf("Failed to mkdir '%s': %s", fm.TargetPath, err)
				return err
			}

			// // Verify mountPoint exists. If ready a file named 'ready' will appear in mountPoint directory.
			// util.SetReady(mountPoint)
			// is_ready := util.IsReady(mountPoint)
			// zlog.Debug().Msgf("Check that mountPoint is ready: %t", is_ready)
		} else {
			zlog.Debug().Msgf("mkdir of mountPoint not required. '%s' already exists", mountPoint)
		}

		// TODO: validate this further, see CSIC-341
		options := []string{}
		if fm.ReadOnly { // TODO: address in CSIC-343
			options = append(options, "ro")
		} else {
			options = append(options, "rw")
		}
		options = append(options, fm.MountOptions...)

		if fm.FsType == "xfs" {
			zlog.Debug().Msgf("Device %s is of type XFS. Mounting using 'nouuid' option.", devicePath)
			options = append(options, "nouuid")
		}

		err = fm.Mounter.FormatAndMount(devicePath, fm.TargetPath, fm.FsType, options)
		if err != nil {
			zlog.Debug().Msgf("FormatAndMount returned an error. devicePath: %s, targetPath: %s, fsType: %s, error: %s", devicePath, fm.TargetPath, fm.FsType, err)
			searchAlreadyMounted := fmt.Sprintf("already mounted on %s", mountPoint)
			zlog.Debug().Msgf("Search error for matches to handle: %s", err)

			if isAlreadyMounted := strings.Contains(err.Error(), searchAlreadyMounted); isAlreadyMounted {
				zlog.Error().Msgf("Device %s is already mounted on %s", devicePath, mountPoint)
			} else {
				msg := fmt.Sprintf("fc: failed to mount fc volume %s [%s] to %s, err: %v", devicePath, fm.FsType, fm.TargetPath, err)
				zlog.Error().Msgf(msg)
				return status.Errorf(codes.Internal, msg)
			}
		}
	}
	dskinfo := diskInfo{}
	if strings.HasPrefix(devicePath, "/dev/dm-") {
		dskinfo.MpathDevice = devicePath
		dskinfo.IsBlock = fm.fcDisk.isBlock
		dskinfo.VolName = fm.fcDisk.connector.VolumeName
		if err := fc.createFcConfigFile(dskinfo, fm.StagePath); err != nil {
			zlog.Error().Msgf("fc: failed to save fc config with error: %v", err)
			return err
		}
	}
	zlog.Debug().Msgf("FormatAndMount succeeded. devicePath: %s, targetPath: %s, fsType: %s", devicePath, fm.TargetPath, fm.FsType)
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

func (fc *fcstorage) getFCDiskDetails(req *csi.NodePublishVolumeRequest) (*fcDevice, error) {
	volproto := strings.Split(req.GetVolumeId(), "$$")
	volName := volproto[0]
	lun := req.GetPublishContext()["lun"]
	wwids := req.GetVolumeContext()["WWIDs"]
	wwidList := strings.Split(wwids, ",")
	targetList := []string{}
	fcNodes, err := fc.cs.Api.GetFCPorts()
	if err != nil {
		zlog.Err(err)
		return nil, fmt.Errorf("error getting fiber channel details")
	}
	for _, fcnode := range fcNodes {
		for _, fcport := range fcnode.Ports {
			if fcport.WWPn != "" {
				targetList = append(targetList, strings.Replace(fcport.WWPn, ":", "", -1))
			}
		}
	}
	zlog.Debug().Msgf("lun %s , targetList %v , wwidList %v", lun, targetList, wwidList)
	if lun == "" || (len(targetList) == 0 && len(wwidList) == 0) {
		return nil, fmt.Errorf("FC target information is missing")
	}
	fcConnector := &Connector{
		VolumeName: volName,
		TargetWWNs: targetList,
		WWIDs:      wwidList,
		Lun:        lun,
	}
	// Only pass the connector
	return &fcDevice{
		connector: fcConnector,
	}, nil
}

func (fc *fcstorage) getFCDiskMounter(req *csi.NodePublishVolumeRequest, fcDetails fcDevice) (*FCMounter, error) {
	// standard place to define block/file etc
	reqVolCapability := req.GetVolumeCapability()

	// check accessMode - where we will eventually police R/W etc (CSIC-343)
	accessMode := reqVolCapability.GetAccessMode().GetMode() // GetAccessMode() guaranteed not nil from controller.go
	// TODO: set readonly flag for RO accessmodes, any other validations needed?

	// handle file (mount) and block parameters
	mountVolCapability := reqVolCapability.GetMount()
	var fstype string
	mountOptions := []string{}
	blockVolCapability := reqVolCapability.GetBlock()

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
		zlog.Error().Msgf(errMsg)
		return nil, status.Error(codes.InvalidArgument, errMsg)
	}

	return &FCMounter{
		fcDisk:       fcDetails,
		ReadOnly:     false, // TODO: not accurate, address in CSIC-343
		FsType:       fstype,
		MountOptions: mountOptions,
		Mounter:      &mount.SafeFormatAndMount{Interface: mount.NewWithoutSystemd(""), Exec: utilexec.New()},
		Exec:         utilexec.New(),
		DeviceUtil:   util.NewDeviceHandler(util.NewIOHandler()),
		TargetPath:   req.GetTargetPath(),
		StagePath:    req.GetStagingTargetPath(),
	}, nil
}

type ioHandler interface {
	ReadDir(dirname string) ([]os.FileInfo, error)
	Lstat(name string) (os.FileInfo, error)
	EvalSymlinks(path string) (string, error)
	WriteFile(filename string, data []byte, perm os.FileMode) error
}

// Connector provides a struct to hold all of the needed parameters to make our Fibre Channel connection
type Connector struct {
	VolumeName string
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
		zlog.Err(err)
		return infos, err
	}
	infos = make([]fs.FileInfo, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			zlog.Err(err)
			return infos, err

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

// FindMultipathDeviceForDevice given a device name like /dev/sdx, find the devicemapper parent
func (fc *fcstorage) findMultipathDeviceForDevice(device string, io ioHandler) (string, error) {
	zlog.Debug().Msgf("In findMultipathDeviceForDevice")
	disk, err := fc.findDeviceForPath(device)
	if err != nil {
		zlog.Err(err)
		return "", err
	}
	sysPath := "/sys/block/"
	if dirs, err2 := io.ReadDir(sysPath); err2 == nil {
		for _, f := range dirs {
			name := f.Name()
			if strings.HasPrefix(name, "dm-") {
				if _, err1 := io.Lstat(sysPath + name + "/slaves/" + disk); err1 == nil {
					return "/dev/" + name, nil
				}
			}
		}
	} else {
		zlog.Error().Msgf("failed to find multipath device with error %v", err)
		return "", err2
	}
	zlog.Debug().Msgf("multipath not configured")
	return "", nil
}

func (fc *fcstorage) findDeviceForPath(path string) (string, error) {
	zlog.Debug().Msgf("In findDeviceForPath")
	devicePath, err := filepath.EvalSymlinks(path)
	if err != nil {
		zlog.Err(err)
		return "", err
	}
	// if path /dev/hdX split into "", "dev", "hdX" then we will
	// return just the last part
	devicePath = strings.Replace(devicePath, "/host", "", 1)
	parts := strings.Split(devicePath, "/")
	if len(parts) == 3 && strings.HasPrefix(parts[1], "dev") {
		zlog.Debug().Msgf("found device: %s", parts[2])
		return parts[2], nil
	}
	return "", errors.New("Illegal path for device " + devicePath)
}

func (fc *fcstorage) rescanDeviceMap(volumeId string, lun string) error {

	// deviceMu.Lock()
	zlog.Debug().Msgf("Rescan hosts for volume '%s' and lun '%s'", volumeId, lun)

	fcHosts, err := findHosts("fc")
	if err != nil {
		zlog.Err(err)
		return err
	}

	// For each host, scan using lun
	for _, fcHost := range fcHosts {
		scsiHostPath := fmt.Sprintf("/sys/class/scsi_host/host%s/scan", fcHost)
		zlog.Debug().Msgf("Rescanning host path at '%s' for volume ID '%s' and lun '%s'", scsiHostPath, volumeId, lun)
		_, err = execScsi.Command("echo", fmt.Sprintf("'- - %s' > %s", lun, scsiHostPath))
		if err != nil {
			zlog.Error().Msgf("Rescan of host %s failed for volume ID '%s' and lun '%s': %s", scsiHostPath, volumeId, lun, err)
			return err
		}
	}

	for _, fcHost := range fcHosts {
		if err := waitForDeviceState(fcHost, lun, "running"); err != nil {
			return err
		}
	}

	if err := waitForMultipath(fcHosts[0], lun); err != nil {
		zlog.Debug().Msgf("Rescan hosts failed for volume ID '%s' and lun '%s'", volumeId, lun)
		return err
	}

	zlog.Debug().Msgf("Rescan hosts complete for volume ID '%s' and lun '%s'", volumeId, lun)
	return err
}

func (fc *fcstorage) searchDisk(c Connector, io ioHandler) (string, error) {
	zlog.Debug().Msgf("Called searchDisk")
	var diskIds []string // target wwns
	var disk string
	var dm string

	if len(c.TargetWWNs) != 0 {
		diskIds = c.TargetWWNs
	} else {
		diskIds = c.WWIDs
	}

	zlog.Debug().Msgf("searchDisk rescan scsi host")
	_ = fc.rescanDeviceMap(diskIds[0], c.Lun)

	for _, diskID := range diskIds {
		if len(c.TargetWWNs) != 0 {
			disk, dm = fc.findFcDisk(diskID, c.Lun, io)
		} else {
			disk, dm = fc.getDisksWwids(diskID, io)
		}
		// if multipath device is found, break
		zlog.Debug().Msgf("searchDisk() found disk '%s' and dm '%s'", disk, dm)
		if dm != "" {
			break
		}
	}

	// if no disk matches input wwn and lun, exit
	if disk == "" && dm == "" {
		return "", fmt.Errorf("no fc disk found")
	}

	// if multipath devicemapper device is found, use it; otherwise use raw disk
	if dm != "" {
		zlog.Debug().Msgf("Multipath devicemapper device is found %s", disk)
		return dm, nil
	}
	zlog.Debug().Msgf("Multipath devicemapper device not found, using raw disk %s", disk)
	return disk, nil
}

// find the fc device and device mapper parent
func (fc *fcstorage) findFcDisk(wwn, lun string, io ioHandler) (string, string) {
	zlog.Debug().Msgf("findFcDisk called with wwn %s and lun %s", wwn, lun)
	FcPath := "-fc-0x" + wwn + "-lun-" + lun
	DevPath := "/host/dev/disk/by-path/"
	if dirs, err := io.ReadDir(DevPath); err == nil {
		for _, f := range dirs {
			name := f.Name()
			if strings.Contains(name, FcPath) {
				if disk, err1 := io.EvalSymlinks(DevPath + name); err1 == nil {
					if dm, err2 := fc.findMultipathDeviceForDevice(disk, io); err2 == nil {
						return disk, dm
					} else {
						zlog.Error().Msgf("could not find disk with error %v", err2)
					}
				} else {
					zlog.Error().Msgf("could not find disk with error %v", err1)
				}
			}
		}
	} else {
		zlog.Error().Msgf("could not find disk with error %v", err)
	}
	return "", ""
}

func (fc *fcstorage) getDisksWwids(wwid string, io ioHandler) (string, string) {
	FcPath := "scsi-" + wwid
	DevID := "/dev/disk/by-id/"
	if dirs, err := io.ReadDir(DevID); err == nil {
		for _, f := range dirs {
			name := f.Name()
			if name == FcPath {
				disk, err := io.EvalSymlinks(DevID + name)
				if err != nil {
					zlog.Error().Msgf("fc: failed to find a corresponding disk from symlink[%s], error %v", DevID+name, err)
					return "", ""
				}
				if dm, err1 := fc.findMultipathDeviceForDevice(disk, io); err1 != nil {
					return disk, dm
				}
			}
		}
	}
	zlog.Error().Msgf("fc: failed to find a disk [%s]", DevID+FcPath)
	return "", ""
}

// Find and return FC disk
/**
func (fc *fcstorage) getFCDisk2(c Connector, io ioHandler) (string, error) {
	if io == nil {
		io = &OSioHandler{}
	}
	zlog.Debug().Msgf("getFCDisk() called")
	devicePath, err := fc.searchDisk(c, io)
	if err != nil {
		zlog.Debug().Msgf("getFCDisk() failed. Unable to find disk given WWNN or WWIDs: %+v", err)
		return "", err
	}
	devicePath = strings.Replace(devicePath, "/host", "", 1)
	zlog.Debug().Msgf("FC device path %s found", devicePath)

	return devicePath, nil
}
*/

func (fc *fcstorage) createFcConfigFile(conf diskInfo, mnt string) error {
	file := path.Join("/host", mnt, conf.VolName+".json")
	fp, err := os.Create(file)
	if err != nil {
		zlog.Error().Msgf("fc: failed creating persist file with error %v", err)
		return fmt.Errorf("fc: create %s err %s", file, err)
	}
	defer fp.Close()
	encoder := json.NewEncoder(fp)
	if err = encoder.Encode(conf); err != nil {
		zlog.Error().Msgf("fc: failed creating persist file with error %v", err)
		return fmt.Errorf("fc: encode err: %v", err)
	}
	zlog.Debug().Msgf("fc: created persist config file at path %s", file)
	return nil
}

func (fc *fcstorage) loadFcDiskInfoFromFile(conf *diskInfo, mnt string) error {
	file := path.Join("/host", mnt, conf.VolName+".json")
	fp, err := os.Open(file)
	if err != nil {
		zlog.Err(err)
		return fmt.Errorf("fc: open %s err %s", file, err)
	}
	defer fp.Close()
	decoder := json.NewDecoder(fp)
	if err = decoder.Decode(conf); err != nil {
		zlog.Err(err)
		return fmt.Errorf("fc: decode err: %v ", err)
	}
	return nil
}
