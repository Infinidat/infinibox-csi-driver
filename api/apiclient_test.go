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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

func (suite *ApiTestSuite) SetupTest() {
	suite.clientMock = new(MockApiClient)
	suite.serviceMock = new(MockApiService)
}

type ApiTestSuite struct {
	suite.Suite
	clientMock  *MockApiClient
	serviceMock *MockApiService
}

func TestServiceTestSuite(t *testing.T) {
	suite.Run(t, new(ApiTestSuite))
}

func (suite *ApiTestSuite) Test_CreateVolumeGetStoragePoolIDByName_Fail() {
	// Configure
	expectedError := errors.New("failed to get pool ID from pool Name: test_storage_pool")

	suite.clientMock.On("GetWithQueryString").Return(nil, expectedError)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	// Act
	volume := VolumeParam{Name: "test_volume"}
	_, err := service.CreateVolume(&volume, "test_storage_pool")

	// Assert
	assert.NotNil(suite.T(), err, "Error should not be nil")
	assert.Equal(suite.T(), expectedError, err, "Error not returned as expected")
}

func (suite *ApiTestSuite) Test_CreateVolume_Fail() {
	storagePool := []StoragePool{}
	sp := StoragePool{}
	storagePool = append(storagePool, sp)
	expectedResponse := client.ApiResponse{Result: storagePool}
	suite.clientMock.On("GetWithQueryString").Return(expectedResponse, nil)
	expectedError := errors.New("No such pool: test_storage_pool")
	suite.clientMock.On("Post").Return(nil, expectedError)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	volume := VolumeParam{
		Name:       "test_volume",
		PoolId:     1000,
		VolumeSize: 1000000000,
	}
	_, err := service.CreateVolume(&volume, "test_storage_pool")

	// Assert
	assert.NotNil(suite.T(), err, "Error should not be nil")
	assert.Equal(suite.T(), expectedError, err, "Error not returned as expected")
}

func (suite *ApiTestSuite) Test_CreateVolume_Success() {
	storagePools := []StoragePool{}
	sp := StoragePool{}
	storagePools = append(storagePools, sp)
	expectedResponse := client.ApiResponse{Result: storagePools}
	suite.clientMock.On("GetWithQueryString").Return(expectedResponse, nil)
	expectedResponse = client.ApiResponse{Result: &Volume{}}
	suite.clientMock.On("Post").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	volumeparam := VolumeParam{Name: "test_volume", PoolId: 5307, VolumeSize: 1000000000, ProvisionType: "THIN"}
	response, _ := service.CreateVolume(&volumeparam, "test_name")

	// Assert
	assert.NotNil(suite.T(), response, "Response should not be nil")
	assert.Equal(suite.T(), expectedResponse.Result, response, "Response not returned as expected")
}

func (suite *ApiTestSuite) Test_GetStoragePool_Fail() {
	expectedError := errors.New("Unable to get given pool")
	suite.clientMock.On("GetWithQueryString").Return(nil, expectedError)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	_, err := service.GetStoragePool(1001, "test_storage_pool")

	// Assert
	assert.NotNil(suite.T(), err, "Error should not be nil")
	assert.Equal(suite.T(), expectedError, err, "Error not returned as expected")
}

func (suite *ApiTestSuite) Test_GetStoragePool_Success() {
	storagePools := []StoragePool{}
	sp := StoragePool{}
	storagePools = append(storagePools, sp)
	expectedResponse := client.ApiResponse{Result: storagePools}
	suite.clientMock.On("GetWithQueryString").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	response, _ := service.GetStoragePool(1001, "test_storage_pool")

	// Assert
	assert.NotNil(suite.T(), response, "Response should not be nil")
	assert.Equal(suite.T(), expectedResponse.Result, response, "Response not returned as expected")
}

func (suite *ApiTestSuite) Test_GetStoragePoolIDByName_Fail() {
	expectedError := errors.New("failed to get pool ID from pool Name: test_storage_pool")
	suite.clientMock.On("GetWithQueryString").Return(nil, expectedError)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	_, err := service.GetStoragePoolIDByName("test_storage_pool")

	// Assert
	assert.NotNil(suite.T(), err, "Error should not be nil")
	assert.Equal(suite.T(), expectedError, err, "Error not returned as expected")
}

