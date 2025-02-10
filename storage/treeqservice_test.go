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
package storage

import (
	"errors"
	"fmt"
	"infinibox-csi-driver/api"
	"infinibox-csi-driver/common"
	"infinibox-csi-driver/helper"
	"infinibox-csi-driver/iboxapi"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

func (suite *TreeqServiceSuite) SetupTest() {
	suite.api = new(api.MockApiService)
	suite.iboxapi = new(iboxapi.MockApiService)
	suite.accessMock = new(helper.MockAccessModesHelper)
	volproto := &api.VolumeProtocolConfig{
		VolumeID:    1,
		StorageType: "",
	}
	cs := Commonservice{Api: suite.api, IboxApi: suite.iboxapi, VolProto: volproto}
	suite.cs = &cs
	nfs := nfsstorage{cs: cs, capacity: 100 * gib}
	suite.service = treeqstorage{nfsstorage: nfs}

	suite.someError = errors.New("Some error")
}

type TreeqServiceSuite struct {
	suite.Suite
	accessMock *helper.MockAccessModesHelper
	api        *api.MockApiService
	iboxapi    *iboxapi.MockApiService
	cs         *Commonservice
	service    treeqstorage
	someError  error
}

func TestTreeqServiceSuite(t *testing.T) {
	suite.Run(t, new(TreeqServiceSuite))
}

func (suite *TreeqServiceSuite) Test_getExpectedFileSystemID_maxfilesystem() {
	expectedErr := errors.New("some error")
	suite.api.On("GetStoragePoolIDByName", mock.Anything).Return(0, expectedErr)
	nfs := nfsstorage{capacity: 209951162777600}
	service := &TreeqService{
		cs:         *suite.cs,
		nfsstorage: nfs,
	}
	_, err := service.getExpectedFileSystemID(1000)
	assert.NotNil(suite.T(), err, "empty object")
}

func (suite *TreeqServiceSuite) Test_getExpectedFileSystemID_getMaxSize_error() {
	poolID := 10
	suite.api.On("GetStoragePoolIDByName", mock.Anything).Return(poolID, nil)
	nfs := nfsstorage{capacity: 209951162777600}
	service := TreeqService{cs: *suite.cs, nfsstorage: nfs}
	configmap := map[string]string{
		common.SC_MAX_FILESYSTEM_SIZE: "4mib",
	}
	service.nfsstorage.storageClassParameters = configmap
	_, err := service.getExpectedFileSystemID(10)
	fmt.Println(err)
	assert.NotNil(suite.T(), err, "empty object")
}

func (suite *TreeqServiceSuite) Test_getExpectedFileSystemID_FileSystemByPoolID_error() {
	expectedErr := errors.New("some error")
	poolID := 10
	suite.api.On("GetStoragePoolIDByName", mock.Anything).Return(poolID, nil)
	suite.iboxapi.On("GetFileSystemsByPool", mock.Anything, mock.Anything).Return(nil, expectedErr)
	suite.api.On("GetMaxTreeqPerFs").Return(10000, nil)
	service := TreeqService{cs: *suite.cs}
	_, err := service.getExpectedFileSystemID(1000)
	assert.NotNil(suite.T(), err, "empty object")
}

func (suite *TreeqServiceSuite) Test_getExpectedFileSystemID_FilesytemTreeqCount_error() {
	expectedErr := errors.New("some error")
	fsMetada := getfsMetadata2()
	poolID := 10
	suite.api.On("GetStoragePoolIDByName", mock.Anything).Return(poolID, nil)
	suite.iboxapi.On("GetFileSystemsByPool", mock.Anything, mock.Anything).Return(fsMetada, nil)
	suite.api.On("GetFilesystemTreeqCount", mock.Anything).Return(0, expectedErr)
	suite.api.On("GetMaxTreeqPerFs").Return(10000, nil)
	nfs := nfsstorage{capacity: 100}
	service := TreeqService{cs: *suite.cs, nfsstorage: nfs}
	_, err := service.getExpectedFileSystemID(9999990)
	assert.NotNil(suite.T(), err, "empty object")
}

func (suite *TreeqServiceSuite) Test_getExpectedFileSystemID_Success() {
	fsMetada := getfsMetadata()
	poolID := 10
	fsID := 10
	suite.api.On("GetStoragePoolIDByName", mock.Anything).Return(poolID, nil)
	suite.iboxapi.On("GetFileSystemsByPool", mock.Anything, mock.Anything).Return(fsMetada, nil)
	suite.api.On("GetFilesystemTreeqCount", mock.Anything).Return(1, nil)
	suite.api.On("GetMaxTreeqPerFs").Return(10000, nil)

	exportResp := getExportResponse()
	suite.iboxapi.On("GetExportsByFileSystemID", fsID).Return(exportResp, nil)
	fsMetada2 := getfsMetadata2()
	suite.iboxapi.On("GetFileSystemsByPool", poolID, mock.Anything).Return(fsMetada2, nil)
	service := TreeqService{cs: *suite.cs}

	service.nfsstorage.capacity = 1000
	service.nfsstorage.exportPath = "/exportPath"

	fs, err := service.getExpectedFileSystemID(9999999999999)
	assert.Nil(suite.T(), err, "empty object")
	assert.Equal(suite.T(), fs.ID, fsID, "file system ID equal")
}

func getnetworkspace() api.NetworkSpace {
	networkSpace := api.NetworkSpace{}
	var p1 api.Portal
	p1.IpAdress = "10.20.30.40"
	networkSpace.Portals = append(networkSpace.Portals, p1)
	return networkSpace
}

func (suite *TreeqServiceSuite) Test_CreateTreeqVolume_Success() {
	fsMetada := getfsMetadata2()
	poolID := 10
	fsID := 11

	poolResult := &iboxapi.PoolResult{ID: 10}
	suite.iboxapi.On("GetPoolByName", mock.Anything).Return(poolResult, nil)
	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getnetworkspace(), nil)
	suite.iboxapi.On("GetFileSystemsByPool", poolID, mock.Anything).Return(fsMetada, nil)
	suite.api.On("GetFilesystemTreeqCount", fsID).Return(1, nil)
	suite.api.On("GetMaxTreeqPerFs").Return(10000, nil)

	exportResp := getExportResponse()
	suite.iboxapi.On("GetExportsByFileSystemID", fsID).Return(exportResp, nil)

	treeqResp := getTreeQResponse(fsID)
	suite.api.On("CreateTreeq", fsID, mock.Anything).Return(*treeqResp, nil)

	metadataResp := getMetadaResponse()
	suite.iboxapi.On("PutMetadata", fsID, mock.Anything).Return(*metadataResp, nil)

	suite.api.On("UpdateFilesystem", fsID, mock.Anything).Return(nil, nil)
	service := TreeqService{cs: *suite.cs}

	// CreateVolumeRequest parameter values to filesystemService
	var capacity int64 = 1000
	pVName := "csi-TestTreeq"
	configMap := getCreateTreeqVolumeParameter()

	_, err := service.CreateTreeqVolume(configMap, capacity, pVName)
	assert.Nil(suite.T(), err, "empty object")
}

