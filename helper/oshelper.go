/*Copyright 2022 Infinidat
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
	"sync"

	"github.com/stretchr/testify/mock"
	"k8s.io/klog"
)

var execScsi ExecScsi

var nodeVolumeMutex sync.Mutex // Used by NodeStageVolume, NodeUnstageVolume, NodePublishVolume and NodeUnpublishVolume.

// OsHelper interface
type OsHelper interface {
	MkdirAll(path string, perm os.FileMode) error
	IsNotExist(err error) bool
	Remove(name string) error
	ChownVolume(uid string, gid string, targetPath string) error
	ChownVolumeExec(uid string, gid string, targetPath string) error
	ChmodVolume(unixPermissions string, targetPath string) error
	ChmodVolumeExec(unixPermissions string, targetPath string) error
}

// Service service struct
type Service struct{}

// Lock or unlock NodeVolumeMutex. Log taking care to write to log while locked.
// Flush klog for improved mutex log tracing.
func ManageNodeVolumeMutex(isLocking bool, callingFunction string, volumeId string) (err error) {
	defer func() {
		// This might happen if unlocking a mutex that was not locked.
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
			klog.V(4).Infof("manageNodeVolumeMutex, called by %s with volume ID %s, failed with run-time error: %s", callingFunction, volumeId, err)
		}
	}()

	//klog.V(4).Info("Node.*Volume() mutex is disabled")
	//return

	err = nil
	if isLocking {
		nodeVolumeMutex.Lock()
		klog.V(4).Infof("LOCKED: %s() with volume ID %s", callingFunction, volumeId)
		klog.Flush()
	} else {
		klog.V(4).Infof("UNLOCKING: %s() with volume ID %s", callingFunction, volumeId)
		klog.Flush()
		nodeVolumeMutex.Unlock()
	}
	return
}

// MkdirAll method create dir
func (h Service) MkdirAll(path string, perm os.FileMode) error {
	klog.V(4).Infof("MkdirAll with path %s perm %v\n", path, perm)
	err := os.MkdirAll(path, perm)
	if err != nil {
		klog.Errorf("error os.MkdirAll %s", err.Error())
	}
	return err
}

// IsNotExist method check the error type
func (h Service) IsNotExist(err error) bool {
	return os.IsNotExist(err)
}

// Remove method delete the dir
func (h Service) Remove(name string) error {
	klog.V(4).Infof("Calling Remove with name %s", name)
	// debugWalkDir(name)
	return os.Remove(name)
}

func CheckMultipath() {
	isToLogOutput := true
	_, _ = execScsi.Command("multipath", "-ll", isToLogOutput)
}

// ChownVolume method If uid/gid keys are found in req, set UID/GID recursively for target path ommitting a toplevel .snapshot/.
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

// ChownVolumeExec method Execute chown.
func (h Service) ChownVolumeExec(uid string, gid string, targetPath string) error {
	if uid != "" || gid != "" {
		klog.V(4).Infof("Setting volume %s ownership: UID: '%s', GID: '%s'", targetPath, uid, gid)
		ownerGroup := fmt.Sprintf("%s:%s", uid, gid)
		chown := fmt.Sprintf("chown %s %s ", ownerGroup, targetPath)
		klog.V(4).Infof("Run: %s", chown)
		cmd := exec.Command("bash", "-c", chown)
		err := cmd.Run()
		if err != nil {
			msg := fmt.Sprintf("For mount path %s, failed to execute '%s': %s", targetPath, chown, err)
			klog.Errorf(msg)
			return errors.New(msg)
		} else {
			klog.V(4).Infof("Set mount point directory ownership for mount point %s to %s", targetPath, ownerGroup)
		}
	} else {
		klog.V(4).Infof("Using default ownership for mount point %s", targetPath)
	}
	return nil
}

// ChmodVolume method If unixPermissions key is found in req, chmod recursively for target path ommitting a toplevel .snapshot/.
func (h Service) ChmodVolume(unixPermissions string, targetPath string) error {
	return h.ChmodVolumeExec(unixPermissions, targetPath)
}

// Check that permissions are convertable to a uint32 from a string represending an octal integer.
func ValidateUnixPermissions(unixPermissions string) (err error) {
	err = nil
	if _, err8 := strconv.ParseUint(unixPermissions, 8, 32); err8 != nil {
		msg := fmt.Sprintf("Unix permissions [%s] are invalid. Value must be uint32 in octal format. Error: %s", unixPermissions, err8)
		klog.Errorf(msg)
		err = errors.New(msg)
	} else {
		klog.V(4).Infof("Unix permissions [%s] is a valid octal value", unixPermissions)
	}
	return err
}

// ChmodVolumeExec method Execute chmod.
func (h Service) ChmodVolumeExec(unixPermissions string, targetPath string) error {
	if unixPermissions != "" {
		if err := ValidateUnixPermissions(unixPermissions); err != nil {
			return err
		}
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
