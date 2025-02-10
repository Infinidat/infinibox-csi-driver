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
	"context"
	"errors"
	"fmt"
	"infinibox-csi-driver/api/client"
	"infinibox-csi-driver/common"
	"infinibox-csi-driver/iboxapi"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-logr/logr"
	"github.com/go-logr/zerologr"
)

// Client interface
type Client interface {
	NewClient() (*ClientService, error)
	CreateVolume(volume *VolumeParam, storagePoolID int) (*Volume, error)
	FindStoragePool(id int, name string) (StoragePool, error)
	GetNtpStatus() ([]NtpStatus, error)
	GetStoragePool(poolID int, storagepool string) ([]StoragePool, error)
	CreateSnapshotVolume(lockExpiresAt int64, snapshotParam *VolumeSnapshot) (*SnapshotVolumesResp, error)
	GetNetworkSpaceByName(networkSpaceName string) (nspace NetworkSpace, err error)
	GetVolumeSnapshotByParentID(volumeID int) (*[]Volume, error)
	GetAllSnapshots() ([]Volume, error)
	GetAllVolumes() ([]Volume, error)

	GetAllHosts() (host []Host, err error)
	GetHostByName(hostName string) (host Host, err error)
	CreateHost(hostName string) (host Host, err error)
	AddHostPort(portType, portAddress string, hostID int) (hostPort HostPort, err error)
	AddHostSecurity(chapCreds map[string]string, hostID int) (host Host, err error)
	MapVolumeToHost(hostID, volumeID, lun int) (luninfo LunInfo, err error)
	GetLunByHostVolume(hostID, volumeID int) (luninfo LunInfo, err error)
	UnMapVolumeFromHost(hostID, volumeID int) (err error)
	GetFCPorts() (fcNodes []FCNode, err error)
	GetHostPort(hostID int, portAddress string) (hostPort HostPort, err error)
	GetLunByVolume(volumeID int) (luninfo []LunInfo, err error)

	// for consistency group (volume group)
	CreateCG(poolID int, cgName string) (CGInfo, error)
	AddMemberToSnapshotGroup(volumeID int, cgID int) error
	RemoveMemberFromSnapshotGroup(volumeID int, cgID int) error
	GetAllCG() ([]CGInfo, error)
	GetMembersByCGID(cgID int) ([]MemberInfo, error)
	GetCG(name string) (CGInfo, error)
	CreateSnapshotGroup(cgID int, snapName, snapPrefix, snapSuffix string) (CGInfo, error)

	// for nfs
	DeleteFileSystem(fileSystemID int) (*FileSystem, error)
	AddNodeInExport(exportID int, access string, noRootSquash bool, ip string) (*ExportResponse, error)
	DeleteNodeFromExport(exportID int, access string, noRootSquash bool, ip string) (*ExportResponse, error)
	CreateFileSystemSnapshot(lockedExpiresAt int64, snapshotParam *FileSystemSnapshot) (*FileSystemSnapshotResponse, error)
	DeleteFileSystemComplete(fileSystemID int) (err error)
	DeleteParentFileSystem(fileSystemID int) (err error)
	GetParentID(fileSystemID int) int
	FileSystemHasChild(fileSystemID int) bool
	DeleteExportRule(fileSystemID int, ipAddress string) (err error)
	UpdateFilesystem(fileSystemID int, fileSystem FileSystem) (*FileSystem, error)
	GetSnapshotByName(snapshotName string) (*[]FileSystemSnapshotResponse, error)
	RestoreFileSystemFromSnapShot(parentID, srcSnapShotID int) (bool, error)

	GetFilesystemTreeqCount(fileSystemID int) (treeqCnt int, err error)
	CreateTreeq(filesystemID int, treeqParameter map[string]interface{}) (*Treeq, error)
	DeleteTreeq(fileSystemID, treeqID int) (*Treeq, error)
	GetTreeq(fileSystemID, treeqID int) (*Treeq, error)
	UpdateTreeq(fileSystemID, treeqID int, body map[string]interface{}) (*Treeq, error)
	GetTreeqSizeByFileSystemID(filesystemID int) (int64, error)
	GetFileSystemCountByPoolID(poolID int) (int, error)
	GetMaxTreeqPerFs() (int, error)
	GetMaxFileSystems() (int, error)
	GetTreeqByName(fileSystemID int, treeqName string) (*Treeq, error)

	// replication
	CreateReplica(request CreateReplicaRequest) (Replica, error)
	GetLink(linkID int) (*Link, error)
	GetLinks() ([]Link, error)
}