func (suite *TreeqServiceSuite) Test_CreateTreeqVolume_FileSystemCount_Error() {
	var fsMetada api.FSMetadata
	poolID := 10
	expectedErr := errors.New("some error")

	pool := iboxapi.PoolResult{
		Name: "pool_name1",
		ID:   10,
	}
	suite.iboxapi.On("GetPoolByName", mock.Anything).Return(&pool, nil)
	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getnetworkspace(), nil)
	suite.api.On("GetStoragePoolIDByName", mock.Anything).Return(poolID, nil)
	suite.iboxapi.On("GetFileSystemsByPool", poolID, mock.Anything).Return(fsMetada, nil)
	suite.api.On("GetFilesystemTreeqCount", mock.Anything).Return(0, expectedErr)
	suite.api.On("GetMaxTreeqPerFs").Return(10000, nil)
	suite.iboxapi.On("CreateFileSystem", mock.Anything).Return(&iboxapi.FileSystem{}, nil)
	suite.iboxapi.On("CreateExport", mock.Anything).Return(&iboxapi.Export{}, nil)
	suite.iboxapi.On("PutMetadata", mock.Anything, mock.Anything).Return(nil, nil)
	suite.api.On("CreateTreeq", mock.Anything, mock.Anything).Return(nil, nil)

	nfs := nfsstorage{capacity: 100, cs: *suite.cs}
	service := TreeqService{cs: *suite.cs, nfsstorage: nfs}
	service.treeqCnt = -1
	var capacity int64 = 1000
	pVName := "csi-TestTreeq"
	configMap := map[string]string{
		"network_space": "networkspace",
	}
	_, err := service.CreateTreeqVolume(configMap, capacity, pVName)
	assert.NotNil(suite.T(), err, "failed to get filecount")
}

