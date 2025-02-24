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

const (
	CHAP_SECURITY_METHOD   = "security_method"
	CHAP_INBOUND_USERNAME  = "security_chap_inbound_username"
	CHAP_INBOUND_SECRET    = "security_chap_inbound_secret"
	CHAP_OUTBOUND_USERNAME = "security_chap_outbound_username"
	CHAP_OUTBOUND_SECRET   = "security_chap_outbound_secret"
)

type AddHostSecurityRequest struct {
	SecurityMethod               string `json:"security_method"`
	SecurityCHAPInboundUsername  string `json:"security_chap_inbound_username,omitempty"`
	SecurityCHAPInboundSecret    string `json:"security_chap_inbound_secret,omitempty"`
	SecurityCHAPOutboundUsername string `json:"security_chap_outbound_username,omitempty"`
	SecurityCHAPOutboundSecret   string `json:"security_chap_outbound_secret,omitempty"`
}

type CreateHostPost struct {
	Name string `json:"name"`
}

type CreateHostResponse struct {
	Result   Host               `json:"result"`
	Error    Error              `json:"error"`
	Metadata CreateHostMetadata `json:"metadata"`
}

/*
*

	type CreateHostResult struct {
		ID                            int    `json:"id"`
		Name                          string `json:"name"`
		Ports                         []any  `json:"ports"`
		Luns                          []any  `json:"luns"`
		CreatedAt                     int64  `json:"created_at"`
		UpdatedAt                     int64  `json:"updated_at"`
		HostType                      string `json:"host_type"`
		SecurityMethod                string `json:"security_method"`
		SecurityChapInboundUsername   any    `json:"security_chap_inbound_username"`
		SecurityChapOutboundUsername  any    `json:"security_chap_outbound_username"`
		Optimized                     bool   `json:"optimized"`
		SanClientType                 string `json:"san_client_type"`
		HostClusterID                 int    `json:"host_cluster_id"`
		SubsystemNqn                  any    `json:"subsystem_nqn"`
		SecurityChapHasInboundSecret  bool   `json:"security_chap_has_inbound_secret"`
		SecurityChapHasOutboundSecret bool   `json:"security_chap_has_outbound_secret"`
		TenantID                      int    `json:"tenant_id"`
	}
*/
type CreateHostMetadata struct {
	Ready bool `json:"ready"`
}

type DeleteHostResponse struct {
	Result   Host               `json:"result"`
	Error    Error              `json:"error"`
	Metadata CreateHostMetadata `json:"metadata"`
}

type HostResponse struct {
	Result   []Host   `json:"result"`
	Error    Error    `json:"error"`
	Metadata Metadata `json:"metadata"`
}
type Ports struct {
	Address string `json:"address"`
	Type    string `json:"type"`
	HostID  int    `json:"host_id"`
}
type LunInfo struct {
	ID            int  `json:"id,omitempty"`
	Lun           int  `json:"lun,omitempty"`
	CLustered     bool `json:"clustered,omitempty"`
	VolumeID      int  `json:"volume_id,omitempty"`
	HostClusterID int  `json:"host_cluster_id,omitempty"`
	HostID        int  `json:"host_id,omitempty"`
	Udid          any  `json:"udid,omitempty"`
}

type GetAllLunsResponse struct {
	Result   []LunInfo `json:"result"`
	Error    Error     `json:"error"`
	Metadata Metadata  `json:"metadata"`
}

type UnMapVolumeFromHostResponse struct {
	Result   LunInfo  `json:"result"`
	Error    Error    `json:"error"`
	Metadata Metadata `json:"metadata"`
}

