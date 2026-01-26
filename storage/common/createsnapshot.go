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
	"strconv"

	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/infinidat/infinibox-csi-driver/iboxapi"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func CommonCreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest, cs Commonservice) (resp *csi.CreateSnapshotResponse, err error) {
	var snapshotID string
	snapshotName := req.GetName()
	slog.Debug("start", "name", snapshotName, "source volume ID", req.GetSourceVolumeId(), "volproto", cs.VolProto)

	volumeSnapshot, err := cs.IboxAPI.GetVolumeByName(ctx, snapshotName)
	if err != nil {
		if errors.Is(err, iboxapi.ErrNotFound) {
			slog.Debug("snapshot with given name not found", "name", snapshotName)
		} else {
			e := fmt.Sprintf("error from GetVolumeByName - snapshotName: %s error: %s", snapshotName, err.Error())
			slog.Error(e)
			return nil, status.Error(codes.Internal, e)
		}
	} else if volumeSnapshot.ParentID == cs.VolProto.VolumeID {
		snapshotID = strconv.Itoa(volumeSnapshot.ID) + "$$" + cs.VolProto.StorageType
		return &csi.CreateSnapshotResponse{
			Snapshot: &csi.Snapshot{
				SizeBytes:      volumeSnapshot.Size,
				SnapshotId:     snapshotID,
				SourceVolumeId: req.GetSourceVolumeId(),
				CreationTime:   timestamppb.Now(),
				ReadyToUse:     true,
			},
		}, nil
	} else {
		return nil, status.Error(codes.AlreadyExists, fmt.Sprintf("snapshot Name: %s with already existing name and different source volume ID", volumeSnapshot.Name))
	}

	// look up the parent volume so we can get the ssd_enabled value and use that for
	// the snapshot being created next
	parentVolume, err := cs.IboxAPI.GetVolume(ctx, cs.VolProto.VolumeID)
	if err != nil {
		e := fmt.Sprintf("error from GetVolume - getting parent volume when creating snapshot - volumeID: %d, error: %s", cs.VolProto.VolumeID, err.Error())
		slog.Error(e)
		return nil, status.Error(codes.NotFound, e)
	}

	snapshotParam := iboxapi.CreateSnapshotVolumeRequest{
		ParentID:       cs.VolProto.VolumeID,
		SnapshotName:   snapshotName,
		WriteProtected: true,
		SSDEnabled:     parentVolume.SsdEnabled,
	}

	lockExpiresAtParameter := req.Parameters[common.LockExpiresAtParameter]
	var lockExpiresAt int64
	if lockExpiresAtParameter != "" {
		ntpStatus, err := cs.IboxAPI.GetNtpStatus(ctx)
		if err != nil {
			e := fmt.Sprintf("error from GetNtpStatus - error: %s", err.Error())
			slog.Error(e)
			return nil, status.Error(codes.Internal, e)
		}
		lockExpiresAt, err = ValidateSnapshotLockingParameter(ntpStatus[0].LastProbeTimestamp, lockExpiresAtParameter)
		if err != nil {
			e := fmt.Sprintf("failed to create snapshotName: %s error: %s, invalid lock_expires_at parameter ", snapshotName, err.Error())
			slog.Error(e)
			return nil, status.Error(codes.Internal, e)
		}
		slog.Info("snapshot", "name", snapshotName, "snapshot param has a lock_expires_at", lockExpiresAtParameter)
	}

	snapshotParam.LockExpiresAt = lockExpiresAt
	snapshot, err := cs.IboxAPI.CreateSnapshotVolume(ctx, snapshotParam)
	if err != nil {
		e := fmt.Sprintf("error from CreateSnapshotVolume  snapshotName: %s error: %s", snapshotName, err.Error())
		slog.Error(e)
		return nil, status.Error(codes.Internal, e)
	}

	snapshotID = strconv.Itoa(snapshot.SnapShotID) + "$$" + cs.VolProto.StorageType
	csiSnapshot := &csi.Snapshot{
		SnapshotId:     snapshotID,
		SourceVolumeId: req.GetSourceVolumeId(),
		ReadyToUse:     true,
		CreationTime:   timestamppb.Now(),
		SizeBytes:      snapshot.Size,
	}
	slog.Debug("completes", "response", csiSnapshot)
	snapshotResp := &csi.CreateSnapshotResponse{Snapshot: csiSnapshot}
	return snapshotResp, nil
}
