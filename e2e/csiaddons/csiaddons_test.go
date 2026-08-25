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

package csiaddons

import (
	"os"
	"testing"
	"time"

	csiaddonsv1alpha1 "github.com/csi-addons/kubernetes-csi-addons/api/csiaddons/v1alpha1"
	replicationv1alpha1 "github.com/csi-addons/kubernetes-csi-addons/api/replication.storage/v1alpha1"
	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/infinidat/infinibox-csi-driver/e2e"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestVolumeReplication(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, common.ProtocolISCSI)
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

	e2e.Setup(t.Context(), testConfig)

	time.Sleep(time.Second * 5)

	s := runtime.NewScheme()
	scheme.AddToScheme(s)
	csiaddonsv1alpha1.AddToScheme(s)
	replicationv1alpha1.AddToScheme(s)

	// Create controller-runtime client
	addonsClient, err := client.New(testConfig.RestConfig, client.Options{Scheme: s})
	if err != nil {
		t.Fatalf("error creating csiaddons Client %s\n", err.Error())
	}
	provisioner := "infinibox-csi-driver"
	linkRemoteSystemName := os.Getenv(e2e.ENV_IBOXREPLICA_LINK_REMOTE_SYSTEM_NAME)
	if linkRemoteSystemName == "" {
		t.Fatalf("%s env var is not set and is required for this test", e2e.ENV_IBOXREPLICA_LINK_REMOTE_SYSTEM_NAME)
	}
	remotePoolName := os.Getenv(e2e.ENV_IBOXREPLICA_REMOTE_POOL_NAME)
	if remotePoolName == "" {
		t.Fatalf("%s env var is not set and is required for this test", e2e.ENV_IBOXREPLICA_REMOTE_POOL_NAME)
	}
	remoteIboxCredName := os.Getenv(e2e.ENV_IBOXREPLICA_REMOTE_IBOX_SECRET_NAME)
	if remoteIboxCredName == "" {
		t.Fatalf("%s env var is not set and is required for this test", e2e.ENV_IBOXREPLICA_REMOTE_IBOX_SECRET_NAME)
	}
	remoteIboxCredNamespace := os.Getenv(e2e.ENV_IBOXREPLICA_REMOTE_IBOX_SECRET_NAMESPACE)
	if remoteIboxCredNamespace == "" {
		t.Fatalf("%s env var is not set and is required for this test", e2e.ENV_IBOXREPLICA_REMOTE_IBOX_SECRET_NAMESPACE)
	}
	iboxCredName := os.Getenv(e2e.ENV_IBOX_SECRET)
	if iboxCredName == "" {
		t.Fatalf("%s env var is not set and is required for this test", e2e.ENV_IBOX_SECRET)
	}
	iboxCredNamespace := os.Getenv(e2e.ENV_NAMESPACE)
	if iboxCredNamespace == "" {
		t.Fatalf("%s env var is not set and is required for this test", e2e.ENV_NAMESPACE)
	}

	params := map[string]string{
		"replication.storage.openshift.io/replication-secret-name":      iboxCredName,
		"replication.storage.openshift.io/replication-secret-namespace": iboxCredNamespace,
		common.IboxReplicaRemoteIboxLinkNameParameter:                   linkRemoteSystemName,
		common.IboxReplicaCreatePVCPoolName:                             remotePoolName,
		common.IboxReplicaRemoteIboxCredNameParameter:                   remoteIboxCredName,
		common.IboxReplicaRemoteIboxCredNamespaceParameter:              remoteIboxCredNamespace,
	}
	vrc, err := e2e.CreateVolumeReplicationClass(addonsClient, t, testConfig, testConfig.TestNames.NSName, provisioner, params)
	if err != nil {
		t.Fatalf("error creating VolumeReplicationClass %s\n", err.Error())
	}
	t.Logf("VolumeReplicationClass %s created", vrc.Name)

	// create a VolumeReplication for the PVC that was create
	vr, err := e2e.CreateVolumeReplication(addonsClient, t, testConfig, testConfig.TestNames.NSName, testConfig.TestNames.PVCName, vrc.Name, replicationv1alpha1.Primary)
	if err != nil {
		t.Fatalf("error creating VolumeReplication %s\n", err.Error())
	}
	t.Logf("VolumeReplication %s/%s created", vr.Namespace, vr.Name)

	time.Sleep(time.Second * 5)
	// test state of VolumeReplication

	/**
	if *e2e.CleanUp {
		e2e.TearDown(t.Context(), testConfig)
		// cleanup VolumeReplication that was created
		err = e2e.DeleteVolumeReplication(addonsClient, t, testConfig, vr)
		if err != nil {
			t.Fatalf("error deleting VolumeReplication %s\n", err.Error())
		}
		err = e2e.DeleteVolumeReplicationClass(addonsClient, t, testConfig, vrc)
		if err != nil {
			t.Fatalf("error deleting VolumeReplication %s\n", err.Error())
		}
	} else {
		t.Log("not cleaning up namespace")
	}

	err = e2e.CleanISCI(t.Context(), *testConfig)
	if err != nil {
		t.Logf("error cleaning ISCSI %s on node %s\n", err.Error(), testConfig.NodeName)
	}
	*/

}