type Host struct {
	ID                            int       `json:"id"`
	Name                          string    `json:"name"`
	Ports                         []Ports   `json:"ports"`
	Luns                          []LunInfo `json:"luns"`
	CreatedAt                     int64     `json:"created_at"`
	UpdatedAt                     int64     `json:"updated_at"`
	HostType                      string    `json:"host_type"`
	SecurityMethod                string    `json:"security_method"`
	SecurityChapInboundUsername   any       `json:"security_chap_inbound_username"`
	SecurityChapOutboundUsername  any       `json:"security_chap_outbound_username"`
	Optimized                     bool      `json:"optimized"`
	SanClientType                 string    `json:"san_client_type"`
	HostClusterID                 int       `json:"host_cluster_id"`
	SubsystemNqn                  any       `json:"subsystem_nqn"`
	SecurityChapHasInboundSecret  bool      `json:"security_chap_has_inbound_secret"`
	SecurityChapHasOutboundSecret bool      `json:"security_chap_has_outbound_secret"`
	TenantID                      int       `json:"tenant_id"`
}

type AddHostSecurityResponse struct {
	Result   Host               `json:"result"`
	Error    Error              `json:"error"`
	Metadata CreateHostMetadata `json:"metadata"`
}

type AddPortRequest struct {
	Type    string `json:"type"`
	Address string `json:"address"`
}

type HostPort struct {
	HostID      int    `json:"host_id,omitempty"`
	PortType    string `json:"type,omitempty"`
	PortAddress string `json:"address,omitempty"`
}

type GetHostPortResponse struct {
	Metadata Metadata   `json:"metadata"`
	Result   []HostPort `json:"result"`
	Error    Error      `json:"error"`
}

type AddPortResponse struct {
	Metadata Metadata      `json:"metadata"`
	Result   AddPortResult `json:"result"`
	Error    Error         `json:"error"`
}
type AddPortResult struct {
	HostID  int    `json:"host_id"`
	Type    string `json:"type"`
	Address string `json:"address"`
}

type MapVolumeToHostRequest struct {
	VolumeID int `json:"volume_id"`
}

type MapVolumeToHostResponse struct {
	Metadata Metadata `json:"metadata"`
	Result   LunInfo  `json:"result"`
	Error    Error    `json:"error"`
}

func (iboxClient *IboxClient) GetAllHosts() (host []Host, err error) {
	url := iboxClient.Creds.Url + "api/rest/hosts"
	iboxClient.Log.V(TRACE_LEVEL).Info("GetAllHosts", "URL", url)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return host, fmt.Errorf("GetAllHosts - NewRequest - error %w", err)
	}
	SetAuthHeader(req, iboxClient.Creds)

	resp, err := iboxClient.HttpClient.Do(req)
	if err != nil {
		return host, fmt.Errorf("GetAllHosts - Do - error %w", err)
	}
	defer resp.Body.Close()
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return host, fmt.Errorf("GetAllHosts - ReadAll - error %w", err)
	}
	var responseObject HostResponse
	err = json.Unmarshal(bodyBytes, &responseObject)
	if err != nil {
		return host, fmt.Errorf("GetAllHosts - Unmarshal - error %w", err)
	}
	if responseObject.Error.Code != "" {
		return host, fmt.Errorf("GetAllHosts - ibox API - error code: %s message: %s", responseObject.Error.Code, responseObject.Error.Message)
	}

	return responseObject.Result, nil
}

func (iboxClient *IboxClient) GetHostByName(hostName string) (host *Host, err error) {
	url := iboxClient.Creds.Url + "api/rest/hosts"
	iboxClient.Log.V(TRACE_LEVEL).Info("GetHostByName", "URL", url, "host name", hostName)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("GetHostByName - NewRequest - error %w", err)
	}
	SetAuthHeader(req, iboxClient.Creds)

	values := req.URL.Query()
	values.Add("name", hostName)
	req.URL.RawQuery = values.Encode()

	resp, err := iboxClient.HttpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GetHostByName - Do - error %w", err)
	}
	defer resp.Body.Close()
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("GetHostByName - ReadAll - error %w", err)
	}
	var responseObject HostResponse
	err = json.Unmarshal(bodyBytes, &responseObject)
	if err != nil {
		return nil, fmt.Errorf("GetHostByName - Unmarshal - error %w", err)
	}
	if len(responseObject.Result) == 0 {
		return nil, &IboxAPIError{Code: IBOXAPI_RESOURCE_NOT_FOUND_ERROR, Err: fmt.Errorf("GetHostByName - host '%s' not found", hostName)}
	}
	if responseObject.Error.Code != "" {
		return nil, fmt.Errorf("GetHostByName - ibox API - error code: %s message: %s", responseObject.Error.Code, responseObject.Error.Message)

	}
	return &responseObject.Result[0], nil
}

