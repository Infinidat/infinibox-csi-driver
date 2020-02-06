package api

import (
	"errors"
	"fmt"
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

//CreateExportPath :
func (c *ClientService) CreateExportPath(exportRef *ExportPathRef) (*ExportResponse, error) {
	uri := "api/rest/exports/"
	eResp := ExportResponse{}
	resp, err := c.getJSONResponse(http.MethodPost, uri, exportRef, &eResp)
	if err != nil {
		log.Errorf("Error occured while creating export path : %s", err)
		return nil, err
	}
	if reflect.DeepEqual(eResp, (ExportResponse{})) {
		eResp, _ = resp.(ExportResponse)
	}
	return &eResp, nil
}

// DeleteExportPath :
func (c *ClientService) DeleteExportPath(exportID int64) (*ExportResponse, error) {
	uri := "api/rest/exports/" + strconv.FormatInt(exportID, 10) + "?approved=true"
	eResp := ExportResponse{}
	resp, err := c.getJSONResponse(http.MethodDelete, uri, nil, &eResp)
	if err != nil {
		log.Errorf("Error occured while deleting export path : %s ", err)
		return nil, err
	}
	if reflect.DeepEqual(eResp, (ExportResponse{})) {
		eResp, _ = resp.(ExportResponse)
	}
	return &eResp, nil
}

// DeleteFileSystem :
func (c *ClientService) DeleteFileSystem(fileSystemID int64) (*FileSystem, error) {
	uri := "api/rest/filesystems/" + strconv.FormatInt(fileSystemID, 10) + "?approved=true"
	fileSystem := FileSystem{}
	resp, err := c.getJSONResponse(http.MethodDelete, uri, nil, &fileSystem)
	if err != nil {
		log.Errorf("Error occured while deleting file System : %s ", err)
		return nil, err
	}
	if fileSystem == (FileSystem{}) {
		fileSystem, _ = resp.(FileSystem)
	}
	return &fileSystem, nil
}

// AttachMetadataToObject :
func (c *ClientService) AttachMetadataToObject(objectID int64, body map[string]interface{}) (*[]Metadata, error) {
	uri := "api/rest/metadata/" + strconv.FormatInt(objectID, 10)
	metadata := []Metadata{}
	resp, err := c.getJSONResponse(http.MethodPut, uri, body, &metadata)
	if err != nil {
		log.Errorf("Error occured while attaching metadata to object : %s", err)
		return nil, err
	}
	if len(metadata) == 0 {
		metadata, _ = resp.([]Metadata)
	}
	return &metadata, nil
}

// DetachMetadataFromObject :
func (c *ClientService) DetachMetadataFromObject(objectID int64) (*[]Metadata, error) {
	uri := "api/rest/metadata/" + strconv.FormatInt(objectID, 10) + "?approved=true"
	metadata := []Metadata{}
	resp, err := c.getJSONResponse(http.MethodDelete, uri, nil, &metadata)
	if err != nil {
		log.Errorf("Error occured while detaching metadata from object : %s ", err)
		return nil, err
	}
	if len(metadata) == 0 {
		metadata, _ = resp.([]Metadata)
	}
	return &metadata, nil
}

// CreateFilesystem :
func (c *ClientService) CreateFilesystem(fileSysparameter map[string]interface{}) (*FileSystem, error) {
	uri := "api/rest/filesystems/"
	fileSystemResp := FileSystem{}
	_, err := c.getJSONResponse(http.MethodPost, uri, fileSysparameter, &fileSystemResp)
	if err != nil {
		log.Errorf("Error occured while creating filesystem : %s", err)
		return nil, err
	}
	/*if resp != nil {
		fileSystem, _ := resp.(FileSystem)
	}*/
	return &fileSystemResp, nil
}

// GetFileSystemCount :
func (c *ClientService) GetFileSystemCount() (int, error) {
	uri := "api/rest/filesystems"
	filesystems := []FileSystem{}
	resp, err := c.getJSONResponse(http.MethodGet, uri, nil, &filesystems)
	if err != nil {
		log.Errorf("error occured while fetching filesystems : %s ", err)
		return 0, err
	}
	if len(filesystems) == 0 {
		filesystems, _ = resp.([]FileSystem)
	}
	return len(filesystems), nil
}

// ExportFileSystem :
func (c *ClientService) ExportFileSystem(export ExportFileSys) (*ExportResponse, error) {
	urlPost := "api/rest/exports"
	exportResp := ExportResponse{}
	resp, err := c.getJSONResponse(http.MethodPost, urlPost, export, &exportResp)
	if err != nil {
		return nil, err
	}
	if reflect.DeepEqual(exportResp, ExportResponse{}) {
		exportResp, _ = resp.(ExportResponse)
	}
	return &exportResp, nil
}

// GetExportByID :
func (c *ClientService) GetExportByID(exportID int) (*ExportResponse, error) {
	uri := "api/rest/exports/" + strconv.Itoa(exportID)
	eResp := ExportResponse{}
	resp, err := c.getJSONResponse(http.MethodGet, uri, nil, &eResp)
	if err != nil {
		log.Errorf("Error occured while getting export path : %s", err)
		return nil, err
	}
	if reflect.DeepEqual(eResp, ExportResponse{}) {
		eResp, _ = resp.(ExportResponse)
	}
	return &eResp, nil
}

// GetExportByFileSystem :
func (c *ClientService) GetExportByFileSystem(fileSystemID int64) (*[]ExportResponse, error) {
	uri := "api/rest/exports?filesystem_id=" + strconv.FormatInt(fileSystemID, 10)
	eResp := []ExportResponse{}
	resp, err := c.getJSONResponse(http.MethodGet, uri, nil, &eResp)
	if err != nil {
		log.Errorf("Error occured while getting export path : %s", err)
		return nil, err
	}
	if len(eResp) == 0 {
		eResp, _ = resp.([]ExportResponse)
	}
	return &eResp, nil
}

//AddNodeInExport : Export should be updated in case of node addition in k8s cluster
func (c *ClientService) AddNodeInExport(exportID int, access string, noRootSquash bool, ip string) (*ExportResponse, error) {
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
		eResp, _ = resp.(ExportResponse)
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
		//permissionList = removeIndex(permissionList, index)
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
	return &eResp, nil
}

//DeleteExportRule method
func (c *ClientService) DeleteExportRule(fileSystemID int64, ipAddress string) error {
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
			eResp, _ = resp.(ExportResponse)
		}
		permission_list := eResp.Permissions
		for _, permission := range permission_list {
			if permission.Client == ipAddress {
				_, err = c.DeleteNodeFromExport(export.ID, permission.Access, permission.NoRootSquash, ipAddress)
				if err != nil {
					log.Errorf("Error occured while getting export path : %s", err)
					return err
				}
			}
		}
	}
	return nil
}

