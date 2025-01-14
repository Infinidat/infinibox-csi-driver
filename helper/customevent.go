package helper

import (
	"infinibox-csi-driver/api"
	"infinibox-csi-driver/common"
	"os"
	"strconv"
)

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

func CreateCustomEvent(cl api.Client, desc string, eventData []api.CustomEventRequestData) error {

	zlog.Debug().Msgf("CreateCustomEvent: %s", desc)

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
	data := make([]api.CustomEventRequestData, 0)

	data = append(data, eventData...)

	versionData := api.CustomEventRequestData{
		Name:  "csi_driver_version",
		Type:  "String",
		Value: version,
	}
	data = append(data, versionData)

	osVersionData := api.CustomEventRequestData{
		Name:  "os_version",
		Type:  "String",
		Value: osVersion,
	}
	data = append(data, osVersionData)

	kubeVersionData := api.CustomEventRequestData{
		Name:  "kube_version",
		Type:  "String",
		Value: kubeVersion,
	}
	data = append(data, kubeVersionData)

	kubeNodeNameData := api.CustomEventRequestData{
		Name:  "kube_node_name",
		Type:  "String",
		Value: os.Getenv("KUBE_NODE_NAME"),
	}
	data = append(data, kubeNodeNameData)

	kubeNodeCountData := api.CustomEventRequestData{
		Name:  "kube_node_count",
		Type:  "String",
		Value: kubeNodeCount,
	}
	data = append(data, kubeNodeCountData)

	r := api.CustomEventRequest{
		DescriptionTemplate: desc,
		Data:                data,
		Visibility:          "CUSTOMER",
		Level:               "INFO",
	}

	err := cl.CreateCustomEvent(r)
	return err
}

func CreateEvent(cl api.Client, desc string, eventData []api.EventRequestData) error {

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
	data := make([]api.EventRequestData, 0)

	data = append(data, eventData...)

	versionData := api.EventRequestData{
		Name:  "csi_driver_version",
		Type:  "String",
		Value: version,
	}
	data = append(data, versionData)

	osVersionData := api.EventRequestData{
		Name:  "os_version",
		Type:  "String",
		Value: osVersion,
	}
	data = append(data, osVersionData)

	kubeVersionData := api.EventRequestData{
		Name:  "kube_version",
		Type:  "String",
		Value: kubeVersion,
	}
	data = append(data, kubeVersionData)

	kubeNodeNameData := api.EventRequestData{
		Name:  "kube_node_name",
		Type:  "String",
		Value: os.Getenv("KUBE_NODE_NAME"),
	}
	data = append(data, kubeNodeNameData)

	kubeNodeCountData := api.EventRequestData{
		Name:  "kube_node_count",
		Type:  "String",
		Value: kubeNodeCount,
	}
	data = append(data, kubeNodeCountData)

	descData := api.EventRequestData{
		Name:  "event_desc",
		Type:  "String",
		Value: desc,
	}
	data = append(data, descData)

	r := api.EventRequest{
		Code: "ECOSYSTEM_TOOLS_HEARTBEAT",
		Data: data,
	}

	err := cl.CreateEvent(r)
	return err
}
