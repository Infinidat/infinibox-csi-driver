//go:build e2e

package iscsianno

import (
	"infinibox-csi-driver/common"
	"infinibox-csi-driver/e2e"
	"os"
	"testing"
)

func TestIscsiMultipleNetworkSpace(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, common.PROTOCOL_ISCSI)
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

	networkSpace := os.Getenv("_E2E_NETWORK_SPACE")
	if networkSpace == "" {
		t.Fatalf("error - _E2E_NETWORK_SPACE env var is required for this test")
	}
	networkSpace2 := os.Getenv("_E2E_NETWORK_SPACE2")
	if networkSpace2 == "" {
		t.Fatalf("error - _E2E_NETWORK_SPACE2 env var is required for this test")
	}

	networkSpace = networkSpace + "," + networkSpace2

	iboxSecret := os.Getenv("_E2E_IBOX_SECRET")
	if iboxSecret == "" {
		t.Fatalf("error - _E2E_IBOX_SECRET env var is required for this test")
	}
	pvcAnnotations := &e2e.PVCAnnotations{
		IboxNetworkSpace: networkSpace,
		IboxPool:         "",
		IboxSecret:       iboxSecret,
	}

	testConfig.PVCAnnotations = pvcAnnotations

	e2e.Setup(testConfig)

	t.Logf("testing with ibox_secret %s network_space %s\n", iboxSecret, networkSpace)

	if *e2e.CleanUp {
		e2e.TearDown(testConfig)
	} else {
		t.Log("not cleaning up namespace")
	}

}

func TestIscsiNetworkSpace(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, common.PROTOCOL_ISCSI)
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

	networkSpace := os.Getenv("_E2E_NETWORK_SPACE")
	if networkSpace == "" {
		t.Fatalf("error - _E2E_NETWORK_SPACE env var is required for this test")
	}
	iboxSecret := os.Getenv("_E2E_IBOX_SECRET")
	if iboxSecret == "" {
		t.Fatalf("error - _E2E_IBOX_SECRET env var is required for this test")
	}
	pvcAnnotations := &e2e.PVCAnnotations{
		IboxNetworkSpace: networkSpace,
		IboxPool:         "",
		IboxSecret:       iboxSecret,
	}

	testConfig.PVCAnnotations = pvcAnnotations

	e2e.Setup(testConfig)

	t.Logf("testing with ibox_secret %s network_space %s\n", iboxSecret, networkSpace)

	if *e2e.CleanUp {
		e2e.TearDown(testConfig)
	} else {
		t.Log("not cleaning up namespace")
	}

}
func TestIscsiPool(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, common.PROTOCOL_ISCSI)
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

	pool := os.Getenv("_E2E_POOL")
	if pool == "" {
		t.Fatalf("error - _E2E_POOL env var is required for this test")
	}
	iboxSecret := os.Getenv("_E2E_IBOX_SECRET")
	if iboxSecret == "" {
		t.Fatalf("error - ibox_secret env var is required for this test")
	}
	pvcAnnotations := &e2e.PVCAnnotations{
		IboxNetworkSpace: "",
		IboxPool:         pool,
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
func TestIscsiSecret(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, common.PROTOCOL_ISCSI)
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

	iboxSecret := os.Getenv("_E2E_IBOX_SECRET")
	if iboxSecret == "" {
		t.Fatalf("error - _E2E_IBOX_SECRET env var is required for this test")
	}
	pvcAnnotations := &e2e.PVCAnnotations{
		IboxNetworkSpace: "",
		IboxPool:         "",
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
