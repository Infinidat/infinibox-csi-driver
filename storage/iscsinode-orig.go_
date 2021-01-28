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
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	log "infinibox-csi-driver/helper/logger"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/kubernetes/pkg/util/mount"
	"k8s.io/kubernetes/pkg/volume/util"
)

type iscsiDiskUnmounter struct {
	*iscsiDisk
	mounter mount.Interface
	exec    mount.Exec
}
type iscsiDiskMounter struct {
	*iscsiDisk
	readOnly     bool
	fsType       string
	mountOptions []string
	mounter      *mount.SafeFormatAndMount
	exec         mount.Exec
	deviceUtil   util.DeviceUtil
	targetPath   string
	stagePath    string
}
type iscsiDisk struct {
	Portals        []string
	Iqn            string
	lun            string
	Iface          string
	chap_discovery bool
	chap_session   bool
	secret         map[string]string
	InitiatorName  string
	VolName        string
	isBlock        bool
	MpathDevice    string
}

var (
	chap_st = []string{
		"discovery.sendtargets.auth.username",
		"discovery.sendtargets.auth.password",
		"discovery.sendtargets.auth.username_in",
		"discovery.sendtargets.auth.password_in"}
	chap_sess = []string{
		"node.session.auth.username",
		"node.session.auth.password",
		"node.session.auth.username_in",
		"node.session.auth.password_in"}
	ifaceTransportNameRe = regexp.MustCompile(`iface.transport_name = (.*)\n`)
)

// StatFunc stat a path, if not exists, retry maxRetries times
// when iscsi transports other than default are used
type StatFunc func(string) (os.FileInfo, error)

// GlobFunc  use glob instead as pci id of device is unknown
type GlobFunc func(string) ([]string, error)

