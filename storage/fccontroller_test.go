//go:build unit

package storage

import (
	"context"
	"errors"
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

func (suite *FCControllerSuite) SetupTest() {
	suite.api = new(api.MockApiService)
	suite.iboxapi = new(iboxapi.MockApiService)
	suite.accessMock = new(helper.MockAccessModesHelper)
	host := api.Host{
		ID:   1,
		Name: "host1",
	}
	volProto := &api.VolumeProtocolConfig{
		Host:     host,
		VolumeID: 1,
		NodeID:   "node1",
	}
	suite.cs = &Commonservice{Api: suite.api, AccessModesHelper: suite.accessMock, IboxApi: suite.iboxapi, VolProto: volProto}

}

type FCControllerSuite struct {
	suite.Suite
	api        *api.MockApiService
	iboxapi    *iboxapi.MockApiService
	accessMock *helper.MockAccessModesHelper
	cs         *Commonservice
}

func TestFCControllerSuite(t *testing.T) {
	suite.Run(t, new(FCControllerSuite))
}

func (suite *FCControllerSuite) Test_CreateVolume_GetName_fail() {
	service := fcstorage{cs: *suite.cs}
	parameterMap := getFCCreateVolumeParameter()
	createVolReq := tests.GetCreateVolumeRequest("PVName", parameterMap, "")
	expectedErr := errors.New("some Error")

	suite.api.On("GetVolumeByName", mock.Anything).Return(getVolume(), expectedErr)
	suite.api.On("OneTimeValidation", mock.Anything, mock.Anything).Return("", nil)
	_, err := service.CreateVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: fc CreateVolume GetVolumeByName")
}

func (suite *FCControllerSuite) Test_CreateVolume_fail() {
	service := fcstorage{cs: *suite.cs}
	parameterMap := getFCCreateVolumeParameter()
	createVolReq := tests.GetCreateVolumeRequest("PVName", parameterMap, "")
	expectedErr := errors.New("some Error")

	poolResult := &iboxapi.PoolResult{ID: 10}
	suite.iboxapi.On("GetPoolByName", mock.Anything).Return(poolResult, nil)
	suite.api.On("GetVolumeByName", mock.Anything).Return(nil, nil)

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkspace(), nil)
	suite.api.On("CreateVolume", mock.Anything, mock.Anything).Return(nil, expectedErr)
	suite.api.On("OneTimeValidation", mock.Anything, mock.Anything).Return("", nil)

	_, err := service.CreateVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: fc CreateVolume create volume")
}

func (suite *FCControllerSuite) Test_CreateVolume_success() {
	service := fcstorage{cs: *suite.cs}
	parameterMap := getFCCreateVolumeParameter()
	createVolReq := tests.GetCreateVolumeRequest("PVName", parameterMap, "")

	poolResult := &iboxapi.PoolResult{ID: 10}
	suite.iboxapi.On("GetPoolByName", mock.Anything).Return(poolResult, nil)
	suite.api.On("GetVolumeByName", mock.Anything).Return(nil, nil)

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkspace(), nil)
	suite.api.On("CreateVolume", mock.Anything, mock.Anything).Return(getVolume(), nil)
	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	suite.iboxapi.On("PutMetadata", mock.Anything, mock.Anything).Return(nil, nil)
	suite.api.On("OneTimeValidation", mock.Anything, mock.Anything).Return("", nil)

	_, err := service.CreateVolume(context.Background(), createVolReq)
	assert.Nil(suite.T(), err, "expected to succeed: fc CreateVolume")
}

func (suite *FCControllerSuite) Test_CreateVolume_metadataError() {
	service := fcstorage{cs: *suite.cs}
	parameterMap := getFCCreateVolumeParameter()
	createVolReq := tests.GetCreateVolumeRequest("PVName", parameterMap, "")
	expectedErr := errors.New("some Error")

	poolResult := &iboxapi.PoolResult{ID: 10}
	suite.iboxapi.On("GetPoolByName", mock.Anything).Return(poolResult, nil)
	suite.api.On("GetVolumeByName", mock.Anything).Return(nil, nil)

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkspace(), nil)
	suite.api.On("CreateVolume", mock.Anything, mock.Anything).Return(getVolume(), nil)
	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	suite.iboxapi.On("PutMetadata", mock.Anything, mock.Anything).Return(nil, expectedErr)
	suite.api.On("OneTimeValidation", mock.Anything, mock.Anything).Return("", nil)

	_, err := service.CreateVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: fc CreateVolume attach metadata")
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
	suite.iboxapi.On("PutMetadata", mock.Anything, mock.Anything).Return(nil, expectedErr)

	_, err := service.DeleteVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: fc DeleteVolume attach metadata")
}

