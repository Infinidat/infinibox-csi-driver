package helper

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"time"

	"github.com/infinidat/infinibox-csi-driver/api"
	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/infinidat/infinibox-csi-driver/iboxapi"
)

// globals that are used by the background thread

var EventAPIClient api.Client
var EventIboxAPIClient iboxapi.Client

var EventCreatedVolumes int
var EventCreatedSnapshots int
var EventPublishedVolumes map[string]int
var EventNFSVersions map[string]int

var EventRandomHour int

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
func init() {
	minHour := 0
	maxHour := 23

	// Generate a random integer between min and max (inclusive)
	// EventRandomHour = 21
	EventRandomHour = rand.Intn(maxHour-minHour+1) + minHour

	EventPublishedVolumes = map[string]int{
		common.ProtocolFC:    0,
		common.ProtocolNFS:   0,
		common.ProtocolTreeq: 0,
		common.ProtocolNVME:  0,
		common.ProtocolISCSI: 0,
	}
	EventNFSVersions = map[string]int{}

	go ProcessEventCounters()
}

func ProcessEventCounters() {
	const functionName = "ProcessEventCounters"

	for {
		// testing
		// time.Sleep(time.Minute * 5)
		//
		currentTime := time.Now()  // Get the current time
		hour := currentTime.Hour() // Extract the hour component
		fmt.Printf("%s - current hour is: %d   Randomly generated hour: %d", functionName, hour, EventRandomHour)

		if hour == EventRandomHour {
			zlog.Debug().Msgf("%s - sending external events at %s", functionName, currentTime.String())

			var eventData []iboxapi.EventRequestData
			var actionData iboxapi.EventRequestData
			var eventDesc string
			var eventErr error

			if EventCreatedVolumes > 0 {
				eventData = make([]iboxapi.EventRequestData, 0)
				actionData = iboxapi.EventRequestData{
					Name:  common.CustomEventAction,
					Type:  "String",
					Value: "Created Volume",
				}
				eventData = append(eventData, actionData)

				eventDesc = fmt.Sprintf("CSI - Created Volumes: %d", EventCreatedVolumes)
				eventErr = CreateEvent(EventIboxAPIClient, eventDesc, eventData)
				if eventErr != nil {
					zlog.Error().Msgf("%s - CreateEvent - error %s", functionName, eventErr.Error())
					// only log errors if custom event fails
				} else {
					zlog.Debug().Msgf("%s - created external event %+v", functionName, eventData)
				}
			}

			if EventCreatedSnapshots > 0 {
				eventData = make([]iboxapi.EventRequestData, 0)
				actionData = iboxapi.EventRequestData{
					Name:  common.CustomEventAction,
					Type:  "String",
					Value: "Created Snapshot",
				}
				eventData = append(eventData, actionData)

				eventDesc = fmt.Sprintf("CSI - Created Snapshots: %d", EventCreatedSnapshots)
				eventErr = CreateEvent(EventIboxAPIClient, eventDesc, eventData)
				if eventErr != nil {
					zlog.Error().Msgf("%s - CreateEvent - error %s", functionName, eventErr.Error())
					// only log errors if custom event fails
				} else {
					zlog.Debug().Msgf("%s - created external event %+v", functionName, eventData)
				}
			}

			pubCount :=
				EventPublishedVolumes[common.ProtocolNFS] +
					EventPublishedVolumes[common.ProtocolTreeq] +
					EventPublishedVolumes[common.ProtocolISCSI] +
					EventPublishedVolumes[common.ProtocolFC] +
					EventPublishedVolumes[common.ProtocolNVME]

			if pubCount > 0 {
				eventData = make([]iboxapi.EventRequestData, 0)
				actionData = iboxapi.EventRequestData{
					Name:  common.CustomEventAction,
					Type:  "String",
					Value: "Published Volumes",
				}
				eventData = append(eventData, actionData)

				eventDesc = fmt.Sprintf("CSI - Published [%s,%s,%s,%s,%s] [%d,%d,%d,%d,%d] Volumes",
					common.ProtocolNFS,
					common.ProtocolTreeq,
					common.ProtocolISCSI,
					common.ProtocolFC,
					common.ProtocolNVME,
					EventPublishedVolumes[common.ProtocolNFS],
					EventPublishedVolumes[common.ProtocolTreeq],
					EventPublishedVolumes[common.ProtocolISCSI],
					EventPublishedVolumes[common.ProtocolFC],
					EventPublishedVolumes[common.ProtocolNVME])
				eventErr = CreateEvent(EventIboxAPIClient, eventDesc, eventData)
				if eventErr != nil {
					zlog.Error().Msgf("%s - CreateEvent - error %s", functionName, eventErr.Error())
					// only log errors if custom event fails
				} else {
					zlog.Debug().Msgf("%s - created external event %+v", functionName, eventData)
				}
			}

			if len(EventNFSVersions) > 0 {
				eventData = make([]iboxapi.EventRequestData, 0)
				actionData = iboxapi.EventRequestData{
					Name:  common.CustomEventAction,
					Type:  "String",
					Value: "NFS Versions",
				}
				eventData = append(eventData, actionData)

				eventDesc = fmt.Sprintf("CSI - NFS Versions [%v]", EventNFSVersions)
				eventErr = CreateEvent(EventIboxAPIClient, eventDesc, eventData)
				if eventErr != nil {
					zlog.Error().Msgf("%s - CreateEvent - error %s", functionName, eventErr.Error())
					// only log errors if custom event fails
				} else {
					zlog.Debug().Msgf("%s - created external event %+v", functionName, eventData)
				}
			}

			EventCreatedVolumes = 0
			EventCreatedSnapshots = 0
			EventPublishedVolumes[common.ProtocolNFS] = 0
			EventPublishedVolumes[common.ProtocolTreeq] = 0
			EventPublishedVolumes[common.ProtocolISCSI] = 0
			EventPublishedVolumes[common.ProtocolFC] = 0
			EventPublishedVolumes[common.ProtocolNVME] = 0
			EventNFSVersions = map[string]int{}
		}

		currentTime = time.Now() // Get the current time
		zlog.Debug().Msgf("%s - sleeping at %s for 1 hour and 1 second", functionName, currentTime.String())
		time.Sleep(time.Second * 1)
		time.Sleep(time.Hour * 1)
	}
}

