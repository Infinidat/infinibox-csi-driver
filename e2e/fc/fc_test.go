//go:build e2e

package fc

import (
	"context"
	"infinibox-csi-driver/common"
	"infinibox-csi-driver/e2e"
	"strconv"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "k8s.io/api/core/v1"
)

func TestFcSnapshotLocking(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, common.PROTOCOL_FC)
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

	testConfig.UseSnapshotLock = true

	e2e.Setup(testConfig)

	time.Sleep(time.Second * 5)

	err = e2e.CreateSnapshot(testConfig.TestNames.PVCName, testConfig.TestNames.VSCName, testConfig.TestNames.NSName, testConfig.SnapshotClient)
	if err != nil {
		t.Fatalf("error creating volumesnapshot pod %s", err.Error())
	}

	time.Sleep(time.Second * 5)

	err = e2e.WaitForSnapshot(t, e2e.SNAPSHOT_NAME, testConfig.TestNames.NSName, testConfig.SnapshotClient)
	if err != nil {
		t.Fatalf("error waiting for volumesnapshot %s", err.Error())
	}

	// the snapshot should be locked so this delete should not work
	err = e2e.DeleteVolumeSnapshot(context.Background(), testConfig.TestNames.NSName, e2e.SNAPSHOT_NAME, testConfig.SnapshotClient)
	if err != nil {
		testConfig.Testt.Logf("error deleting volume snapshot %s\n", err.Error())
	}
	t.Log("delete attempted of VolumeSnapshot")

	// you should be able to get the snapshot since it was not deleted
	err = e2e.GetVolumeSnapshot(context.Background(), testConfig.TestNames.NSName, e2e.SNAPSHOT_NAME, testConfig.SnapshotClient)
	if err != nil {
		t.Fatalf("error getting volumesnapshot %s", err.Error())
	}
	t.Log("got locked VolumeSnapshot, locking logic worked")

	if *e2e.CleanUp {
		e2e.TearDown(testConfig)
	} else {
		t.Log("not cleaning up namespace")
	}

}

func TestFcSnapshot(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, common.PROTOCOL_FC)
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

	testConfig.UseSnapshot = true

	e2e.Setup(testConfig)

	time.Sleep(time.Second * 5)

	err = e2e.CreateSnapshot(testConfig.TestNames.PVCName, testConfig.TestNames.VSCName, testConfig.TestNames.NSName, testConfig.SnapshotClient)
	if err != nil {
		t.Fatalf("error creating volumesnapshot pod %s", err.Error())
	}

	time.Sleep(time.Second * 5)

	err = e2e.WaitForSnapshot(t, e2e.SNAPSHOT_NAME, testConfig.TestNames.NSName, testConfig.SnapshotClient)
	if err != nil {
		t.Fatalf("error waiting for volumesnapshot %s", err.Error())
	}

	if *e2e.CleanUp {
		e2e.TearDown(testConfig)
	} else {
		t.Log("not cleaning up namespace")
	}

}

func TestFc(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, common.PROTOCOL_FC)
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

	e2e.Setup(testConfig)

	if *e2e.CleanUp {
		e2e.TearDown(testConfig)
	} else {
		t.Log("not cleaning up namespace")
	}

}

func TestFsGroupFc(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, common.PROTOCOL_FC)
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

	testConfig.UseFsGroup = true

	e2e.Setup(testConfig)

	expectedValue := "drwxrwsr-x"
	winning, actual, err := e2e.VerifyDirPermsCorrect(testConfig.ClientSet, testConfig.RestConfig, e2e.POD_NAME, testConfig.TestNames.NSName, expectedValue)
	if err != nil {
		t.Errorf("error verifying directory permissions %s", err.Error())
	}

	if winning {
		t.Log("FSGroupDirPermsCorrect PASSED")
	} else {
		t.Errorf("FSGroupDirPermsCorrect FAILED, expected: %s but got %s", expectedValue, actual)
	}

	expectedValue = strconv.Itoa(e2e.POD_FS_GROUP)
	winning, actual, err = e2e.VerifyGroupIdIsUsed(testConfig.ClientSet, testConfig.RestConfig, e2e.POD_NAME, testConfig.TestNames.NSName, expectedValue)
	if err != nil {
		t.Fatalf("error in VerifygroupIdIsUsed %s", err.Error())
	}

	if winning {
		t.Log("FSGroupIdIsUsed PASSED")
	} else {
		t.Errorf("FsGroupIdIsUsed FAILED, expected: %s but got %s", expectedValue, actual)
	}

	if *e2e.CleanUp {
		e2e.TearDown(testConfig)
	} else {
		t.Log("not cleaning up namespace")
	}
	e2e.TearDown(testConfig)

}