// ClientService : struct having reference of rest client and will host methods which need rest operations
type ClientService struct {
	api        client.RestClient
	Iboxapi    *iboxapi.IboxClient
	SecretsMap map[string]string
	ConfigMap  map[string]string
}

// NewClient : Create New Client
func (c *ClientService) NewClient() (*ClientService, error) {
	zlog.Trace().Msg("NewClient Started")
	restclient, err := client.NewRestClient()
	if err != nil {
		return c, err
	}
	c.api = restclient

	// for setting up iboxapi
	hostconfig, err := c.getAPIConfig()
	if err != nil {
		return nil, err
	}
	creds := iboxapi.Credentials{
		Username: hostconfig.UserName,
		Password: hostconfig.Password,
		Url:      hostconfig.ApiHost,
	}
	var iboxApiLog logr.Logger = zerologr.New(&zlog)
	c.Iboxapi = iboxapi.NewIboxClient(iboxApiLog, creds)

	zlog.Trace().Msg("NewClient Finished")
	return c, nil
}

// AddHostSecurity - add chap security for host with given details
func (c *ClientService) AddHostSecurity(chapCreds map[string]string, hostID int) (host Host, err error) {
	zlog.Trace().Msgf("add chap atuhentication for hostID %d : ", hostID)
	uri := "api/rest/hosts/" + strconv.Itoa(hostID) + "?approved=true"
	_, err = c.getJSONResponse(http.MethodPut, uri, chapCreds, host)
	if err != nil {
		zlog.Error().Msgf("failed to add chap security to host %d with error %v", hostID, err)
		return host, err
	}
	zlog.Trace().Msgf("created chap authentication for host %s: ", host.Name)
	return host, nil
}

// AddHostPort - add port for host with given details
func (c *ClientService) AddHostPort(portType, portAddress string, hostID int) (hostPort HostPort, err error) {
	zlog.Trace().Msgf("add port for hostID %s %d : ", portAddress, hostID)
	uri := "api/rest/hosts/" + strconv.Itoa(hostID) + "/ports?approved=true"
	body := map[string]interface{}{"address": portAddress, "type": portType}
	_, err = c.getJSONResponse(http.MethodPost, uri, body, &hostPort)
	if err != nil {
		if strings.Contains(err.Error(), "PORT_ALREADY_BELONGS_TO_HOST") {
			zlog.Trace().Msgf("Success: No need to add port '%s' to host with ID %d, port already belongs to host", portAddress, hostID)
			return HostPort{}, nil
		} else {
			zlog.Error().Msgf("error adding port '%s' to host with ID %d, error: %+v", portAddress, hostID, err)
			return hostPort, err
		}
	}

	zlog.Trace().Msgf("created host port: %s", hostPort.PortAddress)
	return hostPort, nil
}

