package metric

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
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
	PoolAvailableCap = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: MetricPoolAvailableCap,
		Help: "The pool available capacity",
	}, []string{MetricPoolName, MetricPoolProvisionType, MetricPoolSSDEnabled, MetricPoolNetworkSpace, MetricPoolStorageProtocol})
	PoolUsedCap = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: MetricPoolUsedCap,
		Help: "The pool used capacity",
	}, []string{MetricPoolName, MetricPoolProvisionType, MetricPoolSSDEnabled, MetricPoolNetworkSpace, MetricPoolStorageProtocol})
	PoolPctUtilized = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: MetricPoolPctUtilized,
		Help: "The pool percentage of capacity utilized",
	}, []string{MetricPoolName, MetricPoolProvisionType, MetricPoolSSDEnabled, MetricPoolNetworkSpace, MetricPoolStorageProtocol})
)

func RecordPoolMetrics(ctx context.Context, config *MetricsConfig) {
	slog.Debug("pool metrics recording...")
	go func() {
		for {
			time.Sleep(config.GetDuration(PoolMetrics))
			for _, ibox := range config.Ibox {
				slog.Log(ctx, common.LevelTrace, "pool metrics: creating collectors for", "ibox", ibox.IboxHostname)
				poolInfoList, err := getPoolInfo(ctx, ibox)
				if err != nil {
					slog.Error(err.Error())
					continue
				}

				for _, poolInfo := range poolInfoList {
					labels := prometheus.Labels{
						MetricPoolName:            poolInfo.storageClass.Parameters[common.StorageClassPoolName],
						MetricPoolProvisionType:   poolInfo.storageClass.Parameters[common.StorageClassProvisionType],
						MetricPoolSSDEnabled:      poolInfo.storageClass.Parameters[common.StorageClassSSDEnabled],
						MetricPoolNetworkSpace:    poolInfo.storageClass.Parameters[common.StorageClassNetworkSpace],
						MetricPoolStorageProtocol: poolInfo.storageClass.Parameters[common.StorageClassStorageProtocol],
					}
					PoolAvailableCap.With(labels).Set(float64(poolInfo.pool.PhysicalCapacity))  // pool - physical_capacity
					PoolUsedCap.With(labels).Set(float64(poolInfo.pool.AllocatedPhysicalSpace)) // pool -  allocated_physical_space
					pct := (poolInfo.pool.AllocatedPhysicalSpace / poolInfo.pool.PhysicalCapacity) * 100.00
					PoolPctUtilized.With(labels).Set(float64(pct)) // pool - (allocated_physical_space / physical_capacity) * 100.00
				}
			}
		}
	}()
}

type PoolInfo struct {
	storageClass storagev1.StorageClass
	pool         Pool
}

func getPoolInfo(ctx context.Context, ibox IboxCredentials) ([]PoolInfo, error) {
	poolInfo := make([]PoolInfo, 0)
	storageClasses, err := getStorageClasses(ctx)
	if err != nil {
		slog.Error(err.Error())
		return poolInfo, err
	}
	allPools, err := getPools(ctx, ibox)
	if err != nil {
		slog.Error(err.Error())
		return poolInfo, err
	}
	for _, storageClass := range *storageClasses {
		pool, err := lookupPool(allPools, storageClass.Parameters[common.StorageClassPoolName])
		if err != nil {
			slog.Error("pool_name not found from storage classes", "sc", storageClass.Parameters["pool_name"])
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
func getStorageClasses(ctx context.Context) (*[]storagev1.StorageClass, error) {
	// creates the in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		slog.Error(err.Error())
		return nil, err
	}

	// create the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		slog.Error(err.Error())
		return nil, err
	}

	storageClasses, err := clientset.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
	if err != nil {
		slog.Error(err.Error())
		return nil, err
	}

	ourStorageClasses := make([]storagev1.StorageClass, 0)
	for _, storageClass := range storageClasses.Items {
		if storageClass.Provisioner == "infinibox-csi-driver" {
			// this is a storageclass used by our driver
			slog.Debug("storageclass name", "name", storageClass.Name)
			slog.Debug("storage_protocol", "protocol", storageClass.Parameters[common.StorageClassStorageProtocol])
			slog.Debug("network_space", "networkspace", storageClass.Parameters[common.StorageClassNetworkSpace])
			slog.Debug("pool_name", "pool", storageClass.Parameters[common.StorageClassPoolName])
			slog.Debug("provision_type", "provision type", storageClass.Parameters[common.StorageClassProvisionType])
			slog.Debug("ssd_enabled", "ssdenabled", storageClass.Parameters[common.StorageClassSSDEnabled])
			slog.Debug("--------------------------------------")
			ourStorageClasses = append(ourStorageClasses, storageClass)
		}
	}

	return &ourStorageClasses, nil
}

func getPools(ctx context.Context, ibox IboxCredentials) (*Pools, error) {
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

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+ibox.IboxHostname+"/api/rest/pools", http.NoBody)
	if err != nil {
		slog.Error(err.Error())
		return nil, err
	}

	req.SetBasicAuth(ibox.IboxUsername, ibox.IboxPassword)
	req.Header.Set("Content-Type", "application/json")

	res, err := client.Do(req)
	if err != nil {
		slog.Error(err.Error())
		return nil, err
	}

	defer func() {
		if err := res.Body.Close(); err != nil {
			slog.Error("error in Close()", "error", err.Error())
		}
	}()

	responseData, err := io.ReadAll(res.Body)
	if err != nil {
		slog.Error(err.Error())
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
