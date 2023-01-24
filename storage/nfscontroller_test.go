package storage

import (
	"context"
	"errors"
	"infinibox-csi-driver/api"
	"infinibox-csi-driver/helper"
	tests "infinibox-csi-driver/test_helper"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

func (suite *NFSControllerSuite) SetupTest() {
	suite.api = new(api.MockApiService)
	suite.accessMock = new(helper.MockAccessModesHelper)
	suite.cs = &commonservice{api: suite.api, accessModesHelper: suite.accessMock}

	tests.ConfigureKlog()
}

type NFSControllerSuite struct {
	suite.Suite
	api        *api.MockApiService
	accessMock *helper.MockAccessModesHelper
	cs         *commonservice
}

func TestNfsControllerSuite(t *testing.T) {
	suite.Run(t, new(NFSControllerSuite))
}

func (suite *NFSControllerSuite) Test_CreateVolume_paramerValidation_Fail() {
	service := nfsstorage{cs: *suite.cs}
	parameterMap := getCreateVolumeParameter()
	delete(parameterMap, "pool_name")
	createVolReq := getNFSCreateVolumeRequest("PVName", parameterMap)
	_, err := service.CreateVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: parameter validation ")
}

func (suite *NFSControllerSuite) Test_CreateVolume_NetworkSpaceIP_Error() {
	service := nfsstorage{cs: *suite.cs}
	parameterMap := getCreateVolumeParameter()
	createVolReq := getNFSCreateVolumeRequest("PVName", parameterMap)

	networkSpaceErr := errors.New("Some error")

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(nil, networkSpaceErr)
	_, err := service.CreateVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: get IP address from networkspace")
}

func (suite *NFSControllerSuite) Test_CreateVolume_GetFileSystemByName_Error() {
	service := nfsstorage{cs: *suite.cs}
	parameterMap := getCreateVolumeParameter()
	createVolReq := getNFSCreateVolumeRequest("PVName", parameterMap)
	filesystemErr := errors.New("Some error")

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkSpace(), nil)
	suite.api.On("GetFileSystemByName", mock.Anything).Return(nil, filesystemErr)
	suite.api.On("GetStoragePoolIDByName", parameterMap["pool_name"]).Return(100, nil)
	suite.api.On("OneTimeValidation", mock.Anything, mock.Anything).Return(nil, nil)
	suite.api.On("CreateFilesystem", mock.Anything).Return(getFileSystem(), nil)

	_, err := service.CreateVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: get filesystem by name")
}

func (suite *NFSControllerSuite) Test_CreateVolume_FileNameExist_exportError() {
	service := nfsstorage{cs: *suite.cs}
	parameterMap := getCreateVolumeParameter()
	createVolReq := getNFSCreateVolumeRequest("PVName", parameterMap)
	expectedError := errors.New("Some error")

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkSpace(), nil)
	suite.api.On("GetFileSystemByName", mock.Anything).Return(getFileSystem(), nil)
	suite.api.On("GetExportByFileSystem", mock.Anything).Return(nil, expectedError)

	_, err := service.CreateVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: get export by filesystem")
}

/*
func (suite *NFSControllerSuite) Test_CreateVolume_FileNameExist_exportArryEmpty() {
	service := nfsstorage{cs: *suite.cs}
	parameterMap := getCreateVolumeParameter()
	createVolReq := getNFSCreateVolumeRequest("PVName", parameterMap)
	exportResp := &[]api.ExportResponse{}

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkSpace(), nil)
	suite.api.On("GetFileSystemByName", mock.Anything).Return(getFileSystem(), nil)
	suite.api.On("GetExportByFileSystem", mock.Anything).Return(exportResp, nil)

	_, err := service.CreateVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: fetching export details")
}
*/

func (suite *NFSControllerSuite) Test_CreateVolume_FileNameExist_sucess() {
	service := nfsstorage{cs: *suite.cs}
	parameterMap := getCreateVolumeParameter()
	createVolReq := getNFSCreateVolumeRequest("PVName", parameterMap)

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkSpace(), nil)
	suite.api.On("GetFileSystemByName", mock.Anything).Return(getFileSystem(), nil)
	suite.api.On("GetExportByFileSystem", mock.Anything).Return(getExportPath(), nil)

	resp, err := service.CreateVolume(context.Background(), createVolReq)
	assert.Nil(suite.T(), err, "expected to succeed: CreateVolume when file system exists")
	assert.NotNil(suite.T(), resp, "CreateVolume ok response should be non-empty")
}

func (suite *NFSControllerSuite) Test_CreateVolume_OneTimeValidation_fail() {
	service := nfsstorage{cs: *suite.cs}
	parameterMap := getCreateVolumeParameter()
	createVolReq := getNFSCreateVolumeRequest("PVName", parameterMap)
	expectedError := errors.New("Some error")

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkSpace(), nil)
	suite.api.On("GetFileSystemByName", mock.Anything).Return(nil, nil)
	suite.api.On("OneTimeValidation", mock.Anything, mock.Anything).Return("", expectedError)

	_, err := service.CreateVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: CreateVolume validate the pool and networkspace")
}