func (iboxClient *IboxClient) CreateHost(hostName string) (host *Host, err error) {

	URL := iboxClient.Creds.Url + "api/rest/hosts"
	iboxClient.Log.V(TRACE_LEVEL).Info("CreateHost", "URL", URL, "host name", hostName)

	hp := CreateHostPost{
		Name: hostName,
	}
	jsonBytes, err := json.Marshal(hp)
	if err != nil {
		return nil, fmt.Errorf("CreateHost - Marshal - error %w", err)
	}
	request, err := http.NewRequest(http.MethodPost, URL, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return nil, fmt.Errorf("CreateHost - NewRequest - error %w", err)
	}
	SetAuthHeader(request, iboxClient.Creds)
	request.Header.Set("Content-Type", "application/json; charset=UTF-8")

	response, err := iboxClient.HttpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("CreateHost - Do - error %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("CreateHost -ReadAll - error %w", err)
	}

	var responseObject CreateHostResponse
	err = json.Unmarshal(body, &responseObject)
	if err != nil {
		return nil, fmt.Errorf("CreateHost - Unmarshal - error %w", err)
	}

	if responseObject.Error.Code != "" {
		return nil, fmt.Errorf("CreateHost - ibox API - error code: %s message: %s", responseObject.Error.Code, responseObject.Error.Message)

	}
	return &responseObject.Result, nil
}

func (iboxClient *IboxClient) DeleteHost(hostID int) (response *Host, err error) {
	url := fmt.Sprintf("%s%s/%d", iboxClient.Creds.Url, "api/rest/hosts/", hostID)
	iboxClient.Log.V(TRACE_LEVEL).Info("DeleteHost", "URL", url, "host ID", hostID)

	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return nil, fmt.Errorf("DeleteHost - NewRquest -  error %w", err)
	}
	SetAuthHeader(req, iboxClient.Creds)

	resp, err := iboxClient.HttpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("DeleteHost - Do - error %w", err)
	}
	defer resp.Body.Close()
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("DeleteHost - ReadAll - error %w", err)
	}

	var responseObject DeleteHostResponse
	err = json.Unmarshal(bodyBytes, &responseObject)
	if err != nil {
		return nil, fmt.Errorf("DeleteHost - Unmarshal - error %w", err)
	}

	if responseObject.Error.Code != "" {
		if responseObject.Error.Code == "HOST_NOT_FOUND" {
			return nil, &IboxAPIError{Code: IBOXAPI_RESOURCE_NOT_FOUND_ERROR, Err: fmt.Errorf("DeleteHost - host ID '%d' not found", hostID)}
		}
		return nil, fmt.Errorf("DeleteHost - ibox API - error code: %s message: %s", responseObject.Error.Code, responseObject.Error.Message)
	}

	return &responseObject.Result, nil
}

