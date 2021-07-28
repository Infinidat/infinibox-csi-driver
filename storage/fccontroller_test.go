package storage

import (
	"context"
	"errors"
	"infinibox-csi-driver/api"
	"infinibox-csi-driver/helper"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

func (suite *FCControllerSuite) SetupTest() {
	suite.api = new(api.MockApiService)
	suite.accessMock = new(helper.MockAccessModesHelper)
	suite.cs = &commonservice{api: suite.api, accessModesHelper: suite.accessMock}
}

type FCControllerSuite struct {
	suite.Suite
	api        *api.MockApiService
	accessMock *helper.MockAccessModesHelper
	cs         *commonservice
}

func TestFCControllerSuite(t *testing.T) {
	suite.Run(t, new(FCControllerSuite))
}

func (suite *FCControllerSuite) Test_CreateVolume_InvalidParameter_Fail() {
	service := fcstorage{cs: *suite.cs}
	var parameterMap map[string]string
	crtValReq := getISCSICreateValumeRequest("", parameterMap)
	_, err := service.CreateVolume(context.Background(), crtValReq)
	assert.NotNil(suite.T(), err, "Fail to validate parameter for fc protocol")
}

func (suite *FCControllerSuite) Test_CreateVolume_InvalidParameter_Fail2() {
	service := fcstorage{cs: *suite.cs}
	parameterMap := getFCCreateVolumeParamter()
	delete(parameterMap, "fstype")
	crtValReq := getISCSICreateValumeRequest("", parameterMap)
	_, err := service.CreateVolume(context.Background(), crtValReq)
	assert.NotNil(suite.T(), err, "Fail to validate parameter for fc protocol")
}

func (suite *FCControllerSuite) Test_CreateVolume_GetVolumeCapabilities_fail() {
	service := fcstorage{cs: *suite.cs}
	parameterMap := getFCCreateVolumeParamter()
	crtValReq := getISCSICreateValumeRequest("pvname", parameterMap)
	crtValReq.VolumeCapabilities = nil
	_, err := service.CreateVolume(context.Background(), crtValReq)
	assert.NotNil(suite.T(), err, "Fail to VolumeCapabilitie for fc protocol")
}

func (suite *FCControllerSuite) Test_CreateVolume_GetVolumeCapabilities_Mode_fail() {
	service := fcstorage{cs: *suite.cs}
	parameterMap := getFCCreateVolumeParamter()
	crtValReq := getISCSICreateValumeRequest("pvname", parameterMap)

	capa := csi.VolumeCapability{
		AccessMode: &csi.VolumeCapability_AccessMode{
			Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY,
		},
	}
	var arr []*csi.VolumeCapability
	arr = append(arr, &capa)
	crtValReq.VolumeCapabilities = arr

	_, err := service.CreateVolume(context.Background(), crtValReq)
	assert.NotNil(suite.T(), err, "Fail to validate VolumeCapabilitie for fc protocol")
}

func (suite *FCControllerSuite) Test_CreateVolume_GetPVName_fail() {
	service := fcstorage{cs: *suite.cs}
	parameterMap := getFCCreateVolumeParamter()
	crtValReq := getISCSICreateValumeRequest("", parameterMap)
	_, err := service.CreateVolume(context.Background(), crtValReq)
	assert.NotNil(suite.T(), err, "Name cannot be empty")
}

func (suite *FCControllerSuite) Test_CreateVolume_GetName_fail() {
	service := fcstorage{cs: *suite.cs}
	parameterMap := getFCCreateVolumeParamter()
	crtValReq := getISCSICreateValumeRequest("PVName", parameterMap)
	expectedErr := errors.New("some Error")

	suite.api.On("GetVolumeByName", mock.Anything).Return(getVolume(), expectedErr)
	_, err := service.CreateVolume(context.Background(), crtValReq)
	assert.NotNil(suite.T(), err, "Name cannot be empty")
}

func (suite *FCControllerSuite) Test_CreateVolume_CreateVolume_fail() {
	service := fcstorage{cs: *suite.cs}
	parameterMap := getFCCreateVolumeParamter()
	crtValReq := getISCSICreateValumeRequest("PVName", parameterMap)
	expectedErr := errors.New("some Error")

	suite.api.On("GetVolumeByName", mock.Anything).Return(nil, nil)

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkspace(), nil)
	suite.api.On("CreateVolume", mock.Anything, mock.Anything).Return(nil, expectedErr)

	_, err := service.CreateVolume(context.Background(), crtValReq)
	assert.NotNil(suite.T(), err, "create volumne failed")
}

