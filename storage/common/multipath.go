package common

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"infinibox-csi-driver/helper"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	MULTIPATH_WAIT              = "MULTIPATH_WAIT"
	mpathDeviceCount        int = 6
	MULTIPATH_CLEANUP_DELAY     = "MULTIPATH_CLEANUP_DELAY"
	FC_SEARCH_DISK_DELAY        = "FC_SEARCH_DISK_DELAY"
)

type PortInfo struct {
	HostID    string
	PortName  string
	PortState string
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

type ShowMultipathOutput struct {
	MajorVersion int `json:"major_version"`
	MinorVersion int `json:"minor_version"`
	Map          struct {
		Name       string `json:"name"`
		UUID       string `json:"uuid"`
		Sysfs      string `json:"sysfs"`
		Failback   string `json:"failback"`
		Queueing   string `json:"queueing"`
		Paths      int    `json:"paths"`
		WriteProt  string `json:"write_prot"`
		DmSt       string `json:"dm_st"`
		Features   string `json:"features"`
		Hwhandler  string `json:"hwhandler"`
		Action     string `json:"action"`
		PathFaults int    `json:"path_faults"`
		Vend       string `json:"vend"`
		Prod       string `json:"prod"`
		Rev        string `json:"rev"`
		SwitchGrp  int    `json:"switch_grp"`
		MapLoads   int    `json:"map_loads"`
		TotalQTime int    `json:"total_q_time"`
		QTimeouts  int    `json:"q_timeouts"`
		PathGroups []struct {
			Selector   string `json:"selector"`
			Pri        int    `json:"pri"`
			DmSt       string `json:"dm_st"`
			MarginalSt string `json:"marginal_st"`
			Group      int    `json:"group"`
			Paths      []struct {
				Dev         string `json:"dev"`
				DevT        string `json:"dev_t"`
				DmSt        string `json:"dm_st"`
				DevSt       string `json:"dev_st"`
				ChkSt       string `json:"chk_st"`
				Checker     string `json:"checker"`
				Pri         int    `json:"pri"`
				HostWwnn    string `json:"host_wwnn"`
				TargetWwnn  string `json:"target_wwnn"`
				HostWwpn    string `json:"host_wwpn"`
				TargetWwpn  string `json:"target_wwpn"`
				HostAdapter string `json:"host_adapter"`
				MarginalSt  string `json:"marginal_st"`
			} `json:"paths"`
		} `json:"path_groups"`
	} `json:"map"`
}

func FindMultipathDeviceFromVolumePath(volumePath string) (string, error) {
	pathParts := strings.Split(volumePath, "/")
	lenPathParts := len(pathParts)
	if lenPathParts < 5 {
		return "", fmt.Errorf("error parsing volumepath %s, len %d", volumePath, lenPathParts)
	}
	volumeName := pathParts[lenPathParts-2]
	zlog.Trace().Msgf("volumeName parsed to [%s] from path [%s]", volumeName, volumePath)

	readFile, err := os.Open("/proc/mounts")
	if err != nil {
		zlog.Error().Msgf("error reading /proc/mounts %s", err.Error())
		return "", err
	}
	fileScanner := bufio.NewScanner(readFile)

	fileScanner.Split(bufio.ScanLines)

	var device string
	for fileScanner.Scan() {
		parts := strings.Split(fileScanner.Text(), " ")
		device = parts[0]
		mountPath := parts[1]
		zlog.Trace().Msgf("looking for %s in %s", volumeName, fileScanner.Text())
		if strings.Contains(mountPath, volumeName) {
			zlog.Debug().Msgf("found %s in %s for volume %s", device, mountPath, volumeName)
			break
		}
	}

	if err := readFile.Close(); err != nil {
		zlog.Error().Msgf("error in Close() %s", err.Error())
	}

	if device == "" {
		return "", fmt.Errorf("error finding device from volume in list of mounts - volume %s", volumeName)
	}

	return device, nil
}

// return the dm device path
func GetDMDevicePath(wwid string) (dmDevice string) {
	ioHandler := &OSioHandler{}
	defer helper.TimeTrack(zlog, time.Now())
	wwid = strings.TrimPrefix(wwid, "naa.")
	zlog.Debug().Msgf("wwid [%s]", wwid)
	FcPath := "scsi-3" + wwid
	DevID := "/host/dev/disk/by-id/"
	if dirs, err := ioHandler.ReadDir(DevID); err == nil {
		zlog.Debug().Msgf("read %d dirs", len(dirs))
		for _, f := range dirs {
			name := f.Name()
			zlog.Trace().Msgf("comparing [%s] to [%s] evaluating sym link for [%s]", FcPath, name, DevID+name)
			if name == FcPath {
				dmResult, err := ioHandler.EvalSymlinks(DevID + name)
				if err != nil {
					zlog.Error().Msgf("fc: failed to find a corresponding disk from symlink[%s], error %v", DevID+name, err)
					return ""
				}
				zlog.Debug().Msgf("EvalSymLinks matched return dm [%s]", dmResult)

				return dmResult
			}
		}
	}
	zlog.Error().Msgf("failed to find a dm [%s]", DevID+FcPath)
	return ""
}

func RescanDeviceMap(hosts []string, diskid string, lun string) (string, error) {
	defer helper.TimeTrack(zlog, time.Now())
	// deviceMu.Lock()
	zlog.Debug().Msgf("Rescan hosts for diskid '%s' and lun '%s'", diskid, lun)

	// For each host, scan using lun
	for _, host := range hosts {
		scsiHostPath := fmt.Sprintf("/sys/class/scsi_host/host%s/scan", host)
		zlog.Debug().Msgf("Rescanning host path at '%s' for disk ID '%s' and lun '%s'", scsiHostPath, diskid, lun)
		_, _, err := ExecCommand.Command("echo", fmt.Sprintf("'- - %s' > %s", lun, scsiHostPath))
		if err != nil {
			zlog.Error().Msgf("Rescan of host %s failed for volume ID '%s' and lun '%s': %s", scsiHostPath, diskid, lun, err)
			return "", err
		}
	}

	var wwid string
	var err error
	for _, host := range hosts {
		wwid, err = WaitForDeviceState(host, lun, "running", diskid)
		if err != nil {
			zlog.Error().Msgf("waitForDeviceState hosts failed for host [%s] diskid [%s] lun [%s] error [%s]", host, diskid, lun, err.Error())
			return "", err
		}
		// wwid that is not empty string means we found a wwid and dont need to look at other devices
		if wwid != "" {
			break
		}
	}

	for _, host := range hosts {
		if err := WaitForMultipath(host, lun); err != nil {
			zlog.Error().Msgf("Rescan hosts failed for host [%s] diskid [%s] lun [%s] error [%s]", host, diskid, lun, err.Error())
			return "", err
		}
	}

	zlog.Debug().Msgf("Rescan hosts complete for diskid '%s' and lun '%s'", diskid, lun)
	return wwid, nil
}

func WaitForDeviceState(hostID string, lun string, state string, diskid string) (wwid string, err error) {
	const functionName = "waitForDeviceState"
	zlog.Debug().Msgf("%s hostid %s lun %s state %s diskid %s", functionName, hostID, lun, state, diskid)
	targetsPath := fmt.Sprintf("/sys/class/scsi_disk/%s:*:*:%s", hostID, lun)
	targets, err := filepath.Glob(targetsPath)
	if err != nil || len(targets) == 0 {
		zlog.Warn().Msgf("%s - no fc targets found at path %s: %+v", functionName, targetsPath, err)
		return "", nil
	}

	allWWIDs := make([]string, 0)

	for _, targetString := range targets {
		target := strings.Split(targetString, ":")[2]
		channel := strings.Split(targetString, ":")[1]
		wwid, _ = WaitForOneDeviceState(hostID, channel, target, lun, state)
		allWWIDs = append(allWWIDs, wwid)
	}

	if len(allWWIDs) == 1 {
		zlog.Debug().Msgf("%s - only 1 wwid found [%s]", functionName, wwid)
		return wwid, nil
	}

	if len(allWWIDs) == 0 {
		return "", fmt.Errorf("%s - no wwid found", functionName)
	}

	// this logic uses the disk id to find the correct wwid when there
	// are multiple wwids for different targets.
	// in this case, the disk id looks like:
	// 5742b0f0000bbd11
	// note: that the disk ID is really the FC port Target WWPN on an ibox
	// and the wwids look like naa.6742b0f000000bbd00000000002b3e13
	// we parse out enough unique FC port characters (the key) from the diskid to perform a fuzzy search with
	// on the wwid, in this example 'bdd' is the key we use to search for the correct wwid
	zlog.Debug().Msgf("%s - picking the wwid from multiple wwid - diskid %s allWWIDs are [%v]", functionName, diskid, allWWIDs)
	n := 5
	lastChars := diskid[len(diskid)-n:]
	key := lastChars[:3]
	for _, wwid := range allWWIDs {
		if strings.Contains(wwid, key) {
			zlog.Debug().Msgf("%s - matched wwid using diskid %s key %s, wwid %s", functionName, diskid, key, wwid)
			return wwid, nil
		}
	}

	return "", fmt.Errorf("%s - could not determine the wwid", functionName)
}

func WaitForOneDeviceState(hostID string, channel string, target string, lun string, state string) (string, error) {
	const functionName = "waitForOneDeviceState"
	zlog.Debug().Msgf("%s hostid %s target %s lun %s state %s", functionName, hostID, target, lun, state)
	// Wait for device to be in state.
	var sleepCount time.Duration = 1
	hostPath := fmt.Sprintf("/sys/class/scsi_disk/%s:%s:%s:%s/device/state", hostID, channel, target, lun)
	wwidPath := fmt.Sprintf("/sys/class/scsi_disk/%s:%s:%s:%s/device/wwid", hostID, channel, target, lun)

	var wwid string
	zlog.Debug().Msgf("%s - checking device state within %s", functionName, hostPath)
	for sleepIteration := 1; sleepIteration <= 5; sleepIteration++ {
		// Get state of device
		hostOutput, _, err := ExecCommand.Command("cat", hostPath)
		if err != nil {
			zlog.Warn().Msgf("%s - Failed (%d): Cannot check state of device file %s: %s", functionName, sleepIteration, hostPath, err)
		}
		deviceState := strings.TrimSpace(string(hostOutput))

		// Get wwid of device
		wwidOutput, _, err := ExecCommand.Command("cat", wwidPath)
		if err != nil {
			zlog.Warn().Msgf("%s - Failed (%d): Cannot get wwid of wwid file %s: %s", functionName, sleepIteration, wwidPath, err)
		} else {
			wwid = strings.TrimSpace(wwidOutput)
			zlog.Debug().Msgf("%s - Device %s has wwid '%s'", functionName, wwidPath, wwid)
		}

		if err != nil || deviceState != state {
			if sleepIteration == 5 {
				msg := fmt.Sprintf("%s - Device %s is not in state '%s'. Current state is '%s'", functionName, hostPath, state, deviceState)
				zlog.Warn().Msg(msg)
			}
			time.Sleep(sleepCount * time.Second)
		} else {
			zlog.Debug().Msgf("%s - Device %s is in state '%s'", functionName, hostPath, state)
			break
		}
	}
	return wwid, nil
}

func WaitForMultipath(hostID string, lun string) error {
	const functionName = "waitForMultipath"
	defer helper.TimeTrack(zlog, time.Now())
	const defaultMultipathWait = 250
	var sleepCount time.Duration
	sleepCount = time.Duration(defaultMultipathWait)
	tmp := os.Getenv(MULTIPATH_WAIT)
	if tmp != "" {
		userSpecifiedValue, err := strconv.Atoi(tmp)
		if err != nil {
			zlog.Error().Msgf("%s - error converting user specified env var %s, using default value of %d instead", functionName, MULTIPATH_WAIT, defaultMultipathWait)
		} else {
			zlog.Warn().Msgf("%s - using non-default value for %s env var, user has specified %d, default is %d", functionName, MULTIPATH_WAIT, userSpecifiedValue, defaultMultipathWait)
			sleepCount = time.Duration(userSpecifiedValue)
		}
	}
	masterPath := fmt.Sprintf("/sys/class/scsi_disk/%s:*:*:%s/device/block/*/holders/*/slaves/*", hostID, lun)
	loopCount := 40
	for sleepIteration := 1; sleepIteration <= loopCount; sleepIteration++ {
		zlog.Trace().Msgf("%s - looping in waitForMultipath host %s lun %s", functionName, hostID, lun)
		devices, err := filepath.Glob(masterPath)
		if err != nil {
			zlog.Debug().Msgf("%s - failed to glob devices using path '%s': %+v", functionName, masterPath, err)
		} else {
			zlog.Trace().Msgf("%s - glob devices '%s'", functionName, devices)
		}

		if err != nil || len(devices) < mpathDeviceCount {
			if sleepIteration == loopCount {
				msg := fmt.Sprintf("%s - Multipath device found only %d devices for host ID '%s' and lun '%s'", functionName, len(devices), hostID, lun)
				zlog.Warn().Msg(msg)
			}
			time.Sleep(sleepCount * time.Millisecond)
		} else {
			break
		}
	}

	zlog.Debug().Msgf("%s - multipath device is online for host ID %s and lun '%s'", functionName, hostID, lun)
	return nil
}

// FindSlaveDevicesOnMultipath returns all slaves on the multipath device given the device path
func FindSlaveDevicesOnMultipath(dmDevice string) ([]string, error) {
	const functionName = "findSlaveDevicesOnMultipath"
	devices := []string{}
	// Split path /dev/dm-1 into "", "dev", "dm-1"
	parts := strings.Split(dmDevice, "/")
	if len(parts) != 3 || !strings.HasPrefix(parts[1], "dev") {
		err := fmt.Errorf("%s() for dm '%s' failed", functionName, dmDevice)
		zlog.Error().Msg(err.Error())
		return nil, err
	}
	disk := parts[2]
	slavesPath := path.Join("/sys/block/", disk, "/slaves/")

	files, err := os.ReadDir(slavesPath)
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		devices = append(devices, path.Join("/dev/", f.Name()))
	}
	if len(devices) == 0 {
		err := fmt.Errorf("%s for dm %s found no devices", functionName, dmDevice)
		zlog.Error().Msg(err.Error())
		return nil, err
	}
	return devices, nil
}

