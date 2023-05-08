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
	"context"
	"errors"
	"fmt"
	"infinibox-csi-driver/api"
	"infinibox-csi-driver/helper"
	tests "infinibox-csi-driver/test_helper"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

func (suite *TreeqControllerSuite) SetupTest() {
	suite.nfsMountMock = new(MockNfsMounter)
	suite.storageHelperMock = new(MockStorageHelper)
	suite.osHelperMock = new(helper.MockOsHelper)
	suite.filesystem = new(FileSystemInterfaceMock)
	suite.api = new(api.MockApiService)
	suite.cs = &Commonservice{Api: suite.api}

	tests.ConfigureKlog()
}

type TreeqControllerSuite struct {
	suite.Suite
	osHelperMock      *helper.MockOsHelper
	filesystem        *FileSystemInterfaceMock
	api               *api.MockApiService
	cs                *Commonservice
	storageHelperMock *MockStorageHelper
	nfsMountMock      *MockNfsMounter
}

// func (suite *TreeqControllerSuite) Test_CreateVolume_validation() {
// 	mapParameter := make(map[string]string)
// 	mapParameter["pool_name"] = "invalid poolName"
// 	service := treeqstorage{filesysService: suite.filesystem}
// 	_, err := service.CreateVolume(context.Background(), getCreateVolumeRequest())
// 	assert.NotNil(suite.T(), err, "empty error")
// }

func (suite *TreeqControllerSuite) Test_CreateVolume_Error() {
	nfs := nfsstorage{storageHelper: suite.storageHelperMock, cs: *suite.cs, mounter: suite.nfsMountMock, osHelper: suite.osHelperMock}
	service := treeqstorage{treeqService: suite.filesystem, nfsstorage: nfs}

	//mapParameter := make(map[string]string)
	volumeResponse := make(map[string]string)
	expectedErr := errors.New("some error")

	//networkSpaceErr := errors.New("Some error")
	networkSpace := getTreeQTestNetworkSpace()
	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(networkSpace, nil)

	suite.filesystem.On("IsTreeqAlreadyExist", mock.Anything, mock.Anything, mock.Anything).Return(volumeResponse, nil)
	suite.filesystem.On("CreateTreeqVolume", mock.Anything, mock.Anything, mock.Anything).Return(volumeResponse, expectedErr)
	_, err := service.CreateVolume(context.Background(), getCreateVolumeRequest())
	assert.NotNil(suite.T(), err, "empty error")
}

func (suite *TreeqControllerSuite) Test_CreateVolume_Success() {
	nfs := nfsstorage{storageHelper: suite.storageHelperMock, cs: *suite.cs, mounter: suite.nfsMountMock, osHelper: suite.osHelperMock}
	service := treeqstorage{treeqService: suite.filesystem, nfsstorage: nfs}

	volumeResponse := getCreateVolumeResponse()
	volumeRespoance := make(map[string]string)
	volumeRespoance["ID"] = "100"
	volumeRespoance["TREEQID"] = "200"
	networkSpace := getTreeQTestNetworkSpace()

	suite.filesystem.On("IsTreeqAlreadyExist", mock.Anything, mock.Anything, mock.Anything).Return(volumeRespoance, nil)
	suite.filesystem.On("CreateTreeqVolume", mock.Anything, mock.Anything, mock.Anything).Return(volumeResponse, nil)
	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(networkSpace, nil)

	result, err := service.CreateVolume(context.Background(), getCreateVolumeRequest())
	assert.Nil(suite.T(), err, "empty error")
	correctVolId := fmt.Sprintf("%s#%s", volumeRespoance["ID"], volumeRespoance["TREEQID"])
	val := result.GetVolume()
	assert.Equal(suite.T(),
		correctVolId,
		val.GetVolumeId(),
		"ID shoulde be equal")
}

func (suite *TreeqControllerSuite) Test_DeleteVolume_VolumeID_empty() {
	service := treeqstorage{treeqService: suite.filesystem}
	_, err := service.DeleteVolume(context.Background(), getDeleteVolumeRequest(""))
	assert.NotNil(suite.T(), err, "Volume ID missing in request")
}

func (suite *TreeqControllerSuite) Test_DeleteVolume_InvalidVolumeID() {
	service := treeqstorage{treeqService: suite.filesystem}
	volumeID := "100"
	_, err := service.DeleteVolume(context.Background(), getDeleteVolumeRequest(volumeID))
	assert.NotNil(suite.T(), err, "Volume ID missing in request")
}

func (suite *TreeqControllerSuite) Test_DeleteVolume_Error() {
	service := treeqstorage{treeqService: suite.filesystem}
	volumeID := "100#200"
	expectedErr := errors.New("Some error")
	var filesytemID, treeqID int64 = 100, 200
	suite.filesystem.On("DeleteTreeqVolume", filesytemID, treeqID).Return(expectedErr)
	_, err := service.DeleteVolume(context.Background(), getDeleteVolumeRequest(volumeID))
	assert.NotNil(suite.T(), err, "error expected")
}

