package storage

import (
	"context"
	"errors"
	"path"
	"strconv"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (treeq *treeqstorage) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (csiResp *csi.CreateVolumeResponse, err error) {
	config := req.GetParameters()
	pvName := req.GetName()
	log.Debugf("Creating fileystem %s of nfs_treeq protocol ", pvName)

	//Validating the reqired parameters
	validationStatus, validationStatusMap := treeq.filesysService.validateTreeqParameters(config)
	if !validationStatus {
		log.Errorf("Fail to validate parameter for nfs_treeq protocol %v ", validationStatusMap)
		return nil, status.Error(codes.InvalidArgument, "Fail to validate parameter for nfs_treeq protocol")
	}
	treeq.filesysService.treeqVolume["storage_protocol"] = config["storage_protocol"]
	treeq.filesysService.treeqVolume["nfs_mount_options"] = config["nfs_mount_options"]

	capacity := int64(req.GetCapacityRange().GetRequiredBytes())
	treeq.filesysService.pVName = pvName
	treeq.filesysService.configmap = config
	treeq.filesysService.capacity = capacity                      //NO minimum size required for treeq so minimimum 1gb validation removed
	treeq.filesysService.exportpath = path.Join(dataRoot, pvName) //TODO: export path prefix need to add here

	ipAddress, err := treeq.filesysService.cs.getNetworkSpaceIP(strings.Trim(config["nfs_networkspace"], " "))
	if err != nil {
		log.Errorf("fail to get networkspace ipaddress %v", err)
		return
	}
	treeq.filesysService.ipAddress = ipAddress
	log.Debugf("getNetworkSpaceIP ipAddress %s", ipAddress)

	err = treeq.filesysService.CreateTreeqVolume()
	if err != nil {
		log.Errorf("fail to create volume %v", err)
		return &csi.CreateVolumeResponse{}, err
	}
	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      treeq.filesysService.treeqVolume["ID"] + "#" + treeq.filesysService.treeqVolume["TREEQID"],
			CapacityBytes: capacity,
			VolumeContext: treeq.filesysService.treeqVolume,
			ContentSource: req.GetVolumeContentSource(),
		},
	}, nil
}

func getVolumeIDs(volumeID string) (filesystemID, treeqID int64, err error) {
	volproto := strings.Split(volumeID, "#")
	if len(volproto) != 2 {
		err = errors.New("volume Id and other details not found")
		return
	}
	filesystemID, err = strconv.ParseInt(volproto[0], 10, 64)
	treeqID, err = strconv.ParseInt(volproto[1], 10, 64)
	return
}

func (treeq *treeqstorage) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	if len(req.GetVolumeId()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID missing in request")
	}
	filesystemID, treeqID, err := getVolumeIDs(req.GetVolumeId())
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
