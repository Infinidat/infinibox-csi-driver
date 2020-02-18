package storage

import (
	"context"
	"path"
	"strings"
	"time"

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

	err = treeq.filesysService.CreateNFSVolume()
	if err != nil {
		log.Errorf("fail to create volume %v", err)
		return &csi.CreateVolumeResponse{}, err
	}
	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      treeq.filesysService.treeqVolume["ID"],
			CapacityBytes: capacity,
			VolumeContext: treeq.filesysService.treeqVolume,
			ContentSource: req.GetVolumeContentSource(),
		},
	}, nil
}

func (treeq *treeqstorage) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	return &csi.DeleteVolumeResponse{}, status.Error(codes.Unimplemented, time.Now().String())

}

func (treeq *treeqstorage) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	log.Debugf("ControllerPublishVolume %v", req)
	return &csi.ControllerPublishVolumeResponse{}, nil

}

func (treeq *treeqstorage) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	log.Debugf("ControllerUnpublishVolume %v", req)
	return &csi.ControllerUnpublishVolumeResponse{}, nil
}
