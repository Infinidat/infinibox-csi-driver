package storage

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"k8s.io/kubernetes/pkg/util/mount"
)

func (suite *NodeSuite) SetupTest() {
	suite.nfsMountMock = new(MockNfsMounter)
	suite.osmock = new(mockOS)
}

type NodeSuite struct {
	suite.Suite
	nfsMountMock *MockNfsMounter
	osmock       *mockOS
}

func TestNodeSuite(t *testing.T) {
	suite.Run(t, new(NodeSuite))
}

func (suite *NodeSuite) Test_NodePublishVolume_mnt_false() {
	service := nfsstorage{mounter: suite.nfsMountMock}
	suite.nfsMountMock.On("IsLikelyNotMountPoint", mock.Anything).Return(false, nil)
	targetPath := "/var/lib/kublet/"
	responce, err := service.NodePublishVolume(context.Background(), getNodePublishVolumeRequest(targetPath, getPublishContexMap()))
	assert.Nil(suite.T(), err, "empty object")
	assert.NotNil(suite.T(), responce, "empty object")
}

func (suite *NodeSuite) Test_NodePublishVolume_mnt_true() {
	service := nfsstorage{mounter: suite.nfsMountMock}
	mountErr := errors.New("mount error")
	suite.nfsMountMock.On("IsLikelyNotMountPoint", mock.Anything).Return(false, mountErr)
	suite.osmock.On("IsNotExist", mountErr).Return(true)
	targetPath := "/var/lib/kublet/"
	responce, err := service.NodePublishVolume(context.Background(), getNodePublishVolumeRequest(targetPath, getPublishContexMap()))
	assert.Equal(suite.T(), err.Error(), mountErr.Error())
	assert.Nil(suite.T(), responce, "empty object")
}

func (suite *NodeSuite) Test_NodePublishVolume_success() {
	service := nfsstorage{mounter: suite.nfsMountMock}

	suite.nfsMountMock.On("IsLikelyNotMountPoint", mock.Anything).Return(true, nil)
	suite.nfsMountMock.On("Mount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	targetPath := "/var/lib/kublet/"
	_, err := service.NodePublishVolume(context.Background(), getNodePublishVolumeRequest(targetPath, getPublishContexMap()))

	assert.Nil(suite.T(), err, " error should be nil")
}

func (suite *NodeSuite) Test_NodeUnpublishVolume_MountPoint_fail() {
	service := nfsstorage{mounter: suite.nfsMountMock}
	volumeID := "1234"
	mountErr := errors.New("some error")
	suite.nfsMountMock.On("IsLikelyNotMountPoint", mock.Anything).Return(false, mountErr)
	targetPath := "/var/lib/kublet/"
	_, err := service.NodeUnpublishVolume(context.Background(), getNodeUnPublishVolumeRequest(targetPath, volumeID))
	assert.Equal(suite.T(), err.Error(), mountErr.Error())
}

func (suite *NodeSuite) Test_NodeUnpublishVolume_MountPoint_dir_notexist() {
	service := nfsstorage{mounter: suite.nfsMountMock}
	volumeID := "1234"
	mountErr := os.ErrNotExist
	suite.nfsMountMock.On("IsLikelyNotMountPoint", mock.Anything).Return(false, mountErr)
	suite.osmock.On("IsNotExist", mountErr).Return(true)
	targetPath := "/var/lib/kublet/"
	_, err := service.NodeUnpublishVolume(context.Background(), getNodeUnPublishVolumeRequest(targetPath, volumeID))
	assert.Nil(suite.T(), err, " error should be nil")
}

func (suite *NodeSuite) Test_NodeUnpublishVolume_unmount_error() {
	service := nfsstorage{mounter: suite.nfsMountMock}
	volumeID := "1234"
	mountErr := errors.New("some error")
	suite.nfsMountMock.On("IsLikelyNotMountPoint", mock.Anything).Return(false, nil)
	suite.nfsMountMock.On("Unmount", mock.Anything).Return(mountErr)
	targetPath := "/var/lib/kublet/"
	_, err := service.NodeUnpublishVolume(context.Background(), getNodeUnPublishVolumeRequest(targetPath, volumeID))
	assert.NotNil(suite.T(), err, " error should be nil")
}

//**************************
func getNodePublishVolumeRequest(tagetPath string, publishContexMap map[string]string) *csi.NodePublishVolumeRequest {
	return &csi.NodePublishVolumeRequest{
		TargetPath:     tagetPath,
		PublishContext: publishContexMap,
	}
}
func getPublishContexMap() map[string]string {
	contextMap := make(map[string]string)
	contextMap["nfs_mount_options"] = "hard,rsize=1048576,wsize=1048576"
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

//OS package -- mock method
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

//MockNfsMounter - mount mock
type MockNfsMounter struct {
	mount.Interface
	mock.Mock
}

func (m *MockNfsMounter) IsLikelyNotMountPoint(file string) (bool, error) {
	args := m.Called(file)
	resp, _ := args.Get(0).(bool)
	err, _ := args.Get(1).(error)
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
	err := args.Get(0).(error)
	return err
}
