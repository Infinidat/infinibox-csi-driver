//go:build unit

package iscsi

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

func (suite *ISCSIControllerSuite) SetupTest() {
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
	suite.service = ISCSIstorage{CS: *suite.cs}
}

type ISCSIControllerSuite struct {
	suite.Suite
	api        *api.MockAPIService
	iboxapi    *iboxapi.MockAPIService
	accessMock *helper.MockAccessModesHelper
	cs         *storagecommon.Commonservice
	someError  error
	service    ISCSIstorage
}

func TestISCSIControllerSuite(t *testing.T) {
	suite.Run(t, new(ISCSIControllerSuite))
}

func (suite *ISCSIControllerSuite) Test_ValidateStorageClass_InvalidProvisionType_Fail() {
	parameterMap := map[string]string{
		common.StorageClassProvisionType: "somethinginvalid",
	}
	err := suite.service.ValidateStorageClass(parameterMap)
	assert.NotNil(suite.T(), err, "expected to fail: iscsi invalid provision type sc parameter")
}
func (suite *ISCSIControllerSuite) Test_ValidateStorageClass_ValidPoolName() {
	parameterMap := getISCSICreateVolumeParameters()
	err := suite.service.ValidateStorageClass(parameterMap)
	assert.Nil(suite.T(), err, "expected to pass: iscsi valid pool name sc parameter")
}
func (suite *ISCSIControllerSuite) Test_ValidateStorageClass_InvalidPoolName() {
	parameterMap := map[string]string{
		common.StorageClassPoolName: "",
	}
	err := suite.service.ValidateStorageClass(parameterMap)
	assert.NotNil(suite.T(), err, "expected to fail: iscsi validate sc parameters invalid parameter")
}
func (suite *ISCSIControllerSuite) Test_ValidateStorageClass_InvalidParameter_No_Parameters() {
	var parameterMap map[string]string
	err := suite.service.ValidateStorageClass(parameterMap)
	assert.NotNil(suite.T(), err, "expected to fail: iscsi validate sc parameters invalid parameter")
}

func (suite *ISCSIControllerSuite) Test_ValidateStorageClass_InvalidParameter_Missing_CHAP() {
	parameterMap := getISCSICreateVolumeParameters()
	delete(parameterMap, common.StorageClassUseCHAP)
	err := suite.service.ValidateStorageClass(parameterMap)
	assert.NotNil(suite.T(), err, "expected to fail: iscsi validate sc parameters missing parameter")
}
func (suite *ISCSIControllerSuite) Test_ValidateStorageClass_InvalidParameter_Missing_Network_Space() {
	parameterMap := getISCSICreateVolumeParameters()
	delete(parameterMap, common.StorageClassNetworkSpace)
	err := suite.service.ValidateStorageClass(parameterMap)
	assert.NotNil(suite.T(), err, "expected to fail: iscsi validate sc parameters missing parameter")
}
func (suite *ISCSIControllerSuite) Test_ValidateStorageClass_InvalidParameter_Missing_Pool() {
	parameterMap := getISCSICreateVolumeParameters()
	delete(parameterMap, common.StorageClassPoolName)
	err := suite.service.ValidateStorageClass(parameterMap)
	assert.NotNil(suite.T(), err, "expected to fail: iscsi validate sc parameters missing parameter")
}

func (suite *ISCSIControllerSuite) Test_CreateVolume_GetName_fail() {
	parameterMap := getISCSICreateVolumeParameters()
	createVolReq := testhelper.GetCreateVolumeRequest("pvname", parameterMap, "")

	suite.iboxapi.On("GetVolumeByName", mock.Anything).Return(storagecommon.GetVolume(), suite.someError)
	suite.iboxapi.On("GetSystem", mock.Anything).Return(storagecommon.GetSystem(), nil)
	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(storagecommon.GetNetworkspace(), nil)
	suite.api.On("OneTimeValidation", mock.Anything, mock.Anything).Return("", nil)
	_, err := suite.service.CreateVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: iscsi CreateVolume GetVolumeByName")
}

