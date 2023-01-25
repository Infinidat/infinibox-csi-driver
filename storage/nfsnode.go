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
	"strings"

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

	tmp := strings.Split(req.GetVolumeId(), "$$")[0]
	fileSystemId, err := strconv.ParseInt(tmp, 10, 64)
	if err != nil {
		return nil, err
	}

	if req.GetVolumeContext()[common.SC_NFS_EXPORT_PERMISSIONS] == "" {
		nfs.snapdirVisible = false
		nfs.usePrivilegedPorts = false
		snapDir := req.GetVolumeContext()[common.SC_SNAPDIR_VISIBLE]
		if snapDir != "" {
			nfs.snapdirVisible, err = strconv.ParseBool(snapDir)
			if err != nil {
				klog.Errorf("error %s", err.Error())
				return nil, err
			}
		}
		privPorts := req.GetVolumeContext()[common.SC_PRIV_PORTS]
		if privPorts != "" {
			nfs.usePrivilegedPorts, err = strconv.ParseBool(privPorts)
			if err != nil {
				klog.Errorf("error %s", err.Error())
				return nil, err
			}
		}
		err = nfs.createExportRule(fileSystemId, req.GetVolumeContext()["nodeID"])
		if err != nil {
			return nil, err
		}
	} else {
		klog.V(4).Infof("nfs_export_permissions was specified %s, will not create default export rule", req.GetVolumeContext()[common.SC_NFS_EXPORT_PERMISSIONS])
	}

	_, err = os.Stat(hostTargetPath)
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

	mountOptions, err := nfs.storageHelper.GetNFSMountOptions(req)
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

	svc := Service{}
	if req.GetReadonly() {
		klog.V(2).Info("this is a readonly volume, skipping setting volume permissions")
		return &csi.NodePublishVolumeResponse{}, nil
	}

	err = svc.SetVolumePermissions(req)
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

func (nfs *nfsstorage) createExportRule(filesystemId int64, ipAddress string) (err error) {
	//lookup file system information
	fs, err := nfs.cs.api.GetFileSystemByID(filesystemId)
	if err != nil {
		klog.Errorf("failed to get filesystem by id %d\n", filesystemId)
		return status.Errorf(codes.Internal, "failed to get filesystem by id  %v", err)
	}

	// use the volumeId to get the filesystem information,
	//example rule {'access':'RW','client':'192.168.0.110', 'no_root_squash':true}
	exportFileSystem := api.ExportFileSys{
		FilesystemID:        filesystemId,
		Transport_protocols: "TCP",
		Privileged_port:     nfs.usePrivilegedPorts,
		SnapdirVisible:      nfs.snapdirVisible,
		Export_path:         "/" + fs.Name, // convention is /csi-xxxxxxxx  where xxxx is the filesystem name/pvname
	}
	exportPerms := "[{'access':'RW','client':'" + ipAddress + "','no_root_squash':true}]"
	permissionsMapArray, err := getPermissionMaps(exportPerms)
	if err != nil {
		klog.Errorf("failed to parse permission map string %s", exportPerms)
		return err
	}

	// remove an existing export if it exists, this occurs when a pod restarts
	resp, err := nfs.cs.api.GetExportByFileSystem(filesystemId)
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
				deleteResp, err := nfs.cs.api.DeleteExportPath(r.ID)
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
	exportResp, err := nfs.cs.api.ExportFileSystem(exportFileSystem)
	if err != nil {
		klog.Errorf("failed to create export path of filesystem %s", fs.Name)
		return err
	}
	nfs.exportID = exportResp.ID
	nfs.exportBlock = exportResp.ExportPath
	klog.V(4).Infof("Created nfs export for PV '%s', snapdirVisible: %t", fs.Name, exportFileSystem.SnapdirVisible)

	return nil
}