func (suite *NFSControllerSuite) Test_CreateVolume_StoragePoolIDByName_Error() {
	service := nfsstorage{cs: *suite.cs}
	parameterMap := getCreateVolumeParameter()
	createVolReq := getNFSCreateVolumeRequest("PVName", parameterMap)
	expectedError := errors.New("failed to create volume Some error")

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkSpace(), nil)
	suite.api.On("GetFileSystemByName", mock.Anything).Return(nil, nil)
	suite.api.On("OneTimeValidation", mock.Anything, mock.Anything).Return("networkspace", nil)
	suite.api.On("GetStoragePoolIDByName", parameterMap["pool_name"]).Return(0, expectedError)

	_, err := service.CreateVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: CreateVolume get poolID by poolName")
	assert.Equal(suite.T(), err.Error(), expectedError.Error(), "expected to get the mocked err")
}

func (suite *NFSControllerSuite) Test_CreateVolume_CreateFilesystem_Error() {
	service := nfsstorage{cs: *suite.cs}
	parameterMap := getCreateVolumeParameter()
	createVolReq := getNFSCreateVolumeRequest("PVName", parameterMap)
	expectedError := errors.New("failed to create volume Some error")

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkSpace(), nil)
	suite.api.On("GetFileSystemByName", mock.Anything).Return(nil, nil)
	suite.api.On("OneTimeValidation", mock.Anything, mock.Anything).Return("networkspace", nil)
	suite.api.On("GetStoragePoolIDByName", parameterMap["pool_name"]).Return(100, nil)
	suite.api.On("CreateFilesystem", mock.Anything).Return(0, expectedError)

	_, err := service.CreateVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: CreateVolume create the file system")
	assert.Equal(suite.T(), err.Error(), expectedError.Error(), "expected to get the mocked err")
}

func (suite *NFSControllerSuite) Test_CreateVolume_createExportPath_Error() {
	service := nfsstorage{cs: *suite.cs}
	parameterMap := getCreateVolumeParameter()
	createVolReq := getNFSCreateVolumeRequest("PVName", parameterMap)
	expectedError := errors.New("failed to create volume Some error")

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkSpace(), nil)
	suite.api.On("GetFileSystemByName", mock.Anything).Return(nil, nil)
	suite.api.On("OneTimeValidation", mock.Anything, mock.Anything).Return("networkspace", nil)
	suite.api.On("GetStoragePoolIDByName", parameterMap["pool_name"]).Return(100, nil)
	suite.api.On("CreateFilesystem", mock.Anything).Return(1, nil)
	suite.api.On("ExportFileSystem", mock.Anything).Return(nil, expectedError)

	_, err := service.CreateVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: create export path")
	assert.Equal(suite.T(), err.Error(), expectedError.Error(), "expected to get the mocked err")
}

func (suite *NFSControllerSuite) Test_CreateVolume_success() {
	service := nfsstorage{cs: *suite.cs}
	parameterMap := getCreateVolumeParameter()
	createVolReq := getNFSCreateVolumeRequest("PVName", parameterMap)

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkSpace(), nil)
	suite.api.On("GetFileSystemByName", mock.Anything).Return(nil, nil)
	suite.api.On("GetFileSystemByID", mock.Anything).Return(getFileSystem(), nil)
	suite.api.On("OneTimeValidation", mock.Anything, mock.Anything).Return("networkspace", nil)
	suite.api.On("GetStoragePoolIDByName", parameterMap["pool_name"]).Return(100, nil)
	suite.api.On("CreateFilesystem", mock.Anything).Return(getFileSystem(), nil)

	suite.api.On("ExportFileSystem", mock.Anything).Return(getExportResponseValue(), nil)
	suite.api.On("AttachMetadataToObject", mock.Anything, mock.Anything).Return(nil, nil)

	resp, err := service.CreateVolume(context.Background(), createVolReq)
	assert.Nil(suite.T(), err, "expected succeed: CreateVolume create the file system")
	assert.Equal(suite.T(), resp.GetVolume().GetVolumeId(), "1", "expected to get volume ID")
}

//=================================================Create Volume END=================================//

func (suite *NFSControllerSuite) Test_CreateVolume_Snapshot_Invalid_volumeID() {
	service := nfsstorage{cs: *suite.cs}
	parameterMap := getCreateVolumeParameter()
	createVolReq := getCreateVolumeSnapshotRequest("PVName", parameterMap)
	createVolReq.GetVolumeContentSource().GetSnapshot().SnapshotId = "a$$nfs"

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkSpace(), nil)
	suite.api.On("GetFileSystemByName", mock.Anything).Return(nil, nil)

	_, err := service.CreateVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err.Error(), "failed to get filesystem name")
}

func (suite *NFSControllerSuite) Test_CreateVolume_Snapshot_Invalid_volumeID2() {
	service := nfsstorage{cs: *suite.cs}
	parameterMap := getCreateVolumeParameter()
	createVolReq := getCreateVolumeSnapshotRequest("PVName", parameterMap)
	createVolReq.GetVolumeContentSource().GetSnapshot().SnapshotId = "a$$nfs$$123"

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkSpace(), nil)
	suite.api.On("GetFileSystemByName", mock.Anything).Return(nil, nil)

	_, err := service.CreateVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err.Error(), "invalid size")
}

func (suite *NFSControllerSuite) Test_CreateVolume_Snapshot_GetFileSystemByID_Error() {
	service := nfsstorage{cs: *suite.cs}
	parameterMap := getCreateVolumeParameter()
	createVolReq := getCreateVolumeSnapshotRequest("PVName", parameterMap)
	createVolReq.GetVolumeContentSource().GetSnapshot().SnapshotId = "1$$nfs"
	filesystemErr := errors.New("Some error")

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkSpace(), nil)
	suite.api.On("GetFileSystemByName", mock.Anything).Return(nil, nil)
	suite.api.On("GetFileSystemByID", mock.Anything).Return(nil, filesystemErr)

	_, err := service.CreateVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err.Error(), "failed to get filesystemID")
}

