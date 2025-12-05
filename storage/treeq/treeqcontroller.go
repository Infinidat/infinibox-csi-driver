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
	"log/slog"
	"strconv"
	"strings"

	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/infinidat/infinibox-csi-driver/helper"
	storagecommon "github.com/infinidat/infinibox-csi-driver/storage/common"
	"github.com/infinidat/infinibox-csi-driver/storage/nfs"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/mount-utils"
)

type Treeqstorage struct {
	csi.ControllerServer
	csi.NodeServer
	TreeqService Interface
	NFSstorage   nfs.NFSstorage
}

func NewTreeqstorage(capacity int64, comnserv storagecommon.Commonservice) (treeq *Treeqstorage) {
	nfs := nfs.NFSstorage{
		Capacity:               capacity,
		StorageClassParameters: make(map[string]string),
		CS:                     comnserv,
		StorageHelper:          storagecommon.StorageService{},
		OSHelper:               helper.Service{},
		Mounter:                mount.NewWithoutSystemd(""),
	}
	service := &Service{
		NFSstorage: nfs,
		CS:         comnserv,
	}
	treeq = &Treeqstorage{
		NFSstorage:   nfs,
		TreeqService: service,
	}
	return treeq
}

func (treeq *Treeqstorage) ValidateStorageClass(params map[string]string) error {
	requiredParams := map[string]string{
		common.StorageClassNetworkSpace: `\A.*\z`,    // TODO: could make this enforce IBOX network_space requirements, but probably not necessary
		common.StorageClassPoolName:     `[a-zA-Z]+`, // match all strings except empty string or blank string
	}
	optionalParams := map[string]string{
		common.StorageClassUID:               `^\d+$`,
		common.StorageClassGID:               `^\d+$`,
		common.StorageClassMaxFilesystems:    `^\d+$`,
		common.StorageClassMaxTreeqsPerFS:    `^\d+$`,
		common.StorageClassMaxFilesystemSize: `\A.*\z`, // TODO: add more specific pattern
	}

	err := storagecommon.ValidateRequiredOptionalSCParameters(requiredParams, optionalParams, params)
	if err != nil {
		e := fmt.Errorf("error: %s", err.Error())
		slog.Error(e.Error())
		return status.Error(codes.InvalidArgument, e.Error())
	}

	err = nfs.ValidateNFSExportPermissions(params)
	if err != nil {
		e := fmt.Errorf("error: %s", err.Error())
		slog.Error(e.Error())
		return status.Error(codes.InvalidArgument, e.Error())
	}

	return nil
}

func (treeq *Treeqstorage) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (csiResp *csi.CreateVolumeResponse, err error) {
	slog.Debug("start", "name", req.GetName(), "params", req.GetParameters(),
		"iboxInfo", storagecommon.GetHostInfo(ctx, req.GetSecrets(), treeq.NFSstorage.CS.IboxAPI))

	params := req.GetParameters()

	for _, cap := range req.GetVolumeCapabilities() {
		if block := cap.GetBlock(); block != nil {
			e := fmt.Errorf("from GetBlock - block access requested for %s PV: %s", params[common.StorageClassStorageProtocol], req.GetName())
			slog.Error(e.Error())
			return nil, status.Error(codes.InvalidArgument, e.Error())
		}
	}

	treeq.NFSstorage.StorageClassParameters = params
	fsPrefix := params[common.StorageClassFSPrefix]
	if fsPrefix == "" {
		fsPrefix = common.StorageClassFSPrefixDefault
	}
	treeqVolumeContext, err := treeq.TreeqService.IsTreeqAlreadyExist(ctx, params[common.StorageClassPoolName], strings.Trim(params[common.StorageClassNetworkSpace], ""), req.GetName(), fsPrefix)
	if err != nil {
		e := fmt.Errorf("from IsTreeqAlreadyExist - error: %s", err.Error())
		slog.Error(e.Error())
		return nil, e
	}
	if len(treeqVolumeContext) == 0 {
		treeqVolumeContext, err = treeq.TreeqService.CreateTreeqVolume(ctx, params, treeq.NFSstorage.Capacity, req.GetName())
		if err != nil {
			e := fmt.Errorf("from CreateTreeqVolume - error: %s", err.Error())
			slog.Error(e.Error())
			return nil, e
		}
	}

	treeqVolumeContext[common.StorageClassNFSExportPermissions] = params[common.StorageClassNFSExportPermissions]
	treeqVolumeContext[common.StorageClassStorageProtocol] = params[common.StorageClassStorageProtocol]
	treeqVolumeContext[common.StorageClassUID] = params[common.StorageClassUID]
	treeqVolumeContext[common.StorageClassGID] = params[common.StorageClassGID]

	volumeID := treeqVolumeContext["ID"] + "#" + treeqVolumeContext["TREEQID"]
	slog.Debug("final", "context", treeqVolumeContext, "volume id", volumeID)
	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      volumeID,
			CapacityBytes: treeq.NFSstorage.Capacity,
			VolumeContext: treeqVolumeContext,
			ContentSource: req.GetVolumeContentSource(),
		},
	}, nil
}

