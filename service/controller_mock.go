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

func (m *ControllerMock) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (deleteResponce *csi.DeleteVolumeResponse, err error) {
	return &csi.DeleteVolumeResponse{}, nil
}

func (m *ControllerMock) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (controlePublishResponce *csi.ControllerPublishVolumeResponse, err error) {
	return &csi.ControllerPublishVolumeResponse{}, nil
}

func (m *ControllerMock) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (controleUnPublishResponce *csi.ControllerUnpublishVolumeResponse, err error) {
	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

func (m *ControllerMock) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (createSnapshot *csi.CreateSnapshotResponse, err error) {
	return &csi.CreateSnapshotResponse{}, nil
}

func (m *ControllerMock) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (deleteSnapshot *csi.DeleteSnapshotResponse, err error) {
	return &csi.DeleteSnapshotResponse{}, nil
}

func (s *ControllerMock) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (expandVolume *csi.ControllerExpandVolumeResponse, err error) {
	return &csi.ControllerExpandVolumeResponse{}, nil
}
