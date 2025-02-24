package iboxapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"infinibox-csi-driver/common"
	"io"
	"net/http"
	"strconv"
)

type Treeq struct {
	ID           int    `json:"id,omitempty"`
	FilesystemID int    `json:"filesystem_id,omitempty"`
	Name         string `json:"name,omitempty"`
	Path         string `json:"path,omitempty"`
	HardCapacity int64  `json:"hard_capacity,omitempty"`
	UsedCapacity int64  `json:"used_capacity,omitempty"`
}

type GetTreeqByNameResponse struct {
	Metadata Metadata `json:"metadata"`
	Result   []Treeq  `json:"result"`
	Error    Error    `json:"error"`
}
type GetTreeqByFileSystemResponse struct {
	Metadata Metadata `json:"metadata"`
	Result   []Treeq  `json:"result"`
	Error    Error    `json:"error"`
}
type GetFileSystemTreeqCountResponse struct {
	Metadata Metadata `json:"metadata"`
	Result   []Treeq  `json:"result"`
	Error    Error    `json:"error"`
}

type GetTreeqResponse struct {
	Metadata Metadata `json:"metadata"`
	Result   Treeq    `json:"result"`
	Error    Error    `json:"error"`
}

type CreateTreeqRequest struct {
	SoftInodes   int    `json:"soft_inodes,omitempty"`
	Path         string `json:"path"`
	HardCapacity int64  `json:"hard_capacity"`
	HardInodes   int    `json:"hard_inodes,omitempty"`
	Name         string `json:"name"`
}
type CreateTreeqResponse struct {
	Metadata Metadata `json:"metadata"`
	Result   Treeq    `json:"result"`
	Error    Error    `json:"error"`
}

type DeleteTreeqResponse struct {
	Metadata Metadata `json:"metadata"`
	Result   Treeq    `json:"result"`
	Error    Error    `json:"error"`
}
type UpdateTreeqRequest struct {
	SoftInodes   int    `json:"soft_inodes,omitempty"`
	Path         string `json:"path,omitempty"`
	HardCapacity int64  `json:"hard_capacity,omitempty"`
	HardInodes   int    `json:"hard_inodes,omitempty"`
	Name         string `json:"name,omitempty"`
}
type UpdateTreeqResponse struct {
	Metadata Metadata `json:"metadata"`
	Result   Treeq    `json:"result"`
	Error    Error    `json:"error"`
}

func (iboxClient *IboxClient) GetTreeqByName(fsID int, name string) (treeq *Treeq, err error) {
	URL := fmt.Sprintf("%s/api/rest/filesystems/%d/treeqs", iboxClient.Creds.Url, fsID)
	iboxClient.Log.V(TRACE_LEVEL).Info("GetTreeqByName", "URL", URL, "filesystem ID", fsID, "treeq name", name)

	req, err := http.NewRequest("GET", URL, nil)
	if err != nil {
		return nil, fmt.Errorf("GetTreeqByName - NewRequest - error %w", err)
	}

	values := req.URL.Query()
	values.Add("name", name)
	req.URL.RawQuery = values.Encode()

	SetAuthHeader(req, iboxClient.Creds)

	resp, err := iboxClient.HttpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GetTreeqByName - Do - error %w", err)
	}
	defer resp.Body.Close()
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("GetTreeqByName - ReadAll - error %w", err)
	}
	var responseObject GetTreeqByNameResponse
	err = json.Unmarshal(bodyBytes, &responseObject)
	if err != nil {
		return nil, fmt.Errorf("GetTreeqByName - Unmarshal - error %w", err)
	}

	if responseObject.Error.Code != "" {
		if responseObject.Error.Code == "FILESYSTEM_NOT_FOUND" {
			return nil, &IboxAPIError{Code: IBOXAPI_RESOURCE_NOT_FOUND_ERROR, Err: fmt.Errorf("GetTreeqByName - fs ID '%d' not found", fsID)}
		}
		return nil, fmt.Errorf("GetTreeqByName - ibox API - error:  code: %s message: %s", responseObject.Error.Code, responseObject.Error.Message)
	}

	if len(responseObject.Result) == 0 {
		return nil, &IboxAPIError{Code: IBOXAPI_RESOURCE_NOT_FOUND_ERROR, Err: fmt.Errorf("GetTreeqByName - treeq %s not found", name)}
	}
	return &responseObject.Result[0], nil
}

