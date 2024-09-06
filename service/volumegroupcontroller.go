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
	"infinibox-csi-driver/api"
	"infinibox-csi-driver/storage"
	"strconv"
	"strings"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// VolumeGroupServer controller server setting
type VolumeGroupServer struct {
	Driver *Driver
}

func (s *VolumeGroupServer) CreateVolumeGroupSnapshot(ctx context.Context, req *csi.CreateVolumeGroupSnapshotRequest) (resp *csi.CreateVolumeGroupSnapshotResponse, err error) {
	zlog.Info().Msgf("CreateVolumeGroupSnapshot Start - req: %v", req)

	zlog.Debug().Msgf("parameters are %v", req.GetParameters())

	cs, err := storage.BuildCommonService(make(map[string]string), req.Secrets)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get API connection error %v", err)
	}

	cl, err := cs.Api.NewClient()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get api client error %v", err)
	}

	// create a CG - use a pool ID from one of the volumes we looked up
	// the cgname is specified in the volumesnapshotgroupclass as a parameter unique to this driver
	// we create the cg on the ibox if it doesnt exist
	var newCG api.CGInfo
	cgName := req.Parameters["infinidat.com/cgname"]

	newCG, err = cl.GetCG(cgName)
	if err != nil {
		//TODO check here for not found error
		zlog.Error().Msgf("getCG error %+v", err)
		//return nil, status.Errorf(codes.Internal, "failed to get api client error %v", err)
		var poolID int
		var allVolumeIDs []int

		// get the volume ids that are in the group
		for _, id := range req.SourceVolumeIds {
			zlog.Debug().Msgf("source Volume ID : %s", id)
			volproto := strings.Split(id, "$$")
			if len(volproto) != 2 {
				return nil, errors.New("volume Id and other details not found")
			}
			volumeID, err := strconv.ParseInt(volproto[0], 0, 64)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "failed to convert volume ID %s to int error %v", volproto[0], err)
			}
			zlog.Debug().Msgf("volume ID : %d", volumeID)
			// look up the volume
			vol, err := cs.Api.GetVolume(int(volumeID))
			if err != nil {
				return nil, status.Errorf(codes.Internal, "failed to get Volume with ID %d error %v", volumeID, err)
			}
			zlog.Debug().Msgf("volume %s found with ID : %d poolID: %d", vol.Name, volumeID, vol.PoolId)
			poolID = int(vol.PoolId)
			allVolumeIDs = append(allVolumeIDs, int(volumeID))
		}

		newCG, err = cl.CreateCG(poolID, cgName)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to create cg error %v", err)
		}
		zlog.Debug().Msgf("new CG ID %d", newCG.ID)
		// add members to the CG
		for _, id := range allVolumeIDs {
			err = cl.AddMemberToSnapshotGroup(id, newCG.ID)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "failed to add volume to cg error %v", err)
			}
		}
	} else {
		zlog.Error().Msgf("that CG already exists %s", cgName)
	}

	// we are using the VolumeGroupSnapshot name for the snap group name and the prefix since
	// users will create n-number of uniquely named volumegroupsnapshots potentially
	vgsName := req.Parameters["csi.storage.k8s.io/volumegroupsnapshot/name"]

	// see if the SG name has already been used and fail if so
	_, err = cl.GetCG(vgsName)
	if err != nil {
		//TODO need to parse for notfound error
		zlog.Debug().Msgf("get snapshot group %s error %+v", vgsName, err)
	} else {
		return nil, status.Errorf(codes.InvalidArgument, "Snap Group already exists with that name %s error %+v", vgsName, err)
	}

	snapGroupCG, err := cl.CreateSnapshotGroup(newCG.ID, vgsName, vgsName, "")
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create snap group error %s error %+v", vgsName, err)
	}
	zlog.Debug().Msgf("snapshot group ID %d name %s has %d members", snapGroupCG.ID, vgsName, snapGroupCG.MembersCount)

	creationTime := timestamppb.New(time.Now())

	// get snapgroup members, then make a list of Snapshots to return based on those members
	var members []api.MemberInfo
	members, err = cl.GetMembersByCGID(snapGroupCG.ID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get snapgroup CG members error %+v", err)
	}
	zlog.Debug().Msgf("members from snapgroup CG %d", len(members))

	snapshots := make([]*csi.Snapshot, 0)
	for _, m := range members {
		snapshotName := m.CGName + m.Name // prefix + volume name
		zlog.Debug().Msgf("member is snapshot name [%s] member info %+v", snapshotName, m)
		v, err := cl.GetVolume(m.ID)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to get snapshot volume  error %v", err)
		}

		// for the returned snapshots, we need the xxx$$proto version of the source volume ID
		// instead of the integer key
		var sourceVolumeId string

		for _, sv := range req.SourceVolumeIds {
			idString := strconv.Itoa(m.FamilyID)
			zlog.Debug().Msgf("contains check %s %s", sv, idString)
			if strings.Contains(sv, idString) {
				sourceVolumeId = sv
				zlog.Debug().Msgf("found member id %d is proto %s", m.ID, sourceVolumeId)
			}
		}

		zlog.Debug().Msgf("assembling snapshot result with ID %d sourceVolumeID %s", v.ID, sourceVolumeId)
		s := strings.Split(sourceVolumeId, "$$")
		if len(s) != 2 {
			return nil, status.Errorf(codes.Internal, "sourceVolumeId not parsing correctly %+v", s)
		}
		example := csi.Snapshot{
			SizeBytes:      int64(m.Size),
			SnapshotId:     strconv.Itoa(v.ID) + "$$" + s[1],
			SourceVolumeId: sourceVolumeId,
			//CreationTime:    timestamppb.New(time.Unix(int64(m.CreatedAt), 0)),
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
	zlog.Info().Msgf("CreateVolumeGroupSnapshot Finish - req %v", req)
	return resp, nil
}