// TODO duplicated code needs to be removed
func getVolumeIDs(volumeID string) (filesystemID, treeqID int, err error) {
	volproto := strings.Split(volumeID, "#")
	if len(volproto) != 2 {
		e := fmt.Errorf("volumeID: %s and other details not found", volumeID)
		slog.Error(e.Error())
		return 0, 0, e
	}
	if filesystemID, err = strconv.Atoi(volproto[0]); err != nil {
		e := fmt.Errorf("error parsing filesystem ID  error: %s", err.Error())
		slog.Error(e.Error())
		return 0, 0, e
	}

	// volumeID example := "94148131#20000$$nfs_treeq"
	treeqdetails := strings.Split(volproto[1], "$")

	if treeqID, err = strconv.Atoi(treeqdetails[0]); err != nil {
		e := fmt.Errorf("error parsing treeq ID error: %s", err.Error())
		slog.Error(e.Error())
		return 0, 0, e
	}

	return filesystemID, treeqID, nil
}

func (treeq *Treeqstorage) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	slog.Debug("start", "volume id", req.GetVolumeId())

	filesystemID := treeq.NFSstorage.CS.VolProto.VolumeID
	treeqID := treeq.NFSstorage.CS.VolProto.TreeqID
	nfsDeleteErr := treeq.TreeqService.DeleteTreeqVolume(ctx, filesystemID, treeqID)
	if nfsDeleteErr != nil {
		slog.Error(nfsDeleteErr.Error())
		if strings.Contains(nfsDeleteErr.Error(), "FILESYSTEM_NOT_FOUND") {
			slog.Error("already deleted from ibox")
			return &csi.DeleteVolumeResponse{}, nil
		}
		return nil, nfsDeleteErr
	}
	slog.Debug("filesystem treeq successfully deleted", "filesystem id", filesystemID, "treeq id", treeqID)
	return &csi.DeleteVolumeResponse{}, nil
}

func (treeq *Treeqstorage) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	return &csi.ControllerPublishVolumeResponse{}, nil
}

func (treeq *Treeqstorage) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	volproto := treeq.NFSstorage.CS.VolProto
	slog.Debug("start", "volproto", volproto)
	err := treeq.NFSstorage.CS.API.DeleteExportRule(ctx, volproto.VolumeID, volproto.NodeID)
	if err != nil {
		e := fmt.Errorf("from DeleteExportRule - failed to delete Export Rule fileystemID: %d error: %s", volproto.VolumeID, err.Error())
		slog.Error(e.Error())
		return nil, status.Error(codes.Internal, e.Error())
	}
	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

func (treeq *Treeqstorage) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	return nil, status.Error(codes.Unimplemented, "Unsupported operation for treeq")
}

func (treeq *Treeqstorage) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	return nil, status.Error(codes.Unimplemented, "Unsupported operation for treeq")
}

func (treeq *Treeqstorage) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (expandVolume *csi.ControllerExpandVolumeResponse, err error) {
	slog.Debug("starts")

	maxFileSystemSize := treeq.NFSstorage.StorageClassParameters[common.StorageClassMaxFilesystemSize]
	filesystemID, treeqID, err := getVolumeIDs(req.GetVolumeId())
	if err != nil {
		e := fmt.Errorf("from getVolumeIDs - volumeID: %s error: %s", req.GetVolumeId(), err.Error())
		slog.Error(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	capacity := req.GetCapacityRange().GetRequiredBytes()
	if capacity < storagecommon.GIB {
		capacity = storagecommon.GIB
		slog.Warn("volume minimum capacity should be greater 1 GB")
	}

	slog.Debug("info", "file system id", filesystemID, "treeq id", treeqID, "capacity", capacity, "max filesystem size", maxFileSystemSize)
	err = treeq.TreeqService.UpdateTreeqVolume(ctx, filesystemID, treeqID, capacity, maxFileSystemSize)
	if err != nil {
		e := fmt.Errorf("from UpdateTreeqVolume - error: %s", err.Error())
		slog.Error(e.Error())
		return nil, e
	}
	return &csi.ControllerExpandVolumeResponse{
		CapacityBytes:         capacity,
		NodeExpansionRequired: false,
	}, nil
}
