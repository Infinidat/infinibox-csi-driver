/*Copyright 2022 Infinidat
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.*/
package api

import (
	"context"
	"errors"
	"fmt"
	"infinibox-csi-driver/api/client"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"

	"k8s.io/klog"
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
	DeleteVolume(volumeID int) (err error)
	UpdateVolume(volumeID int, volume Volume) (*Volume, error)
	GetVolumeSnapshotByParentID(volumeID int) (*[]Volume, error)

	GetHostByName(hostName string) (host Host, err error)
	CreateHost(hostName string) (host Host, err error)
	AddHostPort(portType, portAddress string, hostID int) (hostPort HostPort, err error)
	AddHostSecurity(chapCreds map[string]string, hostID int) (host Host, err error)
	MapVolumeToHost(hostID, volumeID, lun int) (luninfo LunInfo, err error)
	DeleteHost(hostID int) (err error)
	GetLunByHostVolume(hostID, volumeID int) (luninfo LunInfo, err error)
	GetAllLunByHost(hostID int) (luninfo []LunInfo, err error)
	UnMapVolumeFromHost(hostID, volumeID int) (err error)
	GetFCPorts() (fcNodes []FCNode, err error)
	GetHostPort(hostID int, portAddress string) (hostPort HostPort, err error)

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

	GetFileSystemsByPoolID(poolID int64, page int) (*FSMetadata, error)
	GetFilesytemTreeqCount(fileSystemID int64) (treeqCnt int, err error)
	CreateTreeq(filesystemID int64, treeqParameter map[string]interface{}) (*Treeq, error)
	DeleteTreeq(fileSystemID, treeqID int64) (*Treeq, error)
	GetTreeq(fileSystemID, treeqID int64) (*Treeq, error)
	UpdateTreeq(fileSystemID, treeqID int64, body map[string]interface{}) (*Treeq, error)
	GetTreeqSizeByFileSystemID(filesystemID int64) (int64, error)
	GetFileSystemCountByPoolID(poolID int64) (int, error)
	GetTreeqByName(fileSystemID int64, treeqName string) (*Treeq, error)
}

// ClientService : struct having reference of rest client and will host methods which need rest operations
type ClientService struct {
	api        client.RestClient
	SecretsMap map[string]string
}

// NewClient : Create New Client
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

// DeleteVolume : Delete volume by volume id
func (c *ClientService) DeleteVolume(volumeID int) (err error) {
	klog.V(2).Infof("Delete Volume with ID %d", volumeID)
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("DeleteVolume Panic occured -  " + fmt.Sprint(res))
		}
	}()
	_, err = c.DetachMetadataFromObject(int64(volumeID))
	if err != nil {
		if strings.Contains(err.Error(), "METADATA_IS_NOT_SUPPORTED_FOR_ENTITY") {
			err = nil
		} else {
			klog.Errorf("failed to delete metadata %v", err)
			return
		}
	}

	path := "/api/rest/volumes/" + strconv.Itoa(volumeID) + "?approved=true"
	_, err = c.getJSONResponse(http.MethodDelete, path, nil, nil)
	if err != nil {
		return err
	}
	klog.V(2).Infof("Deleted Volume : %d", volumeID)
	return
}

// AddHostSecurity - add chap security for host with given details
func (c *ClientService) AddHostSecurity(chapCreds map[string]string, hostID int) (host Host, err error) {
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("AddHostSecurity Panic occured -  " + fmt.Sprint(res))
		}
	}()
	klog.V(2).Infof("add chap atuhentication for hostID %d : ", hostID)
	uri := "api/rest/hosts/" + strconv.Itoa(hostID) + "?approved=true"
	resp, err := c.getJSONResponse(http.MethodPut, uri, chapCreds, host)
	if err != nil {
		klog.Errorf("failed to add chap security to host %d with error %v", hostID, err)
		return host, err
	}
	if reflect.DeepEqual(host, (Host{})) {
		apiresp := resp.(client.ApiResponse)
		host, _ = apiresp.Result.(Host)
	}
	klog.V(2).Infof("created chap authentication for host %s: ", host.Name)
	return host, nil
}

