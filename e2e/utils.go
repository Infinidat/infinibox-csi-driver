package e2e

import (
	"context"
	"flag"
	"fmt"
	"infinibox-csi-driver/common"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"k8s.io/client-go/rest"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	kubeapi "k8s.io/kubernetes/pkg/api/v1/pod"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"gopkg.in/yaml.v2"
	v1 "k8s.io/api/core/v1"

	snapshotapi "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	snapshotv6 "github.com/kubernetes-csi/external-snapshotter/client/v6/clientset/versioned"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	KubeConfigPath = flag.String(
		"kubeconfig", "", "path to the kubeconfig file")
	OperatorNamespace = flag.String(
		"operatornamespace", "infinidat-csi", "namespace the operator runs within")
	CleanUp = flag.Bool(
		"cleanup", true, "clean up test namespace on exit")
	StorageClassPath string
)

const (
	SLEEP_BETWEEN_STEPS   = 5
	VOLUME_SNAPSHOT_CLASS = "e2e-"
	IMAGE_PULL_SECRET     = "private-docker-reg-secret"
	POD_NAME              = "e2e-test-pod"
	ANTI_AF_POD_NAME      = "e2e-test-anti-pod"
	PVC_NAME              = "ibox-%s-pvc-demo"
	SNAPSHOT_NAME         = "e2e-test-snapshot"
	E2E_NAMESPACE         = "e2e-%s-"
	SC_NAME               = "e2e-%s-"
	DRIVER_NAMESPACE      = "infinidat-csi"
	MOUNT_PATH            = "/tmp/csitesting"
	BLOCK_DEV_PATH        = "/dev/xvda"
	BLOCK_MOUNT_POINT     = "/var"
	POD_FS_GROUP          = 2000
)

type TestResourceNames struct {
	NSName            string
	SCName            string
	PVCName           string
	PVName            string
	SnapshotName      string
	SnapshotClassName string
	StorageClassName  string
	UniqueSuffix      string
}

