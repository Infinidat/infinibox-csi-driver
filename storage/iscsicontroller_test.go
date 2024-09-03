//go:build unit

package storage

import (
	"context"
	"errors"
	"infinibox-csi-driver/api"
	"infinibox-csi-driver/common"
	"infinibox-csi-driver/helper"
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
	suite.accessMock = new(helper.MockAccessModesHelper)
	suite.cs = &Commonservice{Api: suite.api, AccessModesHelper: suite.accessMock}

}

type ISCSIControllerSuite struct {
	suite.Suite
	api        *api.MockApiService
	accessMock *helper.MockAccessModesHelper
	cs         *Commonservice
}

func TestISCSIControllerSuite(t *testing.T) {
	suite.Run(t, new(ISCSIControllerSuite))
}

func (suite *ISCSIControllerSuite) Test_ValidateStorageClass_InvalidParameter_Fail() {
	service := iscsistorage{cs: *suite.cs}
	var parameterMap map[string]string
	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkspace(), nil)
	err := service.ValidateStorageClass(parameterMap)
	assert.NotNil(suite.T(), err, "expected to fail: iscsi CreateVolume invalid parameter")
}

func (suite *ISCSIControllerSuite) Test_ValidateStorageClass_InvalidParameter_Fail2() {
	service := iscsistorage{cs: *suite.cs}
	parameterMap := getISCSICreateVolumeParameters()
	delete(parameterMap, common.SC_USE_CHAP)
	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkspace(), nil)
	err := service.ValidateStorageClass(parameterMap)
	assert.NotNil(suite.T(), err, "expected to fail: iscsi CreateVolumevalidate missing parameter")
}

func (suite *ISCSIControllerSuite) Test_CreateVolume_GetName_fail() {
	service := iscsistorage{cs: *suite.cs}
	parameterMap := getISCSICreateVolumeParameters()
	createVolReq := tests.GetCreateVolumeRequest("pvname", parameterMap, "")
	expectedErr := errors.New("some Error")

	suite.api.On("GetVolumeByName", mock.Anything).Return(getVolume(), expectedErr)
	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkspace(), nil)
	suite.api.On("OneTimeValidation", mock.Anything, mock.Anything).Return("", nil)
	_, err := service.CreateVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: iscsi CreateVolume GetVolumeByName")
}

func (suite *ISCSIControllerSuite) Test_CreateVolume_fail() {
	service := iscsistorage{cs: *suite.cs}
	parameterMap := getISCSICreateVolumeParameters()
	createVolReq := tests.GetCreateVolumeRequest("pvname", parameterMap, "")
	expectedErr := errors.New("some Error")

	suite.api.On("GetVolumeByName", mock.Anything).Return(nil, nil)

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkspace(), nil)
	suite.api.On("CreateVolume", mock.Anything, mock.Anything).Return(nil, expectedErr)
	suite.api.On("OneTimeValidation", mock.Anything, mock.Anything).Return("", nil)

	_, err := service.CreateVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: iscsi CreateVolume")
}

func (suite *ISCSIControllerSuite) Test_CreateVolume_success() {
	service := iscsistorage{cs: *suite.cs}
	parameterMap := getISCSICreateVolumeParameters()
	createVolReq := tests.GetCreateVolumeRequest("pvname", parameterMap, "")

	suite.api.On("GetVolumeByName", mock.Anything).Return(nil, nil)

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkspace(), nil)
	suite.api.On("CreateVolume", mock.Anything, mock.Anything).Return(getVolume(), nil)
	// suite.api.On("FindStoragePool", mock.Anything,mock.Anything).Return(getStoragePool(), nil)
	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	suite.api.On("OneTimeValidation", mock.Anything, mock.Anything).Return("", nil)
	suite.api.On("AttachMetadataToObject", mock.Anything, mock.Anything).Return(nil, nil)
	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkspace(), nil)

	_, err := service.CreateVolume(context.Background(), createVolReq)
	assert.Nil(suite.T(), err, "expected to succeed: CreateVolume")
}