// CreateVolume : create volume with volume details provided in storage pool provided
func (c *ClientService) CreateVolume(volume *VolumeParam, storagePoolID int) (*Volume, error) {
	path := "/api/rest/volumes"
	zlog.Trace().Msgf("Creating volume in storage pool ID %d of size %d bytes", storagePoolID, volume.VolumeSize)

	volume.PoolId = storagePoolID
	volumeParameter := make(map[string]interface{})
	volumeParameter["pool_id"] = volume.PoolId
	volumeParameter["size"] = volume.VolumeSize
	volumeParameter["name"] = volume.Name
	volumeParameter["provtype"] = volume.ProvisionType
	volumeParameter[common.SC_SSD_ENABLED] = volume.SsdEnabled
	vol := Volume{}
	resp, err := c.getJSONResponse(http.MethodPost, path, volumeParameter, &vol)
	if err != nil {
		return nil, err
	}
	if (Volume{}) == vol {
		apiresp := resp.(client.ApiResponse)
		vol, _ = apiresp.Result.(Volume)
	}
	zlog.Trace().Msgf("Created Volume with ID %d", vol.ID)
	return &vol, nil
}

// FindStoragePool : Find storage pool either by id or name
func (c *ClientService) FindStoragePool(id int, name string) (StoragePool, error) {
	zlog.Trace().Msgf("FindStoragePool called with either id %d or name %s", id, name)
	storagePools, err := c.GetStoragePool(id, name)
	if err != nil {
		return StoragePool{}, fmt.Errorf("error getting storage pool %s", err)
	}

	for _, storagePool := range storagePools {
		if storagePool.ID == id || storagePool.Name == name {
			zlog.Trace().Msgf("Got storage pool: %s", storagePool.Name)
			return storagePool, nil
		}
	}
	return StoragePool{}, errors.New("couldn't find storage pool")
}

// GetStoragePool : Get storage pool(s) either by id or name
func (c *ClientService) GetStoragePool(poolID int, storagepoolname string) ([]StoragePool, error) {
	zlog.Trace().Msgf("GetStoragePool called with either id %d or name %s", poolID, storagepoolname)
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
		_, err := c.getResponseWithQueryString("api/rest/pools", queryParam, &storagePool)
		if err != nil {
			return nil, err
		}
	}

	if storagepoolname != "" {
		storagePools = append(storagePools, storagePool)
	}
	return storagePools, nil
}

// CreateSnapshotVolume : Create volume from snapshot
func (c *ClientService) CreateSnapshotVolume(lockExpiresAt int64, snapshotParam *VolumeSnapshot) (*SnapshotVolumesResp, error) {
	zlog.Trace().Msgf("Create a snapshot: %s", snapshotParam.SnapshotName)
	path := "/api/rest/volumes"
	snapResp := SnapshotVolumesResp{}
	parameters := make(map[string]interface{})
	parameters["parent_id"] = snapshotParam.ParentID
	parameters["name"] = snapshotParam.SnapshotName
	parameters["write_protected"] = snapshotParam.WriteProtected
	parameters[common.SC_SSD_ENABLED] = snapshotParam.SsdEnabled
	if lockExpiresAt > 0 {
		path = path + "?approved=true"
		parameters["lock_expires_at"] = lockExpiresAt
	}

	_, err := c.getJSONResponse(http.MethodPost, path, parameters, &snapResp)
	if err != nil {
		return nil, err
	}
	zlog.Trace().Msgf("Created snapshot: %s", snapResp.Name)
	return &snapResp, nil
}

// GetNetworkSpaceByName - Get networkspace by name
func (c *ClientService) GetNetworkSpaceByName(networkSpaceName string) (nspace NetworkSpace, err error) {
	zlog.Trace().Msgf("Get network space by name: %s", networkSpaceName)
	netspaces := []NetworkSpace{}
	path := "api/rest/network/spaces"
	queryParam := map[string]interface{}{"name": networkSpaceName}
	resp, err := c.getResponseWithQueryString(path, queryParam, &netspaces)

	if err != nil {
		zlog.Error().Msgf("unexpected error retrieving network space: %s", networkSpaceName)
		return nspace, err
	}
	if len(netspaces) == 0 {
		apiresp := resp.(client.ApiResponse)
		netspaces, _ = apiresp.Result.([]NetworkSpace)
		return nspace, fmt.Errorf("no such network space: %s", networkSpaceName)
	}

	if len(netspaces) > 0 {
		nspace = netspaces[0]
	}
	zlog.Trace().Msgf("Got network space: %s", networkSpaceName)
	return nspace, nil
}

