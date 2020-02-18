package api

import (
	"context"
	"errors"
	"fmt"
	"infinibox-csi-driver/api/client"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"strconv"

	log "github.com/sirupsen/logrus"
)

// Client interface
type Client interface {
	NewClient() (*ClientService, error)
	CreateVolume(volume *VolumeParam, storagePoolName string) (*Volume, error)
	GetStoragePoolIDByName(name string) (id int64, err error)
	FindStoragePool(id int64, name string) (StoragePool, error)
	GetStoragePool(poolID int64, storagepool string) ([]StoragePool, error)
	GetVolumeByName(volumename string) (*Volume, error)
	GetVolume(volumeid int) (*Volume, error)
	CreateSnapshotVolume(snapshotParam *VolumeSnapshot) (*SnapshotVolumesResp, error)
	GetNetworkSpaceByName(networkSpaceName string) (nspace NetworkSpace, err error)
	GetHostByName(hostName string) (host Host, err error)
	MapVolumeToHost(hostID, volumeID int) (luninfo LunInfo, err error)
	UnMapVolumeFromHost(hostID, volumeID int) (err error)
	DeleteVolume(volumeID int) (err error)
	GetVolumeSnapshotByParentID(volumeID int) (*[]Volume, error)
	UpdateVolume(volumeID int, volume Volume) (*Volume, error)

	// for nfs
	OneTimeValidation(poolname string, networkspace string) (list string, err error)
	ExportFileSystem(export ExportFileSys) (*ExportResponse, error)
	DeleteExportPath(exportID int64) (*ExportResponse, error)
	DeleteFileSystem(fileSystemID int64) (*FileSystem, error)
	AttachMetadataToObject(objectID int64, body map[string]interface{}) (*[]Metadata, error)
	DetachMetadataFromObject(objectID int64) (*[]Metadata, error)
	CreateFilesystem(fileSysparameter map[string]interface{}) (*FileSystem, error)
	GetFileSystemCount() (int, error)
	GetExportByFileSystem(filesystemID int64) (*[]ExportResponse, error)
	AddNodeInExport(exportID int, access string, noRootSquash bool, ip string) (*ExportResponse, error)
	DeleteNodeFromExport(exportID int64, access string, noRootSquash bool, ip string) (*ExportResponse, error)
	CreateFileSystemSnapshot(snapshotParam *FileSystemSnapshot) (*FileSystemSnapshotResponce, error)
	DeleteFileSystemComplete(fileSystemID int64) (err error)
	DeleteParentFileSystem(fileSystemID int64) (err error)
	GetParentID(fileSystemID int64) int64
	GetFileSystemByID(fileSystemID int64) (*FileSystem, error)
	GetFileSystemByName(fileSystemName string) (*FileSystem, error)
	GetMetadataStatus(fileSystemID int64) bool
	FileSystemHasChild(fileSystemID int64) bool
	DeleteExportRule(fileSystemID int64, ipAddress string) (err error)
	UpdateFilesystem(fileSystemID int64, fileSystem FileSystem) (*FileSystem, error)
	GetSnapshotByName(snapshotName string) (*[]FileSystemSnapshotResponce, error)
	RestoreFileSystemFromSnapShot(parentID, srcSnapShotID int64) (bool, error)
}

//ClientService : struct having reference of rest client and will host methods which need rest operations
type ClientService struct {
	api        client.RestClient
	SecretsMap map[string]string
}

//NewClient : Create New Client
func (c *ClientService) NewClient() (*ClientService, error) {
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("NewClient Panic occured -  " + fmt.Sprint(res))
		}
	}()
	restclient, err := client.NewRestClient()
	if err != nil {
		return c, err
	}
	c.api = restclient
	return c, nil
}

//DeleteVolume : Delete volume by volume id
func (c *ClientService) DeleteVolume(volumeID int) (err error) {
	log.Info("Delete Volume : ", volumeID)
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("DeleteVolume Panic occured -  " + fmt.Sprint(res))
		}
	}()
	path := "/api/rest/volumes/" + strconv.Itoa(volumeID)
	_, err = c.getJSONResponse(http.MethodDelete, path, nil, nil)
	if err != nil {
		return err
	}
	log.Info("Deleted Volume : ", volumeID)
	return nil
}

