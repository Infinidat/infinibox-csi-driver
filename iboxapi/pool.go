package iboxapi

import (
	"encoding/json"
	"fmt"
	"infinibox-csi-driver/common"
	"io"
	"net/http"
	"strconv"
)

type GetPoolByNameResponse struct {
	Metadata Metadata     `json:"metadata"`
	Result   []PoolResult `json:"result"`
	Error    Error        `json:"error"`
}
type PoolResult struct {
	VolumesCount                     int     `json:"volumes_count"`
	StandardEntitiesCount            int     `json:"standard_entities_count"`
	UpdatedAt                        int64   `json:"updated_at"`
	StandardFilesystemSnapshotsCount int     `json:"standard_filesystem_snapshots_count"`
	StandardSnapshotsCount           int     `json:"standard_snapshots_count"`
	MaxExtend                        int     `json:"max_extend"`
	AllocatedPhysicalSpace           int     `json:"allocated_physical_space"`
	FreeVirtualSpace                 int64   `json:"free_virtual_space"`
	StandardFilesystemsCount         int     `json:"standard_filesystems_count"`
	ID                               int     `json:"id"`
	ReservedCapacity                 int64   `json:"reserved_capacity"`
	FilesystemsCount                 int     `json:"filesystems_count"`
	SsdEnabled                       bool    `json:"ssd_enabled"`
	VvolEntitiesCount                int     `json:"vvol_entities_count"`
	SnapshotsCount                   int     `json:"snapshots_count"`
	State                            string  `json:"state"`
	VvolVolumesCount                 int     `json:"vvol_volumes_count"`
	Type                             string  `json:"type"`
	FreePhysicalSpace                int64   `json:"free_physical_space"`
	DataReductionRatio               float64 `json:"data_reduction_ratio"`
	TotalDiskUsage                   any     `json:"total_disk_usage"`
	VvolSnapshotsCount               int     `json:"vvol_snapshots_count"`
	ThinCapacitySavings              any     `json:"thin_capacity_savings"`
	EntitiesCount                    int     `json:"entities_count"`
	PhysicalCapacityCritical         int     `json:"physical_capacity_critical"`
	StandardVolumesCount             int     `json:"standard_volumes_count"`
	Owners                           []any   `json:"owners"`
	CapacitySavings                  any     `json:"capacity_savings"`
	Name                             string  `json:"name"`
	VirtualCapacity                  int64   `json:"virtual_capacity"`
	TenantID                         int     `json:"tenant_id"`
	CreatedAt                        int64   `json:"created_at"`
	FilesystemSnapshotsCount         int     `json:"filesystem_snapshots_count"`
	CompressionEnabled               bool    `json:"compression_enabled"`
	QosPolicies                      []any   `json:"qos_policies"`
	PhysicalCapacityWarning          int     `json:"physical_capacity_warning"`
	PhysicalCapacity                 int64   `json:"physical_capacity"`
	ThickCapacitySavings             any     `json:"thick_capacity_savings"`
}

func (client *IboxClient) GetPoolByName(name string) (pool *PoolResult, err error) {
	url := fmt.Sprintf("%s/api/rest/pools", client.Creds.Url)
	client.Log.V(DEBUG_LEVEL).Info("GetPoolByName", "URL", url, "name", name)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("error in NewRequest %w", err)
	}
	values := req.URL.Query()
	values.Add("page_size", strconv.Itoa(common.IBOX_DEFAULT_QUERY_PAGE_SIZE))
	values.Add("page", strconv.Itoa(1))
	values.Add("name", name)
	req.URL.RawQuery = values.Encode()

	SetAuthHeader(req, client.Creds)

	resp, err := client.HttpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error with client.Do %w", err)
	}
	defer resp.Body.Close()
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body %w", err)
	}
	var responseObject GetPoolByNameResponse
	err = json.Unmarshal(bodyBytes, &responseObject)
	if err != nil {
		return nil, fmt.Errorf("error in Unmarshal %w", err)
	}

	if len(responseObject.Result) > 0 {
		pool = &responseObject.Result[0]
	} else {
		return nil, ErrNotFound
	}

	return pool, nil
}
