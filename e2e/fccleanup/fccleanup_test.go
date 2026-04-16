//go:build e2e

package fccleanup

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/infinidat/infinibox-csi-driver/e2e"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestFcCleanup(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, common.ProtocolFC)
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

	e2e.Setup(t.Context(), testConfig)

	// get node name that pod is running on
	p, err := testConfig.ClientSet.CoreV1().Pods(testConfig.TestNames.NSName).Get(t.Context(), e2e.POD_NAME, v1.GetOptions{})
	if err != nil {
		t.Fatalf("error getting pod for nodeName %s\n", err.Error())
	}
	nodeName := p.Spec.NodeName
	t.Logf("nodeName %s\n", nodeName)

	// get the mpath device path that the pod has mounted
	mpathDevicePath, err := e2e.GetMpathDevicePath(t.Context(), testConfig.ClientSet, testConfig.RestConfig, e2e.POD_NAME, testConfig.TestNames.NSName)
	if err != nil {
		t.Fatalf("error getting mpath device path %s\n", err.Error())
	}
	t.Logf("mpath device %s\n", mpathDevicePath)

	mpathName := filepath.Base(mpathDevicePath)
	devices, err := e2e.FindDevicesForMpath(t.Context(), testConfig, mpathName)
	if err != nil {
		t.Fatalf("error checking for mpath device %s\n", err.Error())
	}
	t.Logf("mpath devices %v\n", devices)

	if len(devices) == 0 {
		t.Fatalf("error mpath %s ... no underlying devices found", mpathName)
	}

	// normally delete the pod which should remove the mpath device from the node after some time
	if *e2e.CleanUp {
		e2e.TearDown(t.Context(), testConfig)
	} else {
		t.Log("not cleaning up namespace")
	}

	// give it some time to remove the mpath device
	t.Log("sleeping to give mpath time to be cleaned up")
	time.Sleep(time.Second * 10)

	// find the csi driver node pod
	ns := os.Getenv("_E2E_NAMESPACE")
	fieldSelector := fmt.Sprintf("spec.nodeName=%s", nodeName)
	labelSelector := "app=infinidat-csi-driver-node"
	listOptions := v1.ListOptions{
		FieldSelector: fieldSelector,
		LabelSelector: labelSelector,
	}
	csiPods, err := testConfig.ClientSet.CoreV1().Pods(ns).List(t.Context(), listOptions)
	if err != nil {
		t.Fatalf("error getting csi driver pod for nodeName %s fieldSelector %s labelSelector %s error %s\n", nodeName, fieldSelector, labelSelector, err.Error())
	}

	for _, pod := range csiPods.Items {
		t.Logf("csi pod that matches is %s and using mpath name of %s\n", pod.Name, mpathName)
		mpathExists, err := e2e.MpathExists(t.Context(), testConfig.ClientSet, testConfig.RestConfig, pod.Name, ns, mpathName)
		if err != nil {
			t.Fatalf("error checking for mpath device %s\n", err.Error())
		}
		t.Logf("mpath exists %t\n", mpathExists)
		if mpathExists {
			t.Fatalf("error mpath %s still exists and is considered an orphan device\n", mpathName)
		}
	}

	t.Log("sleeping for 10 seconds")
	time.Sleep(10 * time.Second)

	for _, dev := range devices {
		fileExists, err := e2e.FileExists(t.Context(), testConfig, dev)
		if err != nil {
			t.Fatalf("error checking for device %s - error %s\n", dev, err.Error())
		}
		if fileExists {
			t.Fatalf("error mpath %s dev %s exists, should have been removed as part of cleanup", mpathName, dev)
		}
	}

}

func TestFcBlockCleanup(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, common.ProtocolFC)
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

	testConfig.UseBlock = true
	e2e.Setup(t.Context(), testConfig)

	// get node name that pod is running on
	p, err := testConfig.ClientSet.CoreV1().Pods(testConfig.TestNames.NSName).Get(t.Context(), e2e.POD_NAME, v1.GetOptions{})
	if err != nil {
		t.Fatalf("error getting pod for nodeName %s\n", err.Error())
	}
	nodeName := p.Spec.NodeName
	t.Logf("nodeName %s\n", nodeName)

	// get the mpath device path that the pod has mounted
	mpathDevicePath, err := e2e.GetMpathForBlockVolume(t.Context(), testConfig, testConfig.TestNames.PVCName)
	if err != nil {
		t.Fatalf("error getting mpath device path %s for pvc %s\n", err.Error(), testConfig.TestNames.PVCName)
	}
	t.Logf("mpath device %s\n", mpathDevicePath)

	mpathName := filepath.Base(mpathDevicePath)
	devices, err := e2e.FindDevicesForMpath(t.Context(), testConfig, mpathName)
	if err != nil {
		t.Fatalf("error checking for mpath device %s\n", err.Error())
	}
	t.Logf("mpath devices %v\n", devices)

	if len(devices) == 0 {
		t.Fatalf("error mpath %s ... no underlying devices found", mpathName)
	}

	// normally delete the pod which should remove the mpath device from the node after some time
	if *e2e.CleanUp {
		e2e.TearDown(t.Context(), testConfig)
	} else {
		t.Log("not cleaning up namespace")
	}

	// give it some time to remove the mpath device
	t.Log("sleeping to give mpath time to be cleaned up")
	time.Sleep(time.Second * 10)

	// find the csi driver node pod
	ns := os.Getenv("_E2E_NAMESPACE")
	fieldSelector := fmt.Sprintf("spec.nodeName=%s", nodeName)
	labelSelector := "app=infinidat-csi-driver-node"
	listOptions := v1.ListOptions{
		FieldSelector: fieldSelector,
		LabelSelector: labelSelector,
	}
	csiPods, err := testConfig.ClientSet.CoreV1().Pods(ns).List(t.Context(), listOptions)
	if err != nil {
		t.Fatalf("error getting csi driver pod for nodeName %s fieldSelector %s labelSelector %s error %s\n", nodeName, fieldSelector, labelSelector, err.Error())
	}

	for _, pod := range csiPods.Items {
		t.Logf("csi pod that matches is %s and using mpath name of %s\n", pod.Name, mpathName)
		mpathExists, err := e2e.MpathExists(t.Context(), testConfig.ClientSet, testConfig.RestConfig, pod.Name, ns, mpathName)
		if err != nil {
			t.Fatalf("error checking for mpath device %s\n", err.Error())
		}
		t.Logf("mpath exists %t\n", mpathExists)
		if mpathExists {
			t.Fatalf("error mpath %s still exists and is considered an orphan device\n", mpathName)
		}
	}

	t.Log("sleeping for 10 seconds")
	time.Sleep(10 * time.Second)

	for _, dev := range devices {
		fileExists, err := e2e.FileExists(t.Context(), testConfig, dev)
		if err != nil {
			t.Fatalf("error checking for device %s - error %s\n", dev, err.Error())
		}
		if fileExists {
			t.Fatalf("error mpath %s dev %s exists, should have been removed as part of cleanup", mpathName, dev)
		}
	}

}
