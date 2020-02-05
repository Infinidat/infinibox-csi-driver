package api

import (
	"context"
	"errors"
	"fmt"
	"infinibox-csi-driver/api/client"
	"infinibox-csi-driver/api/clientgo"
	"net/http"
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
	GetVolume(volumeid int) ([]Volume, error)
	CreateSnapshotVolume(snapshotParam *SnapshotDef) (*SnapshotVolumesResp, error)
	GetNetworkSpaceByName(networkSpaceName string) (nspace NetworkSpace, err error)
	GetLunByVolumeID(volumeID string) (lunID LunInfo, err error)
	GetHostByName(hostName string) (host Host, err error)
	MapVolumeToHost(hostID, volumeID int) (luninfo LunInfo, err error)
	UnMapVolumeFromHost(hostID, volumeID int) (err error)
	DeleteVolume(volumeID int) (err error)

	// for nfs
	OneTimeValidation(poolname string, networkspace string) (list string, err error)
	ExportFileSystem(export ExportFileSys) (*ExportResponse, error)
	CreateExportPath(exportRef *ExportPathRef) (*ExportResponse, error)
	DeleteExportPath(exportID int64) (*ExportResponse, error)
	DeleteFileSystem(fileSystemID int64) (*FileSystem, error)
	AttachMetadataToObject(objectID int64, body map[string]interface{}) (*[]Metadata, error)
	DetachMetadataFromObject(objectID int64) (*[]Metadata, error)
	CreateFilesystem(fileSystem FileSystem) (*FileSystem, error)
	GetFileSystemCount() (int, error)
	GetExportByID(exportID int) (*ExportResponse, error)
	GetExportByFileSystem(filesystemID int64) (*[]ExportResponse, error)
	AddNodeInExport(exportID int, access string, noRootSquash bool, ip string) (*ExportResponse, error)
	DeleteNodeFromExport(exportID int, access string, noRootSquash bool, ip string) (*ExportResponse, error)
	UpdateFilesystem(fileSystemID int64, fileSystem FileSystem) (*FileSystem, error)
}

//ClientService : struct having reference of rest client and will host methods which need rest operations
type ClientService struct {
	api              client.RestClient
	StroageClassName string
	NameSpace        string
	SecretName       string
}

//NewClient : Create New Client
func (c *ClientService) NewClient() (*ClientService, error) {
	restclient, err := client.NewRestClient()
	if err != nil {
		return c, err
	}
	c.api = restclient
	return c, nil
}

//DeleteVolume : Delete volume by volume id
func (c *ClientService) DeleteVolume(volumeID int) (err error) {
	path := "/api/rest/volumes/" + strconv.Itoa(volumeID)
	_, err = c.getJSONResponse(http.MethodDelete, path, nil, nil)
	if err != nil {
		return err
	}
	return nil
}

//CreateVolume : create volume with volume details provided in storage pool provided
func (c *ClientService) CreateVolume(
	volume *VolumeParam,
	storagePoolName string) (*Volume, error) {
	log.Debugf("CreateVolume called with storagepoolname %s", storagePoolName)
	path := "/api/rest/volumes"
	poolID, err := c.GetStoragePoolIDByName(storagePoolName)
	log.Debugf("CreateVolume fetched storagepool poolID %s", poolID)
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
		vol, _ = resp.(Volume)
	}
	log.Debugf("CreateVolume post api response %v", vol)
	return &vol, nil
}

//FindStoragePool : Find storage pool either by id or name
func (c *ClientService) FindStoragePool(id int64, name string) (StoragePool, error) {
	log.Debugf("FindStoragePool called with either id %d or name %s", id, name)
	storagePools, err := c.GetStoragePool(id, name)
	log.Debugf("FindStoragePool GetStoragePool got storagePools %v", storagePools)
	if err != nil {
		return StoragePool{}, fmt.Errorf("Error getting storage pool %s", err)
	}

	for _, storagePool := range storagePools {
		if storagePool.ID == id || storagePool.Name == name {
			return storagePool, nil
		}
	}

	return StoragePool{}, errors.New("Couldn't find storage pool")
}