func XTestVolumeGroupReplication(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, common.ProtocolISCSI)
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

	s := runtime.NewScheme()
	scheme.AddToScheme(s)
	csiaddonsv1alpha1.AddToScheme(s)
	replicationv1alpha1.AddToScheme(s)

	// Create controller-runtime client
	addonsClient, err := client.New(testConfig.RestConfig, client.Options{Scheme: s})
	if err != nil {
		t.Fatalf("error creating csiaddons Client %s\n", err.Error())
	}
	provisioner := "infinibox-csi-driver"
	iboxCredName := os.Getenv(e2e.ENV_IBOX_SECRET)
	if iboxCredName == "" {
		t.Fatalf("%s env var is not set and is required for this test", e2e.ENV_IBOX_SECRET)
	}
	iboxCredNamespace := os.Getenv(e2e.ENV_NAMESPACE)
	if iboxCredNamespace == "" {
		t.Fatalf("%s env var is not set and is required for this test", e2e.ENV_NAMESPACE)
	}
	poolName := os.Getenv(e2e.ENV_POOL)
	if poolName == "" {
		t.Fatalf("%s env var is not set and is required for this test", e2e.ENV_POOL)
	}
	params := map[string]string{
		"replication.storage.openshift.io/replication-secret-name":      iboxCredName,
		"replication.storage.openshift.io/replication-secret-namespace": iboxCredNamespace,
		common.StorageClassPoolName:                                     poolName,
	}
	vgrc, err := e2e.CreateVolumeGroupReplicationClass(addonsClient, t, testConfig, testConfig.TestNames.NSName, provisioner, params)
	if err != nil {
		t.Fatalf("error creating VolumeGroupReplicationClass %s\n", err.Error())
	}
	t.Logf("VolumeGroupReplicationClass created %s", vgrc.Name)

	time.Sleep(time.Second * 5)

	vgrc, err = e2e.GetVolumeGroupReplicationClass(addonsClient, t, testConfig, vgrc.Name)
	if err != nil {
		t.Fatalf("error getting VolumeGroupReplicationClass %s\n", err.Error())
	}
	t.Logf("VolumeGroupReplicationClass read %s", vgrc.Name)

	// test state of VolumeGroupReplication
	if *e2e.CleanUp {
		e2e.TearDown(t.Context(), testConfig)
		// clean up VolumeGroupReplicationClass
		err = e2e.DeleteVolumeGroupReplicationClass(addonsClient, t, vgrc)
		if err != nil {
			t.Fatalf("error deleting VolumeGroupReplicationClass %s\n", err.Error())
		}
	} else {
		t.Log("not cleaning up namespace")
	}
	err = e2e.CleanISCI(t.Context(), *testConfig)
	if err != nil {
		t.Logf("error cleaning ISCSI %s on node %s\n", err.Error(), testConfig.NodeName)
	}

}