// AddHostPort - add port for host with given details
func (c *ClientService) AddHostPort(portType, portAddress string, hostID int) (hostPort HostPort, err error) {
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("AddHostPort Panic occured -  " + fmt.Sprint(res))
		}
	}()
	klog.V(2).Infof("add port for hostID %s %d : ", portAddress, hostID)
	uri := "api/rest/hosts/" + strconv.Itoa(hostID) + "/ports?approved=true"
	body := map[string]interface{}{"address": portAddress, "type": portType}
	resp, err := c.getJSONResponse(http.MethodPost, uri, body, &hostPort)
	if err != nil {
		if strings.Contains(err.Error(), "PORT_ALREADY_BELONGS_TO_HOST") {
			klog.V(4).Infof("Success: No need to add port '%s' to host with ID %d, port already belongs to host", portAddress, hostID)
		} else {
			klog.Errorf("Error adding port '%s' to host with ID %d, error: %+v", portAddress, hostID, err)
			return hostPort, err
		}
	}
	if reflect.DeepEqual(hostPort, (HostPort{})) {
		apiresp := resp.(client.ApiResponse)
		hostPort, _ = apiresp.Result.(HostPort)
	}

	klog.V(2).Infof("created host port: %s", hostPort.PortAddress)
	return hostPort, nil
}

// CreateVolume : create volume with volume details provided in storage pool provided
func (c *ClientService) CreateVolume(volume *VolumeParam, storagePoolName string) (*Volume, error) {
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("CreateVolume Panic occured -  " + fmt.Sprint(res))
		}
	}()
	klog.V(2).Infof("Create Volume with storagepoolname: %s", storagePoolName)

	path := "/api/rest/volumes"
	poolID, err := c.GetStoragePoolIDByName(storagePoolName)
	klog.V(4).Infof("CreateVolume fetched storagepool poolID %d", poolID)
	if err != nil {
		return nil, err
	}
	volume.PoolId = poolID
	valumeParameter := make(map[string]interface{})
	valumeParameter["pool_id"] = poolID
	valumeParameter["size"] = volume.VolumeSize
	valumeParameter["name"] = volume.Name
	valumeParameter["provtype"] = volume.ProvisionType
	valumeParameter["ssd_enabled"] = volume.SsdEnabled
	vol := Volume{}
	resp, err := c.getJSONResponse(http.MethodPost, path, valumeParameter, &vol)
	if err != nil {
		return nil, err
	}
	if (Volume{}) == vol {
		apiresp := resp.(client.ApiResponse)
		vol, _ = apiresp.Result.(Volume)
	}
	klog.V(2).Infof("Created Volume with ID %d", vol.ID)
	return &vol, nil
}

// FindStoragePool : Find storage pool either by id or name
func (c *ClientService) FindStoragePool(id int64, name string) (StoragePool, error) {
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("FindStoragePool Panic occured -  " + fmt.Sprint(res))
		}
	}()
	klog.V(2).Infof("FindStoragePool called with either id %d or name %s", id, name)
	storagePools, err := c.GetStoragePool(id, name)
	if err != nil {
		return StoragePool{}, fmt.Errorf("Error getting storage pool %s", err)
	}

	for _, storagePool := range storagePools {
		if storagePool.ID == id || storagePool.Name == name {
			klog.V(2).Infof("Got storage pool: %s", storagePool.Name)
			return storagePool, nil
		}
	}
	return StoragePool{}, errors.New("Couldn't find storage pool")
}

// GetStoragePool : Get storage pool(s) either by id or name
func (c *ClientService) GetStoragePool(poolID int64, storagepoolname string) ([]StoragePool, error) {
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("GetStoragePool Panic occured -  " + fmt.Sprint(res))
		}
	}()
	klog.V(2).Infof("GetStoragePool called with either id %d or name %s", poolID, storagepoolname)
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

// GetStoragePoolIDByName : Returns poolID of provided pool name
func (c *ClientService) GetStoragePoolIDByName(name string) (id int64, err error) {
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("error while Get Pool ID  " + fmt.Sprint(res))
		}
	}()
	klog.V(2).Infof("Get ID of a storage pool by Name : %s", name)
	storagePools := []StoragePool{}
	// To get the pool_id for corresponding poolname
	var poolID int64 = -1
	urlpool := "api/rest/pools"
	queryParam := make(map[string]interface{})
	queryParam["name"] = name
	resp, err := c.getResponseWithQueryString(urlpool, queryParam, &storagePools)
	if err != nil {
		return -1, fmt.Errorf("failed to get pool ID from pool Name: %s", name)
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
	klog.V(2).Infof("Got ID of a storage pool: %d", poolID)
	return poolID, nil
}

