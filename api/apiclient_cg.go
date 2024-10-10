/*
Copyright 2024 Infinidat
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
package api

import (
	"fmt"
	"infinibox-csi-driver/api/client"
	"infinibox-csi-driver/common"
	"net/http"
	"strconv"
)

type CGInfo struct {
	CreatedBySnapshotPolicyID   int      `json:"created_by_snapshot_policy_id,omitempty"`
	UpdateAt                    int      `json:"updated_at,omitempty"`
	SnapshotRetention           int      `json:"snapshot_retention,omitempty"`
	CreatedBySnapshotPolicyName string   `json:"created_by_snapshot_policy_name,omitempty"`
	MembersCount                int      `json:"members_count,omitempty"`
	SnapshotPolicyName          string   `json:"snapshot_policy_name,omitempty"`
	ID                          int      `json:"id,omitempty"`
	ParentID                    int      `json:"parent_id,omitempty"`
	LockState                   string   `json:"lock_state,omitempty"`
	CreatedByScheduleID         int      `json:"created_by_schedule_id,omitempty"`
	CGType                      string   `json:"type,omitempty"`
	PoolName                    string   `json:"pool_name,omitempty"`
	ReplicationTypes            []string `json:"replication_types,omitempty"`
	HasChildren                 bool     `json:"has_children,omitempty"`
	LockExpiresAt               int      `json:"lock_expires_at,omitempty"`
	CreatedByScheduleName       string   `json:"created_by_schedule_name,omitempty"`
	RmrSnapshotGUID             string   `json:"rmr_snapshot_guid,omitempty"`
	Name                        string   `json:"name,omitempty"`
	TenantID                    int      `json:"tenant_id,omitempty"`
	CreatedAt                   int      `json:"created_at,omitempty"`
	SnapshotExpiresAt           int      `json:"snapshot_expires_at,omitempty"`
	PoolID                      int      `json:"pool_id,omitempty"`
	SnapshotPolicyID            int      `json:"snapshot_policy_id,omitempty"`
	IsReplicated                bool     `json:"is_replicated,omitempty"`
}
type MemberInfo struct {
	MemberType                          string   `json:"type,omitempty"`
	Depth                               int      `json:"depth,omitempty"`
	ID                                  int      `json:"id,omitempty"`
	Name                                string   `json:"name,omitempty"`
	CreatedAt                           int      `json:"created_at,omitempty"`
	UpdateAt                            int      `json:"updated_at,omitempty"`
	Mapped                              bool     `json:"mapped,omitempty"`
	WriteProtected                      bool     `json:"write_protected,omitempty"`
	Size                                int      `json:"size,omitempty"`
	ProvType                            string   `json:"provtype,omitempty"`
	SSDEnabled                          bool     `json:"ssd_enabled,omitempty"`
	SSAExpressEnabled                   bool     `json:"ssa_express_enabled,omitempty"`
	SSAExpressStatus                    string   `json:"ssa_express_status,omitempty"`
	CompressionEnabled                  bool     `json:"compression_enabled,omitempty"`
	Serial                              string   `json:"serial,omitempty"`
	RmrTarget                           bool     `json:"rmr_target,omitempty"`
	RMRSource                           bool     `json:"rmr_source,omitempty"`
	RMRActiveActivePeer                 bool     `json:"rmr_active_active_peer,omitempty"`
	MobilitySource                      string   `json:"mobility_source,omitempty"`
	RMRSnapshotGUID                     string   `json:"rmr_snapshot_guid,omitempty"`
	DataSnapshotGUID                    string   `json:"data_snapshot_guid,omitempty"`
	MgmtSnapshotGUID                    string   `json:"mgmt_snapshot_guid,omitempty"`
	CGSnapshotGUID                      string   `json:"_cg_snapshot_guid,omitempty"`
	CGGUID                              string   `json:"_cg_guid,omitempty"`
	FamilyID                            int      `json:"family_id,omitempty"`
	LockExpiresAt                       int      `json:"lock_expires_at,omitempty"`
	ReclaimedSnapshotRemoteSystemSerial string   `json:"_reclaimed_snapshot_remote_system_serial,omitempty"`
	SnapshotRetention                   string   `json:"snapshot_retention,omitempty"`
	DatasetType                         string   `json:"dataset_type,omitempty"`
	Used                                int      `json:"used,omitempty"`
	TreeAllocated                       int      `json:"tree_allocated,omitempty"`
	Allocated                           int      `json:"allocated,omitempty"`
	CompressionSuppressed               bool     `json:"compression_suppressed,omitempty"`
	CapacitySaving                      int      `json:"capacity_savings,omitempty"`
	UDID                                int      `json:"udid,omitempty"`
	PathsAvailable                      bool     `json:"paths_available,omitempty"`
	SourceReplicatedSGID                int      `json:"source_replicated_sg_id,omitempty"`
	PoolID                              int      `json:"pool_id,omitempty"`
	ParentID                            int      `json:"parent_id,omitempty"`
	CGName                              string   `json:"cg_name,omitempty"`
	CGID                                int      `json:"cg_id,omitempty"`
	SnapshotPolicyID                    int      `json:"snapshot_policy_id,omitempty"`
	HasChildren                         bool     `json:"has_children,omitempty"`
	SnapshotExpiresAt                   int      `json:"snapshot_expires_at,omitempty"`
	CreatedBySnapshotPolicyID           int      `json:"created_by_snapshot_policy_id,omitempty"`
	CreatedByScheduleID                 int      `json:"created_by_schedule_id,omitempty"`
	TenantID                            int      `json:"tenant_id,omitempty"`
	QOSPolicyName                       string   `json:"qos_policy_name,omitempty"`
	PoolName                            string   `json:"pool_name,omitempty"`
	NGUID                               string   `json:"nguid,omitempty"`
	ReplicaIDs                          []int    `json:"replica_ids,omitempty"`
	ReplicationTypes                    []string `json:"replication_types,omitempty"`
	NumBlocks                           int      `json:"num_blocks,omitempty"`
	QOSPolicyID                         int      `json:"qos_policy_id,omitempty"`
	QOSSharedPolicyID                   int      `json:"qos_shared_policy_id,omitempty"`
	QOSSharedPolicyName                 string   `json:"qos_shared_policy_name,omitempty"`
	LockState                           string   `json:"lock_state,omitempty"`
	SnapshotPolicyName                  string   `json:"snapshot_policy_name,omitempty"`
	CreatedBySnapshotPolicyName         string   `json:"created_by_snapshot_policy_name,omitempty"`
	CreatedByScheduleName               string   `json:"created_by_schedule_name,omitempty"`
}

// CreateCG creates a consistency group (aka volume group)
func (c *ClientService) CreateCG(poolID int, cgName string) (newCG CGInfo, err error) {
	zlog.Trace().Msgf("CreateCG Name poolID %d cdName %s", poolID, cgName)

	path := "/api/rest/cgs?replicate_to_async_target=false"
	body := map[string]interface{}{
		"name":    cgName,
		"pool_id": poolID,
	}

	_, err = c.getJSONResponse(http.MethodPost, path, body, &newCG)

	if err != nil {
		return newCG, err
	}

	zlog.Trace().Msgf("CreateCG Name : poolID %d cdName %s newCG ID %d", poolID, cgName, newCG.ID)
	return newCG, nil

}

// AddMemberToSnapshotGroup adds a dataset into a consistency group
func (c *ClientService) AddMemberToSnapshotGroup(volumeID int, cgID int) (err error) {
	zlog.Trace().Msgf("AddMemberToSnapshotGroup volume ID %d cg ID %d", volumeID, cgID)

	body := map[string]interface{}{
		"dataset_id": volumeID,
	}

	path := fmt.Sprintf("/api/rest/cgs/%s/members", strconv.Itoa(cgID))
	info := CGInfo{}
	resp, err := c.getJSONResponse(http.MethodPost, path, body, &info)
	if err != nil {
		return err
	}

	zlog.Trace().Msgf("AddMemberToSnapshotGroup volume ID %d cg ID %d completed response %v", volumeID, cgID, resp)
	return nil

}

// RemoveMemberFromSnapshotGroup removes a dataset from a consistency group
func (c *ClientService) RemoveMemberFromSnapshotGroup(volumeID int, cgID int) (err error) {
	zlog.Trace().Msgf("RemoveMemberFromSnapshotGroup volume ID %d cg ID %d", volumeID, cgID)

	path := fmt.Sprintf("/api/rest/cgs/%s/members/%s?approved=true", strconv.Itoa(cgID), strconv.Itoa(volumeID))
	_, err = c.getJSONResponse(http.MethodDelete, path, nil, nil)
	if err != nil {
		return err
	}
	zlog.Trace().Msgf("RemoveMemberFromSnapshotGroup volume ID %d cg %d", volumeID, cgID)
	return nil

}

// GetAllCG gets all the consistency groups from the ibox
func (c *ClientService) GetAllCG() (cgInfo []CGInfo, err error) {
	zlog.Trace().Msg("GetAllCG")

	page := 1
	page_size := common.IBOX_DEFAULT_QUERY_PAGE_SIZE
	total_pages := 1 // start with 1, update after first query.

	for ok := true; ok; ok = page <= total_pages {
		uri := fmt.Sprintf("api/rest/cgs?page_size=%s&page=%s", strconv.Itoa(page_size), strconv.Itoa(page))

		resp, err := c.getResponseWithQueryString(uri, nil, &cgInfo)

		if err != nil {
			zlog.Error().Msgf("failed to get CGs with error %v", err)
			return cgInfo, err
		}

		apiresp := resp.(client.ApiResponse)
		currentResults, _ := apiresp.Result.([]CGInfo)
		cgInfo = append(cgInfo, currentResults...)
		responseSize := apiresp.MetaData.NoOfObject
		zlog.Trace().Msgf("added %d CGs to results", responseSize)
		if page == 1 {
			total_pages = apiresp.MetaData.TotalPages
		}
		page++
	}

	zlog.Trace().Msgf("GetAllCG completed with %d results", len(cgInfo))
	return cgInfo, nil

}

// GetMembersByCGID gets all the datasets for a consistency group by its ID
func (c *ClientService) GetMembersByCGID(cgID int) (memberInfo []MemberInfo, err error) {
	zlog.Trace().Msgf("GetMembersByCGID cg ID %d", cgID)

	page := 1
	page_size := common.IBOX_DEFAULT_QUERY_PAGE_SIZE
	total_pages := 1 // start with 1, update after first query.

	for ok := true; ok; ok = page <= total_pages {
		uri := fmt.Sprintf("api/rest/cgs/%s/members?page_size=%s&page=%s", strconv.Itoa(cgID), strconv.Itoa(page_size), strconv.Itoa(page))

		resp, err := c.getResponseWithQueryString(uri, nil, &memberInfo)

		if err != nil {
			zlog.Error().Msgf("failed to get members by CG with error %v", err)
			return memberInfo, err
		}

		apiresp := resp.(client.ApiResponse)
		currentResults, _ := apiresp.Result.([]MemberInfo)
		memberInfo = append(memberInfo, currentResults...)
		responseSize := apiresp.MetaData.NoOfObject
		zlog.Trace().Msgf("added %d members to results", responseSize)
		if page == 1 {
			total_pages = apiresp.MetaData.TotalPages
		}
		page++
	}
	zlog.Trace().Msgf("GetMembersByCGID completed with %d results for cg ID %d", len(memberInfo), cgID)
	return memberInfo, nil
}

// CreateSnapshotGroup creates a snapshot group
func (c *ClientService) CreateSnapshotGroup(cgID int, snapName, snapPrefix, snapSuffix string) (newCG CGInfo, err error) {
	zlog.Trace().Msgf("CreateSnapshotGroup Name cgID %d snapName %s snapPrefix %s snapSuffix %s", cgID, snapName, snapPrefix, snapSuffix)

	path := "/api/rest/cgs"
	body := map[string]interface{}{
		"name":        snapName,
		"parent_id":   cgID,
		"snap_prefix": snapPrefix,
		"snap_suffix": snapSuffix,
	}

	_, err = c.getJSONResponse(http.MethodPost, path, body, &newCG)
	if err != nil {
		return newCG, err
	}

	zlog.Trace().Msgf("CreateSnapshotGroup Name : cgID %d snapName: %s", cgID, snapName)
	return newCG, nil

}

// GetCG gets the consistency group (by name) from the ibox
func (c *ClientService) GetCG(name string) (cg CGInfo, err error) {
	cgInfo := make([]CGInfo, 0)
	zlog.Trace().Msgf("GetCG %s", name)

	page := 1
	page_size := common.IBOX_DEFAULT_QUERY_PAGE_SIZE
	total_pages := 1 // start with 1, update after first query.

	for ok := true; ok; ok = page <= total_pages {
		uri := fmt.Sprintf("api/rest/cgs?name=%s&page_size=%s&page=%s", name, strconv.Itoa(page_size), strconv.Itoa(page))

		resp, err := c.getResponseWithQueryString(uri, nil, &cgInfo)

		if err != nil {
			zlog.Error().Msgf("failed to get CG with error %v", err)
			return cg, err
		}

		apiresp := resp.(client.ApiResponse)
		currentResults, _ := apiresp.Result.([]CGInfo)
		cgInfo = append(cgInfo, currentResults...)
		responseSize := apiresp.MetaData.NoOfObject
		zlog.Trace().Msgf("added %d CG to results", responseSize)
		if page == 1 {
			total_pages = apiresp.MetaData.TotalPages
		}
		page++
	}

	if len(cgInfo) == 0 {
		return cg, fmt.Errorf("%s cg not found", name)
	}
	zlog.Trace().Msgf("GetCG %s completed with %d results", name, len(cgInfo))
	cg = cgInfo[0]
	return cg, nil

}

// DeleteSG removes a snap group (aka consistency group),
// we will choose to remove its snapshot members as well
func (c *ClientService) DeleteSG(sgID int) (err error) {
	zlog.Trace().Msgf("DeleteSG sg ID %d", sgID)

	path := fmt.Sprintf("/api/rest/cgs/%s?approved=true&delete_members=true", strconv.Itoa(sgID))
	_, err = c.getJSONResponse(http.MethodDelete, path, nil, nil)
	if err != nil {
		return err
	}
	zlog.Trace().Msgf("DeleteSG cg %d", sgID)
	return nil

}

// GetCGByID gets the consistency group (by ID) from the ibox
func (c *ClientService) GetCGByID(id int) (*CGInfo, error) {
	cg := CGInfo{}
	path := "/api/rest/cgs/" + strconv.Itoa(id)
	resp, err := c.getJSONResponse(http.MethodGet, path, nil, &cg)
	if err != nil {
		return nil, err
	}
	apiresp := resp.(client.ApiResponse)
	cg, _ = apiresp.Result.(CGInfo)
	return &cg, nil
}
