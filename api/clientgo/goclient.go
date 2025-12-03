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
	"log/slog"
	"maps"

	"github.com/infinidat/infinibox-csi-driver/common"

	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
)

type KubeClient interface {
	GetSecret(secretName, nameSpace string) (map[string]string, error)
	GetClusterVerion() (string, error)
}

type kubeclient struct {
	client     kubernetes.Interface
	restConfig *rest.Config
}

var clientAPI kubeclient

func BuildOffClusterClient(kubeConfigPath string) (kubeClient *kubeclient, err error) {
	slog.Debug("BuildOffClusterClient called.")
	if clientAPI.client == nil {
		config, err := clientcmd.BuildConfigFromFlags("", kubeConfigPath)
		if err != nil {
			return nil, err
		}
		clientset, err := kubernetes.NewForConfig(config)
		if err != nil {
			return nil, err
		}

		clientAPI = kubeclient{client: clientset, restConfig: config}
	}
	return &clientAPI, err
}

// BuildClient
func BuildClient() (kubeClient *kubeclient, err error) {
	slog.Debug("BuildClient called.")
	if clientAPI.client == nil {
		config, err := rest.InClusterConfig()
		if err != nil {
			slog.Error("error", "BuildClient Error while getting cluster config", err)
			return nil, err
		}
		// creates the clientset
		clientset, err := kubernetes.NewForConfig(config)
		if err != nil {
			slog.Error("error", "BuildClient Error while creating client", err)
			return nil, err
		}
		clientAPI = kubeclient{client: clientset, restConfig: config}
	}
	return &clientAPI, err
}

func (kc *kubeclient) GetSecret(ctx context.Context, secretName, namespace string) (map[string]string, error) {
	slog.Debug("get request for secret", "namespace", namespace, "secretname", secretName)
	secretMap := make(map[string]string)
	secret, err := kc.client.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		slog.Error("Error Getting secret", "namespace", namespace, "secretname", secretName, "Error", err.Error())
		return secretMap, err
	}
	for key, value := range secret.Data {
		secretMap[key] = string(value)
	}
	maps.Copy(secretMap, secret.StringData)
	return secretMap, nil
}

func (kc *kubeclient) GetSecrets(ctx context.Context, namespace string) ([]map[string]string, error) {
	slog.Debug("get request for secrets", "namespace", namespace)
	secretMaps := make([]map[string]string, 0)
	options := metav1.ListOptions{
		LabelSelector: "app=infinidat-csi-driver",
	}
	secrets, err := kc.client.CoreV1().Secrets(namespace).List(ctx, options)
	if err != nil {
		slog.Error("Error Getting secrets", "namespace", namespace, "error", err)
		return secretMaps, err
	}
	slog.Debug("got secrets for app=infinidat-csi-driver", "item count", len(secrets.Items), "namespace", namespace)
	for _, secret := range secrets.Items {
		newMap := make(map[string]string)
		for key, value := range secret.Data {
			newMap[key] = string(value)
		}
		maps.Copy(newMap, secret.StringData)
		secretMaps = append(secretMaps, newMap)
	}
	return secretMaps, nil
}

func (kc *kubeclient) GetPersistantVolumeByName(ctx context.Context, volumeName string) (*v1.PersistentVolume, error) {
	persistVol, err := kc.client.CoreV1().PersistentVolumes().Get(ctx, volumeName, metav1.GetOptions{})
	if err != nil {
		slog.Error(err.Error())
		return nil, err
	}
	return persistVol, nil
}

// Return a PersistentVolumeList listing PVs created by this CSI Driver.
func (kc *kubeclient) GetAllPersistentVolumes(ctx context.Context) (*v1.PersistentVolumeList, error) {
	slog.Debug("GetAllPersistentVolumes() called")
	persistentVolumes, err := kc.client.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{})
	if err != nil {
		slog.Error("Failed to get all persistent volumes", "error", err.Error())
		return nil, err
	}
	slog.Debug("There are persistent volumes in the cluster", "count", len(persistentVolumes.Items))

	var infiPersistentVolumeList v1.PersistentVolumeList
	for _, persistentVolume := range persistentVolumes.Items {
		persistentVolumeName := persistentVolume.GetName()
		provisionedBy := persistentVolume.GetAnnotations()["pv.kubernetes.io/provisioned-by"]
		slog.Log(ctx, common.LevelTrace, "info", "pv name", persistentVolumeName)
		if provisionedBy == common.ServiceName {
			slog.Log(ctx, common.LevelTrace, "info", "pv provisioned by Infinidat CSI driver", persistentVolumeName)
			infiPersistentVolumeList.Items = append(infiPersistentVolumeList.Items, persistentVolume)
		} else {
			slog.Log(ctx, common.LevelTrace, "pv provisioned", "pvname", persistentVolumeName, "by foreign CSI driver", provisionedBy)
		}
	}
	return &infiPersistentVolumeList, nil
}

