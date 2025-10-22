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

	"github.com/infinidat/infinibox-csi-driver/common"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	storagev1 "k8s.io/api/storage/v1"
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
	zlog.Debug().Msgf("pool metrics recording...")
	go func() {
		for {
			time.Sleep(config.GetDuration(METRIC_POOL_METRICS))
			for _, ibox := range config.Ibox {
				zlog.Trace().Msgf("pool metrics: creating collectors for %s...", ibox.IboxHostname)
				poolInfoList, err := getPoolInfo(ibox)
				if err != nil {
					zlog.Err(err)
					continue
				}

				for _, poolInfo := range poolInfoList {
					labels := prometheus.Labels{
						METRIC_POOL_NAME:             poolInfo.storageClass.Parameters[common.StorageClassPoolName],
						METRIC_POOL_PROVISION_TYPE:   poolInfo.storageClass.Parameters[common.StorageClassProvisionType],
						METRIC_POOL_SSD_ENABLED:      poolInfo.storageClass.Parameters[common.StorageClassSSDEnabled],
						METRIC_POOL_NETWORK_SPACE:    poolInfo.storageClass.Parameters[common.StorageClassNetworkSpace],
						METRIC_POOL_STORAGE_PROTOCOL: poolInfo.storageClass.Parameters[common.StorageClassStorageProtocol],
					}
					MetricPoolAvailableCapGauge.With(labels).Set(float64(poolInfo.pool.PhysicalCapacity))  // pool - physical_capacity
					MetricPoolUsedCapGauge.With(labels).Set(float64(poolInfo.pool.AllocatedPhysicalSpace)) // pool -  allocated_physical_space
					pct := (poolInfo.pool.AllocatedPhysicalSpace / poolInfo.pool.PhysicalCapacity) * 100.00
					MetricPoolPctUtilizedGauge.With(labels).Set(float64(pct)) // pool - (allocated_physical_space / physical_capacity) * 100.00
				}
			}
		}
	}()
}

type PoolInfo struct {
	storageClass storagev1.StorageClass
	pool         Pool
}

func getPoolInfo(ibox IboxCredentials) ([]PoolInfo, error) {
	poolInfo := make([]PoolInfo, 0)
	storageClasses, err := getStorageClasses()
	if err != nil {
		zlog.Err(err)
		return poolInfo, err
	}
	allPools, err := getPools(ibox)
	if err != nil {
		zlog.Err(err)
		return poolInfo, err
	}
	for _, storageClass := range *storageClasses {
		pool, err := lookupPool(allPools, storageClass.Parameters[common.StorageClassPoolName])
		if err != nil {
			zlog.Error().Msgf("pool_name not found from storage classes %s", storageClass.Parameters["pool_name"])
		} else {
			pi := PoolInfo{
				storageClass: storageClass,
				pool:         *pool,
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
		zlog.Err(err)
		return nil, err
	}

	// create the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		zlog.Err(err)
		return nil, err
	}

	storageClasses, err := clientset.StorageV1().StorageClasses().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		zlog.Err(err)
		return nil, err
	}

	ourStorageClasses := make([]storagev1.StorageClass, 0)
	for _, storageClass := range storageClasses.Items {
		if storageClass.Provisioner == "infinibox-csi-driver" {
			// this is a storageclass used by our driver
			zlog.Debug().Msgf("storageclass name %s", storageClass.Name)
			zlog.Debug().Msgf("storage_protocol %s", storageClass.Parameters[common.StorageClassStorageProtocol])
			zlog.Debug().Msgf("network_space %s", storageClass.Parameters[common.StorageClassNetworkSpace])
			zlog.Debug().Msgf("pool_name %s", storageClass.Parameters[common.StorageClassPoolName])
			zlog.Debug().Msgf("provision_type %s", storageClass.Parameters[common.StorageClassProvisionType])
			zlog.Debug().Msgf("ssd_enabled %s", storageClass.Parameters[common.StorageClassSSDEnabled])
			zlog.Debug().Msgf("--------------------------------------")
			ourStorageClasses = append(ourStorageClasses, storageClass)
		}
	}

	return &ourStorageClasses, nil
}

func getPools(ibox IboxCredentials) (*Pools, error) {
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

	req, err := http.NewRequest(http.MethodGet, "https://"+ibox.IboxHostname+"/api/rest/pools", http.NoBody)
	if err != nil {
		zlog.Err(err)
		return nil, err
	}

	req.SetBasicAuth(ibox.IboxUsername, ibox.IboxPassword)
	req.Header.Set("Content-Type", "application/json")

	res, err := client.Do(req)
	if err != nil {
		zlog.Err(err)
		return nil, err
	}

	defer func() {
		if err := res.Body.Close(); err != nil {
			zlog.Error().Msgf("error in Close() %s", err.Error())
		}
	}()

	responseData, err := io.ReadAll(res.Body)
	if err != nil {
		zlog.Err(err)
		return nil, err
	}
	// fmt.Println(string(responseData))

	pools := Pools{}
	err = json.Unmarshal(responseData, &pools)
	if err != nil {
		log.Fatal(err)
	}
	// fmt.Printf("API Result %+v\n", r.Result)
	return &pools, nil
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
	for i := range allPools.Result {
		p := allPools.Result[i]
		if p.Name == poolName {
			return &p, nil
		}
	}
	return nil, fmt.Errorf("pool %s not found", poolName)
}
