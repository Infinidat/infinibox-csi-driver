//go:build e2e

package iscsi

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

func TestIscsiSnapshotLocking(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, common.PROTOCOL_ISCSI)
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

func TestIscsiSnapshot(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, common.PROTOCOL_ISCSI)
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

func TestIscsi(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, common.PROTOCOL_ISCSI)
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

func TestIscsiFsGroup(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, common.PROTOCOL_ISCSI)
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

	testConfig.UseFsGroup = true
	e2e.Setup(testConfig)

	time.Sleep(10 * time.Second) //sleep to avoid a race condition

	expectedValue := "drwxrwsr-x"
	winning, actual, err := e2e.VerifyDirPermsCorrect(testConfig.ClientSet, testConfig.RestConfig, e2e.POD_NAME, testConfig.TestNames.NSName, expectedValue)
	if err != nil {
		t.Fatalf("error verifying dir perms %s", err.Error())
	}

	if winning {
		t.Log("FSGroupDirPermsCorrect PASSED")
	} else {
		t.Errorf("FSGroupDirPermsCorrect FAILED, expected: %s but got %s", expectedValue, actual)
	}

	expectedValue = strconv.Itoa(e2e.POD_FS_GROUP)
	winning, actual, err = e2e.VerifyGroupIdIsUsed(testConfig.ClientSet, testConfig.RestConfig, e2e.POD_NAME, testConfig.TestNames.NSName, expectedValue)
	if err != nil {
		t.Fatalf("error in VerifyGroupIdIsUsed %s", err.Error())
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

}

func TestIscsiBlock(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, common.PROTOCOL_ISCSI)
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

func TestIscsiROX(t *testing.T) {
	testConfig, err := e2e.GetTestConfig(t, common.PROTOCOL_ISCSI)
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

	testConfig.UseRetainStorageClass = true
	e2e.Setup(testConfig)

	// get the PV name, we'll construct a 2nd PVC using that volume name for the ROX test

	pvName, err := e2e.GetPVName(testConfig.TestNames.PVCName, testConfig.TestNames.NSName, testConfig.ClientSet)
	if err != nil {
		t.Fatalf("error getting PV name %s\n", err.Error())
	}

	// delete the Pod and PVC, the PV will be retained because we set the StorageClass to Retain
	ctx := context.Background()
	err = e2e.DeletePod(ctx, testConfig.TestNames.NSName, e2e.POD_NAME, testConfig.ClientSet)
	if err != nil {
		t.Fatalf("error deleting Pod %s\n", err.Error())
	}

	err = e2e.DeletePVC(ctx, testConfig.TestNames.NSName, testConfig.TestNames.PVCName, testConfig.ClientSet)
	if err != nil {
		t.Fatalf("error deleting Pod %s\n", err.Error())
	}

	// we are going to reuse the PVC name, so give it some time to be deleted before reusing
	time.Sleep(time.Second * 10)

	// update the PV to use ROX access mode and remove the existing claimRef so that the new PVC can bind to it

	err = e2e.UpdatePV(ctx, pvName, testConfig.ClientSet)
	if err != nil {
		t.Fatalf("error updating PV %s\n", err.Error())
	}

	// create the ROX PVC using the PV from above as the volumeName
	testConfig.AccessMode = v1.ReadOnlyMany
	testConfig.ReadOnlyPod = true
	testConfig.UsePVCVolumeRef = true
	testConfig.TestNames.PVName = pvName

	err = e2e.CreatePVC(testConfig)
	if err != nil {
		t.Fatalf("error creating ROX PVC %s\n", err.Error())
	}

	readOnlyManyPodName := e2e.POD_NAME + "-rox"
	testConfig.UseSELinux = true

	err = e2e.CreatePod(testConfig, testConfig.TestNames.NSName, readOnlyManyPodName)
	if err != nil {
		t.Fatalf("error creating ROX Pod %s\n", err.Error())
	}

	err = e2e.WaitForPod(testConfig.Testt, readOnlyManyPodName, testConfig.TestNames.NSName, testConfig.ClientSet, time.Second*5, time.Minute*4)
	if err != nil {
		e2e.DescribePVC(testConfig.Testt, readOnlyManyPodName, testConfig.TestNames.NSName, testConfig.ClientSet)
		testConfig.Testt.Fatalf("error waiting for rox pod %s", err.Error())
	}

	testConfig.Testt.Logf("✓ Pod %s is running\n", readOnlyManyPodName)

	// lastly, verify that the mount is ro inside the running pod
	err = e2e.VerifyReadOnlyMount(testConfig.ClientSet, testConfig.RestConfig, readOnlyManyPodName, testConfig.TestNames.NSName)
	if err != nil {
		t.Errorf("error verifying read-only %s\n", err.Error())
		t.Fail()
	} else {
		testConfig.Testt.Logf("✓ Pod %s volume is mounted read only\n", readOnlyManyPodName)
	}

	err = e2e.DeletePod(ctx, testConfig.TestNames.NSName, readOnlyManyPodName, testConfig.ClientSet)
	if err != nil {
		t.Fatalf("error deleting rox pod %s\n", err.Error())
	}

	err = e2e.DeletePVC(ctx, testConfig.TestNames.NSName, testConfig.TestNames.PVCName, testConfig.ClientSet)
	if err != nil {
		t.Fatalf("error deleting rox pvc %s\n", err.Error())
	}

	// because of Retain being used, we delete the PV
	err = e2e.DeletePV(ctx, pvName, testConfig.ClientSet)
	if err != nil {
		t.Fatalf("error deleting PV %s\n", err.Error())
	}

	err = e2e.DeleteStorageClass(ctx, testConfig.TestNames.SCName, testConfig.ClientSet)
	if err != nil {
		t.Fatalf("error deleting StorageClass %s\n", err.Error())
	}

}

