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
	"flag"
	"os"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/infinidat/infinibox-csi-driver/api"
	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/infinidat/infinibox-csi-driver/helper"
	"github.com/infinidat/infinibox-csi-driver/iboxapi"
	"github.com/infinidat/infinibox-csi-driver/storage/luks"

	"github.com/stretchr/testify/assert"
	"k8s.io/klog/v2"

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

func (suite *MountSuite) Test_VolumeConfigFile_Encryption_Success() {
	rootDir := suite.T().TempDir()
	stagePath := "globalmount"
	err := os.Mkdir(rootDir+"/"+stagePath, os.ModePerm)
	assert.Nil(suite.T(), err, "expected nil returned on mkdir ")

	config := DiskInfo{RootDir: rootDir, MpathDevice: "mpatha", VolumeID: 42}
	config.SetEncryption(nil)
	assert.Equal(suite.T(), "", config.LuksDevice, "not encrypted")
	assert.Nil(suite.T(), config.encryptionSecret())

	config.SetEncryption(&luks.SecretRef{Name: "data-luks", Namespace: "tenant-a"})
	err = CreateConfigFile(config, stagePath)
	assert.Nil(suite.T(), err, "expected nil returned on createConfigFile ")

	loaded := DiskInfo{RootDir: rootDir, VolumeID: 42}
	err = LoadDiskInfoFromFile(&loaded, stagePath)
	assert.Nil(suite.T(), err, "expected nil returned on loadDiskInfoFromFile ")
	assert.Equal(suite.T(), "infinibox-luks-42", loaded.LuksDevice)
	assert.Equal(suite.T(), &luks.SecretRef{Name: "data-luks", Namespace: "tenant-a"}, loaded.encryptionSecret())

	// disk info written by a driver without encryption support
	err = os.WriteFile(rootDir+"/"+stagePath+"/7.json", []byte(`{"rootdir":"/host","mpathdevice":"mpathb","isblock":false,"volumeid":7}`), 0o600)
	assert.Nil(suite.T(), err)
	old := DiskInfo{RootDir: rootDir, VolumeID: 7}
	assert.Nil(suite.T(), LoadDiskInfoFromFile(&old, stagePath))
	assert.Nil(suite.T(), old.encryptionSecret())
	assert.Nil(suite.T(), CloseEncryptedDevice(old, true), "nothing to close for a volume that is not encrypted")
}

func (suite *MountSuite) Test_GetDiskMounter_Encryption() {
	if flag.Lookup("v") == nil {
		klog.InitFlags(nil) // GetDiskMounter sets the klog verbosity, as the driver does in main
	}
	req := &csi.NodePublishVolumeRequest{
		VolumeId: "42$$iscsi",
		VolumeCapability: &csi.VolumeCapability{
			AccessType: &csi.VolumeCapability_Mount{Mount: &csi.VolumeCapability_MountVolume{FsType: "xfs"}},
			AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
		},
		VolumeContext: map[string]string{},
	}
	m, err := GetDiskMounter(req)
	assert.Nil(suite.T(), err)
	assert.Nil(suite.T(), m.Encryption, "not encrypted")

	// a volume provisioned before encryption existed, from a StorageClass that had an
	// "encrypted" parameter (ignored then): it is mounted as before
	req.VolumeContext[common.StorageClassEncrypted] = "true"
	m, err = GetDiskMounter(req)
	assert.Nil(suite.T(), err)
	assert.Nil(suite.T(), m.Encryption, "legacy volume, not encrypted")

	req.VolumeContext[common.StorageClassEncryptionSecretName] = "data-luks"
	_, err = GetDiskMounter(req)
	assert.NotNil(suite.T(), err, "an encrypted volume with an incomplete Secret reference must not be mounted")

	req.VolumeContext[common.StorageClassEncryptionSecretNamespace] = "tenant-a"
	m, err = GetDiskMounter(req)
	assert.Nil(suite.T(), err)
	assert.Equal(suite.T(), &luks.SecretRef{Name: "data-luks", Namespace: "tenant-a"}, m.Encryption)
}

func (suite *MountSuite) Test_FindEncryptedVolume_NotEncrypted() {
	v, err := FindEncryptedVolume(suite.T().TempDir(), 42)
	assert.Nil(suite.T(), err)
	assert.Nil(suite.T(), v, "no disk info: not encrypted")
}

// the disk info of a volume that is not encrypted is written exactly as before
func (suite *MountSuite) Test_VolumeConfigFile_NotEncrypted_Unchanged() {
	rootDir := suite.T().TempDir()
	assert.Nil(suite.T(), os.Mkdir(rootDir+"/globalmount", os.ModePerm))
	config := DiskInfo{RootDir: "/host", MpathDevice: "mpatha", IsBlock: true, VolumeID: 7}
	config.SetEncryption(nil)
	config.RootDir = rootDir
	assert.Nil(suite.T(), CreateConfigFile(config, "globalmount"))
	content, err := os.ReadFile(rootDir + "/globalmount/7.json")
	assert.Nil(suite.T(), err)
	assert.Equal(suite.T(), `{"rootdir":"`+rootDir+`","mpathdevice":"mpatha","isblock":true,"volumeid":7}`+"\n", string(content))
}
