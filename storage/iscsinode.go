package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	log "github.com/sirupsen/logrus"
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
	diskUnmounter := iscsi.getISCSIDiskUnmounter(req)
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
	hstID, _ := strconv.Atoi(hostID)
	log.Debugf("publishig volume to host id is %s", hostID)
	//validate host exists
	if hstID < 1 {
		log.Errorf("hostID %d is not valid host ID")
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
			log.Error("error creating host port ", err)
			return &csi.NodeStageVolumeResponse{}, status.Error(codes.Internal, err.Error())
		}
	}

	return &csi.NodeStageVolumeResponse{}, nil
}
func (iscsi *iscsistorage) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
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
	var devicePath string
	var devicePaths []string
	var iscsiTransport string
	var lastErr error

	log.Info("Called AttachDisk")
	log.WithFields(log.Fields{"iqn": b.iscsiDisk.Iqn, "lun": b.iscsiDisk.lun,
		"chap_session": b.chap_session}).Info("Mounting Volume")

	if "debug" == log.GetLevel().String() {
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
			devicePath = strings.Join([]string{"/dev/disk/by-path/ip", tp, "iscsi", b.Iqn, "lun", b.lun}, "-")
		} else {
			devicePath = strings.Join([]string{"/dev/disk/by-path/pci", "*", "ip", tp, "iscsi", b.Iqn, "lun", b.lun}, "-")
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

	if b.isBlock {
		log.Debugf("Block volume will be mount at file %s", b.targetPath)
		if b.readOnly {
			return "", status.Error(codes.Internal, "Read only is not supported for Block Volume")
		}

		err = b.mounter.MakeDir(filepath.Dir(b.targetPath))
		if err != nil {
			log.Errorf("failed to create target directory %q: %v", filepath.Dir(b.targetPath), err)
			return "", fmt.Errorf("failed to create target directory for raw block bind mount: %v", err)
		}

		err = b.mounter.MakeFile(b.targetPath)
		if err != nil {
			log.Errorf("failed to create target file %q: %v", b.targetPath, err)
			return "", fmt.Errorf("failed to create target file for raw block bind mount: %v", err)
		}

		symLink, err := filepath.EvalSymlinks(devicePath)
		if err != nil {
			log.Errorf("could not resolve symlink %q: %v", devicePath, err)
			return "", fmt.Errorf("could not resolve symlink %q: %v", devicePath, err)
		}

		if !strings.HasPrefix(symLink, "/dev") {
			log.Errorf("resolved symlink %q for %q was unexpected", symLink, devicePath)
			return "", fmt.Errorf("resolved symlink %q for %q was unexpected", symLink, devicePath)
		}

		options := []string{"bind"}
		options = append(options, "rw")
		if err := b.mounter.Mount(symLink, b.targetPath, "", options); err != nil {
			log.Errorf("iscsi: failed to mount iscsi volume %s [%s] to %s, error %v", symLink, b.fsType, b.targetPath, err)
			return "", err
		}
		log.Debug("Block volume mounted successfully")
		return symLink, err
	} else {
		log.Debugf("mount volume to given path %s", b.targetPath)
		if err := os.MkdirAll(mntPath, 0750); err != nil {
			log.Errorf("iscsi: failed to mkdir %s, error", mntPath)
			return "", err
		}

		for _, path := range devicePaths {
			// There shouldnt be any empty device paths. However adding this check
			// for safer side to avoid the possibility of an empty entry.
			if path == "" {
				continue
			}
			// check if the dev is using mpio and if so mount it via the dm-XX device
			if mappedDevicePath := b.deviceUtil.FindMultipathDeviceForDevice(path); mappedDevicePath != "" {
				devicePath = mappedDevicePath
				break
			}
		}

		var options []string

		if b.readOnly {
			options = append(options, "ro")
		} else {
			options = append(options, "rw")
		}
		options = append(options, b.mountOptions...)
		log.Debug("format (if needed) and mount volume")
		err = b.mounter.FormatAndMount(devicePath, mntPath, b.fsType, options)
		if err != nil {
			log.Errorf("iscsi: failed to mount iscsi volume %s [%s] to %s, error %v", devicePath, b.fsType, mntPath, err)
		}
		log.Debug("Persist iscsi disk config to json file for DetachDisk path")
		// Persist iscsi disk config to json file for DetachDisk path
		if err := iscsi.createISCSIConfigFile(*(b.iscsiDisk), mntPath); err != nil {
			log.Errorf("iscsi: failed to save iscsi config with error: %v", err)
			return "", err
		}
	}
	log.Debug("mounted volume successfully at %s", mntPath)
	return devicePath, err

}

