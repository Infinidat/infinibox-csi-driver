//go:build unit

package nvme

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/infinidat/infinibox-csi-driver/api"
	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/infinidat/infinibox-csi-driver/helper"
	"github.com/infinidat/infinibox-csi-driver/iboxapi"
	storagecommon "github.com/infinidat/infinibox-csi-driver/storage/common"
	"github.com/infinidat/infinibox-csi-driver/testhelper"
	tests "github.com/infinidat/infinibox-csi-driver/testhelper"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

func (suite *NVMEControllerSuite) SetupTest() {
	suite.api = new(api.MockAPIService)
	suite.iboxapi = new(iboxapi.MockAPIService)
	suite.accessMock = new(helper.MockAccessModesHelper)
	host := &iboxapi.Host{
		ID:   1,
		Name: "host1",
	}
	volProto := &api.VolumeProtocolConfig{
		Host:     host,
		VolumeID: 1,
		NodeID:   "node1",
	}
	suite.cs = &storagecommon.Commonservice{API: suite.api, AccessModesHelper: suite.accessMock, IboxAPI: suite.iboxapi, VolProto: volProto}
	suite.someError = errors.New("some Error")
	suite.service = NVMEstorage{CS: *suite.cs}
}

type NVMEControllerSuite struct {
	suite.Suite
	api        *api.MockAPIService
	iboxapi    *iboxapi.MockAPIService
	accessMock *helper.MockAccessModesHelper
	cs         *storagecommon.Commonservice
	someError  error
	service    NVMEstorage
}

func TestNVMEControllerSuite(t *testing.T) {
	suite.Run(t, new(NVMEControllerSuite))
}

func (suite *NVMEControllerSuite) Test_ValidateStorageClass_InvalidProvisionType_Fail() {
	parameterMap := map[string]string{
		common.StorageClassProvisionType: "somethinginvalid",
	}
	err := suite.service.ValidateStorageClass(parameterMap)
	assert.NotNil(suite.T(), err, "expected to fail: nvme invalid provision type sc parameter")
}
func (suite *NVMEControllerSuite) Test_ValidateStorageClass_ValidPoolName() {
	parameterMap := getNVMECreateVolumeParameters()
	err := suite.service.ValidateStorageClass(parameterMap)
	assert.Nil(suite.T(), err, "expected to pass: nvme valid pool name sc parameter")
}
func (suite *NVMEControllerSuite) Test_ValidateStorageClass_InvalidPoolName() {
	parameterMap := map[string]string{
		common.StorageClassPoolName: "",
	}
	err := suite.service.ValidateStorageClass(parameterMap)
	assert.NotNil(suite.T(), err, "expected to fail: nvme CreateVolume invalid parameter")
}
func (suite *NVMEControllerSuite) Test_ValidateStorageClass_InvalidParameter_No_Parameters() {
	var parameterMap map[string]string
	err := suite.service.ValidateStorageClass(parameterMap)
	assert.NotNil(suite.T(), err, "expected to fail: nvme CreateVolume invalid parameter")
}

func (suite *NVMEControllerSuite) Test_ValidateStorageClass_InvalidParameter_Missing_Network_Space() {
	parameterMap := getNVMECreateVolumeParameters()
	delete(parameterMap, common.StorageClassNetworkSpace)
	err := suite.service.ValidateStorageClass(parameterMap)
	assert.NotNil(suite.T(), err, "expected to fail: nvme CreateVolumeValidate missing parameter")
}
func (suite *NVMEControllerSuite) Test_ValidateStorageClass_InvalidParameter_Missing_Pool() {
	parameterMap := getNVMECreateVolumeParameters()
	delete(parameterMap, common.StorageClassPoolName)
	err := suite.service.ValidateStorageClass(parameterMap)
	assert.NotNil(suite.T(), err, "expected to fail: nvme CreateVolumeValidate missing parameter")
}
func getNVMECreateVolumeParameters() map[string]string {
	return map[string]string{
		common.StorageClassGID:             "2468",
		common.StorageClassMaxVolsPerHost:  "19",
		common.StorageClassNetworkSpace:    "network_space1",
		common.StorageClassPoolName:        "pool_name1",
		common.StorageClassProvisionType:   common.StorageClassThinProvision,
		common.StorageClassSSDEnabled:      "true",
		common.StorageClassStorageProtocol: "nvme",
		common.StorageClassUID:             "1234",
		common.StorageClassUNIXPermissions: "0777",
	}
}

