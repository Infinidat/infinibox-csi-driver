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