func (iscsi *iscsistorage) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	log.Debugf("NodePublishVolume called")
	iscsiInfo, err := iscsi.getISCSIInfo(req)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	volCap := req.GetVolumeCapability()
	if volCap == nil {
		return nil, status.Error(codes.InvalidArgument, "Volume capability not provided")
	}
	switch volCap.GetAccessType().(type) {
	case *csi.VolumeCapability_Block:
		iscsiInfo.isBlock = true
	}
	diskMounter := iscsi.getISCSIDiskMounter(iscsiInfo, req)

	_, err = iscsi.AttachDisk(*diskMounter)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func (iscsi *iscsistorage) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("Recovered from ISCSI NodeUnpublishVolume  " + fmt.Sprint(res))
		}
	}()
	diskUnmounter := iscsi.getISCSIDiskUnmounter(req.GetVolumeId())
	targetPath := req.GetTargetPath()

	err = iscsi.DetachDisk(*diskUnmounter, targetPath)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (iscsi *iscsistorage) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("Recovered from ISCSI NodeStageVolume  " + fmt.Sprint(res))
		}
	}()
	log.Info("NodeStageVolume called with ", req.GetPublishContext())
	hostID := req.GetPublishContext()["hostID"]
	ports := req.GetPublishContext()["hostPorts"]
	hostSecurity := req.GetPublishContext()["securityMethod"]
	useChap := req.GetVolumeContext()["useCHAP"]
	hstID, _ := strconv.Atoi(hostID)
	log.Debugf("publishing volume to host id is %s", hostID)
	//validate host exists
	if hstID < 1 {
		log.Errorf("hostID %d is not valid host ID", hstID)
		return &csi.NodeStageVolumeResponse{}, status.Error(codes.Internal, "not a valid host")
	}
	initiatorName := getInitiatorName()
	if initiatorName == "" {
		log.Error("initiator name not found")
		return &csi.NodeStageVolumeResponse{}, status.Error(codes.Internal, "Inititator name not found")
	}
	if !strings.Contains(ports, initiatorName) {
		log.Debug("host port is not created, creating one")
		err = iscsi.cs.AddPortForHost(hstID, "ISCSI", initiatorName)
		if err != nil {
			log.Errorf("error creating host port %v", err)
			return &csi.NodeStageVolumeResponse{}, status.Error(codes.Internal, err.Error())
		}
	}
	log.Debugf("setup chap auth as %s", useChap)
	if strings.ToLower(hostSecurity) != useChap || !strings.Contains(ports, initiatorName) {
		secrets := req.GetSecrets()
		chapCreds := make(map[string]string)
		if useChap != "none" {
			if useChap == "chap" || useChap == "mutual_chap" {
				if secrets["node.session.auth.username"] != "" && secrets["node.session.auth.password"] != "" {
					chapCreds["security_chap_inbound_username"] = secrets["node.session.auth.username"]
					chapCreds["security_chap_inbound_secret"] = secrets["node.session.auth.password"]
					chapCreds["security_method"] = "CHAP"
				} else {
					return &csi.NodeStageVolumeResponse{}, status.Error(codes.Internal, "chap credentials not provided")
				}
			}
			if useChap == "mutual_chap" {
				if secrets["node.session.auth.username_in"] != "" && secrets["node.session.auth.password_in"] != "" && chapCreds["security_method"] == "CHAP" {
					chapCreds["security_chap_outbound_username"] = secrets["node.session.auth.username_in"]
					chapCreds["security_chap_outbound_secret"] = secrets["node.session.auth.password_in"]
					chapCreds["security_method"] = "MUTUAL_CHAP"
				} else {
					return &csi.NodeStageVolumeResponse{}, status.Error(codes.Internal, "mutual chap credentials not provided")
				}
			}
			if len(chapCreds) > 1 {
				log.Debugf("create chap authentication for host %d", hstID)
				err := iscsi.cs.AddChapSecurityForHost(hstID, chapCreds)
				if err != nil {
					return &csi.NodeStageVolumeResponse{}, status.Error(codes.Internal, err.Error())
				}
			}
		} else if hostSecurity != "NONE" {
			log.Debugf("remove chap authentication for host %d", hstID)
			chapCreds["security_method"] = "NONE"
			err := iscsi.cs.AddChapSecurityForHost(hstID, chapCreds)
			if err != nil {
				return &csi.NodeStageVolumeResponse{}, status.Error(codes.Internal, err.Error())
			}
		}
	}
	log.Debug("NodeStageVolume completed")
	return &csi.NodeStageVolumeResponse{}, nil
}
func (iscsi *iscsistorage) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (res *csi.NodeUnstageVolumeResponse, err error) {
	log.Info("Called ISCSI NodeUnstageVolume")
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("Recovered from ISCSI NodeUnstageVolume  " + fmt.Sprint(res))
		}
	}()
	diskUnmounter := iscsi.getISCSIDiskUnmounter(req.GetVolumeId())
	stagePath := req.GetStagingTargetPath()
	var bkpPortal []string
	var volName, iqn, iface, initiatorName, mpathDevice string
	diskConfigFound := true
	found := true

	// load iscsi disk config from json file
	if err := iscsi.loadDiskInfoFromFile(diskUnmounter.iscsiDisk, stagePath); err == nil {
		bkpPortal, iqn, iface, volName, initiatorName, mpathDevice = diskUnmounter.iscsiDisk.Portals, diskUnmounter.iscsiDisk.Iqn, diskUnmounter.iscsiDisk.Iface,
			diskUnmounter.iscsiDisk.VolName, diskUnmounter.iscsiDisk.InitiatorName, diskUnmounter.iscsiDisk.MpathDevice
	} else {
		confFile := path.Join("/host", stagePath, diskUnmounter.iscsiDisk.VolName+".json")
		log.Debug("check if iscsi config file exists")
		pathExist, pathErr := iscsi.cs.pathExists(confFile)
		if pathErr == nil {
			if !pathExist {
				log.Debug("iscsi config file is not exists")
				if err := os.RemoveAll(stagePath); err != nil {
					log.Errorf("iscsi: failed to remove mount path Error: %v", err)
					return nil, err
				}
				log.Debug("removed stage path: ", stagePath)
				return &csi.NodeUnstageVolumeResponse{}, nil
			}
		}
		log.Warnf("iscsi detach disk: failed to get iscsi config from path %s Error: %v", stagePath, err)
		diskConfigFound = false
	}

	if diskConfigFound {
		// disconnecting iscsi session
		log.Debugf("logout session for initiatorName %s, iqn %s, volume id %s", initiatorName, iqn, volName)
		portals := iscsi.removeDuplicate(bkpPortal)
		if len(portals) == 0 {
			return res, fmt.Errorf("iscsi detach disk: failed to detach iscsi disk, Couldn't get connected portals from configurations")
		}

		for _, portal := range portals {
			logoutArgs := []string{"-m", "node", "-p", portal, "-T", iqn, "--logout"}
			deleteArgs := []string{"-m", "node", "-p", portal, "-T", iqn, "-o", "delete"}
			if found {
				logoutArgs = append(logoutArgs, []string{"-I", iface}...)
				deleteArgs = append(deleteArgs, []string{"-I", iface}...)
			}
			out, err := diskUnmounter.exec.Run("iscsiadm", logoutArgs...)
			if err != nil {
				log.Errorf("iscsi: failed to detach disk Error: %s", string(out))
			}
			// Delete the node record
			out, err = diskUnmounter.exec.Run("iscsiadm", deleteArgs...)
			if err != nil {
				log.Errorf("iscsi: failed to delete node record Error: %s", string(out))
			}
		}

		// Delete the iface after all sessions have logged out
		// If the iface is not created via iscsi plugin, skip to delete
		for _, portal := range portals {
			if initiatorName != "" && found && iface == (portal+":"+volName) {
				log.Debugf("Delete the iface %s", iface)
				deleteArgs := []string{"-m", "iface", "-I", iface, "-o", "delete"}
				out, err := diskUnmounter.exec.Run("iscsiadm", deleteArgs...)
				if err != nil {
					log.Errorf("iscsi: failed to delete iface Error: %s", string(out))
				}
				break
			}
		}
		log.Debug("Detach Disk Successfully!")

		// rescan disks
		log.Debug("rescan sessions to discover newly mapped LUNs")
		for _, portal := range portals {
			diskUnmounter.exec.Run("iscsiadm", "-m", "node", "-p", portal, "-T", iqn, "-R")
		}
		log.Debug("Rescan Disk Successfully!")
	}
	// remove multipath
	var devices []string
	multiPath := false
	dstPath := mpathDevice
	if dstPath != "" {
		if strings.HasPrefix(dstPath, "/host") {
			dstPath = strings.Replace(dstPath, "/host", "", 1)
		}

		log.Debugf("remove multipath device %s", dstPath)
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
				log.Errorf("iscsi: detachFCDisk failed. device: %v err: %v", device, err)
				lastErr = fmt.Errorf("iscsi: detachFCDisk failed. device: %v err: %v", device, err)
			}
		}
		if lastErr != nil {
			log.Errorf("iscsi: last error occurred during detach disk:\n%v", lastErr)
			return res, lastErr
		}
		if multiPath {
			log.Debug("flush multipath device using multipath -f ", dstPath)
			_, err := iscsi.cs.ExecuteWithTimeout(4000, "multipath", []string{"-f", dstPath})
			if err != nil {
				if _, e := os.Stat("/host" + dstPath); os.IsNotExist(e) {
					log.Debugf("multipath device %s deleted", dstPath)
				} else {
					log.Errorf("multipath -f %s failed to device with error %v", dstPath, err.Error())
					return res, err
				}
			}
		}
		log.Debug("Removed multipath sucessfully!")
	}
	if err := os.RemoveAll("/host" + stagePath); err != nil {
		log.Errorf("iscsi: failed to remove mount path Error: %v", err)
		return nil, err
	}
	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (iscsi *iscsistorage) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (
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

func (iscsi *iscsistorage) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (
	*csi.NodeGetInfoResponse, error) {
	return &csi.NodeGetInfoResponse{}, nil
}

func (iscsi *iscsistorage) NodeGetVolumeStats(
	ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	return &csi.NodeGetVolumeStatsResponse{}, status.Error(codes.Unimplemented, time.Now().String())

}

func (iscsi *iscsistorage) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	return &csi.NodeExpandVolumeResponse{}, status.Error(codes.Unimplemented, time.Now().String())
}

