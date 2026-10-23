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
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/infinidat/infinibox-csi-driver/iboxapi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	RestoreTypeVolume   = "Volume"
	RestoreTypeSnapshot = "Snapshot"
)

// validateSnapshotLockingParameter validates an input lock_expires parameter string and returns
// the computed expire time in Unix Milliseconds or an error if the validation fails
func ValidateSnapshotLockingParameter(nowTime int64, input string) (timeInUnixMilli int64, err error) {
	parts := strings.Split(input, " ")
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid format of lock_expires_at parameter, should only have 2 values (int string)")
	}

	// we except the 1st part of the parameter to be an integer
	count, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("invalid format of lock_expires_at count, should be in the format of an integer")
	}

	if count < 1 {
		return 0, fmt.Errorf("invalid lock_expires_at count, should be greater than 0")
	}

	var futureTime int64

	// input will look like '1 Hours', '1 Days', '1 Weeks', '1 Months', '1 Years'
	// this function converts an input value into a numerical value representing
	// a date in the future from the current time

	const millisPerHour = 3600000
	const millisPerDay = 24 * millisPerHour
	const millisPerWeek = 7 * millisPerDay
	const millisPerMonth = 4 * millisPerWeek
	const millisPerYear = 12 * millisPerMonth

	switch parts[1] {
	case "Hours":
		futureTime = nowTime + int64((count * millisPerHour))
	case "Days":
		futureTime = nowTime + int64((count * millisPerDay))
	case "Weeks":
		futureTime = nowTime + int64((count * millisPerWeek))
	case "Months":
		futureTime = nowTime + int64((count * millisPerMonth))
	case "Years":
		futureTime = nowTime + int64((count * millisPerYear))
	default:
		return 0, fmt.Errorf("invalid format of lock_expires_at frequency, should be either Days, Hours, Weeks, Months, Years")
	}

	return futureTime, nil
}