// GetVolumeByName : find volume with given name
func (c *ClientService) GetVolumeByName(volumename string) (*Volume, error) {
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("GetVolumeByName Panic occured -  " + fmt.Sprint(res))
		}
	}()
	klog.V(2).Infof("Get a Volume by Name: %s", volumename)
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
			klog.V(2).Infof("Got a Volume of Name: %s", volumename)
			return &vol, nil
		}
	}

	return nil, errors.New("volume with given name not found")
}

// GetVolume : get volume by id
func (c *ClientService) GetVolume(volumeid int) (*Volume, error) {
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("GetVolume Panic occured -  " + fmt.Sprint(res))
		}
	}()
	klog.V(2).Infof("Get a Volume of ID: %d", volumeid)
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

// CreateSnapshotVolume : Create volume from snapshot
func (c *ClientService) CreateSnapshotVolume(snapshotParam *VolumeSnapshot) (*SnapshotVolumesResp, error) {
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("CreateSnapshotVolume Panic occured -  " + fmt.Sprint(res))
		}
	}()
	klog.V(2).Infof("Create a snapshot: %s", snapshotParam.SnapshotName)
	path := "/api/rest/volumes"
	snapResp := SnapshotVolumesResp{}
	valumeParameter := make(map[string]interface{})
	valumeParameter["parent_id"] = snapshotParam.ParentID
	valumeParameter["name"] = snapshotParam.SnapshotName
	valumeParameter["write_protected"] = snapshotParam.WriteProtected
	valumeParameter["ssd_enabled"] = snapshotParam.SsdEnabled
	resp, err := c.getJSONResponse(http.MethodPost, path, valumeParameter, &snapResp)
	if err != nil {
		return nil, err
	}
	if reflect.DeepEqual(snapResp, (SnapshotVolumesResp{})) {
		apiresp := resp.(client.ApiResponse)
		snapResp, _ = apiresp.Result.(SnapshotVolumesResp)
	}
	klog.V(2).Infof("Created snapshot: %s", snapResp.Name)
	return &snapResp, nil
}

// GetNetworkSpaceByName - Get networkspace by name
func (c *ClientService) GetNetworkSpaceByName(networkSpaceName string) (nspace NetworkSpace, err error) {
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("GetNetworkSpaceByName Panic occured -  " + fmt.Sprint(res))
		}
	}()
	klog.V(2).Infof("Get network space by name: %s", networkSpaceName)
	netspaces := []NetworkSpace{}
	path := "api/rest/network/spaces"
	queryParam := map[string]interface{}{"name": networkSpaceName}
	resp, err := c.getResponseWithQueryString(path, queryParam, &netspaces)
	if err != nil {
		klog.Errorf("No such network space: %s", networkSpaceName)
		return nspace, err
	}
	if len(netspaces) == 0 {
		apiresp := resp.(client.ApiResponse)
		netspaces, _ = apiresp.Result.([]NetworkSpace)
	}
	if len(netspaces) > 0 {
		nspace = netspaces[0]
	}
	klog.V(2).Infof("Got network space: %s", networkSpaceName)
	return nspace, nil
}

// DeleteHost - delete host by given host ID
func (c *ClientService) DeleteHost(hostID int) (err error) {
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("DeleteHost Panic occured -  " + fmt.Sprint(res))
		}
	}()
	klog.V(2).Infof("delete host with host ID %d", hostID)
	uri := "api/rest/hosts/" + strconv.Itoa(hostID)
	_, err = c.getJSONResponse(http.MethodDelete, uri, nil, nil)
	if err != nil {
		if !strings.Contains(err.Error(), "HOST_NOT_FOUND") {
			klog.Errorf("failed to delete host with id %d with error %v", hostID, err)
		}
		return err
	}
	klog.V(2).Infof("delete host with id %d", hostID)
	return nil
}

