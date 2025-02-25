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
	"infinibox-csi-driver/api/client"

	"github.com/stretchr/testify/mock"
)

type MockApiService struct {
	mock.Mock
	Client
}

type MockApiClient struct {
	mock.Mock
}

// Get : mock for get request
func (m *MockApiClient) Get(ctx context.Context, url string, hostconfig client.HostConfig, expectedResp interface{}) (interface{}, error) {
	args := m.Called()
	resp := args.Get(0) //.(interface{})
	err, _ := args.Get(1).(error)
	return resp, err
}

// Post : mock for post request
func (m *MockApiClient) Post(ctx context.Context, url string, hostconfig client.HostConfig, body, expectedResp interface{}) (interface{}, error) {
	args := m.Called()
	resp := args.Get(0) //.(interface{})
	err, _ := args.Get(1).(error)
	return resp, err
}

// Put : mock for put request
func (m *MockApiClient) Put(ctx context.Context, url string, hostconfig client.HostConfig, body, expectedResp interface{}) (interface{}, error) {
	args := m.Called()
	response := args.Get(0) //.(interface{})
	err, _ := args.Get(1).(error)
	return response, err
}

// Delete : mock for Delete request
func (m *MockApiClient) Delete(ctx context.Context, url string, hostconfig client.HostConfig) (interface{}, error) {
	args := m.Called()
	resp := args.Get(0) //.(interface{})
	err, _ := args.Get(1).(error)
	return resp, err
}

// GetWithQueryString : mock for GetWithQueryString request
func (m *MockApiClient) GetWithQueryString(ctx context.Context, url string, hostconfig client.HostConfig, queryString string, expectedResp interface{}) (interface{}, error) {
	args := m.Called()
	resp := args.Get(0) //.(interface{})
	err, _ := args.Get(1).(error)
	return resp, err
}

func (m *MockApiService) DeleteFileSystem(fileSystemID int) (*FileSystem, error) {
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
func (m *MockApiService) GetExportByFileSystem(fileSystemID int) (*[]ExportResponse, error) {
	args := m.Called(fileSystemID)
	resp, _ := args.Get(0).([]ExportResponse)
	err, _ := args.Get(1).(error)
	return &resp, err
}

// DeleteExportPath
func (m *MockApiService) DeleteExportPath(fileSystemID int) (*ExportResponse, error) {
	args := m.Called(fileSystemID)
	resp, _ := args.Get(0).(ExportResponse)
	err, _ := args.Get(1).(error)
	return &resp, err
}

// GetNetworkSpaceByName
func (m *MockApiService) GetNetworkSpaceByName(networkSpaceName string) (NetworkSpace, error) {
	args := m.Called(networkSpaceName)
	resp, _ := args.Get(0).(NetworkSpace)
	err, _ := args.Get(1).(error)
	return resp, err
}

// GetFileSystemCount
func (m *MockApiService) GetFileSystemCount() (int, error) {
	args := m.Called()
	resp, _ := args.Get(0).(int)
	err, _ := args.Get(1).(error)
	return resp, err
}

// OneTimeValidation
func (m *MockApiService) OneTimeValidation(poolname string, networkspace string) (string, error) {
	args := m.Called(poolname, networkspace)
	resp, _ := args.Get(0).(string)
	err, _ := args.Get(1).(error)
	return resp, err
}

// CreateFilesystem
func (m *MockApiService) CreateFilesystem(fileSysparameter map[string]interface{}) (*FileSystem, error) {
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
func (m *MockApiService) ExportFileSystem(export ExportFileSys) (*ExportResponse, error) {
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
func (m *MockApiService) CreateFileSystemSnapshot(lockExpiresAt int64, snapshotParam *FileSystemSnapshot) (*FileSystemSnapshotResponse, error) {
	args := m.Called(snapshotParam)
	resp, _ := args.Get(0).(FileSystemSnapshotResponse)
	err, _ := args.Get(1).(error)
	return &resp, err
}

// DeleteFileSystemComplete
func (m *MockApiService) DeleteFileSystemComplete(fileSystemID int) (err error) {
	args := m.Called(fileSystemID)
	err, _ = args.Get(0).(error)
	return err
}

// DeleteParentFileSystem
func (m *MockApiService) DeleteParentFileSystem(fileSystemID int) (err error) {
	args := m.Called(fileSystemID)
	err, _ = args.Get(0).(error)
	return err
}

// GetMetadataStatus
func (m *MockApiService) GetMetadataStatus(fileSystemID int) bool {
	args := m.Called(fileSystemID)
	err, _ := args.Get(0).(bool)
	return err
}

// GetSnapshotByName
func (m *MockApiService) GetSnapshotByName(snapshotName string) (*[]FileSystemSnapshotResponse, error) {
	args := m.Called(snapshotName)
	resp, _ := args.Get(0).([]FileSystemSnapshotResponse)
	err, _ := args.Get(1).(error)
	return &resp, err
}

// AddNodeInExport
func (m *MockApiService) AddNodeInExport(exportID int, access string, noRootSquash bool, ip string) (*ExportResponse, error) {
	argsArray := m.Called(exportID, access, noRootSquash, ip)
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

// DeleteExportRule
func (m *MockApiService) DeleteExportRule(fileSystemID int, ipAddress string) error {
	args := m.Called(fileSystemID, ipAddress)
	err, _ := args.Get(0).(error)
	return err
}

// CreateVolume
func (m *MockApiService) CreateVolume(volume *VolumeParam, storagePoolID int) (*Volume, error) {
	args := m.Called(volume, storagePoolID)
	var vol Volume
	if args.Get(0) != nil {
		vol, _ = args.Get(0).(Volume)
	}
	err, _ := args.Get(1).(error)
	return &vol, err
}

// GetHostByName
func (m *MockApiService) GetHostByName(hostName string) (Host, error) {
	args := m.Called(hostName)
	host, _ := args.Get(0).(Host)
	err, _ := args.Get(1).(error)
	return host, err
}

func (m *MockApiService) CreateHost(hostName string) (Host, error) {
	args := m.Called(hostName)
	hosts, _ := args.Get(0).(Host)
	err, _ := args.Get(1).(error)
	return hosts, err

}

func (m *MockApiService) MapVolumeToHost(hostID, volumeID, lun int) (LunInfo, error) {
	args := m.Called(hostID)
	lunInfo, _ := args.Get(0).(LunInfo)
	err, _ := args.Get(1).(error)
	return lunInfo, err
}

func (m *MockApiService) GetLunByHostVolume(hostID, volumeID int) (LunInfo, error) {
	args := m.Called(hostID)
	lunInfo, _ := args.Get(0).(LunInfo)
	err, _ := args.Get(1).(error)
	return lunInfo, err
}

func (m *MockApiService) UnMapVolumeFromHost(hostID, volumeID int) error {
	args := m.Called(hostID, volumeID)
	err, _ := args.Get(0).(error)
	return err
}