func (iboxClient *IboxClient) GetTreeq(fsID, treeqID int) (treeq *Treeq, err error) {
	URL := fmt.Sprintf("%s/api/rest/filesystems/%d/treeqs/%d", iboxClient.Creds.Url, fsID, treeqID)
	iboxClient.Log.V(TRACE_LEVEL).Info("GetTreeq", "URL", URL, "fs ID", fsID, "treeq ID", treeqID)

	req, err := http.NewRequest(http.MethodGet, URL, nil)
	if err != nil {
		return nil, fmt.Errorf("GetTreeq - NewRequest - error %w", err)
	}
	SetAuthHeader(req, iboxClient.Creds)

	resp, err := iboxClient.HttpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GetTreeq - Do - error %w", err)
	}
	defer resp.Body.Close()
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("GetTreeq - ReadAll - error %w", err)
	}
	var responseObject GetTreeqResponse
	err = json.Unmarshal(bodyBytes, &responseObject)
	if err != nil {
		return nil, fmt.Errorf("GetTreeq - Unmarshal - error %w", err)
	}

	if responseObject.Error.Code != "" {
		if responseObject.Error.Code == "TREEQ_ID_DOES_NOT_EXIST" {
			return nil, &IboxAPIError{Code: IBOXAPI_RESOURCE_NOT_FOUND_ERROR, Err: fmt.Errorf("GetTreeq - fs ID '%d' treeq ID '%d' not found", fsID, treeqID)}
		}
		return nil, fmt.Errorf("GetTreeq - ibox API - error:  code: %s message: %s", responseObject.Error.Code, responseObject.Error.Message)
	}
	return &responseObject.Result, nil
}

func (iboxClient *IboxClient) DeleteTreeq(fsID, treeqID int) (response *Treeq, err error) {
	tmpurl := fmt.Sprintf("api/rest/filesystems/%d/treeq/%d", fsID, treeqID)
	url := fmt.Sprintf("%s%s", iboxClient.Creds.Url, tmpurl)
	iboxClient.Log.V(TRACE_LEVEL).Info("DeleteTreeq", "URL", url, "fs ID", fsID, "treeq ID", treeqID)

	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return nil, fmt.Errorf("DeleteTreeq - NewRquest -  error %w", err)
	}
	SetAuthHeader(req, iboxClient.Creds)

	resp, err := iboxClient.HttpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("DeleteTreeq - Do - error %w", err)
	}
	defer resp.Body.Close()
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("DeleteTreeq - ReadAll - error %w", err)
	}

	var responseObject DeleteTreeqResponse
	err = json.Unmarshal(bodyBytes, &responseObject)
	if err != nil {
		return nil, fmt.Errorf("DeleteTreeq - Unmarshal - error %w", err)
	}

	if responseObject.Error.Code != "" {
		if responseObject.Error.Code == "TREEQ_ID_DOES_NOT_EXIST" {
			return nil, &IboxAPIError{Code: IBOXAPI_RESOURCE_NOT_FOUND_ERROR, Err: fmt.Errorf("DeleteTreeq - fs ID '%d' treeq ID '%d' not found", fsID, treeqID)}
		}
		return nil, fmt.Errorf("DeleteTreeq - ibox API - error code: %s message: %s", responseObject.Error.Code, responseObject.Error.Message)
	}

	return &responseObject.Result, nil
}

func (iboxClient *IboxClient) CreateTreeq(fsID int, treeqRequest CreateTreeqRequest) (treeq *Treeq, err error) {

	URL := iboxClient.Creds.Url + fmt.Sprintf("api/rest/filesystems/%d/treeqs", fsID)
	iboxClient.Log.V(TRACE_LEVEL).Info("CreateTreeq", "URL", URL, "fs ID", fsID)

	jsonBytes, err := json.Marshal(treeqRequest)
	if err != nil {
		return nil, fmt.Errorf("CreateTreeq - Marshal - error %w", err)
	}
	request, err := http.NewRequest(http.MethodPost, URL, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return nil, fmt.Errorf("CreateTreeq - NewRequest - error %w", err)
	}
	SetAuthHeader(request, iboxClient.Creds)
	request.Header.Set("Content-Type", "application/json; charset=UTF-8")

	response, err := iboxClient.HttpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("CreateTreeq - Do - error %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("CreateTreeq -ReadAll - error %w", err)
	}

	var responseObject CreateTreeqResponse
	err = json.Unmarshal(body, &responseObject)
	if err != nil {
		return nil, fmt.Errorf("CreateTreeq - Unmarshal - error %w", err)
	}

	if responseObject.Error.Code != "" {
		return nil, fmt.Errorf("CreateTreeq - ibox API - error code: %s message: %s", responseObject.Error.Code, responseObject.Error.Message)

	}
	return &responseObject.Result, nil
}

