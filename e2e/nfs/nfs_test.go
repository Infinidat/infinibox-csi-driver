//go:build e2e

package nfs

import (
	"infinibox-csi-driver/e2e"
	"strconv"
	"testing"

	snapshotv6 "github.com/kubernetes-csi/external-snapshotter/client/v6/clientset/versioned"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

const (
	PROTOCOL = "nfs"
)

func TestNfs(t *testing.T) {

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

	testNames := setup(PROTOCOL, t, clientSet, dynamicClient, snapshotClient, false, false)

	t.Logf("testing in namespace %+v\n", testNames)
	// run the test

	/** TODO snapshots not working currently from this test

	time.Sleep(time.Second * 5)

	err = e2e.CreateSnapshot(testNames.PVCName, testNames.SnapshotClassName, testNames.NSName, snapshotClient)
	if err != nil {
		t.Fatalf("error creating volumesnapshot pod %s", err.Error())
	}

	time.Sleep(time.Second * 5)

	err = e2e.WaitForSnapshot(t, e2e.SNAPSHOT_NAME, testNames.NSName, snapshotClient)
	if err != nil {
		t.Fatalf("error waiting for volumesnapshot %s", err.Error())
	}
	*/

	if *e2e.CleanUp {
		tearDown(t, testNames, clientSet, dynamicClient, snapshotClient)
	} else {
		t.Log("not cleaning up namespace")
	}
}

func TestNfsFsGroup(t *testing.T) {

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

	testNames := setup(PROTOCOL, t, clientSet, dynamicClient, snapshotClient, true, false)

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

	// exec into pod, make sure directory permissions are set properly.

	if *e2e.CleanUp {
		tearDown(t, testNames, clientSet, dynamicClient, snapshotClient)
	} else {
		t.Log("not cleaning up namespace")
	}
}

func setup(protocol string, t *testing.T, client *kubernetes.Clientset, dynamicClient *dynamic.DynamicClient, snapshotClient *snapshotv6.Clientset, useFsGroup bool, useBlock bool) (testNames e2e.TestResourceNames) {
	return e2e.Setup(protocol, t, client, dynamicClient, snapshotClient, useFsGroup, useBlock)
}

func tearDown(t *testing.T, testNames e2e.TestResourceNames, client *kubernetes.Clientset, dynamicClient dynamic.Interface, snapshotClient *snapshotv6.Clientset) {
	e2e.TearDown(t, testNames, client, dynamicClient, snapshotClient)
}
