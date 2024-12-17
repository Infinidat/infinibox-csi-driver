//go:build e2e

package nvme

import (
	"fmt"
	"infinibox-csi-driver/common"
	"infinibox-csi-driver/e2e"
	"testing"
	"time"
)

func TestNvme(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, common.PROTOCOL_NVME)
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

	e2e.Setup(testConfig)

	volumesToCreate := 15
	sleepBetweenPods := 15
	t.Logf("creating %d nvme volumes", volumesToCreate)

	originalPVCName := testConfig.TestNames.PVCName

	testConfig.UseFsGroup = true

	for i := 0; i < volumesToCreate; i++ {
		testConfig.TestNames.PVCName = fmt.Sprintf("%s-%d", originalPVCName, i)
		e2e.CreatePVC(testConfig)
		podName := testConfig.TestNames.PVCName
		e2e.CreatePod(testConfig, testConfig.TestNames.NSName, podName)
		time.Sleep(time.Second * time.Duration(sleepBetweenPods))
		t.Logf("creating nvme volume %d", i)
	}

	/**
	if *e2e.CleanUp {
		e2e.TearDown(testConfig)
		ctx := context.Background()
		for i := 0; i < volumesToCreate; i++ {
			testConfig.TestNames.PVCName = fmt.Sprintf("%s-%d", testConfig.TestNames.PVCName, i)
			e2e.DeletePVC(ctx, testConfig.TestNames.NSName, testConfig.TestNames.PVCName, testConfig.ClientSet)
			e2e.DeletePod(ctx, testConfig.TestNames.NSName, testConfig.TestNames.PVCName, testConfig.ClientSet)
			time.Sleep(time.Second * 5)
		}
	} else {
		t.Log("not cleaning up namespace")
	}
	*/

}
