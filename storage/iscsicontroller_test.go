package storage

import (
	"context"
	"errors"
	"infinibox-csi-driver/api"
	"testing"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

func (suite *ISCSIControllerSuite) SetupTest() {
	suite.api = new(api.MockApiService)
	suite.cs = &commonservice{api: suite.api}
}

type ISCSIControllerSuite struct {
	suite.Suite
	api *api.MockApiService
	cs  *commonservice
}

func TestIscsiControllerSuite(t *testing.T) {
	suite.Run(t, new(ISCSIControllerSuite))
}

func (suite *ISCSIControllerSuite) Test_IscsiControllerExpandVolume_VolumeID_empty() {
	service := iscsistorage{cs: *suite.cs}
	_, err := service.ControllerExpandVolume(context.Background(), getIscsiExpandVolumeRequest(""))
	assert.NotNil(suite.T(), err, "Volume ID missing in request")
}

func (suite *ISCSIControllerSuite) Test_IscsiControllerExpandVolume_InvalidVolumeID() {
	volumeID := "100"
	service := iscsistorage{cs: *suite.cs}
	_, err := service.ControllerExpandVolume(context.Background(), getIscsiExpandVolumeRequest(volumeID))
	assert.NotNil(suite.T(), err, "Volume ID missing in request")
}

func (suite *ISCSIControllerSuite) Test_IscsiControllerExpandVolume_Error() {
	volumeID := "100#"
	volume := api.Volume{}
	expectedErr := errors.New("Some error")
	suite.api.On("UpdateVolume", volumeID, volume).Return(expectedErr)
	service := iscsistorage{cs: *suite.cs}
	_, err := service.ControllerExpandVolume(context.Background(), getIscsiExpandVolumeRequest(volumeID))
	assert.NotNil(suite.T(), err, "error expected")
}

func (suite *ISCSIControllerSuite) Test_IscsiControllerExpandVolume_Error_filenotfound() {
	service := iscsistorage{cs: *suite.cs}
	volumeID := "100#"
	volume := api.Volume{}
	expectedErr := errors.New("Some error")
	suite.api.On("UpdateVolume", volumeID, volume).Return(expectedErr)
	_, err := service.ControllerExpandVolume(context.Background(), getIscsiExpandVolumeRequest(volumeID))
	assert.NotNil(suite.T(), err, "error expected")
}

func (suite *ISCSIControllerSuite) Test_IscsiControllerExpandVolume_success() {
	service := iscsistorage{cs: *suite.cs}
	volumeID := "100#"
	volume := api.Volume{}
	suite.api.On("UpdateVolume", volumeID, volume).Return(nil)
	_, err := service.ControllerExpandVolume(context.Background(), getIscsiExpandVolumeRequest(volumeID))
	assert.NotNil(suite.T(), err, "error expected")
}

func (suite *ISCSIControllerSuite) Test_IscsiCreateSnapshot_GetSnapshot_Error() {
	expectedErr := errors.New("Snapshot Name is must")
	suite.api.On("GetVolumeByName", mock.Anything).Return(nil, expectedErr)
	service := iscsistorage{cs: *suite.cs}
	_, err := service.CreateSnapshot(context.Background(), getIscsiCreateSnapshotRequest("100"))
	assert.NotNil(suite.T(), err, "empty error")
}

func (suite *ISCSIControllerSuite) Test_IscsiCreateSnapshot_SourceVolumeID_Error() {
	volume := api.Volume{}
	expectedErr := errors.New("SourceVolumeID is must")
	suite.api.On("GetVolumeByName", mock.Anything).Return(volume, nil)
	suite.api.On("CreateSnapshotVolume", mock.Anything).Return(nil, expectedErr)
	service := iscsistorage{cs: *suite.cs}
	_, err := service.CreateSnapshot(context.Background(), getIscsiCreateSnapshotRequest("100"))
	assert.NotNil(suite.T(), err, "empty error")
}

func (suite *ISCSIControllerSuite) Test_IscsiCreateSnapshot_Success() {
	volume := api.Volume{}
	snapshotParam := &api.VolumeSnapshot{
		ParentID:       100,
		SnapshotName:   "test_snapshot",
		WriteProtected: true,
	}
	expectedErr := errors.New("SourceVolumeID is must")
	suite.api.On("GetVolumeByName", mock.Anything).Return(volume, nil)
	suite.api.On("CreateSnapshotVolume", snapshotParam).Return(nil, expectedErr)
	service := iscsistorage{cs: *suite.cs}
	result, err := service.CreateSnapshot(context.Background(), getIscsiCreateSnapshotRequest("100"))
	assert.NotNil(suite.T(), err, "empty error")
	val := result.GetSnapshot().GetSnapshotId()
	assert.Equal(suite.T(), "", val, "ID shoulde be equal")
}

func (suite *ISCSIControllerSuite) Test_IscsiDeleteSnapshot_SourceVolumeID_empty() {
	service := iscsistorage{cs: *suite.cs}
	expectedErr := errors.New("VOLUME_NOT_FOUND")
	suite.api.On("GetVolume", 0).Return(nil, expectedErr)
	_, err := service.DeleteSnapshot(context.Background(), getIscsiDeleteSnapshotRequest(""))
	assert.NotNil(suite.T(), err, "Source Volume ID missing in request")
}

func (suite *ISCSIControllerSuite) Test_IscsiDeleteSnapshot_InvalidSourceVolumeID() {
	service := iscsistorage{cs: *suite.cs}
	snapshotID := "1000000000000000000"
	volumeID := 1000000000000000000
	expectedErr := errors.New("VOLUME_NOT_FOUND")
	suite.api.On("GetVolume", volumeID).Return(nil, expectedErr)
	_, err := service.DeleteSnapshot(context.Background(), getIscsiDeleteSnapshotRequest(snapshotID))
	assert.NotNil(suite.T(), err, "Invalid Snapshot ID in request")
}

func (suite *ISCSIControllerSuite) Test_IscsiDeleteSnapshot_Error() {
	service := iscsistorage{cs: *suite.cs}
	snapshotID := "100"
	volumeID := 100
	expectedErr := errors.New("VOLUME_NOT_FOUND")
	suite.api.On("GetVolume", volumeID).Return(nil, expectedErr)
	_, err := service.DeleteSnapshot(context.Background(), getIscsiDeleteSnapshotRequest(snapshotID))
	assert.NotNil(suite.T(), err, "error expected")
}

func (suite *ISCSIControllerSuite) Test_IscsiValidateDeleteVolume_GetVolume_error() {
	service := iscsistorage{cs: *suite.cs}
	snapshotID := 100
	expectedErr := errors.New("VOLUME_NOT_FOUND")
	suite.api.On("GetVolume", snapshotID).Return(nil, expectedErr)
	err := service.ValidateDeleteVolume(snapshotID)
	assert.NotNil(suite.T(), err, "Error should not be nil")
	assert.Equal(suite.T(), expectedErr, err, "Error not returned as expected")
}

func (suite *ISCSIControllerSuite) Test_IscsiValidateDeleteVolume_GetVolume_InvalidID() {
	service := iscsistorage{cs: *suite.cs}
	snapshotID := 100999999999999
	expectedErr := errors.New("Invalid_ID")
	suite.api.On("GetVolume", snapshotID).Return(nil, expectedErr)
	err := service.ValidateDeleteVolume(snapshotID)
	assert.NotNil(suite.T(), err, "Error should not be nil")
}

//GetVolumeSnapshotByParentID
func (suite *ISCSIControllerSuite) Test_IscsiValidateDeleteVolume_GetVolumeSnapshotByParentID_error() {
	service := iscsistorage{cs: *suite.cs}
	snapshotID := 100
	volume := api.Volume{}
	expectedErr := errors.New("some error")
	suite.api.On("GetVolume", snapshotID).Return(volume, nil)
	suite.api.On("GetVolumeSnapshotByParentID", volume.ParentId).Return(nil, expectedErr)
	err := service.ValidateDeleteVolume(snapshotID)
	assert.NotNil(suite.T(), err, "Error should not be nil")
	assert.Equal(suite.T(), expectedErr, err, "Error not returned as expected")
}

func (suite *ISCSIControllerSuite) Test_IscsiValidateDeleteVolume_DeleteVolume_error() {
	service := iscsistorage{cs: *suite.cs}
	snapshotID := 100
	volume := api.Volume{ID: 100}
	volumes := []api.Volume{}
	expectedErr := errors.New("Volume not found")
	suite.api.On("GetVolume", snapshotID).Return(volume, nil)
	suite.api.On("GetVolumeSnapshotByParentID", volume.ID).Return(volumes, nil)
	suite.api.On("DeleteVolume", snapshotID).Return(expectedErr)
	err := service.ValidateDeleteVolume(snapshotID)
	assert.NotNil(suite.T(), err, "Error should not be nil")
}

func (suite *ISCSIControllerSuite) Test_IscsiValidateDeleteVolume_Success() {
	service := iscsistorage{cs: *suite.cs}
	snapshotID := 100
	volume := api.Volume{ID: 100, ParentId: 200}
	volumes := []api.Volume{}
	suite.api.On("GetVolume", snapshotID).Return(volume, nil)
	suite.api.On("GetVolumeSnapshotByParentID", volume.ID).Return(volumes, nil)
	suite.api.On("DeleteVolume", snapshotID).Return(nil)
	suite.api.On("GetMetadataStatus", int64(volume.ParentId)).Return(false)
	err := service.ValidateDeleteVolume(snapshotID)
	assert.Nil(suite.T(), err, "Error should not be nil")
}

func getIscsiCreateSnapshotRequest(vID string) *csi.CreateSnapshotRequest {
	return &csi.CreateSnapshotRequest{
		SourceVolumeId: vID,
	}
}

func getIscsiDeleteSnapshotRequest(sID string) *csi.DeleteSnapshotRequest {
	return &csi.DeleteSnapshotRequest{
		SnapshotId: sID,
	}
}

func getIscsiExpandVolumeRequest(vID string) *csi.ControllerExpandVolumeRequest {
	return &csi.ControllerExpandVolumeRequest{
		VolumeId: vID,
	}
}
