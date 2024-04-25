//go:build e2e

package fcanno

import (
	"infinibox-csi-driver/common"
	"infinibox-csi-driver/e2e"
	"os"
	"testing"
)

func TestFc(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, common.PROTOCOL_FC)
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
