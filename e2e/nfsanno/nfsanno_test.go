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

package nfsanno

import (
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/infinidat/infinibox-csi-driver/e2e"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	PROTOCOL = "nfs"
)

func TestNfs(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, PROTOCOL)
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

	iboxSecret := os.Getenv(e2e.ENV_IBOX_SECRET)
	if iboxSecret == "" {
		t.Fatalf("error - %s env var is required for this test", e2e.ENV_IBOX_SECRET)
	}
	networkSpace := os.Getenv(e2e.ENV_NAS_NETWORK_SPACE)
	if networkSpace == "" {
		t.Fatalf("error - %s env var is required for this test", e2e.ENV_NAS_NETWORK_SPACE)
	}
	poolName := os.Getenv(e2e.ENV_POOL)
	if poolName == "" {
		t.Fatalf("error - %s env var is required for this test", e2e.ENV_POOL)
	}

	pvcAnnotations := &e2e.PVCAnnotations{
		IboxNetworkSpace: networkSpace,
		IboxPool:         poolName,
		IboxSecret:       iboxSecret,
	}
	testConfig.PVCAnnotations = pvcAnnotations

	e2e.Setup(t.Context(), testConfig)

	if *e2e.CleanUp {
		e2e.TearDown(t.Context(), testConfig)
	} else {
		t.Log("not cleaning up namespace")
	}
}

func TestNfsMetadataAnno(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, PROTOCOL)
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

	pvcAnnotations := &e2e.PVCAnnotations{
		IboxNetworkSpace: "",
		IboxPool:         "",
		IboxSecret:       "",
		IboxMetadata:     "user-defined-stuff",
	}
	testConfig.PVCAnnotations = pvcAnnotations

	e2e.Setup(t.Context(), testConfig)

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

	metadata, err := testConfig.ClientService.IboxAPI.GetMetadata(testConfig.Testt.Context(), volumeID)
	if err != nil {
		t.Fatalf("error getting metadata for volumeID - error %s", err.Error())
	}

	t.Logf("metadata for the test volume %d is %v len=%d\n", volumeID, metadata, len(metadata))

	if len(metadata) != 3 {
		t.Fatalf("error expected 3 metadata values for volumeID - got %v", metadata)
	}

	if *e2e.CleanUp {
		e2e.TearDown(t.Context(), testConfig)
	} else {
		t.Log("not cleaning up namespace")
	}
}
