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
	"infinibox-csi-driver/api"
	"infinibox-csi-driver/common"
	"os"
	"strconv"

	"k8s.io/klog"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const DEFAULT_HOST_MOUNT_POINT = "/host/"

func (treeq *treeqstorage) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	klog.V(2).Info("treeq NodePublishVolume")

	targetPath := req.GetTargetPath() // this is the path on the host node
	containerHostMountPoint := req.PublishContext["csiContainerHostMountPoint"]
	if containerHostMountPoint == "" {
		containerHostMountPoint = DEFAULT_HOST_MOUNT_POINT
	}
	hostTargetPath := containerHostMountPoint + targetPath // this is the path inside the csi container

	klog.V(4).Infof("NodePublishVolume with targetPath %s volumeId %s\n", hostTargetPath, req.GetVolumeId())

	fileSystemId, treeqId, err := getVolumeIDs(req.GetVolumeId())
	if err != nil {
		klog.V(4).Infof("error parsing fileSystemId %s from %s", err.Error(), req.GetVolumeId())
		return nil, err
	}
	klog.V(4).Infof("fileSystemId %d treeqId %d\n", fileSystemId, treeqId)
	klog.V(4).Infof("volumeContext=%+v", req.GetVolumeContext())
	klog.V(4).Infof("treeq.configmap=%+v", treeq.configmap)

	if req.GetVolumeContext()[common.SC_NFS_EXPORT_PERMISSIONS] == "" {
		treeq.snapdirVisible = false
		treeq.usePrivilegedPorts = false

		snapDirVisible := req.GetVolumeContext()[common.SC_SNAPDIR_VISIBLE]
		if snapDirVisible != "" {
			treeq.snapdirVisible, err = strconv.ParseBool(snapDirVisible)
			if err != nil {
				klog.Errorf("error %s", err.Error())
				return nil, err
			}
		}
		privPorts := req.GetVolumeContext()[common.SC_PRIV_PORTS]
		if privPorts != "" {
			treeq.usePrivilegedPorts, err = strconv.ParseBool(privPorts)
			if err != nil {
				klog.Errorf("error %s", err.Error())
				return nil, err
			}
		}
		err = treeq.createExportRule(fileSystemId, req.GetVolumeContext()["nodeID"])
		if err != nil {
			return nil, err
		}
	} else {
		klog.V(4).Infof("%s was specified %s, will not create default export rule", common.SC_NFS_EXPORT_PERMISSIONS, req.GetVolumeContext()[common.SC_NFS_EXPORT_PERMISSIONS])
	}

	_, err = os.Stat(hostTargetPath)
	if os.IsNotExist(err) {
		klog.V(4).Infof("targetPath %s does not exist, will create", targetPath)
		if err := os.MkdirAll(hostTargetPath, 0750); err != nil {
			klog.Errorf("error in MkdirAll %s", err.Error())
			return nil, err
		}
	} else {
		klog.V(4).Infof("targetPath %s already exists, will not do anything", targetPath)
		// TODO do I need or care about checking for existing Mount Refs?  k8s.io/utils/GetMountRefs
		return &csi.NodePublishVolumeResponse{}, nil
	}

	mountOptions, err := treeq.storageHelper.GetNFSMountOptions(req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Failed to get mount options for targetPath '%s': %s", hostTargetPath, err.Error())
	}

	sourceIP := req.GetVolumeContext()["ipAddress"]
	ep := req.GetVolumeContext()["volumePath"]
	source := fmt.Sprintf("%s:%s", sourceIP, ep)
	klog.V(4).Infof("Mount sourcePath %v, targetPath %v", source, targetPath)
	err = treeq.mounter.Mount(source, targetPath, "nfs", mountOptions)
	if err != nil {
		klog.Errorf("failed to mount source path '%s' : %s", source, err)
		return nil, status.Errorf(codes.Internal, "Failed to mount target path '%s': %s", targetPath, err)
	}
	klog.V(2).Infof("mounted treeq volume: '%s' volumeID: %s to mount point: '%s' with options %s", source, req.GetVolumeId(), targetPath, mountOptions)

	if req.GetReadonly() {
		klog.V(2).Info("this is a readonly volume, skipping setting volume permissions")
		return &csi.NodePublishVolumeResponse{}, nil
	}

	svc := Service{}
	err = svc.SetVolumePermissions(req)
	if err != nil {
		msg := fmt.Sprintf("Failed to set volume permissions '%s'", err.Error())
		klog.Errorf(msg)
		return nil, status.Errorf(codes.Internal, msg)
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func (treeq *treeqstorage) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	klog.V(2).Info("treeq NodeUnpublishVolume")
	targetPath := req.GetTargetPath()
	notMnt, err := treeq.mounter.IsLikelyNotMountPoint(targetPath)
	if err != nil {
		if treeq.osHelper.IsNotExist(err) {
			klog.V(2).Infof("mount point '%s' already doesn't exist: '%s', return OK", targetPath, err)
			return &csi.NodeUnpublishVolumeResponse{}, nil
		}
		return nil, err
	}
	if notMnt {
		if err := treeq.mounter.Unmount(targetPath); err != nil {
			return nil, status.Errorf(codes.Internal, "Failed to unmount target path '%s': %s", targetPath, err)
		}
	}
	if err := treeq.osHelper.Remove(targetPath); err != nil && !treeq.osHelper.IsNotExist(err) {
		return nil, status.Errorf(codes.Internal, "Cannot remove unmounted target path '%s': %s", targetPath, err)
	}
	klog.V(2).Infof("pod successfully unmounted from volumeID %s", req.GetVolumeId())
	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (treeq *treeqstorage) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	return &csi.NodeStageVolumeResponse{}, nil
}

