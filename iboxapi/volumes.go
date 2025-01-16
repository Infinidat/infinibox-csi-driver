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

func (client *IboxClient) GetLunsByVolume(volumeID int) (results []Luns, err error) {
	URL := fmt.Sprintf("%sapi/rest/volumes/%d/luns", client.Creds.Url, volumeID)
	client.Log.V(TRACE_LEVEL).Info("GetLunsByVolume", "URL", URL, "volume ID", volumeID)

	pageSize := common.IBOX_DEFAULT_QUERY_PAGE_SIZE
	totalPages := 1 // start with 1, update after first query.
	for page := 1; page <= totalPages; page++ {
		client.Log.V(TRACE_LEVEL).Info("GetLunsByVolume loop", "page", page, "totalPages", totalPages)

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
		var responseObject GetLunsByVolumeResponse
		err = json.Unmarshal(bodyBytes, &responseObject)
		if err != nil {
			return results, fmt.Errorf("error in Unmarshal %w", err)
		}
		results = append(results, responseObject.Result...)

		if page == 1 {
			totalPages = responseObject.Metadata.PagesTotal
		}
	}

	return results, nil
}
