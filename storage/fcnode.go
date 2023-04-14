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
	"k8s.io/klog/v2"
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
	/**
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = fmt.Errorf("Recovered from NodeStageVolume: %+v", res)
		}
		if err == nil {
			klog.V(4).Infof("NodeStageVolume completed")
		} else {
			klog.V(4).Infof("NodeStageVolume failed for request %+v", req)
		}
	}()
	*/
	klog.V(2).Infof("NodeStageVolume called with PublishContext: %+v", req.GetPublishContext())
	hostIdString := req.GetPublishContext()["hostID"]
	ports := req.GetPublishContext()["hostPorts"]

	hostId, err := strconv.Atoi(hostIdString)
	if err != nil {
		klog.Error(err)
		err := fmt.Errorf("hostID string '%s' is not valid host ID: %s", hostIdString, err)
		klog.Error(err.Error())
		return nil, status.Error(codes.Internal, err.Error())
	}

	klog.V(4).Infof("Publishing volume to host with host ID %d", hostId)
	// validate host exists
	if hostId < 1 {
		err := fmt.Errorf("hostId %d is not valid host Id", hostId)
		klog.Error(err.Error())
		return nil, status.Error(codes.Internal, err.Error())
	}

	fcPorts := getPortName()
	if len(fcPorts) == 0 {
		klog.Errorf("port name not found on worker")
		return nil, status.Error(codes.Internal, "Port name not found")
	}
	for _, fcp := range fcPorts {
		if !strings.Contains(ports, fcp) {
			klog.V(4).Infof("host port %s is not created, creating it", fcp)
			err = fc.cs.AddPortForHost(hostId, "FC", fcp)
			if err != nil {
				klog.Errorf("error creating host port %v", err)
				return nil, status.Error(codes.Internal, err.Error())
			}
			_, err := fc.cs.api.GetHostPort(hostId, fcp)
			if err != nil {
				klog.Errorf("failed to get host port %s with error %v", fcp, err)
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
			klog.V(4).Infof("NodePublishVolume succeeded with volume ID %s", req.GetVolumeId())
		} else {
			klog.V(4).Infof("NodePublishVolume failed with volume ID %s: %+v", req.GetVolumeId(), err)
		}
	}()

	klog.V(2).Infof("NodePublishVolume volumecontext %v", req.GetVolumeContext())
	klog.V(4).Infof("uid %s gid %s unix_perm %s", req.GetVolumeContext()[common.SC_UID], req.GetVolumeContext()[common.SC_GID], req.GetVolumeContext()[common.SC_UNIX_PERMISSIONS])
	klog.V(4).Infof("NodePublishVolume called with volume ID %s", req.GetVolumeId())

	fcDetails, err := fc.getFCDiskDetails(req)
	if err != nil {
		klog.Error(err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	/**
	devicePath, err := fc.getFCDisk(*fcDetails.connector, &OSioHandler{})
	if err != nil {
		klog.Errorf("")
		return nil, status.Error(codes.Internal, err.Error())
	}
	*/

	devicePath, err := fc.searchDisk(*fcDetails.connector, &OSioHandler{})
	if err != nil {
		klog.Errorf("fc.searchDisk() failed. Unable to find disk given WWNN or WWIDs: %+v", err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	// remove the leading "/host" to reveal the path on the actual node host
	devicePath = strings.Replace(devicePath, "/host", "", 1)
	klog.V(4).Infof("FC device path %s found", devicePath)

	diskMounter, err := fc.getFCDiskMounter(req, *fcDetails)
	if err != nil {
		klog.Error(err)
		return nil, err
	}

	err = fc.MountFCDisk(*diskMounter, devicePath)
	if err != nil {
		klog.Error(err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	// set volume permissions based on uid/uid/unix_permissions
	klog.V(4).Infof("after mount targetPath %s, devicePath %s ", diskMounter.TargetPath, devicePath)
	// print out the target permissions
	logPermissions("after mount targetPath ", filepath.Dir("/host"+diskMounter.TargetPath))
	logPermissions("after mount devicePath ", "/host"+devicePath)
	err = fc.storageHelper.SetVolumePermissions(req)
	if err != nil {
		klog.Errorf("error in setting volume permissions %s on volume %s\n", err.Error(), req.GetVolumeId())
		return nil, err
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func (fc *fcstorage) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	var err error
	/**
	defer func() {
		if err == nil {
			klog.V(4).Infof("NodeUnpublishVolume succeeded with volume ID %s", req.GetVolumeId())
		} else {
			klog.V(4).Infof("NodeUnpublishVolume failed with volume ID %s: %+v", req.GetVolumeId(), err)
		}
	}()
	*/

	klog.V(2).Infof("NodeUnpublishVolume called with volume ID %s", req.GetVolumeId())

	targetPath := req.GetTargetPath()
	err = unmountAndCleanUp(targetPath)
	if err != nil {
		klog.Error(err)
		return nil, err
	}
	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (fc *fcstorage) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	klog.V(2).Infof("Called FC NodeUnstageVolume")
	var err error
	/**
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("Recovered from FC NodeUnstageVolume  " + fmt.Sprint(res))
		}
	}()
	*/
	var mpathDevice string
	stagePath := req.GetStagingTargetPath()

	volproto := strings.Split(req.GetVolumeId(), "$$")
	volName := volproto[0]

	dskInfo := diskInfo{}
	dskInfo.VolName = volName

	// load fc disk config from json file
	klog.V(4).Infof("read fc config from staging path")
	if err := fc.loadFcDiskInfoFromFile(&dskInfo, stagePath); err == nil {
		mpathDevice = dskInfo.MpathDevice
		klog.V(4).Infof("fc config: mpathDevice %s", mpathDevice)
	} else {
		klog.V(4).Infof("fc config not existing at staging path")
		confFile := path.Join("/host", stagePath, volName+".json")
		klog.V(4).Infof("check if fc config file exists")
		pathExist, pathErr := fc.cs.pathExists(confFile)
		if pathErr == nil {
			if !pathExist {
				klog.V(4).Infof("Config file not found at %s", confFile)
				if err := os.RemoveAll(stagePath); err != nil {
					klog.Errorf("Failed to remove mount path Error: %v", err)
					return nil, err
				}
				klog.V(4).Infof("Removed stage path at %s", stagePath)
				return &csi.NodeUnstageVolumeResponse{}, nil
			}
		}
		klog.Warningf("fc detach disk: failed to get fc config from path %s Error: %v", stagePath, err)
	}

	// remove multipath device
	protocol := "fc"
	err = detachMpathDevice(mpathDevice, protocol)
	if err != nil {
		klog.Error(err)
		klog.Warningf("NodeUnstageVolume cannot detach volume with ID %s: %+v", req.GetVolumeId(), err)
	}

	if err := os.RemoveAll("/host" + stagePath); err != nil {
		klog.Errorf("fc: failed to remove mount path Error: %v", err)
		return nil, err
	}
	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (fc *fcstorage) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (
	*csi.NodeGetCapabilitiesResponse, error,
) {
	return &csi.NodeGetCapabilitiesResponse{
		Capabilities: []*csi.NodeServiceCapability{
			{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_UNKNOWN,
					},
				},
			},
		},
	}, nil
}

func (fc *fcstorage) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (
	*csi.NodeGetInfoResponse, error,
) {
	return &csi.NodeGetInfoResponse{}, nil
}

func (fc *fcstorage) NodeGetVolumeStats(
	ctx context.Context, req *csi.NodeGetVolumeStatsRequest,
) (*csi.NodeGetVolumeStatsResponse, error) {
	return nil, status.Error(codes.Unimplemented, time.Now().String())
}

func (fc *fcstorage) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, time.Now().String())
}

func (fc *fcstorage) MountFCDisk(fm FCMounter, devicePath string) error {
	notMnt, err := fm.Mounter.IsLikelyNotMountPoint(fm.TargetPath)
	if err != nil {
		klog.Error(err)
	}
	if err == nil {
		if !notMnt {
			// ToDo: check that it is mounted on the right directory
			klog.V(4).Infof("fc: %s already mounted", fm.TargetPath)
			return nil
		}
	} else if !os.IsNotExist(err) {
		return status.Errorf(codes.Internal, "%s exists but IsLikelyNotMountPoint failed: %v", fm.TargetPath, err)
	}

	if fm.fcDisk.isBlock {
		// option A: raw block volume access
		klog.V(4).Infof("mounting raw block volume at given path %s", fm.TargetPath)
		if fm.ReadOnly {
			// TODO: actually implement this - CSIC-343
			return status.Error(codes.Internal, "Read only is not supported for Block Volume")
		}

		klog.V(4).Infof("Mount point does not exist. Creating mount point.")
		klog.V(4).Infof("Run: mkdir --parents --mode 0750 '%s' ", filepath.Dir(fm.TargetPath))
		// Do not use os.MkdirAll(). This ignores the mount chroot defined in the Dockerfile.
		// MkdirAll() will cause hard-to-grok mount errors.
		cmd := exec.Command("mkdir", "--parents", "--mode", "0750", filepath.Dir(fm.TargetPath))
		err = cmd.Run()
		if err != nil {
			klog.Errorf("failed to mkdir '%s': %s", fm.TargetPath, err)
			return err
		}

		klog.V(4).Infof("Creating file: /host/%s", fm.TargetPath)
		_, err = os.Create("/host/" + fm.TargetPath)
		if err != nil {
			klog.Errorf("failed to create target path for raw bind mount: %q, err: %v", fm.TargetPath, err)
			return status.Errorf(codes.Internal, "failed to create target path for raw block bind mount: %v", err)
		}
		devicePath = strings.Replace(devicePath, "/host", "", 1)

		// TODO: validate this further, see CSIC-341
		options := []string{"bind"}
		options = append(options, "rw") // TODO: address in CSIC-343
		if err := fm.Mounter.Mount(devicePath, fm.TargetPath, "", options); err != nil {
			klog.Errorf("fc: failed to mount fc volume %s to %s, error %v", devicePath, fm.TargetPath, err)
			return err
		}
		klog.V(4).Infof("Block volume mounted successfully")
	} else {
		// option B: local filesystem access
		klog.V(4).Infof("mounting volume with filesystem at given path %s", fm.TargetPath)

		// Create mountPoint, with prepended /host, if it does not exist.
		mountPoint := "/host" + fm.TargetPath
		_, err := os.Stat(mountPoint)
		if err != nil {
			klog.Error(err)
		}
		if os.IsNotExist(err) {
			klog.V(4).Infof("Mount point does not exist. Creating mount point.")
			// Do not use os.MkdirAll(). This ignores the mount chroot defined in the Dockerfile.
			// MkdirAll() will cause hard-to-grok mount errors.
			_, err := execFc.Command("mkdir", fmt.Sprintf("--parents --mode 0750 '%s'", fm.TargetPath))
			if err != nil {
				klog.Errorf("Failed to mkdir '%s': %s", fm.TargetPath, err)
				return err
			}

			// // Verify mountPoint exists. If ready a file named 'ready' will appear in mountPoint directory.
			// util.SetReady(mountPoint)
			// is_ready := util.IsReady(mountPoint)
			// klog.V(2).Infof("Check that mountPoint is ready: %t", is_ready)
		} else {
			klog.V(4).Infof("mkdir of mountPoint not required. '%s' already exists", mountPoint)
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
			klog.V(4).Infof("Device %s is of type XFS. Mounting using 'nouuid' option.", devicePath)
			options = append(options, "nouuid")
		}

		err = fm.Mounter.FormatAndMount(devicePath, fm.TargetPath, fm.FsType, options)
		if err != nil {
			klog.V(4).Infof("FormatAndMount returned an error. devicePath: %s, targetPath: %s, fsType: %s, error: %s", devicePath, fm.TargetPath, fm.FsType, err)
			searchAlreadyMounted := fmt.Sprintf("already mounted on %s", mountPoint)
			klog.V(4).Infof("Search error for matches to handle: %s", err)

			if isAlreadyMounted := strings.Contains(err.Error(), searchAlreadyMounted); isAlreadyMounted {
				klog.Errorf("Device %s is already mounted on %s", devicePath, mountPoint)
			} else {
				msg := fmt.Sprintf("fc: failed to mount fc volume %s [%s] to %s, err: %v", devicePath, fm.FsType, fm.TargetPath, err)
				klog.Errorf(msg)
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
			klog.Errorf("fc: failed to save fc config with error: %v", err)
			return err
		}
	}
	klog.V(4).Infof("FormatAndMount succeeded. devicePath: %s, targetPath: %s, fsType: %s", devicePath, fm.TargetPath, fm.FsType)
	return nil
}

func getPortName() []string {
	var err error
	/**
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("Recovered from FC getPortName  " + fmt.Sprint(res))
		}
	}()
	*/
	ports := []string{}
	cmd := "cat /sys/class/fc_host/host*/port_name"
	out, err := exec.Command("bash", "-c", cmd).Output()
	if err != nil {
		klog.Errorf("Failed to get port name using command '%s': %v", cmd, err)
		return ports
	}
	portName := string(out)
	if portName != "" {
		for _, port := range strings.Split(strings.TrimSuffix(portName, "\n"), "\n") {
			ports = append(ports, strings.Replace(port, "0x", "", 1))
		}
	}
	klog.V(4).Infof("fc ports found %v ", ports)
	return ports
}

func (fc *fcstorage) getFCDiskDetails(req *csi.NodePublishVolumeRequest) (*fcDevice, error) {
	var err error
	/**
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("Recovered from FC getFCDiskDetails " + fmt.Sprint(res))
		}
	}()
	*/
	volproto := strings.Split(req.GetVolumeId(), "$$")
	volName := volproto[0]
	lun := req.GetPublishContext()["lun"]
	wwids := req.GetVolumeContext()["WWIDs"]
	wwidList := strings.Split(wwids, ",")
	targetList := []string{}
	fcNodes, err := fc.cs.api.GetFCPorts()
	if err != nil {
		klog.Error(err)
		return nil, fmt.Errorf("error getting fiber channel details")
	}
	for _, fcnode := range fcNodes {
		for _, fcport := range fcnode.Ports {
			if fcport.WWPn != "" {
				targetList = append(targetList, strings.Replace(fcport.WWPn, ":", "", -1))
			}
		}
	}
	klog.V(4).Infof("lun %s , targetList %v , wwidList %v", lun, targetList, wwidList)
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
	fstype := ""
	mountOptions := []string{}
	blockVolCapability := reqVolCapability.GetBlock()

	// LEGACY MITIGATION: accept but warn about old opaque fstype parameter if present - remove in the future with CSIC-344
	fstype, oldFstypeParamProvided := req.GetVolumeContext()["fstype"]
	if oldFstypeParamProvided {
		klog.Warningf("Deprecated 'fstype' parameter %s provided, will NOT be supported in future releases - please move to 'csi.storage.k8s.io/fstype'", fstype)
	}

	// protocol-specific paths below
	if mountVolCapability != nil && blockVolCapability == nil {
		// option A. user wants file access to their FC device
		fcDetails.isBlock = false

		// filesystem type and reconciliation with older nonstandard param
		// LEGACY MITIGATION: remove !oldFstypeParamProvided in the future with CSIC-344
		if mountVolCapability.GetFsType() != "" {
			fstype = mountVolCapability.GetFsType()
		} else if !oldFstypeParamProvided {
			errMsg := "No fstype in VolumeCapability for volume: " + req.GetVolumeId()
			klog.Errorf(errMsg)
			return nil, status.Error(codes.InvalidArgument, errMsg)
		}

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
			klog.Warning("MULTI_NODE_MULTI_WRITER AccessMode requested for raw block volume, could be dangerous")
		}
		// TODO: something about SINGLE_NODE_MULTI_WRITER (alpha feature) as well?

		// don't need to look at FsType or MountFlags here, only relevant for mountVol
		// TODO: other validations needed for block?
		// - something about read-only access?
	} else {
		errMsg := "Bad VolumeCapability parameters: both block and mount modes, for volume: " + req.GetVolumeId()
		klog.Errorf(errMsg)
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
		klog.Error(err)
		return infos, err
	}
	infos = make([]fs.FileInfo, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			klog.Error(err)
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
	klog.V(4).Infof("In findMultipathDeviceForDevice")
	disk, err := fc.findDeviceForPath(device)
	if err != nil {
		klog.Error(err)
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
		klog.Errorf("failed to find multipath device with error %v", err)
		return "", err2
	}
	klog.V(4).Infof("multipath not configured")
	return "", nil
}

func (fc *fcstorage) findDeviceForPath(path string) (string, error) {
	klog.V(4).Infof("In findDeviceForPath")
	devicePath, err := filepath.EvalSymlinks(path)
	if err != nil {
		klog.Error(err)
		return "", err
	}
	// if path /dev/hdX split into "", "dev", "hdX" then we will
	// return just the last part
	devicePath = strings.Replace(devicePath, "/host", "", 1)
	parts := strings.Split(devicePath, "/")
	if len(parts) == 3 && strings.HasPrefix(parts[1], "dev") {
		klog.V(4).Infof("found device: %s", parts[2])
		return parts[2], nil
	}
	return "", errors.New("Illegal path for device " + devicePath)
}

func (fc *fcstorage) rescanDeviceMap(volumeId string, lun string) error {
	/**
	defer func() {
		klog.V(4).Infof("rescanDeviceMap() with volume %s and lun %s completed", volumeId, lun)
		klog.Flush()
		// deviceMu.Unlock()
		// May happen if unlocking a mutex that was not locked
		if r := recover(); r != nil {
			err := fmt.Errorf("%v", r)
			klog.V(4).Infof("rescanDeviceMap(), with volume ID '%s' and lun '%s', failed with run-time error: %+v", volumeId, lun, err)
		}
	}()
	*/

	// deviceMu.Lock()
	klog.V(4).Infof("Rescan hosts for volume '%s' and lun '%s'", volumeId, lun)

	fcHosts, err := findHosts("fc")
	if err != nil {
		klog.Error(err)
		return err
	}

	// For each host, scan using lun
	for _, fcHost := range fcHosts {
		scsiHostPath := fmt.Sprintf("/sys/class/scsi_host/host%s/scan", fcHost)
		klog.V(4).Infof("Rescanning host path at '%s' for volume ID '%s' and lun '%s'", scsiHostPath, volumeId, lun)
		_, err = execScsi.Command("echo", fmt.Sprintf("'- - %s' > %s", lun, scsiHostPath))
		if err != nil {
			klog.Errorf("Rescan of host %s failed for volume ID '%s' and lun '%s': %s", scsiHostPath, volumeId, lun, err)
			return err
		}
	}

	for _, fcHost := range fcHosts {
		if err := waitForDeviceState(fcHost, lun, "running"); err != nil {
			return err
		}
	}

	if err := waitForMultipath(fcHosts[0], lun); err != nil {
		klog.V(4).Infof("Rescan hosts failed for volume ID '%s' and lun '%s'", volumeId, lun)
		return err
	}

	klog.V(4).Infof("Rescan hosts complete for volume ID '%s' and lun '%s'", volumeId, lun)
	return err
}

func (fc *fcstorage) searchDisk(c Connector, io ioHandler) (string, error) {
	klog.V(4).Infof("Called searchDisk")
	var diskIds []string // target wwns
	var disk string
	var dm string

	if len(c.TargetWWNs) != 0 {
		diskIds = c.TargetWWNs
	} else {
		diskIds = c.WWIDs
	}

	klog.V(4).Infof("searchDisk rescan scsi host")
	_ = fc.rescanDeviceMap(diskIds[0], c.Lun)

	for _, diskID := range diskIds {
		if len(c.TargetWWNs) != 0 {
			disk, dm = fc.findFcDisk(diskID, c.Lun, io)
		} else {
			disk, dm = fc.getDisksWwids(diskID, io)
		}
		// if multipath device is found, break
		klog.V(4).Infof("searchDisk() found disk '%s' and dm '%s'", disk, dm)
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
		klog.V(4).Infof("Multipath devicemapper device is found %s", disk)
		return dm, nil
	}
	klog.V(4).Infof("Multipath devicemapper device not found, using raw disk %s", disk)
	return disk, nil
}

// find the fc device and device mapper parent
func (fc *fcstorage) findFcDisk(wwn, lun string, io ioHandler) (string, string) {
	klog.V(4).Infof("findFcDisk called with wwn %s and lun %s", wwn, lun)
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
						klog.Errorf("could not find disk with error %v", err2)
					}
				} else {
					klog.Errorf("could not find disk with error %v", err1)
				}
			}
		}
	} else {
		klog.Errorf("could not find disk with error %v", err)
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
					klog.Errorf("fc: failed to find a corresponding disk from symlink[%s], error %v", DevID+name, err)
					return "", ""
				}
				if dm, err1 := fc.findMultipathDeviceForDevice(disk, io); err1 != nil {
					return disk, dm
				}
			}
		}
	}
	klog.Errorf("fc: failed to find a disk [%s]", DevID+FcPath)
	return "", ""
}

