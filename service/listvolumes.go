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
package service

import (
	"context"
	"log/slog"

	"github.com/infinidat/infinibox-csi-driver/api/clientgo"
	"github.com/infinidat/infinibox-csi-driver/common"

	"github.com/container-storage-interface/spec/lib/go/csi"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func ListVolumesImplementation(ctx context.Context, client *clientgo.KubeClient) (*csi.ListVolumesResponse, error) {
	slog.Info("Started")

	res := &csi.ListVolumesResponse{
		Entries: make([]*csi.ListVolumesResponse_Entry, 0),
	}

	// Find PVs managed by this CSI driver
	pvList, err := client.GetAllPersistentVolumes(ctx)
	if err != nil {
		e := common.Errorf("GetAllPersistentVolumes - error: %w", err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Unavailable, e.Error())
	}
	slog.Info("pvList", "count", len(pvList.Items))

	for _, persistentVolume := range pvList.Items {
		slog.Info("pv", "capacity", persistentVolume.Spec.Capacity, "name", persistentVolume.GetName(), "pv anno", persistentVolume.GetAnnotations()["pv.kubernetes.io/provisioned-by"])
		if persistentVolume.GetAnnotations()["pv.kubernetes.io/provisioned-by"] == common.ServiceName {
			var status csi.ListVolumesResponse_VolumeStatus
			status.PublishedNodeIds = append(status.PublishedNodeIds, persistentVolume.GetName())
			slog.Info("info", "status", status.String())

			var volume csi.Volume

			volume.CapacityBytes = persistentVolume.Spec.Capacity.Storage().AsDec().UnscaledBig().Int64()
			volume.VolumeId = persistentVolume.GetName()
			volume.VolumeContext = map[string]string{
				common.StorageClassNetworkSpace:    persistentVolume.Spec.CSI.VolumeAttributes[common.StorageClassNetworkSpace],
				common.StorageClassPoolName:        persistentVolume.Spec.CSI.VolumeAttributes[common.StorageClassPoolName],
				common.StorageClassStorageProtocol: persistentVolume.Spec.CSI.VolumeAttributes[common.StorageClassStorageProtocol],
			}
			volume.ContentSource = nil
			volume.AccessibleTopology = nil

			var entry csi.ListVolumesResponse_Entry
			entry.Volume = &volume
			entry.Status = &status
			slog.Info("info", "entry", entry.String())

			res.Entries = append(res.Entries, &entry)
		}
	}

	return res, nil
}
