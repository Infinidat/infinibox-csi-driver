//go:build e2e

package fc

import (
	"infinibox-csi-driver/e2e"
	"strconv"
	"testing"

	snapshotv6 "github.com/kubernetes-csi/external-snapshotter/client/v6/clientset/versioned"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

const (
	PROTOCOL = "fc"
)

func TestFc(t *testing.T) {

	e2e.GetFlags(t)

	//connect to kube
	clientSet, dynamicClient, snapshotClient, err := e2e.GetKubeClient(*e2e.KubeConfigPath)
	if err != nil {
		t.Fatalf("error creating clients %s\n", err.Error())
	}
	if clientSet == nil {
		t.Fatalf("error creating k8s client")
	}

	testNames := setup(PROTOCOL, t, clientSet, dynamicClient, snapshotClient, false, false, false, nil)

	t.Logf("testing in namespace %+v\n", testNames)

	if *e2e.CleanUp {
		tearDown(t, testNames, clientSet, dynamicClient, snapshotClient)
	} else {
		t.Log("not cleaning up namespace")
	}

}

func TestFsGroupFc(t *testing.T) {

	e2e.GetFlags(t)

	config := e2e.GetRestConfig(*e2e.KubeConfigPath)

	if config == nil {
		t.Fatalf("error getting rest config")
	}

	//connect to kube
	clientSet, dynamicClient, snapshotClient, err := e2e.GetKubeClient(*e2e.KubeConfigPath)
	if err != nil {
		t.Fatalf("error creating clients %s\n", err.Error())
	}
	if clientSet == nil {
		t.Fatalf("error creating k8s client")
	}

	testNames := setup(PROTOCOL, t, clientSet, dynamicClient, snapshotClient, true, false, false, nil)

	t.Logf("testing in namespace %+v\n", testNames)

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

func TestFcBlock(t *testing.T) {

	e2e.GetFlags(t)

	//connect to kube
	clientSet, dynamicClient, snapshotClient, err := e2e.GetKubeClient(*e2e.KubeConfigPath)
	if err != nil {
		t.Fatalf("error creating clients %s\n", err.Error())
	}
	if clientSet == nil {
		t.Fatalf("error creating k8s client")
	}

	testNames := setup(PROTOCOL, t, clientSet, dynamicClient, snapshotClient, false, true, false, nil)

	t.Logf("testing in namespace %+v\n", testNames)

	if *e2e.CleanUp {
		tearDown(t, testNames, clientSet, dynamicClient, snapshotClient)
	} else {
		t.Log("not cleaning up namespace")
	}

}

func TestFcBlockRWX(t *testing.T) {

	e2e.GetFlags(t)

	config := e2e.GetRestConfig(*e2e.KubeConfigPath)

	if config == nil {
		t.Fatalf("error getting rest config")
	}

	//connect to kube
	clientSet, dynamicClient, snapshotClient, err := e2e.GetKubeClient(*e2e.KubeConfigPath)
	if err != nil {
		t.Fatalf("error creating clients %s\n", err.Error())
	}
	if clientSet == nil {
		t.Fatalf("error creating k8s client")
	}

	nodeCount := e2e.GetTestSystemNodecount(t, clientSet)

	if nodeCount < 2 {
		t.Fatalf("System needs at least 2 nodes and only has %d ", nodeCount)
	}

	testNames := setup(PROTOCOL, t, clientSet, dynamicClient, snapshotClient, false, true, true, nil) // setup test with anti-affinity pod also.

	t.Logf("testing in namespace %+v\n", testNames)

	firstSuccess, _ := e2e.VerifyBlockWriteInPod(clientSet, config, e2e.POD_NAME, testNames.NSName)

	if !firstSuccess {
		t.Fatalf("Test of Blockwrite in %s pod failed.", e2e.POD_NAME)
	}

	secondSuccess, _ := e2e.VerifyBlockWriteInPod(clientSet, config, e2e.ANTI_AF_POD_NAME, testNames.NSName)

	if !secondSuccess {
		t.Fatalf("Test of Blockwrite in %s pod failed.", e2e.POD_NAME)
	}

	if *e2e.CleanUp {
		tearDown(t, testNames, clientSet, dynamicClient, snapshotClient)
	} else {
		t.Logf("not cleaning up namespace")
	}

}

func setup(protocol string, t *testing.T, client *kubernetes.Clientset, dynamicClient *dynamic.DynamicClient, snapshotClient *snapshotv6.Clientset, useFsGroup bool, useBlock bool, useAntiAffinity bool, pvcAnnotations *e2e.PVCAnnotations) (testNames e2e.TestResourceNames) {
	return e2e.Setup(protocol, t, client, dynamicClient, snapshotClient, useFsGroup, useBlock, useAntiAffinity, pvcAnnotations)
}

func tearDown(t *testing.T, testNames e2e.TestResourceNames, client *kubernetes.Clientset, dynamicClient dynamic.Interface, snapshotClient *snapshotv6.Clientset) {
	e2e.TearDown(t, testNames, client, dynamicClient, snapshotClient)
}