func (suite *ISCSIControllerSuite) Test_CreateVolume_metadataError() {
	service := iscsistorage{cs: *suite.cs}
	parameterMap := getISCSICreateVolumeParameters()
	createVolReq := tests.GetCreateVolumeRequest("pvname", parameterMap, "")
	expectedErr := errors.New("some Error")

	suite.api.On("GetVolumeByName", mock.Anything).Return(nil, nil)

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkspace(), nil)
	suite.api.On("CreateVolume", mock.Anything, mock.Anything).Return(getVolume(), nil)
	// suite.api.On("FindStoragePool", mock.Anything,mock.Anything).Return(getStoragePool(), nil)
	suite.api.On("OneTimeValidation", mock.Anything, mock.Anything).Return("", nil)
	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	suite.api.On("AttachMetadataToObject", mock.Anything, mock.Anything).Return(nil, expectedErr)

	_, err := service.CreateVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: iscsi CreateVolume attach metadata")
}

func (suite *ISCSIControllerSuite) Test_DeleteVolume_InvalidVolumeID() {
	service := iscsistorage{cs: *suite.cs}
	createVolReq := getISCSIDeleteRequest()
	createVolReq.VolumeId = ""
	_, err := service.DeleteVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: iscsi DeleteVolume invalid volumneID")
}

func (suite *ISCSIControllerSuite) Test_DeleteVolume_casting_Error() {
	service := iscsistorage{cs: *suite.cs}
	createVolReq := getISCSIDeleteRequest()
	createVolReq.VolumeId = "1bc"
	_, err := service.DeleteVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "casting failed")
}

func (suite *ISCSIControllerSuite) Test_DeleteVolume_GetVolume_Error() {
	service := iscsistorage{cs: *suite.cs}
	createVolReq := getISCSIDeleteRequest()
	expectedErr := errors.New("some Error")
	suite.api.On("GetVolume", mock.Anything).Return(nil, expectedErr)
	_, err := service.DeleteVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: iscsi DeleteVolume getVolume")
}

func (suite *ISCSIControllerSuite) Test_DeleteVolume_GetVolumeSnapshot_metadataError() {
	service := iscsistorage{cs: *suite.cs}
	createVolReq := getISCSIDeleteRequest()
	expectedErr := errors.New("some Error")
	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	suite.api.On("GetVolumeSnapshotByParentID", mock.Anything).Return(getVolumeArray(), nil)
	suite.api.On("AttachMetadataToObject", mock.Anything, mock.Anything).Return(nil, expectedErr)

	_, err := service.DeleteVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: iscsi DeleteVolume GetVolumeSnapshot attach metadata")
}

func (suite *ISCSIControllerSuite) Test_DeleteVolume_Error() {
	service := iscsistorage{cs: *suite.cs}
	createVolReq := getISCSIDeleteRequest()
	expectedErr := errors.New("some Error")
	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	suite.api.On("GetVolumeSnapshotByParentID", mock.Anything).Return([]api.Volume{}, nil)
	suite.api.On("DeleteVolume", mock.Anything).Return(expectedErr)

	_, err := service.DeleteVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: iscsi DeleteVolume delete volume")
}

func (suite *ISCSIControllerSuite) Test_DeleteVolume_success() {
	service := iscsistorage{cs: *suite.cs}
	createVolReq := getISCSIDeleteRequest()
	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	suite.api.On("GetVolumeSnapshotByParentID", mock.Anything).Return([]api.Volume{}, nil)
	suite.api.On("DeleteVolume", mock.Anything).Return(nil)
	suite.api.On("GetMetadataStatus", mock.Anything).Return(false)
	_, err := service.DeleteVolume(context.Background(), createVolReq)
	assert.Nil(suite.T(), err, "expected to succeed: iscsi DeleteVolume")
}

func (suite *ISCSIControllerSuite) Test_DeleteVolume_AlreadyDelete() {
	service := iscsistorage{cs: *suite.cs}
	createVolReq := getISCSIDeleteRequest()
	expectedErr := errors.New("VOLUME_NOT_FOUND")
	suite.api.On("GetVolume", mock.Anything).Return(nil, expectedErr)

	_, err := service.DeleteVolume(context.Background(), createVolReq)
	assert.Nil(suite.T(), err, "expected to succeed: iscsi DeleteVolume when already deleted")
}

