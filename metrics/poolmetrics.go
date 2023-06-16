package metric

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/klog/v2"
)

var (
	MetricPoolAvailableCapGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: METRIC_POOL_AVAILABLE_CAP,
		Help: "The pool available capacity",
	}, []string{METRIC_POOL_NAME, METRIC_POOL_PROVISION_TYPE, METRIC_POOL_SSD_ENABLED, METRIC_POOL_NETWORK_SPACE, METRIC_POOL_STORAGE_PROTOCOL})
	MetricPoolUsedCapGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: METRIC_POOL_USED_CAP,
		Help: "The pool used capacity",
	}, []string{METRIC_POOL_NAME, METRIC_POOL_PROVISION_TYPE, METRIC_POOL_SSD_ENABLED, METRIC_POOL_NETWORK_SPACE, METRIC_POOL_STORAGE_PROTOCOL})
	MetricPoolPctUtilizedGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: METRIC_POOL_PCT_UTILIZED,
		Help: "The pool percentage of capacity utilized",
	}, []string{METRIC_POOL_NAME, METRIC_POOL_PROVISION_TYPE, METRIC_POOL_SSD_ENABLED, METRIC_POOL_NETWORK_SPACE, METRIC_POOL_STORAGE_PROTOCOL})
)

func RecordPoolMetrics(config *MetricsConfig) {
	klog.V(4).Infof("pool metrics recording...")
	go func() {
		for {
			time.Sleep(config.GetDuration(METRIC_POOL_METRICS))

			poolInfo, err := getPoolInfo(config)
			if err != nil {
				klog.Error(err)
				continue
			}

			for i := 0; i < len(poolInfo); i++ {
				pi := poolInfo[i]
				labels := prometheus.Labels{
					METRIC_POOL_NAME:             pi.storageClass.Parameters["pool_name"],
					METRIC_POOL_PROVISION_TYPE:   pi.storageClass.Parameters["provision_type"],
					METRIC_POOL_SSD_ENABLED:      pi.storageClass.Parameters["ssd_enabled"],
					METRIC_POOL_NETWORK_SPACE:    pi.storageClass.Parameters["network_space"],
					METRIC_POOL_STORAGE_PROTOCOL: pi.storageClass.Parameters["storage_protocol"],
				}
				MetricPoolAvailableCapGauge.With(labels).Set(float64(pi.pool.PhysicalCapacity))  // pool - physical_capacity
				MetricPoolUsedCapGauge.With(labels).Set(float64(pi.pool.AllocatedPhysicalSpace)) // pool -  allocated_physical_space
				pct := (pi.pool.AllocatedPhysicalSpace / pi.pool.PhysicalCapacity) * 100.00
				MetricPoolPctUtilizedGauge.With(labels).Set(float64(pct)) // pool - (allocated_physical_space / physical_capacity) * 100.00
			}

		}
	}()
}

type PoolInfo struct {
	storageClass storagev1.StorageClass
	pool         Pool
}

func getPoolInfo(config *MetricsConfig) ([]PoolInfo, error) {
	poolInfo := make([]PoolInfo, 0)
	storageClasses, err := getStorageClasses()
	if err != nil {
		klog.Error(err)
		return nil, err
	}
	allPools, err := getPools(config)
	if err != nil {
		klog.Error(err)
		return poolInfo, err
	}
	for i := 0; i < len(*storageClasses); i++ {
		sc := (*storageClasses)[i]
		p, err := lookupPool(allPools, sc.Parameters["pool_name"])
		if err != nil {
			klog.Error("pool_name not found from storage classes %s", sc.Parameters["pool_name"])
		} else {
			pi := PoolInfo{
				storageClass: sc,
				pool:         *p,
			}
			poolInfo = append(poolInfo, pi)
		}
	}
	return poolInfo, nil
}
func getStorageClasses() (*[]storagev1.StorageClass, error) {
	// creates the in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		klog.Error(err)
		return nil, err
	}

	// create the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		klog.Error(err)
		return nil, err
	}

	storageClasses, err := clientset.StorageV1().StorageClasses().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		klog.Error(err)
		return nil, err
	}

	ourStorageClasses := make([]storagev1.StorageClass, 0)
	for i := 0; i < len(storageClasses.Items); i++ {
		s := storageClasses.Items[i]
		if s.Provisioner == "infinibox-csi-driver" {
			// this is a storageclass used by our driver
			klog.V(4).Infof("storageclass name %s\n", s.Name)
			klog.V(4).Infof("storage_protocol %s\n", s.Parameters["storage_protocol"])
			klog.V(4).Infof("network_space %s\n", s.Parameters["network_space"])
			klog.V(4).Infof("pool_name %s\n", s.Parameters["pool_name"])
			klog.V(4).Infof("provision_type %s\n", s.Parameters["provision_type"])
			klog.V(4).Infof("ssd_enabled %s\n", s.Parameters["ssd_enabled"])
			klog.V(4).Infof("--------------------------------------")
			ourStorageClasses = append(ourStorageClasses, s)
		}
	}

	return &ourStorageClasses, nil
}

