//go:build unit

package storage

import (
	"errors"
	"infinibox-csi-driver/api"
	"infinibox-csi-driver/common"
	"infinibox-csi-driver/helper"
	"infinibox-csi-driver/iboxapi"
	"testing"

	"github.com/stretchr/testify/assert"
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
		common.SC_STORAGE_PROTOCOL:  "iscsi",
		common.SC_UID:               "1234",
		common.SC_UNIX_PERMISSIONS:  "0777",
	}
}
