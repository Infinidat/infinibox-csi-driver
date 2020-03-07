package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/kubernetes/pkg/util/mount"
)

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

func updateISCSIDiscoverydb(b iscsiDiskMounter2, tp string) error {
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

func updateISCSINode(b iscsiDiskMounter2, tp string) error {
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

// StatFunc stat a path, if not exists, retry maxRetries times
// when iscsi transports other than default are used
type StatFunc func(string) (os.FileInfo, error)

// GlobFunc  use glob instead as pci id of device is unknown
type GlobFunc func(string) ([]string, error)

func waitForPathToExist(devicePath *string, maxRetries int, deviceTransport string) bool {
	// This makes unit testing a lot easier
	return waitForPathToExistInternal(devicePath, maxRetries, deviceTransport, os.Stat, filepath.Glob)
}

func waitForPathToExistInternal(devicePath *string, maxRetries int, deviceTransport string, osStat StatFunc, filepathGlob GlobFunc) bool {
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

type ISCSIUtil struct{}

func (util *ISCSIUtil) persistISCSI(conf iscsiDisk2, mnt string) error {
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

func (util *ISCSIUtil) loadISCSI(conf *iscsiDisk2, mnt string) error {
	// NOTE: The iscsi config json is not deleted after logging out from target portals.
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

func (util *ISCSIUtil) AttachDisk2(b iscsiDiskMounter2) (string, error) {
	log.Debug("IN AttachDisk")
	var devicePath string
	var devicePaths []string
	var iscsiTransport string
	var lastErr error
	log.Debug("check provided iface is available")
	out, err := b.exec.Run("iscsiadm", "-m", "iface", "-I", b.Iface, "-o", "show")
	if err != nil {
		log.Errorf("iscsi: could not read iface %s error: %s", b.Iface, string(out))
		return "", err
	}

	iscsiTransport = extractTransportname(string(out))

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
			err = cloneIface(b, newIface)
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

		if exist := waitForPathToExist(&devicePath, 1, iscsiTransport); exist {
			log.Infof("iscsi: devicepath (%s) exists", devicePath)
			devicePaths = append(devicePaths, devicePath)
			continue
		}
		log.Debug("build discoverydb and discover iscsi target")
		// build discoverydb and discover iscsi target
		b.exec.Run("iscsiadm", "-m", "discoverydb", "-t", "sendtargets", "-p", tp, "-I", b.Iface, "-o", "new")
		// update discoverydb with CHAP secret
		err = updateISCSIDiscoverydb(b, tp)
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
		err = updateISCSINode(b, tp)
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
		if exist := waitForPathToExist(&devicePath, 10, iscsiTransport); !exist {
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
	// Mount device
	mntPath := b.targetPath
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
		if err := util.persistISCSI(*(b.iscsiDisk2), mntPath); err != nil {
			log.Errorf("iscsi: failed to save iscsi config with error: %v", err)
			return "", err
		}
	}
	log.Debug("mounted volume successfully at %s", mntPath)
	return devicePath, err
}
func isCorruptedMnt(err error) bool {
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
func pathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	} else if os.IsNotExist(err) {
		return false, nil
	} else if isCorruptedMnt(err) {
		return true, err
	} else {
		return false, err
	}
}
func (util *ISCSIUtil) DetachDisk2(c iscsiDiskUnmounter2, targetPath string) error {
	log.Debugf("In DetachDisk targetpath: %s", targetPath)
	var bkpPortal []string
	var volName, iqn, iface, initiatorName string
	diskConfigFound := true
	found := true
	// load iscsi disk config from json file
	if err := util.loadISCSI(c.iscsiDisk2, targetPath); err == nil {
		bkpPortal, iqn, iface, volName = c.iscsiDisk2.Portals, c.iscsiDisk2.Iqn, c.iscsiDisk2.Iface, c.iscsiDisk2.VolName
		initiatorName = c.iscsiDisk2.InitiatorName
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
	if pathExists, pathErr := pathExists(targetPath); pathErr != nil {
		return fmt.Errorf("Error checking if path exists: %v", pathErr)
	} else if !pathExists {
		log.Warningf("Warning: Unmount skipped because path does not exist: %v", targetPath)
		return nil
	}
	log.Debugf("unmout volume")
	if err = c.mounter.Unmount(targetPath); err != nil {
		if strings.Contains(err.Error(), "not mounted") {
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
	portals := removeDuplicate(bkpPortal)
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

func extractTransportname(ifaceOutput string) (iscsiTransport string) {
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
func removeDuplicate(s []string) []string {
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

func parseIscsiadmShow(output string) (map[string]string, error) {
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

func cloneIface(b iscsiDiskMounter2, newIface string) error {
	var lastErr error
	// get pre-configured iface records
	out, err := b.exec.Run("iscsiadm", "-m", "iface", "-I", b.Iface, "-o", "show")
	if err != nil {
		lastErr = fmt.Errorf("iscsi: failed to show iface records: %s (%v)", string(out), err)
		return lastErr
	}
	// parse obtained records
	params, err := parseIscsiadmShow(string(out))
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
