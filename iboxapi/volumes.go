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

type GetLunsByVolumeResponse struct {
	Metadata Metadata `json:"metadata"`
	Result   []Luns   `json:"result"`
	Error    Error    `json:"error"`
}

type Volume struct {
	CgId                  int    `json:"cg_id,omitempty"`
	RmrTarget             bool   `json:"rmr_target,omitempty"`
	UpdatedAt             int    `json:"updated_at,omitempty"`
	NumBlocks             int    `json:"num_blocks,omitempty"`
	Allocated             int    `json:"allocated,omitempty"`
	Serial                string `json:"serial,omitempty"`
	Size                  int64  `json:"size,omitempty"`
	SsdEnabled            bool   `json:"ssd_enabled,omitempty"`
	ID                    int    `json:"id,omitempty"`
	ParentId              int    `json:"parent_id,omitempty"`
	CompressionSuppressed bool   `json:"compression_suppressed,omitempty"`
	Type                  string `json:"type,omitempty"`
	RmrSource             bool   `json:"rmr_source,omitempty"`
	Used                  int    `json:"used,omitempty"`
	TreeAllocated         int    `json:"tree_allocated,omitempty"`
	HasChildren           bool   `json:"has_children,omitempty"`
	DatasetType           string `json:"dataset_type,omitempty"`
	Provtype              string `json:"provtype,omitempty"`
	RmrSnapshotGuid       string `json:"rmr_snapshot_guid,omitempty"`
	CapacitySavings       int    `json:"capacity_savings,omitempty"`
	Name                  string `json:"name,omitempty"`
	CreatedAt             int64  `json:"created_at,omitempty"`
	PoolId                int    `json:"pool_id,omitempty"`
	PoolName              string `json:"pool_name,omitempty"`
	CompressionEnabled    bool   `json:"compression_enabled,omitempty"`
	FamilyId              int    `json:"family_id,omitempty"`
	Depth                 int    `json:"depth,omitempty"`
	WriteProtected        bool   `json:"write_protected,omitempty"`
	Mapped                bool   `json:"mapped,omitempty"`
	LockExpiresAt         int64  `json:"lock_expires_at,omitempty"`
	LockState             string `json:"lock_state,omitempty"`
}

type CreateVolumeRequest struct {
	PoolId        int    `json:"pool_id,omitempty"`
	VolumeSize    int64  `json:"size,omitempty"`
	Name          string `json:"name,omitempty"`
	ProvisionType string `json:"provtype,omitempty"`
	SsdEnabled    bool   `json:"ssd_enabled,omitempty"`
}

type CreateVolumeResponse struct {
	Metadata Metadata `json:"metadata"`
	Result   Volume   `json:"result"`
	Error    Error    `json:"error"`
}

type UpdateVolumeResponse struct {
	Metadata Metadata `json:"metadata"`
	Result   Volume   `json:"result"`
	Error    Error    `json:"error"`
}

type DeleteVolumeResponse struct {
	Metadata Metadata `json:"metadata"`
	Result   Volume   `json:"result"`
	Error    Error    `json:"error"`
}

type GetVolumeByNameResponse struct {
	Metadata Metadata `json:"metadata"`
	Result   []Volume `json:"result"`
	Error    Error    `json:"error"`
}

type GetVolumeResponse struct {
	Metadata Metadata `json:"metadata"`
	Result   Volume   `json:"result"`
	Error    Error    `json:"error"`
}

func (iboxClient *IboxClient) GetLunsByVolume(volumeID int) (results []Luns, err error) {
	URL := fmt.Sprintf("%sapi/rest/volumes/%d/luns", iboxClient.Creds.Url, volumeID)
	iboxClient.Log.V(TRACE_LEVEL).Info("GetLunsByVolume", "URL", URL, "volume ID", volumeID)

	pageSize := common.IBOX_DEFAULT_QUERY_PAGE_SIZE
	totalPages := 1 // start with 1, update after first query.
	for page := 1; page <= totalPages; page++ {
		iboxClient.Log.V(TRACE_LEVEL).Info("GetLunsByVolume loop", "page", page, "totalPages", totalPages)

		req, err := http.NewRequest(http.MethodGet, URL, nil)
		if err != nil {
			return results, fmt.Errorf("GetLunsByVolume - NewRequest - error %w", err)
		}

		values := req.URL.Query()
		values.Add("page_size", strconv.Itoa(pageSize))
		values.Add("page", strconv.Itoa(page))
		req.URL.RawQuery = values.Encode()

		SetAuthHeader(req, iboxClient.Creds)

		resp, err := iboxClient.HttpClient.Do(req)
		if err != nil {
			return results, fmt.Errorf("GetLunsByVolume - Do - error %w", err)
		}
		defer resp.Body.Close()
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return results, fmt.Errorf("GetLunsByVolume - ReadAll - error %w", err)
		}
		var responseObject GetLunsByVolumeResponse
		err = json.Unmarshal(bodyBytes, &responseObject)
		if err != nil {
			return results, fmt.Errorf("GetLunsByVolume - Unmarshal - error %w", err)
		}
		results = append(results, responseObject.Result...)

		if page == 1 {
			totalPages = responseObject.Metadata.PagesTotal
		}
	}

	return results, nil
}

