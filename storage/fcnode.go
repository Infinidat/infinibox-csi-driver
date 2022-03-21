/*Copyright 2020 Infinidat
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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"infinibox-csi-driver/helper"
	"io/ioutil"
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
	"k8s.io/klog"
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

func (fc *fcstorage) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	klog.V(4).Infof("NodePublishVolume called")
	fcDetails, err := fc.getFCDiskDetails(req)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// Warning: AttachFCDisk attaches nothing. It only finds the device and returns it.
	devicePath, err := fc.AttachFCDisk(*fcDetails.connector, &OSioHandler{})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	diskMounter, err := fc.getFCDiskMounter(req, *fcDetails)
	if err != nil {
		return nil, err
	}
	err = fc.MountFCDisk(*diskMounter, devicePath)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &csi.NodePublishVolumeResponse{}, nil
}

func (fc *fcstorage) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("Recovered from FC NodeUnpublishVolume  " + fmt.Sprint(res))
		}
	}()
	targetPath := req.GetTargetPath()
	if err := fc.DetachFCDisk(targetPath, &OSioHandler{}); err != nil {
		return nil, err
	}
	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (fc *fcstorage) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("Recovered from FC NodeStageVolume  " + fmt.Sprint(res))
		}
	}()
	klog.V(2).Infof("NodeStageVolume called with PublishContext: %v", req.GetPublishContext())
	hostID := req.GetPublishContext()["hostID"]
	ports := req.GetPublishContext()["hostPorts"]

	hstID, _ := strconv.Atoi(hostID)
	klog.V(4).Infof("publishing volume to host id is %s", hostID)
	// validate host exists
	if hstID < 1 {
		klog.Errorf("hostID %d is not valid host ID", hstID)
		return nil, status.Error(codes.Internal, "not a valid host")
	}
	fcPorts := getPortName()
	if len(fcPorts) == 0 {
		klog.Errorf("port name not found on worker")
		return nil, status.Error(codes.Internal, "Port name not found")
	}
	for _, fcp := range fcPorts {
		if !strings.Contains(ports, fcp) {
			klog.V(4).Infof("host port %s is not created, creating it", fcp)
			err = fc.cs.AddPortForHost(hstID, "FC", fcp)
			if err != nil {
				klog.Errorf("error creating host port %v", err)
				return nil, status.Error(codes.Internal, err.Error())
			}
			_, err := fc.cs.api.GetHostPort(hstID, fcp)
			if err != nil {
				klog.Errorf("failed to get host port %s with error %v", fcp, err)
				return nil, status.Error(codes.Internal, err.Error())
			}
		}
	}
	klog.V(4).Infof("NodeStageVolume completed")
	return &csi.NodeStageVolumeResponse{}, nil
}

func (fc *fcstorage) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	klog.V(2).Infof("Called FC NodeUnstageVolume")
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("Recovered from FC NodeUnstageVolume  " + fmt.Sprint(res))
		}
	}()
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
				klog.V(4).Infof("fc config file is not exists")
				if err := os.RemoveAll(stagePath); err != nil {
					klog.Errorf("fc: failed to remove mount path Error: %v", err)
					return nil, err
				}
				klog.V(4).Infof("removed stage path: %s", stagePath)
				return &csi.NodeUnstageVolumeResponse{}, nil
			}
		}
		klog.Warningf("fc detach disk: failed to get fc config from path %s Error: %v", stagePath, err)
	}

	// remove multipath
	var devices []string
	multiPath := false
	dstPath := mpathDevice

	klog.V(4).Infof("removing mpath")
	if strings.HasPrefix(dstPath, "/host") {
		dstPath = strings.Replace(dstPath, "/host", "", 1)
	}

	klog.V(4).Infof("remove multipath device %s", dstPath)
	if strings.HasPrefix(dstPath, "/dev/dm-") {
		multiPath = true
		devices = findSlaveDevicesOnMultipath(dstPath)
	} else {
		// Add single targetPath to devices
		devices = append(devices, dstPath)
	}
	var lastErr error
	for _, device := range devices {
		err := detachDisk(device)
		if err != nil {
			klog.Errorf("fc: detachFCDisk failed. device: %v err: %v", device, err)
			lastErr = fmt.Errorf("fc: detach disk failed. device: %v err: %v", device, err)
		}
	}
	if lastErr != nil {
		klog.Errorf("fc: last error occurred during detach disk:\n%v", lastErr)
		return nil, lastErr
	}
	if multiPath {
		klog.V(4).Infof("flush multipath device using multipath -f: %s", dstPath)
		_, err := fc.cs.ExecuteWithTimeout(4000, "multipath", []string{"-f", dstPath})
		if err != nil {
			if _, e := os.Stat("/host" + dstPath); os.IsNotExist(e) {
				klog.V(4).Infof("multipath device %s deleted", dstPath)
			} else {
				klog.Errorf("multipath -f %s failed to device with error %v", dstPath, err.Error())
				return nil, err
			}
		}
	}
	klog.V(4).Infof("Removed multipath sucessfully!")

	if err := os.RemoveAll("/host" + stagePath); err != nil {
		klog.Errorf("fc: failed to remove mount path Error: %v", err)
		return nil, err
	}
	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (fc *fcstorage) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (
	*csi.NodeGetCapabilitiesResponse, error) {
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
	*csi.NodeGetInfoResponse, error) {
	return &csi.NodeGetInfoResponse{}, nil
}

func (fc *fcstorage) NodeGetVolumeStats(
	ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	return nil, status.Error(codes.Unimplemented, time.Now().String())
}

func (fc *fcstorage) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, time.Now().String())
}

// ------------------------------------ Supporting methods  ---------------------------

func (fc *fcstorage) MountFCDisk(fm FCMounter, devicePath string) error {
	notMnt, err := fm.Mounter.IsLikelyNotMountPoint(fm.TargetPath)
	if err == nil {
		if !notMnt {
			// ToDo: check that it is mounted on the right directory
			klog.V(2).Infof("fc: %s already mounted", fm.TargetPath)
			return nil
		}
	} else if !os.IsNotExist(err) {
		return status.Errorf(codes.Internal, "%s exists but IsLikelyNotMountPoint failed: %v", fm.TargetPath, err)
	}

	if fm.fcDisk.isBlock {
		// option A: raw block volume access
		klog.V(2).Infof("mounting raw block volume at given path %s", fm.TargetPath)
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
		klog.V(2).Infof("mounting volume with filesystem at given path %s", fm.TargetPath)

		// Create mountPoint, with prepended /host, if it does not exist.
		mountPoint := "/host" + fm.TargetPath
		_, err := os.Stat(mountPoint)
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

		err = fm.Mounter.FormatAndMount(devicePath, fm.TargetPath, fm.FsType, options)
		if err != nil {
			klog.V(4).Infof("FormatAndMount returned an error. devicePath: %s, targetPath: %s, fsType: %s, error: %s", devicePath, fm.TargetPath, fm.FsType, err)
			searchAlreadyMounted := fmt.Sprintf("already mounted on %s", mountPoint)
			searchBadSuperBlock := "wrong fs type, bad option, bad superblock"
			klog.V(4).Infof("Search error for matches to handle: %s", err)

			if isAlreadyMounted := strings.Contains(err.Error(), searchAlreadyMounted); isAlreadyMounted {
				klog.Errorf("Device %s is already mounted on %s", devicePath, mountPoint)
			} else if isBadSuperBlock := strings.Contains(err.Error(), searchBadSuperBlock); isBadSuperBlock {
				if err := helper.RegenerateXfsFilesystemUuid(devicePath); err != nil {
					return status.Errorf(codes.Internal, err.Error())
				}
				klog.V(4).Infof("Run FormatAndMount, after UUID change")
				err = fm.Mounter.FormatAndMount(devicePath, mountPoint, fm.FsType, options)
				if err != nil {
					return status.Errorf(codes.Internal, err.Error())
				}
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
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("Recovered from FC getPortName  " + fmt.Sprint(res))
		}
	}()
	ports := []string{}
	cmd := "cat /sys/class/fc_host/host*/port_name"
	out, err := exec.Command("bash", "-c", cmd).Output()
	if err != nil {
		klog.Errorf("Failed to port name with error %v", err)
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
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("Recovered from FC getFCDiskDetails " + fmt.Sprint(res))
		}
	}()
	volproto := strings.Split(req.GetVolumeId(), "$$")
	volName := volproto[0]
	lun := req.GetPublishContext()["lun"]
	wwids := req.GetVolumeContext()["WWIDs"]
	wwidList := strings.Split(wwids, ",")
	targetList := []string{}
	fcNodes, err := fc.cs.api.GetFCPorts()
	if err != nil {
		return nil, fmt.Errorf("Error getting fiber channel details")
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
		Mounter:      &mount.SafeFormatAndMount{Interface: mount.New(""), Exec: utilexec.New()},
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
func (handler *OSioHandler) ReadDir(dirname string) ([]os.FileInfo, error) {
	return ioutil.ReadDir(dirname)
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
	return ioutil.WriteFile(filename, data, perm)
}

// FindMultipathDeviceForDevice given a device name like /dev/sdx, find the devicemapper parent
func (fc *fcstorage) findMultipathDeviceForDevice(device string, io ioHandler) (string, error) {
	klog.V(4).Infof("In findMultipathDeviceForDevice")
	disk, err := fc.findDeviceForPath(device)
	if err != nil {
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

func scsiHostRescan(io ioHandler) {
	scsiPath := "/sys/class/scsi_host/"
	if dirs, errRead := io.ReadDir(scsiPath); errRead == nil {
		for _, f := range dirs {
			name := scsiPath + f.Name() + "/scan"
			data := []byte("- - -")
			if errWrite := io.WriteFile(name, data, 0o666); errWrite != nil {
				klog.Errorf("failed to write rescan cmd to %s", name)
			}
		}
	}
}

func (fc *fcstorage) searchDisk(c Connector, io ioHandler) (string, error) {
	klog.V(4).Infof("Called searchDisk")
	var diskIds []string
	var disk string
	var dm string

	if len(c.TargetWWNs) != 0 {
		diskIds = c.TargetWWNs
	} else {
		diskIds = c.WWIDs
	}

	rescaned := false
	for {
		for _, diskID := range diskIds {
			if len(c.TargetWWNs) != 0 {
				disk, dm = fc.findFcDisk(diskID, c.Lun, io)
			} else {
				disk, dm = fc.getDisksWwids(diskID, io)
			}
			// if multipath device is found, break
			klog.V(4).Infof("searchDisk: found disk %s and dm %s", disk, dm)
			if dm != "" {
				break
			}
		}
		// if a dm is found, exit loop
		if rescaned || dm != "" {
			break
		}
		// rescan and search again
		// rescan scsi bus
		klog.V(4).Infof("searchDisk rescan scsi host")
		scsiHostRescan(io)
		rescaned = true
	}
	// if no disk matches input wwn and lun, exit
	if disk == "" && dm == "" {
		return "", fmt.Errorf("no fc disk found")
	}

	// if multipath devicemapper device is found, use it; otherwise use raw disk
	if dm != "" {
		klog.V(4).Infof("multipath devicemapper device is found")
		return dm, nil
	}
	klog.V(4).Infof("multipath devicemapper device not found, using raw disk")
	return disk, nil
}

// find the fc device and device mapper parent
func (fc *fcstorage) findFcDisk(wwn, lun string, io ioHandler) (string, string) {
	klog.V(4).Infof("In findFcDisk")
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

// Attach attempts to attach a fc volume to a node using the provided Connector info
func (fc *fcstorage) AttachFCDisk(c Connector, io ioHandler) (string, error) {
	if io == nil {
		io = &OSioHandler{}
	}
	klog.V(2).Infof("Attaching fc volume")
	devicePath, err := fc.searchDisk(c, io)
	if err != nil {
		klog.V(2).Infof("unable to find disk given WWNN or WWIDs with error %v", err)
		return "", err
	}
	devicePath = strings.Replace(devicePath, "/host", "", 1)
	klog.V(4).Infof("Attaching fc volume successful, device path %s", devicePath)

	return devicePath, nil
}

// Detach performs a detach operation on a volume
func (fc *fcstorage) DetachFCDisk(targetPath string, io ioHandler) (err error) {
	klog.V(2).Infof("Detaching FC volume, targetpath: %s", targetPath)
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("Recovered from FC DetachFCDisk  " + fmt.Sprint(res))
		}
	}()
	if io == nil {
		io = &OSioHandler{}
	}

	mounter := mount.New("")
	mntPath := path.Join("/host", targetPath)
	mntPathParent := filepath.Dir(mntPath)

	// Unmount volume
	klog.V(4).Infof("Unmounting volume mounted at %s", targetPath)
	if err := mounter.Unmount(targetPath); err != nil {
		if strings.Contains(err.Error(), "not mounted") {
			klog.V(4).Infof("While unmounting, found volume was not mounted to %s", targetPath)
		} else {
			klog.Errorf("Failed unmounting volume from %s, Error: %v", targetPath, err)
			return err
		}
	}

	if err := os.RemoveAll(mntPathParent); err != nil {
		klog.Errorf("Failed to remove mntPathParent %s, Error: %v", mntPathParent, err)
		return err
	}
	klog.V(2).Infof("Volume unmounted successfully")
	return nil
}

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
		return fmt.Errorf("fc: open %s err %s", file, err)
	}
	defer fp.Close()
	decoder := json.NewDecoder(fp)
	if err = decoder.Decode(conf); err != nil {
		return fmt.Errorf("fc: decode err: %v ", err)
	}
	return nil
}