func (suite *FCControllerSuite) Test_CreateVolume_CreateVolume_success() {
	service := fcstorage{cs: *suite.cs}
	parameterMap := getFCCreateVolumeParamter()
	crtValReq := getISCSICreateValumeRequest("PVName", parameterMap)

	suite.api.On("GetVolumeByName", mock.Anything).Return(nil, nil)

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkspace(), nil)
	suite.api.On("CreateVolume", mock.Anything, mock.Anything).Return(getVolume(), nil)
	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	suite.api.On("AttachMetadataToObject", mock.Anything, mock.Anything).Return(nil, nil)

	_, err := service.CreateVolume(context.Background(), crtValReq)
	assert.Nil(suite.T(), err, "should not be nil")
}

func (suite *FCControllerSuite) Test_CreateVolume_CreateVolume_metadataError() {
	service := fcstorage{cs: *suite.cs}
	parameterMap := getFCCreateVolumeParamter()
	crtValReq := getISCSICreateValumeRequest("PVName", parameterMap)
	expectedErr := errors.New("some Error")

	suite.api.On("GetVolumeByName", mock.Anything).Return(nil, nil)

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkspace(), nil)
	suite.api.On("CreateVolume", mock.Anything, mock.Anything).Return(getVolume(), nil)
	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	suite.api.On("AttachMetadataToObject", mock.Anything, mock.Anything).Return(nil, expectedErr)

	_, err := service.CreateVolume(context.Background(), crtValReq)
	assert.NotNil(suite.T(), err, "should return metadata")
}

func (suite *FCControllerSuite) Test_DeleteVolume_InvalidVolumeID() {
	service := fcstorage{cs: *suite.cs}
	crtValReq := getISCSIDeleteRequest()
	crtValReq.VolumeId = ""
	_, err := service.DeleteVolume(context.Background(), crtValReq)
	assert.NotNil(suite.T(), err, "invalid volumneID for fc protocol")
}

func (suite *FCControllerSuite) Test_DeleteVolume_casting_Error() {
	service := fcstorage{cs: *suite.cs}
	crtValReq := getISCSIDeleteRequest()
	crtValReq.VolumeId = "1bc"
	_, err := service.DeleteVolume(context.Background(), crtValReq)
	assert.NotNil(suite.T(), err, "casting failed for fc protocol")
}

func (suite *FCControllerSuite) Test_DeleteVolume_GetVolume_Error() {
	service := fcstorage{cs: *suite.cs}
	crtValReq := getISCSIDeleteRequest()
	expectedErr := errors.New("some Error")
	suite.api.On("GetVolume", mock.Anything).Return(nil, expectedErr)
	_, err := service.DeleteVolume(context.Background(), crtValReq)
	assert.NotNil(suite.T(), err, "Fail to validate getVolume for fc protocol")
}

func (suite *FCControllerSuite) Test_DeleteVolume_GetVolumeSnapshot_metadataError() {
	service := fcstorage{cs: *suite.cs}
	crtValReq := getISCSIDeleteRequest()
	expectedErr := errors.New("some Error")
	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	suite.api.On("GetVolumeSnapshotByParentID", mock.Anything).Return(getVolumeArray(), nil)
	suite.api.On("AttachMetadataToObject", mock.Anything, mock.Anything).Return(nil, expectedErr)

	_, err := service.DeleteVolume(context.Background(), crtValReq)
	assert.NotNil(suite.T(), err, "Fail to GetVolumeSnapshot for fc protocol")
}

func (suite *FCControllerSuite) Test_DeleteVolume_DeleteVolume_Error() {
	service := fcstorage{cs: *suite.cs}
	crtValReq := getISCSIDeleteRequest()
	expectedErr := errors.New("some Error")
	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	suite.api.On("GetVolumeSnapshotByParentID", mock.Anything).Return([]api.Volume{}, nil)
	suite.api.On("DeleteVolume", mock.Anything).Return(expectedErr)

	_, err := service.DeleteVolume(context.Background(), crtValReq)
	assert.NotNil(suite.T(), err, "Fail to delete for fc protocol")
}