func (suite *NFSControllerSuite) Test_CreateVolume_Snapshot_invalidSize() {
	service := nfsstorage{cs: *suite.cs}
	parameterMap := getCreateVolumeParameter()
	createVolReq := getCreateVolumeSnapshotRequest("PVName", parameterMap)
	createVolReq.GetVolumeContentSource().GetSnapshot().SnapshotId = "1$$nfs"

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkSpace(), nil)
	suite.api.On("GetFileSystemByName", mock.Anything).Return(nil, nil)
	suite.api.On("GetFileSystemByID", mock.Anything).Return(getFileSystem(), nil)
	suite.api.On("GetStoragePoolIDByName", mock.Anything).Return(1, nil)

	_, err := service.CreateVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err.Error(), "invalid snapshot size")
}

func (suite *NFSControllerSuite) Test_CreateVolume_Snapshot_GetStoragePoolIDByName_error() {
	service := nfsstorage{cs: *suite.cs}
	parameterMap := getCreateVolumeParameter()
	createVolReq := getCreateVolumeSnapshotRequest("PVName", parameterMap)
	createVolReq.GetVolumeContentSource().GetSnapshot().SnapshotId = "1$$nfs"
	filesystemErr := errors.New("Some error")

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkSpace(), nil)
	suite.api.On("GetFileSystemByName", mock.Anything).Return(nil, nil)
	suite.api.On("GetFileSystemByID", mock.Anything).Return(getFileSystem(), nil)
	suite.api.On("GetStoragePoolIDByName", mock.Anything).Return(nil, filesystemErr)

	_, err := service.CreateVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err.Error(), "failed to storage pool name")
}

func (suite *NFSControllerSuite) Test_CreateVolume_Snapshot_poolID_name_invalid() {
	service := nfsstorage{cs: *suite.cs}
	parameterMap := getCreateVolumeParameter()
	createVolReq := getCreateVolumeSnapshotRequest("PVName", parameterMap)
	createVolReq.GetVolumeContentSource().GetSnapshot().SnapshotId = "1$$nfs"

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkSpace(), nil)
	suite.api.On("GetFileSystemByName", mock.Anything).Return(nil, nil)
	suite.api.On("GetFileSystemByID", mock.Anything).Return(getFileSystem(), nil)
	var poolID int64 = 101
	suite.api.On("GetStoragePoolIDByName", mock.Anything).Return(poolID, nil)

	_, err := service.CreateVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err.Error(), "failed to get PooldID by  storage pool name")
}

func (suite *NFSControllerSuite) Test_CreateVolume_Snapshot_createSnapshot_failed() {
	service := nfsstorage{cs: *suite.cs}
	parameterMap := getCreateVolumeParameter()
	createVolReq := getCreateVolumeSnapshotRequest("PVName", parameterMap)
	createVolReq.GetVolumeContentSource().GetSnapshot().SnapshotId = "1$$nfs"
	filesystemErr := errors.New("Some error")

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkSpace(), nil)
	suite.api.On("GetFileSystemByName", mock.Anything).Return(nil, nil)
	suite.api.On("GetFileSystemByID", mock.Anything).Return(getFileSystem(), nil)
	var poolID int64 = 100
	suite.api.On("GetStoragePoolIDByName", mock.Anything).Return(poolID, nil)
	suite.api.On("CreateFileSystemSnapshot", mock.Anything).Return(nil, filesystemErr)

	_, err := service.CreateVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err.Error(), "failed to create snapshot")
}

func (suite *NFSControllerSuite) Test_CreateVolume_Snapshot_exportPath_failed() {
	service := nfsstorage{cs: *suite.cs}
	parameterMap := getCreateVolumeParameter()
	createVolReq := getCreateVolumeSnapshotRequest("PVName", parameterMap)
	createVolReq.GetVolumeContentSource().GetSnapshot().SnapshotId = "1$$nfs"
	filesystemErr := errors.New("Some error")

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkSpace(), nil)
	suite.api.On("GetFileSystemByName", mock.Anything).Return(nil, nil)
	suite.api.On("GetFileSystemByID", mock.Anything).Return(getFileSystem(), nil)
	var poolID int64 = 100
	suite.api.On("GetStoragePoolIDByName", mock.Anything).Return(poolID, nil)
	suite.api.On("CreateFileSystemSnapshot", mock.Anything).Return(GetFileSystemSnapshotResponce(1), nil)
	suite.api.On("ExportFileSystem", mock.Anything).Return(nil, filesystemErr)

	_, err := service.CreateVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err.Error(), "failed to get export path")
}

func (suite *NFSControllerSuite) Test_CreateVolume_Snapshot_metadatafailed() {
	service := nfsstorage{cs: *suite.cs}
	parameterMap := getCreateVolumeParameter()
	createVolReq := getCreateVolumeSnapshotRequest("PVName", parameterMap)
	createVolReq.GetVolumeContentSource().GetSnapshot().SnapshotId = "1$$nfs"
	filesystemErr := errors.New("Some error")

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkSpace(), nil)
	suite.api.On("GetFileSystemByName", mock.Anything).Return(nil, nil)
	suite.api.On("GetFileSystemByID", mock.Anything).Return(getFileSystem(), nil)
	var poolID int64 = 100
	suite.api.On("GetStoragePoolIDByName", mock.Anything).Return(poolID, nil)
	suite.api.On("CreateFileSystemSnapshot", mock.Anything).Return(GetFileSystemSnapshotResponce(1), nil)
	suite.api.On("ExportFileSystem", mock.Anything).Return(getExportResponseValue(), nil)
	suite.api.On("AttachMetadataToObject", mock.Anything, mock.Anything).Return(nil, filesystemErr)

	_, err := service.CreateVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err.Error(), "failed to update metadata")
}

