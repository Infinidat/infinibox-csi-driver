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

type MetadataResult struct {
	ID         int    `json:"id"`
	ObjectID   int    `json:"object_id"`
	Key        string `json:"key"`
	Value      string `json:"value"`
	ObjectType string `json:"object_type"`
}

type DeleteMetadataResponse struct {
	Results  []MetadataResult `json:"results"`
	Error    Error            `json:"error"`
	Metadata Metadata         `json:"metadata"`
}

type PutMetadataResponse struct {
	Results  []MetadataResult `json:"results"`
	Error    Error            `json:"error"`
	Metadata Metadata         `json:"metadata"`
}

type GetMetadataResponse struct {
	Metadata Metadata            `json:"metadata"`
	Result   []GetMetadataResult `json:"result"`
	Error    any                 `json:"error"`
}
type GetMetadataResult struct {
	Key        string `json:"key"`
	Value      string `json:"value"`
	ObjectType string `json:"object_type"`
	ID         int    `json:"id"`
	ObjectID   int    `json:"object_id"`
}

func (iboxClient *IboxClient) PutMetadata(objectID int, metadata map[string]interface{}) (r *PutMetadataResponse, err error) {
	url := fmt.Sprintf("%s%s/%d", iboxClient.Creds.Url, "api/rest/metadata/", objectID)
	iboxClient.Log.V(TRACE_LEVEL).Info("PutMetadata", "URL", url, "object ID", objectID, "map", metadata)

	jsonBytes, err := json.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("PutMetadata - Marshal - error %w", err)
	}
	request, err := http.NewRequest("PUT", url, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return nil, fmt.Errorf("PutMetadata - NewRequest - error %w", err)
	}

	SetAuthHeader(request, iboxClient.Creds)

	request.Header.Set("Content-Type", "application/json; charset=UTF-8")

	response, err := iboxClient.HttpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("PutMetadata - Do - error %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("PutMetadata - ReadAll - error %w", err)
	}

	var responseObject PutMetadataResponse
	err = json.Unmarshal(body, &responseObject)
	if err != nil {
		return nil, fmt.Errorf("PutMetadata - Unmarshal - error %w", err)
	}
	if responseObject.Error.Code != "" {
		return nil, fmt.Errorf("PutMetadata - ibox API - error:  code: %s message: %s", responseObject.Error.Code, responseObject.Error.Message)
	}
	return &responseObject, nil
}

func (iboxClient *IboxClient) GetMetadata(objectID int) (results []GetMetadataResult, err error) {
	URL := fmt.Sprintf("%sapi/rest/metadata/%d", iboxClient.Creds.Url, objectID)
	iboxClient.Log.V(TRACE_LEVEL).Info("GetMetadata", "URL", URL, "object ID", objectID)

	pageSize := common.IBOX_DEFAULT_QUERY_PAGE_SIZE
	totalPages := 1 // start with 1, update after first query.
	for page := 1; page <= totalPages; page++ {
		iboxClient.Log.V(TRACE_LEVEL).Info("GetMetadata loop", "page", page, "totalPages", totalPages)

		req, err := http.NewRequest("GET", URL, nil)
		if err != nil {
			return results, fmt.Errorf("GetMetadata - NewRequest - error %w", err)
		}

		values := req.URL.Query()
		values.Add("page_size", strconv.Itoa(pageSize))
		values.Add("page", strconv.Itoa(page))
		req.URL.RawQuery = values.Encode()

		SetAuthHeader(req, iboxClient.Creds)

		resp, err := iboxClient.HttpClient.Do(req)
		if err != nil {
			return results, fmt.Errorf("GetMetadata - Do - error %w", err)
		}
		defer resp.Body.Close()
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return results, fmt.Errorf("GetMetadata - ReadAll - error %w", err)
		}
		var responseObject GetMetadataResponse
		err = json.Unmarshal(bodyBytes, &responseObject)
		if err != nil {
			return results, fmt.Errorf("GetMetadata - Unmarshal - error %w", err)
		}
		iboxClient.Log.V(TRACE_LEVEL).Info("GetMetadata resp", "resp", responseObject)
		results = append(results, responseObject.Result...)

		if page == 1 {
			totalPages = responseObject.Metadata.PagesTotal
		}
	}

	return results, nil
}

func (iboxClient *IboxClient) DeleteMetadata(objectID int) (response *DeleteMetadataResponse, err error) {
	url := fmt.Sprintf("%sapi/rest/metadata/%d", iboxClient.Creds.Url, objectID)
	iboxClient.Log.V(DEBUG_LEVEL).Info("DeleteMetadata", "URL", url, "object ID", objectID)

	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return nil, fmt.Errorf("DeleteMetadata - NewRequest - error %w", err)
	}

	values := req.URL.Query()
	values.Add("approved", "true")
	req.URL.RawQuery = values.Encode()

	SetAuthHeader(req, iboxClient.Creds)

	resp, err := iboxClient.HttpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("DeleteMetadata - Do - error %w", err)
	}
	defer resp.Body.Close()
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("DeleteMetadata - ReadAll - error %w", err)
	}
	var responseObject DeleteMetadataResponse
	err = json.Unmarshal(bodyBytes, &responseObject)
	if err != nil {
		return nil, fmt.Errorf("DeleteMetadata - Unmarshal - error %w", err)
	}
	if responseObject.Error.Code != "" {
		return nil, fmt.Errorf("DeleteMetadata - ibox API - error:  code: %s message: %s", responseObject.Error.Code, responseObject.Error.Message)
	}
	return &responseObject, nil
}
