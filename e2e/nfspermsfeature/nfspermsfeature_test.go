//go:build e2e

package nfspermsfeature

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/infinidat/infinibox-csi-driver/e2e"
	"github.com/infinidat/infinibox-csi-driver/iboxapi"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNfsPermsFeatureRemoveSinglePerm(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, common.ProtocolNFS)
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

	e2e.Setup(t.Context(), testConfig)

	// get the filesystem
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
	exports, err := testConfig.ClientService.IboxAPI.GetExportsByFileSystemID(t.Context(), volumeID)
	if err != nil {
		t.Fatalf("could not get exports for volumeID %d - error %s", volumeID, err.Error())
	}
	if len(exports) != 1 {
		t.Fatalf("exports len (%d) is not 1 for volumeID %d - error %s", len(exports), volumeID, err.Error())
	}
	existingExport := exports[0]
	t.Logf("before adding phony perm...export %d has %d permissions", existingExport.ID, len(existingExport.Permissions))

	// add a phony export rule permission to that filesystem export
	phonyPerm := iboxapi.Permissions{
		NoRootSquash: true,
		Access:       "RW",
		Client:       "192.168.0.102",
	}
	phonyExportPermRef := iboxapi.ExportPathRef{
		Permissions: append(existingExport.Permissions, phonyPerm),
	}
	updateExport, err := testConfig.ClientService.IboxAPI.UpdateExportPermissions(t.Context(), existingExport, phonyExportPermRef)
	if err != nil {
		t.Fatalf("could not update export permissions for volumeID %d - error %s", volumeID, err.Error())
	}

	// delete only the pod so that we keep the exports
	err = e2e.DeletePod(t.Context(), testConfig.TestNames.NSName, e2e.POD_NAME, testConfig.ClientSet)
	if err != nil {
		t.Fatalf("could not delete pod after permissions updated - error %s", err.Error())
	}
	// wait 10 seconds to give some time for the export rule permission for
	// the IP address of the kube node to be removed
	t.Log("sleeping after deleting only the pod as part of this test")
	time.Sleep(10 * time.Second)
	// get the export rule permissions, there should be only a single permission
	// at this point, the phony one we added
	ex, err := testConfig.ClientService.IboxAPI.GetExportByID(t.Context(), updateExport.ID)
	if err != nil {
		t.Fatalf("could not get updated export %d for volumeID %d - error %s", updateExport.ID, volumeID, err.Error())
	}
	t.Logf("export ID %d had permissions count %d", updateExport.ID, len(ex.Permissions))
	if len(ex.Permissions) != 1 {
		t.Fatalf("export perms len (%d) is not 1 for export %d volumeID %d", len(ex.Permissions), updateExport.ID, volumeID)
	}
	// recreate the pod, this should re-add the kube node ip address export rule
	err = e2e.CreatePod(t.Context(), testConfig, testConfig.TestNames.NSName, e2e.POD_NAME)
	if err != nil {
		t.Fatalf("could not create pod - error %s", err.Error())
	}
	// wait 10 seconds, give time for the pod to start
	time.Sleep(time.Second * 10)
	// verify the export rule permissions now has 2 export rule permissions
	ex, err = testConfig.ClientService.IboxAPI.GetExportByID(t.Context(), updateExport.ID)
	if err != nil {
		t.Fatalf("could not get updated export %d after re-creating pod for volumeID %d - error %s", updateExport.ID, volumeID, err.Error())
	}
	if len(ex.Permissions) != 2 {
		t.Fatalf("export perms len (%d) is not 2 for export %d volumeID %d - error %s", len(ex.Permissions), updateExport.ID, volumeID, err.Error())
	}

	if *e2e.CleanUp {
		e2e.TearDown(t.Context(), testConfig)
	} else {
		t.Log("not cleaning up namespace")
	}
}

func TestNfsPermsFeatureRemoveExport(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, common.ProtocolNFS)
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

	e2e.Setup(t.Context(), testConfig)

	// get the filesystem
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
	exports, err := testConfig.ClientService.IboxAPI.GetExportsByFileSystemID(t.Context(), volumeID)
	if err != nil {
		t.Fatalf("could not get exports for volumeID %d - error %s", volumeID, err.Error())
	}
	if len(exports) != 1 {
		t.Fatalf("exports len (%d) is not 1 for volumeID %d - error %s", len(exports), volumeID, err.Error())
	}
	existingExport := exports[0]
	t.Logf("before adding phony perm...export %d has %d permissions", existingExport.ID, len(existingExport.Permissions))

	// delete only the pod which should cause the entire Export to be removed
	err = e2e.DeletePod(t.Context(), testConfig.TestNames.NSName, e2e.POD_NAME, testConfig.ClientSet)
	if err != nil {
		t.Fatalf("could not delete pod after permissions updated - error %s", err.Error())
	}
	// wait 10 seconds to give some time for the export to be removed
	t.Log("sleeping after deleting only the pod as part of this test")
	time.Sleep(10 * time.Second)

	// try to get the Export, this should fail because the export should
	// be removed since it only had the single IP address export rule permission prior
	// to the pod being removed
	_, err = testConfig.ClientService.IboxAPI.GetExportByID(t.Context(), existingExport.ID)
	if err == nil {
		t.Fatalf("got export %d for volumeID %d after pod was removed, this should not happen", existingExport.ID, volumeID)
	}
	t.Logf("export %d was removed which is expected", existingExport.ID)
	// recreate the pod, this should re-add the kube node ip address export rule
	err = e2e.CreatePod(t.Context(), testConfig, testConfig.TestNames.NSName, e2e.POD_NAME)
	if err != nil {
		t.Fatalf("could not create pod - error %s", err.Error())
	}
	// wait 10 seconds, give time for the pod to start
	time.Sleep(time.Second * 10)
	// verify the export rule permissions now has 1 export rule permissions
	exports, err = testConfig.ClientService.IboxAPI.GetExportsByFileSystemID(t.Context(), volumeID)
	if err != nil {
		t.Fatalf("could not get exports for volumeID %d - error %s", volumeID, err.Error())
	}
	if len(exports) != 1 {
		t.Fatalf("exports len (%d) is not 1 for volumeID %d - error %s", len(exports), volumeID, err.Error())
	}

	if *e2e.CleanUp {
		e2e.TearDown(t.Context(), testConfig)
	} else {
		t.Log("not cleaning up namespace")
	}
}
