//go:build e2e

package treeq

import (
	"infinibox-csi-driver/e2e"
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

	testNames := setup(t, clientSet, dynamicClient, snapshotClient)

	t.Logf("testing in namespace %+v\n", testNames)
	// run the test

	if *e2e.CleanUp {
		tearDown(t, testNames, clientSet, dynamicClient, snapshotClient)
	} else {
		t.Log("not cleaning up namespace")
	}
}

func setup(t *testing.T, client *kubernetes.Clientset, dynamicClient *dynamic.DynamicClient, snapshotClient *snapshotv6.Clientset) (testNames e2e.TestResourceNames) {
	return e2e.Setup(PROTOCOL, t, client, dynamicClient, snapshotClient)
}

func tearDown(t *testing.T, testNames e2e.TestResourceNames, client *kubernetes.Clientset, dynamicClient *dynamic.DynamicClient, snapshotClient *snapshotv6.Clientset) {
	e2e.TearDown(t, testNames, client, dynamicClient, snapshotClient)
}
