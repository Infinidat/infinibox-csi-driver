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
	"errors"
	"infinibox-csi-driver/api/client"
	"net/http"
	"strconv"
)

// TREEQCOUNT
const (
	TREEQCOUNT = "host.k8s.treeqs"
)

// FSMetadata struct
type FSMetadata struct {
	FileSystemArry []FileSystem
	Filemetadata   FileSystemMetaData
}

// Treeq struct
type Treeq struct {
	ID           int64  `json:"id,omitempty"`
	FilesystemID int64  `json:"filesystem_id,omitempty"`
	Name         string `json:"name,omitempty"`
	Path         string `json:"path,omitempty"`
	HardCapacity int64  `json:"hard_capacity,omitempty"`
	UsedCapacity int64  `json:"used_capacity,omitempty"`
}

// GetFileSystemsByPoolID get filesystem by poolID
func (c *ClientService) GetFileSystemsByPoolID(poolID int64, page int, fsPrefix string) (fsmetadata *FSMetadata, err error) {
	zlog.Trace().Msgf("GetFileSystemsByPoolID poolID %d page %d fsPrefix %s", poolID, page, fsPrefix)
	uri := "/api/rest/filesystems?pool_id=" + strconv.FormatInt(poolID, 10) +
		"&name=like:" + fsPrefix +
		"&sort=size&page=" + strconv.Itoa(page) + "&page_size=1000&fields=id,size,name"
	filesystems := []FileSystem{}
	resp, err := c.getJSONResponse(http.MethodGet, uri, nil, &filesystems)
	if err != nil {
		zlog.Error().Msgf("error occured while fetching filesystems from pool : %s ", err)
		return
	}
	apiresp := resp.(client.ApiResponse)
	mdata := apiresp.MetaData
	if len(filesystems) == 0 {
		filesystems, _ = apiresp.Result.([]FileSystem)
	}
	fileMetadata := FileSystemMetaData{}
	fileMetadata.NumberOfObjects = mdata.NoOfObject
	fileMetadata.Page = mdata.Page
	fileMetadata.PageSize = mdata.PageSize
	fileMetadata.PagesTotal = mdata.TotalPages

	var fs FSMetadata
	fs.FileSystemArry = filesystems
	fs.Filemetadata = fileMetadata
	fsmetadata = &fs

	return
}

// GetFilesytemTreeqCount method return the treeq count
func (c *ClientService) GetFilesystemTreeqCount(fileSystemID int64) (treeqCnt int, err error) {
	path := "/api/rest/filesystems/" + strconv.FormatInt(fileSystemID, 10) + "/treeqs"
	treeqArry := []Treeq{}
	resp, err := c.getJSONResponse(http.MethodGet, path, nil, &treeqArry)
	if err != nil {
		zlog.Error().Msgf("Error occured while getting treeq count value: %s", err)
		return
	}
	apiresp := resp.(client.ApiResponse)
	mdata := apiresp.MetaData
	treeqCnt = mdata.NoOfObject

	if len(treeqArry) == 0 {
		treeqArry, _ = apiresp.Result.([]Treeq)
	}

	zlog.Trace().Msgf("Total number of Treeq : %d", treeqCnt)
	return
}

// CreateTreeq method create treeq
func (c *ClientService) CreateTreeq(filesystemID int64, treeqParameter map[string]interface{}) (*Treeq, error) {
	zlog.Trace().Msgf("Create filesystem")
	uri := "api/rest/filesystems/" + strconv.FormatInt(filesystemID, 10) + "/treeqs"
	treeq := Treeq{}
	resp, err := c.getJSONResponse(http.MethodPost, uri, treeqParameter, &treeq)
	if err != nil {
		zlog.Error().Msgf("Error occured while creating treeq  : %s", err)
		return nil, err
	}
	if treeq == (Treeq{}) {
		apiresp := resp.(client.ApiResponse)
		treeq, _ = apiresp.Result.(Treeq)
	}

	zlog.Trace().Msgf("treeq created : %s", treeq.Name)
	return &treeq, nil
}

// getTreeqSizeByFileSystemID method return the sum of size
func (c *ClientService) GetTreeqSizeByFileSystemID(filesystemID int64) (int64, error) {
	var size int64
	uri := "api/rest/filesystems/" + strconv.FormatInt(filesystemID, 10) + "/treeqs"
	treeqArray := []Treeq{}
	_, err := c.getJSONResponse(http.MethodGet, uri, nil, &treeqArray)
	if err != nil {
		zlog.Error().Msgf("error occured while fetching treeq list : %s ", err)
		return 0, err
	}
	for _, treeq := range treeqArray {
		size = size + treeq.HardCapacity
	}
	return size, nil
}