// Find and return FC disk
/**
func (fc *fcstorage) getFCDisk2(c Connector, io ioHandler) (string, error) {
	if io == nil {
		io = &OSioHandler{}
	}
	klog.V(4).Infof("getFCDisk() called")
	devicePath, err := fc.searchDisk(c, io)
	if err != nil {
		klog.V(2).Infof("getFCDisk() failed. Unable to find disk given WWNN or WWIDs: %+v", err)
		return "", err
	}
	devicePath = strings.Replace(devicePath, "/host", "", 1)
	klog.V(4).Infof("FC device path %s found", devicePath)

	return devicePath, nil
}
*/

func (fc *fcstorage) createFcConfigFile(conf diskInfo, mnt string) error {
	file := path.Join("/host", mnt, conf.VolName+".json")
	fp, err := os.Create(file)
	if err != nil {
		klog.Errorf("fc: failed creating persist file with error %v", err)
		return fmt.Errorf("fc: create %s err %s", file, err)
	}
	defer fp.Close()
	encoder := json.NewEncoder(fp)
	if err = encoder.Encode(conf); err != nil {
		klog.Errorf("fc: failed creating persist file with error %v", err)
		return fmt.Errorf("fc: encode err: %v", err)
	}
	klog.V(4).Infof("fc: created persist config file at path %s", file)
	return nil
}

func (fc *fcstorage) loadFcDiskInfoFromFile(conf *diskInfo, mnt string) error {
	file := path.Join("/host", mnt, conf.VolName+".json")
	fp, err := os.Open(file)
	if err != nil {
		klog.Error(err)
		return fmt.Errorf("fc: open %s err %s", file, err)
	}
	defer fp.Close()
	decoder := json.NewDecoder(fp)
	if err = decoder.Decode(conf); err != nil {
		klog.Error(err)
		return fmt.Errorf("fc: decode err: %v ", err)
	}
	return nil
}