func (suite *TreeqControllerSuite) Test_DeleteVolume_Error_filenotfound() {
	service := treeqstorage{treeqService: suite.filesystem}
	volumeID := "100#200$$"
	expectedErr := errors.New("FILESYSTEM_NOT_FOUND error")
	var filesytemID, treeqID int64 = 100, 200
	suite.filesystem.On("DeleteTreeqVolume", filesytemID, treeqID).Return(expectedErr)
	_, err := service.DeleteVolume(context.Background(), getDeleteVolumeRequest(volumeID))
	assert.Nil(suite.T(), err, "error Not expected")
}

func (suite *TreeqControllerSuite) Test_DeleteVolume_success() {
	service := treeqstorage{treeqService: suite.filesystem}
	volumeID := "100#200$$"
	var filesytemID, treeqID int64 = 100, 200
	suite.filesystem.On("DeleteTreeqVolume", filesytemID, treeqID).Return(nil)
	resp, err := service.DeleteVolume(context.Background(), getDeleteVolumeRequest(volumeID))
	assert.Nil(suite.T(), err, "error Not expected")
	assert.NotNil(suite.T(), resp, "response should not be nil")
}

func (suite *TreeqControllerSuite) Test_ControllerExpandVolume_VolumeID_empty() {
	service := treeqstorage{treeqService: suite.filesystem}
	_, err := service.ControllerExpandVolume(context.Background(), getExpandVolumeRequest(""))
	assert.NotNil(suite.T(), err, "Volume ID missing in request")
}

func (suite *TreeqControllerSuite) Test_ControllerExpandVolume_InvalidVolumeID() {
	service := treeqstorage{treeqService: suite.filesystem}
	volumeID := "100"
	_, err := service.ControllerExpandVolume(context.Background(), getExpandVolumeRequest(volumeID))
	assert.NotNil(suite.T(), err, "Volume ID missing in request")
}

func (suite *TreeqControllerSuite) Test_ControllerExpandVolume_Error() {
	service := treeqstorage{treeqService: suite.filesystem}
	volumeID := "100#200"
	expectedErr := errors.New("Some error")
	var filesytemID, treeqID, capacity int64 = 100, 200, 1073741824
	maxSize := ""
	suite.filesystem.On("UpdateTreeqVolume", filesytemID, treeqID, capacity, maxSize).Return(expectedErr)
	_, err := service.ControllerExpandVolume(context.Background(), getExpandVolumeRequest(volumeID))
	assert.NotNil(suite.T(), err, "error expected")
}

func (suite *TreeqControllerSuite) Test_ControllerExpandVolume_Error_filenotfound() {
	service := treeqstorage{treeqService: suite.filesystem}
	volumeID := "100#200$$"
	var filesytemID, treeqID, capacity int64 = 100, 200, 1073741824
	maxSize := ""
	suite.filesystem.On("UpdateTreeqVolume", filesytemID, treeqID, capacity, maxSize).Return(nil)
	_, err := service.ControllerExpandVolume(context.Background(), getExpandVolumeRequest(volumeID))
	assert.Nil(suite.T(), err, "error Not expected")
}

func (suite *TreeqControllerSuite) Test_ControllerExpandVolume_success() {
	service := treeqstorage{treeqService: suite.filesystem}
	volumeID := "100#200$$"
	var filesytemID, treeqID, capacity int64 = 100, 200, 1073741824
	maxSize := ""
	suite.filesystem.On("UpdateTreeqVolume", filesytemID, treeqID, capacity, maxSize).Return(nil)
	resp, err := service.ControllerExpandVolume(context.Background(), getExpandVolumeRequest(volumeID))
	assert.Nil(suite.T(), err, "error Not expected")
	assert.NotNil(suite.T(), resp, "response should not be nil")
}

func TestTreeqControllerSuite(t *testing.T) {
	suite.Run(t, new(TreeqControllerSuite))
}

// test data

func getExpandVolumeRequest(vID string) *csi.ControllerExpandVolumeRequest {
	return &csi.ControllerExpandVolumeRequest{
		VolumeId: vID,
	}
}

func getCreateVolumeResponse() map[string]string {
	result := make(map[string]string)
	result["ID"] = "100"
	result["TREEQID"] = "200"
	return result
}

func getDeleteVolumeRequest(vID string) *csi.DeleteVolumeRequest {
	return &csi.DeleteVolumeRequest{
		VolumeId: vID,
	}
}

// mock method
type FileSystemInterfaceMock struct {
	mock.Mock
}

func (m *FileSystemInterfaceMock) CreateTreeqVolume(config map[string]string, capacity int64, pvName string) (map[string]string, error) {
	status := m.Called(config, capacity, pvName)
	st, _ := status.Get(0).(map[string]string)
	err, _ := status.Get(1).(error)
	return st, err
}

func (m *FileSystemInterfaceMock) DeleteTreeqVolume(filesystemID, treeqID int64) error {
	status := m.Called(filesystemID, treeqID)
	st, _ := status.Get(0).(error)
	return st
}

func (m *FileSystemInterfaceMock) UpdateTreeqVolume(filesystemID, treeqID, capacity int64, maxSize string) error {
	status := m.Called(filesystemID, treeqID, capacity, maxSize)
	err, _ := status.Get(0).(error)
	return err
}

func (m *FileSystemInterfaceMock) IsTreeqAlreadyExist(pool_name, network_space, pVName string) (map[string]string, error) {
	status := m.Called(pool_name, network_space, pVName)
	st, _ := status.Get(0).(map[string]string)
	err, _ := status.Get(1).(error)
	return st, err
}
