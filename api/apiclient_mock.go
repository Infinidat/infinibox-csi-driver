package api

import (
	"context"

	//"infinibox-csi-driver/api"
	"github.com/stretchr/testify/mock"
)

type MockApiService struct {
	mock.Mock
}

type MockApiClient struct {
	mock.Mock
}

func (m *MockApiClient) Get(ctx context.Context, url string, headerMap map[string]string, expectedResp interface{}) (interface{}, error) {
	args := m.Called(ctx, url, headerMap, &expectedResp)
	resp, _ := args.Get(0).(interface{})
	err, _ := args.Get(1).(error)
	return resp, err
}

func (m *MockApiClient) Post(ctx context.Context, url string, headerMap map[string]string, body, expectedResp interface{}) (interface{}, error) {
	args := m.Called()
	resp, _ := args.Get(0).(interface{})
	err, _ := args.Get(1).(error)
	return resp, err
}

func (m *MockApiClient) Put(ctx context.Context, url string, headerMap map[string]string, body, expectedResp interface{}) (interface{}, error) {
	args := m.Called()
	response, _ := args.Get(0).(interface{})
	err, _ := args.Get(1).(error)
	return response, err
}

func (m *MockApiClient) Delete(ctx context.Context, url string, headerMap map[string]string) (interface{}, error) {
	args := m.Called()
	resp, _ := args.Get(0).(interface{})
	err, _ := args.Get(1).(error)
	return resp, err
}

func (m *MockApiClient) GetWithQueryString(ctx context.Context, url, queryString string, expectedResp interface{}) (interface{}, error) {
	args := m.Called()
	resp, _ := args.Get(0).(interface{})
	err, _ := args.Get(1).(error)
	return resp, err
}

// func (m *MockApiService) CreateVolume(volume *VolumeParam, storagePoolName string) (*VolumeResp, error) {
// 	args := m.Called()
// 	csiresp, _ := args.Get(0).(*VolumeResp)
// 	err, _ := args.Get(1).(error)
// 	return csiresp, err
// }

// func (m *MockApiService) GetStoragePoolIDByName(name string) (id int64, err error) {
// 	args := m.Called()
// 	resp, _ := args.Get(0).(int64)
// 	err, _ = args.Get(1).(error)
// 	return resp, err
// }

// func (m *MockApiService) GetVolume(volumehref string, volumeid int, parentVolumeId int, volumename string,
// 	getSnapshots bool) ([]*Volume, error) {
// 	args := m.Called()
// 	resp, _ := args.Get(0).([]*Volume)
// 	err, _ := args.Get(1).(error)
// 	return resp, err
// }

// func (m *MockApiService) GetInstance(systemhref string) ([]*SystemDetails, error) {
// 	args := m.Called()
// 	resp, _ := args.Get(0).([]*SystemDetails)
// 	err, _ := args.Get(1).(error)
// 	return resp, err
// }

// func (m *MockApiService) FindVolumeID(volumename string) (int, error) {
// 	args := m.Called()
// 	resp, _ := args.Get(0).(int)
// 	err, _ := args.Get(1).(error)
// 	return resp, err

// }

// func (m *MockApiService) GetStoragePool(storagepoolhref string) ([]StoragePool, error) {
// 	args := m.Called()
// 	resp, _ := args.Get(0).([]StoragePool)
// 	err, _ := args.Get(1).(error)
// 	return resp, err
// }

// func (m *MockApiService) FindStoragePool(id int, name, href string) (StoragePool, error) {
// 	args := m.Called()
// 	resp, _ := args.Get(0).(StoragePool)
// 	err, _ := args.Get(1).(error)
// 	return resp, err

// }

// func (m *MockApiService) NewClient() (*ClientService, error) {
// 	args := m.Called()
// 	resp, _ := args.Get(0).(*ClientService)
// 	err, _ := args.Get(1).(error)
// 	return resp, err
// }

// func (m *MockApiService) CreateSnapshotVolume(snapshotParam *SnapshotDef) (*SnapshotVolumesResp, error) {
// 	args := m.Called()
// 	resp, _ := args.Get(0).(*SnapshotVolumesResp)
// 	err, _ := args.Get(1).(error)
// 	return resp, err
// }

// func (m *MockApiService) GetVolumeByName(volumename string) (*Volume, error) {
// 	args := m.Called()
// 	resp, _ := args.Get(0).(*Volume)
// 	err, _ := args.Get(1).(error)
// 	return resp, err
// }

// func (m *MockApiService) GetNetworkSpaceByName(networkSpaceName string) (NetworkSpace, error) {
// 	args := m.Called()
// 	resp, _ := args.Get(0).(NetworkSpace)
// 	err, _ := args.Get(1).(error)
// 	return resp, err
// }

// func (m *MockApiService) GetLunByVolumeID(volumeID string) (LunInfo, error) {
// 	args := m.Called()
// 	resp, _ := args.Get(0).(LunInfo)
// 	err, _ := args.Get(1).(error)
// 	return resp, err
// }

// func (m *MockApiService) GetHostByName(hostName string) (Host, error) {
// 	args := m.Called()
// 	resp, _ := args.Get(0).(Host)
// 	err, _ := args.Get(1).(error)
// 	return resp, err
// }

// func (m *MockApiService) getJSONResponse(method, apiuri string, body, expectedResp interface{}) error {
// 	args := m.Called()
// 	err, _ := args.Get(0).(error)
// 	return err
// }

// func (m *MockApiService) getResponseWithQueryString(apiuri string, queryParam map[string]interface{}, expectedResp interface{}) (interface{}, error) {
// 	args := m.Called()
// 	resp, _ := args.Get(0).(interface{})
// 	err, _ := args.Get(1).(error)
// 	return resp, err
// }

// func (m *MockClient) MapVolumeToHost(hostID, volumeID int) (LunInfo, error) {
// 	args := m.Called()
// 	resp, _ := args.Get(0).(LunInfo)
// 	err, _ := args.Get(1).(error)
// 	return resp, err
// }
