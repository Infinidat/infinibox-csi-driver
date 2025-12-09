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
	"context"
	"errors"
	"log/slog"
	"net"
	"strings"

	"github.com/infinidat/infinibox-csi-driver/common"
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
func (c *ClientService) AddNodeInExport(ctx context.Context, exportID int, access string, noRootSquash bool, ipAddress string) (*iboxapi.Export, error) {
	slog.Log(ctx, common.LevelTrace, "AddNodeInExport() called")
	slog.Debug("Adding node to export", "ip address", ipAddress, "export id", exportID, "access", access)
	flag := false

	export, err := c.IboxAPI.GetExportByID(ctx, exportID)
	if err != nil {
		slog.Error("Error occurred while getting export path for export", "export id", export.ID, "error", err)
		return nil, err
	}

	slog.Debug("Current export", "export id", export.ID, "export", export)

	index := -1
	permissionList := export.Permissions
	for permissionIndex, permission := range permissionList {
		if compareClientIP(permission.Client, ipAddress) {
			flag = true
			slog.Debug("Node IP address already added in export rule", "ip address", ipAddress, "export id", export.ID)
		} else if permission.Client == "*" {
			index = permissionIndex
			flag = true
			slog.Debug("Node IP address already covered by '*' export rule", "ipaddress", ipAddress, "export id", export.ID)
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

		slog.Debug("Setting export", "export id", export.ID, "permissions", permissionList)

		exportPathRef := iboxapi.ExportPathRef{
			Permissions: permissionList,
		}
		export, err = c.IboxAPI.UpdateExportPermissions(ctx, *export, exportPathRef)
		if err != nil {
			slog.Error("Error: updating export rule", "export id", exportID, "access", access, "norootsquash", noRootSquash, "ipaddress", ipAddress, "error", err)
			return nil, err
		}
		slog.Debug("Updated export rule", "export id", export.ID, "export", export)
	}
	slog.Debug("Completed adding node to export", "ipaddress", ipAddress, "export id", export.ID, "export", export)
	return export, nil
}

// DeleteExportRule method
func (c *ClientService) DeleteExportRule(ctx context.Context, fileSystemID int, ipAddress string) error {
	slog.Log(ctx, common.LevelTrace, "Delete export rule from filesystem", "filesystem id", fileSystemID)
	exports, err := c.IboxAPI.GetExportsByFileSystemID(ctx, fileSystemID)
	if err != nil {
		slog.Error("Error occurred while getting export", "error", err)
		return err
	}
	for _, export := range exports {
		permissionList := export.Permissions
		for _, permission := range permissionList {
			if permission.Client == ipAddress {
				_, err = c.DeleteNodeFromExport(ctx, export, permission.NoRootSquash, ipAddress)
				if err != nil {
					slog.Error("Error occurred while getting export path", "error", err)
					return err
				}
			}
		}
	}
	slog.Log(ctx, common.LevelTrace, "Deleted export rule from filesystem", "fs id", fileSystemID)
	return nil
}

// DeleteNodeFromExport Export should be updated in case of node deletion in k8s cluster
func (c *ClientService) DeleteNodeFromExport(ctx context.Context, export iboxapi.Export, noRootSquash bool, ipAddress string) (*iboxapi.Export, error) {
	slog.Log(ctx, common.LevelTrace, "Delete node from export", "export id", export.ID)
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
		exportResponse, err = c.IboxAPI.UpdateExportPermissions(ctx, export, exportPathRef)
		if err != nil {
			slog.Error("Error occurred while updating permission", "error", err)
			return nil, err
		}
	} else {
		slog.Error("Given ip address not found in the list", "ip address", ipAddress)
	}
	slog.Log(ctx, common.LevelTrace, "Deleted node from export", "export id", export.ID)
	return exportResponse, nil
}

const (
	// TOBEDELETED status
	TOBEDELETED = "host.k8s.to_be_deleted"
)

// DeleteParentFileSystem method delete the ascenders of fileystem
func (c *ClientService) DeleteParentFileSystem(ctx context.Context, fileSystemID int) (err error) { // delete fileystem's parent ID
	var metadataList []iboxapi.GetMetadataResult
	metadataList, err = c.IboxAPI.GetMetadata(ctx, fileSystemID)
	if err != nil {
		slog.Error("Failed to delete filesystem", "fs id", fileSystemID, "error", err)
		return err
	}
	var toBeDeleted bool
	for _, metadata := range metadataList {
		if metadata.Key == TOBEDELETED {
			toBeDeleted = true
		}
	}

	childFileSystems, err := c.IboxAPI.GetFileSystemsByParentID(ctx, fileSystemID)
	if err != nil {
		slog.Error("Failed to get filesystem", "fs id", fileSystemID, "error", err)
		return err
	}

	if len(childFileSystems) == 0 && toBeDeleted { // If No child and to_be_delete_status =true in metadata then
		// get the filesystem before deleting so we can recall the ParentID
		fileSystem, err := c.IboxAPI.GetFileSystemByID(ctx, fileSystemID)
		if err != nil {
			slog.Error("Failed to get filesystem", "fs id", fileSystemID, "error", err)
			return err
		}
		err = c.DeleteFileSystemComplete(ctx, fileSystemID) // delete the filesystem
		if err != nil {
			slog.Error("Failed to delete filesystem", "fs id", fileSystemID, "error", err)
			return err
		}
		if fileSystem.ParentID != 0 {
			err = c.DeleteParentFileSystem(ctx, fileSystem.ParentID)
			if err != nil {
				slog.Error("Failed to delete parent filesystem with parent ID", "parent fs id", fileSystemID, "error", err)
				return err
			}
		}
	}
	return
}

// DeleteFileSystemComplete method delete the fileystem
func (c *ClientService) DeleteFileSystemComplete(ctx context.Context, fileSystemID int) (err error) {
	// 1. Delete export path
	exportResp, err := c.IboxAPI.GetExportsByFileSystemID(ctx, fileSystemID)
	if err != nil {
		if !strings.Contains(err.Error(), "EXPORT_NOT_FOUND") {
			slog.Error("failed to delete export path", "error", err)
			return
		}
	}
	for _, export := range exportResp {
		_, err = c.IboxAPI.DeleteExport(ctx, export.ID)
		if err != nil {
			if errors.Is(err, iboxapi.ErrNotFound) {
				slog.Error("failed to delete export path", "error", err)
				return
			}
		}
	}

	slog.Log(ctx, common.LevelTrace, "Export path deleted successfully")

	// 2.delete metadata
	_, err = c.IboxAPI.DeleteMetadata(ctx, fileSystemID)
	if err != nil {
		if !strings.Contains(err.Error(), "METADATA_IS_NOT_SUPPORTED_FOR_ENTITY") {
			slog.Error("failed to delete metadata", "error", err)
			return
		}
	}

	// 3. delete file system
	slog.Log(ctx, common.LevelTrace, "delete FileSystem", "fs id", fileSystemID)
	err = c.IboxAPI.DeleteFileSystem(ctx, fileSystemID)
	if err != nil {
		slog.Error("failed to delete filesystem", "error", err)
		return
	}
	return
}

func removeIndex(s []iboxapi.Permissions, index int) []iboxapi.Permissions {
	return append(s[:index], s[index+1:]...)
}