// CreateHost - create host  with given details
func (c *ClientService) CreateHost(hostName string) (host Host, err error) {
	zlog.Trace().Msgf("create host with name %s", hostName)
	uri := "api/rest/hosts"
	body := map[string]interface{}{"name": hostName}
	_, err = c.getJSONResponse(http.MethodPost, uri, body, &host)
	if err != nil {
		zlog.Error().Msgf("error creating host : %s error : %v", hostName, err)
		return host, err
	}

	zlog.Trace().Msgf("created host with name %s ID %d", host.Name, host.ID)
	return host, nil
}

// GetHostPort - get host port details
func (c *ClientService) GetHostPort(hostID int, portAddress string) (hostPort HostPort, err error) {
	zlog.Trace().Msgf("get host port by port address %s", portAddress)
	uri := "api/rest/hosts/" + strconv.Itoa(hostID) + "/ports"
	hostPorts := []HostPort{}
	resp, err := c.getJSONResponse(http.MethodGet, uri, nil, &hostPorts)
	if err != nil {
		zlog.Error().Msgf("unable to get host port %s with error ", portAddress)
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
	zlog.Trace().Msgf("fetched hostPort with address %s", hostPort.PortAddress)
	return hostPort, nil
}

// GetHostByName - get host details for given hostname
func (c *ClientService) GetHostByName(hostName string) (host Host, err error) {
	zlog.Trace().Msgf("get host by name %s", hostName)
	uri := "api/rest/hosts"
	hosts := []Host{}
	queryParam := map[string]interface{}{"name": hostName}
	resp, err := c.getResponseWithQueryString(uri, queryParam, &hosts)
	if err != nil {
		zlog.Error().Msgf("host %s not found ", hostName)
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
	zlog.Trace().Msgf("fetched host with name %s", host.Name)
	return host, nil
}

// GetFCPorts - get fc ports details
func (c *ClientService) GetFCPorts() (fcNodes []FCNode, err error) {
	zlog.Trace().Msgf("get fc ports")
	uri := "api/rest/components/nodes?fields=fc_ports"
	resp, err := c.getJSONResponse(http.MethodGet, uri, nil, &fcNodes)
	if err != nil {
		zlog.Error().Msgf("error occured while fetching fc_ports ")
		return fcNodes, err
	}
	if len(fcNodes) == 0 {
		apiresp := resp.(client.ApiResponse)
		fcNodes, _ = apiresp.Result.([]FCNode)
	}

	if len(fcNodes) == 0 {
		return fcNodes, errors.New("fc port not found")
	}
	zlog.Trace().Msgf("fetched fc ports successfully ")
	return fcNodes, nil
}

// UnMapVolumeFromHost - Remove mapping of volume with host
func (c *ClientService) UnMapVolumeFromHost(hostID, volumeID int) (err error) {
	zlog.Trace().Msgf("Remove mapping of volume %d from host %d", volumeID, hostID)
	uri := "api/rest/hosts/" + strconv.Itoa(hostID) + "/luns/volume_id/" + strconv.Itoa(volumeID) + "?approved=true"
	_, err = c.getJSONResponse(http.MethodDelete, uri, nil, nil)
	if err != nil {
		if !strings.Contains(err.Error(), "HOST_NOT_FOUND") && !strings.Contains(err.Error(), "VOLUME_NOT_FOUND") && !strings.Contains(err.Error(), "LUN_NOT_FOUND") {
			zlog.Error().Msgf("failed to unmap volume %d from host %d with error %v", volumeID, hostID, err)
		}
		return err
	}
	zlog.Trace().Msgf("successfully unmapped volume %d from host %d", volumeID, hostID)
	return nil
}

// MapVolumeToHost - Map volume with given volumeID to Host with given hostID
func (c *ClientService) MapVolumeToHost(hostID, volumeID, lun int) (luninfo LunInfo, err error) {
	zlog.Trace().Msgf("map volume %d to host %d", volumeID, hostID)
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
			zlog.Error().Msgf("error occured while mapping volume to host %v", err)
		}
		return luninfo, err
	}
	if luninfo == (LunInfo{}) {
		apiresp := resp.(client.ApiResponse)
		luninfo, _ = apiresp.Result.(LunInfo)
	}
	zlog.Trace().Msgf("Successfully mapped volume %d to host %d", volumeID, hostID)
	return luninfo, nil
}

