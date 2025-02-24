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
	"bytes"
	"encoding/json"
	"fmt"
	"infinibox-csi-driver/common"
	"io"
	"net/http"
	"strconv"
)

type Permissions struct {
	Access       string `json:"access,omitempty"`
	NoRootSquash bool   `json:"no_root_squash,omitempty"`
	Client       string `json:"client,omitempty"`
}

type Export struct {
	InnerPath             string        `json:"inner_path,omitempty"`
	PrefWrite             int           `json:"pref_write,omitempty"`
	BitFileID             bool          `json:"32bit_file_id,omitempty"`
	PrefRead              int           `json:"pref_read,omitempty"`
	MaxRead               int           `json:"max_read,omitempty"`
	Permissions           []Permissions `json:"permissions,omitempty"`
	TenantId              int           `json:"tenant_id,omitempty"`
	CreatedAt             int           `json:"created_at,omitempty"`
	PrefReaddir           int           `json:"pref_readdir,omitempty"`
	Enabled               bool          `json:"enabled,omitempty"`
	UpdatedAt             int           `json:"updated_at,omitempty"`
	MakeAllUsersAnonymous bool          `json:"make_all_users_anonymous,omitempty"`
	SnapdirVisible        bool          `json:"snapdir_visible,omitempty"`
	TransportProtocols    string        `json:"transport_protocols,omitempty"`
	AnonymousGid          int           `json:"anonymous_gid,omitempty"`
	AnonymousUid          int           `json:"anonymous_uid,omitempty"`
	FilesystemId          int           `json:"filesystem_id,omitempty"`
	MaxWrite              int           `json:"max_write,omitempty"`
	PrivilegedPort        bool          `json:"privileged_port,omitempty"`
	ID                    int           `json:"id,omitempty"`
	ExportPath            string        `json:"export_path,omitempty"`
}

type GetExportByIDResponse struct {
	Metadata Metadata `json:"metadata"`
	Result   Export   `json:"result"`
	Error    Error    `json:"error"`
}

type DeleteExportResponse struct {
	Metadata Metadata `json:"metadata"`
	Result   Export   `json:"result"`
	Error    Error    `json:"error"`
}

type GetExportsByFileSystemIDResponse struct {
	Metadata Metadata `json:"metadata"`
	Result   []Export `json:"result"`
	Error    Error    `json:"error"`
}

type CreateExportRequest struct {
	FilesystemID        int                      `json:"filesystem_id,omitempty"`
	Name                string                   `json:"name,omitempty"`
	Transport_protocols string                   `json:"transport_protocols,omitempty"`
	Privileged_port     bool                     `json:"privileged_port"`
	Export_path         string                   `json:"export_path,omitempty"`
	Permissionsput      []map[string]interface{} `json:"permissions,omitempty"`
	SnapdirVisible      bool                     `json:"snapdir_visible"`
}
type CreateExportResponse struct {
	Metadata Metadata `json:"metadata"`
	Result   Export   `json:"result"`
	Error    Error    `json:"error"`
}

func (iboxClient *IboxClient) GetExportByID(exportID int) (ex *Export, err error) {
	URL := fmt.Sprintf("%s/api/rest/exports/%d", iboxClient.Creds.Url, exportID)
	iboxClient.Log.V(TRACE_LEVEL).Info("GetExportByID", "URL", URL, "export ID", exportID)

	req, err := http.NewRequest(http.MethodGet, URL, nil)
	if err != nil {
		return nil, fmt.Errorf("GetExportByID - NewRequest - error %w", err)
	}
	SetAuthHeader(req, iboxClient.Creds)

	resp, err := iboxClient.HttpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GetExportByID - Do - error %w", err)
	}
	defer resp.Body.Close()
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("GetExportByID - ReadAll - error %w", err)
	}
	var responseObject GetExportByIDResponse
	err = json.Unmarshal(bodyBytes, &responseObject)
	if err != nil {
		return nil, fmt.Errorf("GetExportByID - Unmarshal - error %w", err)
	}

	if responseObject.Error.Code != "" {
		if responseObject.Error.Code == "EXPORT_NOT_FOUND" {
			return nil, &IboxAPIError{Code: IBOXAPI_RESOURCE_NOT_FOUND_ERROR, Err: fmt.Errorf("GetExportByID - export ID '%d' not found", exportID)}
		}
		return nil, fmt.Errorf("GetExportByID - ibox API - error:  code: %s message: %s", responseObject.Error.Code, responseObject.Error.Message)
	}
	return &responseObject.Result, nil
}

