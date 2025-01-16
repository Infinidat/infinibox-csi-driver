package iboxapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
)

/**
log levels
logr.V(0) - Info level logging in zerolog
logr.V(1) - Debug level logging in zerolog
logr.V(2) - Trace level logging in zerolog
*/

type EventRequest struct {
	Data []EventRequestData `json:"data"`
	Code string             `json:"code"`
}
type EventRequestData struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Value string `json:"value"`
}

type CreateEventResponse struct {
	Result   CreateEventResult `json:"result"`
	Error    Error             `json:"error"`
	Metadata Metadata          `json:"metadata"`
}

type CreateEventResult struct {
	AffectedEntityID    int    `json:"affected_entity_id"`
	Username            string `json:"username"`
	Code                string `json:"code"`
	Description         string `json:"description"`
	Timestamp           int64  `json:"timestamp"`
	Level               string `json:"level"`
	SeqNum              int    `json:"seq_num"`
	TenantID            int    `json:"tenant_id"`
	Reporter            string `json:"reporter"`
	Visibility          string `json:"visibility"`
	SystemVersion       string `json:"system_version"`
	SourceNodeID        int    `json:"source_node_id"`
	DescriptionTemplate string `json:"description_template"`
	Data                []any  `json:"data"`
	ID                  int    `json:"id"`
}

func (client *IboxClient) CreateEvent(eventRequest EventRequest) (err error) {

	URL := client.Creds.Url + "api/rest/events"
	client.Log.V(TRACE_LEVEL).Info("CreateEvent", "URL", URL, "event", eventRequest)

	jsonBytes, err := json.Marshal(eventRequest)
	if err != nil {
		return err
	}
	request, err := http.NewRequest("POST", URL, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return err
	}
	SetAuthHeader(request, client.Creds)
	request.Header.Set("Content-Type", "application/json; charset=UTF-8")

	response, err := client.HttpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	body, _ := io.ReadAll(response.Body)

	var responseObject CreateEventResponse
	err = json.Unmarshal(body, &responseObject)
	if err != nil {
		return err
	}
	client.Log.V(DEBUG_LEVEL).Info("CreateEvent", "Event ID", responseObject.Result.ID)
	if responseObject.Error.Code != "" {
		client.Log.V(DEBUG_LEVEL).Info("CreateEvent", "Response", responseObject)
	}
	return nil
}
