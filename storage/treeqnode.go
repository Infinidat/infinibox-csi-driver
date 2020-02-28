package storage

import (
	"context"
	"fmt"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (treeq *treeqstorage) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	log.Debug("treeq NodePublishVolume")
	targetPath := req.GetTargetPath()
	notMnt, err := treeq.mounter.IsLikelyNotMountPoint(targetPath)
	if err != nil {
		if treeq.osHelper.IsNotExist(err) {
			if err := treeq.osHelper.MkdirAll(targetPath, 0750); err != nil {
				log.Errorf("Error while mkdir %v", err)
				return nil, err
			}
			notMnt = true
		} else {
			log.Errorf("IsLikelyNotMountPint method error  %v", err)
			return nil, err
		}
	}
	if !notMnt {
		return &csi.NodePublishVolumeResponse{}, nil
	}
	mountOptions := []string{}
	configMountOptions := req.GetVolumeContext()["nfs_mount_options"]
	if configMountOptions == "" {
		configMountOptions = MountOptions
	}
	for _, option := range strings.Split(configMountOptions, ",") {
		if option != "" {
			mountOptions = append(mountOptions, option)
		}
	}
	if req.GetReadonly() {
		mountOptions = append(mountOptions, "ro")
	}
	sourceIP := req.GetVolumeContext()["ipAddress"]
	ep := req.GetVolumeContext()["volumePath"]
	source := fmt.Sprintf("%s:%s", sourceIP, ep)
	log.Debugf("Mount sourcePath %v, tagetPath %v", source, targetPath)
	err = treeq.mounter.Mount(source, targetPath, "nfs", mountOptions)
	if err != nil {
		log.Errorf("fail to mount source path '%s' : %s", source, err)
		return nil, status.Errorf(codes.Internal, "Failed to mount target path '%s': %s", targetPath, err)
	}
	log.Debugf("pod successfully mounted to volumeID %s", req.GetVolumeId())
	return &csi.NodePublishVolumeResponse{}, nil
}
func (treeq *treeqstorage) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	log.Debug("treeq NodeUnpublishVolume")
	targetPath := req.GetTargetPath()
	notMnt, err := treeq.mounter.IsLikelyNotMountPoint(targetPath)
	if err != nil {
		if treeq.osHelper.IsNotExist(err) {
			log.Warnf("mount point '%s' already doesn't exist: '%s', return OK", targetPath, err)
			return &csi.NodeUnpublishVolumeResponse{}, nil
		}
		return nil, err
	}
	if notMnt {
		if err := treeq.osHelper.Remove(targetPath); err != nil {
			log.Errorf("Remove target path error: %s", err.Error())
		}
		return &csi.NodeUnpublishVolumeResponse{}, nil
	}
	if err := treeq.mounter.Unmount(targetPath); err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to unmount target path '%s': %s", targetPath, err)
	}
	if err := treeq.osHelper.Remove(targetPath); err != nil && !treeq.osHelper.IsNotExist(err) {
		return nil, status.Errorf(codes.Internal, "Cannot remove unmounted target path '%s': %s", targetPath, err)
	}
	log.Debugf("pod successfully unmounted from volumeID %s", req.GetVolumeId())
	return &csi.NodeUnpublishVolumeResponse{}, nil
}
