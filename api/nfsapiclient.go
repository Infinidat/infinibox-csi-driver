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
	"errors"
	"fmt"
	"infinibox-csi-driver/api/client"
	"infinibox-csi-driver/iboxapi"
	"net"
	"net/http"
	"reflect"
	"strconv"
	"strings"
)

// AttachMetadataToObject :
func (c *ClientService) AttachMetadataToObject(objectID int, body map[string]interface{}) (*[]Metadata, error) {
	zlog.Trace().Msgf("Attach metadata: %v to object id: %d", body, objectID)
	uri := "api/rest/metadata/" + strconv.Itoa(objectID)
	metadata := []Metadata{}
	resp, err := c.getJSONResponse(http.MethodPut, uri, body, &metadata)
	if err != nil {
		zlog.Error().Msgf("Error occured while attaching metadata to object id: %d, %s", objectID, err)
		return nil, err
	}
	if len(metadata) == 0 {
		apiresp := resp.(client.ApiResponse)
		metadata, _ = apiresp.Result.([]Metadata)
	}
	zlog.Trace().Msgf("Attached metadata to object id: %d", objectID)
	return &metadata, nil
}

// DetachMetadataFromObject :
func (c *ClientService) DetachMetadataFromObject(objectID int) (*[]Metadata, error) {
	zlog.Trace().Msgf("Detach metadata from object with ID %d", objectID)
	uri := "api/rest/metadata/" + strconv.Itoa(objectID) + "?approved=true"
	metadata := []Metadata{}
	resp, err := c.getJSONResponse(http.MethodDelete, uri, nil, &metadata)
	if err != nil {
		if strings.Contains(err.Error(), "METADATA_IS_NOT_SUPPORTED_FOR_ENTITY") {
			err = nil
		}
		zlog.Error().Msgf("Error occured while detaching metadata from object : %s ", err)
		return nil, err
	}
	if len(metadata) == 0 {
		apiresp := resp.(client.ApiResponse)
		metadata, _ = apiresp.Result.([]Metadata)
	}
	zlog.Trace().Msgf("Detached metadata from object with ID %d", objectID)
	return &metadata, nil
}

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
func (c *ClientService) AddNodeInExport(exportID int, access string, noRootSquash bool, ip string) (*ExportResponse, error) {
	zlog.Trace().Msgf("AddNodeInExport() called")
	zlog.Trace().Msgf("Adding node with IP %s to export with export ID %d using access '%s'", ip, exportID, access)
	flag := false

	uri := "api/rest/exports/" + strconv.Itoa(exportID)
	eResp := ExportResponse{}

	resp, err := c.getJSONResponse(http.MethodGet, uri, nil, &eResp)
	if err != nil {
		zlog.Error().Msgf("Error occurred while getting export path for export with ID %d: %s", exportID, err)
		return nil, err
	}

	zlog.Trace().Msgf("Current export with export ID %d. Response type: %T,  response: %v", exportID, resp, resp)
	if respApiResponse, ok := resp.(client.ApiResponse); !ok {
		msg := fmt.Sprintf("Getting current export with ID %d returned a resp that is not of type client.ApiResponse", exportID)
		zlog.Error().Msg(msg)
		err = errors.New(msg)
		return nil, err
	} else {
		var respResult interface{} = respApiResponse.Result

		if exportResponse, ok := respResult.(*ExportResponse); !ok {
			msg := fmt.Sprintf("Export response for export with ID %d is not of type ExportResponse", exportID)
			zlog.Trace().Msg(msg)
		} else {
			zlog.Trace().Msgf("Current export with export ID %d. exportResponse: %v", exportID, *exportResponse)
		}
	}

	// TODO - Remove this block. Needed only for allowing UT to pass.
	//        UT: TestServiceTestSuite/Test_AddNodeInExport_IPAddress_exist_success
	if reflect.DeepEqual(eResp, ExportResponse{}) {
		zlog.Trace().Msgf("DeepEqual(eResp, ExportResponse{}) is true")
		apiresp := resp.(client.ApiResponse)
		eResp, _ = apiresp.Result.(ExportResponse)
		zlog.Trace().Msgf("Current export with export ID %d. apiresp type: %T, apiresp: %v", exportID, apiresp, apiresp)
		zlog.Trace().Msgf("Current export with export ID %d. eResp type: %T, eResp: %v", exportID, eResp, eResp)
	} else {
		zlog.Trace().Msgf("DeepEqual(eResp, ExportResponse{}) is false")
	}

	index := -1
	permissionList := eResp.Permissions
	for i, permission := range permissionList {
		if compareClientIP(permission.Client, ip) {
			flag = true
			zlog.Trace().Msgf("Node IP address %s already added in export rule with ID %d", ip, exportID)
		} else if permission.Client == "*" {
			index = i
			flag = true
			zlog.Trace().Msgf("Node IP address %s already covered by '*' export rule for export ID %d", ip, exportID)
		}
	}
	if index != -1 {
		permissionList = removeIndex(permissionList, index)
	}
	if !flag {
		newPermission := Permissions{
			Access:       access,
			NoRootSquash: noRootSquash,
			Client:       ip,
		}
		permissionList = append(permissionList, newPermission)

		exportPermissions := ExportPermissions{}
		exportPermissions.Permissions = permissionList
		zlog.Trace().Msgf("Setting export with ID %d permissions to %+v", exportID, exportPermissions)
		resp, err = c.getJSONResponse(http.MethodPut, uri, exportPermissions, &eResp)
		if err != nil {
			zlog.Error().Msgf("Error occurred while updating export rule for export with ID %d: %s", exportID, err)
			return nil, err
		} else {
			zlog.Trace().Msgf("Updated export rule for export with ID %d, resp %v, eResp %v", exportID, resp, eResp)
		}
	}
	zlog.Trace().Msgf("Completed adding node %s to export with export ID %d: %+v", ip, exportID, eResp)
	return &eResp, nil
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
		uri := "api/rest/exports/" + strconv.Itoa(export.ID)
		eResp := ExportResponse{}
		_, err := c.getJSONResponse(http.MethodGet, uri, nil, &eResp)
		if err != nil {
			zlog.Error().Msgf("Error occured while getting export path : %s", err)
			return err
		}
		permissionList := eResp.Permissions
		for _, permission := range permissionList {
			if permission.Client == ipAddress {
				_, err = c.DeleteNodeFromExport(export.ID, permission.Access, permission.NoRootSquash, ipAddress)
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
func (c *ClientService) DeleteNodeFromExport(exportID int, access string, noRootSquash bool, ip string) (*ExportResponse, error) {
	zlog.Trace().Msgf("Delete node from export with export ID %d", exportID)
	flag := false
	var index int
	exportPathRef := ExportPathRef{}
	uri := "api/rest/exports/" + strconv.Itoa(exportID)
	eResp := ExportResponse{}
	_, err := c.getJSONResponse(http.MethodGet, uri, nil, &eResp)
	if err != nil {
		zlog.Error().Msgf("Error occured while getting export path : %s", err)
		return nil, err
	}
	permissionList := eResp.Permissions
	for i, permission := range permissionList {
		if permission.Client == ip {
			flag = true
			index = i
		}
	}

	if flag {
		permissionList = removeIndex(permissionList, index)
		if len(permissionList) == 0 {
			defaultPermission := Permissions{}
			defaultPermission.Access = "RW"
			defaultPermission.Client = "*"
			defaultPermission.NoRootSquash = true
			permissionList = append(permissionList, defaultPermission)
		}
		exportPathRef.Permissions = permissionList
		resp, err := c.getJSONResponse(http.MethodPut, uri, exportPathRef, &eResp)
		if err != nil {
			zlog.Error().Msgf("Error occured while updating permission : %s", err)
			return nil, err
		}
		if reflect.DeepEqual(eResp, ExportResponse{}) {
			zlog.Trace().Msgf("inside DeepEquals Deleted node from export with ID %d", exportID)
			eResp, _ = resp.(ExportResponse)
		}
	} else {
		zlog.Error().Msgf("Given Ip %s address not found in the list", ip)
	}
	zlog.Trace().Msgf("Deleted node from export with ID %d", exportID)
	return &eResp, nil
}

// CreateFileSystemSnapshot method create the filesystem snapshot
func (c *ClientService) CreateFileSystemSnapshot(lockExpiresAt int64, snapshotParam *FileSystemSnapshot) (*FileSystemSnapshotResponse, error) {
	zlog.Trace().Msgf("Create a snapshot of filesystem params %+v", snapshotParam)
	path := "/api/rest/filesystems"
	snapShotResponse := FileSystemSnapshotResponse{}
	if lockExpiresAt > 0 {
		path = path + "?approved=true"
		tmp := &FileSystemSnapshotLocked{}
		tmp.LockExpiresAt = lockExpiresAt
		tmp.ParentID = snapshotParam.ParentID
		tmp.SnapshotName = snapshotParam.SnapshotName
		tmp.WriteProtected = snapshotParam.WriteProtected
		resp, err := c.getJSONResponse(http.MethodPost, path, tmp, &snapShotResponse)
		if err != nil {
			zlog.Error().Msgf("failed to create %v", err)
			return nil, err
		}
		if (FileSystemSnapshotResponse{}) == snapShotResponse {
			apiresp := resp.(client.ApiResponse)
			snapShotResponse, _ = apiresp.Result.(FileSystemSnapshotResponse)
		}
	} else {
		resp, err := c.getJSONResponse(http.MethodPost, path, snapshotParam, &snapShotResponse)
		if err != nil {
			zlog.Error().Msgf("failed to create %v", err)
			return nil, err
		}
		if (FileSystemSnapshotResponse{}) == snapShotResponse {
			apiresp := resp.(client.ApiResponse)
			snapShotResponse, _ = apiresp.Result.(FileSystemSnapshotResponse)
		}
	}
	zlog.Trace().Msgf("Created snapshot: %s", snapShotResponse.Name)
	return &snapShotResponse, nil
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

func removeIndex(s []Permissions, index int) []Permissions {
	return append(s[:index], s[index+1:]...)
}

// GetSnapshotByName :
func (c *ClientService) GetSnapshotByName(snapshotName string) (*[]FileSystemSnapshotResponse, error) {
	zlog.Trace().Msgf("Get snapshot %s", snapshotName)
	uri := "api/rest/filesystems?name=" + snapshotName
	snapshot := []FileSystemSnapshotResponse{}
	resp, err := c.getJSONResponse(http.MethodGet, uri, nil, &snapshot)
	if err != nil {
		zlog.Error().Msgf("Error occured while getting snapshot : %s ", err)
		return nil, err
	}
	if len(snapshot) == 0 {
		zlog.Trace().Msgf("no snapshot found for name %s", snapshotName)
		snapshot, _ = resp.([]FileSystemSnapshotResponse)
	}
	zlog.Trace().Msgf("Got snapshot %s", snapshotName)
	return &snapshot, nil
}
