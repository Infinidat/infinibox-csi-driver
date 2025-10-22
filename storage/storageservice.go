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

	"github.com/infinidat/infinibox-csi-driver/common"
	storagecommon "github.com/infinidat/infinibox-csi-driver/storage/common"
	"github.com/infinidat/infinibox-csi-driver/storage/fc"
	"github.com/infinidat/infinibox-csi-driver/storage/iscsi"
	"github.com/infinidat/infinibox-csi-driver/storage/nfs"
	"github.com/infinidat/infinibox-csi-driver/storage/nvme"
	"github.com/infinidat/infinibox-csi-driver/storage/treeq"

	"strings"

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

// NewStorageController : To return specific implementation of storage
func NewStorageController(commonService storagecommon.Commonservice, capacity int64, storageProtocol string) (StorageOperations, error) {
	storageProtocol = strings.ToLower(strings.TrimSpace(storageProtocol))

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
	}
	return nil, errors.New("Error: Invalid storage protocol - " + storageProtocol)
}

// NewStorageNode : To return specific implementation of storage
func NewStorageNode(commonService storagecommon.Commonservice) (StorageOperations, error) {
	volProto := commonService.VolProto

	storageProtocol := volProto.StorageType
	switch storageProtocol {
	case common.ProtocolFC:
		return fc.NewFCstorage(0, commonService), nil
	case common.ProtocolISCSI:
		return iscsi.NewISCSIstorage(0, commonService), nil
	case common.ProtocolNVME:
		return nvme.NewNVMEstorage(0, commonService), nil
	case common.ProtocolNFS:
		return nfs.NewNFSstorage(0, commonService), nil
	case common.ProtocolTreeq:
		return treeq.NewTreeqstorage(0, commonService), nil
	default:
		return nil, errors.New("Error: Invalid storage protocol -" + storageProtocol)
	}
}
