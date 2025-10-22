//go:build unit

package fc

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

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

func (suite *FCControllerSuite) SetupTest() {
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
	cs := storagecommon.Commonservice{API: suite.api, AccessModesHelper: suite.accessMock, IboxAPI: suite.iboxapi, VolProto: volProto}
	suite.service = FCstorage{CS: cs}
	suite.someError = errors.New("some Error")
}

type FCControllerSuite struct {
	suite.Suite
	api        *api.MockAPIService
	iboxapi    *iboxapi.MockAPIService
	accessMock *helper.MockAccessModesHelper
	service    FCstorage
	someError  error
}

func TestFCControllerSuite(t *testing.T) {
	suite.Run(t, new(FCControllerSuite))
}

func (suite *FCControllerSuite) Test_ValidateStorageClass_InvalidProvisionType_Fail() {
	parameterMap := map[string]string{
		common.StorageClassProvisionType: "somethinginvalid",
	}
	err := suite.service.ValidateStorageClass(parameterMap)
	assert.NotNil(suite.T(), err, "expected to fail: fc invalid provision type sc parameter")
}
func (suite *FCControllerSuite) Test_ValidateStorageClass_ValidPoolName() {
	parameterMap := map[string]string{
		common.StorageClassPoolName: "my-test-pool",
	}
	err := suite.service.ValidateStorageClass(parameterMap)
	assert.Nil(suite.T(), err, "expected to pass: fc invalid pool name sc parameter")
}
func (suite *FCControllerSuite) Test_ValidateStorageClass_InvalidPoolName() {
	parameterMap := map[string]string{
		common.StorageClassPoolName: "",
	}
	err := suite.service.ValidateStorageClass(parameterMap)
	assert.NotNil(suite.T(), err, "expected to fail: fc CreateVolume invalid parameter")
}

func (suite *FCControllerSuite) Test_CreateVolume_GetName_fail() {
	parameterMap := getFCCreateVolumeParameter()
	createVolReq := testhelper.GetCreateVolumeRequest("PVName", parameterMap, "")

	suite.iboxapi.On("GetVolumeByName", mock.Anything).Return(storagecommon.GetVolume(), suite.someError)
	suite.iboxapi.On("GetSystem", mock.Anything).Return(storagecommon.GetSystem(), nil)
	suite.api.On("OneTimeValidation", mock.Anything, mock.Anything).Return("", nil)
	_, err := suite.service.CreateVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: fc CreateVolume GetVolumeByName")
}

func (suite *FCControllerSuite) Test_CreateVolume_fail() {
	parameterMap := getFCCreateVolumeParameter()
	createVolReq := testhelper.GetCreateVolumeRequest("PVName", parameterMap, "")

	poolResult := &iboxapi.PoolResult{ID: 10}
	suite.iboxapi.On("GetPoolByName", mock.Anything).Return(poolResult, nil)
	suite.iboxapi.On("GetSystem", mock.Anything).Return(storagecommon.GetSystem(), nil)
	suite.iboxapi.On("GetVolumeByName", mock.Anything).Return(nil, nil)

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(storagecommon.GetNetworkspace(), nil)
	suite.iboxapi.On("CreateVolume", mock.Anything).Return(nil, suite.someError)
	suite.api.On("OneTimeValidation", mock.Anything, mock.Anything).Return("", nil)

	_, err := suite.service.CreateVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: fc CreateVolume create volume")
}

func (suite *FCControllerSuite) Test_CreateVolume_success() {
	parameterMap := getFCCreateVolumeParameter()
	createVolReq := testhelper.GetCreateVolumeRequest("PVName", parameterMap, "")

	poolResult := &iboxapi.PoolResult{ID: 10}
	suite.iboxapi.On("GetPoolByName", mock.Anything).Return(poolResult, nil)
	suite.iboxapi.On("GetVolumeByName", mock.Anything).Return(nil, nil)

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(storagecommon.GetNetworkspace(), nil)
	suite.iboxapi.On("CreateVolume", mock.Anything).Return(storagecommon.GetIboxapiCreateVolumeResponse(), nil)
	suite.iboxapi.On("GetVolume", mock.Anything).Return(storagecommon.GetVolume(), nil)
	suite.iboxapi.On("GetSystem", mock.Anything).Return(storagecommon.GetSystem(), nil)
	suite.iboxapi.On("PutMetadata", mock.Anything, mock.Anything).Return(nil, nil)
	suite.api.On("OneTimeValidation", mock.Anything, mock.Anything).Return("", nil)

	_, err := suite.service.CreateVolume(context.Background(), createVolReq)
	assert.Nil(suite.T(), err, "expected to succeed: fc CreateVolume")
}

