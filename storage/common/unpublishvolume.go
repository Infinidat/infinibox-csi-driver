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

package common

import (
	"context"
	"log/slog"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/infinidat/infinibox-csi-driver/common"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func CommonUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest, cs Commonservice, hostNameSuffix string) (resp *csi.ControllerUnpublishVolumeResponse, err error) {
	slog.Debug("start", "nodeID", req.GetNodeId(), "volumeID", req.GetVolumeId(), "host", cs.VolProto.Host.Name+hostNameSuffix)
	host := cs.VolProto.Host
	slog.Debug("unmapping host's luns", "hostID", host.ID, "host name", host.Name+hostNameSuffix, "luns", len(host.Luns))
	if len(host.Luns) > 0 {
		slog.Debug("unmap volume from host", "volumeID", cs.VolProto.VolumeID, "hostID", host.ID)
		err = cs.UnmapVolumeFromHost(ctx, host.ID, cs.VolProto.VolumeID)
		if err != nil {
			return nil, status.Error(codes.Internal, common.Errorf("from UnmapVolumeFromHost - volumeID: %d hostID: %d - error: %w", cs.VolProto.VolumeID, host.ID, err).Error())
		}
	}

	// avoid a race condition when there is a single LUN that you just unmapped
	if len(host.Luns) == 1 {
		time.Sleep(2 * time.Second)
	}

	luns, err := cs.IboxAPI.GetAllLunByHost(ctx, host.ID)
	if err != nil {
		slog.Error("failed to get LUNs for host", "hostID", host.ID, "error", err)
	}
	if len(luns) == 0 {
		err = HostCleanup(ctx, cs.IboxAPI, host.ID, cs.VolProto.Host.Name+hostNameSuffix)
		if err != nil {
			return nil, status.Error(codes.Internal, common.Errorf("from HostCleanup - hostID: %d - error: %w", host.ID, err).Error())
		}
	}

	slog.Debug("completed", "node id", req.GetNodeId(), "volume id", req.GetVolumeId())
	return &csi.ControllerUnpublishVolumeResponse{}, nil
}
