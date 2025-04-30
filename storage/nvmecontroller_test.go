//go:build unit

package storage

import (
	"context"
	"errors"
	"fmt"
	"infinibox-csi-driver/api"
	"infinibox-csi-driver/common"
	"infinibox-csi-driver/helper"
	"infinibox-csi-driver/iboxapi"
	"infinibox-csi-driver/test_helper"
	tests "infinibox-csi-driver/test_helper"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

func (suite *NVMEControllerSuite) SetupTest() {
	suite.api = new(api.MockApiService)
	suite.iboxapi = new(iboxapi.MockApiService)
	suite.accessMock = new(helper.MockAccessModesHelper)
	host := &iboxapi.Host{
		ID:   1,
		Name: "host1",
	}
	volProto := &api.VolumeProtocolConfig{
		Host:     host,
		VolumeID: 1,
		NodeID:   "node1",
	}
	suite.cs = &Commonservice{Api: suite.api, AccessModesHelper: suite.accessMock, IboxApi: suite.iboxapi, VolProto: volProto}
	suite.someError = errors.New("some Error")
	suite.service = nvmestorage{cs: *suite.cs}
}

type NVMEControllerSuite struct {
	suite.Suite
	api        *api.MockApiService
	iboxapi    *iboxapi.MockApiService
	accessMock *helper.MockAccessModesHelper
	cs         *Commonservice
	someError  error
	service    nvmestorage
}

func TestNVMEControllerSuite(t *testing.T) {
	suite.Run(t, new(NVMEControllerSuite))
}

func (suite *NVMEControllerSuite) Test_ValidateStorageClass_InvalidProvisionType_Fail() {
	parameterMap := map[string]string{
		common.SC_PROVISION_TYPE: "somethinginvalid",
	}
	err := suite.service.ValidateStorageClass(parameterMap)
	assert.NotNil(suite.T(), err, "expected to fail: nvme invalid provision type sc parameter")
}
func (suite *NVMEControllerSuite) Test_ValidateStorageClass_ValidPoolName() {
	parameterMap := getNVMECreateVolumeParameters()
	err := suite.service.ValidateStorageClass(parameterMap)
	assert.Nil(suite.T(), err, "expected to pass: nvme valid pool name sc parameter")
}
func (suite *NVMEControllerSuite) Test_ValidateStorageClass_InvalidPoolName() {
	parameterMap := map[string]string{
		common.SC_POOL_NAME: "",
	}
	err := suite.service.ValidateStorageClass(parameterMap)
	assert.NotNil(suite.T(), err, "expected to fail: nvme CreateVolume invalid parameter")
}
func (suite *NVMEControllerSuite) Test_ValidateStorageClass_InvalidParameter_No_Parameters() {
	var parameterMap map[string]string
	err := suite.service.ValidateStorageClass(parameterMap)
	assert.NotNil(suite.T(), err, "expected to fail: nvme CreateVolume invalid parameter")
}

func (suite *NVMEControllerSuite) Test_ValidateStorageClass_InvalidParameter_Missing_Network_Space() {
	parameterMap := getNVMECreateVolumeParameters()
	delete(parameterMap, common.SC_NETWORK_SPACE)
	err := suite.service.ValidateStorageClass(parameterMap)
	assert.NotNil(suite.T(), err, "expected to fail: nvme CreateVolumeValidate missing parameter")
}
func (suite *NVMEControllerSuite) Test_ValidateStorageClass_InvalidParameter_Missing_Pool() {
	parameterMap := getNVMECreateVolumeParameters()
	delete(parameterMap, common.SC_POOL_NAME)
	err := suite.service.ValidateStorageClass(parameterMap)
	assert.NotNil(suite.T(), err, "expected to fail: nvme CreateVolumeValidate missing parameter")
}
func getNVMECreateVolumeParameters() map[string]string {
	return map[string]string{
		common.SC_GID:               "2468",
		common.SC_MAX_VOLS_PER_HOST: "19",
		common.SC_NETWORK_SPACE:     "network_space1",
		common.SC_POOL_NAME:         "pool_name1",
		common.SC_PROVISION_TYPE:    common.SC_THIN_PROVISION_TYPE,
		common.SC_SSD_ENABLED:       "true",
		common.SC_STORAGE_PROTOCOL:  "nvme",
		common.SC_UID:               "1234",
		common.SC_UNIX_PERMISSIONS:  "0777",
	}
}

func (suite *NVMEControllerSuite) Test_CreateVolume_GetName_fail() {
	parameterMap := getNVMECreateVolumeParameters()
	createVolReq := tests.GetCreateVolumeRequest("pvname", parameterMap, "")

	suite.iboxapi.On("GetVolumeByName", mock.Anything).Return(getVolume(), suite.someError)
	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkspace(), nil)
	suite.api.On("OneTimeValidation", mock.Anything, mock.Anything).Return("", nil)
	_, err := suite.service.CreateVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: nvme CreateVolume GetVolumeByName")
}

