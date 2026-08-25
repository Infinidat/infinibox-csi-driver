/*
Copyright 2026 infinidat

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

package addons

import (
	"context"
	"log/slog"
	"strconv"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/csi-addons/spec/lib/go/volumegroup"
	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/infinidat/infinibox-csi-driver/iboxapi"
	storagecommon "github.com/infinidat/infinibox-csi-driver/storage/common"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// VolumeGroupServer struct of rbd CSI driver with supported methods of
// VolumeGroup controller server spec.
type VolumeGroupServer struct {
	// added UnimplementedControllerServer as a member of ControllerServer.
	// if volumegroup spec add more RPC services in the proto file, then we
	// don't need to add all RPC methods leading to forward compatibility.
	*volumegroup.UnimplementedControllerServer

	// driverInstance is the unique ID for this CSI-driver deployment.
	driverInstance string
}

// NewVolumeGroupServer creates a new VolumeGroupServer which handles the
// VolumeGroup Service requests from the CSI-Addons specification.
func NewVolumeGroupServer(instanceID string) *VolumeGroupServer {
	return &VolumeGroupServer{
		driverInstance: instanceID,
	}
}

func (vs *VolumeGroupServer) RegisterService(server grpc.ServiceRegistrar) {
	slog.Debug("AddOns VolumeGroupServer", "RegisterService", "started")
	volumegroup.RegisterControllerServer(server, vs)
}

// CreateVolumeGroup RPC call to create a volume group.
//
// From the spec:
// This RPC will be called by the CO to create a new volume group on behalf of
// a user. This operation MUST be idempotent. If a volume group corresponding
// to the specified volume group name already exists, is compatible with the
// specified parameters in the CreateVolumeGroupRequest, the Plugin MUST reply
// 0 OK with the corresponding CreateVolumeGroupResponse. CSI Plugins MAY
// create the following types of volume groups:
//
// Create a new empty volume group or a group with specific volumes. Note that
// N volumes with some backend label Y could be considered to be in "group Y"
// which might not be a physical group on the storage backend. In this case, an
// empty group can still be created by the CO to hold volumes. After the empty
// group is created, create a new volume. CO may call
// ModifyVolumeGroupMembership to add new volumes to the group.
//
// Implementation steps:
// 1. resolve all volumes given in the volume_ids list (can be empty)
// 2. check if the volumes belong to a (and all the same) group
// 3. create the Volume Group
// 4. verify that the Volume Group contains all the images (if it pre-exists)
// 5. add all volumes to the Volume Group
//
// Idempotency should be handled by the rbd.Manager, keeping this function and
// the potential error handling as simple as possible.
//
//nolint:gocyclo,cyclop // FIXME: make this function simpler
func (vs *VolumeGroupServer) CreateVolumeGroup(
	ctx context.Context,
	req *volumegroup.CreateVolumeGroupRequest,
) (*volumegroup.CreateVolumeGroupResponse, error) {
	slog.Debug("AddOns VolumeGroupServer", "CreateVolumeGroup", "started")

	config := map[string]string{}
	commonService, err := storagecommon.BuildCommonService(config, req.Secrets, nil)
	if err != nil {
		e := common.Errorf("error building commonService - volumeGroup: %s error: %w", req.Name, err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Unauthenticated, e.Error())
	}

	poolName := req.Parameters[common.StorageClassPoolName]
	if poolName == "" {
		e := common.Errorf("error poolName is empty and required to create a CG: %s", poolName)
		slog.Error(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	pool, err := commonService.IboxAPI.GetPoolByName(ctx, poolName)
	if err != nil {
		e := common.Errorf("error getting pool - poolName: %s error: %w", poolName, err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Unknown, e.Error())
	}

	cgRequest := iboxapi.CreateConsistencyGroupRequest{
		Name:   req.Name,
		PoolID: pool.ID,
	}

	cgInfo, err := commonService.IboxAPI.CreateConsistencyGroup(ctx, cgRequest)
	if err != nil {
		e := common.Errorf("error creating CG - request: %v error: %w", cgInfo, err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Unknown, e.Error())
	}

	// optionally add volumes to the CG
	csiVolumes := make([]*csi.Volume, 0)
	volumeIDs := req.VolumeIds
	if len(volumeIDs) > 0 {
		for _, v := range volumeIDs {
			vID, err := strconv.Atoi(v)
			if err != nil {
				e := common.Errorf("error converting volumeID string to int - volumeID: %s CG: %s error: %w", v, cgInfo.Name, err)
				slog.Error(e.Error())
				return nil, status.Error(codes.Unknown, e.Error())
			}

			volume, err := commonService.IboxAPI.GetVolume(ctx, vID)
			if err != nil {
				e := common.Errorf("error getting volume - volumeID: %s CG: %s error: %w", v, cgInfo.Name, err)
				slog.Error(e.Error())
				return nil, status.Error(codes.Unknown, e.Error())
			}

			if volume.CGID > 0 {
				e := common.Errorf("error adding volume to CG - volumeID: %d CG: %s error: volume already belongs to CG %d", v, cgInfo.Name, volume.CGID)
				slog.Error(e.Error())
				return nil, status.Error(codes.InvalidArgument, e.Error())
			}

			err = commonService.IboxAPI.AddMemberToCG(ctx, volume.ID, cgInfo.ID, volume.Name)
			if err != nil {
				e := common.Errorf("error adding volume to CG - volumeID: %d CG: %s error: %w", v, cgInfo.Name, err)
				slog.Error(e.Error())
				return nil, status.Error(codes.Unknown, e.Error())
			}
			csiVol := &csi.Volume{
				VolumeId:      v,
				CapacityBytes: volume.Size,
			}
			csiVolumes = append(csiVolumes, csiVol)
		}
	}
	vg := &volumegroup.VolumeGroup{
		VolumeGroupId: strconv.Itoa(cgInfo.ID),
		Volumes:       csiVolumes,
	}
	return &volumegroup.CreateVolumeGroupResponse{
		VolumeGroup: vg,
	}, nil
}

// DeleteVolumeGroup RPC call to delete a volume group.
//
// From the spec:
// This RPC will be called by the CO to delete a volume group on behalf of a
// user. This operation MUST be idempotent.
//
// If a volume group corresponding to the specified volume_group_id does not
// exist or the artifacts associated with the volume group do not exist
// anymore, the Plugin MUST reply 0 OK.
//
// A volume cannot be deleted individually when it is part of the group. It has
// to be removed from the group first. Delete a volume group will delete all
// volumes in the group.
//
// Note:
// The undocumented DO_NOT_ALLOW_VG_TO_DELETE_VOLUMES capability is set. There
// is no need to delete each volume that may be part of the volume group. If
// the volume group is not empty, a FAILED_PRECONDITION error will be returned.
func (vs *VolumeGroupServer) DeleteVolumeGroup(
	ctx context.Context,
	req *volumegroup.DeleteVolumeGroupRequest,
) (*volumegroup.DeleteVolumeGroupResponse, error) {
	slog.Debug("AddOns VolumeGroupServer", "DeleteVolumeGroup", "started")

	config := map[string]string{}
	commonService, err := storagecommon.BuildCommonService(config, req.Secrets, nil)
	if err != nil {
		e := common.Errorf("error building commonService - volumeGroup: %s error: %w", req.VolumeGroupId, err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Unauthenticated, e.Error())
	}

	cgID, err := strconv.Atoi(req.VolumeGroupId)
	if err != nil {
		e := common.Errorf("error converting volumeGroupID to int - volumeGroup: %s error: %w", req.VolumeGroupId, err)
		slog.Error(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	cg, err := commonService.IboxAPI.GetConsistencyGroup(ctx, cgID)
	if err != nil {
		e := common.Errorf("error getting CG - volumeGroup: %s error: %w", req.VolumeGroupId, err)
		slog.Error(e.Error())
		// spec says to just return 0, OK if there is no CG
		return &volumegroup.DeleteVolumeGroupResponse{}, nil
	}

	err = commonService.IboxAPI.DeleteConsistencyGroup(ctx, cg.ID)
	if err != nil {
		e := common.Errorf("error deleting CG - volumeGroup: %s error: %w", req.VolumeGroupId, err)
		slog.Error(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	//TODO verify with spec that we should not delete volumes if they delete the CG/volume group
	return &volumegroup.DeleteVolumeGroupResponse{}, nil
}

// ModifyVolumeGroupMembership RPC call to modify a volume group.
//
// From the spec:
// This RPC will be called by the CO to modify an existing volume group on
// behalf of a user. volume_ids provided in the
// ModifyVolumeGroupMembershipRequest will be compared to the ones in the
// existing volume group. New volume_ids in the modified volume group will be
// added to the volume group. Existing volume_ids not in the modified volume
// group will be removed from the volume group. If volume_ids is empty, the
// volume group will be removed of all existing volumes. This operation MUST be
// idempotent.
//
// File-based storage systems usually do not support this PRC. Block-based
// storage systems usually support this PRC.
//
// By adding an existing volume to a group, however, there is no way to pass in
// parameters to influence placement when provisioning a volume.
//
// It is out of the scope of the CSI spec to determine whether a group is
// consistent or not. It is up to the storage provider to clarify that in the
// vendor specific documentation. This is true either when creating a new
// volume with a group id or adding an existing volume to a group.
//
// CSI drivers supporting MODIFY_VOLUME_GROUP_MEMBERSHIP MUST implement
// ModifyVolumeGroupMembership RPC.
//
// Note:
//
// The implementation works as the following:
// - resolve the existing volume group
// - get the CSI-IDs of all volumes
// - create a list of volumes that should be removed
// - create a list of volume IDs that should be added
// - remove the volumes from the group
// - add the volumes to the group
//
// Also, MODIFY_VOLUME_GROUP_MEMBERSHIP does not exist, it is called
// MODIFY_VOLUME_GROUP instead.
func (vs *VolumeGroupServer) ModifyVolumeGroupMembership(
	ctx context.Context,
	req *volumegroup.ModifyVolumeGroupMembershipRequest,
) (*volumegroup.ModifyVolumeGroupMembershipResponse, error) {

	slog.Debug("AddOns VolumeGroupServer", "ModifyVolumeGroupMembership", "started")

	config := map[string]string{}
	commonService, err := storagecommon.BuildCommonService(config, req.Secrets, nil)
	if err != nil {
		e := common.Errorf("error building commonService - volumeGroup: %s error: %w", req.VolumeGroupId, err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Unauthenticated, e.Error())
	}

	cgID, err := strconv.Atoi(req.VolumeGroupId)
	if err != nil {
		e := common.Errorf("error converting volumeGroupID to int: %s error: %w", req.VolumeGroupId, err)
		slog.Error(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	cg, err := commonService.IboxAPI.GetConsistencyGroup(ctx, cgID)
	if err != nil {
		e := common.Errorf("error getting CG volumeGroupID: %s error: %w", req.VolumeGroupId, err)
		slog.Error(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	currentMembers, err := commonService.IboxAPI.GetMembersByCGID(ctx, cg.ID)
	if err != nil {
		e := common.Errorf("error getting CG members volumeGroupID: %s error: %w", req.VolumeGroupId, err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Unknown, e.Error())
	}

	// handle the case where no volumeIDs are passed in, this means
	// you delete any existing members if they exist in the CG
	if len(req.VolumeIds) == 0 && len(currentMembers) > 0 {
		// remove all current members
		for _, v := range currentMembers {
			err := commonService.IboxAPI.RemoveMemberFromCG(ctx, cg.ID, v.ID)
			if err != nil {
				e := common.Errorf("error removing member %s from CG: %s error: %w", v, cg.Name, err)
				slog.Error(e.Error())
				return nil, status.Error(codes.Unknown, e.Error())
			}
		}
		vg := &volumegroup.VolumeGroup{
			VolumeGroupId: req.VolumeGroupId,
		}
		return &volumegroup.ModifyVolumeGroupMembershipResponse{
			VolumeGroup: vg,
		}, nil
	}

	volumeIDs := req.VolumeIds

	// figure out how many volumes need to be added to the CG
	volumesToAdd := make([]int, 0)
	for _, v := range volumeIDs {
		foundInCurrentMembers := false
		for _, cm := range currentMembers {
			stringVersion := strconv.Itoa(cm.ID)
			if stringVersion == v {
				// found in currentMembers, do nothing
				foundInCurrentMembers = true
			}
		}
		if !foundInCurrentMembers {
			// add the volume to the CG
			vID, err := strconv.Atoi(v)
			if err != nil {
				e := common.Errorf("error converting volumeID to int: %s error: %w", v, err)
				slog.Error(e.Error())
				return nil, status.Error(codes.Unknown, e.Error())
			}
			volumesToAdd = append(volumesToAdd, vID)
		}
	}

	//figure out the volumes to be removed from the current CG
	volumesToRemove := make([]int, 0)
	for _, cm := range currentMembers {
		currentMem := strconv.Itoa(cm.ID)
		foundInCurrentMembers := false
		for _, v := range volumeIDs {
			if currentMem == v {
				// found in currentMembers, do nothing
				foundInCurrentMembers = true
			}
		}
		if !foundInCurrentMembers {
			// remove a current member if its not in the request list of volumes
			volumesToRemove = append(volumesToRemove, cm.ID)
		}
	}

	// now we can add new volumes to the CG
	for _, v := range volumesToAdd {
		volume, err := commonService.IboxAPI.GetVolume(ctx, v)
		if err != nil {
			e := common.Errorf("error getting volume - volumeID: %s error: %w", v, err)
			slog.Error(e.Error())
			return nil, status.Error(codes.Unknown, e.Error())
		}
		err = commonService.IboxAPI.AddMemberToCG(ctx, volume.ID, cg.ID, volume.Name)
		if err != nil {
			e := common.Errorf("error adding member to CG - volumeID: %s CG: %s error: %w", v, cg.Name, err)
			slog.Error(e.Error())
			return nil, status.Error(codes.Unknown, e.Error())
		}
	}

	// now we can remove current members from the CG
	for _, v := range volumesToRemove {
		volume, err := commonService.IboxAPI.GetVolume(ctx, v)
		if err != nil {
			e := common.Errorf("error getting volume - volumeID: %s error: %w", v, err)
			slog.Error(e.Error())
			return nil, status.Error(codes.Unknown, e.Error())
		}
		err = commonService.IboxAPI.RemoveMemberFromCG(ctx, cg.ID, volume.ID)
		if err != nil {
			e := common.Errorf("error removing member from CG - volumeID: %s CG: %s error: %w", v, cg.Name, err)
			slog.Error(e.Error())
			return nil, status.Error(codes.Unknown, e.Error())
		}
	}

	currentMembers, err = commonService.IboxAPI.GetMembersByCGID(ctx, cg.ID)
	if err != nil {
		e := common.Errorf("error getting CG members volumeGroupID: %s error: %w", req.VolumeGroupId, err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Unknown, e.Error())
	}

	// get a list of members at the very end to return in the response
	volumesAtEnd := make([]*csi.Volume, 0)
	for _, v := range currentMembers {
		csiVolume := &csi.Volume{
			VolumeId:      strconv.Itoa(v.ID),
			CapacityBytes: int64(v.Size),
		}
		volumesAtEnd = append(volumesAtEnd, csiVolume)
	}

	vg := &volumegroup.VolumeGroup{
		VolumeGroupId: req.VolumeGroupId,
		Volumes:       volumesAtEnd,
	}
	return &volumegroup.ModifyVolumeGroupMembershipResponse{
		VolumeGroup: vg,
	}, nil
}

// ControllerGetVolumeGroup RPC call to get a volume group.
//
// From the spec:
// ControllerGetVolumeGroupResponse should contain current information of a
// volume group if it exists. If the volume group does not exist any more,
// ControllerGetVolumeGroup should return gRPC error code NOT_FOUND.
func (vs *VolumeGroupServer) ControllerGetVolumeGroup(
	ctx context.Context,
	req *volumegroup.ControllerGetVolumeGroupRequest,
) (*volumegroup.ControllerGetVolumeGroupResponse, error) {
	slog.Debug("AddOns VolumeGroupServer", "ControllerGetVolumeGroup", "started")

	config := map[string]string{}
	commonService, err := storagecommon.BuildCommonService(config, req.Secrets, nil)
	if err != nil {
		e := common.Errorf("error building commonService - volumeGroup ID: %s error: %w", req.VolumeGroupId, err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Unauthenticated, e.Error())
	}

	cgID, err := strconv.Atoi(req.VolumeGroupId)
	if err != nil {
		e := common.Errorf("error converting volumeGroupId - volumeGroup ID: %s error: %w", req.VolumeGroupId, err)
		slog.Error(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}
	consistencyGroup, err := commonService.IboxAPI.GetConsistencyGroup(ctx, cgID)
	if err != nil {
		e := common.Errorf("error getting CG - cg ID: %s error: %w", cgID, err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Unknown, e.Error())
	}

	vg := &volumegroup.VolumeGroup{
		VolumeGroupId: strconv.Itoa(consistencyGroup.ID),
	}

	if consistencyGroup.MembersCount > 0 {
		var err error
		vg.Volumes, err = getCGMembers(ctx, commonService, cgID)
		if err != nil {
			e := common.Errorf("error members for CG - cg ID: %s error: %w", cgID, err)
			slog.Error(e.Error())
			return nil, status.Error(codes.Unknown, e.Error())
		}
	}

	return &volumegroup.ControllerGetVolumeGroupResponse{
		VolumeGroup: vg,
	}, nil
}

func getCGMembers(ctx context.Context, commonService storagecommon.Commonservice, cgID int) (volumes []*csi.Volume, err error) {
	members, err := commonService.IboxAPI.GetMembersByCGID(ctx, cgID)
	if err != nil {
		return nil, err
	}

	for _, m := range members {
		v := &csi.Volume{
			VolumeId: strconv.Itoa(m.ID),
		}
		volumes = append(volumes, v)
	}

	return volumes, nil
}