func (suite *ISCSIControllerSuite) Test_CreateVolume_content_success() {
	service := iscsistorage{capacity: common.BytesInOneGibibyte, cs: *suite.cs}
	parameterMap := getISCSICreateVolumeParameters()
	createVolReq := tests.GetCreateVolumeRequest("volumeName", parameterMap, "1$$iscsi")
	suite.api.On("GetVolumeByName", mock.Anything).Return(nil, nil)
	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkspace(), nil)
	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	var poolID int64 = 10
	suite.api.On("GetStoragePoolIDByName", mock.Anything).Return(poolID, nil)
	suite.api.On("CreateSnapshotVolume", mock.Anything).Return(getSnapshotResp(), nil)
	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	suite.api.On("AttachMetadataToObject", mock.Anything, mock.Anything).Return(nil, nil)
	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkspace(), nil)
	suite.api.On("OneTimeValidation", mock.Anything, mock.Anything).Return("", nil)

	_, err := service.CreateVolume(context.Background(), createVolReq)
	assert.Nil(suite.T(), err, "expected to succeed: iscsi CreateVolume success")
}

func (suite *ISCSIControllerSuite) Test_CreateVolume_content_AttachMetadataToObject_err() {
	service := iscsistorage{cs: *suite.cs}
	parameterMap := getISCSICreateVolumeParameters()
	createVolReq := tests.GetCreateVolumeRequest("volumeName", parameterMap, "1$$iscsi")
	expectedErr := errors.New("some Error")
	suite.api.On("GetVolumeByName", mock.Anything).Return(nil, nil)
	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkspace(), nil)
	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	var poolID int64 = 10
	suite.api.On("GetStoragePoolIDByName", mock.Anything).Return(poolID, nil)
	suite.api.On("CreateSnapshotVolume", mock.Anything).Return(getSnapshotResp(), nil)
	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	suite.api.On("AttachMetadataToObject", mock.Anything, mock.Anything).Return(nil, expectedErr)
	suite.api.On("OneTimeValidation", mock.Anything, mock.Anything).Return("", nil)
	_, err := service.CreateVolume(context.Background(), createVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: iscsi CreateVolume attach metadata")
}

func (suite *ISCSIControllerSuite) Test_ControllerPublishVolume() {
	service := iscsistorage{cs: *suite.cs}
	//	var parameterMap map[string]string
	ctrPublishValReq := getISCSIControllerPublishVolumeRequest()
	suite.api.On("GetMetadata", mock.Anything).Return(test_helper.GetHostMetadata(), nil)
	suite.accessMock.On("IsValidAccessMode", mock.Anything, mock.Anything).Return(true, nil)
	suite.api.On("GetHostByName", mock.Anything).Return(getHostByName(), nil)
	suite.api.On("GetAllLunByHost", mock.Anything).Return(getLunInfoArry(), nil)
	suite.api.On("MapVolumeToHost", mock.Anything).Return(getLunInf(), nil)
	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	_, err := service.ControllerPublishVolume(context.Background(), ctrPublishValReq)
	assert.Nil(suite.T(), err, "expected to succeed: iscsi ControllerPublishVolume")
}

func (suite *ISCSIControllerSuite) Test_ControllerPublishVolume_VolumeIDFormatError() {
	service := iscsistorage{cs: *suite.cs}
	//	var parameterMap map[string]string
	ctrPublishValReq := getISCSIControllerPublishVolumeRequest()
	ctrPublishValReq.VolumeId = "1$"
	_, err := service.ControllerPublishVolume(context.Background(), ctrPublishValReq)
	assert.NotNil(suite.T(), err, "expected to fail: iscsi ControllerPublishVolume volume ID format invalid protocol")
}

func (suite *ISCSIControllerSuite) Test_ControllerPublishVolume_MaxVolumeError() {
	service := iscsistorage{cs: *suite.cs}
	ctrPublishValReq := getISCSIControllerPublishVolumeRequest()
	suite.api.On("GetHostByName", mock.Anything).Return(getHostByName(), nil)
	suite.api.On("GetAllLunByHost", mock.Anything).Return(getLunInfoArry(), nil)
	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	suite.accessMock.On("IsValidAccessMode", mock.Anything, mock.Anything).Return(true, nil)
	ctrPublishValReq.VolumeContext = map[string]string{common.SC_MAX_VOLS_PER_HOST: "AA"}
	_, err := service.ControllerPublishVolume(context.Background(), ctrPublishValReq)
	assert.NotNil(suite.T(), err, "expected to fail: iscsi ControllerPublishVolume invalid max_vols_per_host value")
}