func (suite *ISCSIControllerSuite) Test_CreateVolume_fail() {
	parameterMap := getISCSICreateVolumeParameters()
	createVolReq := testhelper.GetCreateVolumeRequest("pvname", parameterMap, "")

	poolResult := &iboxapi.PoolResult{ID: 10}
	suite.iboxapi.On("GetPoolByName", mock.Anything).Return(poolResult, nil)
	suite.iboxapi.On("GetSystem", mock.Anything).Return(storagecommon.GetSystem(), nil)

	suite.iboxapi.On("GetVolumeByName", mock.Anything).Return(storagecommon.GetVolume(), nil)

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(storagecommon.GetNetworkspace(), nil)
	suite.api.On("CreateVolume", mock.Anything, mock.Anything).Return(nil, suite.someError)
	suite.api.On("OneTimeValidation", mock.Anything, mock.Anything).Return("", nil)

	_, err := suite.service.CreateVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: iscsi CreateVolume")
}

func (suite *ISCSIControllerSuite) Test_CreateVolume_success() {
	suite.service.Capacity = common.BytesInOneGibibyte
	parameterMap := getISCSICreateVolumeParameters()
	createVolReq := testhelper.GetCreateVolumeRequest("pvname", parameterMap, "")

	poolResult := &iboxapi.PoolResult{ID: 10}
	suite.iboxapi.On("GetPoolByName", mock.Anything).Return(poolResult, nil)
	suite.iboxapi.On("GetSystem", mock.Anything).Return(storagecommon.GetSystem(), nil)
	suite.iboxapi.On("GetVolumeByName", mock.Anything).Return(storagecommon.GetVolume(), nil)

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(storagecommon.GetNetworkspace(), nil)
	suite.api.On("CreateVolume", mock.Anything, mock.Anything).Return(storagecommon.GetVolume(), nil)
	suite.iboxapi.On("GetVolume", mock.Anything).Return(storagecommon.GetVolume(), nil)
	suite.api.On("OneTimeValidation", mock.Anything, mock.Anything).Return("", nil)
	suite.iboxapi.On("PutMetadata", mock.Anything, mock.Anything).Return(nil, nil)
	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(storagecommon.GetNetworkspace(), nil)

	_, err := suite.service.CreateVolume(context.Background(), createVolReq)
	assert.Nil(suite.T(), err, "expected to succeed: CreateVolume")
}

func (suite *ISCSIControllerSuite) Test_CreateVolume_metadataError() {
	parameterMap := getISCSICreateVolumeParameters()
	createVolReq := testhelper.GetCreateVolumeRequest("pvname", parameterMap, "")

	poolResult := &iboxapi.PoolResult{ID: 10}
	suite.iboxapi.On("GetSystem", mock.Anything).Return(storagecommon.GetSystem(), nil)
	suite.iboxapi.On("GetPoolByName", mock.Anything).Return(poolResult, nil)

	suite.iboxapi.On("GetVolumeByName", mock.Anything).Return(storagecommon.GetVolume(), nil)

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(storagecommon.GetNetworkspace(), nil)
	suite.api.On("CreateVolume", mock.Anything, mock.Anything).Return(storagecommon.GetVolume(), nil)
	suite.api.On("OneTimeValidation", mock.Anything, mock.Anything).Return("", nil)
	suite.iboxapi.On("GetVolume", mock.Anything).Return(storagecommon.GetVolume(), nil)
	suite.iboxapi.On("PutMetadata", mock.Anything, mock.Anything).Return(nil, suite.someError)

	_, err := suite.service.CreateVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: iscsi CreateVolume attach metadata")
}

