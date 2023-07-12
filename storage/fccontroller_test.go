//go:build unit

package storage

import (
	"context"
	"errors"
	"infinibox-csi-driver/api"
	"infinibox-csi-driver/common"
	"infinibox-csi-driver/helper"
	tests "infinibox-csi-driver/test_helper"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

func (suite *FCControllerSuite) SetupTest() {
	suite.api = new(api.MockApiService)
	suite.accessMock = new(helper.MockAccessModesHelper)
	suite.cs = &Commonservice{Api: suite.api, AccessModesHelper: suite.accessMock}

}

type FCControllerSuite struct {
	suite.Suite
	api        *api.MockApiService
	accessMock *helper.MockAccessModesHelper
	cs         *Commonservice
}

func TestFCControllerSuite(t *testing.T) {
	suite.Run(t, new(FCControllerSuite))
}

// BUG: bad test - FCController doesn't need to validate fstype, that's on fcnode to do.
// func (suite *FCControllerSuite) Test_CreateVolume_InvalidParameter_NoFsType() {
// 	service := fcstorage{cs: *suite.cs}
// 	parameterMap := getFCCreateVolumeParameter()
// 	delete(parameterMap, "fstype") // this is an old parameter anyway
// 	createVolReq := tests.GetCreateVolumeRequest("", parameterMap, "")
// 	createVolReq.VolumeCapabilities[0].AccessType.Mount.FsType = "" // this is where we should fail at the node level
// 	_, err := service.CreateVolume(context.Background(), createVolReq)
// 	assert.NotNil(suite.T(), err, "expected to fail: fc CreateVolume missing fstype parameter")
// }

func (suite *FCControllerSuite) Test_CreateVolume_GetName_fail() {
	service := fcstorage{cs: *suite.cs}
	parameterMap := getFCCreateVolumeParameter()
	createVolReq := tests.GetCreateVolumeRequest("PVName", parameterMap, "")
	expectedErr := errors.New("some Error")

	suite.api.On("GetVolumeByName", mock.Anything).Return(getVolume(), expectedErr)
	_, err := service.CreateVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: fc CreateVolume GetVolumeByName")
}

func (suite *FCControllerSuite) Test_CreateVolume_fail() {
	service := fcstorage{cs: *suite.cs}
	parameterMap := getFCCreateVolumeParameter()
	createVolReq := tests.GetCreateVolumeRequest("PVName", parameterMap, "")
	expectedErr := errors.New("some Error")

	suite.api.On("GetVolumeByName", mock.Anything).Return(nil, nil)

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkspace(), nil)
	suite.api.On("CreateVolume", mock.Anything, mock.Anything).Return(nil, expectedErr)

	_, err := service.CreateVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: fc CreateVolume create volume")
}

func (suite *FCControllerSuite) Test_CreateVolume_success() {
	service := fcstorage{cs: *suite.cs}
	parameterMap := getFCCreateVolumeParameter()
	createVolReq := tests.GetCreateVolumeRequest("PVName", parameterMap, "")

	suite.api.On("GetVolumeByName", mock.Anything).Return(nil, nil)

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkspace(), nil)
	suite.api.On("CreateVolume", mock.Anything, mock.Anything).Return(getVolume(), nil)
	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	suite.api.On("AttachMetadataToObject", mock.Anything, mock.Anything).Return(nil, nil)

	_, err := service.CreateVolume(context.Background(), createVolReq)
	assert.Nil(suite.T(), err, "expected to succeed: fc CreateVolume")
}

func (suite *FCControllerSuite) Test_CreateVolume_metadataError() {
	service := fcstorage{cs: *suite.cs}
	parameterMap := getFCCreateVolumeParameter()
	createVolReq := tests.GetCreateVolumeRequest("PVName", parameterMap, "")
	expectedErr := errors.New("some Error")

	suite.api.On("GetVolumeByName", mock.Anything).Return(nil, nil)

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkspace(), nil)
	suite.api.On("CreateVolume", mock.Anything, mock.Anything).Return(getVolume(), nil)
	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	suite.api.On("AttachMetadataToObject", mock.Anything, mock.Anything).Return(nil, expectedErr)

	_, err := service.CreateVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: fc CreateVolume attach metadata")
}

func (suite *FCControllerSuite) Test_DeleteVolume_InvalidVolumeID() {
	service := fcstorage{cs: *suite.cs}
	createVolReq := getISCSIDeleteRequest()
	createVolReq.VolumeId = ""
	_, err := service.DeleteVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: fc DeleteVolume empty volume ID")
}

func (suite *FCControllerSuite) Test_DeleteVolume_casting_Error() {
	service := fcstorage{cs: *suite.cs}
	createVolReq := getISCSIDeleteRequest()
	createVolReq.VolumeId = "1bc"
	_, err := service.DeleteVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: fc DeleteVolume invalid volume ID due to bad casting")
}

