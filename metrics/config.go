package metric

import (
	"errors"
	"flag"
	"net"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/klog/v2"
)

// command line flags
var (
	PortFlag *string
)

const (
	DEFAULT_INTERVAL = "30s"

	// pool metrics
	METRIC_POOL_METRICS = "pool_metrics"

	METRIC_POOL_AVAILABLE_CAP = "pool_available_cap"
	METRIC_POOL_USED_CAP      = "pool_used_cap"
	METRIC_POOL_PCT_UTILIZED  = "pool_pct_utilized"

	// pool metric general infomation
	METRIC_POOL_NAME             = "pool_name"
	METRIC_POOL_PROVISION_TYPE   = "pool_provision_type"
	METRIC_POOL_SSD_ENABLED      = "pool_ssd_enabled"
	METRIC_POOL_NETWORK_SPACE    = "pool_network_space"
	METRIC_POOL_STORAGE_PROTOCOL = "pool_storage_protocol"

	// pv metrics
	METRIC_PV_METRICS = "pv_metrics"

	METRIC_PV_TOTAL_SIZE = "pv_total_size"

	// pv metric general information
	METRIC_PV_NAME             = "pv_name"
	METRIC_PV_STORAGE_CLASS    = "pv_storage_class"
	METRIC_PV_PROVISION_TYPE   = "pv_provision_type"
	METRIC_PV_SSD_ENABLED      = "pv_ssd_enabled"
	METRIC_PV_NETWORK_SPACE    = "pv_network_space"
	METRIC_PV_STORAGE_PROTOCOL = "pv_storage_protocol"

	// ibox performance metrics
	METRIC_IBOX_PROTOCOL            = "ibox_protocol"
	METRIC_IBOX_PERFORMANCE_METRICS = "ibox_performance_metrics"

	METRIC_IBOX_PERFORMANCE_IOPS       = "ibox_perf_iops"
	METRIC_IBOX_PERFORMANCE_THROUGHPUT = "ibox_perf_throughput"
	METRIC_IBOX_PERFORMANCE_LATENCY    = "ibox_perf_latency"

	// ibox system metrics
	METRIC_IBOX_NODE_NAME      = "node"
	METRIC_IBOX_SYSTEM_METRICS = "ibox_system_metrics"

	METRIC_IBOX_ERROR_RATES                         = "ibox_error_rates"
	METRIC_IBOX_ACTIVE_CACHE_SSD_DEVICES            = "ibox_active_cache_ssd_devices"
	METRIC_IBOX_ACTIVE_DRIVES                       = "ibox_active_drives"
	METRIC_IBOX_ACTIVE_ENCRYPTED_CACHE_SSD_DEVICES  = "ibox_active_encrypted_cache_ssd_devices"
	METRIC_IBOX_ACTIVE_ENCRYPTED_DRIVES             = "ibox_active_encrypted_drives"
	METRIC_IBOX_BBU_AGGREGATE_CHARGE_PCT            = "ibox_bbu_aggregate_charge_percent"
	METRIC_IBOX_BBU_CHARGE_LEVEL                    = "ibox_bbu_charge_level"
	METRIC_IBOX_BBU_PROTECTED_NODES                 = "ibox_bbu_protected_nodes"
	METRIC_IBOX_ENCLOSURE_FAILURE_SAFE_DISTRIBUTION = "ibox_enclosure_failure_safe_distribution"
	METRIC_IBOX_ENCRYPTION_ENABLED                  = "ibox_encryption_enabled"
	METRIC_IBOX_FAILED_DRIVES                       = "ibox_failed_drives"
	METRIC_IBOX_INACTIVE_NODES                      = "ibox_inactive_nodes"
	METRIC_IBOX_MISSING_DRIVES                      = "ibox_missing_drives"
	METRIC_IBOX_NODE_BBU_PROTECTION                 = "ibox_node_bbu_protection"
	METRIC_IBOX_PHASING_OUT_DRIVES                  = "ibox_phasing_out_drives"
	METRIC_IBOX_RAID_GROUPS_PENDING_REBUILD_1       = "ibox_raid_groups_pending_rebuild_1"
	METRIC_IBOX_RAID_GROUPS_PENDING_REBUILD_2       = "ibox_raid_groups_pending_rebuild_2"
	METRIC_IBOX_READY_DRIVES                        = "ibox_ready_drives"
	METRIC_IBOX_REBUILD_1_INPROGRESS                = "ibox_rebuild_1_inprogress"
	METRIC_IBOX_REBUILD_2_INPROGRESS                = "ibox_rebuild_2_inprogress"
	METRIC_IBOX_TESTING_DRIVES                      = "ibox_testing_drives"
	METRIC_IBOX_UNKNOWN_DRIVES                      = "ibox_unknown_drives"

	// ibox metric general information
	METRIC_IBOX_NAME                      = "ibox_name"
	METRIC_IBOX_IP                        = "ibox_ip_address"
	METRIC_IBOX_HOSTNAME                  = "ibox_ip_hostname"
	METRIC_IBOX_BBU_CHARGE_LEVEL_NAME     = "ibox_bbu_charge_level_name"
	METRIC_IBOX_BBU_CHARGE_LEVEL_VALUE    = "ibox_bbu_charge_level_value"
	METRIC_IBOX_NODE_BBU_PROTECTION_NAME  = "ibox_node_bbu_bbu_protection_name"
	METRIC_IBOX_NODE_BBU_PROTECTION_VALUE = "ibox_node_bbu_protection_value"
)

