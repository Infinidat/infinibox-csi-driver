package metric

import (
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"k8s.io/klog/v2"
)

var (
	MetricIboxBBUChargeLevelGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: METRIC_IBOX_BBU_CHARGE_LEVEL,
		Help: "The ibox BBU charge level",
	}, []string{METRIC_IBOX_NAME, METRIC_IBOX_IP, METRIC_IBOX_HOSTNAME, METRIC_IBOX_NODE_NAME})
	MetricIboxNodeBBUProtectionGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: METRIC_IBOX_NODE_BBU_PROTECTION,
		Help: "The ibox node BBU protection",
	}, []string{METRIC_IBOX_NAME, METRIC_IBOX_IP, METRIC_IBOX_HOSTNAME, METRIC_IBOX_NODE_NAME})
	MetricIboxErrorRatesGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: METRIC_IBOX_ERROR_RATES,
		Help: "The ibox error rates",
	}, []string{METRIC_IBOX_NAME, METRIC_IBOX_IP, METRIC_IBOX_HOSTNAME})
	MetricIboxActiveCacheSSDDevicesGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: METRIC_IBOX_ACTIVE_CACHE_SSD_DEVICES,
		Help: "The ibox active cache ssd devices",
	}, []string{METRIC_IBOX_NAME, METRIC_IBOX_IP, METRIC_IBOX_HOSTNAME})
	MetricIboxActiveDrivesGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: METRIC_IBOX_ACTIVE_DRIVES,
		Help: "The ibox active drives",
	}, []string{METRIC_IBOX_NAME, METRIC_IBOX_IP, METRIC_IBOX_HOSTNAME})
	MetricIboxActiveEncryptedCacheSSDDevicesGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: METRIC_IBOX_ACTIVE_ENCRYPTED_CACHE_SSD_DEVICES,
		Help: "The ibox active encrypted cache ssd devices",
	}, []string{METRIC_IBOX_NAME, METRIC_IBOX_IP, METRIC_IBOX_HOSTNAME})
	MetricIboxActiveEncryptedDrivesGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: METRIC_IBOX_ACTIVE_ENCRYPTED_DRIVES,
		Help: "The ibox active encrypted drivers",
	}, []string{METRIC_IBOX_NAME, METRIC_IBOX_IP, METRIC_IBOX_HOSTNAME})
	MetricIboxBBUAggregateChargePctGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: METRIC_IBOX_BBU_AGGREGATE_CHARGE_PCT,
		Help: "The ibox BBU aggregate charge percentage",
	}, []string{METRIC_IBOX_NAME, METRIC_IBOX_IP, METRIC_IBOX_HOSTNAME})
	MetricIboxBBUProtectedNodesGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: METRIC_IBOX_BBU_PROTECTED_NODES,
		Help: "The ibox BBU protected nodes",
	}, []string{METRIC_IBOX_NAME, METRIC_IBOX_IP, METRIC_IBOX_HOSTNAME})
	MetricIboxEnclosureFailureSafeDistributionGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: METRIC_IBOX_ENCLOSURE_FAILURE_SAFE_DISTRIBUTION,
		Help: "The ibox enclosure failure safe distribution",
	}, []string{METRIC_IBOX_NAME, METRIC_IBOX_IP, METRIC_IBOX_HOSTNAME})
	MetricIboxEncryptionEnabledGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: METRIC_IBOX_ENCRYPTION_ENABLED,
		Help: "The ibox encryption enabled",
	}, []string{METRIC_IBOX_NAME, METRIC_IBOX_IP, METRIC_IBOX_HOSTNAME})

	MetricIboxFailedDrivesGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: METRIC_IBOX_FAILED_DRIVES,
		Help: "The ibox failed drives",
	}, []string{METRIC_IBOX_NAME, METRIC_IBOX_IP, METRIC_IBOX_HOSTNAME})
	MetricIboxInactiveNodesGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: METRIC_IBOX_INACTIVE_NODES,
		Help: "The ibox inactive nodes",
	}, []string{METRIC_IBOX_NAME, METRIC_IBOX_IP, METRIC_IBOX_HOSTNAME})
	MetricIboxMissingDrivesGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: METRIC_IBOX_MISSING_DRIVES,
		Help: "The ibox missing drives",
	}, []string{METRIC_IBOX_NAME, METRIC_IBOX_IP, METRIC_IBOX_HOSTNAME})
	MetricIboxPhasingOutDrivesGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: METRIC_IBOX_PHASING_OUT_DRIVES,
		Help: "The ibox phasing out drives",
	}, []string{METRIC_IBOX_NAME, METRIC_IBOX_IP, METRIC_IBOX_HOSTNAME})
	MetricIboxRaidGroupsPendingRebuild1Gauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: METRIC_IBOX_RAID_GROUPS_PENDING_REBUILD_1,
		Help: "The ibox raid groups pending rebuild 1",
	}, []string{METRIC_IBOX_NAME, METRIC_IBOX_IP, METRIC_IBOX_HOSTNAME})
	MetricIboxRaidGroupsPendingRebuild2Gauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: METRIC_IBOX_RAID_GROUPS_PENDING_REBUILD_2,
		Help: "The ibox raid groups pending rebuild 2",
	}, []string{METRIC_IBOX_NAME, METRIC_IBOX_IP, METRIC_IBOX_HOSTNAME})
	MetricIboxReadyDrivesGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: METRIC_IBOX_READY_DRIVES,
		Help: "The ibox ready drives",
	}, []string{METRIC_IBOX_NAME, METRIC_IBOX_IP, METRIC_IBOX_HOSTNAME})
	MetricIboxRebuild1InProgressGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: METRIC_IBOX_REBUILD_1_INPROGRESS,
		Help: "The ibox rebuild 1 in progress",
	}, []string{METRIC_IBOX_NAME, METRIC_IBOX_IP, METRIC_IBOX_HOSTNAME})
	MetricIboxRebuild2InProgressGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: METRIC_IBOX_REBUILD_2_INPROGRESS,
		Help: "The ibox rebuild 2 in progress",
	}, []string{METRIC_IBOX_NAME, METRIC_IBOX_IP, METRIC_IBOX_HOSTNAME})
	MetricIboxTestingDrivesGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: METRIC_IBOX_TESTING_DRIVES,
		Help: "The ibox testing drives",
	}, []string{METRIC_IBOX_NAME, METRIC_IBOX_IP, METRIC_IBOX_HOSTNAME})
	MetricIboxUnknownDrivesGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: METRIC_IBOX_UNKNOWN_DRIVES,
		Help: "The ibox unknown drives",
	}, []string{METRIC_IBOX_NAME, METRIC_IBOX_IP, METRIC_IBOX_HOSTNAME})
)