func (suite *TreeqServiceSuite) Test_CreateTreeqVolume_FileSystemCount_notAllowed() {
	var fsMetada api.FSMetadata
	poolID := 10

	pool := iboxapi.PoolResult{
		Name: "pool_name1",
		ID:   10,
	}
	suite.iboxapi.On("GetPoolByName", mock.Anything).Return(&pool, nil)
	expectedErr := errors.New("some error")
	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getnetworkspace(), nil)
	suite.api.On("GetStoragePoolIDByName", mock.Anything).Return(poolID, nil)
	suite.iboxapi.On("GetFileSystemsByPool", poolID, mock.Anything).Return(fsMetada, nil)
	suite.api.On("GetFileSystemCountByPoolID", mock.Anything).Return(20000, nil)
	suite.api.On("GetFilesystemTreeqCount", mock.Anything).Return(0, expectedErr)
	suite.iboxapi.On("CreateFileSystem", mock.Anything).Return(&iboxapi.FileSystem{}, nil)
	suite.api.On("GetMaxTreeqPerFs").Return(10000, nil)
	suite.iboxapi.On("CreateExport", mock.Anything).Return(&iboxapi.Export{}, nil)
	suite.iboxapi.On("PutMetadata", mock.Anything, mock.Anything).Return(nil, nil)
	suite.api.On("CreateTreeq", mock.Anything, mock.Anything).Return(nil, nil)
	nfs := nfsstorage{capacity: 100, cs: *suite.cs}
	service := TreeqService{cs: *suite.cs, nfsstorage: nfs}
	service.treeqCnt = -1

	var capacity int64 = 1000
	pVName := "csi-TestTreeq"
	configMap := getCreateTreeqVolumeParameter()

	_, err := service.CreateTreeqVolume(configMap, capacity, pVName)
	assert.NotNil(suite.T(), err, "failed to get filecount")
}

func (suite *TreeqServiceSuite) Test_CreateTreeqVolume_CreateFileSystem_Error() {
	var fsMetada api.FSMetadata
	expectedErr := errors.New("some error")

	pool := iboxapi.PoolResult{
		Name: "pool_name1",
		ID:   100,
	}
	suite.iboxapi.On("GetPoolByName", mock.Anything).Return(&pool, nil)
	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getnetworkspace(), nil)
	suite.iboxapi.On("GetFileSystemsByPool", pool.ID, mock.Anything).Return(fsMetada, nil)
	suite.api.On("GetFileSystemCountByPoolID", mock.Anything).Return(200, nil)
	suite.iboxapi.On("CreateFileSystem", mock.Anything).Return(nil, expectedErr)
	suite.api.On("GetMaxTreeqPerFs").Return(10000, nil)
	suite.iboxapi.On("PutMetadata", mock.Anything, mock.Anything).Return(nil, nil)

	nfs := nfsstorage{capacity: 100, cs: *suite.cs}
	service := TreeqService{cs: *suite.cs, nfsstorage: nfs}
	// CreateVolumeRequest parameter values to filesystemService
	var capacity int64 = 1000
	pVName := "csi-TestTreeq"
	configMap := getCreateTreeqVolumeParameter()
	configMap[common.SC_FS_PREFIX] = "csit_"

	_, err := service.CreateTreeqVolume(configMap, capacity, pVName)
	assert.NotNil(suite.T(), err, "failed to get filecount")
}

func (suite *TreeqServiceSuite) Test_CreateTreeqVolume_ExportFileSystem_Error() {
	var fsMetada api.FSMetadata
	poolID := 10
	expectedErr := errors.New("some error")
	pool := iboxapi.PoolResult{
		Name: "pool_name1",
		ID:   10,
	}
	suite.iboxapi.On("GetPoolByName", mock.Anything).Return(&pool, nil)

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getnetworkspace(), nil)
	suite.api.On("GetStoragePoolIDByName", mock.Anything).Return(poolID, nil)
	suite.iboxapi.On("GetFileSystemsByPool", poolID, mock.Anything).Return(fsMetada, nil)
	suite.api.On("GetFileSystemCountByPoolID", mock.Anything).Return(200, nil)
	suite.iboxapi.On("CreateFileSystem", mock.Anything).Return(getFileSystem, nil)
	suite.iboxapi.On("CreateExport", mock.Anything).Return(nil, expectedErr)
	suite.api.On("GetMaxTreeqPerFs").Return(10000, nil)

	nfs := nfsstorage{capacity: 100, cs: *suite.cs}
	service := TreeqService{cs: *suite.cs, nfsstorage: nfs}
	var capacity int64 = 1000
	pVName := "csi-TestTreeq"
	configMap := getCreateTreeqVolumeParameter()
	configMap[common.SC_FS_PREFIX] = "csit_"

	_, err := service.CreateTreeqVolume(configMap, capacity, pVName)
	assert.NotNil(suite.T(), err, "failed to get filecount")
}

