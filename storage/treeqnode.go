package storage

import (
	"context"

	"github.com/container-storage-interface/spec/lib/go/csi"
)

func (treeq *treeqstorage) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {

	return &csi.NodePublishVolumeResponse{}, nil
}
func (treeq *treeqstorage) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {

	return &csi.NodeUnpublishVolumeResponse{}, nil

}