// ------------------------------------ Supporting methods  ---------------------------

func (iscsi *iscsistorage) AttachDisk(b iscsiDiskMounter) (mntPath string, err error) {
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("Recovered from ISCSI AttachDisk  " + fmt.Sprint(res))
		}
	}()
	var devicePath string
	var devicePaths []string
	var iscsiTransport string
	var lastErr error

	log.Info("Called AttachDisk")
	log.WithFields(log.Fields{"iqn": b.iscsiDisk.Iqn, "lun": b.iscsiDisk.lun,
		"chap_session": b.chap_session}).Info("Mounting Volume")

	if "debug" == log.GetLevel() {
		//iscsi_lib.EnableDebugLogging(log.New().Writer())
	}
	log.Debug("check provided iface is available")
	out, err := b.exec.Run("iscsiadm", "-m", "iface", "-I", b.Iface, "-o", "show")
	if err != nil {
		log.Errorf("iscsi: could not read iface %s error: %s", b.Iface, string(out))
		return "", err
	}

	iscsiTransport = iscsi.extractTransportname(string(out))

	bkpPortal := b.Portals

	// create new iface and copy parameters from pre-configured iface to the created iface
	if b.InitiatorName != "" {
		log.Debug("clone iface to new iface specific to volume")
		// new iface name is <target portal>:<volume name>
		newIface := bkpPortal[0] + ":" + b.VolName
		ifaceAvailable := true
		_, err := b.exec.Run("iscsiadm", "-m", "iface", "-I", newIface, "-o", "show")
		if err != nil {
			ifaceAvailable = false
		}
		if !ifaceAvailable {
			err = iscsi.cloneIface(b, newIface)
			if err != nil {
				log.Errorf("iscsi: failed to clone iface: %s error: %v", b.Iface, err)
				return "", err
			}
		}
		log.Debugf("new iface specific for volume %s is %s", b.VolName, newIface)
		// update iface name
		b.Iface = newIface
	}

	for _, tp := range bkpPortal {
		// Rescan sessions to discover newly mapped LUNs. Do not specify the interface when rescanning
		// to avoid establishing additional sessions to the same target.
		log.Debug("rescan sessions to discover newly mapped LUNs")
		out, err := b.exec.Run("iscsiadm", "-m", "node", "-p", tp, "-T", b.Iqn, "-R")
		if err != nil {
			log.Errorf("iscsi: failed to rescan session with error: %s (%v)", string(out), err)
		}

		if iscsiTransport == "" {
			log.Errorf("iscsi: could not find transport name in iface %s", b.Iface)
			return "", fmt.Errorf("Could not parse iface file for %s", b.Iface)
		}
		if iscsiTransport == "tcp" {
			devicePath = strings.Join([]string{"/host/dev/disk/by-path/ip", tp, "iscsi", b.Iqn, "lun", b.lun}, "-")
		} else {
			devicePath = strings.Join([]string{"/host/dev/disk/by-path/pci", "*", "ip", tp, "iscsi", b.Iqn, "lun", b.lun}, "-")
		}

		if exist := iscsi.waitForPathToExist(&devicePath, 1, iscsiTransport); exist {
			log.Infof("iscsi: devicepath (%s) exists", devicePath)
			devicePaths = append(devicePaths, devicePath)
			continue
		}
		log.Debug("build discoverydb and discover iscsi target")
		// build discoverydb and discover iscsi target
		b.exec.Run("iscsiadm", "-m", "discoverydb", "-t", "sendtargets", "-p", tp, "-I", b.Iface, "-o", "new")
		// update discoverydb with CHAP secret
		err = iscsi.updateISCSIDiscoverydb(b, tp)
		if err != nil {
			lastErr = fmt.Errorf("iscsi: failed to update discoverydb to portal %s error: %v", tp, err)
			continue
		}
		log.Debug("do discovery without chap")
		out, err = b.exec.Run("iscsiadm", "-m", "discoverydb", "-t", "sendtargets", "-p", tp, "-I", b.Iface, "--discover")
		if err != nil {
			// delete discoverydb record
			b.exec.Run("iscsiadm", "-m", "discoverydb", "-t", "sendtargets", "-p", tp, "-I", b.Iface, "-o", "delete")
			lastErr = fmt.Errorf("iscsi: failed to sendtargets to portal %s output: %s, err %v", tp, string(out), err)
			continue
		}

		log.Debug("do session auth with chap")
		err = iscsi.updateISCSINode(b, tp)
		if err != nil {
			// failure to update node db is rare. But deleting record will likely impact those who already start using it.
			lastErr = fmt.Errorf("iscsi: failed to update iscsi node to portal %s error: %v", tp, err)
			continue
		}

		log.Debug(" login to iscsi target")
		// login to iscsi target
		out, err = b.exec.Run("iscsiadm", "-m", "node", "-p", tp, "-T", b.Iqn, "-I", b.Iface, "--login")
		if err != nil {
			// delete the node record from database
			b.exec.Run("iscsiadm", "-m", "node", "-p", tp, "-I", b.Iface, "-T", b.Iqn, "-o", "delete")
			lastErr = fmt.Errorf("iscsi: failed to attach disk: Error: %s (%v)", string(out), err)
			continue
		}
		if exist := iscsi.waitForPathToExist(&devicePath, 10, iscsiTransport); !exist {
			log.Errorf("Could not attach disk: Timeout after 10s")
			// update last error
			lastErr = fmt.Errorf("Could not attach disk: Timeout after 10s")
			continue
		} else {
			devicePaths = append(devicePaths, devicePath)
		}
	}

	if len(devicePaths) == 0 {
		// delete cloned iface
		log.Debug(" device path not found, deleting iface")
		b.exec.Run("iscsiadm", "-m", "iface", "-I", b.Iface, "-o", "delete")
		log.Errorf("iscsi: failed to get any path for iscsi disk, last err seen:\n%v", lastErr)
		return "", fmt.Errorf("failed to get any path for iscsi disk, last err seen:\n%v", lastErr)
	}
	if lastErr != nil {
		log.Errorf("iscsi: last error occurred during iscsi init:\n%v", lastErr)
	}

	// Make sure we use a valid devicepath to find mpio device.
	devicePath = devicePaths[0]
	mntPath = b.targetPath
	// Mount device
	notMnt, err := b.mounter.IsLikelyNotMountPoint(mntPath)
	if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("Heuristic determination of mount point failed:%v", err)
	}
	if !notMnt {
		log.Infof("iscsi: %s already mounted", mntPath)
		return "", nil
	}

	for _, path := range devicePaths {
		if path == "" {
			continue
		}
		// check if the dev is using mpio and if so mount it via the dm-XX device
		if mappedDevicePath := iscsi.findMultipathDeviceForDevice(path); mappedDevicePath != "" {
			devicePath = mappedDevicePath
			b.iscsiDisk.MpathDevice = mappedDevicePath
			break
		}
	}

	if b.isBlock {
		log.Debugf("Block volume will be mount at file %s", b.targetPath)
		if b.readOnly {
			return "", status.Error(codes.Internal, "Read only is not supported for Block Volume")
		}

		if err := os.MkdirAll(filepath.Dir(b.targetPath), 0750); err != nil {
			log.Errorf("iscsi: failed to mkdir %s, error", filepath.Dir(b.targetPath))
			return "", err
		}

		_, err = os.Create("/host/" + b.targetPath)
		if err != nil {
			log.Errorf("failed to create target file %q: %v", b.targetPath, err)
			return "", fmt.Errorf("failed to create target file for raw block bind mount: %v", err)
		}
		devicePath = strings.Replace(devicePath, "/host", "", 1)
		options := []string{"bind"}
		options = append(options, "rw")
		if err := b.mounter.Mount(devicePath, b.targetPath, "", options); err != nil {
			log.Errorf("iscsi: failed to mount iscsi volume %s [%s] to %s, error %v", devicePath, b.fsType, b.targetPath, err)
			return "", err
		}
		if err := iscsi.createISCSIConfigFile(*(b.iscsiDisk), b.stagePath); err != nil {
			log.Errorf("iscsi: failed to save iscsi config with error: %v", err)
			return "", err
		}
		log.Debug("Block volume mounted successfully")
		return devicePath, err
	} else {
		log.Debugf("mount volume to given path %s", b.targetPath)
		if err := os.MkdirAll(mntPath, 0750); err != nil {
			log.Errorf("iscsi: failed to mkdir %s, error", mntPath)
			return "", err
		}

		var options []string

		if b.readOnly {
			options = append(options, "ro")
		} else {
			options = append(options, "rw")
		}
		options = append(options, b.mountOptions...)

		log.Debug("devicePath is ", devicePath)
		log.Debug("format (if needed) and mount volume")
		devicePath = strings.Replace(devicePath, "/host", "", 1)
		err = b.mounter.FormatAndMount(devicePath, mntPath, b.fsType, options)
		if err != nil {
			log.Errorf("iscsi: failed to mount iscsi volume %s [%s] to %s, error %v", devicePath, b.fsType, mntPath, err)
			return "", err
		}
		log.Debug("Persist iscsi disk config to json file for DetachDisk path")
		if err := iscsi.createISCSIConfigFile(*(b.iscsiDisk), b.stagePath); err != nil {
			log.Errorf("iscsi: failed to save iscsi config with error: %v", err)
			return "", err
		}
	}
	log.Debugf("mounted volume successfully at %s", mntPath)
	return devicePath, err

}