func GetPortInfo() (ports []PortInfo) {
	const functionName = "getPortInfo"
	leadPart := "/sys/class/fc_host/host"
	goFiles, err := filepath.Glob("/sys/class/fc_host/host*")
	if err != nil {
		fmt.Printf("%s - failed. error: %s", functionName, err.Error())
		return ports
	}
	for _, file := range goFiles {
		fmt.Println(file)
		data, err := os.ReadFile(file + "/port_name")
		if err != nil {
			fmt.Printf("%s - unable to read port_name file. error: %s", functionName, err.Error())

			continue
		}

		hostID := strings.Replace(file, leadPart, "", 1)
		portName := strings.TrimSpace(string(data))
		portName = strings.Replace(portName, "0x", "", 1)

		data, err = os.ReadFile(file + "/port_state")
		if err != nil {
			fmt.Printf("%s - getPortName unable to read port_state file. error: %s", functionName, err.Error())

			continue
		}
		portState := strings.TrimSpace(string(data))
		pi := PortInfo{
			HostID:    hostID,
			PortName:  portName,
			PortState: portState,
		}
		ports = append(ports, pi) // test, add both Online and other Ports
	}
	return ports
}

func removeMultipathDevices(devices []string) error {
	zlog.Debug().Msgf("removeMultipathDevices() called with hosts %+v", devices)

	for _, device := range devices {
		command := fmt.Sprintf("multipathd del path %s", device)
		pipefailCmd := fmt.Sprintf("set -o pipefail; %s", command)

		// we only care about the stdout, you can get stderr output from multipath.conf (invalid and deprecated lines)
		out, err := exec.Command("bash", "-c", pipefailCmd).Output()
		if err != nil {
			zlog.Error().Msgf("%s command failed %s", command, err.Error())
		} else {
			zlog.Debug().Msgf("%s command succeeded %s", command, out)
		}
	}
	return nil
}

