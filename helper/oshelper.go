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
	"os"

	"github.com/stretchr/testify/mock"
)

//OsHelper interface
type OsHelper interface {
	MkdirAll(path string, perm os.FileMode) error
	IsNotExist(err error) bool
	Remove(name string) error
}

//Service service struct
type Service struct {
}

//MkdirAll method create dir
func (h Service) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

//IsNotExist method check the error type
func (h Service) IsNotExist(err error) bool {
	return os.IsNotExist(err)
}

//Remove method delete the dir
func (h Service) Remove(name string) error {
	return os.Remove(name)
}

/*OsHelper method mock services */

//MockOsHelper -- mock method
type MockOsHelper struct {
	mock.Mock
	OsHelper
}

func (m *MockOsHelper) IsNotExist(err error) bool {
	status := m.Called(err)
	st, _ := status.Get(0).(bool)
	return st
}

func (m *MockOsHelper) MkdirAll(path string, perm os.FileMode) error {
	status := m.Called(path, perm)
	if status.Get(0) == nil {
		return nil
	}
	return status.Get(0).(error)
}

func (m *MockOsHelper) Remove(path string) error {
	status := m.Called(path)
	if status.Get(0) == nil {
		return nil
	}
	st, _ := status.Get(0).(error)
	return st
}
