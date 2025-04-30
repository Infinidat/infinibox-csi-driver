//go:build unit

package storage

import (
	"context"
	"errors"
	"fmt"
	"infinibox-csi-driver/api"
	"infinibox-csi-driver/common"
	"infinibox-csi-driver/helper"
	"infinibox-csi-driver/iboxapi"
	"infinibox-csi-driver/test_helper"
	tests "infinibox-csi-driver/test_helper"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

func (suite *ISCSIControllerSuite) SetupTest() {
	suite.api = new(api.MockApiService)
	suite.iboxapi = new(iboxapi.MockApiService)
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
	suite.cs = &Commonservice{Api: suite.api, AccessModesHelper: suite.accessMock, IboxApi: suite.iboxapi, VolProto: volProto}
	suite.someError = errors.New("some Error")
	suite.service = iscsistorage{cs: *suite.cs}
}

type ISCSIControllerSuite struct {
	suite.Suite
	api        *api.MockApiService
	iboxapi    *iboxapi.MockApiService
	accessMock *helper.MockAccessModesHelper
	cs         *Commonservice
	someError  error
	service    iscsistorage
}

func TestISCSIControllerSuite(t *testing.T) {
	suite.Run(t, new(ISCSIControllerSuite))
}

func (suite *ISCSIControllerSuite) Test_ValidateStorageClass_InvalidProvisionType_Fail() {
	parameterMap := map[string]string{
		common.SC_PROVISION_TYPE: "somethinginvalid",
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
		common.SC_POOL_NAME: "",
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
	delete(parameterMap, common.SC_USE_CHAP)
	err := suite.service.ValidateStorageClass(parameterMap)
	assert.NotNil(suite.T(), err, "expected to fail: iscsi validate sc parameters missing parameter")
}
func (suite *ISCSIControllerSuite) Test_ValidateStorageClass_InvalidParameter_Missing_Network_Space() {
	parameterMap := getISCSICreateVolumeParameters()
	delete(parameterMap, common.SC_NETWORK_SPACE)
	err := suite.service.ValidateStorageClass(parameterMap)
	assert.NotNil(suite.T(), err, "expected to fail: iscsi validate sc parameters missing parameter")
}
func (suite *ISCSIControllerSuite) Test_ValidateStorageClass_InvalidParameter_Missing_Pool() {
	parameterMap := getISCSICreateVolumeParameters()
	delete(parameterMap, common.SC_POOL_NAME)
	err := suite.service.ValidateStorageClass(parameterMap)
	assert.NotNil(suite.T(), err, "expected to fail: iscsi validate sc parameters missing parameter")
}

func (suite *ISCSIControllerSuite) Test_CreateVolume_GetName_fail() {
	parameterMap := getISCSICreateVolumeParameters()
	createVolReq := tests.GetCreateVolumeRequest("pvname", parameterMap, "")

	suite.iboxapi.On("GetVolumeByName", mock.Anything).Return(getVolume(), suite.someError)
	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkspace(), nil)
	suite.api.On("OneTimeValidation", mock.Anything, mock.Anything).Return("", nil)
	_, err := suite.service.CreateVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: iscsi CreateVolume GetVolumeByName")
}

func (suite *ISCSIControllerSuite) Test_CreateVolume_fail() {
	parameterMap := getISCSICreateVolumeParameters()
	createVolReq := tests.GetCreateVolumeRequest("pvname", parameterMap, "")

	poolResult := &iboxapi.PoolResult{ID: 10}
	suite.iboxapi.On("GetPoolByName", mock.Anything).Return(poolResult, nil)

	suite.iboxapi.On("GetVolumeByName", mock.Anything).Return(getVolume(), nil)

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkspace(), nil)
	suite.api.On("CreateVolume", mock.Anything, mock.Anything).Return(nil, suite.someError)
	suite.api.On("OneTimeValidation", mock.Anything, mock.Anything).Return("", nil)

	_, err := suite.service.CreateVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: iscsi CreateVolume")
}

