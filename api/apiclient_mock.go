package api

import (
	"context"
	"infinibox-csi-driver/api/client"
	//"infinibox-csi-driver/api"

	"github.com/stretchr/testify/mock"
)

type MockApiService struct {
	mock.Mock
	Client
}

type MockApiClient struct {
	mock.Mock
}

//Get : mock for get request
func (m *MockApiClient) Get(ctx context.Context, url string, hostconfig client.HostConfig, expectedResp interface{}) (interface{}, error) {
	args := m.Called()
	resp, _ := args.Get(0).(interface{})
	err, _ := args.Get(1).(error)
	return resp, err
}

//Post : mock for post request
func (m *MockApiClient) Post(ctx context.Context, url string, hostconfig client.HostConfig, body, expectedResp interface{}) (interface{}, error) {
	args := m.Called()
	resp, _ := args.Get(0).(interface{})
	err, _ := args.Get(1).(error)
	return resp, err
}

//Put : mock for put request
func (m *MockApiClient) Put(ctx context.Context, url string, hostconfig client.HostConfig, body, expectedResp interface{}) (interface{}, error) {
	args := m.Called()
	response, _ := args.Get(0).(interface{})
	err, _ := args.Get(1).(error)
	return response, err
}

//Delete : mock for Delete request
func (m *MockApiClient) Delete(ctx context.Context, url string, hostconfig client.HostConfig) (interface{}, error) {
	args := m.Called()
	resp, _ := args.Get(0).(interface{})
	err, _ := args.Get(1).(error)
	return resp, err
}

//GetWithQueryString : mock for GetWithQueryString request
func (m *MockApiClient) GetWithQueryString(ctx context.Context, url string, hostconfig client.HostConfig, queryString string, expectedResp interface{}) (interface{}, error) {
	args := m.Called()
	resp, _ := args.Get(0).(interface{})
	err, _ := args.Get(1).(error)
	return resp, err
}

//GetStoragePoolIDByName mock
func (m *MockApiService) GetStoragePoolIDByName(poolName string) (int64, error) {
	args := m.Called(poolName)
	resp, _ := args.Get(0).(int64)
	err, _ := args.Get(1).(error)
	return resp, err
}

//GetFileSystemsByPoolID mock
func (m *MockApiService) GetFileSystemsByPoolID(poolID int64, page int) (*FSMetadata, error) {
	args := m.Called(poolID, page)
	resp, _ := args.Get(0).(FSMetadata)
	err, _ := args.Get(1).(error)
	return &resp, err
}

//GetFilesytemTreeqCount mock
func (m *MockApiService) GetFilesytemTreeqCount(filesystemID int64) (int, error) {
	args := m.Called(filesystemID)
	resp, _ := args.Get(0).(int)
	err, _ := args.Get(1).(error)
	return resp, err
}

//CreateTreeq mock
func (m *MockApiService) CreateTreeq(filesystemID int64, treeqParameter map[string]interface{}) (*Treeq, error) {
	args := m.Called(filesystemID, treeqParameter)
	resp, _ := args.Get(0).(Treeq)
	err, _ := args.Get(1).(error)
	return &resp, err
}

//AttachMetadataToObject mock
func (m *MockApiService) AttachMetadataToObject(objectID int64, body map[string]interface{}) (*[]Metadata, error) {
	args := m.Called(objectID, body)
	resp, _ := args.Get(0).([]Metadata)
	err, _ := args.Get(1).(error)
	return &resp, err
}

//UpdateFilesystem
func (m *MockApiService) UpdateFilesystem(fileSystemID int64, fileSystem FileSystem) (*FileSystem, error) {
	args := m.Called(fileSystemID, fileSystem)
	resp, _ := args.Get(0).(FileSystem)
	err, _ := args.Get(1).(error)
	return &resp, err
}

//UpdateTreeq
func (m *MockApiService) UpdateTreeq(fileSystemID, treeqID int64, body map[string]interface{}) (*Treeq, error) {
	args := m.Called(fileSystemID, treeqID, body)
	resp, _ := args.Get(0).(Treeq)
	err, _ := args.Get(1).(error)
	return &resp, err
}

