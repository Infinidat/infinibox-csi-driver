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
	"net"
	"net/http"
	"reflect"
	"strconv"
	"strings"
)

// DeleteExportPath :
func (c *ClientService) DeleteExportPath(exportID int64) (*ExportResponse, error) {
	zlog.Trace().Msgf("Deleting export path with ID %d", exportID)
	uri := "api/rest/exports/" + strconv.FormatInt(exportID, 10) + "?approved=true"
	eResp := ExportResponse{}
	resp, err := c.getJSONResponse(http.MethodDelete, uri, nil, &eResp)
	if err != nil {
		zlog.Error().Msgf("Error occured while deleting export path : %s ", err)
		return nil, err
	}
	// this next DeepEqual block actually gets called
	if reflect.DeepEqual(eResp, (ExportResponse{})) {
		apiresp := resp.(client.ApiResponse)
		eResp, _ = apiresp.Result.(ExportResponse)
	}
	zlog.Trace().Msgf("Deleted export path with ID %d", exportID)
	return &eResp, nil
}

// DeleteFileSystem :
func (c *ClientService) DeleteFileSystem(fileSystemID int64) (*FileSystem, error) {
	zlog.Trace().Msgf("Delete filesystem with ID %d", fileSystemID)
	uri := "api/rest/filesystems/" + strconv.FormatInt(fileSystemID, 10) + "?approved=true"
	fileSystem := FileSystem{}
	resp, err := c.getJSONResponse(http.MethodDelete, uri, nil, &fileSystem)
	if err != nil {
		zlog.Error().Msgf("Error occured while deleting file System : %s ", err)
		return nil, err
	}
	if fileSystem == (FileSystem{}) {
		apiresp := resp.(client.ApiResponse)
		fileSystem, _ = apiresp.Result.(FileSystem)
	}
	zlog.Trace().Msgf("Deleted filesystem with ID %d", fileSystemID)
	return &fileSystem, nil
}

