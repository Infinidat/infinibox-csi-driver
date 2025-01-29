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

/**
log levels
logr.V(0) - Info level logging in zerolog
logr.V(1) - Debug level logging in zerolog
logr.V(2) - Trace level logging in zerolog
*/

type FileSystem struct {
	Type                                string   `json:"type"`
	Depth                               int      `json:"depth"`
	ID                                  int      `json:"id"`
	Name                                string   `json:"name"`
	CreatedAt                           int64    `json:"created_at"`
	UpdatedAt                           int64    `json:"updated_at"`
	Mapped                              bool     `json:"mapped"`
	WriteProtected                      bool     `json:"write_protected"`
	Size                                int      `json:"size"`
	Provtype                            string   `json:"provtype"`
	SsdEnabled                          bool     `json:"ssd_enabled"`
	SsaExpressEnabled                   bool     `json:"ssa_express_enabled"`
	SsaExpressStatus                    any      `json:"ssa_express_status"`
	CompressionEnabled                  bool     `json:"compression_enabled"`
	Serial                              string   `json:"serial"`
	RmrTarget                           bool     `json:"rmr_target"`
	RmrSource                           bool     `json:"rmr_source"`
	RmrActiveActivePeer                 bool     `json:"rmr_active_active_peer"`
	MobilitySource                      any      `json:"mobility_source"`
	RmrSnapshotGUID                     any      `json:"rmr_snapshot_guid"`
	DataSnapshotGUID                    any      `json:"data_snapshot_guid"`
	MgmtSnapshotGUID                    any      `json:"mgmt_snapshot_guid"`
	CgSnapshotGUID                      any      `json:"_cg_snapshot_guid"`
	CgGUID                              any      `json:"_cg_guid"`
	FamilyID                            int      `json:"family_id"`
	LockExpiresAt                       any      `json:"lock_expires_at"`
	ReclaimedSnapshotRemoteSystemSerial any      `json:"_reclaimed_snapshot_remote_system_serial"`
	SnapshotRetention                   any      `json:"snapshot_retention"`
	DatasetType                         string   `json:"dataset_type"`
	Used                                int      `json:"used"`
	TreeAllocated                       int      `json:"tree_allocated"`
	Allocated                           int      `json:"allocated"`
	CompressionSuppressed               bool     `json:"compression_suppressed"`
	CapacitySavings                     int      `json:"capacity_savings"`
	CapacitySavingsPerEntity            int      `json:"capacity_savings_per_entity"`
	DiskUsage                           int      `json:"disk_usage"`
	DataReductionRatio                  float64  `json:"data_reduction_ratio"`
	WormLegalHold                       any      `json:"worm_legal_hold"`
	WormDefaultRetention                any      `json:"worm_default_retention"`
	WormMaxRetention                    any      `json:"worm_max_retention"`
	NfsFilesystemID                     int      `json:"nfs_filesystem_id"`
	AtimeMode                           string   `json:"atime_mode"`
	IsConsistent                        bool     `json:"is_consistent"`
	IsEstablished                       bool     `json:"_is_established"`
	SnapdirName                         string   `json:"snapdir_name"`
	VisibleInSnapdir                    bool     `json:"visible_in_snapdir"`
	SnapdirAccessible                   bool     `json:"snapdir_accessible"`
	SuspendState                        string   `json:"suspend_state"`
	SecurityStyle                       string   `json:"security_style"`
	AtimeGranularity                    int      `json:"atime_granularity"`
	WormLevel                           string   `json:"worm_level"`
	ParentID                            int      `json:"parent_id"`
	Modified                            bool     `json:"modified"`
	Data                                int      `json:"data"`
	PoolID                              int      `json:"pool_id"`
	CgName                              any      `json:"cg_name"`
	CgID                                any      `json:"cg_id"`
	HasChildren                         bool     `json:"has_children"`
	SnapshotPolicyID                    any      `json:"snapshot_policy_id"`
	SnapshotExpiresAt                   any      `json:"snapshot_expires_at"`
	CreatedBySnapshotPolicyID           any      `json:"created_by_snapshot_policy_id"`
	CreatedByScheduleID                 any      `json:"created_by_schedule_id"`
	TenantID                            int      `json:"tenant_id"`
	QosPolicyName                       any      `json:"qos_policy_name"`
	LockState                           string   `json:"lock_state"`
	SnapshotPolicyName                  any      `json:"snapshot_policy_name"`
	CreatedBySnapshotPolicyName         any      `json:"created_by_snapshot_policy_name"`
	CreatedByScheduleName               any      `json:"created_by_schedule_name"`
	QosPolicyID                         any      `json:"qos_policy_id"`
	QosSharedPolicyID                   any      `json:"qos_shared_policy_id"`
	QosSharedPolicyName                 any      `json:"qos_shared_policy_name"`
	PoolName                            string   `json:"pool_name"`
	Nguid                               string   `json:"nguid"`
	ReplicaIds                          []any    `json:"replica_ids"`
	ReplicationTypes                    []string `json:"replication_types"`
	NumBlocks                           int      `json:"num_blocks"`
}

type GetFileSystemsByPoolResponse struct {
	Metadata Metadata     `json:"metadata"`
	Result   []FileSystem `json:"result"`
	Error    Error        `json:"error"`
}

func (iboxClient *IboxClient) GetFileSystemsByPool(poolID int, fsPrefix string) (results []FileSystem, err error) {
	URL := fmt.Sprintf("%sapi/rest/filesystems", iboxClient.Creds.Url)
	iboxClient.Log.V(TRACE_LEVEL).Info("GetFileSystemsByPool", "URL", URL, "pool ID", poolID, "fsprefix", fsPrefix)

	pageSize := common.IBOX_DEFAULT_QUERY_PAGE_SIZE
	totalPages := 1 // start with 1, update after first query.
	for page := 1; page <= totalPages; page++ {
		iboxClient.Log.V(TRACE_LEVEL).Info("GetFileSystemsByPool loop", "page", page, "totalPages", totalPages)

		req, err := http.NewRequest("GET", URL, nil)
		if err != nil {
			return results, fmt.Errorf("GetFileSystemsByPool - NewRequest - error %w", err)
		}

		values := req.URL.Query()
		values.Add("pool_id", strconv.Itoa(poolID))
		values.Add("name", "like:"+fsPrefix)
		values.Add("page_size", strconv.Itoa(pageSize))
		values.Add("page", strconv.Itoa(page))
		req.URL.RawQuery = values.Encode()

		SetAuthHeader(req, iboxClient.Creds)

		resp, err := iboxClient.HttpClient.Do(req)
		if err != nil {
			return results, fmt.Errorf("GetFileSystemsByPool - Do - error %w", err)
		}
		defer resp.Body.Close()
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return results, fmt.Errorf("GetFileSystemsByPool - ReadAll - error %w", err)
		}
		var responseObject GetFileSystemsByPoolResponse
		err = json.Unmarshal(bodyBytes, &responseObject)
		if err != nil {
			return results, fmt.Errorf("GetFileSystemsByPool - Unmarshal - error %w", err)
		}
		results = append(results, responseObject.Result...)

		if page == 1 {
			totalPages = responseObject.Metadata.PagesTotal
		}
	}

	return results, nil
}