func (suite *NVMEControllerSuite) Test_CreateVolume_GetName_fail() {
	parameterMap := getNVMECreateVolumeParameters()
	createVolReq := tests.GetCreateVolumeRequest("pvname", parameterMap, "")

	suite.iboxapi.On("GetVolumeByName", mock.Anything).Return(storagecommon.GetVolume(), suite.someError)
	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(storagecommon.GetNetworkspace(), nil)
	suite.iboxapi.On("GetSystem", mock.Anything).Return(storagecommon.GetSystem(), nil)
	suite.api.On("OneTimeValidation", mock.Anything, mock.Anything).Return("", nil)
	_, err := suite.service.CreateVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: nvme CreateVolume GetVolumeByName")
}

func (suite *NVMEControllerSuite) Test_CreateVolume_fail() {
	parameterMap := getNVMECreateVolumeParameters()
	createVolReq := tests.GetCreateVolumeRequest("pvname", parameterMap, "")

	poolResult := &iboxapi.PoolResult{ID: 10}
	suite.iboxapi.On("GetPoolByName", mock.Anything).Return(poolResult, nil)

	suite.iboxapi.On("GetVolumeByName", mock.Anything).Return(storagecommon.GetVolume(), nil)

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(storagecommon.GetNetworkspace(), nil)
	suite.api.On("CreateVolume", mock.Anything, mock.Anything).Return(nil, suite.someError)
	suite.iboxapi.On("GetSystem", mock.Anything).Return(storagecommon.GetSystem(), nil)
	suite.api.On("OneTimeValidation", mock.Anything, mock.Anything).Return("", nil)

	_, err := suite.service.CreateVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: nvme CreateVolume")
}

func (suite *NVMEControllerSuite) Test_CreateVolume_success() {
	suite.service.Capacity = common.BytesInOneGibibyte
	parameterMap := getNVMECreateVolumeParameters()
	createVolReq := tests.GetCreateVolumeRequest("pvname", parameterMap, "")

	poolResult := &iboxapi.PoolResult{ID: 10}
	suite.iboxapi.On("GetSystem", mock.Anything).Return(storagecommon.GetSystem(), nil)
	suite.iboxapi.On("GetPoolByName", mock.Anything).Return(poolResult, nil)
	suite.iboxapi.On("GetVolumeByName", mock.Anything).Return(storagecommon.GetVolume(), nil)

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(storagecommon.GetNetworkspace(), nil)
	suite.api.On("CreateVolume", mock.Anything, mock.Anything).Return(storagecommon.GetVolume(), nil)
	suite.iboxapi.On("GetVolume", mock.Anything).Return(storagecommon.GetVolume(), nil)
	suite.api.On("OneTimeValidation", mock.Anything, mock.Anything).Return("", nil)
	suite.iboxapi.On("PutMetadata", mock.Anything, mock.Anything).Return(nil, nil)
	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(storagecommon.GetNetworkspace(), nil)

	_, err := suite.service.CreateVolume(context.Background(), createVolReq)
	assert.Nil(suite.T(), err, "expected to succeed: nvme CreateVolume")
}