// removeWWIDEntry causes WWID entries to be removed/cleaned up in /etc/multipath/wwids
// this is accomplished by runnning 'multipath -w %s' (or WWID)'", mpath
func removeWWIDEntry(mpath string) error {
	command := fmt.Sprintf("multipath -w  %s", mpath)
	pipefailCmd := fmt.Sprintf("set -o pipefail; %s", command)
	zlog.Debug().Msgf("command [%s]", command)

	// we only care about the stdout, you can get stderro output from multipath.conf being misconfigured
	out, err := exec.Command("bash", "-c", pipefailCmd).Output()
	if err != nil {
		zlog.Error().Msgf("%s command failed %s", command, err.Error())
	} else {
		zlog.Debug().Msgf("%s command succeeded: %s", command, out)
	}
	return nil
}

func findDevicesForMpath(mpath string) (devices []string, err error) {
	const functionName = "findDevicesForMpath"
	command := fmt.Sprintf("multipathd show multipath %s json", mpath)
	pipefailCmd := fmt.Sprintf("set -o pipefail; %s", command)
	zlog.Debug().Msgf("%s - command [%s]", functionName, command)

	// we only care about the stdout, you can get stderro output from multipath.conf being misconfigured
	out, err := exec.Command("bash", "-c", pipefailCmd).Output()
	if err != nil {
		e := fmt.Errorf("%s - mpath: %s, error: %s", functionName, mpath, err)
		zlog.Error().Msg(e.Error())
		return devices, e
	}
	var mpathOutput ShowMultipathOutput
	err = json.Unmarshal(out, &mpathOutput)
	if err != nil {
		e := fmt.Errorf("%s - error unmarshalling output: %s, error: %s", functionName, string(out), err)
		zlog.Error().Msg(e.Error())
		return devices, e
	}

	pathGroups := mpathOutput.Map.PathGroups
	for i := range pathGroups {
		paths := pathGroups[i]
		for j := range paths.Paths {
			devices = append(devices, "/dev/"+paths.Paths[j].Dev)
		}
	}

	zlog.Debug().Msgf("%s - devices %v for multipath %s", functionName, devices, mpath)
	return devices, nil
}