func (suite *FCControllerSuite) Test_CreateVolume_metadataError() {
	parameterMap := getFCCreateVolumeParameter()
	createVolReq := testhelper.GetCreateVolumeRequest("PVName", parameterMap, "")

	poolResult := &iboxapi.PoolResult{ID: 10}
	suite.iboxapi.On("GetPoolByName", mock.Anything).Return(poolResult, nil)
	suite.iboxapi.On("GetVolumeByName", mock.Anything).Return(nil, nil)

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(storagecommon.GetNetworkspace(), nil)
	suite.iboxapi.On("CreateVolume", mock.Anything).Return(storagecommon.GetIboxapiCreateVolumeResponse(), nil)
	suite.iboxapi.On("GetSystem", mock.Anything).Return(storagecommon.GetSystem(), nil)
	suite.iboxapi.On("GetVolume", mock.Anything).Return(storagecommon.GetVolume(), nil)
	suite.iboxapi.On("PutMetadata", mock.Anything, mock.Anything).Return(nil, suite.someError)
	suite.api.On("OneTimeValidation", mock.Anything, mock.Anything).Return("", nil)

	_, err := suite.service.CreateVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: fc CreateVolume attach metadata")
}

func (suite *FCControllerSuite) Test_DeleteVolume_GetVolume_Error() {
	createVolReq := storagecommon.GetDeleteRequest()
	suite.iboxapi.On("GetVolume", mock.Anything).Return(nil, suite.someError)
	_, err := suite.service.DeleteVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: fc DeleteVolume GetVolume")
}

func (suite *FCControllerSuite) Test_DeleteVolume_GetVolumeSnapshot_metadataError() {
	createVolReq := storagecommon.GetDeleteRequest()
	suite.iboxapi.On("GetVolume", mock.Anything).Return(storagecommon.GetVolume(), nil)
	suite.iboxapi.On("DeleteMetadata", mock.Anything).Return(nil, suite.someError)
	suite.iboxapi.On("GetVolumesByParentID", mock.Anything).Return(storagecommon.GetVolumeArray(), nil)
	suite.iboxapi.On("PutMetadata", mock.Anything, mock.Anything).Return(nil, suite.someError)

	_, err := suite.service.DeleteVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: fc DeleteVolume attach metadata")
}

func (suite *FCControllerSuite) Test_DeleteVolume_Error() {
	createVolReq := storagecommon.GetDeleteRequest()
	deleteMetadataResponse := &iboxapi.DeleteMetadataResponse{}
	suite.iboxapi.On("DeleteMetadata", mock.Anything).Return(deleteMetadataResponse, nil)
	suite.iboxapi.On("GetVolume", mock.Anything).Return(storagecommon.GetVolume(), nil)
	suite.iboxapi.On("GetVolumesByParentID", mock.Anything).Return([]iboxapi.Volume{}, nil)
	suite.iboxapi.On("DeleteVolume", mock.Anything).Return(nil, suite.someError)

	_, err := suite.service.DeleteVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: fc DeleteVolume delete volume")
}

func (suite *FCControllerSuite) Test_DeleteVolume_success() {
	createVolReq := storagecommon.GetDeleteRequest()
	suite.iboxapi.On("GetVolume", mock.Anything).Return(storagecommon.GetVolume(), nil)
	suite.iboxapi.On("GetVolumesByParentID", mock.Anything).Return([]iboxapi.Volume{}, nil)
	deleteMetadataResponse := &iboxapi.DeleteMetadataResponse{}
	suite.iboxapi.On("DeleteMetadata", mock.Anything).Return(deleteMetadataResponse, nil)
	suite.iboxapi.On("DeleteVolume", mock.Anything).Return(iboxapi.DeleteVolumeResponse{}, nil)
	suite.iboxapi.On("GetMetadata", mock.Anything).Return(testhelper.GetHostMetadata(), nil)
	_, err := suite.service.DeleteVolume(context.Background(), createVolReq)
	assert.Nil(suite.T(), err, "expected to succeed: fc DeleteVolume")
}

