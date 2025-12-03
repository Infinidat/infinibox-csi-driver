package metric

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net"
	"os"
	"time"

	"github.com/infinidat/infinibox-csi-driver/common"

	"k8s.io/apimachinery/pkg/util/yaml"
)

// command line flags
var (
	PortFlag *string
)

const (
	DefaultInterval = "30s"

	// pool metrics
	PoolMetrics = "pool_metrics"

	MetricPoolAvailableCap = "ibox_pool_available_cap"
	MetricPoolUsedCap      = "ibox_pool_used_cap"
	MetricPoolPctUtilized  = "ibox_pool_pct_utilized"

	// pool metric general information
	MetricPoolName            = "pool_name"
	MetricPoolProvisionType   = "pool_provision_type"
	MetricPoolSSDEnabled      = "pool_ssd_enabled"
	MetricPoolNetworkSpace    = "pool_network_space"
	MetricPoolStorageProtocol = "pool_storage_protocol"

	// pv metrics
	PVMetrics = "pv_metrics"

	MetricPVTotalSize = "ibox_pv_total_size"

	// pv metric general information
	MetricPVName            = "pv_name"
	MetricPVStorageClass    = "pv_storage_class"
	MetricPVProvisionType   = "pv_provision_type"
	MetricPVSSDEnabled      = "pv_ssd_enabled"
	MetricPVNetworkSpace    = "pv_network_space"
	MetricPVStorageProtocol = "pv_storage_protocol"

	// ibox performance metrics
	MetricIboxProtocol    = "ibox_protocol"
	MetricIboxPerfMetrics = "ibox_performance_metrics"

	MetricIboxPerfIOPS       = "ibox_perf_iops"
	MetricIboxPerfThroughput = "ibox_perf_throughput"
	MetricIboxPerfLatency    = "ibox_perf_latency"

	// ibox system metrics
	MetricIboxNodeName      = "node"
	MetricIboxSystemMetrics = "ibox_system_metrics"

	MetricIboxErrorRates                       = "ibox_error_rates" // TODO
	MetricIboxActiveCacheSSDDevices            = "ibox_active_cache_ssd_devices"
	MetricIboxActiveDrives                     = "ibox_active_drives"
	MetricIboxActiveEncryptedCacheSSDDevices   = "ibox_active_encrypted_cache_ssd_devices"
	MetricIboxActiveEncryptedDrives            = "ibox_active_encrypted_drives"
	MetricIboxBBUAggregateChargePct            = "ibox_bbu_aggregate_charge_percent"
	MetricIboxBBUChargeLevel                   = "ibox_bbu_charge_level"
	MetricIboxBBUProtectedNodes                = "ibox_bbu_protected_nodes"
	MetricIboxEnclosureFailureSafeDistribution = "ibox_enclosure_failure_safe_distribution"
	MetricIboxEncryptionEnabled                = "ibox_encryption_enabled"
	MetricIboxFailedDrives                     = "ibox_failed_drives"
	MetricIboxInactiveNodes                    = "ibox_inactive_nodes"
	MetricIboxMissingDrives                    = "ibox_missing_drives"
	MetricIboxNodeBBUProtection                = "ibox_node_bbu_protection"
	MetricIboxPhasingOutDrives                 = "ibox_phasing_out_drives"
	MetricIboxRAIDGroupsPendingRebuild1        = "ibox_raid_groups_pending_rebuild_1"
	MetricIboxRAIDGroupsPendingRebuild2        = "ibox_raid_groups_pending_rebuild_2"
	MetricIboxReadyDrives                      = "ibox_ready_drives"
	MetricIboxRebuild1InProgress               = "ibox_rebuild_1_inprogress"
	MetricIboxRebuild2InProgress               = "ibox_rebuild_2_inprogress"
	MetricIboxTestingDrives                    = "ibox_testing_drives"
	MetricIboxUnknownDrives                    = "ibox_unknown_drives"

	// ibox metric general information
	MetricIboxName                   = "ibox_name"
	MetricIboxIP                     = "ibox_ip_address"
	MetricIboxHostname               = "ibox_ip_hostname"
	MetricIboxBBUChargeLevelName     = "ibox_bbu_charge_level_name"
	MetricIboxBBUChargeLevelValue    = "ibox_bbu_charge_level_value"
	MetricIboxNodeBBUProtectionName  = "ibox_node_bbu_bbu_protection_name"
	MetricIboxNodeBBUProtectionValue = "ibox_node_bbu_protection_value"
)

