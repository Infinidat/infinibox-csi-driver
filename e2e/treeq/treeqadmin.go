package treeq

import (
	"context"
	"fmt"
	"infinibox-csi-driver/api"
	"infinibox-csi-driver/common"
	"infinibox-csi-driver/e2e"
	"os"
	"strconv"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var TREEQ_USERS []string

func init() {
	TREEQ_USERS = []string{"user1", "user2"}
}

func VerifyAdminTreeqs(config *e2e.TestConfig) (err error) {
	// there should be 2 running pods at this point, user1-app and user2-app
	for i := 0; i < len(TREEQ_USERS); i++ {
		err = e2e.WaitForPod(config.Testt, TREEQ_USERS[i]+"-app", config.TestNames.NSName, config.ClientSet, time.Second*5, time.Minute*4)
		if err != nil {
			return err
		}
	}
	return nil
}

func CreateAdminTreeqs(config *e2e.TestConfig) (fileSystemID int64, err error) {

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

	fileSystemName := "e2e-treeq-admin" + config.TestNames.UniqueSuffix

	c := make(map[string]string)
	secrets := map[string]string{
		"hostname": hostname,
		"password": password,
		"username": username,
	}

	x := api.ClientService{
		ConfigMap:  c,
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
	config.Testt.Logf("✓ Filesystem %s %d is created\n", fileSystemName, fs.ID)

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
		config.Testt.Logf("✓ TreeQ %s %d is created\n", resp.Name, resp.ID)
		treeqIDs[i] = resp.ID
	}
	err = CreatePersistentVolumesForTreeqs(fs, treeqIDs, networkSpaceIPAddress, config)
	if err != nil {
		return 0, err
	}

	err = CreatePersistentVolumeClaimsForTreeqs(config)
	if err != nil {
		return 0, err
	}
	err = CreateTreeqApps(config)
	if err != nil {
		return 0, err
	}

	return fs.ID, nil
}

func CreatePersistentVolumesForTreeqs(fs *api.FileSystem, treeqIDs []int64, networkSpaceIPAddress string, config *e2e.TestConfig) (err error) {
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
				Name:      TREEQ_USERS[i] + "-pv-" + config.TestNames.UniqueSuffix,
				Namespace: config.TestNames.NSName,
			},
			Spec: v1.PersistentVolumeSpec{
				Capacity:               rList,
				PersistentVolumeSource: persistentVolumeSource,
				AccessModes:            []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
				//PersistentVolumeReclaimPolicy: &scName,
				StorageClassName: config.TestNames.SCName,
				MountOptions:     []string{"hard", "rsize=1048576", "wsize=1048576"},
			},
		}

		_, err = config.ClientSet.CoreV1().PersistentVolumes().Create(context.TODO(), pv, metav1.CreateOptions{})
		if err != nil {
			return err
		}
		config.Testt.Logf("✓ PV %s is created\n", pv.Name)
	}
	return nil
}

func CreatePersistentVolumeClaimsForTreeqs(config *e2e.TestConfig) (err error) {
	accessModes := make([]v1.PersistentVolumeAccessMode, 0)
	accessModes = append(accessModes, config.AccessMode)

	rList := make(map[v1.ResourceName]resource.Quantity)
	rList[v1.ResourceStorage], err = resource.ParseQuantity("1Gi")
	if err != nil {
		return err
	}
	requirements := v1.VolumeResourceRequirements{
		Requests: rList,
	}

	for i := 0; i < len(TREEQ_USERS); i++ {
		pvc := &v1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      TREEQ_USERS[i] + "-pvc",
				Namespace: config.TestNames.NSName,
			},
			Spec: v1.PersistentVolumeClaimSpec{
				Resources:        requirements,
				AccessModes:      accessModes,
				StorageClassName: &config.TestNames.SCName,
				VolumeName:       TREEQ_USERS[i] + "-pv-" + config.TestNames.UniqueSuffix,
			},
		}

		_, err = config.ClientSet.CoreV1().PersistentVolumeClaims(config.TestNames.NSName).Create(context.TODO(), pvc, metav1.CreateOptions{})
		if err != nil {
			return err
		}
		config.Testt.Logf("✓ PVC %s is created\n", pvc.Name)
	}
	return nil
}

func CleanupAdminTreeqs(fileSystemID int64, config *e2e.TestConfig) (err error) {
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
		err := e2e.DeletePod(ctx, config.TestNames.NSName, podName, config.ClientSet)
		if err != nil {
			fmt.Printf("error deleting pod %s %s\n", podName, err.Error())
		}
		config.Testt.Logf("✓ Pod %s is deleted\n", podName)
		// delete pvcs
		pvcName := TREEQ_USERS[i] + "-pvc"
		err = e2e.DeletePVC(ctx, config.TestNames.NSName, pvcName, config.ClientSet)
		if err != nil {
			fmt.Printf("error deleting pvc %s %s\n", pvcName, err.Error())
		}
		config.Testt.Logf("✓ PVC %s is deleted\n", pvcName)
		// delete pvs
		pvName := TREEQ_USERS[i] + "-pv-" + config.TestNames.UniqueSuffix
		err = e2e.DeletePV(ctx, pvName, config.ClientSet)
		if err != nil {
			fmt.Printf("error deleting pv %s %s\n", pvName, err.Error())
		}
		config.Testt.Logf("✓ PV %s is deleted\n", pvName)
	}
	c := make(map[string]string)
	secrets := map[string]string{
		"hostname": hostname,
		"password": password,
		"username": username,
	}

	x := api.ClientService{
		ConfigMap:  c,
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
	config.Testt.Logf("✓ Filesystem %d is deleted\n", fileSystemID)
	return nil
}

func CreateTreeqApps(config *e2e.TestConfig) (err error) {
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
				Value: strconv.FormatBool(config.ReadOnlyPod),
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
			Namespace: config.TestNames.NSName,
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

		_, err = config.ClientSet.CoreV1().Pods(config.TestNames.NSName).Create(context.TODO(), &pod, createOptions)
		if err != nil {
			return err
		}
		config.Testt.Logf("✓ Pod %s is created\n", pod.Name)
	}
	return nil
}
