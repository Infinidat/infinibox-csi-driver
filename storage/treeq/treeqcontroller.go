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
	"infinibox-csi-driver/common"
	"infinibox-csi-driver/helper"
	"infinibox-csi-driver/log"
	storagecommon "infinibox-csi-driver/storage/common"
	"infinibox-csi-driver/storage/nfs"
	"strconv"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/mount-utils"
)

type Treeqstorage struct {
	csi.ControllerServer
	csi.NodeServer
	TreeqService TreeqInterface
	NFSstorage   nfs.NFSstorage
}

var zlog = log.Get() // grab the logger for package use

func NewTreeqstorage(capacity int64, comnserv storagecommon.Commonservice) (treeq *Treeqstorage) {
	nfs := nfs.NFSstorage{
		Capacity:               capacity,
		StorageClassParameters: make(map[string]string),
		CS:                     comnserv,
		StorageHelper:          storagecommon.StorageService{},
		OSHelper:               helper.Service{},
		Mounter:                mount.NewWithoutSystemd(""),
	}
	service := &TreeqService{
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
		common.SC_NETWORK_SPACE: `\A.*\z`,    // TODO: could make this enforce IBOX network_space requirements, but probably not necessary
		common.SC_POOL_NAME:     `[a-zA-Z]+`, //match all strings except empty string or blank string
	}
	optionalParams := map[string]string{
		common.SC_UID:                       `^\d+$`,
		common.SC_GID:                       `^\d+$`,
		common.SC_MAX_FILESYSTEMS:           `^\d+$`,
		common.SC_MAX_TREEQS_PER_FILESYSTEM: `^\d+$`,
		common.SC_MAX_FILESYSTEM_SIZE:       `\A.*\z`, // TODO: add more specific pattern
	}

	err := storagecommon.ValidateRequiredOptionalSCParameters(requiredParams, optionalParams, params)
	if err != nil {
		e := fmt.Errorf("ValidateStorageClass (treeq) - %s", err.Error())
		zlog.Error().Msg(e.Error())
		return status.Error(codes.InvalidArgument, e.Error())
	}

	err = nfs.ValidateNFSExportPermissions(params)
	if err != nil {
		e := fmt.Errorf("ValidateStorageClass (treeq) - %s", err.Error())
		zlog.Error().Msg(e.Error())
		return status.Error(codes.InvalidArgument, e.Error())
	}

	return nil
}

func (treeq *Treeqstorage) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (csiResp *csi.CreateVolumeResponse, err error) {
	zlog.Debug().Msgf("CreateVolume (treeq) - called pvName %s parameters %v - %s", req.GetName(), req.GetParameters(),
		storagecommon.GetHostInfo(req.GetSecrets(), treeq.NFSstorage.CS.IboxApi))

	params := req.GetParameters()

	for _, cap := range req.GetVolumeCapabilities() {
		if block := cap.GetBlock(); block != nil {
			e := fmt.Errorf("CreateVolume (treeq) - GetBlock - block access requested for %s PV %s", params[common.SC_STORAGE_PROTOCOL], req.GetName())
			zlog.Err(e)
			return nil, status.Error(codes.InvalidArgument, e.Error())
		}
	}

	treeq.NFSstorage.StorageClassParameters = params
	fsPrefix := params[common.SC_FS_PREFIX]
	if fsPrefix == "" {
		fsPrefix = common.SC_FS_PREFIX_DEFAULT
	}
	treeqVolumeContext, err := treeq.TreeqService.IsTreeqAlreadyExist(params[common.SC_POOL_NAME], strings.Trim(params[common.SC_NETWORK_SPACE], ""), req.GetName(), fsPrefix)
	if err != nil {
		e := fmt.Errorf("CreateVolume (treeq) - IsTreeqAlreadyExist - error: %s", err.Error())
		zlog.Error().Msg(e.Error())
		return nil, e
	}
	if len(treeqVolumeContext) == 0 {
		treeqVolumeContext, err = treeq.TreeqService.CreateTreeqVolume(params, treeq.NFSstorage.Capacity, req.GetName())
		if err != nil {
			e := fmt.Errorf("CreateVolume (treeq) - CreateTreeqVolume - error: %s", err.Error())
			zlog.Error().Msg(e.Error())
			return nil, e
		}
	}

	treeqVolumeContext[common.SC_NFS_EXPORT_PERMISSIONS] = params[common.SC_NFS_EXPORT_PERMISSIONS]
	treeqVolumeContext[common.SC_STORAGE_PROTOCOL] = params[common.SC_STORAGE_PROTOCOL]
	treeqVolumeContext[common.SC_UID] = params[common.SC_UID]
	treeqVolumeContext[common.SC_GID] = params[common.SC_GID]

	volumeID := treeqVolumeContext["ID"] + "#" + treeqVolumeContext["TREEQID"]
	zlog.Debug().Msgf("CreateVolume (treeq) -  final treeqVolumeMap %v volumeID %s", treeqVolumeContext, volumeID)
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
		e := fmt.Errorf("volume Id %s and other details not found", volumeID)
		zlog.Error().Msg(e.Error())
		return 0, 0, e
	}
	if filesystemID, err = strconv.Atoi(volproto[0]); err != nil {
		e := fmt.Errorf("error parsing filesystem ID %s", err.Error())
		zlog.Err(e)
		return 0, 0, e
	}

	// volumeID example := "94148131#20000$$nfs_treeq"
	treeqdetails := strings.Split(volproto[1], "$")

	if treeqID, err = strconv.Atoi(treeqdetails[0]); err != nil {
		e := fmt.Errorf("error parsing treeq ID %s", err.Error())
		zlog.Err(e)
		return 0, 0, e
	}

	return filesystemID, treeqID, nil
}