// AttachMetadataToObject :
func (c *ClientService) AttachMetadataToObject(objectID int64, body map[string]interface{}) (*[]Metadata, error) {
	zlog.Trace().Msgf("Attach metadata: %v to object id: %d", body, objectID)
	uri := "api/rest/metadata/" + strconv.FormatInt(objectID, 10)
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
func (c *ClientService) DetachMetadataFromObject(objectID int64) (*[]Metadata, error) {
	zlog.Trace().Msgf("Detach metadata from object with ID %d", objectID)
	uri := "api/rest/metadata/" + strconv.FormatInt(objectID, 10) + "?approved=true"
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

// CreateFilesystem :
func (c *ClientService) CreateFilesystem(fileSysparameter map[string]interface{}) (*FileSystem, error) {
	zlog.Trace().Msgf("Create filesystem")
	uri := "api/rest/filesystems/"
	fileSystemResp := FileSystem{}
	resp, err := c.getJSONResponse(http.MethodPost, uri, fileSysparameter, &fileSystemResp)
	if err != nil {
		zlog.Error().Msgf("Error occured while creating filesystem : %s", err)
		return nil, err
	}
	if fileSystemResp == (FileSystem{}) {
		apiresp := resp.(client.ApiResponse)
		fileSystemResp, _ = apiresp.Result.(FileSystem)
	}
	zlog.Trace().Msgf("Created filesystem: %s", fileSystemResp.Name)
	return &fileSystemResp, nil
}

// ExportFileSystem :
func (c *ClientService) ExportFileSystem(export ExportFileSys) (*ExportResponse, error) {
	zlog.Trace().Msgf("Export FileSystem with ID %d", export.FilesystemID)
	urlPost := "api/rest/exports"
	exportResp := ExportResponse{}
	_, err := c.getJSONResponse(http.MethodPost, urlPost, export, &exportResp)
	if err != nil {
		return nil, err
	}

	zlog.Trace().Msgf("Exported FileSystem with ID %d", exportResp.FilesystemId)
	return &exportResp, nil
}

// GetExportByFileSystem :
func (c *ClientService) GetExportByFileSystem(fileSystemID int64) (*[]ExportResponse, error) {
	zlog.Trace().Msgf("Get export paths of filesystem with ID %d", fileSystemID)
	uri := "api/rest/exports?filesystem_id=" + strconv.FormatInt(fileSystemID, 10)
	eResp := []ExportResponse{}
	resp, err := c.getJSONResponse(http.MethodGet, uri, nil, &eResp)
	if err != nil {
		zlog.Error().Msgf("Error occured while getting export path : %s", err)
		return nil, err
	}
	if len(eResp) == 0 {
		apiresp := resp.(client.ApiResponse)
		eResp, _ = apiresp.Result.([]ExportResponse)
	}
	zlog.Trace().Msgf("Got export paths of filesystem with ID %d", fileSystemID)
	return &eResp, nil
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
		zlog.Error().Msgf(msg)
		err = errors.New(msg)
		return nil, err
	} else {
		var respResult interface{} = respApiResponse.Result

		if exportResponse, ok := respResult.(*ExportResponse); !ok {
			msg := fmt.Sprintf("Export response for export with ID %d is not of type ExportResponse", exportID)
			zlog.Trace().Msgf(msg)
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
func (c *ClientService) DeleteExportRule(fileSystemID int64, ipAddress string) error {
	zlog.Trace().Msgf("Delete export rule from filesystem with file system ID %d", fileSystemID)
	exportArray, err := c.GetExportByFileSystem(fileSystemID)
	if err != nil {
		zlog.Error().Msgf("Error occured while getting export : %v", err)
		return err
	}
	for _, export := range *exportArray {
		uri := "api/rest/exports/" + strconv.FormatInt(export.ID, 10)
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
func (c *ClientService) DeleteNodeFromExport(exportID int64, access string, noRootSquash bool, ip string) (*ExportResponse, error) {
	zlog.Trace().Msgf("Delete node from export with export ID %d", exportID)
	flag := false
	var index int
	exportPathRef := ExportPathRef{}
	uri := "api/rest/exports/" + strconv.FormatInt(exportID, 10)
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
func (c *ClientService) CreateFileSystemSnapshot(lockExpiresAt int64, snapshotParam *FileSystemSnapshot) (*FileSystemSnapshotResponce, error) {
	zlog.Trace().Msgf("Create a snapshot of filesystem params %+v", snapshotParam)
	path := "/api/rest/filesystems"
	snapShotResponse := FileSystemSnapshotResponce{}
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
		if (FileSystemSnapshotResponce{}) == snapShotResponse {
			apiresp := resp.(client.ApiResponse)
			snapShotResponse, _ = apiresp.Result.(FileSystemSnapshotResponce)
		}
	} else {
		resp, err := c.getJSONResponse(http.MethodPost, path, snapshotParam, &snapShotResponse)
		if err != nil {
			zlog.Error().Msgf("failed to create %v", err)
			return nil, err
		}
		if (FileSystemSnapshotResponce{}) == snapShotResponse {
			apiresp := resp.(client.ApiResponse)
			snapShotResponse, _ = apiresp.Result.(FileSystemSnapshotResponce)
		}
	}
	zlog.Trace().Msgf("Created snapshot: %s", snapShotResponse.Name)
	return &snapShotResponse, nil
}

// FileSystemHasChild method return true is the filesystemID has child else false
func (c *ClientService) FileSystemHasChild(fileSystemID int64) bool {
	hasChild := false
	voluri := "/api/rest/filesystems/"
	filesystem := []FileSystem{}
	queryParam := make(map[string]interface{})
	queryParam["parent_id"] = fileSystemID
	resp, err := c.getResponseWithQueryString(voluri, queryParam, &filesystem)
	if err != nil {
		zlog.Error().Msgf("failed to check FileSystemHasChild %v", err)
		return hasChild
	}
	if len(filesystem) == 0 {
		apiresp := resp.(client.ApiResponse)
		filesystem, _ = apiresp.Result.([]FileSystem)
	}
	if len(filesystem) > 0 {
		hasChild = true
	}
	return hasChild
}

const (
	// TOBEDELETED status
	TOBEDELETED = "host.k8s.to_be_deleted"
)

// GetMetadataStatus :
func (c *ClientService) GetMetadataStatus(fileSystemID int64) bool {
	zlog.Trace().Msgf("Get metadata status of IBox object with ID %d", fileSystemID)
	path := "/api/rest/metadata/" + strconv.FormatInt(fileSystemID, 10) + "/" + TOBEDELETED
	metadata := Metadata{}
	resp, err := c.getJSONResponse(http.MethodGet, path, nil, &metadata)
	if err != nil {
		zlog.Trace().Msgf("Getting metadata did not return a value: %s", err)
		return false
	}

	zlog.Trace().Msgf("GetMetadataStatus for IBox object with ID %d: %v", fileSystemID, resp)

	if metadata == (Metadata{}) {
		apiresp := resp.(client.ApiResponse)
		metadata, _ = apiresp.Result.(Metadata)
	}
	status, statusErr := strconv.ParseBool(metadata.Value)
	if statusErr != nil {
		zlog.Trace().Msgf("Error occured while converting metadata key : %sTOBEDELETED ,value: %s", TOBEDELETED, err)
		status = false
	}
	zlog.Trace().Msgf("Got metadata status of IBox object with ID %d", fileSystemID)
	zlog.Trace().Msgf("Got metadata status of IBox object with ID %d: %t", fileSystemID, status)
	return status
}

// GetFileSystemByName :
func (c *ClientService) GetFileSystemByName(fileSystemName string) (*FileSystem, error) {
	zlog.Trace().Msgf("Get filesystem %s", fileSystemName)
	uri := "/api/rest/filesystems"
	fsystems := []FileSystem{}
	queryParam := make(map[string]interface{})
	queryParam["name"] = fileSystemName
	resp, err := c.getResponseWithQueryString(uri,
		queryParam, &fsystems)
	if err != nil {
		return nil, err
	}
	if len(fsystems) == 0 {
		apiresp := resp.(client.ApiResponse)
		fsystems, _ = apiresp.Result.([]FileSystem)
	}
	for _, fsystem := range fsystems {
		if fsystem.Name == fileSystemName {
			zlog.Trace().Msgf("Got filesystem %s", fileSystemName)
			return &fsystem, nil
		}
	}
	return nil, errors.New("filesystem with given name not found")
}

// GetFileSystemByID :
func (c *ClientService) GetFileSystemByID(fileSystemID int64) (*FileSystem, error) {
	zlog.Trace().Msgf("Get filesystem with ID %d", fileSystemID)
	uri := "/api/rest/filesystems/" + strconv.FormatInt(fileSystemID, 10)
	eResp := FileSystem{}
	_, err := c.getJSONResponse(http.MethodGet, uri, nil, &eResp)
	if err != nil {
		zlog.Error().Msgf("Error occured while getting fileSystem: %s", err)
		return nil, err
	}
	zlog.Trace().Msgf("Got filesystem with ID %d", fileSystemID)
	return &eResp, nil
}

// GetParentID method return the
func (c *ClientService) GetParentID(fileSystemID int64) int64 {
	zlog.Trace().Msgf("Get parent of file system with ID %d", fileSystemID)
	fileSystem, err := c.GetFileSystemByID(fileSystemID)
	if err != nil {
		zlog.Error().Msgf("Error occured while getting file system: %s", err)
		return 0
	}
	zlog.Trace().Msgf("Got parent of file system with ID %d", fileSystemID)
	return fileSystem.ParentID
}

// DeleteParentFileSystem method delete the ascenders of fileystem
func (c *ClientService) DeleteParentFileSystem(fileSystemID int64) (err error) { // delete fileystem's parent ID
	// first check .. hasChild ...
	hasChild := c.FileSystemHasChild(fileSystemID)
	if !hasChild && c.GetMetadataStatus(fileSystemID) { // If No child and to_be_delete_status =true in metadata then
		parentID := c.GetParentID(fileSystemID)        // get the parentID .. before delete
		err = c.DeleteFileSystemComplete(fileSystemID) // delete the filesystem
		if err != nil {
			zlog.Error().Msgf("Failed to delete filesystem with ID %d: %v", fileSystemID, err)
			return
		}
		if parentID != 0 {
			err = c.DeleteParentFileSystem(parentID)
			if err != nil {
				zlog.Error().Msgf("Failed to delete parent filesystem with parent ID %d: %v", fileSystemID, err)
				return
			}
		}
	}
	return
}

// DeleteFileSystemComplete method delete the fileystem
func (c *ClientService) DeleteFileSystemComplete(fileSystemID int64) (err error) {
	// 1. Delete export path
	exportResp, err := c.GetExportByFileSystem(fileSystemID)
	if err != nil {
		if strings.Contains(err.Error(), "EXPORT_NOT_FOUND") {
			err = nil
		} else {
			zlog.Error().Msgf("failed to delete export path %v", err)
			return
		}
	}
	if exportResp != nil {
		for _, ep := range *exportResp {
			_, err = c.DeleteExportPath(ep.ID)
			if err != nil {
				if strings.Contains(err.Error(), "EXPORT_NOT_FOUND") {
					err = nil
				} else {
					zlog.Error().Msgf("failed to delete export path %v", err)
					return
				}
			}
		}
	}

	zlog.Trace().Msgf("Export path deleted successfully")

	// 2.delete metadata
	_, err = c.DetachMetadataFromObject(fileSystemID)
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
	_, err = c.DeleteFileSystem(fileSystemID)
	if err != nil {
		zlog.Error().Msgf("failed to delete filesystem %v", err)
		return
	}
	return
}

// UpdateFilesystem : update file system
func (c *ClientService) UpdateFilesystem(fileSystemID int64, fileSystem FileSystem) (*FileSystem, error) {
	zlog.Trace().Msgf("Update filesystem with ID %d", fileSystemID)
	uri := "api/rest/filesystems/" + strconv.FormatInt(fileSystemID, 10)
	fileSystemResp := FileSystem{}

	resp, err := c.getJSONResponse(http.MethodPut, uri, fileSystem, &fileSystemResp)
	if err != nil {
		zlog.Error().Msgf("Error occured while updating filesystem : %s", err)
		return nil, err
	}

	if fileSystem == (FileSystem{}) {
		apiresp := resp.(client.ApiResponse)
		fileSystemResp, _ = apiresp.Result.(FileSystem)
	}
	zlog.Trace().Msgf("Updated filesystem with ID %d", fileSystemID)
	return &fileSystemResp, nil
}

// RestoreFileSystemFromSnapShot :
func (c *ClientService) RestoreFileSystemFromSnapShot(parentID, srcSnapShotID int64) (bool, error) {
	zlog.Trace().Msgf("Restore filesystem from snapshot with snapshot ID %d", srcSnapShotID)
	uri := "api/rest/filesystems/" + strconv.FormatInt(parentID, 10) + "/restore?approved=true"
	var result bool
	body := map[string]interface{}{"source_id": srcSnapShotID}
	resp, err := c.getJSONResponse(http.MethodPost, uri, body, &result)
	if err != nil {
		zlog.Error().Msgf("Error occured while updating filesystem : %s", err)
		return false, err
	}

	if !result {
		apiresp := resp.(client.ApiResponse)
		result, _ = apiresp.Result.(bool)
	}
	zlog.Trace().Msgf("Restored filesystem from snapshot with snapsshot ID %d", srcSnapShotID)
	return result, nil
}

func removeIndex(s []Permissions, index int) []Permissions {
	return append(s[:index], s[index+1:]...)
}

// GetSnapshotByName :
func (c *ClientService) GetSnapshotByName(snapshotName string) (*[]FileSystemSnapshotResponce, error) {
	zlog.Trace().Msgf("Get snapshot %s", snapshotName)
	uri := "api/rest/filesystems?name=" + snapshotName
	snapshot := []FileSystemSnapshotResponce{}
	resp, err := c.getJSONResponse(http.MethodGet, uri, nil, &snapshot)
	if err != nil {
		zlog.Error().Msgf("Error occured while getting snapshot : %s ", err)
		return nil, err
	}
	if len(snapshot) == 0 {
		zlog.Trace().Msgf("no snapshot found for name %s", snapshotName)
		snapshot, _ = resp.([]FileSystemSnapshotResponce)
	}
	zlog.Trace().Msgf("Got snapshot %s", snapshotName)
	return &snapshot, nil
}

// GetFileSystemCountByPoolID :
func (c *ClientService) GetFileSystemCountByPoolID(poolID int64) (fileSysCnt int, err error) {
	zlog.Trace().Msgf("Get FileSystem Count")
	uri := "api/rest/filesystems?pool_id=" + strconv.FormatInt(poolID, 10)
	filesystems := []FileSystem{}
	resp, err := c.getJSONResponse(http.MethodGet, uri, nil, &filesystems)
	if err != nil {
		zlog.Error().Msgf("error occured while fetching filesystems : %s ", err)
		return
	}
	apiresp := resp.(client.ApiResponse)
	metadata := apiresp.MetaData
	if len(filesystems) == 0 {
		filesystems, _ = apiresp.Result.([]FileSystem)
	}
	zlog.Trace().Msgf("Total number of filesystems: %d", metadata.NoOfObject)
	fileSysCnt = metadata.NoOfObject
	return
}
