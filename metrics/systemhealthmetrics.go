package metric

import (
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	BBUChargeLevel = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: MetricIboxBBUChargeLevel,
		Help: "The ibox BBU charge level",
	}, []string{MetricIboxName, MetricIboxIP, MetricIboxHostname, MetricIboxNodeName})
	NodeBBUProtection = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: MetricIboxNodeBBUProtection,
		Help: "The ibox node BBU protection",
	}, []string{MetricIboxName, MetricIboxIP, MetricIboxHostname, MetricIboxNodeName})
	ErrorRates = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: MetricIboxErrorRates,
		Help: "The ibox error rates",
	}, []string{MetricIboxName, MetricIboxIP, MetricIboxHostname})
	ActiveCacheSSDDevices = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: MetricIboxActiveCacheSSDDevices,
		Help: "The ibox active cache ssd devices",
	}, []string{MetricIboxName, MetricIboxIP, MetricIboxHostname})
	ActiveDrives = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: MetricIboxActiveDrives,
		Help: "The ibox active drives",
	}, []string{MetricIboxName, MetricIboxIP, MetricIboxHostname})
	ActiveEncryptedCacheSSDDevices = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: MetricIboxActiveEncryptedCacheSSDDevices,
		Help: "The ibox active encrypted cache ssd devices",
	}, []string{MetricIboxName, MetricIboxIP, MetricIboxHostname})
	ActiveEncryptedDrives = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: MetricIboxActiveEncryptedDrives,
		Help: "The ibox active encrypted drivers",
	}, []string{MetricIboxName, MetricIboxIP, MetricIboxHostname})
	BBUAggregateChargePct = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: MetricIboxBBUAggregateChargePct,
		Help: "The ibox BBU aggregate charge percentage",
	}, []string{MetricIboxName, MetricIboxIP, MetricIboxHostname})
	BBUProtectedNodes = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: MetricIboxBBUProtectedNodes,
		Help: "The ibox BBU protected nodes",
	}, []string{MetricIboxName, MetricIboxIP, MetricIboxHostname})
	EnclosureFailureSafeDistribution = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: MetricIboxEnclosureFailureSafeDistribution,
		Help: "The ibox enclosure failure safe distribution",
	}, []string{MetricIboxName, MetricIboxIP, MetricIboxHostname})
	EncryptionEnabled = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: MetricIboxEncryptionEnabled,
		Help: "The ibox encryption enabled",
	}, []string{MetricIboxName, MetricIboxIP, MetricIboxHostname})

	FailedDrives = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: MetricIboxFailedDrives,
		Help: "The ibox failed drives",
	}, []string{MetricIboxName, MetricIboxIP, MetricIboxHostname})
	InactiveNodes = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: MetricIboxInactiveNodes,
		Help: "The ibox inactive nodes",
	}, []string{MetricIboxName, MetricIboxIP, MetricIboxHostname})
	MissingDrives = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: MetricIboxMissingDrives,
		Help: "The ibox missing drives",
	}, []string{MetricIboxName, MetricIboxIP, MetricIboxHostname})
	PhasingOutDrives = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: MetricIboxPhasingOutDrives,
		Help: "The ibox phasing out drives",
	}, []string{MetricIboxName, MetricIboxIP, MetricIboxHostname})
	RaidGroupsPendingRebuild1 = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: MetricIboxRAIDGroupsPendingRebuild1,
		Help: "The ibox raid groups pending rebuild 1",
	}, []string{MetricIboxName, MetricIboxIP, MetricIboxHostname})
	RaidGroupsPendingRebuild2 = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: MetricIboxRAIDGroupsPendingRebuild2,
		Help: "The ibox raid groups pending rebuild 2",
	}, []string{MetricIboxName, MetricIboxIP, MetricIboxHostname})
	ReadyDrives = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: MetricIboxReadyDrives,
		Help: "The ibox ready drives",
	}, []string{MetricIboxName, MetricIboxIP, MetricIboxHostname})
	Rebuild1InProgress = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: MetricIboxRebuild1InProgress,
		Help: "The ibox rebuild 1 in progress",
	}, []string{MetricIboxName, MetricIboxIP, MetricIboxHostname})
	Rebuild2InProgress = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: MetricIboxRebuild2InProgress,
		Help: "The ibox rebuild 2 in progress",
	}, []string{MetricIboxName, MetricIboxIP, MetricIboxHostname})
	TestingDrives = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: MetricIboxTestingDrives,
		Help: "The ibox testing drives",
	}, []string{MetricIboxName, MetricIboxIP, MetricIboxHostname})
	UnknownDrives = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: MetricIboxUnknownDrives,
		Help: "The ibox unknown drives",
	}, []string{MetricIboxName, MetricIboxIP, MetricIboxHostname})
)

