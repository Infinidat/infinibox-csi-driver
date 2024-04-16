package treeq

import (
	"context"
	"fmt"
	"infinibox-csi-driver/api"
	"infinibox-csi-driver/common"
	"infinibox-csi-driver/e2e"
	"os"
	"strconv"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

var TREEQ_USERS []string

func init() {
	TREEQ_USERS = []string{"user1", "user2"}
	//TREEQ_USERS = []string{"user1"}
}

func VerifyAdminTreeqs(t *testing.T, testNames e2e.TestResourceNames, clientSet *kubernetes.Clientset) (err error) {
	// there should be 2 running pods at this point, user1-app and user2-app
	for i := 0; i < len(TREEQ_USERS); i++ {
		err = e2e.WaitForPod(t, TREEQ_USERS[i]+"-app", testNames.NSName, clientSet, time.Second*5, time.Minute*4)
		if err != nil {
			return err
		}
	}
	return nil
}

func CreateAdminTreeqs(t *testing.T, testNames e2e.TestResourceNames, clientSet *kubernetes.Clientset) (fileSystemID int64, err error) {

	hostname := os.Getenv("_E2E_IBOX_HOSTNAME")
	if hostname == "" {
		return 0, fmt.Errorf("_E2E_IBOX_HOSTNAME env var required")
	}
	username := os.Getenv("_E2E_IBOX_USERNAME")
	if username == "" {
		return 0, fmt.Errorf("_E2E_IBOX_USERNAME env var required")
	}
	password := os.Getenv("_E2E_IBOX_PASSWORD")
	if password == "" {
		return 0, fmt.Errorf("_E2E_IBOX_PASSWORD env var required")
	}
	poolName := os.Getenv("_E2E_POOL")
	if poolName == "" {
		return 0, fmt.Errorf("_E2E_POOL env var required")
	}

	fileSystemName := "e2e-treeq-admin" + testNames.UniqueSuffix

	config := make(map[string]string)
	secrets := map[string]string{
		"hostname": hostname,
		"password": password,
		"username": username,
	}

	x := api.ClientService{
		ConfigMap:  config,
		SecretsMap: secrets,
	}

	clientsvc, err := x.NewClient()
	if err != nil {
		return 0, err
	}

	// get ip address for network space
	networkSpace := os.Getenv("_E2E_NETWORK_SPACE")
	if networkSpace == "" {
		return 0, fmt.Errorf("_E2E_NETWORK_SPACE env var not set, required")
	}
	networkSpaceResponse, err := clientsvc.GetNetworkSpaceByName(networkSpace)
	if err != nil {
		return 0, err
	}

	if len(networkSpaceResponse.Name) == 0 {
		return 0, fmt.Errorf("networkpace name does not exist: %s", networkSpace)
	}

	networkSpaceIPAddress := networkSpaceResponse.Portals[0].IpAdress

	poolID, err := clientsvc.GetStoragePoolIDByName(poolName)
	if err != nil {
		return 0, err
	}

	mapRequest := map[string]interface{}{
		"pool_id":  poolID,
		"name":     fileSystemName,
		"size":     8589934592, // 8Gb
		"provtype": common.SC_THIN_PROVISION_TYPE,
	}

	fs, err := clientsvc.CreateFilesystem(mapRequest)
	if err != nil {
		return 0, err
	}
	t.Logf("✓ Filesystem %s %d is created\n", fileSystemName, fs.ID)

	treeqIDs := make([]int64, len(TREEQ_USERS))

	for i := 0; i < len(TREEQ_USERS); i++ {
		treeqParameters := map[string]interface{}{
			"path":          "/" + TREEQ_USERS[i],
			"name":          TREEQ_USERS[i],
			"hard_capacity": 1073741824, // 1G
		}

		resp, err := clientsvc.CreateTreeq(fs.ID, treeqParameters)
		if err != nil {
			return 0, err
		}
		t.Logf("✓ TreeQ %s %d is created\n", resp.Name, resp.ID)
		treeqIDs[i] = resp.ID
	}
	err = CreatePersistentVolumesForTreeqs(t, fs, treeqIDs, networkSpaceIPAddress, testNames, clientSet)
	if err != nil {
		return 0, err
	}

	err = CreatePersistentVolumeClaimsForTreeqs(t, testNames, clientSet)
	if err != nil {
		return 0, err
	}
	err = CreateTreeqApps(t, testNames, clientSet)
	if err != nil {
		return 0, err
	}

	return fs.ID, nil
}

func CreatePersistentVolumesForTreeqs(t *testing.T, fs *api.FileSystem, treeqIDs []int64, networkSpaceIPAddress string, testNames e2e.TestResourceNames, clientSet *kubernetes.Clientset) (err error) {
	rList := make(map[v1.ResourceName]resource.Quantity)
	rList[v1.ResourceStorage], err = resource.ParseQuantity("1Gi")
	if err != nil {
		return err
	}
	ns := os.Getenv("_E2E_NAMESPACE")
	if ns == "" {
		return fmt.Errorf("_E2E_NAMESPACE env var not set, required")
	}
	secretRef := &v1.SecretReference{
		Name:      "infinibox-creds",
		Namespace: ns,
	}
	csiSource := &v1.CSIPersistentVolumeSource{
		ControllerExpandSecretRef:  secretRef,
		ControllerPublishSecretRef: secretRef,
		NodePublishSecretRef:       secretRef,
		NodeStageSecretRef:         secretRef,
		Driver:                     common.SERVICE_NAME,
		VolumeAttributes: map[string]string{
			"ipAddress":              networkSpaceIPAddress,
			"storage_protocol":       common.PROTOCOL_TREEQ,
			"nfs_export_permissions": `[{"access":"RW","client":"*","no_root_squash":true}]`,
		},
	}
	persistentVolumeSource := v1.PersistentVolumeSource{}
	persistentVolumeSource.CSI = csiSource
	for i := 0; i < len(TREEQ_USERS); i++ {
		csiSource.VolumeAttributes["volumePath"] = "/" + fs.Name + "/" + TREEQ_USERS[i]
		csiSource.VolumeHandle = strconv.FormatInt(fs.ID, 10) + "#" + strconv.FormatInt(treeqIDs[i], 10) + "$$" + common.PROTOCOL_TREEQ
		pv := &v1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{
				Name:      TREEQ_USERS[i] + "-pv-" + testNames.UniqueSuffix,
				Namespace: testNames.NSName,
			},
			Spec: v1.PersistentVolumeSpec{
				Capacity:               rList,
				PersistentVolumeSource: persistentVolumeSource,
				AccessModes:            []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
				//PersistentVolumeReclaimPolicy: &scName,
				StorageClassName: testNames.SCName,
				MountOptions:     []string{"hard", "rsize=1048576", "wsize=1048576"},
			},
		}

		_, err = clientSet.CoreV1().PersistentVolumes().Create(context.TODO(), pv, metav1.CreateOptions{})
		if err != nil {
			return err
		}
		t.Logf("✓ PV %s is created\n", pv.Name)
	}
	return nil
}

