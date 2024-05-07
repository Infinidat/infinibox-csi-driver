//go:build e2e

package treeq

import (
	"context"
	"infinibox-csi-driver/e2e"
	"strconv"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
)

func TestTreeq(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, "treeq")
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

func TestFsGroupTreeq(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, "treeq")
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

	testConfig.UseFsGroup = true

	e2e.Setup(testConfig)

	expectedValue := "drwxrwsr-x"

	winning, actual, err := e2e.VerifyDirPermsCorrect(testConfig.ClientSet, testConfig.RestConfig, e2e.POD_NAME, testConfig.TestNames.NSName, expectedValue)
	if err != nil {
		t.Fatalf("error in VerifyDirPermsCorrect check %s", err.Error())
	}

	if winning {
		t.Log("FSGroupDirPermsCorrect PASSED")
	} else {
		t.Errorf("FSGroupDirPermsCorrect FAILED, expected: %s but got %s", expectedValue, actual)
	}

	expectedValue = strconv.Itoa(e2e.POD_FS_GROUP)
	winning, actual, err = e2e.VerifyGroupIdIsUsed(testConfig.ClientSet, testConfig.RestConfig, e2e.POD_NAME, testConfig.TestNames.NSName, expectedValue)
	if err != nil {
		t.Fatalf("error in VerifyGroupIdIsUsed check %s", err.Error())
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

func TestTreeqAdmin(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, "treeq")
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

	e2e.Setup(testConfig)

	fileSystemID, err := CreateAdminTreeqs(testConfig)
	if err != nil {
		t.Fatalf("CreateAdminTreeqs FAILED, got error %s", err.Error())
	}

	time.Sleep(time.Second * 30)

	err = VerifyAdminTreeqs(testConfig)
	if err != nil {
		t.Fatalf("VerifyAdminTreeqs FAILED, got error %s", err.Error())
	}

	if *e2e.CleanUp {
		err := CleanupAdminTreeqs(fileSystemID, testConfig)
		if err != nil {
			t.Errorf("CleanupAdminTreeqs FAILED, got error %s", err.Error())
		}
		e2e.TearDown(testConfig)
	} else {
		t.Log("not cleaning up namespace")
	}
}

func TestTreeqRO(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, "treeq")
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

	e2e.Setup(testConfig)

	fileSystemID, err := CreateAdminTreeqs(testConfig)
	if err != nil {
		t.Fatalf("CreateAdminTreeqs FAILED, got error %s", err.Error())
	}

	time.Sleep(time.Second * 30)

	err = VerifyAdminTreeqs(testConfig)
	if err != nil {
		t.Fatalf("VerifyAdminTreeqs FAILED, got error %s", err.Error())
	}

	// delete the user1 app pod
	user1PodName := "user1-app"
	ctx := context.Background()
	err = e2e.DeletePod(ctx, testConfig.TestNames.NSName, user1PodName, testConfig.ClientSet)
	if err != nil {
		t.Fatalf("DeletePod FAILED, got error %s", err.Error())
	}
	testConfig.Testt.Logf("✓ Pod %s is deleted", user1PodName)
	// create a readOnly user1 app pod with a readOnly on the volumeMount
	testConfig.ReadOnlyPod = true
	testConfig.ReadOnlyPodVolume = true

	err = e2e.CreatePod(testConfig, testConfig.TestNames.NSName, user1PodName)
	if err != nil {
		t.Fatalf("CreatePod FAILED, got error %s", err.Error())
	}
	testConfig.Testt.Logf("✓ ReadOnly Pod %s is created", user1PodName)

	// verify the user1 app pod is running and has a readonly mounted filesystem
	err = e2e.WaitForPod(testConfig.Testt, user1PodName, testConfig.TestNames.NSName, testConfig.ClientSet, time.Second*5, time.Minute*4)
	if err != nil {
		t.Fatalf("CreatePod FAILED, got error %s", err.Error())
	}
	testConfig.Testt.Logf("✓ ReadOnly Pod %s is ready", user1PodName)

	err = e2e.VerifyReadOnlyMount(testConfig.ClientSet, testConfig.RestConfig, user1PodName, testConfig.TestNames.NSName)
	if err != nil {
		t.Fatalf("VerifyReadOnlyMount FAILED, got error %s", err.Error())
	}
	testConfig.Testt.Logf("✓ ReadOnly Pod %s is readOnly on the mount", user1PodName)

	if *e2e.CleanUp {
		err := CleanupAdminTreeqs(fileSystemID, testConfig)
		if err != nil {
			t.Errorf("CleanupAdminTreeqs FAILED, got error %s", err.Error())
		}
		e2e.TearDown(testConfig)
	} else {
		t.Log("not cleaning up namespace")
	}
}

