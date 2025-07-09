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
package storage

import (
	"encoding/json"
	"fmt"
	"infinibox-csi-driver/common"
	"infinibox-csi-driver/iboxapi"
	"strconv"
	"strings"
)

const (
	NFS_EXPORT_PERM_NO_ROOT_SQUASH = "no_root_squash"
	NFS_EXPORT_PERM_CLIENT         = "client"
	NFS_EXPORT_PERM_ACCESS         = "access"
)

func getPermissionMaps(permission string) ([]map[string]interface{}, error) {
	permissionFixed := strings.ReplaceAll(permission, "'", "\"")
	var permissionsMapArray []map[string]interface{}
	err := json.Unmarshal([]byte(permissionFixed), &permissionsMapArray)
	if err != nil {
		zlog.Error().Msgf("invalid %s format %v raw [%s] fixed [%s]", common.SC_NFS_EXPORT_PERMISSIONS, err, permission, permissionFixed)
		return permissionsMapArray, err
	}

	for _, pass := range permissionsMapArray {
		no_root_squash_str, ok := pass[NFS_EXPORT_PERM_NO_ROOT_SQUASH].(string)
		if ok {
			rootsq, err := strconv.ParseBool(no_root_squash_str)
			if err != nil {
				zlog.Debug().Msgf("failed to cast no_root_squash value in export permission - setting default value 'true'")
				rootsq = true
			}
			pass[NFS_EXPORT_PERM_NO_ROOT_SQUASH] = rootsq
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
		ap.NoRootSquash = pass[NFS_EXPORT_PERM_NO_ROOT_SQUASH].(bool)
		ap.Access = pass[NFS_EXPORT_PERM_ACCESS].(string)
		ap.Client = pass[NFS_EXPORT_PERM_CLIENT].(string)
		apiPermissions = append(apiPermissions, ap)
	}
	return apiPermissions
}

// uid should be integer >= -1, if set to -1, then it means don't change
// gid should be integer >= -1, if set to -1, then it means don't change
// unix_permissions should be valid octal value
func validateNFSExportPermissions(scParameters map[string]string) error {
	if scParameters[common.SC_NFS_EXPORT_PERMISSIONS] == "" {
		// the case when nfs_export_permissions is not set by a user in the SC
	} else {
		permissionsMapArray, err := getPermissionMaps(scParameters[common.SC_NFS_EXPORT_PERMISSIONS])
		if err != nil {
			zlog.Err(err)
			return err
		}

		// validation for uid,gid,unix_permissions
		if scParameters[common.SC_UID] != "" || scParameters[common.SC_GID] != "" || scParameters[common.SC_UNIX_PERMISSIONS] != "" {
			if len(permissionsMapArray) > 0 {
				noRootSquash := permissionsMapArray[0][NFS_EXPORT_PERM_NO_ROOT_SQUASH]
				if noRootSquash == false {
					e := fmt.Errorf("error: uid, gid, or unix_permissions were set, but no_root_squash is false, this is not valid, no_root_squash is required to be true for uid,gid,unix_permissions to be applied")
					zlog.Err(e)
					return e
				}
			}
		}
	}
	return nil
}
