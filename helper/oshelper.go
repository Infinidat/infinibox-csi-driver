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
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"

	"github.com/stretchr/testify/mock"
	"k8s.io/klog"
)

//OsHelper interface
type OsHelper interface {
	MkdirAll(path string, perm os.FileMode) error
	IsNotExist(err error) bool
	Remove(name string) error
	ChownVolume(uid string, gid string, targetPath string) error
	ChownVolumeExec(uid string, gid string, targetPath string) error
	ChmodVolume(unixPermissions string, targetPath string) error
	ChmodVolumeExec(unixPermissions string, targetPath string) error
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

//ChownVolume method If uid/gid keys are found in req, set UID/GID recursively for target path ommitting a toplevel .snapshot/.
func (h Service) ChownVolume(uid string, gid string, targetPath string) error {
	// Sanity check values.
	if uid != "" {
		uid_int, err := strconv.Atoi(uid)
		if err != nil || uid_int < 0 {
			msg := fmt.Sprintf("Storage class specifies an invalid volume UID with value [%s]: %s", uid, err)
			klog.Errorf(msg)
			return errors.New(msg)
		}
	}
	if gid != "" {
		gid_int, err := strconv.Atoi(gid)
		if err != nil || gid_int < 0 {
			msg := fmt.Sprintf("Storage class specifies an invalid volume GID with value [%s]: %s", gid, err)
			klog.Errorf(msg)
			return errors.New(msg)
		}
	}

	return h.ChownVolumeExec(uid, gid, targetPath)
}

//ChownVolumeExec method Execute chown.
func (h Service) ChownVolumeExec(uid string, gid string, targetPath string) error {
	if uid != "" || gid != "" {
		klog.V(4).Infof("Specified UID: '%s', GID: '%s'", uid, gid)
		ownerGroup := fmt.Sprintf("%s:%s", uid, gid)
		// .snapshot within the mounted volume is readonly. Find will ignore.
		chown := fmt.Sprintf("find %s -maxdepth 1 -name '*' -exec chown --recursive %s '{}' \\;", targetPath, ownerGroup)
		klog.V(4).Infof("Run: %s", chown)
		cmd := exec.Command("bash", "-c", chown)
		err := cmd.Run()
		if err != nil {
			msg := fmt.Sprintf("Failed to execute '%s': %s", chown, err)
			klog.Errorf(msg)
			return errors.New(msg)
		} else {
			klog.V(4).Infof("Set mount point directory and contents ownership.")
		}
	} else {
		klog.V(4).Infof("Using default ownership for mount point %s", targetPath)
	}
	return nil
}

//ChmodVolume method If unixPermissions key is found in req, chmod recursively for target path ommitting a toplevel .snapshot/.
func (h Service) ChmodVolume(unixPermissions string, targetPath string) error {
	return h.ChmodVolumeExec(unixPermissions, targetPath)
}

//ChmodVolumeExec method Execute chmod.
func (h Service) ChmodVolumeExec(unixPermissions string, targetPath string) error {
	if unixPermissions != "" {
		klog.V(4).Infof("Specified unix permissions: '%s'", unixPermissions)
		// .snapshot within the mounted volume is readonly. Find will ignore.
		chmod := fmt.Sprintf("find %s -maxdepth 1 -name '*' -exec chmod --recursive %s '{}' \\;", targetPath, unixPermissions)
		klog.V(4).Infof("Run: %s", chmod)
		cmd := exec.Command("bash", "-c", chmod)
		err := cmd.Run()
		if err != nil {
			msg := fmt.Sprintf("Failed to execute '%s': error: %s", chmod, err)
			klog.Errorf(msg)
			return errors.New(msg)
		} else {
			klog.V(4).Infof("Set mount point directory and contents mode bits.")
		}
	} else {
		klog.V(4).Infof("Using default mode bits for mount point %s", targetPath)
	}
	return nil
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
