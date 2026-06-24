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

	"github.com/infinidat/infinibox-csi-driver/storage"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/mock"
)

type NodeMock struct {
	mock.Mock
	storage.StorageOperations
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

func (m *NodeMock) NodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	m.Called(ctx, req)
	return &csi.NodeGetVolumeStatsResponse{}, nil
}
