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
package api

import (
	"bytes"
	"net"
	"strings"

	"github.com/infinidat/infinibox-csi-driver/iboxapi"
)

func compareClientIP(permissionIP, ipAddress string) bool {
	flag := false
	if strings.Contains(permissionIP, "-") {
		iprange := strings.Split(permissionIP, "-")
		ip1 := net.ParseIP(iprange[0])
		ip2 := net.ParseIP(iprange[1])
		clientIP := net.ParseIP(ipAddress)
		if bytes.Compare(clientIP, ip1) >= 0 && bytes.Compare(clientIP, ip2) <= 0 {
			flag = true
		}
	} else if permissionIP == ipAddress {
		flag = true
	}
	return flag
}

// AddNodeInExport : Export should be updated in case of node addition in k8s cluster
func (c *ClientService) AddNodeInExport(exportID int, access string, noRootSquash bool, ipAddress string) (*iboxapi.Export, error) {
	zlog.Trace().Msgf("AddNodeInExport() called")
	zlog.Debug().Msgf("Adding node with IP %s to export with export ID %d using access '%s'", ipAddress, exportID, access)
	flag := false

	export, err := c.IboxAPI.GetExportByID(exportID)
	if err != nil {
		zlog.Error().Msgf("Error occurred while getting export path for export with ID %d: %s", export.ID, err)
		return nil, err
	}

	zlog.Debug().Msgf("Current export with export ID %d. export: %v", export.ID, export)

	index := -1
	permissionList := export.Permissions
	for permissionIndex, permission := range permissionList {
		if compareClientIP(permission.Client, ipAddress) {
			flag = true
			zlog.Debug().Msgf("Node IP address %s already added in export rule with ID %d", ipAddress, export.ID)
		} else if permission.Client == "*" {
			index = permissionIndex
			flag = true
			zlog.Debug().Msgf("Node IP address %s already covered by '*' export rule for export ID %d", ipAddress, export.ID)
		}
	}
	if index != -1 {
		permissionList = removeIndex(permissionList, index)
	}
	if !flag {
		newPermission := iboxapi.Permissions{
			Access:       access,
			NoRootSquash: noRootSquash,
			Client:       ipAddress,
		}
		permissionList = append(permissionList, newPermission)

		zlog.Debug().Msgf("Setting export with ID %d permissions to %+v", export.ID, permissionList)

		exportPathRef := iboxapi.ExportPathRef{
			Permissions: permissionList,
		}
		export, err = c.IboxAPI.UpdateExportPermissions(*export, exportPathRef)
		if err != nil {
			zlog.Error().Msgf("Error: updating export rule for export with ID: %d access: %s noRootSquash: %t ip:%s error: %s", exportID, access, noRootSquash, ipAddress, err)
			return nil, err
		}
		zlog.Debug().Msgf("Updated export rule for export with ID %d, export Response %v", export.ID, export)
	}
	zlog.Debug().Msgf("Completed adding node %s to export with export ID %d: %+v", ipAddress, export.ID, export)
	return export, nil
}

// DeleteExportRule method
func (c *ClientService) DeleteExportRule(fileSystemID int, ipAddress string) error {
	zlog.Trace().Msgf("Delete export rule from filesystem with file system ID %d", fileSystemID)
	exports, err := c.IboxAPI.GetExportsByFileSystemID(fileSystemID)
	if err != nil {
		zlog.Error().Msgf("Error occurred while getting export : %v", err)
		return err
	}
	for _, export := range exports {
		permissionList := export.Permissions
		for _, permission := range permissionList {
			if permission.Client == ipAddress {
				_, err = c.DeleteNodeFromExport(export, permission.NoRootSquash, ipAddress)
				if err != nil {
					zlog.Error().Msgf("Error occurred while getting export path : %s", err)
					return err
				}
			}
		}
	}
	zlog.Trace().Msgf("Deleted export rule from filesystem with ID %d", fileSystemID)
	return nil
}

