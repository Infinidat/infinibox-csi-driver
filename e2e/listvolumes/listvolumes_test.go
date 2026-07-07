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

package listvolumes

import (
	"context"
	"testing"

	"github.com/infinidat/infinibox-csi-driver/api/clientgo"
	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/infinidat/infinibox-csi-driver/e2e"
	"github.com/infinidat/infinibox-csi-driver/service"
)

func TestIscsiListVolumes(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, common.ProtocolISCSI)
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

	e2e.Setup(t.Context(), testConfig)

	cl, err := clientgo.BuildOffClusterClient(e2e.KubeConfigPath)
	if err != nil {
		t.Fatalf("error getting kube client %s\n", err.Error())
	}

	// call ListVolumes and make sure a volume exists
	resp, err := service.ListVolumesImplementation(context.Background(), cl)
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}
	if len(resp.Entries) < 1 {
		t.Fatal("error expected at least 1 volume from ListVolumes")
	}
	t.Logf("got volumes %d from ListVolumes call\n", len(resp.Entries))

	if *e2e.CleanUp {
		e2e.TearDown(t.Context(), testConfig)
	} else {
		t.Log("not cleaning up namespace")
	}
	err = e2e.CleanISCI(t.Context(), *testConfig)
	if err != nil {
		t.Logf("error cleaning ISCSI %s on node %s\n", err.Error(), testConfig.NodeName)
	}

}
