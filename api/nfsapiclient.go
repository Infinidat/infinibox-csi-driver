package api

import (
	"errors"
	"fmt"
	"infinibox-csi-driver/api/client"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"github.com/prometheus/common/log"
)

// OneTimeValidation :
func (c *ClientService) OneTimeValidation(poolname string, networkspace string) (list string, err error) {
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("error while One Time Validation   " + fmt.Sprint(res))

		}
	}()
	//validating pool
	var validList = ""
	_, err = c.GetStoragePoolIDByName(poolname)
	if err != nil {
		return validList, err
	}

	arrayofNetworkSpaces := strings.Split(networkspace, ",")
	var arrayOfValidnetspaces []string

	for _, name := range arrayofNetworkSpaces {
		nspace, err := c.GetNetworkSpaceByName(name)
		if err != nil {
			log.Error(err)
		}
		if nspace.Name == name {
			arrayOfValidnetspaces = append(arrayOfValidnetspaces, name)
		}
	}
	if len(arrayOfValidnetspaces) > 0 {
		validList = strings.Join(arrayOfValidnetspaces, ",")
		return validList, nil
	}

	return validList, errors.New("provide valid network spaces")
}

// DeleteExportPath :
func (c *ClientService) DeleteExportPath(exportID int64) (*ExportResponse, error) {
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("DeleteExportPath Panic occured -  " + fmt.Sprint(res))
		}
	}()
	log.Info("Delete export path : ", exportID)
	uri := "api/rest/exports/" + strconv.FormatInt(exportID, 10) + "?approved=true"
	eResp := ExportResponse{}
	resp, err := c.getJSONResponse(http.MethodDelete, uri, nil, &eResp)
	if err != nil {
		log.Errorf("Error occured while deleting export path : %s ", err)
		return nil, err
	}
	if reflect.DeepEqual(eResp, (ExportResponse{})) {
		apiresp := resp.(client.ApiResponse)
		eResp, _ = apiresp.Result.(ExportResponse)
	}
	log.Info("Deleted export path : ", exportID)
	return &eResp, nil
}

// DeleteFileSystem :
func (c *ClientService) DeleteFileSystem(fileSystemID int64) (*FileSystem, error) {
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("DeleteFileSystem Panic occured -  " + fmt.Sprint(res))
		}
	}()
	log.Info("Delete filesystem : ", fileSystemID)
	uri := "api/rest/filesystems/" + strconv.FormatInt(fileSystemID, 10) + "?approved=true"
	fileSystem := FileSystem{}
	resp, err := c.getJSONResponse(http.MethodDelete, uri, nil, &fileSystem)
	if err != nil {
		log.Errorf("Error occured while deleting file System : %s ", err)
		return nil, err
	}
	if fileSystem == (FileSystem{}) {
		apiresp := resp.(client.ApiResponse)
		fileSystem, _ = apiresp.Result.(FileSystem)
	}
	log.Info("Deleted filesystem : ", fileSystemID)
	return &fileSystem, nil
}

// AttachMetadataToObject :
func (c *ClientService) AttachMetadataToObject(objectID int64, body map[string]interface{}) (*[]Metadata, error) {
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("AttachMetadataToObject Panic occured -  " + fmt.Sprint(res))
		}
	}()
	log.Info("Attach metadata to object : ", objectID)
	uri := "api/rest/metadata/" + strconv.FormatInt(objectID, 10)
	metadata := []Metadata{}
	resp, err := c.getJSONResponse(http.MethodPut, uri, body, &metadata)
	if err != nil {
		log.Errorf("Error occured while attaching metadata to object : %s", err)
		return nil, err
	}
	if len(metadata) == 0 {
		apiresp := resp.(client.ApiResponse)
		metadata, _ = apiresp.Result.([]Metadata)
	}
	log.Info("Attached metadata to object : ", objectID)
	return &metadata, nil
}

