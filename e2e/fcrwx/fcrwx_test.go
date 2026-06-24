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

package fcrwx

import (
	"testing"
	"time"

	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/infinidat/infinibox-csi-driver/e2e"

	v1 "k8s.io/api/core/v1"
)

func TestFcBlockRWX(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, common.ProtocolFC)
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

	nodeCount := e2e.GetTestSystemNodecount(t, testConfig.ClientSet)

	if nodeCount < 2 {
		t.Fatalf("System needs at least 2 nodes and only has %d ", nodeCount)
	}

	testConfig.AccessMode = v1.ReadWriteMany
	testConfig.UseAntiAffinity = true
	testConfig.UseBlock = true

	e2e.Setup(t.Context(), testConfig)

	time.Sleep(time.Second * 10)

	firstSuccess, _, err := e2e.VerifyBlockWriteInPod(t.Context(), testConfig.ClientSet, testConfig.RestConfig, e2e.POD_NAME, testConfig.TestNames.NSName)
	if err != nil {
		t.Fatalf("Verify Block Write In Pod had unexpected error %s", err.Error())
	}

	if !firstSuccess {
		t.Fatalf("Test of Blockwrite in %s pod failed.", e2e.POD_NAME)
	}

	secondSuccess, _, err := e2e.VerifyBlockWriteInPod(t.Context(), testConfig.ClientSet, testConfig.RestConfig, e2e.ANTI_AF_POD_NAME, testConfig.TestNames.NSName)
	if err != nil {
		t.Fatalf("Verify Block Write In Pod had unexpected error %s", err.Error())
	}

	if !secondSuccess {
		t.Fatalf("Test of Blockwrite in %s pod failed.", e2e.POD_NAME)
	}

	if *e2e.CleanUp {
		e2e.TearDown(t.Context(), testConfig)
	} else {
		t.Logf("not cleaning up namespace")
	}

}