func (suite *NVMEControllerSuite) Test_CreateVolume_fail() {
	parameterMap := getNVMECreateVolumeParameters()
	createVolReq := tests.GetCreateVolumeRequest("pvname", parameterMap, "")

	poolResult := &iboxapi.PoolResult{ID: 10}
	suite.iboxapi.On("GetPoolByName", mock.Anything).Return(poolResult, nil)

	suite.iboxapi.On("GetVolumeByName", mock.Anything).Return(getVolume(), nil)

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkspace(), nil)
	suite.api.On("CreateVolume", mock.Anything, mock.Anything).Return(nil, suite.someError)
	suite.api.On("OneTimeValidation", mock.Anything, mock.Anything).Return("", nil)

	_, err := suite.service.CreateVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: nvme CreateVolume")
}

func (suite *NVMEControllerSuite) Test_CreateVolume_success() {
	suite.service.capacity = common.BytesInOneGibibyte
	parameterMap := getNVMECreateVolumeParameters()
	createVolReq := tests.GetCreateVolumeRequest("pvname", parameterMap, "")

	poolResult := &iboxapi.PoolResult{ID: 10}
	suite.iboxapi.On("GetPoolByName", mock.Anything).Return(poolResult, nil)
	suite.iboxapi.On("GetVolumeByName", mock.Anything).Return(getVolume(), nil)

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkspace(), nil)
	suite.api.On("CreateVolume", mock.Anything, mock.Anything).Return(getVolume(), nil)
	suite.iboxapi.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	suite.api.On("OneTimeValidation", mock.Anything, mock.Anything).Return("", nil)
	suite.iboxapi.On("PutMetadata", mock.Anything, mock.Anything).Return(nil, nil)
	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkspace(), nil)

	_, err := suite.service.CreateVolume(context.Background(), createVolReq)
	assert.Nil(suite.T(), err, "expected to succeed: nvme CreateVolume")
}

func (suite *NVMEControllerSuite) Test_CreateVolume_metadataError() {
	parameterMap := getNVMECreateVolumeParameters()
	createVolReq := tests.GetCreateVolumeRequest("pvname", parameterMap, "")

	poolResult := &iboxapi.PoolResult{ID: 10}
	suite.iboxapi.On("GetPoolByName", mock.Anything).Return(poolResult, nil)

	suite.iboxapi.On("GetVolumeByName", mock.Anything).Return(getVolume(), nil)

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkspace(), nil)
	suite.api.On("CreateVolume", mock.Anything, mock.Anything).Return(getVolume(), nil)
	suite.api.On("OneTimeValidation", mock.Anything, mock.Anything).Return("", nil)
	suite.iboxapi.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	suite.iboxapi.On("PutMetadata", mock.Anything, mock.Anything).Return(nil, suite.someError)

	_, err := suite.service.CreateVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: nvme CreateVolume attach metadata")
}