// Given a device like '/dev/dm-0', find its matching multipath name such as 'mpathab'.
func FindMpathFromDevice(device string) (mpath string, err error) {
	const functionName = "findMpathFromDevice"
	deviceName := strings.Replace(device, "/dev/", "", 1)
	wildcards := "\"%n_%d_\""
	command := fmt.Sprintf("multipathd show maps raw format %s | grep %s", wildcards, deviceName)
	out, _, err := ExecCommand.Command(command, "")
	if err != nil {
		e := fmt.Errorf("%s - command: %s error: %s", functionName, command, err.Error())
		zlog.Error().Msg(e.Error())
		return "", e
	}
	zlog.Debug().Msgf("%s - command [%s]", functionName, command)

	outParts := strings.Split(out, "_")
	if len(outParts) < 1 {
		e := fmt.Errorf("%s - cannot correctly parse findMpathFromDevice: %s, out: %s", functionName, device, outParts)
		zlog.Error().Msg(e.Error())
		return mpath, e
	}
	if len(outParts) > 0 {
		mpath = outParts[0]
	}

	zlog.Debug().Msgf("%s - device %s corresponds to multipath %s", functionName, device, mpath)
	return
}

func DetachMpathDevice(mpathDevice string, protocol string) error {
	const functionName = "detachMpathDevice"
	var err error
	var devices []string
	dstPath := mpathDevice
	var mpath string
	zlog.Debug().Msgf("%s called with mpathDevice '%s' for protocol '%s'", functionName, mpathDevice, protocol)
	if dstPath != "" {
		if strings.HasPrefix(dstPath, "/host") {
			dstPath = strings.Replace(dstPath, "/host", "", 1)
		}

		if strings.HasPrefix(dstPath, "/dev/dm-") {
			// older versions of the driver < 2.21.0 would pass a dm- device here instead of an mpath name
			devices, err = FindSlaveDevicesOnMultipath(dstPath)
			if err != nil {
				zlog.Error().Msgf("%s - error looking for slave devices for multipath [%s]", functionName, dstPath)
				return err
			}
			mpath, err = FindMpathFromDevice(mpathDevice)
			if err != nil {
				zlog.Error().Msgf("%s - for mpathDevice %s failed: %s", functionName, mpathDevice, err)
				return err
			}
		} else {
			mpath = mpathDevice
			devices, err = findDevicesForMpath(mpath)
			if err != nil {
				zlog.Error().Msgf("%s - error looking for devices for multipath [%s]", functionName, mpath)
				return err
			}
		}

		helper.PrettyKlogDebug("multipath devices", devices)

		zlog.Debug().Msgf("%s - mpath device is %s", functionName, mpath)

		// 1
		multipathFlush(mpath)

		const defaultSleepAfterFlush = 1
		sleepAfterFlushThisExecution := defaultSleepAfterFlush
		tmp := os.Getenv(MULTIPATH_CLEANUP_DELAY)
		if tmp != "" {
			userSpecifiedValue, err := strconv.Atoi(tmp)
			if err != nil {
				zlog.Error().Msgf("%s - conversion of %s env var failed, using default value of %d instead", functionName, MULTIPATH_CLEANUP_DELAY, defaultSleepAfterFlush)
			} else {
				sleepAfterFlushThisExecution = userSpecifiedValue
				zlog.Warn().Msgf("%s - using non-default value for %s env var, user has specified %d, default is %d", functionName, MULTIPATH_CLEANUP_DELAY, sleepAfterFlushThisExecution, defaultSleepAfterFlush)
			}
		}
		zlog.Debug().Msgf("%s - sleeping in between flush of device and detach of scsi disks for %d seconds", functionName, sleepAfterFlushThisExecution)
		time.Sleep(time.Second * time.Duration(sleepAfterFlushThisExecution))

		// Warn if there are not exactly mpathDeviceCount devices
		if deviceCount := len(devices); deviceCount != mpathDeviceCount {
			zlog.Warn().Msgf("%s - invalid mpath device count found while unstaging. Devices: %+v", functionName, devices)
		}

		// 2
		for i := range devices {
			err = detachDiskByDeviceName(devices[i])
			if err != nil {
				zlog.Error().Msgf("%s - error : %s", functionName, err)
			}
		}

		// 3
		err = removeMultipathDevices(devices)
		if err != nil {
			zlog.Debug().Msgf("%s - error from removeMultipathDevices but continuing: %s", functionName, err.Error())
		}

		// 4
		err = removeWWIDEntry(mpath)
		if err != nil {
			zlog.Debug().Msgf("%s - error from removeWWIDEntry but continuing: %s", functionName, err.Error())
		}
	}
	zlog.Debug().Msgf("%s completed with mpathDevice '%s' for protocol '%s'", functionName, mpathDevice, protocol)
	return nil
}