// DeleteNodeFromExport Export should be updated in case of node deletion in k8s cluster
func (c *ClientService) DeleteNodeFromExport(exportID int64, access string, noRootSquash bool, ip string) (*ExportResponse, error) {
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
		eResp, _ = resp.(ExportResponse)
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
	return &eResp, nil
}

func removeIndex(s []Permissions, index int) []Permissions {
	return append(s[:index], s[index+1:]...)
}

//FileSystemSnapshot file system snapshot request parameter
type FileSystemSnapshot struct {
	ParentID       int64  `json:"parent_id"`
	SnpashotName   string `json:"name"`
	WriteProtected bool   `json:"write_protected"`
}

//FileSystemSnapshotResponce file system snapshot Response
type FileSystemSnapshotResponce struct {
	SnapshotID  int64  `json:"id"`
	Name        string `json:"name,omitempty"`
	DatasetType string `json:"dataset_type,omitempty"`
}

//CreateFileSystemSnapshot method create the filesystem snapshot
func (c *ClientService) CreateFileSystemSnapshot(sourceFileSystemID int64, snapshotName string) (*FileSystemSnapshotResponce, error) {
	path := "/api/rest/filesystems"
	fileSysSnap := FileSystemSnapshot{}
	fileSysSnap.ParentID = sourceFileSystemID
	fileSysSnap.SnpashotName = snapshotName
	fileSysSnap.WriteProtected = true
	log.Error("fileSysSnap", fileSysSnap)
	snapShotResponce := FileSystemSnapshotResponce{}
	resp, err := c.getJSONResponse(http.MethodPost, path, fileSysSnap, &snapShotResponce)
	if err != nil {
		log.Errorf("fail to create %v", err)
		return nil, err
	}
	if (FileSystemSnapshotResponce{}) == snapShotResponce {
		snapShotResponce, _ = resp.(FileSystemSnapshotResponce)
	}
	log.Errorf("CreateFileSystemSnapshot post api response %v", snapShotResponce)
	return &snapShotResponce, nil
}