func (kc *kubeclient) GetAllStorageClasses(ctx context.Context) (*storagev1.StorageClassList, error) {
	storageclasses, err := kc.client.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
	if err != nil {
		slog.Error(err.Error())
		return nil, err
	}
	slog.Debug("GetStorageClasses", "storageclasses in the cluster", len(storageclasses.Items))
	for _, sc := range storageclasses.Items {
		storageClassName := sc.GetName()
		slog.Debug("storageclass", "name", storageClassName)

		poolName := sc.Parameters["pool_name"]
		slog.Debug("pool", "name", poolName)
	}
	return storageclasses, nil
}

func (kc *kubeclient) GetNodes(ctx context.Context) (nodes []v1.Node, err error) {
	nodeList, err := kc.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nodes, err
	}
	return nodeList.Items, nil
}
func (kc *kubeclient) GetPV(ctx context.Context, name string) (pv *v1.PersistentVolume, err error) {
	pv, err = kc.client.CoreV1().PersistentVolumes().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return pv, nil
}

func (kc *kubeclient) GetPVByVolumeID(ctx context.Context, volumeID int, protocol string) (*v1.PersistentVolume, error) {
	volumeHandle := fmt.Sprintf("%d$$%s", volumeID, protocol)
	pvList, err := kc.client.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{})
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
		slog.Error(err.Error())
		return "", err
	}
	return info.GitVersion, nil
}

func (kc *kubeclient) GetPVCs(ctx context.Context, namespace string) (pvcList *v1.PersistentVolumeClaimList, err error) {
	pvcList, err = kc.client.CoreV1().PersistentVolumeClaims(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		slog.Error("Error Getting PVCs", "Error", err)
		return nil, err
	}
	return pvcList, nil
}

func (kc *kubeclient) GetPVC(ctx context.Context, namespace, name string) (pvc *v1.PersistentVolumeClaim, err error) {
	pvc, err = kc.client.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		slog.Error("Error Getting PVC", "Error", err)
		return nil, err
	}
	return pvc, nil
}

// GetPVCAnnotations : Get pvc annotations for a given volumeName
func (kc *kubeclient) GetPVCAnnotations(ctx context.Context, pvcName, pvcNamespace string) (annotations map[string]string, err error) {
	slog.Log(ctx, common.LevelTrace, "GetPVCAnnotations called", "pvcName", pvcName, "namespace", pvcNamespace)

	var pvc *v1.PersistentVolumeClaim
	pvc, err = kc.GetPVC(ctx, pvcNamespace, pvcName)
	if err != nil {
		slog.Error("error getting PVC", "error", err.Error())
		return annotations, err
	}
	return pvc.Annotations, nil
}

// get the CSI Driver pods that would be created by the driver's Daemonset
func (kc *kubeclient) GetRunningDriverNodePods(ctx context.Context, namespace string) (pods []v1.Pod, err error) {
	options := metav1.ListOptions{
		LabelSelector: "part-of=infiniboxcsidriver-node",
	}
	podList, err := kc.client.CoreV1().Pods(namespace).List(ctx, options)
	if err != nil {
		slog.Error("Error Getting Driver Node Pods", "Error", err)
		return pods, err
	}
	for _, pod := range podList.Items {
		if pod.Status.Phase == "Running" {
			pods = append(pods, pod)
		}
	}
	if len(pods) == 0 {
		e := fmt.Errorf("no CSI driver node pods are in running status")
		slog.Error(e.Error())
		return pods, e
	}

	return pods, nil
}

// ExecCmdInPod - exec command on specific pod and wait the command's output.
func (kc *kubeclient) ExecCmdInPod(ctx context.Context, podName, nameSpace, command, containerName string) (string, string, error) {
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

	// fmt.Printf("execCmdInPod - Running command: %s\n", command)

	exec, err := remotecommand.NewSPDYExecutor(kc.restConfig, "POST", req.URL())
	if err != nil {
		return stdOut.String(), stdErr.String(), err
	}
	err = exec.StreamWithContext(ctx,
		remotecommand.StreamOptions{
			Stdin:  nil,
			Stdout: stdOut,
			Stderr: stdErr,
		})

	return stdOut.String(), stdErr.String(), err
}
