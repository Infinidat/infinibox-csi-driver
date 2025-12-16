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
	"log/slog"
	"net/url"

	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/infinidat/infinibox-csi-driver/iboxapi"
)

// Client interface
type Client interface {
	NewClient() (*ClientService, error)

	// for nfs
	AddNodeInExport(ctx context.Context, exportID int, access string, noRootSquash bool, ip string) (*iboxapi.Export, error)
	DeleteNodeFromExport(ctx context.Context, export iboxapi.Export, noRootSquash bool, ip string) (*iboxapi.Export, error)
	DeleteFileSystemComplete(ctx context.Context, fileSystemID int) (err error)
	DeleteParentFileSystem(ctx context.Context, fileSystemID int) (err error)
	DeleteExportRule(ctx context.Context, fileSystemID int, ipAddress string) (err error)
}

// ClientService : struct having reference of rest client and will host methods which need rest operations
type ClientService struct {
	IboxAPI    *iboxapi.IboxClient
	SecretsMap map[string]string
	ConfigMap  map[string]string
}
type HostConfig struct {
	APIHost  string
	UserName string
	Password string
}

// NewClient : Create New Client
func (c *ClientService) NewClient() (*ClientService, error) {
	ctx := context.Background()
	slog.Log(ctx, common.LevelTrace, "NewClient Started")

	// for setting up iboxapi
	hostConfig, err := c.getAPIConfig()
	if err != nil {
		return nil, err
	}
	creds := iboxapi.Credentials{
		Username: hostConfig.UserName,
		Password: hostConfig.Password,
		URL:      hostConfig.APIHost,
	}

	c.IboxAPI = iboxapi.NewIboxClient(creds)

	slog.Log(ctx, common.LevelTrace, "NewClient Finished")
	return c, nil
}

func (c *ClientService) getAPIConfig() (hostConfig HostConfig, err error) {
	ctx := context.Background()
	if c.SecretsMap == nil {
		return hostConfig, errors.New("secret not found")
	}
	if c.SecretsMap[common.CredentialHostname] != "" && c.SecretsMap[common.CredentialUsername] != "" && c.SecretsMap[common.CredentialPassword] != "" {
		hostnameURL, err := url.Parse(c.SecretsMap[common.CredentialHostname])

		if err != nil {
			slog.Error("error parsing ibox hostname", "error", err.Error())
		}

		// check for scheme, add if missing.
		urlScheme := hostnameURL.Scheme

		if urlScheme == "" {
			slog.Log(ctx, common.LevelTrace, "ibox hostname is missing scheme, setting https as scheme")
			hostConfig.APIHost = "https://" + c.SecretsMap[common.CredentialHostname] + "/"
		} else {
			hostConfig.APIHost = hostnameURL.String()
		}

		// check for URI validity.
		hostnameURL, err = url.ParseRequestURI(hostConfig.APIHost)
		if err != nil {
			slog.Error("ibox hostname is invalid", "URI", hostnameURL.String(), "error", err.Error())
		} else {
			slog.Log(ctx, common.LevelTrace, "info", "IBox URL", hostConfig.APIHost)
		}

		hostConfig.UserName = c.SecretsMap[common.CredentialUsername]
		hostConfig.Password = c.SecretsMap[common.CredentialPassword]
		return hostConfig, nil
	}
	return hostConfig, errors.New("host configuration is not valid")
}