func (suite *ISCSIControllerSuite) Test_DeleteVolume_GetVolume_Error() {
	createVolReq := storagecommon.GetDeleteRequest()
	deleteMetadataResponse := &iboxapi.DeleteMetadataResponse{}
	suite.iboxapi.On("DeleteMetadata", mock.Anything).Return(deleteMetadataResponse, nil)
	suite.iboxapi.On("GetVolume", mock.Anything).Return(nil, suite.someError)
	_, err := suite.service.DeleteVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: iscsi DeleteVolume storagecommon.GetVolume")
}

func (suite *ISCSIControllerSuite) Test_DeleteVolume_GetVolumeSnapshot_metadataError() {
	createVolReq := storagecommon.GetDeleteRequest()
	deleteMetadataResponse := &iboxapi.DeleteMetadataResponse{}
	suite.iboxapi.On("DeleteMetadata", mock.Anything).Return(deleteMetadataResponse, nil)
	deleteVolumeResponse := iboxapi.DeleteVolumeResponse{}
	suite.iboxapi.On("DeleteVolume", mock.Anything).Return(deleteVolumeResponse, suite.someError)
	suite.iboxapi.On("GetVolume", mock.Anything).Return(storagecommon.GetVolume(), nil)
	suite.iboxapi.On("GetVolumesByParentID", mock.Anything).Return(storagecommon.GetVolumeArray(), nil)
	suite.iboxapi.On("PutMetadata", mock.Anything, mock.Anything).Return(nil, suite.someError)

	_, err := suite.service.DeleteVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: iscsi DeleteVolume GetVolumeSnapshot attach metadata")
}

func (suite *ISCSIControllerSuite) Test_DeleteVolume_Error() {
	createVolReq := storagecommon.GetDeleteRequest()
	suite.iboxapi.On("GetVolume", mock.Anything).Return(storagecommon.GetVolume(), nil)
	suite.iboxapi.On("GetVolumesByParentID", mock.Anything).Return([]iboxapi.Volume{}, nil)
	deleteMetadataResponse := &iboxapi.DeleteMetadataResponse{}
	suite.iboxapi.On("DeleteMetadata", mock.Anything).Return(deleteMetadataResponse, nil)
	deleteVolumeResponse := iboxapi.DeleteVolumeResponse{}
	suite.iboxapi.On("DeleteVolume", mock.Anything).Return(deleteVolumeResponse, suite.someError)

	_, err := suite.service.DeleteVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: iscsi DeleteVolume delete volume")
}

func (suite *ISCSIControllerSuite) Test_DeleteVolume_success() {
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
	assert.Nil(suite.T(), err, "expected to succeed: iscsi DeleteVolume")
}

func (suite *ISCSIControllerSuite) Test_DeleteVolume_AlreadyDelete() {
	createVolReq := storagecommon.GetDeleteRequest()
	notFoundError := &iboxapi.APIError{Code: iboxapi.RESOURCE_NOT_FOUND, Err: fmt.Errorf("volume not found")}
	suite.iboxapi.On("GetVolume", mock.Anything).Return(nil, notFoundError)
	_, err := suite.service.DeleteVolume(context.Background(), createVolReq)
	assert.Nil(suite.T(), err, "expected to succeed: iscsi DeleteVolume when already deleted")
}

func (suite *ISCSIControllerSuite) Test_CreateVolume_content_success() {
	suite.service.Capacity = common.BytesInOneGibibyte
	parameterMap := getISCSICreateVolumeParameters()
	createVolReq := testhelper.GetCreateVolumeRequest("volumeName", parameterMap, "1$$iscsi")
	poolResult := &iboxapi.PoolResult{ID: 10}
	suite.iboxapi.On("GetPoolByName", mock.Anything).Return(poolResult, nil)
	suite.iboxapi.On("GetSystem", mock.Anything).Return(storagecommon.GetSystem(), nil)
	suite.iboxapi.On("GetVolumeByName", mock.Anything).Return(nil, nil)
	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(storagecommon.GetNetworkspace(), nil)
	suite.iboxapi.On("CreateSnapshotVolume", mock.Anything).Return(storagecommon.GetSnapshotResp(), nil)
	suite.iboxapi.On("GetVolume", mock.Anything).Return(storagecommon.GetVolume(), nil)
	suite.iboxapi.On("PutMetadata", mock.Anything, mock.Anything).Return(nil, nil)
	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(storagecommon.GetNetworkspace(), nil)
	suite.api.On("OneTimeValidation", mock.Anything, mock.Anything).Return("", nil)

	_, err := suite.service.CreateVolume(context.Background(), createVolReq)
	assert.Nil(suite.T(), err, "expected to succeed: iscsi CreateVolume success")
}