func RecordSystemHealthMetrics(cfg *MetricsConfig) {
	klog.V(4).Infof("system health metrics recording...")
	go func() {

		for {
			time.Sleep(cfg.GetDuration(METRIC_IBOX_SYSTEM_METRICS))

			results, err := getResult(cfg)
			if err != nil {
				klog.Error(err)
				continue
			}

			labels := prometheus.Labels{
				METRIC_IBOX_NAME:     results.Name,
				METRIC_IBOX_IP:       cfg.IboxIpAddress,
				METRIC_IBOX_HOSTNAME: cfg.IboxHostname,
			}
			MetricIboxActiveCacheSSDDevicesGauge.With(labels).Set(float64(results.HealthState.ActiveCacheSsdDevices))
			MetricIboxActiveDrivesGauge.With(labels).Set(float64(results.HealthState.ActiveDrives))
			MetricIboxActiveEncryptedCacheSSDDevicesGauge.With(labels).Set(float64(results.HealthState.ActiveEncryptedCacheSsdDevices))
			MetricIboxActiveEncryptedDrivesGauge.With(labels).Set(float64(results.HealthState.ActiveEncryptedDrives))
			MetricIboxBBUAggregateChargePctGauge.With(labels).Set(float64(results.HealthState.BbuAggregateChargePercent))

			for k, v := range results.HealthState.BbuChargeLevel {
				klog.V(4).Infof("bbucharge level k %s v %f", k, v)
				l := prometheus.Labels{
					METRIC_IBOX_NAME:      results.Name,
					METRIC_IBOX_IP:        cfg.IboxIpAddress,
					METRIC_IBOX_HOSTNAME:  cfg.IboxHostname,
					METRIC_IBOX_NODE_NAME: k}
				MetricIboxBBUChargeLevelGauge.With(l).Set(v.(float64))
			}
			MetricIboxBBUProtectedNodesGauge.With(labels).Set(float64(results.HealthState.BbuProtectedNodes))
			boolValue := 0
			if results.HealthState.EnclosureFailureSafeDistribution {
				boolValue = 1
			}
			MetricIboxEnclosureFailureSafeDistributionGauge.With(labels).Set(float64(boolValue))
			boolValue = 0
			if results.HealthState.EnclosureFailureSafeDistribution {
				boolValue = 1
			}
			MetricIboxEncryptionEnabledGauge.With(labels).Set(float64(boolValue))
			MetricIboxFailedDrivesGauge.With(labels).Set(float64(results.HealthState.FailedDrives))
			MetricIboxInactiveNodesGauge.With(labels).Set(float64(results.HealthState.InactiveNodes))
			MetricIboxMissingDrivesGauge.With(labels).Set(float64(results.HealthState.MissingDrives))

			klog.V(4).Infof("system health: nodebbuprotection %+v", results.HealthState.NodeBbuProtection)
			for k, v := range results.HealthState.NodeBbuProtection {
				klog.V(4).Infof("system health: nodebbuprotection k %s v %s", k, v)
				l := prometheus.Labels{
					METRIC_IBOX_NAME:      results.Name,
					METRIC_IBOX_IP:        cfg.IboxIpAddress,
					METRIC_IBOX_HOSTNAME:  cfg.IboxHostname,
					METRIC_IBOX_NODE_NAME: k}
				protectedValue := 0
				if v == "protected" {
					protectedValue = 1
				}
				MetricIboxNodeBBUProtectionGauge.With(l).Set(float64(protectedValue))
			}
			MetricIboxPhasingOutDrivesGauge.With(labels).Set(float64(results.HealthState.PhasingOutDrives))
			MetricIboxRaidGroupsPendingRebuild1Gauge.With(labels).Set(float64(results.HealthState.RaidGroupsPendingRebuild1))
			MetricIboxRaidGroupsPendingRebuild2Gauge.With(labels).Set(float64(results.HealthState.RaidGroupsPendingRebuild2))
			MetricIboxReadyDrivesGauge.With(labels).Set(float64(results.HealthState.ReadyDrives))
			boolValue = 0
			if results.HealthState.Rebuild1Inprogress {
				boolValue = 1
			}
			MetricIboxRebuild1InProgressGauge.With(labels).Set(float64(boolValue))
			boolValue = 0
			if results.HealthState.Rebuild2Inprogress {
				boolValue = 1
			}
			MetricIboxRebuild2InProgressGauge.With(labels).Set(float64(boolValue))
			MetricIboxTestingDrivesGauge.With(labels).Set(float64(results.HealthState.TestingDrives))
			MetricIboxUnknownDrivesGauge.With(labels).Set(float64(results.HealthState.UnknownDrives))

		}
	}()
}

