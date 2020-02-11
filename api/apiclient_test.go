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
	snapshotParams := SnapshotDef{ParentID: 1001}
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
	snapshotParams := SnapshotDef{ParentID: 1001, SnapshotName: "test_volume_resp"}
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

func setSecret() map[string]string {
	secretMap := make(map[string]string)
	secretMap["username"] = "admin"
	secretMap["password"] = "123456"
	secretMap["hosturl"] = "https://172.17.35.61/"
	return secretMap
}
