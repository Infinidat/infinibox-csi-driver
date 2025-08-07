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
	"github.com/infinidat/infinibox-csi-driver/api"
	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/infinidat/infinibox-csi-driver/helper"
	"github.com/infinidat/infinibox-csi-driver/iboxapi"
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
	host := &iboxapi.Host{
		ID:   1,
		Name: "host1",
	}
	volProto := &api.VolumeProtocolConfig{
		Host:     host,
		VolumeID: 1,
		NodeID:   "node1",
		TreeqID:  1,
	}
	suite.cs = &Commonservice{Api: suite.api, VolProto: volProto}
	suite.someError = errors.New("some error")
	nfs := nfsstorage{storageHelper: suite.storageHelperMock, cs: *suite.cs, mounter: suite.nfsMountMock, osHelper: suite.osHelperMock}
	suite.service = treeqstorage{treeqService: suite.filesystem, nfsstorage: nfs}
}

type TreeqControllerSuite struct {
	suite.Suite
	osHelperMock      *helper.MockOsHelper
	filesystem        *FileSystemInterfaceMock
	api               *api.MockApiService
	cs                *Commonservice
	storageHelperMock *MockStorageHelper
	nfsMountMock      *MockNfsMounter
	someError         error
	service           treeqstorage
}

func (suite *TreeqControllerSuite) Test_ValidateStorageClass_ValidPoolName() {
	parameterMap := getTreeqCreateVolumeParameters()
	err := suite.service.ValidateStorageClass(parameterMap)
	assert.Nil(suite.T(), err, "expected to pass: treeq valid pool name sc parameter")
}
func (suite *TreeqControllerSuite) Test_ValidateStorageClass_InvalidPoolName() {
	parameterMap := map[string]string{
		common.SC_POOL_NAME: "  ",
	}
	err := suite.service.ValidateStorageClass(parameterMap)
	assert.NotNil(suite.T(), err, "expected to fail: treeq invalid pool sc parameter")
}
func (suite *TreeqControllerSuite) Test_ValidateStorageClass_InvalidNetworkSpace() {
	parameterMap := map[string]string{
		common.SC_NETWORK_SPACE: "  ",
	}
	err := suite.service.ValidateStorageClass(parameterMap)
	assert.NotNil(suite.T(), err, "expected to fail: treeq invalid network space sc parameter")
}
func (suite *TreeqControllerSuite) Test_ValidateStorageClass_InvalidParameter_Missing_Pool() {
	parameterMap := getTreeqCreateVolumeParameters()
	delete(parameterMap, common.SC_POOL_NAME)
	err := suite.service.ValidateStorageClass(parameterMap)
	assert.NotNil(suite.T(), err, "expected to fail: treeq missing pool sc parameter")
}
func (suite *TreeqControllerSuite) Test_ValidateStorageClass_InvalidParameter_Missing_Network_Space() {
	parameterMap := getTreeqCreateVolumeParameters()
	delete(parameterMap, common.SC_NETWORK_SPACE)
	err := suite.service.ValidateStorageClass(parameterMap)
	assert.NotNil(suite.T(), err, "expected to fail: treeq missing network space sc parameter")
}
func (suite *TreeqControllerSuite) Test_ValidateStorageClass_Missing_Max_Filesystems() {
	parameterMap := getTreeqCreateVolumeParameters()
	delete(parameterMap, common.SC_MAX_FILESYSTEMS)
	err := suite.service.ValidateStorageClass(parameterMap)
	assert.Nil(suite.T(), err, "expected to pass: treeq missing max filesystems sc parameter")
}
func (suite *TreeqControllerSuite) Test_ValidateStorageClass_Missing_Max_Treeqs() {
	parameterMap := getTreeqCreateVolumeParameters()
	delete(parameterMap, common.SC_MAX_TREEQS_PER_FILESYSTEM)
	err := suite.service.ValidateStorageClass(parameterMap)
	assert.Nil(suite.T(), err, "expected to pass: treeq missing max treeqs sc parameter")
}
func (suite *TreeqControllerSuite) Test_ValidateStorageClass_InvalidParameter_Invalid_GID() {
	parameterMap := getTreeqCreateVolumeParameters()
	parameterMap[common.SC_GID] = "abc"
	err := suite.service.ValidateStorageClass(parameterMap)
	assert.NotNil(suite.T(), err, "expected to fail: treeq invalid gid sc parameter")
}
func (suite *TreeqControllerSuite) Test_ValidateStorageClass_InvalidParameter_Invalid_UID() {
	parameterMap := getTreeqCreateVolumeParameters()
	parameterMap[common.SC_UID] = "abc"
	err := suite.service.ValidateStorageClass(parameterMap)
	assert.NotNil(suite.T(), err, "expected to fail: treeq invalid uid sc parameter")
}