func (suite *NFSControllerSuite) Test_CreateVolume_Snapshot_Success() {
	service := nfsstorage{cs: *suite.cs}
	parameterMap := getCreateVolumeParameter()
	createVolReq := getCreateVolumeSnapshotRequest("PVName", parameterMap)
	createVolReq.GetVolumeContentSource().GetSnapshot().SnapshotId = "1$$nfs"

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkSpace(), nil)
	suite.api.On("GetFileSystemByName", mock.Anything).Return(nil, nil)
	suite.api.On("GetFileSystemByID", mock.Anything).Return(getFileSystem(), nil)
	var poolID int64 = 100
	suite.api.On("GetStoragePoolIDByName", mock.Anything).Return(poolID, nil)
	suite.api.On("CreateFileSystemSnapshot", mock.Anything).Return(GetFileSystemSnapshotResponce(1), nil)
	suite.api.On("ExportFileSystem", mock.Anything).Return(getExportResponseValue(), nil)
	suite.api.On("AttachMetadataToObject", mock.Anything, mock.Anything).Return(nil, nil)

	_, err := service.CreateVolume(context.Background(), createVolReq)
	assert.Nil(suite.T(), err, "expected to succeed: CreateSnapshot")
}

func (suite *NFSControllerSuite) Test_CreateVolume_Clone_Success() {
	service := nfsstorage{cs: *suite.cs}
	parameterMap := getCreateVolumeParameter()
	createVolReq := getCreateVolumeCloneRequest("PVName", parameterMap)
	createVolReq.GetVolumeContentSource().GetVolume().VolumeId = "1$$nfs"

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkSpace(), nil)
	suite.api.On("GetFileSystemByName", mock.Anything).Return(nil, nil)
	suite.api.On("GetFileSystemByID", mock.Anything).Return(getFileSystem(), nil)
	var poolID int64 = 100
	suite.api.On("GetStoragePoolIDByName", mock.Anything).Return(poolID, nil)
	suite.api.On("CreateFileSystemSnapshot", mock.Anything).Return(GetFileSystemSnapshotResponce(1), nil)
	suite.api.On("ExportFileSystem", mock.Anything).Return(getExportResponseValue(), nil)
	suite.api.On("AttachMetadataToObject", mock.Anything, mock.Anything).Return(nil, nil)

	_, err := service.CreateVolume(context.Background(), createVolReq)
	assert.Nil(suite.T(), err, "expected clone success")
}

func (suite *NFSControllerSuite) Test_CreateVolume_Clone_failed() {
	service := nfsstorage{cs: *suite.cs}
	parameterMap := getCreateVolumeParameter()
	createVolReq := getCreateVolumeCloneRequest("PVName", parameterMap)
	createVolReq.GetVolumeContentSource().GetVolume().VolumeId = "1$$nfs"
	filesystemErr := errors.New("Some error")

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkSpace(), nil)
	suite.api.On("GetFileSystemByName", mock.Anything).Return(nil, nil)
	suite.api.On("GetFileSystemByID", mock.Anything).Return(getFileSystem(), nil)
	var poolID int64 = 100
	suite.api.On("GetStoragePoolIDByName", mock.Anything).Return(poolID, nil)
	suite.api.On("CreateFileSystemSnapshot", mock.Anything).Return(GetFileSystemSnapshotResponce(1), nil)
	suite.api.On("ExportFileSystem", mock.Anything).Return(getExportResponseValue(), nil)
	suite.api.On("AttachMetadataToObject", mock.Anything, mock.Anything).Return(nil, filesystemErr)

	_, err := service.CreateVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err.Error(), "failed to clone the volume")
}

//===========================================================================
func (suite *NFSControllerSuite) Test_NfsControllerExpandVolume_VolumeID_empty() {
	service := nfsstorage{cs: *suite.cs}
	_, err := service.ControllerExpandVolume(context.Background(), getNfsExpandVolumeRequest(""))
	assert.NotNil(suite.T(), err, "expected to fail: NfsControllerExpandVolume Volume ID missing in request")
}

func (suite *NFSControllerSuite) Test_NfsControllerExpandVolume_InvalidVolumeID() {
	volumeID := "100"
	service := nfsstorage{cs: *suite.cs}
	_, err := service.ControllerExpandVolume(context.Background(), getNfsExpandVolumeRequest(volumeID))
	assert.NotNil(suite.T(), err, "expected to fail: NfsControllerExpandVolume invalid Volume ID in request")
}

func (suite *NFSControllerSuite) Test_NfsControllerExpandVolume_Error() {
	fileSystemID := "100#"
	fileSystem := api.FileSystem{}
	expectedErr := errors.New("Some error")
	suite.api.On("UpdateFilesystem", fileSystemID, fileSystem).Return(expectedErr)
	service := nfsstorage{cs: *suite.cs}
	_, err := service.ControllerExpandVolume(context.Background(), getNfsExpandVolumeRequest(fileSystemID))
	assert.NotNil(suite.T(), err, "expected to fail: NfsControllerExpandVolume update file system")
}

