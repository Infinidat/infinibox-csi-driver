package metric

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
)

var (
	MetricPVTotalSizeGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: METRIC_PV_TOTAL_SIZE,
		Help: "The persistent volume total size",
	}, []string{METRIC_PV_NAME, METRIC_PV_STORAGE_CLASS, METRIC_PV_PROVISION_TYPE, METRIC_PV_SSD_ENABLED, METRIC_PV_NETWORK_SPACE, METRIC_PV_STORAGE_PROTOCOL})
)

func RecordPVMetrics(config *MetricsConfig) {
	klog.V(4).Infof("pv metrics recording...")
	go func() {
		for {
			pvInfo, err := getPVInfo()
			if err != nil {
				klog.Error(err)
			} else {
				for i := 0; i < len(*pvInfo); i++ {
					p := (*pvInfo)[i]
					labels := prometheus.Labels{
						METRIC_PV_NAME:             p.PVol.Name,
						METRIC_PV_STORAGE_CLASS:    p.SClass.Name,
						METRIC_PV_PROVISION_TYPE:   p.SClass.Parameters["provision_type"],
						METRIC_PV_SSD_ENABLED:      p.SClass.Parameters["ssd_enabled"],
						METRIC_PV_NETWORK_SPACE:    p.SClass.Parameters["network_space"],
						METRIC_PV_STORAGE_PROTOCOL: p.SClass.Parameters["storage_protocol"],
					}
					MetricPVTotalSizeGauge.With(labels).Set(float64(p.PVol.Spec.Capacity.Storage().Value()))
				}
			}

			time.Sleep(config.GetDuration(METRIC_PV_METRICS))
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
		klog.Error(err)
		return nil, err
	}

	// create the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		klog.Error(err)
		return nil, err
	}

	pVols, err := clientset.CoreV1().PersistentVolumes().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		klog.Error(err)
		return nil, err
	}
	for i := 0; i < len(pVols.Items); i++ {
		pvItem := pVols.Items[i]
		if pvItem.Annotations["pv.kubernetes.io/provisioned-by"] == "infinibox-csi-driver" {
			klog.V(4).Infof("pv metrics: pv %s sc %s found\n", pvItem.Name, pvItem.Spec.StorageClassName)
			sc, err := clientset.StorageV1().StorageClasses().Get(context.Background(), pvItem.Spec.StorageClassName, metav1.GetOptions{})
			if err != nil {
				klog.Error(err)
				return nil, err
			}
			/**
			fmt.Printf("sc details name: %s \n", sc.Name)
			fmt.Printf("provision_type %s \n", sc.Parameters["provision_type"])
			fmt.Printf("ssd_enabled %s\n", sc.Parameters["ssd_enabled"])
			fmt.Printf("network_space %s\n", sc.Parameters["network_space"])
			fmt.Printf("storage_protocol %s\n", sc.Parameters["storage_protocol"])
			fmt.Println("---------------------")
			*/
			info := PVInfo{
				PVol:   pvItem,
				SClass: *sc,
			}
			pvInfo = append(pvInfo, info)
		}
	}

	return &pvInfo, nil
}
