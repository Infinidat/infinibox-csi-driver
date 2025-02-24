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

func (iboxClient *IboxClient) GetMaxFileSystems() (cnt int, err error) {
	type ParameterResult struct {
		Result struct {
			NasMaxFilesystemsInSystem int `json:"nas.max_filesystems_in_system"`
		} `json:"result"`
		Error    interface{} `json:"error"`
		Metadata struct {
			Ready bool `json:"ready"`
		} `json:"metadata"`
	}
	url := fmt.Sprintf("%sapi/rest/config/limits?fields=nas.max_filesystems_in_system", iboxClient.Creds.Url)
	iboxClient.Log.V(TRACE_LEVEL).Info("GetMaxFileSystems", "URL", url)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return 0, fmt.Errorf("GetMaxFileSystems - NewRequest - error %w", err)
	}
	SetAuthHeader(req, iboxClient.Creds)

	resp, err := iboxClient.HttpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("GetMaxFileSystems - Do - error %w", err)
	}
	defer resp.Body.Close()
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("GetMaxFileSystems - ReadAll - error %w", err)
	}
	var responseObject ParameterResult
	err = json.Unmarshal(bodyBytes, &responseObject)
	if err != nil {
		return 0, fmt.Errorf("GetMaxFileSystems - Unmarshal - error %w", err)
	}
	return responseObject.Result.NasMaxFilesystemsInSystem, nil
}

func (iboxClient *IboxClient) GetMaxTreeqPerFs() (cnt int, err error) {
	type ParameterResult struct {
		Result struct {
			NasTreeqMaxCountPerFilesystem int `json:"nas.treeq_max_count_per_filesystem"`
		} `json:"result"`
		Error    interface{} `json:"error"`
		Metadata struct {
			Ready bool `json:"ready"`
		} `json:"metadata"`
	}
	url := fmt.Sprintf("%sapi/rest/config/limits?fields=nas.treeq_max_count_per_filesystem", iboxClient.Creds.Url)
	iboxClient.Log.V(TRACE_LEVEL).Info("GetMaxTreeqPerFs", "URL", url)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return 0, fmt.Errorf("GetMaxTreeqPerFs - NewRequest - error %w", err)
	}
	SetAuthHeader(req, iboxClient.Creds)

	resp, err := iboxClient.HttpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("GetMaxTreeqPerFs - Do - error %w", err)
	}
	defer resp.Body.Close()
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("GetMaxTreeqPerFs - ReadAll - error %w", err)
	}
	var responseObject ParameterResult
	err = json.Unmarshal(bodyBytes, &responseObject)
	if err != nil {
		return 0, fmt.Errorf("GetMaxTreeqPerFs - Unmarshal - error %w", err)
	}
	return responseObject.Result.NasTreeqMaxCountPerFilesystem, nil
}