func (suite *NFSControllerSuite) Test_NfsControllerExpandVolume_Error_filenotfound() {
	service := nfsstorage{cs: *suite.cs}
	fileSystemID := "100#"
	fileSystem := api.FileSystem{}
	expectedErr := errors.New("Some error")
	suite.api.On("UpdateVolume", fileSystemID, fileSystem).Return(expectedErr)
	_, err := service.ControllerExpandVolume(context.Background(), getNfsExpandVolumeRequest(fileSystemID))
	assert.NotNil(suite.T(), err, "expected to fail: NfsControllerExpandVolume update volume")
}

func (suite *NFSControllerSuite) Test_NfsControllerExpandVolume_success() {
	service := nfsstorage{cs: *suite.cs}
	fileSystemID := "100#"
	fileSystem := api.FileSystem{}
	suite.api.On("UpdateVolume", fileSystemID, fileSystem).Return(nil)
	_, err := service.ControllerExpandVolume(context.Background(), getNfsExpandVolumeRequest(fileSystemID))
	assert.NotNil(suite.T(), err, "expected to succeed: NfsControllerExpandVolume UpdateVolume")
}

func (suite *NFSControllerSuite) Test_NfsControllerExpandVolume_UpdateVolume_Error() {
	service := nfsstorage{cs: *suite.cs}
	expectedErr := errors.New("some error")
	fileSystemID := "100"
	suite.api.On("UpdateFilesystem", mock.Anything, mock.Anything).Return(nil, expectedErr)
	_, err := service.ControllerExpandVolume(context.Background(), getNfsExpandVolumeRequest(fileSystemID))
	assert.NotNil(suite.T(), err, "expected to fail: NfsControllerExpandVolume UpdateFilesystem")
}

func (suite *NFSControllerSuite) Test_NfsControllerExpandVolume_success_expand() {
	service := nfsstorage{cs: *suite.cs}
	fileSystemID := "100"
	suite.api.On("UpdateFilesystem", mock.Anything, mock.Anything).Return(nil, nil)
	_, err := service.ControllerExpandVolume(context.Background(), getNfsExpandVolumeRequest(fileSystemID))
	assert.Nil(suite.T(), err, "expected to succeed: NfsControllerExpandVolume UpdateFilesystem")
}

func (suite *NFSControllerSuite) Test_NfsCreateSnapshot_GetSnapshot_Error() {
	expectedErr := errors.New("Snapshot Name is must")
	suite.api.On("GetSnapshotByName", mock.Anything).Return(nil, expectedErr)
	service := nfsstorage{cs: *suite.cs}
	_, err := service.CreateSnapshot(context.Background(), getNfsCreateSnapshotRequest("100"))
	assert.NotNil(suite.T(), err, "expected to fail: CreateSnapshot get snapshot by name")
}

func (suite *NFSControllerSuite) Test_NfsCreateSnapshot_SourceVolumeID_Error() {
	fileSystem := api.FileSystem{}
	expectedErr := errors.New("SourceVolumeID is must")
	suite.api.On("GetSnapshotByName", mock.Anything).Return(fileSystem, nil)
	suite.api.On("CreateFileSystemSnapshot", mock.Anything).Return(nil, expectedErr)
	service := nfsstorage{cs: *suite.cs}
	_, err := service.CreateSnapshot(context.Background(), getNfsCreateSnapshotRequest("100"))
	assert.NotNil(suite.T(), err, "expected to fail: CreateSnapshot create snapshot by name")
}

func (suite *NFSControllerSuite) Test_NfsCreateSnapshot_Success() {
	var fileSysSnapshotResp api.FileSystemSnapshotResponce
	fileSysSnapshotResp.SnapshotID = 1
	fileSysSnapshotResp.Name = "snapshot"
	fileSysSnapshotResp.ParentId = 1

	var fileSysSnapshotRespArry []api.FileSystemSnapshotResponce
	fileSysSnapshotRespArry = append(fileSysSnapshotRespArry, fileSysSnapshotResp)

	suite.api.On("GetSnapshotByName", mock.Anything).Return(fileSysSnapshotRespArry, nil)

	service := nfsstorage{cs: *suite.cs}
	_, err := service.CreateSnapshot(context.Background(), getNfsCreateSnapshotRequest("1$$nfs"))
	assert.Nil(suite.T(), err, "expected to succeed: CreateSnapshot")
}

func (suite *NFSControllerSuite) Test_NfsCreateSnapshot_CreateFileSystemS_Error() {
	var fileSysSnapshotRespArry []api.FileSystemSnapshotResponce
	expectedErr := errors.New("some error")
	suite.api.On("GetSnapshotByName", mock.Anything).Return(fileSysSnapshotRespArry, nil)
	suite.api.On("CreateFileSystemSnapshot", mock.Anything).Return(nil, expectedErr)
	service := nfsstorage{cs: *suite.cs}
	_, err := service.CreateSnapshot(context.Background(), getNfsCreateSnapshotRequest("1$$nfs"))
	assert.NotNil(suite.T(), err, "expected to fail: CreateSnapshot create file system snapshot")
}

