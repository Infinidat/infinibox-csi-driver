//go:build unit

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
	"errors"
	"fmt"
	"infinibox-csi-driver/api"
	"infinibox-csi-driver/helper"
	"infinibox-csi-driver/iboxapi"
	storagecommon "infinibox-csi-driver/storage/common"
	"infinibox-csi-driver/storage/nfs"
	"os"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

func (suite *TreeqNodeSuite) SetupTest() {
	suite.nfsMountMock = new(storagecommon.MockNfsMounter)
	suite.osHelperMock = new(helper.MockOsHelper)
	suite.storageHelperMock = new(storagecommon.MockStorageHelper)
	suite.api = new(api.MockAPIService)
	suite.iboxapi = new(iboxapi.MockAPIService)
	suite.cs = &storagecommon.Commonservice{IboxAPI: suite.iboxapi, API: suite.api, AccessModesHelper: suite.accessMock}
}

type TreeqNodeSuite struct {
	suite.Suite
	nfsMountMock      *storagecommon.MockNfsMounter
	osHelperMock      *helper.MockOsHelper
	accessMock        *helper.MockAccessModesHelper
	api               *api.MockAPIService
	iboxapi           *iboxapi.MockAPIService
	cs                *storagecommon.Commonservice
	storageHelperMock *storagecommon.MockStorageHelper
}

func TestTreeqNodeSuite(t *testing.T) {
	suite.Run(t, new(TreeqNodeSuite))
}

