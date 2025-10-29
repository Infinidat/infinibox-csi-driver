//go:build e2e

package fcstress

import (
	"fmt"
	"testing"
	"time"

	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/infinidat/infinibox-csi-driver/e2e"
)

func TestFc(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, common.ProtocolFC)
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

	e2e.Setup(t.Context(), testConfig)

	originalPVCName := testConfig.TestNames.PVCName

	testConfig.UseFsGroup = true
	for i := range testConfig.StressIterations {
		testConfig.TestNames.PVCName = fmt.Sprintf("%s-%d", originalPVCName, i)
		t.Logf("creating pvc %s", testConfig.TestNames.PVCName)
		e2e.CreatePVC(t.Context(), testConfig)
		podName := testConfig.TestNames.PVCName
		t.Logf("creating pod %s", podName)
		e2e.CreatePod(t.Context(), testConfig, testConfig.TestNames.NSName, podName)
		time.Sleep(time.Second * time.Duration(testConfig.StressSleepSeconds))
	}

	if *e2e.CleanUp {
		for i := range testConfig.StressIterations {
			testConfig.TestNames.PVCName = fmt.Sprintf("%s-%d", testConfig.TestNames.PVCName, i)
			t.Logf("deleting pod %s", testConfig.TestNames.PVCName)
			e2e.DeletePod(t.Context(), testConfig.TestNames.NSName, testConfig.TestNames.PVCName, testConfig.ClientSet)
			t.Logf("deleting pvc %s", testConfig.TestNames.PVCName)
			e2e.DeletePVC(t.Context(), testConfig.TestNames.NSName, testConfig.TestNames.PVCName, testConfig.ClientSet)
			time.Sleep(time.Second * 5)
		}
		testConfig.TestNames.PVCName = originalPVCName
		e2e.TearDown(t.Context(), testConfig)
	} else {
		t.Log("not cleaning up namespace")
	}

}
