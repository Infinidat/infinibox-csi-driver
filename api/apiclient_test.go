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
package api

import (
	"errors"
	"infinibox-csi-driver/api/client"
	"infinibox-csi-driver/iboxapi"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

func (suite *ApiTestSuite) SetupTest() {
	suite.clientMock = new(MockApiClient)
	suite.iboxapi = new(iboxapi.MockApiClient)
	suite.serviceMock = new(MockApiService)
}

type ApiTestSuite struct {
	suite.Suite
	clientMock  *MockApiClient
	iboxapi     *iboxapi.MockApiClient
	serviceMock *MockApiService
}

func TestServiceTestSuite(t *testing.T) {
	suite.Run(t, new(ApiTestSuite))
}

func (suite *ApiTestSuite) Test_GetStoragePool_Fail() {
	expectedError := errors.New("Unable to get given pool")
	suite.clientMock.On("GetWithQueryString").Return(nil, expectedError)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	_, err := service.GetStoragePool(1001, "test_storage_pool")

	assert.NotNil(suite.T(), err, "Error should not be nil")
	assert.Equal(suite.T(), expectedError, err, "Error not returned as expected")
}

func (suite *ApiTestSuite) Test_GetStoragePool_Success() {
	storagePool := []StoragePool{
		{},
	}
	expectedResponse := client.ApiResponse{Result: storagePool}
	suite.clientMock.On("GetWithQueryString").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	response, _ := service.GetStoragePool(1001, "test_storage_pool")

	assert.NotNil(suite.T(), response, "Response should not be nil")
	assert.Equal(suite.T(), expectedResponse.Result, response, "Response not returned as expected")
}

func (suite *ApiTestSuite) Test_CreateSnapshotVolume_Fail() {
	expectedError := errors.New("Missing parameters")
	suite.clientMock.On("Post").Return(nil, expectedError)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	snapshotParams := VolumeSnapshot{ParentID: 1001}
	_, err := service.CreateSnapshotVolume(0, &snapshotParams)

	assert.NotNil(suite.T(), err, "Error should not be nil")
	assert.Equal(suite.T(), expectedError, err, "Error not returned as expected")
}

func (suite *ApiTestSuite) Test_CreateSnapshotVolume_Success() {
	// Test volume snapshot will be created
	expectedResponse := client.ApiResponse{Result: &SnapshotVolumesResp{}}
	suite.clientMock.On("Post").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	snapshotParams := VolumeSnapshot{ParentID: 1001, SnapshotName: "test_volume_resp"}
	response, _ := service.CreateSnapshotVolume(0, &snapshotParams)

	assert.NotNil(suite.T(), response, "Response should not be nil")
	assert.Equal(suite.T(), expectedResponse.Result, response, "Response not returned as expected")
}

func (suite *ApiTestSuite) Test_MapVolumeToHost_Fail() {
	expectedError := errors.New("Volume ID is missing")
	suite.clientMock.On("Post").Return(nil, expectedError)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	_, err := service.MapVolumeToHost(1, 2, 2)

	assert.NotNil(suite.T(), err, "Error should not be nil")
	assert.Equal(suite.T(), expectedError, err, "Error not returned as expected")
}

func (suite *ApiTestSuite) Test_MapVolumeToHost_Success() {
	expectedResponse := client.ApiResponse{Result: LunInfo{HostClusterID: 0, VolumeID: 0, CLustered: false, HostID: 0, ID: 0, Lun: 0}}

	suite.clientMock.On("Post").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	response, _ := service.MapVolumeToHost(1001, 2, 2)

	assert.NotNil(suite.T(), response, "Response should not be nil")
	assert.Equal(suite.T(), expectedResponse.Result, response, "Response not returned as expected")
}

func (suite *ApiTestSuite) Test_UpdateFilesystem_Fail() {
	// Test volume snapshot will not be created
	expectedError := errors.New("Missing parameters")
	suite.clientMock.On("Put").Return(nil, expectedError)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	fileSystem := FileSystem{}
	_, err := service.UpdateFilesystem(1001, fileSystem)

	assert.NotNil(suite.T(), err, "Error should not be nil")
	assert.Equal(suite.T(), expectedError, err, "Error not returned as expected")
}

func (suite *ApiTestSuite) Test_UpdateFilesystem_Success() {
	// Test volume snapshot will be created
	expectedResponse := client.ApiResponse{Result: &FileSystem{}}

	suite.clientMock.On("Put").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	fileSystem := FileSystem{Size: 100}
	response, _ := service.UpdateFilesystem(1001, fileSystem)

	assert.NotNil(suite.T(), response, "Response should not be nil")
	assert.Equal(suite.T(), expectedResponse.Result, response, "Response not returned as expected")
}

func (suite *ApiTestSuite) Test_CreateFileSystemSnapshot_Fail() {
	// Test volume snapshot will not be created
	expectedError := errors.New("Missing parameters")
	suite.clientMock.On("Post").Return(nil, expectedError)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	fileSystemSnapshot := &FileSystemSnapshot{
		ParentID:       1000,
		WriteProtected: true,
	}
	_, err := service.CreateFileSystemSnapshot(0, fileSystemSnapshot)

	assert.NotNil(suite.T(), err, "Error should not be nil")
	assert.Equal(suite.T(), expectedError, err, "Error not returned as expected")
}

func (suite *ApiTestSuite) Test_CreateFileSystemSnapshot_Success() {
	expectedResponse := client.ApiResponse{Result: &FileSystemSnapshotResponse{SnapshotID: 0, Name: "", DatasetType: "", ParentId: 0, Size: 0, CreatedAt: 0}}

	suite.clientMock.On("Post").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	fileSystemSnapshot := &FileSystemSnapshot{
		ParentID:       1000,
		SnapshotName:   "test_snapshot",
		WriteProtected: true,
	}

	response, _ := service.CreateFileSystemSnapshot(0, fileSystemSnapshot)

	assert.NotNil(suite.T(), response, "Response should not be nil")
	assert.Equal(suite.T(), expectedResponse.Result, response, "Response not returned as expected")
}

func (suite *ApiTestSuite) Test_DeleteFileSystem_Fail() {
	// Test volume snapshot will not be created
	expectedError := errors.New("Missing parameters")
	suite.clientMock.On("Delete").Return(nil, expectedError)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	_, err := service.DeleteFileSystem(1001)

	assert.NotNil(suite.T(), err, "Error should not be nil")
	assert.Equal(suite.T(), expectedError, err, "Error not returned as expected")
}

func (suite *ApiTestSuite) Test_DeleteFileSystem_Success() {
	expectedResponse := client.ApiResponse{Result: &FileSystem{ID: 0, PoolID: 0, Name: "", SsdEnabled: false, Provtype: "", Size: 0, ParentID: 0, PoolName: "", CreatedAt: 0}}
	suite.clientMock.On("Delete").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	response, _ := service.DeleteFileSystem(1001)

	assert.NotNil(suite.T(), response, "Response should not be nil")
	assert.Equal(suite.T(), expectedResponse.Result, response, "Response not returned as expected")
}

// ****************************************
func (suite *ApiTestSuite) Test_GetFilesystemTreeqCount_error() {
	expectedError := errors.New("some error")
	suite.clientMock.On("Get").Return(nil, expectedError)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	response, err := service.GetFilesystemTreeqCount(1001)
	expectedResponse := 0
	assert.NotNil(suite.T(), err, "Response should not be nil")
	assert.Equal(suite.T(), expectedResponse, response, "Response should not be nil")
}

func (suite *ApiTestSuite) Test_GetFilesystemTreeqCount_Success() {
	expectedResponse := client.ApiResponse{MetaData: client.Resultmetadata{NoOfObject: 10}}
	suite.clientMock.On("Get").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	response, err := service.GetFilesystemTreeqCount(1001)
	expectedvalue := 10
	assert.Nil(suite.T(), err, "Response should not be nil")
	assert.Equal(suite.T(), expectedvalue, response, "Response should not be nil")
}

func (suite *ApiTestSuite) Test_CreateTreeq_success() {
	fileSysID := 100
	expectedResponse := client.ApiResponse{Result: Treeq{ID: 1, FilesystemID: fileSysID, Name: "treeq", Path: "\treeq", HardCapacity: 100}}
	suite.clientMock.On("Post").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	pvName := "treeq"
	treeqParameter := map[string]interface{}{
		"path":          "\\" + pvName,
		"name":          pvName,
		"hard_capacity": 100,
	}

	response, err := service.CreateTreeq(fileSysID, treeqParameter)

	assert.Nil(suite.T(), err, "Response should not be nil")
	assert.Equal(suite.T(), fileSysID, response.FilesystemID, "filesystemID should be equal")
	assert.Equal(suite.T(), "\treeq", response.Path, "path should be equal")
	assert.Equal(suite.T(), treeqParameter["name"], response.Name, "name should be equal")
}

func (suite *ApiTestSuite) Test_CreateTreeq_Error() {
	fileSysID := 100
	expectedErr := errors.New("some error")
	suite.clientMock.On("Post").Return(nil, expectedErr)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	pvName := "treeq"
	treeqParameter := map[string]interface{}{
		"path":          "\\" + pvName,
		"name":          pvName,
		"hard_capacity": 100,
	}
	response, err := service.CreateTreeq(fileSysID, treeqParameter)
	assert.NotNil(suite.T(), err, "Response should not be nil")
	assert.Nil(suite.T(), response, "response should be nil")
}

func (suite *ApiTestSuite) Test_DeleteTreeq_Success() {
	resp := client.ApiResponse{}
	suite.clientMock.On("Delete").Return(resp, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	FilesystemID := 3111
	treeqID := 20000
	_, err := service.DeleteTreeq(FilesystemID, treeqID)
	assert.Nil(suite.T(), err, "Response should not be nil")
}

func (suite *ApiTestSuite) Test_DeleteTreeq_Error() {
	expectedErr := errors.New("some error occured")
	suite.clientMock.On("Delete").Return(nil, expectedErr)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	FilesystemID := 3111
	treeqID := 20000
	_, err := service.DeleteTreeq(FilesystemID, treeqID)
	assert.NotNil(suite.T(), err, "Response should not be nil")
}

func (suite *ApiTestSuite) Test_GetSnapshotByName_Fail() {
	// Test volume snapshot will not be created
	expectedError := errors.New("Missing parameters")
	suite.clientMock.On("Get").Return(nil, expectedError)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	_, err := service.GetSnapshotByName("test_snapshot")

	assert.NotNil(suite.T(), err, "Error should not be nil")
	assert.Equal(suite.T(), expectedError, err, "Error not returned as expected")
}

func (suite *ApiTestSuite) Test_GetSnapshotByName_Success() {
	var snapResponse []FileSystemSnapshotResponse
	expectedResponse := client.ApiResponse{Result: &snapResponse}

	suite.clientMock.On("Get").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	response, _ := service.GetSnapshotByName("test_snapshot")

	assert.NotNil(suite.T(), response, "Response should not be nil")
	assert.Equal(suite.T(), expectedResponse.Result, response, "Response not returned as expected")
}

func (suite *ApiTestSuite) Test_RestoreFileSystemFromSnapShot_Fail() {
	// Test volume snapshot will not be created
	expectedError := errors.New("Missing parameters")
	suite.clientMock.On("Post").Return(nil, expectedError)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	_, err := service.RestoreFileSystemFromSnapShot(1001, 1002)

	assert.NotNil(suite.T(), err, "Error should not be nil")
	assert.Equal(suite.T(), expectedError, err, "Error not returned as expected")
}

func (suite *ApiTestSuite) Test_RestoreFileSystemFromSnapShot_Success() {
	// Test volume snapshot will be created
	var expectedResponse client.ApiResponse
	suite.clientMock.On("Post").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	response, _ := service.RestoreFileSystemFromSnapShot(1001, 1002)

	assert.NotNil(suite.T(), response, "Response should not be nil")
	assert.Equal(suite.T(), false, response, "Response not returned as expected")
}

func (suite *ApiTestSuite) Test_GetVolumeSnapshotByParentID_Fail() {
	// Test volume snapshot will not be created
	expectedError := errors.New("Missing parameters")
	suite.clientMock.On("GetWithQueryString").Return(nil, expectedError)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	_, err := service.GetVolumeSnapshotByParentID(1001)

	assert.NotNil(suite.T(), err, "Error should not be nil")
	assert.Equal(suite.T(), expectedError, err, "Error not returned as expected")
}

func (suite *ApiTestSuite) Test_GetVolumeSnapshotByParentID_Success() {
	var volumeResponse []Volume
	expectedResponse := client.ApiResponse{Result: &volumeResponse}

	suite.clientMock.On("GetWithQueryString").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	response, _ := service.GetVolumeSnapshotByParentID(1001)

	assert.NotNil(suite.T(), response, "Response should not be nil")
	assert.Equal(suite.T(), expectedResponse.Result, response, "Response not returned as expected")
}

func (suite *ApiTestSuite) Test_GetTreeq_Success() {
	FilesystemID := 3111
	treeqID := 20000
	expectedResponse := client.ApiResponse{Result: Treeq{ID: treeqID, FilesystemID: FilesystemID, HardCapacity: 10000, Name: "treeq1", Path: "/treeqPath", UsedCapacity: 10}}
	suite.clientMock.On("Get").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	resp, err := service.GetTreeq(FilesystemID, treeqID)
	assert.Nil(suite.T(), err, "err should  nil")
	assert.Equal(suite.T(), FilesystemID, resp.FilesystemID, "file systemID should be equal")
}

func (suite *ApiTestSuite) Test_GetTreeq_fail() {
	FilesystemID := 3111
	treeqID := 20000
	expectedErr := errors.New("some error")
	suite.clientMock.On("Get").Return(nil, expectedErr)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	_, err := service.GetTreeq(FilesystemID, treeqID)
	assert.NotNil(suite.T(), err, "err should  nil")
}

func (suite *ApiTestSuite) Test_UpdateTreeq_Success() {
	FilesystemID := 3111
	treeqID := 20000
	expectedResponse := client.ApiResponse{Result: Treeq{ID: treeqID, FilesystemID: FilesystemID, HardCapacity: 10000, Name: "treeq1", Path: "/treeqPath", UsedCapacity: 10}}
	suite.clientMock.On("Put").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	body := map[string]interface{}{"hard_capacity": 1000000}
	resp, err := service.UpdateTreeq(FilesystemID, treeqID, body)
	assert.Nil(suite.T(), err, "err should  nil")
	assert.Equal(suite.T(), FilesystemID, resp.FilesystemID, "file systemID should be equal")
}

func (suite *ApiTestSuite) Test_UpdateTreeq_fail() {
	FilesystemID := 3111
	treeqID := 20000
	expectedErr := errors.New("some error")
	suite.clientMock.On("Put").Return(nil, expectedErr)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	body := map[string]interface{}{"hard_capacity": 1000000}
	_, err := service.UpdateTreeq(FilesystemID, treeqID, body)
	assert.NotNil(suite.T(), err, "Error should not be nil")
	assert.Equal(suite.T(), expectedErr, err, "Error not returned as expected")
}

func (suite *ApiTestSuite) Test_FileSystemHasChild_success() {
	var FilesystemID = 3111
	fileSysArry := []FileSystem{
		{ID: 3111},
	}

	expectedResponse := client.ApiResponse{Result: fileSysArry}
	suite.clientMock.On("GetWithQueryString").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	status := service.FileSystemHasChild(FilesystemID)
	assert.True(suite.T(), status)
}

func (suite *ApiTestSuite) Test_FileSystemHasChild_Error() {
	var FilesystemID = 3111
	expectedErr := errors.New("some error")
	suite.clientMock.On("GetWithQueryString").Return(nil, expectedErr)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	status := service.FileSystemHasChild(FilesystemID)
	assert.False(suite.T(), status)
}

func (suite *ApiTestSuite) Test_AddNodeInExport_Error() {
	expectedErr := errors.New("some error")
	suite.clientMock.On("Get").Return(nil, expectedErr)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	_, err := service.AddNodeInExport(100, "", false, "10.20.30.40")
	assert.NotNil(suite.T(), err, "Error should not be nil")
}

func (suite *ApiTestSuite) Test_AddNodeInExport_IPAddress_exist_success() {
	exportResp := ExportResponse{
		ID: 1009,
	}

	permissionsArry := []Permissions{
		{
			Access:       "RW",
			Client:       "10.20.30.40",
			NoRootSquash: false,
		},
	}

	exportResp.Permissions = append(exportResp.Permissions, permissionsArry...)

	expectedResponse := client.ApiResponse{Result: exportResp}
	suite.clientMock.On("Get").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	_, err := service.AddNodeInExport(100, "RW", false, "10.20.30.40")
	assert.Nil(suite.T(), err, "Error should not be nil")
}

func (suite *ApiTestSuite) Test_AddNodeInExport_IP_not_exist_success() {
	exportResp := ExportResponse{
		ID: 1009,
	}

	permissionsArry := []Permissions{
		{
			Access:       "RW",
			Client:       "10.20.30.30-10.20.30.41",
			NoRootSquash: false,
		},
	}

	exportResp.Permissions = append(exportResp.Permissions, permissionsArry...)

	expectedResponse := client.ApiResponse{Result: exportResp}
	suite.clientMock.On("Get").Return(expectedResponse, nil)

	var putresp ExportResponse
	exportResp.ID = 123
	updateResponse := client.ApiResponse{Result: putresp}

	suite.clientMock.On("Put").Return(updateResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	_, err := service.AddNodeInExport(100, "RW", false, "10.20.30.40")
	assert.Nil(suite.T(), err, "Error should be nil")
}

func (suite *ApiTestSuite) Test_AddNodeInExport_IP_outside_range_added_success() {
	exportResp := ExportResponse{
		ID: 1009,
	}

	permissionsArry := []Permissions{
		{
			Access:       "RW",
			Client:       "10.20.30.30-10.20.30.40",
			NoRootSquash: false,
		},
	}

	exportResp.Permissions = append(exportResp.Permissions, permissionsArry...)

	expectedResponse := client.ApiResponse{Result: exportResp}
	suite.clientMock.On("Get").Return(expectedResponse, nil)

	var putresp ExportResponse
	exportResp.ID = 123
	updateResponse := client.ApiResponse{Result: putresp}

	suite.clientMock.On("Put").Return(updateResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	_, err := service.AddNodeInExport(100, "RW", false, "10.20.30.99")

	assert.Nil(suite.T(), err, "Error should be nil")
}

func (suite *ApiTestSuite) Test_AddNodeInExport_update_error() {
	exportResp := ExportResponse{
		ID: 1009,
	}

	permissionsArry := []Permissions{
		{
			Access:       "RW",
			Client:       "10.20.30.41",
			NoRootSquash: false,
		},
	}

	exportResp.Permissions = append(exportResp.Permissions, permissionsArry...)

	expectedResponse := client.ApiResponse{Result: exportResp}
	suite.clientMock.On("Get").Return(expectedResponse, nil)

	expectedErr := errors.New("some error")
	suite.clientMock.On("Put").Return(nil, expectedErr)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	_, err := service.AddNodeInExport(100, "RW", false, "10.20.30.40")
	assert.NotNil(suite.T(), err, "Error should not be nil")
}

func (suite *ApiTestSuite) Test_DeleteNodeFromExport_Error() {
	expectedErr := errors.New("some error")
	suite.clientMock.On("Get").Return(nil, expectedErr)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	_, err := service.DeleteNodeFromExport(100, "RW", false, "10.20.30.40")
	assert.NotNil(suite.T(), err, "Error should not be nil")
}

func (suite *ApiTestSuite) Test_DeleteNodeFromExport_already_deleted() {
	exportResp := ExportResponse{
		ID: 1009,
	}

	permissionsArry := []Permissions{
		{
			Access:       "RW",
			Client:       "10.20.30.41",
			NoRootSquash: false,
		},
	}

	exportResp.Permissions = append(exportResp.Permissions, permissionsArry...)

	expectedResponse := client.ApiResponse{Result: exportResp}

	suite.clientMock.On("Get").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	_, err := service.DeleteNodeFromExport(100, "RW", false, "10.20.30.40")
	assert.Nil(suite.T(), err, "Error should not be nil")
}

func (suite *ApiTestSuite) Test_DeleteNodeFromExport_update_success() {
	exportResp := ExportResponse{
		ID: 1009,
	}

	permissionsArry := []Permissions{
		{
			Access:       "RW",
			Client:       "10.20.30.40",
			NoRootSquash: false,
		},
	}

	exportResp.Permissions = append(exportResp.Permissions, permissionsArry...)

	expectedResponse := client.ApiResponse{Result: exportResp}

	suite.clientMock.On("Get").Return(expectedResponse, nil)

	response := client.ApiResponse{Result: ExportResponse{}}
	suite.clientMock.On("Put").Return(response, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	_, err := service.DeleteNodeFromExport(100, "RW", false, "10.20.30.40")
	assert.Nil(suite.T(), err, "Error should not be nil")
}

func (suite *ApiTestSuite) Test_GetFileSystemCountByPoolID_success() {
	expectedResponse := client.ApiResponse{Result: getFilesystemArry(), MetaData: client.Resultmetadata{NoOfObject: 100}}
	suite.clientMock.On("Get").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	var poolID = 1
	response, err := service.GetFileSystemCountByPoolID(poolID)
	assert.Nil(suite.T(), err, "Response should not be nil")
	assert.Equal(suite.T(), 100, response, "response should be nil")
}

func (suite *ApiTestSuite) Test_GetFileSystemCountByPoolID_Error() {
	expectedErr := errors.New("some error")
	suite.clientMock.On("Get").Return(nil, expectedErr)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	var poolID = 1
	_, err := service.GetFileSystemCountByPoolID(poolID)
	assert.NotNil(suite.T(), err, "Response should not be nil")
}

func (suite *ApiTestSuite) Test_GetTreeqSizeByFileSystemID_success() {
	treeqArr := []Treeq{
		{
			ID:           111,
			Name:         "treeqName",
			HardCapacity: 100,
		},
	}

	expectedResponse := client.ApiResponse{Result: treeqArr}
	suite.clientMock.On("Get").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	var filesystemID = 100
	_, err := service.GetTreeqSizeByFileSystemID(filesystemID)
	assert.Nil(suite.T(), err, "Response should not be nil")
	// assert.Equal(suite.T(), 100, response, "response should be nil")
}

func (suite *ApiTestSuite) Test_GetTreeqSizeByFileSystemID_Error() {
	expecteErr := errors.New("some Error")
	suite.clientMock.On("Get").Return(nil, expecteErr)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	var filesystemID = 100
	_, err := service.GetTreeqSizeByFileSystemID(filesystemID)
	assert.NotNil(suite.T(), err, "Response should not be nil")
}

func (suite *ApiTestSuite) Test_GetTreeqByName_Error() {
	expecteErr := errors.New("some Error")
	suite.clientMock.On("GetWithQueryString").Return(nil, expecteErr)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	var filesystemID = 100
	_, err := service.GetTreeqByName(filesystemID, "treeqName")
	assert.NotNil(suite.T(), err, "Response should not be nil")
}

func (suite *ApiTestSuite) Test_GetTreeqByName_success() {
	treeqArr := []Treeq{
		{
			ID:           111,
			Name:         "treeqName",
			HardCapacity: 100,
		},
	}

	expectedResponse := client.ApiResponse{Result: treeqArr}
	suite.clientMock.On("GetWithQueryString").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	var filesystemID = 100
	_, err := service.GetTreeqByName(filesystemID, "treeqName")
	assert.Nil(suite.T(), err, "Response should not be nil")
}

func getExportResponse() *[]ExportResponse {
	exportRespArry := []ExportResponse{}

	return &exportRespArry
}

func setSecret() map[string]string {
	secretMap := map[string]string{
		"username": "admin",
		"password": "123456",
		"hostname": "http://172.17.35.61/",
	}
	return secretMap
}

func getFilesystemArry() []FileSystem {
	filesystems := []FileSystem{
		{PoolID: 1, Size: 10000},
		{PoolID: 1, Size: 10000},
	}
	return filesystems
}

func getMetaData() client.Resultmetadata {
	metaData := client.Resultmetadata{
		NoOfObject: 100,
		Page:       1,
		PageSize:   50,
		TotalPages: 2,
	}
	return metaData
}

func getLunMetaData100() client.Resultmetadata {
	metaData := client.Resultmetadata{
		NoOfObject: 100,
		Page:       1,
		PageSize:   1000,
		TotalPages: 1,
	}
	return metaData
}

func getLunMetaData1010Pg1() client.Resultmetadata {
	metaData := client.Resultmetadata{
		NoOfObject: 1010,
		Page:       1,
		PageSize:   1000,
		TotalPages: 2,
	}
	return metaData
}

func getLunMetaData1010Pg2() client.Resultmetadata {
	metaData := client.Resultmetadata{
		NoOfObject: 1010,
		Page:       1,
		PageSize:   1000,
		TotalPages: 2,
	}
	return metaData
}

// returns a set of LunInfo results like how a rest call would return it
func buildLunQueryResults(numLuns int) interface{} {

	testSlice := []LunInfo{}

	for i := 1; i <= numLuns; i++ {

		lun := LunInfo{
			HostClusterID: 1,
			VolumeID:      1,
			CLustered:     false,
			HostID:        1,
			ID:            i,
			Lun:           1234567890,
		}
		testSlice = append(testSlice, lun)
	}

	return testSlice
}
