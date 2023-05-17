//go:build e2e

package e2etreeq

import (
	"context"
	"infinibox-csi-driver/e2e"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"

	snapshotv6 "github.com/kubernetes-csi/external-snapshotter/client/v6/clientset/versioned"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

const (
	VOLUME_SNAPSHOT_CLASS = "e2e-infinidat-volumesnapshotclass-"
	OPERATOR_NAMESPACE    = "infinidat-csi"
	DRIVER_NAME           = "infinidat-csi-driver"
	E2E_NAMESPACE         = "e2e-treeq-"
	PVC_NAME              = "ibox-treeq-pvc-demo"
	SC_NAME               = "e2e-treeq-"
	POD_NAME              = "e2e-test-pod"
	pollInterval          = 1 * time.Second
	pollDuration          = 2 * time.Minute
	// SnapshotGroup is the snapshot CRD api group
	SnapshotGroup = "snapshot.storage.k8s.io"
	// SnapshotAPIVersion is the snapshot CRD api version
	SnapshotAPIVersion = "snapshot.storage.k8s.io/v1"
)

type TestResourceNames struct {
	NSName            string
	SCName            string
	PVCName           string
	SnapshotName      string
	SnapshotClassName string
	StorageClassName  string
}

func TestTreeq(t *testing.T) {

	e2e.GetFlags(t)

	//connect to kube
	clientSet, dynamicClient, snapshotClient, err := e2e.GetKubeClient(*e2e.KubeConfigPath)
	if err != nil {
		t.Fatalf("error creating clients %s\n", err.Error())
	}
	if clientSet == nil {
		t.Fatalf("error creating k8s client")
	}

	// create a unique namespace to perform the test within

	testNames := setup(t, clientSet, dynamicClient, snapshotClient)

	t.Logf("testing in namespace %+v\n", testNames)
	// run the test
	err = createPod(testNames.NSName, clientSet)
	if err != nil {
		t.Fatalf("error creating test pod %s", err.Error())
	}

	err = e2e.WaitForPod(t, POD_NAME, testNames.NSName, clientSet)
	if err != nil {
		t.Fatalf("error waiting for pod %s", err.Error())
	}

	if *e2e.CleanUp {
		tearDown(t, testNames, clientSet, dynamicClient, snapshotClient)
	} else {
		t.Log("not cleaning up namespace")
	}
}

func setup(t *testing.T, client *kubernetes.Clientset, dynamicClient dynamic.Interface, snapshotClient *snapshotv6.Clientset) (testNames TestResourceNames) {

	t.Log("SETUP STARTS")
	var err error
	ctx := context.Background()
	testNames.NSName, err = e2e.CreateNamespace(ctx, E2E_NAMESPACE, client)
	if err != nil {
		t.Fatalf("error setting up e2e namespace %s\n", err.Error())
	}
	t.Logf("✓ Namespace %s is created\n", testNames.NSName)

	testNames.SCName, err = e2e.CreateStorageClass(SC_NAME, *e2e.StorageClassPath, client)
	if err != nil {
		t.Fatalf("error creating StorageClass %s\n", err.Error())
	}
	t.Logf("✓ StorageClass %s is created\n", testNames.SCName)

	err = e2e.CreatePVC(PVC_NAME, testNames.SCName, testNames.NSName, client)
	if err != nil {
		t.Fatalf("error creating PVC %s\n", err.Error())
	}
	t.Logf("✓ PVC %s is created\n", PVC_NAME)
	testNames.SnapshotClassName, err = e2e.CreateVolumeSnapshotClass(ctx, VOLUME_SNAPSHOT_CLASS, testNames.NSName, snapshotClient)
	if err != nil {
		t.Fatalf("error creating VolumeSnapshotClass %s\n", err.Error())
	}
	t.Logf("✓ VolumeSnapshotClass %s is created\n", testNames.SnapshotClassName)
	t.Log("SETUP ENDS")
	return testNames
}

func tearDown(t *testing.T, testNames TestResourceNames, client *kubernetes.Clientset, dynamicClient dynamic.Interface, snapshotClient *snapshotv6.Clientset) {

	t.Log("TEARDOWN STARTS")
	ctx := context.Background()
	err := e2e.DeleteStorageClass(ctx, testNames.SCName, client)
	if err != nil {
		t.Logf("error deleting storage class %s\n", err.Error())
	}
	t.Logf("✓ StorageClass %s is deleted\n", testNames.SCName)

	err = e2e.DeleteVolumeSnapshotClass(ctx, testNames.SnapshotClassName, snapshotClient)
	if err != nil {
		t.Logf("error deleting VolumeSnapshotClass %s\n", err.Error())
	}
	t.Logf("✓ VolumeSnapshotClass %s is deleted\n", testNames.SnapshotClassName)

	err = e2e.DeletePVC(ctx, testNames.NSName, testNames.PVCName, client)
	if err != nil {
		t.Logf("error deleting PVC %s\n", err.Error())
	}
	t.Logf("✓ PVC %s is deleted\n", PVC_NAME)

	err = e2e.DeleteNamespace(ctx, testNames.NSName, client)
	if err != nil {
		t.Logf("error deleting namespace %s\n", err.Error())
	}
	t.Logf("✓ Namespace %s is deleted\n", testNames.NSName)
	t.Log("TEARDOWN ENDS")
}

func createPod(ns string, clientset *kubernetes.Clientset) (err error) {
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
				ClaimName: PVC_NAME,
			},
		},
	}
	pod := &v1.Pod{
		ObjectMeta: m,
		Spec: v1.PodSpec{
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