func (suite *FCControllerSuite) Test_DeleteVolume_AlreadyDelete() {
	createVolReq := storagecommon.GetDeleteRequest()

	notFoundError := &iboxapi.APIError{Code: iboxapi.IBOXAPI_RESOURCE_NOT_FOUND_ERROR, Err: fmt.Errorf("volume not found")}
	suite.iboxapi.On("GetVolume", mock.Anything).Return(nil, notFoundError)
	_, err := suite.service.DeleteVolume(context.Background(), createVolReq)
	assert.Nil(suite.T(), err, "expected to succeed: fc DeleteVolume already deleted")
}

func (suite *FCControllerSuite) Test_CreateVolume_content_success() {
	suite.service.Capacity = common.BytesInOneGibibyte
	parameterMap := getFCCreateVolumeParameter()
	createVolReq := testhelper.GetCreateVolumeRequest("volumeName", parameterMap, "1$$fc")
	poolResult := &iboxapi.PoolResult{ID: 10}
	suite.iboxapi.On("GetPoolByName", mock.Anything).Return(poolResult, nil)
	suite.iboxapi.On("GetSystem", mock.Anything).Return(storagecommon.GetSystem(), nil)
	suite.iboxapi.On("GetVolumeByName", mock.Anything).Return(nil, nil)
	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(storagecommon.GetNetworkspace(), nil)
	suite.iboxapi.On("CreateSnapshotVolume", mock.Anything).Return(storagecommon.GetSnapshotResp(), nil)
	suite.iboxapi.On("GetVolume", mock.Anything).Return(storagecommon.GetVolume(), nil)
	suite.iboxapi.On("PutMetadata", mock.Anything, mock.Anything).Return(nil, nil)
	suite.api.On("OneTimeValidation", mock.Anything, mock.Anything).Return("", nil)

	_, err := suite.service.CreateVolume(context.Background(), createVolReq)
	assert.Nil(suite.T(), err, "expected to succeed: fc CreateVolume")
}

func (suite *FCControllerSuite) Test_CreateVolume_content_AttachMetadataToObject_err() {
	parameterMap := getFCCreateVolumeParameter()
	createVolReq := testhelper.GetCreateVolumeRequest("volumeName", parameterMap, "1$$fc")
	suite.iboxapi.On("GetVolumeByName", mock.Anything).Return(nil, nil)
	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(storagecommon.GetNetworkspace(), nil)
	suite.iboxapi.On("GetVolume", mock.Anything).Return(storagecommon.GetVolume(), nil)
	suite.iboxapi.On("GetSystem", mock.Anything).Return(storagecommon.GetSystem(), nil)
	suite.api.On("OneTimeValidation", mock.Anything, mock.Anything).Return("", nil)
	suite.iboxapi.On("CreateSnapshotVolume", mock.Anything).Return(storagecommon.GetSnapshotResp(), nil)
	suite.iboxapi.On("GetVolume", mock.Anything).Return(storagecommon.GetVolume(), nil)
	suite.iboxapi.On("PutMetadata", mock.Anything, mock.Anything).Return(nil, suite.someError)
	_, err := suite.service.CreateVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: fc CreateVolume attach metadata")
}

func (suite *FCControllerSuite) Test_ControllerPublishVolume_success() {
	ctrPublishValReq := storagecommon.GetISCSIControllerPublishVolumeRequest()
	suite.accessMock.On("IsValidAccessMode", mock.Anything, mock.Anything).Return(true, nil)
	suite.iboxapi.On("CreateHost", mock.Anything).Return(storagecommon.GetHostByName(), nil)
	suite.iboxapi.On("GetSystem", mock.Anything).Return(storagecommon.GetSystem(), nil)
	suite.iboxapi.On("GetHostByName", mock.Anything).Return(storagecommon.GetHostByName(), nil)
	suite.iboxapi.On("PutMetadata", mock.Anything, mock.Anything).Return(nil, nil)
	suite.iboxapi.On("GetAllLunByHost", mock.Anything).Return(storagecommon.GetLunInfoArry(), nil)
	lunInfo := storagecommon.GetLunInf()
	suite.iboxapi.On("MapVolumeToHost", mock.Anything, mock.Anything, mock.Anything).Return(&lunInfo, nil)
	suite.iboxapi.On("GetVolume", mock.Anything).Return(storagecommon.GetVolume(), nil)
	_, err := suite.service.ControllerPublishVolume(context.Background(), ctrPublishValReq)
	assert.Nil(suite.T(), err, "expected to succeed: fc ControllerPublishVolume")
}