type IboxCredentials struct {
	IboxHostname  string
	IboxIPAddress string
	IboxPassword  string
	IboxUsername  string
}
type MetricsConfig struct {
	// ibox credentials from the Secret that gets mounted at /tmp/infinibox-creds
	// since the config gets passed pretty much everywhere, this is a reasonable
	// place to store these credentials for invoking the ibox REST API
	Ibox []IboxCredentials

	Spec struct {
		Metrics []struct {
			Duration string `yaml:"duration"`
			Name     string `yaml:"name"`
		} `yaml:"metrics"`
	} `yaml:"spec"`
}

func NewConfig(secrets []map[string]string) (*MetricsConfig, error) {
	slog.Log(context.Background(), common.LevelTrace, "getting metrics configuration...")
	PortFlag = flag.String("port", "11007", "metrics port")

	configFileData, err := os.ReadFile("/tmp/infinidat-csi-metrics-config/config.yaml")
	if err != nil {
		return nil, err
	}
	slog.Log(context.Background(), common.LevelTrace, "raw metrics configuration...", "data", string(configFileData))

	var config MetricsConfig
	err = yaml.Unmarshal(configFileData, &config)
	if err != nil {
		return nil, err
	}
	slog.Info("metrics config", "config", config)

	errorsFound := config.Validate()
	if errorsFound {
		return nil, errors.New("validation of the metrics config file failed")
	}

	// lookup all the credentials for n-number of iboxes

	config.Ibox = make([]IboxCredentials, 0)

	for i := range secrets {
		sMap := secrets[i]
		ibox := IboxCredentials{
			IboxHostname: sMap[common.CredentialHostname],
			IboxPassword: sMap[common.CredentialPassword],
			IboxUsername: sMap[common.CredentialUsername],
		}
		ips, err := net.LookupIP(ibox.IboxHostname)
		if err != nil {
			ibox.IboxIPAddress = "unknown"
		} else {
			for _, ip := range ips {
				ibox.IboxIPAddress = ip.String()
			}
		}
		config.Ibox = append(config.Ibox, ibox)
	}

	return &config, nil
}

func (c *MetricsConfig) GetDuration(name string) time.Duration {
	metrics := c.Spec.Metrics
	for i := range metrics {
		if metrics[i].Name == name {
			t, e := time.ParseDuration(metrics[i].Duration)
			if e != nil {
				slog.Error("parse error:  duration found for metrics config, using default", "name", name, "default", DefaultInterval, "error", e.Error())
				t, _ = time.ParseDuration(DefaultInterval)
			}
			return t
		}
	}
	slog.Info("warning:  no value found for metrics config, using default", "name", name, "default", DefaultInterval)
	t, _ := time.ParseDuration(DefaultInterval)
	return t
}

func (c *MetricsConfig) Validate() bool {
	errorFound := false
	metrics := c.Spec.Metrics
	for _, metric := range metrics {
		_, e := time.ParseDuration(metric.Duration)
		if e != nil {
			errorFound = true
			slog.Error("error:  duration found for metrics config did not parse", "name", metric.Name, "error", e.Error())
		}

		switch metric.Name {
		case PoolMetrics, PVMetrics, MetricIboxPerfMetrics, MetricIboxSystemMetrics:
		default:
			errorFound = true
			slog.Error("error:  metric name invalid", "name", metric.Name)
		}
	}

	return errorFound
}