func (suite *FCControllerSuite) Test_DeleteVolume_Error() {
	service := fcstorage{cs: *suite.cs}
	createVolReq := getISCSIDeleteRequest()
	expectedErr := errors.New("some Error")
	deleteMetadataResponse := &iboxapi.DeleteMetadataResponse{}
	suite.iboxapi.On("DeleteMetadata", mock.Anything).Return(deleteMetadataResponse, nil)
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
	deleteMetadataResponse := &iboxapi.DeleteMetadataResponse{}
	suite.iboxapi.On("DeleteMetadata", mock.Anything).Return(deleteMetadataResponse, nil)
	suite.api.On("DeleteVolume", mock.Anything).Return(nil)
	suite.iboxapi.On("GetMetadata", mock.Anything).Return(test_helper.GetHostMetadata(), nil)
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
	service := fcstorage{capacity: common.BytesInOneGibibyte, cs: *suite.cs}
	parameterMap := getFCCreateVolumeParameter()
	createVolReq := tests.GetCreateVolumeRequest("volumeName", parameterMap, "1$$fc")
	poolResult := &iboxapi.PoolResult{ID: 10}
	suite.iboxapi.On("GetPoolByName", mock.Anything).Return(poolResult, nil)
	suite.api.On("GetVolumeByName", mock.Anything).Return(nil, nil)
	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkspace(), nil)
	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	suite.api.On("CreateSnapshotVolume", mock.Anything).Return(getSnapshotResp(), nil)
	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	suite.iboxapi.On("PutMetadata", mock.Anything, mock.Anything).Return(nil, nil)
	suite.api.On("OneTimeValidation", mock.Anything, mock.Anything).Return("", nil)

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
	poolID := 10
	suite.api.On("GetStoragePoolIDByName", mock.Anything).Return(poolID, nil)
	suite.api.On("OneTimeValidation", mock.Anything, mock.Anything).Return("", nil)
	suite.api.On("CreateSnapshotVolume", mock.Anything).Return(getSnapshotResp(), nil)
	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	suite.iboxapi.On("PutMetadata", mock.Anything, mock.Anything).Return(nil, expectedErr)
	_, err := service.CreateVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: fc CreateVolume attach metadata")
}

func (suite *FCControllerSuite) Test_ControllerPublishVolume_success() {
	service := fcstorage{cs: *suite.cs}
	ctrPublishValReq := getISCSIControllerPublishVolumeRequest()
	suite.accessMock.On("IsValidAccessMode", mock.Anything, mock.Anything).Return(true, nil)
	suite.api.On("CreateHost", mock.Anything).Return(getLunInfoArry(), nil)
	suite.api.On("GetHostByName", mock.Anything).Return(getHostByName(), nil)
	suite.iboxapi.On("PutMetadata", mock.Anything, mock.Anything).Return(nil, nil)
	suite.iboxapi.On("GetAllLunByHost", mock.Anything).Return(getLunInfoArry(), nil)
	suite.api.On("MapVolumeToHost", mock.Anything).Return(getLunInf(), nil)
	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	_, err := service.ControllerPublishVolume(context.Background(), ctrPublishValReq)
	assert.Nil(suite.T(), err, "expected to succeed: fc ControllerPublishVolume")
}

func (suite *FCControllerSuite) Test_ControllerPublishVolume_VolumeIDFormatError() {
	service := fcstorage{cs: *suite.cs}
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
	suite.iboxapi.On("GetAllLunByHost", mock.Anything).Return(getLunInfoArry(), nil)
	suite.iboxapi.On("PutMetadata", mock.Anything, mock.Anything).Return(nil, nil)
	suite.api.On("CreateHost", mock.Anything).Return(getLunInfoArry(), nil)
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
	suite.iboxapi.On("GetAllLunByHost", mock.Anything).Return(getLunInfoArry(), nil)
	suite.iboxapi.On("PutMetadata", mock.Anything, mock.Anything).Return(nil, nil)
	suite.api.On("CreateHost", mock.Anything).Return(getLunInfoArry(), nil)
	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	suite.accessMock.On("IsValidAccessMode", mock.Anything, mock.Anything).Return(true, nil)
	ctrPublishValReq.VolumeContext = map[string]string{common.SC_MAX_VOLS_PER_HOST: "0"}
	_, err := service.ControllerPublishVolume(context.Background(), ctrPublishValReq)
	assert.NotNil(suite.T(), err, "expected to fail: fc ControllerPublishVolume max_vols_per_host exceeded")
}

