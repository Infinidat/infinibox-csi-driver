/*Copyright 2022 Infinidat
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.*/
package storage

import (
	"context"
	"fmt"
	"os"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog"
)

func (nfs *nfsstorage) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	return &csi.NodeStageVolumeResponse{}, nil
}

func (nfs *nfsstorage) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (nfs *nfsstorage) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	targetPath := req.GetTargetPath() // this is the path on the host node
	// instead of hard-coding, we get he '/host' mount prefix via configuration, this lets us unit test with '/tmp' easier
	containerHostMountPoint := req.PublishContext["csiContainerHostMountPoint"]
	if containerHostMountPoint == "" {
		containerHostMountPoint = DEFAULT_HOST_MOUNT_POINT
	}
	hostTargetPath := containerHostMountPoint + targetPath // this is the path inside the csi container

	klog.V(4).Infof("NodePublishVolume targetPath=%s", hostTargetPath)

	_, err := os.Stat(hostTargetPath)
	if os.IsNotExist(err) {
		klog.V(4).Infof("targetPath %s does not exist, will create", targetPath)
		if err := os.MkdirAll(hostTargetPath, 0750); err != nil {
			klog.Errorf("Error in MkdirAll %s", err.Error())
			return nil, err
		}
	} else {
		klog.V(4).Infof("targetPath %s already exists, will not do anything", targetPath)
		// TODO do I need or care about checking for existing Mount Refs?  k8s.io/utils/GetMountRefs
		return &csi.NodePublishVolumeResponse{}, nil
	}

	mountOptions, err := nfs.nfsHelper.GetNFSMountOptions(req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to get mount options for targetPath '%s': %s", hostTargetPath, err.Error())
	}

	klog.V(4).Infof("nfs mount options are [%v]", mountOptions)

	sourceIP := req.GetVolumeContext()["ipAddress"]
	ep := req.GetVolumeContext()["volPathd"]
	source := fmt.Sprintf("%s:%s", sourceIP, ep)
	klog.V(4).Infof("Mount sourcePath %v, targetPath %v", source, targetPath)
	err = nfs.mounter.Mount(source, targetPath, "nfs", mountOptions)
	if err != nil {
		klog.Errorf("Failed to mount source path '%s' : %s", source, err)
		return nil, status.Errorf(codes.Internal, "Failed to mount target path '%s': %s", targetPath, err)
	}
	klog.V(2).Infof("Successfully mounted nfs volume '%s' to mount point '%s' with options %s", source, targetPath, mountOptions)

	err = nfs.nfsHelper.SetVolumePermissions(req)
	if err != nil {
		msg := fmt.Sprintf("Failed to set volume permissions '%s'", err.Error())
		klog.Errorf(msg)
		return nil, status.Errorf(codes.Internal, msg)
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func (nfs *nfsstorage) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	klog.V(4).Infof("NodeUnpublishVolume")
	targetPath := req.GetTargetPath()
	klog.V(4).Infof("Unmounting path '%s'", targetPath)
	err := unmountAndCleanUp(targetPath)
	if err != nil {
		return nil, err
	}

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (nfs *nfsstorage) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	// should never be called
	return nil, status.Error(codes.Unimplemented, "nfs NodeGetCapabilities not implemented")
}

func (nfs *nfsstorage) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	// should never be called
	return nil, status.Error(codes.Unimplemented, "nfs NodeGetInfo not implemented")
}

func (nfs *nfsstorage) NodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "NodeGetVolumeStats not implemented")
}

func (nfs *nfsstorage) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "NodeExpandVolume not implemented")
}
