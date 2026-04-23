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
package nfs

import (
	"encoding/json"
	"log/slog"
	"strconv"
	"strings"

	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/infinidat/infinibox-csi-driver/iboxapi"

	"github.com/container-storage-interface/spec/lib/go/csi"
)

const (
	NFSExportPermNoRootSquash = "no_root_squash"
	NFSExportPermClient       = "client"
	NFSExportPermAccess       = "access"
)

type StorageHelper interface {
	GetNFSMountOptions(req *csi.NodePublishVolumeRequest) ([]string, error)
}

type StorageService struct{}

func getPermissionMaps(permission string) ([]map[string]any, error) {
	permissionFixed := strings.ReplaceAll(permission, "'", "\"")
	var permissionsMapArray []map[string]any
	err := json.Unmarshal([]byte(permissionFixed), &permissionsMapArray)
	if err != nil {
		return permissionsMapArray, common.Errorf("unmarshal error permission: %s permissionFixed: %s perms: %s error: %w", permission, permissionFixed, common.StorageClassNFSExportPermissions, err)
	}

	for _, pass := range permissionsMapArray {
		no_root_squash_str, ok := pass[NFSExportPermNoRootSquash].(string)
		if ok {
			rootsq, err := strconv.ParseBool(no_root_squash_str)
			if err != nil {
				slog.Debug("failed to cast no_root_squash value in export permission - setting default value 'true'")
				rootsq = true
			}
			pass[NFSExportPermNoRootSquash] = rootsq
		}
	}
	return permissionsMapArray, nil
}

// convertToExportRulePermissions converts the permissions from the JSON marshalled format to
// the iboxapi.Permissions, to be used later for updating the permissions with
// the iboxapi
func convertToExportRulePermissions(permissionsMapArray []map[string]interface{}) (apiPermissions []iboxapi.Permissions) {
	for _, pass := range permissionsMapArray {
		ap := iboxapi.Permissions{}
		ap.NoRootSquash = pass[NFSExportPermNoRootSquash].(bool)
		ap.Access = pass[NFSExportPermAccess].(string)
		ap.Client = pass[NFSExportPermClient].(string)
		apiPermissions = append(apiPermissions, ap)
	}
	return apiPermissions
}

// uid should be integer >= -1, if set to -1, then it means don't change
// gid should be integer >= -1, if set to -1, then it means don't change
// unix_permissions should be valid octal value
func ValidateNFSExportPermissions(scParameters map[string]string) error {
	if scParameters[common.StorageClassNFSExportPermissions] != "" {
		permissionsMapArray, err := getPermissionMaps(scParameters[common.StorageClassNFSExportPermissions])
		if err != nil {
			return err
		}

		// validation for uid,gid,unix_permissions
		if scParameters[common.StorageClassUID] != "" || scParameters[common.StorageClassGID] != "" || scParameters[common.StorageClassUNIXPermissions] != "" {
			if len(permissionsMapArray) > 0 {
				noRootSquash := permissionsMapArray[0][NFSExportPermNoRootSquash]
				if noRootSquash == false {
					return common.Errorf("error: uid, gid, or unix_permissions were set, but no_root_squash is false, this is not valid, no_root_squash is required to be true for uid,gid,unix_permissions to be applied")
				}
			}
		}
	}
	return nil
}
