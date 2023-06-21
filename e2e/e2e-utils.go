package e2e

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"testing"
	"time"

	kubeapi "k8s.io/kubernetes/pkg/api/v1/pod"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"gopkg.in/yaml.v2"
	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"

	snapshotapi "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	snapshotv6 "github.com/kubernetes-csi/external-snapshotter/client/v6/clientset/versioned"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	KubeConfigPath = flag.String(
		"kubeconfig", "", "path to the kubeconfig file")
	StorageClassPath = flag.String(
		"storageclass", "", "path to the storageclass file")
	OperatorNamespace = flag.String(
		"operatornamespace", "infinidat-csi", "namespace the operator runs within")
	CleanUp = flag.Bool(
		"cleanup", true, "clean up test namespace on exit")
)

func GetKubeClient(kubeConfigPath string) (*kubernetes.Clientset, dynamic.Interface, *snapshotv6.Clientset, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeConfigPath)
	if err != nil {
		return nil, nil, nil, err
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, nil, nil, err
	}
	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, nil, nil, err
	}

	snapshotClient, err := snapshotv6.NewForConfig(config)
	if err != nil {
		return nil, nil, nil, err
	}

	return clientset, dynamicClient, snapshotClient, nil
}

var letters = []rune("abcdefghijklmnopqrstuvwxyz")

func RandSeq(n int) string {
	//rand.Seed(time.Now().UnixNano()) - not needed as of go 1.20 - automatically seeded
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[r.Intn(len(letters))]
	}
	return string(b)
}

func DeleteStorageClass(ctx context.Context, scName string, clientSet *kubernetes.Clientset) (err error) {
	err = clientSet.StorageV1().StorageClasses().Delete(context.TODO(), scName, metav1.DeleteOptions{})
	if err != nil {
		return err
	}
	return nil
}

func DeleteNamespace(ctx context.Context, nsName string, clientSet *kubernetes.Clientset) (err error) {
	err = clientSet.CoreV1().Namespaces().Delete(context.TODO(), nsName, metav1.DeleteOptions{})
	if err != nil {
		return err
	}
	return nil
}

func DeleteVolumeSnapshotClass(ctx context.Context, snapshotClassName string, snapshotClient *snapshotv6.Clientset) error {
	err := snapshotClient.SnapshotV1().VolumeSnapshotClasses().Delete(ctx, snapshotClassName, metav1.DeleteOptions{})
	if err != nil {
		return err
	}
	return nil
}

func DeletePVC(ctx context.Context, ns string, pvcName string, clientSet *kubernetes.Clientset) (err error) {
	err = clientSet.CoreV1().PersistentVolumeClaims(ns).Delete(ctx, pvcName, metav1.DeleteOptions{})
	if err != nil {
		return err
	}
	return nil
}
func DeletePod(ctx context.Context, ns string, podName string, clientSet *kubernetes.Clientset) (err error) {
	var sec int64
	opts := metav1.DeleteOptions{
		GracePeriodSeconds: &sec,
	}
	err = clientSet.CoreV1().Pods(ns).Delete(ctx, podName, opts)
	if err != nil {
		return err
	}
	return nil
}

func CreateStorageClass(prefix string, path string, clientSet *kubernetes.Clientset) (scName string, err error) {
	fileContent, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	uniqueSCName := prefix + RandSeq(2)
	fmt.Printf("unique sc %s\n", uniqueSCName)
	sc := &storagev1.StorageClass{}

	err = yaml.Unmarshal(fileContent, sc)
	if err != nil {
		return "", err
	}

	sc.Name = uniqueSCName

	createOptions := metav1.CreateOptions{}

	sc, err = clientSet.StorageV1().StorageClasses().Create(context.TODO(), sc, createOptions)
	if err != nil {
		return "", err
	}

	return sc.Name, nil
}

