//go:build e2e

package treeqanno

import (
	"infinibox-csi-driver/e2e"
	"os"
	"testing"

	snapshotv6 "github.com/kubernetes-csi/external-snapshotter/client/v6/clientset/versioned"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

const (
	PROTOCOL = "treeq"
)

func TestTreeq(t *testing.T) {

	e2e.GetFlags(t)

	//connect to kube
	clientSet, dynamicClient, snapshotClient, err := e2e.GetKubeClient(*e2e.KubeConfigPath)
	if err != nil {
		t.Fatalf("error creating clients %s\n", err.Error())
	}
	if clientSet == nil {
		t.Fatalf("error creating k8s client")
	}

	// create a unique namespace to perform the test within
	iboxSecret := os.Getenv("ibox_secret")
	if iboxSecret == "" {
		t.Fatalf("error - ibox_secret env var is required for this test")
	}
	networkSpace := os.Getenv("network_space")
	if networkSpace == "" {
		t.Fatalf("error - network_space env var is required for this test")
	}
	poolName := os.Getenv("pool_name")
	if poolName == "" {
		t.Fatalf("error - pool_name env var is required for this test")
	}
	pvcAnnotations := &e2e.PVCAnnotations{
		IboxNetworkSpace: networkSpace,
		IboxPool:         poolName,
		IboxSecret:       iboxSecret,
	}

	testNames := setup(t, clientSet, dynamicClient, snapshotClient, false, false, false, pvcAnnotations)

	t.Logf("testing in namespace %+v\n", testNames)
	// run the test

	if *e2e.CleanUp {
		tearDown(t, testNames, clientSet, dynamicClient, snapshotClient)
	} else {
		t.Log("not cleaning up namespace")
	}
}

func setup(t *testing.T, client *kubernetes.Clientset, dynamicClient *dynamic.DynamicClient, snapshotClient *snapshotv6.Clientset, fsGroup bool,
	useBlock bool, useAntiAffinity bool, pvcAnnotations *e2e.PVCAnnotations) (testNames e2e.TestResourceNames) {
	return e2e.Setup(PROTOCOL, t, client, dynamicClient, snapshotClient, fsGroup, useBlock, useAntiAffinity, pvcAnnotations)
}

func tearDown(t *testing.T, testNames e2e.TestResourceNames, client *kubernetes.Clientset, dynamicClient *dynamic.DynamicClient, snapshotClient *snapshotv6.Clientset) {
	e2e.TearDown(t, testNames, client, dynamicClient, snapshotClient)
}
