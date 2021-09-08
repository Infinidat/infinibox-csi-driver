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

func (suite *ISCSIControllerSuite) SetupTest() {
	suite.api = new(api.MockApiService)
	suite.accessMock = new(helper.MockAccessModesHelper)
	suite.cs = &commonservice{api: suite.api, accessModesHelper: suite.accessMock}
}

type ISCSIControllerSuite struct {
	suite.Suite
	api        *api.MockApiService
	accessMock *helper.MockAccessModesHelper
	cs         *commonservice
}

func TestISCSIControllerSuite(t *testing.T) {
	suite.Run(t, new(ISCSIControllerSuite))
}

func (suite *ISCSIControllerSuite) Test_CreateVolume_InvalidParameter_Fail() {
	service := iscsistorage{cs: *suite.cs}
	var parameterMap map[string]string
	crtValReq := getISCSICreateValumeRequest("", parameterMap)
	_, err := service.CreateVolume(context.Background(), crtValReq)
	assert.NotNil(suite.T(), err, "Fail to validate parameter for iscsi protocol")
}

func (suite *ISCSIControllerSuite) Test_CreateVolume_InvalidParameter_Fail2() {
	service := iscsistorage{cs: *suite.cs}
	parameterMap := getISCSICreateVolumeParamter()
	delete(parameterMap, "useCHAP")
	crtValReq := getISCSICreateValumeRequest("", parameterMap)
	_, err := service.CreateVolume(context.Background(), crtValReq)
	assert.NotNil(suite.T(), err, "Fail to validate parameter for iscsi protocol")
}

func (suite *ISCSIControllerSuite) Test_CreateVolume_GetVolumeCapabilities_fail() {
	service := iscsistorage{cs: *suite.cs}
	parameterMap := getISCSICreateVolumeParamter()
	crtValReq := getISCSICreateValumeRequest("pvname", parameterMap)
	crtValReq.VolumeCapabilities = nil
	_, err := service.CreateVolume(context.Background(), crtValReq)
	assert.NotNil(suite.T(), err, "Fail to VolumeCapabilitie for iscsi protocol")
}

func (suite *ISCSIControllerSuite) Test_CreateVolume_GetVolumeCapabilities_Mode_fail() {
	service := iscsistorage{cs: *suite.cs}
	parameterMap := getISCSICreateVolumeParamter()
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
	assert.NotNil(suite.T(), err, "Fail to validate VolumeCapabilitie for iscsi protocol")
}

func (suite *ISCSIControllerSuite) Test_CreateVolume_GetPVName_fail() {
	service := iscsistorage{cs: *suite.cs}
	parameterMap := getISCSICreateVolumeParamter()
	crtValReq := getISCSICreateValumeRequest("", parameterMap)
	_, err := service.CreateVolume(context.Background(), crtValReq)
	assert.NotNil(suite.T(), err, "Name cannot be empty")
}

func (suite *ISCSIControllerSuite) Test_CreateVolume_GetName_fail() {
	service := iscsistorage{cs: *suite.cs}
	parameterMap := getISCSICreateVolumeParamter()
	crtValReq := getISCSICreateValumeRequest("PVName", parameterMap)
	expectedErr := errors.New("some Error")

	suite.api.On("GetVolumeByName", mock.Anything).Return(getVolume(), expectedErr)
	_, err := service.CreateVolume(context.Background(), crtValReq)
	assert.NotNil(suite.T(), err, "Name cannot be empty")
}

func (suite *ISCSIControllerSuite) Test_CreateVolume_CreateVolume_fail() {
	service := iscsistorage{cs: *suite.cs}
	parameterMap := getISCSICreateVolumeParamter()
	crtValReq := getISCSICreateValumeRequest("PVName", parameterMap)
	expectedErr := errors.New("some Error")

	suite.api.On("GetVolumeByName", mock.Anything).Return(nil, nil)

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkspace(), nil)
	suite.api.On("CreateVolume", mock.Anything, mock.Anything).Return(nil, expectedErr)

	_, err := service.CreateVolume(context.Background(), crtValReq)
	assert.NotNil(suite.T(), err, "create volumne failed")
}

