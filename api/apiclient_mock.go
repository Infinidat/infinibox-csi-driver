/*
Copyright 2022 Infinidat
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package api

import (
	"context"

	"github.com/infinidat/infinibox-csi-driver/iboxapi"

	"github.com/stretchr/testify/mock"
)

type MockAPIService struct {
	mock.Mock
	Client
}

type MockAPIClient struct {
	mock.Mock
}

func (m *MockAPIService) DeleteFileSystem(fileSystemID int) (*FileSystem, error) {
	args := m.Called(fileSystemID)
	var filessy FileSystem
	if args.Get(0) != nil {
		filessy, _ = args.Get(0).(FileSystem)
	}

	var err error
	if args.Get(0) != nil {
		err, _ = args.Get(0).(error)
	}
	return &filessy, err
}

// GetExportByFileSystem
func (m *MockAPIService) GetExportByFileSystem(fileSystemID int) (*[]ExportResponse, error) {
	args := m.Called(fileSystemID)
	resp, _ := args.Get(0).([]ExportResponse)
	err, _ := args.Get(1).(error)
	return &resp, err
}

// DeleteExportPath
func (m *MockAPIService) DeleteExportPath(fileSystemID int) (*ExportResponse, error) {
	args := m.Called(fileSystemID)
	resp, _ := args.Get(0).(ExportResponse)
	err, _ := args.Get(1).(error)
	return &resp, err
}

// GetNetworkSpaceByName
func (m *MockAPIService) GetNetworkSpaceByName(networkSpaceName string) (NetworkSpace, error) {
	args := m.Called(networkSpaceName)
	resp, _ := args.Get(0).(NetworkSpace)
	err, _ := args.Get(1).(error)
	return resp, err
}

// GetFileSystemCount
func (m *MockAPIService) GetFileSystemCount() (int, error) {
	args := m.Called()
	resp, _ := args.Get(0).(int)
	err, _ := args.Get(1).(error)
	return resp, err
}

// OneTimeValidation
func (m *MockAPIService) OneTimeValidation(poolname string, networkspace string) (string, error) {
	args := m.Called(poolname, networkspace)
	resp, _ := args.Get(0).(string)
	err, _ := args.Get(1).(error)
	return resp, err
}

// CreateFilesystem
func (m *MockAPIService) CreateFilesystem(fileSysparameter map[string]interface{}) (*FileSystem, error) {
	args := m.Called(fileSysparameter)
	var resp FileSystem
	if args.Get(0) != nil {
		resp, _ = args.Get(0).(FileSystem)
	}
	var err error
	if args.Get(1) != nil {
		err, _ = args.Get(1).(error)
	}
	return &resp, err
}

// ExportFileSystem
func (m *MockAPIService) ExportFileSystem(export ExportFileSys) (*ExportResponse, error) {
	argsArray := m.Called(export)
	args := argsArray[0]
	var resp ExportResponse
	if argsArray.Get(0) != nil {
		resp, _ = args.(ExportResponse)
	}
	var err error
	if argsArray.Get(1) != nil {
		err, _ = argsArray.Get(1).(error)
	}
	return &resp, err
}

// CreateFileSystemSnapshot
func (m *MockAPIService) CreateFileSystemSnapshot(snapshot *FileSystemSnapshot) (*FileSystemSnapshotResponse, error) {
	args := m.Called(snapshot)
	resp, _ := args.Get(0).(FileSystemSnapshotResponse)
	err, _ := args.Get(1).(error)
	return &resp, err
}

// DeleteFileSystemComplete
func (m *MockAPIService) DeleteFileSystemComplete(ctx context.Context, fileSystemID int) (err error) {
	args := m.Called(ctx, fileSystemID)
	err, _ = args.Get(0).(error)
	return err
}

// DeleteParentFileSystem
func (m *MockAPIService) DeleteParentFileSystem(ctx context.Context, fileSystemID int) (err error) {
	args := m.Called(ctx, fileSystemID)
	err, _ = args.Get(0).(error)
	return err
}

// GetMetadataStatus
func (m *MockAPIService) GetMetadataStatus(fileSystemID int) bool {
	args := m.Called(fileSystemID)
	err, _ := args.Get(0).(bool)
	return err
}

// AddNodeInExport
func (m *MockAPIService) AddNodeInExport(ctx context.Context, exportID int, access string, noRootSquash bool, ip string) (*iboxapi.Export, error) {
	argsArray := m.Called(ctx, exportID, access, noRootSquash, ip)
	args := argsArray[0]
	var resp iboxapi.Export
	if argsArray.Get(0) != nil {
		resp, _ = args.(iboxapi.Export)
	}
	var err error
	if argsArray.Get(1) != nil {
		err, _ = argsArray.Get(1).(error)
	}
	return &resp, err
}

// DeleteExportRule
func (m *MockAPIService) DeleteExportRule(ctx context.Context, fileSystemID int, ipAddress string) error {
	args := m.Called(ctx, fileSystemID, ipAddress)
	err, _ := args.Get(0).(error)
	return err
}

// CreateVolume
func (m *MockAPIService) CreateVolume(volume *VolumeParam, storagePoolID int) (*Volume, error) {
	args := m.Called(volume, storagePoolID)
	var vol Volume
	if args.Get(0) != nil {
		vol, _ = args.Get(0).(Volume)
	}
	err, _ := args.Get(1).(error)
	return &vol, err
}

// GetHostByName
func (m *MockAPIService) GetHostByName(hostName string) (Host, error) {
	args := m.Called(hostName)
	host, _ := args.Get(0).(Host)
	err, _ := args.Get(1).(error)
	return host, err
}

func (m *MockAPIService) CreateHost(hostName string) (Host, error) {
	args := m.Called(hostName)
	hosts, _ := args.Get(0).(Host)
	err, _ := args.Get(1).(error)
	return hosts, err
}

func (m *MockAPIService) MapVolumeToHost(hostID, volumeID, lun int) (LunInfo, error) {
	args := m.Called(hostID)
	lunInfo, _ := args.Get(0).(LunInfo)
	err, _ := args.Get(1).(error)
	return lunInfo, err
}

func (m *MockAPIService) UnMapVolumeFromHost(hostID, volumeID int) error {
	args := m.Called(hostID, volumeID)
	err, _ := args.Get(0).(error)
	return err
}

func (m *MockAPIService) DeleteNodeFromExport(ctx context.Context, export iboxapi.Export, noRootSquash bool, ip string) (*iboxapi.Export, error) {
	args := m.Called(ctx, export, noRootSquash, ip)
	resp, _ := args.Get(0).(iboxapi.Export)
	err, _ := args.Get(0).(error)
	return &resp, err
}
