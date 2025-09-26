package helper

import (
	"fmt"
	"infinibox-csi-driver/api"
	"infinibox-csi-driver/common"
	"infinibox-csi-driver/iboxapi"
	"math/rand"
	"os"
	"strconv"
	"time"
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
	min := 0
	max := 23

	// Generate a random integer between min and max (inclusive)
	//EventRandomHour = 21
	EventRandomHour = rand.Intn(max-min+1) + min

	EventPublishedVolumes = map[string]int{
		common.PROTOCOL_FC:    0,
		common.PROTOCOL_NFS:   0,
		common.PROTOCOL_TREEQ: 0,
		common.PROTOCOL_NVME:  0,
		common.PROTOCOL_ISCSI: 0,
	}
	EventNFSVersions = map[string]int{}

	go ProcessEventCounters()
}

func ProcessEventCounters() {

	const FN = "ProcessEventCounters"

	for {

		// testing
		//time.Sleep(time.Minute * 5)
		//

		currentTime := time.Now()  // Get the current time
		hour := currentTime.Hour() // Extract the hour component
		fmt.Printf("%s - current hour is: %d   Randomly generated hour: %d", FN, hour, EventRandomHour)

		if hour == EventRandomHour {
			zlog.Debug().Msgf("%s - sending external events at %s", FN, currentTime.String())

			var eventData []iboxapi.EventRequestData
			var actionData iboxapi.EventRequestData
			var eventDesc string
			var eventErr error

			if EventCreatedVolumes > 0 {
				eventData = make([]iboxapi.EventRequestData, 0)
				actionData = iboxapi.EventRequestData{
					Name:  common.CUSTOM_EVENT_ACTION,
					Type:  "String",
					Value: "Created Volume",
				}
				eventData = append(eventData, actionData)

				eventDesc = fmt.Sprintf("CSI - Created Volumes: %d", EventCreatedVolumes)
				eventErr = CreateEvent(EventAPIClient, EventIboxAPIClient, eventDesc, eventData)
				if eventErr != nil {
					zlog.Error().Msgf("%s - CreateEvent - error %s", FN, eventErr.Error())
					// only log errors if custom event fails
				} else {
					zlog.Debug().Msgf("%s - created external event %+v", FN, eventData)
				}
			}

			if EventCreatedSnapshots > 0 {
				eventData = make([]iboxapi.EventRequestData, 0)
				actionData = iboxapi.EventRequestData{
					Name:  common.CUSTOM_EVENT_ACTION,
					Type:  "String",
					Value: "Created Snapshot",
				}
				eventData = append(eventData, actionData)

				eventDesc = fmt.Sprintf("CSI - Created Snapshots: %d", EventCreatedSnapshots)
				eventErr = CreateEvent(EventAPIClient, EventIboxAPIClient, eventDesc, eventData)
				if eventErr != nil {
					zlog.Error().Msgf("%s - CreateEvent - error %s", FN, eventErr.Error())
					// only log errors if custom event fails
				} else {
					zlog.Debug().Msgf("%s - created external event %+v", FN, eventData)
				}
			}

			pubCount :=
				EventPublishedVolumes[common.PROTOCOL_NFS] +
					EventPublishedVolumes[common.PROTOCOL_TREEQ] +
					EventPublishedVolumes[common.PROTOCOL_ISCSI] +
					EventPublishedVolumes[common.PROTOCOL_FC] +
					EventPublishedVolumes[common.PROTOCOL_NVME]

			if pubCount > 0 {
				eventData = make([]iboxapi.EventRequestData, 0)
				actionData = iboxapi.EventRequestData{
					Name:  common.CUSTOM_EVENT_ACTION,
					Type:  "String",
					Value: "Published Volumes",
				}
				eventData = append(eventData, actionData)

				eventDesc = fmt.Sprintf("CSI - Published [%s,%s,%s,%s,%s] [%d,%d,%d,%d,%d] Volumes",
					common.PROTOCOL_NFS,
					common.PROTOCOL_TREEQ,
					common.PROTOCOL_ISCSI,
					common.PROTOCOL_FC,
					common.PROTOCOL_NVME,
					EventPublishedVolumes[common.PROTOCOL_NFS],
					EventPublishedVolumes[common.PROTOCOL_TREEQ],
					EventPublishedVolumes[common.PROTOCOL_ISCSI],
					EventPublishedVolumes[common.PROTOCOL_FC],
					EventPublishedVolumes[common.PROTOCOL_NVME])
				eventErr = CreateEvent(EventAPIClient, EventIboxAPIClient, eventDesc, eventData)
				if eventErr != nil {
					zlog.Error().Msgf("%s - CreateEvent - error %s", FN, eventErr.Error())
					// only log errors if custom event fails
				} else {
					zlog.Debug().Msgf("%s - created external event %+v", FN, eventData)
				}
			}

			if len(EventNFSVersions) > 0 {
				eventData = make([]iboxapi.EventRequestData, 0)
				actionData = iboxapi.EventRequestData{
					Name:  common.CUSTOM_EVENT_ACTION,
					Type:  "String",
					Value: "NFS Versions",
				}
				eventData = append(eventData, actionData)

				eventDesc = fmt.Sprintf("CSI - NFS Versions [%v]", EventNFSVersions)
				eventErr = CreateEvent(EventAPIClient, EventIboxAPIClient, eventDesc, eventData)
				if eventErr != nil {
					zlog.Error().Msgf("%s - CreateEvent - error %s", FN, eventErr.Error())
					// only log errors if custom event fails
				} else {
					zlog.Debug().Msgf("%s - created external event %+v", FN, eventData)
				}
			}

			EventCreatedVolumes = 0
			EventCreatedSnapshots = 0
			EventPublishedVolumes[common.PROTOCOL_NFS] = 0
			EventPublishedVolumes[common.PROTOCOL_TREEQ] = 0
			EventPublishedVolumes[common.PROTOCOL_ISCSI] = 0
			EventPublishedVolumes[common.PROTOCOL_FC] = 0
			EventPublishedVolumes[common.PROTOCOL_NVME] = 0
			EventNFSVersions = map[string]int{}
		}

		currentTime = time.Now() // Get the current time
		zlog.Debug().Msgf("%s - sleeping at %s for 1 hour and 1 second", FN, currentTime.String())
		time.Sleep(time.Second * 1)
		time.Sleep(time.Hour * 1)

	}
}

func CreateEvent(cl api.Client, iboxApi iboxapi.Client, desc string, eventData []iboxapi.EventRequestData) error {

	zlog.Debug().Msgf("CreateEvent: %s", desc)

	//verify creating events is enabled
	createEvent := true
	tmp := os.Getenv(common.ENV_VAR_CREATE_EVENTS)
	if tmp != "" {
		boolValue, err := strconv.ParseBool(tmp)
		if err != nil {
			zlog.Error().Msgf("%s env var is not a valid boolean value, [%s] was entered", common.ENV_VAR_CREATE_EVENTS, tmp)
			return err
		}
		createEvent = boolValue
	}
	if !createEvent {
		return nil
	}

	version := os.Getenv(common.ENV_VAR_CSI_DRIVER_VERSION)
	osVersion := os.Getenv(common.ENV_VAR_OS_VERSION)
	kubeVersion := os.Getenv(common.ENV_VAR_KUBE_VERSION)
	kubeNodeCount := os.Getenv(common.ENV_VAR_NODE_COUNT)
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

	systemDetails, err := iboxApi.GetSystem()
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

	err = iboxApi.CreateEvent(r)
	return err
}