//CreateVolume : create volume with volume details provided in storage pool provided
func (c *ClientService) CreateVolume(volume *VolumeParam, storagePoolName string) (*Volume, error) {
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("CreateVolume Panic occured -  " + fmt.Sprint(res))
		}
	}()
	log.Info("Create Volume with storagepoolname : ", storagePoolName)

	path := "/api/rest/volumes"
	poolID, err := c.GetStoragePoolIDByName(storagePoolName)
	log.Debugf("CreateVolume fetched storagepool poolID %d", poolID)
	if err != nil {
		return nil, err
	}
	volume.PoolId = poolID

	vol := Volume{}
	resp, err := c.getJSONResponse(http.MethodPost, path, volume, &vol)
	if err != nil {
		return nil, err
	}
	if (Volume{}) == vol {
		apiresp := resp.(client.ApiResponse)
		vol, _ = apiresp.Result.(Volume)
	}
	log.Info("Created Volume : ", vol.ID)
	return &vol, nil
}

//FindStoragePool : Find storage pool either by id or name
func (c *ClientService) FindStoragePool(id int64, name string) (StoragePool, error) {
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("FindStoragePool Panic occured -  " + fmt.Sprint(res))
		}
	}()
	log.Infof("FindStoragePool called with either id %d or name %s", id, name)
	storagePools, err := c.GetStoragePool(id, name)
	if err != nil {
		return StoragePool{}, fmt.Errorf("Error getting storage pool %s", err)
	}

	for _, storagePool := range storagePools {
		if storagePool.ID == id || storagePool.Name == name {
			log.Info("Got storage pool : ", storagePool.Name)
			return storagePool, nil
		}
	}
	return StoragePool{}, errors.New("Couldn't find storage pool")
}

//GetStoragePool : Get storage pool(s) either by id or name
func (c *ClientService) GetStoragePool(poolID int64, storagepoolname string) ([]StoragePool, error) {
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("GetStoragePool Panic occured -  " + fmt.Sprint(res))
		}
	}()
	log.Infof("GetStoragePool called with either id %d or name %s", poolID, storagepoolname)
	storagePool := StoragePool{}
	storagePools := []StoragePool{}

	if storagepoolname == "" && poolID != -1 {
		resp, err := c.getJSONResponse(http.MethodGet, "/api/rest/pools", nil, &storagePools)
		if err != nil {
			return nil, err
		}
		if len(storagePools) == 0 {
			apiresp := resp.(client.ApiResponse)
			storagePools, _ = apiresp.Result.([]StoragePool)
		}
	} else {
		queryParam := make(map[string]interface{})
		if poolID != -1 {
			queryParam["id"] = poolID
		} else {
			queryParam["name"] = storagepoolname
		}
		storagePool := StoragePool{}
		resp, err := c.getResponseWithQueryString("api/rest/pools", queryParam, &storagePool)
		if err != nil {
			return nil, err
		}
		if reflect.DeepEqual(storagePool, (StoragePool{})) {
			apiresp := resp.(client.ApiResponse)
			storagePool, _ = apiresp.Result.(StoragePool)
		}
	}

	if storagepoolname != "" {
		storagePools = append(storagePools, storagePool)
	}
	return storagePools, nil
}

//GetStoragePoolIDByName : Returns poolID of provided pool name
func (c *ClientService) GetStoragePoolIDByName(name string) (id int64, err error) {
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("error while Get Pool ID  " + fmt.Sprint(res))
		}
	}()
	log.Info("Get ID of a storage pool by Name : ", name)
	storagePools := []StoragePool{}
	//To get the pool_id for corresponding poolname
	var poolID int64 = -1
	urlpool := "api/rest/pools"
	queryParam := make(map[string]interface{})
	queryParam["name"] = name
	resp, err := c.getResponseWithQueryString(urlpool, queryParam, &storagePools)
	if err != nil {
		return -1, fmt.Errorf("volume with given name not found")
	}
	if len(storagePools) == 0 {
		apiresp := resp.(client.ApiResponse)
		storagePools, _ = apiresp.Result.([]StoragePool)
	}
	if len(storagePools) > 0 {
		return storagePools[0].ID, nil
	}
	if poolID == -1 {
		return poolID, errors.New("No such pool: " + name)
	}
	log.Info("Got ID of a storage pool : ", poolID)
	return poolID, nil
}

