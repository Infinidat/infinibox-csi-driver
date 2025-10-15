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
	"infinibox-csi-driver/common"
	storagecommon "infinibox-csi-driver/storage/common"
	"infinibox-csi-driver/storage/fc"
	"infinibox-csi-driver/storage/iscsi"
	"infinibox-csi-driver/storage/nfs"
	"infinibox-csi-driver/storage/nvme"
	"infinibox-csi-driver/storage/treeq"

	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
)

const (
	Name = "infinibox-csi-driver"
)

// env vars that let users override various delay times
type Storageoperations interface {
	csi.ControllerServer
	csi.NodeServer
	ValidateStorageClass(params map[string]string) error
}

// Mutex protecting device rescan and delete operations
// var deviceMu sync.Mutex

// NewStorageController : To return specific implementation of storage
func NewStorageController(comnserv storagecommon.Commonservice, capacity int64, storageProtocol string, configparams ...map[string]string) (Storageoperations, error) {
	storageProtocol = strings.ToLower(strings.TrimSpace(storageProtocol))

	switch storageProtocol {
	case common.PROTOCOL_FC:
		return fc.NewFCstorage(capacity, comnserv), nil
	case common.PROTOCOL_ISCSI:
		return iscsi.NewISCSIstorage(capacity, comnserv), nil
	case common.PROTOCOL_NVME:
		return nvme.NewNVMEstorage(capacity, comnserv), nil
	case common.PROTOCOL_NFS:
		return nfs.NewNFSstorage(capacity, comnserv), nil
	case common.PROTOCOL_TREEQ:
		return treeq.NewTreeqstorage(capacity, comnserv), nil
	}
	return nil, errors.New("Error: Invalid storage protocol - " + storageProtocol)
}

// NewStorageNode : To return specific implementation of storage
func NewStorageNode(comnserv storagecommon.Commonservice, configparams ...map[string]string) (Storageoperations, error) {
	volProto := comnserv.VolProto

	storageProtocol := volProto.StorageType
	switch storageProtocol {
	case common.PROTOCOL_FC:
		return fc.NewFCstorage(0, comnserv), nil
	case common.PROTOCOL_ISCSI:
		return iscsi.NewISCSIstorage(0, comnserv), nil
	case common.PROTOCOL_NVME:
		return nvme.NewNVMEstorage(0, comnserv), nil
	case common.PROTOCOL_NFS:
		return nfs.NewNFSstorage(0, comnserv), nil
	case common.PROTOCOL_TREEQ:
		return treeq.NewTreeqstorage(0, comnserv), nil
	default:
		return nil, errors.New("Error: Invalid storage protocol -" + storageProtocol)
	}
}
