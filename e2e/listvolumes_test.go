//go:build e2e

package e2e

import (
	"context"

	pb "github.com/container-storage-interface/spec/lib/go/csi"

	"testing"
)

func TestListVolumes(t *testing.T) {
	cl, err := SetupControllerClient()
	if err != nil {
		t.Error(err)
	}

	var resp *pb.ListVolumesResponse
	resp, err = cl.ListVolumes(context.Background(), &pb.ListVolumesRequest{})
	if err != nil {
		t.Error(err)
	}
	t.Logf("volumes %d", len(resp.Entries))

}
func TestListSnapshots(t *testing.T) {
	cl, err := SetupControllerClient()
	if err != nil {
		t.Error(err)
	}

	var resp *pb.ListSnapshotsResponse
	resp, err = cl.ListSnapshots(context.Background(), &pb.ListSnapshotsRequest{})
	if err != nil {
		t.Error(err)
	}
	t.Logf("snapshots %d", len(resp.Entries))

}

func TestGetCapabilities(t *testing.T) {
	cl, err := SetupControllerClient()
	if err != nil {
		t.Error(err)
	}

	var resp *pb.ControllerGetCapabilitiesResponse
	resp, err = cl.ControllerGetCapabilities(context.Background(), &pb.ControllerGetCapabilitiesRequest{})
	if err != nil {
		t.Error(err)
	}
	t.Logf("capabilities %d", len(resp.Capabilities))

}
