//go:build e2e

package iscsichap

import (
	"testing"

	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/infinidat/infinibox-csi-driver/e2e"
)

func TestIscsi(t *testing.T) {

	testConfig, err := e2e.GetTestConfig(t, common.ProtocolISCSI)
	if err != nil {
		t.Fatalf("error getting TestConfig %s\n", err.Error())
	}

	e2e.Setup(testConfig)

	if *e2e.CleanUp {
		e2e.TearDown(testConfig)
	} else {
		t.Log("not cleaning up namespace")
	}

	err = e2e.CleanISCI(*testConfig)
	if err != nil {
		t.Logf("error cleaning ISCSI %s on node %s\n", err.Error(), testConfig.NodeName)
	}

}