func (suite *ISCSIControllerSuite) Test_ControllerPublishVolume_MaxAllowedError() {
	service := iscsistorage{cs: *suite.cs}
	ctrPublishValReq := getISCSIControllerPublishVolumeRequest()
	suite.api.On("GetHostByName", mock.Anything).Return(getHostByName(), nil)
	suite.api.On("GetAllLunByHost", mock.Anything).Return(getLunInfoArry(), nil)
	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	suite.accessMock.On("IsValidAccessMode", mock.Anything, mock.Anything).Return(true, nil)
	ctrPublishValReq.VolumeContext = map[string]string{common.SC_MAX_VOLS_PER_HOST: "0"}
	_, err := service.ControllerPublishVolume(context.Background(), ctrPublishValReq)
	assert.NotNil(suite.T(), err, "expected to fail: iscsi ControllerPublishVolume max_vols_per_host exceeded")
}

func (suite *ISCSIControllerSuite) Test_ControllerUnpublishVolume_success() {
	service := iscsistorage{cs: *suite.cs}
	ctrUnPublishValReq := getISCSIControllerUnpublishVolume()
	suite.api.On("GetMetadata", mock.Anything).Return(test_helper.GetHostMetadata(), nil)
	suite.api.On("GetHostByName", mock.Anything).Return(getHostByName(), nil)
	suite.api.On("UnMapVolumeFromHost", mock.Anything, mock.Anything).Return(nil)
	suite.api.On("GetAllLunByHost", mock.Anything).Return([]api.LunInfo{}, nil)
	suite.api.On("DeleteHost", mock.Anything, mock.Anything).Return(nil)
	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	_, err := service.ControllerUnpublishVolume(context.Background(), ctrUnPublishValReq)
	assert.Nil(suite.T(), err, "expected to succeed: iscsi ControllerUnpublishVolume")
}

func (suite *ISCSIControllerSuite) Test_ControllerUnpublishVolume_hostNameErr() {
	service := iscsistorage{cs: *suite.cs}
	expectedErr := errors.New("some Error")
	ctrUnPublishValReq := getISCSIControllerUnpublishVolume()
	suite.api.On("GetHostByName", mock.Anything).Return(nil, expectedErr)
	_, err := service.ControllerUnpublishVolume(context.Background(), ctrUnPublishValReq)
	assert.NotNil(suite.T(), err, "expected to fail: iscsi ControllerUnpublishVolume GetHostByName")
}

func (suite *ISCSIControllerSuite) Test_ControllerUnpublishVolume_VolumeIDFormatError() {
	service := iscsistorage{cs: *suite.cs}
	ctrUnPublishValReq := getISCSIControllerUnpublishVolume()
	ctrUnPublishValReq.VolumeId = "1$"
	_, err := service.ControllerUnpublishVolume(context.Background(), ctrUnPublishValReq)
	assert.NotNil(suite.T(), err, "expected to fail: iscsi ControllerUnpublishVolume volume ID format invalid protocol")
}

func (suite *ISCSIControllerSuite) Test_ControllerUnpublishVolume_UnMapVolumeErr() {
	service := iscsistorage{cs: *suite.cs}
	expectedErr := errors.New("some Error")
	ctrUnPublishValReq := getISCSIControllerUnpublishVolume()
	suite.api.On("GetHostByName", mock.Anything).Return(getHostByName(), nil)
	suite.api.On("UnMapVolumeFromHost", mock.Anything, mock.Anything).Return(expectedErr)
	_, err := service.ControllerUnpublishVolume(context.Background(), ctrUnPublishValReq)
	assert.NotNil(suite.T(), err, "expected to fail: iscsi ControllerUnpublishVolume UnMapVolumeFromHost")
}

