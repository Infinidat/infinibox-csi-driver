/*Copyright 2020 Infinidat
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.*/
package service

import (
	"context"
	"infinibox-csi-driver/storage"
	"testing"

	"bou.ke/monkey"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type ControllerTestSuite struct {
	suite.Suite
}

func TestControllerTestSuite(t *testing.T) {
	suite.Run(t, new(ControllerTestSuite))
}

func (suite *ControllerTestSuite) Test_CreateVolume_Fail() {
	parameterMap := getContrCreateVolumeParamter()
	delete(parameterMap, "storage_protocol")
	createVolumeReq := getControllerCreateVolumeRequest("pvcName", parameterMap)
	s := getService()
	_, err := s.CreateVolume(context.Background(), createVolumeReq)
	assert.NotNil(suite.T(), err, "storage_protocol value missing")
}

func (suite *ControllerTestSuite) Test_storageController_Fail() {
	parameterMap := getContrCreateVolumeParamter()
	parameterMap["storage_protocol"] = "unknow"
	createVolumeReq := getControllerCreateVolumeRequest("pvcName", parameterMap)
	s := getService()
	_, err := s.CreateVolume(context.Background(), createVolumeReq)
	assert.NotNil(suite.T(), err, "storage_protocol value missing")

}

func (suite *ControllerTestSuite) Test_CreateVolme_fail() {
	parameterMap := getContrCreateVolumeParamter()
	createVolumeReq := getControllerCreateVolumeRequest("pvcName", parameterMap)
	s := getService()

	_, err := s.CreateVolume(context.Background(), createVolumeReq)
	assert.NotNil(suite.T(), err, "nfs valume should ge fail")
}

func (suite *ControllerTestSuite) Test_CreateVolme_success() {
	parameterMap := getContrCreateVolumeParamter()
	createVolumeReq := getControllerCreateVolumeRequest("pvcName", parameterMap)
	s := getService()

	patch := monkey.Patch(storage.NewStorageController, func(_ string, _ ...map[string]string) (storage.Storageoperations, error) {
		return &ControllerMock{}, nil
	})
	defer patch.Unpatch()

	resp, err := s.CreateVolume(context.Background(), createVolumeReq)
	assert.Nil(suite.T(), err, "nfs valume should ge fail")
	assert.NotNil(suite.T(), resp)
}

func (suite *ControllerTestSuite) Test_DeleteVolume_InvalidID() {
	deleteVolumeReq := getCtrDeleteVolumeRequest()
	deleteVolumeReq.VolumeId = "100"
	s := getService()
	_, err := s.DeleteVolume(context.Background(), deleteVolumeReq)
	assert.Nil(suite.T(), err, "DeleteVolume with invalid volume ID should be success")
}

func (suite *ControllerTestSuite) Test_DeleteVolume_invalidProtocol() {
	deleteVolumeReq := getCtrDeleteVolumeRequest()
	deleteVolumeReq.VolumeId = "100$$unknown"
	s := getService()
	_, err := s.DeleteVolume(context.Background(), deleteVolumeReq)
	assert.NotNil(suite.T(), err, "Invalid Protocol")
}

func (suite *ControllerTestSuite) Test_DeleteVolume_Success() {
	deleteVolumeReq := getCtrDeleteVolumeRequest()
	s := getService()

	patch := monkey.Patch(storage.NewStorageController, func(_ string, _ ...map[string]string) (storage.Storageoperations, error) {
		return &ControllerMock{}, nil
	})
	defer patch.Unpatch()

	_, err := s.DeleteVolume(context.Background(), deleteVolumeReq)
	assert.Nil(suite.T(), err, "Invalid volume ID")
}

/*
func (suite *ControllerTestSuite) Test_DeleteVolume_Error() {
	deleteVolumeReq := getCtrDeleteVolumeRequest()
	s := getService()
	_, err := s.DeleteVolume(context.Background(), deleteVolumeReq)
	assert.NotNil(suite.T(), err, "Invalid volume ID")
}
*/

func (suite *ControllerTestSuite) Test_ControllerPublishVolume_InvalidID() {
	crtPublishVolumeReq := getCrtControllerPublishVolumeRequest()
	crtPublishVolumeReq.VolumeId = "100$$unknown$$123"
	s := getService()
	_, err := s.ControllerPublishVolume(context.Background(), crtPublishVolumeReq)
	assert.NotNil(suite.T(), err, "Invalid volume ID")
}

func (suite *ControllerTestSuite) Test_ControllerPublishVolume_Invalid_protocol() {
	crtPublishVolumeReq := getCrtControllerPublishVolumeRequest()
	crtPublishVolumeReq.VolumeId = "100$$unknown"
	s := getService()
	_, err := s.ControllerPublishVolume(context.Background(), crtPublishVolumeReq)
	assert.NotNil(suite.T(), err, "Invalid volume protocol")
}

func (suite *ControllerTestSuite) Test_ControllerPublishVolume_success() {
	crtPublishVolumeReq := getCrtControllerPublishVolumeRequest()
	crtPublishVolumeReq.VolumeId = "100$$nfs"

	s := getService()
	patch := monkey.Patch(storage.NewStorageController, func(_ string, _ ...map[string]string) (storage.Storageoperations, error) {
		return &ControllerMock{}, nil
	})
	defer patch.Unpatch()

	_, err := s.ControllerPublishVolume(context.Background(), crtPublishVolumeReq)
	assert.Nil(suite.T(), err, "Invalid volume protocol")
}

func (suite *ControllerTestSuite) Test_ControllerUnpublishVolume_InvalidID() {
	crtUnPublishReq := getCrtControllerUnpublishVolume()
	crtUnPublishReq.VolumeId = "100"
	s := getService()
	_, err := s.ControllerUnpublishVolume(context.Background(), crtUnPublishReq)
	assert.NotNil(suite.T(), err, "Invalid volume ID")
}

func (suite *ControllerTestSuite) Test_ControllerUnpublishVolume_InvalidProtocol() {
	crtUnPublishReq := getCrtControllerUnpublishVolume()
	crtUnPublishReq.VolumeId = "100$$unknown"
	s := getService()
	_, err := s.ControllerUnpublishVolume(context.Background(), crtUnPublishReq)
	assert.NotNil(suite.T(), err, "Invalid volume Protocol")
}

func (suite *ControllerTestSuite) Test_ControllerUnpublishVolume_success() {
	crtUnPublishReq := getCrtControllerUnpublishVolume()
	crtUnPublishReq.VolumeId = "100$$nfs"
	s := getService()
	patch := monkey.Patch(storage.NewStorageController, func(_ string, _ ...map[string]string) (storage.Storageoperations, error) {
		return &ControllerMock{}, nil
	})
	defer patch.Unpatch()

	_, err := s.ControllerUnpublishVolume(context.Background(), crtUnPublishReq)
	assert.Nil(suite.T(), err, "Invalid volume Protocol")
}

func (suite *ControllerTestSuite) Test_CreateSnapshot_InvalidID() {
	crtCreateSnapshotReq := getCtrCreateSnapshotRequest()
	crtCreateSnapshotReq.SourceVolumeId = "100"
	s := getService()
	_, err := s.CreateSnapshot(context.Background(), crtCreateSnapshotReq)
	assert.NotNil(suite.T(), err, "Invalid volume ID")
}

func (suite *ControllerTestSuite) Test_CreateSnapshot_Invalid_protocol() {
	crtCreateSnapshotReq := getCtrCreateSnapshotRequest()
	crtCreateSnapshotReq.SourceVolumeId = "100$$unknown"
	s := getService()
	_, err := s.CreateSnapshot(context.Background(), crtCreateSnapshotReq)
	assert.NotNil(suite.T(), err, "Invalid protocol")
}

func (suite *ControllerTestSuite) Test_CreateSnapshot_success() {
	crtCreateSnapshotReq := getCtrCreateSnapshotRequest()
	crtCreateSnapshotReq.SourceVolumeId = "100$$nfs"
	s := getService()

	patch := monkey.Patch(storage.NewStorageController, func(_ string, _ ...map[string]string) (storage.Storageoperations, error) {
		return &ControllerMock{}, nil
	})
	defer patch.Unpatch()

	_, err := s.CreateSnapshot(context.Background(), crtCreateSnapshotReq)
	assert.Nil(suite.T(), err, "success protocol")
}

func (suite *ControllerTestSuite) Test_DeleteSnapshot_InvalidID() {
	crtDeleteSnapshotReq := getCtrDeleteSnapshotRequest()
	crtDeleteSnapshotReq.SnapshotId = "100"
	s := getService()
	_, err := s.DeleteSnapshot(context.Background(), crtDeleteSnapshotReq)
	assert.Nil(suite.T(), err, "DeleteSnapshot with invalid snapshot ID should be success")
}

func (suite *ControllerTestSuite) Test_DeleteSnapshot_Invalid_protocol() {
	crtDeleteSnapshotReq := getCtrDeleteSnapshotRequest()
	crtDeleteSnapshotReq.SnapshotId = "100$$unknown"
	s := getService()
	_, err := s.DeleteSnapshot(context.Background(), crtDeleteSnapshotReq)
	assert.NotNil(suite.T(), err, "Invalid protocol")
}

func (suite *ControllerTestSuite) Test_DeleteSnapshot_success() {
	crtDeleteSnapshotReq := getCtrDeleteSnapshotRequest()

	s := getService()
	patch := monkey.Patch(storage.NewStorageController, func(_ string, _ ...map[string]string) (storage.Storageoperations, error) {
		return &ControllerMock{}, nil
	})
	defer patch.Unpatch()

	_, err := s.DeleteSnapshot(context.Background(), crtDeleteSnapshotReq)
	assert.Nil(suite.T(), err, "Invalid protocol")
}

func (suite *ControllerTestSuite) Test_ControllerExpandVolume_InvalidID() {
	crtexpandReq := getCrtControllerExpandVolumeRequest()
	crtexpandReq.VolumeId = "100"
	s := getService()
	_, err := s.ControllerExpandVolume(context.Background(), crtexpandReq)
	assert.NotNil(suite.T(), err, "Invalid volume ID")
}

func (suite *ControllerTestSuite) Test_ControllerExpandVolume_Invalid_protocol() {
	crtexpandReq := getCrtControllerExpandVolumeRequest()
	crtexpandReq.VolumeId = "100$$unknow"
	s := getService()
	_, err := s.ControllerExpandVolume(context.Background(), crtexpandReq)
	assert.NotNil(suite.T(), err, "Invalid volume ID")
}

func (suite *ControllerTestSuite) Test_ControllerExpandVolume_success() {
	crtexpandReq := getCrtControllerExpandVolumeRequest()

	patch := monkey.Patch(storage.NewStorageController, func(_ string, _ ...map[string]string) (storage.Storageoperations, error) {
		return &ControllerMock{}, nil
	})
	defer patch.Unpatch()

	s := getService()
	_, err := s.ControllerExpandVolume(context.Background(), crtexpandReq)
	assert.Nil(suite.T(), err, "Invalid volume ID")
}

func (suite *ControllerTestSuite) Test_ControllerGetCapabilities_() {
	crtCapabilitiesReqReq := getCtrControllerGetCapabilitiesRequest()
	s := getService()
	_, err := s.ControllerGetCapabilities(context.Background(), crtCapabilitiesReqReq)
	assert.Nil(suite.T(), err, "Invalid volume ID")
}

func (suite *ControllerTestSuite) Test_ValidateVolumeCapabilities() {
	crtValidateVolumeCapabilitiesReq := getCtrValidateVolumeCapabilitiesRequest()
	s := getService()
	_, err := s.ValidateVolumeCapabilities(context.Background(), crtValidateVolumeCapabilitiesReq)
	assert.Nil(suite.T(), err, "Invalid volume ID")
}

func (suite *ControllerTestSuite) Test_ListVolumes() {
	s := getService()
	_, err := s.ListVolumes(context.Background(), &csi.ListVolumesRequest{})
	assert.NotNil(suite.T(), err, "Invalid volume ID")
}

func (suite *ControllerTestSuite) Test_ListSnapshots() {
	s := getService()
	_, err := s.ListSnapshots(context.Background(), &csi.ListSnapshotsRequest{})
	assert.NotNil(suite.T(), err, "Invalid volume ID")
}

func (suite *ControllerTestSuite) Test_GetCapacity() {
	s := getService()
	_, err := s.GetCapacity(context.Background(), &csi.GetCapacityRequest{})
	assert.Nil(suite.T(), err, "Invalid volume ID")
}

//=============================

func getCtrControllerGetCapabilitiesRequest() *csi.ControllerGetCapabilitiesRequest {
	return &csi.ControllerGetCapabilitiesRequest{}
}
func getCrtControllerExpandVolumeRequest() *csi.ControllerExpandVolumeRequest {
	return &csi.ControllerExpandVolumeRequest{
		VolumeId:      "100$$nfs",
		CapacityRange: &csi.CapacityRange{RequiredBytes: 10000},
		Secrets:       getSecret(),
	}
}

func getCtrDeleteSnapshotRequest() *csi.DeleteSnapshotRequest {
	return &csi.DeleteSnapshotRequest{
		SnapshotId: "100$$nfs",
		Secrets:    getSecret(),
	}
}
func getCtrCreateSnapshotRequest() *csi.CreateSnapshotRequest {
	return &csi.CreateSnapshotRequest{
		SourceVolumeId: "100$$nfs",
		Secrets:        getSecret(),
	}
}
func getCrtControllerUnpublishVolume() *csi.ControllerUnpublishVolumeRequest {
	return &csi.ControllerUnpublishVolumeRequest{
		VolumeId: "100$$nfs",
		Secrets:  getSecret(),
	}
}
func getCrtControllerPublishVolumeRequest() *csi.ControllerPublishVolumeRequest {
	return &csi.ControllerPublishVolumeRequest{
		VolumeId: "100$$nfs",
		NodeId:   "test$$10.20.30.50",
		Secrets:  getSecret(),
	}
}
func getCtrDeleteVolumeRequest() *csi.DeleteVolumeRequest {
	return &csi.DeleteVolumeRequest{
		VolumeId: "100$$nfs",
		Secrets:  getSecret(),
	}
}
func getCtrValidateVolumeCapabilitiesRequest() *csi.ValidateVolumeCapabilitiesRequest {
	return &csi.ValidateVolumeCapabilitiesRequest{
		VolumeId: "100$$nfs",
		Secrets:  getSecret(),
	}
}

func getContrCreateVolumeParamter() map[string]string {
	return map[string]string{"storage_protocol": "nfs", "pool_name": "pool_name1", "network_space": "network_space1", "nfs_export_permissions": "[{'access':'RW','client':'192.168.147.190-192.168.147.199','no_root_squash':false},{'access':'RW','client':'192.168.147.10-192.168.147.20','no_root_squash':'false'}]"}

}

func getSecret() map[string]string {
	secretMap := make(map[string]string)
	secretMap["username"] = "admin"
	secretMap["password"] = "123456"
	secretMap["hostname"] = "https://172.17.35.61/"
	return secretMap
}

func getControllerCreateVolumeRequest(name string, parameterMap map[string]string) *csi.CreateVolumeRequest {
	return &csi.CreateVolumeRequest{
		Name:          "volumeName",
		CapacityRange: &csi.CapacityRange{RequiredBytes: 1000},
		//VolumeCapabilities []*VolumeCapability
		Parameters:          parameterMap,
		Secrets:             getSecret(),
		VolumeContentSource: nil,
	}
}
func getControllerCreateVolumeReqonse() *csi.CreateVolumeResponse {
	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId: "100",
		},
	}
}

func getService() Service {
	configParam := make(map[string]string)
	configParam["nodeid"] = "10.20.30.50"
	configParam["drivername"] = "csi-driver"
	configParam["nodeIPAddress"] = "10.20.30.50"
	configParam["nodeName"] = "ubuntu"
	configParam["initiatorPrefix"] = "iscsi"
	configParam["hostclustername"] = "clusterName"
	configParam["driverversion"] = "1.1.0.5s"
	return New(configParam)
}