func (suite *TreeqServiceSuite) Test_CreateTreeqVolume_metadata_Error() {
	var fsMetada api.FSMetadata
	poolID := 10
	expectedErr := errors.New("some error")
	pool := iboxapi.PoolResult{
		Name: "pool_name1",
		ID:   10,
	}
	suite.iboxapi.On("GetPoolByName", mock.Anything).Return(&pool, nil)

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getnetworkspace(), nil)
	suite.iboxapi.On("GetFileSystemsByPool", poolID, mock.Anything).Return(fsMetada, nil)
	suite.api.On("GetFileSystemCountByPoolID", mock.Anything).Return(200, nil)
	suite.iboxapi.On("CreateFileSystem", mock.Anything).Return(getFileSystem, nil)
	suite.iboxapi.On("CreateExport", mock.Anything).Return(&iboxapi.Export{}, nil)
	suite.iboxapi.On("PutMetadata", mock.Anything, mock.Anything).Return(nil, expectedErr)
	suite.api.On("GetMaxTreeqPerFs").Return(10000, nil)

	nfs := nfsstorage{capacity: 100, cs: *suite.cs}
	service := TreeqService{cs: *suite.cs, nfsstorage: nfs}
	var capacity int64 = 1000
	pVName := "csi-TestTreeq"
	configMap := getCreateTreeqVolumeParameter()
	configMap[common.SC_FS_PREFIX] = "csit_"

	_, err := service.CreateTreeqVolume(configMap, capacity, pVName)
	assert.NotNil(suite.T(), err, "failed to get filecount")
}

func (suite *TreeqServiceSuite) Test_UpdateTreeqCnt_Success1() {
	fsID := 11
	expectedCnt := 10
	currentTreeqCnt := 9
	metadataResp := getMetadaResponse()
	suite.api.On("GetFilesystemTreeqCount", fsID).Return(currentTreeqCnt, nil)
	suite.iboxapi.On("PutMetadata", fsID, mock.Anything).Return(*metadataResp, nil)
	service := TreeqService{cs: *suite.cs}
	cnt, err := service.UpdateTreeqCnt(fsID, IncrementTreeqCount, 0)
	assert.Nil(suite.T(), err, "empty object")
	assert.Equal(suite.T(), expectedCnt, cnt, "treeq count shoude be same")
}

func (suite *TreeqServiceSuite) Test_UpdateTreeqCnt_Success2() {
	fsID := 11
	expectedCnt := 10
	currentTreeqCnt := 9
	metadataResp := getMetadaResponse()
	suite.iboxapi.On("PutMetadata", fsID, mock.Anything).Return(*metadataResp, nil)
	service := TreeqService{cs: *suite.cs}
	cnt, err := service.UpdateTreeqCnt(fsID, IncrementTreeqCount, currentTreeqCnt)
	assert.Nil(suite.T(), err, "empty object")
	assert.Equal(suite.T(), expectedCnt, cnt, "treeq count shoude be same")
}

func (suite *TreeqServiceSuite) Test_UpdateTreeqCnt_Error1() {
	fsID := 11
	currentTreeqCnt := 9
	expectedErr := errors.New("some error")
	suite.iboxapi.On("PutMetadata", fsID, mock.Anything).Return(nil, expectedErr)
	service := TreeqService{cs: *suite.cs}
	_, err := service.UpdateTreeqCnt(fsID, IncrementTreeqCount, currentTreeqCnt)
	assert.NotNil(suite.T(), err, "err should not be nil")
}

func (suite *TreeqServiceSuite) Test_UpdateTreeqCnt_Error2() {
	fsID := 11
	expectedErr := errors.New("some error")
	suite.api.On("GetFilesystemTreeqCount", fsID).Return(nil, expectedErr)
	service := TreeqService{cs: *suite.cs}
	_, err := service.UpdateTreeqCnt(fsID, IncrementTreeqCount, 0)
	assert.NotNil(suite.T(), err, "err should not be nil")
}