func (suite *ISCSIControllerSuite) Test_ControllerUnpublishVolume_DeleteHostErr() {
	service := iscsistorage{cs: *suite.cs}
	expectedErr := errors.New("some Error")
	ctrUnPublishValReq := getISCSIControllerUnpublishVolume()
	suite.api.On("GetMetadata", mock.Anything).Return(test_helper.GetHostMetadata(), nil)
	suite.api.On("GetHostByName", mock.Anything).Return(getHostByName(), nil)
	suite.api.On("UnMapVolumeFromHost", mock.Anything, mock.Anything).Return(nil)
	suite.api.On("GetAllLunByHost", mock.Anything).Return([]api.LunInfo{}, nil)
	suite.api.On("DeleteHost", mock.Anything, mock.Anything).Return(expectedErr)
	_, err := service.ControllerUnpublishVolume(context.Background(), ctrUnPublishValReq)
	assert.NotNil(suite.T(), err, "expected to fail: iscsi ControllerUnpublishVolume DeleteHost")
}

func (suite *ISCSIControllerSuite) Test_ControllerUnpublishVolume_Metadata_Error() {
	service := iscsistorage{cs: *suite.cs}
	expectedErr := errors.New("some Error")
	ctrUnPublishValReq := getISCSIControllerUnpublishVolume()
	suite.api.On("GetMetadata", mock.Anything).Return(test_helper.GetHostMetadata(), errors.New("some error"))
	suite.api.On("GetHostByName", mock.Anything).Return(getHostByName(), nil)
	suite.api.On("UnMapVolumeFromHost", mock.Anything, mock.Anything).Return(nil)
	suite.api.On("GetAllLunByHost", mock.Anything).Return([]api.LunInfo{}, nil)
	suite.api.On("DeleteHost", mock.Anything, mock.Anything).Return(expectedErr)
	_, err := service.ControllerUnpublishVolume(context.Background(), ctrUnPublishValReq)
	assert.NotNil(suite.T(), err, "expected to fail: iscsi ControllerUnpublishVolume Metadata Error")
}

func (suite *ISCSIControllerSuite) Test_CreateSnapshot() {
	service := iscsistorage{cs: *suite.cs}
	expectedErr := errors.New("some Error")
	//	var parameterMap map[string]string
	ctrUnPublishValReq := getISCSICreateSnapshotRequest()
	suite.api.On("GetVolumeByName", mock.Anything).Return(getVolume(), expectedErr)
	suite.api.On("CreateSnapshotVolume", mock.Anything).Return(getSnapshotResp(), nil)

	_, err := service.CreateSnapshot(context.Background(), ctrUnPublishValReq)
	assert.Nil(suite.T(), err, "expected to fail: iscsi CreateSnapshot GetVolumeByName")
}

func (suite *ISCSIControllerSuite) Test_CreateSnapshot_already_Created() {
	service := iscsistorage{cs: *suite.cs}
	//	var parameterMap map[string]string
	ctrUnPublishValReq := getISCSICreateSnapshotRequest()
	ctrUnPublishValReq.SourceVolumeId = "1001$$iscsi"
	suite.api.On("GetVolumeByName", mock.Anything).Return(getVolume(), nil)
	suite.api.On("CreateSnapshotVolume", mock.Anything).Return(getSnapshotResp(), nil)

	_, err := service.CreateSnapshot(context.Background(), ctrUnPublishValReq)
	assert.Nil(suite.T(), err, "expected to succeed: iscsi CreateSnapshot")
}

func (suite *ISCSIControllerSuite) Test_DeleteSnapshot() {
	service := iscsistorage{cs: *suite.cs}
	//	var parameterMap map[string]string
	ctrdeleteSnapValReq := getISCSIDeleteSnapshotRequest()
	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	suite.api.On("GetVolumeSnapshotByParentID", mock.Anything).Return([]api.Volume{}, nil)
	suite.api.On("DeleteVolume", mock.Anything).Return(nil)
	suite.api.On("GetMetadataStatus", mock.Anything).Return(false)

	_, err := service.DeleteSnapshot(context.Background(), ctrdeleteSnapValReq)
	assert.Nil(suite.T(), err, "expected to succeed: iscsi DeleteSnapshot")
}

func (suite *ISCSIControllerSuite) Test_ControllerExpandVolume() {
	service := iscsistorage{cs: *suite.cs}
	//	var parameterMap map[string]string
	ctrExpandValReq := getISCSIExpandVolumeRequest()
	suite.api.On("UpdateVolume", mock.Anything, mock.Anything).Return(nil, nil)
	_, err := service.ControllerExpandVolume(context.Background(), ctrExpandValReq)
	assert.Nil(suite.T(), err, "expected to succeed: iscsi ControllerExpandVolume")
}

