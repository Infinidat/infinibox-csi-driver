//go:build e2e

package iscsipromote

import (
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/infinidat/infinibox-csi-driver/api/clientgo"
	v1 "github.com/infinidat/infinibox-csi-driver/iboxpromote/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/infinidat/infinibox-csi-driver/e2e"
)

func TestIscsiPromote(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, common.ProtocolISCSI)
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

	testConfig.UseSnapshot = true

	e2e.Setup(t.Context(), testConfig)

	// get the volume name from the PVC, which will be the PV name
	existingPVC, err := testConfig.ClientSet.CoreV1().PersistentVolumeClaims(testConfig.TestNames.NSName).Get(t.Context(), testConfig.TestNames.PVCName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("error getting existing PVC %s", err.Error())
	}

	t.Logf("existing PVC %s has %s volume", existingPVC.Name, existingPVC.Spec.VolumeName)

	secretName := os.Getenv(e2e.ENV_IBOX_SECRET)
	if secretName == "" {
		t.Fatalf("%s env var is not set and is required for this test", e2e.ENV_IBOX_SECRET)
	}
	secretNamespace := os.Getenv(e2e.ENV_NAMESPACE)
	if secretNamespace == "" {
		t.Fatalf("%s env var is not set and is required for this test", e2e.ENV_NAMESPACE)
	}

	time.Sleep(time.Second * 5)

	err = e2e.CreateSnapshot(t.Context(), testConfig.TestNames.PVCName, testConfig.TestNames.VSCName, testConfig.TestNames.NSName, testConfig.SnapshotClient)
	if err != nil {
		t.Fatalf("error creating volumesnapshot pod %s", err.Error())
	}

	time.Sleep(time.Second * 5)

	err = e2e.WaitForSnapshot(t, e2e.SNAPSHOT_NAME, testConfig.TestNames.NSName, testConfig.SnapshotClient)
	if err != nil {
		t.Fatalf("error waiting for volumesnapshot %s", err.Error())
	}

	// look up the ibox volume name for the snapshot just created, we pass
	// that into the iboxpromote spec as the 'snapshot' name

	// 1 - get the volumesnapshot
	getOptions := metav1.GetOptions{}
	volumeSnapshot, err := testConfig.SnapshotClient.SnapshotV1().VolumeSnapshots(testConfig.TestNames.NSName).Get(t.Context(), e2e.SNAPSHOT_NAME, getOptions)
	if err != nil {
		t.Fatalf("error getting volumesnapshot %s", err.Error())
	}
	// 2 - get the volumesnapshotcontent
	vscName := volumeSnapshot.Status.BoundVolumeSnapshotContentName
	if vscName == nil {
		t.Fatal("error volumeSnapshotContentName is nil")
	}
	volumeSnapshotContent, err := testConfig.SnapshotClient.SnapshotV1().VolumeSnapshotContents().Get(t.Context(), *vscName, getOptions)
	if err != nil {
		t.Fatalf("error getting volumesnapshot %s", err.Error())
	}

	// 3 - look up the volume using the volume handle in the volumesnapshotcontent
	snapshotHandle := volumeSnapshotContent.Status.SnapshotHandle
	if snapshotHandle == nil {
		t.Fatal("error volumeSnapshotContentName snapshotHandle is nil")
	}

	handleParts := strings.Split(*snapshotHandle, "$$")
	if len(handleParts) != 2 {
		t.Fatalf("error snapshot Handle is not formatted correctly %v", handleParts)
	}
	volumeIDString := handleParts[0]
	volumeID, err := strconv.Atoi(volumeIDString)
	if err != nil {
		t.Fatalf("error volumeID incorrect integer conversion %s - %s", err.Error(), volumeIDString)
	}

	volume, err := testConfig.ClientService.IboxAPI.GetVolume(t.Context(), volumeID)
	if err != nil {
		t.Fatalf("error getting snapshot Volume %s - %d", err.Error(), volumeID)
	}
	t.Logf("got volume for snapshot %s", volume.Name)
	promoteName := "iboxpromote-volume-e2e-test-" + testConfig.TestNames.UniqueSuffix

	// create the iboxpromote CR
	promoteCR := v1.Iboxpromote{
		ObjectMeta: metav1.ObjectMeta{
			Name: promoteName,
			Annotations: map[string]string{
				common.PVCAnnotationSecretName:      secretName,
				common.PVCAnnotationSecretNamespace: secretNamespace,
			},
		},
		Spec: v1.IboxpromoteSpec{
			Description: "iscsi-test-volume-promote",
			EntityType:  "SNAPSHOT",
			EntityName:  volume.Name,
			BaseAction:  "NEW",
		},
	}
	t.Logf("creating iboxpromote %s", promoteCR.Name)

	kclient, err := clientgo.BuildOffClusterClient(e2e.KubeConfigPath)
	if err != nil {
		t.Fatalf("error getting cluster client %s", err.Error())
	}
	err = kclient.CreateIboxpromote(t.Context(), promoteCR)
	if err != nil {
		t.Fatalf("error creating iboxpromote %s", err.Error())
	}

	// wait for promote status
	t.Logf("sleeping 50s after creating iboxpromote %s", promoteCR.Name)
	time.Sleep(time.Second * 50)

	runningPromote, err := kclient.GetIboxpromote(t.Context(), promoteName)
	if err != nil {
		t.Fatalf("error getting iboxpromote %s", err.Error())
	}
	t.Logf("iboxpromote created ID %d State %s", runningPromote.Status.ID, runningPromote.Status.State)
	if runningPromote.Status.State != "completed" {
		t.Fatalf("error iboxpromote state is not completed %s", runningPromote.Status.State)
	}

	if *e2e.CleanUp {
		e2e.TearDown(t.Context(), testConfig)
		// delete the iboxpromote
		err := kclient.DeleteIboxpromote(t.Context(), runningPromote)
		if err != nil {
			t.Fatalf("error deleting iboxpromote %s", err.Error())
		}
	} else {
		t.Log("not cleaning up namespace")
	}
	err = e2e.CleanISCI(t.Context(), *testConfig)
	if err != nil {
		t.Logf("error cleaning ISCSI %s on node %s\n", err.Error(), testConfig.NodeName)
	}

}