// GetVolumeByName : find volume with given name
func (c *ClientService) GetVolumeByName(volumename string) (*Volume, error) {
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("GetStoragePool Panic occured -  " + fmt.Sprint(res))
		}
	}()
	log.Info("Get a Volume by Name : ", volumename)
	voluri := "/api/rest/volumes"
	volumes := []Volume{}
	queryParam := make(map[string]interface{})
	queryParam["name"] = volumename
	resp, err := c.getResponseWithQueryString(voluri,
		queryParam, &volumes)
	if err != nil {
		return nil, err
	}
	if len(volumes) == 0 {
		apiresp := resp.(client.ApiResponse)
		volumes, _ = apiresp.Result.([]Volume)
	}
	for _, vol := range volumes {
		if vol.Name == volumename {
			log.Info("Got a Volume of Name : ", volumename)
			return &vol, nil
		}
	}

	return nil, errors.New("volume with given name not found")
}

//GetVolume : get volume by id
func (c *ClientService) GetVolume(volumeid int) (*Volume, error) {
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("GetVolume Panic occured -  " + fmt.Sprint(res))
		}
	}()
	log.Info("Get a Volume of ID : ", volumeid)
	volume := Volume{}
	path := "/api/rest/volumes/" + strconv.Itoa(volumeid)
	resp, err := c.getJSONResponse(http.MethodGet, path, nil, &volume)
	if err != nil {
		return nil, err
	}
	if volume == (Volume{}) {
		apiresp := resp.(client.ApiResponse)
		volume, _ = apiresp.Result.(Volume)
	}
	return &volume, nil
}

//CreateSnapshotVolume : Create volume from snapshot
func (c *ClientService) CreateSnapshotVolume(snapshotParam *VolumeSnapshot) (*SnapshotVolumesResp, error) {
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("CreateSnapshotVolume Panic occured -  " + fmt.Sprint(res))
		}
	}()
	log.Debug(" Call in Create snapshot")
	log.Info("Create a volume from snapshot : ", snapshotParam.SnapshotName)
	path := "/api/rest/volumes"
	snapResp := SnapshotVolumesResp{}
	resp, err := c.getJSONResponse(http.MethodPost, path, snapshotParam, &snapResp)
	if err != nil {
		return nil, err
	}
	if reflect.DeepEqual(snapResp, (SnapshotVolumesResp{})) {
		apiresp := resp.(client.ApiResponse)
		snapResp, _ = apiresp.Result.(SnapshotVolumesResp)
	}
	log.Info("Created snapshot : ", snapResp.Name)
	return &snapResp, nil
}

//GetNetworkSpaceByName - Get networkspace by name
func (c *ClientService) GetNetworkSpaceByName(networkSpaceName string) (nspace NetworkSpace, err error) {
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("GetNetworkSpaceByName Panic occured -  " + fmt.Sprint(res))
		}
	}()
	log.Info("Get network space by name : ", networkSpaceName)
	netspaces := []NetworkSpace{}
	path := "api/rest/network/spaces"
	queryParam := map[string]interface{}{"name": networkSpaceName}
	resp, err := c.getResponseWithQueryString(path, queryParam, &netspaces)
	if err != nil {
		log.Errorf("No such network space : %s", networkSpaceName)
		return nspace, err
	}
	if len(netspaces) == 0 {
		apiresp := resp.(client.ApiResponse)
		netspaces, _ = apiresp.Result.([]NetworkSpace)
	}
	if len(netspaces) > 0 {
		nspace = netspaces[0]
	}
	log.Info("Got network space : ", networkSpaceName)
	return nspace, nil
}

