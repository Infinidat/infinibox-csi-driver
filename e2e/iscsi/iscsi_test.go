//go:build e2e

package iscsi

import (
	"infinibox-csi-driver/e2e"
	"strconv"
	"testing"

	snapshotv6 "github.com/kubernetes-csi/external-snapshotter/client/v6/clientset/versioned"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

const (
	PROTOCOL = "iscsi"
)

func TestIscsi(t *testing.T) {

	e2e.GetFlags(t)

	//connect to kube
	clientSet, dynamicClient, snapshotClient, err := e2e.GetKubeClient(*e2e.KubeConfigPath)
	if err != nil {
		t.Fatalf("error creating clients %s\n", err.Error())
	}
	if clientSet == nil {
		t.Fatalf("error creating k8s client")
	}

	testNames := setup(PROTOCOL, t, clientSet, dynamicClient, snapshotClient, false, false, nil)

	t.Logf("testing in namespace %+v\n", testNames)

	/**
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

func TestIscsiFsGroup(t *testing.T) {

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

	testNames := setup(PROTOCOL, t, clientSet, dynamicClient, snapshotClient, true, false, nil)

	t.Logf("testing in namespace %+v\n", testNames)

	/**
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
func TestIscsiBlock(t *testing.T) {

	e2e.GetFlags(t)

	//connect to kube
	clientSet, dynamicClient, snapshotClient, err := e2e.GetKubeClient(*e2e.KubeConfigPath)
	if err != nil {
		t.Fatalf("error creating clients %s\n", err.Error())
	}
	if clientSet == nil {
		t.Fatalf("error creating k8s client")
	}

	testNames := setup(PROTOCOL, t, clientSet, dynamicClient, snapshotClient, false, true, nil)

	t.Logf("testing in namespace %+v\n", testNames)

	if *e2e.CleanUp {
		tearDown(t, testNames, clientSet, dynamicClient, snapshotClient)
	} else {
		t.Log("not cleaning up namespace")
	}

}

func setup(protocol string, t *testing.T, client *kubernetes.Clientset, dynamicClient *dynamic.DynamicClient, snapshotClient *snapshotv6.Clientset, useFsGroup bool, useBlock bool, pvcAnnotations *e2e.PVCAnnotations) (testNames e2e.TestResourceNames) {
	return e2e.Setup(protocol, t, client, dynamicClient, snapshotClient, useFsGroup, useBlock, pvcAnnotations)
}

func tearDown(t *testing.T, testNames e2e.TestResourceNames, client *kubernetes.Clientset, dynamicClient dynamic.Interface, snapshotClient *snapshotv6.Clientset) {
	e2e.TearDown(t, testNames, client, dynamicClient, snapshotClient)
}
