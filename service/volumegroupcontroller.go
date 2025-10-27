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
	const functionName = "CreateVolumeGroupSnapshot"
	zlog.Info().Msgf("%s Start - req: %v", functionName, req)

	zlog.Debug().Msgf("parameters are %v", req.GetParameters())

	commonService, err := storagecommon.BuildCommonService(make(map[string]string), req.Secrets, nil)
	if err != nil {
		zlog.Error().Msgf("%s - BuildCommonService - error: %s", functionName, err.Error())
		return nil, status.Errorf(codes.Internal, "failed to get API connection error %v", err)
	}

	client, err := commonService.API.NewClient()
	if err != nil {
		zlog.Error().Msgf("%s - NewClient - error: %s", functionName, err.Error())
		return nil, status.Errorf(codes.Internal, "failed to get api client error %v", err)
	}

	// create a CG - use a pool ID from one of the volumes we looked up
	// the cgname is specified in the volumesnapshotgroupclass as a parameter unique to this driver
	// we create the cg on the ibox if it doesnt exist
	var newCG *iboxapi.ConsistencyGroupInfo
	cgName := req.Parameters["infinidat.com/cgname"]

	newCG, err = client.IboxAPI.GetConsistencyGroupByName(cgName)
	if err != nil {
		re, ok := err.(*iboxapi.APIError)
		if ok && re.Code == iboxapi.RESOURCE_NOT_FOUND {
			var poolID int
			var allVolumeIDs []int

			// get the volume ids that are in the group
			for _, id := range req.SourceVolumeIds {
				zlog.Debug().Msgf("source Volume ID : %s", id)
				volproto := strings.Split(id, "$$")
				if len(volproto) != 2 {
					zlog.Error().Msgf("%s - vol proto invalid %v", functionName, volproto)
					return nil, errors.New("volume Id and other details not found")
				}
				volumeID, err := strconv.Atoi(volproto[0])
				if err != nil {
					zlog.Error().Msgf("%s - parseInt - error: %s", functionName, err.Error())
					return nil, status.Errorf(codes.Internal, "failed to convert volume ID %s to int error %v", volproto[0], err)
				}
				zlog.Debug().Msgf("volume ID : %d", volumeID)
				// look up the volume
				volume, err := commonService.IboxAPI.GetVolume(volumeID)
				if err != nil {
					zlog.Error().Msgf("%s - GetVolume - error: %s", functionName, err.Error())
					return nil, status.Errorf(codes.Internal, "failed to get Volume with ID %d error %v", volumeID, err)
				}
				zlog.Debug().Msgf("volume %s found with ID : %d poolID: %d", volume.Name, volumeID, volume.PoolID)
				poolID = volume.PoolID
				allVolumeIDs = append(allVolumeIDs, volumeID)
			}

			createCGRequest := iboxapi.CreateConsistencyGroupRequest{
				PoolID: poolID,
				Name:   cgName,
			}
			newCG, err = client.IboxAPI.CreateConsistencyGroup(createCGRequest)
			if err != nil {
				zlog.Error().Msgf("%s - CreateCG - error: %s", functionName, err.Error())
				return nil, status.Errorf(codes.Internal, "failed to create cg error %v", err)
			}
			zlog.Debug().Msgf("new CG ID %d", newCG.ID)
			// add members to the CG
			for _, volumeID := range allVolumeIDs {
				err = client.IboxAPI.AddMemberToSnapshotGroup(volumeID, newCG.ID)
				if err != nil {
					zlog.Error().Msgf("%s - AddMemberToSnapshotGroup - error: %s", functionName, err.Error())
					return nil, status.Errorf(codes.Internal, "failed to add volume to cg error %v", err)
				}
			}
		} else {
			zlog.Error().Msgf("getCG error %+v", err)
			return nil, status.Errorf(codes.Internal, "error getting cg %v", err)
		}
	} else {
		zlog.Info().Msgf("%s - CG already exists %s, will not create", functionName, cgName)
	}

	// we are using the VolumeGroupSnapshot name for the snap group name and the prefix since
	// users will create n-number of uniquely named volumegroupsnapshots potentially
	vgsName := req.Parameters["csi.storage.k8s.io/volumegroupsnapshot/name"]

	// see if the SG name has already been used and fail if so
	_, err = client.IboxAPI.GetConsistencyGroupByName(vgsName)
	if err != nil {
		re, ok := err.(*iboxapi.APIError)
		if ok && re.Code == iboxapi.RESOURCE_NOT_FOUND {
		} else {
			zlog.Error().Msgf("%s - error getting SG CG by name %s", functionName, err.Error())
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

	snapGroupCG, err := client.IboxAPI.CreateSnapshotGroup(createRequest)
	if err != nil {
		zlog.Error().Msgf("%s - CreateSnapshotGrup - error: %s", functionName, err.Error())
		return nil, status.Errorf(codes.Internal, "failed to create snap group error %s error %+v", vgsName, err)
	}
	zlog.Debug().Msgf("snapshot group ID %d name %s has %d members", snapGroupCG.ID, vgsName, snapGroupCG.MembersCount)

	creationTime := timestamppb.New(time.Now())

	// get snapgroup snapGroupMembers, then make a list of Snapshots to return based on those snapGroupMembers
	var snapGroupMembers []iboxapi.MemberInfo
	snapGroupMembers, err = client.IboxAPI.GetMembersByCGID(snapGroupCG.ID)
	if err != nil {
		zlog.Error().Msgf("%s - GetMembersByCGID - error: %s", functionName, err.Error())
		return nil, status.Errorf(codes.Internal, "failed to get snapgroup CG members error %+v", err)
	}
	zlog.Debug().Msgf("members from snapgroup CG %d", len(snapGroupMembers))

	snapshots := make([]*csi.Snapshot, 0)
	for _, member := range snapGroupMembers {
		snapshotName := member.CGName + member.Name // prefix + volume name
		zlog.Debug().Msgf("member is snapshot name [%s] member info %+v", snapshotName, member)
		volume, err := client.IboxAPI.GetVolume(member.ID)
		if err != nil {
			zlog.Error().Msgf("%s - GetVolume - error: %s", functionName, err.Error())
			return nil, status.Errorf(codes.Internal, "failed to get snapshot volume  error %v", err)
		}

		// for the returned snapshots, we need the xxx$$proto version of the source volume ID
		// instead of the integer key
		var sourceVolumeId string

		for _, someVolumeID := range req.SourceVolumeIds {
			idString := strconv.Itoa(member.FamilyID)
			zlog.Debug().Msgf("contains check %s %s", someVolumeID, idString)
			if strings.Contains(someVolumeID, idString) {
				sourceVolumeId = someVolumeID
				zlog.Debug().Msgf("found member id %d is proto %s", member.ID, sourceVolumeId)
			}
		}

		zlog.Debug().Msgf("assembling snapshot result with ID %d sourceVolumeID %s", volume.ID, sourceVolumeId)
		sourceVolumeIDParts := strings.Split(sourceVolumeId, "$$")
		if len(sourceVolumeIDParts) != 2 {
			zlog.Error().Msgf("%s - source volume id not valid %s", functionName, sourceVolumeId)
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
	zlog.Debug().Msgf("snapshots/members %d", len(snapshots))

	resp = &csi.CreateVolumeGroupSnapshotResponse{
		GroupSnapshot: &csi.VolumeGroupSnapshot{
			GroupSnapshotId: strconv.Itoa(snapGroupCG.ID),
			Snapshots:       snapshots,
			CreationTime:    creationTime,
			ReadyToUse:      true,
		},
	}
	zlog.Info().Msgf("%s Finish - req %v", functionName, req)
	return resp, nil
}

func (s *VolumeGroupServer) DeleteVolumeGroupSnapshot(ctx context.Context, req *csi.DeleteVolumeGroupSnapshotRequest) (resp *csi.DeleteVolumeGroupSnapshotResponse, err error) {
	const functionName = "DeleteVolumeGroupSnapshot"
	zlog.Info().Msgf("%s Start - req: %v", functionName, req)
	zlog.Debug().Msgf("group_snapshot_id %s", req.GroupSnapshotId)

	for _, snapshotID := range req.SnapshotIds {
		zlog.Debug().Msgf("Snap Group %s has snapshot ID %s", req.GroupSnapshotId, snapshotID)
	}

	commonService, err := storagecommon.BuildCommonService(make(map[string]string), req.Secrets, nil)
	if err != nil {
		zlog.Error().Msgf("%s - BuildCommonService - error: %s", functionName, err.Error())
		return nil, status.Errorf(codes.Internal, "failed to get API connection error %v", err)
	}

	client, err := commonService.API.NewClient()
	if err != nil {
		zlog.Error().Msgf("%s - NewClient - error: %s", functionName, err.Error())
		return nil, status.Errorf(codes.Internal, "failed to get api client error %v", err)
	}

	snapshotGroupID, err := strconv.Atoi(req.GroupSnapshotId)
	if err != nil {
		zlog.Error().Msgf("%s - ParseInt request %s - error: %s", functionName, req.GroupSnapshotId, err.Error())
		return nil, status.Errorf(codes.InvalidArgument, "failed to convert group_snapshot_id %s to int error %v", req.GroupSnapshotId, err)
	}
	err = client.IboxAPI.DeleteConsistencyGroup(snapshotGroupID)
	if err != nil {
		zlog.Error().Msgf("%s - DeleteSG - error: %s", functionName, err.Error())
		return nil, status.Errorf(codes.Internal, "error deleting SG %s error %v", req.GroupSnapshotId, err)
	}
	resp = &csi.DeleteVolumeGroupSnapshotResponse{}
	zlog.Info().Msgf("%s Finish - req %v", functionName, req)
	return resp, nil
}

func (s *VolumeGroupServer) GetVolumeGroupSnapshot(ctx context.Context, req *csi.GetVolumeGroupSnapshotRequest) (resp *csi.GetVolumeGroupSnapshotResponse, err error) {
	const functionName = "GetVolumeGroupSnapshot"
	zlog.Info().Msgf("%s Start - req: %v", functionName, req)
	zlog.Debug().Msgf("req.GroupSnapshotId=%s", req.GroupSnapshotId)
	zlog.Debug().Msgf("req.SnapshotIds=%v", req.SnapshotIds)
	// zlog.Debug().Msgf("req.Secrets=%v", req.Secrets)

	commonService, err := storagecommon.BuildCommonService(make(map[string]string), req.Secrets, nil)
	if err != nil {
		zlog.Error().Msgf("%s - BuildCommonService - error: %s", functionName, err.Error())
		return nil, status.Errorf(codes.Internal, "failed to get API connection error %v", err)
	}

	client, err := commonService.API.NewClient()
	if err != nil {
		zlog.Error().Msgf("%s - NewClient - error: %s", functionName, err.Error())
		return nil, status.Errorf(codes.Internal, "failed to get api client error %v", err)
	}

	snapshotGroupID, err := strconv.Atoi(req.GroupSnapshotId)
	if err != nil {
		zlog.Error().Msgf("%s - ParseInt request %s - error: %s", functionName, req.GroupSnapshotId, err.Error())
		return nil, status.Errorf(codes.InvalidArgument, "failed to convert group_snapshot_id %s to int error %v", req.GroupSnapshotId, err)
	}

	// sgID is the volume ID of the snap group, the parent_id will be the consistencyGroup MASTER volume
	consistencyGroup, err := client.IboxAPI.GetConsistencyGroup(snapshotGroupID)
	if err != nil {
		zlog.Error().Msgf("%s - GetCGByID - error: %s", functionName, err.Error())
		return nil, status.Errorf(codes.NotFound, "error getting CG group_snapshot_id %s error %v", req.GroupSnapshotId, err)
	}

	creationTime := timestamppb.New(time.Now())

	// get snapgroup snapshotMembers, then make a list of Snapshots to return based on those snapshotMembers
	var snapshotMembers []iboxapi.MemberInfo
	snapshotMembers, err = client.IboxAPI.GetMembersByCGID(snapshotGroupID)
	if err != nil {
		zlog.Error().Msgf("%s - GetMembersByCGID - error: %s", functionName, err.Error())
		return nil, status.Errorf(codes.Internal, "failed to get snapgroup CG members error %+v", err)
	}
	zlog.Debug().Msgf("members from snapgroup CG %d", len(snapshotMembers))

	snapshots := make([]*csi.Snapshot, 0)
	for _, member := range snapshotMembers {
		snapshotName := member.CGName + member.Name // prefix + volume name
		zlog.Debug().Msgf("member is snapshot name [%s] member info %+v", snapshotName, member)
		volume, err := client.IboxAPI.GetVolume(member.ID)
		if err != nil {
			zlog.Error().Msgf("%s - GetVolume - error: %s", functionName, err.Error())
			return nil, status.Errorf(codes.InvalidArgument, "failed to get snapshot volume  error %v", err)
		}

		// for the returned snapshots, we need the xxx$$proto version of the source volume ID
		// instead of the integer key
		var sourceVolumeID string

		for _, snapshotID := range req.SnapshotIds {
			idString := strconv.Itoa(member.ID)
			zlog.Debug().Msgf("contains check %s %s", snapshotID, idString)
			if strings.Contains(snapshotID, idString) {
				sourceVolumeID = snapshotID
				zlog.Debug().Msgf("found member id %d is proto %s", member.ID, sourceVolumeID)
			}
		}

		zlog.Debug().Msgf("assembling snapshot result with ID %d sourceVolumeID %s", volume.ID, sourceVolumeID)
		volumeIDParts := strings.Split(sourceVolumeID, "$$")
		if len(volumeIDParts) != 2 {
			zlog.Error().Msgf("%s - source volume id not valid %s", functionName, sourceVolumeID)
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
	zlog.Debug().Msgf("snapshots/members %d", len(snapshots))

	resp = &csi.GetVolumeGroupSnapshotResponse{
		GroupSnapshot: &csi.VolumeGroupSnapshot{
			GroupSnapshotId: strconv.Itoa(snapshotGroupID),
			Snapshots:       snapshots,
			CreationTime:    creationTime, // TODO fix this with the right creation time
			ReadyToUse:      true,
		},
	}
	zlog.Info().Msgf("%s Finish - req %v", functionName, req)
	return resp, nil
}

func (s *VolumeGroupServer) GroupControllerGetCapabilities(ctx context.Context, req *csi.GroupControllerGetCapabilitiesRequest) (*csi.GroupControllerGetCapabilitiesResponse, error) {
	return &csi.GroupControllerGetCapabilitiesResponse{
		Capabilities: s.Driver.groupcap,
	}, nil
}