// DetachMetadataFromObject :
func (c *ClientService) DetachMetadataFromObject(objectID int64) (*[]Metadata, error) {
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("DetachMetadataFromObject Panic occured -  " + fmt.Sprint(res))
		}
	}()
	log.Info("Detach metadata from object : ", objectID)
	uri := "api/rest/metadata/" + strconv.FormatInt(objectID, 10) + "?approved=true"
	metadata := []Metadata{}
	resp, err := c.getJSONResponse(http.MethodDelete, uri, nil, &metadata)
	if err != nil {
		log.Errorf("Error occured while detaching metadata from object : %s ", err)
		return nil, err
	}
	if len(metadata) == 0 {
		apiresp := resp.(client.ApiResponse)
		metadata, _ = apiresp.Result.([]Metadata)
	}
	log.Info("Detached metadata from object : ", objectID)
	return &metadata, nil
}

// CreateFilesystem :
func (c *ClientService) CreateFilesystem(fileSysparameter map[string]interface{}) (*FileSystem, error) {
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("CreateFilesystem Panic occured -  " + fmt.Sprint(res))
		}
	}()
	log.Info("Create filesystem")
	uri := "api/rest/filesystems/"
	fileSystemResp := FileSystem{}
	resp, err := c.getJSONResponse(http.MethodPost, uri, fileSysparameter, &fileSystemResp)
	if err != nil {
		log.Errorf("Error occured while creating filesystem : %s", err)
		return nil, err
	}
	if fileSystemResp == (FileSystem{}) {
		apiresp := resp.(client.ApiResponse)
		fileSystemResp, _ = apiresp.Result.(FileSystem)
	}
	log.Info("Created filesystem : ", fileSystemResp.Name)
	return &fileSystemResp, nil
}

// GetFileSystemCount :
func (c *ClientService) GetFileSystemCount() (int, error) {
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("GetFileSystemCount Panic occured -  " + fmt.Sprint(res))
		}
	}()
	log.Info("Get FileSystem Count")
	uri := "api/rest/filesystems"
	filesystems := []FileSystem{}
	resp, err := c.getJSONResponse(http.MethodGet, uri, nil, &filesystems)
	if err != nil {
		log.Errorf("error occured while fetching filesystems : %s ", err)
		return 0, err
	}

	apiresp := resp.(client.ApiResponse)
	metadata := apiresp.MetaData

	if len(filesystems) == 0 {
		filesystems, _ = apiresp.Result.([]FileSystem)
	}
	// return len(filesystems), nil
	log.Info("Total number of filesystem : ", metadata.NoOfObject)
	return metadata.NoOfObject, nil
}

// ExportFileSystem :
func (c *ClientService) ExportFileSystem(export ExportFileSys) (*ExportResponse, error) {
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("ExportFileSystem Panic occured -  " + fmt.Sprint(res))
		}
	}()
	log.Info("Export FileSystem : ", export.FilesystemID)
	urlPost := "api/rest/exports"
	exportResp := ExportResponse{}
	resp, err := c.getJSONResponse(http.MethodPost, urlPost, export, &exportResp)
	if err != nil {
		return nil, err
	}

	if reflect.DeepEqual(exportResp, ExportResponse{}) {
		apiresp := resp.(client.ApiResponse)
		exportResp, _ = apiresp.Result.(ExportResponse)
	}
	log.Info("Exported FileSystem : ", exportResp.FilesystemId)
	return &exportResp, nil
}

// GetExportByFileSystem :
func (c *ClientService) GetExportByFileSystem(fileSystemID int64) (*[]ExportResponse, error) {
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("GetExportByFileSystem Panic occured -  " + fmt.Sprint(res))
		}
	}()
	log.Info("Get export paths of filesystem : ", fileSystemID)
	uri := "api/rest/exports?filesystem_id=" + strconv.FormatInt(fileSystemID, 10)
	eResp := []ExportResponse{}
	resp, err := c.getJSONResponse(http.MethodGet, uri, nil, &eResp)
	if err != nil {
		log.Errorf("Error occured while getting export path : %s", err)
		return nil, err
	}
	if len(eResp) == 0 {
		apiresp := resp.(client.ApiResponse)
		eResp, _ = apiresp.Result.([]ExportResponse)
	}
	log.Info("Got export paths of filesystem : ", fileSystemID)
	return &eResp, nil
}