func (suite *ISCSIControllerSuite) Test_CreateVolume_content_AttachMetadataToObject_err() {
	parameterMap := getISCSICreateVolumeParameters()
	createVolReq := testhelper.GetCreateVolumeRequest("volumeName", parameterMap, "1$$iscsi")
	suite.iboxapi.On("GetVolumeByName", mock.Anything).Return(nil, nil)
	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(storagecommon.GetNetworkspace(), nil)
	suite.iboxapi.On("GetVolume", mock.Anything).Return(storagecommon.GetVolume(), nil)
	suite.iboxapi.On("GetSystem", mock.Anything).Return(storagecommon.GetSystem(), nil)
	suite.iboxapi.On("CreateSnapshotVolume", mock.Anything).Return(storagecommon.GetSnapshotResp(), nil)
	suite.iboxapi.On("GetVolume", mock.Anything).Return(storagecommon.GetVolume(), nil)
	suite.iboxapi.On("PutMetadata", mock.Anything, mock.Anything).Return(nil, suite.someError)
	suite.api.On("OneTimeValidation", mock.Anything, mock.Anything).Return("", nil)
	_, err := suite.service.CreateVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: iscsi CreateVolume attach metadata")
}

func (suite *ISCSIControllerSuite) Test_ControllerPublishVolume() {
	ctrPublishValReq := storagecommon.GetISCSIControllerPublishVolumeRequest()
	suite.iboxapi.On("GetMetadata", mock.Anything).Return(testhelper.GetHostMetadata(), nil)
	suite.iboxapi.On("PutMetadata", mock.Anything, mock.Anything).Return(nil, nil)
	suite.accessMock.On("IsValidAccessMode", mock.Anything, mock.Anything).Return(true, nil)
	suite.iboxapi.On("GetSystem", mock.Anything).Return(storagecommon.GetSystem(), nil)
	suite.iboxapi.On("CreateHost", mock.Anything).Return(storagecommon.GetHostByName(), nil)
	suite.iboxapi.On("GetHostByName", mock.Anything).Return(storagecommon.GetHostByName(), nil)
	suite.iboxapi.On("GetAllLunByHost", mock.Anything).Return(storagecommon.GetLunInfoArry(), nil)
	lunInfo := storagecommon.GetLunInf()
	suite.iboxapi.On("MapVolumeToHost", mock.Anything, mock.Anything, mock.Anything).Return(&lunInfo, nil)
	suite.iboxapi.On("GetVolume", mock.Anything).Return(storagecommon.GetVolume(), nil)
	_, err := suite.service.ControllerPublishVolume(context.Background(), ctrPublishValReq)
	assert.Nil(suite.T(), err, "expected to succeed: iscsi ControllerPublishVolume")
}

func (suite *ISCSIControllerSuite) Test_ControllerPublishVolume_VolumeIDFormatError() {
	ctrPublishValReq := storagecommon.GetISCSIControllerPublishVolumeRequest()
	ctrPublishValReq.VolumeId = "1$"
	suite.iboxapi.On("GetSystem", mock.Anything).Return(storagecommon.GetSystem(), nil)
	_, err := suite.service.ControllerPublishVolume(context.Background(), ctrPublishValReq)
	assert.NotNil(suite.T(), err, "expected to fail: iscsi ControllerPublishVolume volume ID format invalid protocol")
}

