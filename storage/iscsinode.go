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

	"infinibox-csi-driver/helper"
	log "infinibox-csi-driver/helper/logger"
	"k8s.io/klog"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/containerd/containerd/snapshots/devmapper/dmsetup"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/kubernetes/pkg/volume/util"
	utilexec "k8s.io/utils/exec"
	"k8s.io/utils/mount"
)

const (
	devMapperDir string = dmsetup.DevMapperDir // ie /dev/mapper/
)

type iscsiDiskUnmounter struct {
	*iscsiDisk
	mounter mount.Interface
	exec    utilexec.Interface //mount.Exec
}
type iscsiDiskMounter struct {
	*iscsiDisk
	readOnly     bool
	fsType       string
	mountOptions []string
	mounter      *mount.SafeFormatAndMount
	exec         utilexec.Interface
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

// Global resouce contains a sync.Mutex. Used to serialize iSCSI resource accesses.
var execScsi helper.ExecScsi

var volumeIdCache string

// StatFunc stat a path, if not exists, retry maxRetries times
// when iscsi transports other than default are used
type StatFunc func(string) (os.FileInfo, error)

// GlobFunc  use glob instead as pci id of device is unknown
type GlobFunc func(string) ([]string, error)

func (iscsi *iscsistorage) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	var err error
	err = nil
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("iscsi: Recovered from ISCSI NodeStageVolume  " + fmt.Sprint(res))
		}
		if err == nil {
			klog.V(4).Infof("NodeStageVolume completed")
		} else {
			klog.V(4).Infof("NodeStageVolume failed. Clearing volumeIdCache.")
			volumeIdCache = ""
		}
	}()
	klog.V(2).Infof("NodeStageVolume called with publish context: %s", req.GetPublishContext())

	if len(volumeIdCache) != 0 && volumeIdCache != req.GetVolumeId() {
		msg := fmt.Sprintf("NodeStageVolume ABORT: volume '%s'", req.GetVolumeId())
		klog.V(2).Infof(msg)
		return &csi.NodeStageVolumeResponse{}, status.Error(codes.Internal, msg)
	} else if len(volumeIdCache) != 0 {
		klog.V(2).Infof("NodeStageVolume volume '%s' is being serviced (again)", req.GetVolumeId())
	} else {
		klog.V(2).Infof("NodeStageVolume volume '%s' is being serviced", req.GetVolumeId())
	}
	volumeIdCache = req.GetVolumeId()

	hostIDString := req.GetPublishContext()["hostID"]
	hostID, err := strconv.Atoi(hostIDString)
	if err != nil {
		klog.Errorf("hostID string %s is not valid host ID: %s", hostIDString, err)
		return &csi.NodeStageVolumeResponse{}, status.Error(codes.Internal, "not a valid host")
	}
	ports := req.GetPublishContext()["hostPorts"]
	hostSecurity := req.GetPublishContext()["securityMethod"]
	useChap := req.GetVolumeContext()["useCHAP"]
	klog.V(4).Infof("Publishing volume to host with hostID %d", hostID)

	//validate host exists
	if hostID < 1 {
		klog.Errorf("hostID %d is not valid host ID", hostID)
		return &csi.NodeStageVolumeResponse{}, status.Error(codes.Internal, "not a valid host")
	}
	initiatorName := getInitiatorName()
	if initiatorName == "" {
		msg := "Initiator name not found"
		klog.Errorf(msg)
		return &csi.NodeStageVolumeResponse{}, status.Error(codes.Internal, "iscsi: "+msg)
	}
	if !strings.Contains(ports, initiatorName) {
		klog.V(4).Infof("Host port is not created, creating one")
		err = iscsi.cs.AddPortForHost(hostID, "ISCSI", initiatorName)
		if err != nil {
			klog.Errorf("Error creating host port %v", err)
			return &csi.NodeStageVolumeResponse{}, status.Error(codes.Internal, err.Error())
		}
	}
	klog.V(4).Infof("Setup chap auth as '%s'", useChap)
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
					msg := "Mutual chap credentials not provided"
					klog.V(4).Infof(msg)
					return &csi.NodeStageVolumeResponse{}, status.Error(codes.Internal, "iscsi: "+msg)
				}
			}
			if useChap == "mutual_chap" {
				if secrets["node.session.auth.username_in"] != "" && secrets["node.session.auth.password_in"] != "" && chapCreds["security_method"] == "CHAP" {
					chapCreds["security_chap_outbound_username"] = secrets["node.session.auth.username_in"]
					chapCreds["security_chap_outbound_secret"] = secrets["node.session.auth.password_in"]
					chapCreds["security_method"] = "MUTUAL_CHAP"
				} else {
					msg := "Mutual chap credentials not provided"
					klog.V(4).Infof(msg)
					return &csi.NodeStageVolumeResponse{}, status.Error(codes.Internal, "iscsi: "+msg)
				}
			}
			if len(chapCreds) > 1 {
				klog.V(4).Infof("Create chap authentication for host %d", hostID)
				err := iscsi.cs.AddChapSecurityForHost(hostID, chapCreds)
				if err != nil {
					return &csi.NodeStageVolumeResponse{}, status.Error(codes.Internal, err.Error())
				}
			}
		} else if hostSecurity != "NONE" {
			klog.V(4).Infof("Remove chap authentication for host %d", hostID)
			chapCreds["security_method"] = "NONE"
			err := iscsi.cs.AddChapSecurityForHost(hostID, chapCreds)
			if err != nil {
				return &csi.NodeStageVolumeResponse{}, status.Error(codes.Internal, err.Error())
			}
		}
	}

	return &csi.NodeStageVolumeResponse{}, nil
}

