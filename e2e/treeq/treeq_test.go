//go:build e2e

package treeq

import (
	"infinibox-csi-driver/e2e"
	"strconv"
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

	testNames := setup(t, clientSet, dynamicClient, snapshotClient, false)

	t.Logf("testing in namespace %+v\n", testNames)
	// run the test

	if *e2e.CleanUp {
		tearDown(t, testNames, clientSet, dynamicClient, snapshotClient)
	} else {
		t.Log("not cleaning up namespace")
	}
}

func TestFsGroupTreeq(t *testing.T) {

	e2e.GetFlags(t)

	//connect to kube
	clientSet, dynamicClient, snapshotClient, err := e2e.GetKubeClient(*e2e.KubeConfigPath)
	if err != nil {
		t.Fatalf("error creating clients %s\n", err.Error())
	}
	if clientSet == nil {
		t.Fatalf("error creating k8s client")
	}

	config := e2e.GetRestConfig(*e2e.KubeConfigPath)

	if config == nil {
		t.Fatalf("error getting rest config")
	}

	// create a unique namespace to perform the test within

	testNames := setup(t, clientSet, dynamicClient, snapshotClient, true)

	t.Logf("testing in namespace %+v\n", testNames)
	// run the test

	expectedValue := "drwxrwsr-x"
	winning, actual := e2e.VerifyDirPermsCorrect(clientSet, config, e2e.POD_NAME, testNames.NSName, expectedValue)

	if winning {
		t.Log("FSGroupDirPermsCorrect PASSED")
	} else {
		t.Errorf("FSGroupDirPermsCorrect FAILED, expected: %s but got %s", expectedValue, actual)
	}

	expectedValue = strconv.Itoa(e2e.POD_FS_GROUP)
	winning, actual = e2e.VerifyGroupIdIsUsed(clientSet, config, e2e.POD_NAME, testNames.NSName, expectedValue)

	if winning {
		t.Log("FSGroupIdIsUsed PASSED")
	} else {
		t.Errorf("FsGroupIdIsUsed FAILED, expected: %s but got %s", expectedValue, actual)
	}

	if *e2e.CleanUp {
		tearDown(t, testNames, clientSet, dynamicClient, snapshotClient)
	} else {
		t.Log("not cleaning up namespace")
	}
}

func setup(t *testing.T, client *kubernetes.Clientset, dynamicClient *dynamic.DynamicClient, snapshotClient *snapshotv6.Clientset, fsGroup bool) (testNames e2e.TestResourceNames) {
	return e2e.Setup(PROTOCOL, t, client, dynamicClient, snapshotClient, fsGroup)
}

func tearDown(t *testing.T, testNames e2e.TestResourceNames, client *kubernetes.Clientset, dynamicClient *dynamic.DynamicClient, snapshotClient *snapshotv6.Clientset) {
	e2e.TearDown(t, testNames, client, dynamicClient, snapshotClient)
}