func (suite *ISCSIControllerSuite) Test_ControllerPublishVolume_MaxVolumeError() {
	ctrPublishValReq := storagecommon.GetISCSIControllerPublishVolumeRequest()
	suite.iboxapi.On("GetHostByName", mock.Anything).Return(storagecommon.GetHostByName(), nil)
	suite.iboxapi.On("GetSystem", mock.Anything).Return(storagecommon.GetSystem(), nil)
	suite.iboxapi.On("GetAllLunByHost", mock.Anything).Return(storagecommon.GetLunInfoArry(), nil)
	suite.iboxapi.On("PutMetadata", mock.Anything, mock.Anything).Return(nil, nil)
	suite.iboxapi.On("CreateHost", mock.Anything).Return(storagecommon.GetHostByName(), nil)
	suite.iboxapi.On("GetVolume", mock.Anything).Return(storagecommon.GetVolume(), nil)
	suite.accessMock.On("IsValidAccessMode", mock.Anything, mock.Anything).Return(true, nil)
	ctrPublishValReq.VolumeContext = map[string]string{common.StorageClassMaxVolsPerHost: "AA"}
	_, err := suite.service.ControllerPublishVolume(context.Background(), ctrPublishValReq)
	assert.NotNil(suite.T(), err, "expected to fail: iscsi ControllerPublishVolume invalid max_vols_per_host value")
}

func (suite *ISCSIControllerSuite) Test_ControllerPublishVolume_MaxAllowedError() {
	ctrPublishValReq := storagecommon.GetISCSIControllerPublishVolumeRequest()
	suite.iboxapi.On("GetHostByName", mock.Anything).Return(storagecommon.GetHostByName(), nil)
	suite.iboxapi.On("GetAllLunByHost", mock.Anything).Return(storagecommon.GetLunInfoArry(), nil)
	suite.iboxapi.On("GetSystem", mock.Anything).Return(storagecommon.GetSystem(), nil)
	suite.iboxapi.On("PutMetadata", mock.Anything, mock.Anything).Return(nil, nil)
	suite.iboxapi.On("GetVolume", mock.Anything).Return(storagecommon.GetVolume(), nil)
	suite.iboxapi.On("CreateHost", mock.Anything).Return(storagecommon.GetHostByName(), nil)
	suite.accessMock.On("IsValidAccessMode", mock.Anything, mock.Anything).Return(true, nil)
	ctrPublishValReq.VolumeContext = map[string]string{common.StorageClassMaxVolsPerHost: "0"}
	_, err := suite.service.ControllerPublishVolume(context.Background(), ctrPublishValReq)
	assert.NotNil(suite.T(), err, "expected to fail: iscsi ControllerPublishVolume max_vols_per_host exceeded")
}

func (suite *ISCSIControllerSuite) Test_ControllerUnpublishVolume_success() {
	deleteHostResponse := &iboxapi.DeleteHostResponse{
		Error: iboxapi.Error{},
	}
	ctrUnPublishValReq := storagecommon.GetISCSIControllerUnpublishVolume()
	suite.iboxapi.On("GetMetadata", mock.Anything).Return(testhelper.GetHostMetadata(), nil)
	suite.iboxapi.On("GetHostByName", mock.Anything).Return(storagecommon.GetHostByName(), nil)
	suite.iboxapi.On("UnMapVolumeFromHost", mock.Anything, mock.Anything).Return(mock.Anything, nil)
	suite.iboxapi.On("GetAllLunByHost", mock.Anything).Return([]api.LunInfo{}, nil)
	suite.iboxapi.On("DeleteHost", mock.Anything).Return(deleteHostResponse, nil)
	suite.iboxapi.On("GetVolume", mock.Anything).Return(storagecommon.GetVolume(), nil)
	_, err := suite.service.ControllerUnpublishVolume(context.Background(), ctrUnPublishValReq)
	assert.Nil(suite.T(), err, "expected to succeed: iscsi ControllerUnpublishVolume")
}

