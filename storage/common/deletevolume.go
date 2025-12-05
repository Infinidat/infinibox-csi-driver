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
		re, ok := err.(*iboxapi.APIError)
		if ok && re.Code == iboxapi.RESOURCE_NOT_FOUND {
			slog.Debug("volume already deleted", "volume ID", volumeID)
			return &csi.DeleteVolumeResponse{}, nil
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	if vol.LockState == common.LockedState {
		e := fmt.Sprintf("volume ID: %d was locked, can not delete till expire date %s is reached", volumeID, time.UnixMilli(vol.LockExpiresAt))
		slog.Error(e)
		return nil, status.Error(codes.Aborted, e)
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
			e := fmt.Sprintf("failed to update host.k8s.to_be_deleted for volume %s error: %v", vol.Name, err)
			slog.Error(e)
			err = errors.New(e)
			return nil, err
		}
		return &csi.DeleteVolumeResponse{}, nil
	}
	slog.Debug("deleting volume", "name", vol.Name, "ID", vol.ID)
	_, err = cs.IboxAPI.DeleteMetadata(ctx, vol.ID)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	_, err = cs.IboxAPI.DeleteVolume(ctx, vol.ID)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	if vol.ParentID != 0 {
		slog.Debug("checking if parent volume can be", "name", vol.Name, "ID", vol.ID)
		var metadata []iboxapi.GetMetadataResult
		metadata, err = cs.IboxAPI.GetMetadata(ctx, vol.ParentID)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		var toBeDeleted bool
		for _, m := range metadata {
			if m.Key == api.TOBEDELETED {
				toBeDeleted = true
			}
		}
		if toBeDeleted {
			_, err = DeleteVolume(ctx, cs, vol.ParentID)
			if err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
		}
	}
	return &csi.DeleteVolumeResponse{}, nil
}
