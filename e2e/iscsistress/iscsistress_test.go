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

package iscsistress

import (
	"fmt"
	"testing"
	"time"

	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/infinidat/infinibox-csi-driver/e2e"
)

func TestIscsi(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, common.ProtocolISCSI)
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

	e2e.Setup(t.Context(), testConfig)

	volumesToCreate := 20

	originalPVCName := testConfig.TestNames.PVCName

	testConfig.UseFsGroup = true

	for i := range volumesToCreate {
		testConfig.TestNames.PVCName = fmt.Sprintf("%s-%d", originalPVCName, i)
		e2e.CreatePVC(t.Context(), testConfig)
		podName := testConfig.TestNames.PVCName
		e2e.CreatePod(t.Context(), testConfig, testConfig.TestNames.NSName, podName)
		time.Sleep(time.Second * 15)
		t.Logf("creating volume %d", i)
	}

	/**
	if *e2e.CleanUp {
		e2e.TearDown(testConfig)
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
