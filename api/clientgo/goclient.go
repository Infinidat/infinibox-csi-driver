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
	"bytes"
	"context"
	"fmt"
	"infinibox-csi-driver/common"

	"infinibox-csi-driver/log"

	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
)

var zlog = log.Get() // grab the logger for package use

type KubeClient interface {
	GetSecret(secretName, nameSpace string) (map[string]string, error)
	GetClusterVerion() (string, error)
}

type kubeclient struct {
	client     kubernetes.Interface
	restConfig *rest.Config
}

var clientapi kubeclient

func BuildOffClusterClient(kubeConfigPath string) (kc *kubeclient, err error) {
	zlog.Debug().Msgf("BuildOffClusterClient called.")
	if clientapi.client == nil {
		config, err := clientcmd.BuildConfigFromFlags("", kubeConfigPath)
		if err != nil {
			return nil, err
		}
		clientset, err := kubernetes.NewForConfig(config)
		if err != nil {
			return nil, err
		}

		clientapi = kubeclient{client: clientset, restConfig: config}
	}
	return &clientapi, err
}

// BuildClient
func BuildClient() (kc *kubeclient, err error) {
	zlog.Debug().Msgf("BuildClient called.")
	if clientapi.client == nil {
		config, err := rest.InClusterConfig()
		if err != nil {
			zlog.Error().Msgf("BuildClient Error while getting cluster config: %s", err)
			return nil, err
		}
		// creates the clientset
		clientset, err := kubernetes.NewForConfig(config)
		if err != nil {
			zlog.Error().Msgf("BuildClient Error while creating client: %s", err)
			return nil, err
		}
		clientapi = kubeclient{client: clientset, restConfig: config}
	}
	return &clientapi, err
}

func (kc *kubeclient) GetSecret(secretName, nameSpace string) (map[string]string, error) {
	zlog.Debug().Msgf("get request for secret with namespace %s and secretname %s", nameSpace, secretName)
	secretMap := make(map[string]string)
	secret, err := kc.client.CoreV1().Secrets(nameSpace).Get(context.TODO(), secretName, metav1.GetOptions{})
	if err != nil {
		zlog.Error().Msgf("Error Getting secret with namespace %s and secretname %s Error: %v ", nameSpace, secretName, err)
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

func (kc *kubeclient) GetSecrets(nameSpace string) ([]map[string]string, error) {
	zlog.Debug().Msgf("get request for secrets with namespace %s", nameSpace)
	secretMaps := make([]map[string]string, 0)
	options := metav1.ListOptions{
		LabelSelector: "app=infinidat-csi-driver",
	}
	secrets, err := kc.client.CoreV1().Secrets(nameSpace).List(context.TODO(), options)
	if err != nil {
		zlog.Error().Msgf("Error Getting secrets with namespace %s Error: %v ", nameSpace, err)
		return secretMaps, err
	}
	zlog.Debug().Msgf("got %d secrets for app=infinidat-csi-driver in namespace %s", len(secrets.Items), nameSpace)
	for i := 0; i < len(secrets.Items); i++ {
		m := make(map[string]string)
		secret := secrets.Items[i]
		for key, value := range secret.Data {
			m[key] = string(value)
		}
		for key, value := range secret.StringData {
			m[key] = string(value)
		}
		secretMaps = append(secretMaps, m)
	}
	return secretMaps, nil
}

func (kc *kubeclient) GetPersistantVolumeByName(volumeName string) (*v1.PersistentVolume, error) {
	persistVol, err := kc.client.CoreV1().PersistentVolumes().Get(context.TODO(), volumeName, metav1.GetOptions{})
	if err != nil {
		zlog.Error().Msg(err.Error())
		return nil, err
	}
	return persistVol, nil
}

// Return a PersistentVolumeList listing PVs created by this CSI Driver.
func (kc *kubeclient) GetAllPersistentVolumes() (*v1.PersistentVolumeList, error) {
	zlog.Debug().Msgf("GetAllPersistentVolumes() called")
	persistentVolumes, err := kc.client.CoreV1().PersistentVolumes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		zlog.Error().Msgf("Failed to get all persistent volumes: %s", err.Error())
		return nil, err
	}
	zlog.Trace().Msgf("There are %d persistent volumes in the cluster\n", len(persistentVolumes.Items))

	var infiPersistentVolumeList v1.PersistentVolumeList
	for _, pv := range persistentVolumes.Items {
		persistentVolumeName := pv.GetName()
		provisionedBy := pv.GetAnnotations()["pv.kubernetes.io/provisioned-by"]
		zlog.Trace().Msgf("pv name: %+v\n", persistentVolumeName)
		if provisionedBy == common.SERVICE_NAME {
			zlog.Trace().Msgf("pv %s provisioned by Infinidat CSI driver", persistentVolumeName)
			infiPersistentVolumeList.Items = append(infiPersistentVolumeList.Items, pv)
		} else {
			zlog.Trace().Msgf("pv %s provisioned by foreign CSI driver %s", persistentVolumeName, provisionedBy)
		}
	}
	return &infiPersistentVolumeList, nil
}

func (kc *kubeclient) GetAllStorageClasses() (*storagev1.StorageClassList, error) {
	storageclasses, err := kc.client.StorageV1().StorageClasses().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		zlog.Error().Msg(err.Error())
		return nil, err
	}
	zlog.Debug().Msgf("GetStorageClasses() called")
	zlog.Debug().Msgf("There are %d storageclasses in the cluster\n", len(storageclasses.Items))
	for _, sc := range storageclasses.Items {
		storage_class_name := sc.GetName()
		zlog.Debug().Msgf("storageclass name: %+v\n", storage_class_name)

		pool_name := sc.Parameters["pool_name"]
		zlog.Debug().Msgf("pool name: %s\n", pool_name)
	}
	return storageclasses, nil
}