func (suite *ISCSIControllerSuite) Test_CreateVolume_success() {
	suite.service.capacity = common.BytesInOneGibibyte
	parameterMap := getISCSICreateVolumeParameters()
	createVolReq := tests.GetCreateVolumeRequest("pvname", parameterMap, "")

	poolResult := &iboxapi.PoolResult{ID: 10}
	suite.iboxapi.On("GetPoolByName", mock.Anything).Return(poolResult, nil)
	suite.iboxapi.On("GetVolumeByName", mock.Anything).Return(getVolume(), nil)

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkspace(), nil)
	suite.api.On("CreateVolume", mock.Anything, mock.Anything).Return(getVolume(), nil)
	suite.iboxapi.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	suite.api.On("OneTimeValidation", mock.Anything, mock.Anything).Return("", nil)
	suite.iboxapi.On("PutMetadata", mock.Anything, mock.Anything).Return(nil, nil)
	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkspace(), nil)

	_, err := suite.service.CreateVolume(context.Background(), createVolReq)
	assert.Nil(suite.T(), err, "expected to succeed: CreateVolume")
}

func (suite *ISCSIControllerSuite) Test_CreateVolume_metadataError() {
	parameterMap := getISCSICreateVolumeParameters()
	createVolReq := tests.GetCreateVolumeRequest("pvname", parameterMap, "")

	poolResult := &iboxapi.PoolResult{ID: 10}
	suite.iboxapi.On("GetPoolByName", mock.Anything).Return(poolResult, nil)

	suite.iboxapi.On("GetVolumeByName", mock.Anything).Return(getVolume(), nil)

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkspace(), nil)
	suite.api.On("CreateVolume", mock.Anything, mock.Anything).Return(getVolume(), nil)
	suite.api.On("OneTimeValidation", mock.Anything, mock.Anything).Return("", nil)
	suite.iboxapi.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	suite.iboxapi.On("PutMetadata", mock.Anything, mock.Anything).Return(nil, suite.someError)

	_, err := suite.service.CreateVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: iscsi CreateVolume attach metadata")
}

func (suite *ISCSIControllerSuite) Test_DeleteVolume_GetVolume_Error() {
	createVolReq := getDeleteRequest()
	deleteMetadataResponse := &iboxapi.DeleteMetadataResponse{}
	suite.iboxapi.On("DeleteMetadata", mock.Anything).Return(deleteMetadataResponse, nil)
	suite.iboxapi.On("GetVolume", mock.Anything).Return(nil, suite.someError)
	_, err := suite.service.DeleteVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: iscsi DeleteVolume getVolume")
}

func (suite *ISCSIControllerSuite) Test_DeleteVolume_GetVolumeSnapshot_metadataError() {
	createVolReq := getDeleteRequest()
	deleteMetadataResponse := &iboxapi.DeleteMetadataResponse{}
	suite.iboxapi.On("DeleteMetadata", mock.Anything).Return(deleteMetadataResponse, nil)
	deleteVolumeResponse := iboxapi.DeleteVolumeResponse{}
	suite.iboxapi.On("DeleteVolume", mock.Anything).Return(deleteVolumeResponse, suite.someError)
	suite.iboxapi.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	suite.iboxapi.On("GetVolumesByParentID", mock.Anything).Return(getVolumeArray(), nil)
	suite.iboxapi.On("PutMetadata", mock.Anything, mock.Anything).Return(nil, suite.someError)

	_, err := suite.service.DeleteVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: iscsi DeleteVolume GetVolumeSnapshot attach metadata")
}

func (suite *ISCSIControllerSuite) Test_DeleteVolume_Error() {
	createVolReq := getDeleteRequest()
	suite.iboxapi.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	suite.iboxapi.On("GetVolumesByParentID", mock.Anything).Return([]iboxapi.Volume{}, nil)
	deleteMetadataResponse := &iboxapi.DeleteMetadataResponse{}
	suite.iboxapi.On("DeleteMetadata", mock.Anything).Return(deleteMetadataResponse, nil)
	deleteVolumeResponse := iboxapi.DeleteVolumeResponse{}
	suite.iboxapi.On("DeleteVolume", mock.Anything).Return(deleteVolumeResponse, suite.someError)

	_, err := suite.service.DeleteVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: iscsi DeleteVolume delete volume")
}