func (iscsi *iscsistorage) DetachDisk(c iscsiDiskUnmounter, targetPath string) (err error) {
	log.Debugf("Called DetachDisk targetpath: %s", targetPath)
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("Recovered from ISCSI DetachDisk  " + fmt.Sprint(res))
		}
	}()
	mntPath := path.Join("/host", targetPath)
	if pathExist, pathErr := iscsi.pathExists(targetPath); pathErr != nil {
		return fmt.Errorf("Error checking if path exists: %v", pathErr)
	} else if !pathExist {
		if pathExist, _ = iscsi.pathExists(mntPath); pathErr == nil {
			if !pathExist {
				log.Warnf("Warning: Unmount skipped because path does not exist: %v", targetPath)
				return nil
			}
		}
	}
	log.Debug("unmout volume from tagetpath ", targetPath)
	if err = c.mounter.Unmount(targetPath); err != nil {
		if strings.Contains(err.Error(), "not mounted") {
			log.Debug("volume not mounted removing files ", targetPath)
			if err := os.RemoveAll(filepath.Dir(mntPath)); err != nil {
				log.Errorf("iscsi: failed to remove mount path Error: %v", err)
			}
			return nil
		}
		log.Errorf("iscsi detach disk: failed to unmount: %s\nError: %v", targetPath, err)
		return err
	}
	if err := os.RemoveAll(filepath.Dir(mntPath)); err != nil {
		log.Errorf("iscsi: failed to remove mount path Error: %v", err)
		return err
	}
	log.Debug("Unmout volume successfully!")
	return nil
}