//AddNodeInExport : Export should be updated in case of node addition in k8s cluster
func (c *ClientService) AddNodeInExport(exportID int, access string, noRootSquash bool, ip string) (*ExportResponse, error) {
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("AddNodeInExport Panic occured -  " + fmt.Sprint(res))
		}
	}()
	log.Info("Add node in export path : ", exportID)
	flag := false
	exportPathRef := ExportPathRef{}
	uri := "api/rest/exports/" + strconv.Itoa(exportID)
	eResp := ExportResponse{}
	resp, err := c.getJSONResponse(http.MethodGet, uri, nil, &eResp)
	if err != nil {
		log.Errorf("Error occured while getting export path : %s", err)
		return nil, err
	}
	if reflect.DeepEqual(eResp, ExportResponse{}) {
		apiresp := resp.(client.ApiResponse)
		eResp, _ = apiresp.Result.(ExportResponse)
	}
	index := -1
	permissionList := eResp.Permissions
	for i, permission := range permissionList {
		if permission.Client == ip {
			flag = true
		}
		if permission.Client == "*" {
			index = i
		}
	}
	if index != -1 {
		permissionList = removeIndex(permissionList, index)
	}
	if flag == false {
		newPermission := Permissions{
			Access:       access,
			NoRootSquash: noRootSquash,
			Client:       ip,
		}
		permissionList = append(permissionList, newPermission)
		exportPathRef.Permissions = permissionList
		resp, err = c.getJSONResponse(http.MethodPut, uri, exportPathRef, &eResp)
		if err != nil {
			log.Errorf("Error occured while getting export path : %s", err)
			return nil, err
		}
		if reflect.DeepEqual(eResp, ExportResponse{}) {
			eResp, _ = resp.(ExportResponse)
		}
	}
	log.Info("Added node in export path : ", exportID)
	return &eResp, nil
}

//DeleteExportRule method
func (c *ClientService) DeleteExportRule(fileSystemID int64, ipAddress string) error {
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("DeleteExportRule Panic occured -  " + fmt.Sprint(res))
		}
	}()
	log.Info("Delete export rule from filesystem : ", fileSystemID)
	exportArray, err := c.GetExportByFileSystem(fileSystemID)
	if err != nil {
		log.Errorf("Error occured while getting export : %v", err)
		return err
	}
	for _, export := range *exportArray {
		uri := "api/rest/exports/" + strconv.FormatInt(export.ID, 10)
		eResp := ExportResponse{}
		resp, err := c.getJSONResponse(http.MethodGet, uri, nil, &eResp)
		if err != nil {
			log.Errorf("Error occured while getting export path : %s", err)
			return err
		}
		if reflect.DeepEqual(eResp, ExportResponse{}) {
			apiresp := resp.(client.ApiResponse)
			eResp, _ = apiresp.Result.(ExportResponse)
		}
		permissionList := eResp.Permissions
		for _, permission := range permissionList {
			if permission.Client == ipAddress {
				_, err = c.DeleteNodeFromExport(export.ID, permission.Access, permission.NoRootSquash, ipAddress)
				if err != nil {
					log.Errorf("Error occured while getting export path : %s", err)
					return err
				}
			}
		}
	}
	log.Info("Deleted export rule from filesystem : ", fileSystemID)
	return nil
}