func RecordSystemHealthMetrics(cfg *MetricsConfig) {
	zlog.Trace().Msgf("system health metrics recording...")
	go func() {
		for {
			time.Sleep(cfg.GetDuration(MetricIboxSystemMetrics))

			for _, credential := range cfg.Ibox {
				zlog.Trace().Msgf("system health metrics: creating collectors for %s...", credential.IboxHostname)
				results, err := getResult(credential)
				if err != nil {
					zlog.Err(err)
					continue
				}

				labels := prometheus.Labels{
					MetricIboxName:     results.Name,
					MetricIboxIP:       credential.IboxIPAddress,
					MetricIboxHostname: credential.IboxHostname,
				}
				ActiveCacheSSDDevices.With(labels).Set(float64(results.HealthState.ActiveCacheSsdDevices))
				ActiveDrives.With(labels).Set(float64(results.HealthState.ActiveDrives))
				ActiveEncryptedCacheSSDDevices.With(labels).Set(float64(results.HealthState.ActiveEncryptedCacheSsdDevices))
				ActiveEncryptedDrives.With(labels).Set(float64(results.HealthState.ActiveEncryptedDrives))
				BBUAggregateChargePct.With(labels).Set(float64(results.HealthState.BbuAggregateChargePercent))

				for index, bbuChargeLevel := range results.HealthState.BbuChargeLevel {
					zlog.Trace().Msgf("bbucharge level k %s v %f", index, bbuChargeLevel)
					l := prometheus.Labels{
						MetricIboxName:     results.Name,
						MetricIboxIP:       credential.IboxIPAddress,
						MetricIboxHostname: credential.IboxHostname,
						MetricIboxNodeName: index}
					BBUChargeLevel.With(l).Set(bbuChargeLevel.(float64))
				}
				BBUProtectedNodes.With(labels).Set(float64(results.HealthState.BbuProtectedNodes))
				var boolValue int
				if results.HealthState.EnclosureFailureSafeDistribution {
					boolValue = 1
				}
				EnclosureFailureSafeDistribution.With(labels).Set(float64(boolValue))
				boolValue = 0
				if results.HealthState.EnclosureFailureSafeDistribution {
					boolValue = 1
				}
				EncryptionEnabled.With(labels).Set(float64(boolValue))
				FailedDrives.With(labels).Set(float64(results.HealthState.FailedDrives))
				InactiveNodes.With(labels).Set(float64(results.HealthState.InactiveNodes))
				MissingDrives.With(labels).Set(float64(results.HealthState.MissingDrives))

				zlog.Trace().Msgf("system health: nodebbuprotection %+v", results.HealthState.NodeBbuProtection)
				for index, nodeBBUProt := range results.HealthState.NodeBbuProtection {
					zlog.Trace().Msgf("system health: nodebbuprotection k %s v %s", index, nodeBBUProt)
					label := prometheus.Labels{
						MetricIboxName:     results.Name,
						MetricIboxIP:       credential.IboxIPAddress,
						MetricIboxHostname: credential.IboxHostname,
						MetricIboxNodeName: index}
					var protectedValue int
					if nodeBBUProt == "protected" {
						protectedValue = 1
					}
					NodeBBUProtection.With(label).Set(float64(protectedValue))
				}
				PhasingOutDrives.With(labels).Set(float64(results.HealthState.PhasingOutDrives))
				RaidGroupsPendingRebuild1.With(labels).Set(float64(results.HealthState.RaidGroupsPendingRebuild1))
				RaidGroupsPendingRebuild2.With(labels).Set(float64(results.HealthState.RaidGroupsPendingRebuild2))
				ReadyDrives.With(labels).Set(float64(results.HealthState.ReadyDrives))
				boolValue = 0
				if results.HealthState.Rebuild1Inprogress {
					boolValue = 1
				}
				Rebuild1InProgress.With(labels).Set(float64(boolValue))
				boolValue = 0
				if results.HealthState.Rebuild2Inprogress {
					boolValue = 1
				}
				Rebuild2InProgress.With(labels).Set(float64(boolValue))
				TestingDrives.With(labels).Set(float64(results.HealthState.TestingDrives))
				UnknownDrives.With(labels).Set(float64(results.HealthState.UnknownDrives))
			}
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

func getResult(ibox IboxCredentials) (Result, error) {
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

	req, err := http.NewRequest(http.MethodGet, "https://"+ibox.IboxHostname+"/api/rest/system", http.NoBody)
	if err != nil {
		zlog.Err(err)
		return Result{}, err
	}

	req.SetBasicAuth(ibox.IboxUsername, ibox.IboxPassword)
	req.Header.Set("Content-Type", "application/json")

	res, err := client.Do(req)
	if err != nil {
		zlog.Err(err)
		return Result{}, err
	}

	defer func() {
		if err := res.Body.Close(); err != nil {
			zlog.Error().Msgf("error in Close() %s", err.Error())
		}
	}()

	responseData, err := io.ReadAll(res.Body)
	if err != nil {
		zlog.Err(err)
		return Result{}, err
	}
	// fmt.Println(string(responseData))

	systemStatus := SystemStatus{}
	err = json.Unmarshal(responseData, &systemStatus)
	if err != nil {
		zlog.Err(err)
		return Result{}, err
	}
	// fmt.Printf("API Result %+v\n", r.Result)
	return systemStatus.Result, nil
}