type PVCAnnotations struct {
	IboxSecret       string
	IboxNetworkSpace string
	IboxPool         string
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

func GetRestConfig(kubeConfigPath string) *rest.Config {
	config, err := clientcmd.BuildConfigFromFlags("", kubeConfigPath)
	if err != nil {
		return nil
	}
	return config
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
func DeletePV(ctx context.Context, pvName string, clientSet *kubernetes.Clientset) (err error) {
	err = clientSet.CoreV1().PersistentVolumes().Delete(ctx, pvName, metav1.DeleteOptions{})
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

func UpdatePV(ctx context.Context, pvName string, clientSet *kubernetes.Clientset) error {

	pv, err := clientSet.CoreV1().PersistentVolumes().Get(ctx, pvName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	pv.Spec.ClaimRef = nil
	pv.Spec.AccessModes = make([]v1.PersistentVolumeAccessMode, 1)
	pv.Spec.AccessModes[0] = v1.ReadOnlyMany

	_, err = clientSet.CoreV1().PersistentVolumes().Update(ctx, pv, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	return nil
}

//***************** Storage Class ***************** //

func CreateStorageClass(testConfig *TestConfig, path string) (err error) {

	fileContent, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	sc := &storagev1.StorageClass{}

	err = yaml.Unmarshal(fileContent, sc)
	if err != nil {
		return err
	}

	poolToUse := os.Getenv("_E2E_POOL")
	if poolToUse == "" {
		return fmt.Errorf("_E2E_POOL env var is not set and is required")
	}
	sc.Name = testConfig.TestNames.SCName
	sc.Parameters["pool_name"] = poolToUse

	sc.Parameters[common.SC_SNAPDIR_VISIBLE] = strconv.FormatBool(testConfig.UseSnapdirVisible)

	if testConfig.UseRetainStorageClass {
		rp := v1.PersistentVolumeReclaimRetain
		sc.ReclaimPolicy = &rp
	}

	createOptions := metav1.CreateOptions{}

	_, err = testConfig.ClientSet.StorageV1().StorageClasses().Create(context.TODO(), sc, createOptions)
	if err != nil {
		return err
	}

	return nil
}

func CreatePVC(config *TestConfig) (err error) {
	rList := make(map[v1.ResourceName]resource.Quantity)
	rList[v1.ResourceStorage], err = resource.ParseQuantity("1Gi")
	if err != nil {
		return err
	}
	requirements := v1.VolumeResourceRequirements{
		Requests: rList,
	}

	accessModes := []v1.PersistentVolumeAccessMode{config.AccessMode}

	pvc := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.TestNames.PVCName,
			Namespace: config.TestNames.NSName,
		},
		Spec: v1.PersistentVolumeClaimSpec{
			AccessModes:      accessModes,
			Resources:        requirements,
			StorageClassName: &config.TestNames.SCName,
		},
	}

	if config.PVCAnnotations != nil {
		pvc.ObjectMeta.Annotations = make(map[string]string)
		if config.PVCAnnotations.IboxNetworkSpace != "" {
			pvc.ObjectMeta.Annotations[common.PVC_ANNOTATION_NETWORK_SPACE] = config.PVCAnnotations.IboxNetworkSpace
		}
		if config.PVCAnnotations.IboxSecret != "" {
			pvc.ObjectMeta.Annotations[common.PVC_ANNOTATION_IBOX_SECRET] = config.PVCAnnotations.IboxSecret
		}
		if config.PVCAnnotations.IboxPool != "" {
			pvc.ObjectMeta.Annotations[common.PVC_ANNOTATION_POOL_NAME] = config.PVCAnnotations.IboxPool
		}
	}
	if config.UseBlock {
		mode := v1.PersistentVolumeBlock
		pvc.Spec.VolumeMode = &mode
	}

	if config.UsePVCVolumeRef {
		pvc.Spec.VolumeName = config.TestNames.PVName
	}
	_, err = config.ClientSet.CoreV1().PersistentVolumeClaims(config.TestNames.NSName).Create(context.TODO(), pvc, metav1.CreateOptions{})
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

func CreateNamespace(ctx context.Context, uniqueName string, clientset *kubernetes.Clientset) (err error) {
	createOptions := metav1.CreateOptions{}

	m := metav1.ObjectMeta{
		Name: uniqueName,
	}
	ns := &v1.Namespace{
		ObjectMeta: m,
	}
	_, err = clientset.CoreV1().Namespaces().Create(ctx, ns, createOptions)
	if err != nil {
		return err
	}
	return nil
}

func WaitForPod(t *testing.T, podName string, ns string, clientset *kubernetes.Clientset, pollInterval time.Duration, pollDuration time.Duration) error {
	err := wait.PollUntilContextTimeout(context.Background(), pollInterval, pollDuration, false, func(ctx context.Context) (bool, error) {
		getOptions := metav1.GetOptions{}

		t.Logf("waiting for infinidat csi test pod to show up in namespace %s", ns)
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
	pollInterval := 5 * time.Second
	pollDuration := 2 * time.Minute
	err := wait.PollUntilContextTimeout(context.Background(), pollInterval, pollDuration, false, func(ctx context.Context) (bool, error) {
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
	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("error in Getwd %s\n", err.Error())
	}
	t.Logf("workingDir is %s\n", workingDir)
	StorageClassPath = workingDir + "/storageclass.yaml"
	_, err = os.Stat(StorageClassPath)
	if err != nil {
		t.Fatalf(" %s not found\n", StorageClassPath)
	}
	t.Logf("%s was found\n", StorageClassPath)

	x := os.Getenv("_E2E_NAMESPACE")
	if x == "" {
		t.Logf("_E2E_NAMESPACE env var not set, using flag value %s", *OperatorNamespace)
	} else {
		t.Logf("_E2E_NAMESPACE env var set, using value %s", x)
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

func CreatePod(testConfig *TestConfig, ns string, podName string) (err error) {
	pvcName := fmt.Sprintf(PVC_NAME, testConfig.Protocol)
	createOptions := metav1.CreateOptions{}
	podFSGroup := int64(POD_FS_GROUP)

	var m metav1.ObjectMeta

	if testConfig.UseAntiAffinity { // if this pod checks affinity, don't use a label

		m = metav1.ObjectMeta{
			Name: podName,
		}

	} else {

		m = metav1.ObjectMeta{
			Name: podName,
			Labels: map[string]string{ // used for multi-pod antiaffinity test
				"security": "s1",
			},
		}
	}

	volumeMounts := make([]v1.VolumeMount, 0)
	volumeDevices := make([]v1.VolumeDevice, 0)
	privileged := false
	allowPrivilegeEscalation := false
	runAsNonRoot := true
	image := "infinidat/csitestimage:latest"
	if testConfig.UseBlock {
		image = "infinidat/csitestimageblock:latest"
		device := v1.VolumeDevice{
			Name:       "ibox-csi-volume",
			DevicePath: "/dev/xvda",
		}
		volumeDevices = append(volumeDevices, device)
	} else {
		volumeMount := v1.VolumeMount{
			MountPath: MOUNT_PATH,
			Name:      "ibox-csi-volume",
		}
		volumeMounts = append(volumeMounts, volumeMount)
	}
	container := v1.Container{
		Name:            "e2e-test",
		Image:           image,
		ImagePullPolicy: v1.PullAlways,
		VolumeMounts:    volumeMounts,
		VolumeDevices:   volumeDevices,
		Env: []v1.EnvVar{
			{
				Name: "KUBE_NODE_NAME",
				ValueFrom: &v1.EnvVarSource{
					FieldRef: &v1.ObjectFieldSelector{
						FieldPath: "spec.nodeName",
					},
				},
			},
			{
				Name:  "READ_ONLY",
				Value: strconv.FormatBool(testConfig.ReadOnlyPod),
			},
		},
		SecurityContext: &v1.SecurityContext{
			Privileged:               &privileged,
			AllowPrivilegeEscalation: &allowPrivilegeEscalation,
			RunAsNonRoot:             &runAsNonRoot,
			SeccompProfile: &v1.SeccompProfile{
				Type: v1.SeccompProfileTypeRuntimeDefault,
			},
			/**
			Capabilities: &v1.Capabilities{
				Drop: []v1.Capability{"ALL"},
			},
			*/
		},
	}

	if testConfig.UseSELinux {
		container.SecurityContext.SELinuxOptions = &v1.SELinuxOptions{
			Type: "spc_t",
		}
	}

	volume := v1.Volume{
		Name: "ibox-csi-volume",
		VolumeSource: v1.VolumeSource{
			PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{
				ClaimName: pvcName,
				ReadOnly:  testConfig.ReadOnlyPodVolume,
			},
		},
	}

	var pod v1.Pod

	podAntiAffinity := v1.Affinity{
		PodAntiAffinity: &v1.PodAntiAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
				{
					LabelSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"security": "s1",
						},
					},
					TopologyKey: "kubernetes.io/hostname",
				},
			},
		},
	}

	// determine which affinity for pod here, so correct one is assigned below.

	if testConfig.UseFsGroup && !testConfig.UseAntiAffinity {
		pod = v1.Pod{
			ObjectMeta: m,
			Spec: v1.PodSpec{
				ImagePullSecrets: []v1.LocalObjectReference{
					{
						Name: IMAGE_PULL_SECRET,
					},
				},
				Containers: []v1.Container{container},
				Volumes:    []v1.Volume{volume},
				SecurityContext: &v1.PodSecurityContext{
					FSGroup: &podFSGroup,
				},
			},
		}

	} else if testConfig.UseFsGroup && testConfig.UseAntiAffinity {

		pod = v1.Pod{
			ObjectMeta: m,
			Spec: v1.PodSpec{
				Affinity: &podAntiAffinity,
				ImagePullSecrets: []v1.LocalObjectReference{
					{
						Name: IMAGE_PULL_SECRET,
					},
				},
				Containers: []v1.Container{container},
				Volumes:    []v1.Volume{volume},
				SecurityContext: &v1.PodSecurityContext{
					FSGroup: &podFSGroup,
				},
			},
		}

	} else if !testConfig.UseFsGroup && !testConfig.UseAntiAffinity {
		pod = v1.Pod{
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

	} else if !testConfig.UseFsGroup && testConfig.UseAntiAffinity {
		pod = v1.Pod{
			ObjectMeta: m,
			Spec: v1.PodSpec{
				Affinity: &podAntiAffinity,
				ImagePullSecrets: []v1.LocalObjectReference{
					{
						Name: IMAGE_PULL_SECRET,
					},
				},
				Containers: []v1.Container{container},
				Volumes:    []v1.Volume{volume},
			},
		}

	}

	_, err = testConfig.ClientSet.CoreV1().Pods(ns).Create(context.TODO(), &pod, createOptions)
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

func Setup(testConfig *TestConfig) {

	testConfig.Testt.Log("SETUP STARTS")
	testConfig.Testt.Log(GetEnvVars())

	var err error
	ctx := context.Background()
	err = CreateNamespace(ctx, testConfig.TestNames.NSName, testConfig.ClientSet)
	if err != nil {
		testConfig.Testt.Fatalf("error setting up e2e namespace %s\n", err.Error())
	}
	testConfig.Testt.Logf("✓ Namespace %s is created\n", testConfig.TestNames.NSName)

	time.Sleep(time.Second * SLEEP_BETWEEN_STEPS)

	err = CreateStorageClass(testConfig, StorageClassPath)
	if err != nil {
		testConfig.Testt.Fatalf("error creating StorageClass %s\n", err.Error())
	}
	testConfig.Testt.Logf("✓ StorageClass %s is created\n", testConfig.TestNames.SCName)

	time.Sleep(time.Second * SLEEP_BETWEEN_STEPS)

	pvcName := fmt.Sprintf(PVC_NAME, testConfig.Protocol)
	testConfig.TestNames.PVCName = pvcName

	if testConfig.UseAntiAffinity {
		testConfig.AccessMode = v1.ReadWriteMany
	}
	err = CreatePVC(testConfig)

	if err != nil {
		testConfig.Testt.Fatalf("error creating PVC %s\n", err.Error())
	}
	testConfig.Testt.Logf("✓ PVC %s is created\n", pvcName)

	err = WaitForPVC(testConfig.Testt, testConfig.TestNames.PVCName, testConfig.TestNames.NSName, testConfig.ClientSet, time.Second*5, time.Minute*1)
	if err != nil {
		DescribePVC(testConfig.Testt, testConfig.TestNames.PVCName, testConfig.TestNames.NSName, testConfig.ClientSet)
		testConfig.Testt.Fatalf("error binding PVC %s\n", err.Error())
	}
	testConfig.Testt.Logf("✓ PVC %s is bound\n", pvcName)

	time.Sleep(time.Second * SLEEP_BETWEEN_STEPS)

	err = CreateImagePullSecret(testConfig.Testt, testConfig.TestNames.NSName, testConfig.ClientSet)
	if err != nil {
		testConfig.Testt.Fatalf("error creating image pull secret %s", err.Error())
	}
	testConfig.Testt.Logf("✓ Image Pull Secret %s is created\n", IMAGE_PULL_SECRET)

	time.Sleep(time.Second * SLEEP_BETWEEN_STEPS)

	realAntiAffinity := testConfig.UseAntiAffinity // save user intent

	testConfig.UseAntiAffinity = false // first pod is never anti-affinity
	err = CreatePod(testConfig, testConfig.TestNames.NSName, POD_NAME)
	if err != nil {
		testConfig.Testt.Fatalf("error creating test pod %s", err.Error())
	}
	testConfig.Testt.Logf("✓ Pod %s is created\n", POD_NAME)

	err = WaitForPod(testConfig.Testt, POD_NAME, testConfig.TestNames.NSName, testConfig.ClientSet, time.Second*5, time.Minute*4)
	if err != nil {
		DescribePVC(testConfig.Testt, POD_NAME, testConfig.TestNames.NSName, testConfig.ClientSet)
		testConfig.Testt.Fatalf("error waiting for pod %s", err.Error())
	}
	testConfig.Testt.Logf("✓ Pod %s is running\n", POD_NAME)

	time.Sleep(time.Second * SLEEP_BETWEEN_STEPS)

	testConfig.UseAntiAffinity = true

	// anti-affinity pod for two-node specific tests.

	testConfig.UseAntiAffinity = realAntiAffinity

	if testConfig.UseAntiAffinity {
		err = CreatePod(testConfig, testConfig.TestNames.NSName, ANTI_AF_POD_NAME)
		if err != nil {
			DescribePVC(testConfig.Testt, ANTI_AF_POD_NAME, testConfig.TestNames.NSName, testConfig.ClientSet)
			testConfig.Testt.Fatalf("error creating test anti-affinity pod %s", err.Error())
		}
		testConfig.Testt.Logf("✓ Pod %s is created\n", ANTI_AF_POD_NAME)

		err = WaitForPod(testConfig.Testt, ANTI_AF_POD_NAME, testConfig.TestNames.NSName, testConfig.ClientSet, time.Second*5, time.Minute*4)
		if err != nil {
			testConfig.Testt.Fatalf("error waiting for pod %s", err.Error())
		}
		testConfig.Testt.Logf("✓ Pod %s is running\n", ANTI_AF_POD_NAME)
	}

	/**
	testConfig.TestNames.SnapshotClassName, err = CreateVolumeSnapshotClassDynamically(ctx, VOLUME_SNAPSHOT_CLASS, testConfig.TestNames.UniqueSuffix, testConfig.DynamicClient)
	if err != nil {
		testConfig.Testt.Fatalf("error creating VolumeSnapshotClass %s\n", err.Error())
	}
	testConfig.Testt.Logf("✓ VolumeSnapshotClass %s is created\n", testConfig.TestNames.SnapshotClassName)
	*/

	testConfig.Testt.Log("SETUP ENDS")
}

func TearDown(testConfig *TestConfig) {

	testConfig.Testt.Log("TEARDOWN STARTS")
	ctx := context.Background()

	err := DeletePod(ctx, testConfig.TestNames.NSName, POD_NAME, testConfig.ClientSet)
	if err != nil {
		testConfig.Testt.Logf("error deleting pod %s\n", err.Error())
	}
	testConfig.Testt.Logf("✓ pod %s is deleted\n", POD_NAME)

	err = DeletePVC(ctx, testConfig.TestNames.NSName, testConfig.TestNames.PVCName, testConfig.ClientSet)
	if err != nil {
		testConfig.Testt.Logf("error deleting PVC %s\n", err.Error())
	}
	testConfig.Testt.Logf("✓ PVC %s is deleted\n", testConfig.TestNames.PVCName)

	err = DeleteStorageClass(ctx, testConfig.TestNames.SCName, testConfig.ClientSet)
	if err != nil {
		testConfig.Testt.Logf("error deleting storage class %s\n", err.Error())
	}
	testConfig.Testt.Logf("✓ StorageClass %s is deleted\n", testConfig.TestNames.SCName)

	/**
	err = DeleteVolumeSnapshotClass(ctx, testConfig.TestNames.SnapshotClassName, testConfig.SnapshotClient)
	if err != nil {
		testConfig.Testt.Logf("error deleting VolumeSnapshotClass %s\n", err.Error())
	}
	testConfig.Testt.Logf("✓ VolumeSnapshotClass %s is deleted\n", testConfig.TestNames.SnapshotClassName)
	*/

	err = DeleteNamespace(ctx, testConfig.TestNames.NSName, testConfig.ClientSet)
	if err != nil {
		testConfig.Testt.Logf("error deleting namespace %s\n", err.Error())
	}
	testConfig.Testt.Logf("✓ Namespace %s is deleted\n", testConfig.TestNames.NSName)
	testConfig.Testt.Log("TEARDOWN ENDS")
}

func WaitForDeployment(t *testing.T, deploymentName string, ns string, clientset *kubernetes.Clientset) error {
	pollInterval := 5 * time.Second
	pollDuration := 4 * time.Minute
	err := wait.PollUntilContextTimeout(context.Background(), pollInterval, pollDuration, false, func(ctx context.Context) (bool, error) {
		getOptions := metav1.GetOptions{}

		t.Logf("waiting for deployment %s to show up in namespace %s", deploymentName, ns)
		p, err := clientset.AppsV1().Deployments(ns).Get(context.TODO(), deploymentName, getOptions)
		if err != nil && apierrors.IsNotFound(err) {
			t.Logf("deployment %s pod not found!\n", deploymentName)
			return false, nil
		}
		if err != nil {
			return false, err
		}

		if p != nil {
			status := p.Status
			if status.AvailableReplicas == 1 {
				t.Logf("✓ deployment %s is created and ready\n", deploymentName)
				return true, nil
			}
		}
		t.Logf("deployment %s found but not ready\n", deploymentName)

		return false, nil
	})

	return err
}

func GetTestSystemNodecount(t *testing.T, clientset *kubernetes.Clientset) int {

	nodes, err := clientset.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})

	if err != nil {
		t.Logf("Error while getting node count: %s", err.Error())
		return 0
	}

	var readyNodes int
	for _, item := range nodes.Items {
		for _, cond := range item.Status.Conditions {
			if cond.Type == v1.NodeReady && cond.Status == "True" {
				readyNodes++
				t.Logf("node %s is Ready", item.Name)
			}
		}
	}

	nodeCount := len(nodes.Items)

	t.Logf("Test System Node count: %d, readyCount: %d", nodeCount, readyNodes)
	return readyNodes

}

func GetEnvVars() string {

	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("_E2E_NAMESPACE [%s]\n", os.Getenv("_E2E_NAMESPACE")))
	sb.WriteString(fmt.Sprintf("_E2E_POOL [%s]\n", os.Getenv("_E2E_POOL")))
	sb.WriteString(fmt.Sprintf("_E2E_PROTOCOL [%s]\n", os.Getenv("_E2E_PROTOCOL")))
	sb.WriteString(fmt.Sprintf("_E2E_NETWORK_SPACE [%s]\n", os.Getenv("_E2E_NETWORK_SPACE")))
	sb.WriteString(fmt.Sprintf("_E2E_NETWORK_SPACE2 [%s]\n", os.Getenv("_E2E_NETWORK_SPACE2")))
	sb.WriteString(fmt.Sprintf("_E2E_IBOX_SECRET [%s]\n", os.Getenv("_E2E_IBOX_SECRET")))
	sb.WriteString(fmt.Sprintf("_E2E_K8S_VERSION [%s]\n", os.Getenv("_E2E_K8S_VERSION")))
	sb.WriteString(fmt.Sprintf("_E2E_OCP_VERSION [%s]\n", os.Getenv("_E2E_OCP_VERSION")))
	sb.WriteString(fmt.Sprintf("_E2E_IBOX_USERNAME [%s]\n", os.Getenv("_E2E_IBOX_USERNAME")))
	sb.WriteString(fmt.Sprintf("_E2E_IBOX_PASSWORD [%s]\n", os.Getenv("_E2E_IBOX_PASSWORD")))
	sb.WriteString(fmt.Sprintf("_E2E_IBOX_HOSTNAME [%s]\n", os.Getenv("_E2E_IBOX_HOSTNAME")))

	return sb.String()
}

func WaitForPVC(t *testing.T, pvcName string, ns string, clientset *kubernetes.Clientset, pollInterval time.Duration, pollDuration time.Duration) error {
	err := wait.PollUntilContextTimeout(context.Background(), pollInterval, pollDuration, false, func(ctx context.Context) (bool, error) {
		getOptions := metav1.GetOptions{}

		t.Logf("waiting for infinidat csi test pvc %s to show up in namespace %s", pvcName, ns)
		p, err := clientset.CoreV1().PersistentVolumeClaims(ns).Get(context.TODO(), pvcName, getOptions)
		if err != nil && apierrors.IsNotFound(err) {
			t.Logf("PVC %s not found!\n", pvcName)
			return false, nil
		}
		if err != nil {
			return false, err
		}

		if p != nil {
			if p.Status.Phase == v1.ClaimBound {

				t.Logf("✓ PVC %s is created and bound\n", pvcName)
				return true, nil
			}
		}
		t.Logf("PVC %s found but not ready, %v\n", pvcName, p.Status.Phase)
		DescribePVC(t, pvcName, ns, clientset)

		return false, nil
	})

	return err
}

func DescribePVC(t *testing.T, pvcName string, ns string, clientset *kubernetes.Clientset) {
	listOptions := metav1.ListOptions{FieldSelector: "involvedObject.name=" + pvcName, TypeMeta: metav1.TypeMeta{Kind: "PersistentVolumeClaim"}}

	t.Logf("describe details for infinidat csi test PVC %s in namespace %s", pvcName, ns)
	events, err := clientset.CoreV1().Events(ns).List(context.TODO(), listOptions)
	if err != nil {
		t.Logf("error getting describe details for infinidat csi test PVC %s in namespace %s", pvcName, ns)
		return
	}
	for _, item := range events.Items {
		t.Logf("PVC describe message %s reason %s", item.Message, item.Reason)
	}

}

func DescribePod(t *testing.T, podName string, ns string, clientset *kubernetes.Clientset) {
	listOptions := metav1.ListOptions{FieldSelector: "involvedObject.name=" + podName, TypeMeta: metav1.TypeMeta{Kind: "Pod"}}

	t.Logf("describe details for infinidat csi test Pod %s in namespace %s", podName, ns)
	events, err := clientset.CoreV1().Events(ns).List(context.TODO(), listOptions)
	if err != nil {
		t.Logf("error getting describe details for infinidat csi test Pod %s in namespace %s", podName, ns)
		return
	}
	for _, item := range events.Items {
		t.Logf("Pod describe message %s reason %s", item.Message, item.Reason)
	}

}

func GetPVName(pvcName string, ns string, clientset *kubernetes.Clientset) (string, error) {
	getOptions := metav1.GetOptions{}

	pvc, err := clientset.CoreV1().PersistentVolumeClaims(ns).Get(context.TODO(), pvcName, getOptions)
	if err != nil && apierrors.IsNotFound(err) {
		return "", err
	}

	return pvc.Spec.VolumeName, nil
}