func (suite *ISCSIControllerSuite) Test_CreateVolume_CreateVolume_success() {
	service := iscsistorage{cs: *suite.cs}
	parameterMap := getISCSICreateVolumeParamter()
	crtValReq := getISCSICreateValumeRequest("PVName", parameterMap)

	suite.api.On("GetVolumeByName", mock.Anything).Return(nil, nil)

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkspace(), nil)
	suite.api.On("CreateVolume", mock.Anything, mock.Anything).Return(getVolume(), nil)
	//suite.api.On("FindStoragePool", mock.Anything,mock.Anything).Return(getStoragePool(), nil)
	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	suite.api.On("AttachMetadataToObject", mock.Anything, mock.Anything).Return(nil, nil)

	_, err := service.CreateVolume(context.Background(), crtValReq)
	assert.Nil(suite.T(), err)
}

func (suite *ISCSIControllerSuite) Test_CreateVolume_CreateVolume_metadataError() {
	service := iscsistorage{cs: *suite.cs}
	parameterMap := getISCSICreateVolumeParamter()
	crtValReq := getISCSICreateValumeRequest("PVName", parameterMap)
	expectedErr := errors.New("some Error")

	suite.api.On("GetVolumeByName", mock.Anything).Return(nil, nil)

	suite.api.On("GetNetworkSpaceByName", mock.Anything).Return(getNetworkspace(), nil)
	suite.api.On("CreateVolume", mock.Anything, mock.Anything).Return(getVolume(), nil)
	//suite.api.On("FindStoragePool", mock.Anything,mock.Anything).Return(getStoragePool(), nil)
	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	suite.api.On("AttachMetadataToObject", mock.Anything, mock.Anything).Return(nil, expectedErr)

	_, err := service.CreateVolume(context.Background(), crtValReq)
	assert.NotNil(suite.T(), err, "should return metadata")
}

func (suite *ISCSIControllerSuite) Test_DeleteVolume_InvalidVolumeID() {
	service := iscsistorage{cs: *suite.cs}
	crtValReq := getISCSIDeleteRequest()
	crtValReq.VolumeId = ""
	_, err := service.DeleteVolume(context.Background(), crtValReq)
	assert.NotNil(suite.T(), err, "invalid volumneID for iscsi protocol")
}

func (suite *ISCSIControllerSuite) Test_DeleteVolume_casting_Error() {
	service := iscsistorage{cs: *suite.cs}
	crtValReq := getISCSIDeleteRequest()
	crtValReq.VolumeId = "1bc"
	_, err := service.DeleteVolume(context.Background(), crtValReq)
	assert.NotNil(suite.T(), err, "casting failed for iscsi protocol")
}

func (suite *ISCSIControllerSuite) Test_DeleteVolume_GetVolume_Error() {
	service := iscsistorage{cs: *suite.cs}
	crtValReq := getISCSIDeleteRequest()
	expectedErr := errors.New("some Error")
	suite.api.On("GetVolume", mock.Anything).Return(nil, expectedErr)
	_, err := service.DeleteVolume(context.Background(), crtValReq)
	assert.NotNil(suite.T(), err, "Fail to validate getVolume for iscsi protocol")
}

func (suite *ISCSIControllerSuite) Test_DeleteVolume_GetVolumeSnapshot_metadataError() {
	service := iscsistorage{cs: *suite.cs}
	crtValReq := getISCSIDeleteRequest()
	expectedErr := errors.New("some Error")
	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	suite.api.On("GetVolumeSnapshotByParentID", mock.Anything).Return(getVolumeArray(), nil)
	suite.api.On("AttachMetadataToObject", mock.Anything, mock.Anything).Return(nil, expectedErr)

	_, err := service.DeleteVolume(context.Background(), crtValReq)
	assert.NotNil(suite.T(), err, "Fail to GetVolumeSnapshot for iscsi protocol")
}

func (suite *ISCSIControllerSuite) Test_DeleteVolume_DeleteVolume_Error() {
	service := iscsistorage{cs: *suite.cs}
	crtValReq := getISCSIDeleteRequest()
	expectedErr := errors.New("some Error")
	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	suite.api.On("GetVolumeSnapshotByParentID", mock.Anything).Return([]api.Volume{}, nil)
	suite.api.On("DeleteVolume", mock.Anything).Return(expectedErr)

	_, err := service.DeleteVolume(context.Background(), crtValReq)
	assert.NotNil(suite.T(), err, "Fail to delete for iscsi protocol")
}