//GetFileSystemByID
func (m *MockApiService) GetFileSystemByID(fileSystemID int64) (*FileSystem, error) {
	args := m.Called(fileSystemID)
	resp, _ := args.Get(0).(FileSystem)
	err, _ := args.Get(1).(error)
	return &resp, err
}

//GetTreeqSizeByFileSystemID
func (m *MockApiService) GetTreeqSizeByFileSystemID(fileSystemID int64) (int64, error) {
	args := m.Called(fileSystemID)
	resp, _ := args.Get(0).(int64)
	err, _ := args.Get(1).(error)
	return resp, err
}

//GetExportByFileSystem
func (m *MockApiService) GetExportByFileSystem(fileSystemID int64) (*[]ExportResponse, error) {
	args := m.Called(fileSystemID)
	resp, _ := args.Get(0).([]ExportResponse)
	err, _ := args.Get(1).(error)
	return &resp, err
}

//GetTreeq mock
func (m *MockApiService) GetTreeq(fileSystemID, treeqID int64) (*Treeq, error) {
	args := m.Called(fileSystemID, treeqID)
	resp, _ := args.Get(0).(Treeq)
	err, _ := args.Get(1).(error)
	return &resp, err
}

//DeleteTreeq
func (m *MockApiService) DeleteTreeq(fileSystemID, treeqID int64) (*Treeq, error) {
	args := m.Called(fileSystemID, treeqID)
	resp, _ := args.Get(0).(Treeq)
	err, _ := args.Get(1).(error)
	return &resp, err
}

//GetNetworkSpaceByName
func (m *MockApiService) GetNetworkSpaceByName(networkSpaceName string) (NetworkSpace, error) {
	args := m.Called(networkSpaceName)
	resp, _ := args.Get(0).(NetworkSpace)
	err, _ := args.Get(1).(error)
	return resp, err
}

//GetVolume
func (m *MockApiService) GetVolume(volumeid int) (*Volume, error) {
	args := m.Called(volumeid)
	resp, _ := args.Get(0).(Volume)
	err, _ := args.Get(1).(error)
	return &resp, err
}

//GetVolumeSnapshotByParentID
func (m *MockApiService) GetVolumeSnapshotByParentID(volumeID int) (*[]Volume, error) {
	args := m.Called(volumeID)
	resp, _ := args.Get(0).([]Volume)
	err, _ := args.Get(1).(error)
	return &resp, err
}

//DeleteVolume
func (m *MockApiService) DeleteVolume(volumeID int) (err error) {
	args := m.Called(volumeID)
	err, _ = args.Get(0).(error)
	return err
}

//GetMetadataStatus
func (m *MockApiService) GetMetadataStatus(fileSystemID int64) bool {
	args := m.Called(fileSystemID)
	err, _ := args.Get(0).(bool)
	return err
}

//GetSnapshotByName
func (m *MockApiService) GetSnapshotByName(snapshotName string) (*[]FileSystemSnapshotResponce, error) {
	args := m.Called(snapshotName)
	resp, _ := args.Get(0).([]FileSystemSnapshotResponce)
	err, _ := args.Get(1).(error)
	return &resp, err
}

//CreateFileSystemSnapshot
func (m *MockApiService) CreateFileSystemSnapshot(snapshotParam *FileSystemSnapshot) (*FileSystemSnapshotResponce, error) {
	args := m.Called(snapshotParam)
	resp, _ := args.Get(0).(FileSystemSnapshotResponce)
	err, _ := args.Get(1).(error)
	return &resp, err
}

//FileSystemHasChild
func (m *MockApiService) FileSystemHasChild(fileSystemID int64) bool {
	args := m.Called(fileSystemID)
	err, _ := args.Get(0).(bool)
	return err
}

//GetParentID
func (m *MockApiService) GetParentID(fileSystemID int64) int64 {
	args := m.Called(fileSystemID)
	resp, _ := args.Get(0).(int64)
	return resp
}

//DeleteFileSystemComplete
func (m *MockApiService) DeleteFileSystemComplete(fileSystemID int64) (err error) {
	args := m.Called(fileSystemID)
	err, _ = args.Get(0).(error)
	return err
}

//DeleteParentFileSystem
func (m *MockApiService) DeleteParentFileSystem(fileSystemID int64) (err error) {
	args := m.Called(fileSystemID)
	err, _ = args.Get(0).(error)
	return err
}