func (iscsi *iscsistorage) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	var err error
	err = nil
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("iscsi: Recovered from ISCSI NodePublishVolume  " + fmt.Sprint(res))
		}
		if err == nil {
			klog.V(4).Infof("NodePublishVolume completed. Clearing volumeIdCache.")
		} else {
			klog.V(4).Infof("NodePublishVolume failed. Clearing volumeIdCache.")
		}
		volumeIdCache = ""
	}()

	klog.V(4).Infof("NodePublishVolume called")
	klog.V(4).Infof("Publishing volume ID '%s'", req.VolumeId)
	//helper.PrettyKlogDebug("NodePublishVolume req: ", req)

	iscsiInfo, err := iscsi.getISCSIInfo(req)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	volCap := req.GetVolumeCapability()
	if volCap == nil {
		err = errors.New("GetVolumeCapability failed")
		return nil, status.Error(codes.InvalidArgument, "iscsi: Volume capability not provided")
	}
	switch volCap.GetAccessType().(type) {
	case *csi.VolumeCapability_Block:
		iscsiInfo.isBlock = true
	}
	diskMounter := iscsi.getISCSIDiskMounter(iscsiInfo, req)

	_, err = iscsi.AttachDisk(*diskMounter)
	if err != nil {
		klog.Errorf("AttachDisk failed")
		//rescanDeviceMap(req.VolumeId)
		return nil, status.Error(codes.Internal, err.Error())
	} else {
		klog.Errorf("AttachDisk succeeded")
	}

	klog.V(4).Infof("NodePublishVolume succeeded.")
	helper.PrettyKlogDebug("NodePublishVolume returning csi.NodePublishVolumeResponse:", csi.NodePublishVolumeResponse{})

	return &csi.NodePublishVolumeResponse{}, nil
}