func (suite *ISCSIControllerSuite) Test_DeleteVolume_DeleteVolume_success() {
	service := iscsistorage{cs: *suite.cs}
	crtValReq := getISCSIDeleteRequest()
	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	suite.api.On("GetVolumeSnapshotByParentID", mock.Anything).Return([]api.Volume{}, nil)
	suite.api.On("DeleteVolume", mock.Anything).Return(nil)
	suite.api.On("GetMetadataStatus", mock.Anything).Return(false)
	_, err := service.DeleteVolume(context.Background(), crtValReq)
	assert.Nil(suite.T(), err, "delete success for iscsi protocol")
}

func (suite *ISCSIControllerSuite) Test_DeleteVolume_DeleteVolume_AlreadyDelete() {
	service := iscsistorage{cs: *suite.cs}
	crtValReq := getISCSIDeleteRequest()
	expectedErr := errors.New("VOLUME_NOT_FOUND")
	suite.api.On("GetVolume", mock.Anything).Return(nil, expectedErr)

	_, err := service.DeleteVolume(context.Background(), crtValReq)
	assert.Nil(suite.T(), err, "already delete for iscsi protocol")
}

func (suite *ISCSIControllerSuite) Test_CreateVolume_CreateVolume_content_succes() {
	service := iscsistorage{cs: *suite.cs}
	parameterMap := getISCSICreateVolumeParamter()
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
	assert.Nil(suite.T(), err, "fail to clone test")
}