func (suite *ApiTestSuite) Test_GetStoragePoolIDByName_Success() {
	//var poolID int64
	//suite.clientMock.On("GetWithQueryString").Return(poolID, nil)
	resp := client.ApiResponse{}
	suite.clientMock.On("GetWithQueryString").Return(resp, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	response, _ := service.GetStoragePoolIDByName("test_storage_pool")

	// Assert
	assert.NotNil(suite.T(), response, "Response should not be nil")
	//assert.Equal(suite.T(), poolID, response, "Response not returned as expected")
}

func (suite *ApiTestSuite) Test_GetVolumeByName_Fail() {
	expectedError := errors.New("Unable to get given volume by name")
	suite.clientMock.On("GetWithQueryString").Return(nil, expectedError)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	_, err := service.GetVolumeByName("test_storage_pool")

	// Assert
	assert.NotNil(suite.T(), err, "Error should not be nil")
	assert.Equal(suite.T(), expectedError, err, "Error not returned as expected")
}

func (suite *ApiTestSuite) Test_GetVolumeByName_Success() {
	volumes := []Volume{}
	volume := Volume{Name: "test1"}
	volumes = append(volumes, volume)
	expectedResponse := client.ApiResponse{Result: volumes}
	suite.clientMock.On("GetWithQueryString").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	response, _ := service.GetVolumeByName("test1")

	// Assert
	assert.NotNil(suite.T(), response, "Response should not be nil")
	assert.Equal(suite.T(), volume, *response, "Response not returned as expected")
}

func (suite *ApiTestSuite) Test_CreateSnapshotVolume_Fail() {
	expectedError := errors.New("Missing parameters")
	suite.clientMock.On("Post").Return(nil, expectedError)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	snapshotParams := VolumeSnapshot{ParentID: 1001}
	_, err := service.CreateSnapshotVolume(&snapshotParams)

	// Assert
	assert.NotNil(suite.T(), err, "Error should not be nil")
	assert.Equal(suite.T(), expectedError, err, "Error not returned as expected")
}

func (suite *ApiTestSuite) Test_CreateSnapshotVolume_Success() {
	// Test volume snapshot will be created
	// expectedResponse := &SnapshotVolumesResp{}
	expectedResponse := client.ApiResponse{Result: &SnapshotVolumesResp{}}
	suite.clientMock.On("Post").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	snapshotParams := VolumeSnapshot{ParentID: 1001, SnapshotName: "test_volume_resp"}
	response, _ := service.CreateSnapshotVolume(&snapshotParams)

	// Assert
	assert.NotNil(suite.T(), response, "Response should not be nil")
	assert.Equal(suite.T(), expectedResponse.Result, response, "Response not returned as expected")
}

func (suite *ApiTestSuite) Test_GetVolume_Fail() {
	expectedError := errors.New("Unable to get given volume")
	suite.clientMock.On("Get").Return(nil, expectedError)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	_, err := service.GetVolume(101)

	// Assert
	assert.NotNil(suite.T(), err, "Error should not be nil")
	assert.Equal(suite.T(), expectedError, err, "Error not returned as expected")
}

func (suite *ApiTestSuite) Test_GetVolume_Success() {
	expectedResponse := client.ApiResponse{Result: &Volume{}}
	suite.clientMock.On("Get").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	response, _ := service.GetVolume(101)

	// Assert
	assert.NotNil(suite.T(), response, "Response should not be nil")
	assert.Equal(suite.T(), expectedResponse.Result, response, "Response not returned as expected")
}

func (suite *ApiTestSuite) Test_GetNetworkSpaceByName_Fail() {
	expectedError := errors.New("Unable to get given network space by name")
	suite.clientMock.On("GetWithQueryString").Return(nil, expectedError)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	_, err := service.GetNetworkSpaceByName("test_network_space")

	// Assert
	assert.NotNil(suite.T(), err, "Error should not be nil")
	assert.Equal(suite.T(), expectedError, err, "Error not returned as expected")
}

func (suite *ApiTestSuite) Test_GetNetworkSpaceByName_Success() {
	// networks := []NetworkSpace{}
	expectedResponse := client.ApiResponse{Result: NetworkSpace{}}

	// networks = append(networks, network)
	suite.clientMock.On("GetWithQueryString").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	response, _ := service.GetNetworkSpaceByName("test_network_space")

	// Assert
	assert.NotNil(suite.T(), response, "Response should not be nil")
	assert.Equal(suite.T(), expectedResponse.Result, response, "Response not returned as expected")
}

func (suite *ApiTestSuite) Test_GetHostByName_Fail() {
	expectedError := errors.New("Unable to get host by given name")
	suite.clientMock.On("GetWithQueryString").Return(nil, expectedError)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	_, err := service.GetHostByName("test_host")

	// Assert
	assert.NotNil(suite.T(), err, "Error should not be nil")
	assert.Equal(suite.T(), expectedError, err, "Error not returned as expected")
}

func (suite *ApiTestSuite) Test_GetHostByName_Success() {
	expectedResponse := client.ApiResponse{Result: Host{}}

	suite.clientMock.On("GetWithQueryString").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	response, _ := service.GetHostByName("test_host")

	// Assert
	assert.NotNil(suite.T(), response, "Response should not be nil")
	assert.Equal(suite.T(), expectedResponse.Result, response, "Response not returned as expected")
}

func (suite *ApiTestSuite) Test_MapVolumeToHost_Fail() {
	expectedError := errors.New("Volume ID is missing")
	suite.clientMock.On("Post").Return(nil, expectedError)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	_, err := service.MapVolumeToHost(1, 2, 2)

	// Assert
	assert.NotNil(suite.T(), err, "Error should not be nil")
	assert.Equal(suite.T(), expectedError, err, "Error not returned as expected")
}

func (suite *ApiTestSuite) Test_MapVolumeToHost_Success() {
	expectedResponse := client.ApiResponse{Result: LunInfo{HostClusterID: 0, VolumeID: 0, CLustered: false, HostID: 0, ID: 0, Lun: 0}}

	suite.clientMock.On("Post").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	response, _ := service.MapVolumeToHost(1001, 2, 2)

	// Assert
	assert.NotNil(suite.T(), response, "Response should not be nil")
	assert.Equal(suite.T(), expectedResponse.Result, response, "Response not returned as expected")
}

func (suite *ApiTestSuite) Test_GetAllLunByHost_SinglePage() {

	expectedResponse := client.ApiResponse{Result: buildLunQueryResults(100), MetaData: getLunMetaData100()}

	suite.clientMock.On("GetWithQueryString", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(expectedResponse, nil)

	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	testResults, _ := service.GetAllLunByHost(1)

	assert.Equal(suite.T(), 100, len(testResults), "Size of results should be 100")

}

func (suite *ApiTestSuite) Test_GetAllLunByHost_TwoPage() {

	expectedResponsePg1 := client.ApiResponse{Result: buildLunQueryResults(1000), MetaData: getLunMetaData1010Pg1()}
	expectedResponsePg2 := client.ApiResponse{Result: buildLunQueryResults(10), MetaData: getLunMetaData1010Pg2()}

	suite.clientMock.On("GetWithQueryString", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(expectedResponsePg1, nil).Once()
	suite.clientMock.On("GetWithQueryString", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(expectedResponsePg2, nil).Once()

	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	testResults, _ := service.GetAllLunByHost(1)

	assert.Equal(suite.T(), 1010, len(testResults), "Size of results should be 1010")

}

func (suite *ApiTestSuite) Test_UpdateFilesystem_Fail() {
	// Test volume snapshot will not be created
	expectedError := errors.New("Missing parameters")
	suite.clientMock.On("Put").Return(nil, expectedError)
	// service := api.ClientService{api: nil}
	// service := suite.serviceMock
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	fileSystem := FileSystem{}
	_, err := service.UpdateFilesystem(1001, fileSystem)

	// Assert
	assert.NotNil(suite.T(), err, "Error should not be nil")
	assert.Equal(suite.T(), expectedError, err, "Error not returned as expected")
}

func (suite *ApiTestSuite) Test_UpdateFilesystem_Success() {
	// Test volume snapshot will be created
	expectedResponse := client.ApiResponse{Result: &FileSystem{}}

	suite.clientMock.On("Put").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	// Act
	fileSystem := FileSystem{Size: 100}
	response, _ := service.UpdateFilesystem(1001, fileSystem)

	// Assert
	assert.NotNil(suite.T(), response, "Response should not be nil")
	assert.Equal(suite.T(), expectedResponse.Result, response, "Response not returned as expected")
}

func (suite *ApiTestSuite) Test_CreateFileSystemSnapshot_Fail() {
	// Test volume snapshot will not be created
	expectedError := errors.New("Missing parameters")
	suite.clientMock.On("Post").Return(nil, expectedError)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	fileSystemSnapshot := &FileSystemSnapshot{
		ParentID:       1000,
		WriteProtected: true,
	}
	_, err := service.CreateFileSystemSnapshot(fileSystemSnapshot)

	// Assert
	assert.NotNil(suite.T(), err, "Error should not be nil")
	assert.Equal(suite.T(), expectedError, err, "Error not returned as expected")
}

func (suite *ApiTestSuite) Test_CreateFileSystemSnapshot_Success() {
	expectedResponse := client.ApiResponse{Result: &FileSystemSnapshotResponce{SnapshotID: 0, Name: "", DatasetType: "", ParentId: 0, Size: 0, CreatedAt: 0}}

	suite.clientMock.On("Post").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	fileSystemSnapshot := &FileSystemSnapshot{
		ParentID:       1000,
		SnapshotName:   "test_snapshot",
		WriteProtected: true,
	}

	// Act
	response, _ := service.CreateFileSystemSnapshot(fileSystemSnapshot)

	// Assert
	assert.NotNil(suite.T(), response, "Response should not be nil")
	assert.Equal(suite.T(), expectedResponse.Result, response, "Response not returned as expected")
}

func (suite *ApiTestSuite) Test_DeleteFileSystem_Fail() {
	// Test volume snapshot will not be created
	expectedError := errors.New("Missing parameters")
	suite.clientMock.On("Delete").Return(nil, expectedError)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	_, err := service.DeleteFileSystem(1001)

	// Assert
	assert.NotNil(suite.T(), err, "Error should not be nil")
	assert.Equal(suite.T(), expectedError, err, "Error not returned as expected")
}

func (suite *ApiTestSuite) Test_DeleteFileSystem_Success() {
	expectedResponse := client.ApiResponse{Result: &FileSystem{ID: 0, PoolID: 0, Name: "", SsdEnabled: false, Provtype: "", Size: 0, ParentID: 0, PoolName: "", CreatedAt: 0}}
	suite.clientMock.On("Delete").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	// Act
	response, _ := service.DeleteFileSystem(1001)

	// Assert
	assert.NotNil(suite.T(), response, "Response should not be nil")
	assert.Equal(suite.T(), expectedResponse.Result, response, "Response not returned as expected")
}

// ****************************************
func (suite *ApiTestSuite) Test_GetFilesytemTreeqCount_error() {
	expectedError := errors.New("some error")
	suite.clientMock.On("Get").Return(nil, expectedError)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	// Act
	response, err := service.GetFilesytemTreeqCount(1001)
	var expectedResponse int = 0
	// Assert
	assert.NotNil(suite.T(), err, "Response should not be nil")
	assert.Equal(suite.T(), expectedResponse, response, "Response should not be nil")
}

func (suite *ApiTestSuite) Test_GetFilesytemTreeqCount_Success() {
	expectedResponse := client.ApiResponse{MetaData: client.Resultmetadata{NoOfObject: 10}}
	suite.clientMock.On("Get").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	// Act
	response, err := service.GetFilesytemTreeqCount(1001)
	var expectedvalue int = 10
	// Assert
	assert.Nil(suite.T(), err, "Response should not be nil")
	assert.Equal(suite.T(), expectedvalue, response, "Response should not be nil")
}

func (suite *ApiTestSuite) Test_CreateTreeq_success() {
	var fileSysID int64 = 100
	expectedResponse := client.ApiResponse{Result: Treeq{ID: 1, FilesystemID: fileSysID, Name: "treeq", Path: "\treeq", HardCapacity: 100}}
	suite.clientMock.On("Post").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	treeqParameter := make(map[string]interface{})
	pvName := "treeq"
	treeqParameter["path"] = "\\" + pvName
	treeqParameter["name"] = pvName
	treeqParameter["hard_capacity"] = 100

	response, err := service.CreateTreeq(fileSysID, treeqParameter)

	// Assert
	assert.Nil(suite.T(), err, "Response should not be nil")
	assert.Equal(suite.T(), fileSysID, response.FilesystemID, "filesystemID should be equal")
	assert.Equal(suite.T(), "\treeq", response.Path, "path should be equal")
	assert.Equal(suite.T(), treeqParameter["name"], response.Name, "name should be equal")
}

func (suite *ApiTestSuite) Test_CreateTreeq_Error() {
	var fileSysID int64 = 100
	// expectedResponse := client.ApiResponse{Result: Treeq{ID: 1, FilesystemID: fileSysID, Name: "treeq", Path: "\treeq", HardCapacity: 100}}
	expectedErr := errors.New("some error")
	suite.clientMock.On("Post").Return(nil, expectedErr)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	treeqParameter := make(map[string]interface{})
	pvName := "treeq"
	treeqParameter["path"] = "\\" + pvName
	treeqParameter["name"] = pvName
	treeqParameter["hard_capacity"] = 100
	response, err := service.CreateTreeq(fileSysID, treeqParameter)
	// Assert
	assert.NotNil(suite.T(), err, "Response should not be nil")
	assert.Nil(suite.T(), response, "response should be nil")
}

func (suite *ApiTestSuite) Test_GetFileSystemsByPoolID_success() {
	expectedResponse := client.ApiResponse{Result: getFilesystemArry(), MetaData: getMetaData()}
	suite.clientMock.On("Get").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	// Act
	var poolID int64 = 1
	var page int = 1
	response, err := service.GetFileSystemsByPoolID(poolID, page)
	// Assert
	assert.Nil(suite.T(), err, "Response should not be nil")
	assert.Equal(suite.T(), 50, response.Filemetadata.PageSize, "response should be nil")
}

func (suite *ApiTestSuite) Test_GetFileSystemsByPoolID_Error() {
	// expectedResponse := client.ApiResponse{Result: getFilesystemArry(), MetaData: getMetaData()}
	expectedErr := errors.New("some error")
	suite.clientMock.On("Get").Return(nil, expectedErr)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	// Act
	var poolID int64 = 1
	var page int = 1
	_, err := service.GetFileSystemsByPoolID(poolID, page)
	// Assert
	assert.NotNil(suite.T(), err, "Response should not be nil")
}

func (suite *ApiTestSuite) Test_DeleteTreeq_Success() {
	resp := client.ApiResponse{}
	suite.clientMock.On("Delete").Return(resp, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	// Act
	var FilesystemID int64 = 3111
	var treeqID int64 = 20000
	_, err := service.DeleteTreeq(FilesystemID, treeqID)
	// Assert
	assert.Nil(suite.T(), err, "Response should not be nil")
}

func (suite *ApiTestSuite) Test_DeleteTreeq_Error() {
	expectedErr := errors.New("some error occured")
	suite.clientMock.On("Delete").Return(nil, expectedErr)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	// Act
	var FilesystemID int64 = 3111
	var treeqID int64 = 20000
	_, err := service.DeleteTreeq(FilesystemID, treeqID)
	// Assert
	assert.NotNil(suite.T(), err, "Response should not be nil")
}

func (suite *ApiTestSuite) Test_GetSnapshotByName_Fail() {
	// Test volume snapshot will not be created
	expectedError := errors.New("Missing parameters")
	suite.clientMock.On("Get").Return(nil, expectedError)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	_, err := service.GetSnapshotByName("test_snapshot")

	// Assert
	assert.NotNil(suite.T(), err, "Error should not be nil")
	assert.Equal(suite.T(), expectedError, err, "Error not returned as expected")
}

func (suite *ApiTestSuite) Test_GetSnapshotByName_Success() {
	var snapResponse []FileSystemSnapshotResponce
	expectedResponse := client.ApiResponse{Result: &snapResponse}

	suite.clientMock.On("Get").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	response, _ := service.GetSnapshotByName("test_snapshot")

	// Assert
	assert.NotNil(suite.T(), response, "Response should not be nil")
	assert.Equal(suite.T(), expectedResponse.Result, response, "Response not returned as expected")
}

func (suite *ApiTestSuite) Test_GetExportByFileSystem_Fail() {
	// Test volume snapshot will not be created
	expectedError := errors.New("Missing parameters")
	suite.clientMock.On("Get").Return(nil, expectedError)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	_, err := service.GetExportByFileSystem(1001)

	// Assert
	assert.NotNil(suite.T(), err, "Error should not be nil")
	assert.Equal(suite.T(), expectedError, err, "Error not returned as expected")
}

func (suite *ApiTestSuite) Test_GetExportByFileSystem_Success() {
	var exportResponse []ExportResponse

	expectedResponse := client.ApiResponse{Result: &exportResponse}
	suite.clientMock.On("Get").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	response, _ := service.GetExportByFileSystem(1001)

	// Assert
	assert.NotNil(suite.T(), response, "Response should not be nil")
	assert.Equal(suite.T(), expectedResponse.Result, response, "Response not returned as expected")
}

func (suite *ApiTestSuite) Test_RestoreFileSystemFromSnapShot_Fail() {
	// Test volume snapshot will not be created
	expectedError := errors.New("Missing parameters")
	suite.clientMock.On("Post").Return(nil, expectedError)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	_, err := service.RestoreFileSystemFromSnapShot(1001, 1002)

	// Assert
	assert.NotNil(suite.T(), err, "Error should not be nil")
	assert.Equal(suite.T(), expectedError, err, "Error not returned as expected")
}

func (suite *ApiTestSuite) Test_RestoreFileSystemFromSnapShot_Success() {
	// Test volume snapshot will be created
	var expectedResponse client.ApiResponse
	suite.clientMock.On("Post").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	// Act
	response, _ := service.RestoreFileSystemFromSnapShot(1001, 1002)

	// Assert
	assert.NotNil(suite.T(), response, "Response should not be nil")
	assert.Equal(suite.T(), false, response, "Response not returned as expected")
}

func (suite *ApiTestSuite) Test_UpdateVolume_Fail() {
	// Test volume snapshot will not be created
	expectedError := errors.New("Missing parameters")
	suite.clientMock.On("Put").Return(nil, expectedError)
	// service := api.ClientService{api: nil}
	// service := suite.serviceMock
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	volume := Volume{}
	_, err := service.UpdateVolume(1001, volume)

	// Assert
	assert.NotNil(suite.T(), err, "Error should not be nil")
	assert.Equal(suite.T(), expectedError, err, "Error not returned as expected")
}

func (suite *ApiTestSuite) Test_UpdateVolume_Success() {
	// Test volume snapshot will be created
	expectedResponse := client.ApiResponse{Result: &Volume{}}

	suite.clientMock.On("Put").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	// Act
	volume := Volume{Size: 100}
	response, _ := service.UpdateVolume(1001, volume)

	// Assert
	assert.NotNil(suite.T(), response, "Response should not be nil")
	assert.Equal(suite.T(), expectedResponse.Result, response, "Response not returned as expected")
}

func (suite *ApiTestSuite) Test_GetVolumeSnapshotByParentID_Fail() {
	// Test volume snapshot will not be created
	expectedError := errors.New("Missing parameters")
	suite.clientMock.On("GetWithQueryString").Return(nil, expectedError)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	_, err := service.GetVolumeSnapshotByParentID(1001)

	// Assert
	assert.NotNil(suite.T(), err, "Error should not be nil")
	assert.Equal(suite.T(), expectedError, err, "Error not returned as expected")
}

func (suite *ApiTestSuite) Test_GetVolumeSnapshotByParentID_Success() {
	var volumeResponse []Volume
	expectedResponse := client.ApiResponse{Result: &volumeResponse}

	suite.clientMock.On("GetWithQueryString").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	response, _ := service.GetVolumeSnapshotByParentID(1001)

	// Assert
	assert.NotNil(suite.T(), response, "Response should not be nil")
	assert.Equal(suite.T(), expectedResponse.Result, response, "Response not returned as expected")
}

func (suite *ApiTestSuite) Test_DeleteVolume_Fail() {
	var metadatas []Metadata
	expectedResponse := client.ApiResponse{Result: &metadatas}
	suite.clientMock.On("Delete").Return(expectedResponse, nil)
	expectedError := errors.New("Given volume ID doesnt exist")
	suite.clientMock.On("Delete").Return(nil, expectedError)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	err := service.DeleteVolume(1001)

	// Assert
	assert.Equal(suite.T(), nil, err, "Error not returned as expected")
}

func (suite *ApiTestSuite) Test_DeleteVolume_Success() {
	var metadatas []Metadata
	expectedResponse := client.ApiResponse{Result: &metadatas}
	suite.clientMock.On("Delete").Return(expectedResponse, nil)
	suite.clientMock.On("Delete").Return(nil, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	// Act
	err := service.DeleteVolume(1001)

	// Assert
	assert.Equal(suite.T(), nil, err, "Response not returned as expected")
}

func (suite *ApiTestSuite) Test_GetTreeq_Success() {
	var FilesystemID int64 = 3111
	var treeqID int64 = 20000
	expectedResponse := client.ApiResponse{Result: Treeq{ID: treeqID, FilesystemID: FilesystemID, HardCapacity: 10000, Name: "treeq1", Path: "/treeqPath", UsedCapacity: 10}}
	suite.clientMock.On("Get").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	// Act
	resp, err := service.GetTreeq(FilesystemID, treeqID)
	// Assert
	assert.Nil(suite.T(), err, "err should  nil")
	assert.Equal(suite.T(), FilesystemID, resp.FilesystemID, "file systemID should be equal")
}

func (suite *ApiTestSuite) Test_GetTreeq_fail() {
	var FilesystemID int64 = 3111
	var treeqID int64 = 20000
	// expectedResponse := client.ApiResponse{Result: Treeq{ID: treeqID, FilesystemID: FilesystemID, HardCapacity: 10000, Name: "treeq1", Path: "/treeqPath", UsedCapacity: 10}}
	expectedErr := errors.New("some error")
	suite.clientMock.On("Get").Return(nil, expectedErr)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	// Act
	_, err := service.GetTreeq(FilesystemID, treeqID)
	// Assert
	assert.NotNil(suite.T(), err, "err should  nil")
}

func (suite *ApiTestSuite) Test_UpdateTreeq_Success() {
	var FilesystemID int64 = 3111
	var treeqID int64 = 20000
	expectedResponse := client.ApiResponse{Result: Treeq{ID: treeqID, FilesystemID: FilesystemID, HardCapacity: 10000, Name: "treeq1", Path: "/treeqPath", UsedCapacity: 10}}
	suite.clientMock.On("Put").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	// Act
	body := map[string]interface{}{"hard_capacity": 1000000}
	resp, err := service.UpdateTreeq(FilesystemID, treeqID, body)
	// Assert
	assert.Nil(suite.T(), err, "err should  nil")
	assert.Equal(suite.T(), FilesystemID, resp.FilesystemID, "file systemID should be equal")
}

func (suite *ApiTestSuite) Test_UpdateTreeq_fail() {
	var FilesystemID int64 = 3111
	var treeqID int64 = 20000
	// expectedResponse := client.ApiResponse{Result: Treeq{ID: treeqID, FilesystemID: FilesystemID, HardCapacity: 10000, Name: "treeq1", Path: "/treeqPath", UsedCapacity: 10}}
	expectedErr := errors.New("some error")
	suite.clientMock.On("Put").Return(nil, expectedErr)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	// Act
	body := map[string]interface{}{"hard_capacity": 1000000}
	_, err := service.UpdateTreeq(FilesystemID, treeqID, body)
	// Assert
	assert.NotNil(suite.T(), err, "Error should not be nil")
	assert.Equal(suite.T(), expectedErr, err, "Error not returned as expected")
}

func (suite *ApiTestSuite) Test_DeleteFileSystemComplete_fail() {
	var FilesystemID int64 = 3111
	//	var treeqID int64 = 20000
	// expectedResponse := client.ApiResponse{Result: Treeq{ID: treeqID, FilesystemID: FilesystemID, HardCapacity: 10000, Name: "treeq1", Path: "/treeqPath", UsedCapacity: 10}}
	expectedErr := errors.New("some error")
	suite.clientMock.On("Get").Return(nil, expectedErr)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	// Act
	// body := map[string]interface{}{"hard_capacity": 1000000}
	err := service.DeleteFileSystemComplete(FilesystemID)
	// Assert
	assert.NotNil(suite.T(), err, "Error should not be nil")
}

func (suite *ApiTestSuite) Test_DeleteFileSystemComplete_export_delete_fail() {
	var FilesystemID int64 = 3111
	var treeqID int64 = 20000
	expectedResponse := client.ApiResponse{Result: Treeq{ID: treeqID, FilesystemID: FilesystemID, HardCapacity: 10000, Name: "treeq1", Path: "/treeqPath", UsedCapacity: 10}}
	expectedErr := errors.New("some error")
	suite.clientMock.On("Get").Return(expectedResponse, nil)
	suite.clientMock.On("Delete").Return(nil, expectedErr)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	// Act
	// body := map[string]interface{}{"hard_capacity": 1000000}
	err := service.DeleteFileSystemComplete(FilesystemID)
	// Assert
	assert.NotNil(suite.T(), err, "Error should not be nil")
}

func (suite *ApiTestSuite) Test_DeleteFileSystemComplete_metadata_delete_fail() {
	var FilesystemID int64 = 3111
	//	var treeqID int64 = 20000
	// expectedResponse := client.ApiResponse{Result: Treeq{ID: treeqID, FilesystemID: FilesystemID, HardCapacity: 10000, Name: "treeq1", Path: "/treeqPath", UsedCapacity: 10}}
	expectedErr := errors.New("EXPORT_NOT_FOUND")
	suite.clientMock.On("Get").Return(nil, expectedErr)
	// suite.clientMock.On("Delete").Return(nil, nil)
	suite.clientMock.On("Delete").Return([]Metadata{}, expectedErr)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	// Act
	// body := map[string]interface{}{"hard_capacity": 1000000}
	err := service.DeleteFileSystemComplete(FilesystemID)
	// Assert
	assert.NotNil(suite.T(), err, "Error should not be nil")
}

func (suite *ApiTestSuite) Test_DeleteFileSystemComplete_delete_fail1() {
	var FilesystemID int64 = 3111
	//	var treeqID int64 = 20000
	// expectedResponse := client.ApiResponse{Result: Treeq{ID: treeqID, FilesystemID: FilesystemID, HardCapacity: 10000, Name: "treeq1", Path: "/treeqPath", UsedCapacity: 10}}
	exportNotFoundErr := errors.New("EXPORT_NOT_FOUND")
	suite.clientMock.On("Get").Return(nil, exportNotFoundErr)
	// suite.clientMock.On("Delete").Return(nil, nil)
	metaDataErr := errors.New("METADATA_IS_NOT_SUPPORTED_FOR_ENTITY")
	suite.clientMock.On("Delete").Return([]Metadata{}, metaDataErr)
	expectedErr := errors.New("some Error")
	suite.clientMock.On("Delete").Return(nil, expectedErr)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	// Act
	// body := map[string]interface{}{"hard_capacity": 1000000}
	err := service.DeleteFileSystemComplete(FilesystemID)
	// Assert
	assert.NotNil(suite.T(), err, "Error should not be nil")
}

func (suite *ApiTestSuite) Test_DeleteFileSystemComplete_delete_success() {
	var FilesystemID int64 = 3111
	exportNotFoundErr := errors.New("EXPORT_NOT_FOUND")
	suite.clientMock.On("Get").Return(nil, exportNotFoundErr)
	resp := client.ApiResponse{}
	suite.clientMock.On("Delete").Return(resp, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	err := service.DeleteFileSystemComplete(FilesystemID)
	// Assert
	assert.Nil(suite.T(), err, "Error should not be nil")
}

func (suite *ApiTestSuite) Test_DeleteFileSystemComplete_deletes() {
	var FilesystemID int64 = 3111
	//	var treeqID int64 = 20000
	expectedResponse := client.ApiResponse{Result: Treeq{ID: 1, FilesystemID: FilesystemID, HardCapacity: 10000, Name: "treeq1", Path: "/treeqPath", UsedCapacity: 10}}
	// expectedErr := errors.New("some error")
	//suite.clientMock.On("Get").Return(getExportResponse(), nil)
	suite.clientMock.On("Get").Return(expectedResponse, nil)
	resp := client.ApiResponse{}
	suite.clientMock.On("Delete").Return(resp, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	// Act
	// body := map[string]interface{}{"hard_capacity": 1000000}
	err := service.DeleteFileSystemComplete(FilesystemID)
	// Assert
	assert.Nil(suite.T(), err, "Error should not be nil")
}

func (suite *ApiTestSuite) Test_DeleteParentFileSystem_haschild_false() {
	var FilesystemID int64 = 3111
	resp := client.ApiResponse{}
	suite.clientMock.On("Get").Return(resp, nil)
	suite.clientMock.On("GetWithQueryString", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(resp, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	err := service.DeleteParentFileSystem(FilesystemID)
	// Assert
	assert.Nil(suite.T(), err, "Error should not be nil")
}

func (suite *ApiTestSuite) Test_DeleteParentFileSystem() {
	var FilesystemID int64 = 3111
	resp := client.ApiResponse{}
	suite.clientMock.On("Get").Return(resp, nil)
	suite.clientMock.On("Delete").Return(resp, nil)
	suite.clientMock.On("GetWithQueryString", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(resp, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	err := service.DeleteParentFileSystem(FilesystemID)
	// Assert
	assert.Nil(suite.T(), err, "Error should not be nil")
}

func (suite *ApiTestSuite) Test_GetParentID() {
	var FilesystemID int64 = 3111
	expectedErr := errors.New("some error")
	suite.clientMock.On("Get").Return(0, expectedErr)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	parentID := service.GetParentID(FilesystemID)
	// Assert
	assert.Equal(suite.T(), int64(0), parentID)
}

func (suite *ApiTestSuite) Test_GetParentID_success() {
	var FilesystemID int64 = 3111
	// expectedErr := errors.New("some error")
	expectedResponse := client.ApiResponse{Result: FileSystem{ParentID: 100}}
	suite.clientMock.On("Get").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	parentID := service.GetParentID(FilesystemID)
	// Assert
	assert.Equal(suite.T(), int64(100), parentID)
}

func (suite *ApiTestSuite) Test_GetFileSystemByID_success() {
	var FilesystemID int64 = 3111
	var parentID int64 = 100

	expectedResponse := client.ApiResponse{Result: FileSystem{ParentID: 100}}
	suite.clientMock.On("Get").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	filesys, err := service.GetFileSystemByID(FilesystemID)
	// Assert
	assert.Nil(suite.T(), err)
	assert.Equal(suite.T(), filesys.ParentID, parentID)
}

func (suite *ApiTestSuite) Test_GetFileSystemByID_Error() {
	var FilesystemID int64 = 3111
	expectedErr := errors.New("some error")

	// expectedResponse := client.ApiResponse{Result: FileSystem{ParentID: 100}}
	suite.clientMock.On("Get").Return(nil, expectedErr)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	_, err := service.GetFileSystemByID(FilesystemID)
	// Assert
	assert.NotNil(suite.T(), err)
}

func (suite *ApiTestSuite) Test_GetMetadataStatus_success() {
	var FilesystemID int64 = 3111

	expectedResponse := client.ApiResponse{Result: Metadata{Value: "true"}}
	suite.clientMock.On("Get").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	status := service.GetMetadataStatus(FilesystemID)
	// Assert
	assert.True(suite.T(), status)
}

func (suite *ApiTestSuite) Test_GetMetadataStatus_Error() {
	var FilesystemID int64 = 3111
	expectedErr := errors.New("some error")
	suite.clientMock.On("Get").Return(nil, expectedErr)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	err := service.GetMetadataStatus(FilesystemID)
	// Assert
	assert.NotNil(suite.T(), err, "Error should not be nil")
}

func (suite *ApiTestSuite) Test_FileSystemHasChild_success() {
	var FilesystemID int64 = 3111
	var fileSysArry []FileSystem
	fileSys := FileSystem{ID: 3111}
	fileSysArry = append(fileSysArry, fileSys)
	expectedResponse := client.ApiResponse{Result: fileSysArry}
	suite.clientMock.On("GetWithQueryString").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	status := service.FileSystemHasChild(FilesystemID)
	// Assert
	assert.True(suite.T(), status)
}

func (suite *ApiTestSuite) Test_FileSystemHasChild_Error() {
	var FilesystemID int64 = 3111
	expectedErr := errors.New("some error")
	suite.clientMock.On("GetWithQueryString").Return(nil, expectedErr)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	status := service.FileSystemHasChild(FilesystemID)
	// Assert
	assert.False(suite.T(), status)
}

func (suite *ApiTestSuite) Test_CreateFilesystem_success() {
	// var FilesystemID int64 = 3111
	fileSys := FileSystem{ID: 3111}
	expectedResponse := client.ApiResponse{Result: fileSys}
	suite.clientMock.On("Post").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	fileSysparameter := make(map[string]interface{})
	fileSysparameter["ID"] = "100"
	_, err := service.CreateFilesystem(fileSysparameter)
	// Assert
	assert.Nil(suite.T(), err, "Error should not be nil")
}

func (suite *ApiTestSuite) Test_CreateFilesystem_Error() {
	// var FilesystemID int64 = 3111
	expectedErr := errors.New("some error")
	suite.clientMock.On("Post").Return(nil, expectedErr)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	fileSysparameter := make(map[string]interface{})
	fileSysparameter["ID"] = "100"
	_, err := service.CreateFilesystem(fileSysparameter)
	// Assert
	assert.NotNil(suite.T(), err, "Error should not be nil")
}

func (suite *ApiTestSuite) Test_AttachMetadataToObject_success() {
	var ObjectID int64 = 3111
	var metaArry []Metadata
	var meta Metadata
	meta.ID = 100
	meta.Key = "keay"
	meta.Value = "value"
	metaArry = append(metaArry, meta)

	expectedResponse := client.ApiResponse{Result: metaArry}
	suite.clientMock.On("Put").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	fileSysparameter := make(map[string]interface{})
	fileSysparameter["ID"] = "100"
	_, err := service.AttachMetadataToObject(ObjectID, fileSysparameter)
	// Assert
	assert.Nil(suite.T(), err, "Error should not be nil")
}

func (suite *ApiTestSuite) Test_AttachMetadataToObject_Error() {
	var ObjectID int64 = 3111

	expectedErr := errors.New("some error")
	suite.clientMock.On("Put").Return(nil, expectedErr)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	fileSysparameter := make(map[string]interface{})
	fileSysparameter["ID"] = "100"
	_, err := service.AttachMetadataToObject(ObjectID, fileSysparameter)
	// Assert
	assert.NotNil(suite.T(), err, "Error should not be nil")
}

func (suite *ApiTestSuite) Test_DeleteExportPath_Error() {
	var exportID int64 = 3111
	expectedErr := errors.New("some error")
	suite.clientMock.On("Delete").Return(nil, expectedErr)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	_, err := service.DeleteExportPath(exportID)
	// Assert
	assert.NotNil(suite.T(), err, "Error should not be nil")
}

func (suite *ApiTestSuite) Test_DeleteExportPath_Success() {
	var exportID int64 = 3111
	var exportReps ExportResponse
	exportReps.ID = 100
	expectedResponse := client.ApiResponse{Result: exportReps}
	suite.clientMock.On("Delete").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	_, err := service.DeleteExportPath(exportID)
	// Assert
	assert.Nil(suite.T(), err, "Error should  be nil")
}

func (suite *ApiTestSuite) Test_OneTimeValidation_PoolIDByName_Error() {
	var poolArry []StoragePool
	var pool StoragePool
	pool.ID = 123
	poolArry = append(poolArry, pool)
	poolIDResponse := client.ApiResponse{Result: poolArry}
	suite.clientMock.On("GetWithQueryString").Return(poolIDResponse, nil)
	expectedErr := errors.New("some error")
	suite.clientMock.On("Get").Return(nil, expectedErr)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	_, err := service.OneTimeValidation("poolName", "newtworkSpace")
	// Assert
	assert.NotNil(suite.T(), err, "Error should not be nil")
}

func (suite *ApiTestSuite) Test_OneTimeValidation_NetworkSpaceByName_Error() {
	expectedErr := errors.New("some error")
	var poolArry []StoragePool
	var pool StoragePool
	pool.ID = 123
	poolArry = append(poolArry, pool)
	poolIDResponse := client.ApiResponse{Result: poolArry}
	suite.clientMock.On("GetWithQueryString").Return(poolIDResponse, nil)
	suite.clientMock.On("Get").Return("networkSpace", nil)
	suite.clientMock.On("Get").Return("networkSpace", expectedErr)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	_, err := service.OneTimeValidation("poolName", "newtworkSpace")
	// Assert
	assert.NotNil(suite.T(), err, "Error should not be nil")
}

func (suite *ApiTestSuite) Test_OneTimeValidation_NetworkSpaceByName_notmatch() {
	var poolArry []StoragePool
	var pool StoragePool
	pool.ID = 123
	poolArry = append(poolArry, pool)
	poolIDResponse := client.ApiResponse{Result: poolArry}

	suite.clientMock.On("GetWithQueryString").Return(poolIDResponse, nil)

	var netArry []NetworkSpace
	var network NetworkSpace
	network.Name = "networkSpace"
	netArry = append(netArry, network)
	expectedResponse := client.ApiResponse{Result: netArry}

	suite.clientMock.On("GetWithQueryString").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	_, err := service.OneTimeValidation("poolName", "newtworkSpace")
	// Assert
	assert.NotNil(suite.T(), err, "Error should not be nil")
}

func (suite *ApiTestSuite) Test_GetFileSystemByName_Error() {
	expectedErr := errors.New("some error")
	suite.clientMock.On("GetWithQueryString").Return(nil, expectedErr)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	_, err := service.GetFileSystemByName("fs_name")
	// Assert
	assert.NotNil(suite.T(), err, "Error should not be nil")
}

func (suite *ApiTestSuite) Test_GetFileSystemByName_Success() {
	fsystems := []FileSystem{}
	var fs FileSystem
	fs.ID = 100
	fs.Name = "fs_1"
	fsystems = append(fsystems, fs)
	expectedResponse := client.ApiResponse{Result: fsystems}

	suite.clientMock.On("GetWithQueryString").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	_, err := service.GetFileSystemByName("fs_1")
	// Assert
	assert.Nil(suite.T(), err, "Error should not be nil")
}

func (suite *ApiTestSuite) Test_GetFileSystemByName_file_not_found() {
	fsystems := []FileSystem{}
	var fs FileSystem
	fs.ID = 100
	fs.Name = "fs_new"
	fsystems = append(fsystems, fs)
	expectedResponse := client.ApiResponse{Result: fsystems}

	suite.clientMock.On("GetWithQueryString").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	_, err := service.GetFileSystemByName("fs_1")
	// Assert
	assert.NotNil(suite.T(), err, "Error should not be nil")
}

func (suite *ApiTestSuite) Test_ExportFileSystem_Error() {
	var export ExportFileSys
	export.FilesystemID = 100
	export.Export_path = "/exportPath"
	export.Name = "exportName"

	var exportResp ExportResponse
	exportResp.ID = 100
	exportResp.ExportPath = "/exportPath"
	exportResp.FilesystemId = 101

	expectedResponse := client.ApiResponse{}
	expectedErr := errors.New("some error")
	//suite.clientMock.On("GetWithQueryString").Return(nil, expectedErr)
	suite.clientMock.On("Post").Return(expectedResponse, expectedErr)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	_, err := service.ExportFileSystem(export)
	// Assert
	assert.NotNil(suite.T(), err, "Error should not be nil")
}

func (suite *ApiTestSuite) Test_ExportFileSystem_Success() {
	var export ExportFileSys
	export.FilesystemID = 100
	export.Export_path = "/exportPath"
	export.Name = "exportName"

	var exportResp ExportResponse
	exportResp.ID = 100
	exportResp.ExportPath = "/exportPath"
	exportResp.FilesystemId = 100

	expectedResponse := client.ApiResponse{Result: exportResp}

	suite.clientMock.On("Post").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	_, err := service.ExportFileSystem(export)
	// Assert
	assert.Nil(suite.T(), err, "Error should not be nil")
}

func (suite *ApiTestSuite) Test_AddNodeInExport_Error() {
	expectedErr := errors.New("some error")
	suite.clientMock.On("Get").Return(nil, expectedErr)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	_, err := service.AddNodeInExport(100, "", false, "10.20.30.40")
	// Assert
	assert.NotNil(suite.T(), err, "Error should not be nil")
}

func (suite *ApiTestSuite) Test_AddNodeInExport_IPAddress_exist_success() {
	// expectedErr := errors.New("some error")
	exportResp := ExportResponse{}
	exportResp.ID = 1009

	permissionsArry := []Permissions{}
	permissions := Permissions{}
	permissions.Access = "RW"
	permissions.Client = "10.20.30.40"
	permissions.NoRootSquash = false
	permissionsArry = append(permissionsArry, permissions)

	exportResp.Permissions = append(exportResp.Permissions, permissionsArry...)

	expectedResponse := client.ApiResponse{Result: exportResp}
	suite.clientMock.On("Get").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	_, err := service.AddNodeInExport(100, "RW", false, "10.20.30.40")
	// Assert
	assert.Nil(suite.T(), err, "Error should not be nil")
}

func (suite *ApiTestSuite) Test_AddNodeInExport_IP_not_exist_success() {
	// expectedErr := errors.New("some error")
	exportResp := ExportResponse{}
	exportResp.ID = 1009

	permissionsArry := []Permissions{}
	permissions := Permissions{}
	permissions.Access = "RW"
	permissions.Client = "10.20.30.30-10.20.30.41"
	permissions.NoRootSquash = false
	permissionsArry = append(permissionsArry, permissions)

	exportResp.Permissions = append(exportResp.Permissions, permissionsArry...)

	expectedResponse := client.ApiResponse{Result: exportResp}
	suite.clientMock.On("Get").Return(expectedResponse, nil)

	var putresp ExportResponse
	exportResp.ID = 123
	updateResponse := client.ApiResponse{Result: putresp}

	suite.clientMock.On("Put").Return(updateResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	_, err := service.AddNodeInExport(100, "RW", false, "10.20.30.40")
	// Assert
	assert.Nil(suite.T(), err, "Error should be nil")
}

func (suite *ApiTestSuite) Test_AddNodeInExport_IP_outside_range_added_success() {
	// expectedErr := errors.New("some error")
	exportResp := ExportResponse{}
	exportResp.ID = 1009

	permissionsArry := []Permissions{}
	permissions := Permissions{}
	permissions.Access = "RW"
	permissions.Client = "10.20.30.30-10.20.30.40"
	permissions.NoRootSquash = false
	permissionsArry = append(permissionsArry, permissions)

	exportResp.Permissions = append(exportResp.Permissions, permissionsArry...)

	expectedResponse := client.ApiResponse{Result: exportResp}
	suite.clientMock.On("Get").Return(expectedResponse, nil)

	var putresp ExportResponse
	exportResp.ID = 123
	updateResponse := client.ApiResponse{Result: putresp}

	suite.clientMock.On("Put").Return(updateResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	_, err := service.AddNodeInExport(100, "RW", false, "10.20.30.99")

	// Assert
	assert.Nil(suite.T(), err, "Error should be nil")
}

func (suite *ApiTestSuite) Test_AddNodeInExport_update_error() {
	// expectedErr := errors.New("some error")
	exportResp := ExportResponse{}
	exportResp.ID = 1009

	permissionsArry := []Permissions{}
	permissions := Permissions{}
	permissions.Access = "RW"
	permissions.Client = "10.20.30.41"
	permissions.NoRootSquash = false
	permissionsArry = append(permissionsArry, permissions)

	exportResp.Permissions = append(exportResp.Permissions, permissionsArry...)

	expectedResponse := client.ApiResponse{Result: exportResp}
	suite.clientMock.On("Get").Return(expectedResponse, nil)

	expectedErr := errors.New("some error")
	suite.clientMock.On("Put").Return(nil, expectedErr)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	_, err := service.AddNodeInExport(100, "RW", false, "10.20.30.40")
	// Assert
	assert.NotNil(suite.T(), err, "Error should not be nil")
}

func (suite *ApiTestSuite) Test_DeleteExportRule_Error() {
	expectedErr := errors.New("some error")
	suite.clientMock.On("Get").Return(nil, expectedErr)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	var fsID int64 = 100
	err := service.DeleteExportRule(fsID, "10.20.30.40")
	// Assert
	assert.NotNil(suite.T(), err, "Error should not be nil")
}

func (suite *ApiTestSuite) Test_DeleteExportRule_success() {
	var exportRespArry []ExportResponse

	var expoResp ExportResponse
	expoResp.ID = 100
	exportRespArry = append(exportRespArry, expoResp)
	expectedResponse := client.ApiResponse{Result: exportRespArry}

	suite.clientMock.On("Get").Return(expectedResponse, nil)

	expectedErr := errors.New("some error")
	suite.clientMock.On("Get").Return(nil, expectedErr)

	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	var fsID int64 = 100
	err := service.DeleteExportRule(fsID, "10.20.30.40")
	// Assert
	assert.Nil(suite.T(), err, "Error should not be nil")
}

func (suite *ApiTestSuite) Test_DeleteNodeFromExport_Error() {
	expectedErr := errors.New("some error")
	suite.clientMock.On("Get").Return(nil, expectedErr)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	_, err := service.DeleteNodeFromExport(100, "RW", false, "10.20.30.40")
	// Assert
	assert.NotNil(suite.T(), err, "Error should not be nil")
}

func (suite *ApiTestSuite) Test_DeleteNodeFromExport_already_deleted() {
	exportResp := ExportResponse{}
	exportResp.ID = 1009

	permissionsArry := []Permissions{}
	permissions := Permissions{}
	permissions.Access = "RW"
	permissions.Client = "10.20.30.41"
	permissions.NoRootSquash = false
	permissionsArry = append(permissionsArry, permissions)

	exportResp.Permissions = append(exportResp.Permissions, permissionsArry...)

	expectedResponse := client.ApiResponse{Result: exportResp}

	suite.clientMock.On("Get").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	_, err := service.DeleteNodeFromExport(100, "RW", false, "10.20.30.40")
	// Assert
	assert.Nil(suite.T(), err, "Error should not be nil")
}

func (suite *ApiTestSuite) Test_DeleteNodeFromExport_updateError() {
	exportResp := ExportResponse{}
	exportResp.ID = 1009

	permissionsArry := []Permissions{}
	permissions := Permissions{}
	permissions.Access = "RW"
	permissions.Client = "10.20.30.40"
	permissions.NoRootSquash = false
	permissionsArry = append(permissionsArry, permissions)

	exportResp.Permissions = append(exportResp.Permissions, permissionsArry...)

	expectedResponse := client.ApiResponse{Result: exportResp}

	suite.clientMock.On("Get").Return(expectedResponse, nil)
	expectedErr := errors.New("some error")
	suite.clientMock.On("Put").Return(nil, expectedErr)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	_, err := service.DeleteNodeFromExport(100, "RW", false, "10.20.30.40")
	// Assert
	assert.NotNil(suite.T(), err, "Error should not be nil")
}

func (suite *ApiTestSuite) Test_DeleteNodeFromExport_update_success() {
	exportResp := ExportResponse{}
	exportResp.ID = 1009

	permissionsArry := []Permissions{}
	permissions := Permissions{}
	permissions.Access = "RW"
	permissions.Client = "10.20.30.40"
	permissions.NoRootSquash = false
	permissionsArry = append(permissionsArry, permissions)

	exportResp.Permissions = append(exportResp.Permissions, permissionsArry...)

	expectedResponse := client.ApiResponse{Result: exportResp}

	suite.clientMock.On("Get").Return(expectedResponse, nil)

	response := client.ApiResponse{Result: ExportResponse{}}
	suite.clientMock.On("Put").Return(response, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	_, err := service.DeleteNodeFromExport(100, "RW", false, "10.20.30.40")
	// Assert
	assert.Nil(suite.T(), err, "Error should not be nil")
}

func (suite *ApiTestSuite) Test_GetFileSystemCountByPoolID_success() {
	expectedResponse := client.ApiResponse{Result: getFilesystemArry(), MetaData: client.Resultmetadata{NoOfObject: 100}}
	suite.clientMock.On("Get").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	// Act
	var poolID int64 = 1
	response, err := service.GetFileSystemCountByPoolID(poolID)
	// Assert
	assert.Nil(suite.T(), err, "Response should not be nil")
	assert.Equal(suite.T(), 100, response, "response should be nil")
}

func (suite *ApiTestSuite) Test_GetFileSystemCountByPoolID_Error() {
	expectedErr := errors.New("some error")
	suite.clientMock.On("Get").Return(nil, expectedErr)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	// Act
	var poolID int64 = 1
	_, err := service.GetFileSystemCountByPoolID(poolID)
	// Assert
	assert.NotNil(suite.T(), err, "Response should not be nil")
}

func (suite *ApiTestSuite) Test_GetTreeqSizeByFileSystemID_success() {
	treeqArr := []Treeq{}
	tq := Treeq{}
	tq.ID = 111
	tq.Name = "treeqName"
	tq.HardCapacity = 100
	treeqArr = append(treeqArr, tq)

	expectedResponse := client.ApiResponse{Result: treeqArr}
	suite.clientMock.On("Get").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	// Act
	var filesystemID int64 = 100
	_, err := service.GetTreeqSizeByFileSystemID(filesystemID)
	// Assert
	assert.Nil(suite.T(), err, "Response should not be nil")
	// assert.Equal(suite.T(), 100, response, "response should be nil")
}

func (suite *ApiTestSuite) Test_GetTreeqSizeByFileSystemID_Error() {
	// expectedResponse := client.ApiResponse{Result: treeqArr}
	expecteErr := errors.New("some Error")
	suite.clientMock.On("Get").Return(nil, expecteErr)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	// Act
	var filesystemID int64 = 100
	_, err := service.GetTreeqSizeByFileSystemID(filesystemID)
	// Assert
	assert.NotNil(suite.T(), err, "Response should not be nil")
	// assert.Equal(suite.T(), 100, response, "response should be nil")
}

func (suite *ApiTestSuite) Test_GetTreeqByName_Error() {
	expecteErr := errors.New("some Error")
	suite.clientMock.On("GetWithQueryString").Return(nil, expecteErr)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	// Act
	var filesystemID int64 = 100
	_, err := service.GetTreeqByName(filesystemID, "treeqName")
	// Assert
	assert.NotNil(suite.T(), err, "Response should not be nil")
}

func (suite *ApiTestSuite) Test_GetTreeqByName_success() {
	treeqArr := []Treeq{}
	tq := Treeq{}
	tq.ID = 111
	tq.Name = "treeqName"
	tq.HardCapacity = 100
	treeqArr = append(treeqArr, tq)

	expectedResponse := client.ApiResponse{Result: treeqArr}
	suite.clientMock.On("GetWithQueryString").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	// Act
	var filesystemID int64 = 100
	_, err := service.GetTreeqByName(filesystemID, "treeqName")
	// Assert
	assert.Nil(suite.T(), err, "Response should not be nil")
	// assert.Equal(suite.T(), 100, response, "response should be nil")
}

//=============

func getExportResponse() *[]ExportResponse {
	exportRespArry := []ExportResponse{}

	exportResp := ExportResponse{}
	exportResp.ID = 100
	exportResp.ExportPath = "/exportPath"
	return &exportRespArry
}

func setSecret() map[string]string {
	secretMap := make(map[string]string)
	secretMap["username"] = "admin"
	secretMap["password"] = "123456"
	secretMap["hostname"] = "https://172.17.35.61/"
	return secretMap
}

func getFilesystemArry() []FileSystem {
	var filesystems []FileSystem
	fs1 := FileSystem{}
	fs1.PoolID = 1
	fs1.Size = 10000
	fs2 := FileSystem{}
	fs2.PoolID = 1
	fs2.Size = 10000
	filesystems = append(filesystems, fs1, fs2)
	return filesystems
}

func getMetaData() client.Resultmetadata {
	metaData := client.Resultmetadata{}
	metaData.NoOfObject = 100
	metaData.Page = 1
	metaData.PageSize = 50
	metaData.TotalPages = 2
	return metaData
}

func getLunMetaData100() client.Resultmetadata {
	metaData := client.Resultmetadata{}
	metaData.NoOfObject = 100
	metaData.Page = 1
	metaData.PageSize = 1000
	metaData.TotalPages = 1
	return metaData
}

func getLunMetaData1010Pg1() client.Resultmetadata {
	metaData := client.Resultmetadata{}
	metaData.NoOfObject = 1010
	metaData.Page = 1
	metaData.PageSize = 1000
	metaData.TotalPages = 2
	return metaData
}

func getLunMetaData1010Pg2() client.Resultmetadata {
	metaData := client.Resultmetadata{}
	metaData.NoOfObject = 1010
	metaData.Page = 1
	metaData.PageSize = 1000
	metaData.TotalPages = 2
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
