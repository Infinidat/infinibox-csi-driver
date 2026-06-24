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

package iscsicg

import (
	"os"
	"testing"
	"time"

	"github.com/infinidat/infinibox-csi-driver/api/clientgo"
	v1 "github.com/infinidat/infinibox-csi-driver/iboxcg/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/infinidat/infinibox-csi-driver/e2e"
)

func TestIscsiCG(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, common.ProtocolISCSI)
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

	secretName := os.Getenv(e2e.ENV_IBOX_SECRET)
	if secretName == "" {
		t.Fatalf("%s env var is not set and is required for this test", e2e.ENV_IBOX_SECRET)
	}
	secretNamespace := os.Getenv(e2e.ENV_NAMESPACE)
	if secretNamespace == "" {
		t.Fatalf("%s env var is not set and is required for this test", e2e.ENV_NAMESPACE)
	}

	// currently this test assumes you have created a replicated CG already and it
	// is referenced by the env var
	localCGName := os.Getenv(e2e.ENV_IBOXREPLICA_CG)
	if localCGName == "" {
		t.Fatalf("%s env var is not set and is required for this test", e2e.ENV_IBOXREPLICA_CG)
	}

	e2e.Setup(t.Context(), testConfig)

	// get the volume name from the PVC, which will be the PV name
	existingPVC, err := testConfig.ClientSet.CoreV1().PersistentVolumeClaims(testConfig.TestNames.NSName).Get(t.Context(), testConfig.TestNames.PVCName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("error getting existing PVC %s", err.Error())
	}

	t.Logf("existing PVC %s has %s volume", existingPVC.Name, existingPVC.Spec.VolumeName)

	time.Sleep(time.Second * 5)

	cgName := "iboxcg-e2e-test-" + testConfig.TestNames.UniqueSuffix
	t.Logf("preparing an iboxcg %s for CG %s and volume %s", cgName, localCGName, existingPVC.Spec.VolumeName)

	// create the iboxcg CR
	cgCR := v1.Iboxcg{
		ObjectMeta: metav1.ObjectMeta{
			Name: cgName,
			Annotations: map[string]string{
				common.PVCAnnotationSecretName:      secretName,
				common.PVCAnnotationSecretNamespace: secretNamespace,
			},
		},
		Spec: v1.IboxcgSpec{
			Description:     "iscsi-test-volume-cg",
			LocalCGName:     localCGName,
			LocalVolumeName: existingPVC.Spec.VolumeName,
			BaseAction:      "ADD",
		},
	}
	t.Logf("creating iboxcg %s", cgCR.Name)

	kclient, err := clientgo.BuildOffClusterClient(e2e.KubeConfigPath)
	if err != nil {
		t.Fatalf("error getting cluster client %s", err.Error())
	}
	err = kclient.CreateIboxcg(t.Context(), cgCR)
	if err != nil {
		t.Fatalf("error creating iboxcg %s", err.Error())
	}

	// wait for promote status
	t.Logf("sleeping 10s after creating iboxcg %s", cgCR.Name)
	time.Sleep(time.Second * 10)

	runningCG, err := kclient.GetIboxcg(t.Context(), cgName)
	if err != nil {
		t.Fatalf("error getting iboxcg %s", err.Error())
	}
	t.Logf("iboxcg created ID %d State %s", runningCG.Status.ID, runningCG.Status.State)
	if runningCG.Status.State != "completed" {
		t.Fatalf("error iboxcg state is not completed %s", runningCG.Status.State)
	}

	if *e2e.CleanUp {
		e2e.TearDown(t.Context(), testConfig)
		// delete the iboxcg
		err := kclient.DeleteIboxcg(t.Context(), runningCG)
		if err != nil {
			t.Fatalf("error deleting iboxcg %s", err.Error())
		}
	} else {
		t.Log("not cleaning up namespace")
	}
	err = e2e.CleanISCI(t.Context(), *testConfig)
	if err != nil {
		t.Logf("error cleaning ISCSI %s on node %s\n", err.Error(), testConfig.NodeName)
	}

}