// GetLunByHostVolume - Get Lun details for volume and host provided
func (c *ClientService) GetLunByHostVolume(hostID, volumeID int) (luninfo LunInfo, err error) {
	luns := []LunInfo{}
	zlog.Trace().Msgf("get lun for volume %d and host %d", volumeID, hostID)
	uri := "api/rest/hosts/" + strconv.Itoa(hostID) + "/luns"
	data := map[string]interface{}{"volume_id": volumeID}
	resp, err := c.getResponseWithQueryString(uri, data, &luns)
	if err != nil {
		zlog.Error().Msgf("error occured while get luns for volumeID %d and host %d err %v", volumeID, hostID, err)
		return luninfo, err
	}
	if len(luns) == 0 {
		apiresp := resp.(client.ApiResponse)
		luns, _ = apiresp.Result.([]LunInfo)
	}
	if len(luns) > 0 {
		luninfo = luns[0]
	}
	zlog.Trace().Msgf("got %d lun for volume %d and host %d", luninfo.Lun, volumeID, hostID)
	return luninfo, nil
}

// GetLunByVolume - Get all luns for volume id provided
func (c *ClientService) GetLunByVolume(volumeID int) (luninfo []LunInfo, err error) {

	page := 1
	page_size := common.IBOX_DEFAULT_QUERY_PAGE_SIZE

	zlog.Trace().Msgf("Get luns for volume %d", volumeID)

	uri := "api/rest/volumes/" + strconv.Itoa(volumeID) + "/luns" + "?page_size=" + strconv.Itoa(page_size) + "&page=" + strconv.Itoa(page)

	resp, err := c.getResponseWithQueryString(uri, nil, &luninfo)

	if err != nil {
		zlog.Error().Msgf("failed to get luns for volume %d with error %v", volumeID, err)
		return luninfo, err
	}

	apiresp := resp.(client.ApiResponse)
	currentResults, _ := apiresp.Result.([]LunInfo)
	luninfo = append(luninfo, currentResults...)
	responseSize := apiresp.MetaData.NoOfObject
	zlog.Trace().Msgf("added %d items to results", responseSize)

	zlog.Trace().Msgf("got %d Luns for host %d", len(luninfo), volumeID)
	return luninfo, nil
}

// GetVolumeSnapshotByParentID method return true is the filesystemID has child else false
func (c *ClientService) GetVolumeSnapshotByParentID(volumeID int) (*[]Volume, error) {
	voluri := "/api/rest/volumes/"
	volumes := []Volume{}
	queryParam := make(map[string]interface{})
	queryParam["parent_id"] = volumeID
	resp, err := c.getResponseWithQueryString(voluri, queryParam, &volumes)
	if err != nil {
		zlog.Error().Msgf("failed to check GetVolumeSnapshotByParentID %v", err)
		return &volumes, err
	}
	if len(volumes) == 0 {
		apiresp := resp.(client.ApiResponse)
		volumes, _ = apiresp.Result.([]Volume)
	}
	return &volumes, err
}

func (c *ClientService) getJSONResponse(method, apiuri string, body, expectedResp interface{}) (resp interface{}, err error) {
	hostsecret, err := c.getAPIConfig()
	if err != nil {
		zlog.Error().Msgf("error occured: %v ", err)
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
		zlog.Error().Msgf("api json response error occured, method: %s URL: %s, error: %+v", hostsecret.ApiHost, apiuri, err)
		return
	}
	zlog.Trace().Msgf("Requesting method: %s , %s%s successful", method, hostsecret.ApiHost, apiuri)
	return
}