//GetStoragePool : Get storage pool(s) either by id or name
func (c *ClientService) GetStoragePool(poolID int64,
	storagepoolname string) ([]StoragePool, error) {
	log.Debugf("GetStoragePool called with either id %d or name %s", poolID, storagepoolname)
	storagePool := StoragePool{}
	storagePools := []StoragePool{}

	if storagepoolname == "" && poolID != -1 {
		resp, err := c.getJSONResponse(http.MethodGet, "/api/rest/pools", nil, &storagePools)
		if err != nil {
			return nil, err
		}
		if len(storagePools) == 0 {
			storagePools, _ = resp.([]StoragePool)
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
			storagePool, _ = resp.(StoragePool)
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
		storagePools, _ = resp.([]StoragePool)
	}
	if len(storagePools) > 0 {
		return storagePools[0].ID, nil
	}
	if poolID == -1 {
		return poolID, errors.New("No such pool: " + name)
	}
	log.Debugf("GetStoragePoolIDByName returning result poolID %d", poolID)
	return poolID, nil
}

// GetVolumeByName : find volume with given name
func (c *ClientService) GetVolumeByName(volumename string) (*Volume, error) {
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
		volumes, _ = resp.([]Volume)
	}
	for _, vol := range volumes {
		if vol.Name == volumename {
			return &vol, nil
		}
	}

	return nil, errors.New("volume with given name not found")
}

//GetVolume : get volume by id
func (c *ClientService) GetVolume(volumeid int) ([]Volume, error) {
	var (
		path    string
		volume  = Volume{}
		volumes = []Volume{}
	)
	if volumeid != -1 {
		path = "/api/rest/volumes/" + strconv.Itoa(volumeid)
	} else {
		path = "/api/rest/volumes"
	}

	if volumeid == -1 {
		resp, err := c.getJSONResponse(http.MethodGet, path, nil, &volumes)
		if err != nil {
			return nil, err
		}
		if len(volumes) == 0 {
			volumes, _ = resp.([]Volume)
		}
	} else {
		resp, err := c.getJSONResponse(http.MethodGet, path, nil, &volume)
		if err != nil {
			return nil, err
		}
		if volume == (Volume{}) {
			volume, _ = resp.(Volume)
		}
	}

	if volumeid == -1 {
		var volumesNew []Volume
		for _, volume := range volumes {
			volumesNew = append(volumesNew, volume)
		}
		volumes = volumesNew
	} else {
		volumes = append(volumes, volume)
	}
	return volumes, nil
}

//CreateSnapshotVolume : Create volume from snapshot
func (c *ClientService) CreateSnapshotVolume(snapshotParam *SnapshotDef) (*SnapshotVolumesResp, error) {
	path := "/api/rest/volumes"
	snapResp := SnapshotVolumesResp{}
	resp, err := c.getJSONResponse(
		http.MethodPost, path, snapshotParam, &snapResp)
	if err != nil {
		return nil, err
	}
	if reflect.DeepEqual(snapResp, (SnapshotVolumesResp{})) {
		snapResp, _ = resp.(SnapshotVolumesResp)
	}
	return &snapResp, nil
}

//GetNetworkSpaceByName - Get networkspace by name
func (c *ClientService) GetNetworkSpaceByName(networkSpaceName string) (nspace NetworkSpace, err error) {
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("error while getting target iqn " + fmt.Sprint(res))
		}
	}()
	netspaces := []NetworkSpace{}
	path := "api/rest/network/spaces"
	queryParam := map[string]interface{}{"name": networkSpaceName}
	resp, err := c.getResponseWithQueryString(path, queryParam, &netspaces)
	if err != nil {
		log.Errorf("error occured whilte rtriving IQN: %v", err)
		return nspace, err
	}
	if len(netspaces) == 0 {
		netspaces, _ = resp.([]NetworkSpace)
	}
	if len(netspaces) > 0 {
		nspace = netspaces[0]
	}
	return nspace, nil
}

