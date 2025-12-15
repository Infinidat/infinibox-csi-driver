//go:build e2e

package iscsireplica

import (
	"log/slog"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/infinidat/infinibox-csi-driver/api/clientgo"
	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/infinidat/infinibox-csi-driver/e2e"
	v1 "github.com/infinidat/infinibox-csi-driver/iboxreplica/api/v1"

	ctrl "sigs.k8s.io/controller-runtime"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestIscsiReplica(t *testing.T) {
	setupSlog()

	testConfig, err := e2e.GetTestConfig(t, common.ProtocolISCSI)
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

	e2e.Setup(t.Context(), testConfig)

	// at this point we should have a running iscsi volume, as a test we'll create a replica for that

	// get the volume name from the PVC, which will be the PV name

	existingPVC, err := testConfig.ClientSet.CoreV1().PersistentVolumeClaims(testConfig.TestNames.NSName).Get(t.Context(), testConfig.TestNames.PVCName, metav1.GetOptions{})
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
				common.PVCAnnotationSecretName:      secretName,
				common.PVCAnnotationSecretNamespace: secretNamespace,
			},
		},
		Spec: v1.IboxreplicaSpec{
			Description:          "iscsi-test-volume-replica",
			EntityType:           common.ReplicaEntityVolume,
			LocalEntityName:      existingPVC.Spec.VolumeName,
			LinkRemoteSystemName: linkRemoteSystemName,
			ReplicationType:      common.ReplicationTypeASYNC,
			RemotePoolID:         poolID,
		},
	}
	t.Logf("creating iboxreplica %s", replica.Name)

	kclient, err := clientgo.BuildOffClusterClient(*e2e.KubeConfigPath)
	if err != nil {
		t.Fatalf("error getting cluster client %s", err.Error())
	}
	err = kclient.CreateIboxreplica(t.Context(), replica)
	if err != nil {
		t.Fatalf("error creating iboxreplica %s", err.Error())
	}

	// verify the iboxreplica status is ACTIVE
	time.Sleep(time.Second * 5)

	runningReplica, err := kclient.GetIboxreplica(t.Context(), replica.Name)
	if err != nil {
		t.Fatalf("error getting iboxreplica %s", err.Error())
	}
	t.Logf("iboxreplica created ID %d State %s", runningReplica.Status.ID, runningReplica.Status.State)
	if runningReplica.Status.State != "ACTIVE" {
		t.Fatalf("error iboxreplica state is not ACTIVE %s", runningReplica.Status.State)
	}

	if *e2e.CleanUp {
		e2e.TearDown(t.Context(), testConfig)
		// delete the iboxreplica
		err := kclient.DeleteIboxreplica(t.Context(), runningReplica)
		if err != nil {
			t.Fatalf("error deleting iboxreplica %s", err.Error())
		}
	} else {
		t.Log("not cleaning up namespace")
	}

	err = e2e.CleanISCI(t.Context(), *testConfig)
	if err != nil {
		t.Logf("error cleaning ISCSI %s on node %s\n", err.Error(), testConfig.NodeName)
	}

}

func TestIscsiActiveActiveReplica(t *testing.T) {

	setupSlog()

	testConfig, err := e2e.GetTestConfig(t, common.ProtocolISCSI)
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

	e2e.Setup(t.Context(), testConfig)

	// at this point we should have a running iscsi volume, as a test we'll create a replica for that

	// get the volume name from the PVC, which will be the PV name

	existingPVC, err := testConfig.ClientSet.CoreV1().PersistentVolumeClaims(testConfig.TestNames.NSName).Get(t.Context(), testConfig.TestNames.PVCName, metav1.GetOptions{})
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
				common.PVCAnnotationSecretName:      secretName,
				common.PVCAnnotationSecretNamespace: secretNamespace,
			},
		},
		Spec: v1.IboxreplicaSpec{
			Description:          "iscsi-test-volume-aa-replica",
			EntityType:           common.ReplicaEntityVolume,
			LocalEntityName:      existingPVC.Spec.VolumeName,
			LinkRemoteSystemName: linkRemoteSystemName,
			RemotePoolID:         poolID,
			IsPreferred:          isPreferredPtr,
			ReplicationType:      common.IboxreplicaReplicaTypeACTIVE_ACTIVE,
		},
	}
	t.Logf("creating iboxreplica %s", replica.Name)

	kclient, err := clientgo.BuildOffClusterClient(*e2e.KubeConfigPath)
	if err != nil {
		t.Fatalf("error getting cluster client %s", err.Error())
	}
	err = kclient.CreateIboxreplica(t.Context(), replica)
	if err != nil {
		t.Fatalf("error creating iboxreplica %s", err.Error())
	}

	// verify the iboxreplica status is ACTIVE
	time.Sleep(time.Second * 5)

	runningReplica, err := kclient.GetIboxreplica(t.Context(), replica.Name)
	if err != nil {
		t.Fatalf("error getting iboxreplica %s", err.Error())
	}
	t.Logf("iboxreplica created ID %d State %s", runningReplica.Status.ID, runningReplica.Status.State)
	if runningReplica.Status.State != "ACTIVE" {
		t.Fatalf("error iboxreplica state is not ACTIVE %s", runningReplica.Status.State)
	}

	if *e2e.CleanUp {
		e2e.TearDown(t.Context(), testConfig)
		// delete the iboxreplica
		err := kclient.DeleteIboxreplica(t.Context(), runningReplica)
		if err != nil {
			t.Fatalf("error deleting iboxreplica %s", err.Error())
		}
	} else {
		t.Log("not cleaning up namespace")
	}
	err = e2e.CleanISCI(t.Context(), *testConfig)
	if err != nil {
		t.Logf("error cleaning ISCSI %s on node %s\n", err.Error(), testConfig.NodeName)
	}

}

