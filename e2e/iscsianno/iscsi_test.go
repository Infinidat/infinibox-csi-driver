//go:build e2e

package iscsianno

import (
	"infinibox-csi-driver/e2e"
	"os"
	"testing"

	snapshotv6 "github.com/kubernetes-csi/external-snapshotter/client/v6/clientset/versioned"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

const (
	PROTOCOL = "iscsi"
)

func TestIscsiMultipleNetworkSpace(t *testing.T) {

	e2e.GetFlags(t)

	//connect to kube
	clientSet, dynamicClient, snapshotClient, err := e2e.GetKubeClient(*e2e.KubeConfigPath)
	if err != nil {
		t.Fatalf("error creating clients %s\n", err.Error())
	}
	if clientSet == nil {
		t.Fatalf("error creating k8s client")
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
	testNames := setup(PROTOCOL, t, clientSet, dynamicClient, snapshotClient, false, false, false, pvcAnnotations)

	t.Logf("testing in namespace %+v\n", testNames)
	t.Logf("testing with ibox_secret %s network_space %s\n", iboxSecret, networkSpace)

	if *e2e.CleanUp {
		tearDown(t, testNames, clientSet, dynamicClient, snapshotClient)
	} else {
		t.Log("not cleaning up namespace")
	}

}

func TestIscsiNetworkSpace(t *testing.T) {

	e2e.GetFlags(t)

	//connect to kube
	clientSet, dynamicClient, snapshotClient, err := e2e.GetKubeClient(*e2e.KubeConfigPath)
	if err != nil {
		t.Fatalf("error creating clients %s\n", err.Error())
	}
	if clientSet == nil {
		t.Fatalf("error creating k8s client")
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
	testNames := setup(PROTOCOL, t, clientSet, dynamicClient, snapshotClient, false, false, false, pvcAnnotations)

	t.Logf("testing in namespace %+v\n", testNames)
	t.Logf("testing with ibox_secret %s network_space %s\n", iboxSecret, networkSpace)

	if *e2e.CleanUp {
		tearDown(t, testNames, clientSet, dynamicClient, snapshotClient)
	} else {
		t.Log("not cleaning up namespace")
	}

}
func TestIscsiPool(t *testing.T) {

	e2e.GetFlags(t)

	//connect to kube
	clientSet, dynamicClient, snapshotClient, err := e2e.GetKubeClient(*e2e.KubeConfigPath)
	if err != nil {
		t.Fatalf("error creating clients %s\n", err.Error())
	}
	if clientSet == nil {
		t.Fatalf("error creating k8s client")
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
	testNames := setup(PROTOCOL, t, clientSet, dynamicClient, snapshotClient, false, false, false, pvcAnnotations)

	t.Logf("testing in namespace %+v\n", testNames)

	if *e2e.CleanUp {
		tearDown(t, testNames, clientSet, dynamicClient, snapshotClient)
	} else {
		t.Log("not cleaning up namespace")
	}

}
func TestIscsiSecret(t *testing.T) {

	e2e.GetFlags(t)

	//connect to kube
	clientSet, dynamicClient, snapshotClient, err := e2e.GetKubeClient(*e2e.KubeConfigPath)
	if err != nil {
		t.Fatalf("error creating clients %s\n", err.Error())
	}
	if clientSet == nil {
		t.Fatalf("error creating k8s client")
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
	testNames := setup(PROTOCOL, t, clientSet, dynamicClient, snapshotClient, false, false, false, pvcAnnotations)

	t.Logf("testing in namespace %+v\n", testNames)

	if *e2e.CleanUp {
		tearDown(t, testNames, clientSet, dynamicClient, snapshotClient)
	} else {
		t.Log("not cleaning up namespace")
	}

}
func setup(protocol string, t *testing.T, client *kubernetes.Clientset, dynamicClient *dynamic.DynamicClient, snapshotClient *snapshotv6.Clientset,
	useFsGroup bool, useBlock bool, useAntiAffinity bool, pvcAnnotations *e2e.PVCAnnotations) (testNames e2e.TestResourceNames) {
	return e2e.Setup(protocol, t, client, dynamicClient, snapshotClient, useFsGroup, useBlock, useAntiAffinity, pvcAnnotations)
}

func tearDown(t *testing.T, testNames e2e.TestResourceNames, client *kubernetes.Clientset, dynamicClient dynamic.Interface, snapshotClient *snapshotv6.Clientset) {
	e2e.TearDown(t, testNames, client, dynamicClient, snapshotClient)
}