func (iboxClient *IboxClient) CreateVolume(req CreateVolumeRequest) (*Volume, error) {

	URL := iboxClient.Creds.Url + "api/rest/volumes"
	iboxClient.Log.V(TRACE_LEVEL).Info("CreateVolume", "URL", URL, "request", req)

	jsonBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("CreateVolume - Marshal - error %w", err)
	}
	request, err := http.NewRequest(http.MethodPost, URL, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return nil, fmt.Errorf("CreateVolume - NewRequest - error %w", err)
	}
	SetAuthHeader(request, iboxClient.Creds)
	request.Header.Set("Content-Type", "application/json; charset=UTF-8")

	response, err := iboxClient.HttpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("CreateVolume - Do - error %w", err)
	}
	defer response.Body.Close()

	body, _ := io.ReadAll(response.Body)

	var responseObject CreateVolumeResponse
	err = json.Unmarshal(body, &responseObject)
	if err != nil {
		return nil, fmt.Errorf("CreateVolume - Unmarshal - error %w", err)
	}
	if responseObject.Error.Code != "" {
		return nil, fmt.Errorf("CreateVolume - ibox API - error:  code: %s message: %s", responseObject.Error.Code, responseObject.Error.Message)
	}
	iboxClient.Log.V(TRACE_LEVEL).Info("CreateVolume", "Volume ID", responseObject.Result.ID)
	return &responseObject.Result, nil
}

func (iboxClient *IboxClient) DeleteVolume(volumeID int) (response *DeleteVolumeResponse, err error) {
	url := fmt.Sprintf("%sapi/rest/volumes/%d", iboxClient.Creds.Url, volumeID)
	iboxClient.Log.V(TRACE_LEVEL).Info("DeleteVolume", "URL", url, "volume ID", volumeID)

	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return nil, fmt.Errorf("DeleteVolume - NewRequest - error %w", err)
	}

	values := req.URL.Query()
	values.Add("approved", "true")
	req.URL.RawQuery = values.Encode()

	SetAuthHeader(req, iboxClient.Creds)

	resp, err := iboxClient.HttpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("DeleteVolume - Do - error %w", err)
	}
	defer resp.Body.Close()
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("DeleteVolume - ReadAll -error %w", err)
	}
	var responseObject DeleteVolumeResponse
	err = json.Unmarshal(bodyBytes, &responseObject)
	if err != nil {
		return nil, fmt.Errorf("DeleteVolume - Unmarshal - error %w", err)
	}
	if responseObject.Error.Code != "" {
		//TODO check for NOT FOUND?  have callers check for ErrNotFound?
		return nil, fmt.Errorf("DeleteVolume - ibox API - error:  code: %s message: %s", responseObject.Error.Code, responseObject.Error.Message)
	}
	return &responseObject, nil
}

