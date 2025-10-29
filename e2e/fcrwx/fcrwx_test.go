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
