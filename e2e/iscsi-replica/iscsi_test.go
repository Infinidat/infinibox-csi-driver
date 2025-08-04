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

	linkRemoteSystemName := os.Getenv(e2e.ENV_IBOXREPLICA_LINK_REMOTE_SYSTEM_NAME)
	if linkRemoteSystemName == "" {
		t.Fatalf("%s env var is not set and is required for this test", e2e.ENV_IBOXREPLICA_LINK_REMOTE_SYSTEM_NAME)
	}
	remotePoolID := os.Getenv(e2e.ENV_IBOXREPLICA_REMOTE_POOL_ID)
	if remotePoolID == "" {
		t.Fatalf("%s env var is not set and is required for this test", e2e.ENV_IBOXREPLICA_REMOTE_POOL_ID)
	}
	poolID, err := strconv.Atoi(remotePoolID)
	if err != nil {
		t.Fatalf("%s env var is not a valid integer %s", e2e.ENV_IBOXREPLICA_REMOTE_POOL_ID, remotePoolID)
	}
	secretName := os.Getenv(e2e.ENV_IBOX_SECRET)
	if secretName == "" {
		t.Fatalf("%s env var is not set and is required for this test", e2e.ENV_IBOX_SECRET)
	}
	secretNamespace := os.Getenv(e2e.ENV_NAMESPACE)
	if secretNamespace == "" {
		t.Fatalf("%s env var is not set and is required for this test", e2e.ENV_NAMESPACE)
	}

	replicaName := "iboxreplica-volume-e2e-test-" + testConfig.TestNames.UniqueSuffix

	// create the iboxreplica CR
	replica := v1.Iboxreplica{
		ObjectMeta: metav1.ObjectMeta{
			Name: replicaName,
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
			ReplicationType:      common.REPLICATION_TYPE_ASYNC,
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

	err = e2e.CleanISCI(*testConfig)
	if err != nil {
		t.Fatalf("error cleaning ISCSI %s on node %s\n", err.Error(), testConfig.NodeName)
	}

}

func TestIscsiActiveActiveReplica(t *testing.T) {

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

	linkRemoteSystemName := os.Getenv(e2e.ENV_IBOXREPLICA_LINK_REMOTE_SYSTEM_NAME)
	if linkRemoteSystemName == "" {
		t.Fatalf("%s env var is not set and is required for this test", e2e.ENV_IBOXREPLICA_LINK_REMOTE_SYSTEM_NAME)
	}
	remotePoolID := os.Getenv(e2e.ENV_IBOXREPLICA_REMOTE_POOL_ID)
	if remotePoolID == "" {
		t.Fatalf("%s env var is not set and is required for this test", e2e.ENV_IBOXREPLICA_REMOTE_POOL_ID)
	}
	poolID, err := strconv.Atoi(remotePoolID)
	if err != nil {
		t.Fatalf("%s env var is not a valid integer %s", e2e.ENV_IBOXREPLICA_REMOTE_POOL_ID, remotePoolID)
	}
	secretName := os.Getenv(e2e.ENV_IBOX_SECRET)
	if secretName == "" {
		t.Fatalf("%s env var is not set and is required for this test", e2e.ENV_IBOX_SECRET)
	}
	secretNamespace := os.Getenv(e2e.ENV_NAMESPACE)
	if secretNamespace == "" {
		t.Fatalf("%s env var is not set and is required for this test", e2e.ENV_NAMESPACE)
	}

	replicaName := "iboxreplica-volume-e2e-test-" + testConfig.TestNames.UniqueSuffix

	// create the iboxreplica CR
	var isPreferred bool
	isPreferred = true
	isPreferredPtr := &isPreferred

	replica := v1.Iboxreplica{
		ObjectMeta: metav1.ObjectMeta{
			Name: replicaName,
			Annotations: map[string]string{
				common.PVC_ANNOTATION_SECRET_NAME:      secretName,
				common.PVC_ANNOTATION_SECRET_NAMESPACE: secretNamespace,
			},
		},
		Spec: v1.IboxreplicaSpec{
			Description:          "iscsi-test-volume-aa-replica",
			EntityType:           common.REPLICA_ENTITY_VOLUME,
			LocalEntityName:      existingPVC.Spec.VolumeName,
			LinkRemoteSystemName: linkRemoteSystemName,
			RemotePoolID:         poolID,
			IsPreferred:          isPreferredPtr,
			ReplicationType:      common.IBOXREPLICA_REPLICA_TYPE_ACTIVE_ACTIVE,
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
	err = e2e.CleanISCI(*testConfig)
	if err != nil {
		t.Fatalf("error cleaning ISCSI %s on node %s\n", err.Error(), testConfig.NodeName)
	}

}
