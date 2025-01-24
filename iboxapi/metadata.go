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

func (client *IboxClient) PutMetadata(objectID int, metadata map[string]interface{}) (r *PutMetadataResponse, err error) {
	url := fmt.Sprintf("%s%s/%d", client.Creds.Url, "api/rest/metadata/", objectID)
	client.Log.V(TRACE_LEVEL).Info("PutMetadata", "URL", url, "object ID", objectID, "map", metadata)

	jsonBytes, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequest("PUT", url, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return nil, err
	}

	SetAuthHeader(request, client.Creds)

	request.Header.Set("Content-Type", "application/json; charset=UTF-8")

	response, err := client.HttpClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	var responseObject PutMetadataResponse
	err = json.Unmarshal(body, &responseObject)
	if err != nil {
		return nil, err
	}
	return &responseObject, nil
}

func (client *IboxClient) GetMetadata(objectID int) (results []GetMetadataResult, err error) {
	URL := fmt.Sprintf("%sapi/rest/metadata/%d", client.Creds.Url, objectID)
	client.Log.V(TRACE_LEVEL).Info("GetMetadata", "URL", URL, "object ID", objectID)

	pageSize := common.IBOX_DEFAULT_QUERY_PAGE_SIZE
	totalPages := 1 // start with 1, update after first query.
	for page := 1; page <= totalPages; page++ {
		client.Log.V(TRACE_LEVEL).Info("GetMetadata loop", "page", page, "totalPages", totalPages)

		req, err := http.NewRequest("GET", URL, nil)
		if err != nil {
			return results, fmt.Errorf("error in NewRequest %w", err)
		}

		values := req.URL.Query()
		values.Add("page_size", strconv.Itoa(pageSize))
		values.Add("page", strconv.Itoa(page))
		req.URL.RawQuery = values.Encode()

		SetAuthHeader(req, client.Creds)

		resp, err := client.HttpClient.Do(req)
		if err != nil {
			return results, fmt.Errorf("error with client.Do %w", err)
		}
		defer resp.Body.Close()
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return results, fmt.Errorf("error reading response body %w", err)
		}
		var responseObject GetMetadataResponse
		err = json.Unmarshal(bodyBytes, &responseObject)
		if err != nil {
			return results, fmt.Errorf("error in Unmarshal %w", err)
		}
		client.Log.V(TRACE_LEVEL).Info("GetMetadata resp", "resp", responseObject)
		results = append(results, responseObject.Result...)

		if page == 1 {
			totalPages = responseObject.Metadata.PagesTotal
		}
	}

	return results, nil
}

func (client *IboxClient) DeleteMetadata(objectID int) (response *DeleteMetadataResponse, err error) {
	url := fmt.Sprintf("%sapi/rest/metadata/%d", client.Creds.Url, objectID)
	client.Log.V(DEBUG_LEVEL).Info("DeleteMetadata", "URL", url, "object ID", objectID)

	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return nil, err
	}

	values := req.URL.Query()
	values.Add("approved", "true")
	req.URL.RawQuery = values.Encode()

	SetAuthHeader(req, client.Creds)

	resp, err := client.HttpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var responseObject DeleteMetadataResponse
	err = json.Unmarshal(bodyBytes, &responseObject)
	if err != nil {
		return nil, err
	}
	return &responseObject, nil
}
