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
package storage

import (
	"context"
	"fmt"
	"infinibox-csi-driver/common"
	"strconv"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (treeq *treeqstorage) ValidateStorageClass(params map[string]string) error {
	requiredParams := map[string]string{
		common.SC_NETWORK_SPACE: `\A.*\z`, // TODO: could make this enforce IBOX network_space requirements, but probably not necessary
	}
	optionalParams := map[string]string{
		common.SC_MAX_FILESYSTEMS:           `\A\d+\z`,
		common.SC_MAX_TREEQS_PER_FILESYSTEM: `\A\d+\z`,
		common.SC_MAX_FILESYSTEM_SIZE:       `\A.*\z`, // TODO: add more specific pattern
	}

	err := ValidateRequiredOptionalSCParameters(requiredParams, optionalParams, params)
	if err != nil {
		zlog.Err(err)
		return status.Error(codes.InvalidArgument, err.Error())
	}

	err = validateNFSExportPermissions(params)
	if err != nil {
		zlog.Err(err)
		return status.Error(codes.InvalidArgument, err.Error())
	}

	return nil
}

func (treeq *treeqstorage) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (csiResp *csi.CreateVolumeResponse, err error) {
	zlog.Debug().Msgf("CreateVolume called pvName %s parameters %v", req.GetName(), req.GetParameters())

	params := req.GetParameters()

	for _, cap := range req.GetVolumeCapabilities() {
		if block := cap.GetBlock(); block != nil {
			e := fmt.Errorf("block access requested for %s PV %s", params[common.SC_STORAGE_PROTOCOL], req.GetName())
			zlog.Err(e)
			return nil, status.Error(codes.InvalidArgument, e.Error())
		}
	}

	treeq.nfsstorage.storageClassParameters = params
	fsPrefix := params[common.SC_FS_PREFIX]
	if fsPrefix == "" {
		fsPrefix = common.SC_FS_PREFIX_DEFAULT
	}
	treeqVolumeContext, err := treeq.treeqService.IsTreeqAlreadyExist(params[common.SC_POOL_NAME], strings.Trim(params[common.SC_NETWORK_SPACE], ""), req.GetName(), fsPrefix)
	if err != nil {
		zlog.Err(err)
		return nil, err
	}
	if len(treeqVolumeContext) == 0 {
		treeqVolumeContext, err = treeq.treeqService.CreateTreeqVolume(params, treeq.nfsstorage.capacity, req.GetName())
		if err != nil {
			zlog.Err(err)
			return nil, err
		}
	}

	treeqVolumeContext[common.SC_NFS_EXPORT_PERMISSIONS] = params[common.SC_NFS_EXPORT_PERMISSIONS]
	treeqVolumeContext[common.SC_STORAGE_PROTOCOL] = params[common.SC_STORAGE_PROTOCOL]
	treeqVolumeContext[common.SC_UID] = params[common.SC_UID]
	treeqVolumeContext[common.SC_GID] = params[common.SC_GID]

	volumeID := treeqVolumeContext["ID"] + "#" + treeqVolumeContext["TREEQID"]
	zlog.Debug().Msgf("CreateVolume final treeqVolumeMap %v volumeID %s", treeqVolumeContext, volumeID)
	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      volumeID,
			CapacityBytes: treeq.nfsstorage.capacity,
			VolumeContext: treeqVolumeContext,
			ContentSource: req.GetVolumeContentSource(),
		},
	}, nil
}

func getVolumeIDs(volumeID string) (filesystemID, treeqID int64, err error) {
	volproto := strings.Split(volumeID, "#")
	if len(volproto) != 2 {
		err = fmt.Errorf("volume Id %s and other details not found", volumeID)
		return 0, 0, err
	}
	if filesystemID, err = strconv.ParseInt(volproto[0], 10, 64); err != nil {
		zlog.Err(err)
		return 0, 0, err
	}

	// volumeID example := "94148131#20000$$nfs_treeq"
	treeqdetails := strings.Split(volproto[1], "$")

	if treeqID, err = strconv.ParseInt(treeqdetails[0], 10, 64); err != nil {
		zlog.Err(err)
		return 0, 0, err
	}

	return filesystemID, treeqID, nil
}

