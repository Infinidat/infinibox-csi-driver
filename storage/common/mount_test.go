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
	"os"
	"testing"

	"github.com/infinidat/infinibox-csi-driver/api"
	"github.com/infinidat/infinibox-csi-driver/helper"
	"github.com/infinidat/infinibox-csi-driver/iboxapi"

	"github.com/stretchr/testify/assert"

	"github.com/stretchr/testify/suite"
)

func (suite *MountSuite) SetupTest() {
	suite.api = new(api.MockAPIService)
	suite.iboxapi = new(iboxapi.MockAPIService)
	suite.accessMock = new(helper.MockAccessModesHelper)
	suite.cs = &Commonservice{IboxAPI: suite.iboxapi, API: suite.api, AccessModesHelper: suite.accessMock}
}

type MountSuite struct {
	suite.Suite
	api        *api.MockAPIService
	iboxapi    *iboxapi.MockAPIService
	accessMock *helper.MockAccessModesHelper
	cs         *Commonservice
}

func TestMountSuite(t *testing.T) {
	suite.Run(t, new(MountSuite))
}

// test mountLogic
func (suite *MountSuite) Test_VolumeConfigFile_Success() {

	config := DiskInfo{
		RootDir:     "/tmp",
		MpathDevice: "foo",
		IsBlock:     false,
		VolumeID:    1,
	}
	err := os.Mkdir(config.RootDir+"/"+"foo", os.ModePerm)
	assert.Nil(suite.T(), err, "expected nil returned on mkdir ")
	err = CreateConfigFile(config, "foo")
	assert.Nil(suite.T(), err, "expected nil returned on createConfigFile ")
	err = LoadDiskInfoFromFile(&config, "foo")
	assert.Nil(suite.T(), err, "expected nil returned on loadDiskInfoFromFile ")
	err = os.RemoveAll(config.RootDir + "/" + "foo")
	assert.Nil(suite.T(), err, "expected nil returned on remove dir ")

}