func (suite *FCControllerSuite) Test_DeleteVolume_GetVolume_Error() {
	service := fcstorage{cs: *suite.cs}
	createVolReq := getISCSIDeleteRequest()
	expectedErr := errors.New("some Error")
	suite.api.On("GetVolume", mock.Anything).Return(nil, expectedErr)
	_, err := service.DeleteVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: fc DeleteVolume GetVolume")
}

func (suite *FCControllerSuite) Test_DeleteVolume_GetVolumeSnapshot_metadataError() {
	service := fcstorage{cs: *suite.cs}
	createVolReq := getISCSIDeleteRequest()
	expectedErr := errors.New("some Error")
	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	suite.api.On("GetVolumeSnapshotByParentID", mock.Anything).Return(getVolumeArray(), nil)
	suite.api.On("AttachMetadataToObject", mock.Anything, mock.Anything).Return(nil, expectedErr)

	_, err := service.DeleteVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: fc DeleteVolume attach metadata")
}

func (suite *FCControllerSuite) Test_DeleteVolume_Error() {
	service := fcstorage{cs: *suite.cs}
	createVolReq := getISCSIDeleteRequest()
	expectedErr := errors.New("some Error")
	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	suite.api.On("GetVolumeSnapshotByParentID", mock.Anything).Return([]api.Volume{}, nil)
	suite.api.On("DeleteVolume", mock.Anything).Return(expectedErr)

	_, err := service.DeleteVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: fc DeleteVolume delete volume")
}

func (suite *FCControllerSuite) Test_DeleteVolume_success() {
	service := fcstorage{cs: *suite.cs}
	createVolReq := getISCSIDeleteRequest()
	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	suite.api.On("GetVolumeSnapshotByParentID", mock.Anything).Return([]api.Volume{}, nil)
	suite.api.On("DeleteVolume", mock.Anything).Return(nil)
	suite.api.On("GetMetadataStatus", mock.Anything).Return(false)
	_, err := service.DeleteVolume(context.Background(), createVolReq)
	assert.Nil(suite.T(), err, "expected to succeed: fc DeleteVolume")
}

func (suite *FCControllerSuite) Test_DeleteVolume_AlreadyDelete() {
	service := fcstorage{cs: *suite.cs}
	createVolReq := getISCSIDeleteRequest()
	expectedErr := errors.New("VOLUME_NOT_FOUND")
	suite.api.On("GetVolume", mock.Anything).Return(nil, expectedErr)

	_, err := service.DeleteVolume(context.Background(), createVolReq)
	assert.Nil(suite.T(), err, "expected to succeed: fc DeleteVolume already deleted")
}

func (suite *FCControllerSuite) Test_CreateVolume_content_success() {
	service := fcstorage{cs: *suite.cs}
	parameterMap := getFCCreateVolumeParameter()
	createVolReq := tests.GetCreateVolumeRequest("volumeName", parameterMap, "1$$fc")
	suite.api.On("GetVolumeByName", mock.Anything).Return(nil, nil)
	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkspace(), nil)
	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	var poolID int64 = 10
	suite.api.On("GetStoragePoolIDByName", mock.Anything).Return(poolID, nil)
	suite.api.On("CreateSnapshotVolume", mock.Anything).Return(getSnapshotResp(), nil)
	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	suite.api.On("AttachMetadataToObject", mock.Anything, mock.Anything).Return(nil, nil)

	_, err := service.CreateVolume(context.Background(), createVolReq)
	assert.Nil(suite.T(), err, "expected to succeed: fc CreateVolume")
}

func (suite *FCControllerSuite) Test_CreateVolume_content_AttachMetadataToObject_err() {
	service := fcstorage{cs: *suite.cs}
	parameterMap := getFCCreateVolumeParameter()
	createVolReq := tests.GetCreateVolumeRequest("volumeName", parameterMap, "1$$fc")
	expectedErr := errors.New("some Error")
	suite.api.On("GetVolumeByName", mock.Anything).Return(nil, nil)
	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkspace(), nil)
	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	var poolID int64 = 10
	suite.api.On("GetStoragePoolIDByName", mock.Anything).Return(poolID, nil)
	suite.api.On("CreateSnapshotVolume", mock.Anything).Return(getSnapshotResp(), nil)
	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	suite.api.On("AttachMetadataToObject", mock.Anything, mock.Anything).Return(nil, expectedErr)
	_, err := service.CreateVolume(context.Background(), createVolReq)
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
	ctrPublishValReq.VolumeContext = map[string]string{common.SC_MAX_VOLS_PER_HOST: "AA"}
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
	ctrPublishValReq.VolumeContext = map[string]string{common.SC_MAX_VOLS_PER_HOST: "0"}
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
	validateVolCapsReq := getISCSIValidateVolumeCapabilitiesRequest("", parameterMap)

	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	_, err := service.ValidateVolumeCapabilities(context.Background(), validateVolCapsReq)
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
		common.SC_MAX_VOLS_PER_HOST: "19",
		common.SC_POOL_NAME:         "pool_name1",
		common.SC_PROVISION_TYPE:    "THIN",
		common.SC_SSD_ENABLED:       "true",
		common.SC_STORAGE_PROTOCOL:  "fc",
	}
}
