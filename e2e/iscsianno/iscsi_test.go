//go:build e2e

package iscsianno

import (
	"github.com/amitosw15/infinibox-csi-driver/common"
	"github.com/amitosw15/infinibox-csi-driver/e2e"
	"os"
	"testing"
)

func TestIscsiMultipleNetworkSpace(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, common.PROTOCOL_ISCSI)
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

	networkSpace := os.Getenv(e2e.ENV_ISCSI_NETWORK_SPACE)
	if networkSpace == "" {
		networkSpace = os.Getenv(e2e.ENV_NETWORK_SPACE)
		if networkSpace == "" {
			t.Fatalf("error - %s or %s env var is required for this test", e2e.ENV_NETWORK_SPACE, e2e.ENV_ISCSI_NETWORK_SPACE)
		}
	}
	networkSpace2 := os.Getenv(e2e.ENV_ISCSI_NETWORK_SPACE2)
	if networkSpace2 == "" {
		networkSpace2 = os.Getenv(e2e.ENV_NETWORK_SPACE2)
		if networkSpace2 == "" {
			t.Fatalf("error - %s or %s env var is required for this test", e2e.ENV_ISCSI_NETWORK_SPACE2, e2e.ENV_NETWORK_SPACE2)
		}
	}

	networkSpace = networkSpace + "," + networkSpace2

	iboxSecret := os.Getenv(e2e.ENV_IBOX_SECRET)
	if iboxSecret == "" {
		t.Fatalf("error - %s env var is required for this test", e2e.ENV_IBOX_SECRET)
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

	networkSpace := os.Getenv(e2e.ENV_ISCSI_NETWORK_SPACE)
	if networkSpace == "" {
		networkSpace = os.Getenv(e2e.ENV_NETWORK_SPACE)
		if networkSpace == "" {
			t.Fatalf("error - %s or %s env var is required for this test", e2e.ENV_NETWORK_SPACE, e2e.ENV_ISCSI_NETWORK_SPACE)
		}
	}
	iboxSecret := os.Getenv(e2e.ENV_IBOX_SECRET)
	if iboxSecret == "" {
		t.Fatalf("error - %s env var is required for this test", e2e.ENV_IBOX_SECRET)
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

	pool := os.Getenv(e2e.ENV_POOL)
	if pool == "" {
		t.Fatalf("error - %s env var is required for this test", e2e.ENV_POOL)
	}
	iboxSecret := os.Getenv(e2e.ENV_IBOX_SECRET)
	if iboxSecret == "" {
		t.Fatalf("error - %s env var is required for this test", e2e.ENV_IBOX_SECRET)
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

	iboxSecret := os.Getenv(e2e.ENV_IBOX_SECRET)
	if iboxSecret == "" {
		t.Fatalf("error - %s env var is required for this test", e2e.ENV_IBOX_SECRET)
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