func getPools(config *MetricsConfig) (*Pools, error) {
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

	req, err := http.NewRequest(http.MethodGet, "https://"+config.IboxHostname+"/api/rest/pools", http.NoBody)
	if err != nil {
		klog.Error(err)
		return nil, err
	}

	req.SetBasicAuth(config.IboxUsername, config.IboxPassword)
	req.Header.Set("Content-Type", "application/json")

	res, err := client.Do(req)
	if err != nil {
		klog.Error(err)
		return nil, err
	}

	defer res.Body.Close()

	responseData, err := io.ReadAll(res.Body)
	if err != nil {
		klog.Error(err)
		return nil, err
	}
	//fmt.Println(string(responseData))

	r := Pools{}
	err = json.Unmarshal(responseData, &r)
	if err != nil {
		log.Fatal(err)
	}
	//fmt.Printf("API Result %+v\n", r.Result)
	return &r, nil
}

type Pool struct {
	ID                               int    `json:"id"`
	Name                             string `json:"name"`
	CreatedAt                        int64  `json:"created_at"`
	UpdatedAt                        int64  `json:"updated_at"`
	PhysicalCapacity                 int64  `json:"physical_capacity"`
	VirtualCapacity                  int64  `json:"virtual_capacity"`
	PhysicalCapacityWarning          int    `json:"physical_capacity_warning"`
	PhysicalCapacityCritical         int    `json:"physical_capacity_critical"`
	State                            string `json:"state"`
	FreePhysicalSpace                int64  `json:"free_physical_space"`
	ReservedCapacity                 int64  `json:"reserved_capacity"`
	MaxExtend                        int    `json:"max_extend"`
	SsdEnabled                       bool   `json:"ssd_enabled"`
	CompressionEnabled               bool   `json:"compression_enabled"`
	Type                             string `json:"type"`
	AllocatedPhysicalSpace           int64  `json:"allocated_physical_space"`
	CapacitySavings                  int64  `json:"capacity_savings"`
	TenantID                         int    `json:"tenant_id"`
	EntitiesCount                    int    `json:"entities_count"`
	FreeVirtualSpace                 int64  `json:"free_virtual_space"`
	StandardFilesystemSnapshotsCount int    `json:"standard_filesystem_snapshots_count"`
	VolumesCount                     int    `json:"volumes_count"`
	FilesystemsCount                 int    `json:"filesystems_count"`
	SnapshotsCount                   int    `json:"snapshots_count"`
	FilesystemSnapshotsCount         int    `json:"filesystem_snapshots_count"`
	StandardVolumesCount             int    `json:"standard_volumes_count"`
	StandardFilesystemsCount         int    `json:"standard_filesystems_count"`
	StandardSnapshotsCount           int    `json:"standard_snapshots_count"`
	StandardEntitiesCount            int    `json:"standard_entities_count"`
	VvolVolumesCount                 int    `json:"vvol_volumes_count"`
	VvolSnapshotsCount               int    `json:"vvol_snapshots_count"`
	VvolEntitiesCount                int    `json:"vvol_entities_count"`
	Owners                           []any  `json:"owners"`
	QosPolicies                      []any  `json:"qos_policies"`
}
type PoolMetadata struct {
	Ready           bool `json:"ready"`
	NumberOfObjects int  `json:"number_of_objects"`
	PageSize        int  `json:"page_size"`
	PagesTotal      int  `json:"pages_total"`
	Page            int  `json:"page"`
}

type Pools struct {
	Result   []Pool       `json:"result"`
	Error    any          `json:"error"`
	Metadata PoolMetadata `json:"metadata"`
}

func lookupPool(allPools *Pools, poolName string) (*Pool, error) {
	for i := 0; i < len(allPools.Result); i++ {
		p := allPools.Result[i]
		if p.Name == poolName {
			return &p, nil
		}
	}
	return nil, fmt.Errorf("pool %s not found", poolName)
}