func (suite *FCControllerSuite) Test_DeleteVolume_DeleteVolume_success() {
	service := fcstorage{cs: *suite.cs}
	crtValReq := getISCSIDeleteRequest()
	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	suite.api.On("GetVolumeSnapshotByParentID", mock.Anything).Return([]api.Volume{}, nil)
	suite.api.On("DeleteVolume", mock.Anything).Return(nil)
	suite.api.On("GetMetadataStatus", mock.Anything).Return(false)
	_, err := service.DeleteVolume(context.Background(), crtValReq)
	assert.Nil(suite.T(), err, "expected no error")
}

func (suite *FCControllerSuite) Test_DeleteVolume_DeleteVolume_AlreadyDelete() {
	service := fcstorage{cs: *suite.cs}
	crtValReq := getISCSIDeleteRequest()
	expectedErr := errors.New("VOLUME_NOT_FOUND")
	suite.api.On("GetVolume", mock.Anything).Return(nil, expectedErr)

	_, err := service.DeleteVolume(context.Background(), crtValReq)
	assert.Nil(suite.T(), err, "already delete for fc protocol")
}

func (suite *FCControllerSuite) Test_CreateVolume_CreateVolume_content_succes() {
	service := fcstorage{cs: *suite.cs}
	parameterMap := getFCCreateVolumeParamter()
	crtValReq := getISCSICreateVolumeCloneRequest(parameterMap)
	suite.api.On("GetVolumeByName", mock.Anything).Return(nil, nil)
	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkspace(), nil)
	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	var poolID int64 = 10
	suite.api.On("GetStoragePoolIDByName", mock.Anything).Return(poolID, nil)
	suite.api.On("CreateSnapshotVolume", mock.Anything).Return(getSnapshotResp(), nil)
	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	suite.api.On("AttachMetadataToObject", mock.Anything, mock.Anything).Return(nil, nil)
	_, err := service.CreateVolume(context.Background(), crtValReq)
	assert.Nil(suite.T(), err, "expected no error")
}

func (suite *FCControllerSuite) Test_CreateVolume_CreateVolume_content_AttachMetadataToObject_err() {
	service := fcstorage{cs: *suite.cs}
	parameterMap := getFCCreateVolumeParamter()
	crtValReq := getISCSICreateVolumeCloneRequest(parameterMap)
	expectedErr := errors.New("some Error")
	suite.api.On("GetVolumeByName", mock.Anything).Return(nil, nil)
	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkspace(), nil)
	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	var poolID int64 = 10
	suite.api.On("GetStoragePoolIDByName", mock.Anything).Return(poolID, nil)
	suite.api.On("CreateSnapshotVolume", mock.Anything).Return(getSnapshotResp(), nil)
	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	suite.api.On("AttachMetadataToObject", mock.Anything, mock.Anything).Return(nil, expectedErr)
	_, err := service.CreateVolume(context.Background(), crtValReq)
	assert.NotNil(suite.T(), err, "metada data error")
}

func (suite *FCControllerSuite) Test_ControllerPublishVolume_success() {
	service := fcstorage{cs: *suite.cs}
	//	var parameterMap map[string]string
	ctrPublishValReq := getISCSIControllerPublishVolumeRequest()
	suite.accessMock.On("IsValidAccessMode", mock.Anything, mock.Anything).Return(true, nil)
	suite.api.On("GetHostByName", mock.Anything).Return(getHostByName(), nil)
	suite.api.On("GetAllLunByHost", mock.Anything).Return(getLunInfoArry(), nil)
	suite.api.On("MapVolumeToHost", mock.Anything).Return(getLunInf(), nil)
	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	_, err := service.ControllerPublishVolume(context.Background(), ctrPublishValReq)
	assert.Nil(suite.T(), err, "expected no error")
}

func (suite *FCControllerSuite) Test_ControllerPublishVolume_storageClassError() {
	service := fcstorage{cs: *suite.cs}
	//	var parameterMap map[string]string
	ctrPublishValReq := getISCSIControllerPublishVolumeRequest()
	ctrPublishValReq.VolumeId = "1$"
	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	_, err := service.ControllerPublishVolume(context.Background(), ctrPublishValReq)
	assert.NotNil(suite.T(), err, "Fail to storage class for fc protocol")
}

