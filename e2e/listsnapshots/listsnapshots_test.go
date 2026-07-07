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

package listsnapshots

import (
	"context"
	"testing"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/infinidat/infinibox-csi-driver/api/clientgo"
	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/infinidat/infinibox-csi-driver/e2e"
	"github.com/infinidat/infinibox-csi-driver/service"
)

func TestIscsiListSnapshots(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, common.ProtocolISCSI)
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

	testConfig.UseSnapshot = true

	e2e.Setup(t.Context(), testConfig)

	time.Sleep(time.Second * 5)

	err = e2e.CreateSnapshot(t.Context(), testConfig.TestNames.PVCName, testConfig.TestNames.VSCName, testConfig.TestNames.NSName, testConfig.SnapshotClient)
	if err != nil {
		t.Fatalf("error creating volumesnapshot pod %s", err.Error())
	}
	time.Sleep(time.Second * 5)
	err = e2e.WaitForSnapshot(t, e2e.SNAPSHOT_NAME, testConfig.TestNames.NSName, testConfig.SnapshotClient)
	if err != nil {
		t.Fatalf("error waiting for volumesnapshot %s", err.Error())
	}

	cl, err := clientgo.BuildOffClusterClient(e2e.KubeConfigPath)
	if err != nil {
		t.Fatalf("error getting kube client %s\n", err.Error())
	}

	// call ListVolumes and make sure a volume exists
	req := &csi.ListSnapshotsRequest{}

	//resp, err := service.ListSnapshotsImplementation(context.Background(), req, cl, e2e.E2E_NAMESPACE)
	resp, err := service.ListSnapshotsImplementation(context.Background(), req, cl, "infinidat-csi")
	if err != nil {
		t.Fatalf("error calling ListSnapshotsImpl %s\n", err.Error())
	}
	if len(resp.Entries) < 1 {
		t.Fatal("error expected at least 1 volume from ListSnapshots")
	}
	t.Logf("got volumes %d from ListSnapshots call\n", len(resp.Entries))

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
