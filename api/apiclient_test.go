package api

import (
	"errors"
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
	sp.Name = "name1"
	sp.ID = 1
	storagePool = append(storagePool, sp)
	expectedError := errors.New("No such pool: test_storage_pool")
	suite.clientMock.On("GetWithQueryString").Return(&storagePool, nil)
	suite.serviceMock.On("getJSONResponse").Return(expectedError)
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
	sp.Name = "name1"
	sp.ID = 1
	storagePools = append(storagePools, sp)
	vol := &Volume{}
	suite.clientMock.On("GetWithQueryString").Return(storagePools, nil)
	suite.clientMock.On("Post").Return(vol, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	volume := VolumeParam{Name: "test_volume", PoolId: 1000, VolumeSize: 1000000000, ProvisionType: "THIN"}
	response, _ := service.CreateVolume(&volume, "name1")

	// Assert
	assert.NotNil(suite.T(), response, "Response should not be nil")
	assert.Equal(suite.T(), vol, response, "Response not returned as expected")
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
	sp.Name = ""
	sp.ID = 0
	storagePools = append(storagePools, sp)
	suite.clientMock.On("GetWithQueryString").Return(storagePools, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	response, _ := service.GetStoragePool(1001, "test_storage_pool")

	// Assert
	assert.NotNil(suite.T(), response, "Response should not be nil")
	assert.Equal(suite.T(), storagePools, response, "Response not returned as expected")
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
	volume := Volume{}
	volume.Name = "test_storage_pool"
	volume.ID = 0
	volumes = append(volumes, volume)
	suite.clientMock.On("GetWithQueryString").Return(volumes, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	response, _ := service.GetVolumeByName("test_storage_pool")

	// Assert
	assert.NotNil(suite.T(), response, "Response should not be nil")
	assert.Equal(suite.T(), &volume, response, "Response not returned as expected")
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
	expectedResponse := &SnapshotVolumesResp{}
	expectedResponse.SnapshotGroupID = ""
	suite.clientMock.On("Post").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	snapshotParams := SnapshotDef{ParentID: 1001, SnapshotName: "test_volume_resp"}
	response, _ := service.CreateSnapshotVolume(&snapshotParams)

	// Assert
	assert.NotNil(suite.T(), response, "Response should not be nil")
	assert.Equal(suite.T(), expectedResponse, response, "Response not returned as expected")
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
	volume.Name = ""
	volume.ID = 0
	volumes = append(volumes, volume)
	suite.clientMock.On("Get").Return(volumes, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	response, _ := service.GetVolume(101)

	// Assert
	assert.NotNil(suite.T(), response, "Response should not be nil")
	assert.Equal(suite.T(), volumes, response, "Response not returned as expected")
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
	network := NetworkSpace{}
	network.Name = ""
	network.ID = 0
	//networks = append(networks, network)
	suite.clientMock.On("GetWithQueryString").Return(network, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	response, _ := service.GetNetworkSpaceByName("test_network_space")

	// Assert
	assert.NotNil(suite.T(), response, "Response should not be nil")
	assert.Equal(suite.T(), network, response, "Response not returned as expected")
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
	hostResp := Host{}
	hostResp.ID = 0
	suite.clientMock.On("GetWithQueryString").Return(hostResp, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	response, _ := service.GetHostByName("test_host")

	// Assert
	assert.NotNil(suite.T(), response, "Response should not be nil")
	assert.Equal(suite.T(), hostResp, response, "Response not returned as expected")
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
	luninfo := LunInfo{}
	luninfo.ID = 0
	suite.clientMock.On("Post").Return(luninfo, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	response, _ := service.MapVolumeToHost(1001, 2)

	// Assert
	assert.NotNil(suite.T(), response, "Response should not be nil")
	assert.Equal(suite.T(), luninfo, response, "Response not returned as expected")
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
	expectedResponse := &FileSystem{}
	expectedResponse.Size = 0
	suite.clientMock.On("Put").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	// Act
	fileSystem := FileSystem{Size: 100}
	response, _ := service.UpdateFilesystem(1001, fileSystem)

	// Assert
	assert.NotNil(suite.T(), response, "Response should not be nil")
	assert.Equal(suite.T(), expectedResponse, response, "Response not returned as expected")
}

func setSecret() map[string]string {
	secretMap := make(map[string]string)
	secretMap["username"] = "admin"
	secretMap["password"] = "123456"
	secretMap["hosturl"] = "https://172.17.35.61/"
	return secretMap
}
