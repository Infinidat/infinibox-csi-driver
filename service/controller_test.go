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
	tests "infinibox-csi-driver/test_helper"
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

func (suite *ControllerTestSuite) Test_CreateVolume_NoParameters_Fail() {
	var parameterMap map[string]string
	createVolumeReq := tests.GetCreateVolumeRequest("", parameterMap, "")
	s := getService()
	_, err := s.CreateVolume(context.Background(), createVolumeReq)
	assert.NotNil(suite.T(), err, "expected to fail: Controller CreateVolume no parameters")
}

func (suite *ControllerTestSuite) Test_CreateVolume_MissingStorageProtocol() {
	parameterMap := getControllerCreateVolumeParameters()
	delete(parameterMap, "storage_protocol")
	createVolumeReq := tests.GetCreateVolumeRequest("pvcName", parameterMap, "")
	s := getService()
	_, err := s.CreateVolume(context.Background(), createVolumeReq)
	assert.NotNil(suite.T(), err, "expected to fail: Controller CreateVolume storage protocol value missing")
}

func (suite *ControllerTestSuite) Test_CreateVolume_InvalidStorageProtocol() {
	parameterMap := getControllerCreateVolumeParameters()
	parameterMap["storage_protocol"] = "unknown"
	createVolumeReq := tests.GetCreateVolumeRequest("pvcName", parameterMap, "")
	s := getService()
	_, err := s.CreateVolume(context.Background(), createVolumeReq)
	assert.NotNil(suite.T(), err, "expected to fail: Controller CreateVolume storage_protocol invalid")
}

func (suite *ControllerTestSuite) Test_CreateVolume_No_VolumeCapabilities_fail() {
	parameterMap := getControllerCreateVolumeParameters()
	createVolumeReq := tests.GetCreateVolumeRequest("pvcName", parameterMap, "")
	createVolumeReq.VolumeCapabilities = nil // force volume caps to be empty

	s := getService()
	_, err := s.CreateVolume(context.Background(), createVolumeReq)
	assert.NotNil(suite.T(), err, "expected to fail: Controller CreateVolume no VolumeCapabilities")
}

func (suite *ControllerTestSuite) Test_CreateVolume_VolumeCapabilities_MultiNodeReadAccess_success() {
	parameterMap := getControllerCreateVolumeParameters()
	createVolumeReq := tests.GetCreateVolumeRequest("pvcName", parameterMap, "")

	// force volume caps with multi-node access
	capa := csi.VolumeCapability{
		AccessMode: &csi.VolumeCapability_AccessMode{
			Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY,
		},
	}
	var arr []*csi.VolumeCapability
	arr = append(arr, &capa)
	createVolumeReq.VolumeCapabilities = arr

	s := getService()
	patch := monkey.Patch(storage.NewStorageController, func(_ string, _ ...map[string]string) (storage.Storageoperations, error) {
		return &ControllerMock{}, nil
	})
	defer patch.Unpatch()

	_, err := s.CreateVolume(context.Background(), createVolumeReq)
	assert.Nil(suite.T(), err, "expected to succeed: Controller CreateVolume VolumeCapabilities with MULTI_NODE_READER access")
}

func (suite *ControllerTestSuite) Test_CreateVolume_UnmockedFail() {
	parameterMap := getControllerCreateVolumeParameters()
	createVolumeReq := tests.GetCreateVolumeRequest("pvcName", parameterMap, "")
	s := getService()
	_, err := s.CreateVolume(context.Background(), createVolumeReq)
	assert.NotNil(suite.T(), err, "expected to fail: Controller CreateVolume with unmocked Controller")
}

func (suite *ControllerTestSuite) Test_CreateVolume_success() {
	parameterMap := getControllerCreateVolumeParameters()
	createVolumeReq := tests.GetCreateVolumeRequest("pvcName", parameterMap, "")
	s := getService()

	patch := monkey.Patch(storage.NewStorageController, func(_ string, _ ...map[string]string) (storage.Storageoperations, error) {
		return &ControllerMock{}, nil
	})
	defer patch.Unpatch()

	resp, err := s.CreateVolume(context.Background(), createVolumeReq)
	assert.Nil(suite.T(), err, "expected to succeed: Controller CreateVolume")
	assert.NotNil(suite.T(), resp)
}

