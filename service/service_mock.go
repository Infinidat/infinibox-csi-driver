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
package service

import (
	"infinibox-csi-driver/api"

	"github.com/stretchr/testify/mock"
)

type MockService struct {
	mock.Mock
}

type MockClient struct {
	mock.Mock
	// api client.RestClient
}

func (m *MockClient) CreateVolume(volume *api.VolumeParam, storagePoolName string) (*api.Volume, error) {
	args := m.Called()
	csiresp, _ := args.Get(0).(*api.Volume)
	err, _ := args.Get(1).(error)
	return csiresp, err
}

func (m *MockClient) DeleteVolume(volumeId int) error {
	args := m.Called()
	err, _ := args.Get(0).(error)
	return err
}

func (m *MockClient) GetStoragePoolIDByName(name string) (id int64, err error) {
	args := m.Called()
	resp, _ := args.Get(0).(int64)
	err, _ = args.Get(1).(error)
	return resp, err
}

func (m *MockClient) GetVolume(volumeid int) ([]api.Volume, error) {
	args := m.Called()
	resp, _ := args.Get(0).([]api.Volume)
	err, _ := args.Get(1).(error)
	return resp, err
}

func (m *MockClient) FindVolumeID(volumename string) (int, error) {
	args := m.Called()
	resp, _ := args.Get(0).(int)
	err, _ := args.Get(1).(error)
	return resp, err
}

func (m *MockClient) GetStoragePool(poolId int64,
	storagepoolname string) ([]api.StoragePool, error) {
	args := m.Called()
	resp, _ := args.Get(0).([]api.StoragePool)
	err, _ := args.Get(1).(error)
	return resp, err
}

func (m *MockClient) FindStoragePool(id int64, name string) (api.StoragePool, error) {
	args := m.Called()
	resp, _ := args.Get(0).(api.StoragePool)
	err, _ := args.Get(1).(error)
	return resp, err
}

func (m *MockClient) NewClient() (*api.ClientService, error) {
	args := m.Called()
	resp, _ := args.Get(0).(*api.ClientService)
	err, _ := args.Get(1).(error)
	return resp, err
}

func (m *MockClient) CreateSnapshotVolume(snapshotParam *api.VolumeSnapshot) (*api.SnapshotVolumesResp, error) {
	args := m.Called()
	resp, _ := args.Get(0).(*api.SnapshotVolumesResp)
	err, _ := args.Get(1).(error)
	return resp, err
}

func (m *MockClient) GetVolumeByName(volumename string) (*api.Volume, error) {
	args := m.Called()
	resp, _ := args.Get(0).(*api.Volume)
	err, _ := args.Get(1).(error)
	return resp, err
}

func (m *MockClient) GetNetworkSpaceByName(networkSpaceName string) (api.NetworkSpace, error) {
	args := m.Called()
	resp, _ := args.Get(0).(api.NetworkSpace)
	err, _ := args.Get(1).(error)
	return resp, err
}

func (m *MockClient) GetHostByName(hostName string) (api.Host, error) {
	args := m.Called()
	resp, _ := args.Get(0).(api.Host)
	err, _ := args.Get(1).(error)
	return resp, err
}

// func (m *MockClient) getJSONResponse(method, apiuri string, body, expectedResp interface{}) error {
// 	args := m.Called()
// 	err, _ := args.Get(0).(error)
// 	return err
// }

// func (m *MockClient) getResponseWithQueryString(apiuri string, queryParam map[string]interface{}, expectedResp interface{}) (interface{}, error) {
// 	args := m.Called()
// 	resp, _ := args.Get(0).(interface{})
// 	err, _ := args.Get(1).(error)
// 	return resp, err
// }

func (m *MockClient) MapVolumeToHost(hostID, volumeID int) (api.LunInfo, error) {
	args := m.Called()
	resp, _ := args.Get(0).(api.LunInfo)
	err, _ := args.Get(1).(error)
	return resp, err
}

func (m *MockClient) InitRestClient() error {
	args := m.Called()
	err, _ := args.Get(0).(error)
	return err
}

func (m *MockClient) UnMapVolumeFromHost(hostID, volumeID int) error {
	args := m.Called()
	err, _ := args.Get(0).(error)
	return err
}
