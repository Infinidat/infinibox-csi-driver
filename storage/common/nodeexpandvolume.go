/*
Copyright 2024 Infinidat
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
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/infinidat/infinibox-csi-driver/common"
)

const Resize2FSDelay = "RESIZE2FS_DELAY"

/*
BlockExpandVolume

here are the linux host steps required to resize the block volume, these commands
are implemented in the golang code....

<<pass in the volume path to the findmnt command to get the multipath device name>>

[root@csidev-ocp416 ~]# findmnt --noheadings -l -o SOURCE --target /var/lib/kubelet/plugins/kubernetes.io/csi/volumeDevices/publish/ocp416-03b8803572/7f222874-46fa-4c7f-93d1-0f7651dd7955
devtmpfs[/dm-1]

<< parse out /dm-1 >>

<<get the multipath devices >>
[root@csidev-ocp416 ~]# multipath -ll /dev/dm-1
mpatht (36742b0f000000bbd000000000000196b) dm-1 NFINIDAT,InfiniBox
size=1.0G features='0' hwhandler='1 alua' wp=rw
`-+- policy='round-robin 0' prio=50 status=active

	|- 33:0:1:2 sdf 8:80 active ready running
	|- 33:0:2:2 sdg 8:96 active ready running
	`- 33:0:0:2 sde 8:64 active ready running

alternatively

# multipathd show paths format "%m,%d" | grep mpatht
mpatht    ,sdf
mpatht    ,sdg
mpatht    ,sde

<< parse our the mpath devices (sdf, sdg, sde) to use for the echo rescan command >>
<< also parse out the mpath device name (e.g. mpatht) >>

<< echo to the device to cause a resize >>
$ sudo bash -c 'echo 1 > /sys/block/sdf/device/rescan'
$ sudo bash -c 'echo 1 > /sys/block/sdg/device/rescan'
$ sudo bash -c 'echo 1 > /sys/block/sde/device/rescan'

<< lastly, run the mpath resize map command on the mpath device name >>
$ multipathd resize map mpatht
*/
func BlockExpandVolume(volumePath string) error {
	findmntCommand := fmt.Sprintf("findmnt --noheadings -l -o SOURCE --target %s", volumePath)
	slog.Debug("executing", "command", findmntCommand)

	out, _, err := ExecCommand.Command(findmntCommand, "")
	if err != nil {
		return fmt.Errorf("error running findmnt command name %s - %s", findmntCommand, err.Error())
	}

	if out == "" {
		return fmt.Errorf("error getting findmnt command output %s on path %s", findmntCommand, volumePath)
	}

	output := strings.TrimSpace(out)
	outputParts := strings.Split(output, "[")
	if len(outputParts) < 2 {
		return fmt.Errorf("format error in findmnt command output %s on path %s output %v", findmntCommand, volumePath, outputParts)
	}
	deviceNameParts := strings.Split(outputParts[1], "]")
	if len(deviceNameParts) == 0 {
		return fmt.Errorf("format error in findmnt command output %s on path %s deviceParts %v", findmntCommand, volumePath, deviceNameParts)
	}

	multipathDevice := "/dev" + deviceNameParts[0]
	slog.Debug("findmnt", "output", output, "mpath device", multipathDevice)

	// multipathCommand := fmt.Sprintf("multipath -ll %s", multipathDevice)
	wildcards := "\"%n_/%d_\""
	multipathCommand := fmt.Sprintf("multipathd show maps raw format %s 2> /dev/null | grep %s", wildcards, deviceNameParts[0]+"_")
	slog.Debug("executing", "command", multipathCommand)
	out, _, err = ExecCommand.Command(multipathCommand, "")
	if err != nil {
		return fmt.Errorf("error running multipath command name %s - %s", multipathCommand, err.Error())
	}

	if out == "" {
		return errors.New("error getting multipath command output")
	}
	slog.Debug("multipathd show maps", "output", out)
	if out == "" {
		return fmt.Errorf("error with multipath command name output %s, lines were zero ", out)
	}
	multipathOutput := strings.Split(out, "_")
	userFriendlyName := multipathOutput[0]
	slog.Debug("user friendly name", "value", userFriendlyName)

	format := "\"%m,%d\""
	multipathdCommand := fmt.Sprintf("multipathd show paths format %s 2> /dev/null | grep %s", format, "\""+userFriendlyName+"\"")
	out, _, err = ExecCommand.Command(multipathdCommand, "")
	if err != nil {
		return fmt.Errorf("error running multipathd command name %s - %s", multipathdCommand, err.Error())
	}
	slog.Debug("command", "multipathd show paths", multipathdCommand)

	if out == "" {
		return errors.New("error getting multipathd command output")
	}
	slog.Debug("multipathd output", "output", out)
	devices := make([]string, 0)
	for _, line := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
		multipathdLine := strings.Split(line, ",")
		if len(multipathdLine) < 2 {
			return fmt.Errorf("error in multipathd command output %v", multipathdLine)
		}
		devicePart := strings.TrimSpace(multipathdLine[1])
		slog.Debug("multipathd", "line read", line, "devicePath", devicePart)
		devices = append(devices, devicePart)
	}
	slog.Debug("info", "multipathd devices", devices)
	for i := range devices {
		rescanPath := fmt.Sprintf("/sys/block/%s/device/rescan", devices[i])
		echoCommand := fmt.Sprintf("echo 1 > %s", rescanPath)
		slog.Debug("command", "value", echoCommand)
		out, _, err := ExecCommand.Command(echoCommand, "")
		if err != nil {
			return fmt.Errorf("error writing rescan on multipath devices %s", err.Error())
		}
		slog.Debug("rescan", "output", strings.TrimSpace(out))
	}
	resizeCommand := fmt.Sprintf("multipathd resize map %s 2> /dev/null", userFriendlyName)
	out, _, err = ExecCommand.Command(resizeCommand, "")
	if err != nil {
		return fmt.Errorf("error running multipathd resize map command %s - %s", resizeCommand, err.Error())
	}
	slog.Debug("resize", "output", strings.TrimSpace(out))

	return nil
}

func ExpandFileSystem(multipathDevice string, fsType string) error {
	const defaultResizeDelay = 5
	resizeDelayForThisExecution := defaultResizeDelay
	tmp := os.Getenv(Resize2FSDelay)
	if tmp != "" {
		userSpecifiedValue, err := strconv.Atoi(tmp)
		if err != nil {
			slog.Error("conversion failed", "env var", Resize2FSDelay, "using default value instead", defaultResizeDelay)
		} else {
			resizeDelayForThisExecution = userSpecifiedValue
			slog.Warn("using non-default value", "env var", Resize2FSDelay, "user has specified", resizeDelayForThisExecution, "default", defaultResizeDelay)
		}
	}
	time.Sleep(time.Second * time.Duration(resizeDelayForThisExecution))

	command := fmt.Sprintf("resize2fs %s", multipathDevice)
	slog.Debug("ExpandFileSystem", "volume fsType", fsType)
	if fsType == common.FSTypeXFS {
		command = fmt.Sprintf("xfs_growfs %s", multipathDevice)
	}
	out, _, err := ExecCommand.Command(command, "")
	slog.Debug("command", "value", command)
	if err != nil {
		e := fmt.Errorf("ExpandFileSystem - Command %s - error: %s", command, err.Error())
		slog.Error(e.Error())
		return e
	}
	slog.Debug("command", "output", strings.TrimSpace(out))
	return nil
}
