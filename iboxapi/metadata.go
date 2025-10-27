package iboxapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/infinidat/infinibox-csi-driver/common"
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

func (client *IboxClient) PutMetadata(objectID int, metadata map[string]any) (r *PutMetadataResponse, err error) {
	const functionName = "PutMetadata"
	url := fmt.Sprintf("%s%s/%d", client.Creds.URL, "api/rest/metadata/", objectID)
	client.Log.V(TRACE_LEVEL).Info(functionName, "URL", url, "object ID", objectID, "map", metadata)

	jsonBytes, err := json.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("%s - Marshal - error %w", functionName, err)
	}
	request, err := http.NewRequest(http.MethodPut, url, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return nil, fmt.Errorf("%s - NewRequest - error %w", functionName, err)
	}

	SetAuthHeader(request, client.Creds)

	request.Header.Set(CONTENT_TYPE, JSON_CONTENT_TYPE)

	response, err := client.HTTPClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("%s - Do - error %w", functionName, err)
	}
	defer func() {
		if err := response.Body.Close(); err != nil {
			client.Log.V(INFO_LEVEL).Error(err, functionName, "error in Close()", err.Error())
		}
	}()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("%s - ReadAll - error %w", functionName, err)
	}

	var responseObject PutMetadataResponse
	err = json.Unmarshal(body, &responseObject)
	if err != nil {
		return nil, fmt.Errorf("%s - Unmarshal - error %w", functionName, err)
	}
	if responseObject.Error.Code != "" {
		return nil, fmt.Errorf("%s - ibox API - error:  code: %s message: %s", functionName, responseObject.Error.Code, responseObject.Error.Message)
	}
	return &responseObject, nil
}

func (client *IboxClient) GetMetadata(objectID int) (results []GetMetadataResult, err error) {
	const functionName = "GetMetadata"
	url := fmt.Sprintf("%s%s/%d", client.Creds.URL, "api/rest/metadata", objectID)
	client.Log.V(TRACE_LEVEL).Info(functionName, "URL", url, "object ID", objectID)

	pageSize := common.IBOXDefaultQueryPageSize
	totalPages := 1 // start with 1, update after first query.
	for page := 1; page <= totalPages; page++ {
		client.Log.V(TRACE_LEVEL).Info(functionName, "page", page, "totalPages", totalPages)

		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return results, fmt.Errorf("%s - NewRequest - error %w", functionName, err)
		}

		values := req.URL.Query()
		values.Add(PARAMETER_PAGE_SIZE, strconv.Itoa(pageSize))
		values.Add(PARAMETER_PAGE, strconv.Itoa(page))
		req.URL.RawQuery = values.Encode()

		SetAuthHeader(req, client.Creds)

		resp, err := client.HTTPClient.Do(req)
		if err != nil {
			return results, fmt.Errorf("%s - Do - error %w", functionName, err)
		}
		defer func() {
			if err := resp.Body.Close(); err != nil {
				client.Log.V(INFO_LEVEL).Error(err, functionName, "error in Close()", err.Error())
			}
		}()
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return results, fmt.Errorf("%s - ReadAll - error %w", functionName, err)
		}
		var responseObject GetMetadataResponse
		err = json.Unmarshal(bodyBytes, &responseObject)
		if err != nil {
			return results, fmt.Errorf("%s - Unmarshal - error %w", functionName, err)
		}
		client.Log.V(TRACE_LEVEL).Info(functionName, "resp", responseObject)
		results = append(results, responseObject.Result...)

		if page == 1 {
			totalPages = responseObject.Metadata.PagesTotal
		}
	}

	return results, nil
}

func (client *IboxClient) DeleteMetadata(objectID int) (response *DeleteMetadataResponse, err error) {
	const functionName = "DeleteMetadata"
	url := fmt.Sprintf("%s%s/%d", client.Creds.URL, "api/rest/metadata", objectID)
	client.Log.V(DEBUG_LEVEL).Info(functionName, "URL", url, "object ID", objectID)

	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return nil, fmt.Errorf("%s - NewRequest - error %w", functionName, err)
	}

	values := req.URL.Query()
	values.Add(PARAMETER_APPROVED, PARAMETER_VALUE_TRUE)
	req.URL.RawQuery = values.Encode()

	SetAuthHeader(req, client.Creds)

	resp, err := client.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s - Do - error %w", functionName, err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			client.Log.V(INFO_LEVEL).Error(err, functionName, "error in Close()", err.Error())
		}
	}()
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%s - ReadAll - error %w", functionName, err)
	}
	var responseObject DeleteMetadataResponse
	err = json.Unmarshal(bodyBytes, &responseObject)
	if err != nil {
		return nil, fmt.Errorf("%s - Unmarshal - error %w", functionName, err)
	}
	if responseObject.Error.Code != "" {
		return nil, fmt.Errorf("%s - ibox API - error:  code: %s message: %s", functionName, responseObject.Error.Code, responseObject.Error.Message)
	}
	return &responseObject, nil
}