func (suite *NVMEControllerSuite) Test_CreateVolume_metadataError() {
	parameterMap := getNVMECreateVolumeParameters()
	createVolReq := tests.GetCreateVolumeRequest("pvname", parameterMap, "")

	poolResult := &iboxapi.PoolResult{ID: 10}
	suite.iboxapi.On("GetPoolByName", mock.Anything).Return(poolResult, nil)

	suite.iboxapi.On("GetVolumeByName", mock.Anything).Return(storagecommon.GetVolume(), nil)

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(storagecommon.GetNetworkspace(), nil)
	suite.iboxapi.On("GetSystem", mock.Anything).Return(storagecommon.GetSystem(), nil)
	suite.api.On("CreateVolume", mock.Anything, mock.Anything).Return(storagecommon.GetVolume(), nil)
	suite.api.On("OneTimeValidation", mock.Anything, mock.Anything).Return("", nil)
	suite.iboxapi.On("GetVolume", mock.Anything).Return(storagecommon.GetVolume(), nil)
	suite.iboxapi.On("PutMetadata", mock.Anything, mock.Anything).Return(nil, suite.someError)

	_, err := suite.service.CreateVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: nvme CreateVolume attach metadata")
}

func (suite *NVMEControllerSuite) Test_DeleteVolume_GetVolume_Error() {
	createVolReq := storagecommon.GetDeleteRequest()
	deleteMetadataResponse := &iboxapi.DeleteMetadataResponse{}
	suite.iboxapi.On("DeleteMetadata", mock.Anything).Return(deleteMetadataResponse, nil)
	suite.iboxapi.On("GetVolume", mock.Anything).Return(nil, suite.someError)
	_, err := suite.service.DeleteVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: nvme DeleteVolume storagecommon.GetVolume")
}

func (suite *NVMEControllerSuite) Test_DeleteVolume_GetVolumeSnapshot_metadataError() {
	createVolReq := storagecommon.GetDeleteRequest()
	deleteMetadataResponse := &iboxapi.DeleteMetadataResponse{}
	suite.iboxapi.On("DeleteMetadata", mock.Anything).Return(deleteMetadataResponse, nil)
	deleteVolumeResponse := iboxapi.DeleteVolumeResponse{}
	suite.iboxapi.On("DeleteVolume", mock.Anything).Return(deleteVolumeResponse, suite.someError)
	suite.iboxapi.On("GetVolume", mock.Anything).Return(storagecommon.GetVolume(), nil)
	suite.iboxapi.On("GetVolumesByParentID", mock.Anything).Return(storagecommon.GetVolumeArray(), nil)
	suite.iboxapi.On("PutMetadata", mock.Anything, mock.Anything).Return(nil, suite.someError)

	_, err := suite.service.DeleteVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: nvme DeleteVolume GetVolumeSnapshot attach metadata")
}

func (suite *NVMEControllerSuite) Test_DeleteVolume_Error() {
	createVolReq := storagecommon.GetDeleteRequest()
	suite.iboxapi.On("GetVolume", mock.Anything).Return(storagecommon.GetVolume(), nil)
	suite.iboxapi.On("GetVolumesByParentID", mock.Anything).Return([]iboxapi.Volume{}, nil)
	deleteMetadataResponse := &iboxapi.DeleteMetadataResponse{}
	suite.iboxapi.On("DeleteMetadata", mock.Anything).Return(deleteMetadataResponse, nil)
	deleteVolumeResponse := iboxapi.DeleteVolumeResponse{}
	suite.iboxapi.On("DeleteVolume", mock.Anything).Return(deleteVolumeResponse, suite.someError)

	_, err := suite.service.DeleteVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: nvme DeleteVolume delete volume")
}

func (suite *NVMEControllerSuite) Test_DeleteVolume_success() {
	createVolReq := storagecommon.GetDeleteRequest()
	suite.iboxapi.On("GetVolume", mock.Anything).Return(storagecommon.GetVolume(), nil)
	deleteMetadataResponse := &iboxapi.DeleteMetadataResponse{}
	suite.iboxapi.On("DeleteMetadata", mock.Anything).Return(deleteMetadataResponse, nil)
	suite.iboxapi.On("GetVolumesByParentID", mock.Anything).Return([]iboxapi.Volume{}, nil)
	suite.iboxapi.On("GetMetadata", mock.Anything).Return(testhelper.GetHostMetadata(), nil)
	deleteVolumeResponse := iboxapi.DeleteVolumeResponse{}
	suite.iboxapi.On("DeleteVolume", mock.Anything).Return(deleteVolumeResponse, nil)
	suite.api.On("GetMetadataStatus", mock.Anything).Return(false)
	_, err := suite.service.DeleteVolume(context.Background(), createVolReq)
	assert.Nil(suite.T(), err, "expected to succeed: nvme DeleteVolume")
}

