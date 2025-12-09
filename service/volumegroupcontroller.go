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
package service

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/infinidat/infinibox-csi-driver/iboxapi"
	storagecommon "github.com/infinidat/infinibox-csi-driver/storage/common"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// VolumeGroupServer controller server setting
type VolumeGroupServer struct {
	Driver *Driver
	csi.UnimplementedGroupControllerServer
}

func (s *VolumeGroupServer) CreateVolumeGroupSnapshot(ctx context.Context, req *csi.CreateVolumeGroupSnapshotRequest) (resp *csi.CreateVolumeGroupSnapshotResponse, err error) {
	slog.Info("Start", "request", req, "parameters", req.GetParameters())

	commonService, err := storagecommon.BuildCommonService(make(map[string]string), req.Secrets, nil)
	if err != nil {
		slog.Error(" BuildCommonService - error", "error", err.Error())
		return nil, status.Errorf(codes.Internal, "failed to get API connection error %v", err)
	}

	client, err := commonService.API.NewClient()
	if err != nil {
		slog.Error("NewClient - error", "error", err.Error())
		return nil, status.Errorf(codes.Internal, "failed to get api client error %v", err)
	}

	// create a CG - use a pool ID from one of the volumes we looked up
	// the cgname is specified in the volumesnapshotgroupclass as a parameter unique to this driver
	// we create the cg on the ibox if it doesnt exist
	var newCG *iboxapi.ConsistencyGroupInfo
	cgName := req.Parameters["infinidat.com/cgname"]

	newCG, err = client.IboxAPI.GetConsistencyGroupByName(ctx, cgName)
	if err != nil {
		if errors.Is(err, iboxapi.ErrNotFound) {
			var poolID int
			var allVolumeIDs []int

			// get the volume ids that are in the group
			for _, id := range req.SourceVolumeIds {
				slog.Debug("info", "source Volume ID", id)
				volproto := strings.Split(id, "$$")
				if len(volproto) != 2 {
					slog.Error("vol proto invalid", "volproto", volproto)
					return nil, errors.New("volume Id and other details not found")
				}
				volumeID, err := strconv.Atoi(volproto[0])
				if err != nil {
					slog.Error("parseInt - error", "error", err.Error())
					return nil, status.Errorf(codes.Internal, "failed to convert volume ID %s to int error %v", volproto[0], err)
				}
				slog.Debug("info", "volume ID", volumeID)
				// look up the volume
				volume, err := commonService.IboxAPI.GetVolume(ctx, volumeID)
				if err != nil {
					slog.Error("GetVolume - error", "error", err.Error())
					return nil, status.Errorf(codes.Internal, "failed to get Volume with ID %d error %v", volumeID, err)
				}
				slog.Debug("volume found", "name", volume.Name, "id", volumeID, "pool id", volume.PoolID)
				poolID = volume.PoolID
				allVolumeIDs = append(allVolumeIDs, volumeID)
			}

			createCGRequest := iboxapi.CreateConsistencyGroupRequest{
				PoolID: poolID,
				Name:   cgName,
			}
			newCG, err = client.IboxAPI.CreateConsistencyGroup(ctx, createCGRequest)
			if err != nil {
				slog.Error("CreateCG - error", "error", err.Error())
				return nil, status.Errorf(codes.Internal, "failed to create cg error %v", err)
			}
			slog.Debug("new CG", "id", newCG.ID)
			// add members to the CG
			for _, volumeID := range allVolumeIDs {
				err = client.IboxAPI.AddMemberToSnapshotGroup(ctx, volumeID, newCG.ID)
				if err != nil {
					slog.Error("AddMemberToSnapshotGroup - error", "error", err.Error())
					return nil, status.Errorf(codes.Internal, "failed to add volume to cg error %v", err)
				}
			}
		} else {
			slog.Error("getCG", "error", err)
			return nil, status.Errorf(codes.Internal, "error getting cg %v", err)
		}
	} else {
		slog.Info("CG already exists, will not create", "cg", cgName)
	}

	// we are using the VolumeGroupSnapshot name for the snap group name and the prefix since
	// users will create n-number of uniquely named volumegroupsnapshots potentially
	vgsName := req.Parameters["csi.storage.k8s.io/volumegroupsnapshot/name"]

	// see if the SG name has already been used and fail if so
	_, err = client.IboxAPI.GetConsistencyGroupByName(ctx, vgsName)
	if err != nil {
		if !errors.Is(err, iboxapi.ErrNotFound) {
			slog.Error("error getting SG CG by name", "error", err.Error())
			return nil, status.Errorf(codes.Internal, "error getting sg cg %v", err)
		}
	} else {
		return nil, status.Errorf(codes.InvalidArgument, "Snap Group already exists with that name %s error %+v", vgsName, err)
	}

	createRequest := iboxapi.CreateSnapshotGroupRequest{
		CGID:       newCG.ID,
		SnapName:   vgsName,
		SnapPrefix: vgsName,
		SnapSuffix: "",
	}

	snapGroupCG, err := client.IboxAPI.CreateSnapshotGroup(ctx, createRequest)
	if err != nil {
		slog.Error("CreateSnapshotGroup - error", "error", err.Error())
		return nil, status.Errorf(codes.Internal, "failed to create snap group error %s error %+v", vgsName, err)
	}
	slog.Debug("snapshot group", "id", snapGroupCG.ID, "name", vgsName, "members", snapGroupCG.MembersCount)

	creationTime := timestamppb.New(time.Now())

	// get snapgroup snapGroupMembers, then make a list of Snapshots to return based on those snapGroupMembers
	var snapGroupMembers []iboxapi.MemberInfo
	snapGroupMembers, err = client.IboxAPI.GetMembersByCGID(ctx, snapGroupCG.ID)
	if err != nil {
		slog.Error("GetMembersByCGID - error", "error", err.Error())
		return nil, status.Errorf(codes.Internal, "failed to get snapgroup CG members error %+v", err)
	}
	slog.Debug("members from snapgroup CG", "count", len(snapGroupMembers))

	snapshots := make([]*csi.Snapshot, 0)
	for _, member := range snapGroupMembers {
		snapshotName := member.CGName + member.Name // prefix + volume name
		slog.Debug("member info", "snapshot", snapshotName, "member", member)
		volume, err := client.IboxAPI.GetVolume(ctx, member.ID)
		if err != nil {
			slog.Error("from GetVolume", "error", err.Error())
			return nil, status.Errorf(codes.Internal, "failed to get snapshot volume  error %v", err)
		}

		// for the returned snapshots, we need the xxx$$proto version of the source volume ID
		// instead of the integer key
		var sourceVolumeId string

		for _, someVolumeID := range req.SourceVolumeIds {
			idString := strconv.Itoa(member.FamilyID)
			slog.Debug("contains check", "volume id", someVolumeID, "id", idString)
			if strings.Contains(someVolumeID, idString) {
				sourceVolumeId = someVolumeID
				slog.Debug("found", "member id", member.ID, "source volume id", sourceVolumeId)
			}
		}

		slog.Debug("assembling snapshot result", "volume id", volume.ID, "source volume id", sourceVolumeId)
		sourceVolumeIDParts := strings.Split(sourceVolumeId, "$$")
		if len(sourceVolumeIDParts) != 2 {
			slog.Error("source volume id not valid", "source volume id", sourceVolumeId)
			return nil, status.Errorf(codes.Internal, "sourceVolumeId not parsing correctly %+v", sourceVolumeIDParts)
		}
		example := csi.Snapshot{
			SizeBytes:       int64(member.Size),
			SnapshotId:      strconv.Itoa(volume.ID) + "$$" + sourceVolumeIDParts[1],
			SourceVolumeId:  sourceVolumeId,
			CreationTime:    creationTime,
			ReadyToUse:      true,
			GroupSnapshotId: snapGroupCG.Name,
		}
		snapshots = append(snapshots, &example)
	}
	slog.Debug("snapshots/members", "length", len(snapshots))

	resp = &csi.CreateVolumeGroupSnapshotResponse{
		GroupSnapshot: &csi.VolumeGroupSnapshot{
			GroupSnapshotId: strconv.Itoa(snapGroupCG.ID),
			Snapshots:       snapshots,
			CreationTime:    creationTime,
			ReadyToUse:      true,
		},
	}
	slog.Info(" Finish", "req", req)
	return resp, nil
}