// GetLunByVolumeID - Get Lun details by volumeID
func (c *ClientService) GetLunByVolumeID(volumeID string) (lun LunInfo, err error) {
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("error while getting target iqn " + fmt.Sprint(res))
		}
	}()
	lunInfo := []LunInfo{}
	uri := "api/rest/volumes/" + volumeID + "/luns"
	resp, err := c.getJSONResponse(http.MethodGet, uri, nil, &lunInfo)
	if err != nil {
		log.Errorf("error occured whilte rtriving IQN: %v", err)
		return lun, err
	}
	if len(lunInfo) == 0 {
		lunInfo, _ = resp.([]LunInfo)
	}
	if len(lunInfo) > 0 {
		lun = lunInfo[0]
	}
	return lun, nil
}

//GetHostByName - get host details for given hostname
func (c *ClientService) GetHostByName(hostName string) (host Host, err error) {
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("error while getting host " + fmt.Sprint(res))
		}
	}()

	uri := "api/rest/hosts"
	hosts := []Host{}
	queryParam := map[string]interface{}{"name": hostName}
	resp, err := c.getResponseWithQueryString(uri, queryParam, &hosts)
	if err != nil {
		log.Errorf("error occured whilte rtriving host: %v", err)
		return host, err
	}
	if len(hosts) == 0 {
		hosts, _ = resp.([]Host)
	}

	if len(hosts) > 0 {
		host = hosts[0]
	}
	return host, nil
}

// UnMapVolumeFromHost - Remove mapping of volume and host
func (c *ClientService) UnMapVolumeFromHost(hostID, volumeID int) (err error) {
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("error while unmapping volume from host " + fmt.Sprint(res))
		}
	}()
	uri := "api/rest/hosts/" + strconv.Itoa(hostID) + "/luns"
	body := map[string]interface{}{"volume_id": volumeID}
	_, err = c.getJSONResponse(http.MethodDelete, uri, body, nil)
	if err != nil {
		log.Errorf("error occured while unmapping volume from host %v", err)
		return err
	}
	return nil
}

// MapVolumeToHost - Map volume with given volumeID to Host with given hostID
func (c *ClientService) MapVolumeToHost(hostID, volumeID int) (luninfo LunInfo, err error) {
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("error while mapping volume to host " + fmt.Sprint(res))
		}
	}()
	uri := "api/rest/hosts/" + strconv.Itoa(hostID) + "/luns"
	body := map[string]interface{}{"volume_id": volumeID}
	resp, err := c.getJSONResponse(http.MethodPost, uri, body, &luninfo)
	if err != nil {
		log.Errorf("error occured while mapping volume to host %v", err)
		return luninfo, err
	}
	if luninfo == (LunInfo{}) {
		luninfo, _ = resp.(LunInfo)
	}
	return luninfo, nil
}

// **************************************************Util Methods*********************************************
func (c *ClientService) getJSONResponse(method, apiuri string, body, expectedResp interface{}) (resp interface{}, err error) {
	log.Debugf("getJSONResponse request made for method: %s and apiuri %s", method, apiuri)
	hostsecret, err := c.getAPIConfigForStorageClass()
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
	log.Debugf("request made for apiuri %s", apiuri)
	hostsecret, err := c.getAPIConfigForStorageClass()
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
	//expectedResp = resp
	log.Debugf("getResponseWithQueryString return err %v ", err)

	return resp, err
}

func (c *ClientService) getAPIConfigForStorageClass() (client.HostConfig, error) {
	// if hs, ok := secretsMap[cs.SecretName]; ok {
	// 	return hs, nil
	// } else {
	kubeclient := clientgo.BuildClient()
	secrets, err := kubeclient.GetSecret(c.SecretName, c.NameSpace)
	if err != nil {
		return client.HostConfig{}, err
	}
	if secrets["hostip"] != "" && secrets["username"] != "" && secrets["password"] != "" {
		hs := client.HostConfig{}
		hs.ApiHost = "https://" + secrets["hostip"] + "/"
		hs.UserName = secrets["username"]
		hs.Password = secrets["password"]
		// secretsMap[cs.StroageClassName] = hs
		return hs, nil
	}
	//}
	return client.HostConfig{}, errors.New("Secret not found with name " + c.SecretName)
}