func TestIscsiRO(t *testing.T) {
	testConfig, err := e2e.GetTestConfig(t, common.PROTOCOL_ISCSI)
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

	testConfig.AccessMode = v1.ReadWriteMany
	e2e.Setup(testConfig)

	time.Sleep(time.Second * 20)

	err = e2e.DeletePod(context.Background(), testConfig.TestNames.NSName, e2e.POD_NAME, testConfig.ClientSet)
	if err != nil {
		testConfig.Testt.Fatalf("error deleting test pod %s", err.Error())
	}
	testConfig.Testt.Logf("✓ Pod %s is deleted\n", e2e.POD_NAME)

	readOnlyPodName := e2e.POD_NAME + "-ro"
	testConfig.ReadOnlyPodVolume = true
	testConfig.ReadOnlyPod = true

	err = e2e.CreatePod(testConfig, testConfig.TestNames.NSName, readOnlyPodName)
	if err != nil {
		testConfig.Testt.Fatalf("error creating test pod %s", err.Error())
	}
	testConfig.Testt.Logf("✓ Pod %s is created\n", readOnlyPodName)

	err = e2e.WaitForPod(testConfig.Testt, readOnlyPodName, testConfig.TestNames.NSName, testConfig.ClientSet, time.Second*5, time.Minute*4)
	if err != nil {
		e2e.DescribePVC(testConfig.Testt, readOnlyPodName, testConfig.TestNames.NSName, testConfig.ClientSet)
		testConfig.Testt.Fatalf("error waiting for pod %s", err.Error())
	}
	testConfig.Testt.Logf("✓ Pod %s is running\n", readOnlyPodName)

	// lastly, verify that the mount is ro inside the running pod
	err = e2e.VerifyReadOnlyMount(testConfig.ClientSet, testConfig.RestConfig, readOnlyPodName, testConfig.TestNames.NSName)
	if err != nil {
		t.Errorf("error verifying read-only %s\n", err.Error())
		t.Fail()
	} else {
		testConfig.Testt.Logf("✓ Pod %s volume is mounted read only\n", readOnlyPodName)
	}

	// delete the ro pod
	err = e2e.DeletePod(context.Background(), testConfig.TestNames.NSName, readOnlyPodName, testConfig.ClientSet)
	if err != nil {
		testConfig.Testt.Logf("error deleting pod %s\n", err.Error())
	}
	testConfig.Testt.Logf("✓ pod %s is deleted\n", readOnlyPodName)

	if *e2e.CleanUp {
		e2e.TearDown(testConfig)
	} else {
		t.Log("not cleaning up namespace")
	}

}