func (suite *FCControllerSuite) Test_ControllerPublishVolume_VolumeIDFormatError() {
	ctrPublishValReq := storagecommon.GetISCSIControllerPublishVolumeRequest()
	ctrPublishValReq.VolumeId = "1$"
	suite.iboxapi.On("GetVolume", mock.Anything).Return(storagecommon.GetVolume(), nil)
	suite.iboxapi.On("GetSystem", mock.Anything).Return(storagecommon.GetSystem(), nil)
	_, err := suite.service.ControllerPublishVolume(context.Background(), ctrPublishValReq)
	assert.NotNil(suite.T(), err, "expected to fail: fc ControllerPublishVolume volume ID format invalid protocol")
}

func (suite *FCControllerSuite) Test_ControllerPublishVolume_MaxVolumeError() {
	ctrPublishValReq := storagecommon.GetISCSIControllerPublishVolumeRequest()
	suite.iboxapi.On("GetHostByName", mock.Anything).Return(storagecommon.GetHostByName(), nil)
	suite.iboxapi.On("GetAllLunByHost", mock.Anything).Return(storagecommon.GetLunInfoArry(), nil)
	suite.iboxapi.On("PutMetadata", mock.Anything, mock.Anything).Return(nil, nil)
	suite.iboxapi.On("CreateHost", mock.Anything).Return(storagecommon.GetHostByName(), nil)
	suite.iboxapi.On("GetSystem", mock.Anything).Return(storagecommon.GetSystem(), nil)
	suite.iboxapi.On("GetVolume", mock.Anything).Return(storagecommon.GetVolume(), nil)
	suite.accessMock.On("IsValidAccessMode", mock.Anything, mock.Anything).Return(true, nil)
	ctrPublishValReq.VolumeContext = map[string]string{common.StorageClassMaxVolsPerHost: "AA"}
	_, err := suite.service.ControllerPublishVolume(context.Background(), ctrPublishValReq)
	assert.NotNil(suite.T(), err, "expected to fail: fc ControllerPublishVolume invalid max_vols_per_host value")
}

func (suite *FCControllerSuite) Test_ControllerPublishVolume_MaxAllowedError() {
	ctrPublishValReq := storagecommon.GetISCSIControllerPublishVolumeRequest()
	suite.iboxapi.On("GetHostByName", mock.Anything).Return(storagecommon.GetHostByName(), nil)
	suite.iboxapi.On("GetAllLunByHost", mock.Anything).Return(storagecommon.GetLunInfoArry(), nil)
	suite.iboxapi.On("PutMetadata", mock.Anything, mock.Anything).Return(nil, nil)
	suite.iboxapi.On("GetSystem", mock.Anything).Return(storagecommon.GetSystem(), nil)
	suite.iboxapi.On("CreateHost", mock.Anything).Return(storagecommon.GetHostByName(), nil)
	suite.iboxapi.On("GetVolume", mock.Anything).Return(storagecommon.GetVolume(), nil)
	suite.accessMock.On("IsValidAccessMode", mock.Anything, mock.Anything).Return(true, nil)
	ctrPublishValReq.VolumeContext = map[string]string{common.StorageClassMaxVolsPerHost: "0"}
	_, err := suite.service.ControllerPublishVolume(context.Background(), ctrPublishValReq)
	assert.NotNil(suite.T(), err, "expected to fail: fc ControllerPublishVolume max_vols_per_host exceeded")
}

