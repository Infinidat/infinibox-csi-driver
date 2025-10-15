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
package nfs

import (
	"context"
	"errors"
	"fmt"
	"infinibox-csi-driver/api"
	"infinibox-csi-driver/common"
	"infinibox-csi-driver/helper"
	"infinibox-csi-driver/iboxapi"
	storagecommon "infinibox-csi-driver/storage/common"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

func (suite *NodeSuite) SetupTest() {
	suite.nfsMountMock = new(storagecommon.MockNfsMounter)
	suite.api = new(api.MockApiService)
	suite.iboxapi = new(iboxapi.MockApiService)
	suite.osmock = new(helper.MockOsHelper)
	suite.storageHelperMock = new(storagecommon.MockStorageHelper)
	suite.accessMock = new(helper.MockAccessModesHelper)
	suite.cs = &storagecommon.Commonservice{IboxApi: suite.iboxapi, Api: suite.api, AccessModesHelper: suite.accessMock}
	suite.cs.VolProto = &api.VolumeProtocolConfig{
		VolumeID: 1,
	}
	service := NFSstorage{CS: *suite.cs, Mounter: suite.nfsMountMock, StorageHelper: suite.storageHelperMock, OSHelper: suite.osmock}
	suite.service = service

}

type NodeSuite struct {
	suite.Suite
	nfsMountMock      *storagecommon.MockNfsMounter
	api               *api.MockApiService
	iboxapi           *iboxapi.MockApiService
	osmock            *helper.MockOsHelper
	accessMock        *helper.MockAccessModesHelper
	storageHelperMock *storagecommon.MockStorageHelper
	cs                *storagecommon.Commonservice
	service           NFSstorage
}

func TestNodeSuite(t *testing.T) {
	suite.Run(t, new(NodeSuite))
}

func (suite *NodeSuite) Test_NodePublishVolume_success() {
	randomDir := storagecommon.RandomString(10)
	targetPath := randomDir
	err := os.Mkdir("/tmp/"+targetPath, os.ModePerm)
	assert.Nil(suite.T(), err)
	defer func() {
		err := os.RemoveAll("/tmp/" + targetPath)
		assert.Nil(suite.T(), err)
	}()

	contex := storagecommon.GetPublishContexMap()
	contex["csiContainerHostMountPoint"] = "/tmp/"

	suite.nfsMountMock.On("Mount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	suite.iboxapi.On("GetSystem", mock.Anything).Return(storagecommon.GetSystem(), nil)
	suite.storageHelperMock.On("SetVolumePermissions", mock.Anything).Return(nil)
	suite.storageHelperMock.On("ValidateIPAddress", mock.Anything, mock.Anything).Return(nil)
	suite.storageHelperMock.On("GetNFSMountOptions", mock.Anything).Return([]string{}, nil)

	req := storagecommon.GetNodePublishVolumeRequest(targetPath, contex)
	req.VolumeContext = storagecommon.GetVolumeContexMap()
	req.VolumeId = "1234$$nfs"
	_, err = suite.service.NodePublishVolume(context.Background(), req)

	assert.Nil(suite.T(), err, " error should be nil")
}

// the test for when nfs_export_permissions is not set by a user, in this
// case a default export rule should be created, this test has no existing
// exports
func (suite *NodeSuite) Test_NodePublishVolume_DefaultExport_success() {
	randomDir := storagecommon.RandomString(10)
	targetPath := randomDir
	err := os.Mkdir("/tmp/"+targetPath, os.ModePerm)
	assert.Nil(suite.T(), err)
	defer func() {
		err := os.RemoveAll("/tmp/" + targetPath)
		assert.Nil(suite.T(), err)
	}()

	contex := storagecommon.GetPublishContexMap()
	contex["csiContainerHostMountPoint"] = "/tmp/"

	suite.nfsMountMock.On("Mount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	suite.storageHelperMock.On("SetVolumePermissions", mock.Anything).Return(nil)
	suite.storageHelperMock.On("ValidateIPAddress", mock.Anything, mock.Anything).Return(nil)
	suite.storageHelperMock.On("GetNFSMountOptions", mock.Anything).Return([]string{}, nil)
	suite.iboxapi.On("GetFileSystemByID", mock.Anything).Return(&iboxapi.FileSystem{}, nil)
	suite.iboxapi.On("GetSystem", mock.Anything).Return(storagecommon.GetSystem(), nil)
	suite.iboxapi.On("CreateExport", mock.Anything).Return(storagecommon.GetExportResponseValue(), nil)
	exportResp := storagecommon.GetExportResponse()
	suite.iboxapi.On("GetExportsByFileSystemID", mock.Anything).Return(exportResp, nil)
	suite.iboxapi.On("DeleteExport", mock.Anything).Return(exportResp, nil)

	req := storagecommon.GetNodePublishVolumeRequest(targetPath, contex)
	req.VolumeContext = storagecommon.GetVolumeContexMap()
	req.VolumeId = "1234$$nfs"
	req.VolumeContext[common.SC_NFS_EXPORT_PERMISSIONS] = ""
	req.Secrets = map[string]string{
		"nodeID": "192.168.0.110",
	}
	req.VolumeContext[common.SC_SNAPDIR_VISIBLE] = "true"
	req.VolumeContext[common.SC_PRIV_PORTS] = "false"
	_, err = suite.service.NodePublishVolume(context.Background(), req)

	assert.Nil(suite.T(), err, " error should be nil")
}

// the test for when nfs_export_permissions is not set by a user, in this
// case a default export rule should be created, this test has 1 existing
// exports that will test deleting it and recreating the export logic, this
// is the case when a pod restarts having an existing export
func (suite *NodeSuite) XTest_NodePublishVolume_DefaultExport_PodRestart_success() {
	randomDir := storagecommon.RandomString(10)
	targetPath := randomDir
	err := os.Mkdir("/tmp/"+targetPath, os.ModePerm)
	assert.Nil(suite.T(), err)
	defer func() {
		err := os.RemoveAll("/tmp/" + targetPath)
		assert.Nil(suite.T(), err)
	}()

	contex := storagecommon.GetPublishContexMap()
	contex["csiContainerHostMountPoint"] = "/tmp/"

	suite.nfsMountMock.On("Mount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	suite.storageHelperMock.On("SetVolumePermissions", mock.Anything).Return(nil)
	suite.storageHelperMock.On("ValidateIPAddress", mock.Anything, mock.Anything).Return(nil)
	suite.storageHelperMock.On("GetNFSMountOptions", mock.Anything).Return([]string{}, nil)
	suite.iboxapi.On("GetFileSystemByID", mock.Anything).Return(&iboxapi.FileSystem{}, nil)
	suite.iboxapi.On("CreateExport", mock.Anything).Return(storagecommon.GetExportResponseValue(), nil)
	tmp := storagecommon.GetExportResponseWithExports()
	suite.iboxapi.On("GetExportsByFileSystemID", mock.Anything).Return(tmp, nil)
	suite.iboxapi.On("DeleteExport", mock.Anything).Return(storagecommon.GetExportResponse(), nil)

	req := storagecommon.GetNodePublishVolumeRequest(targetPath, contex)
	req.VolumeContext = storagecommon.GetVolumeContexMap()
	req.VolumeId = "1234$$nfs"
	req.VolumeContext[common.SC_NFS_EXPORT_PERMISSIONS] = ""
	req.VolumeContext["nodeID"] = "192.168.0.110"
	req.VolumeContext[common.SC_SNAPDIR_VISIBLE] = "true"
	req.VolumeContext[common.SC_PRIV_PORTS] = "false"
	_, err = suite.service.NodePublishVolume(context.Background(), req)

	assert.Nil(suite.T(), err, " error should be nil")
}

func (suite *NodeSuite) Test_NodePublishVolume_mount_fail() {
	randomDir := storagecommon.RandomString(10)
	targetPath := randomDir

	contex := storagecommon.GetPublishContexMap()
	contex["csiContainerHostMountPoint"] = "/tmp/"
	mountErr := errors.New("mount error")
	suite.iboxapi.On("CreateExport", mock.Anything).Return(storagecommon.GetExportResponseValue(), nil)
	suite.iboxapi.On("GetSystem", mock.Anything).Return(storagecommon.GetSystem(), nil)
	exportResp := storagecommon.GetExportResponse()
	suite.iboxapi.On("GetExportsByFileSystemID", mock.Anything).Return(exportResp, nil)
	suite.iboxapi.On("GetFileSystemByID", mock.Anything).Return(&iboxapi.FileSystem{}, nil)
	suite.storageHelperMock.On("SetVolumePermissions", mock.Anything).Return(nil)
	suite.storageHelperMock.On("ValidateIPAddress", mock.Anything, mock.Anything).Return(nil)
	suite.storageHelperMock.On("GetNFSMountOptions", mock.Anything).Return([]string{}, nil)

	suite.nfsMountMock.On("Mount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(mountErr)
	_, err := suite.service.NodePublishVolume(context.Background(), storagecommon.GetNodePublishVolumeRequest(targetPath, contex))

	assert.NotNil(suite.T(), err, " error NOT should be nil")
}

func (suite *NodeSuite) Test_updateNfsMountOptions_badNfsVersion() {
	head := "nfs version mount option '"
	tail := "' encountered, but only NFS versions 3 and 4 are supported"
	tests := []struct {
		version string
		wanterr error
	}{
		{"vers=", fmt.Errorf("%svers=%s", head, tail)},
		{"vers=1", fmt.Errorf("%svers=1%s", head, tail)},
		{"vers=2", fmt.Errorf("%svers=2%s", head, tail)},
		{"vers=3", nil},
		{"vers=4", nil},
		{"vers=5", fmt.Errorf("%svers=5%s", head, tail)},
		{"vers=33", fmt.Errorf("%svers=33%s", head, tail)},
		{"nfsvers=1", fmt.Errorf("%snfsvers=1%s", head, tail)},
		{"nfsvers=2", fmt.Errorf("%snfsvers=2%s", head, tail)},
		{"nfsvers=3", nil},
		{"nfsvers=4", nil},
		{"nfsvers=5", fmt.Errorf("%snfsvers=5%s", head, tail)},
		{"nfsvers=33", fmt.Errorf("%snfsvers=33%s", head, tail)},
	}

	for _, test := range tests {
		mountOptions := []string{
			"tcp",
			"rsize=262144",
			"wsize=262144",
			test.version,
		}

		targetPath := "/var/lib/kublet/"
		req := storagecommon.GetNodePublishVolumeRequest(targetPath, storagecommon.GetPublishContexMap())
		_, err := storagecommon.UpdateNfsMountOptions(mountOptions, req)
		fmt.Printf("version: %s\n", test.version)
		fmt.Printf("err    : %s\n", err)
		fmt.Printf("wanterr: %s\n", test.wanterr)
		assert.Equal(suite.T(), err, test.wanterr, fmt.Sprintf("updateNfsMountOptions(version=%s) has error %v", test.version, err))
	}
}

func (suite *NodeSuite) Test_updateNfsMountOptions_hard() {
	tests := []struct {
		recovery string
		want     int
	}{
		{"hard", 1},
		{"soft", 0},
		{"intr", 1}, // Filler for not-specified. Option ignored in modern kernels
	}

	for _, test := range tests {
		mountOptions := []string{
			"tcp",
			"rsize=262144",
			"wsize=262144",
			test.recovery,
		}

		targetPath := "/var/lib/kublet/"
		req := storagecommon.GetNodePublishVolumeRequest(targetPath, storagecommon.GetPublishContexMap())
		updated, _ := storagecommon.UpdateNfsMountOptions(mountOptions, req)
		countFound := countValsInSlice(updated, "hard")
		assert.Equal(suite.T(), countFound, test.want, fmt.Sprintf("For option %s, found %d, but should find exactly %d 'ro'", test.recovery, countFound, test.want))
	}
}

func (suite *NodeSuite) Test_updateNfsMountOptions_ro() {
	tests := []struct {
		readonly bool
		want     int
		wanterr  string
	}{
		{true, 1, ""},
		{false, 0, ""},
	}

	for _, test := range tests {
		mountOptions := []string{
			"tcp",
			"rsize=262144",
			"wsize=262144",
		}

		targetPath := "/var/lib/kublet/"
		// Create req with Readonly option
		req := storagecommon.GetNodePublishVolumeRequestReadonly(targetPath, test.readonly, storagecommon.GetPublishContexMap())
		updated, _ := storagecommon.UpdateNfsMountOptions(mountOptions, req)
		countFound := countValsInSlice(updated, "ro")
		assert.Equal(suite.T(), countFound, test.want, fmt.Sprintf("For readonly %t, found %d, but should find exactly %d 'ro'", test.readonly, countFound, test.want))
	}
}

func (suite *NodeSuite) Test_updateNfsMountOptions_no_ro() {
	mountOptions := []string{
		"tcp",
		"rsize=262144",
		"wsize=262144",
	}

	targetPath := "/var/lib/kublet/"
	readonly := false
	req := storagecommon.GetNodePublishVolumeRequestReadonly(targetPath, readonly, storagecommon.GetPublishContexMap())
	updated, _ := storagecommon.UpdateNfsMountOptions(mountOptions, req)
	countFound := countValsInSlice(updated, "ro")
	assert.Equal(suite.T(), countFound, 0, "Should find exactly zero 'ro'")
}

func (suite *NodeSuite) Test_updateNfsMountOptions_mustAddVers() {
	mountOptions := []string{
		"tcp",
		"rsize=262144",
		"wsize=262144",
	}

	targetPath := "/var/lib/kublet/"
	req := storagecommon.GetNodePublishVolumeRequest(targetPath, storagecommon.GetPublishContexMap())
	updated, _ := storagecommon.UpdateNfsMountOptions(mountOptions, req)
	countFound := countValsInSlice(updated, "vers=3")
	assert.Equal(suite.T(), countFound, 1, "Should find exactly one vers=3")
}

func (suite *NodeSuite) Test_updateNfsMountOptions_mustNotAddVersAgain() {
	mountOptions := []string{
		"tcp",
		"rsize=262144",
		"vers=3",
		"wsize=262144",
	}

	targetPath := "/var/lib/kublet/"
	req := storagecommon.GetNodePublishVolumeRequest(targetPath, storagecommon.GetPublishContexMap())
	updated, _ := storagecommon.UpdateNfsMountOptions(mountOptions, req)
	countFound := countValsInSlice(updated, "vers=3")
	assert.Equal(suite.T(), countFound, 1, "Should find exactly one vers=3")
}

func (suite *NodeSuite) Test_updateNfsMountOptions_mustNotAddVers() {
	mountOptions := []string{
		"tcp",
		"nfsvers=3",
		"rsize=262144",
		"wsize=262144",
	}

	targetPath := "/var/lib/kublet/"
	req := storagecommon.GetNodePublishVolumeRequest(targetPath, storagecommon.GetPublishContexMap())
	updated, _ := storagecommon.UpdateNfsMountOptions(mountOptions, req)
	countFound := countValsInSlice(updated, "vers=3")
	assert.Equal(suite.T(), countFound, 0, "Should not have found vers=3")
	countFoundNfs := countValsInSlice(updated, "nfsvers=3")
	assert.Equal(suite.T(), countFoundNfs, 1, "Should find exactly one nfsvers=3")
}

func countValsInSlice(slice []string, val string) int {
	var count int
	for _, item := range slice {
		if item == val {
			count += 1
		}
	}
	return count
}
