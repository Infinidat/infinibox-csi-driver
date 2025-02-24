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
	"infinibox-csi-driver/iboxapi"
	"net/http"
	"net/url"

	"github.com/go-logr/logr"
	"github.com/go-logr/zerologr"
)

// Client interface
type Client interface {
	NewClient() (*ClientService, error)

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