// DeleteTreeq :
func (c *ClientService) DeleteTreeq(fileSystemID, treeqID int64) (*Treeq, error) {
	uri := "api/rest/filesystems/" + strconv.FormatInt(fileSystemID, 10) + "/treeqs/" + strconv.FormatInt(treeqID, 10)
	treeq := Treeq{}
	resp, err := c.getJSONResponse(http.MethodDelete, uri, nil, &treeq)
	if err != nil {
		zlog.Error().Msgf("Error occured while deleting treeq : %s ", err)
		return nil, err
	}
	if treeq == (Treeq{}) {
		apiresp := resp.(client.ApiResponse)
		treeq, _ = apiresp.Result.(Treeq)
	}
	zlog.Trace().Msgf("Treeq deleted successfully: %d", fileSystemID)
	return &treeq, nil
}

// GetTreeq
func (c *ClientService) GetTreeq(fileSystemID, treeqID int64) (*Treeq, error) {
	uri := "/api/rest/filesystems/" + strconv.FormatInt(fileSystemID, 10) + "/treeqs/" + strconv.FormatInt(treeqID, 10)
	eResp := Treeq{}
	resp, err := c.getJSONResponse(http.MethodGet, uri, nil, &eResp)
	if err != nil {
		return nil, err
	}
	if eResp == (Treeq{}) {
		apiresp := resp.(client.ApiResponse)
		eResp, _ = apiresp.Result.(Treeq)
	}
	return &eResp, nil
}

// UpdateTreeq :
func (c *ClientService) UpdateTreeq(fileSystemID, treeqID int64, body map[string]interface{}) (*Treeq, error) {
	uri := "api/rest/filesystems/" + strconv.FormatInt(fileSystemID, 10) + "/treeqs/" + strconv.FormatInt(treeqID, 10)
	treeq := Treeq{}
	resp, err := c.getJSONResponse(http.MethodPut, uri, body, &treeq)
	if err != nil {
		zlog.Error().Msgf("Error occured while updating file System : %s ", err)
		return nil, err
	}
	if treeq == (Treeq{}) {
		apiresp := resp.(client.ApiResponse)
		treeq, _ = apiresp.Result.(Treeq)
	}
	zlog.Trace().Msgf("Treeq updated successfully: %d", fileSystemID)
	return &treeq, nil
}

// GetFileSystemByName :
func (c *ClientService) GetTreeqByName(fileSystemID int64, treeqName string) (*Treeq, error) {
	uri := "api/rest/filesystems/" + strconv.FormatInt(fileSystemID, 10) + "/treeqs"
	treeq := []Treeq{}
	queryParam := make(map[string]interface{})
	queryParam["name"] = treeqName
	resp, err := c.getResponseWithQueryString(uri, queryParam, &treeq)
	if err != nil {
		return nil, err
	}
	if len(treeq) == 0 {
		apiresp := resp.(client.ApiResponse)
		treeq, _ = apiresp.Result.([]Treeq)
	}
	for _, fsystem := range treeq {
		if fsystem.Name == treeqName {
			zlog.Trace().Msgf("Got treeq : %s", treeqName)
			return &fsystem, nil
		}
	}
	return nil, errors.New("treeq with given name not found")
}

// GetMaxTreeqPerFs method return the maximum number of treeqs allowed in a file system
func (c *ClientService) GetMaxTreeqPerFs() (int, error) {
	type ParameterResult struct {
		Result struct {
			NasTreeqMaxCountPerFilesystem int `json:"nas.treeq_max_count_per_filesystem"`
		} `json:"result"`
		Error    interface{} `json:"error"`
		Metadata struct {
			Ready bool `json:"ready"`
		} `json:"metadata"`
	}
	uri := "api/rest/config/limits?fields=nas.treeq_max_count_per_filesystem"
	queryResult := ParameterResult{}
	_, err := c.getJSONResponse(http.MethodGet, uri, nil, &queryResult)
	if err != nil {
		zlog.Error().Msgf("error occured while fetching treeq max parameter : %s ", err)
		return 0, err
	}
	return queryResult.Result.NasTreeqMaxCountPerFilesystem, nil
}

// GetMaxFileSystems method return the maximum number of file systems allowed on an ibox
func (c *ClientService) GetMaxFileSystems() (int, error) {
	type ParameterResult struct {
		Result struct {
			NasMaxFilesystemsInSystem int `json:"nas.max_filesystems_in_system"`
		} `json:"result"`
		Error    interface{} `json:"error"`
		Metadata struct {
			Ready bool `json:"ready"`
		} `json:"metadata"`
	}
	uri := "api/rest/config/limits?fields=nas.max_filesystems_in_system"
	queryResult := ParameterResult{}
	_, err := c.getJSONResponse(http.MethodGet, uri, nil, &queryResult)
	if err != nil {
		zlog.Error().Msgf("error occured while fetching max filesystems parameter : %s ", err)
		return 0, err
	}
	return queryResult.Result.NasMaxFilesystemsInSystem, nil
}