func CreateEvent(iboxAPI iboxapi.Client, desc string, eventData []iboxapi.EventRequestData) error {
	zlog.Debug().Msgf("CreateEvent: %s", desc)

	// verify creating events is enabled
	createEvent := true
	tmp := os.Getenv(common.EnvVarCreateEvents)
	if tmp != "" {
		boolValue, err := strconv.ParseBool(tmp)
		if err != nil {
			zlog.Error().Msgf("%s env var is not a valid boolean value, [%s] was entered", common.EnvVarCreateEvents, tmp)
			return err
		}
		createEvent = boolValue
	}
	if !createEvent {
		return nil
	}

	version := os.Getenv(common.EnvVarCSIDriverVersion)
	osVersion := os.Getenv(common.EnvVarOSVersion)
	kubeVersion := os.Getenv(common.EnvVarKubeVersion)
	kubeNodeCount := os.Getenv(common.EnvVarNodeCount)
	data := make([]iboxapi.EventRequestData, 0)

	data = append(data, eventData...)

	versionData := iboxapi.EventRequestData{
		Name:  "csi_driver_version",
		Type:  "String",
		Value: version,
	}
	data = append(data, versionData)

	osVersionData := iboxapi.EventRequestData{
		Name:  "os_version",
		Type:  "String",
		Value: osVersion,
	}
	data = append(data, osVersionData)

	kubeVersionData := iboxapi.EventRequestData{
		Name:  "kube_version",
		Type:  "String",
		Value: kubeVersion,
	}
	data = append(data, kubeVersionData)

	kubeNodeNameData := iboxapi.EventRequestData{
		Name:  "kube_node_name",
		Type:  "String",
		Value: os.Getenv("KUBE_NODE_NAME"),
	}
	data = append(data, kubeNodeNameData)

	kubeNodeCountData := iboxapi.EventRequestData{
		Name:  "kube_node_count",
		Type:  "String",
		Value: kubeNodeCount,
	}
	data = append(data, kubeNodeCountData)

	descData := iboxapi.EventRequestData{
		Name:  "event_desc",
		Type:  "String",
		Value: desc,
	}
	data = append(data, descData)

	systemDetails, err := iboxAPI.GetSystem(context.Background())
	if err != nil {
		zlog.Error().Msg(err.Error())
	} else {
		serialNumberData := iboxapi.EventRequestData{
			Name:  "serial_number",
			Type:  "String",
			Value: strconv.Itoa(systemDetails.SerialNumber),
		}
		data = append(data, serialNumberData)
	}

	r := iboxapi.EventRequest{
		Code: "ECOSYSTEM_TOOLS_HEARTBEAT",
		Data: data,
	}

	err = iboxAPI.CreateEvent(context.Background(), r)
	return err
}
