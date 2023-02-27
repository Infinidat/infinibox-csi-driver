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

	v1 "k8s.io/api/core/v1"
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

func (kc *kubeclient) GetNodeIpsByMountedVolume(volumeName string) ([]string, error) {
	pv, err := kc.GetPersistantVolumeByName(volumeName)
	if err != nil {
		return nil, err
	}
	nameSpace := pv.Spec.ClaimRef.Namespace
	pvcName := pv.Spec.ClaimRef.Name
	pods, err := kc.client.CoreV1().Pods(nameSpace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		klog.Errorf("Error while attempting to list pods: %s", err)
		return nil, err
	}
	nodeIps := []string{}
	for _, p := range pods.Items {
		for _, volume := range p.Spec.Volumes {
			claim := volume.VolumeSource.PersistentVolumeClaim
			if claim != nil && claim.ClaimName == pvcName {
				nodeName, err := kc.GetNodeIdByNodeName(p.Spec.NodeName)
				if err != nil {
					return nil, err
				}
				nodeIps = append(nodeIps, nodeName)
			}
		}
	}
	return nodeIps, nil
}

func (kc *kubeclient) GetPersistantVolumeByName(volumeName string) (*v1.PersistentVolume, error) {
	persistVol, err := kc.client.CoreV1().PersistentVolumes().Get(context.TODO(), volumeName, metav1.GetOptions{})
	if err != nil {
		klog.Errorf(err.Error())
		return nil, err
	}
	return persistVol, nil
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
