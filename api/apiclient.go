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
	CreateSnapshotVolume(lockExpiresAt int64, snapshotParam *VolumeSnapshot) (*SnapshotVolumesResp, error)
	GetVolumeSnapshotByParentID(volumeID int) (*[]Volume, error)

	MapVolumeToHost(hostID, volumeID, lun int) (luninfo LunInfo, err error)
	GetLunByHostVolume(hostID, volumeID int) (luninfo LunInfo, err error)
	UnMapVolumeFromHost(hostID, volumeID int) (err error)

	// for consistency group (volume group)
	CreateCG(poolID int, cgName string) (CGInfo, error)
	AddMemberToSnapshotGroup(volumeID int, cgID int) error
	GetMembersByCGID(cgID int) ([]MemberInfo, error)
	GetCG(name string) (CGInfo, error)
	CreateSnapshotGroup(cgID int, snapName, snapPrefix, snapSuffix string) (CGInfo, error)

	// for nfs
	AddNodeInExport(exportID int, access string, noRootSquash bool, ip string) (*ExportResponse, error)
	DeleteNodeFromExport(exportID int, access string, noRootSquash bool, ip string) (*ExportResponse, error)
	CreateFileSystemSnapshot(lockedExpiresAt int64, snapshotParam *FileSystemSnapshot) (*FileSystemSnapshotResponse, error)
	DeleteFileSystemComplete(fileSystemID int) (err error)
	DeleteParentFileSystem(fileSystemID int) (err error)
	DeleteExportRule(fileSystemID int, ipAddress string) (err error)
	GetSnapshotByName(snapshotName string) (*[]FileSystemSnapshotResponse, error)

	GetFilesystemTreeqCount(fileSystemID int) (treeqCnt int, err error)
	CreateTreeq(filesystemID int, treeqParameter map[string]interface{}) (*Treeq, error)
	DeleteTreeq(fileSystemID, treeqID int) (*Treeq, error)
	GetTreeq(fileSystemID, treeqID int) (*Treeq, error)
	UpdateTreeq(fileSystemID, treeqID int, body map[string]interface{}) (*Treeq, error)
	GetTreeqSizeByFileSystemID(filesystemID int) (int64, error)
	GetTreeqByName(fileSystemID int, treeqName string) (*Treeq, error)

	// replication
	CreateReplica(request CreateReplicaRequest) (Replica, error)
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
