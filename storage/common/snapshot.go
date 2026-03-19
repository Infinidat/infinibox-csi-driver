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

func CreateVolumeFromVolumeContent(ctx context.Context, cs Commonservice, req *csi.CreateVolumeRequest, name string, sizeInBytes int64, storagePool string) (*csi.CreateVolumeResponse, error) {
	var err error

	volumecontent := req.GetVolumeContentSource()
	var volumeContentID string
	var restoreType string
	if volumecontent.GetSnapshot() != nil {
		restoreType = RestoreTypeSnapshot
		volumeContentID = volumecontent.GetSnapshot().GetSnapshotId()
	} else if volumecontent.GetVolume() != nil {
		volumeContentID = volumecontent.GetVolume().GetVolumeId()
		restoreType = RestoreTypeVolume
	}
	slog.Debug("info", "volume content id", volumeContentID, "restore type", restoreType, "size", sizeInBytes)

	// Validate the source content id
	volproto, err := ValidateVolumeID(volumeContentID)
	if err != nil {
		e := fmt.Sprintf("error from ValidateVolumeID - restoreType: %s volumeContentID: %s, error: %s", restoreType, volumeContentID, err.Error())
		slog.Error(e)
		return nil, status.Error(codes.NotFound, e)
	}

	srcVol, err := cs.IboxAPI.GetVolume(ctx, volproto.VolumeID)
	if err != nil {
		e := fmt.Sprintf("error from GetVolume - restoreType: %s volumeID: %d error: %s", restoreType, volproto.VolumeID, err.Error())
		slog.Error(e)
		return nil, status.Error(codes.NotFound, e)
	}

	// Validate the size is the same.
	if srcVol.Size != sizeInBytes {
		return nil, status.Errorf(codes.InvalidArgument,
			restoreType+" %s has incompatible size %d bytes with requested %d bytes",
			volumeContentID, srcVol.Size, sizeInBytes)
	}

	// Validate the storagePool is the same.
	pool, err := cs.IboxAPI.GetPoolByName(ctx, storagePool)
	if err != nil {
		e := fmt.Sprintf("error from GetPoolByName - storagePool: %s  error: %s", storagePool, err.Error())
		slog.Error(e)
		return nil, status.Error(codes.Internal, e)
	}
	if pool.ID != srcVol.PoolID {
		e := fmt.Sprintf("volume storage pool is different than requested storagePool: %s", storagePool)
		slog.Error(e)
		return nil, status.Error(codes.InvalidArgument, e)
	}
	ssd := req.GetParameters()[common.StorageClassSSDEnabled]
	if ssd == "" {
		ssd = strconv.FormatBool(false)
	}
	ssdEnabled, _ := strconv.ParseBool(ssd)
	snapshotParam := iboxapi.CreateSnapshotVolumeRequest{
		ParentID:       volproto.VolumeID,
		SnapshotName:   name,
		WriteProtected: false,
		SSDEnabled:     ssdEnabled,
		LockExpiresAt:  0,
	}
	// Create snapshot
	snapResponse, err := cs.IboxAPI.CreateSnapshotVolume(ctx, snapshotParam)
	if err != nil {
		e := fmt.Sprintf("error from CreateSnapshotVolume - error: %s", err.Error())
		slog.Error(e)
		return nil, status.Error(codes.Internal, e)
	}

	// Retrieve created destination volume
	volID := snapResponse.SnapShotID
	dstVol, err := cs.IboxAPI.GetVolume(ctx, volID)
	if err != nil {
		e := fmt.Sprintf("error from GetVolume - volumeID: %d error: %s", volID, err.Error())
		slog.Error(e)
		return nil, status.Error(codes.Internal, e)
	}

	// ibox 7.x doesn't support snapshot promote so we check for ibox feature support for it
	enabled, err := featureEnabled(ctx, cs.IboxAPI, common.IboxFeaturePromoteSnapshot)
	if err != nil {
		e := fmt.Errorf("from featureEnabled - error: %s", err.Error())
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	slog.Debug("snapshot promote feature", "enabled", enabled)
	if enabled {
		// promote the snapshot created just now to a MASTER volume
		_, err = cs.IboxAPI.PromoteSnapshot(ctx, dstVol.ID)
		if err != nil {
			e := fmt.Errorf("from PromoteSnapshot - error: %s", err.Error())
			slog.Error(e.Error())
			return nil, status.Error(codes.Internal, e.Error())
		}
		slog.Debug("snapshot promoted to volume", "volume id", dstVol.ID)
	} else {
		slog.Debug("snapshot promoted not enabled on this ibox, so not promoting volume", "volume id", dstVol.ID)
	}

	// Create a volume response and return it
	csiVolume := cs.GetCSIResponse(ctx, dstVol, req)
	CopyRequestParameters(req.GetParameters(), csiVolume.VolumeContext)

	metadata := map[string]any{
		"host.k8s.pvname": dstVol.Name,
	}
	_, err = cs.IboxAPI.PutMetadata(ctx, dstVol.ID, metadata)
	if err != nil {
		e := fmt.Sprintf("error from PutMetadata - volumeName: %s, error: %s", dstVol.Name, err.Error())
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
