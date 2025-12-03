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
	"context"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/infinidat/infinibox-csi-driver/common"
	storagecommon "github.com/infinidat/infinibox-csi-driver/storage/common"

	"os"
	"strings"
)

const (
	NVMEVersion260    = "2.6.0"
	NVMEVersion211    = "2.11.0"
	NVMEDiscoveryPort = 8009
)

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

type Devices struct {
	Devices []Device `json:"Devices"`
}

type NVMEDeviceInfo struct {
	Node      string
	Namespace int
}

func getHostNQN() (string, error) {
	fileContent, err := os.ReadFile("/host/etc/nvme/hostnqn")
	if err != nil {
		slog.Error("getHostNQN (nvme) - failed to read hostnqn file", "error", err.Error())
		return "", err
	}
	hostnqn := string(fileContent)
	hostnqn = strings.TrimSuffix(hostnqn, "\n")
	slog.Debug("getHostNQN (nvme)", "host nqn", hostnqn)
	return hostnqn, nil
}

func getNVMENamespacesByNormalOutput() (devices []NVMEDeviceInfo, err error) {
	cmd := "nvme list"
	rawOutput, _, err := storagecommon.ExecCommand.Command(cmd, "")
	if err != nil {
		slog.Error("getNVMENamespacesByNormalOutput (nvme) failed", "command", cmd, "error", err, "output", rawOutput)
		return devices, err
	}
	slog.Log(context.Background(), common.LevelTrace, "getNVMENamespacesByNormalOutput (nvme)", "command", cmd, "raw output", rawOutput)

	lines := strings.Split(rawOutput, "\n")
	slog.Debug("nvmeOutput", "lines", len(lines), "line contents", lines)
	for lineNo, line := range lines {
		slog.Debug("line", "number", lineNo, "length", len(line), "value", line)
	}

	//remove the header which is 2 lines
	slog.Debug("nvmeOutput with lines removed...")
	lines = append(lines[:0], lines[2:]...)
	for lineNo, line := range lines {
		slog.Debug("line", "number", lineNo, "len", len(line), "value", line)
		if len(line) > 0 {
			fields := strings.Fields(line)
			slog.Debug("parsed", "fields", fields)
			// Remove the "0x" prefix if present
			namespaceField := fields[4]
			if len(namespaceField) > 2 && namespaceField[0:2] == "0x" {
				namespaceField = namespaceField[2:]
			}
			namespaceNumber, err := strconv.ParseInt(namespaceField, 16, 0)
			if err != nil {
				slog.Debug("error converting namespace to int", "field", namespaceField, "error", err.Error())
				return devices, err
			}
			element := NVMEDeviceInfo{
				Node: fields[0],
				//Generic:   fields[1],
				//SN:        fields[2],
				//Model:     fields[3],
				Namespace: int(namespaceNumber),
				//Unused:    "unused",
			}
			slog.Debug("parsed", "element", element)
			devices = append(devices, element)
		}
	}

	return devices, nil
}

/**
func getNVMENamespaces() (devices Devices, err error) {
	cmd := "nvme list -o json"
	rawOutput, _, err := storagecommon.ExecCommand.Command(cmd, "")
	if err != nil {
		slog.Error("getNVMENamespaces (nvme) - %s failed, err: %v, %s", cmd, err, rawOutput)
		return devices, err
	}
	slog.Log(ctx,("getNVMENamespaces (nvme) - %s raw output %s", cmd, rawOutput)

	version, err := getNVMEVersion()
	if err != nil {
		slog.Error("getNVMENamespaces (nvme) - error unmarshalling %s output - error %s", cmd, err.Error())
		return devices, err
	}

	if version == NVMEVersion260 {
		err = json.Unmarshal([]byte(rawOutput), &devices)
		if err != nil {
			slog.Error("getNVMENamespaces (nvme) - error unmarshalling %s output - error %s", cmd, err.Error())
			return devices, err
		}
	} else {
		var nvme211Output NVME211
		err = json.Unmarshal([]byte(rawOutput), &nvme211Output)
		if err != nil {
			slog.Error("getNVMENamespaces (nvme) - error unmarshalling 211 %s output - error %s", cmd, err.Error())
			return devices, err
		}
		devices = parseNVME211Devices(nvme211Output)
	}
	return devices, nil
}a
*/

// nvme connect-all -t tcp -a 172.20.51.170
func nvmeConnectAll(ipAddress string) (err error) {
	cmd := fmt.Sprintf("nvme connect-all -t tcp -a %s", ipAddress)
	rawOutput, _, err := storagecommon.ExecCommand.Command(cmd, "")
	if err != nil {
		slog.Error("nvmeConnectAll (nvme) - failed", "command", cmd, "ipaddress", ipAddress, "output", rawOutput, "error", err)
		return err
	}
	slog.Debug("nvmeConnectAll (nvme)", "command", cmd, "output", rawOutput)

	return nil
}

// nvme discover -t tcp -a 172.20.51.170 -s 8009
func nvmeDiscover(ipAddress string) (err error) {
	cmd := fmt.Sprintf("nvme discover -t tcp -a %s -s %d", ipAddress, NVMEDiscoveryPort)
	rawOutput, _, err := storagecommon.ExecCommand.Command(cmd, "")
	if err != nil {
		slog.Error("nvmeDiscover (nvme) failed", "command", cmd, "error", err, "output", rawOutput)
		return err
	}
	slog.Log(context.Background(), common.LevelTrace, "nvmeDiscover (nvme)", "command", cmd, "raw output", rawOutput)

	return nil
}