func (iscsi *iscsistorage) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	var err error
	err = nil
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("iscsi: Recovered from ISCSI NodeUnpublishVolume  " + fmt.Sprint(res))
		}
		if err == nil {
			klog.V(4).Infof("NodeUnpublishVolume completed")
		} else {
			klog.V(4).Infof("NodeUnpublishVolume failed. Clearing volumeIdCache.")
			volumeIdCache = ""
		}
	}()
	klog.V(4).Infof("NodeUnpublishVolume called")
	diskUnmounter := iscsi.getISCSIDiskUnmounter(req.GetVolumeId())
	targetPath := req.GetTargetPath()

	err = iscsi.DetachDisk(*diskUnmounter, targetPath)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (iscsi *iscsistorage) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (res *csi.NodeUnstageVolumeResponse, err error) {
	err = nil
	klog.V(2).Infof("Called NodeUnstageVolume")
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("iscsi: Recovered from NodeUnstageVolume  " + fmt.Sprint(res))
		}
		if err == nil {
			klog.V(4).Infof("NodeUnstageVolume completed. Clearing volumeIdCache.")
		} else {
			klog.V(4).Infof("NodeUnstageVolume failed. Clearing volumeIdCache.")
		}
		volumeIdCache = ""
	}()
	diskUnmounter := iscsi.getISCSIDiskUnmounter(req.GetVolumeId())
	stagePath := req.GetStagingTargetPath()
	var bkpPortal []string
	var volName, iqn, iface, initiatorName, mpathDevice string
	klog.V(2).Infof("%s %s %s %s %s", volName, iqn, iface, initiatorName, mpathDevice)

	// Load iscsi disk config from json file
	diskConfigFound := true
	if err := iscsi.loadDiskInfoFromFile(diskUnmounter.iscsiDisk, stagePath); err == nil {
		bkpPortal, iqn, iface, volName, initiatorName, mpathDevice = diskUnmounter.iscsiDisk.Portals, diskUnmounter.iscsiDisk.Iqn, diskUnmounter.iscsiDisk.Iface,
			diskUnmounter.iscsiDisk.VolName, diskUnmounter.iscsiDisk.InitiatorName, diskUnmounter.iscsiDisk.MpathDevice
	} else {
		confFile := path.Join("/host", stagePath, diskUnmounter.iscsiDisk.VolName+".json")
		klog.V(4).Infof("Check if config file exists")
		pathExist, pathErr := iscsi.cs.pathExists(confFile)
		if pathErr == nil {
			if !pathExist {
				klog.V(4).Infof("Config file does not exist")
				if err := os.RemoveAll(stagePath); err != nil {
					klog.Errorf("Failed to remove mount path Error: %v", err)
					return nil, err
				}
				klog.V(4).Infof("Removed stage path: ", stagePath)
				return &csi.NodeUnstageVolumeResponse{}, nil
			}
		}
		klog.Warningf("detach disk: failed to get iscsi config from path %s Error: %v", stagePath, err)
		diskConfigFound = false
	}
	err = nil

	if diskConfigFound {
		// Disconnecting iscsi session
		klog.V(4).Infof("Logout session")
		portals := iscsi.removeDuplicate(bkpPortal)
		klog.V(4).Infof("Detach Disk Successfully!")

		// rescan disks
		klog.V(4).Infof("Rescan sessions to discover newly mapped LUNs")
		for _, portal := range portals {
			klog.V(4).Infof("Rescan node")
			_, err := execScsi.Command(fmt.Sprintf("iscsiadm --mode node --portal %s --targetname %s --rescan", portal, iqn))
			if err != nil {
				klog.Errorf("Failed to rescan node with portal '%s' and targetname '%s': %s", portal, iqn, err)
				err = nil
			}
		}
		klog.V(4).Infof("Successfully Rescanned disk")
	}

	// remove multipath
	var devices []string
	multiPath := false
	dstPath := mpathDevice

	if dstPath != "" {
		if strings.HasPrefix(dstPath, "/host") {
			dstPath = strings.Replace(dstPath, "/host", "", 1)
		}

		if strings.HasPrefix(dstPath, "/dev/dm-") {
			multiPath = true
			devices = findSlaveDevicesOnMultipath(dstPath)
		} else {
			// Add single targetPath to devices
			devices = append(devices, dstPath)
		}
		helper.PrettyKlogDebug("multipath devices", devices)

		// blockdev --flushbufs
		if multiPath {
			// TODO: https://access.redhat.com/documentation/en-us/red_hat_enterprise_linux/6/html/storage_administration_guide/removing_devices
			// Step: 5
			for _, device := range devices {
				klog.V(4).Infof("Flush device '%s'", device)
				var blockdevOut string
				blockdevOut, err = execScsi.Command(fmt.Sprintf("blockdev --flushbufs %s", device))
				if err != nil {
					klog.V(4).Infof("blockdev --flushbufs failed: %s", err)
					return res, err
				}
				klog.V(4).Infof("Flush device '%s' output: %s", device, blockdevOut)
			}
			klog.V(4).Infof("Flush blockdev succeeded")
		}

		// detachDisk() - Echo 1  to delete devices
		err := deleteMultipathDevices(devices)
		if err != nil {
			klog.V(4).Infof("deleteMultipathDevices failed: %s", err)
			return res, err
		}
		//deleteMultipathMap(dstPath)

		var lastErr error
		for _, device := range devices {
			err := detachDisk(device)
			if err != nil {
				klog.Errorf("detachDisk failed. device: %v err: %v", device, err)
				lastErr = fmt.Errorf("iscsi: detachDisk failed. device: %v err: %v", device, err)
			}
		}
		if lastErr != nil {
			klog.Errorf("Last error occurred during detach disk:\n%v", lastErr)
			return res, lastErr
		}
		klog.V(4).Infof("detachDisk() succeeded")

		// Multipath -f
		// TODO Fix next
		multiPath = false
		if multiPath {
			klog.V(4).Infof("Flush multipath device '%s'", dstPath)
			_, err := execScsi.Command(fmt.Sprintf("multipath -f %s", dstPath))
			if err != nil {
				if _, e := os.Stat("/host" + dstPath); os.IsNotExist(e) {
					klog.V(4).Infof("multipath device %s deleted", dstPath)
				} else {
					klog.Errorf("multipath -f %s failed to flush device with error %v", dstPath, err.Error())
					return res, err
				}
			}
			klog.V(4).Infof("Flush multipath succeeded for '%s'", dstPath)

			// TODO: https://access.redhat.com/documentation/en-us/red_hat_enterprise_linux/6/html/storage_administration_guide/removing_devices
			// Step: 5
			for _, device := range devices {
				klog.V(4).Infof("Flush device '%s'", device)
				blockdevOut, blockdevErr := execScsi.Command(fmt.Sprintf("blockdev --flushbufs %s", device))
				if blockdevErr != nil {
					klog.V(4).Infof("blockdev --flushbufs failed: %s", blockdevErr)
					return res, blockdevErr
				}
				klog.V(4).Infof("Flush device '%s' output: %s", device, blockdevOut)
			}
			klog.V(4).Infof("Flush blockdev succeeded")
		}
		klog.V(4).Infof("Removed multipath sucessfully")

	}
	if err := os.RemoveAll("/host" + stagePath); err != nil {
		klog.Errorf("Failed to remove mount path Error: %v", err)
		return nil, err
	}

	klog.V(2).Infof("NodeUnstageVolume is removing %s", req.GetVolumeId())
	volumeIdCache = ""

	klog.V(2).Infof("NodeUnstageVolume returned")
	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (iscsi *iscsistorage) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
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

func (iscsi *iscsistorage) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	return &csi.NodeGetInfoResponse{}, nil
}

func (iscsi *iscsistorage) NodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	return &csi.NodeGetVolumeStatsResponse{}, status.Error(codes.Unimplemented, time.Now().String())

}

func (iscsi *iscsistorage) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	return &csi.NodeExpandVolumeResponse{}, status.Error(codes.Unimplemented, time.Now().String())
}

// ------------------------------------ Supporting methods  ---------------------------
func rescanDeviceMap(volumeId string) error {
	klog.V(4).Infof("*************************** Rescan hosts for volume '%s'", volumeId)
	hostIds, err := execScsi.Command(fmt.Sprintf("iscsiadm -m session -P3 | awk '{ if (NF > 3 && $1 == \"Host\" && $2 == \"Number:\") printf(\"%%s \", $3) }'"))
	if err != nil {
		klog.V(4).Infof("Finding hosts failed: %s", err)
		return err
	}

	hosts := string(hostIds[:])
	for _, host := range strings.Fields(hosts) {
		scsiHostPath := fmt.Sprintf("/sys/class/scsi_host/host%s/scan", host)
		_, err = execScsi.Command(fmt.Sprintf("echo '- - -' > %s", scsiHostPath))
		if err != nil {
			klog.V(4).Infof("Rescan of host %s failed: %s", scsiHostPath, err)
			return err
		}
	}

	sleep_sec := 5
	klog.V(4).Infof("Sleep %d seconds after rescan", sleep_sec)
	time.Sleep(time.Duration(sleep_sec) * time.Second)

	klog.V(4).Infof("Rescan hosts complete")
	return err
}

