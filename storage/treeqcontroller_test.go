package storage

import (
	"context"
	"errors"
	"fmt"
	"infinibox-csi-driver/helper"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

func (suite *TreeqControllerSuite) SetupTest() {
	suite.osHelperMock = new(helper.MockOsHelper)
	suite.filesystem = new(FileSystemInterfaceMock)
}

type TreeqControllerSuite struct {
	suite.Suite
	osHelperMock *helper.MockOsHelper
	filesystem   *FileSystemInterfaceMock
}

func (suite *TreeqControllerSuite) Test_CreateVolume_validation() {
	mapParameter := make(map[string]string)
	mapParameter["pool_name"] = "invalid poolName"
	suite.filesystem.On("validateTreeqParameters", mock.Anything).Return(false, mapParameter)
	service := treeqstorage{filesysService: suite.filesystem}
	_, err := service.CreateVolume(context.Background(), getCreateVolumeRequest())
	assert.NotNil(suite.T(), err, "empty error")
}

func (suite *TreeqControllerSuite) Test_CreateVolume_Error() {
	mapParameter := make(map[string]string)
	suite.filesystem.On("validateTreeqParameters", mock.Anything).Return(true, mapParameter)
	volumeRespoance := make(map[string]string)
	expectedErr := errors.New("some error")

	suite.filesystem.On("CreateTreeqVolume", mock.Anything, mock.Anything, mock.Anything).Return(volumeRespoance, expectedErr)
	service := treeqstorage{filesysService: suite.filesystem}
	_, err := service.CreateVolume(context.Background(), getCreateVolumeRequest())
	assert.NotNil(suite.T(), err, "empty error")
}

func (suite *TreeqControllerSuite) Test_CreateVolume_Success() {
	mapParameter := make(map[string]string)
	suite.filesystem.On("validateTreeqParameters", mock.Anything).Return(true, mapParameter)
	volumeResponse := getCreateVolumeResponse()
	suite.filesystem.On("CreateTreeqVolume", mock.Anything, mock.Anything, mock.Anything).Return(volumeResponse, nil)
	service := treeqstorage{filesysService: suite.filesystem}
	result, err := service.CreateVolume(context.Background(), getCreateVolumeRequest())
	assert.Nil(suite.T(), err, "empty error")
	val := result.GetVolume()
	fmt.Println("val", val.GetVolumeId())
	assert.Equal(suite.T(), "100#200#", val.GetVolumeId(), "ID shoulde be equal")
}

func (suite *TreeqControllerSuite) Test_DeleteVolume_VolumeID_empty() {
	service := treeqstorage{filesysService: suite.filesystem}
	_, err := service.DeleteVolume(context.Background(), getDeleteVolumeRequest(""))
	assert.NotNil(suite.T(), err, "Volume ID missing in request")
}
func (suite *TreeqControllerSuite) Test_DeleteVolume_InvalidVolumeID() {
	service := treeqstorage{filesysService: suite.filesystem}
	volumeID := "100"
	_, err := service.DeleteVolume(context.Background(), getDeleteVolumeRequest(volumeID))
	assert.NotNil(suite.T(), err, "Volume ID missing in request")
}

func (suite *TreeqControllerSuite) Test_DeleteVolume_Error() {
	service := treeqstorage{filesysService: suite.filesystem}
	volumeID := "100#200"
	expectedErr := errors.New("Some error")
	var filesytemID, treeqID int64 = 100, 200
	suite.filesystem.On("DeleteTreeqVolume", filesytemID, treeqID).Return(expectedErr)
	_, err := service.DeleteVolume(context.Background(), getDeleteVolumeRequest(volumeID))
	assert.NotNil(suite.T(), err, "error expected")
}

func (suite *TreeqControllerSuite) Test_DeleteVolume_Error_filenotfound() {
	service := treeqstorage{filesysService: suite.filesystem}
	volumeID := "100#200#"
	expectedErr := errors.New("FILESYSTEM_NOT_FOUND error")
	var filesytemID, treeqID int64 = 100, 200
	suite.filesystem.On("DeleteTreeqVolume", filesytemID, treeqID).Return(expectedErr)
	_, err := service.DeleteVolume(context.Background(), getDeleteVolumeRequest(volumeID))
	assert.Nil(suite.T(), err, "error Not expected")
}

func (suite *TreeqControllerSuite) Test_DeleteVolume_success() {
	service := treeqstorage{filesysService: suite.filesystem}
	volumeID := "100#200#"
	var filesytemID, treeqID int64 = 100, 200
	suite.filesystem.On("DeleteTreeqVolume", filesytemID, treeqID).Return(nil)
	resp, err := service.DeleteVolume(context.Background(), getDeleteVolumeRequest(volumeID))
	assert.Nil(suite.T(), err, "error Not expected")
	assert.NotNil(suite.T(), resp, "response should not be nil")
}
func TestTreeqControllerSuite(t *testing.T) {
	suite.Run(t, new(TreeqControllerSuite))
}

//test data

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
func getTreeCreateVolumeRequest() *csi.CreateVolumeRequest {
	return &csi.CreateVolumeRequest{}
}

//mock method
type FileSystemInterfaceMock struct {
	mock.Mock
}

func (m *FileSystemInterfaceMock) CreateTreeqVolume(config map[string]string, capacity int64, pvName string) (map[string]string, error) {
	status := m.Called(config, capacity, pvName)
	st, _ := status.Get(0).(map[string]string)
	err, _ := status.Get(1).(error)
	return st, err
}
func (m *FileSystemInterfaceMock) validateTreeqParameters(config map[string]string) (bool, map[string]string) {
	status := m.Called(config)
	st, _ := status.Get(0).(bool)
	parameter, _ := status.Get(1).(map[string]string)
	return st, parameter
}
func (m *FileSystemInterfaceMock) DeleteTreeqVolume(filesystemID, treeqID int64) error {
	status := m.Called(filesystemID, treeqID)
	st, _ := status.Get(0).(error)
	return st
}

func (m *FileSystemInterfaceMock) UpdateTreeqVolume(filesystemID, treeqID, capacity int64, maxSize string) error {
	status := m.Called(filesystemID, treeqID, capacity)
	err, _ := status.Get(0).(error)
	return err
}