func portalMounter(portal string) string {
	if !strings.Contains(portal, ":") {
		portal = portal + ":3260"
	}
	return portal
}

func getInitiatorName() string {
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("Recovered from ISCSI getInitiatorName  " + fmt.Sprint(res))
		}
	}()
	cmd := "cat /etc/iscsi/initiatorname.iscsi | grep InitiatorName="
	out, err := exec.Command("bash", "-c", cmd).Output()
	if err != nil {
		log.Errorf("Failed to get initiator name with error %v", err)
		return ""
	}
	initiatorName := string(out)
	initiatorName = strings.TrimSuffix(initiatorName, "\n")
	log.Debugf("host initiator name %s ", initiatorName)
	arr := strings.Split(initiatorName, "=")
	return arr[1]
}

func (iscsi *iscsistorage) getISCSIInfo(req *csi.NodePublishVolumeRequest) (*iscsiDisk, error) {
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("Recovered from ISCSI getISCSIInfo  " + fmt.Sprint(res))
		}
	}()
	log.Debug("Called getISCSIInfo")
	initiatorName := getInitiatorName()

	useChap := req.GetVolumeContext()["useCHAP"]
	chapSession := false
	if useChap != "none" {
		chapSession = true
	}
	chapDiscovery := false
	if req.GetVolumeContext()["discoveryCHAPAuth"] == "true" {
		chapDiscovery = true
	}
	secret := req.GetSecrets()
	if chapSession {
		secret, err = iscsi.parseSessionSecret(useChap, secret)
		//secret["node.session.auth.username"] = initiatorName
		if err != nil {
			return nil, err
		}
	}

	volproto := strings.Split(req.GetVolumeId(), "$$")
	volName := volproto[0]
	iqn := req.GetVolumeContext()["iqn"]
	lun := req.GetPublishContext()["lun"]
	portals := req.GetVolumeContext()["portals"]
	portalList := strings.Split(portals, ",")

	if len(portalList) == 0 || iqn == "" || lun == "" {
		return nil, fmt.Errorf("iSCSI target information is missing")
	}
	bkportal := []string{}
	for _, portal := range portalList {
		bkportal = append(bkportal, portalMounter(string(portal)))
	}
	return &iscsiDisk{
		VolName:        volName,
		Portals:        bkportal,
		Iqn:            iqn,
		lun:            lun,
		Iface:          "default",
		chap_discovery: chapDiscovery,
		chap_session:   chapSession,
		secret:         secret,
		InitiatorName:  initiatorName}, nil
}

