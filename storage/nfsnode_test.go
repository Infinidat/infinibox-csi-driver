/*Copyright 2021 Infinidat
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.*/
package storage

import (
	"context"
	"errors"
	"infinibox-csi-driver/helper"
	tests "infinibox-csi-driver/test_helper"
	"os"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"k8s.io/utils/mount"
)

func (suite *NodeSuite) SetupTest() {
	suite.nfsMountMock = new(MockNfsMounter)
	suite.osmock = new(helper.MockOsHelper)

	tests.ConfigureKlog()
}

type NodeSuite struct {
	suite.Suite
	nfsMountMock *MockNfsMounter
	osmock       *helper.MockOsHelper
}

func TestNodeSuite(t *testing.T) {
	suite.Run(t, new(NodeSuite))
}

func (suite *NodeSuite) Test_NodePublishVolume_mnt_false() {
	service := nfsstorage{mounter: suite.nfsMountMock, osHelper: suite.osmock}
	suite.nfsMountMock.On("Mount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	suite.nfsMountMock.On("IsLikelyNotMountPoint", mock.Anything).Return(false, nil)
	suite.osmock.On("ChownVolume", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	suite.osmock.On("ChmodVolume", mock.Anything, mock.Anything).Return(nil)
	targetPath := "/var/lib/kublet/"
	responce, err := service.NodePublishVolume(context.Background(), getNodePublishVolumeRequest(targetPath, getPublishContexMap()))
	assert.Nil(suite.T(), err, "empty object")
	assert.NotNil(suite.T(), responce, "empty object")
}

func (suite *NodeSuite) Test_NodePublishVolume_mkdir_error() {
	service := nfsstorage{mounter: suite.nfsMountMock, osHelper: suite.osmock}
	mountErr := errors.New("mount error")
	suite.nfsMountMock.On("IsLikelyNotMountPoint", mock.Anything).Return(true, mountErr)
	suite.nfsMountMock.On("Mount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	suite.osmock.On("IsNotExist", mountErr).Return(true)
	suite.osmock.On("MkdirAll", mock.Anything, mock.Anything).Return(mountErr)

	targetPath := "/var/lib/kublet/"
	responce, err := service.NodePublishVolume(context.Background(), getNodePublishVolumeRequest(targetPath, getPublishContexMap()))
	assert.Equal(suite.T(), err.Error(), mountErr.Error())
	assert.Nil(suite.T(), responce, "empty object")
}

func (suite *NodeSuite) Test_NodePublishVolume_Exist_true() {
	service := nfsstorage{mounter: suite.nfsMountMock, osHelper: suite.osmock}
	mountErr := errors.New("mount error")
	suite.nfsMountMock.On("IsLikelyNotMountPoint", mock.Anything).Return(true, mountErr)
	suite.osmock.On("IsNotExist", mountErr).Return(false)
	suite.nfsMountMock.On("Mount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	targetPath := "/var/lib/kublet/"
	responce, err := service.NodePublishVolume(context.Background(), getNodePublishVolumeRequest(targetPath, getPublishContexMap()))
	assert.Equal(suite.T(), err.Error(), mountErr.Error())
	assert.Nil(suite.T(), responce, "empty object")
}

func (suite *NodeSuite) Test_NodePublishVolume_success() {
	service := nfsstorage{mounter: suite.nfsMountMock, osHelper: suite.osmock}
	suite.nfsMountMock.On("IsLikelyNotMountPoint", mock.Anything).Return(true, nil)
	suite.nfsMountMock.On("Mount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	suite.osmock.On("ChownVolume", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	suite.osmock.On("ChmodVolume", mock.Anything, mock.Anything).Return(nil)
	targetPath := "/var/lib/kublet/"
	_, err := service.NodePublishVolume(context.Background(), getNodePublishVolumeRequest(targetPath, getPublishContexMap()))

	assert.Nil(suite.T(), err, " error should be nil")
}

func (suite *NodeSuite) Test_NodePublishVolume_mount_fail() {
	mountErr := errors.New("mount error")
	service := nfsstorage{mounter: suite.nfsMountMock}
	suite.nfsMountMock.On("IsLikelyNotMountPoint", mock.Anything).Return(true, nil)
	suite.nfsMountMock.On("Mount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(mountErr)
	suite.osmock.On("ChownVolume", mock.Anything).Return(nil)
	targetPath := "/var/lib/kublet/"
	_, err := service.NodePublishVolume(context.Background(), getNodePublishVolumeRequest(targetPath, getPublishContexMap()))

	assert.NotNil(suite.T(), err, " error NOT should be nil")
}

// func (suite *NodeSuite) Test_NodeUnpublishVolume_MountPoint_fail() {
// 	service := nfsstorage{mounter: suite.nfsMountMock, osHelper: suite.osmock}
// 	volumeID := "1234"
// 	mountErr := errors.New("some error")
// 	suite.nfsMountMock.On("IsLikelyNotMountPoint", mock.Anything).Return(true, mountErr)
// 	suite.nfsMountMock.On("Mount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
// 	suite.nfsMountMock.On("Unmount", mock.Anything).Return(nil)
// 	suite.osmock.On("IsNotExist", mountErr).Return(false)
// 	targetPath := "/var/lib/kublet/"
// 	suite.osmock.On("Remove", targetPath).Return(nil)
// 	_, err := service.NodeUnpublishVolume(context.Background(), getNodeUnPublishVolumeRequest(targetPath, volumeID))
// 	assert.Equal(suite.T(), err.Error(), mountErr.Error())
// }

// func (suite *NodeSuite) Test_NodeUnpublishVolume_MountPoint_dir_notexist() {
// 	service := nfsstorage{mounter: suite.nfsMountMock, osHelper: suite.osmock}
// 	volumeID := "1234"
// 	mountErr := os.ErrNotExist
// 	suite.nfsMountMock.On("IsLikelyNotMountPoint", mock.Anything).Return(true, nil)
// 	suite.nfsMountMock.On("IsLikelyNotMountPoint", mock.Anything).Return(false, mountErr)
// 	suite.nfsMountMock.On("Mount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
// 	suite.nfsMountMock.On("Unmount", mock.Anything).Return(nil)
// 	suite.osmock.On("IsNotExist", mountErr).Return(true)
// 	targetPath := "/var/lib/kublet/"
// 	suite.osmock.On("Remove", targetPath).Return(nil)
// 	_, err := service.NodeUnpublishVolume(context.Background(), getNodeUnPublishVolumeRequest(targetPath, volumeID))
// 	assert.Nil(suite.T(), err, " error should be nil")
// }

// func (suite *NodeSuite) Test_NodeUnpublishVolume_unmount_error() {
// 	service := nfsstorage{mounter: suite.nfsMountMock, osHelper: suite.osmock}
// 	volumeID := "1234"
// 	mountErr := errors.New("some error")
// 	suite.nfsMountMock.On("IsLikelyNotMountPoint", mock.Anything).Return(false, nil)
// 	suite.nfsMountMock.On("IsLikelyNotMountPoint", mock.Anything).Return(false, nil)
// 	suite.nfsMountMock.On("Mount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
// 	suite.nfsMountMock.On("Unmount", mock.Anything).Return(mountErr)
// 	suite.osmock.On("Remove", mock.Anything).Return(nil)
// 	targetPath := "/var/lib/kublet/"
// 	_, err := service.NodeUnpublishVolume(context.Background(), getNodeUnPublishVolumeRequest(targetPath, volumeID))
// 	assert.NotNil(suite.T(), err, " error should not be nil")
// }

// func (suite *NodeSuite) Test_NodeUnpublishVolume_remove_error() {
// 	service := nfsstorage{mounter: suite.nfsMountMock, osHelper: suite.osmock}
// 	volumeID := "1234"
// 	removeErr := errors.New("some error")
// 	suite.nfsMountMock.On("IsLikelyNotMountPoint", mock.Anything).Return(true, nil)
// 	suite.nfsMountMock.On("IsLikelyNotMountPoint", mock.Anything).Return(false, nil)
// 	suite.nfsMountMock.On("Unmount", mock.Anything).Return(nil)
// 	targetPath := "/var/lib/kublet/"
// 	suite.osmock.On("Remove", targetPath).Return(removeErr)
// 	suite.osmock.On("IsNotExist", removeErr).Return(false)

// 	_, err := service.NodeUnpublishVolume(context.Background(), getNodeUnPublishVolumeRequest(targetPath, volumeID))
// 	assert.NotNil(suite.T(), err, " error should not be nil")
// }

// func (suite *NodeSuite) Test_NodeUnpublishVolume_Success() {
// 	service := nfsstorage{mounter: suite.nfsMountMock, osHelper: suite.osmock}
// 	volumeID := "1234"
// 	suite.nfsMountMock.On("IsLikelyNotMountPoint", mock.Anything).Return(false, nil)
// 	suite.nfsMountMock.On("IsLikelyNotMountPoint", mock.Anything).Return(false, nil)
// 	suite.nfsMountMock.On("Unmount", mock.Anything).Return(nil)
// 	targetPath := "/var/lib/kublet/"
// 	suite.osmock.On("Remove", targetPath).Return(nil)

// 	_, err := service.NodeUnpublishVolume(context.Background(), getNodeUnPublishVolumeRequest(targetPath, volumeID))
// 	assert.Nil(suite.T(), err, " error should be nil")
// }

//**************************
func getNodePublishVolumeRequest(tagetPath string, publishContexMap map[string]string) *csi.NodePublishVolumeRequest {
	return &csi.NodePublishVolumeRequest{
		TargetPath:     tagetPath,
		PublishContext: publishContexMap,
	}
}

func getPublishContexMap() map[string]string {
	contextMap := make(map[string]string)
	contextMap["ipAddress"] = "10.2.2.112"
	contextMap["volPathd"] = "12345"
	contextMap["volPathd"] = "/fs/filesytem/"
	return contextMap
}

func getNodeUnPublishVolumeRequest(tagetPath string, volumeID string) *csi.NodeUnpublishVolumeRequest {
	return &csi.NodeUnpublishVolumeRequest{
		TargetPath: tagetPath,
		VolumeId:   volumeID,
	}
}

// OS package -- mock method
type mockOS struct {
	mock.Mock
}

func (m *mockOS) IsNotExist(err error) bool {
	status := m.Called(err)
	st, _ := status.Get(0).(bool)
	return st
}

func (m *mockOS) MkdirAll(path string, perm os.FileMode) bool {
	status := m.Called(path, perm)
	return status.Get(0).(bool)
}

func (m *mockOS) Remove(path string) bool {
	status := m.Called(path)
	st, _ := status.Get(0).(bool)
	return st
}

// MockNfsMounter - mount mock
type MockNfsMounter struct {
	mount.Interface
	mock.Mock
}

func (m *MockNfsMounter) IsLikelyNotMountPoint(file string) (bool, error) {
	args := m.Called(file)
	resp, _ := args.Get(0).(bool)
	var err error
	if args.Get(1) == nil {
		err = nil
	} else {
		err, _ = args.Get(1).(error)
	}

	return resp, err
}

func (m *MockNfsMounter) Mount(source string, target string, fstype string, options []string) error {
	args := m.Called(source, target, fstype, options)
	var err error
	if args.Get(0) != nil {
		err = args.Get(0).(error)
	}
	return err
}

func (m *MockNfsMounter) Unmount(targetPath string) error {
	args := m.Called(targetPath)
	if args.Get(0) == nil {
		return nil
	}
	err := args.Get(0).(error)
	return err
}