func (suite *NFSControllerSuite) Test_NfsCreateSnapshot_CreateFileSystemS_success() {
	var fileSysSnapshotRespArry []api.FileSystemSnapshotResponce
	var filesystem api.FileSystemSnapshot
	filesystem.ParentID = 10
	filesystem.SnapshotName = "snapshot"
	filesystem.WriteProtected = false

	suite.api.On("GetSnapshotByName", mock.Anything).Return(fileSysSnapshotRespArry, nil)
	suite.api.On("CreateFileSystemSnapshot", mock.Anything).Return(filesystem, nil)
	service := nfsstorage{cs: *suite.cs}
	_, err := service.CreateSnapshot(context.Background(), getNfsCreateSnapshotRequest("1$$nfs"))
	assert.Nil(suite.T(), err, "expected to succeed: CreateSnapshot CreateFileSystemSnapshot")
}

func (suite *NFSControllerSuite) Test_NfsDeleteSnapshot_SourceVolumeID_empty() {
	service := nfsstorage{cs: *suite.cs}
	var snapshotID int64
	expectedErr := errors.New("Invalid Source ID")
	suite.api.On("GetFileSystemByID", snapshotID).Return(nil, expectedErr)
	_, err := service.DeleteSnapshot(context.Background(), getNfsDeleteSnapshotRequest(""))
	assert.NotNil(suite.T(), err, "expected to fail: NfsDeleteSnapshot GetFileSystemByID Source Volume ID missing in request")
}

func (suite *NFSControllerSuite) Test_NfsDeleteSnapshot_InvalidSourceVolumeID() {
	service := nfsstorage{cs: *suite.cs}
	var snapshotID int64 = 1000000000000000000
	expectedErr := errors.New("Invalid Source ID")
	suite.api.On("GetFileSystemByID", snapshotID).Return(nil, expectedErr)
	_, err := service.DeleteSnapshot(context.Background(), getNfsDeleteSnapshotRequest("1000000000000000000"))
	assert.NotNil(suite.T(), err, "expected to fail: NfsDeleteSnapshot GetFileSystemByID Invalid Snapshot ID in request")
}

func (suite *NFSControllerSuite) Test_NfsDeleteSnapshot_Error() {
	service := nfsstorage{cs: *suite.cs, uniqueID: 100}
	var snapshotID int64 = 100
	expectedErr := errors.New("some error")
	suite.api.On("GetFileSystemByID", snapshotID).Return(nil, expectedErr)
	_, err := service.DeleteSnapshot(context.Background(), getNfsDeleteSnapshotRequest("100"))
	assert.NotNil(suite.T(), err, "expected to fail: NfsDeleteSnapshot GetFileSystemByID")
}

func (suite *NFSControllerSuite) Test_NfsDeleteSnapshot_file_not_found() {
	service := nfsstorage{cs: *suite.cs, uniqueID: 100}
	var snapshotID int64 = 100
	expectedErr := errors.New("FILESYSTEM_NOT_FOUND")
	suite.api.On("GetFileSystemByID", snapshotID).Return(nil, expectedErr)
	_, err := service.DeleteSnapshot(context.Background(), getNfsDeleteSnapshotRequest("100"))
	assert.Nil(suite.T(), err, "expected to fail: NfsDeleteSnapshot GetFileSystemByID fs not found")
}

func (suite *NFSControllerSuite) Test_NfsDeleteNFSVolume_GetFileSystemByID_error() {
	service := nfsstorage{cs: *suite.cs, uniqueID: 100}
	var snapshotID int64 = 100
	expectedErr := errors.New("FILESYSTEM_NOT_FOUND")
	suite.api.On("GetFileSystemByID", snapshotID).Return(nil, expectedErr)
	err := service.DeleteNFSVolume()
	assert.NotNil(suite.T(), err, "expected to fail: DeleteNFSVolume GetFileSystemByID fs not found")
	assert.Equal(suite.T(), expectedErr, err, "Error not returned as expected")
}

func (suite *NFSControllerSuite) Test_NfsDeleteNFSVolume_GetFileSystemByID_InvalidID() {
	service := nfsstorage{cs: *suite.cs, uniqueID: 100}
	snapshotID := 100999999999999
	expectedErr := errors.New("Invalid_ID")
	suite.api.On("GetFileSystemByID", snapshotID).Return(nil, expectedErr)
	err := service.DeleteNFSVolume()
	assert.NotNil(suite.T(), err, "expected to fail: DeleteNFSVolume GetFileSystemByID invalid ID")
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
	assert.Nil(suite.T(), err, "expected to succeed: DeleteNFSVolume")
}

func (suite *NFSControllerSuite) Test_DeleteVolume_InvalidaID() {
	service := nfsstorage{cs: *suite.cs}
	delValReq := getNFSDeletRequest()
	delValReq.VolumeId = ""
	_, err := service.DeleteVolume(context.Background(), delValReq)
	assert.NotNil(suite.T(), err, "expected to fail: DeleteVolume empty volume ID")

	delValReq.VolumeId = "a$$1234"
	_, err = service.DeleteVolume(context.Background(), delValReq)
	assert.NotNil(suite.T(), err, "expected to fail: DeleteVolume invalid volume ID")
}

func (suite *NFSControllerSuite) Test_DeleteVolume_fileNotFound_success() {
	service := nfsstorage{cs: *suite.cs}
	delValReq := getNFSDeletRequest()
	expectedErr := errors.New("FILESYSTEM_NOT_FOUND")
	suite.api.On("GetFileSystemByID", mock.Anything).Return(nil, expectedErr)
	_, err := service.DeleteVolume(context.Background(), delValReq)
	assert.Nil(suite.T(), err, "expected to succeed: DeleteVolume when fs not found")
}

