package api

import (
	"context"
	"infinibox-csi-driver/api/client"

	//"infinibox-csi-driver/api"
	"github.com/stretchr/testify/mock"
)

type MockApiService struct {
	mock.Mock
}

type MockApiClient struct {
	mock.Mock
}

//Get : mock for get request
func (m *MockApiClient) Get(ctx context.Context, url string, hostconfig client.HostConfig, expectedResp interface{}) (interface{}, error) {
	args := m.Called(ctx, url, hostconfig, &expectedResp)
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