func (iscsi *iscsistorage) getISCSIDiskMounter(iscsiInfo *iscsiDisk, req *csi.NodePublishVolumeRequest) *iscsiDiskMounter {
	fstype := req.GetVolumeContext()["fstype"]
	mountOptions := req.GetVolumeCapability().GetMount().GetMountFlags()
	return &iscsiDiskMounter{
		iscsiDisk:    iscsiInfo,
		fsType:       fstype,
		readOnly:     false,
		mountOptions: mountOptions,
		mounter:      &mount.SafeFormatAndMount{Interface: mount.New(""), Exec: mount.NewOsExec()},
		exec:         mount.NewOsExec(),
		targetPath:   req.GetTargetPath(),
		stagePath:    req.GetStagingTargetPath(),
		deviceUtil:   util.NewDeviceHandler(util.NewIOHandler()),
	}
}

func (iscsi *iscsistorage) getISCSIDiskUnmounter(volumeID string) *iscsiDiskUnmounter {
	volproto := strings.Split(volumeID, "$$")
	volName := volproto[0]
	return &iscsiDiskUnmounter{
		iscsiDisk: &iscsiDisk{
			VolName: volName,
		},
		mounter: mount.New(""),
		exec:    mount.NewOsExec(),
	}
}

func (iscsi *iscsistorage) parseSessionSecret(useChap string, secretParams map[string]string) (map[string]string, error) {
	var ok bool
	secret := make(map[string]string)

	if useChap == "chap" || useChap == "mutual_chap" {
		if len(secretParams) == 0 {
			return secret, errors.New("required chap secrets not provided")
		}
		if secret["node.session.auth.username"], ok = secretParams["node.session.auth.username"]; !ok {
			return secret, fmt.Errorf("node.session.auth.username not found in secret")
		}
		if secret["node.session.auth.password"], ok = secretParams["node.session.auth.password"]; !ok {
			return secret, fmt.Errorf("node.session.auth.password not found in secret")
		}
		if useChap == "mutual_chap" {
			if secret["node.session.auth.username_in"], ok = secretParams["node.session.auth.username_in"]; !ok {
				return secret, fmt.Errorf("node.session.auth.username_in not found in secret")
			}
			if secret["node.session.auth.password_in"], ok = secretParams["node.session.auth.password_in"]; !ok {
				return secret, fmt.Errorf("node.session.auth.password_in not found in secret")
			}
		}
		secret["SecretsType"] = "chap"
	}
	return secret, nil
}