func (treeq *Treeqstorage) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	zlog.Debug().Msgf("DeleteVolume (treeq) - volume ID %s", req.GetVolumeId())

	filesystemID := treeq.NFSstorage.CS.VolProto.VolumeID
	treeqID := treeq.NFSstorage.CS.VolProto.TreeqID
	nfsDeleteErr := treeq.TreeqService.DeleteTreeqVolume(filesystemID, treeqID)
	if nfsDeleteErr != nil {
		zlog.Err(nfsDeleteErr)
		if strings.Contains(nfsDeleteErr.Error(), "FILESYSTEM_NOT_FOUND") {
			zlog.Error().Msg("DeleteVolume (treeq) - already deleted from ibox")
			return &csi.DeleteVolumeResponse{}, nil
		}
		return nil, nfsDeleteErr
	}
	zlog.Debug().Msgf("DeleteVolume (treeq) - filesystem ID %d treeq ID %d successfully deleted", filesystemID, treeqID)
	return &csi.DeleteVolumeResponse{}, nil
}

func (treeq *Treeqstorage) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	return &csi.ControllerPublishVolumeResponse{}, nil
}

func (treeq *Treeqstorage) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	volproto := treeq.NFSstorage.CS.VolProto
	zlog.Debug().Msgf("ControllerUnpublishVolume (treeq) - volproto %+v fileId %d nodeId %s", volproto, volproto.VolumeID, volproto.NodeID)
	err := treeq.NFSstorage.CS.Api.DeleteExportRule(volproto.VolumeID, volproto.NodeID)
	if err != nil {
		e := fmt.Errorf("ControllerUnpublishVolume (treeq) - DeleteExportRule - failed to delete Export Rule fileystemID %d error %v", volproto.VolumeID, err)
		zlog.Error().Msg(e.Error())
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
	zlog.Debug().Msgf("ControllerExpandVolume (treeq) starts")

	maxFileSystemSize := treeq.NFSstorage.StorageClassParameters[common.SC_MAX_FILESYSTEM_SIZE]
	filesystemID, treeqID, err := getVolumeIDs(req.GetVolumeId())
	if err != nil {
		e := fmt.Errorf("ControllerExpandVolume (treeq) - getVolumeIDs - invalid volume id %v", err)
		zlog.Err(e)
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	capacity := int64(req.GetCapacityRange().GetRequiredBytes())
	if capacity < storagecommon.GIB {
		capacity = storagecommon.GIB
		zlog.Warn().Msg("ControllerExpandVolume (treeq) - volume minimum capacity should be greater 1 GB")
	}

	zlog.Debug().Msgf("ControllerExpandVolume (treeq) - filesystemID %d treeqID %d capacity %d maxSize %s\n", filesystemID, treeqID, capacity, maxFileSystemSize)
	err = treeq.TreeqService.UpdateTreeqVolume(filesystemID, treeqID, capacity, maxFileSystemSize)
	if err != nil {
		e := fmt.Errorf("ControllerUnpublishVolume (treeq) - UpdateTreeqVolume - error: %s", err.Error())
		zlog.Error().Msg(e.Error())
		return nil, e
	}
	return &csi.ControllerExpandVolumeResponse{
		CapacityBytes:         capacity,
		NodeExpansionRequired: false,
	}, nil
}