func (suite *TreeqServiceSuite) Test_DeleteTreeqVolume_GetTreeq_error() {
	fsID := 11
	treeqID := 10
	expectedErr := errors.New("TREEQ_ID_DOES_NOT_EXIST")
	suite.api.On("GetTreeq", fsID, treeqID).Return(nil, expectedErr)
	service := TreeqService{cs: *suite.cs}
	err := service.DeleteTreeqVolume(fsID, treeqID)
	assert.Nil(suite.T(), err, "empty object")
}

func (suite *TreeqServiceSuite) Test_DeleteTreeqVolume_GetTreeq_error2() {
	fsID := 11
	treeqID := 10
	expectedErr := errors.New("some other error")
	suite.api.On("GetTreeq", fsID, treeqID).Return(nil, expectedErr)
	service := TreeqService{cs: *suite.cs}
	err := service.DeleteTreeqVolume(fsID, treeqID)
	assert.NotNil(suite.T(), err, "empty object")
}

func (suite *TreeqServiceSuite) Test_DeleteTreeqVolume_GetTreeq_treeqNotEmpty() {
	fsID := 11
	treeqID := 10
	expectedResponse := getTreeQResponse(fsID)
	suite.api.On("GetTreeq", fsID, treeqID).Return(*expectedResponse, nil)
	service := TreeqService{cs: *suite.cs}
	err := service.DeleteTreeqVolume(fsID, treeqID)
	assert.NotNil(suite.T(), err, "empty object")
	assert.Equal(suite.T(), err.Error(), "can't delete NFS-treeq PV with data", "unexpected error message")
}

func (suite *TreeqServiceSuite) Test_DeleteTreeqVolume_TreeqCount_fail() {
	fsID := 11
	treeqID := 10
	expectedErr := errors.New("some other error")
	expectedResponse := getTreeQResponse(fsID)
	expectedResponse.UsedCapacity = 0
	suite.api.On("GetTreeq", fsID, treeqID).Return(*expectedResponse, nil)
	suite.api.On("GetFilesystemTreeqCount", fsID).Return(0, expectedErr)
	service := TreeqService{cs: *suite.cs}
	err := service.DeleteTreeqVolume(fsID, treeqID)
	assert.NotNil(suite.T(), err, "empty object")
}

func (suite *TreeqServiceSuite) Test_DeleteTreeqVolume_TreeqCount_fail2() {
	fsID := 11
	treeqID := 10
	expectedErr := errors.New("some other error")
	expectedResponse := getTreeQResponse(fsID)
	expectedResponse.UsedCapacity = 0
	suite.api.On("GetTreeq", fsID, treeqID).Return(*expectedResponse, nil)
	suite.api.On("GetFilesystemTreeqCount", fsID).Return(10, nil)
	suite.iboxapi.On("PutMetadata", fsID, mock.Anything).Return(nil, expectedErr)
	service := TreeqService{cs: *suite.cs}
	err := service.DeleteTreeqVolume(fsID, treeqID)
	assert.NotNil(suite.T(), err, "empty object")
}

func (suite *TreeqServiceSuite) Test_DeleteTreeqVolume_DeleteTreeq_success() {
	fsID := 11
	treeqID := 10
	expectedResponse := getTreeQResponse(fsID)
	expectedResponse.UsedCapacity = 0
	suite.api.On("GetTreeq", fsID, treeqID).Return(*expectedResponse, nil)
	suite.api.On("GetFilesystemTreeqCount", fsID).Return(10, nil)
	suite.iboxapi.On("PutMetadata", fsID, mock.Anything).Return(nil, nil)
	suite.api.On("DeleteTreeq", fsID, treeqID).Return(nil, nil)
	service := TreeqService{cs: *suite.cs}
	err := service.DeleteTreeqVolume(fsID, treeqID)
	assert.Nil(suite.T(), err, "empty object")
}

func (suite *TreeqServiceSuite) Test_DeleteTreeqVolume_DeleteTreeq_Error() {
	fsID := 11
	treeqID := 10
	expectedResponse := getTreeQResponse(fsID)
	expectedErr := errors.New("some other error")
	expectedResponse.UsedCapacity = 0
	suite.api.On("GetTreeq", fsID, treeqID).Return(*expectedResponse, nil)
	suite.api.On("GetFilesystemTreeqCount", fsID).Return(10, nil)
	suite.iboxapi.On("PutMetadata", fsID, mock.Anything).Return(nil, nil)
	suite.api.On("DeleteTreeq", fsID, treeqID).Return(nil, expectedErr)
	suite.api.On("GetFilesystemTreeqCount", mock.Anything).Return(nil, expectedErr)

	service := TreeqService{cs: *suite.cs}
	err := service.DeleteTreeqVolume(fsID, treeqID)
	assert.NotNil(suite.T(), err, "empty object")
}