func (s *VolumeGroupServer) DeleteVolumeGroupSnapshot(ctx context.Context, req *csi.DeleteVolumeGroupSnapshotRequest) (resp *csi.DeleteVolumeGroupSnapshotResponse, err error) {
	zlog.Info().Msgf("DeleteVolumeGroupSnapshot Start - req: %v", req)
	zlog.Debug().Msgf("group_snapshot_id %s", req.GroupSnapshotId)

	for _, s := range req.SnapshotIds {
		zlog.Debug().Msgf("Snap Group %s has snapshot ID %s", req.GroupSnapshotId, s)
	}

	cs, err := storage.BuildCommonService(make(map[string]string), req.Secrets)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get API connection error %v", err)
	}

	cl, err := cs.Api.NewClient()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get api client error %v", err)
	}

	sgID, err := strconv.ParseInt(req.GroupSnapshotId, 0, 64)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "failed to convert group_snapshot_id %s to int error %v", req.GroupSnapshotId, err)
	}
	err = cl.DeleteSG(int(sgID))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "error deleting SG %s error %v", req.GroupSnapshotId, err)
	}
	resp = &csi.DeleteVolumeGroupSnapshotResponse{}
	zlog.Info().Msgf("DeleteVolumeGroupSnapshot Finish - req %v", req)
	return resp, nil
}

func (s *VolumeGroupServer) GetVolumeGroupSnapshot(ctx context.Context, req *csi.GetVolumeGroupSnapshotRequest) (resp *csi.GetVolumeGroupSnapshotResponse, err error) {
	zlog.Info().Msgf("GetVolumeGroupSnapshot Start - req: %v", req)
	zlog.Debug().Msgf("req.GroupSnapshotId=%s", req.GroupSnapshotId)
	zlog.Debug().Msgf("req.SnapshotIds=%v", req.SnapshotIds)
	//zlog.Debug().Msgf("req.Secrets=%v", req.Secrets)

	cs, err := storage.BuildCommonService(make(map[string]string), req.Secrets)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get API connection error %v", err)
	}

	cl, err := cs.Api.NewClient()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get api client error %v", err)
	}

	sgID, err := strconv.ParseInt(req.GroupSnapshotId, 0, 64)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "failed to convert group_snapshot_id %s to int error %v", req.GroupSnapshotId, err)
	}

	//sgID is the volume ID of the snap group, the parent_id will be the cg MASTER volume
	cg, err := cl.GetCGByID(int(sgID))
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "error getting CG group_snapshot_id %s error %v", req.GroupSnapshotId, err)
	}

	creationTime := timestamppb.New(time.Now())

	// get snapgroup members, then make a list of Snapshots to return based on those members
	var members []api.MemberInfo
	members, err = cl.GetMembersByCGID(int(sgID))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get snapgroup CG members error %+v", err)
	}
	zlog.Debug().Msgf("members from snapgroup CG %d", len(members))

	snapshots := make([]*csi.Snapshot, 0)
	for _, m := range members {
		snapshotName := m.CGName + m.Name // prefix + volume name
		zlog.Debug().Msgf("member is snapshot name [%s] member info %+v", snapshotName, m)
		v, err := cl.GetVolume(m.ID)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "failed to get snapshot volume  error %v", err)
		}

		// for the returned snapshots, we need the xxx$$proto version of the source volume ID
		// instead of the integer key
		var sourceVolumeId string

		for _, sv := range req.SnapshotIds {
			//idString := strconv.Itoa(m.FamilyID)
			idString := strconv.Itoa(m.ID)
			zlog.Debug().Msgf("contains check %s %s", sv, idString)
			if strings.Contains(sv, idString) {
				sourceVolumeId = sv
				zlog.Debug().Msgf("found member id %d is proto %s", m.ID, sourceVolumeId)
			}
		}

		zlog.Debug().Msgf("assembling snapshot result with ID %d sourceVolumeID %s", v.ID, sourceVolumeId)
		s := strings.Split(sourceVolumeId, "$$")
		if len(s) != 2 {
			return nil, status.Errorf(codes.InvalidArgument, "sourceVolumeId not parsing correctly %+v", s)
		}
		example := csi.Snapshot{
			SizeBytes: int64(m.Size),
			//SnapshotId:     strconv.Itoa(v.ID), // the ID of the snapshot volume
			SnapshotId:     strconv.Itoa(v.ID) + "$$" + s[1],
			SourceVolumeId: sourceVolumeId,
			//CreationTime:    timestamppb.New(time.Unix(int64(m.CreatedAt), 0)),
			CreationTime:    creationTime,
			ReadyToUse:      true,
			GroupSnapshotId: cg.Name,
		}
		snapshots = append(snapshots, &example)
	}
	zlog.Debug().Msgf("snapshots/members %d", len(snapshots))

	resp = &csi.GetVolumeGroupSnapshotResponse{
		GroupSnapshot: &csi.VolumeGroupSnapshot{
			GroupSnapshotId: strconv.Itoa(int(sgID)),
			Snapshots:       snapshots,
			CreationTime:    creationTime, //TODO fix this with the right creation time
			ReadyToUse:      true,
		},
	}
	zlog.Info().Msgf("GetVolumeGroupSnapshot Finish - req %v", req)
	return resp, nil
}

func (s *VolumeGroupServer) GroupControllerGetCapabilities(ctx context.Context, req *csi.GroupControllerGetCapabilitiesRequest) (*csi.GroupControllerGetCapabilitiesResponse, error) {
	return &csi.GroupControllerGetCapabilitiesResponse{
		Capabilities: s.Driver.groupcap,
	}, nil
}