func (treeq *treeqstorage) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (treeq *treeqstorage) createExportRule(filesystemId int64, ipAddress string) (err error) {
	klog.V(4).Infof("createExportRule filesystemId=%d, ipAddress: %s", filesystemId, ipAddress)
	//lookup file system information
	fs, err := treeq.cs.api.GetFileSystemByID(filesystemId)
	if err != nil {
		klog.Errorf("failed to get filesystem by id %d\n", filesystemId)
		return status.Errorf(codes.Internal, "failed to get filesystem by id  %v", err)
	}

	// use the volumeId to get the filesystem information,
	//example rule {'access':'RW','client':'192.168.0.110', 'no_root_squash':true}
	exportFileSystem := api.ExportFileSys{
		FilesystemID:        filesystemId,
		Transport_protocols: "TCP",
		Privileged_port:     treeq.usePrivilegedPorts,
		SnapdirVisible:      treeq.snapdirVisible,
		Export_path:         "/" + fs.Name, // convention is /csi-xxxxxxxx  where xxxx is the filesystem name/pvname
	}
	exportPerms := "[{'access':'RW','client':'" + ipAddress + "','no_root_squash':true}]"
	permissionsMapArray, err := getPermissionMaps(exportPerms)
	if err != nil {
		klog.Errorf("failed to parse permission map string %s", exportPerms)
		return err
	}

	// remove an existing export if it exists, this occurs when a pod restarts
	resp, err := treeq.cs.api.GetExportByFileSystem(filesystemId)
	if err != nil {
		klog.Errorf("error from GetExportByFileSystem filesystemId %d %s", filesystemId, err.Error())
		return err
	}
	klog.V(4).Infof("GetExportByFileSystem response =%+v", resp)
	if resp != nil {
		responses := *resp
		for i := 0; i < len(responses); i++ {
			r := responses[i]
			if r.ExportPath == exportFileSystem.Export_path {
				klog.V(4).Infof("export path was found to already exist %s", r.ExportPath)
				// here is where we would delete the existing export
				deleteResp, err := treeq.cs.api.DeleteExportPath(r.ID)
				if err != nil {
					klog.Errorf("error from DeleteExportPath ID %d filesystemId %d", r.ID, filesystemId)
					return err
				}
				klog.V(4).Infof("delete export path response %+v\n", deleteResp)
			}
		}
	}

	// create the export rule
	exportFileSystem.Permissionsput = append(exportFileSystem.Permissionsput, permissionsMapArray...)
	klog.V(4).Infof("exportFileSystem =%+v", exportFileSystem)
	exportResp, err := treeq.cs.api.ExportFileSystem(exportFileSystem)
	if err != nil {
		klog.Errorf("failed to create export path of filesystem %s", fs.Name)
		return err
	}
	treeq.exportID = exportResp.ID
	treeq.exportBlock = exportResp.ExportPath
	klog.V(4).Infof("Created nfs treeq export for PV '%s', snapdirVisible: %t", fs.Name, exportFileSystem.SnapdirVisible)

	return nil
}