// DeleteNodeFromExport Export should be updated in case of node deletion in k8s cluster
func (c *ClientService) DeleteNodeFromExport(exportID int64, access string, noRootSquash bool, ip string) (*ExportResponse, error) {
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("DeleteNodeFromExport Panic occured -  " + fmt.Sprint(res))
		}
	}()
	log.Info("Delete node from export : ", exportID)
	flag := false
	var index int
	exportPathRef := ExportPathRef{}
	uri := "api/rest/exports/" + strconv.FormatInt(exportID, 10)
	eResp := ExportResponse{}
	resp, err := c.getJSONResponse(http.MethodGet, uri, nil, &eResp)
	if err != nil {
		log.Errorf("Error occured while getting export path : %s", err)
		return nil, err
	}
	if reflect.DeepEqual(eResp, ExportResponse{}) {
		apiresp := resp.(client.ApiResponse)
		eResp, _ = apiresp.Result.(ExportResponse)
	}
	permissionList := eResp.Permissions
	for i, permission := range permissionList {
		if permission.Client == ip {
			flag = true
			index = i
		}
	}

	if flag == true {
		permissionList = removeIndex(permissionList, index)
		if len(permissionList) == 0 {
			defaultPermission := Permissions{}
			defaultPermission.Access = "RW"
			defaultPermission.Client = "*"
			defaultPermission.NoRootSquash = true
			permissionList = append(permissionList, defaultPermission)
		}
		exportPathRef.Permissions = permissionList
		resp, err = c.getJSONResponse(http.MethodPut, uri, exportPathRef, &eResp)
		if err != nil {
			log.Errorf("Error occured while updating permission : %s", err)
			return nil, err
		}
		if reflect.DeepEqual(eResp, ExportResponse{}) {
			eResp, _ = resp.(ExportResponse)
		}
	} else {
		log.Errorf("Given Ip %s address not found in the list", ip)
	}
	log.Info("Deleted node from export : ", exportID)
	return &eResp, nil
}

//CreateFileSystemSnapshot method create the filesystem snapshot
func (c *ClientService) CreateFileSystemSnapshot(snapshotParam *FileSystemSnapshot) (*FileSystemSnapshotResponce, error) {
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("CreateFileSystemSnapshot Panic occured -  " + fmt.Sprint(res))
		}
	}()
	log.Info("Create a snapshot of filesystem : ", snapshotParam.ParentID)
	path := "/api/rest/filesystems"
	snapShotResponse := FileSystemSnapshotResponce{}
	resp, err := c.getJSONResponse(http.MethodPost, path, snapshotParam, &snapShotResponse)
	if err != nil {
		log.Errorf("fail to create %v", err)
		return nil, err
	}
	if (FileSystemSnapshotResponce{}) == snapShotResponse {
		apiresp := resp.(client.ApiResponse)
		snapShotResponse, _ = apiresp.Result.(FileSystemSnapshotResponce)
	}
	log.Info("Created snapshot : ", snapShotResponse.Name)
	return &snapShotResponse, nil
}

//FileSystemHasChild method return true is the filesystemID has child else false
func (c *ClientService) FileSystemHasChild(fileSystemID int64) bool {
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("FileSystemHasChild Panic occured -  " + fmt.Sprint(res))
		}
	}()
	hasChild := false
	voluri := "/api/rest/filesystems/"
	filesystem := []FileSystem{}
	queryParam := make(map[string]interface{})
	queryParam["parent_id"] = fileSystemID
	resp, err := c.getResponseWithQueryString(voluri, queryParam, &filesystem)
	if err != nil {
		log.Errorf("fail to check FileSystemHasChild %v", err)
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

//
const (
	//TOBEDELETED status
	TOBEDELETED = "host.k8s.to_be_deleted"
)

//GetMetadataStatus :
func (c *ClientService) GetMetadataStatus(fileSystemID int64) bool {
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("GetMetadataStatus Panic occured -  " + fmt.Sprint(res))
		}
	}()
	log.Info("Get metadata status of filesystem : ", fileSystemID)
	path := "/api/rest/metadata/" + strconv.FormatInt(fileSystemID, 10) + "/" + TOBEDELETED
	metadata := Metadata{}
	resp, err := c.getJSONResponse(http.MethodGet, path, nil, &metadata)
	if err != nil {
		log.Debugf("Error occured while getting metadata value: %s", err)
		return false
	}
	if metadata == (Metadata{}) {
		apiresp := resp.(client.ApiResponse)
		metadata, _ = apiresp.Result.(Metadata)
	}
	status, statusErr := strconv.ParseBool(metadata.Value)
	if statusErr != nil {
		log.Debugf("Error occured while converting metadata key : %sTOBEDELETED ,value: %s", TOBEDELETED, err)
		status = false
	}
	log.Info("Got metadata status of filesystem : ", fileSystemID)
	return status

}