// CreateHost - create host  with given details
func (c *ClientService) CreateHost(hostName string) (host Host, err error) {
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("CreateHost Panic occured -  " + fmt.Sprint(res))
		}
	}()
	klog.V(2).Infof("create host with name %s", hostName)
	uri := "api/rest/hosts"
	body := map[string]interface{}{"name": hostName}
	resp, err := c.getJSONResponse(http.MethodPost, uri, body, &host)
	if err != nil {
		klog.Errorf("error creating host : %s error : %v", hostName, err)
		return host, err
	}
	if reflect.DeepEqual(host, (Host{})) {
		apiresp := resp.(client.ApiResponse)
		host, _ = apiresp.Result.(Host)
	}

	klog.V(2).Infof("created host with name %s", host.Name)
	return host, nil
}

// GetHostPort - get host port details
func (c *ClientService) GetHostPort(hostID int, portAddress string) (hostPort HostPort, err error) {
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("GetHostPort Panic occured -  " + fmt.Sprint(res))
		}
	}()
	klog.V(2).Infof("get host port by port address %s", portAddress)
	uri := "api/rest/hosts/" + strconv.Itoa(hostID) + "/ports"
	hostPorts := []HostPort{}
	resp, err := c.getJSONResponse(http.MethodGet, uri, nil, &hostPorts)
	if err != nil {
		klog.Errorf("unable to get host port %s with error ", portAddress)
		return hostPort, err
	}
	if len(hostPorts) == 0 {
		apiresp := resp.(client.ApiResponse)
		hostPorts, _ = apiresp.Result.([]HostPort)
	}

	for _, port := range hostPorts {
		if port.PortAddress == portAddress {
			hostPort = port
		}
	}
	if hostPort.HostID == 0 && hostPort.PortAddress == "" {
		return hostPort, errors.New("HOST_PORT_NOT_FOUND")
	}
	klog.V(2).Infof("fetched hostPort with address %s", hostPort.PortAddress)
	return hostPort, nil
}

// GetHostByName - get host details for given hostname
func (c *ClientService) GetHostByName(hostName string) (host Host, err error) {
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("GetHostByName Panic occured -  " + fmt.Sprint(res))
		}
	}()
	klog.V(2).Infof("get host by name %s", hostName)
	uri := "api/rest/hosts"
	hosts := []Host{}
	queryParam := map[string]interface{}{"name": hostName}
	resp, err := c.getResponseWithQueryString(uri, queryParam, &hosts)
	if err != nil {
		klog.Errorf("host %s not found ", hostName)
		return host, err
	}
	if len(hosts) == 0 {
		apiresp := resp.(client.ApiResponse)
		hosts, _ = apiresp.Result.([]Host)
	}

	if len(hosts) > 0 {
		host = hosts[0]
	}
	if host.ID == 0 && host.Name == "" {
		return host, errors.New("HOST_NOT_FOUND")
	}
	klog.V(2).Infof("fetched host with name %s", host.Name)
	return host, nil
}

// GetFCPorts - get fc ports details
func (c *ClientService) GetFCPorts() (fcNodes []FCNode, err error) {
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("GetHostByName Panic occured -  " + fmt.Sprint(res))
		}
	}()
	klog.V(2).Infof("get fc ports")
	uri := "api/rest/components/nodes?fields=fc_ports"
	resp, err := c.getJSONResponse(http.MethodGet, uri, nil, &fcNodes)
	if err != nil {
		klog.Errorf("error occured while fetching fc_ports ")
		return fcNodes, err
	}
	if len(fcNodes) == 0 {
		apiresp := resp.(client.ApiResponse)
		fcNodes, _ = apiresp.Result.([]FCNode)
	}

	if len(fcNodes) == 0 {
		return fcNodes, errors.New("fc port not found")
	}
	klog.V(2).Infof("fetched fc ports successfully ")
	return fcNodes, nil
}

