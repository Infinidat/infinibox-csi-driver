package storage

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (treeq *treeqstorage) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (csiResp *csi.CreateVolumeResponse, err error) {
	var treeqVolumeMap map[string]string
	config := req.GetParameters()
	pvName := req.GetName()
	log.Debugf("Creating fileystem %s of nfs_treeq protocol ", pvName)
	log.Debug("-----------------------------------------------------------------------------------------")

	//Validating the reqired parameters
	validationStatus, validationStatusMap := treeq.filesysService.validateTreeqParameters(config)
	if !validationStatus {
		log.Errorf("Fail to validate parameter for nfs_treeq protocol %v ", validationStatusMap)
		return nil, status.Error(codes.InvalidArgument, "Fail to validate parameter for nfs_treeq protocol")
	}
	capacity := int64(req.GetCapacityRange().GetRequiredBytes())

	treeqVolumeMap, err = treeq.filesysService.CreateTreeqVolume(config, capacity, pvName)
	if err != nil {
		log.Errorf("fail to create volume %v", err)
		return &csi.CreateVolumeResponse{}, err
	}
	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      treeqVolumeMap["ID"] + "#" + treeqVolumeMap["TREEQID"] + "#" + config[MAXFILESYSTEMSIZE],
			CapacityBytes: capacity,
			VolumeContext: treeqVolumeMap,
			ContentSource: req.GetVolumeContentSource(),
		},
	}, nil
}

func getVolumeIDs(volumeID string) (filesystemID, treeqID int64, size string, err error) {
	volproto := strings.Split(volumeID, "#")
	if len(volproto) != 3 {
		err = errors.New("volume Id and other details not found")
		return
	}
	filesystemID, err = strconv.ParseInt(volproto[0], 10, 64)
	treeqID, err = strconv.ParseInt(volproto[1], 10, 64)
	size = volproto[2]
	return
}

func (treeq *treeqstorage) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	if len(req.GetVolumeId()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID missing in request")
	}

	filesystemID, treeqID, _, err := getVolumeIDs(req.GetVolumeId())
	if err != nil {
		log.Errorf("Invalid Volume ID %v", err)
		return nil, status.Error(codes.InvalidArgument, "Invalid volume ID")
	}
	nfsDeleteErr := treeq.filesysService.DeleteTreeqVolume(filesystemID, treeqID)
	if nfsDeleteErr != nil {
		if strings.Contains(nfsDeleteErr.Error(), "FILESYSTEM_NOT_FOUND") {
			log.Error("treeq already delete from infinibox")
			return &csi.DeleteVolumeResponse{}, nil
		}
		return &csi.DeleteVolumeResponse{}, nfsDeleteErr
	}
	log.Infof("treeq ID %s successfully deleted", req.GetVolumeId())
	return &csi.DeleteVolumeResponse{}, nil
}

func (treeq *treeqstorage) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	log.Debugf("ControllerPublishVolume %v", req)
	return &csi.ControllerPublishVolumeResponse{}, nil

}

func (treeq *treeqstorage) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	log.Debugf("ControllerUnpublishVolume %v", req)
	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

func (treeq *treeqstorage) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	return &csi.CreateSnapshotResponse{}, status.Error(codes.Unimplemented, "Unsupported operation for treeq")
}

func (treeq *treeqstorage) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	return &csi.DeleteSnapshotResponse{}, status.Error(codes.Unimplemented, "Unsupported operation for treeq")

}

func (treeq *treeqstorage) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (expandVolume *csi.ControllerExpandVolumeResponse, err error) {
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("Recovered from CSI ControllerExpandVolume " + fmt.Sprint(res))
		}
	}()

	filesystemID, treeqID, maxSize, err := getVolumeIDs(req.GetVolumeId())
	if err != nil {
		log.Errorf("Invalid Volume ID %v", err)
		return nil, status.Error(codes.InvalidArgument, "Invalid volume ID")
	}

	capacity := int64(req.GetCapacityRange().GetRequiredBytes())
	if capacity < gib {
		capacity = gib
		log.Warn("Volume Minimum capacity should be greater 1 GB")
	}

	err = treeq.filesysService.UpdateTreeqVolume(filesystemID, treeqID, capacity, maxSize)
	if err != nil {
		return
	}
	return &csi.ControllerExpandVolumeResponse{
		CapacityBytes:         capacity,
		NodeExpansionRequired: false,
	}, nil
}
