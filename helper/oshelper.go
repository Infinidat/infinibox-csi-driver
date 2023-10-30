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
package helper

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"

	"infinibox-csi-driver/log"

	"github.com/stretchr/testify/mock"
)

var nodeVolumeMutex sync.Mutex // Used by NodeStageVolume, NodeUnstageVolume, NodePublishVolume and NodeUnpublishVolume.
var zlog = log.Get()           // grab the logger for package use

// OsHelper interface
type OsHelper interface {
	MkdirAll(path string, perm os.FileMode) error
	IsNotExist(err error) bool
	Remove(name string) error
}

// Service service struct
type Service struct{}

// Lock or unlock NodeVolumeMutex. Log taking care to write to log while locked.
func ManageNodeVolumeMutex(isLocking bool, callingFunction string, volumeId string) (err error) {
	defer func() {
		// This might happen if unlocking a mutex that was not locked.
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
			zlog.Debug().Msgf("manageNodeVolumeMutex, called by %s with volume ID %s, failed with run-time error: %s", callingFunction, volumeId, err)
		}
	}()

	err = nil
	if isLocking {
		nodeVolumeMutex.Lock()
		zlog.Debug().Msgf("LOCKED: %s() with volume ID %s", callingFunction, volumeId)
	} else {
		zlog.Debug().Msgf("UNLOCKING: %s() with volume ID %s", callingFunction, volumeId)
		nodeVolumeMutex.Unlock()
	}
	return
}

// MkdirAll method create dir
func (h Service) MkdirAll(path string, perm os.FileMode) error {
	zlog.Debug().Msgf("MkdirAll with path %s perm %v\n", path, perm)
	err := os.MkdirAll(path, perm)
	if err != nil {
		zlog.Error().Msgf("error os.MkdirAll %s", err.Error())
	}
	return err
}

// IsNotExist method check the error type
func (h Service) IsNotExist(err error) bool {
	return os.IsNotExist(err)
}

// Remove method delete the dir
func (h Service) Remove(name string) error {
	zlog.Debug().Msgf("Calling Remove with name %s", name)
	// debugWalkDir(name)
	return os.Remove(name)
}

// Check that permissions are convertable to a uint32 from a string represending an octal integer.
func ValidateUnixPermissions(unixPermissions string) (err error) {
	err = nil
	if _, err8 := strconv.ParseUint(unixPermissions, 8, 32); err8 != nil {
		msg := fmt.Sprintf("Unix permissions [%s] are invalid. Value must be uint32 in octal format. Error: %s", unixPermissions, err8)
		zlog.Error().Msgf(msg)
		err = errors.New(msg)
	} else {
		zlog.Debug().Msgf("Unix permissions [%s] is a valid octal value", unixPermissions)
	}
	return err
}

/*OsHelper method mock services */

// MockOsHelper -- mock method
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

func (m *MockOsHelper) ChownVolume(uid string, gid string, targetPath string) error {
	status := m.Called(uid, gid, targetPath)
	if status.Get(0) == nil {
		return nil
	}
	st, _ := status.Get(0).(error)
	return st
}

func (m *MockOsHelper) ChownVolumeExec(uid string, gid string, targetPath string) error {
	status := m.Called(uid, gid, targetPath)
	if status.Get(0) == nil {
		return nil
	}
	st, _ := status.Get(0).(error)
	return st
}

func (m *MockOsHelper) ChmodVolume(unixPermissions string, targetPath string) error {
	status := m.Called(unixPermissions, targetPath)
	if status.Get(0) == nil {
		return nil
	}
	st, _ := status.Get(0).(error)
	return st
}

func (m *MockOsHelper) ChmodVolumeExec(unixPermissions string, targetPath string) error {
	status := m.Called(unixPermissions, targetPath)
	if status.Get(0) == nil {
		return nil
	}
	st, _ := status.Get(0).(error)
	return st
}