func (suite *TreeqNodeSuite) Test_TreeqNodePublishVolume_IsNotExist_false() {

	// mountOptions, err := treeq.NFSstorage.NFSStorageHelper.GetNFSMountOptions(req)

	nfs := nfs.NFSstorage{StorageHelper: suite.storageHelperMock, CS: *suite.cs, Mounter: suite.nfsMountMock, OSHelper: suite.osHelperMock}
	service := Treeqstorage{NFSstorage: nfs}
	randomDir := storagecommon.RandomString(10)
	targetPath := randomDir
	fmt.Printf("creating %s\n", "/tmp/"+targetPath)
	err := os.Mkdir("/tmp/"+targetPath, os.ModePerm)
	assert.Nil(suite.T(), err)
	defer func() {
		fmt.Printf("removing %s\n", "/tmp/"+targetPath)
		err := os.RemoveAll("/tmp/" + targetPath)
		assert.Nil(suite.T(), err)
	}()

	suite.iboxapi.On("GetFileSystemByID", mock.Anything).Return(&iboxapi.FileSystem{}, nil)
	suite.storageHelperMock.On("SetVolumePermissions", mock.Anything).Return(nil)
	suite.storageHelperMock.On("ValidateIPAddress", mock.Anything, mock.Anything).Return(nil)
	suite.storageHelperMock.On("GetNFSMountOptions", mock.Anything).Return([]string{}, nil)
	suite.iboxapi.On("CreateExport", mock.Anything).Return(storagecommon.GetExportResponseValue(), nil)
	suite.iboxapi.On("GetSystem").Return(storagecommon.GetSystem(), nil)
	exportResp := storagecommon.GetExportResponse()
	suite.iboxapi.On("GetExportsByFileSystemID", mock.Anything).Return(exportResp, nil)
	suite.iboxapi.On("DeleteExport", mock.Anything).Return(&iboxapi.Export{}, nil)
	suite.nfsMountMock.On("Mount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	suite.iboxapi.On("GetFileSystemTreeqCount", mock.Anything).Return(nil, nil)

	contex := storagecommon.GetPublishContexMap()
	contex["csiContainerHostMountPoint"] = "/tmp/"

	req := storagecommon.GetNodePublishVolumeRequest(targetPath, contex)
	req.VolumeId = "94148131#20000$$nfs_treeq"
	req.Secrets = map[string]string{
		"one":   "one",
		"two":   "two",
		"three": "three",
	}
	responce, err := service.NodePublishVolume(context.Background(), req)
	assert.Nil(suite.T(), err)
	assert.NotNil(suite.T(), responce, "empty object")
}

func (suite *TreeqNodeSuite) Test_TreeqNodePublishVolume_mount_success() {
	contex := storagecommon.GetPublishContexMap()
	contex["csiContainerHostMountPoint"] = "/tmp/"
	randomDir := storagecommon.RandomString(10)
	targetPath := randomDir
	err := os.Mkdir("/tmp/"+targetPath, os.ModePerm)
	assert.Nil(suite.T(), err)
	defer func() {
		err := os.RemoveAll("/tmp/" + targetPath)
		assert.Nil(suite.T(), err)
	}()
	nfs := nfs.NFSstorage{StorageHelper: suite.storageHelperMock, CS: *suite.cs, Mounter: suite.nfsMountMock, OSHelper: suite.osHelperMock}
	service := Treeqstorage{NFSstorage: nfs}
	suite.storageHelperMock.On("SetVolumePermissions", mock.Anything).Return(nil)
	suite.storageHelperMock.On("ValidateIPAddress", mock.Anything, mock.Anything).Return(nil)
	suite.storageHelperMock.On("GetNFSMountOptions", mock.Anything).Return([]string{}, nil)
	suite.nfsMountMock.On("Mount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	fs := &iboxapi.FileSystem{}
	suite.iboxapi.On("GetFileSystemByID", mock.Anything).Return(fs, nil)
	suite.iboxapi.On("GetSystem").Return(storagecommon.GetSystem(), nil)
	suite.iboxapi.On("CreateExport", mock.Anything).Return(storagecommon.GetExportResponseValue(), nil)
	exportResp := storagecommon.GetExportResponse()
	suite.iboxapi.On("GetExportsByFileSystemID", mock.Anything).Return(exportResp, nil)
	suite.iboxapi.On("DeleteExport", mock.Anything).Return(&iboxapi.Export{}, nil)
	suite.iboxapi.On("GetFileSystemByID", mock.Anything).Return(nil, nil)
	suite.iboxapi.On("GetFileSystemTreeqCount", mock.Anything).Return(nil, nil)

	req := storagecommon.GetNodePublishVolumeRequest(targetPath, contex)
	req.VolumeId = "94148131#20000$$nfs_treeq"
	req.Secrets = map[string]string{
		"one":   "one",
		"two":   "two",
		"three": "three",
	}
	_, err = service.NodePublishVolume(context.Background(), req)
	assert.Nil(suite.T(), err, "empty error")
}

func (suite *TreeqNodeSuite) Test_TreeqNodePublishVolume_mount_Error() {
	contex := storagecommon.GetPublishContexMap()
	contex["csiContainerHostMountPoint"] = "/tmp/"
	randomDir := storagecommon.RandomString(10)
	targetPath := randomDir
	mountErr := errors.New("mount error")
	nfs := nfs.NFSstorage{StorageHelper: suite.storageHelperMock, CS: *suite.cs, Mounter: suite.nfsMountMock, OSHelper: suite.osHelperMock}
	service := Treeqstorage{NFSstorage: nfs}
	// nfs := nfsstorage{mounter: suite.nfsMountMock, storageHelper: suite.storageHelperMock, osHelper: suite.osHelperMock}
	// service := treeqstorage{nfsstorage: nfs}
	suite.iboxapi.On("GetSystem", mock.Anything).Return(storagecommon.GetSystem(), nil)
	suite.storageHelperMock.On("SetVolumePermissions", mock.Anything).Return(nil)
	suite.storageHelperMock.On("ValidateIPAddress", mock.Anything, mock.Anything).Return(nil)
	suite.storageHelperMock.On("GetNFSMountOptions", mock.Anything).Return([]string{}, nil)
	suite.nfsMountMock.On("Mount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(mountErr)
	_, err := service.NodePublishVolume(context.Background(), storagecommon.GetNodePublishVolumeRequest(targetPath, contex))
	assert.NotNil(suite.T(), err, "not nil error")
}

func (suite *TreeqNodeSuite) Test_TreeqNodeUnpublishVolume_NotMountPoint_IsNotExist_true() {
	mountErr := errors.New("mount error")
	nfs := nfs.NFSstorage{Mounter: suite.nfsMountMock, OSHelper: suite.osHelperMock}
	service := Treeqstorage{NFSstorage: nfs}
	suite.nfsMountMock.On("IsLikelyNotMountPoint", mock.Anything).Return(true, nil)
	suite.osHelperMock.On("IsNotExist", mountErr).Return(true)
	suite.nfsMountMock.On("Mount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	suite.nfsMountMock.On("Unmount", mock.Anything).Return(nil)
	targetPath := "/var/lib/kublet/"
	suite.osHelperMock.On("Remove", targetPath).Return(nil)
	volumeID := "1234"
	_, err := service.NodeUnpublishVolume(context.Background(), storagecommon.GetNodeUnPublishVolumeRequest(targetPath, volumeID))
	assert.Nil(suite.T(), err, "empty error")
}

func (suite *TreeqNodeSuite) Test_TreeqNodeUnpublishVolume_notMnt_true() {
	targetPath := "/var/lib/kublet/"
	volumeID := "1234"
	nfs := nfs.NFSstorage{Mounter: suite.nfsMountMock, OSHelper: suite.osHelperMock}
	service := Treeqstorage{NFSstorage: nfs}
	suite.nfsMountMock.On("IsLikelyNotMountPoint", mock.Anything).Return(true, nil)
	suite.nfsMountMock.On("IsNotMountPoint", mock.Anything).Return(true, nil)
	suite.osHelperMock.On("Remove", targetPath).Return(nil)
	suite.nfsMountMock.On("Unmount", mock.Anything).Return(nil)

	_, err := service.NodeUnpublishVolume(context.Background(), storagecommon.GetNodeUnPublishVolumeRequest(targetPath, volumeID))
	assert.Nil(suite.T(), err, "empty err")
}

func (suite *TreeqNodeSuite) Test_TreeqNodeUnpublishVolume_unmount_sucess() {
	targetPath := "/var/lib/kublet/"
	volumeID := "1234"
	nfs := nfs.NFSstorage{Mounter: suite.nfsMountMock, OSHelper: suite.osHelperMock}
	service := Treeqstorage{NFSstorage: nfs}
	suite.nfsMountMock.On("IsLikelyNotMountPoint", mock.Anything).Return(true, nil)
	suite.nfsMountMock.On("IsNotMountPoint", mock.Anything).Return(true, nil)
	suite.nfsMountMock.On("Unmount", targetPath).Return(nil)
	suite.osHelperMock.On("Remove", targetPath).Return(nil)
	_, err := service.NodeUnpublishVolume(context.Background(), storagecommon.GetNodeUnPublishVolumeRequest(targetPath, volumeID))
	assert.Nil(suite.T(), err, "empty err")
}

func (suite *TreeqNodeSuite) Test_NodeStageVolume() {
	nfs := nfs.NFSstorage{Mounter: suite.nfsMountMock, OSHelper: suite.osHelperMock}
	service := Treeqstorage{NFSstorage: nfs}

	_, err := service.NodeStageVolume(context.Background(), &csi.NodeStageVolumeRequest{})
	assert.Nil(suite.T(), err, "empty err")
}

func (suite *TreeqNodeSuite) Test_NodeUnstageVolume() {
	nfs := nfs.NFSstorage{Mounter: suite.nfsMountMock, OSHelper: suite.osHelperMock}
	service := Treeqstorage{NFSstorage: nfs}

	_, err := service.NodeUnstageVolume(context.Background(), &csi.NodeUnstageVolumeRequest{})
	assert.Nil(suite.T(), err, "empty err")
}
