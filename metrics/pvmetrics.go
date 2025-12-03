package metric

import (
	"context"
	"log/slog"
	"time"

	"github.com/infinidat/infinibox-csi-driver/common"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var (
	MetricPVTotalSizeGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: MetricPVTotalSize,
		Help: "The persistent volume total size",
	}, []string{
		MetricPVName,
		MetricPVStorageClass,
		MetricPVProvisionType,
		MetricPVSSDEnabled,
		MetricPVNetworkSpace,
		MetricPVStorageProtocol})
)

func RecordPVMetrics(ctx context.Context, config *MetricsConfig) {
	slog.Log(ctx, common.LevelTrace, "pv metrics recording...")
	go func() {
		for {
			time.Sleep(config.GetDuration(PVMetrics))

			pvInfo, err := getPVInfo(ctx)
			if err != nil {
				slog.Error(err.Error())
				continue
			}

			slog.Log(ctx, common.LevelTrace, "creating metrics for PVs", "cnt", len(*pvInfo))
			for _, persistentVolume := range *pvInfo {
				labels := prometheus.Labels{
					MetricPVName:            persistentVolume.PVol.Name,
					MetricPVStorageClass:    persistentVolume.SClass.Name,
					MetricPVProvisionType:   persistentVolume.SClass.Parameters[common.StorageClassProvisionType],
					MetricPVSSDEnabled:      persistentVolume.SClass.Parameters[common.StorageClassSSDEnabled],
					MetricPVNetworkSpace:    persistentVolume.SClass.Parameters[common.StorageClassNetworkSpace],
					MetricPVStorageProtocol: persistentVolume.SClass.Parameters[common.StorageClassStorageProtocol],
				}
				MetricPVTotalSizeGauge.With(labels).Set(float64(persistentVolume.PVol.Spec.Capacity.Storage().Value()))
			}
		}
	}()
}

type PVInfo struct {
	PVol   corev1.PersistentVolume
	SClass storagev1.StorageClass
}

func getPVInfo(ctx context.Context) (*[]PVInfo, error) {
	pvInfo := make([]PVInfo, 0)

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

	persistentVolumes, err := clientset.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{})
	if err != nil {
		slog.Error(err.Error())
		return nil, err
	}
	for _, pv := range persistentVolumes.Items {
		if pv.Annotations["pv.kubernetes.io/provisioned-by"] == "infinibox-csi-driver" {
			slog.Log(ctx, common.LevelTrace, "pv metrics", "pv", pv.Name, "sc", pv.Spec.StorageClassName)
			storageClass, err := clientset.StorageV1().StorageClasses().Get(ctx, pv.Spec.StorageClassName, metav1.GetOptions{})
			if err != nil {
				slog.Error("error getting StorageClass", "sc", pv.Spec.StorageClassName, "error", err.Error())
			} else {
				/**
				fmt.Printf("sc details name: %s \n", sc.Name)
				fmt.Printf("provision_type %s \n", sc.Parameters["provision_type"])
				fmt.Printf("ssd_enabled %s\n", sc.Parameters["ssd_enabled"])
				fmt.Printf("network_space %s\n", sc.Parameters["network_space"])
				fmt.Printf("storage_protocol %s\n", sc.Parameters["storage_protocol"])
				fmt.Println("---------------------")
				*/
				info := PVInfo{
					PVol:   pv,
					SClass: *storageClass,
				}
				pvInfo = append(pvInfo, info)
			}
		}
	}

	return &pvInfo, nil
}