func (iboxClient *IboxClient) AddHostSecurity(chapCreds map[string]string, hostID int) (host *AddHostSecurityResponse, err error) {
	url := fmt.Sprintf("%s%s/%d", iboxClient.Creds.Url, "api/rest/hosts/", hostID)
	iboxClient.Log.V(TRACE_LEVEL).Info("AddHostSecurity", "URL", url, "host ID", hostID)

	hp := AddHostSecurityRequest{
		SecurityMethod:               chapCreds[CHAP_SECURITY_METHOD],
		SecurityCHAPInboundUsername:  chapCreds[CHAP_INBOUND_USERNAME],
		SecurityCHAPInboundSecret:    chapCreds[CHAP_INBOUND_SECRET],
		SecurityCHAPOutboundUsername: chapCreds[CHAP_OUTBOUND_USERNAME],
		SecurityCHAPOutboundSecret:   chapCreds[CHAP_OUTBOUND_SECRET],
	}

	jsonBytes, err := json.Marshal(hp)
	if err != nil {
		return nil, fmt.Errorf("AddHostSecurity - Marshal - error %w", err)
	}
	request, err := http.NewRequest(http.MethodPut, url, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return nil, fmt.Errorf("AddHostSecurity - NewRequest - error %w", err)
	}

	values := request.URL.Query()
	values.Add("approved", "true")
	request.URL.RawQuery = values.Encode()

	SetAuthHeader(request, iboxClient.Creds)

	request.Header.Set("Content-Type", "application/json; charset=UTF-8")

	response, err := iboxClient.HttpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("AddHostSecurity - Do - error %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("AddHostSecurity - ReadAll - error %w", err)
	}

	var responseObject AddHostSecurityResponse
	err = json.Unmarshal(body, &responseObject)
	if err != nil {
		return nil, fmt.Errorf("AddHostSecurity - Unmarshal - error %w", err)
	}
	if responseObject.Error.Code != "" {
		return nil, fmt.Errorf("AddHostSecurity - ibox API - error:  code: %s message: %s", responseObject.Error.Code, responseObject.Error.Message)
	}
	return &responseObject, nil
}

func (iboxClient *IboxClient) AddHostPort(portType, portAddress string, hostID int) (addPortResponse *AddPortResponse, err error) {

	URL := fmt.Sprintf("%s/api/rest/hosts/%d/ports", iboxClient.Creds.Url, hostID)
	iboxClient.Log.V(TRACE_LEVEL).Info("AddHostPort", "URL", URL, "port type", portType, "port address", portAddress, "host ID", hostID)

	hp := AddPortRequest{
		Type:    portType,
		Address: portAddress,
	}

	jsonBytes, err := json.Marshal(hp)
	if err != nil {
		return nil, fmt.Errorf("AddHostPort - Marshal - error %w", err)
	}
	request, err := http.NewRequest(http.MethodPost, URL, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return nil, fmt.Errorf("AddHostPort - NewRequest - error %w", err)
	}

	values := request.URL.Query()
	values.Add("approved", "true")
	request.URL.RawQuery = values.Encode()

	SetAuthHeader(request, iboxClient.Creds)
	request.Header.Set("Content-Type", "application/json; charset=UTF-8")

	response, err := iboxClient.HttpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("AddHostPort - Do - error %w", err)
	}
	defer response.Body.Close()

	body, _ := io.ReadAll(response.Body)

	var responseObject AddPortResponse
	err = json.Unmarshal(body, &responseObject)
	if err != nil {
		return nil, fmt.Errorf("AddHostPort - Unmarshal -error %w", err)
	}
	return &responseObject, nil

}

func (iboxClient *IboxClient) GetHostPort(hostID int, portAddress string) (hostPort *HostPort, err error) {
	URL := fmt.Sprintf("%s/api/rest/hosts/%d/ports", iboxClient.Creds.Url, hostID)
	iboxClient.Log.V(TRACE_LEVEL).Info("GetHostPort", "URL", URL, "host ID", hostID, "port address", portAddress)

	req, err := http.NewRequest(http.MethodGet, URL, nil)
	if err != nil {
		return nil, fmt.Errorf("GetHostPort - NewRequest - error %w", err)
	}
	SetAuthHeader(req, iboxClient.Creds)

	resp, err := iboxClient.HttpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GetHostPort - Do - error %w", err)
	}
	defer resp.Body.Close()
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("GetHostPort - ReadAll - error %w", err)
	}
	var responseObject GetHostPortResponse
	err = json.Unmarshal(bodyBytes, &responseObject)
	if err != nil {
		return nil, fmt.Errorf("GetHostPort - Unmarshal - error %w", err)
	}

	var portFound bool
	for _, port := range responseObject.Result {
		if port.PortAddress == portAddress {
			hostPort = &port
			portFound = true
		}
	}
	if !portFound {
		return nil, &IboxAPIError{Code: IBOXAPI_RESOURCE_NOT_FOUND_ERROR, Err: fmt.Errorf("GetHostPort - portAddress '%s' not found", portAddress)}
	}
	if responseObject.Error.Code != "" {
		return nil, fmt.Errorf("GetHostPort - ibox API - error:  code: %s message: %s", responseObject.Error.Code, responseObject.Error.Message)
	}
	return hostPort, nil
}

