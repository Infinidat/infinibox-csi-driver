//go:build e2e

package iscsi

import (
	"context"
	"infinibox-csi-driver/common"
	"infinibox-csi-driver/e2e"
	"testing"
	"time"

	v1alpha1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumegroupsnapshot/v1alpha1"
	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestIscsiVolumeGroup(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, common.PROTOCOL_ISCSI)
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

	e2e.Setup(testConfig)

	time.Sleep(time.Second * 5)

	const vgLabelKey = "app.kubernetes.io/name"
	const vgLabelValue = "mygroup"

	// apply label to PVC
	// like this:  kubectl label pvc iscsi-pvc iscsi-pvc-anno app.kubernetes.io/name=mygroup

	existingPVC, err := testConfig.ClientSet.CoreV1().PersistentVolumeClaims(testConfig.TestNames.NSName).Get(context.TODO(), testConfig.TestNames.PVCName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("error getting existing PVC %s", err.Error())
	}

	existingPVC.ObjectMeta.Labels = map[string]string{
		vgLabelKey: vgLabelValue,
	}

	_, err = testConfig.ClientSet.CoreV1().PersistentVolumeClaims(testConfig.TestNames.NSName).Update(context.TODO(), existingPVC, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("error updating label on existing PVC %s", err.Error())
	}

	// create the VolumeGroupSnapshotClass, using the StorageClass name for uniqueness, we also will use
	// the StorageClass name for the CG name
	vgcsName := testConfig.TestNames.SCName
	vgsc := &v1alpha1.VolumeGroupSnapshotClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: vgcsName,
		},
		Driver:         common.SERVICE_NAME,
		DeletionPolicy: snapshotv1.VolumeSnapshotContentDelete,
		Parameters: map[string]string{
			"csi.storage.k8s.io/group-snapshotter-secret-name":      "infinibox-creds",
			"csi.storage.k8s.io/group-snapshotter-secret-namespace": "infinidat-csi",
			"infinidat.com/cgname":                                  testConfig.TestNames.SCName,
		},
	}

	createdVgsc, err := testConfig.GroupSnapshotClient.VolumeGroupSnapshotClasses().Create(context.Background(), vgsc, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("error creating VolumeGroupSnapshotClass %s", err.Error())
	}

	t.Logf("created VolumeGroupSnapshotClass %s", createdVgsc.Name)
	time.Sleep(time.Second * 5)

	// create the VolumeGroupSnapshot
	const VGSName = "mygroup-volumegroupsnapshot"
	vgs := &v1alpha1.VolumeGroupSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name: VGSName,
		},
		Spec: v1alpha1.VolumeGroupSnapshotSpec{
			Source: v1alpha1.VolumeGroupSnapshotSource{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						vgLabelKey: vgLabelValue,
					},
				},
			},
			VolumeGroupSnapshotClassName: &createdVgsc.Name,
		},
	}

	createdVgs, err := testConfig.GroupSnapshotClient.VolumeGroupSnapshots(testConfig.TestNames.NSName).Create(context.Background(), vgs, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("error creating VolumeGroupSnapshot %s", err.Error())
	}

	time.Sleep(time.Second * 5)

	// verify the VolumeGroupSnapshot was created
	currentVgs, err := testConfig.GroupSnapshotClient.VolumeGroupSnapshots(testConfig.TestNames.NSName).Get(context.Background(), createdVgs.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("error creating VolumeGroupSnapshot %s", err.Error())
	}
	t.Logf("created VolumeGroupSnapshot %s", currentVgs.Name)

	// verify the VolumeGroupSnapshot is ready
	if *currentVgs.Status.ReadyToUse {
		t.Logf("VolumeGroupSnapshot %s is ready", currentVgs.Name)
	} else {
		t.Fatalf("VolumeGroupSnapshot %s is NOT ready", currentVgs.Name)
	}

	if *e2e.CleanUp {
		// delete the VolumeGroupSnapshot
		err := testConfig.GroupSnapshotClient.VolumeGroupSnapshots(testConfig.TestNames.NSName).Delete(context.Background(), createdVgs.Name, metav1.DeleteOptions{})
		if err != nil {
			t.Fatalf("error deleting VolumeGroupSnapshot %s", err.Error())
		}
		t.Logf("deleted VolumeGroupSnapshot %s", currentVgs.Name)

		// delete the VolumeGroupSnapshotClass
		err = testConfig.GroupSnapshotClient.VolumeGroupSnapshotClasses().Delete(context.Background(), vgcsName, metav1.DeleteOptions{})
		if err != nil {
			t.Fatalf("error deleting VolumeGroupSnapshotClass %s", err.Error())
		}

		t.Logf("deleted VolumeGroupSnapshotClass %s", createdVgsc.Name)

		e2e.TearDown(testConfig)
	} else {
		t.Log("not cleaning up namespace")
	}

}
