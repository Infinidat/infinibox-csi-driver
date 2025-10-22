//go:build unit

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