func (iboxClient *IboxClient) MapVolumeToHost(hostID, volumeID, lun int) (lunInfo *LunInfo, err error) {

	URL := fmt.Sprintf("%s/api/rest/hosts/%d/luns", iboxClient.Creds.Url, hostID)
	iboxClient.Log.V(TRACE_LEVEL).Info("MapVolumeToHost", "URL", URL, "volume ID", volumeID, "lun", lun, "host ID", hostID)

	hp := MapVolumeToHostRequest{
		VolumeID: volumeID,
	}

	jsonBytes, err := json.Marshal(hp)
	if err != nil {
		return nil, fmt.Errorf("MapVolumeToHost - Marshal - error %w", err)
	}
	request, err := http.NewRequest(http.MethodPost, URL, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return nil, fmt.Errorf("MapVolumeToHost - NewRequest - error %w", err)
	}

	values := request.URL.Query()
	values.Add("approved", "true")
	request.URL.RawQuery = values.Encode()

	SetAuthHeader(request, iboxClient.Creds)
	request.Header.Set("Content-Type", "application/json; charset=UTF-8")

	response, err := iboxClient.HttpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("MapVolumeToHost - Do - error %w", err)
	}
	defer response.Body.Close()

	body, _ := io.ReadAll(response.Body)

	var responseObject MapVolumeToHostResponse
	err = json.Unmarshal(body, &responseObject)
	if err != nil {
		return nil, fmt.Errorf("MapVolumeToHost - Unmarshal - error %w", err)
	}
	if responseObject.Error.Code != "" {
		return nil, fmt.Errorf("MapVolumeToHost - ibox API - error:  code: %s message: %s", responseObject.Error.Code, responseObject.Error.Message)
	}
	return &responseObject.Result, nil

}

func (iboxClient *IboxClient) GetAllLunByHost(hostID int) (luns []LunInfo, err error) {
	url := fmt.Sprintf("%s%s/%d/luns", iboxClient.Creds.Url, "api/rest/hosts/", hostID)
	iboxClient.Log.V(TRACE_LEVEL).Info("GetAllLunByHost", "URL", url, "host ID", hostID)

	pageSize := common.IBOX_DEFAULT_QUERY_PAGE_SIZE
	totalPages := 1 // start with 1, update after first query.

	for page := 1; page <= totalPages; page++ {
		iboxClient.Log.V(TRACE_LEVEL).Info("GetAllLunByHost loop", "page", page, "totalPages", totalPages)
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return luns, fmt.Errorf("GetAllLunByHost - NewRequest - error %w", err)
		}
		values := req.URL.Query()
		values.Add("page_size", strconv.Itoa(pageSize))
		values.Add("page", strconv.Itoa(page))
		req.URL.RawQuery = values.Encode()
		iboxClient.Log.V(TRACE_LEVEL).Info("GetAllLunByHost loop", "page", page, "totalPages", totalPages, "URL", req.URL.RawQuery)

		SetAuthHeader(req, iboxClient.Creds)

		resp, err := iboxClient.HttpClient.Do(req)
		if err != nil {
			return luns, fmt.Errorf("GetAllLunByHost - Do - error %w", err)
		}
		defer resp.Body.Close()
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return luns, fmt.Errorf("GetAllLunByHost - ReadAll - error %w", err)
		}
		var responseObject GetAllLunsResponse
		err = json.Unmarshal(bodyBytes, &responseObject)
		if err != nil {
			return luns, fmt.Errorf("GetAllLunByHost - Unmarshal - error %w", err)
		}

		luns = append(luns, responseObject.Result...)

		if page == 1 {
			totalPages = responseObject.Metadata.PagesTotal
		}
	}

	return luns, nil
}