//GetHostByName - get host details for given hostname
func (c *ClientService) GetHostByName(hostName string) (host Host, err error) {
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("GetHostByName Panic occured -  " + fmt.Sprint(res))
		}
	}()
	log.Info("Get Host by name : ", hostName)
	uri := "api/rest/hosts"
	hosts := []Host{}
	queryParam := map[string]interface{}{"name": hostName}
	resp, err := c.getResponseWithQueryString(uri, queryParam, &hosts)
	if err != nil {
		log.Errorf("No such host : %s", hostName)
		return host, err
	}
	if len(hosts) == 0 {
		apiresp := resp.(client.ApiResponse)
		hosts, _ = apiresp.Result.([]Host)
	}

	if len(hosts) > 0 {
		host = hosts[0]
	}
	log.Info("Got Host: ", hostName)
	return host, nil
}

// UnMapVolumeFromHost - Remove mapping of volume and host
func (c *ClientService) UnMapVolumeFromHost(hostID, volumeID int) (err error) {
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("UnMapVolumeFromHost Panic occured -  " + fmt.Sprint(res))
		}
	}()
	log.Infof("Remove mapping of volume %d from host %d", volumeID, hostID)
	uri := "api/rest/hosts/" + strconv.Itoa(hostID) + "/luns"
	body := map[string]interface{}{"volume_id": volumeID}
	_, err = c.getJSONResponse(http.MethodDelete, uri, body, nil)
	if err != nil {
		log.Errorf("Error occured while unmapping volume from host %v", err)
		return err
	}
	log.Infof("Successfully removed mapping of volume %d from host %d", volumeID, hostID)
	return nil
}

// MapVolumeToHost - Map volume with given volumeID to Host with given hostID
func (c *ClientService) MapVolumeToHost(hostID, volumeID int) (luninfo LunInfo, err error) {
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("MapVolumeToHost Panic occured -  " + fmt.Sprint(res))
		}
	}()
	log.Infof("Map volume of %d to host %d", volumeID, hostID)
	uri := "api/rest/hosts/" + strconv.Itoa(hostID) + "/luns"
	body := map[string]interface{}{"volume_id": volumeID}
	resp, err := c.getJSONResponse(http.MethodPost, uri, body, &luninfo)
	if err != nil {
		log.Errorf("error occured while mapping volume to host %v", err)
		return luninfo, err
	}
	if luninfo == (LunInfo{}) {
		apiresp := resp.(client.ApiResponse)
		luninfo, _ = apiresp.Result.(LunInfo)
	}
	log.Infof("Successfully mapped volume of %d to host %d", volumeID, hostID)
	return luninfo, nil
}

// **************************************************Util Methods*********************************************
func (c *ClientService) getJSONResponse(method, apiuri string, body, expectedResp interface{}) (resp interface{}, err error) {
	log.Debugf("Request made for method: %s and apiuri %s", method, apiuri)
	defer func() {
		if res := recover(); res != nil && err == nil {
			log.Errorf("Error in getJSONResponse while makeing %s request on %s url error : %v ", method, apiuri, err)
			err = errors.New("error in getJSONResponse " + fmt.Sprint(res))
		}
	}()
	hostsecret, err := c.getAPIConfig()
	if err != nil {
		log.Errorf("Error occured: %v ", err)
		return nil, err
	}
	if method == http.MethodPost {
		resp, err = c.api.Post(context.Background(), apiuri, hostsecret, body, expectedResp)
	} else if method == http.MethodGet {
		resp, err = c.api.Get(context.Background(), apiuri, hostsecret, expectedResp)
	} else if method == http.MethodDelete {
		resp, err = c.api.Delete(context.Background(), apiuri, hostsecret)
	} else if method == http.MethodPut {
		resp, err = c.api.Put(context.Background(), apiuri, hostsecret, body, expectedResp)
	}
	if err != nil {
		log.Errorf("Error occured: %v ", err)
		return
	}
	if expectedResp == nil {
		expectedResp = resp
	}
	log.Debugf("getJSONResponse response: method %s and apiuri %s and err %v", method, apiuri, err)
	return
}

