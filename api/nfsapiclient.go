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

func (c *ClientService) DeleteExportPath(exportId int64) (*ExportResponse, error) {
	uri := "api/rest/exports/" + strconv.FormatInt(exportId, 10) + "?approved=true"
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

func (c *ClientService) DeleteFileSystem(fileSystemId int64) (*FileSystem, error) {
	uri := "api/rest/filesystems/" + strconv.FormatInt(fileSystemId, 10) + "?approved=true"
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

func (c *ClientService) AttachMetadataToObject(object_id int64, body map[string]interface{}) (*[]Metadata, error) {
	uri := "api/rest/metadata/" + strconv.FormatInt(object_id, 10)
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

func (c *ClientService) DetachMetadataFromObject(object_id int64) (*[]Metadata, error) {
	uri := "api/rest/metadata/" + strconv.FormatInt(object_id, 10) + "?approved=true"
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

func (c *ClientService) CreateFilesystem(fileSystem FileSystem) (*FileSystem, error) {
	uri := "api/rest/filesystems/"
	fileSystemResp := FileSystem{}
	resp, err := c.getJSONResponse(http.MethodPost, uri, fileSystem, &fileSystemResp)
	if err != nil {
		log.Errorf("Error occured while creating filesystem : %s", err)
		return nil, err
	}
	if fileSystem == (FileSystem{}) {
		fileSystem, _ = resp.(FileSystem)
	}
	return &fileSystemResp, nil
}

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

func (c *ClientService) ExportFileSystem(export ExportFileSys) (ExportResponse, error) {
	urlPost := "api/rest/exports"
	exportResp := ExportResponse{}
	resp, err := c.getJSONResponse(http.MethodPost, urlPost, export, exportResp)
	if err != nil {
		return exportResp, err
	}
	if reflect.DeepEqual(exportResp, ExportResponse{}) {
		exportResp, _ = resp.(ExportResponse)
	}
	return exportResp, nil
}

func (c *ClientService) GetExportByID(export_id int) (*ExportResponse, error) {
	uri := "api/rest/exports/" + strconv.Itoa(export_id)
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

func (c *ClientService) GetExportByFileSystem(fileSystemID int64) (*[]ExportResponse, error) {
	uri := "api/rest/exports?filesystem_id" + strconv.FormatInt(fileSystemID, 10)
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

//Export should be updated in case of node addition in k8s cluster
func (c *ClientService) AddNodeInExport(export_id int, access string, noRootSquash bool, ip string) (*ExportResponse, error) {
	flag := false
	exportPathRef := ExportPathRef{}
	uri := "api/rest/exports/" + strconv.Itoa(export_id)
	eResp := ExportResponse{}
	resp, err := c.getJSONResponse(http.MethodGet, uri, nil, &eResp)
	if err != nil {
		log.Errorf("Error occured while getting export path : %s", err)
		return nil, err
	}
	if reflect.DeepEqual(eResp, ExportResponse{}) {
		eResp, _ = resp.(ExportResponse)
	}
	permission_list := eResp.Permissions
	for _, permission := range permission_list {
		if permission.Client == ip {
			flag = true
		}
	}
	if flag == false {
		newPermission := Permissions{
			Access:       access,
			NoRootSquash: noRootSquash,
			Client:       ip,
		}
		permission_list = append(permission_list, newPermission)
		exportPathRef.Permissions = permission_list
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

//Export should be updated in case of node deletion in k8s cluster
func (c *ClientService) DeleteNodeFromExport(export_id int, access string, noRootSquash bool, ip string) (*ExportResponse, error) {
	flag := false
	var index int
	exportPathRef := ExportPathRef{}
	uri := "api/rest/exports/" + strconv.Itoa(export_id)
	eResp := ExportResponse{}
	resp, err := c.getJSONResponse(http.MethodGet, uri, nil, &eResp)
	if err != nil {
		log.Errorf("Error occured while getting export path : %s", err)
		return nil, err
	}
	if reflect.DeepEqual(eResp, ExportResponse{}) {
		eResp, _ = resp.(ExportResponse)
	}
	permission_list := eResp.Permissions
	for i, permission := range permission_list {
		if permission.Client == ip {
			flag = true
			index = i
		}
	}

	if flag == true {
		permission_list = RemoveIndex(permission_list, index)
		exportPathRef.Permissions = permission_list
		resp, err = c.getJSONResponse(http.MethodPut, uri, exportPathRef, &eResp)
		if err != nil {
			log.Errorf("Error occured while updating permission : %s", err)
			return nil, err
		}
		if reflect.DeepEqual(eResp, ExportResponse{}) {
			eResp, _ = resp.(ExportResponse)
		}
	} else {
		fmt.Printf("Given Ip %s address not found in the list", ip)
	}
	return &eResp, nil
}

func RemoveIndex(s []Permissions, index int) []Permissions {
	return append(s[:index], s[index+1:]...)
}