func (s *VolumeGroupServer) DeleteVolumeGroupSnapshot(ctx context.Context, req *csi.DeleteVolumeGroupSnapshotRequest) (resp *csi.DeleteVolumeGroupSnapshotResponse, err error) {
	slog.Info("Start", "req", req, "group_snapshot_id", req.GroupSnapshotId)

	for _, snapshotID := range req.SnapshotIds {
		slog.Debug("Snap Group has snapshot ID", "group snapshot id", req.GroupSnapshotId, "snapshot id", snapshotID)
	}

	commonService, err := storagecommon.BuildCommonService(make(map[string]string), req.Secrets, nil)
	if err != nil {
		slog.Error("from BuildCommonService", "error", err.Error())
		return nil, status.Errorf(codes.Internal, "failed to get API connection error %v", err)
	}

	client, err := commonService.API.NewClient()
	if err != nil {
		slog.Error("from NewClient", "error", err.Error())
		return nil, status.Errorf(codes.Internal, "failed to get api client error %v", err)
	}

	snapshotGroupID, err := strconv.Atoi(req.GroupSnapshotId)
	if err != nil {
		slog.Error("parsing error", "group snapshot id", req.GroupSnapshotId, "error", err.Error())
		return nil, status.Errorf(codes.InvalidArgument, "failed to convert group_snapshot_id %s to int error %v", req.GroupSnapshotId, err)
	}
	err = client.IboxAPI.DeleteConsistencyGroup(ctx, snapshotGroupID)
	if err != nil {
		slog.Error("from DeleteCG", "error", err.Error())
		return nil, status.Errorf(codes.Internal, "error deleting CG %s error %v", req.GroupSnapshotId, err)
	}
	resp = &csi.DeleteVolumeGroupSnapshotResponse{}
	slog.Info("Finish", "req", req)
	return resp, nil
}