//FileSystemHasChild method return true is the filesystemID has child else false
func (c *ClientService) FileSystemHasChild(fileSystemID int64) bool {
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
		filesystem, _ = resp.([]FileSystem)
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

func (c *ClientService) GetMetadataStatus(fileSystemID int64) bool {
	path := "/api/rest/metadata/" + strconv.FormatInt(fileSystemID, 10) + "/" + TOBEDELETED
	metadata := Metadata{}
	resp, err := c.getJSONResponse(http.MethodGet, path, nil, &metadata)
	if err != nil {
		log.Debugf("Error occured while getting metadata value: %s", err)
		return false
	}
	if metadata == (Metadata{}) {
		metadata, _ = resp.(Metadata)
	}
	status, statusErr := strconv.ParseBool(metadata.Value)
	if statusErr != nil {
		log.Debugf("Error occured while converting metadata key : %sTOBEDELETED ,value: %s", TOBEDELETED, err)
		status = false
	}
	return status

}

func (c *ClientService) GetFileSystemByID(fileSystemID int64) (*FileSystem, error) {
	uri := "/api/rest/filesystems/" + strconv.FormatInt(fileSystemID, 10)
	eResp := FileSystem{}
	resp, err := c.getJSONResponse(http.MethodGet, uri, nil, &eResp)
	if err != nil {
		log.Errorf("Error occured while getting fileSystem: %s", err)
		return nil, err
	}
	if reflect.DeepEqual(eResp, FileSystem{}) {
		eResp, _ = resp.(FileSystem)
	}
	return &eResp, nil
}

func (c *ClientService) GetParentID(fileSystemID int64) int64 {
	fileSystem, err := c.GetFileSystemByID(fileSystemID)
	if err != nil {
		log.Errorf("Error occured while getting fileSystem: %s", err)
		return 0
	}
	return fileSystem.ParentID
}

//DeleteParentFileSystem method delete the ascenders of fileystem
func (c *ClientService) DeleteParentFileSystem(fileSystemID int64) (err error) { //delete fileystem's parent ID
	//first check .. hasChild ...
	hasChild := c.FileSystemHasChild(fileSystemID)
	if !hasChild && c.GetMetadataStatus(fileSystemID) { //If No child and to_be_delete_status =true in metadata then
		parentID := c.GetParentID(fileSystemID)        // get the parentID .. before delete
		err = c.DeleteFileSystemComplete(fileSystemID) //delete the filesystem
		if err != nil {
			log.Errorf("fail to delete filesystem,filesystemID:%d error:%v", fileSystemID, err)
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
			err = errors.New("error while deleting filesystem " + fmt.Sprint(res))
		}
	}()

	//1. Delete export path
	exportResp, err := c.GetExportByFileSystem(fileSystemID)
	if err != nil {
		log.Errorf("fail to delete export path %v", err)
		return
	}
	for _, ep := range *exportResp {
		_, err = c.DeleteExportPath(ep.ID)
		if err != nil {
			log.Errorf("fail to delete export path %v", err)
			return
		}
	}
	log.Debug("Export path deleted successfully")

	//2.delete metadata
	_, err = c.DetachMetadataFromObject(fileSystemID)
	if err != nil {
		log.Errorf("fail to delete metadata %v", err)
		return
	}

	//3. delete file system
	log.Infof("delete FileSystem FileSystemID %v", fileSystemID)
	_, err = c.DeleteFileSystem(fileSystemID)
	if err != nil {
		log.Errorf("fail to delete filesystem %v", err)
		return
	}
	return
}

func (c *ClientService) UpdateFilesystem(fileSystemID int64, fileSystem FileSystem) (*FileSystem, error) {
	uri := "api/rest/filesystems/" + strconv.FormatInt(fileSystemID, 10)
	fileSystemResp := FileSystem{}

	resp, err := c.getJSONResponse(http.MethodPut, uri, fileSystem, &fileSystemResp)
	if err != nil {
		log.Errorf("Error occured while updating filesystem : %s", err)
		return nil, err
	}

	if fileSystem == (FileSystem{}) {
		fileSystem, _ = resp.(FileSystem)
	}
	return &fileSystemResp, nil
}
