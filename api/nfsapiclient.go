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
	"infinibox-csi-driver/iboxapi"
	"net"
	"strings"
)

func compareClientIP(permissionIP, ip string) bool {
	flag := false
	if strings.Contains(permissionIP, "-") {
		iprange := strings.Split(permissionIP, "-")
		ip1 := net.ParseIP(iprange[0])
		ip2 := net.ParseIP(iprange[1])
		clientIP := net.ParseIP(ip)
		if bytes.Compare(clientIP, ip1) >= 0 && bytes.Compare(clientIP, ip2) <= 0 {
			flag = true
		}
	} else if permissionIP == ip {
		flag = true
	}
	return flag
}

// AddNodeInExport : Export should be updated in case of node addition in k8s cluster
func (c *ClientService) AddNodeInExport(id int, access string, noRootSquash bool, ip string) (*iboxapi.Export, error) {
	zlog.Trace().Msgf("AddNodeInExport() called")
	zlog.Trace().Msgf("Adding node with IP %s to export with export ID %d using access '%s'", ip, id, access)
	flag := false

	ex, err := c.Iboxapi.GetExportByID(id)
	if err != nil {
		zlog.Error().Msgf("Error occurred while getting export path for export with ID %d: %s", ex.ID, err)
		return nil, err
	}

	zlog.Trace().Msgf("Current export with export ID %d. export: %v", ex.ID, ex)

	index := -1
	permissionList := ex.Permissions
	for i, permission := range permissionList {
		if compareClientIP(permission.Client, ip) {
			flag = true
			zlog.Trace().Msgf("Node IP address %s already added in export rule with ID %d", ip, ex.ID)
		} else if permission.Client == "*" {
			index = i
			flag = true
			zlog.Trace().Msgf("Node IP address %s already covered by '*' export rule for export ID %d", ip, ex.ID)
		}
	}
	if index != -1 {
		permissionList = removeIndex(permissionList, index)
	}
	if !flag {
		newPermission := iboxapi.Permissions{
			Access:       access,
			NoRootSquash: noRootSquash,
			Client:       ip,
		}
		permissionList = append(permissionList, newPermission)

		zlog.Trace().Msgf("Setting export with ID %d permissions to %+v", ex.ID, permissionList)

		exportPathRef := iboxapi.ExportPathRef{
			Permissions: permissionList,
		}
		ex, err = c.Iboxapi.UpdateExport(*ex, exportPathRef)
		if err != nil {
			zlog.Error().Msgf("Error occurred while updating export rule for export with ID %d: %s", ex.ID, err)
			return nil, err
		} else {
			zlog.Trace().Msgf("Updated export rule for export with ID %d, export Response %v", ex.ID, ex)
		}
	}
	zlog.Trace().Msgf("Completed adding node %s to export with export ID %d: %+v", ip, ex.ID, ex)
	return ex, nil
}

// DeleteExportRule method
func (c *ClientService) DeleteExportRule(fileSystemID int, ipAddress string) error {
	zlog.Trace().Msgf("Delete export rule from filesystem with file system ID %d", fileSystemID)
	exportArray, err := c.Iboxapi.GetExportsByFileSystemID(fileSystemID)
	if err != nil {
		zlog.Error().Msgf("Error occured while getting export : %v", err)
		return err
	}
	for _, export := range exportArray {
		permissionList := export.Permissions
		for _, permission := range permissionList {
			if permission.Client == ipAddress {
				_, err = c.DeleteNodeFromExport(export, permission.Access, permission.NoRootSquash, ipAddress)
				if err != nil {
					zlog.Error().Msgf("Error occured while getting export path : %s", err)
					return err
				}
			}
		}
	}
	zlog.Trace().Msgf("Deleted export rule from filesystem with ID %d", fileSystemID)
	return nil
}

// DeleteNodeFromExport Export should be updated in case of node deletion in k8s cluster
func (c *ClientService) DeleteNodeFromExport(export iboxapi.Export, access string, noRootSquash bool, ip string) (*iboxapi.Export, error) {
	zlog.Trace().Msgf("Delete node from export with export ID %d", export.ID)
	flag := false
	var index int
	exportPathRef := iboxapi.ExportPathRef{}
	var exportResponse *iboxapi.Export
	permissionList := export.Permissions
	for i, permission := range permissionList {
		if permission.Client == ip {
			flag = true
			index = i
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
		exportResponse, err = c.Iboxapi.UpdateExport(export, exportPathRef)
		if err != nil {
			zlog.Error().Msgf("Error occured while updating permission : %s", err)
			return nil, err
		}
	} else {
		zlog.Error().Msgf("Given Ip %s address not found in the list", ip)
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

	var metadata []iboxapi.GetMetadataResult
	metadata, err = c.Iboxapi.GetMetadata(fileSystemID)
	if err != nil {
		zlog.Error().Msgf("Failed to delete filesystem with ID %d, error getting metadata: %v", fileSystemID, err)
		return err
	}
	var toBeDeleted bool
	for _, m := range metadata {
		if m.Key == TOBEDELETED {
			toBeDeleted = true
		}
	}

	childFileSystems, err := c.Iboxapi.GetFileSystemsByParentID(fileSystemID)
	if err != nil {
		zlog.Error().Msgf("Failed to get filesystem with parent ID %d: %v", fileSystemID, err)
		return err
	}

	if len(childFileSystems) == 0 && toBeDeleted { // If No child and to_be_delete_status =true in metadata then

		// get the filesystem before deleting so we can recall the ParentID
		fs, err := c.Iboxapi.GetFileSystemByID(fileSystemID)
		if err != nil {
			zlog.Error().Msgf("Failed to get filesystem with ID %d: %v", fileSystemID, err)
			return err
		}
		err = c.DeleteFileSystemComplete(fileSystemID) // delete the filesystem
		if err != nil {
			zlog.Error().Msgf("Failed to delete filesystem with ID %d: %v", fileSystemID, err)
			return err
		}
		if fs.ParentID != 0 {
			err = c.DeleteParentFileSystem(fs.ParentID)
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
	exportResp, err := c.Iboxapi.GetExportsByFileSystemID(fileSystemID)
	if err != nil {
		if strings.Contains(err.Error(), "EXPORT_NOT_FOUND") {
			err = nil
		} else {
			zlog.Error().Msgf("failed to delete export path %v", err)
			return
		}
	}
	for _, ep := range exportResp {
		_, err = c.Iboxapi.DeleteExport(ep.ID)
		if err != nil {
			re, ok := err.(*iboxapi.IboxAPIError)
			if ok && re.Code == iboxapi.IBOXAPI_RESOURCE_NOT_FOUND_ERROR {
				err = nil
			} else {
				zlog.Error().Msgf("failed to delete export path %v", err)
				return
			}
		}
	}

	zlog.Trace().Msgf("Export path deleted successfully")

	// 2.delete metadata
	_, err = c.Iboxapi.DeleteMetadata(fileSystemID)
	if err != nil {
		if strings.Contains(err.Error(), "METADATA_IS_NOT_SUPPORTED_FOR_ENTITY") {
			err = nil
		} else {
			zlog.Error().Msgf("failed to delete metadata %v", err)
			return
		}
	}

	// 3. delete file system
	zlog.Trace().Msgf("delete FileSystem FileSystemID %d", fileSystemID)
	err = c.Iboxapi.DeleteFileSystem(fileSystemID)
	if err != nil {
		zlog.Error().Msgf("failed to delete filesystem %v", err)
		return
	}
	return
}

func removeIndex(s []iboxapi.Permissions, index int) []iboxapi.Permissions {
	return append(s[:index], s[index+1:]...)
}