func (suite *TreeqServiceSuite) Test_DeleteTreeqVolume_DeleteTreeq_errorToDeletefile() {
	fsID := 11
	treeqID := 10
	cnt := 1
	expectedErr := errors.New("some other error")
	expectedResponse := getTreeQResponse(fsID)
	expectedResponse.UsedCapacity = 0
	suite.api.On("GetTreeq", fsID, treeqID).Return(*expectedResponse, nil)
	suite.api.On("GetFilesystemTreeqCount", fsID).Return(cnt, nil)
	suite.iboxapi.On("PutMetadata", fsID, mock.Anything).Return(nil, nil)
	suite.api.On("DeleteTreeq", fsID, treeqID).Return(nil, nil)
	suite.api.On("DeleteFileSystemComplete", fsID).Return(expectedErr)
	service := TreeqService{cs: *suite.cs}
	err := service.DeleteTreeqVolume(fsID, treeqID)
	assert.NotNil(suite.T(), err, "empty object")
}

func (suite *TreeqServiceSuite) Test_UpdateTreeqVolume_GetFileSystemByID_error() {
	var filesytemID, treeqID = 100, 200
	var capacity int64 = common.BytesInOneGibibyte
	var maxSize string
	expectedErr := errors.New("FILESYSTEM_ID_DOES_NOT_EXIST")
	suite.iboxapi.On("GetFileSystemByID", filesytemID).Return(nil, expectedErr)
	service := TreeqService{cs: *suite.cs}
	err := service.UpdateTreeqVolume(filesytemID, treeqID, capacity, maxSize)
	assert.Equal(suite.T(), expectedErr, err, "Unexpected error")
}

func (suite *TreeqServiceSuite) Test_UpdateTreeqVolume_GetTreeqSizeByFileSystemID_error() {
	var filesytemID, treeqID = 100, 200
	var capacity int64 = common.BytesInOneGibibyte
	maxSize := "3gib"
	expectedFileSystemResponse := api.FileSystem{}
	expectedResponse := getTreeQResponse(filesytemID)
	expectedResponse.UsedCapacity = 0
	expectedErr := errors.New("FIALED_TO_GET_TREEQ_SIZE")
	suite.iboxapi.On("GetFileSystemByID", filesytemID).Return(expectedFileSystemResponse, nil)
	suite.api.On("GetTreeq", filesytemID, treeqID).Return(*expectedResponse, nil)
	suite.api.On("GetTreeqSizeByFileSystemID", filesytemID).Return(0, expectedErr)
	service := TreeqService{cs: *suite.cs}
	err := service.UpdateTreeqVolume(filesytemID, treeqID, capacity, maxSize)
	assert.Equal(suite.T(), expectedErr, err, "Unexpected error")
}

func (suite *TreeqServiceSuite) Test_UpdateTreeqVolume_GetTreeq_Not_found_error() {
	var filesytemID, treeqID = 100, 200
	var capacity int64 = common.BytesInOneGibibyte
	maxSize := "3gib"
	expectedFileSystemResponse := api.FileSystem{}
	expectedResponse := getTreeQResponse(filesytemID)
	expectedResponse.UsedCapacity = 0
	expectedErr := errors.New("TREEQ_ID_DOES_NOT_EXIST")
	suite.iboxapi.On("GetFileSystemByID", filesytemID).Return(expectedFileSystemResponse, nil)
	suite.api.On("GetTreeq", filesytemID, treeqID).Return(nil, expectedErr)
	service := TreeqService{cs: *suite.cs}
	err := service.UpdateTreeqVolume(filesytemID, treeqID, capacity, maxSize)
	assert.Nil(suite.T(), err, "Response not returned as expected")
}