func TestFcBlock(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, common.PROTOCOL_FC)
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

	testConfig.UseBlock = true

	e2e.Setup(testConfig)

	if *e2e.CleanUp {
		e2e.TearDown(testConfig)
	} else {
		t.Log("not cleaning up namespace")
	}

}

func TestFcBlockRWX(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, common.PROTOCOL_FC)
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

	nodeCount := e2e.GetTestSystemNodecount(t, testConfig.ClientSet)

	if nodeCount < 2 {
		t.Fatalf("System needs at least 2 nodes and only has %d ", nodeCount)
	}

	testConfig.AccessMode = v1.ReadWriteMany
	testConfig.UseAntiAffinity = true
	testConfig.UseBlock = true

	e2e.Setup(testConfig)

	time.Sleep(10)

	firstSuccess, _, err := e2e.VerifyBlockWriteInPod(testConfig.ClientSet, testConfig.RestConfig, e2e.POD_NAME, testConfig.TestNames.NSName)
	if err != nil {
		t.Fatalf("Verify Block Write In Pod had unexpected error %s", err.Error())
	}

	if !firstSuccess {
		t.Fatalf("Test of Blockwrite in %s pod failed.", e2e.POD_NAME)
	}

	secondSuccess, _, err := e2e.VerifyBlockWriteInPod(testConfig.ClientSet, testConfig.RestConfig, e2e.ANTI_AF_POD_NAME, testConfig.TestNames.NSName)
	if err != nil {
		t.Fatalf("Verify Block Write In Pod had unexpected error %s", err.Error())
	}

	if !secondSuccess {
		t.Fatalf("Test of Blockwrite in %s pod failed.", e2e.POD_NAME)
	}

	if *e2e.CleanUp {
		e2e.TearDown(testConfig)
	} else {
		t.Logf("not cleaning up namespace")
	}

}

func TestFcClone(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, common.PROTOCOL_FC)
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

	e2e.Setup(testConfig)

	// create a PVC that references the previously created PVC
	// this is what a clone is, a PVC based off of an existing PVC

	existingPVC, err := testConfig.ClientSet.CoreV1().PersistentVolumeClaims(testConfig.TestNames.NSName).Get(context.TODO(), testConfig.TestNames.PVCName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("error getting existing PVC %s", err.Error())
	}

	clonePVC := existingPVC
	ds := v1.TypedLocalObjectReference{
		Kind: "PersistentVolumeClaim",
		Name: testConfig.TestNames.PVCName,
	}
	clonePVC.Spec.DataSource = &ds
	clonePVC.ObjectMeta = metav1.ObjectMeta{
		Name:      testConfig.TestNames.PVCName + "-clone",
		Namespace: testConfig.TestNames.NSName,
	}
	clonePVC.Spec.VolumeMode = nil
	clonePVC.Spec.VolumeName = ""

	_, err = testConfig.ClientSet.CoreV1().PersistentVolumeClaims(testConfig.TestNames.NSName).Create(context.TODO(), clonePVC, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("error creating clone PVC %s", err.Error())
	}

	err = e2e.WaitForPVC(t, clonePVC.ObjectMeta.Name, testConfig.TestNames.NSName, testConfig.ClientSet, time.Second*5, time.Minute*1)
	if err != nil {
		t.Fatalf("error waiting on clone PVC %s", err.Error())
	}

	if *e2e.CleanUp {
		e2e.TearDown(testConfig)
	} else {
		t.Log("not cleaning up namespace")
	}

}