func (iboxClient *IboxClient) GetLunByHostVolume(hostID, volumeID int) (lun *LunInfo, err error) {
	url := fmt.Sprintf("%s%s/%d/luns", iboxClient.Creds.Url, "api/rest/hosts/", hostID)
	iboxClient.Log.V(TRACE_LEVEL).Info("GetLunByHostVolume", "URL", url, "host ID", hostID, "volume ID", volumeID)

	pageSize := common.IBOX_DEFAULT_QUERY_PAGE_SIZE
	totalPages := 1 // start with 1, update after first query.

	for page := 1; page <= totalPages; page++ {
		iboxClient.Log.V(TRACE_LEVEL).Info("GetLunByHostVolume loop", "page", page, "totalPages", totalPages)
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("GetLunByHostVolume - NewRequest - error %w", err)
		}
		values := req.URL.Query()
		values.Add("volume_id", strconv.Itoa(volumeID))
		values.Add("page_size", strconv.Itoa(pageSize))
		values.Add("page", strconv.Itoa(page))
		req.URL.RawQuery = values.Encode()
		iboxClient.Log.V(TRACE_LEVEL).Info("GetLunByHostVolume loop", "page", page, "totalPages", totalPages, "URL", req.URL.RawQuery)

		SetAuthHeader(req, iboxClient.Creds)

		resp, err := iboxClient.HttpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("GetLunByHostVolume - Do - error %w", err)
		}
		defer resp.Body.Close()
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("GetLunByHostVolume - ReadAll - error %w", err)
		}
		var responseObject GetAllLunsResponse
		err = json.Unmarshal(bodyBytes, &responseObject)
		if err != nil {
			return nil, fmt.Errorf("GetLunByHostVolume - Unmarshal - error %w", err)
		}

		if page == 1 {
			totalPages = responseObject.Metadata.PagesTotal
		}

		if len(responseObject.Result) > 0 {
			lun = &responseObject.Result[0]
			break
		}
	}

	if lun == nil {
		return nil, &IboxAPIError{Code: IBOXAPI_RESOURCE_NOT_FOUND_ERROR, Err: fmt.Errorf("GetLunByHostVolume - host ID '%d' volume ID '%d' not found", hostID, volumeID)}
	}

	return lun, nil
}

func (iboxClient *IboxClient) UnMapVolumeFromHost(hostID, volumeID int) (unmapResponse *UnMapVolumeFromHostResponse, err error) {
	url := fmt.Sprintf("%s%s/%d/luns/volume_id/%d", iboxClient.Creds.Url, "api/rest/hosts/", hostID, volumeID)
	iboxClient.Log.V(TRACE_LEVEL).Info("UnMapVolumeFromHost", "URL", url, "host ID", hostID, "volume ID", volumeID)

	request, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return nil, fmt.Errorf("UnMapVolumeFromHost - NewRequest - error %w", err)
	}

	values := request.URL.Query()
	values.Add("approved", "true")
	request.URL.RawQuery = values.Encode()

	SetAuthHeader(request, iboxClient.Creds)

	resp, err := iboxClient.HttpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("UnMapVolumeFromHost - Do - error %w", err)
	}
	defer resp.Body.Close()
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("UnMapVolumeFromHost - ReadAll - error %w", err)
	}

	var responseObject UnMapVolumeFromHostResponse
	err = json.Unmarshal(bodyBytes, &responseObject)
	if err != nil {
		return nil, fmt.Errorf("UnMapVolumeFromHost - Unmarshal - error %w", err)
	}
	if responseObject.Error.Code != "" {
		return nil, fmt.Errorf("UnMapVolumeFromHost - ibox API - error:  code: %s message: %s", responseObject.Error.Code, responseObject.Error.Message)
	}
	return &responseObject, nil
}
