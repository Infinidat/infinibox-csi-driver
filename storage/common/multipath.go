/*
Copyright 2026 Infinidat
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

package common

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/infinidat/infinibox-csi-driver/helper"
)

const (
	MultipathWait                           = "MULTIPATH_WAIT"
	mpathDeviceCount                    int = 6
	MultipathCleanupDelay                   = "MULTIPATH_CLEANUP_DELAY"
	FCSearchDiskDelay                       = "FC_SEARCH_DISK_DELAY"
	defaultFlushRetries                     = 6
	defaultFlushRetriesDelaySeconds         = 5
	MULTIPATH_FLUSH_RETRIES                 = "MULTIPATH_FLUSH_RETRIES"
	MULTIPATH_FLUSH_RETRY_DELAY_SECONDS     = "MULTIPATH_FLUSH_RETRY_DELAY_SECONDS"
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
		slog.Error(e.Error())
		return infos, e
	}
	infos = make([]fs.FileInfo, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			e := fmt.Errorf("ReadDir error %s", err.Error())
			slog.Error(e.Error())
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
	ctx := context.Background()
	pathParts := strings.Split(volumePath, "/")
	lenPathParts := len(pathParts)
	if lenPathParts < 5 {
		return "", common.Errorf("error parsing volumepath %s, len %d", volumePath, lenPathParts)
	}
	volumeName := pathParts[lenPathParts-2]
	slog.Log(ctx, common.LevelTrace, "parsed", "volumeName", volumeName, "volumePath", volumePath)

	readFile, err := os.Open("/proc/mounts")
	if err != nil {
		return "", common.Errorf("error reading /proc/mounts error %w", err)
	}
	fileScanner := bufio.NewScanner(readFile)
	if fileScanner.Err() != nil {
		return "", common.Errorf("error creating filescanner for reading /proc/mounts error %w", err)
	}

	fileScanner.Split(bufio.ScanLines)

	var device string
	for fileScanner.Scan() {
		parts := strings.Split(fileScanner.Text(), " ")
		device = parts[0]
		mountPath := parts[1]
		slog.Log(ctx, common.LevelTrace, "looking for", "volumeName", volumeName, "in", fileScanner.Text())
		if strings.Contains(mountPath, volumeName) {
			slog.Debug("found", "device", device, "mountPath", mountPath, "volumeName", volumeName)
			break
		}
	}

	if err := readFile.Close(); err != nil {
		slog.Error("error in Close()", "error", err.Error())
	}

	if device == "" {
		return "", common.Errorf("error finding device from volume in list of mounts - volume %s", volumeName)
	}

	return device, nil
}

// return the dm device path
func GetDMDevicePath(wwid string) (dmDevice string) {
	ctx := context.Background()
	ioHandler := &OSioHandler{}
	defer helper.TimeTrack(time.Now())
	wwid = strings.TrimPrefix(wwid, "naa.")
	slog.Debug("value", "wwid", wwid)
	FcPath := "scsi-3" + wwid
	DevID := "/host/dev/disk/by-id/"
	if dirs, err := ioHandler.ReadDir(DevID); err == nil {
		slog.Debug("read", "dir count", len(dirs))
		for _, f := range dirs {
			name := f.Name()
			slog.Log(ctx, common.LevelTrace, "comparing", "fcPath", FcPath, "to", name, "evaluating sym link for", DevID+name)
			if name == FcPath {
				dmResult, err := ioHandler.EvalSymlinks(DevID + name)
				if err != nil {
					slog.Error("fc: failed to find a corresponding disk from", "symlink", DevID+name, "error", err)
					return ""
				}
				slog.Debug("EvalSymLinks matched return", "dm", dmResult)

				return dmResult
			}
		}
	}
	slog.Error("failed to find a dm", "dm", DevID+FcPath)
	return ""
}

func RescanDeviceMap(hosts []string, diskID string, lun string) (string, error) {
	defer helper.TimeTrack(time.Now())
	// deviceMu.Lock()
	slog.Debug("Rescan hosts", "diskid", diskID, "lun", lun)

	// For each host, scan using lun
	for _, host := range hosts {
		scsiHostPath := fmt.Sprintf("/sys/class/scsi_host/host%s/scan", host)
		slog.Debug("Rescanning", "host path", scsiHostPath, "disk ID", diskID, "lun", lun)
		_, _, err := ExecCommand.Command("echo", fmt.Sprintf("'- - %s' > %s", lun, scsiHostPath))
		if err != nil {
			return "", common.Errorf("rescan of host failed scsiHostPath %s volume ID %s lun %s error %w", scsiHostPath, diskID, lun, err)
		}
	}

	var wwid string
	var err error
	for _, host := range hosts {
		wwid, err = WaitForDeviceState(host, lun, "running", diskID)
		if err != nil {
			return "", common.Errorf("waitForDeviceState failed host %s diskID %s lun %s error %w", host, diskID, lun, err)
		}
		// wwid that is not empty string means we found a wwid and dont need to look at other devices
		if wwid != "" {
			break
		}
	}

	for _, host := range hosts {
		if err := WaitForMultipath(host, lun); err != nil {
			return "", common.Errorf("rescan failed host %s diskID %s lun %s error %w", host, diskID, lun, err)
		}
	}

	slog.Debug("rescan hosts complete", "diskid", diskID, "lun", lun)

	if wwid == "" {
		return "", common.Errorf("searchDisk rescan error wwid not found hosts %v lun %s diskID %s", hosts, lun, diskID)
	}

	return wwid, nil
}

func WaitForDeviceState(hostID string, lun string, state string, diskID string) (wwid string, err error) {
	slog.Debug("info", "hostid", hostID, "lun", lun, "state", state, "diskid", diskID)
	targetsPath := fmt.Sprintf("/sys/class/scsi_disk/%s:*:*:%s", hostID, lun)
	targets, err := filepath.Glob(targetsPath)
	if err != nil || len(targets) == 0 {
		slog.Warn("no fc targets found", "targetsPath", targetsPath, "error", err)
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
		slog.Debug("only 1 wwid found", "wwid", wwid)
		return wwid, nil
	}

	if len(allWWIDs) == 0 {
		return "", common.Errorf("no wwid found hostID %s lun %s state %s diskID %s", hostID, lun, state, diskID)
	}

	// this logic uses the disk id to find the correct wwid when there
	// are multiple wwids for different targets.
	// in this case, the disk id looks like:
	// 5742b0f0000bbd11
	// note: that the disk ID is really the FC port Target WWPN on an ibox
	// and the wwids look like naa.6742b0f000000bbd00000000002b3e13
	// we parse out enough unique FC port characters (the key) from the diskid to perform a fuzzy search with
	// on the wwid, in this example 'bdd' is the key we use to search for the correct wwid
	slog.Debug("picking the wwid from multiple wwid", "diskid", diskID, "allWWIDs are", allWWIDs)
	n := 5
	lastChars := diskID[len(diskID)-n:]
	key := lastChars[:3]
	for _, wwid := range allWWIDs {
		if strings.Contains(wwid, key) {
			slog.Debug(" matched wwid ", "diskid", diskID, "key", key, "wwid", wwid)
			return wwid, nil
		}
	}

	return "", fmt.Errorf("could not determine the wwid")
}

func WaitForOneDeviceState(hostID string, channel string, target string, lun string, state string) (string, error) {
	slog.Debug("info", "hostid", hostID, "target", target, "lun", lun, "state", state)
	// Wait for device to be in state.
	var sleepCount time.Duration = 1
	hostPath := fmt.Sprintf("/sys/class/scsi_disk/%s:%s:%s:%s/device/state", hostID, channel, target, lun)
	wwidPath := fmt.Sprintf("/sys/class/scsi_disk/%s:%s:%s:%s/device/wwid", hostID, channel, target, lun)

	var wwid string
	slog.Debug("checking device state", "within hostPath", hostPath)
	for sleepIteration := 1; sleepIteration <= 5; sleepIteration++ {
		// Get state of device
		hostOutput, _, err := ExecCommand.Command("cat", hostPath)
		if err != nil {
			slog.Warn(" Failed: Cannot check state of device file", "hostpath", hostPath, "sleepIteration", sleepIteration, "error", err)
		}
		deviceState := strings.TrimSpace(hostOutput)

		// Get wwid of device
		wwidOutput, _, err := ExecCommand.Command("cat", wwidPath)
		if err != nil {
			slog.Warn(" Failed: Cannot get wwid", "wwidPath", wwidPath, "sleepiteration", sleepIteration, "error", err)
		} else {
			wwid = strings.TrimSpace(wwidOutput)
			slog.Debug("info", "Device", wwidPath, "has wwid", wwid)
		}

		if err != nil || deviceState != state {
			if sleepIteration == 5 {
				msg := fmt.Sprintf("Device %s is not in state '%s'. Current state is '%s'", hostPath, state, deviceState)
				slog.Warn(msg)
			}
			time.Sleep(sleepCount * time.Second)
		} else {
			slog.Debug("info", "Device", hostPath, "is in state", state)
			break
		}
	}
	return wwid, nil
}

func WaitForMultipath(hostID string, lun string) error {
	ctx := context.Background()
	defer helper.TimeTrack(time.Now())
	const defaultMultipathWait = 250
	var sleepCount time.Duration
	sleepCount = time.Duration(defaultMultipathWait)
	tmp := os.Getenv(MultipathWait)
	if tmp != "" {
		userSpecifiedValue, err := strconv.Atoi(tmp)
		if err != nil {
			slog.Error("error converting user specified env var", "multipathWait", MultipathWait, "default", defaultMultipathWait)
		} else {
			slog.Warn("using non-default value for", "env var", MultipathWait, "user specified", userSpecifiedValue, "default", defaultMultipathWait)
			sleepCount = time.Duration(userSpecifiedValue)
		}
	}
	masterPath := fmt.Sprintf("/sys/class/scsi_disk/%s:*:*:%s/device/block/*/holders/*/slaves/*", hostID, lun)
	loopCount := 40
	for sleepIteration := 1; sleepIteration <= loopCount; sleepIteration++ {
		slog.Log(ctx, common.LevelTrace, "looping in waitForMultipath", "host", hostID, "lun", lun)
		devices, err := filepath.Glob(masterPath)
		if err != nil {
			slog.Debug("failed to glob devices using", "path", masterPath, "error", err)
		} else {
			slog.Log(ctx, common.LevelTrace, "glob", "devices", devices)
		}

		if err != nil || len(devices) < mpathDeviceCount {
			if sleepIteration == loopCount {
				msg := fmt.Sprintf("Multipath device found only %d devices for host ID '%s' and lun '%s'", len(devices), hostID, lun)
				slog.Warn(msg)
			}
			time.Sleep(sleepCount * time.Millisecond)
		} else {
			break
		}
	}

	slog.Debug("multipath device is online ", "host ID", hostID, "lun", lun)
	return nil
}

// FindSlaveDevicesOnMultipath returns all slaves on the multipath device given the device path
func FindSlaveDevicesOnMultipath(dmDevice string) ([]string, error) {
	devices := []string{}
	// Split path /dev/dm-1 into "", "dev", "dm-1"
	parts := strings.Split(dmDevice, "/")
	if len(parts) != 3 || !strings.HasPrefix(parts[1], "dev") {
		return nil, common.Errorf("for dm '%s' failed", dmDevice)
	}
	disk := parts[2]
	slavesPath := path.Join("/sys/block/", disk, "/slaves/")

	files, err := os.ReadDir(slavesPath)
	if err != nil {
		return nil, common.Errorf("%w", err)
	}
	for _, f := range files {
		devices = append(devices, path.Join("/dev/", f.Name()))
	}
	if len(devices) == 0 {
		return nil, common.Errorf("for dm %s found no devices", dmDevice)
	}
	return devices, nil
}

func GetPortInfo() (ports []PortInfo) {
	leadPart := "/sys/class/fc_host/host"
	goFiles, err := filepath.Glob("/sys/class/fc_host/host*")
	if err != nil {
		slog.Error(err.Error())
		return ports
	}
	for _, file := range goFiles {
		slog.Info("file", "content", file)
		data, err := os.ReadFile(file + "/port_name")
		if err != nil {
			slog.Error(err.Error())
			continue
		}

		hostID := strings.Replace(file, leadPart, "", 1)
		portName := strings.TrimSpace(string(data))
		portName = strings.Replace(portName, "0x", "", 1)

		data, err = os.ReadFile(file + "/port_state")
		if err != nil {
			slog.Error(err.Error())
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

func removeMultipathDevices(device string) error {
	slog.Debug("removeMultipathDevices() called", "device", device)

	command := fmt.Sprintf("multipathd del path %s", device)
	pipefailCmd := fmt.Sprintf("set -o pipefail; %s", command)

	// we only care about the stdout, you can get stderr output from multipath.conf (invalid and deprecated lines)
	out, err := exec.Command("bash", "-c", pipefailCmd).Output()
	if err != nil {
		return common.Errorf("command failed %s error %w", command, err)
	}
	slog.Debug("command succeeded", "command", command, "output", out)
	return nil
}

// removeWWIDEntry causes WWID entries to be removed/cleaned up in /etc/multipath/wwids
// this is accomplished by runnning 'multipath -w %s' (or WWID)'", mpath
func removeWWIDEntry(mpath string) error {
	command := fmt.Sprintf("multipath -w  %s", mpath)
	pipefailCmd := fmt.Sprintf("set -o pipefail; %s", command)
	slog.Debug("command", "command", command)

	// we only care about the stdout, you can get stderro output from multipath.conf being misconfigured
	out, err := exec.Command("bash", "-c", pipefailCmd).Output()
	if err != nil {
		return common.Errorf("command failed command %s error %w", command, err)
	}
	slog.Debug("command succeeded", "command", command, "out", out)
	return nil
}

func findDevicesForMpath(mpath string) (devices []string, err error) {
	command := fmt.Sprintf("multipathd show multipath %s json", mpath)
	pipefailCmd := fmt.Sprintf("set -o pipefail; %s", command)
	slog.Debug("executing", "command", command)

	// we only care about the stdout, you can get stderro output from multipath.conf being misconfigured
	out, err := exec.Command("bash", "-c", pipefailCmd).Output()
	if err != nil {
		return devices, common.Errorf("command failed %s mpath: %s, error: %w", pipefailCmd, mpath, err)
	}
	var mpathOutput ShowMultipathOutput
	err = json.Unmarshal(out, &mpathOutput)
	if err != nil {
		return devices, common.Errorf("error unmarshalling output: %s, error: %w", string(out), err)
	}

	pathGroups := mpathOutput.Map.PathGroups
	for i := range pathGroups {
		paths := pathGroups[i]
		for j := range paths.Paths {
			devices = append(devices, "/dev/"+paths.Paths[j].Dev)
		}
	}

	slog.Debug("list", "devices", devices, "for multipath", mpath)
	return devices, nil
}

// Given a device like '/dev/dm-0', find its matching multipath name such as 'mpathab'.
func FindMpathFromDevice(device string) (mpath string, err error) {
	deviceName := strings.Replace(device, "/dev/", "", 1)
	wildcards := "\"%n_%d_\""
	command := fmt.Sprintf("multipathd show maps raw format %s | grep _%s_", wildcards, deviceName)
	out, _, err := ExecCommand.Command(command, "")
	if err != nil {
		return "", common.Errorf("command failed command: %s error: %w", command, err)
	}
	slog.Debug("executing", "command", command)

	outParts := strings.Split(out, "_")
	if len(outParts) < 1 {
		return mpath, common.Errorf("cannot correctly parse findMpathFromDevice: %s, out: %s", device, outParts)
	}
	if len(outParts) > 0 {
		mpath = outParts[0]
	}

	slog.Debug("corresponds", "device", device, "multipath", mpath)
	return
}

func DetachMpathDevice(mpathDevice string, protocol string) error {
	var err error
	var devices []string
	dstPath := mpathDevice
	var mpath string
	slog.Debug("called with", "mpathDevice", mpathDevice, "for protocol", protocol)

	if dstPath == "" {
		slog.Debug("completed", "with mpathDevice", mpathDevice, "protocol", protocol)
		return nil
	}

	if strings.HasPrefix(dstPath, "/host") {
		dstPath = strings.Replace(dstPath, "/host", "", 1)
	}

	if strings.HasPrefix(dstPath, "/dev/dm-") {
		// older versions of the driver < 2.21.0 would pass a dm- device here instead of an mpath name
		devices, err = FindSlaveDevicesOnMultipath(dstPath)
		if err != nil {
			return common.Errorf("error looking for slave devices for multipath %s error %w", dstPath, err)
		}
		mpath, err = FindMpathFromDevice(mpathDevice)
		if err != nil {
			return common.Errorf("%w", err)
		}
	} else {
		mpath = mpathDevice
		devices, err = findDevicesForMpath(mpath)
		if err != nil {
			return common.Errorf("%w", mpath, err)
		}
	}

	helper.PrettyKlogDebug("multipath devices", devices)

	slog.Debug("mpath", "device is", mpath)

	// 1
	err = multipathFlush(mpath)
	if err != nil {
		return err
	}

	const defaultSleepAfterFlush = 1
	sleepAfterFlushThisExecution := defaultSleepAfterFlush
	tmp := os.Getenv(MultipathCleanupDelay)
	if tmp != "" {
		userSpecifiedValue, err := strconv.Atoi(tmp)
		if err != nil {
			slog.Error("conversion failed", "env var", MultipathCleanupDelay, "using default value ", defaultSleepAfterFlush)
		} else {
			sleepAfterFlushThisExecution = userSpecifiedValue
			slog.Warn("using non-default value for", "env var", MultipathCleanupDelay, "user has specified", sleepAfterFlushThisExecution, "default is", defaultSleepAfterFlush)
		}
	}
	slog.Debug("sleeping in between flush of device and detach of scsi disks", "for seconds", sleepAfterFlushThisExecution)
	time.Sleep(time.Second * time.Duration(sleepAfterFlushThisExecution))

	// Warn if there are not exactly mpathDeviceCount devices
	if deviceCount := len(devices); deviceCount != mpathDeviceCount {
		slog.Warn("invalid mpath device count found while unstaging.", "Devices", devices)
	}

	// 2
	for i := range devices {
		err = detachDiskByDeviceName(devices[i])
		if err != nil {
			slog.Error(err.Error())
		}
	}

	// 3
	for _, device := range devices {
		err = removeMultipathDevices(device)
		if err != nil {
			slog.Debug("error from removeMultipathDevices but continuing", "error", err.Error())
		}
	}

	// 4
	err = removeWWIDEntry(mpath)
	if err != nil {
		slog.Debug("error from removeWWIDEntry but continuing", "error", err.Error())
	}
	slog.Debug("completed", "with mpathDevice", mpathDevice, "protocol", protocol)
	return nil
}

func removeOneFromScsiSubsystemByHostLun(host string, channel string, target string, lun string) (err error) {
	// fileName := "/sys/block/" + deviceName + "/device/delete"
	// slog.Debug().Msgf("remove device from scsi-subsystem: path: %s", fileName)
	// data := []byte("1\n")
	// ioutil.WriteFile(fileName, data, 0666)
	// slog.Debug().Msgf("Flush device '%s' output: %s", device, blockdevOut)

	defer func() {
		slog.Debug("completed", "with host", host, "channel", channel, "target", target, "lun", lun)
	}()

	slog.Debug("called", "host", host, "channel", channel, "target", target, "lun", lun)

	deletePath := fmt.Sprintf("/sys/class/scsi_disk/%s:%s:%s:%s/device/delete", host, channel, target, lun)
	statePath := fmt.Sprintf("/sys/class/scsi_disk/%s:%s:%s:%s/device/state", host, channel, target, lun)
	var output string

	// Check device is in blocked state.
	var sleepCount time.Duration
	for sleepIteration := 1; sleepIteration <= 5; sleepIteration++ {
		// Get state of device
		slog.Debug("checking device state ", "path", statePath)
		output, _, err = ExecCommand.Command("cat", statePath)
		if err != nil {
			slog.Error("error: cannot check state ", "path", statePath)
			return
		}
		deviceState := strings.TrimSpace(output)
		if deviceState == "blocked" {
			if sleepIteration == 5 {
				err = fmt.Errorf("device %s is blocked", statePath)
				slog.Error(err.Error())
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
		return common.Errorf("error failed to delete device %s output %s error %w", deletePath, output, err)
	}

	// Stat device
	if _, err := os.Stat(deletePath); err == nil {
		slog.Warn("Device still exists", "deletePath", deletePath)
	} else if errors.Is(err, os.ErrNotExist) {
		slog.Debug(" Device no longer exists", "deletePath", deletePath)
		return nil
	} else {
		slog.Debug(" Device may or may not exist.", "deletePath", deletePath, "error", err)
	}

	return err
}

func detachDiskByDeviceName(deviceName string) error {
	// we get in a device name like /dev/sda
	slog.Debug("called", "deviceName", deviceName)
	deviceNameParts := strings.Split(deviceName, "/")
	if len(deviceNameParts) != 3 {
		return common.Errorf("device name %s did not parse to 3 parts as normal", deviceName)
	}
	ctx := context.Background()
	slog.Log(ctx, common.LevelTrace, "device", "length", len(deviceNameParts), "parts", deviceNameParts, "one", deviceNameParts[2])

	blockPath := fmt.Sprintf("/sys/block/%s/device", deviceNameParts[2])
	slog.Debug("called", "blockpath", blockPath)
	hctlPath, err := filepath.EvalSymlinks(blockPath)
	if err != nil {
		return common.Errorf("%w", err)
	}

	// here we are expecting hctlPath to be similar to:
	// /sys/devices/pci0000:00/0000:00:15.0/0000:03:00.0/host32/rport-32:0-7/target32:0:9/32:0:9:1
	// we want the last part which is the H:C:T:L

	hctlPathParts := strings.Split(hctlPath, "/")

	hctl := hctlPathParts[len(hctlPathParts)-1]
	slog.Log(ctx, common.LevelTrace, "parsed", "hctl path", hctlPath, "hctl", hctl)

	hctlParts := strings.Split(hctl, ":")

	host := hctlParts[0]
	channel := hctlParts[1]
	target := hctlParts[2]
	lun := hctlParts[3]
	slog.Debug("details", "hctl path", hctlPath, "host", host, "channel", channel, "target", target, "lun", lun)
	err = removeOneFromScsiSubsystemByHostLun(host, channel, target, lun)
	if err != nil {
		return common.Errorf("%w", err)
	}

	return nil
}

// Flush a multipath device map for device.

func multipathFlush(mpath string) error {
	slog.Debug("running", "multipath -f", mpath)

	flushRetries := getIntEnvVar(MULTIPATH_FLUSH_RETRIES, defaultFlushRetries)
	flushRetriesDelay := getIntEnvVar(MULTIPATH_FLUSH_RETRY_DELAY_SECONDS, defaultFlushRetriesDelaySeconds)

	slog.Info("trying multipath flush", "retries", flushRetries, "retriesDelay", flushRetriesDelay)

	isToLogOutput := true
	for i := range flushRetries {
		out, _, err := ExecCommand.Command("multipath", fmt.Sprintf("-f %s", mpath), isToLogOutput)
		if err != nil && i == (flushRetries-1) {
			// give up on retrying and return an error
			slog.Info("giving up on multipath flush retries", "retryCount", i, "error", err)
			return err
		}
		if err != nil {
			slog.Error("multipath flush error, will retry", "retryCount", i, "retryDelaySeconds", defaultFlushRetriesDelaySeconds, "error", err)
		}
		if err == nil {
			slog.Info("multipath flush success", "mpath", mpath, "out", out)
			break
		}
		slog.Info("multipath flush - sleeping until next try...", "tries", i)
		time.Sleep(time.Second * time.Duration(flushRetriesDelay))
	}
	return nil

	// _, _ = execScsi.Command("ls", "-l /host/dev/mapper/*; echo", isToLogOutput)
	// _, _ = execScsi.Command("ls", "/host/dev/sd*; echo", isToLogOutput)
}

func getIntEnvVar(envVar string, defaultInt int) int {
	envVarValue := os.Getenv(envVar)
	if envVarValue == "" {
		slog.Warn("env var was empty, using default", "envvar", envVar, "default", defaultInt)
		return defaultInt
	}

	x, err := strconv.Atoi(envVarValue)
	if err != nil {
		slog.Error("could not convert env var to int, using default", "env var", envVar, "error", err, "default", defaultInt)
		return defaultInt
	}
	return x
}