func (treeq *treeqstorage) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	zlog.Debug().Msgf("DeleteVolume called on volume ID %s", req.GetVolumeId())

	filesystemID, treeqID, err := getVolumeIDs(req.GetVolumeId())
	if err != nil {
		e := fmt.Errorf("invalid volume id %v", err)
		zlog.Err(e)
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}
	nfsDeleteErr := treeq.treeqService.DeleteTreeqVolume(filesystemID, treeqID)
	if nfsDeleteErr != nil {
		zlog.Err(nfsDeleteErr)
		if strings.Contains(nfsDeleteErr.Error(), "FILESYSTEM_NOT_FOUND") {
			zlog.Error().Msg("treeq already delete from infinibox")
			return &csi.DeleteVolumeResponse{}, nil
		}
		return nil, nfsDeleteErr
	}
	zlog.Debug().Msgf("treeq ID %s successfully deleted", req.GetVolumeId())
	return &csi.DeleteVolumeResponse{}, nil
}

func (treeq *treeqstorage) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	return &csi.ControllerPublishVolumeResponse{}, nil
}

func (treeq *treeqstorage) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	zlog.Debug().Msgf("ControllerUnpublishVolume")
	kubeNodeID := req.GetNodeId()
	if kubeNodeID == "" {
		return nil, status.Error(codes.InvalidArgument, "node ID is required")
	}
	voltype := req.GetVolumeId()
	volproto := strings.Split(voltype, "$$")
	tmp := strings.Split(volproto[0], "#")
	fileID, _ := strconv.ParseInt(tmp[0], 10, 64)
	zlog.Debug().Msgf("ControllerUnpublishVolume volproto %+v fileId %d nodeId %s", volproto, fileID, kubeNodeID)
	err := treeq.nfsstorage.cs.Api.DeleteExportRule(fileID, kubeNodeID)
	if err != nil {
		zlog.Error().Msgf("failed to delete Export Rule fileystemID %d error %v", fileID, err)
		return nil, status.Errorf(codes.Internal, "failed to delete Export Rule  %v", err)
	}
	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

func (treeq *treeqstorage) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	return nil, status.Error(codes.Unimplemented, "Unsupported operation for treeq")
}

func (treeq *treeqstorage) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	return nil, status.Error(codes.Unimplemented, "Unsupported operation for treeq")
}

func (treeq *treeqstorage) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (expandVolume *csi.ControllerExpandVolumeResponse, err error) {
	zlog.Debug().Msgf("ControllerExpandVolume")

	maxFileSystemSize := treeq.nfsstorage.storageClassParameters[common.SC_MAX_FILESYSTEM_SIZE]
	filesystemID, treeqID, err := getVolumeIDs(req.GetVolumeId())
	if err != nil {
		e := fmt.Errorf("invalid volume id %v", err)
		zlog.Err(e)
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	capacity := int64(req.GetCapacityRange().GetRequiredBytes())
	if capacity < gib {
		capacity = gib
		zlog.Warn().Msg("volume minimum capacity should be greater 1 GB")
	}

	zlog.Debug().Msgf("filesystemID %d treeqID %d capacity %d maxSize %s\n", filesystemID, treeqID, capacity, maxFileSystemSize)
	err = treeq.treeqService.UpdateTreeqVolume(filesystemID, treeqID, capacity, maxFileSystemSize)
	if err != nil {
		zlog.Err(err)
		return
	}
	return &csi.ControllerExpandVolumeResponse{
		CapacityBytes:         capacity,
		NodeExpansionRequired: false,
	}, nil
}