func (suite *NVMEControllerSuite) Test_DeleteVolume_AlreadyDelete() {
	createVolReq := storagecommon.GetDeleteRequest()
	notFoundError := &iboxapi.APIError{Code: iboxapi.IBOXAPI_RESOURCE_NOT_FOUND_ERROR, Err: fmt.Errorf("volume not found")}
	suite.iboxapi.On("GetVolume", mock.Anything).Return(nil, notFoundError)
	_, err := suite.service.DeleteVolume(context.Background(), createVolReq)
	assert.Nil(suite.T(), err, "expected to succeed: nvme DeleteVolume when already deleted")
}

func (suite *NVMEControllerSuite) Test_ControllerPublishVolume() {
	ctrPublishValReq := getNVMEControllerPublishVolumeRequest()
	suite.iboxapi.On("GetMetadata", mock.Anything).Return(testhelper.GetHostMetadata(), nil)
	suite.iboxapi.On("PutMetadata", mock.Anything, mock.Anything).Return(nil, nil)
	suite.accessMock.On("IsValidAccessMode", mock.Anything, mock.Anything).Return(true, nil)
	suite.iboxapi.On("CreateHost", mock.Anything).Return(storagecommon.GetHostByName(), nil)
	suite.iboxapi.On("GetSystem", mock.Anything).Return(storagecommon.GetSystem(), nil)
	suite.iboxapi.On("GetHostByName", mock.Anything).Return(storagecommon.GetHostByName(), nil)
	suite.iboxapi.On("GetAllLunByHost", mock.Anything).Return(storagecommon.GetLunInfoArry(), nil)
	lunInfo := storagecommon.GetLunInf()
	suite.iboxapi.On("MapVolumeToHost", mock.Anything, mock.Anything, mock.Anything).Return(&lunInfo, nil)
	suite.iboxapi.On("GetVolume", mock.Anything).Return(storagecommon.GetVolume(), nil)
	_, err := suite.service.ControllerPublishVolume(context.Background(), ctrPublishValReq)
	assert.Nil(suite.T(), err, "expected to succeed: nvme ControllerPublishVolume")
}

func (suite *NVMEControllerSuite) Test_ControllerPublishVolume_VolumeIDFormatError() {
	ctrPublishValReq := getNVMEControllerPublishVolumeRequest()
	ctrPublishValReq.VolumeId = "1$"
	suite.iboxapi.On("GetSystem", mock.Anything).Return(storagecommon.GetSystem(), nil)
	_, err := suite.service.ControllerPublishVolume(context.Background(), ctrPublishValReq)
	assert.NotNil(suite.T(), err, "expected to fail: nvme ControllerPublishVolume volume ID format invalid protocol")
}

func getNVMEControllerPublishVolumeRequest() *csi.ControllerPublishVolumeRequest {
	return &csi.ControllerPublishVolumeRequest{
		VolumeId:      "1$$nvme",
		NodeId:        "10.20.20.50$$nvme",
		VolumeContext: map[string]string{common.StorageClassMaxVolsPerHost: "10"},
	}
}

