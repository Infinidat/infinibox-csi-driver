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
    "github.com/google/uuid"
	"k8s.io/klog"
)

var execScsi ExecScsi

func RegenerateXfsFilesystemUuid(devicePath string) (err error) {
	klog.V(4).Infof("RegenerateXfsFilesystemUuid called")

	allow_xfs_uuid_regeneration := os.Getenv("ALLOW_XFS_UUID_REGENERATION")
	allow_uuid_fix, err := YamlBoolToBool(allow_xfs_uuid_regeneration)
	if err != nil {
		klog.Errorf("Invalid ALLOW_XFS_UUID_REGENERATION variable: %s", err)
		return err
	}

	if allow_uuid_fix {
		newUuid := uuid.New()

		klog.Errorf("Device %s has duplicate XFS UUID. New UUID: %s. ALLOW_XFS_UUID_REGENERATION is set to %s", devicePath, newUuid, allow_xfs_uuid_regeneration)

		klog.V(4).Infof("Update device '%s' UUID with '%s'", devicePath, newUuid)
		_, err_xfs := execScsi.Command("xfs_admin", fmt.Sprintf("-U %s %s", newUuid, devicePath))

		if err_xfs != nil {
			msg := fmt.Sprintf("xfs_admin failed. Volume likely to be read-only: %s", err_xfs)
			klog.V(4).Infof(msg)
			return errors.New(msg)
		}
	} else {
		klog.Errorf("Device %s has duplicate XFS UUID and cannot be mounted. ALLOW_XFS_UUID_REGENERATION is set to %s", devicePath, allow_xfs_uuid_regeneration)
		return err
	}
	return nil
}
