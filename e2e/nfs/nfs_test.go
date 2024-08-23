//go:build e2e

package nfs

import (
	"context"
	"fmt"
	"infinibox-csi-driver/common"
	"infinibox-csi-driver/e2e"
	"strconv"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
)

func TestNfsSnapshotLocking(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, common.PROTOCOL_NFS)
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

func TestNfsSnapshot(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, common.PROTOCOL_NFS)
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

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

func TestNfs(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, common.PROTOCOL_NFS)
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

	fmt.Printf("jeff here in TestNfs vsc = [%s]\n", testConfig.TestNames.VSCName)
	e2e.Setup(testConfig)

	if *e2e.CleanUp {
		e2e.TearDown(testConfig)
	} else {
		t.Log("not cleaning up namespace")
	}
}

func TestNfsFsGroup(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, common.PROTOCOL_NFS)
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

	testConfig.UseFsGroup = true

	e2e.Setup(testConfig)

	expectedValue := "drwxrwsr-x"
	winning, actual, err := e2e.VerifyDirPermsCorrect(testConfig.ClientSet, testConfig.RestConfig, e2e.POD_NAME, testConfig.TestNames.NSName, expectedValue)
	if err != nil {
		t.Fatalf("error verifying dir perms  %s", err.Error())
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

func TestNfsROX(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, common.PROTOCOL_NFS)
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

	testConfig.AccessMode = v1.ReadOnlyMany
	testConfig.ReadOnlyPod = true

	e2e.Setup(testConfig)

	err = e2e.VerifyReadOnlyMount(testConfig.ClientSet, testConfig.RestConfig, e2e.POD_NAME, testConfig.TestNames.NSName)
	if err != nil {
		t.Errorf("error verifying read-only %s\n", err.Error())
		t.Fail()
	}

	if *e2e.CleanUp {
		e2e.TearDown(testConfig)
	} else {
		t.Log("not cleaning up namespace")
	}
}

func TestNfsRO(t *testing.T) {
	testConfig, err := e2e.GetTestConfig(t, common.PROTOCOL_NFS)
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

	testConfig.ReadOnlyPod = true
	testConfig.ReadOnlyPodVolume = true

	e2e.Setup(testConfig)

	err = e2e.VerifyReadOnlyMount(testConfig.ClientSet, testConfig.RestConfig, e2e.POD_NAME, testConfig.TestNames.NSName)
	if err != nil {
		t.Errorf("error verifying read-only %s\n", err.Error())
		t.Fail()
	}

	if *e2e.CleanUp {
		e2e.TearDown(testConfig)
	} else {
		t.Log("not cleaning up namespace")
	}
}

func TestNfsFsGroupWithSnapdir(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, common.PROTOCOL_NFS)
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

	testConfig.UseFsGroup = true
	testConfig.UseSnapdirVisible = true

	e2e.Setup(testConfig)

	expectedValue := "drwxrwsr-x"
	winning, actual, err := e2e.VerifyDirPermsCorrect(testConfig.ClientSet, testConfig.RestConfig, e2e.POD_NAME, testConfig.TestNames.NSName, expectedValue)
	if err != nil {
		t.Fatalf("error verifying dir perms  %s", err.Error())
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
