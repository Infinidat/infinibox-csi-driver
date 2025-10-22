package metric

import (
	"context"
	"infinibox-csi-driver/common"
	"time"

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
		Name: METRIC_PV_TOTAL_SIZE,
		Help: "The persistent volume total size",
	}, []string{METRIC_PV_NAME, METRIC_PV_STORAGE_CLASS, METRIC_PV_PROVISION_TYPE, METRIC_PV_SSD_ENABLED, METRIC_PV_NETWORK_SPACE, METRIC_PV_STORAGE_PROTOCOL})
)

func RecordPVMetrics(config *MetricsConfig) {
	zlog.Trace().Msgf("pv metrics recording...")
	go func() {
		for {
			time.Sleep(config.GetDuration(METRIC_PV_METRICS))

			pvInfo, err := getPVInfo()
			if err != nil {
				zlog.Err(err)
				continue
			}

			zlog.Trace().Msgf("creating metrics for %d PVs", len(*pvInfo))
			for _, persistentVolume := range *pvInfo {
				labels := prometheus.Labels{
					METRIC_PV_NAME:             persistentVolume.PVol.Name,
					METRIC_PV_STORAGE_CLASS:    persistentVolume.SClass.Name,
					METRIC_PV_PROVISION_TYPE:   persistentVolume.SClass.Parameters[common.StorageClassProvisionType],
					METRIC_PV_SSD_ENABLED:      persistentVolume.SClass.Parameters[common.StorageClassSSDEnabled],
					METRIC_PV_NETWORK_SPACE:    persistentVolume.SClass.Parameters[common.StorageClassNetworkSpace],
					METRIC_PV_STORAGE_PROTOCOL: persistentVolume.SClass.Parameters[common.StorageClassStorageProtocol],
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

func getPVInfo() (*[]PVInfo, error) {
	pvInfo := make([]PVInfo, 0)

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

	persistentVolumes, err := clientset.CoreV1().PersistentVolumes().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		zlog.Err(err)
		return nil, err
	}
	for _, persistentVolume := range persistentVolumes.Items {
		if persistentVolume.Annotations["pv.kubernetes.io/provisioned-by"] == "infinibox-csi-driver" {
			zlog.Trace().Msgf("pv metrics: pv %s sc %s found", persistentVolume.Name, persistentVolume.Spec.StorageClassName)
			storageClass, err := clientset.StorageV1().StorageClasses().Get(context.Background(), persistentVolume.Spec.StorageClassName, metav1.GetOptions{})
			if err != nil {
				zlog.Error().Msgf("error getting StorageClass %s error %s", persistentVolume.Spec.StorageClassName, err.Error())
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
					PVol:   persistentVolume,
					SClass: *storageClass,
				}
				pvInfo = append(pvInfo, info)
			}
		}
	}

	return &pvInfo, nil
}