func TestTreeqROX(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, "treeq")
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

	e2e.Setup(testConfig)

	fileSystemID, err := CreateAdminTreeqs(testConfig)
	if err != nil {
		t.Fatalf("CreateAdminTreeqs FAILED, got error %s", err.Error())
	}

	time.Sleep(time.Second * 30)

	err = VerifyAdminTreeqs(testConfig)
	if err != nil {
		t.Fatalf("VerifyAdminTreeqs FAILED, got error %s", err.Error())
	}

	// delete user1 app pods
	user1PodName := "user1-app"
	user1PVCName := "user1-pvc"
	user2PodName := "user2-app"
	user2PVCName := "user2-pvc"

	ctx := context.Background()
	err = e2e.DeletePod(ctx, testConfig.TestNames.NSName, user1PodName, testConfig.ClientSet)
	if err != nil {
		t.Fatalf("DeletePod FAILED, got error %s", err.Error())
	}
	testConfig.Testt.Logf("✓ Pod %s is deleted", user1PodName)

	err = e2e.DeletePod(ctx, testConfig.TestNames.NSName, user2PodName, testConfig.ClientSet)
	if err != nil {
		t.Fatalf("DeletePod FAILED, got error %s", err.Error())
	}
	testConfig.Testt.Logf("✓ Pod %s is deleted", user2PodName)

	// delete user1 pvc
	testConfig.TestNames.PVCName = user1PVCName
	err = e2e.DeletePVC(ctx, testConfig.TestNames.NSName, testConfig.TestNames.PVCName, testConfig.ClientSet)
	if err != nil {
		t.Fatalf("error deleting PVC %s\n", err.Error())
	}
	testConfig.Testt.Logf("✓ PVC %s is deleted\n", testConfig.TestNames.PVCName)

	testConfig.TestNames.PVCName = user2PVCName
	err = e2e.DeletePVC(ctx, testConfig.TestNames.NSName, testConfig.TestNames.PVCName, testConfig.ClientSet)
	if err != nil {
		t.Fatalf("error deleting PVC %s\n", err.Error())
	}
	testConfig.Testt.Logf("✓ PVC %s is deleted\n", testConfig.TestNames.PVCName)
	time.Sleep(time.Second * 10)

	// update user1 pv
	user1PVName := "user1-pv-" + testConfig.TestNames.UniqueSuffix

	err = e2e.UpdatePV(ctx, user1PVName, testConfig.ClientSet)
	if err != nil {
		t.Fatalf("error updating PV %s\n", err.Error())
	}
	testConfig.Testt.Logf("✓ PV %s is updated\n", user1PVName)

	// create user1 pvc with ROX
	testConfig.AccessMode = v1.ReadOnlyMany
	testConfig.ReadOnlyPod = true
	testConfig.UsePVCVolumeRef = true

	err = CreatePersistentVolumeClaimsForTreeqs(testConfig)
	if err != nil {
		t.Fatalf("error creating PVCs for ROX %s\n", err.Error())
	}
	testConfig.Testt.Logf("✓ PVCs with ROX is created\n")

	err = CreateTreeqApps(testConfig)
	if err != nil {
		t.Fatalf("CreateTreeqApps FAILED for ROX, got error %s", err.Error())
	}

	testConfig.Testt.Logf("✓ ReadOnly Pods %s for ROX is created", user1PodName)

	err = e2e.WaitForPod(testConfig.Testt, "user1-app", testConfig.TestNames.NSName, testConfig.ClientSet, time.Second*5, time.Minute*4)
	if err != nil {
		t.Fatalf("CreatePod FAILED, got error %s", err.Error())
	}
	testConfig.Testt.Logf("✓ ReadOnly Pod %s is ready", "user1-app")

	// verify user1 app has readonly mount
	err = e2e.VerifyReadOnlyMount(testConfig.ClientSet, testConfig.RestConfig, "user1-app", testConfig.TestNames.NSName)
	if err != nil {
		t.Fatalf("VerifyReadOnlyMount FAILED, got error %s", err.Error())
	}
	testConfig.Testt.Logf("✓ ReadOnly Pod %s is readOnly on the mount", "user1-app")
	testConfig.Testt.Logf("filesystemid %d", fileSystemID)

	if *e2e.CleanUp {
		err := CleanupAdminTreeqs(fileSystemID, testConfig)
		if err != nil {
			t.Errorf("CleanupAdminTreeqs FAILED, got error %s", err.Error())
		}
		e2e.TearDown(testConfig)
	} else {
		t.Log("not cleaning up namespace")
	}
}