func (suite *ISCSIControllerSuite) Test_DeleteVolume_success() {
	createVolReq := getDeleteRequest()
	suite.iboxapi.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	deleteMetadataResponse := &iboxapi.DeleteMetadataResponse{}
	suite.iboxapi.On("DeleteMetadata", mock.Anything).Return(deleteMetadataResponse, nil)
	suite.iboxapi.On("GetVolumesByParentID", mock.Anything).Return([]iboxapi.Volume{}, nil)
	suite.iboxapi.On("GetMetadata", mock.Anything).Return(test_helper.GetHostMetadata(), nil)
	deleteVolumeResponse := iboxapi.DeleteVolumeResponse{}
	suite.iboxapi.On("DeleteVolume", mock.Anything).Return(deleteVolumeResponse, nil)
	suite.api.On("GetMetadataStatus", mock.Anything).Return(false)
	_, err := suite.service.DeleteVolume(context.Background(), createVolReq)
	assert.Nil(suite.T(), err, "expected to succeed: iscsi DeleteVolume")
}

func (suite *ISCSIControllerSuite) Test_DeleteVolume_AlreadyDelete() {
	createVolReq := getDeleteRequest()
	notFoundError := &iboxapi.IboxAPIError{Code: iboxapi.IBOXAPI_RESOURCE_NOT_FOUND_ERROR, Err: fmt.Errorf("volume not found")}
	suite.iboxapi.On("GetVolume", mock.Anything).Return(nil, notFoundError)
	_, err := suite.service.DeleteVolume(context.Background(), createVolReq)
	assert.Nil(suite.T(), err, "expected to succeed: iscsi DeleteVolume when already deleted")
}

func (suite *ISCSIControllerSuite) Test_CreateVolume_content_success() {
	suite.service.capacity = common.BytesInOneGibibyte
	parameterMap := getISCSICreateVolumeParameters()
	createVolReq := tests.GetCreateVolumeRequest("volumeName", parameterMap, "1$$iscsi")
	poolResult := &iboxapi.PoolResult{ID: 10}
	suite.iboxapi.On("GetPoolByName", mock.Anything).Return(poolResult, nil)
	suite.iboxapi.On("GetVolumeByName", mock.Anything).Return(nil, nil)
	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkspace(), nil)
	suite.iboxapi.On("CreateSnapshotVolume", mock.Anything).Return(getSnapshotResp(), nil)
	suite.iboxapi.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	suite.iboxapi.On("PutMetadata", mock.Anything, mock.Anything).Return(nil, nil)
	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkspace(), nil)
	suite.api.On("OneTimeValidation", mock.Anything, mock.Anything).Return("", nil)

	_, err := suite.service.CreateVolume(context.Background(), createVolReq)
	assert.Nil(suite.T(), err, "expected to succeed: iscsi CreateVolume success")
}

func (suite *ISCSIControllerSuite) Test_CreateVolume_content_AttachMetadataToObject_err() {
	parameterMap := getISCSICreateVolumeParameters()
	createVolReq := tests.GetCreateVolumeRequest("volumeName", parameterMap, "1$$iscsi")
	suite.iboxapi.On("GetVolumeByName", mock.Anything).Return(nil, nil)
	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkspace(), nil)
	suite.iboxapi.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	suite.iboxapi.On("CreateSnapshotVolume", mock.Anything).Return(getSnapshotResp(), nil)
	suite.iboxapi.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	suite.iboxapi.On("PutMetadata", mock.Anything, mock.Anything).Return(nil, suite.someError)
	suite.api.On("OneTimeValidation", mock.Anything, mock.Anything).Return("", nil)
	_, err := suite.service.CreateVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: iscsi CreateVolume attach metadata")
}