func (suite *FCControllerSuite) Test_ControllerUnpublishVolume() {
	deleteHostResponse := &iboxapi.DeleteHostResponse{
		Error: iboxapi.Error{},
	}
	unpublishVolReq := storagecommon.GetISCSIControllerUnpublishVolume()
	suite.iboxapi.On("GetMetadata", mock.Anything).Return(testhelper.GetHostMetadata(), nil)
	suite.iboxapi.On("GetHostByName", mock.Anything).Return(storagecommon.GetHostByName(), nil)
	suite.iboxapi.On("UnMapVolumeFromHost", mock.Anything, mock.Anything).Return(mock.Anything, nil)
	suite.iboxapi.On("GetAllLunByHost", mock.Anything).Return([]api.LunInfo{}, nil)
	suite.iboxapi.On("DeleteHost", mock.Anything).Return(deleteHostResponse, nil)
	_, err := suite.service.ControllerUnpublishVolume(context.Background(), unpublishVolReq)
	assert.Nil(suite.T(), err, "expected to succeed: fc ControllerUnpublishVolume")
}

func (suite *FCControllerSuite) Test_ControllerUnpublishVolume_hostNameErr() {
	unpublishVolReq := storagecommon.GetISCSIControllerUnpublishVolume()
	suite.iboxapi.On("GetMetadata", mock.Anything).Return(testhelper.GetHostMetadata(), nil)
	suite.iboxapi.On("GetHostByName", mock.Anything).Return(nil, suite.someError)
	suite.iboxapi.On("GetAllLunByHost", mock.Anything).Return([]iboxapi.LunInfo{}, nil)
	suite.iboxapi.On("DeleteHost", mock.Anything).Return(nil, suite.someError)
	_, err := suite.service.ControllerUnpublishVolume(context.Background(), unpublishVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: fc ControllerUnpublishVolume GetHostByName")
}

func (suite *FCControllerSuite) Test_ControllerUnpublishVolume_UnMapVolumeErr() {
	unpublishVolReq := storagecommon.GetISCSIControllerUnpublishVolume()
	suite.iboxapi.On("GetMetadata", mock.Anything).Return(testhelper.GetHostMetadata(), nil)
	suite.iboxapi.On("GetHostByName", mock.Anything).Return(storagecommon.GetHostByName(), nil)
	suite.iboxapi.On("GetAllLunByHost", mock.Anything).Return([]iboxapi.LunInfo{}, nil)
	suite.iboxapi.On("DeleteHost", mock.Anything).Return(nil, suite.someError)
	suite.iboxapi.On("UnMapVolumeFromHost", mock.Anything, mock.Anything).Return(nil, suite.someError)
	_, err := suite.service.ControllerUnpublishVolume(context.Background(), unpublishVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: fc ControllerUnpublishVolume UnMapVolumeFromHost")
}

func (suite *FCControllerSuite) Test_ControllerUnpublishVolume_DeleteHostErr() {
	deleteHostResponse := iboxapi.DeleteHostResponse{
		Error: iboxapi.Error{},
	}
	unpublishVolReq := storagecommon.GetISCSIControllerUnpublishVolume()
	suite.iboxapi.On("GetMetadata", mock.Anything).Return(testhelper.GetHostMetadata(), nil)
	suite.iboxapi.On("GetHostByName", mock.Anything).Return(storagecommon.GetHostByName(), nil)
	suite.iboxapi.On("UnMapVolumeFromHost", mock.Anything, mock.Anything).Return(mock.Anything, nil)
	suite.iboxapi.On("GetAllLunByHost", mock.Anything).Return([]iboxapi.LunInfo{}, nil)
	suite.iboxapi.On("DeleteHost", mock.Anything).Return(deleteHostResponse, suite.someError)
	_, err := suite.service.ControllerUnpublishVolume(context.Background(), unpublishVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: fc ControllerUnpublishVolume DeleteHost")
}

func (suite *FCControllerSuite) Test_ControllerUnpublishVolume_MetadataErr() {
	unpublishVolReq := storagecommon.GetISCSIControllerUnpublishVolume()
	suite.iboxapi.On("GetMetadata", mock.Anything).Return(testhelper.GetHostMetadata(), errors.New("some error"))
	suite.iboxapi.On("GetHostByName", mock.Anything).Return(storagecommon.GetHostByName(), nil)
	suite.iboxapi.On("UnMapVolumeFromHost", mock.Anything, mock.Anything).Return(mock.Anything, nil)
	suite.iboxapi.On("GetAllLunByHost", mock.Anything).Return([]iboxapi.LunInfo{}, nil)
	suite.iboxapi.On("DeleteHost", mock.Anything).Return(nil, suite.someError)
	_, err := suite.service.ControllerUnpublishVolume(context.Background(), unpublishVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: fc ControllerUnpublishVolume Metadata Error")
}

func (suite *FCControllerSuite) Test_CreateSnapshot_GetVolumeByNameErr() {
	unpublishVolReq := storagecommon.GetISCSICreateSnapshotRequest()
	suite.iboxapi.On("GetVolume", mock.Anything).Return(storagecommon.GetVolume(), nil)
	suite.iboxapi.On("GetVolumeByName", mock.Anything).Return(nil, suite.someError)
	suite.iboxapi.On("CreateSnapshotVolume", mock.Anything).Return(storagecommon.GetSnapshotResp(), nil)

	_, err := suite.service.CreateSnapshot(context.Background(), unpublishVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: fc CreateSnapshot GetVolumeByName")
}

func (suite *FCControllerSuite) Test_CreateSnapshot_already_Created() {
	suite.service.CS.VolProto.VolumeID = 1001
	unpublishVolReq := storagecommon.GetISCSICreateSnapshotRequest()
	unpublishVolReq.SourceVolumeId = "1001$$iscsi"
	suite.iboxapi.On("GetVolumeByName", mock.Anything).Return(storagecommon.GetVolume(), nil)
	suite.iboxapi.On("CreateSnapshotVolume", mock.Anything).Return(storagecommon.GetSnapshotResp(), nil)

	_, err := suite.service.CreateSnapshot(context.Background(), unpublishVolReq)
	assert.Nil(suite.T(), err, "expected to succeed: fc CreateSnapshot when already exists")
}

func (suite *FCControllerSuite) Test_DeleteSnapshot() {
	ctrdeleteSnapValReq := storagecommon.GetISCSIDeleteSnapshotRequest()
	suite.iboxapi.On("GetVolume", mock.Anything).Return(storagecommon.GetVolume(), nil)
	deleteMetadataResponse := &iboxapi.DeleteMetadataResponse{}
	suite.iboxapi.On("DeleteMetadata", mock.Anything).Return(deleteMetadataResponse, nil)
	suite.iboxapi.On("GetVolumesByParentID", mock.Anything).Return([]iboxapi.Volume{}, nil)
	deleteVolumeResponse := iboxapi.DeleteVolumeResponse{}
	suite.iboxapi.On("DeleteVolume", mock.Anything).Return(deleteVolumeResponse, nil)
	suite.iboxapi.On("GetMetadata", mock.Anything).Return(testhelper.GetHostMetadata(), nil)

	_, err := suite.service.DeleteSnapshot(context.Background(), ctrdeleteSnapValReq)
	assert.Nil(suite.T(), err, "expected to succeed: fc DeleteSnapshot")
}

func (suite *FCControllerSuite) Test_ControllerExpandVolume() {
	ctrExpandValReq := storagecommon.GetISCSIExpandVolumeRequest()
	suite.iboxapi.On("UpdateVolume", mock.Anything, mock.Anything).Return(nil, nil)
	_, err := suite.service.ControllerExpandVolume(context.Background(), ctrExpandValReq)
	assert.Nil(suite.T(), err, "expected to succeed: fc ControllerExpandVolume")
}

func (suite *FCControllerSuite) Test_ListVolumes() {
	_, err := suite.service.ListVolumes(context.Background(), &csi.ListVolumesRequest{})
	assert.Nil(suite.T(), err, "expected to succeed: fc ListVolumes")
}

func (suite *FCControllerSuite) Test_ListSnapshots() {
	_, err := suite.service.ListSnapshots(context.Background(), &csi.ListSnapshotsRequest{})
	assert.Nil(suite.T(), err, "expected to succeed: fc ListSnapshots")
}

func (suite *FCControllerSuite) Test_GetCapacity() {
	_, err := suite.service.GetCapacity(context.Background(), &csi.GetCapacityRequest{})
	assert.Nil(suite.T(), err, "expected to succeed: fc GetCapacity")
}

func getFCCreateVolumeParameter() map[string]string {
	return map[string]string{
		common.StorageClassMaxVolsPerHost:  "19",
		common.StorageClassPoolName:        "pool_name1",
		common.StorageClassProvisionType:   "THIN",
		common.StorageClassSSDEnabled:      "true",
		common.StorageClassStorageProtocol: "fc",
	}
}