func (suite *ControllerTestSuite) Test_DeleteVolume_InvalidID_success() {
	deleteVolumeReq := getControllerDeleteVolumeRequest()
	deleteVolumeReq.VolumeId = "100"
	s := getService()
	_, err := s.DeleteVolume(context.Background(), deleteVolumeReq)
	assert.Nil(suite.T(), err, "expected to succeed: Controller DeleteVolume with invalid volume ID")
}

func (suite *ControllerTestSuite) Test_DeleteVolume_InvalidProtocol() {
	deleteVolumeReq := getControllerDeleteVolumeRequest()
	deleteVolumeReq.VolumeId = "100$$unknown"
	s := getService()
	_, err := s.DeleteVolume(context.Background(), deleteVolumeReq)
	assert.NotNil(suite.T(), err, "expected to fail: Controller DeleteVolume with Invalid Protocol")
}

func (suite *ControllerTestSuite) Test_DeleteVolume_Success() {
	deleteVolumeReq := getControllerDeleteVolumeRequest()
	s := getService()

	patch := monkey.Patch(storage.NewStorageController, func(_ string, _ ...map[string]string) (storage.Storageoperations, error) {
		return &ControllerMock{}, nil
	})
	defer patch.Unpatch()

	_, err := s.DeleteVolume(context.Background(), deleteVolumeReq)
	assert.Nil(suite.T(), err, "expected to succeed: Controller DeleteVolume")
}