func (c *ClientService) getResponseWithQueryString(apiuri string, queryParam map[string]interface{}, expectedResp interface{}) (resp interface{}, err error) {
	hostsecret, err := c.getAPIConfig()
	if err != nil {
		zlog.Error().Msgf("error occured: %v ", err)
		return nil, err
	}
	zlog.Trace().Msgf("Requesting %s%s", hostsecret.ApiHost, apiuri)

	var queryString string
	for key, val := range queryParam {
		if queryString != "" {
			queryString += "&"
		}
		queryString += key + "=" + fmt.Sprintf("%v", val)
	}
	resp, err = c.api.GetWithQueryString(context.Background(), apiuri, hostsecret, queryString, expectedResp)
	return resp, err
}

func (c *ClientService) getAPIConfig() (hostconfig client.HostConfig, err error) {
	if c.SecretsMap == nil {
		return hostconfig, errors.New("secret not found")
	}
	if c.SecretsMap["hostname"] != "" && c.SecretsMap["username"] != "" && c.SecretsMap["password"] != "" {

		hostnameURL, err := url.Parse(c.SecretsMap["hostname"])

		if err != nil {
			zlog.Error().Msgf("Error parsing IBox hostname: %s", err.Error())

		}

		// check for scheme, add if missing.
		urlScheme := hostnameURL.Scheme

		if urlScheme == "" {
			zlog.Trace().Msgf("IBox Hostname is missing scheme, setting https as scheme")
			hostconfig.ApiHost = "https://" + c.SecretsMap["hostname"] + "/"
		} else {
			hostconfig.ApiHost = hostnameURL.String()
		}

		// check for URI validity.
		hostnameURL, err = url.ParseRequestURI(hostconfig.ApiHost)
		if err != nil {
			zlog.Error().Msgf("IBox hostname %s is invalid URI: %s", hostnameURL.String(), err.Error())
		} else {
			zlog.Trace().Msgf("IBox URL: %s", hostconfig.ApiHost)
		}

		//zlog.Trace().Msgf("setting url to %s", hostconfig.ApiHost)
		hostconfig.UserName = c.SecretsMap["username"]
		hostconfig.Password = c.SecretsMap["password"]
		return hostconfig, nil
	}
	return hostconfig, errors.New("host configuration is not valid")
}

// GetAllSnapshots method returns all snapshots for volumes and datasets
func (c *ClientService) GetAllSnapshots() ([]Volume, error) {
	var err error
	uriList := []string{
		"/api/rest/datasets",
		//"/api/rest/volumes",
	}
	allvolumes := make([]Volume, 0)

	for u := 0; u < len(uriList); u++ {
		queryParam := make(map[string]interface{})
		queryParam["type"] = "SNAPSHOT"
		page := 1
		total_pages := 1 // start with 1, update after first query.
		for ok := true; ok; ok = page <= total_pages {
			queryParam["page"] = strconv.Itoa(page)
			volumes := []Volume{}
			resp, err := c.getResponseWithQueryString(uriList[u], queryParam, &volumes)
			if err != nil {
				zlog.Error().Msgf("failed to check GetAllSnapshots %v response: %v", err, resp)
				return allvolumes, err
			}
			apiresp := resp.(client.ApiResponse)
			zlog.Trace().Msgf("uri %s page %d volumes %d", uriList[u], page, len(volumes))

			allvolumes = append(allvolumes, volumes...)
			if page == 1 {
				total_pages = apiresp.MetaData.TotalPages
			}
			zlog.Trace().Msgf("total pages %d\n", total_pages)
			page++
		}
	}

	return allvolumes, err
}

