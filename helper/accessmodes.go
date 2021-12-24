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

func _accessModeToName(mode csi.VolumeCapability_AccessMode_Mode) (modeName string, err error) {
	// Given an access mode, return a human readable mode name.
	// Ref:
	// - https://github.com/container-storage-interface/spec/blob/master/csi.proto
	// - https://github.com/container-storage-interface/spec/blob/master/lib/go/csi/csi.pb.go#L155
	// TODO - Use the name map defined in csi.pb.go.
	switch mode {
	case 0:
		return "", errors.New("invalid CSI AccessMode: 'UNKNOWN'")
	case 1:
		return "SINGLE_NODE_WRITER", nil
	case 2:
		return "SINGLE_NODE_READER_ONLY", nil
	case 3:
		return "MULTI_NODE_READER_ONLY", nil
	case 4:
		return "MULTI_NODE_SINGLE_WRITER", nil
	case 5:
		return "MULTI_NODE_MULTI_WRITER", nil
	default:
		return "", fmt.Errorf("invalid CSI AccessMode: %d", mode)
	}
}

// AcessMode service struct
type AccessMode struct{}

func (a AccessMode) IsValidAccessMode(volume *api.Volume, req *csi.ControllerPublishVolumeRequest) (isValidAccessMode bool, err error) {
	// Compare the volume's write protected state on IBox to the requested access mode. Return an error if incompatible.
	isIboxVolWriteProtected := volume.WriteProtected
	volName := volume.Name
	volId := req.GetVolumeId()
	reqAccessMode := req.VolumeCapability.GetAccessMode().GetMode()
	modeName, err := _accessModeToName(reqAccessMode)
	if err != nil {
		return false, fmt.Errorf("for volume '%s' (%s), an error occurred: %s", volName, volId, err)
	}

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
	modeName, err := _accessModeToName(reqAccessMode)
	if err != nil {
		return false, fmt.Errorf("for NFS export '%s' (%s), an error occurred: %s", exportVolPathd, exportID, err)
	}

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