func (suite *ISCSIControllerSuite) Test_ControllerPublishVolume() {
	ctrPublishValReq := getISCSIControllerPublishVolumeRequest()
	suite.iboxapi.On("GetMetadata", mock.Anything).Return(test_helper.GetHostMetadata(), nil)
	suite.iboxapi.On("PutMetadata", mock.Anything, mock.Anything).Return(nil, nil)
	suite.accessMock.On("IsValidAccessMode", mock.Anything, mock.Anything).Return(true, nil)
	suite.iboxapi.On("CreateHost", mock.Anything).Return(getHostByName(), nil)
	suite.iboxapi.On("GetHostByName", mock.Anything).Return(getHostByName(), nil)
	suite.iboxapi.On("GetAllLunByHost", mock.Anything).Return(getLunInfoArry(), nil)
	lunInfo := getLunInf()
	suite.iboxapi.On("MapVolumeToHost", mock.Anything, mock.Anything, mock.Anything).Return(&lunInfo, nil)
	suite.iboxapi.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	_, err := suite.service.ControllerPublishVolume(context.Background(), ctrPublishValReq)
	assert.Nil(suite.T(), err, "expected to succeed: iscsi ControllerPublishVolume")
}

func (suite *ISCSIControllerSuite) Test_ControllerPublishVolume_VolumeIDFormatError() {
	ctrPublishValReq := getISCSIControllerPublishVolumeRequest()
	ctrPublishValReq.VolumeId = "1$"
	_, err := suite.service.ControllerPublishVolume(context.Background(), ctrPublishValReq)
	assert.NotNil(suite.T(), err, "expected to fail: iscsi ControllerPublishVolume volume ID format invalid protocol")
}

func (suite *ISCSIControllerSuite) Test_ControllerPublishVolume_MaxVolumeError() {
	ctrPublishValReq := getISCSIControllerPublishVolumeRequest()
	suite.iboxapi.On("GetHostByName", mock.Anything).Return(getHostByName(), nil)
	suite.iboxapi.On("GetAllLunByHost", mock.Anything).Return(getLunInfoArry(), nil)
	suite.iboxapi.On("PutMetadata", mock.Anything, mock.Anything).Return(nil, nil)
	suite.iboxapi.On("CreateHost", mock.Anything).Return(getHostByName(), nil)
	suite.iboxapi.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	suite.accessMock.On("IsValidAccessMode", mock.Anything, mock.Anything).Return(true, nil)
	ctrPublishValReq.VolumeContext = map[string]string{common.SC_MAX_VOLS_PER_HOST: "AA"}
	_, err := suite.service.ControllerPublishVolume(context.Background(), ctrPublishValReq)
	assert.NotNil(suite.T(), err, "expected to fail: iscsi ControllerPublishVolume invalid max_vols_per_host value")
}

func (suite *ISCSIControllerSuite) Test_ControllerPublishVolume_MaxAllowedError() {
	ctrPublishValReq := getISCSIControllerPublishVolumeRequest()
	suite.iboxapi.On("GetHostByName", mock.Anything).Return(getHostByName(), nil)
	suite.iboxapi.On("GetAllLunByHost", mock.Anything).Return(getLunInfoArry(), nil)
	suite.iboxapi.On("PutMetadata", mock.Anything, mock.Anything).Return(nil, nil)
	suite.iboxapi.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	suite.iboxapi.On("CreateHost", mock.Anything).Return(getHostByName(), nil)
	suite.accessMock.On("IsValidAccessMode", mock.Anything, mock.Anything).Return(true, nil)
	ctrPublishValReq.VolumeContext = map[string]string{common.SC_MAX_VOLS_PER_HOST: "0"}
	_, err := suite.service.ControllerPublishVolume(context.Background(), ctrPublishValReq)
	assert.NotNil(suite.T(), err, "expected to fail: iscsi ControllerPublishVolume max_vols_per_host exceeded")
}

