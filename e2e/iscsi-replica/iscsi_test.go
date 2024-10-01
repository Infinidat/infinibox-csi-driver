//go:build e2e

package iscsireplica

import (
	"context"
	"infinibox-csi-driver/api/clientgo"
	v1 "infinibox-csi-driver/api/v1"
	"infinibox-csi-driver/common"
	"infinibox-csi-driver/e2e"
	"infinibox-csi-driver/log"
	"os"
	"strconv"
	"testing"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/go-logr/zerologr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestIscsiReplica(t *testing.T) {

	l := log.Get()
	ctrl.SetLogger(zerologr.New(&l))

	testConfig, err := e2e.GetTestConfig(t, common.PROTOCOL_ISCSI)
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

	e2e.Setup(testConfig)

	// at this point we should have a running iscsi volume, as a test we'll create a replica for that

	// get the volume name from the PVC, which will be the PV name

	existingPVC, err := testConfig.ClientSet.CoreV1().PersistentVolumeClaims(testConfig.TestNames.NSName).Get(context.TODO(), testConfig.TestNames.PVCName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("error getting existing PVC %s", err.Error())
	}

	t.Logf("existing PVC %s has %s volume", existingPVC.Name, existingPVC.Spec.VolumeName)

	linkRemoteSystemName := os.Getenv("_E2E_IBOX_LINK_REMOTE_SYSTEM_NAME")
	if linkRemoteSystemName == "" {
		t.Fatal("_E2E_IBOX_LINK_REMOTE_SYSTEM_NAME env var is not set and is required for this test")
	}
	remotePoolID := os.Getenv("_E2E_IBOX_REMOTE_POOL_ID")
	if remotePoolID == "" {
		t.Fatal("_E2E_IBOX_REMOTE_POOL_ID env var is not set and is required for this test")
	}
	poolID, err := strconv.Atoi(remotePoolID)
	if err != nil {
		t.Fatalf("_E2E_IBOX_REMOTE_POOL_ID env var is not a valid integer %s", remotePoolID)
	}
	secretName := os.Getenv("_E2E_IBOX_SECRET")
	if secretName == "" {
		t.Fatal("_E2E_IBOX_SECRET env var is not set and is required for this test")
	}
	secretNamespace := os.Getenv("_E2E_NAMESPACE")
	if secretNamespace == "" {
		t.Fatal("_E2E_NAMESPACE env var is not set and is required for this test")
	}
	// create the iboxreplica CR
	replica := v1.Iboxreplica{
		ObjectMeta: metav1.ObjectMeta{
			Name: "iboxreplica-volume-e2e-test",
			Annotations: map[string]string{
				common.PVC_ANNOTATION_SECRET_NAME:      secretName,
				common.PVC_ANNOTATION_SECRET_NAMESPACE: secretNamespace,
			},
		},
		Spec: v1.IboxreplicaSpec{
			Description:          "iscsi-test-volume-replica",
			EntityType:           common.REPLICA_ENTITY_VOLUME,
			LocalEntityName:      existingPVC.Spec.VolumeName,
			LinkRemoteSystemName: linkRemoteSystemName,
			RemotePoolID:         poolID,
		},
	}
	t.Logf("creating iboxreplica %s", replica.Name)

	kclient, err := clientgo.BuildOffClusterClient(*e2e.KubeConfigPath)
	if err != nil {
		t.Fatalf("error getting cluster client %s", err.Error())
	}
	err = kclient.CreateIboxreplica(replica)
	if err != nil {
		t.Fatalf("error creating iboxreplica %s", err.Error())
	}

	// verify the iboxreplica status is ACTIVE
	time.Sleep(time.Second * 5)

	runningReplica, err := kclient.GetIboxreplica(replica.Name)
	if err != nil {
		t.Fatalf("error getting iboxreplica %s", err.Error())
	}
	t.Logf("iboxreplica created ID %d State %s", runningReplica.Status.ID, runningReplica.Status.State)
	if runningReplica.Status.State != "ACTIVE" {
		t.Fatalf("error iboxreplica state is not ACTIVE %s", runningReplica.Status.State)
	}

	if *e2e.CleanUp {
		e2e.TearDown(testConfig)
		// delete the iboxreplica
		err := kclient.DeleteIboxreplica(runningReplica)
		if err != nil {
			t.Fatalf("error deleting iboxreplica %s", err.Error())
		}
	} else {
		t.Log("not cleaning up namespace")
	}

}