func (iscsi *iscsistorage) updateISCSIDiscoverydb(b iscsiDiskMounter, tp string) error {
	if !b.chap_discovery {
		log.Debug("chap discovery is not allowed")
		return nil
	}
	out, err := b.exec.Run("iscsiadm", "-m", "discoverydb", "-t", "sendtargets", "-p", tp, "-I", b.Iface, "-o", "update", "-n", "discovery.sendtargets.auth.authmethod", "-v", "CHAP")
	if err != nil {
		return fmt.Errorf("iscsi: failed to update discoverydb with CHAP, output: %v", string(out))
	}

	for _, k := range chap_st {
		v := b.secret[k]
		if len(v) > 0 {
			out, err := b.exec.Run("iscsiadm", "-m", "discoverydb", "-t", "sendtargets", "-p", tp, "-I", b.Iface, "-o", "update", "-n", k, "-v", v)
			if err != nil {
				return fmt.Errorf("iscsi: failed to update discoverydb key %q with value %q error: %v", k, v, string(out))
			}
		}
	}
	return nil
}

func (iscsi *iscsistorage) updateISCSINode(b iscsiDiskMounter, tp string) error {
	if !b.chap_session {
		return nil
	}

	out, err := b.exec.Run("iscsiadm", "-m", "node", "-p", tp, "-T", b.Iqn, "-I", b.Iface, "-o", "update", "-n", "node.session.auth.authmethod", "-v", "CHAP")
	if err != nil {
		return fmt.Errorf("iscsi: failed to update node with CHAP, output: %v", string(out))
	}

	for _, k := range chap_sess {
		v := b.secret[k]
		if len(v) > 0 {
			out, err := b.exec.Run("iscsiadm", "-m", "node", "-p", tp, "-T", b.Iqn, "-I", b.Iface, "-o", "update", "-n", k, "-v", v)
			if err != nil {
				return fmt.Errorf("iscsi: failed to update node session key %q with value %q error: %v", k, v, string(out))
			}
		}
	}
	return nil
}

func (iscsi *iscsistorage) waitForPathToExist(devicePath *string, maxRetries int, deviceTransport string) bool {
	// This makes unit testing a lot easier
	return iscsi.waitForPathToExistInternal(devicePath, maxRetries, deviceTransport, os.Stat, filepath.Glob)
}

func (iscsi *iscsistorage) waitForPathToExistInternal(devicePath *string, maxRetries int, deviceTransport string, osStat StatFunc, filepathGlob GlobFunc) bool {
	if devicePath == nil {
		return false
	}

	for i := 0; i < maxRetries; i++ {
		var err error
		if deviceTransport == "tcp" {
			_, err = osStat(*devicePath)
		} else {
			fpath, _ := filepathGlob(*devicePath)
			if fpath == nil {
				err = os.ErrNotExist
			} else {
				// There might be a case that fpath contains multiple device paths if
				// multiple PCI devices connect to same iscsi target. We handle this
				// case at subsequent logic. Pick up only first path here.
				*devicePath = fpath[0]
			}
		}
		if err == nil {
			return true
		}
		if !os.IsNotExist(err) {
			return false
		}
		if i == maxRetries-1 {
			break
		}
		time.Sleep(time.Second)
	}
	return false
}

func (iscsi *iscsistorage) createISCSIConfigFile(conf iscsiDisk, mnt string) error {
	file := path.Join("/host", mnt, conf.VolName+".json")
	log.Debugf("persistISCSI: creating persist file at path %s", file)
	fp, err := os.Create(file)
	if err != nil {
		log.Errorf("persistISCSI: failed creating persist file with error %v", err)
		return fmt.Errorf("iscsi: create %s err %s", file, err)
	}
	defer fp.Close()
	encoder := json.NewEncoder(fp)
	if err = encoder.Encode(conf); err != nil {
		log.Errorf("persistISCSI: failed creating persist file with error %v", err)
		return fmt.Errorf("iscsi: encode err: %v", err)
	}
	return nil
}

func (iscsi *iscsistorage) loadDiskInfoFromFile(conf *iscsiDisk, mnt string) error {
	file := path.Join("/host", mnt, conf.VolName+".json")
	fp, err := os.Open(file)
	if err != nil {
		return fmt.Errorf("iscsi: open %s err %s", file, err)
	}
	defer fp.Close()
	decoder := json.NewDecoder(fp)
	if err = decoder.Decode(conf); err != nil {
		return fmt.Errorf("iscsi: decode err: %v ", err)
	}
	return nil
}

