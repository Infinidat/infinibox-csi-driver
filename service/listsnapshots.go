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
	"strconv"
	"time"

	"github.com/infinidat/infinibox-csi-driver/api"
	"github.com/infinidat/infinibox-csi-driver/api/clientgo"
	"github.com/infinidat/infinibox-csi-driver/common"
	storagecommon "github.com/infinidat/infinibox-csi-driver/storage/common"

	"github.com/container-storage-interface/spec/lib/go/csi"
	tspb "google.golang.org/protobuf/types/known/timestamppb"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func ListSnapshotsImplementation(ctx context.Context, req *csi.ListSnapshotsRequest, client *clientgo.KubeClient, namespace string) (*csi.ListSnapshotsResponse, error) {
	slog.Info("Started", "MaxEntries", req.MaxEntries)

	res := &csi.ListSnapshotsResponse{
		Entries: make([]*csi.ListSnapshotsResponse_Entry, 0),
	}

	secrets, err := client.GetSecrets(ctx, namespace)
	if err != nil {
		e := common.Errorf("GetSecrets - error: %w", err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Unavailable, e.Error())
	}

	for _, secret := range secrets {
		slog.Info("evaluating secret", "hostname", secret["hostname"])
		client := api.ClientService{
			ConfigMap:  make(map[string]string),
			SecretsMap: secret,
		}

		slog.Info("getting connection to ibox", "hostname", secret["hostname"])
		clientService, err := client.NewClient()
		if err != nil {
			e := common.Errorf("NewClient - error: %w", err)
			slog.Error(e.Error())
			return nil, status.Error(codes.Unavailable, e.Error())
		}
		slog.Info("got connection to ibox", "hostname", secret["hostname"])

		snapshots, err := clientService.IboxAPI.GetAllSnapshots(ctx)
		if err != nil {
			e := common.Errorf("GetAllSnapshots - error: %w", err)
			slog.Error(e.Error())
			return nil, status.Error(codes.Unavailable, e.Error())
		}
		slog.Info("got back", "snapshots", len(snapshots))

		// handle the optional case where a SnapshotId is passed in the ListSnapshots request
		var volumeInfo api.VolumeProtocolConfig
		var volumeID int
		if req.SnapshotId != "" {
			volumeInfo, err = storagecommon.ValidateVolumeID(req.SnapshotId)
			if err != nil {
				e := common.Errorf("ValidateVolumeID - error: %w", err)
				slog.Error(e.Error())
				return nil, status.Error(codes.InvalidArgument, e.Error())
			}
			volumeID = volumeInfo.VolumeID
		}

		for _, snapshot := range snapshots {
			createdAtValue := snapshot.CreatedAt / 1000
			timeValue := time.Unix(createdAtValue, 0)
			timestampValue := tspb.New(timeValue)

			var parentName string
			slog.Log(ctx, common.LevelTrace, "info", "snapshot datasettype", snapshot.DatasetType)
			switch snapshot.DatasetType {
			case "VOLUME":
				_, err := clientService.IboxAPI.GetVolume(ctx, snapshot.ParentID)
				if err != nil {
					e := common.Errorf("GetVolume - snapshot error name %s parentID %d error %w", snapshot.Name, snapshot.ParentID, err)
					slog.Error(e.Error())
					parentName = UNKNOWN
				} else {
					parentName = strconv.Itoa(snapshot.ParentID)
				}
			case "FILESYSTEM":
				_, err := clientService.IboxAPI.GetFileSystemByID(ctx, snapshot.ParentID)
				if err != nil {
					e := common.Errorf("GetFileSystemByID - snapshot error name %s parentid %d error %w", snapshot.Name, snapshot.ParentID, err)
					slog.Error(e.Error())
					parentName = UNKNOWN
				} else {
					parentName = strconv.Itoa(snapshot.ParentID)
				}
			default:
				slog.Error("snapshot unknown dataset type", "name", snapshot.Name, "datasettype", snapshot.DatasetType, "parentid", snapshot.ParentID)
				parentName = UNKNOWN
			}

			entry := csi.ListSnapshotsResponse_Entry{
				Snapshot: &csi.Snapshot{
					SnapshotId:     snapshot.Name,
					SourceVolumeId: parentName,
					SizeBytes:      snapshot.Size,
					CreationTime:   timestampValue,
					ReadyToUse:     true, // always true on the ibox according to Jason.
				},
			}

			slog.Log(ctx, common.LevelTrace, "info", "source volume id", req.SourceVolumeId, "snapshot id", req.SnapshotId)

			if req.SourceVolumeId != "" {
				volumeInfo, err := storagecommon.ValidateVolumeID(req.SourceVolumeId)
				if err != nil {
					e := common.Errorf("ValidateVolumeID - error validating sourceVolumeId %s %w", req.SourceVolumeId, err)
					slog.Error(e.Error())
					return nil, status.Error(codes.InvalidArgument, e.Error())
				}
				slog.Log(ctx, common.LevelTrace, "comparing", "volume id", volumeInfo.VolumeID, "source volume id", entry.Snapshot.SourceVolumeId, "snapshot", entry.Snapshot)
				if strconv.Itoa(volumeInfo.VolumeID) == entry.Snapshot.SourceVolumeId {
					slog.Log(ctx, common.LevelTrace, "matches!")
					entry.Snapshot.SourceVolumeId = req.SourceVolumeId // set the SourceVolumeId sent back to the incoming format xxxx$$nfs
					res.Entries = append(res.Entries, &entry)
				}
			} else if req.SnapshotId != "" {
				slog.Log(ctx, common.LevelTrace, "comparing", "volumeid", volumeID, "snapshot id", snapshot.ID)
				if volumeID == snapshot.ID {
					slog.Log(ctx, common.LevelTrace, "req.SnapshotID contains found matching snapshot with ID and name", "snapshot id", req.SnapshotId, "snapshot id 2", snapshot.ID, "name", snapshot.Name)
					entry.Snapshot.SnapshotId = req.SnapshotId
					res.Entries = append(res.Entries, &entry)
				}
			} else {
				res.Entries = append(res.Entries, &entry)
			}
		}
	}

	return res, nil
}
