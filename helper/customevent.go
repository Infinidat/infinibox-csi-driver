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

package helper

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/infinidat/infinibox-csi-driver/api"
	"github.com/infinidat/infinibox-csi-driver/api/clientgo"
	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/infinidat/infinibox-csi-driver/iboxapi"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// globals that are used by the background thread
var EventCreatedVolumes int
var EventCreatedSnapshots int
var EventPublishedVolumes map[string]int
var EventNFSVersions map[string]int

type PoolData struct {
	PercentUsed  int64
	PercentAvail int64
}

const HOURS_BEFORE_EVENT_CREATION = 24

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
	EventPublishedVolumes = map[string]int{
		common.ProtocolFC:    0,
		common.ProtocolNFS:   0,
		common.ProtocolTreeq: 0,
		common.ProtocolNVME:  0,
		common.ProtocolISCSI: 0,
	}
	EventNFSVersions = map[string]int{}

	go UpdatePoolMetrics()
	go ProcessEventCounters()
}

func ProcessEventCounters() {
	ticker := time.NewTicker(HOURS_BEFORE_EVENT_CREATION * time.Hour)
	defer ticker.Stop()

	//uncomment to test
	//processEventCountersImpl()

	for range ticker.C {
		processEventCountersImpl()
	}
}
func processEventCountersImpl() {
	// for testing
	//time.Sleep(time.Minute * 4)

	//
	slog.Debug("sending external events", "at", time.Now().String())

	var eventData []iboxapi.EventRequestData
	var actionData iboxapi.EventRequestData
	var eventDesc string
	var eventErr error

	primaryCredNamespace := os.Getenv(common.EnvVarPodNamespace)
	primaryCredName := os.Getenv(common.EnvVarPrimaryIboxCredential)
	if primaryCredName == "" {
		slog.Error("required setting is not set, this is required for events to be created", "env var", common.EnvVarPrimaryIboxCredential)
		return
	}

	kubeClient, err := clientgo.BuildClient()
	if err != nil {
		slog.Error("error UpdatePoolMetrics getting kube client", "error", err.Error())
		return
	}
	ctx := context.Background()
	secretMap, err := kubeClient.GetSecret(ctx, primaryCredName, primaryCredNamespace)
	if err != nil {
		slog.Error("error UpdatePoolMetrics get secret", "error", err.Error())
		return
	}

	clientService := api.ClientService{
		ConfigMap:  make(map[string]string),
		SecretsMap: secretMap,
	}

	cs, err := clientService.NewClient()
	if err != nil {
		slog.Error("error UpdatePoolMetrics getting ibox client", "error", err.Error())
		return
	}

	if EventCreatedVolumes > 0 {
		eventData = make([]iboxapi.EventRequestData, 0)
		actionData = iboxapi.EventRequestData{
			Name:  common.CustomEventAction,
			Type:  "String",
			Value: "Created Volume",
		}
		eventData = append(eventData, actionData)

		eventDesc = fmt.Sprintf("CSI - Created Volumes: %d", EventCreatedVolumes)
		eventErr = CreateEvent(cs.IboxAPI, eventDesc, eventData)
		if eventErr != nil {
			slog.Error("CreateEvent - error", "error", eventErr.Error())
			// only log errors if custom event fails
		} else {
			slog.Debug("created external event", "data", eventData)
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
		eventErr = CreateEvent(cs.IboxAPI, eventDesc, eventData)
		if eventErr != nil {
			slog.Error("CreateEvent - error", "error", eventErr.Error())
			// only log errors if custom event fails
		} else {
			slog.Debug("created external event", "data", eventData)
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
		eventErr = CreateEvent(cs.IboxAPI, eventDesc, eventData)
		if eventErr != nil {
			slog.Error("CreateEvent - error", "error", eventErr.Error())
			// only log errors if custom event fails
		} else {
			slog.Debug("created external event", "data", eventData)
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
		eventErr = CreateEvent(cs.IboxAPI, eventDesc, eventData)
		if eventErr != nil {
			slog.Error("CreateEvent - error", "error", eventErr.Error())
			// only log errors if custom event fails
		} else {
			slog.Debug("created external event", "data", eventData)
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

func CreateEvent(iboxAPI iboxapi.Client, desc string, eventData []iboxapi.EventRequestData) error {
	slog.Debug("CreateEvent", "desc", desc)

	// verify creating events is enabled
	createEvent := true
	tmp := os.Getenv(common.EnvVarCreateEvents)
	if tmp != "" {
		boolValue, err := strconv.ParseBool(tmp)
		if err != nil {
			slog.Error("env var is not a valid boolean value, was entered", "env var", common.EnvVarCreateEvents, "entered", tmp)
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

	data = append(data, iboxapi.EventRequestData{Name: "csi_driver_version", Type: "String", Value: version})

	data = append(data, iboxapi.EventRequestData{Name: "os_version", Type: "String", Value: osVersion})

	data = append(data, iboxapi.EventRequestData{Name: "kube_version", Type: "String", Value: kubeVersion})

	data = append(data, iboxapi.EventRequestData{Name: "kube_node_name", Type: "String", Value: os.Getenv("KUBE_NODE_NAME")})

	data = append(data, iboxapi.EventRequestData{Name: "kube_node_count", Type: "String", Value: kubeNodeCount})

	data = append(data, iboxapi.EventRequestData{Name: "event_desc", Type: "String", Value: desc})

	systemDetails, err := iboxAPI.GetSystem(context.Background())
	if err != nil {
		slog.Error(err.Error())
	} else {
		data = append(data, iboxapi.EventRequestData{Name: "serial_number", Type: "String", Value: strconv.Itoa(systemDetails.SerialNumber)})
	}

	r := iboxapi.EventRequest{Code: "ECOSYSTEM_TOOLS_HEARTBEAT", Data: data}

	err = iboxAPI.CreateEvent(context.Background(), r)
	return err
}

func UpdatePoolMetrics() {
	ticker := time.NewTicker(HOURS_BEFORE_EVENT_CREATION * time.Hour)
	defer ticker.Stop()
	updatePoolMetricsImpl()

	for range ticker.C {
		updatePoolMetricsImpl()
	}
}

func updatePoolMetricsImpl() {

	slog.Debug("sending external events (pool metrics)", "at", time.Now().String())

	EventPoolUsage := map[string]PoolData{}
	kubeClient, err := clientgo.BuildClient()
	if err != nil {
		slog.Error("error UpdatePoolMetrics getting kube client", "error", err.Error())
		return
	}
	ctx := context.Background()
	// get all the pools used by all the PVs on this cluster
	pvList, err := kubeClient.KubeClientInterface.CoreV1().PersistentVolumes().List(ctx, v1.ListOptions{})
	if err != nil {
		slog.Error("error UpdatePoolMetrics getting pv list", "error", err.Error())
		return
	}

	for _, pv := range pvList.Items {
		poolName := pv.Spec.CSI.VolumeAttributes[common.StorageClassPoolName]
		if poolName != "" {
			EventPoolUsage[poolName] = PoolData{}
		}
	}

	primaryCredNamespace := os.Getenv(common.EnvVarPodNamespace)
	primaryCredName := os.Getenv(common.EnvVarPrimaryIboxCredential)
	if primaryCredName == "" {
		slog.Error("required setting not set, this is required for events to be created", "env var", common.EnvVarPrimaryIboxCredential)
		return
	}
	secretMap, err := kubeClient.GetSecret(ctx, primaryCredName, primaryCredNamespace)
	if err != nil {
		slog.Error("error UpdatePoolMetrics get secret", "error", err.Error())
		return
	}

	clientService := api.ClientService{
		ConfigMap:  make(map[string]string),
		SecretsMap: secretMap,
	}

	cs, err := clientService.NewClient()
	if err != nil {
		slog.Error("error UpdatePoolMetrics getting ibox client", "error", err.Error())
		return
	}

	for poolName := range EventPoolUsage {

		pool, err := cs.IboxAPI.GetPoolByName(ctx, poolName)
		if err != nil {
			slog.Error("error UpdatePoolMetrics", "error", err.Error())
			return
		}
		// metric 1 : (total_disk_usage * 100) / physical_capacity =  percentage used
		pctUsed := (pool.TotalDiskUsage * 100) / pool.PhysicalCapacity
		// metric 2 : 100 - percentage_used = percentage_available
		pctAvail := 100 - pctUsed
		slog.Debug("pool metrics", "pctUsed", pctUsed, "pctAvail", pctAvail)

		eventData := make([]iboxapi.EventRequestData, 0)
		actionData := iboxapi.EventRequestData{
			Name:  common.CustomEventAction,
			Type:  "String",
			Value: "Pool Usage",
		}
		eventData = append(eventData, actionData)

		eventDesc := fmt.Sprintf("Pool [%s] Used [%d] Avail [%d]", poolName, pctUsed, pctAvail)
		eventErr := CreateEvent(cs.IboxAPI, eventDesc, eventData)
		if eventErr != nil {
			slog.Error("CreateEvent - error", "error", eventErr.Error())
			// only log errors if custom event fails
		} else {
			slog.Debug("created external event", "data", eventData)
		}
	}
}