type SystemStatus struct {
	Error    any      `json:"error"`
	Metadata Metadata `json:"metadata"`
	Result   Result   `json:"result"`
}
type Metadata struct {
	Ready bool `json:"ready"`
}
type Capacity struct {
	AllocatedPhysicalSpaceWithinPools int64   `json:"allocated_physical_space_within_pools"`
	AllocatedVirtualSpaceWithinPools  int64   `json:"allocated_virtual_space_within_pools"`
	DataReductionRatio                float64 `json:"data_reduction_ratio"`
	DynamicSpareDriveCost             int     `json:"dynamic_spare_drive_cost"`
	FreePhysicalSpace                 int64   `json:"free_physical_space"`
	FreeVirtualSpace                  int64   `json:"free_virtual_space"`
	TotalAllocatedPhysicalSpace       int64   `json:"total_allocated_physical_space"`
	TotalPhysicalCapacity             int64   `json:"total_physical_capacity"`
	TotalSpareBytes                   int64   `json:"total_spare_bytes"`
	TotalSparePartitions              int     `json:"total_spare_partitions"`
	TotalVirtualCapacity              int64   `json:"total_virtual_capacity"`
	UsedDynamicSpareBytes             int     `json:"used_dynamic_spare_bytes"`
	UsedDynamicSparePartitions        int     `json:"used_dynamic_spare_partitions"`
	UsedSpareBytes                    int     `json:"used_spare_bytes"`
	UsedSparePartitions               int     `json:"used_spare_partitions"`
}
type EntityCounts struct {
	Clusters            int `json:"clusters"`
	ConsistencyGroups   int `json:"consistency_groups"`
	FilesystemSnapshots int `json:"filesystem_snapshots"`
	Filesystems         int `json:"filesystems"`
	Hosts               int `json:"hosts"`
	MappedVolumes       int `json:"mapped_volumes"`
	Pools               int `json:"pools"`
	Replicas            int `json:"replicas"`
	ReplicationGroups   int `json:"replication_groups"`
	RgReplicas          int `json:"rg_replicas"`
	SnapshotGroups      int `json:"snapshot_groups"`
	StandardPools       int `json:"standard_pools"`
	VolumeSnapshots     int `json:"volume_snapshots"`
	Volumes             int `json:"volumes"`
	VvolPools           int `json:"vvol_pools"`
}
type BbuChargeLevel struct {
	Bbu1 int `json:"bbu-1"`
	Bbu2 int `json:"bbu-2"`
	Bbu3 int `json:"bbu-3"`
}
type NodeBbuProtection struct {
	Node1 string `json:"node-1"`
	Node2 string `json:"node-2"`
	Node3 string `json:"node-3"`
}
type HealthState struct {
	ActiveCacheSsdDevices            int                    `json:"active_cache_ssd_devices"`
	ActiveDrives                     int                    `json:"active_drives"`
	ActiveEncryptedCacheSsdDevices   int                    `json:"active_encrypted_cache_ssd_devices"`
	ActiveEncryptedDrives            int                    `json:"active_encrypted_drives"`
	BbuAggregateChargePercent        int                    `json:"bbu_aggregate_charge_percent"`
	BbuChargeLevel                   map[string]interface{} `json:"bbu_charge_level"`
	BbuProtectedNodes                int                    `json:"bbu_protected_nodes"`
	EnclosureFailureSafeDistribution bool                   `json:"enclosure_failure_safe_distribution"`
	EncryptionEnabled                bool                   `json:"encryption_enabled"`
	FailedDrives                     int                    `json:"failed_drives"`
	InactiveNodes                    int                    `json:"inactive_nodes"`
	MissingDrives                    int                    `json:"missing_drives"`
	NodeBbuProtection                map[string]interface{} `json:"node_bbu_protection"`
	PhasingOutDrives                 int                    `json:"phasing_out_drives"`
	RaidGroupsPendingRebuild1        int                    `json:"raid_groups_pending_rebuild_1"`
	RaidGroupsPendingRebuild2        int                    `json:"raid_groups_pending_rebuild_2"`
	ReadyDrives                      int                    `json:"ready_drives"`
	Rebuild1Inprogress               bool                   `json:"rebuild_1_inprogress"`
	Rebuild2Inprogress               bool                   `json:"rebuild_2_inprogress"`
	TestingDrives                    int                    `json:"testing_drives"`
	UnknownDrives                    int                    `json:"unknown_drives"`
}
type Localtime struct {
	UtcTime int64 `json:"utc_time"`
}
type OperationalState struct {
	Description    string `json:"description"`
	InitState      any    `json:"init_state"`
	Mode           string `json:"mode"`
	ReadOnlySystem bool   `json:"read_only_system"`
	State          string `json:"state"`
}
type Gui struct {
	BuildMode any    `json:"build_mode"`
	Revision  string `json:"revision"`
	Version   string `json:"version"`
}
type Infinishell struct {
	BuildMode any    `json:"build_mode"`
	Revision  string `json:"revision"`
	Version   string `json:"version"`
}
type System struct {
	BuildMode string `json:"build_mode"`
	Revision  string `json:"revision"`
	Version   string `json:"version"`
}
type Release struct {
	Gui         Gui         `json:"gui"`
	Infinishell Infinishell `json:"infinishell"`
	System      System      `json:"system"`
}
type FipsBestPractice struct {
	CertificateStrength             int    `json:"certificate_strength"`
	IsCertificateStrengthSufficient bool   `json:"is_certificate_strength_sufficient"`
	IsHTTPRedirection               bool   `json:"is_http_redirection"`
	IsLdapConnectionsSecured        bool   `json:"is_ldap_connections_secured"`
	IsLocalUsersDisabled            bool   `json:"is_local_users_disabled"`
	LocalUsersPasswordHash          string `json:"local_users_password_hash"`
	NumUsersPasswordHashNotSecured  int    `json:"num_users_password_hash_not_secured"`
}
type Security struct {
	EncryptionEnabled     bool             `json:"encryption_enabled"`
	FipsBestPractice      FipsBestPractice `json:"fips_best_practice"`
	KmipConnectivityState string           `json:"kmip_connectivity_state"`
	SystemSecurityState   string           `json:"system_security_state"`
}
type Result struct {
	Capacity               Capacity         `json:"capacity"`
	DeploymentID           string           `json:"deployment_id"`
	EntityCounts           EntityCounts     `json:"entity_counts"`
	FullModel              string           `json:"full_model"`
	HealthState            HealthState      `json:"health_state"`
	InstallTimestamp       int64            `json:"install_timestamp"`
	Localtime              Localtime        `json:"localtime"`
	Model                  string           `json:"model"`
	Name                   string           `json:"name"`
	OperationalState       OperationalState `json:"operational_state"`
	ProductID              string           `json:"product_id"`
	Release                Release          `json:"release"`
	Security               Security         `json:"security"`
	SerialNumber           int              `json:"serial_number"`
	SystemPowerConsumption float64          `json:"system_power_consumption"`
	UpgradeTimestamp       int64            `json:"upgrade_timestamp"`
	Uptime                 int64            `json:"uptime"`
	Version                string           `json:"version"`
	Wwnn                   string           `json:"wwnn"`
}

func getResult(config *MetricsConfig) (Result, error) {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
	}

	transport := &http.Transport{
		TLSClientConfig: tlsConfig,
	}
	client := http.Client{
		Timeout:   60 * time.Second,
		Transport: transport,
	}

	req, err := http.NewRequest(http.MethodGet, "https://"+config.IboxHostname+"/api/rest/system", http.NoBody)
	if err != nil {
		klog.Error(err)
		return Result{}, err
	}

	req.SetBasicAuth(config.IboxUsername, config.IboxPassword)
	req.Header.Set("Content-Type", "application/json")

	res, err := client.Do(req)
	if err != nil {
		klog.Error(err)
		return Result{}, err
	}

	defer res.Body.Close()

	responseData, err := io.ReadAll(res.Body)
	if err != nil {
		klog.Error(err)
		return Result{}, err
	}
	//fmt.Println(string(responseData))

	r := SystemStatus{}
	err = json.Unmarshal(responseData, &r)
	if err != nil {
		klog.Error(err)
		return Result{}, err
	}
	//fmt.Printf("API Result %+v\n", r.Result)
	return r.Result, nil
}