func (iscsi *iscsistorage) isCorruptedMnt(err error) bool {
	if err == nil {
		return false
	}
	var underlyingError error
	switch pe := err.(type) {
	case nil:
		return false
	case *os.PathError:
		underlyingError = pe.Err
	case *os.LinkError:
		underlyingError = pe.Err
	case *os.SyscallError:
		underlyingError = pe.Err
	}

	return underlyingError == syscall.ENOTCONN || underlyingError == syscall.ESTALE || underlyingError == syscall.EIO
}
func (iscsi *iscsistorage) pathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		log.Debug("Path exists: ", path)
		return true, nil
	} else if os.IsNotExist(err) {
		log.Debug("Path not exists: ", path)
		return false, nil
	} else if iscsi.isCorruptedMnt(err) {
		log.Debug("Path is currupted: ", path)
		return true, err
	} else {
		log.Debug("unable to validate path: ", path)
		return false, err
	}
}

func (iscsi *iscsistorage) extractTransportname(ifaceOutput string) (iscsiTransport string) {
	rexOutput := ifaceTransportNameRe.FindStringSubmatch(ifaceOutput)
	if rexOutput == nil {
		return ""
	}
	iscsiTransport = rexOutput[1]

	// While iface.transport_name is a required parameter, handle it being unspecified anyways
	if iscsiTransport == "<empty>" {
		iscsiTransport = "tcp"
	}
	return iscsiTransport
}

// Remove duplicates or string
func (iscsi *iscsistorage) removeDuplicate(s []string) []string {
	m := map[string]bool{}
	for _, v := range s {
		if v != "" && !m[v] {
			s[len(m)] = v
			m[v] = true
		}
	}
	s = s[:len(m)]
	return s
}

func (iscsi *iscsistorage) parseIscsiadmShow(output string) (map[string]string, error) {
	params := make(map[string]string)
	slice := strings.Split(output, "\n")
	for _, line := range slice {
		if !strings.HasPrefix(line, "iface.") || strings.Contains(line, "<empty>") {
			continue
		}
		iface := strings.Fields(line)
		if len(iface) != 3 || iface[1] != "=" {
			return nil, fmt.Errorf("Error: invalid iface setting: %v", iface)
		}
		// iscsi_ifacename is immutable once the iface is created
		if iface[0] == "iface.iscsi_ifacename" {
			continue
		}
		params[iface[0]] = iface[2]
	}
	return params, nil
}

func (iscsi *iscsistorage) cloneIface(b iscsiDiskMounter, newIface string) error {
	var lastErr error
	// get pre-configured iface records
	out, err := b.exec.Run("iscsiadm", "-m", "iface", "-I", b.Iface, "-o", "show")
	if err != nil {
		lastErr = fmt.Errorf("iscsi: failed to show iface records: %s (%v)", string(out), err)
		return lastErr
	}
	// parse obtained records
	params, err := iscsi.parseIscsiadmShow(string(out))
	if err != nil {
		lastErr = fmt.Errorf("iscsi: failed to parse iface records: %s (%v)", string(out), err)
		return lastErr
	}
	// update initiatorname
	params["iface.initiatorname"] = b.InitiatorName
	// create new iface
	out, err = b.exec.Run("iscsiadm", "-m", "iface", "-I", newIface, "-o", "new")
	if err != nil {
		lastErr = fmt.Errorf("iscsi: failed to create new iface: %s (%v)", string(out), err)
		return lastErr
	}
	// update new iface records
	for key, val := range params {
		_, err = b.exec.Run("iscsiadm", "-m", "iface", "-I", newIface, "-o", "update", "-n", key, "-v", val)
		if err != nil {
			b.exec.Run("iscsiadm", "-m", "iface", "-I", newIface, "-o", "delete")
			lastErr = fmt.Errorf("iscsi: failed to update iface records: %s (%v). iface(%s) will be used", string(out), err, b.Iface)
			break
		}
	}
	return lastErr
}

// FindMultipathDeviceForDevice given a device name like /dev/sdx, find the devicemapper parent
func (iscsi *iscsistorage) findMultipathDeviceForDevice(device string) string {
	disk, err := findDeviceForPath(device)
	if err != nil {
		return ""
	}
	sysPath := "/sys/block/"
	if dirs, err := ioutil.ReadDir(sysPath); err == nil {
		for _, f := range dirs {
			name := f.Name()
			if strings.HasPrefix(name, "dm-") {
				if _, err1 := os.Lstat(sysPath + name + "/slaves/" + disk); err1 == nil {
					return "/dev/" + name
				}
			}
		}
	} else {
		log.Errorf("failed to find multipath device with error %v", err)
	}
	return ""
}

func findDeviceForPath(path string) (string, error) {
	devicePath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	// if path /dev/hdX split into "", "dev", "hdX" then we will
	// return just the last part
	devicePath = strings.Replace(devicePath, "/host", "", 1)
	parts := strings.Split(devicePath, "/")
	if len(parts) == 3 && strings.HasPrefix(parts[1], "dev") {
		return parts[2], nil
	}
	return "", errors.New("Illegal path for device " + devicePath)
}
