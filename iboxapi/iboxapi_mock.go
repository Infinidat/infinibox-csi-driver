/*
Copyright 2025 Infinidat
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
package iboxapi

import (

	//	"infinibox-csi-driver/api/client"

	"github.com/stretchr/testify/mock"
)

type MockApiService struct {
	mock.Mock
	Client
}

type MockApiClient struct {
	mock.Mock
}

// GetAllPools mock
func (m *MockApiService) GetPoolByName(name string) (*PoolResult, error) {
	args := m.Called()
	resp, _ := args.Get(0).(*PoolResult)
	err, _ := args.Get(1).(error)
	return resp, err
}
