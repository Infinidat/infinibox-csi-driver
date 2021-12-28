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
package helper

import (
	"errors"
	"fmt"
	"infinibox-csi-driver/api"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/mock"
)

// AccessModesHelper interface
type AccessModesHelper interface {
	IsValidAccessMode(volume *api.Volume, req *csi.ControllerPublishVolumeRequest) (isValidAccessMode bool, err error)
	IsValidAccessModeNfs(req *csi.ControllerPublishVolumeRequest) (isValidAccessMode bool, err error)
}

// AcessMode service struct
type AccessMode struct{}

func (a AccessMode) IsValidAccessMode(volume *api.Volume, req *csi.ControllerPublishVolumeRequest) (isValidAccessMode bool, err error) {
	// Compare the volume's write protected state on IBox to the requested access mode. Return an error if incompatible.
	isIboxVolWriteProtected := volume.WriteProtected
	volName := volume.Name
	volId := req.GetVolumeId()
	reqAccessMode := req.VolumeCapability.GetAccessMode().GetMode()

	switch reqAccessMode {
	case csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
		csi.VolumeCapability_AccessMode_MULTI_NODE_SINGLE_WRITER,
		csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER:
		if isIboxVolWriteProtected {
			return false, fmt.Errorf("IBox Volume name '%s' (%s) is write protected, but the requested access mode is '%s'", volName, volId, modeName)
		}
		return true, nil
	case csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY,
		csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY:
		return true, nil
	}
	return false, fmt.Errorf("unsupported access mode for volume '%s' (%s): '%s'", volName, volId, modeName)
}

func (a AccessMode) IsValidAccessModeNfs(req *csi.ControllerPublishVolumeRequest) (isValidAccessMode bool, err error) {
	// Compare the export permissions on IBox to the requested access mode. Return an error if incompatible.
	nfsExportPermission := req.GetVolumeContext()["nfs_export_permissions"]
	isIboxExportReadonly := strings.Contains(nfsExportPermission, "'access':'RO'") // Could also contain "'access':'RW'"

	exportVolPathd := req.GetVolumeContext()["volPathd"]
	exportID := req.GetVolumeContext()["exportID"]
	reqAccessMode := req.VolumeCapability.GetAccessMode().GetMode()

	switch reqAccessMode {
	case csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
		csi.VolumeCapability_AccessMode_MULTI_NODE_SINGLE_WRITER,
		csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER:
		if isIboxExportReadonly {
			return false, fmt.Errorf("IBox NFS export name '%s' (%s) is write protected, but the requested access mode is '%s'", exportVolPathd, exportID, modeName)
		} else {
			return true, nil
		}
	case csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY,
		csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY:
		return true, nil
	}
	return false, fmt.Errorf("unsupported access mode for NFS export '%s' (%s): '%s'", exportVolPathd, exportID, modeName)
}

// MockAccessModesHelper -- mock method
type MockAccessModesHelper struct {
	mock.Mock
	AccessModesHelper
}

func (m *MockAccessModesHelper) IsValidAccessMode(volume *api.Volume, req *csi.ControllerPublishVolumeRequest) (bool, error) {
	status := m.Called(volume, req)
	isValid, _ := status.Get(0).(bool)
	err, _ := status.Get(1).(error)
	return isValid, err
}

func (m *MockAccessModesHelper) IsValidAccessModeNfs(req *csi.ControllerPublishVolumeRequest) (bool, error) {
	status := m.Called(req)
	isValid, _ := status.Get(0).(bool)
	err, _ := status.Get(1).(error)
	return isValid, err
}
