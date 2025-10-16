//go:build e2e

package fcstress

import (
	"context"
	"fmt"
	"infinibox-csi-driver/common"
	"infinibox-csi-driver/e2e"
	"testing"
	"time"
)

func TestFc(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, common.PROTOCOL_FC)
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

	e2e.Setup(testConfig)

	originalPVCName := testConfig.TestNames.PVCName

	testConfig.UseFsGroup = true
	for i := range testConfig.StressIterations {
		testConfig.TestNames.PVCName = fmt.Sprintf("%s-%d", originalPVCName, i)
		t.Logf("creating pvc %s", testConfig.TestNames.PVCName)
		e2e.CreatePVC(testConfig)
		podName := testConfig.TestNames.PVCName
		t.Logf("creating pod %s", podName)
		e2e.CreatePod(testConfig, testConfig.TestNames.NSName, podName)
		time.Sleep(time.Second * time.Duration(testConfig.StressSleepSeconds))
	}

	if *e2e.CleanUp {
		ctx := context.Background()
		for i := range testConfig.StressIterations {
			testConfig.TestNames.PVCName = fmt.Sprintf("%s-%d", testConfig.TestNames.PVCName, i)
			t.Logf("deleting pod %s", testConfig.TestNames.PVCName)
			e2e.DeletePod(ctx, testConfig.TestNames.NSName, testConfig.TestNames.PVCName, testConfig.ClientSet)
			t.Logf("deleting pvc %s", testConfig.TestNames.PVCName)
			e2e.DeletePVC(ctx, testConfig.TestNames.NSName, testConfig.TestNames.PVCName, testConfig.ClientSet)
			time.Sleep(time.Second * 5)
		}
		testConfig.TestNames.PVCName = originalPVCName
		e2e.TearDown(testConfig)
	} else {
		t.Log("not cleaning up namespace")
	}

}