func (suite *ISCSIControllerSuite) Test_ControllerUnpublishVolume_success() {
	deleteHostResponse := &iboxapi.DeleteHostResponse{
		Error: iboxapi.Error{},
	}
	ctrUnPublishValReq := getISCSIControllerUnpublishVolume()
	suite.iboxapi.On("GetMetadata", mock.Anything).Return(test_helper.GetHostMetadata(), nil)
	suite.iboxapi.On("GetHostByName", mock.Anything).Return(getHostByName(), nil)
	suite.iboxapi.On("UnMapVolumeFromHost", mock.Anything, mock.Anything).Return(mock.Anything, nil)
	suite.iboxapi.On("GetAllLunByHost", mock.Anything).Return([]api.LunInfo{}, nil)
	suite.iboxapi.On("DeleteHost", mock.Anything).Return(deleteHostResponse, nil)
	suite.iboxapi.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	_, err := suite.service.ControllerUnpublishVolume(context.Background(), ctrUnPublishValReq)
	assert.Nil(suite.T(), err, "expected to succeed: iscsi ControllerUnpublishVolume")
}

func (suite *ISCSIControllerSuite) Test_ControllerUnpublishVolume_UnMapVolumeErr() {
	ctrUnPublishValReq := getISCSIControllerUnpublishVolume()
	suite.iboxapi.On("GetHostByName", mock.Anything).Return(getHostByName(), nil)
	suite.iboxapi.On("UnMapVolumeFromHost", mock.Anything, mock.Anything).Return(nil, suite.someError)
	suite.iboxapi.On("GetAllLunByHost", mock.Anything).Return([]api.LunInfo{}, nil)
	suite.iboxapi.On("GetMetadata", mock.Anything).Return(test_helper.GetHostMetadata(), nil)
	suite.iboxapi.On("DeleteHost", mock.Anything).Return(nil, suite.someError)
	_, err := suite.service.ControllerUnpublishVolume(context.Background(), ctrUnPublishValReq)
	assert.NotNil(suite.T(), err, "expected to fail: iscsi ControllerUnpublishVolume UnMapVolumeFromHost")
}

func (suite *ISCSIControllerSuite) Test_ControllerUnpublishVolume_DeleteHostErr() {
	ctrUnPublishValReq := getISCSIControllerUnpublishVolume()
	suite.iboxapi.On("GetMetadata", mock.Anything).Return(test_helper.GetHostMetadata(), nil)
	suite.iboxapi.On("GetHostByName", mock.Anything).Return(getHostByName(), nil)
	suite.iboxapi.On("UnMapVolumeFromHost", mock.Anything, mock.Anything).Return(mock.Anything, nil)
	suite.iboxapi.On("GetAllLunByHost", mock.Anything).Return([]api.LunInfo{}, nil)
	suite.iboxapi.On("DeleteHost", mock.Anything).Return(nil, suite.someError)
	_, err := suite.service.ControllerUnpublishVolume(context.Background(), ctrUnPublishValReq)
	assert.NotNil(suite.T(), err, "expected to fail: iscsi ControllerUnpublishVolume DeleteHost")
}

func (suite *ISCSIControllerSuite) Test_ControllerUnpublishVolume_Metadata_Error() {
	ctrUnPublishValReq := getISCSIControllerUnpublishVolume()
	suite.iboxapi.On("GetMetadata", mock.Anything).Return(test_helper.GetHostMetadata(), errors.New("some error"))
	suite.iboxapi.On("GetHostByName", mock.Anything).Return(getHostByName(), nil)
	suite.iboxapi.On("UnMapVolumeFromHost", mock.Anything, mock.Anything).Return(mock.Anything, nil)
	suite.iboxapi.On("GetAllLunByHost", mock.Anything).Return([]api.LunInfo{}, nil)
	suite.iboxapi.On("DeleteHost", mock.Anything).Return(nil, suite.someError)
	_, err := suite.service.ControllerUnpublishVolume(context.Background(), ctrUnPublishValReq)
	assert.NotNil(suite.T(), err, "expected to fail: iscsi ControllerUnpublishVolume Metadata Error")
}