func TestIscsiBrokenLink(t *testing.T) {

	// we use the fsgroup example as the basis for this test since its where
	// recursive chown is executed which will test this use case (broken sym link)

	testConfig, err := e2e.GetTestConfig(t, common.PROTOCOL_ISCSI)
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

	testConfig.UseFsGroup = true
	e2e.Setup(testConfig)

	time.Sleep(10 * time.Second) //sleep to avoid a race condition

	expectedValue := "drwxrwsr-x"
	winning, actual, err := e2e.VerifyDirPermsCorrect(testConfig.ClientSet, testConfig.RestConfig, e2e.POD_NAME, testConfig.TestNames.NSName, expectedValue)
	if err != nil {
		t.Fatalf("error verifying dir perms %s", err.Error())
	}

	if winning {
		t.Log("FSGroupDirPermsCorrect PASSED")
	} else {
		t.Errorf("FSGroupDirPermsCorrect FAILED, expected: %s but got %s", expectedValue, actual)
	}

	expectedValue = strconv.Itoa(e2e.POD_FS_GROUP)
	winning, actual, err = e2e.VerifyGroupIdIsUsed(testConfig.ClientSet, testConfig.RestConfig, e2e.POD_NAME, testConfig.TestNames.NSName, expectedValue)
	if err != nil {
		t.Fatalf("error in VerifyGroupIdIsUsed %s", err.Error())
	}

	if winning {
		t.Log("FSGroupIdIsUsed PASSED")
	} else {
		t.Errorf("FsGroupIdIsUsed FAILED, expected: %s but got %s", expectedValue, actual)
	}

	// exec into pod and create a broken link
	err = e2e.CreateLinks(testConfig.ClientSet, testConfig.RestConfig, e2e.POD_NAME, testConfig.TestNames.NSName)
	if err != nil {
		t.Fatalf("error in CreateLinks %s", err.Error())
	}

	// we have created a broken link on the mounted volume at this point, we will
	// shut down the pod and restart it to cause SetPermissions() to be called which will
	// test whether or not it handles the broken link, allowing chown/chmod to proceed and skip over
	// the broken link

	// shut down pod
	err = e2e.DeletePod(context.TODO(), testConfig.TestNames.NSName, e2e.POD_NAME, testConfig.ClientSet)
	if err != nil {
		t.Fatalf("error in deleting pod %s", err.Error())
	}
	// restart pod
	err = e2e.CreatePod(testConfig, testConfig.TestNames.NSName, e2e.POD_NAME)
	if err != nil {
		t.Fatalf("error in recreating pod %s", err.Error())
	}
	// verify pod is up and running
	err = e2e.WaitForPod(t, e2e.POD_NAME, testConfig.TestNames.NSName, testConfig.ClientSet, time.Second*5, time.Minute*2)
	if err != nil {
		t.Fatalf("error in recreated pod from starting %s", err.Error())
	}

	if *e2e.CleanUp {
		e2e.TearDown(testConfig)
	} else {
		t.Log("not cleaning up namespace")
	}

}

func TestIscsiClone(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, common.PROTOCOL_ISCSI)
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

func TestIscsiExpand(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, common.PROTOCOL_ISCSI)
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

	e2e.Setup(testConfig)

	var originalSize int64
	originalSize, _, err = e2e.ExpandPVC(t, testConfig)
	if err != nil {
		t.Fatalf("error expanding existing PVC %s", err.Error())
	}

	// wait an undetermined amount of time for the filesystem to be expanded inside the running pod
	time.Sleep(time.Second * 90)

	mountSize, err := e2e.GetMountSize(common.PROTOCOL_ISCSI, testConfig.ClientSet, testConfig.RestConfig, e2e.POD_NAME, testConfig.TestNames.NSName)
	if err != nil {
		t.Fatalf("error execing into pod %s", err.Error())
	}

	if mountSize <= originalSize {
		t.Logf("error expected updates size %d to have been expanded beyond original size %d in pod", mountSize, originalSize)
		t.FailNow()
	}
	t.Logf("volume was expanded in pod size matches %d ", mountSize)

	if *e2e.CleanUp {
		e2e.TearDown(testConfig)
	} else {
		t.Log("not cleaning up namespace")
	}

}

func TestIscsiBlockExpand(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, common.PROTOCOL_ISCSI)
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

	testConfig.UseBlock = true
	e2e.Setup(testConfig)

	var originalSize int64
	var expandedSize int64
	originalSize, expandedSize, err = e2e.ExpandPVC(t, testConfig)
	if err != nil {
		t.Fatalf("error expanding existing PVC %s", err.Error())
	}

	t.Logf("original PVC size %d", originalSize)

	// wait an undetermined amount of time for the block device to be expanded inside the running pod
	time.Sleep(time.Second * 90)

	var resultingSize int64

	resultingSize, err = e2e.GetBlockVolumeSize(testConfig.ClientSet, testConfig.RestConfig, e2e.POD_NAME, testConfig.TestNames.NSName)
	if err != nil {
		t.Fatalf("error getting block volume size %s", err.Error())
	}
	t.Logf("resulting expanded block device size %d", resultingSize)

	if resultingSize != expandedSize {
		t.Fatalf("error expected expandedSize %d doesnt match resulting size %d", expandedSize, resultingSize)
	}
	t.Logf("expected block device size %d matches PVC size", resultingSize)

	if *e2e.CleanUp {
		e2e.TearDown(testConfig)
	} else {
		t.Log("not cleaning up namespace")
	}

}