func (iscsi *iscsistorage) DetachDisk(c iscsiDiskUnmounter, targetPath string) (err error) {
	log.Debugf("Called DetachDisk targetpath: %s", targetPath)
	var bkpPortal []string
	var volName, iqn, iface, initiatorName string
	diskConfigFound := true
	found := true
	// load iscsi disk config from json file
	if err := iscsi.loadDiskInfoFromFile(c.iscsiDisk, targetPath); err == nil {
		bkpPortal, iqn, iface, volName = c.iscsiDisk.Portals, c.iscsiDisk.Iqn, c.iscsiDisk.Iface, c.iscsiDisk.VolName
		initiatorName = c.iscsiDisk.InitiatorName
	} else {
		log.Errorf("iscsi detach disk: failed to get iscsi config from path %s Error: %v", targetPath, err)
		diskConfigFound = false
	}
	mntPath := path.Join("/host", targetPath)
	_, cnt, err := mount.GetDeviceNameFromMount(c.mounter, mntPath)
	if err != nil {
		log.Errorf("iscsi detach disk: failed to get device from mnt: %s\nError: %v", targetPath, err)
		return err
	}
	log.Debugf("mounted on %d pods", cnt)
	if pathExist, pathErr := iscsi.pathExists(targetPath); pathErr != nil {
		return fmt.Errorf("Error checking if path exists: %v", pathErr)
	} else if !pathExist {
		if pathExist, _ = iscsi.pathExists(mntPath); pathErr == nil {
			if !pathExist {
				log.Warningf("Warning: Unmount skipped because path does not exist: %v", targetPath)
				return nil
			}
		}
	}
	log.Debug("unmout volume from tagetpath ", targetPath)
	if err = c.mounter.Unmount(targetPath); err != nil {
		if strings.Contains(err.Error(), "not mounted") {
			log.Debug("volume not mounted removing files ", targetPath)
			if err := os.RemoveAll(targetPath); err != nil {
				log.Errorf("iscsi: failed to remove mount path Error: %v", err)
			}
			return nil
		}
		log.Errorf("iscsi detach disk: failed to unmount: %s\nError: %v", targetPath, err)
		return err
	}
	log.Debug("unmout success")
	cnt--
	if cnt != 0 {
		log.Debug("not disconnecting session volume is refered by other pods")
		return nil
	}
	if !diskConfigFound {
		log.Debug("cannont disconnect session as disk configuration not found")
		return nil
	}

	log.Debug("disconnect session")

	log.Debugf("logout session for initiatorName %s, iqn %s, volume id %s", initiatorName, iqn, volName)
	portals := iscsi.removeDuplicate(bkpPortal)
	if len(portals) == 0 {
		return fmt.Errorf("iscsi detach disk: failed to detach iscsi disk. Couldn't get connected portals from configurations.")
	}

	for _, portal := range portals {
		logoutArgs := []string{"-m", "node", "-p", portal, "-T", iqn, "--logout"}
		deleteArgs := []string{"-m", "node", "-p", portal, "-T", iqn, "-o", "delete"}
		if found {
			logoutArgs = append(logoutArgs, []string{"-I", iface}...)
			deleteArgs = append(deleteArgs, []string{"-I", iface}...)
		}
		out, err := c.exec.Run("iscsiadm", logoutArgs...)
		if err != nil {
			log.Errorf("iscsi: failed to detach disk Error: %s", string(out))
		}
		// Delete the node record
		out, err = c.exec.Run("iscsiadm", deleteArgs...)
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
			out, err := c.exec.Run("iscsiadm", deleteArgs...)
			if err != nil {
				log.Errorf("iscsi: failed to delete iface Error: %s", string(out))
			}
			break
		}
	}

	if err := os.RemoveAll(targetPath); err != nil {
		log.Errorf("iscsi: failed to remove mount path Error: %v", err)
		return err
	}
	log.Debug("Detach Disk Successfully!")
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
	chapDiscovery := false
	if req.GetVolumeContext()["discoveryCHAPAuth"] == "true" {
		chapDiscovery = true
	}

	chapSession := false
	if req.GetVolumeContext()["sessionCHAPAuth"] == "true" {
		chapSession = true
	}
	volproto := strings.Split(req.GetVolumeId(), "$$")
	volName := volproto[0]
	//tp := req.GetVolumeContext()["targetPortal"]
	iqn := req.GetVolumeContext()["iqn"]
	lun := req.GetPublishContext()["lun"]
	portals := req.GetVolumeContext()["portals"]
	portalList := strings.Split(portals, ",")
	if len(portalList) == 0 || iqn == "" || lun == "" {
		return nil, fmt.Errorf("iSCSI target information is missing")
	}
	initiatorName := getInitiatorName()
	secret := req.GetSecrets()
	if chapSession {
		secret, err = iscsi.parseSessionSecret(secret)
		secret["node.session.auth.username"] = initiatorName
		if err != nil {
			return nil, err
		}
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
	readOnly := req.GetReadonly()
	fsType := req.GetVolumeCapability().GetMount().GetFsType()
	mountOptions := req.GetVolumeCapability().GetMount().GetMountFlags()

	return &iscsiDiskMounter{
		iscsiDisk:    iscsiInfo,
		fsType:       fsType,
		readOnly:     readOnly,
		mountOptions: mountOptions,
		mounter:      &mount.SafeFormatAndMount{Interface: mount.New(""), Exec: mount.NewOsExec()},
		exec:         mount.NewOsExec(),
		targetPath:   req.GetTargetPath(),
		deviceUtil:   util.NewDeviceHandler(util.NewIOHandler()),
	}
}