func (suite *ISCSIControllerSuite) Test_CreateVolume_CreateVolume_content_AttachMetadataToObject_err() {
	service := iscsistorage{cs: *suite.cs}
	parameterMap := getISCSICreateVolumeParamter()
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

func (suite *ISCSIControllerSuite) Test_ControllerPublishVolume() {
	service := iscsistorage{cs: *suite.cs}
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

func (suite *ISCSIControllerSuite) Test_ControllerPublishVolume_storageClassError() {
	service := iscsistorage{cs: *suite.cs}
	//	var parameterMap map[string]string
	ctrPublishValReq := getISCSIControllerPublishVolumeRequest()
	ctrPublishValReq.VolumeId = "1$"
	_, err := service.ControllerPublishVolume(context.Background(), ctrPublishValReq)
	assert.NotNil(suite.T(), err, "Fail to storage class for iscsi protocol")
}

func (suite *ISCSIControllerSuite) Test_ControllerPublishVolume_MaxVolumeError() {
	service := iscsistorage{cs: *suite.cs}
	ctrPublishValReq := getISCSIControllerPublishVolumeRequest()
	suite.api.On("GetHostByName", mock.Anything).Return(getHostByName(), nil)
	suite.api.On("GetAllLunByHost", mock.Anything).Return(getLunInfoArry(), nil)
	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	suite.accessMock.On("IsValidAccessMode", mock.Anything, mock.Anything).Return(true, nil)
	ctrPublishValReq.VolumeContext = map[string]string{"max_vols_per_host": "AA"}
	_, err := service.ControllerPublishVolume(context.Background(), ctrPublishValReq)
	assert.NotNil(suite.T(), err, "Fail to storage class for iscsi protocol")
}

func (suite *ISCSIControllerSuite) Test_ControllerPublishVolume_MaxAllowedError() {
	service := iscsistorage{cs: *suite.cs}
	ctrPublishValReq := getISCSIControllerPublishVolumeRequest()
	suite.api.On("GetHostByName", mock.Anything).Return(getHostByName(), nil)
	suite.api.On("GetAllLunByHost", mock.Anything).Return(getLunInfoArry(), nil)
	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	suite.accessMock.On("IsValidAccessMode", mock.Anything, mock.Anything).Return(true, nil)
	ctrPublishValReq.VolumeContext = map[string]string{"max_vols_per_host": "0"}
	_, err := service.ControllerPublishVolume(context.Background(), ctrPublishValReq)
	assert.NotNil(suite.T(), err, "Fail to storage class for iscsi protocol")
}

func (suite *ISCSIControllerSuite) Test_ControllerUnpublishVolume_success() {
	service := iscsistorage{cs: *suite.cs}
	ctrUnPublishValReq := getISCSIControllerUnpublishVolume()
	suite.api.On("GetHostByName", mock.Anything).Return(getHostByName(), nil)
	suite.api.On("UnMapVolumeFromHost", mock.Anything, mock.Anything).Return(nil)
	suite.api.On("GetAllLunByHost", mock.Anything).Return([]api.LunInfo{}, nil)
	suite.api.On("DeleteHost", mock.Anything, mock.Anything).Return(nil)
	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	_, err := service.ControllerUnpublishVolume(context.Background(), ctrUnPublishValReq)
	assert.Nil(suite.T(), err, "controller unpublish for iscsi protocol")
}

func (suite *ISCSIControllerSuite) Test_ControllerUnpublishVolume_hostNameErr() {
	service := iscsistorage{cs: *suite.cs}
	expectedErr := errors.New("some Error")
	ctrUnPublishValReq := getISCSIControllerUnpublishVolume()
	suite.api.On("GetHostByName", mock.Anything).Return(nil, expectedErr)
	_, err := service.ControllerUnpublishVolume(context.Background(), ctrUnPublishValReq)
	assert.NotNil(suite.T(), err, "Fail to hostname for iscsi protocol")
}

func (suite *ISCSIControllerSuite) Test_ControllerUnpublishVolume_StorageErr() {
	service := iscsistorage{cs: *suite.cs}
	ctrUnPublishValReq := getISCSIControllerUnpublishVolume()
	ctrUnPublishValReq.VolumeId = "1$"
	_, err := service.ControllerUnpublishVolume(context.Background(), ctrUnPublishValReq)
	assert.NotNil(suite.T(), err, "Error should be notnil")
}

func (suite *ISCSIControllerSuite) Test_ControllerUnpublishVolume_UnMapVolumeErr() {
	service := iscsistorage{cs: *suite.cs}
	expectedErr := errors.New("some Error")
	ctrUnPublishValReq := getISCSIControllerUnpublishVolume()
	suite.api.On("GetHostByName", mock.Anything).Return(getHostByName(), nil)
	suite.api.On("UnMapVolumeFromHost", mock.Anything, mock.Anything).Return(expectedErr)
	_, err := service.ControllerUnpublishVolume(context.Background(), ctrUnPublishValReq)
	assert.NotNil(suite.T(), err, "Error should be notnil")
}

func (suite *ISCSIControllerSuite) Test_ControllerUnpublishVolume_DeleteHostErr() {
	service := iscsistorage{cs: *suite.cs}
	expectedErr := errors.New("some Error")
	ctrUnPublishValReq := getISCSIControllerUnpublishVolume()
	suite.api.On("GetHostByName", mock.Anything).Return(getHostByName(), nil)
	suite.api.On("UnMapVolumeFromHost", mock.Anything, mock.Anything).Return(nil)
	suite.api.On("GetAllLunByHost", mock.Anything).Return([]api.LunInfo{}, nil)
	suite.api.On("DeleteHost", mock.Anything, mock.Anything).Return(expectedErr)
	_, err := service.ControllerUnpublishVolume(context.Background(), ctrUnPublishValReq)
	assert.NotNil(suite.T(), err, "Error should be notnil")
}

func (suite *ISCSIControllerSuite) Test_CreateSnapshot() {
	service := iscsistorage{cs: *suite.cs}
	expectedErr := errors.New("some Error")
	//	var parameterMap map[string]string
	ctrUnPublishValReq := getISCSICreateSnapshotRequest()
	suite.api.On("GetVolumeByName", mock.Anything).Return(getVolume(), expectedErr)
	suite.api.On("CreateSnapshotVolume", mock.Anything).Return(getSnapshotResp(), nil)

	_, err := service.CreateSnapshot(context.Background(), ctrUnPublishValReq)
	assert.Nil(suite.T(), err, "Error should be notnil")
}

func (suite *ISCSIControllerSuite) Test_CreateSnapshot_already_Created() {
	service := iscsistorage{cs: *suite.cs}
	//	var parameterMap map[string]string
	ctrUnPublishValReq := getISCSICreateSnapshotRequest()
	ctrUnPublishValReq.SourceVolumeId = "1001$$iscsi"
	suite.api.On("GetVolumeByName", mock.Anything).Return(getVolume(), nil)
	suite.api.On("CreateSnapshotVolume", mock.Anything).Return(getSnapshotResp(), nil)

	_, err := service.CreateSnapshot(context.Background(), ctrUnPublishValReq)
	assert.Nil(suite.T(), err, "Error should be notnil")
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
	assert.Nil(suite.T(), err, "Error should be nil")
}

func (suite *ISCSIControllerSuite) Test_ControllerExpandVolume() {
	service := iscsistorage{cs: *suite.cs}
	//	var parameterMap map[string]string
	ctrExpandValReq := getISCSIExpandVolumeRequest()
	suite.api.On("UpdateVolume", mock.Anything, mock.Anything).Return(nil, nil)
	_, err := service.ControllerExpandVolume(context.Background(), ctrExpandValReq)
	assert.Nil(suite.T(), err, "Error should be nil")
}

func (suite *ISCSIControllerSuite) Test_ValidateVolumeCapabilities() {
	service := iscsistorage{cs: *suite.cs}
	var parameterMap map[string]string
	crtValidateVolCapsReq := getISCSIValidateVolumeCapabilitiesRequest("", parameterMap)

	suite.api.On("GetVolume", mock.Anything).Return(getVolume(), nil)
	_, err := service.ValidateVolumeCapabilities(context.Background(), crtValidateVolCapsReq)
	assert.Nil(suite.T(), err, "Invalid volume ID")
}

func (suite *ISCSIControllerSuite) Test_ListVolumes() {
	service := iscsistorage{cs: *suite.cs}
	_, err := service.ListVolumes(context.Background(), &csi.ListVolumesRequest{})
	assert.Nil(suite.T(), err, "Invalid volume ID")
}

func (suite *ISCSIControllerSuite) Test_ListSnapshots() {
	service := iscsistorage{cs: *suite.cs}
	_, err := service.ListSnapshots(context.Background(), &csi.ListSnapshotsRequest{})
	assert.Nil(suite.T(), err, "Invalid volume ID")
}

func (suite *ISCSIControllerSuite) Test_GetCapacity() {
	service := iscsistorage{cs: *suite.cs}
	_, err := service.GetCapacity(context.Background(), &csi.GetCapacityRequest{})
	assert.Nil(suite.T(), err, "Invalid volume ID")
}

//TEST data=============================================

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
		VolumeContext: map[string]string{"max_vols_per_host": "10"},
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

func getStoragePool() api.StoragePool {
	var storagePool api.StoragePool
	storagePool.ID = 100
	storagePool.Name = "storagePoll"
	return storagePool
}
func getVolume() api.Volume {
	var vol api.Volume
	vol.ID = 100
	vol.PoolId = 10
	vol.ParentId = 1001
	vol.Name = "volName"
	vol.PoolName = "poolName"
	vol.Size = 1073741824
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
	return nspace
}

func getISCSICreateValumeRequest(pvName string, parameterMap map[string]string) *csi.CreateVolumeRequest {
	capa := csi.VolumeCapability{
		AccessMode: &csi.VolumeCapability_AccessMode{
			Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
		},
	}
	var arr []*csi.VolumeCapability
	arr = append(arr, &capa)
	return &csi.CreateVolumeRequest{
		Name:               pvName,
		Parameters:         parameterMap,
		VolumeCapabilities: arr,
	}
}

func getISCSIValidateVolumeCapabilitiesRequest(pvName string, parameterMap map[string]string) *csi.ValidateVolumeCapabilitiesRequest {
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

//getCreateVolumeRequestByType - method return the snapshot or clone createVallume request
func getISCSICreateVolumeCloneRequest(parameterMap map[string]string) *csi.CreateVolumeRequest {
	capa := csi.VolumeCapability{
		AccessMode: &csi.VolumeCapability_AccessMode{
			Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
		},
	}
	var arr []*csi.VolumeCapability
	arr = append(arr, &capa)

	createValume := &csi.CreateVolumeRequest{
		Name:          "volumeName",
		CapacityRange: &csi.CapacityRange{RequiredBytes: 1000},
		Parameters:    parameterMap,
		VolumeContentSource: &csi.VolumeContentSource{
			Type: &csi.VolumeContentSource_Volume{
				Volume: &csi.VolumeContentSource_VolumeSource{
					VolumeId: "1$$iscsi",
				},
			},
		},
		VolumeCapabilities: arr,
	}
	return createValume
}

func getISCSICreateVolumeParamter() map[string]string {
	return map[string]string{"useCHAP": "useCHAP1", "fstype": "fstype1", "pool_name": "pool_name1", "network_space": "network_space1", "provision_type": "provision_type1", "storage_protocol": "storage_protocol1", "ssd_enabled": "ssd_enabled1", "max_vols_per_host": "max_vols_per_host"}
}