// DeleteNodeFromExport Export should be updated in case of node deletion in k8s cluster
func (c *ClientService) DeleteNodeFromExport(export iboxapi.Export, noRootSquash bool, ipAddress string) (*iboxapi.Export, error) {
	zlog.Trace().Msgf("Delete node from export with export ID %d", export.ID)
	flag := false
	var index int
	exportPathRef := iboxapi.ExportPathRef{}
	var exportResponse *iboxapi.Export
	permissionList := export.Permissions
	for permissionIndex, permission := range permissionList {
		if permission.Client == ipAddress {
			flag = true
			index = permissionIndex
		}
	}

	if flag {
		permissionList = removeIndex(permissionList, index)
		if len(permissionList) == 0 {
			defaultPermission := iboxapi.Permissions{}
			defaultPermission.Access = "RW"
			defaultPermission.Client = "*"
			defaultPermission.NoRootSquash = true
			permissionList = append(permissionList, defaultPermission)
		}
		exportPathRef.Permissions = permissionList

		var err error
		exportResponse, err = c.IboxAPI.UpdateExportPermissions(export, exportPathRef)
		if err != nil {
			zlog.Error().Msgf("Error occurred while updating permission : %s", err)
			return nil, err
		}
	} else {
		zlog.Error().Msgf("Given Ip %s address not found in the list", ipAddress)
	}
	zlog.Trace().Msgf("Deleted node from export with ID %d", export.ID)
	return exportResponse, nil
}

const (
	// TOBEDELETED status
	TOBEDELETED = "host.k8s.to_be_deleted"
)

// DeleteParentFileSystem method delete the ascenders of fileystem
func (c *ClientService) DeleteParentFileSystem(fileSystemID int) (err error) { // delete fileystem's parent ID
	var metadataList []iboxapi.GetMetadataResult
	metadataList, err = c.IboxAPI.GetMetadata(fileSystemID)
	if err != nil {
		zlog.Error().Msgf("Failed to delete filesystem with ID %d, error getting metadata: %v", fileSystemID, err)
		return err
	}
	var toBeDeleted bool
	for _, metadata := range metadataList {
		if metadata.Key == TOBEDELETED {
			toBeDeleted = true
		}
	}

	childFileSystems, err := c.IboxAPI.GetFileSystemsByParentID(fileSystemID)
	if err != nil {
		zlog.Error().Msgf("Failed to get filesystem with parent ID %d: %v", fileSystemID, err)
		return err
	}

	if len(childFileSystems) == 0 && toBeDeleted { // If No child and to_be_delete_status =true in metadata then
		// get the filesystem before deleting so we can recall the ParentID
		fileSystem, err := c.IboxAPI.GetFileSystemByID(fileSystemID)
		if err != nil {
			zlog.Error().Msgf("Failed to get filesystem with ID %d: %v", fileSystemID, err)
			return err
		}
		err = c.DeleteFileSystemComplete(fileSystemID) // delete the filesystem
		if err != nil {
			zlog.Error().Msgf("Failed to delete filesystem with ID %d: %v", fileSystemID, err)
			return err
		}
		if fileSystem.ParentID != 0 {
			err = c.DeleteParentFileSystem(fileSystem.ParentID)
			if err != nil {
				zlog.Error().Msgf("Failed to delete parent filesystem with parent ID %d: %v", fileSystemID, err)
				return err
			}
		}
	}
	return
}

// DeleteFileSystemComplete method delete the fileystem
func (c *ClientService) DeleteFileSystemComplete(fileSystemID int) (err error) {
	// 1. Delete export path
	exportResp, err := c.IboxAPI.GetExportsByFileSystemID(fileSystemID)
	if err != nil {
		if !strings.Contains(err.Error(), "EXPORT_NOT_FOUND") {
			zlog.Error().Msgf("failed to delete export path %v", err)
			return
		}
	}
	for _, export := range exportResp {
		_, err = c.IboxAPI.DeleteExport(export.ID)
		if err != nil {
			re, ok := err.(*iboxapi.APIError)
			if ok && re.Code != iboxapi.IBOXAPI_RESOURCE_NOT_FOUND_ERROR {
				zlog.Error().Msgf("failed to delete export path %v", err)
				return
			}
		}
	}

	zlog.Trace().Msgf("Export path deleted successfully")

	// 2.delete metadata
	_, err = c.IboxAPI.DeleteMetadata(fileSystemID)
	if err != nil {
		if !strings.Contains(err.Error(), "METADATA_IS_NOT_SUPPORTED_FOR_ENTITY") {
			zlog.Error().Msgf("failed to delete metadata %v", err)
			return
		}
	}

	// 3. delete file system
	zlog.Trace().Msgf("delete FileSystem FileSystemID %d", fileSystemID)
	err = c.IboxAPI.DeleteFileSystem(fileSystemID)
	if err != nil {
		zlog.Error().Msgf("failed to delete filesystem %v", err)
		return
	}
	return
}

func removeIndex(s []iboxapi.Permissions, index int) []iboxapi.Permissions {
	return append(s[:index], s[index+1:]...)
}
