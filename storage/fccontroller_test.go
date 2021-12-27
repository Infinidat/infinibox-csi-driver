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
	helper.ConfigureKlogForTesting()
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
	crtValReq := getISCSICreateVolumeRequest("", parameterMap)
	_, err := service.CreateVolume(context.Background(), crtValReq)
	assert.NotNil(suite.T(), err, "expected to fail: fc CreateVolume no parameters")
}

func (suite *FCControllerSuite) Test_CreateVolume_InvalidParameter_Fail2() {
	service := fcstorage{cs: *suite.cs}
	parameterMap := getFCCreateVolumeParameter()
	delete(parameterMap, "fstype")
	crtValReq := getISCSICreateVolumeRequest("", parameterMap)
	_, err := service.CreateVolume(context.Background(), crtValReq)
	assert.NotNil(suite.T(), err, "expected to fail: fc CreateVolume missing parameter")
}

func (suite *FCControllerSuite) Test_CreateVolume_GetVolumeCapabilities_fail() {
	service := fcstorage{cs: *suite.cs}
	parameterMap := getFCCreateVolumeParameter()
	crtValReq := getISCSICreateVolumeRequest("pvname", parameterMap)
	crtValReq.VolumeCapabilities = nil
	_, err := service.CreateVolume(context.Background(), crtValReq)
	assert.NotNil(suite.T(), err, "expected to fail: fc CreateVolume no VolumeCapabilities")
}

func (suite *FCControllerSuite) Test_CreateVolume_GetVolumeCapabilities_Mode_fail() {
	service := fcstorage{cs: *suite.cs}
	parameterMap := getFCCreateVolumeParameter()
	crtValReq := getISCSICreateVolumeRequest("pvname", parameterMap)

	capa := csi.VolumeCapability{
		AccessMode: &csi.VolumeCapability_AccessMode{
			Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY,
		},
	}
	var arr []*csi.VolumeCapability
	arr = append(arr, &capa)
	crtValReq.VolumeCapabilities = arr

	_, err := service.CreateVolume(context.Background(), crtValReq)
	assert.NotNil(suite.T(), err, "expected to fail: fc CreateVolume VolumeCapabilities invalid mode")
}

func (suite *FCControllerSuite) Test_CreateVolume_GetPVName_fail() {
	service := fcstorage{cs: *suite.cs}
	parameterMap := getFCCreateVolumeParameter()
	crtValReq := getISCSICreateVolumeRequest("", parameterMap)
	_, err := service.CreateVolume(context.Background(), crtValReq)
	assert.NotNil(suite.T(), err, "Name cannot be empty")
}

func (suite *FCControllerSuite) Test_CreateVolume_GetName_fail() {
	service := fcstorage{cs: *suite.cs}
	parameterMap := getFCCreateVolumeParameter()
	crtValReq := getISCSICreateVolumeRequest("PVName", parameterMap)
	expectedErr := errors.New("some Error")

	suite.api.On("GetVolumeByName", mock.Anything).Return(getVolume(), expectedErr)
	_, err := service.CreateVolume(context.Background(), crtValReq)
	assert.NotNil(suite.T(), err, "expected to fail: fc CreateVolume GetVolumeByName")
}

func (suite *FCControllerSuite) Test_CreateVolume_CreateVolume_fail() {
	service := fcstorage{cs: *suite.cs}
	parameterMap := getFCCreateVolumeParameter()
	crtValReq := getISCSICreateVolumeRequest("PVName", parameterMap)
	expectedErr := errors.New("some Error")

	suite.api.On("GetVolumeByName", mock.Anything).Return(nil, nil)

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkspace(), nil)
	suite.api.On("CreateVolume", mock.Anything, mock.Anything).Return(nil, expectedErr)

	_, err := service.CreateVolume(context.Background(), crtValReq)
	assert.NotNil(suite.T(), err, "expected to fail: fc CreateVolume create volume")
}

func (suite *FCControllerSuite) Test_CreateVolume_CreateVolume_success() {
	service := fcstorage{cs: *suite.cs}
	parameterMap := getFCCreateVolumeParameter()
	crtValReq := getISCSICreateVolumeRequest("PVName", parameterMap)

	suite.api.On("GetVolumeByName", mock.Anything).Return(nil, nil)

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkspace(), nil)
	suite.api.On("CreateVolume", mock.Anything, mock.Anything).Return(getVolume(), nil)
	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	suite.api.On("AttachMetadataToObject", mock.Anything, mock.Anything).Return(nil, nil)

	_, err := service.CreateVolume(context.Background(), crtValReq)
	assert.Nil(suite.T(), err, "expected to succeed: fc CreateVolume")
}

