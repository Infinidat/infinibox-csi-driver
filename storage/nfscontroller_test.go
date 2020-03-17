package storage

import (
	"context"
	"errors"
	"infinibox-csi-driver/api"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

func (suite *NFSControllerSuite) SetupTest() {
	suite.api = new(api.MockApiService)
	suite.cs = &commonservice{api: suite.api}
}

type NFSControllerSuite struct {
	suite.Suite
	api *api.MockApiService
	cs  *commonservice
}

func TestNfsControllerSuite(t *testing.T) {
	suite.Run(t, new(NFSControllerSuite))
}

func (suite *NFSControllerSuite) Test_NfsControllerExpandVolume_VolumeID_empty() {
	service := nfsstorage{cs: *suite.cs}
	_, err := service.ControllerExpandVolume(context.Background(), getNfsExpandVolumeRequest(""))
	assert.NotNil(suite.T(), err, "Volume ID missing in request")
}

func (suite *NFSControllerSuite) Test_NfsControllerExpandVolume_InvalidVolumeID() {
	volumeID := "100"
	service := nfsstorage{cs: *suite.cs}
	_, err := service.ControllerExpandVolume(context.Background(), getNfsExpandVolumeRequest(volumeID))
	assert.NotNil(suite.T(), err, "Volume ID missing in request")
}

func (suite *NFSControllerSuite) Test_NfsControllerExpandVolume_Error() {
	fileSystemID := "100#"
	fileSystem := api.FileSystem{}
	expectedErr := errors.New("Some error")
	suite.api.On("UpdateFilesystem", fileSystemID, fileSystem).Return(expectedErr)
	service := nfsstorage{cs: *suite.cs}
	_, err := service.ControllerExpandVolume(context.Background(), getNfsExpandVolumeRequest(fileSystemID))
	assert.NotNil(suite.T(), err, "error expected")
}

func (suite *NFSControllerSuite) Test_NfsControllerExpandVolume_Error_filenotfound() {
	service := nfsstorage{cs: *suite.cs}
	fileSystemID := "100#"
	fileSystem := api.FileSystem{}
	expectedErr := errors.New("Some error")
	suite.api.On("UpdateVolume", fileSystemID, fileSystem).Return(expectedErr)
	_, err := service.ControllerExpandVolume(context.Background(), getNfsExpandVolumeRequest(fileSystemID))
	assert.NotNil(suite.T(), err, "error expected")
}

func (suite *NFSControllerSuite) Test_NfsControllerExpandVolume_success() {
	service := nfsstorage{cs: *suite.cs}
	fileSystemID := "100#"
	fileSystem := api.FileSystem{}
	suite.api.On("UpdateVolume", fileSystemID, fileSystem).Return(nil)
	_, err := service.ControllerExpandVolume(context.Background(), getNfsExpandVolumeRequest(fileSystemID))
	assert.NotNil(suite.T(), err, "error expected")
}

func (suite *NFSControllerSuite) Test_NfsCreateSnapshot_GetSnapshot_Error() {
	expectedErr := errors.New("Snapshot Name is must")
	suite.api.On("GetSnapshotByName", mock.Anything).Return(nil, expectedErr)
	service := nfsstorage{cs: *suite.cs}
	_, err := service.CreateSnapshot(context.Background(), getNfsCreateSnapshotRequest("100"))
	assert.NotNil(suite.T(), err, "empty error")
}

func (suite *NFSControllerSuite) Test_NfsCreateSnapshot_SourceVolumeID_Error() {
	fileSystem := api.FileSystem{}
	expectedErr := errors.New("SourceVolumeID is must")
	suite.api.On("GetSnapshotByName", mock.Anything).Return(fileSystem, nil)
	suite.api.On("CreateFileSystemSnapshot", mock.Anything).Return(nil, expectedErr)
	service := nfsstorage{cs: *suite.cs}
	_, err := service.CreateSnapshot(context.Background(), getNfsCreateSnapshotRequest("100"))
	assert.NotNil(suite.T(), err, "empty error")
}

func (suite *NFSControllerSuite) Test_NfsCreateSnapshot_Success() {
	fileSystem := api.FileSystem{}
	snapshotParam := &api.FileSystemSnapshot{
		ParentID:       100,
		SnapshotName:   "test_snapshot",
		WriteProtected: true,
	}
	expectedErr := errors.New("SourceVolumeID is must")
	suite.api.On("GetSnapshotByName", "test_snapshot").Return(fileSystem, nil)
	suite.api.On("CreateFileSystemSnapshot", snapshotParam).Return(nil, expectedErr)
	service := nfsstorage{cs: *suite.cs}
	result, err := service.CreateSnapshot(context.Background(), getNfsCreateSnapshotRequest("100"))
	assert.NotNil(suite.T(), err, "empty error")
	val := result.GetSnapshot().GetSnapshotId()
	assert.Equal(suite.T(), "", val, "ID shoulde be equal")
}

func (suite *NFSControllerSuite) Test_NfsDeleteSnapshot_SourceVolumeID_empty() {
	service := nfsstorage{cs: *suite.cs}
	var snapshotID int64
	expectedErr := errors.New("Invalid Source ID")
	suite.api.On("GetFileSystemByID", snapshotID).Return(nil, expectedErr)
	_, err := service.DeleteSnapshot(context.Background(), getNfsDeleteSnapshotRequest(""))
	assert.NotNil(suite.T(), err, "Source Volume ID missing in request")
}

func (suite *NFSControllerSuite) Test_NfsDeleteSnapshot_InvalidSourceVolumeID() {
	service := nfsstorage{cs: *suite.cs}
	var snapshotID int64 = 1000000000000000000
	expectedErr := errors.New("Invalid Source ID")
	suite.api.On("GetFileSystemByID", snapshotID).Return(nil, expectedErr)
	_, err := service.DeleteSnapshot(context.Background(), getNfsDeleteSnapshotRequest("1000000000000000000"))
	assert.NotNil(suite.T(), err, "Invalid Snapshot ID in request")
}

func (suite *NFSControllerSuite) Test_NfsDeleteSnapshot_Error() {
	service := nfsstorage{cs: *suite.cs, uniqueID: 100}
	var snapshotID int64 = 100
	expectedErr := errors.New("FILESYSTEM_NOT_FOUND")
	suite.api.On("GetFileSystemByID", snapshotID).Return(nil, expectedErr)
	_, err := service.DeleteSnapshot(context.Background(), getNfsDeleteSnapshotRequest("100"))
	assert.NotNil(suite.T(), err, "error expected")
}

func (suite *NFSControllerSuite) Test_NfsDeleteNFSVolume_GetFileSystemByID_error() {
	service := nfsstorage{cs: *suite.cs, uniqueID: 100}
	var snapshotID int64 = 100
	expectedErr := errors.New("FILESYSTEM_NOT_FOUND")
	suite.api.On("GetFileSystemByID", snapshotID).Return(nil, expectedErr)
	err := service.DeleteNFSVolume()
	assert.NotNil(suite.T(), err, "Error should not be nil")
	assert.Equal(suite.T(), expectedErr, err, "Error not returned as expected")
}

func (suite *NFSControllerSuite) Test_NfsDeleteNFSVolume_GetFileSystemByID_InvalidID() {
	service := nfsstorage{cs: *suite.cs, uniqueID: 100}
	snapshotID := 100999999999999
	expectedErr := errors.New("Invalid_ID")
	suite.api.On("GetFileSystemByID", snapshotID).Return(nil, expectedErr)
	err := service.DeleteNFSVolume()
	assert.NotNil(suite.T(), err, "Error should not be nil")
}

//GetVolumeSnapshotByParentID
func (suite *NFSControllerSuite) Test_NfsDeleteNFSVolume_AttachMetadataToObject_error() {
	service := nfsstorage{cs: *suite.cs, uniqueID: 100}
	var snapshotID int64 = 100
	fileSystem := api.FileSystem{}
	expectedErr := errors.New("error while Set metadata host.k8s.to_be_deleted")
	metadata := make(map[string]interface{})
	metadata["host.k8s.to_be_deleted"] = true
	suite.api.On("GetFileSystemByID", snapshotID).Return(fileSystem, nil)
	suite.api.On("FileSystemHasChild", snapshotID).Return(true)
	suite.api.On("AttachMetadataToObject", snapshotID, metadata).Return(nil, expectedErr)
	err := service.DeleteNFSVolume()
	assert.NotNil(suite.T(), err, "Error should not be nil")
}

func (suite *NFSControllerSuite) Test_NfsDeleteNFSVolume_DeleteFileSystemComplete_error() {
	service := nfsstorage{cs: *suite.cs, uniqueID: 100}
	var snapshotID, parentID int64 = 100, 200
	fileSystem := api.FileSystem{}
	expectedErr := errors.New("Failed to delete complete fileSystem")
	metadata := make(map[string]interface{})
	metadata["host.k8s.to_be_deleted"] = true
	suite.api.On("GetFileSystemByID", snapshotID).Return(fileSystem, nil)
	suite.api.On("FileSystemHasChild", snapshotID).Return(true)
	suite.api.On("AttachMetadataToObject", snapshotID, metadata).Return(nil, nil)
	suite.api.On("GetParentID", snapshotID).Return(parentID)
	suite.api.On("DeleteFileSystemComplete", snapshotID).Return(expectedErr)
	err := service.DeleteNFSVolume()
	assert.NotNil(suite.T(), err, "Error should not be nil")
	assert.Equal(suite.T(), expectedErr, err, "Error not returned as expected")
}

func (suite *NFSControllerSuite) Test_NfsDeleteNFSVolume_DeleteParentFileSystem_error() {
	service := nfsstorage{cs: *suite.cs, uniqueID: 100}
	var snapshotID, parentID int64 = 100, 200
	fileSystem := api.FileSystem{}
	expectedErr := errors.New("Failed to delete complete fileSystem")
	metadata := make(map[string]interface{})
	metadata["host.k8s.to_be_deleted"] = true
	suite.api.On("GetFileSystemByID", snapshotID).Return(fileSystem, nil)
	suite.api.On("FileSystemHasChild", snapshotID).Return(true)
	suite.api.On("AttachMetadataToObject", snapshotID, metadata).Return(nil, nil)
	suite.api.On("GetParentID", snapshotID).Return(parentID)
	suite.api.On("DeleteFileSystemComplete", snapshotID).Return(nil)
	suite.api.On("DeleteParentFileSystem", parentID).Return(expectedErr)
	err := service.DeleteNFSVolume()
	assert.NotNil(suite.T(), err, "Error should not be nil")
	assert.Equal(suite.T(), expectedErr, err, "Error not returned as expected")
}

func (suite *NFSControllerSuite) Test_NfsDeleteNFSVolume_Success() {
	service := nfsstorage{cs: *suite.cs, uniqueID: 100}
	var snapshotID, parentID int64 = 100, 200
	fileSystem := api.FileSystem{}
	metadata := make(map[string]interface{})
	metadata["host.k8s.to_be_deleted"] = true
	suite.api.On("GetFileSystemByID", snapshotID).Return(fileSystem, nil)
	suite.api.On("FileSystemHasChild", snapshotID).Return(true)
	suite.api.On("AttachMetadataToObject", snapshotID, metadata).Return(nil, nil)
	suite.api.On("GetParentID", snapshotID).Return(parentID)
	suite.api.On("DeleteFileSystemComplete", snapshotID).Return(nil)
	suite.api.On("DeleteParentFileSystem", parentID).Return(nil)
	err := service.DeleteNFSVolume()
	assert.Nil(suite.T(), err, "Error should be nil")
}

func getNfsCreateSnapshotRequest(vID string) *csi.CreateSnapshotRequest {
	return &csi.CreateSnapshotRequest{
		SourceVolumeId: vID,
	}
}

func getNfsDeleteSnapshotRequest(sID string) *csi.DeleteSnapshotRequest {
	return &csi.DeleteSnapshotRequest{
		SnapshotId: sID,
	}
}

func getNfsExpandVolumeRequest(vID string) *csi.ControllerExpandVolumeRequest {
	return &csi.ControllerExpandVolumeRequest{
		VolumeId: vID,
	}
}