func removeOneFromScsiSubsystemByHostLun(host string, channel string, target string, lun string) (err error) {
	const functionName = "removeOneFromScsiSubsystemByHostLun"
	// fileName := "/sys/block/" + deviceName + "/device/delete"
	// zlog.Debug().Msgf("remove device from scsi-subsystem: path: %s", fileName)
	// data := []byte("1\n")
	// ioutil.WriteFile(fileName, data, 0666)
	// zlog.Debug().Msgf("Flush device '%s' output: %s", device, blockdevOut)

	defer func() {
		zlog.Debug().Msgf("%s with host %s, channel %s, target %s and lun %s completed", functionName, host, channel, target, lun)
	}()

	zlog.Debug().Msgf("%s called with host %s, channel %s target %s and lun %s", functionName, host, channel, target, lun)

	deletePath := fmt.Sprintf("/sys/class/scsi_disk/%s:%s:%s:%s/device/delete", host, channel, target, lun)
	statePath := fmt.Sprintf("/sys/class/scsi_disk/%s:%s:%s:%s/device/state", host, channel, target, lun)
	var output string

	// Check device is in blocked state.
	var sleepCount time.Duration
	for sleepIteration := 1; sleepIteration <= 5; sleepIteration++ {
		// Get state of device
		zlog.Debug().Msgf("%s - checking device state of %s", functionName, statePath)
		output, _, err = ExecCommand.Command("cat", statePath)
		if err != nil {
			zlog.Error().Msgf("%s - error: cannot check state of %s", functionName, statePath)
			return
		}
		deviceState := strings.TrimSpace(string(output))
		if deviceState == "blocked" {
			if sleepIteration == 5 {
				err = fmt.Errorf("%s - Device %s is blocked", functionName, statePath)
				zlog.Error().Msg(err.Error())
				return
			}
			time.Sleep(sleepCount * time.Second)
		} else {
			break
		}
	}

	// Echo 1 to delete device
	output, _, err = ExecCommand.Command("echo", fmt.Sprintf("1 > %s", deletePath))
	if err != nil {
		zlog.Error().Msgf("%s - error failed to delete device '%s' with output '%s' and error '%v'", functionName, deletePath, output, err.Error())
		return
	}

	// Stat device
	if _, err := os.Stat(deletePath); err == nil {
		zlog.Warn().Msgf("%s - Device %s still exists", functionName, deletePath)
	} else if errors.Is(err, os.ErrNotExist) {
		zlog.Debug().Msgf("%s - Device %s no longer exists", functionName, deletePath)
		return nil
	} else {
		zlog.Debug().Msgf("%s - Device %s may or may not exist. See error: %s", functionName, deletePath, err)
	}

	return err
}

