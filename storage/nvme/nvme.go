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
package nvme

import (
	"encoding/json"
	"fmt"
	"infinibox-csi-driver/log"
	storagecommon "infinibox-csi-driver/storage/common"

	"os"
	"strings"

	"github.com/blang/semver/v4"
)

var zlog = log.Get() // grab the logger for package use

const NVME_VERSION_260 = "2.6.0"
const NVME_VERSION_211 = "2.11.0"

type NVME211 struct {
	Devices []Devices211 `json:"Devices"`
}

type Paths211 struct {
	Path     string `json:"Path"`
	ANAState string `json:"ANAState"`
}
type Controllers211 struct {
	Controller   string     `json:"Controller"`
	Cntlid       string     `json:"Cntlid"`
	SerialNumber string     `json:"SerialNumber"`
	ModelNumber  string     `json:"ModelNumber"`
	Firmware     string     `json:"Firmware"`
	Transport    string     `json:"Transport"`
	Address      string     `json:"Address"`
	Slot         string     `json:"Slot"`
	Namespaces   []any      `json:"Namespaces"`
	Paths        []Paths211 `json:"Paths"`
}
type Namespaces211 struct {
	NameSpace    string `json:"NameSpace"`
	Generic      string `json:"Generic"`
	Nsid         int    `json:"NSID"`
	UsedBytes    int    `json:"UsedBytes"`
	MaximumLBA   int    `json:"MaximumLBA"`
	PhysicalSize int    `json:"PhysicalSize"`
	SectorSize   int    `json:"SectorSize"`
}
type Subsystems211 struct {
	Subsystem    string           `json:"Subsystem"`
	SubsystemNQN string           `json:"SubsystemNQN"`
	Controllers  []Controllers211 `json:"Controllers"`
	Namespaces   []Namespaces211  `json:"Namespaces"`
}
type Devices211 struct {
	HostNQN    string          `json:"HostNQN"`
	HostID     string          `json:"HostID"`
	Subsystems []Subsystems211 `json:"Subsystems"`
}

// the Device struct is the default NVME device representation and
// also the default output for nvme 2.6, later versions of nvme are
// different format, so we convert that newer information into the
// older style of Device struct
//
// for NVMEDevices, note that UsedBytes, MaximumLBA, and PhysicalSize are different types (either int/int64 or string) depending
// on the version of nvme used, that is why they specify 'any' as the type.
// Ubuntu and RHEL return int/int64 for those whereas Suse returns strings.
// Currently these fields are unused so there is no need to check the type that was set in the JSON.
type Device struct {
	NameSpace    int    `json:"NameSpace"`
	DevicePath   string `json:"DevicePath"`
	Firmware     string `json:"Firmware"`
	Index        int    `json:"Index"`
	ModelNumber  string `json:"ModelNumber"`
	SerialNumber string `json:"SerialNumber"`
	UsedBytes    any    `json:"UsedBytes"`
	MaximumLBA   any    `json:"MaximumLBA"`
	PhysicalSize any    `json:"PhysicalSize"`
	SectorSize   int    `json:"SectorSize"`
}

type NVMEDevices struct {
	Devices []Device `json:"Devices"`
}

const NVME_DISCOVERY_PORT = 8009

func getHostNQN() (string, error) {

	fileContent, err := os.ReadFile("/host/etc/nvme/hostnqn")
	if err != nil {
		zlog.Error().Msgf("getHostNQN (nvme) - failed to read hostnqn file %s", err.Error())
		return "", err
	}
	hostnqn := string(fileContent)
	hostnqn = strings.TrimSuffix(hostnqn, "\n")
	zlog.Debug().Msgf("getHostNQN (nvme) - host nqn %s ", hostnqn)
	return hostnqn, nil
}

func getNVMENamespaces() (devices NVMEDevices, err error) {
	cmd := "nvme list -o json"
	rawOutput, _, err := storagecommon.ExecCommand.Command(cmd, "")
	if err != nil {
		zlog.Error().Msgf("getNVMENamespaces (nvme) - %s failed, err: %v, %s", cmd, err, rawOutput)
		return devices, err
	}
	zlog.Trace().Msgf("getNVMENamespaces (nvme) - %s raw output %s", cmd, rawOutput)

	version, err := getNVMEVersion()
	if err != nil {
		zlog.Error().Msgf("getNVMENamespaces (nvme) - error unmarshalling %s output - error %s", cmd, err.Error())
		return devices, err
	}

	if version == NVME_VERSION_260 {
		err = json.Unmarshal([]byte(rawOutput), &devices)
		if err != nil {
			zlog.Error().Msgf("getNVMENamespaces (nvme) - error unmarshalling %s output - error %s", cmd, err.Error())
			return devices, err
		}
	} else {
		var nvme211Output NVME211
		err = json.Unmarshal([]byte(rawOutput), &nvme211Output)
		if err != nil {
			zlog.Error().Msgf("getNVMENamespaces (nvme) - error unmarshalling 211 %s output - error %s", cmd, err.Error())
			return devices, err
		}
		devices = parseNVME211Devices(nvme211Output)

	}
	return devices, nil
}

