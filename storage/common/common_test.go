//go:build unit

package common

import (
	"infinibox-csi-driver/api"
	"infinibox-csi-driver/helper"
	"infinibox-csi-driver/iboxapi"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

func (suite *CommonSuite) SetupTest() {
	suite.api = new(api.MockAPIService)
	suite.iboxapi = new(iboxapi.MockAPIService)
	suite.accessMock = new(helper.MockAccessModesHelper)
	suite.cs = &Commonservice{IboxAPI: suite.iboxapi, API: suite.api, AccessModesHelper: suite.accessMock}
}

type CommonSuite struct {
	suite.Suite
	api        *api.MockAPIService
	iboxapi    *iboxapi.MockAPIService
	accessMock *helper.MockAccessModesHelper
	cs         *Commonservice
}

func TestCommonSuite(t *testing.T) {
	suite.Run(t, new(CommonSuite))
}

func (suite *CommonSuite) Test_ValidateVolumeID_Success() {
	_, err := ValidateVolumeID("1$$iscsi")
	assert.Nil(suite.T(), err)
}

func (suite *CommonSuite) Test_ValidateVolumeID_Fail() {
	_, err := ValidateVolumeID("1$7iscsi")
	assert.NotNil(suite.T(), err)
}
func (suite *CommonSuite) Test_ValidateVolumeID_Fail_NonNumericVolumeID() {
	_, err := ValidateVolumeID("x$7iscsi")
	assert.NotNil(suite.T(), err)
}

func (suite *CommonSuite) Test_ValidateVolumeID_Fail_NoProtocol() {
	_, err := ValidateVolumeID("1$")
	assert.NotNil(suite.T(), err)
}

func (suite *CommonSuite) Test_ValidateVolumeID_Fail_NoVolumeID() {
	_, err := ValidateVolumeID("$nfs")
	assert.NotNil(suite.T(), err)
}

func (suite *CommonSuite) Test_ValidateVolumeID_Fail_TooManyParts() {
	_, err := ValidateVolumeID("1$nfs$doof")
	assert.NotNil(suite.T(), err)
}

func (suite *CommonSuite) Test_ValidateVolumeID_Fail_Empty() {
	_, err := ValidateVolumeID("")
	assert.NotNil(suite.T(), err)
}

func (suite *CommonSuite) Test_ValidateVolumeID_Fail_BadTreeq() {
	_, err := ValidateVolumeID("2942184#200001/nfs_treeq")
	assert.NotNil(suite.T(), err)
}

func (suite *CommonSuite) Test_ValidateVolumeID_Success_Treeq() {
	_, err := ValidateVolumeID("2942184#200001$$nfs_treeq")
	assert.Nil(suite.T(), err)
}