func (iscsi *iscsistorage) AttachDisk(b iscsiDiskMounter) (mntPath string, err error) {
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("iscsi: Recovered from ISCSI AttachDisk  " + fmt.Sprint(res))
		}
	}()
	var devicePath string
	var devicePaths []string
	var iscsiTransport string
	var lastErr error

	klog.V(2).Infof("Called AttachDisk")
	log.WithFields(log.Fields{"iqn": b.iscsiDisk.Iqn, "lun": b.iscsiDisk.lun,
		"chap_session": b.chap_session}).Info("Mounting Volume")

	klog.V(4).Infof("iscsiDiskMounter: %v", b)
	klog.V(4).Infof("Check that provided interface '%s' is available", b.Iface)

	isToLogOutput := false
	out, err := execScsi.Command(fmt.Sprintf("iscsiadm --mode iface --interface %s --op show", b.Iface), isToLogOutput)
	if err != nil {
		klog.Errorf("Cannot read interface %s error: %s", b.Iface, string(out))
		return "", err
	}
	klog.V(4).Infof("Provided interface '%s': ", b.Iface) //, out)

	iscsiTransport = iscsi.extractTransportName(string(out))
	klog.V(4).Infof("iscsiTransport: %s", iscsiTransport)
	bkpPortal := b.Portals
	newIface := bkpPortal[0] // Do not append ':$volume_id'

	// If not found, create new iface and copy parameters from pre-configured (default) iface to the created iface
	// Use one interface per iSCSI network-space, i.e. usually one per IBox.
	// TODO what does a blank Initiator name mean?
	if b.InitiatorName != "" {
		// Look for existing interface named newIface. Clone default iface, if not found.
		klog.V(4).Infof("initiatorName: %s", b.InitiatorName)
		klog.V(4).Infof("Required iface name: '%s'", newIface)
		isToLogOutput := false
		_, err := execScsi.Command(fmt.Sprintf("iscsiadm --mode iface --interface %s --op show", newIface), isToLogOutput)
		if err != nil {
			klog.V(4).Infof("Creating new iface (clone) and copying parameters from pre-configured iface to it")
			err = iscsi.cloneIface(b, newIface)
			if err != nil {
				klog.Errorf("Failed to clone iface: %s error: %v", b.Iface, err)
				return "", err
			}
			klog.V(4).Infof("New iface created '%s'", newIface)
		} else {
			klog.V(4).Infof("Required iface '%s' already exists", newIface)
		}
		// update iface name. TODO - Seems bad form to change a func param.
		b.Iface = newIface
	} else {
		klog.V(4).Infof("Using existing initiator name'%s'", b.InitiatorName)
	}

	klog.V(4).Infof("Discover targets at portal '%s'", bkpPortal[0])
	// Discover all targets associated with a portal.
	_, err = execScsi.Command(fmt.Sprintf("iscsiadm --mode discoverydb --type sendtargets --portal %s --discover --op new --op delete", bkpPortal[0]))
	if err != nil {
		msg := fmt.Sprintf("Failed to discover targets at portal '%s': %s ", out, err)
		klog.Errorf(msg)
		err = errors.New(msg)
		return "", fmt.Errorf(msg)
	}

	for _, tp := range bkpPortal {
		// Loop over portals:
		// - Wait for iSCSI transport paths to appear.
		// - Set CHAP usage.
		// - Login.

		if iscsiTransport == "" {
			klog.Errorf("Could not find transport name in iface %s", b.Iface) // TODO - b.Iface here realy should be newIface...or does it matter?
			err = errors.New(fmt.Sprintf("Could not parse iface file for %s", b.Iface))
			return "", fmt.Errorf("iscsi: Could not parse iface file for %s", b.Iface)
		}
		if iscsiTransport == "tcp" {
			devicePath = strings.Join([]string{"/host/dev/disk/by-path/ip", tp, "iscsi", b.Iqn, "lun", b.lun}, "-")
		} else {
			devicePath = strings.Join([]string{"/host/dev/disk/by-path/pci", "*", "ip", tp, "iscsi", b.Iqn, "lun", b.lun}, "-")
		}

		if exist := iscsi.waitForPathToExist(&devicePath, 1, iscsiTransport); exist {
			klog.V(2).Infof("devicepath (%s) exists", devicePath)
			devicePaths = append(devicePaths, devicePath)
			continue
		}

		// update discoverydb with CHAP secret
		if b.chap_session {
			klog.V(4).Infof("Do session auth with chap")
			err = iscsi.updateISCSINode(b, tp)
			if err != nil {
				// failure to update node db is rare. But deleting record will likely impact those who already start using it.
				lastErr = fmt.Errorf("iscsi: Failed to update iscsi node to portal %s error: %v", tp, err)
				continue
			}
		} else {
			klog.V(4).Infof("Not using CHAP")
		}

		klog.V(4).Infof("Login to iscsi target")
		out, err = execScsi.Command(fmt.Sprintf("iscsiadm --mode node --portal %s --targetname %s --interface %s --login", tp, b.Iqn, b.Iface))
		if err != nil {
			msg := fmt.Sprintf("Login failed: %s", err)
			klog.Errorf(msg)
			lastErr = errors.New(msg)
			continue
		}

		klog.V(4).Infof("Wait for path '%s' to exist", devicePath)
		if exist := iscsi.waitForPathToExist(&devicePath, 10, iscsiTransport); !exist {
			msg := "Could not attach disk: Timeout after 10s"
			klog.Errorf(msg)
			// update last error
			lastErr = fmt.Errorf("iscsi: " + msg)
			continue
		} else {
			klog.Infof("Found path '%s', adding to devicePaths", devicePath)
			devicePaths = append(devicePaths, devicePath)
		}
	}

	helper.PrettyKlogDebug("Found devicePaths:", devicePaths)

	klog.V(4).Infof("Rescan sessions")
	out, err = execScsi.Command(fmt.Sprintf("iscsiadm --mode session --rescan"))
	if err != nil {
		klog.V(4).Infof("Session rescan failed")
		return "", fmt.Errorf("Session rescan failed: %s", err)
	}

	if len(devicePaths) == 0 {
		msg := fmt.Sprintf("Failed to get any path for iscsi disk, last err seen:\n%v", lastErr)
		klog.Errorf(msg)
		err = errors.New(msg)
		return "", fmt.Errorf(msg)
	}
	if lastErr != nil {
		klog.Errorf("Last error occurred during iscsi init:\n%v", lastErr)
	}

	rescanDeviceMap(b.VolName)

	// Make sure we use a valid devicepath to find mpio device.
	devicePath = devicePaths[0]
	mntPath = b.targetPath
	// Mount device
	notMnt, err := b.mounter.IsLikelyNotMountPoint(mntPath)
	if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("iscsi: Heuristic determination of mount point failed: %v", err)
	}
	if !notMnt {
		klog.V(2).Infof("%s already mounted", mntPath)
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
		// A block volume is a volume that will appear as a block device inside the container.
		klog.V(4).Infof("Block volume will be mount at file %s", mntPath)
		if b.readOnly {
			return "", status.Error(codes.Internal, "iscsi: Read-only is not supported for Block Volume")
		}

		if err := os.MkdirAll(filepath.Dir(mntPath), 0750); err != nil {
			klog.Errorf("Failed to mkdir %s, error", filepath.Dir(mntPath))
			return "", err
		}

		_, err = os.Create("/host/" + mntPath)
		if err != nil {
			klog.Errorf("Failed to create target file %q: %v", mntPath, err)
			return "", fmt.Errorf("iscsi: Failed to create target file for raw block bind mount: %v", err)
		}
		devicePath = strings.Replace(devicePath, "/host", "", 1)
		options := []string{"bind"}
		options = append(options, "rw")
		if err := b.mounter.Mount(devicePath, mntPath, "", options); err != nil {
			klog.Errorf("Failed to mount iscsi volume %s [%s] to %s, error %v", devicePath, b.fsType, mntPath, err)
			return "", err
		}
		if err := iscsi.createISCSIConfigFile(*(b.iscsiDisk), b.stagePath); err != nil {
			klog.Errorf("Failed to save iscsi config with error: %v", err)
			return "", err
		}
		klog.V(4).Infof("Block volume mounted successfully")
		return devicePath, err
	} else {
		// A mounted (file) volume is volume that will be mounted using a specified file system
		// and appear as a directory inside the container.
		mountPoint := mntPath
		klog.V(4).Infof("Mounting volume '%s' to mountPoint '%s'", devicePath, mountPoint)

		// Attempt to find a mapper device to use rather than a bare devicePath.
		// If not found, use the devicePath.
		dmsetupInfo, dmsetupErr := dmsetup.Info(devicePath)
		if dmsetupErr != nil {
			klog.Errorf("Failed to execute dmsetup info '%s'. Cannot look up mapper device: %s", devicePath, dmsetupErr)
		} else {
			for i, info := range dmsetupInfo {
				klog.V(4).Infof("dmsetupInfo[%d]: %+v", i, *info)
			}
			if len(dmsetupInfo) == 1 { // One and only one mapper should be in the slice since the devicePath was given.
				mapperPath := devMapperDir + dmsetupInfo[0].Name
				klog.V(2).Infof("Using mapper device. '%s' maps to '%s'", devicePath, mapperPath)
				devicePath = mapperPath
			}
		}

		// Create mountPoint if it does not exist.
		_, err := os.Stat(mountPoint)
		if os.IsNotExist(err) {
			klog.V(4).Infof("Mount point does not exist. Creating mount point.")
			// Do not use os.MkdirAll(). This ignores the mount chroot defined in the Dockerfile.
			// MkdirAll() will cause hard-to-grok mount errors.
			_, err := execScsi.Command(fmt.Sprintf("mkdir --parents --mode 0750 '%s'", mountPoint))
			if err != nil {
				klog.Errorf("Failed to mkdir '%s': %s", mountPoint, err)
				return "", err
			}

			// Verify mountPoint exists. If ready a file named 'ready' will appear in mountPoint directory.
			util.SetReady(mountPoint)
			is_ready := util.IsReady(mountPoint)
			klog.V(2).Infof("Check that mountPoint is ready: %t", is_ready)
		} else {
			klog.V(4).Infof("mkdir of mountPoint not required. '%s' already exists", mountPoint)
		}

		var options []string
		if b.readOnly {
			klog.V(4).Infof("Volume is read-only")
			options = append(options, "ro")
		} else {
			klog.V(4).Infof("Volume is read-write")
			options = append(options, "rw")
		}
		options = append(options, b.mountOptions...)

		klog.V(4).Infof("Strip /host from %s", devicePath)
		devicePath = strings.Replace(devicePath, "/host", "", 1)

		// Persist here so that even if mount fails, the globalmount metadata json
		// file will contain an mpath to use during clean up.
		klog.V(4).Infof("Persist iscsi disk config to json file for later use, when detaching the disk")
		if err = iscsi.createISCSIConfigFile(*(b.iscsiDisk), b.stagePath); err != nil {
			klog.Errorf("Failed to save iscsi config with error: %v", err)
			return "", err
		}

		klog.V(4).Infof("Format '%s' (if needed) and mount volume", devicePath)
		err = b.mounter.FormatAndMount(devicePath, mountPoint, b.fsType, options)
		klog.V(4).Infof("FormatAndMount returned: %s", err)
		if err != nil {
			searchAlreadyMounted := fmt.Sprintf("already mounted on %s", mountPoint)
			searchBadSuperBlock := fmt.Sprintf("wrong fs type, bad option, bad superblock")
			klog.V(4).Infof("Search error for matches to handle: %s", err)

			if isAlreadyMounted := strings.Contains(err.Error(), searchAlreadyMounted); isAlreadyMounted {
				klog.Errorf("Device %s is already mounted on %s", devicePath, mountPoint)
			} else if isBadSuperBlock := strings.Contains(err.Error(), searchBadSuperBlock); isBadSuperBlock {
				if err := regenerateXfsFilesystemUuid(devicePath); err != nil {
					return "", err
				}
				klog.V(4).Infof("Run FormatAndMount, after UUID change")
				err = b.mounter.FormatAndMount(devicePath, mountPoint, b.fsType, options)
				if err != nil {
					return "", err
				}
			} else {
				klog.Errorf("Failed to mount iscsi volume %s [%s] to %s, error %v", devicePath, b.fsType, mountPoint, err)
				mountPathExists(mountPoint)
				return "", err
			}
		} else {
			klog.V(4).Infof("FormatAndMount err is nil")
		}
	}
	klog.V(4).Infof("Mounted volume successfully at '%s'", mntPath)
	return devicePath, nil
}

func mountPathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		klog.V(4).Infof("mountPathExists: Path " + path + " exists")
		return true, nil
	}
	if os.IsNotExist(err) {
		klog.V(4).Infof("mountPathExists: Path " + path + " does not exist")
		return false, nil
	}
	return false, err
}

func (iscsi *iscsistorage) DetachDisk(c iscsiDiskUnmounter, targetPath string) (err error) {
	klog.V(4).Infof("Called DetachDisk targetpath: %s", targetPath)
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("iscsi: Recovered from ISCSI DetachDisk  " + fmt.Sprint(res))
		}
		klog.V(4).Infof("DetachDisk is returning")
	}()
	mntPath := path.Join("/host", targetPath)
	if pathExist, pathErr := iscsi.pathExists(targetPath); pathErr != nil {
		return fmt.Errorf("iscsi: Error checking if path exists: %v", pathErr)
	} else if !pathExist {
		if pathExist, _ = iscsi.pathExists(mntPath); pathErr == nil {
			if !pathExist {
				klog.Warningf("Unmount skipped because path does not exist: %v", targetPath)
				return nil
			}
		}
	}
	klog.V(4).Infof("Umount volume from targetPath '%s'", targetPath)
	if err = c.mounter.Unmount(targetPath); err != nil {
		if strings.Contains(err.Error(), "not mounted") {
			klog.V(4).Infof("Volume not mounted removing files ", targetPath)
			if err := os.RemoveAll(filepath.Dir(mntPath)); err != nil {
				klog.Errorf("Failed to remove mount path Error: %v", err)
			}
			return nil
		}
		klog.Errorf("detach disk: Failed to unmount: %s\nError: %v", targetPath, err)
		return err
	}
	if err := os.RemoveAll(filepath.Dir(mntPath)); err != nil {
		klog.Errorf("Failed to remove mount path Error: %v", err)
		return err
	}
	klog.V(4).Infof("Unmounted volume successfully!")
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
			err = errors.New("iscsi: Recovered from ISCSI getInitiatorName  " + fmt.Sprint(res))
		}
	}()
	cmd := "cat /etc/iscsi/initiatorname.iscsi | grep InitiatorName="
	out, err := exec.Command("bash", "-c", cmd).Output()
	if err != nil {
		klog.Errorf("Failed to get initiator name with error %v", err)
		return ""
	}
	initiatorName := string(out)
	initiatorName = strings.TrimSuffix(initiatorName, "\n")
	klog.V(4).Infof("Host initiator name %s ", initiatorName)
	arr := strings.Split(initiatorName, "=")
	return arr[1]
}