// nvme connect-all -t tcp -a 172.20.51.170
func nvmeConnectAll(ipAddress string) (err error) {
	cmd := fmt.Sprintf("nvme connect-all -t tcp -a %s", ipAddress)
	rawOutput, _, err := storagecommon.ExecCommand.Command(cmd, "")
	if err != nil {
		zlog.Error().Msgf("nvmeConnectAll (nvme) - %s failed, ip: %s err: %v, %s", cmd, ipAddress, err, rawOutput)
		return err
	}
	zlog.Debug().Msgf("nvmeConnectAll (nvme) - %s raw output %s", cmd, rawOutput)

	return nil
}

// nvme discover -t tcp -a 172.20.51.170 -s 8009
func nvmeDiscover(ipAddress string) (err error) {
	cmd := fmt.Sprintf("nvme discover -t tcp -a %s -s %d", ipAddress, NVME_DISCOVERY_PORT)
	rawOutput, _, err := storagecommon.ExecCommand.Command(cmd, "")
	if err != nil {
		zlog.Error().Msgf("nvmeDiscover (nvme) - %s failed, err: %v, %s", cmd, err, rawOutput)
		return err
	}
	zlog.Trace().Msgf("nvmeDiscover (nvme) - %s - raw output %s", cmd, rawOutput)

	return nil
}

/**
// currently no good way to know when to disconnect and really
// no good reason to disconnect on real systems with real workloads
func disconnectNVMEConnections() error {
	cmd := "nvme disconnect-all"
	rawOutput, _, err := execCommand.Command(cmd, "")
	if err != nil {
		zlog.Error().Msgf("disconnectNVMEConnections (nvme) - %s failed, err: %v, %s", cmd, err, rawOutput)
		return err
	}
	zlog.Debug().Msg(cmd)
	return nil
}
*/

/**
// not used for now, but useful for debugging
func getConnectionDetails() (results string, err error) {
	cmd := "nvme list-subsys"
	results, err = execScsi.Command(cmd, "")
	if err != nil {
		zlog.Error().Msgf("%s failed, err: %v", cmd, err)
		return results, err
	}
	return results, nil
}
*/

// parse the NVME version using semver formatting and comparison
func getNVMEVersion() (version string, err error) {
	cmd := "nvme version"
	rawOutput, _, err := storagecommon.ExecCommand.Command(cmd, "")
	if err != nil {
		zlog.Error().Msgf("getNVMEVersion - %s failed, err: %v, %s", cmd, err, rawOutput)
		return "", err
	}
	zlog.Trace().Msgf("getNVMEVersion - %s raw output %s", cmd, rawOutput)

	parts := strings.Split(rawOutput, " ")
	if len(parts) < 3 {
		e := fmt.Errorf("getNVMEVersion - parsing rawOutput %s failed not enough parts %d", rawOutput, len(parts))
		zlog.Error().Msgf("%s", e.Error())
		return "", e

	}
	versionParts := parts[2]
	majorMinorPatch := strings.Split(versionParts, ".")
	zlog.Debug().Msgf("versionParts %s major.minor.patch %s len %d", versionParts, majorMinorPatch, len(majorMinorPatch))
	if len(majorMinorPatch) < 3 {
		versionParts = versionParts + ".0" // add a patch number to make it semver
	}

	// convert the nvme version into a semver representation so we can compare
	var v1, v2 semver.Version
	v1, err = semver.Make(versionParts)
	if err != nil {
		zlog.Error().Msgf("error converting %s to semver %s", versionParts, err.Error())
		return "", err
	}
	v2, err = semver.Make(NVME_VERSION_211)
	if err != nil {
		zlog.Error().Msgf("error converting %s to semver %s", NVME_VERSION_211, err.Error())
	}
	value := v1.Compare(v2)
	if value < 0 {
		// if parsed version is less than 2.11, assume it will parse into the default (2.6) structure
		zlog.Debug().Msgf("nvme version %s is less than %s", versionParts, NVME_VERSION_211)
		return NVME_VERSION_260, nil
	}
	zlog.Debug().Msgf("nvme version %s is greater than or equal to %s", versionParts, NVME_VERSION_211)
	return NVME_VERSION_211, nil
}

// parse out the standard NVME device information from nvme 2.11 output
func parseNVME211Devices(nvme211Output NVME211) (devices NVMEDevices) {
	devices.Devices = make([]Device, 0)
	for _, device211 := range nvme211Output.Devices {
		subsystems := device211.Subsystems

		for _, subsystem := range subsystems {
			namespaces := subsystem.Namespaces
			for _, namespace := range namespaces {
				// append /dev to the device path if necessary to be
				// consistent with all versions of nvme
				devicePath := namespace.NameSpace
				if !strings.Contains(namespace.NameSpace, "/dev") {
					devicePath = "/dev/" + devicePath
				}
				device := Device{
					NameSpace:  namespace.Nsid,
					DevicePath: devicePath,
					//UsedBytes:    namespace.UsedBytes,
					//PhysicalSize: namespace.PhysicalSize,
					//MaximumLBA:   namespace.MaximumLBA,
					//SectorSize:   namespace.SectorSize,
				}

				devices.Devices = append(devices.Devices, device)

			}
		}
	}
	return devices
}