func (iboxClient *IboxClient) GetVolumeByName(volumeName string) (volume *Volume, err error) {
	URL := fmt.Sprintf("%sapi/rest/volumes", iboxClient.Creds.Url)
	iboxClient.Log.V(TRACE_LEVEL).Info("GetVolumeByName", "URL", URL, "volume Name", volumeName)

	pageSize := common.IBOX_DEFAULT_QUERY_PAGE_SIZE
	totalPages := 1 // start with 1, update after first query.
	for page := 1; page <= totalPages; page++ {
		iboxClient.Log.V(TRACE_LEVEL).Info("GetVolumeByName loop", "page", page, "totalPages", totalPages)

		req, err := http.NewRequest(http.MethodGet, URL, nil)
		if err != nil {
			return nil, fmt.Errorf("GetVolumeByName - NewRequest - error %w", err)
		}

		values := req.URL.Query()
		values.Add("name", volumeName)
		values.Add("page_size", strconv.Itoa(pageSize))
		values.Add("page", strconv.Itoa(page))
		req.URL.RawQuery = values.Encode()

		SetAuthHeader(req, iboxClient.Creds)

		resp, err := iboxClient.HttpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("GetVolumeByName - Do - error %w", err)
		}
		defer resp.Body.Close()
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("GetVolumeByName - ReadAll - error %w", err)
		}
		var responseObject GetVolumeByNameResponse
		err = json.Unmarshal(bodyBytes, &responseObject)
		if err != nil {
			return nil, fmt.Errorf("GetVolumeByName - Unmarshal - error %w", err)
		}
		if responseObject.Error.Code != "" {
			//TODO check for NOT FOUND?  return ErrNotFound for callers?
			return nil, fmt.Errorf("GetVolumeByName - ibox API - error code %s message %s", responseObject.Error.Code, responseObject.Error.Message)
		}
		if len(responseObject.Result) > 0 {
			volume = &responseObject.Result[0]
		} else {
			return nil, &IboxAPIError{Code: IBOXAPI_RESOURCE_NOT_FOUND_ERROR, Err: fmt.Errorf("GetVolumeByName - volume name '%s' not found", volumeName)}
		}

		if page == 1 {
			totalPages = responseObject.Metadata.PagesTotal
		}
	}

	return volume, nil
}

func (iboxClient *IboxClient) GetVolume(volumeID int) (volume *Volume, err error) {
	URL := fmt.Sprintf("%s/api/rest/volumes/%d", iboxClient.Creds.Url, volumeID)
	iboxClient.Log.V(TRACE_LEVEL).Info("GetVolume", "URL", URL, "volume ID", volumeID)

	req, err := http.NewRequest(http.MethodGet, URL, nil)
	if err != nil {
		return nil, fmt.Errorf("GetVolume - NewRequest - error %w", err)
	}
	SetAuthHeader(req, iboxClient.Creds)

	resp, err := iboxClient.HttpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GetVolume - Do - error %w", err)
	}
	defer resp.Body.Close()
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("GetVolume - ReadAll - error %w", err)
	}
	var responseObject GetVolumeResponse
	err = json.Unmarshal(bodyBytes, &responseObject)
	if err != nil {
		return nil, fmt.Errorf("GetVolume - Unmarshal - error %w", err)
	}

	if responseObject.Error.Code != "" {
		if responseObject.Error.Code == "VOLUME_NOT_FOUND" {
			return nil, &IboxAPIError{Code: IBOXAPI_RESOURCE_NOT_FOUND_ERROR, Err: fmt.Errorf("GetVolume - volume ID '%d' not found", volumeID)}
		}
		return nil, fmt.Errorf("GetVolume - ibox API - error:  code: %s message: %s", responseObject.Error.Code, responseObject.Error.Message)
	}
	return &responseObject.Result, nil
}

func (iboxClient *IboxClient) UpdateVolume(volumeID int, volume Volume) (*Volume, error) {
	url := fmt.Sprintf("%s%s/%d", iboxClient.Creds.Url, "api/rest/volumes/", volumeID)
	iboxClient.Log.V(TRACE_LEVEL).Info("UpdateVolume", "URL", url, "volume ID", volumeID)

	jsonBytes, err := json.Marshal(volume)
	if err != nil {
		return nil, fmt.Errorf("UpdateVolume - Marshal - error %w", err)
	}
	request, err := http.NewRequest(http.MethodPut, url, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return nil, fmt.Errorf("UpdateVolume - NewRequest - error %w", err)
	}

	SetAuthHeader(request, iboxClient.Creds)

	request.Header.Set("Content-Type", "application/json; charset=UTF-8")

	response, err := iboxClient.HttpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("UpdateVolume - Do - error %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("UpdateVolume - ReadAll - error %w", err)
	}

	var responseObject UpdateVolumeResponse
	err = json.Unmarshal(body, &responseObject)
	if err != nil {
		return nil, fmt.Errorf("UpdateVolume - Unmarshal - error %w", err)
	}
	if responseObject.Error.Code != "" {
		//TODO check for NOT FOUND?  return ErrNotFound for callers?
		return nil, fmt.Errorf("UpdateVolume - ibox API - error:  code: %s message: %s", responseObject.Error.Code, responseObject.Error.Message)
	}
	return &responseObject.Result, nil
}
