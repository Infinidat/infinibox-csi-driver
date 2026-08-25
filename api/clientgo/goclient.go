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
	"strings"

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

/**
type KubeClient interface {
	GetSecret(secretName, nameSpace string) (map[string]string, error)
	GetClusterVerion() (string, error)
}
*/

type KubeClient struct {
	KubeClientInterface kubernetes.Interface
	KubeRestConfig      *rest.Config
}

var clientAPI KubeClient

func BuildOffClusterClient(kubeConfigPath string) (kubeClient *KubeClient, err error) {
	if clientAPI.KubeClientInterface == nil {
		config, err := clientcmd.BuildConfigFromFlags("", kubeConfigPath)
		if err != nil {
			return nil, err
		}
		clientset, err := kubernetes.NewForConfig(config)
		if err != nil {
			return nil, err
		}

		clientAPI = KubeClient{KubeClientInterface: clientset, KubeRestConfig: config}
	}
	return &clientAPI, err
}

// BuildClient
func BuildClient() (kubeClient *KubeClient, err error) {
	if clientAPI.KubeClientInterface == nil {
		config, err := rest.InClusterConfig()
		if err != nil {
			return nil, err
		}
		// creates the clientset
		clientset, err := kubernetes.NewForConfig(config)
		if err != nil {
			return nil, err
		}
		clientAPI = KubeClient{KubeClientInterface: clientset, KubeRestConfig: config}
	}
	return &clientAPI, err
}

func BuildClientFromSecret(secretName, secretNamespace string) (kubeClient *KubeClient, err error) {
	if secretName == "" {
		return nil, fmt.Errorf("secret name for building kubeclient is empty")
	}
	if secretNamespace == "" {
		return nil, fmt.Errorf("secret namespace for building kubeclient is empty")
	}
	if clientAPI.KubeClientInterface == nil {
		config, err := rest.InClusterConfig()
		if err != nil {
			return nil, err
		}
		// creates the clientset
		clientset, err := kubernetes.NewForConfig(config)
		if err != nil {
			return nil, err
		}
		clientAPI = KubeClient{KubeClientInterface: clientset, KubeRestConfig: config}
	}
	alternateKubeconfigSecret, err := clientAPI.GetSecret(context.Background(), secretName, secretNamespace)
	if err != nil {
		fmt.Printf("error getting Secret - error: %s name: %s namespace: %s\n", secretName, secretNamespace, err.Error())
		return nil, err
	}
	alternateBytes := []byte(alternateKubeconfigSecret["config"])
	fmt.Printf("got alternate Kubeconfig Secret name: %s namespace: %s lenght: %d (bytes)\n", secretName, secretNamespace, len(alternateBytes))

	alternateClientConfig, err := clientcmd.NewClientConfigFromBytes(alternateBytes)
	if err != nil {
		fmt.Printf("error creating alternate kubeclient %s\n", err.Error())
		return nil, err
	}
	aRestConfig, err := alternateClientConfig.ClientConfig() // Get *rest.Config
	if err != nil {
		fmt.Printf("error getting alternate restConfig %s\n", err.Error())
		return nil, err
	}
	aClientset, err := kubernetes.NewForConfig(aRestConfig)
	if err != nil {
		fmt.Printf("error creating alternet clientset %s\n", err.Error())
		return nil, err
	}

	aClientAPI := KubeClient{
		KubeClientInterface: aClientset,
		KubeRestConfig:      aRestConfig,
	}
	return &aClientAPI, err
}

func (kc *KubeClient) GetSecret(ctx context.Context, secretName, namespace string) (map[string]string, error) {
	secretMap := make(map[string]string)
	secret, err := kc.KubeClientInterface.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		return secretMap, common.Errorf("error getting secret - namespace: %s secretName: %s error: %w", namespace, secretName, err)
	}
	for key, value := range secret.Data {
		secretMap[key] = string(value)
	}
	maps.Copy(secretMap, secret.StringData)
	return secretMap, nil
}

func (kc *KubeClient) GetSecretContainsName(ctx context.Context, namespace, name string) (*v1.Secret, error) {
	options := metav1.ListOptions{
		LabelSelector: "app=infinidat-csi-driver",
	}
	secrets, err := kc.KubeClientInterface.CoreV1().Secrets(namespace).List(ctx, options)
	if err != nil {
		return nil, common.Errorf("error getting secrets - namespace: %s error: %w", namespace, err)
	}
	slog.Debug("got secrets for app=infinidat-csi-driver", "item count", len(secrets.Items), "namespace", namespace)
	for _, secret := range secrets.Items {
		if strings.Contains(secret.Name, name) {
			return &secret, nil
		}
	}

	return nil, common.Errorf("error getting secrets , no secrets were found with the string %s in namespace: %s", name, namespace)
}