func (kc *kubeclient) GetNodes() (nodes []v1.Node, err error) {
	nodeList, err := kc.client.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nodes, err
	}
	return nodeList.Items, nil
}
func (kc *kubeclient) GetPV(name string) (pv *v1.PersistentVolume, err error) {
	pv, err = kc.client.CoreV1().PersistentVolumes().Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return pv, nil
}

func (kc *kubeclient) GetPVByVolumeID(volumeID int, protocol string) (pv *v1.PersistentVolume, err error) {
	volumeHandle := fmt.Sprintf("%d$$%s", volumeID, protocol)
	pvList, err := kc.client.CoreV1().PersistentVolumes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for _, v := range pvList.Items {
		if v.Spec.CSI.VolumeHandle == volumeHandle {
			return &v, nil
		}
	}
	return nil, fmt.Errorf("no PV found for volumeHandle %d$$nfs", volumeID)
}

func (kc *kubeclient) GetClusterVerion() (string, error) {
	info, err := kc.client.Discovery().ServerVersion()
	if err != nil {
		zlog.Error().Msg(err.Error())
		return "", err
	}
	return info.GitVersion, nil
}

func (kc *kubeclient) GetPVCs(namespace string) (pvcList *v1.PersistentVolumeClaimList, err error) {
	pvcList, err = kc.client.CoreV1().PersistentVolumeClaims(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		zlog.Error().Msgf("Error Getting PVCs Error: %v ", err)
		return nil, err
	}
	return pvcList, nil
}

func (kc *kubeclient) GetPVC(namespace, name string) (pvc *v1.PersistentVolumeClaim, err error) {
	pvc, err = kc.client.CoreV1().PersistentVolumeClaims(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		zlog.Error().Msgf("Error Getting PVC Error: %v ", err)
		return nil, err
	}
	return pvc, nil
}

// GetPVCAnnotations : Get pvc annotations for a given volumeName
func (kc *kubeclient) GetPVCAnnotations(pvcName, pvcNamespace string) (annotations map[string]string, err error) {
	zlog.Trace().Msgf("GetPVCAnnotations called with pvcName %s namespace %s", pvcName, pvcNamespace)

	pvc, err := kc.GetPVC(pvcNamespace, pvcName)
	if err != nil {
		zlog.Error().Msgf("error getting PVC %s", err.Error())
		return annotations, err
	}
	return pvc.Annotations, nil
}

// get the CSI Driver pods that would be created by the driver's Daemonset
func (kc *kubeclient) GetRunningDriverNodePods(namespace string) (pods []v1.Pod, err error) {
	options := metav1.ListOptions{
		LabelSelector: "part-of=infiniboxcsidriver-node",
	}
	podList, err := kc.client.CoreV1().Pods(namespace).List(context.TODO(), options)
	if err != nil {
		zlog.Error().Msgf("Error Getting Driver Node Pods Error: %v ", err)
		return pods, err
	}
	for _, pod := range podList.Items {
		if pod.Status.Phase == "Running" {
			pods = append(pods, pod)
		}
	}
	if len(pods) == 0 {
		e := fmt.Errorf("no CSI driver node pods are in running status")
		zlog.Error().Msg(e.Error())
		return pods, e
	}

	return pods, nil
}

// ExecCmdInPod - exec command on specific pod and wait the command's output.
func (kc *kubeclient) ExecCmdInPod(podName string, nameSpace string, command string, containerName string) (string, string, error) {

	stdOut := &bytes.Buffer{}
	stdErr := &bytes.Buffer{}

	cmd := []string{
		"/bin/sh",
		"-c",
		command,
	}
	req := kc.client.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(nameSpace).
		SubResource("exec")
	// need container name?

	req.VersionedParams(
		&v1.PodExecOptions{
			Container: containerName,
			Command:   cmd,
			Stdin:     false,
			Stdout:    true,
			Stderr:    true,
			TTY:       true,
		},
		scheme.ParameterCodec,
	)

	//fmt.Printf("execCmdInPod - Running command: %s\n", command)

	exec, err := remotecommand.NewSPDYExecutor(kc.restConfig, "POST", req.URL())
	if err != nil {
		return stdOut.String(), stdErr.String(), err
	}
	err = exec.StreamWithContext(context.Background(),
		remotecommand.StreamOptions{
			Stdin:  nil,
			Stdout: stdOut,
			Stderr: stdErr,
		})

	return stdOut.String(), stdErr.String(), err
}