func (iboxClient *IboxClient) UpdateTreeq(fsID, treeqID int, updateRequest UpdateTreeqRequest) (*Treeq, error) {
	tmpurl := fmt.Sprintf("api/rest/filesystems/%d/treeqs/%d", fsID, treeqID)
	url := fmt.Sprintf("%s%s", iboxClient.Creds.Url, tmpurl)
	iboxClient.Log.V(TRACE_LEVEL).Info("UpdateTreeq", "URL", url, "fs ID", fsID, "treeq ID", treeqID)

	jsonBytes, err := json.Marshal(updateRequest)
	if err != nil {
		return nil, fmt.Errorf("UpdateTreeq - Marshal - error %w", err)
	}
	request, err := http.NewRequest(http.MethodPut, url, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return nil, fmt.Errorf("UpdateTreeq - NewRequest - error %w", err)
	}

	SetAuthHeader(request, iboxClient.Creds)

	request.Header.Set("Content-Type", "application/json; charset=UTF-8")

	response, err := iboxClient.HttpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("UpdateTreeq - Do - error %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("UpdateTreeq - ReadAll - error %w", err)
	}

	var responseObject UpdateTreeqResponse
	err = json.Unmarshal(body, &responseObject)
	if err != nil {
		return nil, fmt.Errorf("UpdateTreeq - Unmarshal - error %w", err)
	}
	if responseObject.Error.Code != "" {
		if responseObject.Error.Code == "FILESYSTEM_NOT_FOUND" {
			return nil, &IboxAPIError{Code: IBOXAPI_RESOURCE_NOT_FOUND_ERROR, Err: fmt.Errorf("UpdateTreeq- fs ID '%d' not found", fsID)}
		}
		if responseObject.Error.Code == "TREEQ_ID_DOES_NOT_EXIST" {
			return nil, &IboxAPIError{Code: IBOXAPI_RESOURCE_NOT_FOUND_ERROR, Err: fmt.Errorf("UpdateTreeq- fs ID '%d' treeq ID '%d' treeq does not exist", fsID, treeqID)}
		}
		return nil, fmt.Errorf("UpdateTreeq - ibox API - error:  code: %s message: %s", responseObject.Error.Code, responseObject.Error.Message)
	}
	return &responseObject.Result, nil
}

func (iboxClient *IboxClient) GetTreeqsByFileSystem(fsID int) (results []Treeq, err error) {
	URL := fmt.Sprintf("%sapi/rest/filesystems/%d/treeqs", iboxClient.Creds.Url, fsID)
	iboxClient.Log.V(TRACE_LEVEL).Info("GetTreeqsByFileSystem", "URL", URL, "fs ID", fsID)

	pageSize := common.IBOX_DEFAULT_QUERY_PAGE_SIZE
	totalPages := 1 // start with 1, update after first query.
	for page := 1; page <= totalPages; page++ {
		iboxClient.Log.V(TRACE_LEVEL).Info("GetTreeqsByFileSystem loop", "page", page, "totalPages", totalPages)

		req, err := http.NewRequest(http.MethodGet, URL, nil)
		if err != nil {
			return results, fmt.Errorf("GetTreeqsByFileSystem - NewRequest - error %w", err)
		}

		values := req.URL.Query()
		values.Add("page_size", strconv.Itoa(pageSize))
		values.Add("page", strconv.Itoa(page))
		req.URL.RawQuery = values.Encode()

		SetAuthHeader(req, iboxClient.Creds)

		resp, err := iboxClient.HttpClient.Do(req)
		if err != nil {
			return results, fmt.Errorf("GetTreeqsByFileSystem - Do - error %w", err)
		}
		defer resp.Body.Close()
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return results, fmt.Errorf("GetTreeqsByFileSystem - ReadAll - error %w", err)
		}
		var responseObject GetTreeqByFileSystemResponse
		err = json.Unmarshal(bodyBytes, &responseObject)
		if err != nil {
			return results, fmt.Errorf("GetTreeqsByFileSystem - Unmarshal - error %w", err)
		}
		results = append(results, responseObject.Result...)

		if page == 1 {
			totalPages = responseObject.Metadata.PagesTotal
		}
	}

	return results, nil
}
