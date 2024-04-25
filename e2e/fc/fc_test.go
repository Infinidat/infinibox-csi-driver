//go:build e2e

package fc

import (
	"infinibox-csi-driver/common"
	"infinibox-csi-driver/e2e"
	"strconv"
	"testing"

	v1 "k8s.io/api/core/v1"
)

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

	e2e.Setup(testConfig)

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
