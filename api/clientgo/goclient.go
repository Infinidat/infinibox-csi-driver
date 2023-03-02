/*
Copyright 2022 Infinidat
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package clientgo

import (
	"context"
	"infinibox-csi-driver/common"

	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
)

type KubeClient interface {
	GetSecret(secretName, nameSpace string) (map[string]string, error)
	GetClusterVerion() (string, error)
}

type kubeclient struct {
	client kubernetes.Interface
}

var clientapi kubeclient

// BuildClient
func BuildClient() (kc *kubeclient, err error) {
	klog.V(4).Infof("BuildClient called.")
	if clientapi.client == nil {
		config, err := rest.InClusterConfig()
		if err != nil {
			klog.Errorf("BuildClient Error while getting cluster config: %s", err)
			return nil, err
		}
		// creates the clientset
		clientset, err := kubernetes.NewForConfig(config)
		if err != nil {
			klog.Errorf("BuildClient Error while creating client: %s", err)
			return nil, err
		}
		clientapi = kubeclient{clientset}
	}
	return &clientapi, err
}

func (kc *kubeclient) GetSecret(secretName, nameSpace string) (map[string]string, error) {
	klog.V(4).Infof("get request for secret with namespace %s and secretname %s", nameSpace, secretName)
	secretMap := make(map[string]string)
	secret, err := kc.client.CoreV1().Secrets(nameSpace).Get(context.TODO(), secretName, metav1.GetOptions{})
	if err != nil {
		klog.Errorf("Error Getting secret with namespace %s and secretname %s Error: %v ", nameSpace, secretName, err)
		return secretMap, err
	}
	for key, value := range secret.Data {
		secretMap[key] = string(value)
	}
	for key, value := range secret.StringData {
		secretMap[key] = string(value)
	}
	return secretMap, nil
}

func (kc *kubeclient) GetPersistantVolumeByName(volumeName string) (*v1.PersistentVolume, error) {
	persistVol, err := kc.client.CoreV1().PersistentVolumes().Get(context.TODO(), volumeName, metav1.GetOptions{})
	if err != nil {
		klog.Errorf(err.Error())
		return nil, err
	}
	return persistVol, nil
}

// Return a PersistentVolumeList listing PVs created by this CSI Driver.
func (kc *kubeclient) GetAllPersistentVolumes() (*v1.PersistentVolumeList, error) {
	persistentVolumes, err := kc.client.CoreV1().PersistentVolumes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		klog.Errorf("Failed to get all persistent volumes: %s", err.Error())
		return nil, err
	}
	klog.V(4).Infof("GetAllPersistentVolumes() called")
	klog.V(4).Infof("There are %d persistent volumes in the cluster\n", len(persistentVolumes.Items))

	var infiPersistentVolumeList v1.PersistentVolumeList
	for _, pv := range persistentVolumes.Items {
		// pvJson, err := json.MarshalIndent(pv, "", "\t")
		// if err != nil {
		// 	log.Fatalf(err.Error())
		// }
		// fmt.Printf("persistenvolume: %s\n", string(scJson))

		persistentVolumeName := pv.ObjectMeta.GetName()
		provisionedBy := pv.ObjectMeta.GetAnnotations()["pv.kubernetes.io/provisioned-by"]
		klog.V(4).Infof("pv name: %+v\n", persistentVolumeName)
		if provisionedBy == common.SERVICE_NAME {
			klog.V(4).Infof("pv %s provisioned by Infinidat CSI driver", persistentVolumeName)
			infiPersistentVolumeList.Items = append(infiPersistentVolumeList.Items, pv)
		} else {
			klog.V(4).Infof("pv %s provisioned by foreign CSI driver %s", persistentVolumeName, provisionedBy)
		}
	}
	return &infiPersistentVolumeList, nil
}

func (kc *kubeclient) GetAllStorageClasses() (*storagev1.StorageClassList, error) {
	storageclasses, err := kc.client.StorageV1().StorageClasses().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		klog.Errorf(err.Error())
		return nil, err
	}
	klog.V(4).Infof("GetStorageClasses() called")
	klog.V(4).Infof("There are %d storageclasses in the cluster\n", len(storageclasses.Items))
	for _, sc := range storageclasses.Items {
		// scJson, err := json.MarshalIndent(sc, "", "\t")
		// if err != nil {
		// 	log.Fatalf(err.Error())
		// }
		// fmt.Printf("storageclass: %s\n", string(scJson))

		storage_class_name := sc.ObjectMeta.GetName()
		klog.V(4).Infof("storageclass name: %+v\n", storage_class_name)

		pool_name := sc.Parameters["pool_name"]
		klog.V(4).Infof("pool name: %s\n", pool_name)
	}
	return storageclasses, nil
}

func (kc *kubeclient) GetNodeIdByNodeName(nodeName string) (InternalIp string, err error) {
	node, err := kc.client.CoreV1().Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	nodeip := ""
	for _, addr := range node.Status.Addresses {
		if addr.Type == v1.NodeInternalIP {
			nodeip = addr.Address
		}
	}
	return nodeip, err
}

func (kc *kubeclient) GetClusterVerion() (string, error) {
	info, err := kc.client.Discovery().ServerVersion()
	if err != nil {
		klog.Errorf(err.Error())
		return "", err
	}
	return info.GitVersion, nil
}
