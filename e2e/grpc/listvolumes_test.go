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

package grpc

import (
	pb "github.com/container-storage-interface/spec/lib/go/csi"

	"testing"
)

func TestListVolumes(t *testing.T) {
	cl, err := SetupControllerClient()
	if err != nil {
		t.Error(err)
	}

	var resp *pb.ListVolumesResponse
	resp, err = cl.ListVolumes(t.Context(), &pb.ListVolumesRequest{})
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
	resp, err = cl.ListSnapshots(t.Context(), &pb.ListSnapshotsRequest{})
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
	resp, err = cl.ControllerGetCapabilities(t.Context(), &pb.ControllerGetCapabilitiesRequest{})
	if err != nil {
		t.Error(err)
	}
	t.Logf("capabilities %d", len(resp.Capabilities))

}