type MetricsConfig struct {
	// ibox credentials from the Secret that gets mounted at /tmp/infinibox-creds
	// since the config gets passed pretty much everywhere, this is a reasonable
	// place to store these credentials for invoking the ibox REST API
	IboxHostname  string
	IboxIpAddress string
	IboxPassword  string
	IboxUsername  string

	Spec struct {
		Metrics []struct {
			Duration string `yaml:"duration"`
			Name     string `yaml:"name"`
		} `yaml:"metrics"`
	} `yaml:"spec"`
}

func NewConfig() (*MetricsConfig, error) {
	var config MetricsConfig
	klog.V(4).Info("getting metrics configuration...")
	PortFlag = flag.String("port", "11007", "metrics port")

	configFileData, err := os.ReadFile("/tmp/infinidat-csi-metrics-config/config.yaml")
	if err != nil {
		return nil, err
	}
	klog.V(4).Info("raw metrics configuration...%s", string(configFileData))

	err = yaml.Unmarshal(configFileData, &config)
	if err != nil {
		return nil, err
	}
	klog.V(2).Infof("metrics config is ...%+v\n", config)

	errorsFound := config.Validate()
	if errorsFound {
		return nil, errors.New("validation of the metrics config file failed")
	}

	var tmp []byte
	tmp, err = os.ReadFile("/tmp/infinibox-creds/username")
	if err != nil {
		return nil, err
	}
	config.IboxUsername = string(tmp)

	tmp, err = os.ReadFile("/tmp/infinibox-creds/password")
	if err != nil {
		return nil, err
	}
	config.IboxPassword = string(tmp)

	tmp, err = os.ReadFile("/tmp/infinibox-creds/hostname")
	if err != nil {
		return nil, err
	}
	config.IboxHostname = string(tmp)

	ips, err := net.LookupIP(config.IboxHostname)
	if err != nil {
		config.IboxIpAddress = "unknown"
	} else {
		for _, ip := range ips {
			config.IboxIpAddress = ip.String()
		}
	}

	return &config, nil
}

func (c *MetricsConfig) GetDuration(name string) time.Duration {
	metrics := c.Spec.Metrics
	for i := 0; i < len(metrics); i++ {
		if metrics[i].Name == name {
			t, e := time.ParseDuration(metrics[i].Duration)
			if e != nil {
				klog.V(2).Infof("error:  duration found for metrics config %s did not parse, using default %s, %s", name, DEFAULT_INTERVAL, e.Error())
				t, _ = time.ParseDuration(DEFAULT_INTERVAL)
			}
			return t
		}
	}
	klog.V(2).Infof("warning:  no value found for metrics config %s, using default %s", name, DEFAULT_INTERVAL)
	t, _ := time.ParseDuration(DEFAULT_INTERVAL)
	return t
}

func (c *MetricsConfig) Validate() bool {
	errorFound := false
	metrics := c.Spec.Metrics
	for i := 0; i < len(metrics); i++ {
		_, e := time.ParseDuration(metrics[i].Duration)
		if e != nil {
			errorFound = true
			klog.V(2).Infof("error:  duration found for metrics config %s did not parse, %s", metrics[i].Name, e.Error())
		}

		switch metrics[i].Name {
		case METRIC_POOL_METRICS, METRIC_PV_METRICS, METRIC_IBOX_PERFORMANCE_METRICS, METRIC_IBOX_SYSTEM_METRICS:
		default:
			errorFound = true
			klog.V(2).Infof("error:  metric name %s invalid", metrics[i].Name)
		}
	}

	return errorFound
}
