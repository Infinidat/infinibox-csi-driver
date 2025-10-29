//go:build unit

package nfs

import (
	"errors"
	"fmt"
	"testing"

	"github.com/infinidat/infinibox-csi-driver/api"
	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/infinidat/infinibox-csi-driver/helper"
	"github.com/infinidat/infinibox-csi-driver/iboxapi"
	storagecommon "github.com/infinidat/infinibox-csi-driver/storage/common"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

func (suite *NFSControllerSuite) SetupTest() {
	suite.api = new(api.MockAPIService)
	suite.iboxapi = new(iboxapi.MockAPIService)
	suite.accessMock = new(helper.MockAccessModesHelper)
	volproto := &api.VolumeProtocolConfig{
		VolumeID:    1,
		StorageType: "",
	}
	cs := storagecommon.Commonservice{API: suite.api, AccessModesHelper: suite.accessMock, IboxAPI: suite.iboxapi, VolProto: volproto}

	suite.service = NFSstorage{CS: cs, Capacity: 100 * storagecommon.GIB}
	suite.service.StorageClassParameters = map[string]string{
		common.StorageClassPoolName: "somepoolname",
	}
	suite.someError = errors.New("Some error")
}

type NFSControllerSuite struct {
	suite.Suite
	api        *api.MockAPIService
	iboxapi    *iboxapi.MockAPIService
	accessMock *helper.MockAccessModesHelper
	service    NFSstorage
	someError  error
}

func TestNfsControllerSuite(t *testing.T) {
	suite.Run(t, new(NFSControllerSuite))
}

func (suite *NFSControllerSuite) Test_CreateVolume_parameterValidation_Fail() {
	parameterMap := getCreateVolumeParameter()
	delete(parameterMap, common.StorageClassPoolName)

	suite.iboxapi.On("GetNetworkSpaceByName", suite.Suite.T().Context(), mock.Anything).Return(nil, suite.someError)
	suite.iboxapi.On("GetSystem", suite.Suite.T().Context()).Return(storagecommon.GetSystem(), nil)
	createVolReq := getNFSCreateVolumeRequest("PVName", parameterMap)
	_, err := suite.service.CreateVolume(suite.Suite.T().Context(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: parameter validation ")
}

func (suite *NFSControllerSuite) Test_CreateVolume_NetworkSpaceIP_Error() {
	parameterMap := getCreateVolumeParameter()
	createVolReq := getNFSCreateVolumeRequest("PVName", parameterMap)

	suite.iboxapi.On("GetNetworkSpaceByName", suite.Suite.T().Context(), mock.Anything).Return(nil, suite.someError)
	suite.iboxapi.On("GetSystem", suite.Suite.T().Context()).Return(storagecommon.GetSystem(), nil)
	_, err := suite.service.CreateVolume(suite.Suite.T().Context(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: get IP address from networkspace")
}

func (suite *NFSControllerSuite) Test_CreateVolume_GetFileSystemByName_Error() {
	parameterMap := getCreateVolumeParameter()
	createVolReq := getNFSCreateVolumeRequest("PVName", parameterMap)

	poolResult := &iboxapi.PoolResult{ID: 100}
	suite.iboxapi.On("GetPoolByName", suite.Suite.T().Context(), parameterMap[common.StorageClassPoolName]).Return(poolResult, nil)
	suite.iboxapi.On("GetSystem", suite.Suite.T().Context()).Return(storagecommon.GetSystem(), nil)

	suite.iboxapi.On("GetNetworkSpaceByName", suite.Suite.T().Context(), mock.Anything).Return(storagecommon.GetNetworkSpace(), nil)
	suite.iboxapi.On("GetFileSystemByName", suite.Suite.T().Context(), mock.Anything).Return(nil, suite.someError)

	suite.iboxapi.On("CreateFileSystem", suite.Suite.T().Context(), mock.Anything).Return(storagecommon.GetFileSystem(), nil)
	suite.iboxapi.On("GetExportsByFileSystemID", suite.Suite.T().Context(), mock.Anything).Return(storagecommon.GetExportPath(), nil)
	suite.iboxapi.On("CreateExport", suite.Suite.T().Context(), mock.Anything).Return(&iboxapi.Export{}, nil)
	suite.iboxapi.On("DeleteFileSystem", suite.Suite.T().Context(), mock.Anything).Return(nil)
	suite.iboxapi.On("DeleteExport", suite.Suite.T().Context(), mock.Anything).Return(&iboxapi.Export{}, nil)
	suite.iboxapi.On("PutMetadata", suite.Suite.T().Context(), mock.Anything, mock.Anything).Return(nil, nil)

	_, err := suite.service.CreateVolume(suite.Suite.T().Context(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: get filesystem by name")
}

func (suite *NFSControllerSuite) Test_CreateVolume_FileNameExist_exportError() {
	parameterMap := getCreateVolumeParameter()
	createVolReq := getNFSCreateVolumeRequest("PVName", parameterMap)

	poolResult := &iboxapi.PoolResult{ID: 1}
	suite.iboxapi.On("GetPoolByName", suite.Suite.T().Context(), mock.Anything).Return(poolResult, nil)
	suite.iboxapi.On("GetSystem", suite.Suite.T().Context()).Return(storagecommon.GetSystem(), nil)
	suite.iboxapi.On("GetNetworkSpaceByName", suite.Suite.T().Context(), mock.Anything).Return(storagecommon.GetNetworkSpace(), nil)
	suite.iboxapi.On("GetFileSystemByName", suite.Suite.T().Context(), mock.Anything).Return(storagecommon.GetFileSystem(), nil)
	suite.iboxapi.On("GetExportsByFileSystemID", suite.Suite.T().Context(), mock.Anything).Return(storagecommon.GetExportPath(), suite.someError)

	_, err := suite.service.CreateVolume(suite.Suite.T().Context(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: get export by filesystem")
}

func (suite *NFSControllerSuite) Test_CreateVolume_FileNameExist_sucess() {
	suite.service.Capacity = 100 * storagecommon.GIB
	parameterMap := getCreateVolumeParameter()
	createVolReq := getNFSCreateVolumeRequest("PVName", parameterMap)

	suite.iboxapi.On("GetSystem", suite.Suite.T().Context()).Return(storagecommon.GetSystem(), nil)
	suite.iboxapi.On("GetNetworkSpaceByName", suite.Suite.T().Context(), mock.Anything).Return(storagecommon.GetNetworkSpace(), nil)
	suite.iboxapi.On("GetFileSystemByName", suite.Suite.T().Context(), mock.Anything).Return(storagecommon.GetFileSystemPrior(), nil)
	suite.iboxapi.On("GetExportsByFileSystemID", suite.Suite.T().Context(), mock.Anything).Return(storagecommon.GetExportPath(), nil)

	resp, err := suite.service.CreateVolume(suite.Suite.T().Context(), createVolReq)
	assert.Nil(suite.T(), err, "expected to succeed: CreateVolume when file system exists")
	assert.NotNil(suite.T(), resp, "CreateVolume ok response should be non-empty")
}

func (suite *NFSControllerSuite) Test_CreateVolume_StoragePoolIDByName_Error() {
	suite.service.Capacity = 100 * storagecommon.GIB
	parameterMap := getCreateVolumeParameter()
	suite.iboxapi.On("GetSystem", suite.Suite.T().Context()).Return(storagecommon.GetSystem(), nil)
	suite.iboxapi.On("GetPoolByName", suite.Suite.T().Context(), mock.Anything).Return(nil, suite.someError)
	createVolReq := getNFSCreateVolumeRequest("PVName", parameterMap)
	suite.iboxapi.On("GetExportsByFileSystemID", suite.Suite.T().Context(), mock.Anything).Return(storagecommon.GetExportResponse(), nil)

	suite.iboxapi.On("GetNetworkSpaceByName", suite.Suite.T().Context(), mock.Anything).Return(storagecommon.GetNetworkSpace(), nil)
	notFoundError := &iboxapi.APIError{Code: iboxapi.RESOURCE_NOT_FOUND, Err: fmt.Errorf("not found")}
	suite.iboxapi.On("GetFileSystemByName", suite.Suite.T().Context(), mock.Anything).Return(nil, notFoundError)

	_, err := suite.service.CreateVolume(suite.Suite.T().Context(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: CreateVolume get poolID by poolName")
}

func (suite *NFSControllerSuite) Test_CreateVolume_CreateFileSystem_Error() {
	suite.service.Capacity = 100 * storagecommon.GIB
	parameterMap := getCreateVolumeParameter()
	createVolReq := getNFSCreateVolumeRequest("PVName", parameterMap)

	poolResult := &iboxapi.PoolResult{ID: 100, Name: "pool_name1"}
	suite.iboxapi.On("GetPoolByName", suite.Suite.T().Context(), mock.Anything).Return(poolResult, nil)
	suite.iboxapi.On("GetSystem", suite.Suite.T().Context()).Return(storagecommon.GetSystem(), nil)
	suite.iboxapi.On("GetNetworkSpaceByName", suite.Suite.T().Context(), mock.Anything).Return(storagecommon.GetNetworkSpace(), nil)
	suite.iboxapi.On("GetExportsByFileSystemID", suite.Suite.T().Context(), mock.Anything).Return(storagecommon.GetExportPath(), nil)
	notFoundError := &iboxapi.APIError{Code: iboxapi.RESOURCE_NOT_FOUND, Err: fmt.Errorf("not found")}
	suite.iboxapi.On("GetFileSystemByName", suite.Suite.T().Context(), mock.Anything).Return(nil, notFoundError)
	suite.iboxapi.On("CreateFileSystem", suite.Suite.T().Context(), mock.Anything).Return(nil, suite.someError)

	_, err := suite.service.CreateVolume(suite.Suite.T().Context(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: CreateVolume create the file system")
}

func (suite *NFSControllerSuite) Test_CreateVolume_createExportPath_Error() {
	suite.service.Capacity = 100 * storagecommon.GIB
	parameterMap := getCreateVolumeParameter()
	createVolReq := getNFSCreateVolumeRequest("PVName", parameterMap)

	poolResult := &iboxapi.PoolResult{ID: 1000}
	suite.iboxapi.On("GetPoolByName", suite.Suite.T().Context(), parameterMap[common.StorageClassPoolName]).Return(poolResult, nil)
	suite.iboxapi.On("GetSystem", suite.Suite.T().Context()).Return(storagecommon.GetSystem(), nil)
	pool := iboxapi.PoolResult{
		Name: "pool_name1",
	}

	suite.iboxapi.On("GetExportsByFileSystemID", suite.Suite.T().Context(), mock.Anything).Return(storagecommon.GetExportPath(), nil)
	suite.iboxapi.On("GetPoolByName", suite.Suite.T().Context(), mock.Anything).Return(&pool, nil)
	suite.iboxapi.On("DeleteFileSystem", suite.Suite.T().Context(), mock.Anything).Return(nil)
	suite.iboxapi.On("GetNetworkSpaceByName", suite.Suite.T().Context(), mock.Anything).Return(storagecommon.GetNetworkSpace(), nil)
	notFoundError := &iboxapi.APIError{Code: iboxapi.RESOURCE_NOT_FOUND, Err: fmt.Errorf("not found")}
	suite.iboxapi.On("GetFileSystemByName", suite.Suite.T().Context(), mock.Anything).Return(nil, notFoundError)
	suite.iboxapi.On("CreateFileSystem", suite.Suite.T().Context(), mock.Anything).Return(storagecommon.GetFileSystem(), nil)
	suite.iboxapi.On("CreateExport", suite.Suite.T().Context(), mock.Anything).Return(nil, suite.someError)

	_, err := suite.service.CreateVolume(suite.Suite.T().Context(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: create export path")
	// assert.Equal(suite.T(), err.Error(), suite.someError.Error(), "expected to get the mocked err")
}

func (suite *NFSControllerSuite) Test_CreateVolume_success() {
	parameterMap := getCreateVolumeParameter()
	createVolReq := getNFSCreateVolumeRequest("PVName", parameterMap)

	poolResult := &iboxapi.PoolResult{ID: 100}
	suite.iboxapi.On("GetPoolByName", suite.Suite.T().Context(), parameterMap[common.StorageClassPoolName]).Return(poolResult, nil)
	pool := iboxapi.PoolResult{
		Name: "pool_name1",
	}
	suite.iboxapi.On("GetPoolByName", suite.Suite.T().Context(), mock.Anything).Return(&pool, nil)
	suite.iboxapi.On("GetSystem", suite.Suite.T().Context()).Return(storagecommon.GetSystem(), nil)
	suite.iboxapi.On("GetNetworkSpaceByName", suite.Suite.T().Context(), mock.Anything).Return(storagecommon.GetNetworkSpace(), nil)
	suite.iboxapi.On("GetFileSystemByName", suite.Suite.T().Context(), mock.Anything).Return(storagecommon.GetFileSystem(), nil)
	suite.iboxapi.On("GetFileSystemByID", suite.Suite.T().Context(), mock.Anything).Return(storagecommon.GetFileSystem(), nil)
	suite.iboxapi.On("CreateFileSystem", suite.Suite.T().Context(), mock.Anything).Return(storagecommon.GetFileSystemPrior(), nil)
	suite.iboxapi.On("GetExportsByFileSystemID", suite.Suite.T().Context(), mock.Anything).Return(storagecommon.GetExportPath(), nil)

	suite.iboxapi.On("CreateExport", suite.Suite.T().Context(), mock.Anything).Return(storagecommon.GetExportResponseValue(), nil)
	suite.iboxapi.On("PutMetadata", suite.Suite.T().Context(), mock.Anything, mock.Anything).Return(nil, nil)

	resp, err := suite.service.CreateVolume(suite.Suite.T().Context(), createVolReq)
	assert.Nil(suite.T(), err, "expected succeed: CreateVolume create the file system")
	assert.Equal(suite.T(), resp.GetVolume().GetVolumeId(), "1", "expected to get volume ID")
}

func (suite *NFSControllerSuite) Test_CreateVolume_Snapshot_invalidSize() {
	parameterMap := getCreateVolumeParameter()
	createVolReq := getCreateVolumeSnapshotRequest("PVName", parameterMap)
	createVolReq.GetVolumeContentSource().GetSnapshot().SnapshotId = "1$$nfs"
	suite.service.Capacity = 123

	poolResult := &iboxapi.PoolResult{ID: 1}
	suite.iboxapi.On("GetPoolByName", suite.Suite.T().Context(), mock.Anything).Return(poolResult, nil)
	suite.iboxapi.On("GetSystem", suite.Suite.T().Context()).Return(storagecommon.GetSystem(), nil)
	suite.iboxapi.On("GetNetworkSpaceByName", suite.Suite.T().Context(), mock.Anything).Return(storagecommon.GetNetworkSpace(), nil)
	suite.iboxapi.On("GetExportsByFileSystemID", suite.Suite.T().Context(), mock.Anything).Return(storagecommon.GetExportPath(), nil)
	suite.iboxapi.On("GetFileSystemByName", suite.Suite.T().Context(), mock.Anything).Return(storagecommon.GetFileSystem(), nil)
	suite.iboxapi.On("GetFileSystemByID", suite.Suite.T().Context(), mock.Anything).Return(storagecommon.GetFileSystem(), nil)

	_, err := suite.service.CreateVolume(suite.Suite.T().Context(), createVolReq)
	assert.NotNil(suite.T(), err.Error(), "invalid snapshot size")
}

func (suite *NFSControllerSuite) Test_CreateVolume_Snapshot_poolID_name_invalid() {
	parameterMap := getCreateVolumeParameter()
	createVolReq := getCreateVolumeSnapshotRequest("PVName", parameterMap)
	createVolReq.GetVolumeContentSource().GetSnapshot().SnapshotId = "1$$nfs"

	fs := storagecommon.GetFileSystem()
	suite.service.Capacity = fs.Size
	suite.iboxapi.On("GetNetworkSpaceByName", suite.Suite.T().Context(), mock.Anything).Return(storagecommon.GetNetworkSpace(), nil)
	notFoundError := &iboxapi.APIError{Code: iboxapi.RESOURCE_NOT_FOUND, Err: fmt.Errorf("not found")}
	suite.iboxapi.On("GetFileSystemByName", suite.Suite.T().Context(), mock.Anything).Return(nil, notFoundError)
	suite.iboxapi.On("GetSystem", suite.Suite.T().Context()).Return(storagecommon.GetSystem(), nil)
	suite.iboxapi.On("GetExportsByFileSystemID", suite.Suite.T().Context(), mock.Anything).Return(storagecommon.GetExportPath(), nil)
	suite.iboxapi.On("GetFileSystemByID", suite.Suite.T().Context(), mock.Anything).Return(storagecommon.GetFileSystem(), nil)
	poolResult := &iboxapi.PoolResult{ID: 101}
	suite.iboxapi.On("GetPoolByName", suite.Suite.T().Context(), mock.Anything).Return(poolResult, suite.someError)

	_, err := suite.service.CreateVolume(suite.Suite.T().Context(), createVolReq)
	assert.NotNil(suite.T(), err.Error(), "failed to get PooldID by  storage pool name")
}

func (suite *NFSControllerSuite) Test_CreateVolume_Snapshot_createSnapshot_failed() {
	parameterMap := getCreateVolumeParameter()
	createVolReq := getCreateVolumeSnapshotRequest("PVName", parameterMap)
	createVolReq.GetVolumeContentSource().GetSnapshot().SnapshotId = "1$$nfs"

	suite.iboxapi.On("GetNetworkSpaceByName", suite.Suite.T().Context(), mock.Anything).Return(storagecommon.GetNetworkSpace(), nil)
	notFoundError := &iboxapi.APIError{Code: iboxapi.RESOURCE_NOT_FOUND, Err: fmt.Errorf("not found")}
	suite.iboxapi.On("GetFileSystemByName", suite.Suite.T().Context(), mock.Anything).Return(nil, notFoundError)
	suite.iboxapi.On("GetFileSystemByID", suite.Suite.T().Context(), mock.Anything).Return(storagecommon.GetFileSystem(), nil)
	suite.iboxapi.On("GetSystem", suite.Suite.T().Context()).Return(storagecommon.GetSystem(), nil)
	suite.iboxapi.On("GetExportsByFileSystemID", suite.Suite.T().Context(), mock.Anything).Return(storagecommon.GetExportPath(), nil)
	poolResult := &iboxapi.PoolResult{ID: 100}
	suite.iboxapi.On("GetPoolByName", suite.Suite.T().Context(), mock.Anything).Return(poolResult, nil)
	suite.iboxapi.On("CreateFileSystemSnapshot", suite.Suite.T().Context(), mock.Anything).Return(nil, suite.someError)

	_, err := suite.service.CreateVolume(suite.Suite.T().Context(), createVolReq)
	assert.NotNil(suite.T(), err.Error(), "failed to create snapshot")
}

func (suite *NFSControllerSuite) Test_CreateVolume_Snapshot_exportPath_failed() {
	parameterMap := getCreateVolumeParameter()
	createVolReq := getCreateVolumeSnapshotRequest("PVName", parameterMap)
	createVolReq.GetVolumeContentSource().GetSnapshot().SnapshotId = "1$$nfs"

	suite.iboxapi.On("GetNetworkSpaceByName", suite.Suite.T().Context(), mock.Anything).Return(storagecommon.GetNetworkSpace(), nil)
	suite.iboxapi.On("GetFileSystemByName", suite.Suite.T().Context(), mock.Anything).Return(storagecommon.GetFileSystem(), nil)
	suite.iboxapi.On("GetExportsByFileSystemID", suite.Suite.T().Context(), mock.Anything).Return(nil, suite.someError)
	suite.iboxapi.On("GetFileSystemByID", suite.Suite.T().Context(), mock.Anything).Return(storagecommon.GetFileSystem(), nil)
	suite.iboxapi.On("GetSystem", suite.Suite.T().Context()).Return(storagecommon.GetSystem(), nil)
	poolResult := &iboxapi.PoolResult{ID: 100}
	suite.iboxapi.On("GetPoolByName", suite.Suite.T().Context(), mock.Anything).Return(poolResult, nil)
	suite.iboxapi.On("CreateFileSystemSnapshot", suite.Suite.T().Context(), mock.Anything).Return(GetFileSystemSnapshotResponse(1), nil)
	suite.iboxapi.On("CreateExport", suite.Suite.T().Context(), mock.Anything).Return(nil, suite.someError)
	suite.iboxapi.On("DeleteFileSystem", suite.Suite.T().Context(), mock.Anything).Return(nil)

	_, err := suite.service.CreateVolume(suite.Suite.T().Context(), createVolReq)
	assert.NotNil(suite.T(), err.Error(), "failed to get export path")
}

func (suite *NFSControllerSuite) Test_CreateVolume_Snapshot_metadatafailed() {
	parameterMap := getCreateVolumeParameter()
	createVolReq := getCreateVolumeSnapshotRequest("PVName", parameterMap)
	createVolReq.GetVolumeContentSource().GetSnapshot().SnapshotId = "1$$nfs"

	suite.iboxapi.On("GetNetworkSpaceByName", suite.Suite.T().Context(), mock.Anything).Return(storagecommon.GetNetworkSpace(), nil)
	notFoundError := &iboxapi.APIError{Code: iboxapi.RESOURCE_NOT_FOUND, Err: fmt.Errorf("not found")}
	suite.iboxapi.On("GetFileSystemByName", suite.Suite.T().Context(), mock.Anything).Return(nil, notFoundError)
	suite.iboxapi.On("GetFileSystemByID", suite.Suite.T().Context(), mock.Anything).Return(storagecommon.GetFileSystem(), nil)
	poolResult := &iboxapi.PoolResult{ID: 100}
	suite.iboxapi.On("GetExportsByFileSystemID", suite.Suite.T().Context(), mock.Anything).Return(storagecommon.GetExportPath(), nil)
	suite.iboxapi.On("GetPoolByName", suite.Suite.T().Context(), mock.Anything).Return(poolResult, nil)
	suite.iboxapi.On("GetSystem", suite.Suite.T().Context()).Return(storagecommon.GetSystem(), nil)
	suite.iboxapi.On("CreateFileSystemSnapshot", suite.Suite.T().Context(), mock.Anything).Return(GetFileSystemSnapshotResponse(1), nil)
	suite.iboxapi.On("CreateExport", suite.Suite.T().Context(), mock.Anything).Return(storagecommon.GetExportResponseValue(), nil)
	suite.iboxapi.On("PutMetadata", suite.Suite.T().Context(), mock.Anything, mock.Anything).Return(nil, suite.someError)
	suite.iboxapi.On("DeleteFileSystem", suite.Suite.T().Context(), mock.Anything).Return(nil)
	suite.iboxapi.On("DeleteExport", suite.Suite.T().Context(), mock.Anything).Return(&iboxapi.Export{}, nil)

	_, err := suite.service.CreateVolume(suite.Suite.T().Context(), createVolReq)
	assert.NotNil(suite.T(), err.Error(), "failed to update metadata")
}

func (suite *NFSControllerSuite) Test_CreateVolume_Snapshot_Success() {
	parameterMap := getCreateVolumeParameter()
	createVolReq := getCreateVolumeSnapshotRequest("PVName", parameterMap)
	createVolReq.GetVolumeContentSource().GetSnapshot().SnapshotId = "1$$nfs"

	suite.iboxapi.On("GetNetworkSpaceByName", suite.Suite.T().Context(), mock.Anything).Return(storagecommon.GetNetworkSpace(), nil)
	suite.iboxapi.On("GetFileSystemByName", suite.Suite.T().Context(), mock.Anything).Return(storagecommon.GetFileSystem(), nil)
	suite.iboxapi.On("GetFileSystemByID", suite.Suite.T().Context(), mock.Anything).Return(storagecommon.GetFileSystem(), nil)
	poolResult := iboxapi.PoolResult{
		ID: 100,
	}
	suite.iboxapi.On("GetExportsByFileSystemID", suite.Suite.T().Context(), mock.Anything).Return(storagecommon.GetExportPath(), nil)
	suite.iboxapi.On("GetSystem", suite.Suite.T().Context()).Return(storagecommon.GetSystem(), nil)
	suite.iboxapi.On("GetPoolByName", suite.Suite.T().Context(), mock.Anything).Return(&poolResult, nil)
	suite.iboxapi.On("CreateFileSystemSnapshot", suite.Suite.T().Context(), mock.Anything).Return(GetFileSystemSnapshotResponse(1), nil)
	suite.iboxapi.On("CreateExport", suite.Suite.T().Context(), mock.Anything).Return(storagecommon.GetExportResponseValue(), nil)
	suite.iboxapi.On("PutMetadata", suite.Suite.T().Context(), mock.Anything, mock.Anything).Return(nil, nil)

	_, err := suite.service.CreateVolume(suite.Suite.T().Context(), createVolReq)
	assert.Nil(suite.T(), err, "expected to succeed: CreateSnapshot")
}

func (suite *NFSControllerSuite) Test_CreateVolume_Clone_Success() {
	parameterMap := getCreateVolumeParameter()
	createVolReq := getCreateVolumeCloneRequest("PVName", parameterMap)
	createVolReq.GetVolumeContentSource().GetVolume().VolumeId = "1$$nfs"

	suite.iboxapi.On("GetNetworkSpaceByName", suite.Suite.T().Context(), mock.Anything).Return(storagecommon.GetNetworkSpace(), nil)
	suite.iboxapi.On("GetExportsByFileSystemID", suite.Suite.T().Context(), mock.Anything).Return(storagecommon.GetExportPath(), nil)
	suite.iboxapi.On("GetFileSystemByName", suite.Suite.T().Context(), mock.Anything).Return(storagecommon.GetFileSystem(), nil)
	suite.iboxapi.On("GetSystem", suite.Suite.T().Context()).Return(storagecommon.GetSystem(), nil)
	suite.iboxapi.On("GetFileSystemByID", suite.Suite.T().Context(), mock.Anything).Return(storagecommon.GetFileSystem(), nil)
	poolResult := iboxapi.PoolResult{
		ID: 100,
	}
	suite.iboxapi.On("GetPoolByName", suite.Suite.T().Context(), mock.Anything).Return(&poolResult, nil)
	suite.iboxapi.On("CreateFileSystemSnapshot", suite.Suite.T().Context(), mock.Anything).Return(GetFileSystemSnapshotResponse(1), nil)
	suite.iboxapi.On("CreateExport", suite.Suite.T().Context(), mock.Anything).Return(storagecommon.GetExportResponseValue(), nil)
	suite.iboxapi.On("PutMetadata", suite.Suite.T().Context(), mock.Anything, mock.Anything).Return(nil, nil)

	_, err := suite.service.CreateVolume(suite.Suite.T().Context(), createVolReq)
	assert.Nil(suite.T(), err, "expected clone success")
}

func (suite *NFSControllerSuite) Test_CreateVolume_Clone_failed() {
	parameterMap := getCreateVolumeParameter()
	createVolReq := getCreateVolumeCloneRequest("PVName", parameterMap)
	createVolReq.GetVolumeContentSource().GetVolume().VolumeId = "1$$nfs"

	suite.iboxapi.On("GetNetworkSpaceByName", suite.Suite.T().Context(), mock.Anything).Return(storagecommon.GetNetworkSpace(), nil)
	suite.iboxapi.On("GetFileSystemByName", suite.Suite.T().Context(), mock.Anything).Return(storagecommon.GetFileSystem(), nil)
	suite.iboxapi.On("GetSystem", suite.Suite.T().Context()).Return(storagecommon.GetSystem(), nil)
	suite.iboxapi.On("GetExportsByFileSystemID", suite.Suite.T().Context(), mock.Anything).Return(storagecommon.GetExportPath(), suite.someError)
	suite.iboxapi.On("GetFileSystemByID", suite.Suite.T().Context(), mock.Anything).Return(storagecommon.GetFileSystem(), nil)

	_, err := suite.service.CreateVolume(suite.Suite.T().Context(), createVolReq)
	assert.NotNil(suite.T(), err.Error(), "failed to clone the volume")
}

func (suite *NFSControllerSuite) Test_NfsControllerExpandVolume_VolumeID_empty() {
	suite.iboxapi.On("UpdateFileSystem", suite.Suite.T().Context(), mock.Anything, mock.Anything).Return(mock.Anything, suite.someError)
	_, err := suite.service.ControllerExpandVolume(suite.Suite.T().Context(), getNfsExpandVolumeRequest(""))
	assert.NotNil(suite.T(), err, "expected to fail: NfsControllerExpandVolume Volume ID missing in request")
}

func (suite *NFSControllerSuite) Test_NfsControllerExpandVolume_InvalidVolumeID() {
	volumeID := "10x"
	suite.iboxapi.On("UpdateFileSystem", suite.Suite.T().Context(), mock.Anything, mock.Anything).Return(mock.Anything, suite.someError)
	_, err := suite.service.ControllerExpandVolume(suite.Suite.T().Context(), getNfsExpandVolumeRequest(volumeID))
	assert.NotNil(suite.T(), err, "expected to fail: NfsControllerExpandVolume invalid Volume ID in request")
}

func (suite *NFSControllerSuite) Test_NfsControllerExpandVolume_Error() {
	fileSystemID := "100#"
	suite.iboxapi.On("UpdateFileSystem", suite.Suite.T().Context(), mock.Anything, mock.Anything).Return(mock.Anything, suite.someError)
	_, err := suite.service.ControllerExpandVolume(suite.Suite.T().Context(), getNfsExpandVolumeRequest(fileSystemID))
	assert.NotNil(suite.T(), err, "expected to fail: NfsControllerExpandVolume update file system")
}

func (suite *NFSControllerSuite) Test_NfsControllerExpandVolume_Error_filenotfound() {
	fileSystemID := "100#"
	suite.iboxapi.On("UpdateFileSystem", suite.Suite.T().Context(), mock.Anything, mock.Anything).Return(mock.Anything, suite.someError)
	_, err := suite.service.ControllerExpandVolume(suite.Suite.T().Context(), getNfsExpandVolumeRequest(fileSystemID))
	assert.NotNil(suite.T(), err, "expected to fail: NfsControllerExpandVolume update volume")
}

func (suite *NFSControllerSuite) Test_NfsControllerExpandVolume_success() {
	fileSystemID := "100#"
	suite.iboxapi.On("UpdateFileSystem", suite.Suite.T().Context(), mock.Anything, mock.Anything).Return(mock.Anything, nil)
	_, err := suite.service.ControllerExpandVolume(suite.Suite.T().Context(), getNfsExpandVolumeRequest(fileSystemID))
	assert.Nil(suite.T(), err, "expected to succeed: NfsControllerExpandVolume UpdateVolume")
}

func (suite *NFSControllerSuite) Test_NfsControllerExpandVolume_UpdateVolume_Error() {
	fileSystemID := "100"
	suite.iboxapi.On("UpdateFileSystem", suite.Suite.T().Context(), mock.Anything, mock.Anything).Return(nil, suite.someError)
	_, err := suite.service.ControllerExpandVolume(suite.Suite.T().Context(), getNfsExpandVolumeRequest(fileSystemID))
	assert.NotNil(suite.T(), err, "expected to fail: NfsControllerExpandVolume UpdateFileSystem")
}

func (suite *NFSControllerSuite) Test_NfsControllerExpandVolume_success_expand() {
	fileSystemID := "100"
	suite.iboxapi.On("UpdateFileSystem", suite.Suite.T().Context(), mock.Anything, mock.Anything).Return(nil, nil)
	_, err := suite.service.ControllerExpandVolume(suite.Suite.T().Context(), getNfsExpandVolumeRequest(fileSystemID))
	assert.Nil(suite.T(), err, "expected to succeed: NfsControllerExpandVolume UpdateFileSystem")
}

func (suite *NFSControllerSuite) Test_NfsCreateSnapshot_GetSnapshot_Error() {
	expectedErr := errors.New("Snapshot Name is must")
	suite.iboxapi.On("GetFileSystemByName", suite.Suite.T().Context(), mock.Anything).Return(nil, expectedErr)
	_, err := suite.service.CreateSnapshot(suite.Suite.T().Context(), getNfsCreateSnapshotRequest("100"))
	assert.NotNil(suite.T(), err, "expected to fail: CreateSnapshot get snapshot by name")
}

func (suite *NFSControllerSuite) Test_NfsCreateSnapshot_SourceVolumeID_Error() {
	fileSystem := &iboxapi.FileSystem{}
	suite.iboxapi.On("GetFileSystemByName", suite.Suite.T().Context(), mock.Anything).Return(fileSystem, nil)
	suite.iboxapi.On("GetFileSystemByID", suite.Suite.T().Context(), mock.Anything).Return(fileSystem, nil)
	suite.iboxapi.On("CreateFileSystemSnapshot", suite.Suite.T().Context(), mock.Anything).Return(nil, suite.someError)
	_, err := suite.service.CreateSnapshot(suite.Suite.T().Context(), getNfsCreateSnapshotRequest("100"))
	assert.NotNil(suite.T(), err, "expected to fail: CreateSnapshot create snapshot by name")
}

func (suite *NFSControllerSuite) Test_NfsCreateSnapshot_Success() {
	filesystem := &iboxapi.FileSystem{
		ID:       1,
		Name:     "snapshot",
		ParentID: 1,
	}
	suite.iboxapi.On("GetFileSystemByID", suite.Suite.T().Context(), mock.Anything).Return(filesystem, nil)
	suite.iboxapi.On("GetFileSystemByName", suite.Suite.T().Context(), mock.Anything).Return(filesystem, nil)

	_, err := suite.service.CreateSnapshot(suite.Suite.T().Context(), getNfsCreateSnapshotRequest("1$$nfs"))
	assert.Nil(suite.T(), err, "expected to succeed: CreateSnapshot")
}

func (suite *NFSControllerSuite) Test_NfsCreateSnapshot_CreateFileSystemS_Error() {
	filesystem := &iboxapi.FileSystem{}
	suite.iboxapi.On("GetFileSystemByID", suite.Suite.T().Context(), mock.Anything).Return(filesystem, nil)
	suite.iboxapi.On("GetFileSystemByName", suite.Suite.T().Context(), mock.Anything).Return(filesystem, nil)
	suite.iboxapi.On("CreateFileSystemSnapshot", suite.Suite.T().Context(), mock.Anything).Return(nil, suite.someError)
	_, err := suite.service.CreateSnapshot(suite.Suite.T().Context(), getNfsCreateSnapshotRequest("1$$nfs"))
	assert.NotNil(suite.T(), err, "expected to fail: CreateSnapshot create file system snapshot")
}

func (suite *NFSControllerSuite) Test_NfsCreateSnapshot_CreateFileSystemS_success() {
	filesystem := &iboxapi.FileSystem{
		ID: 1,
	}

	ctx := suite.Suite.T().Context()
	suite.iboxapi.On("GetFileSystemByID", ctx, mock.Anything).Return(filesystem, nil)
	notFoundError := &iboxapi.APIError{Code: iboxapi.RESOURCE_NOT_FOUND, Err: fmt.Errorf("GetFileSystemByName - name '%s' not found", "foo")}
	suite.iboxapi.On("GetFileSystemByName", suite.Suite.T().Context(), mock.Anything).Return(nil, notFoundError)
	suite.iboxapi.On("CreateFileSystemSnapshot", suite.Suite.T().Context(), mock.Anything).Return(filesystem, nil)
	_, err := suite.service.CreateSnapshot(ctx, getNfsCreateSnapshotRequest("1$$nfs"))
	assert.Nil(suite.T(), err, "expected to succeed: CreateSnapshot CreateFileSystemSnapshot")
}

func (suite *NFSControllerSuite) Test_NfsDeleteSnapshot_SourceVolumeID_empty() {
	var snapshotID int
	suite.iboxapi.On("GetFileSystemByID", suite.Suite.T().Context(), snapshotID).Return(nil, suite.someError)
	_, err := suite.service.DeleteSnapshot(suite.Suite.T().Context(), getNfsDeleteSnapshotRequest(""))
	assert.NotNil(suite.T(), err, "expected to fail: NfsDeleteSnapshot GetFileSystemByID Source Volume ID missing in request")
}

func (suite *NFSControllerSuite) Test_NfsDeleteSnapshot_InvalidSourceVolumeID() {
	snapshotID := 1000000000000000000
	suite.iboxapi.On("GetFileSystemByID", suite.Suite.T().Context(), snapshotID).Return(nil, suite.someError)
	_, err := suite.service.DeleteSnapshot(suite.Suite.T().Context(), getNfsDeleteSnapshotRequest("1000000000000000000"))
	assert.NotNil(suite.T(), err, "expected to fail: NfsDeleteSnapshot GetFileSystemByID Invalid Snapshot ID in request")
}

func (suite *NFSControllerSuite) Test_NfsDeleteSnapshot_Error() {
	suite.service.UniqueID = 100
	snapshotID := 100
	suite.iboxapi.On("GetFileSystemByID", suite.Suite.T().Context(), snapshotID).Return(nil, suite.someError)
	_, err := suite.service.DeleteSnapshot(suite.Suite.T().Context(), getNfsDeleteSnapshotRequest("100"))
	assert.NotNil(suite.T(), err, "expected to fail: NfsDeleteSnapshot GetFileSystemByID")
}

func (suite *NFSControllerSuite) Test_NfsDeleteSnapshot_file_not_found() {
	suite.service.UniqueID = 100
	snapshotID := 100
	suite.iboxapi.On("GetFileSystemByID", suite.Suite.T().Context(), snapshotID).Return(nil, suite.someError)
	_, err := suite.service.DeleteSnapshot(suite.Suite.T().Context(), getNfsDeleteSnapshotRequest("100"))
	assert.NotNil(suite.T(), err, "expected to fail: NfsDeleteSnapshot GetFileSystemByID fs not found")
}

func (suite *NFSControllerSuite) Test_NfsDeleteNFSVolume_GetFileSystemByID_error() {
	suite.service.UniqueID = 100
	snapshotID := 100
	suite.iboxapi.On("GetFileSystemByID", suite.Suite.T().Context(), snapshotID).Return(nil, suite.someError)
	err := suite.service.DeleteNFSVolume(suite.Suite.T().Context())
	assert.NotNil(suite.T(), err, "expected to fail: DeleteNFSVolume GetFileSystemByID fs not found")
	assert.Equal(suite.T(), suite.someError, err, "Error not returned as expected")
}

func (suite *NFSControllerSuite) Test_NfsDeleteNFSVolume_GetFileSystemByID_InvalidID() {
	suite.service.UniqueID = 100
	snapshotID := 100
	suite.iboxapi.On("GetFileSystemByID", suite.Suite.T().Context(), snapshotID).Return(nil, suite.someError)
	err := suite.service.DeleteNFSVolume(suite.Suite.T().Context())
	assert.NotNil(suite.T(), err, "expected to fail: DeleteNFSVolume GetFileSystemByID invalid ID")
}

func (suite *NFSControllerSuite) Test_NfsDeleteNFSVolume_Success() {
	var snapshotID, parentID = 100, 200
	fileSystem := &iboxapi.FileSystem{
		ID:       100,
		ParentID: 200,
	}
	suite.service.UniqueID = 100
	metadata := map[string]interface{}{
		"host.k8s.to_be_deleted": true,
	}
	suite.iboxapi.On("GetFileSystemByID", suite.Suite.T().Context(), snapshotID).Return(fileSystem, nil)
	suite.iboxapi.On("GetFileSystemsByParentID", suite.Suite.T().Context(), snapshotID).Return(mock.Anything, nil)
	suite.iboxapi.On("PutMetadata", suite.Suite.T().Context(), snapshotID, metadata).Return(nil, nil)
	suite.iboxapi.On("GetFileSystemByID", suite.Suite.T().Context(), snapshotID).Return(fileSystem, nil)
	suite.api.On("DeleteFileSystemComplete", suite.Suite.T().Context(), snapshotID).Return(nil)
	suite.api.On("DeleteParentFileSystem", suite.Suite.T().Context(), parentID).Return(nil)
	err := suite.service.DeleteNFSVolume(suite.Suite.T().Context())
	assert.Nil(suite.T(), err, "expected to succeed: DeleteNFSVolume")
}

func (suite *NFSControllerSuite) Test_DeleteVolume_fileNotFound_success() {
	nfsDeleteErr := errors.New("FILESYSTEM_NOT_FOUND")
	delValReq := getNFSDeleteRequest()
	suite.iboxapi.On("GetFileSystemByID", suite.Suite.T().Context(), mock.Anything).Return(nil, nfsDeleteErr)
	_, err := suite.service.DeleteVolume(suite.Suite.T().Context(), delValReq)
	assert.Nil(suite.T(), err, "expected to succeed: DeleteVolume when fs not found")
}

func (suite *NFSControllerSuite) Test_DeleteVolume_Error() {
	delValReq := getNFSDeleteRequest()
	suite.iboxapi.On("GetFileSystemByID", suite.Suite.T().Context(), mock.Anything).Return(nil, suite.someError)
	_, err := suite.service.DeleteVolume(suite.Suite.T().Context(), delValReq)
	assert.NotNil(suite.T(), err, "expected to fail: DeleteVolume")
}

func (suite *NFSControllerSuite) Test_DeleteVolume_Metadata_failed() {
	delValReq := getNFSDeleteRequest()
	suite.api.On("DeleteFileSystemComplete", suite.Suite.T().Context(), mock.Anything).Return(suite.someError)
	suite.iboxapi.On("GetFileSystemByID", suite.Suite.T().Context(), mock.Anything).Return(&iboxapi.FileSystem{}, nil)
	suite.iboxapi.On("GetFileSystemsByParentID", suite.Suite.T().Context(), mock.Anything).Return(mock.Anything, nil)
	suite.iboxapi.On("PutMetadata", suite.Suite.T().Context(), mock.Anything, mock.Anything).Return(nil, suite.someError)

	_, err := suite.service.DeleteVolume(suite.Suite.T().Context(), delValReq)
	assert.NotNil(suite.T(), err, "expected to fail: DeleteVolume delete metadata")
}

func (suite *NFSControllerSuite) Test_DeleteVolume_delete_Error() {
	delValReq := getNFSDeleteRequest()
	fs := &iboxapi.FileSystem{
		ParentID: 0,
	}
	emptyFileSystems := make([]iboxapi.FileSystem, 0)
	suite.iboxapi.On("GetFileSystemByID", suite.Suite.T().Context(), mock.Anything).Return(&iboxapi.FileSystem{}, nil)
	suite.iboxapi.On("GetFileSystemsByParentID", suite.Suite.T().Context(), mock.Anything).Return(emptyFileSystems, nil)
	suite.iboxapi.On("PutMetadata", suite.Suite.T().Context(), mock.Anything, mock.Anything).Return(nil, nil)
	suite.iboxapi.On("GetFileSystemByID", suite.Suite.T().Context(), mock.Anything).Return(fs, nil)
	suite.api.On("DeleteFileSystemComplete", suite.Suite.T().Context(), mock.Anything).Return(suite.someError)

	_, err := suite.service.DeleteVolume(suite.Suite.T().Context(), delValReq)
	assert.NotNil(suite.T(), err, "expected to fail: DeleteVolume delete fs complete")
}

func (suite *NFSControllerSuite) Test_DeleteVolume_Err2() {
	delValReq := getNFSDeleteRequest()

	fs := &iboxapi.FileSystem{
		ParentID: 11,
	}
	emptyFileSystems := make([]iboxapi.FileSystem, 0)
	suite.iboxapi.On("GetFileSystemByID", suite.Suite.T().Context(), mock.Anything).Return(fs, nil)
	suite.iboxapi.On("GetFileSystemsByParentID", suite.Suite.T().Context(), mock.Anything).Return(emptyFileSystems, nil)
	suite.iboxapi.On("PutMetadata", suite.Suite.T().Context(), mock.Anything, mock.Anything).Return(nil, nil)
	suite.api.On("DeleteFileSystemComplete", suite.Suite.T().Context(), mock.Anything).Return(nil)
	suite.api.On("DeleteParentFileSystem", suite.Suite.T().Context(), mock.Anything).Return(suite.someError)

	_, err := suite.service.DeleteVolume(suite.Suite.T().Context(), delValReq)
	assert.NotNil(suite.T(), err, "expected to fail: DeleteVolume delete parent fs")
}

func (suite *NFSControllerSuite) Test_DeleteVolume_success() {
	delValReq := getNFSDeleteRequest()

	emptyFileSystems := make([]iboxapi.FileSystem, 0)
	suite.iboxapi.On("GetFileSystemByID", suite.Suite.T().Context(), mock.Anything).Return(&iboxapi.FileSystem{}, nil)
	suite.iboxapi.On("GetFileSystemsByParentID", suite.Suite.T().Context(), mock.Anything).Return(emptyFileSystems, nil)
	suite.iboxapi.On("PutMetadata", suite.Suite.T().Context(), mock.Anything, mock.Anything).Return(nil, nil)
	suite.api.On("DeleteFileSystemComplete", suite.Suite.T().Context(), mock.Anything).Return(nil)
	suite.api.On("DeleteParentFileSystem", suite.Suite.T().Context(), mock.Anything).Return(nil)

	_, err := suite.service.DeleteVolume(suite.Suite.T().Context(), delValReq)
	assert.Nil(suite.T(), err, "expected to succeed: DeleteVolume")
}

func (suite *NFSControllerSuite) Test_ControllerPublishVolume_InvalidNodeID() {
	publishParameter := getPublishVolumeParameter()
	publishVolReq := getNFSControllerPublishVolume(publishParameter)
	publishVolReq.NodeId = "1$12$13"
	suite.accessMock.On("IsValidAccessModeNfs", mock.Anything).Return(true, nil)
	suite.iboxapi.On("GetSystem", suite.Suite.T().Context()).Return(storagecommon.GetSystem(), nil)

	_, err := suite.service.ControllerPublishVolume(suite.Suite.T().Context(), publishVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: ControllerPublishVolume invalid node ID")
}

func (suite *NFSControllerSuite) Test_ControllerPublishVolume_AddNodeInExport_Error() {
	publishParameter := getPublishVolumeParameter()
	publishVolReq := getNFSControllerPublishVolume(publishParameter)
	suite.accessMock.On("IsValidAccessModeNfs", mock.Anything).Return(true, nil)
	suite.api.On("AddNodeInExport", suite.Suite.T().Context(), mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, suite.someError)
	suite.iboxapi.On("GetSystem", suite.Suite.T().Context()).Return(storagecommon.GetSystem(), nil)

	_, err := suite.service.ControllerPublishVolume(suite.Suite.T().Context(), publishVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: ControllerPublishVolume add node in export")
}

func (suite *NFSControllerSuite) Test_ControllerPublishVolume_success() {
	publishParameter := getPublishVolumeParameter()
	publishVolReq := getNFSControllerPublishVolume(publishParameter)
	suite.accessMock.On("IsValidAccessModeNfs", mock.Anything).Return(true, nil)
	suite.api.On("AddNodeInExport", suite.Suite.T().Context(), mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, nil)
	suite.iboxapi.On("GetSystem", suite.Suite.T().Context()).Return(storagecommon.GetSystem(), nil)

	_, err := suite.service.ControllerPublishVolume(suite.Suite.T().Context(), publishVolReq)
	assert.Nil(suite.T(), err, "expected to succeed: ControllerPublishVolume")
}

func (suite *NFSControllerSuite) Test_ControllerUnpublishVolume_DeleteExportRule_error() {
	unpublishVolReq := getNFSControllerUnpublishVolume()
	suite.api.On("DeleteExportRule", suite.Suite.T().Context(), mock.Anything, mock.Anything).Return(suite.someError)
	_, err := suite.service.ControllerUnpublishVolume(suite.Suite.T().Context(), unpublishVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: ControllerUnpublishVolume when DeleteExportRule fails")
}

func (suite *NFSControllerSuite) Test_ControllerUnpublishVolume_DeleteExportRule_success() {
	unpublishVolReq := getNFSControllerUnpublishVolume()
	suite.api.On("DeleteExportRule", suite.Suite.T().Context(), mock.Anything, mock.Anything).Return(nil)
	_, err := suite.service.ControllerUnpublishVolume(suite.Suite.T().Context(), unpublishVolReq)
	assert.Nil(suite.T(), err, "expected to succeed: ControllerUnpublishVolume when DeleteExportRule succeeds")
}

func getNFSControllerUnpublishVolume() *csi.ControllerUnpublishVolumeRequest {
	return &csi.ControllerUnpublishVolumeRequest{
		VolumeId: "1$$nfs",
		NodeId:   "192.168.0.106",
	}
}

func getNFSControllerPublishVolume(parameterMap map[string]string) *csi.ControllerPublishVolumeRequest {
	return &csi.ControllerPublishVolumeRequest{
		VolumeId:      "1",
		VolumeContext: parameterMap,
		NodeId:        "10.20.20.50$$nfs",
	}
}

func getNFSDeleteRequest() *csi.DeleteVolumeRequest {
	return &csi.DeleteVolumeRequest{
		VolumeId: "1",
	}
}

func GetFileSystemSnapshotResponse(snapshotID int) *iboxapi.FileSystemSnapshotResponse {
	return &iboxapi.FileSystemSnapshotResponse{SnapshotID: snapshotID, Name: "snapshotName"}
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
		Name:                name,
		CapacityRange:       &csi.CapacityRange{RequiredBytes: 100 * storagecommon.GIB},
		Parameters:          parameterMap,
		VolumeContentSource: nil,
	}
}

// getCreateVolumeRequestByType - method return the snapshot or clone createVallume request
func getCreateVolumeSnapshotRequest(name string, parameterMap map[string]string) *csi.CreateVolumeRequest {
	createVolume := &csi.CreateVolumeRequest{
		Name:          name,
		CapacityRange: &csi.CapacityRange{RequiredBytes: 100 * storagecommon.GIB},
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
		CapacityRange: &csi.CapacityRange{RequiredBytes: 100 * storagecommon.GIB},
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
		common.StorageClassStorageProtocol:      "nfs",
		common.StorageClassPoolName:             "pool_name1",
		common.StorageClassNetworkSpace:         "network_space1",
		common.StorageClassNFSExportPermissions: "[{'access':'RW','client':'192.168.147.190-192.168.147.199','no_root_squash':false},{'access':'RW','client':'192.168.147.10-192.168.147.20','no_root_squash':'false'}]",
	}
}

func getPublishVolumeParameter() map[string]string {
	return map[string]string{
		"exportID":                              "1",
		common.StorageClassNFSExportPermissions: "[{'access':'RW','client':'192.168.147.190-192.168.147.199','no_root_squash':false},{'access':'RW','client':'192.168.147.10-192.168.147.20','no_root_squash':'false'}]",
	}
}