func (suite *FCControllerSuite) Test_ControllerPublishVolume_MaxVolumeError() {
	service := fcstorage{cs: *suite.cs}
	ctrPublishValReq := getISCSIControllerPublishVolumeRequest()
	suite.api.On("GetHostByName", mock.Anything).Return(getHostByName(), nil)
	suite.api.On("GetAllLunByHost", mock.Anything).Return(getLunInfoArry(), nil)
	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	suite.accessMock.On("IsValidAccessMode", mock.Anything, mock.Anything).Return(true, nil)
	ctrPublishValReq.VolumeContext = map[string]string{"max_vols_per_host": "AA"}
	_, err := service.ControllerPublishVolume(context.Background(), ctrPublishValReq)
	assert.NotNil(suite.T(), err, "Fail to storage class for fc protocol")
}

func (suite *FCControllerSuite) Test_ControllerPublishVolume_MaxAllowedError() {
	service := fcstorage{cs: *suite.cs}
	ctrPublishValReq := getISCSIControllerPublishVolumeRequest()
	suite.api.On("GetHostByName", mock.Anything).Return(getHostByName(), nil)
	suite.api.On("GetAllLunByHost", mock.Anything).Return(getLunInfoArry(), nil)
	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	suite.accessMock.On("IsValidAccessMode", mock.Anything, mock.Anything).Return(true, nil)
	ctrPublishValReq.VolumeContext = map[string]string{"max_vols_per_host": "0"}
	_, err := service.ControllerPublishVolume(context.Background(), ctrPublishValReq)
	assert.NotNil(suite.T(), err, "Fail to storage class for fc protocol")
}

func (suite *FCControllerSuite) Test_UnControllerPublishVolume() {
	service := fcstorage{cs: *suite.cs}
	//	var parameterMap map[string]string
	ctrUnPublishValReq := getISCSIControllerUnpublishVolume()
	suite.api.On("GetHostByName", mock.Anything).Return(getHostByName(), nil)
	suite.api.On("UnMapVolumeFromHost", mock.Anything, mock.Anything).Return(nil)
	suite.api.On("GetAllLunByHost", mock.Anything).Return([]api.LunInfo{}, nil)
	suite.api.On("DeleteHost", mock.Anything, mock.Anything).Return(nil)
	_, err := service.ControllerUnpublishVolume(context.Background(), ctrUnPublishValReq)
	assert.Nil(suite.T(), err, "controller unpublish for fc protocol")
}

func (suite *FCControllerSuite) Test_UnControllerPublishVolume_hostNameErr() {
	service := fcstorage{cs: *suite.cs}
	expectedErr := errors.New("some Error")
	ctrUnPublishValReq := getISCSIControllerUnpublishVolume()
	suite.api.On("GetHostByName", mock.Anything).Return(nil, expectedErr)
	_, err := service.ControllerUnpublishVolume(context.Background(), ctrUnPublishValReq)
	assert.NotNil(suite.T(), err, "Fail to hostname for fc protocol")
}

func (suite *FCControllerSuite) Test_UnControllerPublishVolume_StorageErr() {
	service := fcstorage{cs: *suite.cs}
	ctrUnPublishValReq := getISCSIControllerUnpublishVolume()
	ctrUnPublishValReq.VolumeId = "1$"
	_, err := service.ControllerUnpublishVolume(context.Background(), ctrUnPublishValReq)
	assert.NotNil(suite.T(), err, "Error should be notnil")
}

func (suite *FCControllerSuite) Test_UnControllerPublishVolume_UnMapVolumeErr() {
	service := fcstorage{cs: *suite.cs}
	expectedErr := errors.New("some Error")
	ctrUnPublishValReq := getISCSIControllerUnpublishVolume()
	suite.api.On("GetHostByName", mock.Anything).Return(getHostByName(), nil)
	suite.api.On("UnMapVolumeFromHost", mock.Anything, mock.Anything).Return(expectedErr)
	_, err := service.ControllerUnpublishVolume(context.Background(), ctrUnPublishValReq)
	assert.NotNil(suite.T(), err, "Error should be notnil")
}

func (suite *FCControllerSuite) Test_UnControllerPublishVolume_DeleteHostErr() {
	service := fcstorage{cs: *suite.cs}
	expectedErr := errors.New("some Error")
	ctrUnPublishValReq := getISCSIControllerUnpublishVolume()
	suite.api.On("GetHostByName", mock.Anything).Return(getHostByName(), nil)
	suite.api.On("UnMapVolumeFromHost", mock.Anything, mock.Anything).Return(nil)
	suite.api.On("GetAllLunByHost", mock.Anything).Return([]api.LunInfo{}, nil)
	suite.api.On("DeleteHost", mock.Anything, mock.Anything).Return(expectedErr)
	_, err := service.ControllerUnpublishVolume(context.Background(), ctrUnPublishValReq)
	assert.NotNil(suite.T(), err, "Error should be notnil")
}

