//go:build e2e

package fcanno

import (
	"os"
	"testing"

	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/infinidat/infinibox-csi-driver/e2e"
)

func TestFc(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, common.ProtocolFC)
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

	iboxSecret := os.Getenv("_E2E_IBOX_SECRET")
	if iboxSecret == "" {
		t.Fatalf("error - _E2E_IBOX_SECRET env var is required for this test")
	}
	poolName := os.Getenv("_E2E_POOL")
	if poolName == "" {
		t.Fatalf("error - _E2E_POOL env var is required for this test")
	}
	pvcAnnotations := &e2e.PVCAnnotations{
		IboxNetworkSpace: "",
		IboxPool:         poolName,
		IboxSecret:       iboxSecret,
	}

	testConfig.PVCAnnotations = pvcAnnotations

	e2e.Setup(testConfig)

	if *e2e.CleanUp {
		e2e.TearDown(testConfig)
	} else {
		t.Log("not cleaning up namespace")
	}

}
