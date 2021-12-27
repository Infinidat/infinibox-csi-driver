/*Copyright 2020 Infinidat
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
	"errors"
	"fmt"
	"infinibox-csi-driver/api"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

func (suite *FileSystemServiceSuite) SetupTest() {
	suite.api = new(api.MockApiService)
	suite.cs = &commonservice{api: suite.api}
}

type FileSystemServiceSuite struct {
	suite.Suite
	api *api.MockApiService
	cs  *commonservice
}

func TestFileSystemServiceSuite(t *testing.T) {
	suite.Run(t, new(FileSystemServiceSuite))
}

func (suite *FileSystemServiceSuite) Test_getExpectedFileSystemID_maxfilesystem() {
	expectedErr := errors.New("some error")
	suite.api.On("GetStoragePoolIDByName", mock.Anything).Return(0, expectedErr)
	service := getFilesystemService(NFSTREEQ, *suite.cs)
	service.capacity = 209951162777600
	_, err := service.getExpectedFileSystemID(1000)
	assert.NotNil(suite.T(), err, "empty object")
}

func (suite *FileSystemServiceSuite) Test_getExpectedFileSystemID_getMaxSize_error() {
	var poolID int64 = 10
	suite.api.On("GetStoragePoolIDByName", mock.Anything).Return(poolID, nil)
	service := FilesystemService{cs: *suite.cs}
	configmap := make(map[string]string)
	configmap[MAXFILESYSTEMSIZE] = "4mib"
	service.configmap = configmap
	service.capacity = 209951162777600
	_, err := service.getExpectedFileSystemID(10)
	fmt.Println(err)
	assert.NotNil(suite.T(), err, "empty object")
}

func (suite *FileSystemServiceSuite) Test_getExpectedFileSystemID_FileSystemByPoolID_error() {
	expectedErr := errors.New("some error")
	var poolID int64 = 10
	suite.api.On("GetStoragePoolIDByName", mock.Anything).Return(poolID, nil)
	suite.api.On("GetFileSystemsByPoolID", mock.Anything, 1).Return(nil, expectedErr)
	service := FilesystemService{cs: *suite.cs}
	_, err := service.getExpectedFileSystemID(1000)
	assert.NotNil(suite.T(), err, "empty object")
}

func (suite *FileSystemServiceSuite) Test_getExpectedFileSystemID_FilesytemTreeqCount_error() {
	expectedErr := errors.New("some error")
	fsMetada := getfsMetadata2()
	var poolID int64 = 10
	// var fsID int64 = 11
	suite.api.On("GetStoragePoolIDByName", mock.Anything).Return(poolID, nil)
	suite.api.On("GetFileSystemsByPoolID", mock.Anything, 1).Return(*fsMetada, nil)
	suite.api.On("GetFilesytemTreeqCount", mock.Anything).Return(0, expectedErr)
	service := FilesystemService{cs: *suite.cs, capacity: 100}
	_, err := service.getExpectedFileSystemID(9999990)
	assert.NotNil(suite.T(), err, "empty object")
}

func (suite *FileSystemServiceSuite) Test_getExpectedFileSystemID_Success() {
	fsMetada := getfsMetadata()
	var poolID int64 = 10
	var fsID int64 = 10
	suite.api.On("GetStoragePoolIDByName", mock.Anything).Return(poolID, nil)
	suite.api.On("GetFileSystemsByPoolID", mock.Anything, mock.Anything).Return(*fsMetada, nil)
	suite.api.On("GetFilesytemTreeqCount", mock.Anything).Return(1, nil)

	exportResp := getExportResponse()
	suite.api.On("GetExportByFileSystem", fsID).Return(exportResp, nil)
	fsMetada2 := getfsMetadata2()
	suite.api.On("GetFileSystemsByPoolID", poolID, 2).Return(*fsMetada2, nil)
	service := FilesystemService{cs: *suite.cs}

	service.capacity = 1000
	service.exportpath = "/exportPath"

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

func (suite *FileSystemServiceSuite) Test_CreateTreeqVolume_Success() {
	fsMetada := getfsMetadata2()
	var poolID int64 = 10
	var fsID int64 = 11

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getnetworkspace(), nil)
	suite.api.On("GetStoragePoolIDByName", mock.Anything).Return(poolID, nil)
	suite.api.On("GetFileSystemsByPoolID", poolID, 1).Return(*fsMetada, nil)
	suite.api.On("GetFilesytemTreeqCount", fsID).Return(1, nil)

	exportResp := getExportResponse()
	suite.api.On("GetExportByFileSystem", fsID).Return(exportResp, nil)

	treeqResp := getTreeQResponse(fsID)
	suite.api.On("CreateTreeq", fsID, mock.Anything).Return(*treeqResp, nil)

	metadataResp := getMetadaResponse()
	suite.api.On("AttachMetadataToObject", fsID, mock.Anything).Return(*metadataResp, nil)

	suite.api.On("UpdateFilesystem", fsID, mock.Anything).Return(nil, nil)
	service := FilesystemService{cs: *suite.cs}

	// CreateVolumeRequest parameter values to filesystemService
	var capacity int64 = 1000
	pVName := "csi-TestTreeq"
	configMap := getCreateTreeqVolumeParameter()

	_, err := service.CreateTreeqVolume(configMap, capacity, pVName)
	assert.Nil(suite.T(), err, "empty object")
}

func (suite *FileSystemServiceSuite) Test_CreateTreeqVolume_FileSystemCount_Error() {
	var fsMetada api.FSMetadata
	var poolID int64 = 10
	//	var fsID int64 = 11
	expectedErr := errors.New("some error")

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getnetworkspace(), nil)
	suite.api.On("GetStoragePoolIDByName", mock.Anything).Return(poolID, nil)
	suite.api.On("GetFileSystemsByPoolID", poolID, 1).Return(fsMetada, nil)
	suite.api.On("GetFileSystemCountByPoolID", mock.Anything).Return(0, expectedErr)

	service := FilesystemService{cs: *suite.cs}
	// CreateVolumeRequest parameter values to filesystemService
	var capacity int64 = 1000
	pVName := "csi-TestTreeq"
	configMap := make(map[string]string)
	configMap["network_space"] = "networkspace"
	_, err := service.CreateTreeqVolume(configMap, capacity, pVName)
	assert.NotNil(suite.T(), err, "failed to get filecount")
}

func (suite *FileSystemServiceSuite) Test_CreateTreeqVolume_FileSystemCount_notAllowed() {
	var fsMetada api.FSMetadata
	var poolID int64 = 10

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getnetworkspace(), nil)
	suite.api.On("GetStoragePoolIDByName", mock.Anything).Return(poolID, nil)
	suite.api.On("GetFileSystemsByPoolID", poolID, 1).Return(fsMetada, nil)
	suite.api.On("GetFileSystemCountByPoolID", mock.Anything).Return(20000, nil)

	service := FilesystemService{cs: *suite.cs}
	// CreateVolumeRequest parameter values to filesystemService
	var capacity int64 = 1000
	pVName := "csi-TestTreeq"
	configMap := getCreateTreeqVolumeParameter()

	_, err := service.CreateTreeqVolume(configMap, capacity, pVName)
	assert.NotNil(suite.T(), err, "failed to get filecount")
}

func (suite *FileSystemServiceSuite) Test_CreateTreeqVolume_CreateFilesystem_Error() {
	var fsMetada api.FSMetadata
	var poolID int64 = 10
	expectedErr := errors.New("some error")

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getnetworkspace(), nil)
	suite.api.On("GetStoragePoolIDByName", mock.Anything).Return(poolID, nil)
	suite.api.On("GetFileSystemsByPoolID", poolID, 1).Return(fsMetada, nil)
	suite.api.On("GetFileSystemCountByPoolID", mock.Anything).Return(200, nil)
	suite.api.On("CreateFilesystem", mock.Anything).Return(nil, expectedErr)

	service := FilesystemService{cs: *suite.cs}
	// CreateVolumeRequest parameter values to filesystemService
	var capacity int64 = 1000
	pVName := "csi-TestTreeq"
	configMap := getCreateTreeqVolumeParameter()
	configMap["fs_prefix"] = "csit_"

	_, err := service.CreateTreeqVolume(configMap, capacity, pVName)
	assert.NotNil(suite.T(), err, "failed to get filecount")
}

func (suite *FileSystemServiceSuite) Test_CreateTreeqVolume_ExportFileSystem_Error() {
	var fsMetada api.FSMetadata
	var poolID int64 = 10
	expectedErr := errors.New("some error")

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getnetworkspace(), nil)
	suite.api.On("GetStoragePoolIDByName", mock.Anything).Return(poolID, nil)
	suite.api.On("GetFileSystemsByPoolID", poolID, 1).Return(fsMetada, nil)
	suite.api.On("GetFileSystemCountByPoolID", mock.Anything).Return(200, nil)
	suite.api.On("CreateFilesystem", mock.Anything).Return(getFileSystem, nil)
	suite.api.On("ExportFileSystem", mock.Anything).Return(nil, expectedErr)

	service := FilesystemService{cs: *suite.cs}
	// CreateVolumeRequest parameter values to filesystemService
	var capacity int64 = 1000
	pVName := "csi-TestTreeq"
	configMap := getCreateTreeqVolumeParameter()
	configMap["fs_prefix"] = "csit_"

	_, err := service.CreateTreeqVolume(configMap, capacity, pVName)
	assert.NotNil(suite.T(), err, "failed to get filecount")
}

func (suite *FileSystemServiceSuite) Test_CreateTreeqVolume_metadata_Error() {
	var fsMetada api.FSMetadata
	var poolID int64 = 10
	expectedErr := errors.New("some error")

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getnetworkspace(), nil)
	suite.api.On("GetStoragePoolIDByName", mock.Anything).Return(poolID, nil)
	suite.api.On("GetFileSystemsByPoolID", poolID, 1).Return(fsMetada, nil)
	suite.api.On("GetFileSystemCountByPoolID", mock.Anything).Return(200, nil)
	suite.api.On("CreateFilesystem", mock.Anything).Return(getFileSystem, nil)
	suite.api.On("ExportFileSystem", mock.Anything).Return(getExportResponse(), nil)
	suite.api.On("AttachMetadataToObject", mock.Anything, mock.Anything).Return(nil, expectedErr)

	service := FilesystemService{cs: *suite.cs}
	// CreateVolumeRequest parameter values to filesystemService
	var capacity int64 = 1000
	pVName := "csi-TestTreeq"
	configMap := getCreateTreeqVolumeParameter()
	configMap["fs_prefix"] = "csit_"

	_, err := service.CreateTreeqVolume(configMap, capacity, pVName)
	assert.NotNil(suite.T(), err, "failed to get filecount")
}

func (suite *FileSystemServiceSuite) Test_CreateTreeqVolume_CreateTreeq_Error() {
	var fsMetada api.FSMetadata
	var poolID int64 = 10
	expectedErr := errors.New("some error")

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getnetworkspace(), nil)
	suite.api.On("GetStoragePoolIDByName", mock.Anything).Return(poolID, nil)
	suite.api.On("GetFileSystemsByPoolID", poolID, 1).Return(fsMetada, nil)
	suite.api.On("GetFileSystemCountByPoolID", mock.Anything).Return(200, nil)
	suite.api.On("CreateFilesystem", mock.Anything).Return(getFileSystem, nil)
	suite.api.On("ExportFileSystem", mock.Anything).Return(getExportResponse(), nil)
	suite.api.On("AttachMetadataToObject", mock.Anything, mock.Anything).Return(nil, nil)
	suite.api.On("CreateTreeq", mock.Anything, mock.Anything).Return(nil, expectedErr)
	suite.api.On("DeleteFileSystemComplete", mock.Anything, mock.Anything).Return(expectedErr)

	service := FilesystemService{cs: *suite.cs}
	// CreateVolumeRequest parameter values to filesystemService
	var capacity int64 = 1000
	pVName := "csi-TestTreeq"
	configMap := getCreateTreeqVolumeParameter()
	configMap["fs_prefix"] = "csit_"

	_, err := service.CreateTreeqVolume(configMap, capacity, pVName)
	assert.NotNil(suite.T(), err, "failed to get filecount")
}

func (suite *FileSystemServiceSuite) Test_UpdateTreeqCnt_Success1() {
	var fsID int64 = 11
	expectedCnt := 10
	currentTreeqCnt := 9
	metadataResp := getMetadaResponse()
	suite.api.On("GetFilesytemTreeqCount", fsID).Return(currentTreeqCnt, nil)
	suite.api.On("AttachMetadataToObject", fsID, mock.Anything).Return(*metadataResp, nil)
	service := FilesystemService{cs: *suite.cs}
	cnt, err := service.UpdateTreeqCnt(fsID, IncrementTreeqCount, 0)
	assert.Nil(suite.T(), err, "empty object")
	assert.Equal(suite.T(), expectedCnt, cnt, "treeq count shoude be same")
}

func (suite *FileSystemServiceSuite) Test_UpdateTreeqCnt_Success2() {
	var fsID int64 = 11
	expectedCnt := 10
	currentTreeqCnt := 9
	metadataResp := getMetadaResponse()
	suite.api.On("AttachMetadataToObject", fsID, mock.Anything).Return(*metadataResp, nil)
	service := FilesystemService{cs: *suite.cs}
	cnt, err := service.UpdateTreeqCnt(fsID, IncrementTreeqCount, currentTreeqCnt)
	assert.Nil(suite.T(), err, "empty object")
	assert.Equal(suite.T(), expectedCnt, cnt, "treeq count shoude be same")
}

func (suite *FileSystemServiceSuite) Test_UpdateTreeqCnt_Error1() {
	var fsID int64 = 11
	// expectedCnt := 10
	currentTreeqCnt := 9
	expectedErr := errors.New("some error")
	suite.api.On("AttachMetadataToObject", fsID, mock.Anything).Return(nil, expectedErr)
	service := FilesystemService{cs: *suite.cs}
	_, err := service.UpdateTreeqCnt(fsID, IncrementTreeqCount, currentTreeqCnt)
	assert.NotNil(suite.T(), err, "err should not be nil")
}

func (suite *FileSystemServiceSuite) Test_UpdateTreeqCnt_Error2() {
	var fsID int64 = 11
	// expectedCnt := 10
	// currentTreeqCnt := 9
	expectedErr := errors.New("some error")
	suite.api.On("GetFilesytemTreeqCount", fsID).Return(nil, expectedErr)
	service := FilesystemService{cs: *suite.cs}
	_, err := service.UpdateTreeqCnt(fsID, IncrementTreeqCount, 0)
	assert.NotNil(suite.T(), err, "err should not be nil")
}

func (suite *FileSystemServiceSuite) Test_DeleteTreeqVolume_GetTreeq_error() {
	var fsID int64 = 11
	var treeqID int64 = 10
	expectedErr := errors.New("TREEQ_ID_DOES_NOT_EXIST")
	suite.api.On("GetTreeq", fsID, treeqID).Return(nil, expectedErr)
	service := FilesystemService{cs: *suite.cs}
	err := service.DeleteTreeqVolume(fsID, treeqID)
	assert.Nil(suite.T(), err, "empty object")
}

func (suite *FileSystemServiceSuite) Test_DeleteTreeqVolume_GetTreeq_error2() {
	var fsID int64 = 11
	var treeqID int64 = 10
	expectedErr := errors.New("some other error")
	suite.api.On("GetTreeq", fsID, treeqID).Return(nil, expectedErr)
	service := FilesystemService{cs: *suite.cs}
	err := service.DeleteTreeqVolume(fsID, treeqID)
	assert.NotNil(suite.T(), err, "empty object")
}

func (suite *FileSystemServiceSuite) Test_DeleteTreeqVolume_GetTreeq_treeqNotEmpty() {
	var fsID int64 = 11
	var treeqID int64 = 10
	expectedResponse := getTreeQResponse(fsID)
	suite.api.On("GetTreeq", fsID, treeqID).Return(*expectedResponse, nil)
	service := FilesystemService{cs: *suite.cs}
	err := service.DeleteTreeqVolume(fsID, treeqID)
	assert.NotNil(suite.T(), err, "empty object")
	assert.Equal(suite.T(), err.Error(), "can't delete NFS-treeq PV with data", "unexpected error message")
}

func (suite *FileSystemServiceSuite) Test_DeleteTreeqVolume_TreeqCount_fail() {
	var fsID int64 = 11
	var treeqID int64 = 10
	expectedErr := errors.New("some other error")
	expectedResponse := getTreeQResponse(fsID)
	expectedResponse.UsedCapacity = 0
	suite.api.On("GetTreeq", fsID, treeqID).Return(*expectedResponse, nil)
	suite.api.On("GetFilesytemTreeqCount", fsID).Return(0, expectedErr)
	service := FilesystemService{cs: *suite.cs}
	err := service.DeleteTreeqVolume(fsID, treeqID)
	assert.NotNil(suite.T(), err, "empty object")
}

func (suite *FileSystemServiceSuite) Test_DeleteTreeqVolume_TreeqCount_fail2() {
	var fsID int64 = 11
	var treeqID int64 = 10
	expectedErr := errors.New("some other error")
	expectedResponse := getTreeQResponse(fsID)
	expectedResponse.UsedCapacity = 0
	suite.api.On("GetTreeq", fsID, treeqID).Return(*expectedResponse, nil)
	suite.api.On("GetFilesytemTreeqCount", fsID).Return(10, nil)
	suite.api.On("AttachMetadataToObject", fsID, mock.Anything).Return(nil, expectedErr)
	service := FilesystemService{cs: *suite.cs}
	err := service.DeleteTreeqVolume(fsID, treeqID)
	assert.NotNil(suite.T(), err, "empty object")
}

func (suite *FileSystemServiceSuite) Test_DeleteTreeqVolume_DeleteTreeq_success() {
	var fsID int64 = 11
	var treeqID int64 = 10
	expectedResponse := getTreeQResponse(fsID)
	expectedResponse.UsedCapacity = 0
	suite.api.On("GetTreeq", fsID, treeqID).Return(*expectedResponse, nil)
	suite.api.On("GetFilesytemTreeqCount", fsID).Return(10, nil)
	suite.api.On("AttachMetadataToObject", fsID, mock.Anything).Return(nil, nil)
	suite.api.On("DeleteTreeq", fsID, treeqID).Return(nil, nil)
	service := FilesystemService{cs: *suite.cs}
	err := service.DeleteTreeqVolume(fsID, treeqID)
	assert.Nil(suite.T(), err, "empty object")
}

func (suite *FileSystemServiceSuite) Test_DeleteTreeqVolume_DeleteTreeq_Error() {
	var fsID int64 = 11
	var treeqID int64 = 10
	expectedResponse := getTreeQResponse(fsID)
	expectedErr := errors.New("some other error")
	expectedResponse.UsedCapacity = 0
	suite.api.On("GetTreeq", fsID, treeqID).Return(*expectedResponse, nil)
	suite.api.On("GetFilesytemTreeqCount", fsID).Return(10, nil)
	suite.api.On("AttachMetadataToObject", fsID, mock.Anything).Return(nil, nil)
	suite.api.On("DeleteTreeq", fsID, treeqID).Return(nil, expectedErr)
	suite.api.On("GetFilesytemTreeqCount", mock.Anything).Return(nil, expectedErr)

	service := FilesystemService{cs: *suite.cs}
	err := service.DeleteTreeqVolume(fsID, treeqID)
	assert.NotNil(suite.T(), err, "empty object")
}

func (suite *FileSystemServiceSuite) Test_DeleteTreeqVolume_DeleteTreeq_errorToDeletefile() {
	var fsID int64 = 11
	var treeqID int64 = 10
	cnt := 1
	expectedErr := errors.New("some other error")
	expectedResponse := getTreeQResponse(fsID)
	expectedResponse.UsedCapacity = 0
	suite.api.On("GetTreeq", fsID, treeqID).Return(*expectedResponse, nil)
	suite.api.On("GetFilesytemTreeqCount", fsID).Return(cnt, nil)
	suite.api.On("AttachMetadataToObject", fsID, mock.Anything).Return(nil, nil)
	suite.api.On("DeleteTreeq", fsID, treeqID).Return(nil, nil)
	suite.api.On("DeleteFileSystemComplete", fsID, treeqID).Return(nil, expectedErr)
	service := FilesystemService{cs: *suite.cs}
	err := service.DeleteTreeqVolume(fsID, treeqID)
	assert.NotNil(suite.T(), err, "empty object")
}

func (suite *FileSystemServiceSuite) Test_UpdateTreeqVolume_GetFileSystemByID_error() {
	var filesytemID, treeqID, capacity int64 = 100, 200, 1073741824
	maxSize := ""
	expectedErr := errors.New("FILESYSTEM_ID_DOES_NOT_EXIST")
	suite.api.On("GetFileSystemByID", filesytemID).Return(nil, expectedErr)
	service := FilesystemService{cs: *suite.cs}
	err := service.UpdateTreeqVolume(filesytemID, treeqID, capacity, maxSize)
	// assert.Nil(suite.T(), err, "empty object")
	assert.Equal(suite.T(), expectedErr, err, "Unexpected error")
}

func (suite *FileSystemServiceSuite) Test_UpdateTreeqVolume_GetTreeqSizeByFileSystemID_error() {
	var filesytemID, treeqID, capacity int64 = 100, 200, 1073741824
	maxSize := "3gib"
	expectedFileSystemResponse := api.FileSystem{}
	expectedResponse := getTreeQResponse(filesytemID)
	expectedResponse.UsedCapacity = 0
	expectedErr := errors.New("FIALED_TO_GET_TREEQ_SIZE")
	suite.api.On("GetFileSystemByID", filesytemID).Return(expectedFileSystemResponse, nil)
	suite.api.On("GetTreeq", filesytemID, treeqID).Return(*expectedResponse, nil)
	suite.api.On("GetTreeqSizeByFileSystemID", filesytemID).Return(0, expectedErr)
	service := FilesystemService{cs: *suite.cs}
	err := service.UpdateTreeqVolume(filesytemID, treeqID, capacity, maxSize)
	assert.Equal(suite.T(), expectedErr, err, "Unexpected error")
}

func (suite *FileSystemServiceSuite) Test_UpdateTreeqVolume_GetTreeq_Not_found_error() {
	var filesytemID, treeqID, capacity int64 = 100, 200, 1073741824
	maxSize := "3gib"
	expectedFileSystemResponse := api.FileSystem{}
	expectedResponse := getTreeQResponse(filesytemID)
	expectedResponse.UsedCapacity = 0
	expectedErr := errors.New("TREEQ_ID_DOES_NOT_EXIST")
	suite.api.On("GetFileSystemByID", filesytemID).Return(expectedFileSystemResponse, nil)
	suite.api.On("GetTreeq", filesytemID, treeqID).Return(nil, expectedErr)
	service := FilesystemService{cs: *suite.cs}
	err := service.UpdateTreeqVolume(filesytemID, treeqID, capacity, maxSize)
	assert.Nil(suite.T(), err, "Response not returned as expected")
}

func (suite *FileSystemServiceSuite) Test_UpdateTreeqVolume_UpdateFilesystem_error() {
	var filesytemID, treeqID, capacity, treeqSize int64 = 100, 200, 1073741824, 200
	maxSize := "3gib"
	expectedFileSystemResponse := api.FileSystem{}
	expectedResponse := getTreeQResponse(filesytemID)
	expectedResponse.UsedCapacity = 0
	expectedErr := errors.New("FIALED_TO_UPDATE_FILE")
	suite.api.On("GetFileSystemByID", filesytemID).Return(expectedFileSystemResponse, nil)
	suite.api.On("GetTreeq", filesytemID, treeqID).Return(*expectedResponse, nil)
	suite.api.On("GetTreeqSizeByFileSystemID", filesytemID).Return(treeqSize, nil)
	suite.api.On("UpdateFilesystem", filesytemID, mock.Anything).Return(nil, expectedErr)
	service := FilesystemService{cs: *suite.cs}
	err := service.UpdateTreeqVolume(filesytemID, treeqID, capacity, maxSize)
	assert.Equal(suite.T(), expectedErr, err, "Response not returned as expected")
}

func (suite *FileSystemServiceSuite) Test_UpdateTreeqVolume_UpdateTreeq_error() {
	var filesytemID, treeqID, capacity, treeqSize int64 = 100, 200, 1073741824, 200
	maxSize := "3gib"
	expectedFileSystemResponse := api.FileSystem{}
	expectedResponse := getTreeQResponse(filesytemID)
	expectedResponse.UsedCapacity = 0
	body := map[string]interface{}{"hard_capacity": capacity}
	expectedErr := errors.New("FIALED_TO_UPDATE_TREEQ_SIZE")
	suite.api.On("GetFileSystemByID", filesytemID).Return(expectedFileSystemResponse, nil)
	suite.api.On("GetTreeq", filesytemID, treeqID).Return(*expectedResponse, nil)
	suite.api.On("GetTreeqSizeByFileSystemID", filesytemID).Return(treeqSize, nil)
	suite.api.On("UpdateFilesystem", filesytemID, mock.Anything).Return(expectedFileSystemResponse, nil)
	suite.api.On("UpdateTreeq", filesytemID, treeqID, body).Return(nil, expectedErr)
	service := FilesystemService{cs: *suite.cs}
	err := service.UpdateTreeqVolume(filesytemID, treeqID, capacity, maxSize)
	assert.Equal(suite.T(), expectedErr, err, "Response not returned as expected")
}

func (suite *FileSystemServiceSuite) Test_UpdateTreeqVolume_Success() {
	var filesytemID, treeqID, capacity, treeqSize int64 = 100, 200, 1073741824, 200
	maxSize := "3gib"
	expectedFileSystemResponse := api.FileSystem{}
	expectedResponse := getTreeQResponse(filesytemID)
	expectedResponse.UsedCapacity = 0
	body := map[string]interface{}{"hard_capacity": capacity}
	suite.api.On("GetFileSystemByID", filesytemID).Return(expectedFileSystemResponse, nil)
	suite.api.On("GetTreeq", filesytemID, treeqID).Return(*expectedResponse, nil)
	suite.api.On("GetTreeqSizeByFileSystemID", filesytemID).Return(treeqSize, nil)
	suite.api.On("UpdateFilesystem", filesytemID, mock.Anything).Return(expectedFileSystemResponse, nil)
	suite.api.On("UpdateTreeq", filesytemID, treeqID, body).Return(expectedResponse, nil)
	service := FilesystemService{cs: *suite.cs}
	err := service.UpdateTreeqVolume(filesytemID, treeqID, capacity, maxSize)
	assert.Nil(suite.T(), err, "empty object")
}

func (suite *FileSystemServiceSuite) Test_IsTreeqAlreadyExist_Error() {
	var poolID int64 = 10
	// var fsID int64 = 11

	expectedErr := errors.New("some error")

	// suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(nil, expectedErr)
	suite.api.On("GetStoragePoolIDByName", mock.Anything).Return(poolID, expectedErr)

	service := FilesystemService{cs: *suite.cs}
	_, err := service.IsTreeqAlreadyExist("pool_name", "network_space", "pVName")
	assert.NotNil(suite.T(), err, "err should not be nil")
}

func (suite *FileSystemServiceSuite) Test_IsTreeqAlreadyExist_StoragePoolIDByName_Error() {
	// var poolID int64 = 10
	// var fsID int64 = 11

	expectedErr := errors.New("some error")

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkSpace(), nil)
	suite.api.On("GetStoragePoolIDByName", mock.Anything).Return(0, expectedErr)

	service := FilesystemService{cs: *suite.cs}
	_, err := service.IsTreeqAlreadyExist("pool_name", "network_space", "pVName")
	assert.NotNil(suite.T(), err, "err should not be nil")
}

func (suite *FileSystemServiceSuite) Test_IsTreeqAlreadyExist_FileSystemsByPoolID_Error() {
	var poolID int64 = 10
	// var fsID int64 = 11

	expectedErr := errors.New("some error")
	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkSpace(), nil)
	suite.api.On("GetStoragePoolIDByName", mock.Anything).Return(poolID, nil)
	suite.api.On("GetFileSystemsByPoolID", poolID, 1).Return(nil, expectedErr)

	service := FilesystemService{cs: *suite.cs}
	_, err := service.IsTreeqAlreadyExist("pool_name", "network_space", "pVName")
	assert.NotNil(suite.T(), err, "err should not be nil")
}

func (suite *FileSystemServiceSuite) Test_IsTreeqAlreadyExist_GetExportByFileSystem_Error() {
	var poolID int64 = 10
	var fsID int64 = 0
	fsMetada := getfsMetadata2()

	// expectedErr := errors.New("some error")
	suite.api.On("GetStoragePoolIDByName", mock.Anything).Return(poolID, nil)

	suite.api.On("GetFileSystemsByPoolID", poolID, 1).Return(*fsMetada, nil)
	suite.api.On("GetTreeqByName", mock.Anything, mock.Anything).Return(getTreeQResponse(fsID), nil)

	exportResp := getExportResponse()
	suite.api.On("GetExportByFileSystem", fsID).Return(exportResp, nil)
	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkSpace(), nil)

	service := FilesystemService{cs: *suite.cs}
	_, err := service.IsTreeqAlreadyExist("pool_name", "network_space", "pVName")
	assert.Nil(suite.T(), err, "err should not be nil")
}

//*****Test case Data Generation

func getExportResponse() *[]api.ExportResponse {
	exportRespArry := []api.ExportResponse{}

	exportResp := api.ExportResponse{}
	exportResp.ExportPath = "/exportPath"
	return &exportRespArry
}

func getMetadaResponse() *[]api.Metadata {
	metadataArry := []api.Metadata{}
	return &metadataArry
}

func getTreeQResponse(fileSysID int64) *api.Treeq {
	var treeq api.Treeq
	treeq.FilesystemID = fileSysID
	treeq.HardCapacity = 1000
	treeq.ID = 1
	treeq.Name = "csi-TestTreeq"
	treeq.Path = "/csi-TestTreeq"
	treeq.UsedCapacity = 112345
	return &treeq
}

func getCreateVolumeRequest() *csi.CreateVolumeRequest {
	parameters := map[string]string{
		"pool_name":                 "a_pool",
		"max_filesystem_size":       "30gib",
		"max_filesystems":           "20",
		"max_treeqs_per_filesystem": "21",
		"network_space":             "nas",
	}
	req := csi.CreateVolumeRequest{
		CapacityRange: &csi.CapacityRange{RequiredBytes: 100 * gib},
		Parameters:    parameters,
	}
	return &req
}

func getfsMetadata() *api.FSMetadata {
	var fsMetadata api.FSMetadata
	fs := api.FileSystem{}
	fs.ID = 10
	fs.Size = 1073741824
	fsArry := []api.FileSystem{}
	fsArry = append(fsArry, fs)
	fsMeta := api.FileSystemMetaData{}
	fsMeta.NumberOfObjects = 1
	fsMeta.Page = 1
	fsMeta.PageSize = 50
	fsMeta.PagesTotal = 2
	fsMeta.Ready = true

	fsMetadata.Filemetadata = fsMeta
	fsMetadata.FileSystemArry = fsArry
	return &fsMetadata
}

func getfsMetadata2() *api.FSMetadata {
	var fsMetadata api.FSMetadata
	fs := api.FileSystem{}
	fs.ID = 11
	fs.Size = 10000
	fsArry := []api.FileSystem{}
	fsArry = append(fsArry, fs)
	fsMeta := api.FileSystemMetaData{}
	fsMeta.NumberOfObjects = 1
	fsMeta.Page = 2
	fsMeta.PageSize = 50
	fsMeta.PagesTotal = 2
	fsMeta.Ready = true

	fsMetadata.Filemetadata = fsMeta
	fsMetadata.FileSystemArry = fsArry
	return &fsMetadata
}

func getCreateTreeqVolumeParameter() map[string]string {
	return map[string]string{
		"pool_name":              "pool_name1",
		"network_space":          "network_space1",
		"nfs_export_permissions": "[{'access':'RW','client':'192.168.147.190-192.168.147.199','no_root_squash':false},{'access':'RW','client':'192.168.147.10-192.168.147.20','no_root_squash':'false'}]",
	}
}