func (iboxClient *IboxClient) GetExportsByFileSystemID(fsID int) (results []Export, err error) {
	URL := fmt.Sprintf("%sapi/rest/exports", iboxClient.Creds.Url)
	iboxClient.Log.V(TRACE_LEVEL).Info("GetExportsByFileSystemID", "URL", URL, "filesystem ID", fsID)

	pageSize := common.IBOX_DEFAULT_QUERY_PAGE_SIZE
	totalPages := 1 // start with 1, update after first query.
	for page := 1; page <= totalPages; page++ {
		iboxClient.Log.V(TRACE_LEVEL).Info("GetExportsByFileSystemID loop", "page", page, "totalPages", totalPages)

		req, err := http.NewRequest(http.MethodGet, URL, nil)
		if err != nil {
			return results, fmt.Errorf("GetExportsByFileSystemID - NewRequest - error %w", err)
		}

		values := req.URL.Query()
		values.Add("filesystem_id", strconv.Itoa(fsID))
		values.Add("page_size", strconv.Itoa(pageSize))
		values.Add("page", strconv.Itoa(page))
		req.URL.RawQuery = values.Encode()

		SetAuthHeader(req, iboxClient.Creds)

		resp, err := iboxClient.HttpClient.Do(req)
		if err != nil {
			return results, fmt.Errorf("GetExportsByFileSystemID - Do - error %w", err)
		}
		defer resp.Body.Close()
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return results, fmt.Errorf("GetExportsByFileSystemID - ReadAll - error %w", err)
		}
		var responseObject GetExportsByFileSystemIDResponse
		err = json.Unmarshal(bodyBytes, &responseObject)
		if err != nil {
			return results, fmt.Errorf("GetExportsByFileSystemID - Unmarshal - error %w", err)
		}
		results = append(results, responseObject.Result...)

		if page == 1 {
			totalPages = responseObject.Metadata.PagesTotal
		}
	}

	return results, nil
}

func (iboxClient *IboxClient) DeleteExport(exportID int) (response *Export, err error) {
	url := fmt.Sprintf("%sapi/rest/exports/%d", iboxClient.Creds.Url, exportID)
	iboxClient.Log.V(TRACE_LEVEL).Info("DeleteExport", "URL", url, "export ID", exportID)

	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return nil, fmt.Errorf("DeleteExport - NewRequest - error %w", err)
	}

	values := req.URL.Query()
	values.Add("approved", "true")
	req.URL.RawQuery = values.Encode()

	SetAuthHeader(req, iboxClient.Creds)

	resp, err := iboxClient.HttpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("DeleteExport - Do - error %w", err)
	}
	defer resp.Body.Close()
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("DeleteExport - ReadAll -error %w", err)
	}
	var responseObject DeleteExportResponse
	err = json.Unmarshal(bodyBytes, &responseObject)
	if err != nil {
		return nil, fmt.Errorf("DeleteExport - Unmarshal - error %w", err)
	}
	if responseObject.Error.Code != "" {
		if responseObject.Error.Code == "EXPORT_NOT_FOUND" {
			return nil, &IboxAPIError{Code: IBOXAPI_RESOURCE_NOT_FOUND_ERROR, Err: fmt.Errorf("DeleteExport - export ID '%d' not found", exportID)}
		}

		return nil, fmt.Errorf("DeleteExport - ibox API - error:  code: %s message: %s", responseObject.Error.Code, responseObject.Error.Message)
	}
	return &responseObject.Result, nil
}

func (iboxClient *IboxClient) CreateExport(req CreateExportRequest) (*Export, error) {

	URL := iboxClient.Creds.Url + "api/rest/exports"
	iboxClient.Log.V(TRACE_LEVEL).Info("CreateExport", "URL", URL, "request", req)

	jsonBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("CreateExport - Marshal - error %w", err)
	}
	request, err := http.NewRequest(http.MethodPost, URL, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return nil, fmt.Errorf("CreateExport - NewRequest - error %w", err)
	}
	SetAuthHeader(request, iboxClient.Creds)
	request.Header.Set("Content-Type", "application/json; charset=UTF-8")

	response, err := iboxClient.HttpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("CreateExport - Do - error %w", err)
	}
	defer response.Body.Close()

	body, _ := io.ReadAll(response.Body)

	var responseObject CreateExportResponse
	err = json.Unmarshal(body, &responseObject)
	if err != nil {
		return nil, fmt.Errorf("CreateExport - Unmarshal - error %w", err)
	}
	if responseObject.Error.Code != "" {
		return nil, fmt.Errorf("CreateExport - ibox API - error:  code: %s message: %s", responseObject.Error.Code, responseObject.Error.Message)
	}
	iboxClient.Log.V(TRACE_LEVEL).Info("CreateExport", "Export ID", responseObject.Result.ID)
	return &responseObject.Result, nil
}