func (iscsi *iscsistorage) getISCSIInfo(req *csi.NodePublishVolumeRequest) (*iscsiDisk, error) {
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("iscsi: Recovered from ISCSI getISCSIInfo  " + fmt.Sprint(res))
		}
	}()
	klog.V(4).Infof("Called getISCSIInfo")
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
		return nil, fmt.Errorf("iscsi: iSCSI target information is missing")
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
		mounter:      &mount.SafeFormatAndMount{Interface: mount.New(""), Exec: utilexec.New()},
		exec:         utilexec.New(),
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
		exec:    utilexec.New(),
	}
}

func (iscsi *iscsistorage) parseSessionSecret(useChap string, secretParams map[string]string) (map[string]string, error) {
	var ok bool
	secret := make(map[string]string)

	if useChap == "chap" || useChap == "mutual_chap" {
		if len(secretParams) == 0 {
			return secret, errors.New("iscsi: required chap secrets not provided")
		}
		if secret["node.session.auth.username"], ok = secretParams["node.session.auth.username"]; !ok {
			return secret, fmt.Errorf("iscsi: node.session.auth.username not found in secret")
		}
		if secret["node.session.auth.password"], ok = secretParams["node.session.auth.password"]; !ok {
			return secret, fmt.Errorf("iscsi: node.session.auth.password not found in secret")
		}
		if useChap == "mutual_chap" {
			if secret["node.session.auth.username_in"], ok = secretParams["node.session.auth.username_in"]; !ok {
				return secret, fmt.Errorf("iscsi: node.session.auth.username_in not found in secret")
			}
			if secret["node.session.auth.password_in"], ok = secretParams["node.session.auth.password_in"]; !ok {
				return secret, fmt.Errorf("iscsi: node.session.auth.password_in not found in secret")
			}
		}
		secret["SecretsType"] = "chap"
	}
	return secret, nil
}