func (suite *FCControllerSuite) Test_CreateSnapshot() {
	service := fcstorage{cs: *suite.cs}
	//	var parameterMap map[string]string
	ctrUnPublishValReq := getISCSICreateSnapshotRequest()
	suite.api.On("GetVolumeByName", mock.Anything).Return(getVolume(), nil)
	suite.api.On("CreateSnapshotVolume", mock.Anything).Return(getSnapshotResp(), nil)

	_, err := service.CreateSnapshot(context.Background(), ctrUnPublishValReq)
	assert.Nil(suite.T(), err, "Error should be notnil")
}

func (suite *FCControllerSuite) Test_CreateSnapshot_already_Created() {
	service := fcstorage{cs: *suite.cs}
	//	var parameterMap map[string]string
	ctrUnPublishValReq := getISCSICreateSnapshotRequest()
	ctrUnPublishValReq.SourceVolumeId = "1001$$iscsi"
	suite.api.On("GetVolumeByName", mock.Anything).Return(getVolume(), nil)
	suite.api.On("CreateSnapshotVolume", mock.Anything).Return(getSnapshotResp(), nil)

	_, err := service.CreateSnapshot(context.Background(), ctrUnPublishValReq)
	assert.Nil(suite.T(), err, "Error should be notnil")
}

func (suite *FCControllerSuite) Test_DeleteSnapshot() {
	service := fcstorage{cs: *suite.cs}
	//	var parameterMap map[string]string
	ctrdeleteSnapValReq := getISCSIDeleteSnapshotRequest()
	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	suite.api.On("GetVolumeSnapshotByParentID", mock.Anything).Return([]api.Volume{}, nil)
	suite.api.On("DeleteVolume", mock.Anything).Return(nil)
	suite.api.On("GetMetadataStatus", mock.Anything).Return(false)

	_, err := service.DeleteSnapshot(context.Background(), ctrdeleteSnapValReq)
	assert.Nil(suite.T(), err, "Error should be nil")
}

func (suite *FCControllerSuite) Test_ControllerExpandVolume() {
	service := fcstorage{cs: *suite.cs}
	//	var parameterMap map[string]string
	ctrExpandValReq := getISCSIExpandVolumeRequest()
	suite.api.On("UpdateVolume", mock.Anything, mock.Anything).Return(nil, nil)
	_, err := service.ControllerExpandVolume(context.Background(), ctrExpandValReq)
	assert.Nil(suite.T(), err, "Error should be nil")
}

func (suite *FCControllerSuite) Test_ValidateVolumeCapabilities() {
	service := fcstorage{cs: *suite.cs}
	_, err := service.ValidateVolumeCapabilities(context.Background(), &csi.ValidateVolumeCapabilitiesRequest{})
	assert.Nil(suite.T(), err, "Invalid volume ID")
}

func (suite *FCControllerSuite) Test_ListVolumes() {
	service := fcstorage{cs: *suite.cs}
	_, err := service.ListVolumes(context.Background(), &csi.ListVolumesRequest{})
	assert.Nil(suite.T(), err, "Invalid volume ID")
}

func (suite *FCControllerSuite) Test_ListSnapshots() {
	service := fcstorage{cs: *suite.cs}
	_, err := service.ListSnapshots(context.Background(), &csi.ListSnapshotsRequest{})
	assert.Nil(suite.T(), err, "Invalid volume ID")
}

func (suite *FCControllerSuite) Test_GetCapacity() {
	service := fcstorage{cs: *suite.cs}
	_, err := service.GetCapacity(context.Background(), &csi.GetCapacityRequest{})
	assert.Nil(suite.T(), err, "Invalid volume ID")
}

//Test data ===========

func getFCCreateVolumeParamter() map[string]string {
	return map[string]string{"fstype": "fstype1", "pool_name": "pool_name1", "provision_type": "provision_type1", "storage_protocol": "storage_protocol1", "ssd_enabled": "ssd_enabled1", "max_vols_per_host": "max_vols_per_host"}
}
