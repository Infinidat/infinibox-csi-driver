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
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/infinidat/infinibox-csi-driver/api"
	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/infinidat/infinibox-csi-driver/iboxapi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func DeleteVolume(ctx context.Context, cs Commonservice, volumeID int) (response *csi.DeleteVolumeResponse, err error) {
	vol, err := cs.IboxAPI.GetVolume(ctx, volumeID)
	if err != nil {
		if errors.Is(err, iboxapi.ErrNotFound) {
			slog.Debug("volume already deleted", "volume ID", volumeID)
			return &csi.DeleteVolumeResponse{}, nil
		}
		return nil, common.Errorf("%w", err)
	}

	if vol.LockState == common.LockedState {
		return nil, status.Error(codes.Aborted, fmt.Sprintf("volume ID: %d was locked, can not delete till expire date %s is reached", volumeID, time.UnixMilli(vol.LockExpiresAt)))
	}

	childVolumes, err := cs.IboxAPI.GetVolumesByParentID(ctx, vol.ID)
	if err != nil {
		slog.Error("GetVolumesByParentID", "error", err.Error())
	}
	if len(childVolumes) > 0 {
		metadata := map[string]any{
			ToBeDeleted: true,
		}
		_, err = cs.IboxAPI.PutMetadata(ctx, vol.ID, metadata)
		if err != nil {
			return nil, common.Errorf("failed to update host.k8s.to_be_deleted for volume %s error: %w", vol.Name, err)
		}
		return &csi.DeleteVolumeResponse{}, nil
	}
	slog.Debug("deleting volume", "name", vol.Name, "ID", vol.ID)
	_, err = cs.IboxAPI.DeleteMetadata(ctx, vol.ID)
	if err != nil {
		return nil, common.Errorf("%w", err)
	}
	_, err = cs.IboxAPI.DeleteVolume(ctx, vol.ID)
	if err != nil {
		return nil, common.Errorf("%w", err)
	}
	if vol.ParentID != 0 {
		slog.Debug("checking if parent volume can be", "name", vol.Name, "ID", vol.ID)
		var metadata []iboxapi.GetMetadataResult
		metadata, err = cs.IboxAPI.GetMetadata(ctx, vol.ParentID)
		if err != nil {
			return nil, common.Errorf("%w", err)
		}
		for _, m := range metadata {
			if m.Key == api.TOBEDELETED {
				_, err = DeleteVolume(ctx, cs, vol.ParentID)
				if err != nil {
					return nil, common.Errorf("%w", err)
				}
				return &csi.DeleteVolumeResponse{}, nil
			}
		}
	}
	return &csi.DeleteVolumeResponse{}, nil
}