func (suite *FCControllerSuite) Test_ControllerUnpublishVolume() {
	service := fcstorage{cs: *suite.cs}
	deleteHostResponse := &iboxapi.DeleteHostResponse{
		Error: iboxapi.Error{},
	}
	unpublishVolReq := getISCSIControllerUnpublishVolume()
	suite.iboxapi.On("GetMetadata", mock.Anything).Return(test_helper.GetHostMetadata(), nil)
	suite.api.On("GetHostByName", mock.Anything).Return(getHostByName(), nil)
	suite.api.On("UnMapVolumeFromHost", mock.Anything, mock.Anything).Return(nil)
	suite.iboxapi.On("GetAllLunByHost", mock.Anything).Return([]api.LunInfo{}, nil)
	suite.iboxapi.On("DeleteHost", mock.Anything).Return(deleteHostResponse, nil)
	_, err := service.ControllerUnpublishVolume(context.Background(), unpublishVolReq)
	assert.Nil(suite.T(), err, "expected to succeed: fc ControllerUnpublishVolume")
}

func (suite *FCControllerSuite) Test_ControllerUnpublishVolume_hostNameErr() {
	service := fcstorage{cs: *suite.cs}
	expectedErr := errors.New("some Error")
	unpublishVolReq := getISCSIControllerUnpublishVolume()
	suite.iboxapi.On("GetMetadata", mock.Anything).Return(test_helper.GetHostMetadata(), nil)
	suite.api.On("GetHostByName", mock.Anything).Return(nil, expectedErr)
	suite.iboxapi.On("GetAllLunByHost", mock.Anything).Return([]iboxapi.LunInfo{}, nil)
	suite.iboxapi.On("DeleteHost", mock.Anything).Return(nil, expectedErr)
	_, err := service.ControllerUnpublishVolume(context.Background(), unpublishVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: fc ControllerUnpublishVolume GetHostByName")
}

func (suite *FCControllerSuite) Test_ControllerUnpublishVolume_UnMapVolumeErr() {
	service := fcstorage{cs: *suite.cs}
	expectedErr := errors.New("some Error")
	unpublishVolReq := getISCSIControllerUnpublishVolume()
	suite.iboxapi.On("GetMetadata", mock.Anything).Return(test_helper.GetHostMetadata(), nil)
	suite.api.On("GetHostByName", mock.Anything).Return(getHostByName(), nil)
	suite.iboxapi.On("GetAllLunByHost", mock.Anything).Return([]iboxapi.LunInfo{}, nil)
	suite.iboxapi.On("DeleteHost", mock.Anything).Return(nil, expectedErr)
	suite.api.On("UnMapVolumeFromHost", mock.Anything, mock.Anything).Return(expectedErr)
	_, err := service.ControllerUnpublishVolume(context.Background(), unpublishVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: fc ControllerUnpublishVolume UnMapVolumeFromHost")
}

func (suite *FCControllerSuite) Test_ControllerUnpublishVolume_DeleteHostErr() {
	service := fcstorage{cs: *suite.cs}
	deleteHostResponse := iboxapi.DeleteHostResponse{
		Error: iboxapi.Error{},
	}
	expectedErr := errors.New("some Error")
	unpublishVolReq := getISCSIControllerUnpublishVolume()
	suite.iboxapi.On("GetMetadata", mock.Anything).Return(test_helper.GetHostMetadata(), nil)
	suite.api.On("GetHostByName", mock.Anything).Return(getHostByName(), nil)
	suite.api.On("UnMapVolumeFromHost", mock.Anything, mock.Anything).Return(nil)
	suite.iboxapi.On("GetAllLunByHost", mock.Anything).Return([]iboxapi.LunInfo{}, nil)
	suite.iboxapi.On("DeleteHost", mock.Anything).Return(deleteHostResponse, expectedErr)
	_, err := service.ControllerUnpublishVolume(context.Background(), unpublishVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: fc ControllerUnpublishVolume DeleteHost")
}

func (suite *FCControllerSuite) Test_ControllerUnpublishVolume_MetadataErr() {
	service := fcstorage{cs: *suite.cs}
	expectedErr := errors.New("some Error")
	unpublishVolReq := getISCSIControllerUnpublishVolume()
	suite.iboxapi.On("GetMetadata", mock.Anything).Return(test_helper.GetHostMetadata(), errors.New("some error"))
	suite.api.On("GetHostByName", mock.Anything).Return(getHostByName(), nil)
	suite.api.On("UnMapVolumeFromHost", mock.Anything, mock.Anything).Return(nil)
	suite.iboxapi.On("GetAllLunByHost", mock.Anything).Return([]iboxapi.LunInfo{}, nil)
	suite.iboxapi.On("DeleteHost", mock.Anything).Return(nil, expectedErr)
	_, err := service.ControllerUnpublishVolume(context.Background(), unpublishVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: fc ControllerUnpublishVolume Metadata Error")
}

func (suite *FCControllerSuite) Test_CreateSnapshot_GetVolumeByNameErr() {
	service := fcstorage{cs: *suite.cs}
	expectedErr := errors.New("some Error")
	unpublishVolReq := getISCSICreateSnapshotRequest()
	suite.api.On("GetVolume", mock.Anything).Return(getVolume().ID, nil)
	suite.api.On("GetVolumeByName", mock.Anything).Return(getVolume(), expectedErr)
	suite.api.On("CreateSnapshotVolume", mock.Anything).Return(getSnapshotResp(), nil)

	_, err := service.CreateSnapshot(context.Background(), unpublishVolReq)
	assert.Nil(suite.T(), err, "expected to fail: fc CreateSnapshot GetVolumeByName")
}

func (suite *FCControllerSuite) Test_CreateSnapshot_already_Created() {
	service := fcstorage{cs: *suite.cs}
	suite.cs.VolProto.VolumeID = 1001
	unpublishVolReq := getISCSICreateSnapshotRequest()
	unpublishVolReq.SourceVolumeId = "1001$$iscsi"
	suite.api.On("GetVolumeByName", mock.Anything).Return(getVolume(), nil)
	suite.api.On("CreateSnapshotVolume", mock.Anything).Return(getSnapshotResp(), nil)

	_, err := service.CreateSnapshot(context.Background(), unpublishVolReq)
	assert.Nil(suite.T(), err, "expected to succeed: fc CreateSnapshot when already exists")
}

func (suite *FCControllerSuite) Test_DeleteSnapshot() {
	service := fcstorage{cs: *suite.cs}
	ctrdeleteSnapValReq := getISCSIDeleteSnapshotRequest()
	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	deleteMetadataResponse := &iboxapi.DeleteMetadataResponse{}
	suite.iboxapi.On("DeleteMetadata", mock.Anything).Return(deleteMetadataResponse, nil)
	suite.api.On("GetVolumeSnapshotByParentID", mock.Anything).Return([]api.Volume{}, nil)
	suite.api.On("DeleteVolume", mock.Anything).Return(nil)
	suite.iboxapi.On("GetMetadata", mock.Anything).Return(test_helper.GetHostMetadata(), nil)

	_, err := service.DeleteSnapshot(context.Background(), ctrdeleteSnapValReq)
	assert.Nil(suite.T(), err, "expected to succeed: fc DeleteSnapshot")
}

func (suite *FCControllerSuite) Test_ControllerExpandVolume() {
	service := fcstorage{cs: *suite.cs}
	ctrExpandValReq := getISCSIExpandVolumeRequest()
	suite.api.On("UpdateVolume", mock.Anything, mock.Anything).Return(nil, nil)
	_, err := service.ControllerExpandVolume(context.Background(), ctrExpandValReq)
	assert.Nil(suite.T(), err, "expected to succeed: fc ControllerExpandVolume")
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

func getFCCreateVolumeParameter() map[string]string {
	return map[string]string{
		common.SC_MAX_VOLS_PER_HOST: "19",
		common.SC_POOL_NAME:         "pool_name1",
		common.SC_PROVISION_TYPE:    "THIN",
		common.SC_SSD_ENABLED:       "true",
		common.SC_STORAGE_PROTOCOL:  "fc",
	}
}