func (suite *ISCSIControllerSuite) Test_ControllerUnpublishVolume_UnMapVolumeErr() {
	ctrUnPublishValReq := storagecommon.GetISCSIControllerUnpublishVolume()
	suite.iboxapi.On("GetHostByName", mock.Anything).Return(storagecommon.GetHostByName(), nil)
	suite.iboxapi.On("UnMapVolumeFromHost", mock.Anything, mock.Anything).Return(nil, suite.someError)
	suite.iboxapi.On("GetAllLunByHost", mock.Anything).Return([]api.LunInfo{}, nil)
	suite.iboxapi.On("GetMetadata", mock.Anything).Return(testhelper.GetHostMetadata(), nil)
	suite.iboxapi.On("DeleteHost", mock.Anything).Return(nil, suite.someError)
	_, err := suite.service.ControllerUnpublishVolume(context.Background(), ctrUnPublishValReq)
	assert.NotNil(suite.T(), err, "expected to fail: iscsi ControllerUnpublishVolume UnMapVolumeFromHost")
}

func (suite *ISCSIControllerSuite) Test_ControllerUnpublishVolume_DeleteHostErr() {
	ctrUnPublishValReq := storagecommon.GetISCSIControllerUnpublishVolume()
	suite.iboxapi.On("GetMetadata", mock.Anything).Return(testhelper.GetHostMetadata(), nil)
	suite.iboxapi.On("GetHostByName", mock.Anything).Return(storagecommon.GetHostByName(), nil)
	suite.iboxapi.On("UnMapVolumeFromHost", mock.Anything, mock.Anything).Return(mock.Anything, nil)
	suite.iboxapi.On("GetAllLunByHost", mock.Anything).Return([]api.LunInfo{}, nil)
	suite.iboxapi.On("DeleteHost", mock.Anything).Return(nil, suite.someError)
	_, err := suite.service.ControllerUnpublishVolume(context.Background(), ctrUnPublishValReq)
	assert.NotNil(suite.T(), err, "expected to fail: iscsi ControllerUnpublishVolume DeleteHost")
}

func (suite *ISCSIControllerSuite) Test_ControllerUnpublishVolume_Metadata_Error() {
	ctrUnPublishValReq := storagecommon.GetISCSIControllerUnpublishVolume()
	suite.iboxapi.On("GetMetadata", mock.Anything).Return(testhelper.GetHostMetadata(), errors.New("some error"))
	suite.iboxapi.On("GetHostByName", mock.Anything).Return(storagecommon.GetHostByName(), nil)
	suite.iboxapi.On("UnMapVolumeFromHost", mock.Anything, mock.Anything).Return(mock.Anything, nil)
	suite.iboxapi.On("GetAllLunByHost", mock.Anything).Return([]api.LunInfo{}, nil)
	suite.iboxapi.On("DeleteHost", mock.Anything).Return(nil, suite.someError)
	_, err := suite.service.ControllerUnpublishVolume(context.Background(), ctrUnPublishValReq)
	assert.NotNil(suite.T(), err, "expected to fail: iscsi ControllerUnpublishVolume Metadata Error")
}

func (suite *ISCSIControllerSuite) Test_CreateSnapshot() {
	ctrUnPublishValReq := storagecommon.GetISCSICreateSnapshotRequest()
	suite.iboxapi.On("GetVolume", mock.Anything).Return(storagecommon.GetVolume(), nil)
	suite.iboxapi.On("GetVolumeByName", mock.Anything).Return(storagecommon.GetVolume(), suite.someError)
	suite.iboxapi.On("CreateSnapshotVolume", mock.Anything).Return(storagecommon.GetSnapshotResp(), nil)

	_, err := suite.service.CreateSnapshot(context.Background(), ctrUnPublishValReq)
	assert.NotNil(suite.T(), err, "expected to fail: iscsi CreateSnapshot GetVolumeByName")
}