func (suite *FCControllerSuite) Test_CreateVolume_CreateVolume_metadataError() {
	service := fcstorage{cs: *suite.cs}
	parameterMap := getFCCreateVolumeParameter()
	crtValReq := getISCSICreateVolumeRequest("PVName", parameterMap)
	expectedErr := errors.New("some Error")

	suite.api.On("GetVolumeByName", mock.Anything).Return(nil, nil)

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkspace(), nil)
	suite.api.On("CreateVolume", mock.Anything, mock.Anything).Return(getVolume(), nil)
	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	suite.api.On("AttachMetadataToObject", mock.Anything, mock.Anything).Return(nil, expectedErr)

	_, err := service.CreateVolume(context.Background(), crtValReq)
	assert.NotNil(suite.T(), err, "expected to fail: fc CreateVolume attach metadata")
}

func (suite *FCControllerSuite) Test_DeleteVolume_InvalidVolumeID() {
	service := fcstorage{cs: *suite.cs}
	crtValReq := getISCSIDeleteRequest()
	crtValReq.VolumeId = ""
	_, err := service.DeleteVolume(context.Background(), crtValReq)
	assert.NotNil(suite.T(), err, "expected to fail: fc DeleteVolume empty volume ID")
}

func (suite *FCControllerSuite) Test_DeleteVolume_casting_Error() {
	service := fcstorage{cs: *suite.cs}
	crtValReq := getISCSIDeleteRequest()
	crtValReq.VolumeId = "1bc"
	_, err := service.DeleteVolume(context.Background(), crtValReq)
	assert.NotNil(suite.T(), err, "expected to fail: fc DeleteVolume invalid volume ID due to bad casting")
}

func (suite *FCControllerSuite) Test_DeleteVolume_GetVolume_Error() {
	service := fcstorage{cs: *suite.cs}
	crtValReq := getISCSIDeleteRequest()
	expectedErr := errors.New("some Error")
	suite.api.On("GetVolume", mock.Anything).Return(nil, expectedErr)
	_, err := service.DeleteVolume(context.Background(), crtValReq)
	assert.NotNil(suite.T(), err, "expected to fail: fc DeleteVolume GetVolume")
}

func (suite *FCControllerSuite) Test_DeleteVolume_GetVolumeSnapshot_metadataError() {
	service := fcstorage{cs: *suite.cs}
	crtValReq := getISCSIDeleteRequest()
	expectedErr := errors.New("some Error")
	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	suite.api.On("GetVolumeSnapshotByParentID", mock.Anything).Return(getVolumeArray(), nil)
	suite.api.On("AttachMetadataToObject", mock.Anything, mock.Anything).Return(nil, expectedErr)

	_, err := service.DeleteVolume(context.Background(), crtValReq)
	assert.NotNil(suite.T(), err, "expected to fail: fc DeleteVolume attach metadata")
}

func (suite *FCControllerSuite) Test_DeleteVolume_DeleteVolume_Error() {
	service := fcstorage{cs: *suite.cs}
	crtValReq := getISCSIDeleteRequest()
	expectedErr := errors.New("some Error")
	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	suite.api.On("GetVolumeSnapshotByParentID", mock.Anything).Return([]api.Volume{}, nil)
	suite.api.On("DeleteVolume", mock.Anything).Return(expectedErr)

	_, err := service.DeleteVolume(context.Background(), crtValReq)
	assert.NotNil(suite.T(), err, "expected to fail: fc DeleteVolume delete volume")
}

func (suite *FCControllerSuite) Test_DeleteVolume_DeleteVolume_success() {
	service := fcstorage{cs: *suite.cs}
	crtValReq := getISCSIDeleteRequest()
	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	suite.api.On("GetVolumeSnapshotByParentID", mock.Anything).Return([]api.Volume{}, nil)
	suite.api.On("DeleteVolume", mock.Anything).Return(nil)
	suite.api.On("GetMetadataStatus", mock.Anything).Return(false)
	_, err := service.DeleteVolume(context.Background(), crtValReq)
	assert.Nil(suite.T(), err, "expected to succeed: fc DeleteVolume")
}

func (suite *FCControllerSuite) Test_DeleteVolume_DeleteVolume_AlreadyDelete() {
	service := fcstorage{cs: *suite.cs}
	crtValReq := getISCSIDeleteRequest()
	expectedErr := errors.New("VOLUME_NOT_FOUND")
	suite.api.On("GetVolume", mock.Anything).Return(nil, expectedErr)

	_, err := service.DeleteVolume(context.Background(), crtValReq)
	assert.Nil(suite.T(), err, "expected to succeed: fc DeleteVolume already deleted")
}

