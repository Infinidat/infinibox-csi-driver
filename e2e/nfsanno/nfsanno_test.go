//go:build e2e

package nfsanno

import (
	"os"
	"testing"

	"github.com/infinidat/infinibox-csi-driver/e2e"
)

const (
	PROTOCOL = "nfs"
)

func TestNfs(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, PROTOCOL)
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

	iboxSecret := os.Getenv(e2e.ENV_IBOX_SECRET)
	if iboxSecret == "" {
		t.Fatalf("error - %s env var is required for this test", e2e.ENV_IBOX_SECRET)
	}
	networkSpace := os.Getenv(e2e.ENV_NAS_NETWORK_SPACE)
	if networkSpace == "" {
		t.Fatalf("error - %s env var is required for this test", e2e.ENV_NAS_NETWORK_SPACE)
	}
	poolName := os.Getenv(e2e.ENV_POOL)
	if poolName == "" {
		t.Fatalf("error - %s env var is required for this test", e2e.ENV_POOL)
	}

	pvcAnnotations := &e2e.PVCAnnotations{
		IboxNetworkSpace: networkSpace,
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