func (iscsi *iscsistorage) getISCSIDiskUnmounter(req *csi.NodeUnpublishVolumeRequest) *iscsiDiskUnmounter {
	volproto := strings.Split(req.GetVolumeId(), "$$")
	volName := volproto[0]
	return &iscsiDiskUnmounter{
		iscsiDisk: &iscsiDisk{
			VolName: volName,
		},
		mounter: mount.New(""),
		exec:    mount.NewOsExec(),
	}
}

func (iscsi *iscsistorage) parseSessionSecret(secretParams map[string]string) (map[string]string, error) {
	var ok bool
	secret := make(map[string]string)

	if len(secretParams) == 0 {
		return secret, nil
	}

	if secret["node.session.auth.username"], ok = secretParams["node.session.auth.username"]; !ok {
		return secret, fmt.Errorf("node.session.auth.username not found in secret")
	}
	if secret["node.session.auth.password"], ok = secretParams["node.session.auth.password"]; !ok {
		return secret, fmt.Errorf("node.session.auth.password not found in secret")
	}
	if secret["node.session.auth.username_in"], ok = secretParams["node.session.auth.username_in"]; !ok {
		return secret, fmt.Errorf("node.session.auth.username_in not found in secret")
	}
	if secret["node.session.auth.password_in"], ok = secretParams["node.session.auth.password_in"]; !ok {
		return secret, fmt.Errorf("node.session.auth.password_in not found in secret")
	}

	secret["SecretsType"] = "chap"
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
	log.Debug("persistISCSI: creating persist file at path %s ", file)
	fp, err := os.Create(file)
	if err != nil {
		log.Error("persistISCSI: failed creating persist file with error %v", err)
		return fmt.Errorf("iscsi: create %s err %s", file, err)
	}
	defer fp.Close()
	encoder := json.NewEncoder(fp)
	if err = encoder.Encode(conf); err != nil {
		log.Error("persistISCSI: failed creating persist file with error %v", err)
		return fmt.Errorf("iscsi: encode err: %v.", err)
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
