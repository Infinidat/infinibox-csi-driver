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
	expectedError := errors.New("fail to get pool ID from pool Name: test_storage_pool")

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
	expectedError := errors.New("fail to get pool ID from pool Name: test_storage_pool")
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
	_, err := service.MapVolumeToHost(1, 2,2)

	// Assert
	assert.NotNil(suite.T(), err, "Error should not be nil")
	assert.Equal(suite.T(), expectedError, err, "Error not returned as expected")
}

func (suite *ApiTestSuite) Test_MapVolumeToHost_Success() {
	expectedResponse := client.ApiResponse{Result: LunInfo{HostClusterID: 0, VolumeID: 0, CLustered: false, HostID: 0, ID: 0, Lun: 0}}

	suite.clientMock.On("Post").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}

	// Act
	response, _ := service.MapVolumeToHost(1001, 2,2)

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
	expectedResponse := client.ApiResponse{Result: &FileSystem{ID: 0, PoolID: 0, Name: "", SsdEnabled: false, Provtype: "", Size: 0, ParentID: 0, PoolName: "", CreatedAt: 0}}
	suite.clientMock.On("Delete").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	// Act
	response, _ := service.DeleteFileSystem(1001)

	// Assert
	assert.NotNil(suite.T(), response, "Response should not be nil")
	assert.Equal(suite.T(), expectedResponse.Result, response, "Response not returned as expected")
}

//****************************************
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
	expectedResponse := client.ApiResponse{MetaData: client.Resultmetadata{NoOfObject:10}}
	suite.clientMock.On("Get").Return(expectedResponse, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	// Act
	response, err := service.GetFilesytemTreeqCount(1001)
	var expectedvalue int = 10
	// Assert
	assert.Nil(suite.T(), err, "Response should not be nil")
	assert.Equal(suite.T(), expectedvalue, response, "Response should not be nil")
}

func (suite *ApiTestSuite) Test_GetFilesytemTreeqCount_panic() {
	suite.clientMock.On("Get").Return(nil, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	// Act
	_, err := service.GetFilesytemTreeqCount(1001)

	// Assert
	assert.NotNil(suite.T(), err, "Response should not be nil")

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
	//expectedResponse := client.ApiResponse{Result: Treeq{ID: 1, FilesystemID: fileSysID, Name: "treeq", Path: "\treeq", HardCapacity: 100}}
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
	//expectedResponse := client.ApiResponse{Result: getFilesystemArry(), MetaData: getMetaData()}
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

func (suite *ApiTestSuite) Test_GetFileSystemsByPoolID_panic() {
	//	expectedResponse := client.ApiResponse{Result: getFilesystem(), MetaData: getMetaData()}
	suite.clientMock.On("Get").Return(nil, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	// Act
	var poolID int64 = 1
	var page int = 1
	_, err := service.GetFileSystemsByPoolID(poolID, page)
	// Assert
	assert.NotNil(suite.T(), err, "Response should not be nil")
}

func (suite *ApiTestSuite) Test_DeleteTreeq_Success() {
	suite.clientMock.On("Delete").Return(nil, nil)
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
	//expectedResponse := client.ApiResponse{Result: Treeq{ID: treeqID, FilesystemID: FilesystemID, HardCapacity: 10000, Name: "treeq1", Path: "/treeqPath", UsedCapacity: 10}}
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
	//expectedResponse := client.ApiResponse{Result: Treeq{ID: treeqID, FilesystemID: FilesystemID, HardCapacity: 10000, Name: "treeq1", Path: "/treeqPath", UsedCapacity: 10}}
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


func (suite *ApiTestSuite) Test_GetFileSystemCountByPoolID_success() {
	expectedResponse := client.ApiResponse{Result: getFilesystemArry(), MetaData: client.Resultmetadata{NoOfObject:100}}
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
	suite.clientMock.On("Get").Return(nil,expectedErr)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	// Act
	var poolID int64 = 1
	_, err := service.GetFileSystemCountByPoolID(poolID)
	// Assert
	assert.NotNil(suite.T(), err, "Response should not be nil")	
}
func (suite *ApiTestSuite) Test_GetFileSystemCountByPoolID_Panic() {
	suite.clientMock.On("Get").Return(nil, nil)
	service := ClientService{api: suite.clientMock, SecretsMap: setSecret()}
	// Act
	var poolID int64 = 1
	_, err := service.GetFileSystemCountByPoolID(poolID)
	// Assert
	assert.NotNil(suite.T(), err, "Response should not be nil")	
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
