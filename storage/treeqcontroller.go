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
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog"
)

func (treeq *treeqstorage) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (csiResp *csi.CreateVolumeResponse, err error) {
	var treeqVolumeMap map[string]string
	config := req.GetParameters()
	pvName := req.GetName()
	klog.V(4).Infof("CSI request parameters: %v", config)
	err = validateStorageClassParameters(map[string]string{
		"pool_name":                 `\A.*\z`, // TODO: could make this enforce IBOX pool_name requirements, but probably not necessary
		"network_space":             `\A.*\z`, // TODO: could make this enforce IBOX network_space requirements, but probably not necessary
		"max_filesystems":           `\A\d+\z`,
		"max_treeqs_per_filesystem": `\A\d+\z`,
		"max_filesystem_size":       `\A.*\z`, // TODO: add more specific pattern
	}, config)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	// TODO: negative validation - eg useCHAP should NOT be specified for nfs

	// TODO: move this capacity validation into controller.go
	capacity := int64(req.GetCapacityRange().GetRequiredBytes())
	if capacity < gib {
		capacity = gib
		klog.Warningf("Volume Minimum capacity should be greater 1 GB")
	}

	// basic sanity-checking to ensure the user is not requesting block access to a NFS filesystem
	// TODO: improve and standardize this across protocols - CSIC-304
	for _, cap := range req.GetVolumeCapabilities() {
		if block := cap.GetBlock(); block != nil {
			msg := fmt.Sprintf("Block access requested for NFS TreeQ PV %s", pvName)
			klog.Errorf(msg)
			return nil, status.Error(codes.InvalidArgument, msg)
		}
	}

	treeqVolumeMap, err = treeq.filesysService.IsTreeqAlreadyExist(config["pool_name"], strings.Trim(config["network_space"], ""), pvName)
	if len(treeqVolumeMap) == 0 && err == nil {
		treeqVolumeMap, err = treeq.filesysService.CreateTreeqVolume(config, capacity, pvName)
	}
	if err != nil {
		klog.Errorf("failed to create volume %v", err)
		return nil, err
	}
	klog.V(4).Infof("CreateVolume treeqVolumeMap is %v\n", treeqVolumeMap)
	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      treeqVolumeMap["ID"] + "#" + treeqVolumeMap["TREEQID"], // + "#" + config[MAXFILESYSTEMSIZE],
			CapacityBytes: capacity,
			VolumeContext: treeqVolumeMap,
			ContentSource: req.GetVolumeContentSource(),
		},
	}, nil
}

func getVolumeIDs(volumeID string) (filesystemID, treeqID int64, size string, err error) {
	volproto := strings.Split(volumeID, "#")
	// TODO jeff, this seems to only ever have 2 parameters NOT 3
	if len(volproto) != 2 {
		err = fmt.Errorf("volume Id %s and other details not found", volumeID)
		return 0, 0, "", err
	}
	if filesystemID, err = strconv.ParseInt(volproto[0], 10, 64); err != nil {
		return 0, 0, "", err
	}

	// volumeID example := "94148131#20000$$nfs_treeq"
	treeqdetails := strings.Split(volproto[1], "$")

	if treeqID, err = strconv.ParseInt(treeqdetails[0], 10, 64); err != nil {
		return 0, 0, "", err
	}

	// TODO jeff, I commented this out and provided a default of 0, seems to not be implemented later on
	//size = volproto[2]
	size = ""
	return filesystemID, treeqID, size, nil
}

func (treeq *treeqstorage) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	if len(req.GetVolumeId()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID missing in request")
	}

	filesystemID, treeqID, _, err := getVolumeIDs(req.GetVolumeId())
	if err != nil {
		klog.Errorf("Invalid Volume ID %v", err)
		return nil, status.Error(codes.InvalidArgument, "Invalid volume ID")
	}
	nfsDeleteErr := treeq.filesysService.DeleteTreeqVolume(filesystemID, treeqID)
	if nfsDeleteErr != nil {
		if strings.Contains(nfsDeleteErr.Error(), "FILESYSTEM_NOT_FOUND") {
			klog.Error("treeq already delete from infinibox")
			return &csi.DeleteVolumeResponse{}, nil
		}
		return nil, nfsDeleteErr
	}
	klog.V(2).Infof("treeq ID %s successfully deleted", req.GetVolumeId())
	return &csi.DeleteVolumeResponse{}, nil
}

func (treeq *treeqstorage) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	return &csi.ControllerPublishVolumeResponse{}, nil
}

func (treeq *treeqstorage) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

func (treeq *treeqstorage) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	return nil, status.Error(codes.Unimplemented, "Unsupported operation for treeq")
}

func (treeq *treeqstorage) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	return nil, status.Error(codes.Unimplemented, "Unsupported operation for treeq")
}

func (treeq *treeqstorage) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (expandVolume *csi.ControllerExpandVolumeResponse, err error) {
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("Recovered from CSI ControllerExpandVolume " + fmt.Sprint(res))
		}
	}()

	filesystemID, treeqID, maxSize, err := getVolumeIDs(req.GetVolumeId())
	if err != nil {
		klog.Errorf("Invalid Volume ID %v", err)
		return nil, status.Error(codes.InvalidArgument, "Invalid volume ID")
	}

	capacity := int64(req.GetCapacityRange().GetRequiredBytes())
	if capacity < gib {
		capacity = gib
		klog.Warning("Volume Minimum capacity should be greater 1 GB")
	}

	klog.V(4).Infof("filesystemID %d treeqID %d capacity %d maxSize %s\n", filesystemID, treeqID, capacity, maxSize)
	err = treeq.filesysService.UpdateTreeqVolume(filesystemID, treeqID, capacity, maxSize)
	if err != nil {
		return
	}
	return &csi.ControllerExpandVolumeResponse{
		CapacityBytes:         capacity,
		NodeExpansionRequired: false,
	}, nil
}
