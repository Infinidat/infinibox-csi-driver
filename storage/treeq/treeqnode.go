/*
Copyright 2022 Infinidat
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
package treeq

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"log/slog"

	"github.com/infinidat/infinibox-csi-driver/common"
	storagecommon "github.com/infinidat/infinibox-csi-driver/storage/common"
	"github.com/infinidat/infinibox-csi-driver/storage/nfs"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const DefaultHostMountPoint = "/host/"

func (treeq *Treeqstorage) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	slog.Debug("start", "iboxInfo", storagecommon.GetHostInfo(ctx, req.GetSecrets(), treeq.NFSstorage.CS.IboxAPI))

	targetPath := req.GetTargetPath() // this is the path on the host node
	containerHostMountPoint := req.PublishContext["csiContainerHostMountPoint"]
	if containerHostMountPoint == "" {
		containerHostMountPoint = DefaultHostMountPoint
	}
	hostTargetPath := containerHostMountPoint + targetPath // this is the path inside the csi container

	slog.Debug("info", "targetPath", hostTargetPath, "volume id", req.GetVolumeId())

	volumeInfo, err := storagecommon.ValidateVolumeID(req.GetVolumeId())
	if err != nil {
		e := fmt.Errorf("from ValidateVolumeID - error parsing volumeID: %s error: %s", req.GetVolumeId(), err.Error())
		slog.Error(e.Error())
		return nil, e
	}
	fileSystemID := volumeInfo.VolumeID
	treeqID := volumeInfo.TreeqID

	slog.Debug("info", "filesystem id", fileSystemID, "treeq id", treeqID, "volume context", req.GetVolumeContext(), "storageclass params", treeq.NFSstorage.StorageClassParameters)

	treeq.NFSstorage.SnapdirVisible = false
	treeq.NFSstorage.UsePrivilegedPorts = false

	snapDirVisible := req.GetVolumeContext()[common.StorageClassSnapDirVisible]
	if snapDirVisible != "" {
		treeq.NFSstorage.SnapdirVisible, err = strconv.ParseBool(snapDirVisible)
		if err != nil {
			e := fmt.Errorf("error parsing snapdir visible - error: %s", err.Error())
			slog.Error(e.Error())
			return nil, e
		}
	}
	privPorts := req.GetVolumeContext()[common.StorageClassPrivPorts]
	if privPorts != "" {
		treeq.NFSstorage.UsePrivilegedPorts, err = strconv.ParseBool(privPorts)
		if err != nil {
			e := fmt.Errorf("error parsing priv ports - error: %s", err.Error())
			slog.Error(e.Error())
			return nil, e
		}
	}

	// only update the export if this is the only treeq since treeq's share a single export
	exports, err := treeq.NFSstorage.CS.IboxAPI.GetExportsByFileSystemID(ctx, fileSystemID)
	if err != nil {
		e := fmt.Errorf("from GetExportByFileSystem - error: %s", err.Error())
		slog.Error(e.Error())
		return nil, e
	}
	slog.Debug("info", "exports", len(exports), "file system id", fileSystemID)

	if len(exports) == 0 {
		exportAccess := "RW"
		if req.GetReadonly() || req.VolumeCapability.GetAccessMode().GetMode() == csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY {
			slog.Debug("detected read-only, setting export to RO")
			exportAccess = "RO"
		}
		exportPerms := fmt.Sprintf("[{'access':'%s','client':'"+req.GetVolumeContext()["nodeID"]+"','no_root_squash':true}]", exportAccess)
		if req.GetVolumeContext()[common.StorageClassNFSExportPermissions] != "" {
			exportPerms = req.GetVolumeContext()[common.StorageClassNFSExportPermissions]
			slog.Debug("will not create default export rule, will create this rule instead", "sc export perms", common.StorageClassNFSExportPermissions, "export persm", exportPerms)
		}
		err = treeq.NFSstorage.UpdateExport(ctx, fileSystemID, exportPerms)
		if err != nil {
			e := fmt.Errorf("from updateExport - error: %s", err.Error())
			slog.Error(e.Error())
			return nil, e
		}
	} else {
		slog.Debug("skipping updateExport because other exports exist")
	}

	_, err = os.Stat(hostTargetPath)
	if os.IsNotExist(err) {
		slog.Debug("targetPath does not exist, will create", "targetpath", targetPath)
		if err := os.MkdirAll(hostTargetPath, 0750); err != nil {
			e := fmt.Errorf("from mkdirAll - error: %s", err.Error())
			slog.Error(e.Error())
			return nil, e
		}
	} else {
		if err != nil {
			slog.Error("host target path exists", "error", err.Error())
		}
		slog.Debug("targetPath already exists, will not do anything", "targetpath", targetPath)
	}

	mountOptions, err := treeq.NFSstorage.StorageHelper.GetNFSMountOptions(req)
	if err != nil {
		e := fmt.Errorf("from GetNFSMountOptions - targetPath: %s error: %s", hostTargetPath, err.Error())
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	sourceIP := req.GetVolumeContext()["ipAddress"]
	dnsName := req.GetVolumeContext()["dnsname"]
	if dnsName != "" {
		sourceIP = dnsName
		slog.Debug("storageclass has dnsname specified, using it for mount instead of ipAddress", "dnsname", dnsName)
	}
	ep := req.GetVolumeContext()["volumePath"]
	source := fmt.Sprintf("%s:%s", sourceIP, ep)

	nfsVersion, nfsPort := nfs.GetNFSVersionPort(mountOptions)
	slog.Debug("info", "version", nfsVersion, "port", nfsPort)

	port, err := strconv.Atoi(nfsPort)
	if err != nil {
		e := fmt.Errorf("port parsing error: %s", err.Error())
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	err = treeq.NFSstorage.StorageHelper.ValidateIPAddress(sourceIP, port)
	if err != nil {
		e := fmt.Errorf("ValidateIPAddress error: %s", err.Error())
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	slog.Debug("mount", "sourcePath", source, "targetpath", targetPath)
	err = treeq.NFSstorage.Mounter.Mount(source, targetPath, "nfs", mountOptions)
	if err != nil {
		e := fmt.Errorf("from mount - failed to mount targetPath: %s sourcePath: %s error: %s", targetPath, source, err.Error())
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	slog.Debug("mounted treeq volume", "sourceIP", source, "volume id", req.GetVolumeId(), "targetPath", targetPath, "mountoptions", mountOptions)

	if req.GetReadonly() || req.VolumeCapability.GetAccessMode().GetMode() == csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY {
		slog.Debug("this is a readonly volume, skipping setting volume permissions")
		return &csi.NodePublishVolumeResponse{}, nil
	}

	err = treeq.NFSstorage.StorageHelper.SetVolumePermissions(req)
	if err != nil {
		e := fmt.Errorf("from SetVolumePermissions - failed to set volume permissions error: %s", err.Error())
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func (treeq *Treeqstorage) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	slog.Debug("starts")
	targetPath := req.GetTargetPath()
	err := storagecommon.UnmountAndCleanUp(targetPath)
	if err != nil {
		e := fmt.Errorf("from UnmountAndCleanup - error: %s", err.Error())
		slog.Error(e.Error())
		return nil, e
	}
	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (treeq *Treeqstorage) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	return &csi.NodeStageVolumeResponse{}, nil
}

func (treeq *Treeqstorage) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	return &csi.NodeUnstageVolumeResponse{}, nil
}