// UnMapVolumeFromHost - Remove mapping of volume with host
func (c *ClientService) UnMapVolumeFromHost(hostID, volumeID int) (err error) {
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("UnMapVolumeFromHost Panic occured -  " + fmt.Sprint(res))
		}
	}()
	klog.V(2).Infof("Remove mapping of volume %d from host %d", volumeID, hostID)
	uri := "api/rest/hosts/" + strconv.Itoa(hostID) + "/luns/volume_id/" + strconv.Itoa(volumeID) + "?approved=true"
	_, err = c.getJSONResponse(http.MethodDelete, uri, nil, nil)
	if err != nil {
		if !strings.Contains(err.Error(), "HOST_NOT_FOUND") && !strings.Contains(err.Error(), "VOLUME_NOT_FOUND") && !strings.Contains(err.Error(), "LUN_NOT_FOUND") {
			klog.Errorf("failed to unmap volume %d from host %d with error %v", volumeID, hostID, err)
		}
		return err
	}
	klog.V(2).Infof("successfully unmapped volume %d from host %d", volumeID, hostID)
	return nil
}

// MapVolumeToHost - Map volume with given volumeID to Host with given hostID
func (c *ClientService) MapVolumeToHost(hostID, volumeID, lun int) (luninfo LunInfo, err error) {
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("MapVolumeToHost Panic occured -  " + fmt.Sprint(res))
		}
	}()
	klog.V(2).Infof("map volume %d to host %d", volumeID, hostID)
	uri := "api/rest/hosts/" + strconv.Itoa(hostID) + "/luns?approved=true"
	data := make(map[string]interface{})
	data["volume_id"] = volumeID
	if lun != -1 {
		data["lun"] = lun
	}
	resp, err := c.getJSONResponse(http.MethodPost, uri, data, &luninfo)
	if err != nil {
		// ignore logging for following error code
		if !strings.Contains(err.Error(), "MAPPING_ALREADY_EXISTS") {
			klog.Errorf("error occured while mapping volume to host %v", err)
		}
		return luninfo, err
	}
	if luninfo == (LunInfo{}) {
		apiresp := resp.(client.ApiResponse)
		luninfo, _ = apiresp.Result.(LunInfo)
	}
	klog.V(2).Infof("Successfully mapped volume %d to host %d", volumeID, hostID)
	return luninfo, nil
}

// GetLunByHostVolume - Get Lun details for volume and host provided
func (c *ClientService) GetLunByHostVolume(hostID, volumeID int) (luninfo LunInfo, err error) {
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("GetLunByHostVolume Panic occured -  " + fmt.Sprint(res))
		}
	}()
	luns := []LunInfo{}
	klog.V(2).Infof("get lun for volume %d and host %d", volumeID, hostID)
	uri := "api/rest/hosts/" + strconv.Itoa(hostID) + "/luns"
	data := map[string]interface{}{"volume_id": volumeID}
	resp, err := c.getResponseWithQueryString(uri, data, &luns)
	if err != nil {
		klog.Errorf("error occured while get luns for volumeID %d and host %d err %v", volumeID, hostID, err)
		return luninfo, err
	}
	if len(luns) == 0 {
		apiresp := resp.(client.ApiResponse)
		luns, _ = apiresp.Result.([]LunInfo)
	}
	if len(luns) > 0 {
		luninfo = luns[0]
	}
	klog.V(2).Infof("got %d lun for volume %d and host %d", luninfo.Lun, volumeID, hostID)
	return luninfo, nil
}

// GetAllLunByHost - Get all luns for host id provided
func (c *ClientService) GetAllLunByHost(hostID int) (luninfo []LunInfo, err error) {
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("GetLunByHostVolume Panic occured -  " + fmt.Sprint(res))
		}
	}()
	klog.V(2).Infof("Get all lun for host %d", hostID)
	uri := "api/rest/hosts/" + strconv.Itoa(hostID) + "/luns"
	resp, err := c.getResponseWithQueryString(uri, nil, &luninfo)
	if err != nil {
		klog.Errorf("failed to get luns for host %d with error %v", hostID, err)
		return luninfo, err
	}
	if len(luninfo) == 0 {
		apiresp := resp.(client.ApiResponse)
		luninfo, _ = apiresp.Result.([]LunInfo)
	}
	klog.V(2).Infof("got %d Luns for host %d", len(luninfo), hostID)
	return luninfo, nil
}