//GetFileSystemByName :
func (c *ClientService) GetFileSystemByName(fileSystemName string) (*FileSystem, error) {
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("GetFileSystemByName Panic occured -  " + fmt.Sprint(res))
		}
	}()
	log.Info("Get filesystem : ", fileSystemName)
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
			log.Info("Got filesystem : ", fileSystemName)
			return &fsystem, nil
		}
	}
	return nil, errors.New("filesystem with given name not found")
}

// GetFileSystemByID :
func (c *ClientService) GetFileSystemByID(fileSystemID int64) (*FileSystem, error) {
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("GetFileSystemByID Panic occured -  " + fmt.Sprint(res))
		}
	}()
	log.Info("Get filesystem of Id : ", fileSystemID)
	uri := "/api/rest/filesystems/" + strconv.FormatInt(fileSystemID, 10)
	eResp := FileSystem{}
	resp, err := c.getJSONResponse(http.MethodGet, uri, nil, &eResp)
	if err != nil {
		log.Errorf("Error occured while getting fileSystem: %s", err)
		return nil, err
	}
	if reflect.DeepEqual(eResp, FileSystem{}) {
		apiresp := resp.(client.ApiResponse)
		eResp, _ = apiresp.Result.(FileSystem)
	}
	log.Info("Got filesystem of Id : ", fileSystemID)
	return &eResp, nil
}

//GetParentID method return the
func (c *ClientService) GetParentID(fileSystemID int64) int64 {
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("GetParentID Panic occured -  " + fmt.Sprint(res))
		}
	}()
	log.Info("Get parent Id of : ", fileSystemID)
	fileSystem, err := c.GetFileSystemByID(fileSystemID)
	if err != nil {
		log.Errorf("Error occured while getting fileSystem: %s", err)
		return 0
	}
	log.Info("Got parent Id of : ", fileSystemID)
	return fileSystem.ParentID
}

//DeleteParentFileSystem method delete the ascenders of fileystem
func (c *ClientService) DeleteParentFileSystem(fileSystemID int64) (err error) { //delete fileystem's parent ID
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("DeleteParentFileSystem Panic occured -  " + fmt.Sprint(res))
		}
	}()
	//first check .. hasChild ...
	hasChild := c.FileSystemHasChild(fileSystemID)
	if !hasChild && c.GetMetadataStatus(fileSystemID) { //If No child and to_be_delete_status =true in metadata then
		parentID := c.GetParentID(fileSystemID)        // get the parentID .. before delete
		err = c.DeleteFileSystemComplete(fileSystemID) //delete the filesystem
		if err != nil {
			log.Errorf("Failed to delete filesystem, filesystemID:%d error:%v", fileSystemID, err)
			return
		}
		if parentID != 0 {
			c.DeleteParentFileSystem(parentID)
		}
	}
	return
}

