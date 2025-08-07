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
	"github.com/infinidat/infinibox-csi-driver/iboxapi"
	"net/url"

	"github.com/go-logr/zerologr"
)

// Client interface
type Client interface {
	NewClient() (*ClientService, error)

	// for nfs
	AddNodeInExport(exportID int, access string, noRootSquash bool, ip string) (*iboxapi.Export, error)
	DeleteNodeFromExport(export iboxapi.Export, access string, noRootSquash bool, ip string) (*iboxapi.Export, error)
	DeleteFileSystemComplete(fileSystemID int) (err error)
	DeleteParentFileSystem(fileSystemID int) (err error)
	DeleteExportRule(fileSystemID int, ipAddress string) (err error)
}

// ClientService : struct having reference of rest client and will host methods which need rest operations
type ClientService struct {
	Iboxapi    *iboxapi.IboxClient
	SecretsMap map[string]string
	ConfigMap  map[string]string
}
type HostConfig struct {
	ApiHost  string
	UserName string
	Password string
}

// NewClient : Create New Client
func (c *ClientService) NewClient() (*ClientService, error) {
	zlog.Trace().Msg("NewClient Started")

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
	var iboxApiLog = zerologr.New(&zlog)
	c.Iboxapi = iboxapi.NewIboxClient(iboxApiLog, creds)

	zlog.Trace().Msg("NewClient Finished")
	return c, nil
}

func (c *ClientService) getAPIConfig() (hostconfig HostConfig, err error) {
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
