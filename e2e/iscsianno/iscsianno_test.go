//go:build e2e

package iscsianno

import (
	"os"
	"strconv"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/infinidat/infinibox-csi-driver/e2e"
)

func TestIscsiMultipleNetworkSpace(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, common.ProtocolISCSI)
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

	e2e.Setup(t.Context(), testConfig)

	t.Logf("testing with ibox_secret %s network_space %s\n", iboxSecret, networkSpace)

	if *e2e.CleanUp {
		e2e.TearDown(t.Context(), testConfig)
	} else {
		t.Log("not cleaning up namespace")
	}

	err = e2e.CleanISCI(t.Context(), *testConfig)
	if err != nil {
		t.Logf("error cleaning ISCSI %s on node %s\n", err.Error(), testConfig.NodeName)
	}

}

func TestIscsiNetworkSpace(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, common.ProtocolISCSI)
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

	e2e.Setup(t.Context(), testConfig)

	t.Logf("testing with ibox_secret %s network_space %s\n", iboxSecret, networkSpace)

	if *e2e.CleanUp {
		e2e.TearDown(t.Context(), testConfig)
	} else {
		t.Log("not cleaning up namespace")
	}

	err = e2e.CleanISCI(t.Context(), *testConfig)
	if err != nil {
		t.Logf("error cleaning ISCSI %s on node %s\n", err.Error(), testConfig.NodeName)
	}

}
func TestIscsiPool(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, common.ProtocolISCSI)
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

	e2e.Setup(t.Context(), testConfig)

	if *e2e.CleanUp {
		e2e.TearDown(t.Context(), testConfig)
	} else {
		t.Log("not cleaning up namespace")
	}

	err = e2e.CleanISCI(t.Context(), *testConfig)
	if err != nil {
		t.Logf("error cleaning ISCSI %s on node %s\n", err.Error(), testConfig.NodeName)
	}

}
func TestIscsiSecret(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, common.ProtocolISCSI)
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

	e2e.Setup(t.Context(), testConfig)

	if *e2e.CleanUp {
		e2e.TearDown(t.Context(), testConfig)
	} else {
		t.Log("not cleaning up namespace")
	}

	err = e2e.CleanISCI(t.Context(), *testConfig)
	if err != nil {
		t.Logf("error cleaning ISCSI %s on node %s\n", err.Error(), testConfig.NodeName)
	}

}

func TestIscsiUserMetadata(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, common.ProtocolISCSI)
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

	pvcAnnotations := &e2e.PVCAnnotations{
		IboxNetworkSpace: "",
		IboxPool:         "",
		IboxSecret:       "",
		IboxMetadata:     "user-defined-metadata",
	}

	testConfig.PVCAnnotations = pvcAnnotations

	e2e.Setup(t.Context(), testConfig)

	pvc, err := testConfig.ClientSet.CoreV1().PersistentVolumeClaims(testConfig.TestNames.NSName).Get(t.Context(), testConfig.TestNames.PVCName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("error getting existing PVC %s", err.Error())
	}
	volumeName := pvc.Spec.VolumeName
	volume, err := testConfig.ClientSet.CoreV1().PersistentVolumes().Get(t.Context(), volumeName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("error getting existing PV %s", err.Error())
	}
	volumeHandle := volume.Spec.CSI.VolumeHandle
	volproto := strings.Split(volumeHandle, "$$")
	if len(volproto) != 2 {
		t.Fatalf("volumeHandle was not valid %s", volumeHandle)
	}
	volumeID, err := strconv.Atoi(volproto[0])
	if err != nil {
		t.Fatalf("could not convert volumeID to int - error %s", err.Error())
	}

	metadata, err := testConfig.ClientService.IboxAPI.GetMetadata(testConfig.Testt.Context(), volumeID)
	if err != nil {
		t.Fatalf("error getting metadata for volumeID - error %s", err.Error())
	}

	t.Logf("metadata for the test volume %d is %v len=%d\n", volumeID, metadata, len(metadata))

	if len(metadata) != 2 {
		t.Fatalf("error expected 2 metadata values for volumeID - got %v", metadata)
	}

	if *e2e.CleanUp {
		e2e.TearDown(t.Context(), testConfig)
	} else {
		t.Log("not cleaning up namespace")
	}

	err = e2e.CleanISCI(t.Context(), *testConfig)
	if err != nil {
		t.Logf("error cleaning ISCSI %s on node %s\n", err.Error(), testConfig.NodeName)
	}

}
