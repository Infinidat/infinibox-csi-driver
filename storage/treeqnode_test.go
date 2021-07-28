/*Copyright 2020 Infinidat
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
	"testing"

	"infinibox-csi-driver/helper"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

func (suite *TreeqNodeSuite) SetupTest() {
	suite.nfsMountMock = new(MockNfsMounter)
	suite.osHelperMock = new(helper.MockOsHelper)
}

type TreeqNodeSuite struct {
	suite.Suite
	nfsMountMock *MockNfsMounter
	osHelperMock *helper.MockOsHelper
}

func TestTreeqNodeSuite(t *testing.T) {
	suite.Run(t, new(TreeqNodeSuite))
}

func (suite *TreeqNodeSuite) Test_TreeqNodePublishVolume_mnt_false() {
	service := treeqstorage{mounter: suite.nfsMountMock, osHelper: suite.osHelperMock}
	suite.nfsMountMock.On("IsLikelyNotMountPoint", mock.Anything).Return(true, nil)
	suite.nfsMountMock.On("IsNotMountPoint", mock.Anything).Return(false, nil)
	suite.nfsMountMock.On("Mount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	targetPath := "/var/lib/kublet/"
	responce, err := service.NodePublishVolume(context.Background(), getNodePublishVolumeRequest(targetPath, getPublishContexMap()))
	assert.Nil(suite.T(), err, "empty error")
	assert.NotNil(suite.T(), responce, "empty object")
}

func (suite *TreeqNodeSuite) Test_TreeqNodePublishVolume_MkdirAll_error() {
	service := treeqstorage{mounter: suite.nfsMountMock, osHelper: suite.osHelperMock}
	mountErr := errors.New("mount error")
	suite.nfsMountMock.On("IsLikelyNotMountPoint", mock.Anything).Return(true, mountErr)
	suite.osHelperMock.On("IsNotExist", mountErr).Return(true)
	suite.osHelperMock.On("MkdirAll", mock.Anything, mock.Anything).Return(mountErr)
	suite.nfsMountMock.On("Mount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	targetPath := "/var/lib/kublet/"
	responce, err := service.NodePublishVolume(context.Background(), getNodePublishVolumeRequest(targetPath, getPublishContexMap()))
	assert.Equal(suite.T(), err.Error(), mountErr.Error())
	assert.Nil(suite.T(), responce, "empty object")
}

func (suite *TreeqNodeSuite) Test_TreeqNodePublishVolume_IsNotExist_false() {
	service := treeqstorage{mounter: suite.nfsMountMock, osHelper: suite.osHelperMock}
	mountErr := errors.New("mount error")
	suite.nfsMountMock.On("IsLikelyNotMountPoint", mock.Anything).Return(true, mountErr)
	suite.osHelperMock.On("IsNotExist", mountErr).Return(false)
	suite.nfsMountMock.On("Mount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	targetPath := "/var/lib/kublet/"
	responce, err := service.NodePublishVolume(context.Background(), getNodePublishVolumeRequest(targetPath, getPublishContexMap()))
	assert.Equal(suite.T(), err.Error(), mountErr.Error())
	assert.Nil(suite.T(), responce, "empty object")
}

func (suite *TreeqNodeSuite) Test_TreeqNodePublishVolume_notMnt_false() {
	service := treeqstorage{mounter: suite.nfsMountMock, osHelper: suite.osHelperMock}
	suite.nfsMountMock.On("IsLikelyNotMountPoint", mock.Anything).Return(true, nil)
	suite.nfsMountMock.On("IsNotMountPoint", mock.Anything).Return(false, nil)
	suite.nfsMountMock.On("Mount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	targetPath := "/var/lib/kublet/"
	_, err := service.NodePublishVolume(context.Background(), getNodePublishVolumeRequest(targetPath, getPublishContexMap()))
	assert.Nil(suite.T(), err, "empty error")
}

func (suite *TreeqNodeSuite) Test_TreeqNodePublishVolume_mount_sucess() {
	service := treeqstorage{mounter: suite.nfsMountMock, osHelper: suite.osHelperMock}
	suite.nfsMountMock.On("IsLikelyNotMountPoint", mock.Anything).Return(true, nil)
	suite.nfsMountMock.On("IsNotMountPoint", mock.Anything).Return(true, nil)
	suite.nfsMountMock.On("Mount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	targetPath := "/var/lib/kublet/"
	_, err := service.NodePublishVolume(context.Background(), getNodePublishVolumeRequest(targetPath, getPublishContexMap()))
	assert.Nil(suite.T(), err, "empty error")
}

func (suite *TreeqNodeSuite) Test_TreeqNodePublishVolume_mount_Error() {
	mountErr := errors.New("mount error")
	service := treeqstorage{mounter: suite.nfsMountMock, osHelper: suite.osHelperMock}
	suite.nfsMountMock.On("IsLikelyNotMountPoint", mock.Anything).Return(true, nil)
	suite.nfsMountMock.On("IsNotMountPoint", mock.Anything).Return(true, nil)
	suite.nfsMountMock.On("Mount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(mountErr)
	targetPath := "/var/lib/kublet/"
	_, err := service.NodePublishVolume(context.Background(), getNodePublishVolumeRequest(targetPath, getPublishContexMap()))
	assert.NotNil(suite.T(), err, "not nil error")
}

func (suite *TreeqNodeSuite) Test_TreeqNodeUnpublishVolume_NotMountPoint_error() {
	mountErr := errors.New("mount error")
	service := treeqstorage{mounter: suite.nfsMountMock, osHelper: suite.osHelperMock}
	suite.nfsMountMock.On("IsLikelyNotMountPoint", mock.Anything).Return(true, mountErr)
	suite.osHelperMock.On("IsNotExist", mountErr).Return(false)
	suite.nfsMountMock.On("Mount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	suite.nfsMountMock.On("Unmount", mock.Anything).Return(nil)
	targetPath := "/var/lib/kublet/"
	suite.osHelperMock.On("Remove", targetPath).Return(nil)
	volumeID := "1234"
	_, err := service.NodeUnpublishVolume(context.Background(), getNodeUnPublishVolumeRequest(targetPath, volumeID))
	assert.NotNil(suite.T(), err, "not nil error")
}

func (suite *TreeqNodeSuite) Test_TreeqNodeUnpublishVolume_NotMountPoint_IsNotExist_true() {
	mountErr := errors.New("mount error")
	service := treeqstorage{mounter: suite.nfsMountMock, osHelper: suite.osHelperMock}
	suite.nfsMountMock.On("IsLikelyNotMountPoint", mock.Anything).Return(true, nil)
	suite.nfsMountMock.On("IsNotMountPoint", mock.Anything).Return(true, mountErr)
	suite.osHelperMock.On("IsNotExist", mountErr).Return(true)
	suite.nfsMountMock.On("Mount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	suite.nfsMountMock.On("Unmount", mock.Anything).Return(nil)
	targetPath := "/var/lib/kublet/"
	suite.osHelperMock.On("Remove", targetPath).Return(nil)
	volumeID := "1234"
	_, err := service.NodeUnpublishVolume(context.Background(), getNodeUnPublishVolumeRequest(targetPath, volumeID))
	assert.Nil(suite.T(), err, "empty error")
}

func (suite *TreeqNodeSuite) Test_TreeqNodeUnpublishVolume_NotMountPoint_IsNotExist_false() {
	mountErr := errors.New("not exists")
	service := treeqstorage{mounter: suite.nfsMountMock, osHelper: suite.osHelperMock}
	suite.nfsMountMock.On("IsLikelyNotMountPoint", mock.Anything).Return(true, mountErr)
	suite.osHelperMock.On("IsNotExist", mountErr).Return(false)
	suite.nfsMountMock.On("Mount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	suite.nfsMountMock.On("Unmount", mock.Anything).Return(nil)
	targetPath := "/var/lib/kublet/"
	suite.osHelperMock.On("Remove", targetPath).Return(nil)
	volumeID := "1234"
	_, err := service.NodeUnpublishVolume(context.Background(), getNodeUnPublishVolumeRequest(targetPath, volumeID))
	assert.NotNil(suite.T(), err, "not nil error")
}

func (suite *TreeqNodeSuite) Test_TreeqNodeUnpublishVolume_notMnt_true() {
	targetPath := "/var/lib/kublet/"
	volumeID := "1234"
	service := treeqstorage{mounter: suite.nfsMountMock, osHelper: suite.osHelperMock}
	suite.nfsMountMock.On("IsLikelyNotMountPoint", mock.Anything).Return(true, nil)
	suite.nfsMountMock.On("IsNotMountPoint", mock.Anything).Return(true, nil)
	suite.osHelperMock.On("Remove", targetPath).Return(nil)
	suite.nfsMountMock.On("Unmount", mock.Anything).Return(nil)

	_, err := service.NodeUnpublishVolume(context.Background(), getNodeUnPublishVolumeRequest(targetPath, volumeID))
	assert.Nil(suite.T(), err, "empty err")
}

func (suite *TreeqNodeSuite) Test_TreeqNodeUnpublishVolume_unmount_fail() {
	mountErr := errors.New("mount error")
	targetPath := "/var/lib/kublet/"
	volumeID := "1234"
	service := treeqstorage{mounter: suite.nfsMountMock, osHelper: suite.osHelperMock}
	suite.nfsMountMock.On("IsLikelyNotMountPoint", mock.Anything).Return(true, nil)
	suite.nfsMountMock.On("IsNotMountPoint", mock.Anything).Return(true, nil)
	suite.nfsMountMock.On("Mount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	suite.nfsMountMock.On("Unmount", targetPath).Return(mountErr)
	suite.nfsMountMock.On("Mount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	_, err := service.NodeUnpublishVolume(context.Background(), getNodeUnPublishVolumeRequest(targetPath, volumeID))

	assert.NotNil(suite.T(), err, "not nil error")
}

func (suite *TreeqNodeSuite) Test_TreeqNodeUnpublishVolume_unmount_sucess() {
	targetPath := "/var/lib/kublet/"
	volumeID := "1234"
	service := treeqstorage{mounter: suite.nfsMountMock, osHelper: suite.osHelperMock}
	suite.nfsMountMock.On("IsLikelyNotMountPoint", mock.Anything).Return(true, nil)
	suite.nfsMountMock.On("IsNotMountPoint", mock.Anything).Return(true, nil)
	suite.nfsMountMock.On("Unmount", targetPath).Return(nil)
	suite.osHelperMock.On("Remove", targetPath).Return(nil)
	_, err := service.NodeUnpublishVolume(context.Background(), getNodeUnPublishVolumeRequest(targetPath, volumeID))
	assert.Nil(suite.T(), err, "empty err")
}

func (suite *TreeqNodeSuite) Test_NodeStageVolume() {

	service := treeqstorage{mounter: suite.nfsMountMock, osHelper: suite.osHelperMock}

	_, err := service.NodeStageVolume(context.Background(), &csi.NodeStageVolumeRequest{})
	assert.Nil(suite.T(), err, "empty err")
}

func (suite *TreeqNodeSuite) Test_NodeUnstageVolume() {

	service := treeqstorage{mounter: suite.nfsMountMock, osHelper: suite.osHelperMock}

	_, err := service.NodeUnstageVolume(context.Background(), &csi.NodeUnstageVolumeRequest{})
	assert.Nil(suite.T(), err, "empty err")
}