// GetVolumeSnapshotByParentID method return true is the filesystemID has child else false
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
		klog.Errorf("failed to check GetVolumeSnapshotByParentID %v", err)
		return &volumes, err
	}
	if len(volumes) == 0 {
		apiresp := resp.(client.ApiResponse)
		volumes, _ = apiresp.Result.([]Volume)
	}
	return &volumes, err
}

// UpdateVolume : update volume
func (c *ClientService) UpdateVolume(volumeID int, volume Volume) (*Volume, error) {
	var err error
	defer func() {
		if res := recover(); res != nil && err == nil {
			err = errors.New("UpdateVolume Panic occured -  " + fmt.Sprint(res))
		}
	}()
	klog.V(2).Infof("Update volume %d", volumeID)
	uri := "api/rest/volumes/" + strconv.Itoa(volumeID)
	volumeResp := Volume{}

	resp, err := c.getJSONResponse(http.MethodPut, uri, volume, &volumeResp)
	if err != nil {
		klog.Errorf("Error occured while updating volume : %s", err)
		return nil, err
	}

	if volumeResp == (Volume{}) {
		apiresp := resp.(client.ApiResponse)
		volumeResp, _ = apiresp.Result.(Volume)
	}
	klog.V(2).Infof("Updated volume: %d", volumeID)
	return &volumeResp, nil
}

// **************************************************Util Methods*********************************************
//                                   generic methods to do reset called
//                                   consume by other method intent to do rese calls
// **************************************************Util Methods*********************************************
func (c *ClientService) getJSONResponse(method, apiuri string, body, expectedResp interface{}) (resp interface{}, err error) {
	klog.V(2).Infof("Request made for method: %s and apiuri %s", method, apiuri)
	defer func() {
		if res := recover(); res != nil && err == nil {
			klog.Errorf("Error in getJSONResponse while makeing %s request on %s url error : %v ", method, apiuri, err)
			err = errors.New("error in getJSONResponse " + fmt.Sprint(res))
		}
	}()
	hostsecret, err := c.getAPIConfig()
	if err != nil {
		klog.Errorf("Error occured: %v ", err)
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
		klog.Errorf("An API JSON response error occured, URL: %s, error: %+v ", apiuri, err)
		return
	}
	return
}

func (c *ClientService) getResponseWithQueryString(apiuri string, queryParam map[string]interface{}, expectedResp interface{}) (resp interface{}, err error) {
	klog.V(2).Infof("Request made for apiuri %s", apiuri)
	defer func() {
		if res := recover(); res != nil && err == nil {
			klog.Errorf("Error in getResponseWithQueryString while making request on %s url error : %v ", apiuri, err)
			err = errors.New("error in getResponseWithQueryString " + fmt.Sprint(res))
		}
	}()
	hostsecret, err := c.getAPIConfig()
	if err != nil {
		klog.Errorf("Error occured: %v ", err)
		return nil, err
	}

	var queryString string
	for key, val := range queryParam {
		if queryString != "" {
			queryString += ","
		}
		queryString += key + "=" + fmt.Sprintf("%v", val)
	}
	resp, err = c.api.GetWithQueryString(context.Background(), apiuri, hostsecret, queryString, expectedResp)
	return resp, err
}

func (c *ClientService) getAPIConfig() (hostconfig client.HostConfig, err error) {
	defer func() {
		if res := recover(); res != nil && err == nil {
			klog.Errorf("error in getAPIConfig: %s", err)
			err = errors.New("error in getAPIConfig " + fmt.Sprint(res))
		}
	}()
	if c.SecretsMap == nil {
		return hostconfig, errors.New("Secret not found")
	}
	if c.SecretsMap["hostname"] != "" && c.SecretsMap["username"] != "" && c.SecretsMap["password"] != "" {
		hosturl, err := url.ParseRequestURI(c.SecretsMap["hostname"])
		if err != nil {
			hostconfig.ApiHost = "https://" + c.SecretsMap["hostname"] + "/"
		} else {
			hostconfig.ApiHost = hosturl.String()
		}
		klog.V(2).Infof("setting url to %s", hostconfig.ApiHost)
		hostconfig.UserName = c.SecretsMap["username"]
		hostconfig.Password = c.SecretsMap["password"]
		return hostconfig, nil
	}
	return hostconfig, errors.New("host configuration is not valid")
}