func (suite *FCControllerSuite) Test_CreateVolume_CreateVolume_content_succes() {
	service := fcstorage{cs: *suite.cs}
	parameterMap := getFCCreateVolumeParameter()
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
	assert.Nil(suite.T(), err, "expected to succeed: fc CreateVolume")

	crtValReq.Parameters["storage_protocol"] = "unsupported_protocol"
	_, err = service.CreateVolume(context.Background(), crtValReq)
	assert.NotNil(suite.T(), err, "expected to fail: unrecognized storage_protocol in StorageClass Parameters")
}

func (suite *FCControllerSuite) Test_CreateVolume_CreateVolume_content_AttachMetadataToObject_err() {
	service := fcstorage{cs: *suite.cs}
	parameterMap := getFCCreateVolumeParameter()
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
	assert.NotNil(suite.T(), err, "expected to fail: fc CreateVolume attach metadata")
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
	assert.Nil(suite.T(), err, "expected to succeed: fc ControllerPublishVolume")
}

func (suite *FCControllerSuite) Test_ControllerPublishVolume_VolumeIDFormatError() {
	service := fcstorage{cs: *suite.cs}
	//	var parameterMap map[string]string
	ctrPublishValReq := getISCSIControllerPublishVolumeRequest()
	ctrPublishValReq.VolumeId = "1$"
	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	_, err := service.ControllerPublishVolume(context.Background(), ctrPublishValReq)
	assert.NotNil(suite.T(), err, "expected to fail: fc ControllerPublishVolume volume ID format invalid protocol")
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
	assert.NotNil(suite.T(), err, "expected to fail: fc ControllerPublishVolume invalid max_vols_per_host value")
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
	assert.NotNil(suite.T(), err, "expected to fail: fc ControllerPublishVolume max_vols_per_host exceeded")
}

func (suite *FCControllerSuite) Test_ControllerUnpublishVolume() {
	service := fcstorage{cs: *suite.cs}
	//	var parameterMap map[string]string
	unpublishVolReq := getISCSIControllerUnpublishVolume()
	suite.api.On("GetHostByName", mock.Anything).Return(getHostByName(), nil)
	suite.api.On("UnMapVolumeFromHost", mock.Anything, mock.Anything).Return(nil)
	suite.api.On("GetAllLunByHost", mock.Anything).Return([]api.LunInfo{}, nil)
	suite.api.On("DeleteHost", mock.Anything, mock.Anything).Return(nil)
	_, err := service.ControllerUnpublishVolume(context.Background(), unpublishVolReq)
	assert.Nil(suite.T(), err, "expected to succeed: fc ControllerUnpublishVolume")
}

