package api

import (
	"errors"
	"infinibox-csi-driver/api/client"
	"testing"

	"github.com/stretchr/testify/assert"
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
	expectedError := errors.New("volume with given name not found")

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
	volume := VolumeParam{Name: "test_volume",
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
	expectedError := errors.New("volume with given name not found")
	suite.clientMock.On("GetWithQueryString").Return(nil, expectedError)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	_, err := service.GetStoragePoolIDByName("test_storage_pool")

	// Assert
	assert.NotNil(suite.T(), err, "Error should not be nil")
	assert.Equal(suite.T(), expectedError, err, "Error not returned as expected")
}

func (suite *ApiTestSuite) Test_GetStoragePoolIDByName_Success() {
	var poolID int64
	suite.clientMock.On("GetWithQueryString").Return(poolID, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	response, _ := service.GetStoragePoolIDByName("test_storage_pool")

	// Assert
	assert.NotNil(suite.T(), response, "Response should not be nil")
	assert.Equal(suite.T(), poolID, response, "Response not returned as expected")
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
	//expectedResponse := &SnapshotVolumesResp{}
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
	// Test volume will be created
	volumes := []Volume{}
	volume := Volume{}
	volumes = append(volumes, volume)
	expectedResponse := client.ApiResponse{Result: volumes}
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
	//networks := []NetworkSpace{}
	expectedResponse := client.ApiResponse{Result: NetworkSpace{}}

	//networks = append(networks, network)
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
	_, err := service.MapVolumeToHost(1, 2)

	// Assert
	assert.NotNil(suite.T(), err, "Error should not be nil")
	assert.Equal(suite.T(), expectedError, err, "Error not returned as expected")
}

func (suite *ApiTestSuite) Test_MapVolumeToHost_Success() {
	expectedResponse := client.ApiResponse{Result: LunInfo{HostClusterID: 0, VolumeID: 0, CLustered: false, HostID: 0, ID: 0, Lun: 0}}

	suite.clientMock.On("Post").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	response, _ := service.MapVolumeToHost(1001, 2)

	// Assert
	assert.NotNil(suite.T(), response, "Response should not be nil")
	assert.Equal(suite.T(), expectedResponse.Result, response, "Response not returned as expected")
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
	// Test volume snapshot will be created
	//expectedResponse := &FileSystem{}
	// expectedResponse.Size = 0
	expectedResponse := client.ApiResponse{Result: &FileSystem{ID: 0, PoolID: 0, Name: "", SsdEnabled: false, Provtype: "", Size: 0, ParentID: 0, PoolName: "", CreatedAt: 0}}
	suite.clientMock.On("Delete").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	// Act
	response, _ := service.DeleteFileSystem(1001)

	// Assert
	assert.NotNil(suite.T(), response, "Response should not be nil")
	assert.Equal(suite.T(), expectedResponse.Result, response, "Response not returned as expected")
}

func (suite *ApiTestSuite) Test_ExportFileSystem_Fail() {
	// Test volume snapshot will not be created
	expectedError := errors.New("Missing parameters")
	suite.clientMock.On("Post").Return(nil, expectedError)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	exportfileSys := ExportFileSys{}
	_, err := service.ExportFileSystem(exportfileSys)

	// Assert
	assert.NotNil(suite.T(), err, "Error should not be nil")
	assert.Equal(suite.T(), expectedError, err, "Error not returned as expected")
}

func (suite *ApiTestSuite) Test_ExportFileSystem_Success() {
	expectedResponse := client.ApiResponse{Result: &ExportResponse{}}

	suite.clientMock.On("Post").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	exportfileSys := ExportFileSys{
		FilesystemID: 1000,
	}

	// Act
	response, _ := service.ExportFileSystem(exportfileSys)

	// Assert
	assert.NotNil(suite.T(), response, "Response should not be nil")
	assert.Equal(suite.T(), expectedResponse.Result, response, "Response not returned as expected")
}

func (suite *ApiTestSuite) Test_DeleteExportPath_Fail() {
	// Test volume snapshot will not be created
	expectedError := errors.New("Not found export path id")
	suite.clientMock.On("Delete").Return(nil, expectedError)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	_, err := service.DeleteExportPath(1001)

	// Assert
	assert.NotNil(suite.T(), err, "Error should not be nil")
	assert.Equal(suite.T(), expectedError, err, "Error not returned as expected")
}

func (suite *ApiTestSuite) Test_DeleteExportPath_Success() {
	expectedResponse := client.ApiResponse{Result: &ExportResponse{}}
	suite.clientMock.On("Delete").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	// Act
	response, _ := service.DeleteExportPath(1001)

	// Assert
	assert.NotNil(suite.T(), response, "Response should not be nil")
	assert.Equal(suite.T(), expectedResponse.Result, response, "Response not returned as expected")
}

func (suite *ApiTestSuite) Test_AttachMetadataToObject_Fail() {
	expectedError := errors.New("Unable to get given object")
	suite.clientMock.On("Put").Return(nil, expectedError)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	body := map[string]interface{}{"key": "value"}
	_, err := service.AttachMetadataToObject(1001, body)

	// Assert
	assert.NotNil(suite.T(), err, "Error should not be nil")
	assert.Equal(suite.T(), expectedError, err, "Error not returned as expected")
}

func (suite *ApiTestSuite) Test_AttachMetadataToObject_Success() {
	var metadatas []Metadata
	expectedResponse := client.ApiResponse{Result: &metadatas}
	suite.clientMock.On("Put").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	body := map[string]interface{}{"key": "value"}
	response, _ := service.AttachMetadataToObject(1001, body)

	// Assert
	assert.NotNil(suite.T(), response, "Response should not be nil")
	assert.Equal(suite.T(), expectedResponse.Result, response, "Response not returned as expected")
}

func (suite *ApiTestSuite) Test_DetachMetadataFromObject_Fail() {
	expectedError := errors.New("Unable to get given object")
	suite.clientMock.On("Delete").Return(nil, expectedError)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	_, err := service.DetachMetadataFromObject(1001)

	// Assert
	assert.NotNil(suite.T(), err, "Error should not be nil")
	assert.Equal(suite.T(), expectedError, err, "Error not returned as expected")
}

func (suite *ApiTestSuite) Test_DetachMetadataFromObject_Success() {
	var metadatas []Metadata
	expectedResponse := client.ApiResponse{Result: &metadatas}
	suite.clientMock.On("Delete").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	response, _ := service.DetachMetadataFromObject(1001)

	// Assert
	assert.NotNil(suite.T(), response, "Response should not be nil")
	assert.Equal(suite.T(), expectedResponse.Result, response, "Response not returned as expected")
}

func (suite *ApiTestSuite) Test_CreateFilesystem_Fail() {
	// Test volume snapshot will not be created
	expectedError := errors.New("Missing parameters")
	suite.clientMock.On("Post").Return(nil, expectedError)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	var ID int64 = 7800
	body := map[string]interface{}{"PoolID": ID}
	_, err := service.CreateFilesystem(body)

	// Assert
	assert.NotNil(suite.T(), err, "Error should not be nil")
	assert.Equal(suite.T(), expectedError, err, "Error not returned as expected")
}

func (suite *ApiTestSuite) Test_CreateFilesystem_Success() {
	expectedResponse := client.ApiResponse{Result: &FileSystem{}}
	suite.clientMock.On("Post").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	// Act
	var ID int64 = 7800
	body := map[string]interface{}{"PoolID": ID}
	response, _ := service.CreateFilesystem(body)

	// Assert
	assert.NotNil(suite.T(), response, "Response should not be nil")
	assert.Equal(suite.T(), expectedResponse.Result, response, "Response not returned as expected")
}

func (suite *ApiTestSuite) Test_GetFileSystemCount_Fail() {
	expectedError := errors.New("Failed to get total number of filesystem")
	suite.clientMock.On("Get").Return(nil, expectedError)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	_, err := service.GetFileSystemCount()

	// Assert
	assert.NotNil(suite.T(), err, "Error should not be nil")
	assert.Equal(suite.T(), expectedError, err, "Error not returned as expected")
}

func (suite *ApiTestSuite) Test_GetFileSystemCount_Success() {
	var count int
	suite.clientMock.On("Get").Return(count, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	response, _ := service.GetFileSystemCount()

	// Assert
	assert.NotNil(suite.T(), response, "Response should not be nil")
	assert.Equal(suite.T(), count, response, "Response not returned as expected")
}

func (suite *ApiTestSuite) Test_AddNodeInExport_Fail() {
	// Test volume snapshot will not be created
	expectedError := errors.New("Missing parameters")
	suite.clientMock.On("Get").Return(nil, expectedError)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	_, err := service.AddNodeInExport(1001, "RW", true, "172.19.19.19")

	// Assert
	assert.NotNil(suite.T(), err, "Error should not be nil")
	assert.Equal(suite.T(), expectedError, err, "Error not returned as expected")
}

func (suite *ApiTestSuite) Test_AddNodeInExport_Success() {
	expectedResponse := client.ApiResponse{Result: &ExportResponse{}}

	suite.clientMock.On("Get").Return(expectedResponse, nil)
	suite.clientMock.On("Put").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	response, _ := service.AddNodeInExport(1001, "RW", true, "172.19.19.19")

	// Assert
	assert.NotNil(suite.T(), response, "Response should not be nil")
	assert.Equal(suite.T(), expectedResponse.Result, response, "Response not returned as expected")
}

func (suite *ApiTestSuite) Test_GetParentID_Fail() {
	// Test volume snapshot will not be created
	var expectedError int64 = 0
	suite.clientMock.On("Get").Return(nil, expectedError)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	err := service.GetParentID(1001)

	// Assert
	assert.NotNil(suite.T(), err, "Error should not be nil")
	assert.Equal(suite.T(), expectedError, err, "Error not returned as expected")
}

func (suite *ApiTestSuite) Test_GetParentID_Success() {
	var expectedResponse int64 = 0

	suite.clientMock.On("Get").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	response := service.GetParentID(1001)

	// Assert
	assert.NotNil(suite.T(), response, "Response should not be nil")
	assert.Equal(suite.T(), expectedResponse, response, "Response not returned as expected")
}

func (suite *ApiTestSuite) Test_GetFileSystemByID_Fail() {
	expectedError := errors.New("Missing parameters")
	suite.clientMock.On("Get").Return(nil, expectedError)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	_, err := service.GetFileSystemByID(1001)

	// Assert
	assert.NotNil(suite.T(), err, "Error should not be nil")
	assert.Equal(suite.T(), expectedError, err, "Error not returned as expected")
}

func (suite *ApiTestSuite) Test_GetFileSystemByID_Success() {
	// Test volume snapshot will be created
	expectedResponse := client.ApiResponse{Result: &FileSystem{}}

	suite.clientMock.On("Get").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	response, _ := service.GetFileSystemByID(1001)

	// Assert
	assert.NotNil(suite.T(), response, "Response should not be nil")
	assert.Equal(suite.T(), expectedResponse.Result, response, "Response not returned as expected")
}

func (suite *ApiTestSuite) Test_GetFileSystemByName_Fail() {
	expectedError := errors.New("Missing parameters")
	suite.clientMock.On("GetWithQueryString").Return(nil, expectedError)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	_, err := service.GetFileSystemByName("test_filesystem")

	// Assert
	assert.NotNil(suite.T(), err, "Error should not be nil")
	assert.Equal(suite.T(), expectedError, err, "Error not returned as expected")
}

func (suite *ApiTestSuite) Test_GetFileSystemByName_Success() {
	var filesystem *FileSystem
	expectedResponse := client.ApiResponse{Result: filesystem}

	suite.clientMock.On("GetWithQueryString").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	response, _ := service.GetFileSystemByName("test_filesystem")

	// Assert
	//assert.NotNil(suite.T(), response, "Response should not be nil")
	assert.Equal(suite.T(), expectedResponse.Result, response, "Response not returned as expected")
}

func (suite *ApiTestSuite) Test_GetMetadataStatus_Fail() {
	// Test volume snapshot will not be created
	var expectedError bool
	suite.clientMock.On("Get").Return(nil, expectedError)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	err := service.GetMetadataStatus(1001)

	// Assert
	assert.NotNil(suite.T(), err, "Error should not be nil")
	assert.Equal(suite.T(), expectedError, err, "Error not returned as expected")
}

func (suite *ApiTestSuite) Test_GetMetadataStatus_Success() {
	var expectedResponse bool

	suite.clientMock.On("Get").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	response := service.GetMetadataStatus(1001)

	// Assert
	assert.NotNil(suite.T(), response, "Response should not be nil")
	assert.Equal(suite.T(), expectedResponse, response, "Response not returned as expected")
}

func (suite *ApiTestSuite) Test_DeleteExportRule_Fail() {
	// Test volume snapshot will not be created
	expectedError := errors.New("Missing parameters")
	suite.clientMock.On("Get").Return(nil, expectedError)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	err := service.DeleteExportRule(1001, "172.17.17.17")

	// Assert
	assert.NotNil(suite.T(), err, "Error should not be nil")
	assert.Equal(suite.T(), expectedError, err, "Error not returned as expected")
}

func (suite *ApiTestSuite) Test_DeleteExportRule_Success() {
	suite.clientMock.On("Get").Return(nil, nil)
	suite.clientMock.On("Delete").Return(nil, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	err := service.DeleteExportRule(1001, "172.17.17.17")

	// Assert
	assert.Equal(suite.T(), nil, err, "Response not returned as expected")
}

func (suite *ApiTestSuite) Test_FileSystemHasChildForDeleteParentFileSystem_Fail() {
	// Configure
	suite.clientMock.On("GetWithQueryString").Return(nil, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	// Act
	err := service.DeleteParentFileSystem(1001)

	// Assert
	assert.Equal(suite.T(), nil, err, "Error not returned as expected")
}

func (suite *ApiTestSuite) Test_GetParentIDForDeleteParentFileSystem_Fail() {
	suite.clientMock.On("getJSONResponse").Return(nil)
	service := ClientService{api: suite.clientMock}

	err := service.DeleteParentFileSystem(1001)

	// Assert
	assert.Equal(suite.T(), nil, err, "Error not returned as expected")
}

func (suite *ApiTestSuite) Test_DeleteFileSystemCompleteForDeleteParentFileSystem_Fail() {
	suite.clientMock.On("getJSONResponse").Return(nil)
	service := ClientService{api: suite.clientMock}

	//Act
	err := service.DeleteParentFileSystem(1001)

	// Assert
	assert.Equal(suite.T(), nil, err, "Error not returned as expected")
}

func (suite *ApiTestSuite) Test_DeleteParentFileSystem_Success() {
	suite.clientMock.On("GetWithQueryString").Return(nil)
	suite.clientMock.On("getJSONResponse").Return(nil)
	suite.serviceMock.On("DeleteFileSystemComplete").Return(nil)
	service := ClientService{api: suite.clientMock}

	// Act
	response := service.DeleteParentFileSystem(1001)

	// Assert
	assert.Equal(suite.T(), nil, response, "Response not returned as expected")
}

func (suite *ApiTestSuite) Test_FileSystemHasChild_Fail() {
	// Test volume snapshot will not be created
	var expectedError bool
	suite.clientMock.On("Get").Return(nil, expectedError)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	err := service.FileSystemHasChild(1001)

	// Assert
	assert.NotNil(suite.T(), err, "Error should not be nil")
	assert.Equal(suite.T(), expectedError, err, "Error not returned as expected")
}

func (suite *ApiTestSuite) Test_FileSystemHasChild_Success() {
	var expectedResponse bool

	suite.clientMock.On("Get").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	response := service.FileSystemHasChild(1001)

	// Assert
	assert.NotNil(suite.T(), response, "Response should not be nil")
	assert.Equal(suite.T(), expectedResponse, response, "Response not returned as expected")
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
	var expectedResponse bool
	suite.clientMock.On("Post").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	// Act
	response, _ := service.RestoreFileSystemFromSnapShot(1001, 1002)

	// Assert
	assert.NotNil(suite.T(), response, "Response should not be nil")
	assert.Equal(suite.T(), expectedResponse, response, "Response not returned as expected")
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

func setSecret() map[string]string {
	secretMap := make(map[string]string)
	secretMap["username"] = "admin"
	secretMap["password"] = "123456"
	secretMap["hosturl"] = "https://172.17.35.61/"
	return secretMap
}