func CreatePersistentVolumeClaimsForTreeqs(t *testing.T, testNames e2e.TestResourceNames, clientSet *kubernetes.Clientset) (err error) {
	rList := make(map[v1.ResourceName]resource.Quantity)
	rList[v1.ResourceStorage], err = resource.ParseQuantity("1Gi")
	if err != nil {
		return err
	}
	requirements := v1.ResourceRequirements{
		Requests: rList,
	}

	for i := 0; i < len(TREEQ_USERS); i++ {
		pvc := &v1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      TREEQ_USERS[i] + "-pvc",
				Namespace: testNames.NSName,
			},
			Spec: v1.PersistentVolumeClaimSpec{
				Resources:        requirements,
				AccessModes:      []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
				StorageClassName: &testNames.SCName,
				VolumeName:       TREEQ_USERS[i] + "-pv-" + testNames.UniqueSuffix,
			},
		}

		_, err = clientSet.CoreV1().PersistentVolumeClaims(testNames.NSName).Create(context.TODO(), pvc, metav1.CreateOptions{})
		if err != nil {
			return err
		}
		t.Logf("✓ PVC %s is created\n", pvc.Name)
	}
	return nil
}

func CleanupAdminTreeqs(t *testing.T, testNames e2e.TestResourceNames, fileSystemID int64, clientSet *kubernetes.Clientset) (err error) {
	// put these in testResourceNames
	hostname := os.Getenv("_E2E_IBOX_HOSTNAME")
	if hostname == "" {
		return fmt.Errorf("_E2E_IBOX_HOSTNAME env var required")
	}
	username := os.Getenv("_E2E_IBOX_USERNAME")
	if username == "" {
		return fmt.Errorf("_E2E_IBOX_USERNAME env var required")
	}
	password := os.Getenv("_E2E_IBOX_PASSWORD")
	if password == "" {
		return fmt.Errorf("_E2E_IBOX_PASSWORD env var required")
	}
	for i := 0; i < len(TREEQ_USERS); i++ {
		// delete apps
		ctx := context.Background()
		podName := TREEQ_USERS[i] + "-app"
		err := e2e.DeletePod(ctx, testNames.NSName, podName, clientSet)
		if err != nil {
			fmt.Printf("error deleting pod %s %s\n", podName, err.Error())
		}
		t.Logf("✓ Pod %s is deleted\n", podName)
		// delete pvcs
		pvcName := TREEQ_USERS[i] + "-pvc"
		err = e2e.DeletePVC(ctx, testNames.NSName, pvcName, clientSet)
		if err != nil {
			fmt.Printf("error deleting pvc %s %s\n", pvcName, err.Error())
		}
		t.Logf("✓ PVC %s is deleted\n", pvcName)
		// delete pvs
		pvName := TREEQ_USERS[i] + "-pv-" + testNames.UniqueSuffix
		err = e2e.DeletePV(ctx, pvName, clientSet)
		if err != nil {
			fmt.Printf("error deleting pv %s %s\n", pvName, err.Error())
		}
		t.Logf("✓ PV %s is deleted\n", pvName)
	}
	config := make(map[string]string)
	secrets := map[string]string{
		"hostname": hostname,
		"password": password,
		"username": username,
	}

	x := api.ClientService{
		ConfigMap:  config,
		SecretsMap: secrets,
	}

	clientsvc, err := x.NewClient()
	if err != nil {
		return err
	}
	// delete filesystem
	err = clientsvc.DeleteFileSystemComplete(fileSystemID)
	if err != nil {
		fmt.Printf("error deleting filesystem %d %s\n", fileSystemID, err.Error())
	}
	t.Logf("✓ Filesystem %d is deleted\n", fileSystemID)
	return nil
}