func (suite *ISCSIControllerSuite) Test_ValidateVolumeCapabilities() {
	service := iscsistorage{cs: *suite.cs}
	var parameterMap map[string]string
	validateVolCapsReq := getISCSIValidateVolumeCapabilitiesRequest(parameterMap)

	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	_, err := service.ValidateVolumeCapabilities(context.Background(), validateVolCapsReq)
	assert.Nil(suite.T(), err, "expected to succeed: iscsi ValidateVolumeCapabilities")
}

func (suite *ISCSIControllerSuite) Test_ListVolumes() {
	service := iscsistorage{cs: *suite.cs}
	_, err := service.ListVolumes(context.Background(), &csi.ListVolumesRequest{})
	assert.Nil(suite.T(), err, "expected to succeed: iscsi ListVolumes")
}

func (suite *ISCSIControllerSuite) Test_ListSnapshots() {
	service := iscsistorage{cs: *suite.cs}
	_, err := service.ListSnapshots(context.Background(), &csi.ListSnapshotsRequest{})
	assert.Nil(suite.T(), err, "expected to succeed: iscsi ListSnapshots")
}

func (suite *ISCSIControllerSuite) Test_GetCapacity() {
	service := iscsistorage{cs: *suite.cs}
	_, err := service.GetCapacity(context.Background(), &csi.GetCapacityRequest{})
	assert.Nil(suite.T(), err, "expected to succeed: iscsi GetCapacity")
}

// TEST data=============================================

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

func getLunInf() api.LunInfo {
	var luninfo api.LunInfo
	luninfo.HostID = 100
	luninfo.ID = 1
	return luninfo
}

func getLunInfoArry() []api.LunInfo {
	var lunInfoArry []api.LunInfo
	lunInfoArry = append(lunInfoArry, getLunInf())
	return lunInfoArry
}

func getHostByName() api.Host {
	var host api.Host
	host.ID = 10
	host.Name = "hostName"
	lunInfoArry := getLunInfoArry()
	host.Luns = append(host.Luns, lunInfoArry...)
	var hostportArr []api.HostPort
	var hostport api.HostPort
	hostport.HostID = 10
	hostport.PortAddress = "10.20.20.50"
	hostport.PortType = "ISCSI"
	hostportArr = append(hostportArr, hostport)
	host.Ports = append(host.Ports, hostportArr...)
	return host
}

func getISCSIControllerPublishVolumeRequest() *csi.ControllerPublishVolumeRequest {
	return &csi.ControllerPublishVolumeRequest{
		VolumeId:      "1$$iscsi",
		NodeId:        "10.20.20.50$$iscsi",
		VolumeContext: map[string]string{common.SC_MAX_VOLS_PER_HOST: "10"},
	}
}

func getSnapshotResp() api.SnapshotVolumesResp {
	var snap api.SnapshotVolumesResp
	snap.Name = "snaName"
	snap.SnapShotID = 1000
	snap.PoolID = 10
	return snap
}

func getVolumeArray() []api.Volume {
	var volArry []api.Volume
	vol := getVolume()
	volArry = append(volArry, vol)
	return volArry
}

func getISCSIDeleteRequest() *csi.DeleteVolumeRequest {
	return &csi.DeleteVolumeRequest{
		VolumeId: "103",
	}
}

// func getStoragePool() api.StoragePool {
// 	var storagePool api.StoragePool
// 	storagePool.ID = 100
// 	storagePool.Name = "storagePoll"
// 	return storagePool
// }

func getVolume() api.Volume {
	var vol api.Volume
	vol.ID = 100
	vol.PoolId = 10
	vol.ParentId = 1001
	vol.Name = "volName"
	vol.PoolName = "poolName"
	vol.Size = common.BytesInOneGibibyte
	return vol
}

func getNetworkspace() api.NetworkSpace {
	var nspace api.NetworkSpace
	var p api.Portal
	var pArry []api.Portal
	p.Enabled = true
	p.InterfaceID = 1
	p.IpAdress = "10.20.30.40"
	p.Reserved = false
	p.Tpgt = 100
	p.Type = ""
	p.VlanID = 100

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