func CreateVolumeFromVolumeContent(ctx context.Context, cs Commonservice, req *csi.CreateVolumeRequest, volumeName string, requestedSize int64, storagePool string) (*csi.CreateVolumeResponse, error) {
	var err error
	var volumeContentID string
	var resizeRequired bool
	restoreType := RestoreTypeSnapshot

	if req.GetVolumeContentSource().GetSnapshot() != nil {
		volumeContentID = req.GetVolumeContentSource().GetSnapshot().GetSnapshotId()
	} else if req.GetVolumeContentSource().GetVolume() != nil {
		volumeContentID = req.GetVolumeContentSource().GetVolume().GetVolumeId()
		restoreType = RestoreTypeVolume
	}
	slog.Debug("info", "volume content id", volumeContentID, "restore type", restoreType, "size", requestedSize)

	volumeInfo, err := ValidateVolumeID(volumeContentID)
	if err != nil {
		e := fmt.Sprintf("error from ValidateVolumeID - restoreType: %s volumeContentID: %s, error: %s", restoreType, volumeContentID, err.Error())
		slog.Error(e)
		return nil, status.Error(codes.NotFound, e)
	}

	sourceVolume, err := cs.IboxAPI.GetVolume(ctx, volumeInfo.VolumeID)
	if err != nil {
		e := fmt.Sprintf("error from GetVolume - restoreType: %s volumeID: %d error: %s", restoreType, volumeInfo.VolumeID, err.Error())
		slog.Error(e)
		return nil, status.Error(codes.NotFound, e)
	}

	if sourceVolume.Size > requestedSize {
		slog.Error("source volume size is greater than requested size, which is not supported", "volume", volumeInfo.VolumeID, "restore type", restoreType, "source volume size", sourceVolume.Size, "requested size", requestedSize)
		return nil, status.Error(codes.Internal, "invalid restore sizes")
	}
	if sourceVolume.Size < requestedSize {
		slog.Debug("info", "resizeRequired - source volume size is less than requested size", sourceVolume.Size, "sizeInBytes", requestedSize)
		resizeRequired = true
	}

	// Validate the storagePool is the same.
	pool, err := cs.IboxAPI.GetPoolByName(ctx, storagePool)
	if err != nil {
		e := fmt.Sprintf("GetPoolByName - storagePool: %s  error: %s", storagePool, err.Error())
		slog.Error(e)
		return nil, status.Error(codes.Internal, e)
	}
	if pool.ID != sourceVolume.PoolID {
		e := fmt.Sprintf("volume storage pool is different than requested storagePool: %s", storagePool)
		slog.Error(e)
		return nil, status.Error(codes.InvalidArgument, e)
	}
	ssd := req.GetParameters()[common.StorageClassSSDEnabled]
	if ssd == "" {
		ssd = strconv.FormatBool(false)
	}
	ssdEnabled, _ := strconv.ParseBool(ssd)

	// for clones or restores we create a Snapshot to base the restored volume or clone volume on
	createSnapshotRequest := iboxapi.CreateSnapshotVolumeRequest{
		ParentID:       volumeInfo.VolumeID,
		SnapshotName:   volumeName,
		WriteProtected: false,
		SSDEnabled:     ssdEnabled,
		LockExpiresAt:  0,
	}

	createSnapshotResponse, err := cs.IboxAPI.CreateSnapshotVolume(ctx, createSnapshotRequest)
	if err != nil {
		e := fmt.Sprintf("CreateSnapshotVolume - error: %s", err.Error())
		slog.Error(e)
		return nil, status.Error(codes.Internal, e)
	}

	// Retrieve created destination volume
	newSnapshotVolumeID := createSnapshotResponse.SnapShotID
	newSnapshotVolume, err := cs.IboxAPI.GetVolume(ctx, newSnapshotVolumeID)
	if err != nil {
		e := fmt.Sprintf("GetVolume - volumeID: %d error: %s", newSnapshotVolumeID, err.Error())
		slog.Error(e)
		return nil, status.Error(codes.Internal, e)
	}

	// ibox 7.x doesn't support snapshot promote so we check for ibox feature support for it
	snapshotPromoteEnabled, err := featureEnabled(ctx, cs.IboxAPI, common.IboxFeaturePromoteSnapshot)
	if err != nil {
		e := fmt.Errorf("featureEnabled - error: %s", err.Error())
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	slog.Debug("snapshot promote feature", "enabled", snapshotPromoteEnabled)
	if snapshotPromoteEnabled {
		// promote the newly created snapshot to a MASTER volume
		_, err = cs.IboxAPI.PromoteSnapshot(ctx, newSnapshotVolume.ID)
		if err != nil {
			e := fmt.Errorf("from PromoteSnapshot - error: %s", err.Error())
			slog.Error(e.Error())
			return nil, status.Error(codes.Internal, e.Error())
		}
		slog.Debug("snapshot promoted to volume", "volume id", newSnapshotVolume.ID)

		// resize the new volume if the requested size is larger than the new volume
		if resizeRequired {
			err = cs.IboxAPI.UpdateVolumeSize(ctx, newSnapshotVolume.ID, requestedSize)
			if err != nil {
				e := fmt.Sprintf("UpdateVolumeSize - volumeID: %d error: %s", newSnapshotVolumeID, err.Error())
				slog.Error(e)
				return nil, status.Error(codes.Internal, e)
			}
			newSnapshotVolume.Size = requestedSize
			slog.Info("updated volume size", "size", requestedSize)
		}
	} else {
		slog.Debug("snapshot promoted not enabled on this ibox, so not promoting volume", "volume id", newSnapshotVolume.ID)
	}

	// Create a volume response and return it
	csiVolume := cs.GetCSIResponse(ctx, newSnapshotVolume, req)
	CopyRequestParameters(req.GetParameters(), csiVolume.VolumeContext)
	slog.Info("size of response", "size", csiVolume.CapacityBytes)

	metadata := map[string]any{
		"host.k8s.pvname": newSnapshotVolume.Name,
		ResizeRequired:    resizeRequired,
	}
	_, err = cs.IboxAPI.PutMetadata(ctx, newSnapshotVolume.ID, metadata)
	if err != nil {
		e := fmt.Sprintf("PutMetadata - volumeName: %s, error: %s", newSnapshotVolume.Name, err.Error())
		slog.Error(e)
		return nil, status.Error(codes.Internal, e)
	}
	slog.Debug("completes", "Volume (from snap)", csiVolume.VolumeContext["Name"], "volumeID", csiVolume.VolumeId, "storage pool", csiVolume.VolumeContext["StoragePoolName"])
	return &csi.CreateVolumeResponse{Volume: csiVolume}, nil
}

func featureEnabled(ctx context.Context, cl iboxapi.Client, requestedFeature string) (bool, error) {
	features, err := cl.GetFeatures(ctx)
	if err != nil {
		return false, err
	}
	for _, feature := range features {
		if feature.Name == requestedFeature {
			return feature.Enabled, nil
		}
	}
	slog.Warn("feature not found in Ibox API results, assuming unsupported", "feature", requestedFeature)
	return false, nil
}