func (suite *NFSControllerSuite) Test_DeleteVolume_Error() {
	service := nfsstorage{cs: *suite.cs}
	delValReq := getNFSDeletRequest()
	expectedErr := errors.New("some Error")
	suite.api.On("GetFileSystemByID", mock.Anything).Return(nil, expectedErr)
	_, err := service.DeleteVolume(context.Background(), delValReq)
	assert.NotNil(suite.T(), err, "expected to fail: DeleteVolume")
}

func (suite *NFSControllerSuite) Test_DeleteVolume_Metadata_failed() {
	service := nfsstorage{cs: *suite.cs}
	delValReq := getNFSDeletRequest()
	expectedErr := errors.New("some Error")

	// var filsystemID int64 = 1
	suite.api.On("GetFileSystemByID", mock.Anything).Return(nil, nil)
	suite.api.On("FileSystemHasChild", mock.Anything).Return(true)
	suite.api.On("AttachMetadataToObject", mock.Anything, mock.Anything).Return(nil, expectedErr)

	_, err := service.DeleteVolume(context.Background(), delValReq)
	assert.NotNil(suite.T(), err, "expected to fail: DeleteVolume delete metadata")
}

func (suite *NFSControllerSuite) Test_DeleteVolume_delete_Error() {
	service := nfsstorage{cs: *suite.cs}
	delValReq := getNFSDeletRequest()
	expectedErr := errors.New("some Error")

	var parentID int64 = 0
	suite.api.On("GetFileSystemByID", mock.Anything).Return(nil, nil)
	suite.api.On("FileSystemHasChild", mock.Anything).Return(false)
	suite.api.On("AttachMetadataToObject", mock.Anything, mock.Anything).Return(nil, nil)
	suite.api.On("GetParentID", mock.Anything).Return(parentID)
	suite.api.On("DeleteFileSystemComplete", mock.Anything).Return(expectedErr)

	_, err := service.DeleteVolume(context.Background(), delValReq)
	assert.NotNil(suite.T(), err, "expected to fail: DeleteVolume delete fs complete")
}

func (suite *NFSControllerSuite) Test_DeleteVolume_Err2() {
	service := nfsstorage{cs: *suite.cs}
	delValReq := getNFSDeletRequest()
	expectedErr := errors.New("some Error")

	var parentID int64 = 11
	suite.api.On("GetFileSystemByID", mock.Anything).Return(nil, nil)
	suite.api.On("FileSystemHasChild", mock.Anything).Return(false)
	suite.api.On("AttachMetadataToObject", mock.Anything, mock.Anything).Return(nil, nil)
	suite.api.On("GetParentID", mock.Anything).Return(parentID)
	suite.api.On("DeleteFileSystemComplete", mock.Anything).Return(nil)
	suite.api.On("DeleteParentFileSystem", mock.Anything).Return(expectedErr)

	_, err := service.DeleteVolume(context.Background(), delValReq)
	assert.NotNil(suite.T(), err, "expected to fail: DeleteVolume delete parent fs")
}

func (suite *NFSControllerSuite) Test_DeleteVolume_success() {
	service := nfsstorage{cs: *suite.cs}
	delValReq := getNFSDeletRequest()

	var parentID int64 = 11
	suite.api.On("GetFileSystemByID", mock.Anything).Return(nil, nil)
	suite.api.On("FileSystemHasChild", mock.Anything).Return(false)
	suite.api.On("AttachMetadataToObject", mock.Anything, mock.Anything).Return(nil, nil)
	suite.api.On("GetParentID", mock.Anything).Return(parentID)
	suite.api.On("DeleteFileSystemComplete", mock.Anything).Return(nil)
	suite.api.On("DeleteParentFileSystem", mock.Anything).Return(nil)

	_, err := service.DeleteVolume(context.Background(), delValReq)
	assert.Nil(suite.T(), err, "expected to succeed: DeleteVolume")
}

