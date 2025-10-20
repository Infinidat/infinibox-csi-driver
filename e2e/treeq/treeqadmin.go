package treeq

import (
	"context"
	"fmt"
	"infinibox-csi-driver/api"
	"infinibox-csi-driver/common"
	"infinibox-csi-driver/e2e"
	"infinibox-csi-driver/iboxapi"
	"os"
	"strconv"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var treeqUsers []string

func init() {
	treeqUsers = []string{"user1", "user2"}
}

func VerifyAdminTreeqs(config *e2e.TestConfig) (err error) {
	// there should be 2 running pods at this point, user1-app and user2-app
	for _, treeqUser := range treeqUsers {
		err = e2e.WaitForPod(config.Testt, treeqUser+"-app", config.TestNames.NSName, config.ClientSet, time.Second*5, time.Minute*4)
		if err != nil {
			return err
		}
	}
	return nil
}

func CreateAdminTreeqs(config *e2e.TestConfig) (fileSystemID int, err error) {
	poolName := os.Getenv("_E2E_POOL")
	if poolName == "" {
		return 0, fmt.Errorf("_E2E_POOL env var required")
	}

	fileSystemName := "e2e-treeq-admin" + config.TestNames.UniqueSuffix

	// get ip address for network space
	networkSpace := os.Getenv(e2e.ENV_NAS_NETWORK_SPACE)
	if networkSpace == "" {
		networkSpace = os.Getenv(e2e.ENV_NETWORK_SPACE)
		if networkSpace == "" {
			return 0, fmt.Errorf("%s or %s env vars not set, one is required", e2e.ENV_NETWORK_SPACE, e2e.ENV_NAS_NETWORK_SPACE)
		}
	}
	networkSpaceResponse, err := config.ClientService.IboxAPI.GetNetworkSpaceByName(networkSpace)
	if err != nil {
		return 0, err
	}

	if len(networkSpaceResponse.Name) == 0 {
		return 0, fmt.Errorf("networkpace name does not exist: %s", networkSpace)
	}

	networkSpaceIPAddress := networkSpaceResponse.Portals[0].IPAddress

	pool, err := config.ClientService.IboxAPI.GetPoolByName(poolName)
	if err != nil {
		return 0, err
	}

	fsRequest := iboxapi.CreateFileSystemRequest{
		PoolID:   pool.ID,
		Name:     fileSystemName,
		Size:     8589934592, // 8Gb
		Provtype: common.SC_THIN_PROVISION_TYPE,
	}

	filesystem, err := config.ClientService.IboxAPI.CreateFileSystem(fsRequest)
	if err != nil {
		return 0, err
	}
	config.Testt.Logf("✓ Filesystem %s %d is created\n", fileSystemName, filesystem.ID)

	treeqIDs := make([]int, len(treeqUsers))

	for index := range treeqUsers {
		request := iboxapi.CreateTreeqRequest{
			Path:         "/" + treeqUsers[index],
			Name:         treeqUsers[index],
			HardCapacity: common.BytesInOneGibibyte,
		}
		resp, err := config.ClientService.IboxAPI.CreateTreeq(filesystem.ID, request)
		if err != nil {
			return 0, err
		}
		config.Testt.Logf("✓ TreeQ %s %d is created\n", resp.Name, resp.ID)
		treeqIDs[index] = resp.ID
	}
	err = CreatePersistentVolumesForTreeqs(filesystem, treeqIDs, networkSpaceIPAddress, config)
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

	return filesystem.ID, nil
}

func CreatePersistentVolumesForTreeqs(filesystem *iboxapi.FileSystem, treeqIDs []int, networkSpaceIPAddress string, config *e2e.TestConfig) (err error) {
	resourceList := make(map[v1.ResourceName]resource.Quantity)
	resourceList[v1.ResourceStorage], err = resource.ParseQuantity("1Gi")
	if err != nil {
		return err
	}
	namespace := os.Getenv("_E2E_NAMESPACE")
	if namespace == "" {
		return fmt.Errorf("_E2E_NAMESPACE env var not set, required")
	}
	secretRef := &v1.SecretReference{
		Name:      "infinibox-creds",
		Namespace: namespace,
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
	for index, treeqUser := range treeqUsers {
		csiSource.VolumeAttributes["volumePath"] = "/" + filesystem.Name + "/" + treeqUser
		csiSource.VolumeHandle = strconv.Itoa(filesystem.ID) + "#" + strconv.Itoa(treeqIDs[index]) + "$$" + common.PROTOCOL_TREEQ
		persistentVolume := &v1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{
				Name:      treeqUser + "-pv-" + config.TestNames.UniqueSuffix,
				Namespace: config.TestNames.NSName,
			},
			Spec: v1.PersistentVolumeSpec{
				Capacity:               resourceList,
				PersistentVolumeSource: persistentVolumeSource,
				AccessModes:            []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
				//PersistentVolumeReclaimPolicy: &scName,
				StorageClassName: config.TestNames.SCName,
				MountOptions:     []string{"hard", "rsize=1048576", "wsize=1048576"},
			},
		}

		_, err = config.ClientSet.CoreV1().PersistentVolumes().Create(context.TODO(), persistentVolume, metav1.CreateOptions{})
		if err != nil {
			return err
		}
		config.Testt.Logf("✓ PV %s is created\n", persistentVolume.Name)
	}
	return nil
}

func CreatePersistentVolumeClaimsForTreeqs(config *e2e.TestConfig) (err error) {
	config.UsePVCVolumeRef = true

	for _, treeqUser := range treeqUsers {
		config.TestNames.PVName = treeqUser + "-pv-" + config.TestNames.UniqueSuffix
		config.TestNames.PVCName = treeqUser + "-pvc"

		err = e2e.CreatePVC(config)
		if err != nil {
			return err
		}
		config.Testt.Logf("✓ PVC %s is created\n", config.TestNames.PVCName)
	}
	return nil
}

func CleanupAdminTreeqs(fileSystemID int, config *e2e.TestConfig) (err error) {
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
	for index := range treeqUsers {
		// delete apps
		ctx := context.Background()
		podName := treeqUsers[index] + "-app"
		err := e2e.DeletePod(ctx, config.TestNames.NSName, podName, config.ClientSet)
		if err != nil {
			fmt.Printf("error deleting pod %s %s\n", podName, err.Error())
		}
		config.Testt.Logf("✓ Pod %s is deleted\n", podName)
		// delete pvcs
		pvcName := treeqUsers[index] + "-pvc"
		err = e2e.DeletePVC(ctx, config.TestNames.NSName, pvcName, config.ClientSet)
		if err != nil {
			fmt.Printf("error deleting pvc %s %s\n", pvcName, err.Error())
		}
		config.Testt.Logf("✓ PVC %s is deleted\n", pvcName)
		// delete pvs
		pvName := treeqUsers[index] + "-pv-" + config.TestNames.UniqueSuffix
		err = e2e.DeletePV(ctx, pvName, config.ClientSet)
		if err != nil {
			fmt.Printf("error deleting pv %s %s\n", pvName, err.Error())
		}
		config.Testt.Logf("✓ PV %s is deleted\n", pvName)
	}
	thisMap := make(map[string]string)
	secrets := map[string]string{
		"hostname": hostname,
		"password": password,
		"username": username,
	}

	client := api.ClientService{
		ConfigMap:  thisMap,
		SecretsMap: secrets,
	}

	clientService, err := client.NewClient()
	if err != nil {
		return err
	}
	// delete filesystem
	err = clientService.DeleteFileSystemComplete(fileSystemID)
	if err != nil {
		fmt.Printf("error deleting filesystem %d %s\n", fileSystemID, err.Error())
	}
	config.Testt.Logf("✓ Filesystem %d is deleted\n", fileSystemID)
	return nil
}

func CreateTreeqApps(config *e2e.TestConfig) (err error) {
	for _, treeqUser := range treeqUsers {
		config.TestNames.PVCName = treeqUser + "-pvc"
		podName := treeqUser + "-app"

		err = e2e.CreatePod(config, config.TestNames.NSName, podName)
		if err != nil {
			return err
		}
		config.Testt.Logf("✓ Pod %s is created\n", podName)
	}
	return nil
}