func (suite *TreeqServiceSuite) Test_UpdateTreeqVolume_UpdateFilesystem_error() {
	var filesytemID, treeqID = 100, 200
	var capacity, treeqSize int64 = common.BytesInOneGibibyte, 200
	maxSize := "3gib"
	expectedFileSystemResponse := &iboxapi.FileSystem{}
	expectedResponse := getTreeQResponse(filesytemID)
	expectedResponse.UsedCapacity = 0
	expectedErr := errors.New("FIALED_TO_UPDATE_FILE")
	suite.iboxapi.On("GetFileSystemByID", filesytemID).Return(expectedFileSystemResponse, nil)
	suite.api.On("GetTreeq", filesytemID, treeqID).Return(*expectedResponse, nil)
	suite.api.On("GetTreeqSizeByFileSystemID", filesytemID).Return(treeqSize, nil)
	suite.api.On("UpdateFilesystem", filesytemID, mock.Anything).Return(nil, expectedErr)
	service := TreeqService{cs: *suite.cs}
	err := service.UpdateTreeqVolume(filesytemID, treeqID, capacity, maxSize)
	assert.Equal(suite.T(), expectedErr, err, "Response not returned as expected")
}

func (suite *TreeqServiceSuite) Test_UpdateTreeqVolume_UpdateTreeq_error() {
	var filesytemID, treeqID = 100, 200
	var capacity, treeqSize int64 = common.BytesInOneGibibyte, 200
	maxSize := "3gib"
	expectedFileSystemResponse := &iboxapi.FileSystem{}
	expectedResponse := getTreeQResponse(filesytemID)
	expectedResponse.UsedCapacity = 0
	body := map[string]interface{}{"hard_capacity": capacity}
	expectedErr := errors.New("FIALED_TO_UPDATE_TREEQ_SIZE")
	suite.iboxapi.On("GetFileSystemByID", filesytemID).Return(expectedFileSystemResponse, nil)
	suite.api.On("GetTreeq", filesytemID, treeqID).Return(*expectedResponse, nil)
	suite.api.On("GetTreeqSizeByFileSystemID", filesytemID).Return(treeqSize, nil)
	suite.api.On("UpdateFilesystem", filesytemID, mock.Anything).Return(expectedFileSystemResponse, nil)
	suite.api.On("UpdateTreeq", filesytemID, treeqID, body).Return(nil, expectedErr)
	service := TreeqService{cs: *suite.cs}
	err := service.UpdateTreeqVolume(filesytemID, treeqID, capacity, maxSize)
	assert.Equal(suite.T(), expectedErr, err, "Response not returned as expected")
}

func (suite *TreeqServiceSuite) Test_UpdateTreeqVolume_Success() {
	var filesytemID, treeqID = 100, 200
	var capacity, treeqSize int64 = common.BytesInOneGibibyte, 200
	maxSize := "3gib"
	expectedFileSystemResponse := &iboxapi.FileSystem{}
	expectedResponse := getTreeQResponse(filesytemID)
	expectedResponse.UsedCapacity = 0
	body := map[string]interface{}{"hard_capacity": capacity}
	poolResult := &iboxapi.PoolResult{ID: 100}
	suite.iboxapi.On("GetPoolByName", mock.Anything).Return(poolResult, nil)
	suite.iboxapi.On("GetFileSystemByID", filesytemID).Return(expectedFileSystemResponse, nil)
	suite.api.On("GetTreeq", filesytemID, treeqID).Return(*expectedResponse, nil)
	suite.api.On("GetTreeqSizeByFileSystemID", filesytemID).Return(treeqSize, nil)
	suite.api.On("UpdateFilesystem", filesytemID, mock.Anything).Return(expectedFileSystemResponse, nil)
	suite.api.On("UpdateTreeq", filesytemID, treeqID, body).Return(expectedResponse, nil)
	service := TreeqService{cs: *suite.cs}
	err := service.UpdateTreeqVolume(filesytemID, treeqID, capacity, maxSize)
	assert.Nil(suite.T(), err, "empty object")
}

func (suite *TreeqServiceSuite) Test_IsTreeqAlreadyExist_Error() {
	expectedErr := errors.New("some error")
	poolResult := &iboxapi.PoolResult{ID: 100}
	suite.iboxapi.On("GetPoolByName", mock.Anything).Return(poolResult, expectedErr)

	service := TreeqService{cs: *suite.cs}
	_, err := service.IsTreeqAlreadyExist("pool_name", "network_space", "pVName", "fsPrefix")
	assert.NotNil(suite.T(), err, "err should not be nil")
}

func (suite *TreeqServiceSuite) Test_IsTreeqAlreadyExist_StoragePoolIDByName_Error() {
	expectedErr := errors.New("some error")

	poolResult := &iboxapi.PoolResult{ID: 10}
	suite.iboxapi.On("GetPoolByName", mock.Anything).Return(poolResult, expectedErr)
	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkSpace(), nil)

	service := TreeqService{cs: *suite.cs}
	_, err := service.IsTreeqAlreadyExist("pool_name", "network_space", "pVName", "fsPrefix")
	assert.NotNil(suite.T(), err, "err should not be nil")
}

