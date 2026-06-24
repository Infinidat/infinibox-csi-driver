/*
Copyright 2026 Infinidat
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

//go:build e2e

package nfsstress

import (
	"fmt"
	"testing"
	"time"

	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/infinidat/infinibox-csi-driver/e2e"
)

func TestNfs(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, common.ProtocolNFS)
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

	e2e.Setup(t.Context(), testConfig)

	originalPVCName := testConfig.TestNames.PVCName

	// testConfig.UseFsGroup = true

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
			testConfig.TestNames.PVCName = fmt.Sprintf("%s-%d", originalPVCName, i)
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
