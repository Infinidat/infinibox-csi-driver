//go:build e2e

package treeqanno

import (
	"os"
	"testing"

	"github.com/infinidat/infinibox-csi-driver/e2e"
)

func TestTreeq(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, "treeq")
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

	iboxSecret := os.Getenv("_E2E_IBOX_SECRET")
	if iboxSecret == "" {
		t.Fatalf("error - _E2E_IBOX_SECRET env var is required for this test")
	}
	networkSpace := os.Getenv("_E2E_NETWORK_SPACE")
	if networkSpace == "" {
		t.Fatalf("error - _E2E_NETWORK_SPACE env var is required for this test")
	}
	poolName := os.Getenv("_E2E_POOL")
	if poolName == "" {
		t.Fatalf("error - _E2E_POOL env var is required for this test")
	}
	pvcAnnotations := &e2e.PVCAnnotations{
		IboxNetworkSpace: networkSpace,
		IboxPool:         poolName,
		IboxSecret:       iboxSecret,
	}

	testConfig.PVCAnnotations = pvcAnnotations

	e2e.Setup(t.Context(), testConfig)

	if *e2e.CleanUp {
		e2e.TearDown(t.Context(), testConfig)
	} else {
		t.Log("not cleaning up namespace")
	}
}