func TestIscsiSyncReplica(t *testing.T) {
	setupSlog()

	testConfig, err := e2e.GetTestConfig(t, common.ProtocolISCSI)
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

	e2e.Setup(t.Context(), testConfig)

	// at this point we should have a running iscsi volume, as a test we'll create a replica for that

	// get the volume name from the PVC, which will be the PV name

	existingPVC, err := testConfig.ClientSet.CoreV1().PersistentVolumeClaims(testConfig.TestNames.NSName).Get(t.Context(), testConfig.TestNames.PVCName, metav1.GetOptions{})
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
				common.PVCAnnotationSecretName:      secretName,
				common.PVCAnnotationSecretNamespace: secretNamespace,
			},
		},
		Spec: v1.IboxreplicaSpec{
			Description:          "iscsi-test-volume-sync-replica",
			EntityType:           common.ReplicaEntityVolume,
			LocalEntityName:      existingPVC.Spec.VolumeName,
			LinkRemoteSystemName: linkRemoteSystemName,
			RemotePoolID:         poolID,
			IsPreferred:          isPreferredPtr,
			ReplicationType:      common.IboxreplicaReplicaTypeSYNC,
		},
	}
	t.Logf("creating iboxreplica %s", replica.Name)

	kclient, err := clientgo.BuildOffClusterClient(*e2e.KubeConfigPath)
	if err != nil {
		t.Fatalf("error getting cluster client %s", err.Error())
	}
	err = kclient.CreateIboxreplica(t.Context(), replica)
	if err != nil {
		t.Fatalf("error creating iboxreplica %s", err.Error())
	}

	// verify the iboxreplica status is ACTIVE
	time.Sleep(time.Second * 5)

	runningReplica, err := kclient.GetIboxreplica(t.Context(), replica.Name)
	if err != nil {
		t.Fatalf("error getting iboxreplica %s", err.Error())
	}
	t.Logf("iboxreplica created ID %d State %s", runningReplica.Status.ID, runningReplica.Status.State)
	if runningReplica.Status.State != "ACTIVE" {
		t.Fatalf("error iboxreplica state is not ACTIVE %s", runningReplica.Status.State)
	}

	if *e2e.CleanUp {
		e2e.TearDown(t.Context(), testConfig)
		// delete the iboxreplica
		err := kclient.DeleteIboxreplica(t.Context(), runningReplica)
		if err != nil {
			t.Fatalf("error deleting iboxreplica %s", err.Error())
		}
	} else {
		t.Log("not cleaning up namespace")
	}
	err = e2e.CleanISCI(t.Context(), *testConfig)
	if err != nil {
		t.Logf("error cleaning ISCSI %s on node %s\n", err.Error(), testConfig.NodeName)
	}

}
func customTimeFormatter(groups []string, a slog.Attr) slog.Attr {
	if a.Key == slog.TimeKey {
		// Cast the value to time.Time
		t := a.Value.Any().(time.Time)
		// Format the time as desired (e.g., "2006-01-02 15:04:05 MST")
		a.Value = slog.StringValue(t.Format("2006-01-02 15:04:05.000 MST"))
	}
	return a
}

func setupSlog() {
	opts := &slog.HandlerOptions{
		Level:       slog.LevelDebug,
		AddSource:   true,
		ReplaceAttr: customTimeFormatter,
	}

	ThisLogger := slog.New(slog.NewJSONHandler(os.Stdout, opts))

	// Set the default logger
	slog.SetDefault(ThisLogger)

	ctrl.SetLogger(logr.FromSlogHandler(ThisLogger.Handler()))
}