func (suite *NVMEControllerSuite) Test_DeleteVolume_GetVolume_Error() {
	createVolReq := getDeleteRequest()
	deleteMetadataResponse := &iboxapi.DeleteMetadataResponse{}
	suite.iboxapi.On("DeleteMetadata", mock.Anything).Return(deleteMetadataResponse, nil)
	suite.iboxapi.On("GetVolume", mock.Anything).Return(nil, suite.someError)
	_, err := suite.service.DeleteVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: nvme DeleteVolume getVolume")
}

func (suite *NVMEControllerSuite) Test_DeleteVolume_GetVolumeSnapshot_metadataError() {
	createVolReq := getDeleteRequest()
	deleteMetadataResponse := &iboxapi.DeleteMetadataResponse{}
	suite.iboxapi.On("DeleteMetadata", mock.Anything).Return(deleteMetadataResponse, nil)
	deleteVolumeResponse := iboxapi.DeleteVolumeResponse{}
	suite.iboxapi.On("DeleteVolume", mock.Anything).Return(deleteVolumeResponse, suite.someError)
	suite.iboxapi.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	suite.iboxapi.On("GetVolumesByParentID", mock.Anything).Return(getVolumeArray(), nil)
	suite.iboxapi.On("PutMetadata", mock.Anything, mock.Anything).Return(nil, suite.someError)

	_, err := suite.service.DeleteVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: nvme DeleteVolume GetVolumeSnapshot attach metadata")
}

func (suite *NVMEControllerSuite) Test_DeleteVolume_Error() {
	createVolReq := getDeleteRequest()
	suite.iboxapi.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	suite.iboxapi.On("GetVolumesByParentID", mock.Anything).Return([]iboxapi.Volume{}, nil)
	deleteMetadataResponse := &iboxapi.DeleteMetadataResponse{}
	suite.iboxapi.On("DeleteMetadata", mock.Anything).Return(deleteMetadataResponse, nil)
	deleteVolumeResponse := iboxapi.DeleteVolumeResponse{}
	suite.iboxapi.On("DeleteVolume", mock.Anything).Return(deleteVolumeResponse, suite.someError)

	_, err := suite.service.DeleteVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: nvme DeleteVolume delete volume")
}

func (suite *NVMEControllerSuite) Test_DeleteVolume_success() {
	createVolReq := getDeleteRequest()
	suite.iboxapi.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	deleteMetadataResponse := &iboxapi.DeleteMetadataResponse{}
	suite.iboxapi.On("DeleteMetadata", mock.Anything).Return(deleteMetadataResponse, nil)
	suite.iboxapi.On("GetVolumesByParentID", mock.Anything).Return([]iboxapi.Volume{}, nil)
	suite.iboxapi.On("GetMetadata", mock.Anything).Return(test_helper.GetHostMetadata(), nil)
	deleteVolumeResponse := iboxapi.DeleteVolumeResponse{}
	suite.iboxapi.On("DeleteVolume", mock.Anything).Return(deleteVolumeResponse, nil)
	suite.api.On("GetMetadataStatus", mock.Anything).Return(false)
	_, err := suite.service.DeleteVolume(context.Background(), createVolReq)
	assert.Nil(suite.T(), err, "expected to succeed: nvme DeleteVolume")
}

func (suite *NVMEControllerSuite) Test_DeleteVolume_AlreadyDelete() {
	createVolReq := getDeleteRequest()
	notFoundError := &iboxapi.IboxAPIError{Code: iboxapi.IBOXAPI_RESOURCE_NOT_FOUND_ERROR, Err: fmt.Errorf("volume not found")}
	suite.iboxapi.On("GetVolume", mock.Anything).Return(nil, notFoundError)
	_, err := suite.service.DeleteVolume(context.Background(), createVolReq)
	assert.Nil(suite.T(), err, "expected to succeed: nvme DeleteVolume when already deleted")
}
