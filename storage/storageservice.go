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
	"errors"

	"github.com/infinidat/infinibox-csi-driver/api"
	"github.com/infinidat/infinibox-csi-driver/common"
	storagecommon "github.com/infinidat/infinibox-csi-driver/storage/common"
	"github.com/infinidat/infinibox-csi-driver/storage/fc"
	"github.com/infinidat/infinibox-csi-driver/storage/iscsi"
	"github.com/infinidat/infinibox-csi-driver/storage/nfs"
	"github.com/infinidat/infinibox-csi-driver/storage/nvme"
	"github.com/infinidat/infinibox-csi-driver/storage/treeq"

	"github.com/container-storage-interface/spec/lib/go/csi"
)

const (
	Name = "infinibox-csi-driver"
)

type StorageOperations interface {
	csi.ControllerServer
	csi.NodeServer
	ValidateStorageClass(params map[string]string) error
}

func NewStorageController(config map[string]string, secrets map[string]string, volumePrototype *api.VolumeProtocolConfig, capacity int64) (operations StorageOperations, commonService storagecommon.Commonservice, err error) {

	commonService, err = storagecommon.BuildCommonService(config, secrets, volumePrototype)
	if err != nil {
		return
	}

	operations, err = NewStorageNode(commonService, capacity)
	if err != nil {
		return
	}
	return operations, commonService, nil
}

// NewStorageNode : To return specific implementation of storage
func NewStorageNode(commonService storagecommon.Commonservice, capacity int64) (StorageOperations, error) {
	storageProtocol := commonService.VolProto.StorageType

	switch storageProtocol {
	case common.ProtocolFC:
		return fc.NewFCstorage(capacity, commonService), nil
	case common.ProtocolISCSI:
		return iscsi.NewISCSIstorage(capacity, commonService), nil
	case common.ProtocolNVME:
		return nvme.NewNVMEstorage(capacity, commonService), nil
	case common.ProtocolNFS:
		return nfs.NewNFSstorage(capacity, commonService), nil
	case common.ProtocolTreeq:
		return treeq.NewTreeqstorage(capacity, commonService), nil
	default:
		return nil, errors.New("Error: Invalid storage protocol -" + storageProtocol)
	}
}

func NewStorageNodeAndCommonService(capacity int64, config map[string]string, secrets map[string]string, volumePrototype *api.VolumeProtocolConfig) (operations StorageOperations, commonService storagecommon.Commonservice, err error) {
	commonService, err = storagecommon.BuildCommonService(config, secrets, volumePrototype)
	if err != nil {
		return
	}

	operations, err = NewStorageNode(commonService, capacity)
	return operations, commonService, err

}