func (suite *NVMEControllerSuite) Test_ControllerUnpublishVolume_success() {
	deleteHostResponse := &iboxapi.DeleteHostResponse{
		Error: iboxapi.Error{},
	}
	ctrUnPublishValReq := getNVMEControllerUnpublishVolume()
	suite.iboxapi.On("GetMetadata", mock.Anything).Return(testhelper.GetHostMetadata(), nil)
	suite.iboxapi.On("GetHostByName", mock.Anything).Return(storagecommon.GetHostByName(), nil)
	suite.iboxapi.On("UnMapVolumeFromHost", mock.Anything, mock.Anything).Return(mock.Anything, nil)
	suite.iboxapi.On("GetAllLunByHost", mock.Anything).Return([]api.LunInfo{}, nil)
	suite.iboxapi.On("DeleteHost", mock.Anything).Return(deleteHostResponse, nil)
	suite.iboxapi.On("GetVolume", mock.Anything).Return(storagecommon.GetVolume(), nil)
	_, err := suite.service.ControllerUnpublishVolume(context.Background(), ctrUnPublishValReq)
	assert.Nil(suite.T(), err, "expected to succeed: nvme ControllerUnpublishVolume")
}

func (suite *NVMEControllerSuite) Test_ControllerUnpublishVolume_UnMapVolumeErr() {
	ctrUnPublishValReq := getNVMEControllerUnpublishVolume()
	suite.iboxapi.On("GetHostByName", mock.Anything).Return(storagecommon.GetHostByName(), nil)
	suite.iboxapi.On("UnMapVolumeFromHost", mock.Anything, mock.Anything).Return(nil, suite.someError)
	suite.iboxapi.On("GetAllLunByHost", mock.Anything).Return([]api.LunInfo{}, nil)
	suite.iboxapi.On("GetMetadata", mock.Anything).Return(testhelper.GetHostMetadata(), nil)
	suite.iboxapi.On("DeleteHost", mock.Anything).Return(nil, suite.someError)
	_, err := suite.service.ControllerUnpublishVolume(context.Background(), ctrUnPublishValReq)
	assert.NotNil(suite.T(), err, "expected to fail: nvme ControllerUnpublishVolume UnMapVolumeFromHost")
}

func (suite *NVMEControllerSuite) Test_ControllerUnpublishVolume_DeleteHostErr() {
	ctrUnPublishValReq := getNVMEControllerUnpublishVolume()
	suite.iboxapi.On("GetMetadata", mock.Anything).Return(testhelper.GetHostMetadata(), nil)
	suite.iboxapi.On("GetHostByName", mock.Anything).Return(storagecommon.GetHostByName(), nil)
	suite.iboxapi.On("UnMapVolumeFromHost", mock.Anything, mock.Anything).Return(mock.Anything, nil)
	suite.iboxapi.On("GetAllLunByHost", mock.Anything).Return([]api.LunInfo{}, nil)
	suite.iboxapi.On("DeleteHost", mock.Anything).Return(nil, suite.someError)
	_, err := suite.service.ControllerUnpublishVolume(context.Background(), ctrUnPublishValReq)
	assert.NotNil(suite.T(), err, "expected to fail: nvme ControllerUnpublishVolume DeleteHost")
}

func (suite *NVMEControllerSuite) Test_ControllerUnpublishVolume_Metadata_Error() {
	ctrUnPublishValReq := getNVMEControllerUnpublishVolume()
	suite.iboxapi.On("GetMetadata", mock.Anything).Return(testhelper.GetHostMetadata(), errors.New("some error"))
	suite.iboxapi.On("GetHostByName", mock.Anything).Return(storagecommon.GetHostByName(), nil)
	suite.iboxapi.On("UnMapVolumeFromHost", mock.Anything, mock.Anything).Return(mock.Anything, nil)
	suite.iboxapi.On("GetAllLunByHost", mock.Anything).Return([]api.LunInfo{}, nil)
	suite.iboxapi.On("DeleteHost", mock.Anything).Return(nil, suite.someError)
	_, err := suite.service.ControllerUnpublishVolume(context.Background(), ctrUnPublishValReq)
	assert.NotNil(suite.T(), err, "expected to fail: nvme ControllerUnpublishVolume Metadata Error")
}
func getNVMEControllerUnpublishVolume() *csi.ControllerUnpublishVolumeRequest {
	return &csi.ControllerUnpublishVolumeRequest{
		VolumeId: "1$$nvme",
		NodeId:   "10.20.20.50$$nvme",
	}
}

