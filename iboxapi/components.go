package iboxapi

/*
Copyright 2025 Infinidat
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
import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type FCPort struct {
	PortID       int    `json:"id"`
	State        string `json:"state,omitempty"`
	Enabled      bool   `json:"enabled,omitempty"`
	WWNn         string `json:"wwnn,omitempty"`
	WWPn         string `json:"wwpn,omitempty"`
	SwitchWWNn   string `json:"switch_wwnn,omitempty"`
	Vendor       string `json:"vendor,omitempty"`
	SwitchVendor string `json:"switch_vendor,omitempty"`
}

type FCNode struct {
	Ports []FCPort `json:"fc_ports,omitempty"`
}

type GetFCPortsResponse struct {
	Result   []FCNode `json:"result"`
	Error    Error    `json:"error"`
	Metadata Metadata `json:"metadata"`
}

func (iboxClient *IboxClient) GetFCPorts() (nodes []FCNode, err error) {
	url := iboxClient.Creds.Url + "api/rest/components/nodes"
	iboxClient.Log.V(TRACE_LEVEL).Info("GetFCPorts", "URL", url)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nodes, fmt.Errorf("GetFCPorts - NewRequest - error %w", err)
	}

	values := req.URL.Query()
	values.Add("fields", "fc_ports")
	req.URL.RawQuery = values.Encode()

	SetAuthHeader(req, iboxClient.Creds)

	resp, err := iboxClient.HttpClient.Do(req)
	if err != nil {
		return nodes, fmt.Errorf("GetFCPorts - Do - error %w", err)
	}
	defer resp.Body.Close()
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nodes, fmt.Errorf("GetFCPorts - ReadAll - error %w", err)
	}
	var responseObject GetFCPortsResponse
	err = json.Unmarshal(bodyBytes, &responseObject)
	if err != nil {
		return nodes, fmt.Errorf("GetFCPorts - Unmarshal - error %w", err)
	}
	if responseObject.Error.Code != "" {
		return nodes, fmt.Errorf("GetFCPorts - ibox API - error code: %s message: %s", responseObject.Error.Code, responseObject.Error.Message)
	}

	return responseObject.Result, nil
}