func (suite *ControllerTestSuite) Test_ControllerPublishVolume_InvalidID() {
	publishVolReq := getControllerPublishVolumeRequest()
	publishVolReq.VolumeId = "100$$unknown$$123"
	s := getService()
	_, err := s.ControllerPublishVolume(context.Background(), publishVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: Controller PublishVolume Invalid volume ID format")
}

func (suite *ControllerTestSuite) Test_ControllerPublishVolume_Invalid_protocol() {
	publishVolReq := getControllerPublishVolumeRequest()
	publishVolReq.VolumeId = "100$$unknown"
	s := getService()
	_, err := s.ControllerPublishVolume(context.Background(), publishVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: Controller PublishVolume Invalid volume ID protocol")
}

func (suite *ControllerTestSuite) Test_ControllerPublishVolume_success() {
	publishVolReq := getControllerPublishVolumeRequest()
	publishVolReq.VolumeId = "100$$nfs"
	s := getService()

	patch := monkey.Patch(storage.NewStorageController, func(_ string, _ ...map[string]string) (storage.Storageoperations, error) {
		return &ControllerMock{}, nil
	})
	defer patch.Unpatch()

	_, err := s.ControllerPublishVolume(context.Background(), publishVolReq)
	assert.Nil(suite.T(), err, "expected to succeed: Controller PublishVolume")
}

func (suite *ControllerTestSuite) Test_ControllerUnpublishVolume_InvalidID() {
	unpublishVolReq := getControllerUnpublishVolumeRequest()
	unpublishVolReq.VolumeId = "100"
	s := getService()
	_, err := s.ControllerUnpublishVolume(context.Background(), unpublishVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: Controller UnpublishVolume Invalid volume ID format")
}

func (suite *ControllerTestSuite) Test_ControllerUnpublishVolume_InvalidProtocol() {
	unpublishVolReq := getControllerUnpublishVolumeRequest()
	unpublishVolReq.VolumeId = "100$$unknown"
	s := getService()
	_, err := s.ControllerUnpublishVolume(context.Background(), unpublishVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: Controller UnpublishVolume Invalid volume ID protocol")
}

func (suite *ControllerTestSuite) Test_ControllerUnpublishVolume_success() {
	unpublishVolReq := getControllerUnpublishVolumeRequest()
	unpublishVolReq.VolumeId = "100$$nfs"
	s := getService()

	patch := monkey.Patch(storage.NewStorageController, func(_ string, _ ...map[string]string) (storage.Storageoperations, error) {
		return &ControllerMock{}, nil
	})
	defer patch.Unpatch()

	_, err := s.ControllerUnpublishVolume(context.Background(), unpublishVolReq)
	assert.Nil(suite.T(), err, "expected to succeed: Controller UnpublishVolume")
}

func (suite *ControllerTestSuite) Test_CreateSnapshot_InvalidID() {
	createSnapshotReq := getControllerCreateSnapshotRequest()
	createSnapshotReq.SourceVolumeId = "100"
	s := getService()
	_, err := s.CreateSnapshot(context.Background(), createSnapshotReq)
	assert.NotNil(suite.T(), err, "expected to fail: Controller CreateSnapshot Invalid volume Id format")
}

func (suite *ControllerTestSuite) Test_CreateSnapshot_Invalid_protocol() {
	createSnapshotReq := getControllerCreateSnapshotRequest()
	createSnapshotReq.SourceVolumeId = "100$$unknown"
	s := getService()
	_, err := s.CreateSnapshot(context.Background(), createSnapshotReq)
	assert.NotNil(suite.T(), err, "expected to fail: Controller CreateSnapshot invalid Volume Id protocol")
}

func (suite *ControllerTestSuite) Test_CreateSnapshot_success() {
	createSnapshotReq := getControllerCreateSnapshotRequest()
	createSnapshotReq.SourceVolumeId = "100$$nfs"
	s := getService()

	patch := monkey.Patch(storage.NewStorageController, func(_ string, _ ...map[string]string) (storage.Storageoperations, error) {
		return &ControllerMock{}, nil
	})
	defer patch.Unpatch()

	_, err := s.CreateSnapshot(context.Background(), createSnapshotReq)
	assert.Nil(suite.T(), err, "expected to succeed: Controller CreateSnapshot")
}

func (suite *ControllerTestSuite) Test_DeleteSnapshot_InvalidID_success() {
	deleteSnapshotReq := getControllerDeleteSnapshotRequest()
	deleteSnapshotReq.SnapshotId = "100"
	s := getService()
	_, err := s.DeleteSnapshot(context.Background(), deleteSnapshotReq)
	assert.Nil(suite.T(), err, "expected to succeed: Controller DeleteSnapshot invalid snapshot ID")
}

func (suite *ControllerTestSuite) Test_DeleteSnapshot_Invalid_protocol() {
	deleteSnapshotReq := getControllerDeleteSnapshotRequest()
	deleteSnapshotReq.SnapshotId = "100$$unknown"
	s := getService()
	_, err := s.DeleteSnapshot(context.Background(), deleteSnapshotReq)
	assert.NotNil(suite.T(), err, "expected to fail: Controller DeleteSnapshot Invalid SnapshotId protocol")
}

func (suite *ControllerTestSuite) Test_DeleteSnapshot_success() {
	deleteSnapshotReq := getControllerDeleteSnapshotRequest()
	s := getService()

	patch := monkey.Patch(storage.NewStorageController, func(_ string, _ ...map[string]string) (storage.Storageoperations, error) {
		return &ControllerMock{}, nil
	})
	defer patch.Unpatch()

	_, err := s.DeleteSnapshot(context.Background(), deleteSnapshotReq)
	assert.Nil(suite.T(), err, "expected to succeed: Controller DeleteSnapshot")
}

func (suite *ControllerTestSuite) Test_ControllerExpandVolume_InvalidID() {
	expandVolReq := getControllerExpandVolumeRequest()
	expandVolReq.VolumeId = "100"
	s := getService()
	_, err := s.ControllerExpandVolume(context.Background(), expandVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: Controller ExpandVolume volume ID invalid format")
}

func (suite *ControllerTestSuite) Test_ControllerExpandVolume_Invalid_protocol() {
	expandVolReq := getControllerExpandVolumeRequest()
	expandVolReq.VolumeId = "100$$unknown"
	s := getService()
	_, err := s.ControllerExpandVolume(context.Background(), expandVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: Controller ExpandVolume volume ID invalid protocol")
}

func (suite *ControllerTestSuite) Test_ControllerExpandVolume_success() {
	expandVolReq := getControllerExpandVolumeRequest()
	s := getService()

	patch := monkey.Patch(storage.NewStorageController, func(_ string, _ ...map[string]string) (storage.Storageoperations, error) {
		return &ControllerMock{}, nil
	})
	defer patch.Unpatch()

	_, err := s.ControllerExpandVolume(context.Background(), expandVolReq)
	assert.Nil(suite.T(), err, "expected to succeed: Controller ExpandVolume")
}

func (suite *ControllerTestSuite) Test_ControllerGetCapabilities_success() {
	controllerGetCapsReq := getControllerGetCapabilitiesRequest()
	s := getService()

	_, err := s.ControllerGetCapabilities(context.Background(), controllerGetCapsReq)
	assert.Nil(suite.T(), err, "expected to succeed: Controller GetCapabilities")
}

func (suite *ControllerTestSuite) Test_ValidateVolumeCapabilities() {
	validateVolCapsReq := getControllerValidateVolumeCapabilitiesRequest()
	s := getService()

	patch := monkey.Patch(storage.NewStorageController, func(_ string, _ ...map[string]string) (storage.Storageoperations, error) {
		return &ControllerMock{}, nil
	})
	defer patch.Unpatch()

	_, err := s.ValidateVolumeCapabilities(context.Background(), validateVolCapsReq)
	assert.Nil(suite.T(), err, "expected to succeed: Controller ValidateVolumeCapabilities")
}

func (suite *ControllerTestSuite) Test_ListVolumes_unimplemented() {
	s := getService()
	_, err := s.ListVolumes(context.Background(), &csi.ListVolumesRequest{})
	assert.NotNil(suite.T(), err, "expected to fail: Controller ListVolumes unimplemented")
}

func (suite *ControllerTestSuite) Test_ListSnapshots_unimplemented() {
	s := getService()
	_, err := s.ListSnapshots(context.Background(), &csi.ListSnapshotsRequest{})
	assert.NotNil(suite.T(), err, "expected to fail: Controller ListSnapshots unimplemented")
}

func (suite *ControllerTestSuite) Test_GetCapacity_unimplemented() {
	s := getService()
	_, err := s.GetCapacity(context.Background(), &csi.GetCapacityRequest{})
	assert.NotNil(suite.T(), err, "expected to fail: Controller GetCapacity unimplemented")
}

//=============================

func getControllerGetCapabilitiesRequest() *csi.ControllerGetCapabilitiesRequest {
	return &csi.ControllerGetCapabilitiesRequest{}
}

func getControllerExpandVolumeRequest() *csi.ControllerExpandVolumeRequest {
	return &csi.ControllerExpandVolumeRequest{
		VolumeId:      "100$$nfs",
		CapacityRange: &csi.CapacityRange{RequiredBytes: 10000},
		Secrets:       tests.GetSecret(),
	}
}

func getControllerDeleteSnapshotRequest() *csi.DeleteSnapshotRequest {
	return &csi.DeleteSnapshotRequest{
		SnapshotId: "100$$nfs",
		Secrets:    tests.GetSecret(),
	}
}

func getControllerCreateSnapshotRequest() *csi.CreateSnapshotRequest {
	return &csi.CreateSnapshotRequest{
		SourceVolumeId: "100$$nfs",
		Secrets:        tests.GetSecret(),
	}
}

func getControllerUnpublishVolumeRequest() *csi.ControllerUnpublishVolumeRequest {
	return &csi.ControllerUnpublishVolumeRequest{
		VolumeId: "100$$nfs",
		Secrets:  tests.GetSecret(),
	}
}

func getControllerPublishVolumeRequest() *csi.ControllerPublishVolumeRequest {
	return &csi.ControllerPublishVolumeRequest{
		VolumeId: "100$$nfs",
		NodeId:   "test$$10.20.30.50",
		Secrets:  tests.GetSecret(),
	}
}

func getControllerDeleteVolumeRequest() *csi.DeleteVolumeRequest {
	return &csi.DeleteVolumeRequest{
		VolumeId: "100$$nfs",
		Secrets:  tests.GetSecret(),
	}
}

func getControllerValidateVolumeCapabilitiesRequest() *csi.ValidateVolumeCapabilitiesRequest {
	return &csi.ValidateVolumeCapabilitiesRequest{
		VolumeId: "100$$nfs",
		Secrets:  tests.GetSecret(),
	}
}

func getControllerCreateVolumeParameters() map[string]string {
	return map[string]string{"storage_protocol": "nfs", "pool_name": "pool_name1", "network_space": "network_space1", "nfs_export_permissions": "[{'access':'RW','client':'192.168.147.190-192.168.147.199','no_root_squash':false},{'access':'RW','client':'192.168.147.10-192.168.147.20','no_root_squash':'false'}]"}
}

func getService() Service {
	configParam := make(map[string]string)
	configParam["nodeid"] = "10.20.30.50"
	configParam["nodeName"] = "ubuntu"
	configParam["drivername"] = "csi-driver"
	configParam["driverversion"] = "1.1.0.5s"
	return New(configParam)
}
