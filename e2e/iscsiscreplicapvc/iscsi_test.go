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

//go:build e2e

package iscsiscreplicapvc

import (
	"os"
	"strings"
	"testing"

	"github.com/infinidat/infinibox-csi-driver/api/clientgo"
	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/infinidat/infinibox-csi-driver/e2e"
	v1iboxreplica "github.com/infinidat/infinibox-csi-driver/iboxreplica/api/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestIscsiSCReplicaPVC(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, common.ProtocolISCSI)
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

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

	// verify that an iboxreplica was created, we do this using
	// the unique namespace value as the suffix simply so we can
	// query the iboxreplicas and see if an iboxreplica was created
	// for this run
	// replica's 'remote_pvc_name_suffix' should match the pvc suffix name + unique test suffix
	kclient, err := clientgo.BuildOffClusterClient(e2e.KubeConfigPath)
	if err != nil {
		t.Fatalf("error getting cluster client %s", err.Error())
	}
	iboxReplicas, err := kclient.GetIboxreplicas(t.Context())
	if err != nil {
		t.Fatalf("error getting iboxreplicas %s", err.Error())
	}

	uniqueSuffix := os.Getenv(e2e.ENV_IBOXREPLICA_REMOTE_PVC_NAME_SUFFIX) + testConfig.TestNames.UniqueSuffix
	t.Log("unique suffix for this test run" + uniqueSuffix)

	var replicaFound bool
	var replicaFoundObject v1iboxreplica.Iboxreplica

	for _, rep := range iboxReplicas.Items {
		if rep.Spec.RemotePVCNameSuffix == uniqueSuffix {
			replicaFound = true
			t.Log("replica " + rep.Spec.RemotePVCNameSuffix + " found")
			replicaFoundObject = rep
		}
	}
	if !replicaFound {
		t.Fatalf("error finding iboxreplica %s", uniqueSuffix)
	}

	// verify a PVC was created for the remote volume
	// PVC's name should match the pvc suffix name + unique test suffix
	pvcList, err := testConfig.ClientSet.CoreV1().PersistentVolumeClaims(remotePVCNamespace).List(t.Context(), v1.ListOptions{})
	if err != nil {
		t.Fatalf("error getting pvcs %s in namespace %s", err.Error(), remotePVCNamespace)
	}

	var pvcFound bool
	var pvcFoundName string
	for _, p := range pvcList.Items {
		if strings.Contains(p.Name, uniqueSuffix) {
			pvcFound = true
			t.Log("pvc " + p.Name + " found")
			pvcFoundName = p.Name
		}

	}
	if !pvcFound {
		t.Fatalf("error finding pvc %s", uniqueSuffix)
	}

	if *e2e.CleanUp {
		e2e.TearDown(t.Context(), testConfig)

		// delete the iboxreplica created by this test
		err := kclient.DeleteIboxreplica(t.Context(), replicaFoundObject)
		if err != nil {
			t.Fatalf("error deleting iboxreplica %s  namespace %s - error %s", replicaFoundObject.Name, replicaFoundObject.Namespace, err.Error())
		}
		t.Log("deleted iboxreplica " + replicaFoundObject.Name)

		// delete the pvc for the remote replica that was created by this test
		err = testConfig.ClientSet.CoreV1().PersistentVolumeClaims(remotePVCNamespace).Delete(t.Context(), pvcFoundName, v1.DeleteOptions{})
		if err != nil {
			t.Fatalf("error deleting pvc %s namespace %s error - %s", pvcFoundName, remotePVCNamespace, err.Error())
		}
		t.Log("deleted pvc " + pvcFoundName)
	} else {
		t.Log("not cleaning up namespace")
	}
	err = e2e.CleanISCI(t.Context(), *testConfig)
	if err != nil {
		t.Logf("error cleaning ISCSI %s on node %s\n", err.Error(), testConfig.NodeName)
	}

}