func (suite *ISCSIControllerSuite) Test_CreateSnapshot() {
	ctrUnPublishValReq := getISCSICreateSnapshotRequest()
	suite.iboxapi.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	suite.iboxapi.On("GetVolumeByName", mock.Anything).Return(getVolume(), suite.someError)
	suite.iboxapi.On("CreateSnapshotVolume", mock.Anything).Return(getSnapshotResp(), nil)

	_, err := suite.service.CreateSnapshot(context.Background(), ctrUnPublishValReq)
	assert.NotNil(suite.T(), err, "expected to fail: iscsi CreateSnapshot GetVolumeByName")
}

func (suite *ISCSIControllerSuite) Test_CreateSnapshot_already_Created() {
	suite.cs.VolProto.VolumeID = 1001
	ctrUnPublishValReq := getISCSICreateSnapshotRequest()
	ctrUnPublishValReq.SourceVolumeId = "1001$$iscsi"
	suite.iboxapi.On("GetVolumeByName", mock.Anything).Return(getVolume(), nil)
	suite.iboxapi.On("CreateSnapshotVolume", mock.Anything).Return(getSnapshotResp(), nil)

	_, err := suite.service.CreateSnapshot(context.Background(), ctrUnPublishValReq)
	assert.Nil(suite.T(), err, "expected to succeed: iscsi CreateSnapshot")
}

func (suite *ISCSIControllerSuite) Test_DeleteSnapshot() {
	ctrdeleteSnapValReq := getISCSIDeleteSnapshotRequest()
	suite.iboxapi.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	suite.iboxapi.On("GetVolumesByParentID", mock.Anything).Return([]iboxapi.Volume{}, nil)
	suite.iboxapi.On("GetMetadata", mock.Anything).Return(test_helper.GetHostMetadata(), nil)
	deleteMetadataResponse := &iboxapi.DeleteMetadataResponse{}
	suite.iboxapi.On("DeleteMetadata", mock.Anything).Return(deleteMetadataResponse, nil)
	deleteVolumeResponse := iboxapi.DeleteVolumeResponse{}
	suite.iboxapi.On("DeleteVolume", mock.Anything).Return(deleteVolumeResponse, nil)
	suite.api.On("GetMetadataStatus", mock.Anything).Return(false)

	_, err := suite.service.DeleteSnapshot(context.Background(), ctrdeleteSnapValReq)
	assert.Nil(suite.T(), err, "expected to succeed: iscsi DeleteSnapshot")
}