func (s *VolumeGroupServer) GetVolumeGroupSnapshot(ctx context.Context, req *csi.GetVolumeGroupSnapshotRequest) (resp *csi.GetVolumeGroupSnapshotResponse, err error) {
	slog.Info("Start", "req", req, "GroupSnapshotId", req.GroupSnapshotId, "SnapshotIds", req.SnapshotIds)

	commonService, err := storagecommon.BuildCommonService(make(map[string]string), req.Secrets, nil)
	if err != nil {
		slog.Error("from BuildCommonService", "error", err.Error())
		return nil, status.Errorf(codes.Internal, "failed to get API connection error %v", err)
	}

	client, err := commonService.API.NewClient()
	if err != nil {
		slog.Error("from NewClient", "error", err.Error())
		return nil, status.Errorf(codes.Internal, "failed to get api client error %v", err)
	}

	snapshotGroupID, err := strconv.Atoi(req.GroupSnapshotId)
	if err != nil {
		slog.Error("parsing error", "group snapshot id", req.GroupSnapshotId, "error", err.Error())
		return nil, status.Errorf(codes.InvalidArgument, "failed to convert group_snapshot_id %s to int error %v", req.GroupSnapshotId, err)
	}

	// sgID is the volume ID of the snap group, the parent_id will be the consistencyGroup MASTER volume
	consistencyGroup, err := client.IboxAPI.GetConsistencyGroup(ctx, snapshotGroupID)
	if err != nil {
		slog.Error("from GetConsistencyGroup", "error", err.Error())
		return nil, status.Errorf(codes.NotFound, "error getting CG group_snapshot_id %s error %v", req.GroupSnapshotId, err)
	}

	creationTime := timestamppb.New(time.Now())

	// get snapgroup snapshotMembers, then make a list of Snapshots to return based on those snapshotMembers
	var snapshotMembers []iboxapi.MemberInfo
	snapshotMembers, err = client.IboxAPI.GetMembersByCGID(ctx, snapshotGroupID)
	if err != nil {
		slog.Error("GetMembersByCGID - error", "error", err.Error())
		return nil, status.Errorf(codes.Internal, "failed to get snapgroup CG members error %+v", err)
	}
	slog.Debug("snapgroup CG", "member count", len(snapshotMembers))

	snapshots := make([]*csi.Snapshot, 0)
	for _, member := range snapshotMembers {
		snapshotName := member.CGName + member.Name // prefix + volume name
		slog.Debug("info", "snapshot name", snapshotName, "member", member)
		volume, err := client.IboxAPI.GetVolume(ctx, member.ID)
		if err != nil {
			slog.Error("GetVolume - error", "error", err.Error())
			return nil, status.Errorf(codes.InvalidArgument, "failed to get snapshot volume  error %v", err)
		}

		// for the returned snapshots, we need the xxx$$proto version of the source volume ID
		// instead of the integer key
		var sourceVolumeID string

		for _, snapshotID := range req.SnapshotIds {
			idString := strconv.Itoa(member.ID)
			slog.Debug("contains check", "snapshot id", snapshotID, "id", idString)
			if strings.Contains(snapshotID, idString) {
				sourceVolumeID = snapshotID
				slog.Debug("found member", "member id", member.ID, "source volume id", sourceVolumeID)
			}
		}

		slog.Debug("assembling snapshot result", "volume id", volume.ID, "source volume id", sourceVolumeID)
		volumeIDParts := strings.Split(sourceVolumeID, "$$")
		if len(volumeIDParts) != 2 {
			slog.Error("source volume id not valid", "source volume id", sourceVolumeID)
			return nil, status.Errorf(codes.InvalidArgument, "sourceVolumeId not parsing correctly %+v", volumeIDParts)
		}
		example := csi.Snapshot{
			SizeBytes:       int64(member.Size),
			SnapshotId:      strconv.Itoa(volume.ID) + "$$" + volumeIDParts[1],
			SourceVolumeId:  sourceVolumeID,
			CreationTime:    creationTime,
			ReadyToUse:      true,
			GroupSnapshotId: consistencyGroup.Name,
		}
		snapshots = append(snapshots, &example)
	}
	slog.Debug("snapshots/members", "count", len(snapshots))

	resp = &csi.GetVolumeGroupSnapshotResponse{
		GroupSnapshot: &csi.VolumeGroupSnapshot{
			GroupSnapshotId: strconv.Itoa(snapshotGroupID),
			Snapshots:       snapshots,
			CreationTime:    creationTime,
			ReadyToUse:      true,
		},
	}
	slog.Info("Finish", "req", req)
	return resp, nil
}

func (s *VolumeGroupServer) GroupControllerGetCapabilities(ctx context.Context, req *csi.GroupControllerGetCapabilitiesRequest) (*csi.GroupControllerGetCapabilitiesResponse, error) {
	return &csi.GroupControllerGetCapabilitiesResponse{
		Capabilities: s.Driver.groupcap,
	}, nil
}
