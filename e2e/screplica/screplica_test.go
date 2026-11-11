//go:build e2e

/*
Copyright 2026 Infinidat
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package screplica

import (
	"os"
	"strings"
	"testing"

	"github.com/infinidat/infinibox-csi-driver/api/clientgo"
	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/infinidat/infinibox-csi-driver/e2e"
	v1iboxcg "github.com/infinidat/infinibox-csi-driver/iboxcg/api/v1"
	v1iboxreplica "github.com/infinidat/infinibox-csi-driver/iboxreplica/api/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// this test will create an iboxreplica from a storageclass definition
func TestSCReplicaPVC(t *testing.T) {
	e2e.SetupSlog()

	testConfig, err := e2e.GetTestConfig(t, common.ProtocolISCSI)
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

	// validate the replica env vars
	validateReplicaEnv(t)

	linkRemoteSystemName := os.Getenv(e2e.ENV_IBOXREPLICA_LINK_REMOTE_SYSTEM_NAME)
	remotePVCNamespace := testConfig.TestNames.NSName

	// verify the replica link specified exists and is active
	//linkRemoteSystemName := os.Getenv(e2e.ENV_IBOXREPLICA_LINK_REMOTE_SYSTEM_NAME)
	links, err := testConfig.ClientService.IboxAPI.GetLinks(t.Context())
	if err != nil {
		t.Fatalf("error - %s finding replication links", err.Error())
	}
	var linkFound bool
	for _, l := range links {
		if l.RemoteSystemName == linkRemoteSystemName {
			linkFound = true
			t.Log("link " + l.RemoteSystemName + " found")
		}
	}
	if !linkFound {
		t.Fatalf("error - link %s not found", linkRemoteSystemName)
	}

	// tell the test harness to set up the replication parameters
	// to be used in the generated storage class
	testConfig.UseReplicaParameters = true

	e2e.Setup(t.Context(), testConfig)

	kclient, err := clientgo.BuildOffClusterClient(e2e.KubeConfigPath)
	if err != nil {
		t.Fatalf("error getting cluster client %s", err.Error())
	}

	pvc, err := testConfig.ClientSet.CoreV1().PersistentVolumeClaims(testConfig.TestNames.NSName).Get(t.Context(), testConfig.TestNames.PVCName, v1.GetOptions{})
	if err != nil {
		t.Fatalf("error getting existing PVC %s", err.Error())
	}

	replicaFoundObject := verifyReplica(kclient, pvc.Spec.VolumeName, t)

	// verify a PVC was created for the remote volume
	remotepvc := verifyPVC(testConfig, t)

	if *e2e.CleanUp {
		e2e.TearDown(t.Context(), testConfig)

		// delete the iboxreplica created by this test
		err := kclient.DeleteIboxreplica(t.Context(), *replicaFoundObject)
		if err != nil {
			t.Fatalf("error deleting iboxreplica %s  namespace %s - error %s", replicaFoundObject.Name, replicaFoundObject.Namespace, err.Error())
		}
		t.Log("deleted iboxreplica " + replicaFoundObject.Name)

		// delete the pvc for the remote replica that was created by this test
		err = testConfig.ClientSet.CoreV1().PersistentVolumeClaims(remotePVCNamespace).Delete(t.Context(), remotepvc.Name, v1.DeleteOptions{})
		if err != nil {
			t.Fatalf("error deleting pvc %s namespace %s error - %s", remotepvc.Name, remotePVCNamespace, err.Error())
		}
		t.Log("deleted pvc " + remotepvc.Name)
	} else {
		t.Log("not cleaning up namespace")
	}
	err = e2e.CleanISCI(t.Context(), *testConfig)
	if err != nil {
		t.Logf("error cleaning ISCSI %s on node %s\n", err.Error(), testConfig.NodeName)
	}

}

// this test will create an iboxreplica for a NFS file system from a storageclass definition
// relocate this to a nfs test instead of mixing in with iscsi tests
func TestNFSSCReplicaPVC(t *testing.T) {
	e2e.SetupSlog()

	testConfig, err := e2e.GetTestConfig(t, common.ProtocolNFS)
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

	nfsNetworkSpace := os.Getenv(e2e.ENV_NAS_NETWORK_SPACE)
	if nfsNetworkSpace == "" {
		t.Fatalf("error - %s env var is required for this test", e2e.ENV_NAS_NETWORK_SPACE)
	}
	err = os.Setenv(e2e.ENV_IBOXREPLICA_REMOTE_NETWORK_SPACE, nfsNetworkSpace)
	if err != nil {
		t.Fatalf("error setting remote network space %s\n", err.Error())
	}
	t.Logf("for NFS test, set remote network space to [%s]", nfsNetworkSpace)

	// validate the replica env vars
	validateReplicaEnv(t)

	linkRemoteSystemName := os.Getenv(e2e.ENV_IBOXREPLICA_LINK_REMOTE_SYSTEM_NAME)
	remotePVCNamespace := testConfig.TestNames.NSName

	// verify the replica link specified exists and is active
	//linkRemoteSystemName := os.Getenv(e2e.ENV_IBOXREPLICA_LINK_REMOTE_SYSTEM_NAME)
	links, err := testConfig.ClientService.IboxAPI.GetLinks(t.Context())
	if err != nil {
		t.Fatalf("error - %s finding replication links", err.Error())
	}
	var linkFound bool
	for _, l := range links {
		if l.RemoteSystemName == linkRemoteSystemName {
			linkFound = true
			t.Log("link " + l.RemoteSystemName + " found")
		}
	}
	if !linkFound {
		t.Fatalf("error - link %s not found", linkRemoteSystemName)
	}

	// tell the test harness to set up the replication parameters
	// to be used in the generated storage class
	testConfig.UseReplicaParameters = true

	e2e.Setup(t.Context(), testConfig)

	kclient, err := clientgo.BuildOffClusterClient(e2e.KubeConfigPath)
	if err != nil {
		t.Fatalf("error getting cluster client %s", err.Error())
	}

	pvc, err := testConfig.ClientSet.CoreV1().PersistentVolumeClaims(testConfig.TestNames.NSName).Get(t.Context(), testConfig.TestNames.PVCName, v1.GetOptions{})
	if err != nil {
		t.Fatalf("error getting existing PVC %s", err.Error())
	}

	replicaFoundObject := verifyReplica(kclient, pvc.Spec.VolumeName, t)

	// verify a PVC was created for the remote volume
	remotepvc := verifyPVC(testConfig, t)

	if *e2e.CleanUp {
		e2e.TearDown(t.Context(), testConfig)

		// delete the iboxreplica created by this test
		err := kclient.DeleteIboxreplica(t.Context(), *replicaFoundObject)
		if err != nil {
			t.Fatalf("error deleting iboxreplica %s  namespace %s - error %s", replicaFoundObject.Name, replicaFoundObject.Namespace, err.Error())
		}
		t.Log("deleted iboxreplica " + replicaFoundObject.Name)

		// delete the pvc for the remote replica that was created by this test
		err = testConfig.ClientSet.CoreV1().PersistentVolumeClaims(remotePVCNamespace).Delete(t.Context(), remotepvc.Name, v1.DeleteOptions{})
		if err != nil {
			t.Fatalf("error deleting pvc %s namespace %s error - %s", remotepvc.Name, remotePVCNamespace, err.Error())
		}
		t.Log("deleted pvc " + remotepvc.Name)
	} else {
		t.Log("not cleaning up namespace")
	}
	err = e2e.CleanISCI(t.Context(), *testConfig)
	if err != nil {
		t.Logf("error cleaning %s on node %s\n", err.Error(), testConfig.NodeName)
	}

}

// this test will create an iboxcg, adding the volume into the CG
func TestSCReplicaCG(t *testing.T) {
	e2e.SetupSlog()

	testConfig, err := e2e.GetTestConfig(t, common.ProtocolISCSI)
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

	// validate the replica env vars
	validateReplicaEnv(t)

	linkRemoteSystemName := os.Getenv(e2e.ENV_IBOXREPLICA_LINK_REMOTE_SYSTEM_NAME)
	remotePVCNamespace := testConfig.TestNames.NSName

	// verify the replica link specified exists and is active
	//linkRemoteSystemName := os.Getenv(e2e.ENV_IBOXREPLICA_LINK_REMOTE_SYSTEM_NAME)
	links, err := testConfig.ClientService.IboxAPI.GetLinks(t.Context())
	if err != nil {
		t.Fatalf("error - %s finding replication links", err.Error())
	}
	var linkFound bool
	for _, l := range links {
		if l.RemoteSystemName == linkRemoteSystemName {
			linkFound = true
			t.Log("link " + l.RemoteSystemName + " found")
		}
	}
	if !linkFound {
		t.Fatalf("error - link %s not found", linkRemoteSystemName)
	}

	// tell the test harness to set up the replication parameters
	// to be used in the generated storage class
	testConfig.UseReplicaParameters = true
	testConfig.CGName = os.Getenv(e2e.ENV_IBOXREPLICA_CG)

	e2e.Setup(t.Context(), testConfig)

	kclient, err := clientgo.BuildOffClusterClient(e2e.KubeConfigPath)
	if err != nil {
		t.Fatalf("error getting cluster client %s", err.Error())
	}

	pvc, err := testConfig.ClientSet.CoreV1().PersistentVolumeClaims(testConfig.TestNames.NSName).Get(t.Context(), testConfig.TestNames.PVCName, v1.GetOptions{})
	if err != nil {
		t.Fatalf("error getting existing PVC %s", err.Error())
	}

	// verify a PVC was created for the remote volume
	remotepvc := verifyPVC(testConfig, t)

	iboxcgFoundObject := verifyCG(kclient, pvc.Spec.VolumeName, t)

	if *e2e.CleanUp {
		e2e.TearDown(t.Context(), testConfig)

		// delete the iboxreplica created by this test
		err := kclient.DeleteIboxcg(t.Context(), *iboxcgFoundObject)
		if err != nil {
			t.Fatalf("error deleting iboxcg %s  namespace %s - error %s", iboxcgFoundObject.Name, iboxcgFoundObject.Namespace, err.Error())
		}
		t.Log("deleted iboxcg " + iboxcgFoundObject.Name)

		// delete the pvc for the remote replica that was created by this test
		err = testConfig.ClientSet.CoreV1().PersistentVolumeClaims(remotePVCNamespace).Delete(t.Context(), remotepvc.Name, v1.DeleteOptions{})
		if err != nil {
			t.Fatalf("error deleting pvc %s namespace %s error - %s", remotepvc.Name, remotePVCNamespace, err.Error())
		}
		t.Log("deleted pvc " + remotepvc.Name)
	} else {
		t.Log("not cleaning up namespace")
	}
	err = e2e.CleanISCI(t.Context(), *testConfig)
	if err != nil {
		t.Logf("error cleaning ISCSI %s on node %s\n", err.Error(), testConfig.NodeName)
	}

}

// this test should create a PVC to the remote replica
func TestSCReplicaPVCAndRemotePVC(t *testing.T) {
	e2e.SetupSlog()

	testConfig, err := e2e.GetTestConfig(t, common.ProtocolISCSI)
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

	// validate the replica env vars
	validateReplicaEnv(t)

	linkRemoteSystemName := os.Getenv(e2e.ENV_IBOXREPLICA_LINK_REMOTE_SYSTEM_NAME)
	remotePVCNamespace := testConfig.TestNames.NSName

	// verify the replica link specified exists and is active
	//linkRemoteSystemName := os.Getenv(e2e.ENV_IBOXREPLICA_LINK_REMOTE_SYSTEM_NAME)
	links, err := testConfig.ClientService.IboxAPI.GetLinks(t.Context())
	if err != nil {
		t.Fatalf("error - %s finding replication links", err.Error())
	}
	var linkFound bool
	for _, l := range links {
		if l.RemoteSystemName == linkRemoteSystemName {
			linkFound = true
			t.Log("link " + l.RemoteSystemName + " found")
		}
	}
	if !linkFound {
		t.Fatalf("error - link %s not found", linkRemoteSystemName)
	}

	// tell the test harness to set up the replication parameters
	// to be used in the generated storage class
	testConfig.UseReplicaParameters = true
	testConfig.RemoteCreatePVC = true

	e2e.Setup(t.Context(), testConfig)

	kclient, err := clientgo.BuildOffClusterClient(e2e.KubeConfigPath)
	if err != nil {
		t.Fatalf("error getting cluster client %s", err.Error())
	}

	pvc, err := testConfig.ClientSet.CoreV1().PersistentVolumeClaims(testConfig.TestNames.NSName).Get(t.Context(), testConfig.TestNames.PVCName, v1.GetOptions{})
	if err != nil {
		t.Fatalf("error getting existing PVC %s", err.Error())
	}
	replicaFoundObject := verifyReplica(kclient, pvc.Spec.VolumeName, t)

	// verify a PVC was created for the remote volume
	remotepvc := verifyPVC(testConfig, t)

	if *e2e.CleanUp {
		e2e.TearDown(t.Context(), testConfig)

		// delete the iboxreplica created by this test
		err := kclient.DeleteIboxreplica(t.Context(), *replicaFoundObject)
		if err != nil {
			t.Fatalf("error deleting iboxreplica %s  namespace %s - error %s", replicaFoundObject.Name, replicaFoundObject.Namespace, err.Error())
		}
		t.Log("deleted iboxreplica " + replicaFoundObject.Name)

		// delete the pvc for the remote replica that was created by this test
		err = testConfig.ClientSet.CoreV1().PersistentVolumeClaims(remotePVCNamespace).Delete(t.Context(), remotepvc.Name, v1.DeleteOptions{})
		if err != nil {
			t.Fatalf("error deleting pvc %s namespace %s error - %s", remotepvc.Name, remotePVCNamespace, err.Error())
		}
		t.Log("deleted pvc " + remotepvc.Name)
	} else {
		t.Log("not cleaning up namespace")
	}
	err = e2e.CleanISCI(t.Context(), *testConfig)
	if err != nil {
		t.Logf("error cleaning ISCSI %s on node %s\n", err.Error(), testConfig.NodeName)
	}

}

func validateReplicaEnv(t *testing.T) {
	// validate the replica env vars
	replicationType := os.Getenv(e2e.ENV_IBOXREPLICA_REPLICATION_TYPE)
	if replicationType == "" {
		t.Fatalf("error - %s env var is required for this test", e2e.ENV_IBOXREPLICA_REPLICATION_TYPE)
	}

	linkRemoteSystemName := os.Getenv(e2e.ENV_IBOXREPLICA_LINK_REMOTE_SYSTEM_NAME)
	if linkRemoteSystemName == "" {
		t.Fatalf("error - %s env var is required for this test", e2e.ENV_IBOXREPLICA_LINK_REMOTE_SYSTEM_NAME)
	}

	localSecretName := os.Getenv(e2e.ENV_IBOXREPLICA_LOCAL_IBOX_SECRET_NAME)
	if localSecretName == "" {
		t.Fatalf("error - %s env var is required for this test", e2e.ENV_IBOXREPLICA_LOCAL_IBOX_SECRET_NAME)
	}

	localSecretNamespace := os.Getenv(e2e.ENV_IBOXREPLICA_LOCAL_IBOX_SECRET_NAMESPACE)
	if localSecretNamespace == "" {
		t.Fatalf("error - %s env var is required for this test", e2e.ENV_IBOXREPLICA_LOCAL_IBOX_SECRET_NAMESPACE)
	}

	remoteIboxSecretName := os.Getenv(e2e.ENV_IBOXREPLICA_REMOTE_IBOX_SECRET_NAME)
	if remoteIboxSecretName == "" {
		t.Fatalf("error - %s env var is required for this test", e2e.ENV_IBOXREPLICA_REMOTE_IBOX_SECRET_NAME)
	}

	remoteIboxSecretNamespace := os.Getenv(e2e.ENV_IBOXREPLICA_REMOTE_IBOX_SECRET_NAMESPACE)
	if remoteIboxSecretNamespace == "" {
		t.Fatalf("error - %s env var is required for this test", e2e.ENV_IBOXREPLICA_REMOTE_IBOX_SECRET_NAMESPACE)
	}

	remoteCreatePVC := os.Getenv(e2e.ENV_IBOXREPLICA_REMOTE_CREATE_PVC)
	if remoteCreatePVC == "" {
		t.Fatalf("error - %s env var is required for this test", e2e.ENV_IBOXREPLICA_REMOTE_CREATE_PVC)
	}

	remotePVCNameSuffix := os.Getenv(e2e.ENV_IBOXREPLICA_REMOTE_PVC_NAME_SUFFIX)
	if remotePVCNameSuffix == "" {
		t.Fatalf("error - %s env var is required for this test", e2e.ENV_IBOXREPLICA_REMOTE_PVC_NAME_SUFFIX)
	}
	remotePVCNamespace := os.Getenv(e2e.ENV_IBOXREPLICA_REMOTE_PVC_NAMESPACE)
	if remotePVCNamespace == "" {
		t.Fatalf("error - %s env var is required for this test", e2e.ENV_IBOXREPLICA_REMOTE_PVC_NAMESPACE)
	}
	remoteNetworkSpace := os.Getenv(e2e.ENV_IBOXREPLICA_REMOTE_NETWORK_SPACE)
	if remoteNetworkSpace == "" {
		t.Fatalf("error - %s env var is required for this test", e2e.ENV_IBOXREPLICA_REMOTE_NETWORK_SPACE)
	}
	remotePoolName := os.Getenv(e2e.ENV_IBOXREPLICA_REMOTE_POOL_NAME)
	if remotePoolName == "" {
		t.Fatalf("error - %s env var is required for this test", e2e.ENV_IBOXREPLICA_REMOTE_POOL_NAME)
	}
}

func verifyPVC(testConfig *e2e.TestConfig, t *testing.T) (pvc *corev1.PersistentVolumeClaim) {
	uniqueSuffix := os.Getenv(e2e.ENV_IBOXREPLICA_LINK_REMOTE_SYSTEM_NAME)
	t.Log("unique suffix for this test run" + uniqueSuffix)

	remotePVCNamespace := testConfig.TestNames.NSName
	pvcList, err := testConfig.ClientSet.CoreV1().PersistentVolumeClaims(remotePVCNamespace).List(t.Context(), v1.ListOptions{})
	if err != nil {
		t.Fatalf("error getting pvcs %s in namespace %s", err.Error(), remotePVCNamespace)
	}

	for _, p := range pvcList.Items {
		t.Log("looking for pvc " + p.Name + " with suffix " + uniqueSuffix + " in namespace " + remotePVCNamespace)
		if strings.Contains(p.Name, uniqueSuffix) {
			t.Log("pvc " + p.Name + " found")
			return &p
		}

	}

	t.Fatalf("error finding pvc %s", uniqueSuffix)

	return nil
}

func verifyReplica(kclient *clientgo.KubeClient, volumeName string, t *testing.T) (replicaFoundObject *v1iboxreplica.Iboxreplica) {
	// verify that an iboxreplica was created, we do this using
	// the unique namespace value as the suffix simply so we can
	// query the iboxreplicas and see if an iboxreplica was created
	// for this run
	// replica's 'remote_pvc_name_suffix' should match the pvc suffix name + unique test suffix
	iboxReplicas, err := kclient.GetIboxreplicas(t.Context())
	if err != nil {
		t.Fatalf("error getting iboxreplicas %s", err.Error())
	}

	uniqueName := "iboxreplica-" + volumeName
	t.Log("verifying iboxreplica " + uniqueName)

	for _, rep := range iboxReplicas.Items {
		if rep.Name == uniqueName {
			t.Log("replica " + rep.Name + " found")
			return &rep
		}
	}

	t.Fatalf("error finding iboxreplica %s", uniqueName)
	return nil
}

// format for iboxcg is 'iboxcg-<pv name>'
func verifyCG(kclient *clientgo.KubeClient, volumeName string, t *testing.T) (cgFoundObject *v1iboxcg.Iboxcg) {
	iboxcgName := "iboxcg-" + volumeName
	iboxCGs, err := kclient.GetIboxcgs(t.Context())
	if err != nil {
		t.Fatalf("error getting iboxcgs %s", err.Error())
	}

	for _, cg := range iboxCGs.Items {
		if cg.Name == iboxcgName {
			t.Logf("iboxcg %s found", cg.Spec.RemotePVCNameSuffix)
			return &cg
		}
	}

	t.Fatalf("error finding iboxcg %s", iboxcgName)
	return nil
}