func (kc *KubeClient) GetSecrets(ctx context.Context, namespace string) ([]map[string]string, error) {
	secretMaps := make([]map[string]string, 0)
	options := metav1.ListOptions{
		LabelSelector: "app=infinidat-csi-driver",
	}
	secrets, err := kc.KubeClientInterface.CoreV1().Secrets(namespace).List(ctx, options)
	if err != nil {
		return secretMaps, common.Errorf("error getting secrets - namespace: %s error: %w", namespace, err)
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

	if len(secretMaps) == 0 {
		return secretMaps, common.Errorf("error getting secrets , no secrets were found with the label %s in namespace: %s", options.LabelSelector, namespace)
	}

	return secretMaps, nil
}

func (kc *KubeClient) GetPersistantVolumeByName(ctx context.Context, volumeName string) (*v1.PersistentVolume, error) {
	persistVol, err := kc.KubeClientInterface.CoreV1().PersistentVolumes().Get(ctx, volumeName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return persistVol, nil
}

// Return a PersistentVolumeList listing PVs created by this CSI Driver.
func (kc *KubeClient) GetAllPersistentVolumes(ctx context.Context) (*v1.PersistentVolumeList, error) {
	persistentVolumes, err := kc.KubeClientInterface.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	slog.Debug("PersistentVolumes", "count", len(persistentVolumes.Items))

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

func (kc *KubeClient) GetAllStorageClasses(ctx context.Context) (*storagev1.StorageClassList, error) {
	storageclasses, err := kc.KubeClientInterface.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	return storageclasses, nil
}
func (kc *KubeClient) GetStorageClass(ctx context.Context, scName string) (*storagev1.StorageClass, error) {
	storageclass, err := kc.KubeClientInterface.StorageV1().StorageClasses().Get(ctx, scName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	return storageclass, nil
}

func (kc *KubeClient) GetNodes(ctx context.Context) (nodes []v1.Node, err error) {
	nodeList, err := kc.KubeClientInterface.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nodes, err
	}
	return nodeList.Items, nil
}
func (kc *KubeClient) GetPV(ctx context.Context, name string) (pv *v1.PersistentVolume, err error) {
	pv, err = kc.KubeClientInterface.CoreV1().PersistentVolumes().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return pv, nil
}

func (kc *KubeClient) GetPVByVolumeID(ctx context.Context, volumeID int, protocol string) (*v1.PersistentVolume, error) {
	volumeHandle := fmt.Sprintf("%d$$%s", volumeID, protocol)
	pvList, err := kc.KubeClientInterface.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{})
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

func (kc *KubeClient) GetClusterVerion() (string, error) {
	info, err := kc.KubeClientInterface.Discovery().ServerVersion()
	if err != nil {
		return "", err
	}
	return info.GitVersion, nil
}

func (kc *KubeClient) GetPVCs(ctx context.Context, namespace string) (pvcList *v1.PersistentVolumeClaimList, err error) {
	pvcList, err = kc.KubeClientInterface.CoreV1().PersistentVolumeClaims(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return pvcList, nil
}

func (kc *KubeClient) GetPVC(ctx context.Context, namespace, name string) (pvc *v1.PersistentVolumeClaim, err error) {
	pvc, err = kc.KubeClientInterface.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return pvc, nil
}

// GetPVCAnnotations : Get pvc annotations for a given volumeName
func (kc *KubeClient) GetPVCAnnotations(ctx context.Context, pvcName, pvcNamespace string) (annotations map[string]string, err error) {
	var pvc *v1.PersistentVolumeClaim
	pvc, err = kc.GetPVC(ctx, pvcNamespace, pvcName)
	if err != nil {
		return annotations, err
	}
	return pvc.Annotations, nil
}

// get the CSI Driver pods that would be created by the driver's Daemonset
func (kc *KubeClient) GetRunningDriverNodePods(ctx context.Context, namespace string) (pods []v1.Pod, err error) {
	options := metav1.ListOptions{
		LabelSelector: "part-of=infiniboxcsidriver-node",
	}
	podList, err := kc.KubeClientInterface.CoreV1().Pods(namespace).List(ctx, options)
	if err != nil {
		return pods, err
	}
	for _, pod := range podList.Items {
		if pod.Status.Phase == "Running" {
			pods = append(pods, pod)
		}
	}
	if len(pods) == 0 {
		return pods, fmt.Errorf("no CSI driver node pods are in running status")
	}

	return pods, nil
}

// ExecCmdInPod - exec command on specific pod and wait the command's output.
func (kc *KubeClient) ExecCmdInPod(ctx context.Context, podName, nameSpace, command, containerName string) (string, string, error) {
	stdOut := &bytes.Buffer{}
	stdErr := &bytes.Buffer{}

	cmd := []string{
		"/bin/sh",
		"-c",
		command,
	}
	req := kc.KubeClientInterface.CoreV1().RESTClient().Post().
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

	exec, err := remotecommand.NewSPDYExecutor(kc.KubeRestConfig, "POST", req.URL())
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

func (kc *KubeClient) CreatePersistantVolume(ctx context.Context, newPV *v1.PersistentVolume) (*v1.PersistentVolume, error) {
	pv, err := kc.KubeClientInterface.CoreV1().PersistentVolumes().Create(ctx, newPV, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}
	return pv, nil
}
func (kc *KubeClient) CreateSecret(ctx context.Context, newSecret *v1.Secret) (*v1.Secret, error) {
	secret, err := kc.KubeClientInterface.CoreV1().Secrets(newSecret.Namespace).Create(ctx, newSecret, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}
	return secret, nil
}

func (kc *KubeClient) CreatePersistantVolumeClaim(ctx context.Context, newPVC *v1.PersistentVolumeClaim) (*v1.PersistentVolumeClaim, error) {
	pvc, err := kc.KubeClientInterface.CoreV1().PersistentVolumeClaims(newPVC.Namespace).Create(ctx, newPVC, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}
	return pvc, nil
}
