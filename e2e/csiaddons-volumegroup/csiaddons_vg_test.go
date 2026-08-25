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
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/infinidat/infinibox-csi-driver/e2e"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// for this e2e test, we will use the csi-addons sidecar to run tests against our
// implementation since there is no CRD for VolumeGroup operations (yet?)
//
//
/**
create the Volume Group

kubectl exec -c csiaddons infinidat-csi-driver-driver-0 \
-n infinidat-csi -- csi-addons -operation "CreateVolumeGroup" \
-secret "infinidat-csi/infinibox-creds" -parameters pool_name=csitesting \
-volumegroupname myvolumegoup -endpoint unix:///csi/csi-addons.sock

next, look up the ID of the volume group using the iboxapi or output from the CreateVolumeGroup command

get info about the CG...

kubectl exec -c csiaddons infinidat-csi-driver-driver-0 -n infinidat-csi -- csi-addons \
-operation "ControllerGetVolumeGroup" -secret "infinidat-csi/infinibox-creds" \
-parameters pool_name=csitesting -volumegroupid 7002435 -endpoint unix:///csi/csi-addons.sock

modify the CG members by adding a new member volume

create a volume (1) and another volume (2)

kubectl exec -c csiaddons infinidat-csi-driver-driver-0 -n infinidat-csi -- csi-addons \
-operation "ModifyVolumeGroupMembership" -secret "infinidat-csi/infinibox-creds" \
-parameters pool_name=csitesting -volumegroupid 7002435 -endpoint unix:///csi/csi-addons.sock \
-volumeids "1,2"

verify that CG has added the new volumes

then to delete the  volume group...

kubectl exec -c csiaddons infinidat-csi-driver-driver-0 -n infinidat-csi -- csi-addons \
-operation "DeleteVolumeGroup" -secret "infinidat-csi/infinibox-creds" \
-parameters pool_name=csitesting -volumegroupid 7002435 -endpoint unix:///csi/csi-addons.sock
*/
func TestVolumeGroup(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, common.ProtocolISCSI)
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

	// this will create a test Pod and test Volume we can add to the VolumeGroup
	e2e.Setup(t.Context(), testConfig)

	time.Sleep(time.Second * 5)

	// get parameters for the VolumeGroup we will create

	poolName := os.Getenv(e2e.ENV_POOL)
	if poolName == "" {
		t.Fatalf("%s env var is not set and is required for this test", e2e.ENV_POOL)
	}
	iboxCredName := os.Getenv(e2e.ENV_IBOX_SECRET)
	if iboxCredName == "" {
		t.Fatalf("%s env var is not set and is required for this test", e2e.ENV_IBOX_SECRET)
	}
	iboxCredNamespace := os.Getenv(e2e.ENV_NAMESPACE)
	if iboxCredNamespace == "" {
		t.Fatalf("%s env var is not set and is required for this test", e2e.ENV_NAMESPACE)
	}

	secret := iboxCredNamespace + "/" + iboxCredName
	t.Logf("secret is %s", secret)

	operation := "CreateVolumeGroup"
	volumegroupname := testConfig.TestNames.NSName
	t.Logf("VolumeGroup name %s", volumegroupname)
	params := "pool_name=" + poolName

	args := e2e.AddonsArgs{
		Secret:          secret,
		Params:          params,
		VolumeGroupID:   "",
		Command:         operation,
		VolumeGroupName: volumegroupname,
		VolumeIds:       "",
	}
	output, err := e2e.AddonsCommand(t.Context(), testConfig, args)
	if err != nil {
		t.Fatalf("error creating volume group %s", err.Error())
	}
	t.Logf("create volume group command output %s", output)

	// get the volume created in the Setup
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

	testCG, err := testConfig.ClientService.IboxAPI.GetConsistencyGroupByName(t.Context(), volumegroupname)
	if err != nil {
		t.Fatalf("could not get CG for name %s - error %s", volumegroupname, err.Error())
	}

	testVolume, err := testConfig.ClientService.IboxAPI.GetVolume(t.Context(), volumeID)
	if err != nil {
		t.Fatalf("could not get volume for volumeID %d - error %s", volumeID, err.Error())
	}

	// for more than 1 volume to add we would have the volume IDs comma separated in a string
	volumeIds := strconv.Itoa(testVolume.ID)

	/**
	kubectl exec -c csiaddons infinidat-csi-driver-driver-0 -n infinidat-csi -- csi-addons \
	-operation "ModifyVolumeGroupMembership" -secret "infinidat-csi/infinibox-creds" \
	-parameters pool_name=csitesting -volumegroupid 7002435 -endpoint unix:///csi/csi-addons.sock \
	-volumeids "1,2"
	*/

	args = e2e.AddonsArgs{
		Secret:          secret,
		Params:          params,
		VolumeGroupID:   strconv.Itoa(testCG.ID),
		Command:         "ModifyVolumeGroupMembership",
		VolumeGroupName: "",
		VolumeIds:       volumeIds,
	}

	output, err = e2e.AddonsCommand(t.Context(), testConfig, args)
	if err != nil {
		t.Fatalf("error modifying volume group %s", err.Error())
	}
	t.Logf("modified volume group command output %s", output)

	/**
	kubectl exec -c csiaddons infinidat-csi-driver-driver-0 -n infinidat-csi -- csi-addons \
	-operation "ControllerGetVolumeGroup" -secret "infinidat-csi/infinibox-creds" \
	-volumegroupid 7002435 -endpoint unix:///csi/csi-addons.sock
	*/
	args = e2e.AddonsArgs{
		Secret:          secret,
		Params:          "",
		VolumeGroupID:   strconv.Itoa(testCG.ID),
		Command:         "ControllerGetVolumeGroup",
		VolumeGroupName: "",
		VolumeIds:       "",
	}
	output, err = e2e.AddonsCommand(t.Context(), testConfig, args)
	if err != nil {
		t.Fatalf("error getting volume group %s", err.Error())
	}
	t.Logf("controller get volume group command output %s", output)

	// remove the volume we added earlier to the CG
	args = e2e.AddonsArgs{
		Secret:          secret,
		Params:          params,
		VolumeGroupID:   strconv.Itoa(testCG.ID),
		Command:         "ModifyVolumeGroupMembership",
		VolumeGroupName: "",
		VolumeIds:       "",
	}

	output, err = e2e.AddonsCommand(t.Context(), testConfig, args)
	if err != nil {
		t.Fatalf("error modifying volume group (deleting member) %s", err.Error())
	}
	t.Logf("modified volume group  (deleting member) command output %s", output)

	time.Sleep(5 * time.Second)
	/**
	kubectl exec -c csiaddons infinidat-csi-driver-driver-0 -n infinidat-csi -- csi-addons \
	-operation "DeleteVolumeGroup" -secret "infinidat-csi/infinibox-creds" \
	-volumegroupid 7002435 -endpoint unix:///csi/csi-addons.sock
	*/
	args = e2e.AddonsArgs{
		Secret:          secret,
		Params:          "",
		VolumeGroupID:   strconv.Itoa(testCG.ID),
		Command:         "DeleteVolumeGroup",
		VolumeGroupName: "",
		VolumeIds:       "",
	}
	output, err = e2e.AddonsCommand(t.Context(), testConfig, args)
	if err != nil {
		t.Fatalf("error deleting volume group %s", err.Error())
	}
	t.Logf("delete volume group command output %s", output)

	if *e2e.CleanUp {
		e2e.TearDown(t.Context(), testConfig)
	} else {
		t.Log("not cleaning up namespace")
	}
	/**
	err = e2e.CleanISCI(t.Context(), *testConfig)
	if err != nil {
		t.Logf("error cleaning ISCSI %s on node %s\n", err.Error(), testConfig.NodeName)
	}
	*/

}