func (suite *TreeqServiceSuite) Test_IsTreeqAlreadyExist_FileSystemsByPoolID_Error() {
	poolID := 10

	expectedErr := errors.New("some error")
	poolResult := &iboxapi.PoolResult{ID: 10}
	suite.iboxapi.On("GetPoolByName", mock.Anything).Return(poolResult, nil)
	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkSpace(), nil)
	suite.iboxapi.On("GetFileSystemsByPool", poolID, mock.Anything).Return(nil, expectedErr)

	service := TreeqService{cs: *suite.cs}
	_, err := service.IsTreeqAlreadyExist("pool_name", "network_space", "pVName", "fsPrefix")
	assert.NotNil(suite.T(), err, "err should not be nil")
}

func (suite *TreeqServiceSuite) Test_IsTreeqAlreadyExist_GetExportByFileSystem_Error() {
	fsID := 0
	fsMetada := getfsMetadata2()

	poolResult := &iboxapi.PoolResult{ID: 10}
	suite.iboxapi.On("GetPoolByName", mock.Anything).Return(poolResult, nil)

	suite.iboxapi.On("GetFileSystemsByPool", mock.Anything, mock.Anything).Return(fsMetada, nil)
	suite.api.On("GetTreeqByName", mock.Anything, mock.Anything).Return(getTreeQResponse(fsID), nil)

	exportResp := getExportResponse()
	suite.iboxapi.On("GetExportsByFileSystemID", fsID).Return(exportResp, nil)
	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkSpace(), nil)

	service := TreeqService{cs: *suite.cs}
	_, err := service.IsTreeqAlreadyExist("pool_name", "network_space", "pVName", "fsPrefix")
	assert.Nil(suite.T(), err, "err should not be nil")
}

// test case data generation

func getExportResponse() []iboxapi.Export {
	ex := iboxapi.Export{}
	exportRespArry := []iboxapi.Export{}
	exportRespArry = append(exportRespArry, ex)
	return exportRespArry
}

func getMetadaResponse() *[]api.Metadata {
	metadataArry := []api.Metadata{}
	return &metadataArry
}

func getTreeQResponse(fileSysID int) *api.Treeq {
	treeq := api.Treeq{
		FilesystemID: fileSysID,
		HardCapacity: 1000,
		ID:           1,
		Name:         "csi-TestTreeq",
		Path:         "/csi-TestTreeq",
		UsedCapacity: 112345,
	}
	return &treeq
}

func getCreateVolumeRequest() *csi.CreateVolumeRequest {
	parameters := map[string]string{
		common.SC_POOL_NAME:         "a_pool",
		common.SC_MAX_VOLS_PER_HOST: "30",
		"max_filesystems":           "20",
		"max_treeqs_per_filesystem": "21",
		common.SC_NETWORK_SPACE:     "nas",
		common.SC_STORAGE_PROTOCOL:  common.PROTOCOL_TREEQ,
	}
	req := csi.CreateVolumeRequest{
		CapacityRange: &csi.CapacityRange{RequiredBytes: 100 * gib},
		Parameters:    parameters,
	}
	return &req
}

func getfsMetadata() []iboxapi.FileSystem {
	fs := iboxapi.FileSystem{
		ID:   10,
		Size: common.BytesInOneGibibyte,
	}
	fsArry := []iboxapi.FileSystem{}
	fsArry = append(fsArry, fs)

	return fsArry
}

func getfsMetadata2() []iboxapi.FileSystem {
	fs := iboxapi.FileSystem{
		ID:   11,
		Size: 10000,
	}
	fsArry := []iboxapi.FileSystem{}
	fsArry = append(fsArry, fs)

	return fsArry
}

func getCreateTreeqVolumeParameter() map[string]string {
	return map[string]string{
		common.SC_POOL_NAME:              "pool_name1",
		common.SC_NETWORK_SPACE:          "network_space1",
		common.SC_NFS_EXPORT_PERMISSIONS: "[{'access':'RW','client':'192.168.147.190-192.168.147.199','no_root_squash':false},{'access':'RW','client':'192.168.147.10-192.168.147.20','no_root_squash':'false'}]",
	}
}

func getTreeQTestNetworkSpace() api.NetworkSpace {
	var nws api.NetworkSpace
	nws.Name = "testNWS"
	nws.Service = common.NS_NFS_SVC

	return nws
}