// GetAllVolumes method returns all volumes
func (c *ClientService) GetAllVolumes() ([]Volume, error) {
	var err error
	uriList := []string{
		"/api/rest/datasets",
		"/api/rest/volumes",
	}
	allvolumes := make([]Volume, 0)

	for u := 0; u < len(uriList); u++ {
		queryParam := make(map[string]interface{})
		queryParam["type"] = "MASTER"
		page := 1
		total_pages := 1 // start with 1, update after first query.
		for ok := true; ok; ok = page <= total_pages {
			queryParam["page"] = strconv.Itoa(page)
			volumes := []Volume{}
			resp, err := c.getResponseWithQueryString(uriList[u], queryParam, &volumes)
			if err != nil {
				zlog.Error().Msgf("failed to check GetAllVolumes %v response: %v", err, resp)
				return allvolumes, err
			}
			apiresp := resp.(client.ApiResponse)
			zlog.Trace().Msgf("uri %s page %d volumes %d", uriList[u], page, len(volumes))

			allvolumes = append(allvolumes, volumes...)
			if page == 1 {
				total_pages = apiresp.MetaData.TotalPages
			}
			zlog.Trace().Msgf("total pages %d\n", total_pages)
			page++
		}
	}

	return allvolumes, err
}

// GetNtpStatus
func (c *ClientService) GetNtpStatus() ([]NtpStatus, error) {
	var err error
	uriList := []string{
		"/api/rest/system/ntp_status",
	}
	allNtpStatus := make([]NtpStatus, 0)

	for u := 0; u < len(uriList); u++ {
		queryParam := make(map[string]interface{})
		page := 1
		total_pages := 1 // start with 1, update after first query.
		for ok := true; ok; ok = page <= total_pages {
			queryParam["page"] = strconv.Itoa(page)
			ntpStats := []NtpStatus{}
			resp, err := c.getResponseWithQueryString(uriList[u], queryParam, &ntpStats)
			if err != nil {
				zlog.Error().Msgf("failed to check GetNtpStatus %v response: %v", err, resp)
				return allNtpStatus, err
			}
			apiresp := resp.(client.ApiResponse)
			zlog.Trace().Msgf("uri %s page %d volumes %d", uriList[u], page, len(ntpStats))

			allNtpStatus = append(allNtpStatus, ntpStats...)
			if page == 1 {
				total_pages = apiresp.MetaData.TotalPages
			}
			zlog.Trace().Msgf("total pages %d\n", total_pages)
			page++
		}
	}

	return allNtpStatus, err
}

// GetAllHosts - get all host details
func (c *ClientService) GetAllHosts() ([]Host, error) {
	zlog.Trace().Msgf("get all hosts ")
	uri := "api/rest/hosts"
	hosts := []Host{}
	//queryParam := map[string]interface{}{}
	resp, err := c.getResponseWithQueryString(uri, nil, &hosts)
	if err != nil {
		zlog.Error().Msgf("hosts  not found ")
		return hosts, err
	}
	if len(hosts) == 0 {
		apiresp := resp.(client.ApiResponse)
		hosts, _ = apiresp.Result.([]Host)
	}

	zlog.Trace().Msgf("fetched hosts len %d", len(hosts))
	return hosts, nil
}

func (c *ClientService) CreateCustomEvent(request CustomEventRequest) error {

	path := "/api/rest/events/custom"
	eventResult := CustomEvent{}
	_, err := c.getJSONResponse(http.MethodPost, path, request, &eventResult)
	if err != nil {
		return err
	}
	zlog.Debug().Msgf("Created CustomEvent with ID %d", eventResult.ID)

	return nil
}

/**
func (c *ClientService) CreateEvent(request EventRequest) error {

	path := "/api/rest/events"
	eventResult := EventResponse{}
	_, err := c.getJSONResponse(http.MethodPost, path, request, &eventResult)
	if err != nil {
		return err
	}
	zlog.Debug().Msgf("Created Event with response %+v", eventResult)
	if eventResult.Error.Code != "" {
		zlog.Error().Msgf("Created Event with error code %s", eventResult.Error.Code)
	}

	return nil
}
*/
