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

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	kubeapi "k8s.io/kubernetes/pkg/api/v1/pod"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

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

const (
	VOLUME_SNAPSHOT_CLASS = "e2e-"
	IMAGE_PULL_SECRET     = "private-docker-reg-secret"
	POD_NAME              = "e2e-test-pod"
	PVC_NAME              = "ibox-%s-pvc-demo"
	SNAPSHOT_NAME         = "e2e-test-snapshot"
	E2E_NAMESPACE         = "e2e-%s-"
	SC_NAME               = "e2e-%s-"
	DRIVER_NAMESPACE      = "infinidat-csi"
)

type TestResourceNames struct {
	NSName            string
	SCName            string
	PVCName           string
	SnapshotName      string
	SnapshotClassName string
	StorageClassName  string
}

func GetKubeClient(kubeConfigPath string) (*kubernetes.Clientset, *dynamic.DynamicClient, *snapshotv6.Clientset, error) {
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

func CreateStorageClass(prefix string, uniqueSuffix string, path string, clientSet *kubernetes.Clientset) (scName string, err error) {
	fileContent, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	uniqueSCName := prefix + uniqueSuffix
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

func CreateVolumeSnapshotClass(ctx context.Context, prefix string, uniqueSuffix string, nsName string, clientSet *snapshotv6.Clientset) (string, error) {
	m := metav1.ObjectMeta{
		Name: prefix + uniqueSuffix,
	}

	vsclass := snapshotapi.VolumeSnapshotClass{
		ObjectMeta: m,
		Driver:     "infinidat-csi-driver",
		Parameters: map[string]string{
			"csi.storage.k8s.io/snapshotter-secret-name":      "infinibox-creds",
			"csi.storage.k8s.io/snapshotter-secret-namespace": DRIVER_NAMESPACE,
		},
		DeletionPolicy: snapshotapi.VolumeSnapshotContentDelete,
	}
	_, err := clientSet.SnapshotV1().VolumeSnapshotClasses().Create(ctx, &vsclass, metav1.CreateOptions{})
	if err != nil {
		return "", err
	}
	return m.Name, nil
}

func CreateVolumeSnapshotClassDynamically(ctx context.Context, prefix string, uniqueSuffix string, dClient *dynamic.DynamicClient) (string, error) {
	VOLUMESNAPSHOTCLASS_GVR := schema.GroupVersionResource{Group: "snapshot.storage.k8s.io", Version: "v1", Resource: "volumesnapshotclasses"}

	mdb := &unstructured.Unstructured{}
	mdb.SetUnstructuredContent(map[string]interface{}{
		"apiVersion":     "snapshot.storage.k8s.io/v1",
		"kind":           "VolumeSnapshotClass",
		"driver":         "infinidat-csi-driver",
		"deletionPolicy": snapshotapi.VolumeSnapshotContentDelete,
		"metadata": map[string]interface{}{
			"name": prefix + uniqueSuffix,
		},
		"parameters": map[string]string{
			"csi.storage.k8s.io/snapshotter-secret-name":      "infinibox-creds",
			"csi.storage.k8s.io/snapshotter-secret-namespace": DRIVER_NAMESPACE,
		},
	})

	_, err := dClient.Resource(VOLUMESNAPSHOTCLASS_GVR).Create(ctx, mdb, metav1.CreateOptions{})
	if err != nil {
		return "", err
	}
	return prefix + uniqueSuffix, nil
}

func CreateNamespace(ctx context.Context, prefix string, suffix string, clientset *kubernetes.Clientset) (nsName string, err error) {
	createOptions := metav1.CreateOptions{}
	uniqueName := prefix + suffix

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

func WaitForSnapshot(t *testing.T, snapshotName string, ns string, clientset *snapshotv6.Clientset) error {
	pollInterval := 1 * time.Second
	pollDuration := 2 * time.Minute
	err := wait.Poll(pollInterval, pollDuration, func() (bool, error) {
		getOptions := metav1.GetOptions{}

		t.Logf("waiting for volumesnapshot to show up in namespace %s", ns)
		p, err := clientset.SnapshotV1().VolumeSnapshots(ns).Get(context.TODO(), snapshotName, getOptions)
		if err != nil && apierrors.IsNotFound(err) {
			t.Logf("volumesnapshot %s not found!\n", snapshotName)
			return false, nil
		}
		if err != nil {
			return false, err
		}

		if p != nil {
			if *p.Status.ReadyToUse {
				t.Logf("✓ volumesnapshot %s is created and ready\n", snapshotName)
				return true, nil
			}
		}
		t.Logf("volumesnapshot %s found but not ready\n", snapshotName)

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

func CreatePod(protocol string, ns string, clientset *kubernetes.Clientset) (err error) {
	pvcName := fmt.Sprintf(PVC_NAME, protocol)
	createOptions := metav1.CreateOptions{}

	m := metav1.ObjectMeta{
		Name: POD_NAME,
	}
	volumeMounts := v1.VolumeMount{
		MountPath: "/tmp/data",
		Name:      "ibox-csi-volume",
	}
	noPriv := false
	container := v1.Container{
		Name:            "e2e-test",
		Image:           "git.infinidat.com:4567/host-opensource/infinidat-csi-driver/e2e-test:latest",
		ImagePullPolicy: v1.PullAlways,
		VolumeMounts:    []v1.VolumeMount{volumeMounts},
		SecurityContext: &v1.SecurityContext{
			Privileged:               &noPriv,
			AllowPrivilegeEscalation: &noPriv,
			SeccompProfile: &v1.SeccompProfile{
				Type: v1.SeccompProfileTypeRuntimeDefault,
			},
			Capabilities: &v1.Capabilities{
				Drop: []v1.Capability{"ALL"},
			},
		},
	}

	volume := v1.Volume{
		Name: "ibox-csi-volume",
		VolumeSource: v1.VolumeSource{
			PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
				ClaimName: pvcName,
			},
		},
	}
	pod := &v1.Pod{
		ObjectMeta: m,
		Spec: v1.PodSpec{
			ImagePullSecrets: []v1.LocalObjectReference{
				{
					Name: IMAGE_PULL_SECRET,
				},
			},
			Containers: []v1.Container{container},
			Volumes:    []v1.Volume{volume},
		},
	}
	_, err = clientset.CoreV1().Pods(ns).Create(context.TODO(), pod, createOptions)
	if err != nil {
		return err
	}
	return nil
}

func CreateSnapshot(pvcName string, volumeSnapshotClassName string, ns string, clientSet *snapshotv6.Clientset) (err error) {
	createOptions := metav1.CreateOptions{}

	m := metav1.ObjectMeta{
		Name:      SNAPSHOT_NAME,
		Namespace: ns,
	}
	volumeSnapshotClassName = "ibox-snapshotclass-demo"
	snapshot := snapshotapi.VolumeSnapshot{
		ObjectMeta: m,
		Spec: snapshotapi.VolumeSnapshotSpec{
			VolumeSnapshotClassName: &volumeSnapshotClassName,
			Source: snapshotapi.VolumeSnapshotSource{
				PersistentVolumeClaimName: &pvcName,
			},
		},
	}
	_, err = clientSet.SnapshotV1().VolumeSnapshots(ns).Create(context.TODO(), &snapshot, createOptions)
	if err != nil {
		return err
	}
	return nil
}

func CreateImagePullSecret(t *testing.T, ns string, clientset *kubernetes.Clientset) error {
	result, err := clientset.CoreV1().Secrets(*OperatorNamespace).Get(context.TODO(), IMAGE_PULL_SECRET, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			t.Logf("image pull secret %s not found in operator namespace %s\n", IMAGE_PULL_SECRET, *OperatorNamespace)
			return nil
		} else {
			return err
		}
	}

	t.Logf("found image pull secret %s in operator namespace %s\n", IMAGE_PULL_SECRET, "infinidat-csi")

	result.ObjectMeta.Namespace = ns
	result.Namespace = ns
	result.ResourceVersion = ""
	createOptions := metav1.CreateOptions{}

	_, err = clientset.CoreV1().Secrets(ns).Create(context.TODO(), result, createOptions)
	if err != nil {
		return err

	}
	t.Logf("imagepullsecret %s created in ns %s\n", IMAGE_PULL_SECRET, ns)

	return nil
}

func Setup(protocol string, t *testing.T, client *kubernetes.Clientset, dynamicClient *dynamic.DynamicClient, snapshotClient *snapshotv6.Clientset) (testNames TestResourceNames) {

	t.Log("SETUP STARTS")
	var err error
	ctx := context.Background()
	e2eNamespace := fmt.Sprintf(E2E_NAMESPACE, protocol)
	uniqueSuffix := RandSeq(3)
	testNames.NSName, err = CreateNamespace(ctx, e2eNamespace, uniqueSuffix, client)
	if err != nil {
		t.Fatalf("error setting up e2e namespace %s\n", err.Error())
	}
	t.Logf("✓ Namespace %s is created\n", testNames.NSName)

	scName := fmt.Sprintf(SC_NAME, protocol)
	testNames.SCName, err = CreateStorageClass(scName, uniqueSuffix, *StorageClassPath, client)
	if err != nil {
		t.Fatalf("error creating StorageClass %s\n", err.Error())
	}
	t.Logf("✓ StorageClass %s is created\n", testNames.SCName)

	pvcName := fmt.Sprintf(PVC_NAME, protocol)
	testNames.PVCName = pvcName
	err = CreatePVC(pvcName, testNames.SCName, testNames.NSName, client)
	if err != nil {
		t.Fatalf("error creating PVC %s\n", err.Error())
	}
	t.Logf("✓ PVC %s is created\n", pvcName)

	err = CreateImagePullSecret(t, testNames.NSName, client)
	if err != nil {
		t.Fatalf("error creating image pull secret %s", err.Error())
	}
	t.Logf("✓ Image Pull Secret %s is created\n", IMAGE_PULL_SECRET)

	err = CreatePod(protocol, testNames.NSName, client)
	if err != nil {
		t.Fatalf("error creating test pod %s", err.Error())
	}
	t.Logf("✓ Pod %s is created\n", POD_NAME)

	err = WaitForPod(t, POD_NAME, testNames.NSName, client)
	if err != nil {
		t.Fatalf("error waiting for pod %s", err.Error())
	}
	t.Logf("✓ Pod %s is running\n", POD_NAME)

	//testNames.SnapshotClassName, err = CreateVolumeSnapshotClass(ctx, VOLUME_SNAPSHOT_CLASS, testNames.NSName, uniqueSuffix, snapshotClient)
	testNames.SnapshotClassName, err = CreateVolumeSnapshotClassDynamically(ctx, VOLUME_SNAPSHOT_CLASS, uniqueSuffix, dynamicClient)
	if err != nil {
		t.Fatalf("error creating VolumeSnapshotClass %s\n", err.Error())
	}
	t.Logf("✓ VolumeSnapshotClass %s is created\n", testNames.SnapshotClassName)

	t.Log("SETUP ENDS")
	return testNames
}

func TearDown(t *testing.T, testNames TestResourceNames, client *kubernetes.Clientset, dynamicClient dynamic.Interface, snapshotClient *snapshotv6.Clientset) {

	t.Log("TEARDOWN STARTS")
	ctx := context.Background()

	err := DeletePod(ctx, testNames.NSName, POD_NAME, client)
	if err != nil {
		t.Logf("error deleting pod %s\n", err.Error())
	}
	t.Logf("✓ pod %s is deleted\n", POD_NAME)

	err = DeletePVC(ctx, testNames.NSName, testNames.PVCName, client)
	if err != nil {
		t.Logf("error deleting PVC %s\n", err.Error())
	}
	t.Logf("✓ PVC %s is deleted\n", testNames.PVCName)

	err = DeleteStorageClass(ctx, testNames.SCName, client)
	if err != nil {
		t.Logf("error deleting storage class %s\n", err.Error())
	}
	t.Logf("✓ StorageClass %s is deleted\n", testNames.SCName)

	err = DeleteVolumeSnapshotClass(ctx, testNames.SnapshotClassName, snapshotClient)
	if err != nil {
		t.Logf("error deleting VolumeSnapshotClass %s\n", err.Error())
	}
	t.Logf("✓ VolumeSnapshotClass %s is deleted\n", testNames.SnapshotClassName)

	err = DeleteNamespace(ctx, testNames.NSName, client)
	if err != nil {
		t.Logf("error deleting namespace %s\n", err.Error())
	}
	t.Logf("✓ Namespace %s is deleted\n", testNames.NSName)
	t.Log("TEARDOWN ENDS")
}