func (suite *ISCSIControllerSuite) Test_CreateSnapshot_already_Created() {
	suite.cs.VolProto.VolumeID = 1001
	ctrUnPublishValReq := storagecommon.GetISCSICreateSnapshotRequest()
	ctrUnPublishValReq.SourceVolumeId = "1001$$iscsi"
	suite.iboxapi.On("GetVolumeByName", mock.Anything).Return(storagecommon.GetVolume(), nil)
	suite.iboxapi.On("CreateSnapshotVolume", mock.Anything).Return(storagecommon.GetSnapshotResp(), nil)

	_, err := suite.service.CreateSnapshot(context.Background(), ctrUnPublishValReq)
	assert.Nil(suite.T(), err, "expected to succeed: iscsi CreateSnapshot")
}

func (suite *ISCSIControllerSuite) Test_DeleteSnapshot() {
	ctrdeleteSnapValReq := storagecommon.GetISCSIDeleteSnapshotRequest()
	suite.iboxapi.On("GetVolume", mock.Anything).Return(storagecommon.GetVolume(), nil)
	suite.iboxapi.On("GetVolumesByParentID", mock.Anything).Return([]iboxapi.Volume{}, nil)
	suite.iboxapi.On("GetMetadata", mock.Anything).Return(testhelper.GetHostMetadata(), nil)
	deleteMetadataResponse := &iboxapi.DeleteMetadataResponse{}
	suite.iboxapi.On("DeleteMetadata", mock.Anything).Return(deleteMetadataResponse, nil)
	deleteVolumeResponse := iboxapi.DeleteVolumeResponse{}
	suite.iboxapi.On("DeleteVolume", mock.Anything).Return(deleteVolumeResponse, nil)
	suite.api.On("GetMetadataStatus", mock.Anything).Return(false)

	_, err := suite.service.DeleteSnapshot(context.Background(), ctrdeleteSnapValReq)
	assert.Nil(suite.T(), err, "expected to succeed: iscsi DeleteSnapshot")
}

func (suite *ISCSIControllerSuite) Test_ControllerExpandVolume() {
	ctrExpandValReq := storagecommon.GetISCSIExpandVolumeRequest()
	suite.iboxapi.On("UpdateVolume", mock.Anything, mock.Anything).Return(nil, nil)
	_, err := suite.service.ControllerExpandVolume(context.Background(), ctrExpandValReq)
	assert.Nil(suite.T(), err, "expected to succeed: iscsi ControllerExpandVolume")
}

func (suite *ISCSIControllerSuite) Test_ListVolumes() {
	_, err := suite.service.ListVolumes(context.Background(), &csi.ListVolumesRequest{})
	assert.Nil(suite.T(), err, "expected to succeed: iscsi ListVolumes")
}

func (suite *ISCSIControllerSuite) Test_ListSnapshots() {
	_, err := suite.service.ListSnapshots(context.Background(), &csi.ListSnapshotsRequest{})
	assert.Nil(suite.T(), err, "expected to succeed: iscsi ListSnapshots")
}

func (suite *ISCSIControllerSuite) Test_GetCapacity() {
	_, err := suite.service.GetCapacity(context.Background(), &csi.GetCapacityRequest{})
	assert.Nil(suite.T(), err, "expected to succeed: iscsi GetCapacity")
}

func getISCSICreateVolumeParameters() map[string]string {
	return map[string]string{
		common.StorageClassGID:             "2468",
		common.StorageClassMaxVolsPerHost:  "19",
		common.StorageClassNetworkSpace:    "network_space1",
		common.StorageClassPoolName:        "pool_name1",
		common.StorageClassProvisionType:   common.StorageClassThinProvision,
		common.StorageClassSSDEnabled:      "true",
		common.StorageClassStorageProtocol: "iscsi",
		common.StorageClassUID:             "1234",
		common.StorageClassUNIXPermissions: "0777",
		common.StorageClassUseCHAP:         "none",
	}
}