func (suite *ISCSIControllerSuite) Test_ControllerExpandVolume() {
	ctrExpandValReq := getISCSIExpandVolumeRequest()
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

func getISCSIExpandVolumeRequest() *csi.ControllerExpandVolumeRequest {
	return &csi.ControllerExpandVolumeRequest{
		VolumeId: "1",
	}
}

func getISCSIDeleteSnapshotRequest() *csi.DeleteSnapshotRequest {
	return &csi.DeleteSnapshotRequest{
		SnapshotId: "1$$iscsi",
	}
}

func getISCSICreateSnapshotRequest() *csi.CreateSnapshotRequest {
	return &csi.CreateSnapshotRequest{
		SourceVolumeId: "1$$iscsi",
		Name:           "snapshotName",
	}
}

func getISCSIControllerUnpublishVolume() *csi.ControllerUnpublishVolumeRequest {
	return &csi.ControllerUnpublishVolumeRequest{
		VolumeId: "1$$nfs",
		NodeId:   "10.20.20.50$$iscsi",
	}
}

func getLunInf() iboxapi.LunInfo {
	luninfo := iboxapi.LunInfo{
		HostID: 100,
		ID:     1,
	}
	return luninfo
}

func getLunInfoArry() []iboxapi.LunInfo {
	var lunInfoArry []iboxapi.LunInfo
	lunInfoArry = append(lunInfoArry, getLunInf())
	return lunInfoArry
}

func getHostByName() *iboxapi.Host {
	var host iboxapi.Host
	host.ID = 10
	host.Name = "hostName"
	lunInfoArry := getLunInfoArry()
	host.Luns = append(host.Luns, lunInfoArry...)
	var hostportArr []iboxapi.Ports
	var hostport iboxapi.Ports
	hostport.HostID = 10
	hostport.Address = "10.20.20.50"
	hostport.Type = "ISCSI"
	hostportArr = append(hostportArr, hostport)
	host.Ports = append(host.Ports, hostportArr...)
	return &host
}

func getISCSIControllerPublishVolumeRequest() *csi.ControllerPublishVolumeRequest {
	return &csi.ControllerPublishVolumeRequest{
		VolumeId:      "1$$iscsi",
		NodeId:        "10.20.20.50$$iscsi",
		VolumeContext: map[string]string{common.SC_MAX_VOLS_PER_HOST: "10"},
	}
}

func getSnapshotResp() *iboxapi.Snapshot {
	snap := &iboxapi.Snapshot{
		Name:       "snaName",
		SnapShotID: 1000,
		PoolID:     10,
	}
	return snap
}

func getVolumeArray() []iboxapi.Volume {
	var volArry []iboxapi.Volume
	vol := getVolume()
	volArry = append(volArry, *vol)
	return volArry
}

func getDeleteRequest() *csi.DeleteVolumeRequest {
	return &csi.DeleteVolumeRequest{
		VolumeId: "103",
	}
}

func getVolume() *iboxapi.Volume {
	vol := iboxapi.Volume{
		ID:       100,
		PoolId:   10,
		ParentId: 1001,
		Name:     "volName",
		PoolName: "poolName",
		Size:     common.BytesInOneGibibyte,
	}
	return &vol
}
func getIboxapiCreateVolumeResponse() *iboxapi.Volume {
	vol := iboxapi.Volume{
		ID:       100,
		PoolId:   10,
		ParentId: 1001,
		Name:     "volName",
		PoolName: "poolName",
		Size:     common.BytesInOneGibibyte,
	}
	return &vol
}

func getNetworkspace() api.NetworkSpace {
	var nspace api.NetworkSpace
	var pArry []api.Portal
	p := api.Portal{
		Enabled:     true,
		InterfaceID: 1,
		IpAdress:    "10.20.30.40",
		Reserved:    false,
		Tpgt:        100,
		Type:        "",
		VlanID:      100,
	}

	var netProp api.NetworkSpaceProperty
	netProp.IscsiIqn = "iqn.1991-05.com.infinidate:example"

	nspace.Properties = netProp
	pArry = append(pArry, p)
	nspace.Portals = append(nspace.Portals, pArry...)
	nspace.Service = common.NS_ISCSI_SVC
	return nspace
}

func getISCSIValidateVolumeCapabilitiesRequest(parameterMap map[string]string) *csi.ValidateVolumeCapabilitiesRequest {
	capa := csi.VolumeCapability{
		AccessMode: &csi.VolumeCapability_AccessMode{
			Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
		},
	}
	var arr []*csi.VolumeCapability
	arr = append(arr, &capa)
	return &csi.ValidateVolumeCapabilitiesRequest{
		VolumeId:           "1$$iscsi",
		Parameters:         parameterMap,
		VolumeCapabilities: arr,
	}
}

func getISCSICreateVolumeParameters() map[string]string {
	return map[string]string{
		common.SC_GID:               "2468",
		common.SC_MAX_VOLS_PER_HOST: "19",
		common.SC_NETWORK_SPACE:     "network_space1",
		common.SC_POOL_NAME:         "pool_name1",
		common.SC_PROVISION_TYPE:    common.SC_THIN_PROVISION_TYPE,
		common.SC_SSD_ENABLED:       "true",
		common.SC_STORAGE_PROTOCOL:  "iscsi",
		common.SC_UID:               "1234",
		common.SC_UNIX_PERMISSIONS:  "0777",
		common.SC_USE_CHAP:          "none",
	}
}
