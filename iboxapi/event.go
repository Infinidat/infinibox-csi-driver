package iboxapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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

func (client *IboxClient) CreateEvent(ctx context.Context, eventRequest EventRequest) (err error) {
	const functionName = "CreateEvent"

	url := fmt.Sprintf("%s%s", client.Creds.URL, "api/rest/events")
	client.Log.V(TRACE_LEVEL).Info(functionName, "URL", url, "event", eventRequest)

	jsonBytes, err := json.Marshal(eventRequest)
	if err != nil {
		return fmt.Errorf("%s - Marshal - error %w", functionName, err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return fmt.Errorf("%s - NewRequest - error %w", functionName, err)
	}
	SetAuthHeader(request, client.Creds)
	request.Header.Set(CONTENT_TYPE, JSON_CONTENT_TYPE)

	response, err := client.HTTPClient.Do(request)
	if err != nil {
		return fmt.Errorf("%s - Do - error %w", functionName, err)
	}
	defer func() {
		if err := response.Body.Close(); err != nil {
			client.Log.V(INFO_LEVEL).Error(err, functionName, "error in Close()", err.Error())
		}
	}()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("%s - ReadAll - error %w", functionName, err)
	}

	var responseObject CreateEventResponse
	err = json.Unmarshal(body, &responseObject)
	if err != nil {
		return fmt.Errorf("%s - Unmarshal - error %w", functionName, err)
	}
	client.Log.V(DEBUG_LEVEL).Info("CreateEvent", "Event ID", responseObject.Result.ID)
	if responseObject.Error.Code != "" {
		return fmt.Errorf("%s - ibox API - error: %v", functionName, responseObject.Error)
	}
	return nil
}