func CreatePVC(pvcName string, scName string, ns string, clientSet *kubernetes.Clientset) (err error) {
	rList := make(map[v1.ResourceName]resource.Quantity)
	rList[v1.ResourceStorage], err = resource.ParseQuantity("1Gi")
	if err != nil {
		return err
	}
	requirements := v1.ResourceRequirements{
		Requests: rList,
	}
	pvc := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: ns,
		},
		Spec: v1.PersistentVolumeClaimSpec{
			AccessModes:      []v1.PersistentVolumeAccessMode{v1.ReadWriteMany},
			Resources:        requirements,
			StorageClassName: &scName,
		},
	}

	_, err = clientSet.CoreV1().PersistentVolumeClaims(ns).Create(context.TODO(), pvc, metav1.CreateOptions{})
	if err != nil {
		return err
	}
	return nil
}

func CreateVolumeSnapshotClass(ctx context.Context, prefix string, nsName string, clientSet *snapshotv6.Clientset) (string, error) {
	m := metav1.ObjectMeta{
		Name: prefix + RandSeq(3),
	}

	vsclass := snapshotapi.VolumeSnapshotClass{
		ObjectMeta: m,
		Driver:     "infinidat-csi-driver",
		Parameters: map[string]string{
			"csi.storage.k8s.io/snapshotter-secret-name":      "infinibox-creds",
			"csi.storage.k8s.io/snapshotter-secret-namespace": nsName,
		},
		DeletionPolicy: snapshotapi.VolumeSnapshotContentDelete,
	}
	_, err := clientSet.SnapshotV1().VolumeSnapshotClasses().Create(ctx, &vsclass, metav1.CreateOptions{})
	if err != nil {
		return "", err
	}
	return m.Name, nil
}

func CreateNamespace(ctx context.Context, prefix string, clientset *kubernetes.Clientset) (nsName string, err error) {
	createOptions := metav1.CreateOptions{}
	uniqueName := prefix + RandSeq(3)

	m := metav1.ObjectMeta{
		Name: uniqueName,
	}
	ns := &v1.Namespace{
		ObjectMeta: m,
	}
	_, err = clientset.CoreV1().Namespaces().Create(ctx, ns, createOptions)
	if err != nil {
		return "", err
	}
	return uniqueName, nil
}

func WaitForPod(t *testing.T, podName string, ns string, clientset *kubernetes.Clientset) error {
	pollInterval := 1 * time.Second
	pollDuration := 2 * time.Minute
	err := wait.Poll(pollInterval, pollDuration, func() (bool, error) {
		getOptions := metav1.GetOptions{}

		t.Logf("waiting for infinidat csi operator pod to show up in namespace %s", ns)
		p, err := clientset.CoreV1().Pods(ns).Get(context.TODO(), podName, getOptions)
		if err != nil && apierrors.IsNotFound(err) {
			t.Logf("pod %s pod not found!\n", podName)
			return false, nil
		}
		if err != nil {
			return false, err
		}

		if p != nil {
			if kubeapi.IsPodReady(p) {
				t.Logf("✓ pod %s is created and ready\n", podName)
				return true, nil
			}
		}
		t.Logf("pod %s found but not ready\n", podName)

		return false, nil
	})

	return err
}

func GetFlags(t *testing.T) {

	flag.Parse()

	if *KubeConfigPath == "" {
		t.Log("looking for KUBECONFIG path from env var..")
		*KubeConfigPath = os.Getenv("KUBECONFIG")
	}
	if *KubeConfigPath == "" {
		t.Fatalf("kubeconfigpath flag not set and is required")
	}
	if *StorageClassPath == "" {
		t.Log("looking for STORAGECLASS path from env var..")
		*StorageClassPath = os.Getenv("STORAGECLASS")
	}
	if *StorageClassPath == "" {
		t.Fatalf("storageclass flag not set and is required")
	}
	x := os.Getenv("OPERATORNAMESPACE")
	if x == "" {
		t.Logf("OPERATORNAMESPACE env var not set, using flag value %s", *OperatorNamespace)
	} else {
		t.Logf("OPERATORNAMESPACE env var set, using value %s", x)
		*OperatorNamespace = x
	}
	y := os.Getenv("CLEANUP")
	if y == "" {
		t.Logf("CLEANUP env var not set, using flag value %t", *CleanUp)
	} else {
		t.Logf("CLEANUP env var set, using value %s", y)
		var err error
		*CleanUp, err = strconv.ParseBool(y)
		if err != nil {
			t.Fatalf("CLEANUP env var not valid %s\n", err.Error())
		}
	}
}