func detachDiskByDeviceName(deviceName string) error {
	const functionName = "detatchDiskByDeviceName"
	// we get in a device name like /dev/sda
	zlog.Debug().Msgf("%s - %s called", functionName, deviceName)
	deviceNameParts := strings.Split(deviceName, "/")
	if len(deviceNameParts) != 3 {
		return fmt.Errorf("%s - device name %s did not parse to 3 parts as normal", functionName, deviceName)
	}
	zlog.Trace().Msgf("%s length = %d, parts are [%v] one=[%s]", functionName, len(deviceNameParts), deviceNameParts, deviceNameParts[2])

	blockPath := fmt.Sprintf("/sys/block/%s/device", deviceNameParts[2])
	zlog.Debug().Msgf("%s - blockpath [%s]", functionName, blockPath)
	hctlPath, err := filepath.EvalSymlinks(blockPath)
	if err != nil {
		return err
	}

	// here we are expecting hctlPath to be similar to:
	// /sys/devices/pci0000:00/0000:00:15.0/0000:03:00.0/host32/rport-32:0-7/target32:0:9/32:0:9:1
	// we want the last part which is the H:C:T:L

	hctlPathParts := strings.Split(hctlPath, "/")

	hctl := hctlPathParts[len(hctlPathParts)-1]
	zlog.Trace().Msgf("%s - hctl path [%s] - parsed as [%s]", functionName, hctlPath, hctl)

	hctlParts := strings.Split(hctl, ":")

	host := hctlParts[0]
	channel := hctlParts[1]
	target := hctlParts[2]
	lun := hctlParts[3]
	zlog.Debug().Msgf("%s - hctl path [%s] host [%s] channel [%s] target [%s] lun [%s]", functionName, hctlPath, host, channel, target, lun)
	err = removeOneFromScsiSubsystemByHostLun(host, channel, target, lun)
	if err != nil {
		return err
	}

	return nil
}

// Flush a multipath device map for device.

func multipathFlush(mpath string) {
	const functionName = "multipathFlush"
	zlog.Debug().Msgf("%s - Running multipath -f '%s'", functionName, mpath)

	isToLogOutput := true
	if out, _, err := ExecCommand.Command("multipath", fmt.Sprintf("-f %s", mpath), isToLogOutput); err != nil {
		zlog.Error().Msgf("%s - multipath -f '%s' failed - ignored: %s", functionName, mpath, err)
	} else {
		zlog.Debug().Msgf("%s - multipath -f '%s' succeeded: %s", functionName, mpath, out)
	}

	// _, _ = execScsi.Command("ls", "-l /host/dev/mapper/*; echo", isToLogOutput)
	// _, _ = execScsi.Command("ls", "/host/dev/sd*; echo", isToLogOutput)
}
