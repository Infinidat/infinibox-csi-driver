package api

import (
	"errors"
	"fmt"
	"infinibox-csi-driver/api/client"
	"net/http"
	"strconv"

	log "github.com/sirupsen/logrus"
)

//TREEQCOUNT
const (
	TREEQCOUNT = "host.k8s.treeqs"
)

//FSMetadata struct
type FSMetadata struct {
	FileSystemArry []FileSystem
	Filemetadata   FileSystemMetaData
}

//Treeq struct
type Treeq struct {
	ID           int64  `json:"id,omitempty"`
	FilesystemID int64  `json:"filesystem_id,omitempty"`
	Name         string `json:"name,omitempty"`
	Path         string `json:"path,omitempty"`
	HardCapacity int64  `json:"hard_capacity,omitempty"`
	UsedCapacity int64  `json:"used_capacity,omitempty"`
}

//GetFileSystemsByPoolID get filesystem by poolID
func (c *ClientService) GetFileSystemsByPoolID(poolID int64, page int) (fsmetadata *FSMetadata, err error) {
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("GetFileSystemsByPoolID Panic occured -  " + fmt.Sprint(res))
		}
	}()
	uri := "/api/rest/filesystems?pool_id=" + strconv.FormatInt(poolID, 10) + "&sort=id&page=" + strconv.Itoa(page) + "&fields=id,size,name"
	filesystems := []FileSystem{}
	resp, err := c.getJSONResponse(http.MethodGet, uri, nil, &filesystems)
	if err != nil {
		log.Errorf("error occured while fetching filesystems from pool : %s ", err)
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

//GetFilesytemTreeqCount method return the treeq count
func (c *ClientService) GetFilesytemTreeqCount(fileSystemID int64) (treeqCnt int, err error) {
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("GetFilesytemTreeqCount Panic occured -  " + fmt.Sprint(res))
		}
	}()
	path := "/api/rest/metadata/" + strconv.FormatInt(fileSystemID, 10) + "/" + TREEQCOUNT
	metadata := Metadata{}
	resp, err := c.getJSONResponse(http.MethodGet, path, nil, &metadata)
	if err != nil {
		log.Debugf("Error occured while getting host.k8s.treeqs value: %s", err)
		return 0, err
	}
	if metadata == (Metadata{}) {
		apiresp := resp.(client.ApiResponse)
		metadata = apiresp.Result.(Metadata)
	}
	treeqCnt, err = strconv.Atoi(metadata.Value)
	if err != nil {
		log.Debugf("Error occured while converting metadata key : %s ,value: %v", TREEQCOUNT, err)
		return
	}
	log.Info("Got metadata status of filesystem : ", fileSystemID)
	return

}

//CreateTreeq method create treeq
func (c *ClientService) CreateTreeq(filesystemID int64, treeqParameter map[string]interface{}) (*Treeq, error) {
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("Create Treeq Panic occured -  " + fmt.Sprint(res))
		}
	}()
	log.Info("Create filesystem")
	uri := "api/rest/filesystems/" + strconv.FormatInt(filesystemID, 10) + "/treeqs"
	treeq := Treeq{}
	resp, err := c.getJSONResponse(http.MethodPost, uri, treeqParameter, &treeq)
	if err != nil {
		log.Errorf("Error occured while creating treeq  : %s", err)
		return nil, err
	}
	if treeq == (Treeq{}) {
		apiresp := resp.(client.ApiResponse)
		treeq, _ = apiresp.Result.(Treeq)
	}
	log.Info("treeq created : ", treeq.Name)
	return &treeq, nil
}

//getTreeqSizeByFileSystemID method return the sum of size
func (c *ClientService) getTreeqSizeByFileSystemID(filesystemID int64) int64 {
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("get treeq size Panic occured -  " + fmt.Sprint(res))
		}
	}()
	uri := "api/rest/filesystems/" + strconv.FormatInt(filesystemID, 10) + "/treeqs"
	treeqArry := []Treeq{}
	_, err = c.getJSONResponse(http.MethodGet, uri, nil, &treeqArry)
	if err != nil {
		log.Errorf("error occured while fetching treeq list : %s ", err)

	}
	return 0
}

// DeleteTreeq :
func (c *ClientService) DeleteTreeq(fileSystemID, treeqID int64) (*Treeq, error) {
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("Delete Treeq Panic occured -  " + fmt.Sprint(res))
		}
	}()
	uri := "api/rest/filesystems/" + strconv.FormatInt(fileSystemID, 10) + "/treeqs/" + strconv.FormatInt(treeqID, 10)
	treeq := Treeq{}
	resp, err := c.getJSONResponse(http.MethodDelete, uri, nil, &treeq)
	if err != nil {
		log.Errorf("Error occured while deleting treeq : %s ", err)
		return nil, err
	}
	if treeq == (Treeq{}) {
		apiresp := resp.(client.ApiResponse)
		treeq, _ = apiresp.Result.(Treeq)
	}
	log.Info("Treeq deleted successfully: ", fileSystemID)
	return &treeq, nil
}

//GetTreeq
func (c *ClientService) GetTreeq(fileSystemID, treeqID int64) (*Treeq, error) {
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("Get Treeq Panic occured -  " + fmt.Sprint(res))
		}
	}()
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
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("Update Treeq Panic occured -  " + fmt.Sprint(res))
		}
	}()
	uri := "api/rest/filesystems/" + strconv.FormatInt(fileSystemID, 10) + "/treeqs/" + strconv.FormatInt(treeqID, 10)
	treeq := Treeq{}
	resp, err := c.getJSONResponse(http.MethodPut, uri, body, &treeq)
	if err != nil {
		log.Errorf("Error occured while updating file System : %s ", err)
		return nil, err
	}
	if treeq == (Treeq{}) {
		apiresp := resp.(client.ApiResponse)
		treeq, _ = apiresp.Result.(Treeq)
	}
	log.Info("Treeq updated successfully: ", fileSystemID)
	return &treeq, nil
}
