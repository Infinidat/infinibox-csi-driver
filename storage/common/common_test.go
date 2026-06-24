//go:build unit

/*
Copyright 2026 Infinidat
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

package common

import (
	"testing"

	"github.com/infinidat/infinibox-csi-driver/api"
	"github.com/infinidat/infinibox-csi-driver/helper"
	"github.com/infinidat/infinibox-csi-driver/iboxapi"

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