func CreateTreeqApps(t *testing.T, testNames e2e.TestResourceNames, clientSet *kubernetes.Clientset) (err error) {
	privileged := false
	allowPrivilegeEscalation := false
	runAsNonRoot := true

	createOptions := metav1.CreateOptions{}

	volumeMounts := make([]v1.VolumeMount, 0)
	image := "infinidat/csitestimage:latest"
	volumeMount := v1.VolumeMount{
		MountPath: "/tmp/csitesting",
		Name:      "ibox-csi-volume",
	}
	volumeMounts = append(volumeMounts, volumeMount)
	container := v1.Container{
		Name:            "csitest",
		Image:           image,
		ImagePullPolicy: v1.PullIfNotPresent,
		VolumeMounts:    volumeMounts,
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

	volume := v1.Volume{
		Name: "ibox-csi-volume",
		VolumeSource: v1.VolumeSource{
			PersistentVolumeClaim: &v1.PersistentVolumeClaimVolumeSource{},
		},
	}

	for i := 0; i < len(TREEQ_USERS); i++ {
		pvcName := TREEQ_USERS[i] + "-pvc"
		m := metav1.ObjectMeta{
			Name:      TREEQ_USERS[i] + "-app",
			Namespace: testNames.NSName,
		}
		volume.VolumeSource.PersistentVolumeClaim.ClaimName = pvcName

		pod := v1.Pod{
			ObjectMeta: m,
			Spec: v1.PodSpec{
				ImagePullSecrets: []v1.LocalObjectReference{
					{
						Name: e2e.IMAGE_PULL_SECRET,
					},
				},
				Containers: []v1.Container{container},
				Volumes:    []v1.Volume{volume},
			},
		}

		_, err = clientSet.CoreV1().Pods(testNames.NSName).Create(context.TODO(), &pod, createOptions)
		if err != nil {
			return err
		}
		t.Logf("✓ Pod %s is created\n", pod.Name)
	}
	return nil
}
