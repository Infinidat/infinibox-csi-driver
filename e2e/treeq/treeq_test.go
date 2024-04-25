//go:build e2e

package treeq

import (
	"infinibox-csi-driver/e2e"
	"strconv"
	"testing"
	"time"
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
