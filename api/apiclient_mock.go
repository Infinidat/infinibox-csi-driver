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

//GetExportByFileSystem
func (m *MockApiService) GetExportByFileSystem(fileSystemID int64) (*[]ExportResponse, error) {
	args := m.Called(fileSystemID)
	resp, _ := args.Get(0).([]ExportResponse)
	err, _ := args.Get(1).(error)
	return &resp, err
}