func (iscsi *iscsistorage) updateISCSIDiscoverydb(b iscsiDiskMounter, tp string) error {
	if !b.chap_discovery {
		klog.V(4).Infof("CHAP discovery is not allowed")
		return nil
	}
	klog.V(4).Infof("Update discoverydb with CHAP")
	out, err := execScsi.Command(fmt.Sprintf("iscsiadm --mode discoverydb --type sendtargets --portal %s --interface %s --op update --name discovery.sendtargets.auth.authmethod --value CHAP", tp, b.Iface))
	if err != nil {
		return fmt.Errorf("iscsi: Failed to update discoverydb with CHAP, output: %v", string(out))
	}

	for _, k := range chap_st {
		v := b.secret[k]
		if len(v) > 0 {
			klog.V(4).Infof("Update discoverdb with key/value")
			out, err := execScsi.Command(fmt.Sprintf("iscsiadm --mode discoverydb --type sendtargets --portal %s --interface %s --op update --name %q --value %q", tp, b.Iface, k, v))
			if err != nil {
				return fmt.Errorf("iscsi: Failed to update discoverydb key %q with value %q error: %v", k, v, string(out))
			}
		}
	}
	return nil
}

func (iscsi *iscsistorage) updateISCSINode(b iscsiDiskMounter, tp string) error {
	if !b.chap_session {
		return nil
	}

	klog.V(4).Infof("Update node with CHAP")
	out, err := execScsi.Command(fmt.Sprintf("iscsiadm --mode node --portal %s --targetname %s --interface %s --op update --name node.session.auth.authmethod --value CHAP", tp, b.Iqn, b.Iface))
	if err != nil {
		return fmt.Errorf("iscsi: Failed to update node with CHAP, output: %v", string(out))
	}

	for _, k := range chap_sess {
		v := b.secret[k]
		if len(v) > 0 {
			klog.V(4).Infof("Update node session key/value")
			out, err := execScsi.Command(fmt.Sprintf("iscsiadm --mode node --portal %s --targetname %s --interface %s --op update --name %q --value %q", tp, b.Iqn, b.Iface, k, v))
			if err != nil {
				return fmt.Errorf("iscsi: Failed to update node session key %q with value %q error: %v", k, v, string(out))
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
	klog.V(4).Infof("persistISCSI: Creating persist config file at path %s", file)
	fp, err := os.Create(file)
	if err != nil {
		klog.Errorf("persistISCSI: Failed creating persist config file with error %v", err)
		return fmt.Errorf("iscsi: Create %s err %s", file, err)
	}
	defer fp.Close()
	encoder := json.NewEncoder(fp)
	if err = encoder.Encode(conf); err != nil {
		klog.Errorf("iscsi: persistISCSI: Failed creating persist config file with error %v", err)
		return fmt.Errorf("iscsi: Encode err: %v", err)
	}
	return nil
}

func (iscsi *iscsistorage) loadDiskInfoFromFile(conf *iscsiDisk, mnt string) error {
	file := path.Join("/host", mnt, conf.VolName+".json")
	fp, err := os.Open(file)
	if err != nil {
		return fmt.Errorf("iscsi: Open %s err %s", file, err)
	}
	defer fp.Close()
	decoder := json.NewDecoder(fp)
	if err = decoder.Decode(conf); err != nil {
		return fmt.Errorf("iscsi: Decode err: %v ", err)
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
		klog.V(4).Infof("Path exists: ", path)
		return true, nil
	} else if os.IsNotExist(err) {
		klog.V(4).Infof("Path does not exist: ", path)
		return false, nil
	} else if iscsi.isCorruptedMnt(err) {
		klog.V(4).Infof("Path is corrupted: ", path)
		return true, err
	} else {
		klog.V(4).Infof("Path cannot be validated: ", path)
		return false, err
	}
}

func (iscsi *iscsistorage) extractTransportName(ifaceOutput string) (iscsiTransport string) {
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
			return nil, fmt.Errorf("iscsi: Invalid iface setting: %v", iface)
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
	klog.V(4).Infof("Find pre-configured iface records")
	out, err := execScsi.Command(fmt.Sprintf("iscsiadm --mode iface --interface %s --op show", b.Iface))
	if err != nil {
		lastErr = fmt.Errorf("iscsi: Failed to show iface records: %s (%v)", string(out), err)
		return lastErr
	}
	klog.V(4).Infof("Pre-configured iface records found: %s", out)

	// parse obtained records
	params, err := iscsi.parseIscsiadmShow(string(out))
	if err != nil {
		lastErr = fmt.Errorf("iscsi: Failed to parse iface records: %s (%v)", string(out), err)
		return lastErr
	}
	// update initiatorname
	params["iface.initiatorname"] = b.InitiatorName

	klog.V(4).Infof("Create new interface")
	out, err = execScsi.Command(fmt.Sprintf("iscsiadm --mode iface --interface %s --op new", newIface))
	if err != nil {
		lastErr = fmt.Errorf("iscsi: Failed to create new iface: %s (%v)", string(out), err)
		return lastErr
	}

	// update new iface records
	for key, val := range params {
		klog.V(4).Infof("Update records of interface '%s'", newIface)
		_, err = execScsi.Command(fmt.Sprintf("iscsiadm --mode iface --interface %s --op update --name %q --value %q", newIface, key, val))
		if err != nil {
			klog.V(4).Infof("Failed to update records of interface '%s'", newIface)
			_, err := execScsi.Command(fmt.Sprintf("iscsiadm --mode iface --interface %s --op delete", newIface))
			if err != nil {
				lastErr = fmt.Errorf("Failed to delete iface '%s': %s ", newIface, err)
				return lastErr
			}

			lastErr = fmt.Errorf("iscsi: Failed to update iface records: %s (%v). iface(%s) will be used", string(out), err, b.Iface)
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
		klog.Errorf("Failed to find multipath device with error %v", err)
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
	return "", errors.New("iscsi: Illegal path for device " + devicePath)
}

func regenerateXfsFilesystemUuid(devicePath string) (err error) {
	klog.V(4).Infof("regenerateXfsFilesystemUuid called")

	allow_xfs_uuid_regeneration := os.Getenv("ALLOW_XFS_UUID_REGENERATION")
	allow_uuid_fix, err := helper.YamlBoolToBool(allow_xfs_uuid_regeneration)
	if err != nil {
		klog.Errorf("Invalid ALLOW_XFS_UUID_REGENERATION variable: %s", err)
		return err
	}

	if allow_uuid_fix {
		newUuid := uuid.New()

		klog.Errorf("Device %s has duplicate XFS UUID. New UUID: %s. ALLOW_XFS_UUID_REGENERATION is set to %s", devicePath, newUuid, allow_xfs_uuid_regeneration)

		klog.V(4).Infof("Update device '%s' UUID with '%s'", devicePath, newUuid)
		_, err_xfs := execScsi.Command(fmt.Sprintf("xfs_admin -U %s %s", newUuid, devicePath))

		if err_xfs != nil {
			msg := fmt.Sprintf("xfs_admin failed. Volume likely to be read-only: %s", err_xfs)
			klog.V(4).Infof(msg)
			return errors.New(msg)
		}
	} else {
		klog.Errorf("Device %s has duplicate XFS UUID and cannot be mounted. ALLOW_XFS_UUID_REGENERATION is set to %s", devicePath, allow_xfs_uuid_regeneration)
		return err
	}
	return nil
}

func deleteMultipathDevices(devices []string) (err error) {
	for _, device := range devices {
		device = strings.Replace(device, "/dev/", "", 1)
		klog.V(4).Infof("Delete multipath device %s", device)
		out, err := execScsi.Command(fmt.Sprintf("multipathd -k\"del path %s\"", device))
		if err != nil {
			klog.V(4).Infof("Delete multipath device '%s' failed: %s", device, err)
			return err
		}
		klog.V(4).Infof("Delete device '%s' output: %s", device, out)
	}
	return nil
}

func deleteMultipathMap(multipathMap string) (err error) {
	multipathMap = strings.Replace(multipathMap, "/dev/", "", 1)
	klog.V(4).Infof("Delete multipath map '%s'", multipathMap)
	out, err := execScsi.Command(fmt.Sprintf("multipathd -k\"del map %s\"", multipathMap))
	if err != nil {
		klog.V(4).Infof("Delete multipath map '%s' failed: %s", multipathMap, err)
		return err
	}
	klog.V(4).Infof("Delete map '%s' output: %s", multipathMap, out)
	return nil
}

// // Find returns the smallest index i at which x == a[i],
// // or len(a) if there is no such index.
// // TODO Move to helper pkg.
// func Find(slice []string, x string) int {
// 	for i, n := range slice {
// 		if x == n {
// 			return i
// 		}
// 	}
// 	return len(slice)
// }
//
// func Purge(slice []string, x string) []string {
// 	var pos int
// 	for _, val := range slice {
// 		if val == x {
// 			slice = append(slice[:pos], slice[pos+1:]...)
// 			if pos > 0 {
// 				pos = pos - 1
// 			}
// 			continue
// 		}
// 		pos++
// 	}
// 	return slice
// }