func (suite *TreeqControllerSuite) Test_CreateVolume_Error() {
	volumeResponse := make(map[string]string)
	networkSpace := getTreeQTestNetworkSpace()
	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(networkSpace, nil)

	suite.api.On("OneTimeValidation", mock.Anything, mock.Anything).Return("", nil)
	suite.filesystem.On("IsTreeqAlreadyExist", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(volumeResponse, nil)
	suite.filesystem.On("CreateTreeqVolume", mock.Anything, mock.Anything, mock.Anything).Return(volumeResponse, suite.someError)
	_, err := suite.service.CreateVolume(context.Background(), getCreateVolumeRequest())
	assert.NotNil(suite.T(), err, "empty error")
}

func (suite *TreeqControllerSuite) Test_CreateVolume_Success() {
	suite.service.nfsstorage.cs.VolProto.VolumeID = 100
	suite.service.nfsstorage.cs.VolProto.TreeqID = 200

	volumeResponse := getCreateVolumeResponse()
	volumeResponseMap := map[string]string{
		"ID":      "100",
		"TREEQID": "200",
	}
	networkSpace := getTreeQTestNetworkSpace()

	suite.api.On("OneTimeValidation", mock.Anything, mock.Anything).Return("", nil)
	suite.filesystem.On("IsTreeqAlreadyExist", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(volumeResponseMap, nil)
	suite.filesystem.On("CreateTreeqVolume", mock.Anything, mock.Anything, mock.Anything).Return(volumeResponse, nil)
	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(networkSpace, nil)

	result, err := suite.service.CreateVolume(context.Background(), getCreateVolumeRequest())
	assert.Nil(suite.T(), err, "empty error")
	correctVolId := fmt.Sprintf("%s#%s", volumeResponseMap["ID"], volumeResponseMap["TREEQID"])
	val := result.GetVolume()
	assert.Equal(suite.T(),
		correctVolId,
		val.GetVolumeId(),
		"ID shoulde be equal")
}

func (suite *TreeqControllerSuite) Test_DeleteVolume_VolumeID_empty() {
	suite.service.nfsstorage.cs.VolProto.TreeqID = 200
	suite.service.nfsstorage.cs.VolProto.VolumeID = 100
	var filesytemID, treeqID = 100, 200
	suite.filesystem.On("DeleteTreeqVolume", filesytemID, treeqID).Return(suite.someError)
	_, err := suite.service.DeleteVolume(context.Background(), getDeleteVolumeRequest(""))
	assert.NotNil(suite.T(), err, "Volume ID missing in request")
}

func (suite *TreeqControllerSuite) Test_DeleteVolume_Error() {
	suite.service.nfsstorage.cs.VolProto.TreeqID = 200
	suite.service.nfsstorage.cs.VolProto.VolumeID = 100
	volumeID := "100#200"
	var filesytemID, treeqID = 100, 200
	suite.filesystem.On("DeleteTreeqVolume", filesytemID, treeqID).Return(suite.someError)
	_, err := suite.service.DeleteVolume(context.Background(), getDeleteVolumeRequest(volumeID))
	assert.NotNil(suite.T(), err, "error expected")
}

func (suite *TreeqControllerSuite) Test_DeleteVolume_Error_filenotfound() {
	volumeID := "100#200$$"
	suite.service.nfsstorage.cs.VolProto.TreeqID = 200
	suite.service.nfsstorage.cs.VolProto.VolumeID = 100
	expectedErr := errors.New("FILESYSTEM_NOT_FOUND error")
	var filesytemID, treeqID = 100, 200
	suite.filesystem.On("DeleteTreeqVolume", filesytemID, treeqID).Return(expectedErr)
	_, err := suite.service.DeleteVolume(context.Background(), getDeleteVolumeRequest(volumeID))
	assert.Nil(suite.T(), err, "error Not expected")
}

func (suite *TreeqControllerSuite) Test_DeleteVolume_success() {
	suite.service.nfsstorage.cs.VolProto.TreeqID = 200
	suite.service.nfsstorage.cs.VolProto.VolumeID = 100
	volumeID := "100#200$$"
	var filesytemID, treeqID = 100, 200
	suite.filesystem.On("DeleteTreeqVolume", filesytemID, treeqID).Return(nil)
	resp, err := suite.service.DeleteVolume(context.Background(), getDeleteVolumeRequest(volumeID))
	assert.Nil(suite.T(), err, "error Not expected")
	assert.NotNil(suite.T(), resp, "response should not be nil")
}

func (suite *TreeqControllerSuite) Test_ControllerExpandVolume_VolumeID_empty() {
	_, err := suite.service.ControllerExpandVolume(context.Background(), getExpandVolumeRequest(""))
	assert.NotNil(suite.T(), err, "Volume ID missing in request")
}

func (suite *TreeqControllerSuite) Test_ControllerExpandVolume_InvalidVolumeID() {
	volumeID := "100"
	_, err := suite.service.ControllerExpandVolume(context.Background(), getExpandVolumeRequest(volumeID))
	assert.NotNil(suite.T(), err, "Volume ID missing in request")
}

func (suite *TreeqControllerSuite) Test_ControllerExpandVolume_Error() {
	volumeID := "100#200"
	var filesytemID, treeqID = 100, 200
	var capacity int64 = common.BytesInOneGibibyte
	var maxSize string
	suite.filesystem.On("UpdateTreeqVolume", filesytemID, treeqID, capacity, maxSize).Return(suite.someError)
	_, err := suite.service.ControllerExpandVolume(context.Background(), getExpandVolumeRequest(volumeID))
	assert.NotNil(suite.T(), err, "error expected")
}

func (suite *TreeqControllerSuite) Test_ControllerExpandVolume_Error_filenotfound() {
	volumeID := "100#200$$"
	var filesytemID, treeqID = 100, 200
	var capacity int64 = common.BytesInOneGibibyte
	var maxSize string
	suite.filesystem.On("UpdateTreeqVolume", filesytemID, treeqID, capacity, maxSize).Return(nil)
	_, err := suite.service.ControllerExpandVolume(context.Background(), getExpandVolumeRequest(volumeID))
	assert.Nil(suite.T(), err, "error Not expected")
}

func (suite *TreeqControllerSuite) Test_ControllerExpandVolume_success() {
	volumeID := "100#200$$"
	var filesytemID, treeqID = 100, 200
	var capacity int64 = common.BytesInOneGibibyte
	var maxSize string
	suite.filesystem.On("UpdateTreeqVolume", filesytemID, treeqID, capacity, maxSize).Return(nil)
	resp, err := suite.service.ControllerExpandVolume(context.Background(), getExpandVolumeRequest(volumeID))
	assert.Nil(suite.T(), err, "error Not expected")
	assert.NotNil(suite.T(), resp, "response should not be nil")
}

func TestTreeqControllerSuite(t *testing.T) {
	suite.Run(t, new(TreeqControllerSuite))
}

func getExpandVolumeRequest(vID string) *csi.ControllerExpandVolumeRequest {
	return &csi.ControllerExpandVolumeRequest{
		VolumeId: vID,
	}
}

func getCreateVolumeResponse() map[string]string {
	result := map[string]string{
		"ID":      "100",
		"TREEQID": "200",
	}
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

func (m *FileSystemInterfaceMock) DeleteTreeqVolume(filesystemID, treeqID int) error {
	status := m.Called(filesystemID, treeqID)
	st, _ := status.Get(0).(error)
	return st
}

func (m *FileSystemInterfaceMock) UpdateTreeqVolume(filesystemID, treeqID int, capacity int64, maxSize string) error {
	status := m.Called(filesystemID, treeqID, capacity, maxSize)
	err, _ := status.Get(0).(error)
	return err
}

func (m *FileSystemInterfaceMock) IsTreeqAlreadyExist(pool_name, network_space, pVName, fsPrefix string) (map[string]string, error) {
	status := m.Called(pool_name, network_space, pVName, fsPrefix)
	st, _ := status.Get(0).(map[string]string)
	err, _ := status.Get(1).(error)
	return st, err
}

func getTreeqCreateVolumeParameters() map[string]string {
	return map[string]string{
		common.SC_GID:                       "2468",
		common.SC_MAX_VOLS_PER_HOST:         "19",
		common.SC_NETWORK_SPACE:             "network_space1",
		common.SC_POOL_NAME:                 "pool_name1",
		common.SC_PROVISION_TYPE:            common.SC_THIN_PROVISION_TYPE,
		common.SC_SSD_ENABLED:               "true",
		common.SC_STORAGE_PROTOCOL:          "iscsi",
		common.SC_UID:                       "1234",
		common.SC_UNIX_PERMISSIONS:          "0777",
		common.SC_USE_CHAP:                  "none",
		common.SC_MAX_FILESYSTEMS:           "1234",
		common.SC_MAX_TREEQS_PER_FILESYSTEM: "1234",
	}
}