//DeleteFileSystemComplete method delete the fileystem
func (c *ClientService) DeleteFileSystemComplete(fileSystemID int64) (err error) {

	defer func() {
		if res := recover(); res != nil {
			err = errors.New("DeleteFileSystemComplete panic error " + fmt.Sprint(res))
		}
	}()

	//1. Delete export path
	exportResp, err := c.GetExportByFileSystem(fileSystemID)
	if err != nil {
		if strings.Contains(err.Error(), "EXPORT_NOT_FOUND") {
			err = nil
		} else {
			log.Errorf("fail to delete export path %v", err)
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
					log.Errorf("fail to delete export path %v", err)
					return
				}
			}
		}
	}

	log.Debug("Export path deleted successfully")

	//2.delete metadata
	_, err = c.DetachMetadataFromObject(fileSystemID)
	if err != nil {
		if strings.Contains(err.Error(), "METADATA_IS_NOT_SUPPORTED_FOR_ENTITY") {
			err = nil
		} else {
			log.Errorf("fail to delete metadata %v", err)
			return
		}
	}

	//3. delete file system
	log.Infof("delete FileSystem FileSystemID %d", fileSystemID)
	_, err = c.DeleteFileSystem(fileSystemID)
	if err != nil {
		log.Errorf("fail to delete filesystem %v", err)
		return
	}
	return
}

//UpdateFilesystem : update file system
func (c *ClientService) UpdateFilesystem(fileSystemID int64, fileSystem FileSystem) (*FileSystem, error) {
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("UpdateFilesystem Panic occured -  " + fmt.Sprint(res))
		}
	}()
	log.Info("Update filesystem : ", fileSystemID)
	uri := "api/rest/filesystems/" + strconv.FormatInt(fileSystemID, 10)
	fileSystemResp := FileSystem{}

	resp, err := c.getJSONResponse(http.MethodPut, uri, fileSystem, &fileSystemResp)
	if err != nil {
		log.Errorf("Error occured while updating filesystem : %s", err)
		return nil, err
	}

	if fileSystem == (FileSystem{}) {
		apiresp := resp.(client.ApiResponse)
		fileSystem, _ = apiresp.Result.(FileSystem)
	}
	log.Info("Updated filesystem : ", fileSystemID)
	return &fileSystemResp, nil
}

// RestoreFileSystemFromSnapShot :
func (c *ClientService) RestoreFileSystemFromSnapShot(parentID, srcSnapShotID int64) (bool, error) {
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("RestoreFileSystemFromSnapShot Panic occured -  " + fmt.Sprint(res))
		}
	}()
	log.Info("Restore filesystem from snapshot : ", srcSnapShotID)
	uri := "api/rest/filesystems/" + strconv.FormatInt(parentID, 10) + "/restore?approved=true"
	var result bool
	body := map[string]interface{}{"source_id": srcSnapShotID}
	resp, err := c.getJSONResponse(http.MethodPost, uri, body, &result)
	if err != nil {
		log.Errorf("Error occured while updating filesystem : %s", err)
		return false, err
	}

	if result == false {
		apiresp := resp.(client.ApiResponse)
		result, _ = apiresp.Result.(bool)
	}
	log.Info("Restored filesystem from snapshot : ", srcSnapShotID)
	return result, nil
}

func removeIndex(s []Permissions, index int) []Permissions {
	return append(s[:index], s[index+1:]...)
}

//GetSnapshotByName :
func (c *ClientService) GetSnapshotByName(snapshotName string) (*[]FileSystemSnapshotResponce, error) {
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("UpdateFilesystem Panic occured -  " + fmt.Sprint(res))
		}
	}()
	log.Info("Get snapshot : ", snapshotName)
	uri := "api/rest/filesystems?name=" + snapshotName
	snapshot := []FileSystemSnapshotResponce{}
	resp, err := c.getJSONResponse(http.MethodGet, uri, nil, &snapshot)
	if err != nil {
		log.Errorf("Error occured while getting snapshot : %s ", err)
		return nil, err
	}
	if len(snapshot) == 0 {
		snapshot, _ = resp.([]FileSystemSnapshotResponce)
	}
	log.Info("Got snapshot : ", snapshotName)
	return &snapshot, nil
}