func (suite *FCControllerSuite) Test_ControllerUnpublishVolume_hostNameErr() {
	service := fcstorage{cs: *suite.cs}
	expectedErr := errors.New("some Error")
	unpublishVolReq := getISCSIControllerUnpublishVolume()
	suite.api.On("GetHostByName", mock.Anything).Return(nil, expectedErr)
	_, err := service.ControllerUnpublishVolume(context.Background(), unpublishVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: fc ControllerUnpublishVolume GetHostByName")
}

func (suite *FCControllerSuite) Test_ControllerUnpublishVolume_VolumeIDFormatError() {
	service := fcstorage{cs: *suite.cs}
	unpublishVolReq := getISCSIControllerUnpublishVolume()
	unpublishVolReq.VolumeId = "1$"
	_, err := service.ControllerUnpublishVolume(context.Background(), unpublishVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: fc ControllerUnpublishVolume volume ID format invalid protocol")
}

func (suite *FCControllerSuite) Test_ControllerUnpublishVolume_UnMapVolumeErr() {
	service := fcstorage{cs: *suite.cs}
	expectedErr := errors.New("some Error")
	unpublishVolReq := getISCSIControllerUnpublishVolume()
	suite.api.On("GetHostByName", mock.Anything).Return(getHostByName(), nil)
	suite.api.On("UnMapVolumeFromHost", mock.Anything, mock.Anything).Return(expectedErr)
	_, err := service.ControllerUnpublishVolume(context.Background(), unpublishVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: fc ControllerUnpublishVolume UnMapVolumeFromHost")
}

func (suite *FCControllerSuite) Test_ControllerUnpublishVolume_DeleteHostErr() {
	service := fcstorage{cs: *suite.cs}
	expectedErr := errors.New("some Error")
	unpublishVolReq := getISCSIControllerUnpublishVolume()
	suite.api.On("GetHostByName", mock.Anything).Return(getHostByName(), nil)
	suite.api.On("UnMapVolumeFromHost", mock.Anything, mock.Anything).Return(nil)
	suite.api.On("GetAllLunByHost", mock.Anything).Return([]api.LunInfo{}, nil)
	suite.api.On("DeleteHost", mock.Anything, mock.Anything).Return(expectedErr)
	_, err := service.ControllerUnpublishVolume(context.Background(), unpublishVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: fc ControllerUnpublishVolume DeleteHost")
}

func (suite *FCControllerSuite) Test_CreateSnapshot_GetVolumeByNameErr() {
	service := fcstorage{cs: *suite.cs}
	expectedErr := errors.New("some Error")
	//	var parameterMap map[string]string
	unpublishVolReq := getISCSICreateSnapshotRequest()
	suite.api.On("GetVolumeByName", mock.Anything).Return(getVolume(), expectedErr)
	suite.api.On("CreateSnapshotVolume", mock.Anything).Return(getSnapshotResp(), nil)

	_, err := service.CreateSnapshot(context.Background(), unpublishVolReq)
	assert.Nil(suite.T(), err, "expected to fail: fc CreateSnapshot GetVolumeByName")
}

func (suite *FCControllerSuite) Test_CreateSnapshot_already_Created() {
	service := fcstorage{cs: *suite.cs}
	//	var parameterMap map[string]string
	unpublishVolReq := getISCSICreateSnapshotRequest()
	unpublishVolReq.SourceVolumeId = "1001$$iscsi"
	suite.api.On("GetVolumeByName", mock.Anything).Return(getVolume(), nil)
	suite.api.On("CreateSnapshotVolume", mock.Anything).Return(getSnapshotResp(), nil)

	_, err := service.CreateSnapshot(context.Background(), unpublishVolReq)
	assert.Nil(suite.T(), err, "expected to succeed: fc CreateSnapshot when already exists")
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
	assert.Nil(suite.T(), err, "expected to succeed: fc DeleteSnapshot")
}

func (suite *FCControllerSuite) Test_ControllerExpandVolume() {
	service := fcstorage{cs: *suite.cs}
	//	var parameterMap map[string]string
	ctrExpandValReq := getISCSIExpandVolumeRequest()
	suite.api.On("UpdateVolume", mock.Anything, mock.Anything).Return(nil, nil)
	_, err := service.ControllerExpandVolume(context.Background(), ctrExpandValReq)
	assert.Nil(suite.T(), err, "expected to succeed: fc ControllerExpandVolume")
}

func (suite *FCControllerSuite) Test_ValidateVolumeCapabilities() {
	service := fcstorage{cs: *suite.cs}
	var parameterMap map[string]string
	crtValidateVolCapsReq := getISCSIValidateVolumeCapabilitiesRequest("", parameterMap)

	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	_, err := service.ValidateVolumeCapabilities(context.Background(), crtValidateVolCapsReq)
	assert.Nil(suite.T(), err, "expected to succeed: fc ValidateVolumeCapabilities")
}

func (suite *FCControllerSuite) Test_ListVolumes() {
	service := fcstorage{cs: *suite.cs}
	_, err := service.ListVolumes(context.Background(), &csi.ListVolumesRequest{})
	assert.Nil(suite.T(), err, "expected to succeed: fc ListVolumes")
}

func (suite *FCControllerSuite) Test_ListSnapshots() {
	service := fcstorage{cs: *suite.cs}
	_, err := service.ListSnapshots(context.Background(), &csi.ListSnapshotsRequest{})
	assert.Nil(suite.T(), err, "expected to succeed: fc ListSnapshots")
}

func (suite *FCControllerSuite) Test_GetCapacity() {
	service := fcstorage{cs: *suite.cs}
	_, err := service.GetCapacity(context.Background(), &csi.GetCapacityRequest{})
	assert.Nil(suite.T(), err, "expected to succeed: fc GetCapacity")
}

// Test data ===========

func getFCCreateVolumeParameter() map[string]string {
	return map[string]string{
		"fstype":            "fstype1",
		"max_vols_per_host": "19",
		"pool_name":         "pool_name1",
		"provision_type":    "provision_type1",
		"ssd_enabled":       "true",
		"storage_protocol":  "fc",
	}
}
