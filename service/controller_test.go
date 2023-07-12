//go:build unit

/*Copyright 2022 Infinidat
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
	"infinibox-csi-driver/api"
	"infinibox-csi-driver/common"
	"infinibox-csi-driver/helper"
	"infinibox-csi-driver/storage"
	tests "infinibox-csi-driver/test_helper"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type ControllerTestSuite struct {
	suite.Suite
	api            *api.MockApiService
	accessMock     *helper.MockAccessModesHelper
	cs             *storage.Commonservice
	mockController *ControllerMock
}

func (suite *ControllerTestSuite) SetupTest() {
	suite.mockController = &ControllerMock{}
	x := new(api.MockApiService)
	suite.api = x
	suite.accessMock = new(helper.MockAccessModesHelper)
	suite.cs = &storage.Commonservice{
		Api:               x,
		AccessModesHelper: suite.accessMock,
	}

}
func TestControllerTestSuite(t *testing.T) {
	suite.Run(t, new(ControllerTestSuite))
}

func (suite *ControllerTestSuite) Test_CreateVolume_NoParameters_Fail() {
	var parameterMap map[string]string
	createVolumeReq := tests.GetCreateVolumeRequest("", parameterMap, "")
	cs := ControllerServer{}
	_, err := cs.CreateVolume(context.Background(), createVolumeReq)
	assert.NotNil(suite.T(), err, "expected to fail: Controller CreateVolume no parameters")
}

func (suite *ControllerTestSuite) Test_CreateVolume_MissingStorageProtocol() {
	parameterMap := getControllerCreateVolumeParameters()
	delete(parameterMap, common.SC_STORAGE_PROTOCOL)
	createVolumeReq := tests.GetCreateVolumeRequest("pvcName", parameterMap, "")
	cs := ControllerServer{
		Driver: &Driver{
			nodeID: "n",
		},
	}
	_, err := cs.CreateVolume(context.Background(), createVolumeReq)
	assert.NotNil(suite.T(), err, "expected to fail: Controller CreateVolume storage protocol value missing")
}

func (suite *ControllerTestSuite) Test_CreateVolume_InvalidStorageProtocol() {
	parameterMap := getControllerCreateVolumeParameters()
	parameterMap[common.SC_STORAGE_PROTOCOL] = "unknown"
	createVolumeReq := tests.GetCreateVolumeRequest("pvcName", parameterMap, "")
	cs := ControllerServer{
		Driver: &Driver{
			nodeID: "n",
		},
	}
	_, err := cs.CreateVolume(context.Background(), createVolumeReq)
	assert.NotNil(suite.T(), err, "expected to fail: Controller CreateVolume storage_protocol invalid")
}

func (suite *ControllerTestSuite) Test_CreateVolume_No_VolumeCapabilities_fail() {
	parameterMap := getControllerCreateVolumeParameters()
	createVolumeReq := tests.GetCreateVolumeRequest("pvcName", parameterMap, "")
	createVolumeReq.VolumeCapabilities = nil // force volume caps to be empty
	cs := ControllerServer{
		Driver: &Driver{
			nodeID: "n",
		},
	}
	_, err := cs.CreateVolume(context.Background(), createVolumeReq)
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
	_, err := suite.mockController.CreateVolume(context.Background(), createVolumeReq)
	assert.Nil(suite.T(), err, "expected to succeed: Controller CreateVolume VolumeCapabilities with MULTI_NODE_READER access")
}
func getNetworkSpace() api.NetworkSpace {
	portalArry := []api.Portal{{IpAdress: "10.20.20.50"}}
	return api.NetworkSpace{Portals: portalArry, Service: common.NS_NFS_SVC}
}

func (suite *ControllerTestSuite) Test_CreateVolume_success() {
	parameterMap := getControllerCreateVolumeParameters()
	createVolumeReq := tests.GetCreateVolumeRequest("pvcName", parameterMap, "")

	resp, err := suite.mockController.CreateVolume(context.Background(), createVolumeReq)
	assert.Nil(suite.T(), err, "expected to succeed: Controller CreateVolume")
	assert.NotNil(suite.T(), resp)
}

func (suite *ControllerTestSuite) Test_DeleteVolume_InvalidID_success() {
	deleteVolumeReq := getControllerDeleteVolumeRequest()
	deleteVolumeReq.VolumeId = "100"
	cs := ControllerServer{
		Driver: &Driver{
			nodeID: "na",
		},
	}
	_, err := cs.DeleteVolume(context.Background(), deleteVolumeReq)
	assert.Nil(suite.T(), err, "expected to succeed: Controller DeleteVolume with invalid volume ID")
}

func (suite *ControllerTestSuite) Test_DeleteVolume_InvalidProtocol() {
	deleteVolumeReq := getControllerDeleteVolumeRequest()
	deleteVolumeReq.VolumeId = "100$$unknown"
	cs := ControllerServer{
		Driver: &Driver{
			nodeID: "na",
		},
	}
	_, err := cs.DeleteVolume(context.Background(), deleteVolumeReq)
	assert.NotNil(suite.T(), err, "expected to fail: Controller DeleteVolume with Invalid Protocol")
}

func (suite *ControllerTestSuite) Test_DeleteVolume_Success() {
	deleteVolumeReq := getControllerDeleteVolumeRequest()
	_, err := suite.mockController.DeleteVolume(context.Background(), deleteVolumeReq)
	assert.Nil(suite.T(), err, "expected to succeed: Controller DeleteVolume")
}

func (suite *ControllerTestSuite) Test_ControllerPublishVolume_InvalidID() {
	publishVolReq := getControllerPublishVolumeRequest()
	publishVolReq.VolumeId = "100$$unknown$$123"
	cs := ControllerServer{
		Driver: &Driver{
			nodeID: "na",
		},
	}
	_, err := cs.ControllerPublishVolume(context.Background(), publishVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: Controller PublishVolume Invalid volume ID format")
}

func (suite *ControllerTestSuite) Test_ControllerPublishVolume_Invalid_protocol() {
	publishVolReq := getControllerPublishVolumeRequest()
	publishVolReq.VolumeId = "100$$unknown"
	cs := ControllerServer{
		Driver: &Driver{
			nodeID: "na",
		},
	}
	_, err := cs.ControllerPublishVolume(context.Background(), publishVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: Controller PublishVolume Invalid volume ID protocol")
}

func (suite *ControllerTestSuite) Test_ControllerPublishVolume_success() {
	publishVolReq := getControllerPublishVolumeRequest()
	publishVolReq.VolumeId = "100$$nfs"

	_, err := suite.mockController.ControllerPublishVolume(context.Background(), publishVolReq)
	assert.Nil(suite.T(), err, "expected to succeed: Controller PublishVolume")
}

func (suite *ControllerTestSuite) Test_ControllerUnpublishVolume_InvalidID() {
	unpublishVolReq := getControllerUnpublishVolumeRequest()
	unpublishVolReq.VolumeId = "100"
	cs := ControllerServer{
		Driver: &Driver{
			nodeID: "na",
		},
	}
	_, err := cs.ControllerUnpublishVolume(context.Background(), unpublishVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: Controller UnpublishVolume Invalid volume ID format")
}

func (suite *ControllerTestSuite) Test_ControllerUnpublishVolume_InvalidProtocol() {
	unpublishVolReq := getControllerUnpublishVolumeRequest()
	unpublishVolReq.VolumeId = "100$$unknown"
	cs := ControllerServer{
		Driver: &Driver{
			nodeID: "na",
		},
	}
	_, err := cs.ControllerUnpublishVolume(context.Background(), unpublishVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: Controller UnpublishVolume Invalid volume ID protocol")
}

func (suite *ControllerTestSuite) Test_ControllerUnpublishVolume_success() {
	unpublishVolReq := getControllerUnpublishVolumeRequest()
	unpublishVolReq.VolumeId = "100$$nfs"
	_, err := suite.mockController.ControllerUnpublishVolume(context.Background(), unpublishVolReq)
	assert.Nil(suite.T(), err, "expected to succeed: Controller UnpublishVolume")
}

func (suite *ControllerTestSuite) Test_CreateSnapshot_InvalidID() {
	createSnapshotReq := getControllerCreateSnapshotRequest()
	createSnapshotReq.SourceVolumeId = "100"
	cs := ControllerServer{
		Driver: &Driver{
			nodeID: "na",
		},
	}
	_, err := cs.CreateSnapshot(context.Background(), createSnapshotReq)
	assert.NotNil(suite.T(), err, "expected to fail: Controller CreateSnapshot Invalid volume Id format")
}

func (suite *ControllerTestSuite) Test_CreateSnapshot_Invalid_protocol() {
	createSnapshotReq := getControllerCreateSnapshotRequest()
	createSnapshotReq.SourceVolumeId = "100$$unknown"
	cs := ControllerServer{
		Driver: &Driver{
			nodeID: "na",
		},
	}
	_, err := cs.CreateSnapshot(context.Background(), createSnapshotReq)
	assert.NotNil(suite.T(), err, "expected to fail: Controller CreateSnapshot invalid Volume Id protocol")
}

func (suite *ControllerTestSuite) Test_CreateSnapshot_success() {
	createSnapshotReq := getControllerCreateSnapshotRequest()
	createSnapshotReq.SourceVolumeId = "100$$nfs"
	_, err := suite.mockController.CreateSnapshot(context.Background(), createSnapshotReq)
	assert.Nil(suite.T(), err, "expected to succeed: Controller CreateSnapshot")
}

func (suite *ControllerTestSuite) Test_DeleteSnapshot_InvalidID_success() {
	deleteSnapshotReq := getControllerDeleteSnapshotRequest()
	deleteSnapshotReq.SnapshotId = "100"
	cs := ControllerServer{
		Driver: &Driver{
			nodeID: "na",
		},
	}
	_, err := cs.DeleteSnapshot(context.Background(), deleteSnapshotReq)
	assert.Nil(suite.T(), err, "expected to succeed: Controller DeleteSnapshot invalid snapshot ID")
}

func (suite *ControllerTestSuite) Test_DeleteSnapshot_Invalid_protocol() {
	deleteSnapshotReq := getControllerDeleteSnapshotRequest()
	deleteSnapshotReq.SnapshotId = "100$$unknown"
	cs := ControllerServer{
		Driver: &Driver{
			nodeID: "na",
		},
	}
	_, err := cs.DeleteSnapshot(context.Background(), deleteSnapshotReq)
	assert.NotNil(suite.T(), err, "expected to fail: Controller DeleteSnapshot Invalid SnapshotId protocol")
}

func (suite *ControllerTestSuite) Test_DeleteSnapshot_success() {
	deleteSnapshotReq := getControllerDeleteSnapshotRequest()
	_, err := suite.mockController.DeleteSnapshot(context.Background(), deleteSnapshotReq)

	assert.Nil(suite.T(), err, "expected to succeed: Controller DeleteSnapshot")
}

func (suite *ControllerTestSuite) Test_ControllerExpandVolume_InvalidID() {
	expandVolReq := getControllerExpandVolumeRequest()
	expandVolReq.VolumeId = "100"
	cs := ControllerServer{
		Driver: &Driver{
			nodeID: "na",
		},
	}
	_, err := cs.ControllerExpandVolume(context.Background(), expandVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: Controller ExpandVolume volume ID invalid format")
}

func (suite *ControllerTestSuite) Test_ControllerExpandVolume_Invalid_protocol() {
	expandVolReq := getControllerExpandVolumeRequest()
	expandVolReq.VolumeId = "100$$unknown"
	cs := ControllerServer{
		Driver: &Driver{
			nodeID: "na",
		},
	}
	_, err := cs.ControllerExpandVolume(context.Background(), expandVolReq)
	assert.NotNil(suite.T(), err, "expected to fail: Controller ExpandVolume volume ID invalid protocol")
}

func (suite *ControllerTestSuite) Test_ControllerExpandVolume_success() {
	expandVolReq := getControllerExpandVolumeRequest()
	_, err := suite.mockController.ControllerExpandVolume(context.Background(), expandVolReq)
	assert.Nil(suite.T(), err, "expected to succeed: Controller ExpandVolume")
}

func (suite *ControllerTestSuite) Test_ControllerGetCapabilities_success() {
	controllerGetCapsReq := getControllerGetCapabilitiesRequest()
	cs := ControllerServer{
		Driver: &Driver{
			nodeID: "na",
		},
	}

	_, err := cs.ControllerGetCapabilities(context.Background(), controllerGetCapsReq)
	assert.Nil(suite.T(), err, "expected to succeed: Controller GetCapabilities")
}

func (suite *ControllerTestSuite) Test_ValidateVolumeCapabilities() {
	validateVolCapsReq := getControllerValidateVolumeCapabilitiesRequest()
	_, err := suite.mockController.ValidateVolumeCapabilities(context.Background(), validateVolCapsReq)
	assert.Nil(suite.T(), err, "expected to succeed: Controller ValidateVolumeCapabilities")
}

func (suite *ControllerTestSuite) Test_ListVolumes_unimplemented() {
	cs := ControllerServer{
		Driver: &Driver{
			nodeID: "na",
		},
	}
	_, err := cs.ListVolumes(context.Background(), &csi.ListVolumesRequest{})
	assert.NotNil(suite.T(), err, "expected to fail: Controller ListVolumes unimplemented")
}

func (suite *ControllerTestSuite) Test_ListSnapshots_unimplemented() {
	cs := ControllerServer{
		Driver: &Driver{
			nodeID: "na",
		},
	}
	_, err := cs.ListSnapshots(context.Background(), &csi.ListSnapshotsRequest{})
	assert.NotNil(suite.T(), err, "expected to fail: Controller ListSnapshots unimplemented")
}

func (suite *ControllerTestSuite) Test_GetCapacity_unimplemented() {
	cs := ControllerServer{
		Driver: &Driver{
			nodeID: "na",
		},
	}
	_, err := cs.GetCapacity(context.Background(), &csi.GetCapacityRequest{})
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
	return map[string]string{
		common.SC_STORAGE_PROTOCOL:       "nfs",
		common.SC_POOL_NAME:              "pool_name1",
		common.SC_NETWORK_SPACE:          "network_space1",
		common.SC_NFS_EXPORT_PERMISSIONS: "[{'access':'RW','client':'192.168.147.190-192.168.147.199','no_root_squash':false},{'access':'RW','client':'192.168.147.10-192.168.147.20','no_root_squash':'false'}]"}
}
