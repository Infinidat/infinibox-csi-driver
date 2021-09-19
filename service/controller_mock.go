package service

import (
	"context"
	"infinibox-csi-driver/storage"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/mock"
)

type ControllerMock struct {
	mock.Mock
	storage.Storageoperations
}

func (m *ControllerMock) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	return &csi.CreateVolumeResponse{Volume: &csi.Volume{VolumeId: "100"}}, nil
}

func (m *ControllerMock) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	return &csi.DeleteVolumeResponse{}, nil
}

func (m *ControllerMock) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	return &csi.ControllerPublishVolumeResponse{}, nil
}

func (m *ControllerMock) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

func (m *ControllerMock) ValidateVolumeCapabilities(context.Context, *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	return &csi.ValidateVolumeCapabilitiesResponse{}, nil
}

func (m *ControllerMock) ListVolumes(context.Context, *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	return &csi.ListVolumesResponse{}, nil
}

func (m *ControllerMock) GetCapacity(context.Context, *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error) {
	return &csi.GetCapacityResponse{}, nil
}

func (m *ControllerMock) ControllerGetCapabilities(context.Context, *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	return &csi.ControllerGetCapabilitiesResponse{}, nil
}

func (m *ControllerMock) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	return &csi.CreateSnapshotResponse{}, nil
}

func (m *ControllerMock) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	return &csi.DeleteSnapshotResponse{}, nil
}

func (s *ControllerMock) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	return &csi.ControllerExpandVolumeResponse{}, nil
}

func (s *ControllerMock) ListSnapshots(context.Context, *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	return &csi.ListSnapshotsResponse{}, nil
}

func (s *ControllerMock) ControllerGetVolume(context.Context, *csi.ControllerGetVolumeRequest) (*csi.ControllerGetVolumeResponse, error) {
	return &csi.ControllerGetVolumeResponse{}, nil
}