func (suite *NVMEControllerSuite) Test_CreateSnapshot() {
	ctrUnPublishValReq := getNVMECreateSnapshotRequest()
	suite.iboxapi.On("GetVolume", mock.Anything).Return(storagecommon.GetVolume(), nil)
	suite.iboxapi.On("GetVolumeByName", mock.Anything).Return(storagecommon.GetVolume(), suite.someError)
	suite.iboxapi.On("CreateSnapshotVolume", mock.Anything).Return(storagecommon.GetSnapshotResp(), nil)

	_, err := suite.service.CreateSnapshot(context.Background(), ctrUnPublishValReq)
	assert.NotNil(suite.T(), err, "expected to fail: nvme CreateSnapshot GetVolumeByName")
}

func (suite *NVMEControllerSuite) Test_CreateSnapshot_already_Created() {
	suite.cs.VolProto.VolumeID = 1001
	ctrUnPublishValReq := getNVMECreateSnapshotRequest()
	ctrUnPublishValReq.SourceVolumeId = "1001$$nvme"
	suite.iboxapi.On("GetVolumeByName", mock.Anything).Return(storagecommon.GetVolume(), nil)
	suite.iboxapi.On("CreateSnapshotVolume", mock.Anything).Return(storagecommon.GetSnapshotResp(), nil)

	_, err := suite.service.CreateSnapshot(context.Background(), ctrUnPublishValReq)
	assert.Nil(suite.T(), err, "expected to succeed: nvme CreateSnapshot")
}

func (suite *NVMEControllerSuite) Test_DeleteSnapshot() {
	ctrdeleteSnapValReq := getNVMEDeleteSnapshotRequest()
	suite.iboxapi.On("GetVolume", mock.Anything).Return(storagecommon.GetVolume(), nil)
	suite.iboxapi.On("GetVolumesByParentID", mock.Anything).Return([]iboxapi.Volume{}, nil)
	suite.iboxapi.On("GetMetadata", mock.Anything).Return(testhelper.GetHostMetadata(), nil)
	deleteMetadataResponse := &iboxapi.DeleteMetadataResponse{}
	suite.iboxapi.On("DeleteMetadata", mock.Anything).Return(deleteMetadataResponse, nil)
	deleteVolumeResponse := iboxapi.DeleteVolumeResponse{}
	suite.iboxapi.On("DeleteVolume", mock.Anything).Return(deleteVolumeResponse, nil)
	suite.api.On("GetMetadataStatus", mock.Anything).Return(false)

	_, err := suite.service.DeleteSnapshot(context.Background(), ctrdeleteSnapValReq)
	assert.Nil(suite.T(), err, "expected to succeed: nvme DeleteSnapshot")
}

func getNVMECreateSnapshotRequest() *csi.CreateSnapshotRequest {
	return &csi.CreateSnapshotRequest{
		SourceVolumeId: "1$$nvme",
		Name:           "snapshotName",
	}
}
func getNVMEDeleteSnapshotRequest() *csi.DeleteSnapshotRequest {
	return &csi.DeleteSnapshotRequest{
		SnapshotId: "1$$nvme",
	}
}

func (suite *NVMEControllerSuite) Test_ControllerExpandVolume() {
	ctrExpandValReq := getNVMEExpandVolumeRequest()
	suite.iboxapi.On("UpdateVolume", mock.Anything, mock.Anything).Return(nil, nil)
	_, err := suite.service.ControllerExpandVolume(context.Background(), ctrExpandValReq)
	assert.Nil(suite.T(), err, "expected to succeed: nvme ControllerExpandVolume")
}

func getNVMEExpandVolumeRequest() *csi.ControllerExpandVolumeRequest {
	return &csi.ControllerExpandVolumeRequest{
		VolumeId: "1",
	}
}
