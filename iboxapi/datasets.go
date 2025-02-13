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

type GetAllSnapshotsResponse struct {
	Metadata Metadata `json:"metadata"`
	Result   []Volume `json:"result"`
	Error    Error    `json:"error"`
}

func (iboxClient *IboxClient) GetAllSnapshots() (results []Volume, err error) {
	URL := fmt.Sprintf("%sapi/rest/datasets", iboxClient.Creds.Url)
	iboxClient.Log.V(TRACE_LEVEL).Info("GetAllSnapshots", "URL", URL)

	pageSize := common.IBOX_DEFAULT_QUERY_PAGE_SIZE
	totalPages := 1 // start with 1, update after first query.
	for page := 1; page <= totalPages; page++ {
		iboxClient.Log.V(TRACE_LEVEL).Info("GetAllSnapshots loop", "page", page, "totalPages", totalPages)

		req, err := http.NewRequest("GET", URL, nil)
		if err != nil {
			return results, fmt.Errorf("GetAllSnapshots - NewRequest - error %w", err)
		}

		values := req.URL.Query()
		values.Add("page_size", strconv.Itoa(pageSize))
		values.Add("page", strconv.Itoa(page))
		values.Add("type", "SNAPSHOT")
		req.URL.RawQuery = values.Encode()

		SetAuthHeader(req, iboxClient.Creds)

		resp, err := iboxClient.HttpClient.Do(req)
		if err != nil {
			return results, fmt.Errorf("GetAllSnapshots - Do - error %w", err)
		}
		defer resp.Body.Close()
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return results, fmt.Errorf("GetAllSnapshots - ReadAll - error %w", err)
		}
		var responseObject GetAllSnapshotsResponse
		err = json.Unmarshal(bodyBytes, &responseObject)
		if err != nil {
			return results, fmt.Errorf("GetAllSnapshots - Unmarshal - error %w", err)
		}
		results = append(results, responseObject.Result...)

		if page == 1 {
			totalPages = responseObject.Metadata.PagesTotal
		}
	}

	return results, nil
}