// ControllerPublishVolume=============
func (suite *NFSControllerSuite) Test_ControllerPublishVolume_InvalidaNodeID() {
	service := nfsstorage{cs: *suite.cs}
	publishParameter := getPublishVolumeParameter()
	publishVolReq := getNFSControllerPublishVolume(publishParameter)
	publishVolReq.NodeId = "1$12$13"
	suite.accessMock.On("IsValidAccessModeNfs", mock.Anything).Return(true, nil)

	_, err := service.ControllerPublishVolume(context.Background(), publishVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: ControllerPublishVolume invalid node ID")
}

func (suite *NFSControllerSuite) Test_ControllerPublishVolume_AddNodeInExport_Error() {
	service := nfsstorage{cs: *suite.cs}
	publishParameter := getPublishVolumeParameter()
	publishVolReq := getNFSControllerPublishVolume(publishParameter)
	expectedErr := errors.New("some Error")
	suite.accessMock.On("IsValidAccessModeNfs", mock.Anything).Return(true, nil)
	suite.api.On("AddNodeInExport", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, expectedErr)

	_, err := service.ControllerPublishVolume(context.Background(), publishVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: ControllerPublishVolume add node in export")
}

func (suite *NFSControllerSuite) Test_ControllerPublishVolume_success() {
	service := nfsstorage{cs: *suite.cs}
	publishParameter := getPublishVolumeParameter()
	publishVolReq := getNFSControllerPublishVolume(publishParameter)
	suite.accessMock.On("IsValidAccessModeNfs", mock.Anything).Return(true, nil)
	suite.api.On("AddNodeInExport", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, nil)

	_, err := service.ControllerPublishVolume(context.Background(), publishVolReq)
	assert.Nil(suite.T(), err, "expected to succeed: ControllerPublishVolume")
}

func (suite *NFSControllerSuite) Test_ControllerUnpublishVolume_DeleteExportRule_error() {
	service := nfsstorage{cs: *suite.cs}
	unpublishVolReq := getNFSControllerUnpublishVolume()
	expectedErr := errors.New("some Error")
	suite.api.On("DeleteExportRule", mock.Anything, mock.Anything).Return(expectedErr)
	_, err := service.ControllerUnpublishVolume(context.Background(), unpublishVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: ControllerUnpublishVolume when DeleteExportRule fails")
}

func (suite *NFSControllerSuite) Test_ControllerUnpublishVolume_DeleteExportRule_success() {
	service := nfsstorage{cs: *suite.cs}
	unpublishVolReq := getNFSControllerUnpublishVolume()
	suite.api.On("DeleteExportRule", mock.Anything, mock.Anything).Return(nil)
	_, err := service.ControllerUnpublishVolume(context.Background(), unpublishVolReq)
	assert.Nil(suite.T(), err, "expected to succeed: ControllerUnpublishVolume when DeleteExportRule succeeds")
}

//============================================================

func getNFSControllerUnpublishVolume() *csi.ControllerUnpublishVolumeRequest {
	return &csi.ControllerUnpublishVolumeRequest{
		VolumeId: "1$$nfs",
	}
}

func getNFSControllerPublishVolume(parameterMap map[string]string) *csi.ControllerPublishVolumeRequest {
	return &csi.ControllerPublishVolumeRequest{
		VolumeId:      "1",
		VolumeContext: parameterMap,
		NodeId:        "10.20.20.50$$nfs",
	}
}

func getNFSDeletRequest() *csi.DeleteVolumeRequest {
	return &csi.DeleteVolumeRequest{
		VolumeId: "1",
	}
}

// Test case Data Generation
func getExportResponseValue() api.ExportResponse {
	response := api.ExportResponse{ID: 1, ExportPath: "/exportPath/"}
	return response
}

func getExportPath() *[]api.ExportResponse {
	exportRepo := []api.ExportResponse{}
	responce := api.ExportResponse{ID: 1, ExportPath: "/exportPath/"}

	exportRepo = append(exportRepo, responce)
	return &exportRepo
}

func GetFileSystemSnapshotResponce(snapshotID int64) api.FileSystemSnapshotResponce {
	return api.FileSystemSnapshotResponce{SnapshotID: snapshotID, Name: "snapshotName"}
}

func getFileSystem() api.FileSystem {
	return api.FileSystem{ID: 1, PoolID: 100, Name: "PVName", SsdEnabled: true, Provtype: "thin", Size: 100 * gib, PoolName: "pool_name1"}
}

func getNetworkSpace() api.NetworkSpace {
	portalArry := []api.Portal{{IpAdress: "10.20.20.50"}}
	return api.NetworkSpace{Portals: portalArry}
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

func getNFSCreateVolumeRequest(name string, parameterMap map[string]string) *csi.CreateVolumeRequest {
	return &csi.CreateVolumeRequest{
		Name:          name,
		CapacityRange: &csi.CapacityRange{RequiredBytes: 100 * gib},
		// VolumeCapabilities []*VolumeCapability
		Parameters: parameterMap,
		// Secrets map[string]string
		VolumeContentSource: nil,
	}
}

// getCreateVolumeRequestByType - method return the snapshot or clone createVallume request
func getCreateVolumeSnapshotRequest(name string, parameterMap map[string]string) *csi.CreateVolumeRequest {
	createVolume := &csi.CreateVolumeRequest{
		Name:          name,
		CapacityRange: &csi.CapacityRange{RequiredBytes: 100 * gib},
		Parameters:    parameterMap,
		VolumeContentSource: &csi.VolumeContentSource{
			Type: &csi.VolumeContentSource_Snapshot{
				Snapshot: &csi.VolumeContentSource_SnapshotSource{
					SnapshotId: "1$$nfs",
				},
			},
		},
	}
	return createVolume
}

// getCreateVolumeRequestByType - method return the snapshot or clone createVallume request
func getCreateVolumeCloneRequest(name string, parameterMap map[string]string) *csi.CreateVolumeRequest {
	createVolume := &csi.CreateVolumeRequest{
		Name:          name,
		CapacityRange: &csi.CapacityRange{RequiredBytes: 100 * gib},
		Parameters:    parameterMap,
		VolumeContentSource: &csi.VolumeContentSource{
			Type: &csi.VolumeContentSource_Volume{
				Volume: &csi.VolumeContentSource_VolumeSource{
					VolumeId: "1$$nfs",
				},
			},
		},
	}
	return createVolume
}

func getCreateVolumeParameter() map[string]string {
	return map[string]string{
		"pool_name":                   "pool_name1",
		"network_space":               "network_space1",
		api.SC_NFS_EXPORT_PERMISSIONS: "[{'access':'RW','client':'192.168.147.190-192.168.147.199','no_root_squash':false},{'access':'RW','client':'192.168.147.10-192.168.147.20','no_root_squash':'false'}]",
	}
}

func getPublishVolumeParameter() map[string]string {
	return map[string]string{
		"exportID":                    "1",
		api.SC_NFS_EXPORT_PERMISSIONS: "[{'access':'RW','client':'192.168.147.190-192.168.147.199','no_root_squash':false},{'access':'RW','client':'192.168.147.10-192.168.147.20','no_root_squash':'false'}]",
	}
}