func (c *ClientService) getResponseWithQueryString(apiuri string, queryParam map[string]interface{}, expectedResp interface{}) (resp interface{}, err error) {
	log.Debugf("Request made for apiuri %s", apiuri)
	defer func() {
		if res := recover(); res != nil && err == nil {
			log.Errorf("Error in getResponseWithQueryString while making request on %s url error : %v ", apiuri, err)
			err = errors.New("error in getResponseWithQueryString " + fmt.Sprint(res))
		}
	}()
	hostsecret, err := c.getAPIConfig()
	if err != nil {
		log.Errorf("Error occured: %v ", err)
		return nil, err
	}

	queryString := ""
	for key, val := range queryParam {
		if queryString != "" {
			queryString = queryString + ","
		}
		queryString = key + "=" + fmt.Sprintf("%v", val)
	}
	log.Debugf("getResponseWithQueryString queryString is %s ", queryString)
	resp, err = c.api.GetWithQueryString(context.Background(), apiuri, hostsecret, queryString, expectedResp)
	log.Debugf("getResponseWithQueryString return err %v ", err)

	return resp, err
}

func (c *ClientService) getAPIConfig() (hostconfig client.HostConfig, err error) {
	defer func() {
		if res := recover(); res != nil && err == nil {
			log.Error("error in getAPIConfig : ", err)
			err = errors.New("error in getAPIConfig " + fmt.Sprint(res))
		}
	}()
	if c.SecretsMap == nil {
		return hostconfig, errors.New("Secret not found")
	}
	if c.SecretsMap["hosturl"] != "" && c.SecretsMap["username"] != "" && c.SecretsMap["password"] != "" {

		hosturl, err := url.ParseRequestURI(c.SecretsMap["hosturl"])
		if err != nil {
			log.Warn("hosturl is not url, checking if it is valid IpAddress")
			if net.ParseIP(c.SecretsMap["hosturl"]) != nil {
				hostconfig.ApiHost = "https://" + c.SecretsMap["hosturl"] + "/"
			} else {
				return hostconfig, err
			}
			log.Info("setting url as ", hostconfig.ApiHost)
		} else {
			hostconfig.ApiHost = hosturl.String()
		}
		hostconfig.UserName = c.SecretsMap["username"]
		hostconfig.Password = c.SecretsMap["password"]
		return hostconfig, nil
	}
	return hostconfig, errors.New("host configuration is not valid")
}

//GetVolumeSnapshotByParentID method return true is the filesystemID has child else false
func (c *ClientService) GetVolumeSnapshotByParentID(volumeID int) (*[]Volume, error) {
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("GetVolumeSnapshotByParentID Panic occured -  " + fmt.Sprint(res))
		}
	}()
	voluri := "/api/rest/volumes/"
	volumes := []Volume{}
	queryParam := make(map[string]interface{})
	queryParam["parent_id"] = volumeID
	resp, err := c.getResponseWithQueryString(voluri, queryParam, &volumes)
	if err != nil {
		log.Errorf("fail to check GetVolumeSnapshotByParentID %v", err)
		return &volumes, err
	}
	if len(volumes) == 0 {
		apiresp := resp.(client.ApiResponse)
		volumes, _ = apiresp.Result.([]Volume)
	}
	return &volumes, err
}

//UpdateVolume : update volume
func (c *ClientService) UpdateVolume(volumeID int, volume Volume) (*Volume, error) {
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("UpdateVolume Panic occured -  " + fmt.Sprint(res))
		}
	}()
	log.Info("Update volume : ", volumeID)
	uri := "api/rest/volumes/" + strconv.Itoa(volumeID)
	volumeResp := Volume{}

	resp, err := c.getJSONResponse(http.MethodPut, uri, volume, &volumeResp)
	if err != nil {
		log.Errorf("Error occured while updating volume : %s", err)
		return nil, err
	}

	if volumeResp == (Volume{}) {
		apiresp := resp.(client.ApiResponse)
		volumeResp, _ = apiresp.Result.(Volume)
	}
	log.Info("Updated volume : ", volumeID)
	return &volumeResp, nil
}
