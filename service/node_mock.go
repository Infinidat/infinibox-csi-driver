package service

import (
	"context"
	"infinibox-csi-driver/storage"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/mock"
)

type NodeMock struct {
	mock.Mock
	storage.Storageoperations
}

func (m *NodeMock) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	m.Called(ctx, req)
	return &csi.NodePublishVolumeResponse{}, nil
}

func (m *NodeMock) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	m.Called(ctx, req)
	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (m *NodeMock) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	m.Called(ctx, req)
	return &csi.NodeStageVolumeResponse{}, nil
}
