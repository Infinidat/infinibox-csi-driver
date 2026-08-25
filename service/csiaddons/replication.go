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
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"time"

	"github.com/infinidat/infinibox-csi-driver/api/clientgo"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
	v1 "k8s.io/api/core/v1"

	"github.com/infinidat/infinibox-csi-driver/iboxapi"
	storagecommon "github.com/infinidat/infinibox-csi-driver/storage/common"

	"github.com/csi-addons/spec/lib/go/replication"
	"github.com/infinidat/infinibox-csi-driver/common"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ReplicationServer struct of rbd CSI driver with supported methods of Replication
// controller server spec.
type ReplicationServer struct {
	// added UnimplementedControllerServer as a member of
	// ControllerServer. if replication spec add more RPC services in the proto
	// file, then we don't need to add all RPC methods leading to forward
	// compatibility.
	*replication.UnimplementedControllerServer
	// driverInstance is the unique ID for this CSI-driver deployment.
	driverInstance string
}

// NewReplicationServer creates a new ReplicationServer which handles
// the Replication Service requests from the CSI-Addons specification.
func NewReplicationServer(instanceID string) *ReplicationServer {
	return &ReplicationServer{
		driverInstance: instanceID,
	}
}

func (rs *ReplicationServer) RegisterService(server grpc.ServiceRegistrar) {
	slog.Debug("AddOns ReplicationServer", "RegisterService", "not implemented yet")
	replication.RegisterControllerServer(server, rs)
}

// EnableVolumeReplication extracts the RBD volume information from the
// volumeID, If the image is present it will enable the mirroring based on the
// user provided information.
func (rs *ReplicationServer) EnableVolumeReplication(ctx context.Context,
	req *replication.EnableVolumeReplicationRequest,
) (*replication.EnableVolumeReplicationResponse, error) {
	slog.Debug("AddOns ReplicationServer", "EnableVolumeReplication", "not implemented yet")
	//slog.Debug("AddOns ReplicationServer EnableVolumeReplication", "secrets", req.Secrets)
	slog.Debug("AddOns ReplicationServer EnableVolumeReplication", "volumeId", req.ReplicationSource.GetVolume().VolumeId)
	slog.Debug("AddOns ReplicationServer EnableVolumeReplication", "parameters", req.Parameters)
	//slog.Debug("AddOns ReplicationServer EnableVolumeReplication", "volumeGroupId", req.ReplicationSource.GetVolumegroup().VolumeGroupId)
	// validate required parameters (link_remote_system_name, remote_pool_name, remote_ibox_credential_name, remote_ibox_credential_namespace)

	linkRemoteSystemName := req.Parameters[common.IboxReplicaRemoteIboxLinkNameParameter]
	if linkRemoteSystemName == "" {
		e := common.Errorf("required parameter %s missing, volume ID %s", common.IboxReplicaRemoteIboxLinkNameParameter, req.ReplicationSource.GetVolume().VolumeId)
		slog.Error(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}
	remotePoolName := req.Parameters[common.IboxReplicaCreatePVCPoolName]
	if remotePoolName == "" {
		e := common.Errorf("required parameter %s missing, volume ID %s", common.IboxReplicaCreatePVCPoolName, req.ReplicationSource.GetVolume().VolumeId)
		slog.Error(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	remoteCredName := req.Parameters[common.IboxReplicaRemoteIboxCredNameParameter]
	if remoteCredName == "" {
		e := common.Errorf("required parameter %s missing, volume ID %s", common.IboxReplicaRemoteIboxCredNameParameter, req.ReplicationSource.GetVolume().VolumeId)
		slog.Error(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}
	remoteCredNamespace := req.Parameters[common.IboxReplicaRemoteIboxCredNamespaceParameter]
	if remoteCredNamespace == "" {
		e := common.Errorf("required parameter %s missing, volume ID %s", common.IboxReplicaRemoteIboxCredNamespaceParameter, req.ReplicationSource.GetVolume().VolumeId)
		slog.Error(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	//
	volumeInfo, err := storagecommon.ValidateVolumeID(req.ReplicationSource.GetVolume().VolumeId)
	if err != nil {
		e := common.Errorf("invalid - volume ID: %s error: %w", req.ReplicationSource.GetVolume().VolumeId, err)
		slog.Error(e.Error())
		//TODO check return error codes against spec for this gRPC function
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}
	slog.Debug("AddOns ReplicationServer EnableVolumeReplication", "volumeInfo", volumeInfo)

	// get volume
	config := map[string]string{}
	commonService, err := storagecommon.BuildCommonService(config, req.Secrets, &volumeInfo)
	if err != nil {
		e := common.Errorf("error building commonService - volume ID: %s error: %w", req.ReplicationSource.GetVolume().VolumeId, err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Unauthenticated, e.Error())
	}
	var volume *iboxapi.Volume
	volume, err = commonService.IboxAPI.GetVolume(ctx, volumeInfo.VolumeID)
	if err != nil {
		e := common.Errorf("GetVolume - failed to find volume ID: %d Error: %w", volumeInfo.VolumeID, err)
		slog.Error(e.Error())
		return nil, status.Error(codes.NotFound, e.Error())
	}
	slog.Debug("AddOns ReplicationServer EnableVolumeReplication got volume from ibox", "volumeID", volume.ID, "volumeName", volume.Name)

	// get pool info
	pool, err := commonService.IboxAPI.GetPoolByName(ctx, remotePoolName)
	if err != nil {
		e := common.Errorf("GetPoolByName - failed to find pool name: %s Error: %w", remotePoolName, err)
		slog.Error(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	// validate the name and state of the link
	link, err := validateLink(ctx, commonService, linkRemoteSystemName)
	if err != nil {
		e := common.Errorf("validateLink - failed Error: %w", err)
		slog.Error(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}

	// verify that a replica for this entity doesn't already exist
	replicas, err := commonService.IboxAPI.GetReplicas(ctx)
	if err != nil {
		e := common.Errorf("GetReplicas - failed to get replicas Error: %w", err)
		slog.Error(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}
	for i := range replicas {
		if replicas[i].LocalEntityID == volume.ID {
			e := common.Errorf("AddOns ReplicationServer EnableVolumeReplication volume already replicated", "volumeID", volume.ID, "volumeName", volume.Name)
			slog.Error(e.Error())
			return nil, status.Error(codes.InvalidArgument, e.Error())
		}
	}

	request := iboxapi.CreateReplicaRequest{
		IsPreferred:      nil,
		SyncInterval:     240000,
		Description:      "description goes here",
		EntityType:       "VOLUME",
		LocalEntityID:    volume.ID,
		ReplicationType:  "ASYNC",
		BaseAction:       "NEW",
		LinkID:           link.ID,
		RpoValue:         300000,
		RemotePoolID:     pool.ID,
		RemoteEntityName: volume.Name,
	}

	response, err := commonService.IboxAPI.CreateReplica(ctx, request)
	if err != nil {
		e := common.Errorf("AddOns ReplicationServer EnableVolumeReplication replica creation failed", "volumeID", volume.ID, "volumeName", volume.Name, "error", err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Unknown, e.Error())
	}
	slog.Info("created replica", "replicaId", response.ID)

	//TODO create the replica's PV and PVC on the replica Kube cluster
	//
	return &replication.EnableVolumeReplicationResponse{}, nil
}

// DisableVolumeReplication extracts the RBD volume information from the
// volumeID, If the image is present and the mirroring is enabled on the RBD
// image it will disable the mirroring.
func (rs *ReplicationServer) DisableVolumeReplication(ctx context.Context,
	req *replication.DisableVolumeReplicationRequest,
) (*replication.DisableVolumeReplicationResponse, error) {
	slog.Debug("AddOns ReplicationServer", "DisableVolumeReplication", "not implemented yet")
	//slog.Debug("AddOns ReplicationServer DisableVolumeReplication", "secrets", req.Secrets)
	slog.Debug("AddOns ReplicationServer DisableVolumeReplication", "volumeId", req.ReplicationSource.GetVolume().VolumeId)
	slog.Debug("AddOns ReplicationServer DisableVolumeReplication", "parameters", req.Parameters)
	//slog.Debug("AddOns ReplicationServer DisableVolumeReplication", "volumeGroupId", req.ReplicationSource.GetVolumegroup().VolumeGroupId)
	volumeInfo, err := storagecommon.ValidateVolumeID(req.ReplicationSource.GetVolume().VolumeId)
	if err != nil {
		e := common.Errorf("invalid - volume ID: %s error: %w", req.ReplicationSource.GetVolume().VolumeId, err)
		slog.Error(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}
	slog.Debug("AddOns ReplicationServer DisableVolumeReplication", "volumeInfo", volumeInfo)

	// get volume
	config := map[string]string{}
	commonService, err := storagecommon.BuildCommonService(config, req.Secrets, &volumeInfo)
	if err != nil {
		e := common.Errorf("error building commonService - volume ID: %s error: %w", req.ReplicationSource.GetVolume().VolumeId, err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Unauthenticated, e.Error())
	}
	var volume *iboxapi.Volume
	volume, err = commonService.IboxAPI.GetVolume(ctx, volumeInfo.VolumeID)
	if err != nil {
		e := common.Errorf("GetVolume - failed to find volume ID: %d Error: %w", volumeInfo.VolumeID, err)
		slog.Error(e.Error())
		return nil, status.Error(codes.NotFound, e.Error())
	}

	replica, err := commonService.IboxAPI.GetReplicaForLocalEntityName(ctx, volume.Name)
	if err != nil {
		e := common.Errorf("AddOns ReplicationServer DisableVolumeReplication replica get failed", "volumeID", volume.ID, "volumeName", volume.Name, "error", err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Unknown, e.Error())
	}

	err = commonService.IboxAPI.DeleteReplica(ctx, replica.ID)
	if err != nil {
		e := common.Errorf("AddOns ReplicationServer DisableVolumeReplication replica delete failed", "volumeID", volume.ID, "volumeName", volume.Name, "replicaID", replica.ID, "error", err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Unknown, e.Error())
	}

	//TODO remove the replica PV and PVC on the replica kube cluster
	//
	slog.Info("deleted replica", "replicaId", replica.ID, "volumeName", volume.Name)
	return &replication.DisableVolumeReplicationResponse{}, nil
}

// PromoteVolume extracts the RBD volume information from the volumeID, If the
// image is present, mirroring is enabled and the image is in demoted state it
// will promote the volume as primary.
// If the image is already primary it will return success.
func (rs *ReplicationServer) PromoteVolume(ctx context.Context,
	req *replication.PromoteVolumeRequest,
) (*replication.PromoteVolumeResponse, error) {
	slog.Debug("AddOns ReplicationServer", "PromoteVolume", "not implemented yet")
	//slog.Debug("AddOns ReplicationServer PromoteVolume", "secrets", req.Secrets)
	slog.Debug("AddOns ReplicationServer PromoteVolume", "volumeId", req.ReplicationSource.GetVolume().VolumeId)
	slog.Debug("AddOns ReplicationServer PromoteVolume", "parameters", req.Parameters)
	//slog.Debug("AddOns ReplicationServer PromoteVolume", "volumeGroupId", req.ReplicationSource.GetVolumegroup().VolumeGroupId)
	volumeInfo, err := storagecommon.ValidateVolumeID(req.ReplicationSource.GetVolume().VolumeId)
	if err != nil {
		e := common.Errorf("invalid - volume ID: %s error: %w", req.ReplicationSource.GetVolume().VolumeId, err)
		slog.Error(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}
	slog.Debug("AddOns ReplicationServer PromoteVolume", "volumeInfo", volumeInfo)
	// get volume
	config := map[string]string{}
	commonService, err := storagecommon.BuildCommonService(config, req.Secrets, &volumeInfo)
	if err != nil {
		e := common.Errorf("error building commonService - volume ID: %s error: %w", req.ReplicationSource.GetVolume().VolumeId, err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Unauthenticated, e.Error())
	}

	allLinks, err := commonService.IboxAPI.GetLinks(ctx)
	if err != nil {
		e := common.Errorf("AddOns ReplicationServer PromoteVolume get all links failed", "error", err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Unknown, e.Error())
	}

	var volume *iboxapi.Volume
	volume, err = commonService.IboxAPI.GetVolume(ctx, volumeInfo.VolumeID)
	if err != nil {
		e := common.Errorf("GetVolume - failed to find volume ID: %d Error: %w", volumeInfo.VolumeID, err)
		slog.Error(e.Error())
		return nil, status.Error(codes.NotFound, e.Error())
	}
	slog.Debug("AddOns ReplicationServer PromoteVolume", "volumeId", volume.ID, "volumeName", volume.Name)

	// get the replica
	replica, err := commonService.IboxAPI.GetReplicaForLocalEntityName(ctx, volume.Name)
	if err != nil {
		if errors.Is(err, iboxapi.ErrNotFound) {
			slog.Error("replica not found, doing nothing", "volumeName", volume.Name, "error", err)
			return &replication.PromoteVolumeResponse{}, nil
		} else {
			e := common.Errorf("AddOns ReplicationServer PromoteVolume get replica failed", "volumeID", volume.ID, "volumeName", volume.Name, "error", err)
			slog.Error(e.Error())
			return nil, status.Error(codes.Unknown, e.Error())
		}
	}
	slog.Debug("AddOns ReplicationServer PromoteVolume existing replica found", "volumeName", volume.Name, "replicaID", replica.ID)

	if replica.Role == "SOURCE" {
		slog.Info("existing replica role is SOURCE, no need to promote", "volumeName", volume.Name, "replicaId", replica.ID)
		return &replication.PromoteVolumeResponse{}, nil
	}

	remoteLink, err := commonService.IboxAPI.GetLink(ctx, replica.LinkID)
	if err != nil {
		e := common.Errorf("GetLink - failed to find link ID: %d Error: %w", replica.LinkID, err)
		slog.Error(e.Error())
		return nil, status.Error(codes.NotFound, e.Error())
	}
	slog.Debug("AddOns ReplicationServer PromoteVolume", "linkID", remoteLink.ID, "link remote system name", remoteLink.RemoteSystemName)

	// using the link's remote system name, we will look for a Secret to that remote system
	// using the remote system name which should be part of the Secret name (our convention!)
	// (e.g. ibox2382 link remote system name would have a secret like ibox2382-ibox-creds)
	secret, err := getSecretForLink(ctx, remoteLink.RemoteSystemName)
	if err != nil {
		e := common.Errorf("GetSecret - failed to find secret for remote system name: %s Error: %w", remoteLink.RemoteSystemName, err)
		slog.Error(e.Error())
		return nil, status.Error(codes.NotFound, e.Error())
	}

	secretMap := make(map[string]string)
	for key, value := range secret.Data {
		secretMap[key] = string(value)
	}
	maps.Copy(secretMap, secret.StringData)

	slog.Debug("AddOns ReplicationServer DemoteVolume secret for replica target", "secret Name", secret.Name)
	commonServiceSourceIbox, err := storagecommon.BuildCommonService(config, secretMap, &volumeInfo)
	if err != nil {
		e := common.Errorf("error building commonServiceSourceIbox - volume Name: %s remoteLink: %s error: %w", volume.Name, remoteLink.RemoteSystemName, err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Unauthenticated, e.Error())
	}
	// get the replica for this volume on the primary
	sourceReplica, err := commonServiceSourceIbox.IboxAPI.GetReplicaForLocalEntityName(ctx, volume.Name)
	if err != nil {
		e := common.Errorf("AddOns ReplicationServer PromoteVolume get source replica failed", "volumeID", volume.ID, "volumeName", volume.Name, "error", err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Unknown, e.Error())
	}
	slog.Debug("AddOns ReplicationServer PromoteVolume source replica found", "source replicaID", sourceReplica.ID)

	// delete the replica on the primary ibox
	err = commonServiceSourceIbox.IboxAPI.DeleteReplica(ctx, sourceReplica.ID)
	if err != nil {
		e := common.Errorf("AddOns ReplicationServer PromoteVolume source replica delete failed", "volumeID", volume.ID, "volumeName", volume.Name, "source replicaID", sourceReplica.ID, "error", err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Unknown, e.Error())
	}
	// create new replica on this ibox
	request := iboxapi.CreateReplicaRequest{
		IsPreferred:      nil,
		SyncInterval:     240000,
		Description:      "description goes here",
		EntityType:       "VOLUME",
		LocalEntityID:    volume.ID,
		ReplicationType:  "ASYNC",
		BaseAction:       "NEW",
		RpoValue:         300000,
		RemotePoolID:     replica.RemotePoolID,
		RemoteEntityName: volume.Name,
	}

	// find the correct link, our convention for link names is the remote system name (e.g. ibox2382)
	for _, v := range allLinks {
		if v.Name == remoteLink.RemoteSystemName {
			request.LinkID = v.ID
			slog.Debug("AddOns ReplicationServer PromoteVolume link found", "linkID", v.ID, "remoteSystemName", remoteLink.RemoteSystemName)
			break
		}
	}
	if request.LinkID == 0 {
		e := common.Errorf("AddOns ReplicationServer PromoteVolume link not found", "remoteSystemName", remoteLink.RemoteSystemName, "error", err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Unknown, e.Error())
	}

	response, err := commonService.IboxAPI.CreateReplica(ctx, request)
	if err != nil {
		e := common.Errorf("AddOns ReplicationServer PromoteVolume local replica creation failed", "volumeID", volume.ID, "volumeName", volume.Name, "error", err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Unknown, e.Error())
	}
	slog.Info("PromoteVolume - volume promoted - created replica", "volume name", volume.Name, "replicaId", response.ID)

	return &replication.PromoteVolumeResponse{}, nil
}

// DemoteVolume extracts the volume information from the
// volumeID, If the image is present, mirroring is enabled and the
// image is in promoted state it will demote the volume as secondary.
// If the image is already secondary it will return success.
func (rs *ReplicationServer) DemoteVolume(ctx context.Context,
	req *replication.DemoteVolumeRequest,
) (*replication.DemoteVolumeResponse, error) {
	slog.Debug("AddOns ReplicationServer", "DemoteVolume", "not implemented yet")
	//slog.Debug("AddOns ReplicationServer DemoteVolume", "secrets", req.Secrets)
	slog.Debug("AddOns ReplicationServer DemoteVolume", "volumeId", req.ReplicationSource.GetVolume().VolumeId)
	slog.Debug("AddOns ReplicationServer DemoteVolume", "parameters", req.Parameters)
	//slog.Debug("AddOns ReplicationServer DemoteVolume", "volumeGroupId", req.ReplicationSource.GetVolumegroup().VolumeGroupId)
	volumeInfo, err := storagecommon.ValidateVolumeID(req.ReplicationSource.GetVolume().VolumeId)
	if err != nil {
		e := common.Errorf("invalid - volume ID: %s error: %w", req.ReplicationSource.GetVolume().VolumeId, err)
		slog.Error(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}
	slog.Debug("AddOns ReplicationServer DemoteVolume", "volumeInfo", volumeInfo)

	// get volume
	config := map[string]string{}
	commonService, err := storagecommon.BuildCommonService(config, req.Secrets, &volumeInfo)
	if err != nil {
		e := common.Errorf("error building commonService - volume ID: %s error: %w", req.ReplicationSource.GetVolume().VolumeId, err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Unauthenticated, e.Error())
	}

	v, err := commonService.IboxAPI.GetVolume(ctx, volumeInfo.VolumeID)
	if err != nil {
		e := common.Errorf("GetVolume - failed to find volume ID: %d Error: %w", volumeInfo.VolumeID, err)
		slog.Error(e.Error())
		return nil, status.Error(codes.NotFound, e.Error())
	}
	slog.Debug("AddOns ReplicationServer DemoteVolume", "volumeId", v.ID, "volumeName", v.Name)

	// since we are getting called to Demote a volume there should be a replica for this volume
	replica, err := commonService.IboxAPI.GetReplicaForLocalEntityName(ctx, v.Name)
	if err != nil {
		if errors.Is(err, iboxapi.ErrNotFound) {
			slog.Error("replica not found, nothing to demote", "error", err)
			return &replication.DemoteVolumeResponse{}, nil
		}
		e := common.Errorf("AddOns ReplicationServer DemoteVolume get replica failed", "volumeID", v.ID, "volumeName", v.Name, "error", err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Unknown, e.Error())
	}
	slog.Debug("AddOns ReplicationServer DemoteVolume", "replicaID", replica.ID)

	if replica.Role == "TARGET" {
		slog.Debug("AddOns ReplicationServer DemoteVolume already a TARGET, demote not needed", "volumeName", v.Name, "replicaID", replica.ID)
		return &replication.DemoteVolumeResponse{}, nil
	}

	// get the replication link
	link, err := commonService.IboxAPI.GetLink(ctx, replica.LinkID)
	if err != nil {
		e := common.Errorf("GetLink - failed to find link ID: %d Error: %w", replica.LinkID, err)
		slog.Error(e.Error())
		return nil, status.Error(codes.NotFound, e.Error())
	}
	slog.Debug("AddOns ReplicationServer DemoteVolume", "linkID", link.ID, "link remote system name", link.RemoteSystemName)

	// using the link's remote system name, we will look for a Secret to that remote system
	// using the remote system name which should be part of the Secret name (our convention!)
	// (e.g. ibox2382 link remote system name would have a secret like ibox2382-ibox-creds)
	secret, err := getSecretForLink(ctx, link.RemoteSystemName)
	if err != nil {
		e := common.Errorf("GetSecret - failed to find secret for remote system name: %s Error: %w", link.RemoteSystemName, err)
		slog.Error(e.Error())
		return nil, status.Error(codes.NotFound, e.Error())
	}
	slog.Debug("AddOns ReplicationServer DemoteVolume secret for replica target", "secret Name", secret.Name)
	secretMap := make(map[string]string)
	for key, value := range secret.Data {
		secretMap[key] = string(value)
	}
	maps.Copy(secretMap, secret.StringData)
	commonServiceRemoteSystem, err := storagecommon.BuildCommonService(config, secretMap, &volumeInfo)
	if err != nil {
		e := common.Errorf("error building commonService for replica target - link Name: %s secret Name: %s error: %w", link.Name, secret.Name, err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Unauthenticated, e.Error())
	}

	// delete the replica which will let the secondary be read/write
	err = commonService.IboxAPI.DeleteReplica(ctx, replica.ID)
	if err != nil {
		e := common.Errorf("AddOns ReplicationServer DemoteVolume replica delete failed", "volumeID", v.ID, "volumeName", v.Name, "replicaID", replica.ID, "error", err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Unknown, e.Error())
	}
	// wait a bit for the replica to fully be removed, TODO make this wait smarter
	time.Sleep(5 * time.Second)

	// get the volume on the target
	volumeRemote, err := commonServiceRemoteSystem.IboxAPI.GetVolume(ctx, replica.RemoteEntityID)
	if err != nil {
		e := common.Errorf("GetVolume - failed to find volume ID (remote_entity_id): %d Link: %s Error: %w", replica.RemoteEntityID, link.RemoteSystemName, err)
		slog.Error(e.Error())
		return nil, status.Error(codes.NotFound, e.Error())
	}

	// create a replica on the remote system back to this system
	request := iboxapi.CreateReplicaRequest{
		IsPreferred:      nil,
		SyncInterval:     240000,
		Description:      "description goes here",
		EntityType:       "VOLUME",
		LocalEntityID:    volumeRemote.ID,
		ReplicationType:  "ASYNC",
		BaseAction:       "NEW",
		LinkID:           link.RemoteLinkID,
		RpoValue:         300000,
		RemotePoolID:     replica.LocalPoolID,
		RemoteEntityName: volumeRemote.Name,
	}

	// create the replica on the remote system
	response, err := commonServiceRemoteSystem.IboxAPI.CreateReplica(ctx, request)
	if err != nil {
		e := common.Errorf("AddOns ReplicationServer EnableVolumeReplication remote replica creation failed", "volumeID", volumeRemote.ID, "volumeName", volumeRemote.Name, "error", err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Unknown, e.Error())
	}
	slog.Info("DemoteVolume - successful", "volumeName", v.Name, "replicaId", response.ID)

	return &replication.DemoteVolumeResponse{}, nil
}

func (rs *ReplicationServer) ResyncVolume(ctx context.Context,
	req *replication.ResyncVolumeRequest,
) (*replication.ResyncVolumeResponse, error) {
	slog.Debug("AddOns ReplicationServer", "ResyncVolume", "not implemented yet")
	resp := &replication.ResyncVolumeResponse{
		Ready: true,
	}

	return resp, nil
}

// GetVolumeReplicationInfo extracts the RBD volume information from the volumeID, If the
// image is present, mirroring is enabled and the image is in primary state.
func (rs *ReplicationServer) GetVolumeReplicationInfo(ctx context.Context,
	req *replication.GetVolumeReplicationInfoRequest,
) (*replication.GetVolumeReplicationInfoResponse, error) {
	slog.Debug("AddOns ReplicationServer", "GetVolumeReplicationInfo", "started")
	//slog.Debug("AddOns ReplicationServer GetVolumeReplicationInfo", "secrets", req.Secrets)
	slog.Debug("AddOns ReplicationServer GetVolumeReplicationInfo", "volumeId", req.ReplicationSource.GetVolume().VolumeId)
	slog.Debug("AddOns ReplicationServer DemoteVolume", "replicationId", req.ReplicationId)

	volumeInfo, err := storagecommon.ValidateVolumeID(req.ReplicationSource.GetVolume().VolumeId)
	if err != nil {
		e := common.Errorf("invalid - volume ID: %s error: %w", req.ReplicationSource.GetVolume().VolumeId, err)
		slog.Error(e.Error())
		return nil, status.Error(codes.InvalidArgument, e.Error())
	}
	slog.Debug("AddOns ReplicationServer GetVolumeReplicationInfo", "volumeInfo", volumeInfo)

	// get volume
	config := map[string]string{}
	commonService, err := storagecommon.BuildCommonService(config, req.Secrets, &volumeInfo)
	if err != nil {
		e := common.Errorf("error building commonService - volume ID: %s error: %w", req.ReplicationSource.GetVolume().VolumeId, err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Unauthenticated, e.Error())
	}
	var volume *iboxapi.Volume
	volume, err = commonService.IboxAPI.GetVolume(ctx, volumeInfo.VolumeID)
	if err != nil {
		e := common.Errorf("GetVolume - failed to find volume ID: %d Error: %w", volumeInfo.VolumeID, err)
		slog.Error(e.Error())
		return nil, status.Error(codes.NotFound, e.Error())
	}

	// get replica for this volume
	replica, err := commonService.IboxAPI.GetReplicaForLocalEntityName(ctx, volume.Name)
	if err != nil {
		e := common.Errorf("AddOns ReplicationServer GetVolumeReplicationInfo replica get failed", "volumeID", volume.ID, "volumeName", volume.Name, "error", err)
		slog.Error(e.Error())
		return nil, status.Error(codes.Unknown, e.Error())
	}

	lastSyncTime := timestamppb.New(time.Unix(replica.LastSynchronized, 0))

	var syncStatus replication.GetVolumeReplicationInfoResponse_Status
	switch replica.State {
	case "ACTIVE":
		syncStatus = replication.GetVolumeReplicationInfoResponse_HEALTHY
	default:
		syncStatus = replication.GetVolumeReplicationInfoResponse_ERROR
	}
	//TODO figure out the ibox replica states other than ACTIVE
	//syncStatus = replication.GetVolumeReplicationInfoResponse_UNKNOWN
	//syncStatus = replication.GetVolumeReplicationInfoResponse_DEGRADED

	// replica.LastSynchronized int64
	// replica.SyncDuration int
	lastSyncDuration := durationpb.New(time.Duration(replica.SyncDuration))
	resp := replication.GetVolumeReplicationInfoResponse{
		LastSyncTime: lastSyncTime,
		//LastSyncBytes: optional ?? could not find in the replica
		LastSyncDuration: lastSyncDuration,
		Status:           syncStatus,
		StatusMessage:    replica.State,
	}
	return &resp, nil
}

func validateLink(ctx context.Context, commonService storagecommon.Commonservice, linkName string) (link *iboxapi.Link, err error) {
	// look up the link ID
	links, err := commonService.IboxAPI.GetLinks(ctx)
	if err != nil {
		return nil, err
	}

	var linkFound bool
	for i := range links {
		if links[i].RemoteSystemName == linkName {
			link = &links[i]
			linkFound = true
		}
	}
	if !linkFound {
		err := fmt.Errorf("could not find replication link %s", linkName)
		return nil, err
	}
	if link.ID == 0 {
		err := fmt.Errorf("link ID was 0 for replication link %s", linkName)
		return nil, err
	}

	// validate the link state as being UP
	if link.LinkState != "UP" {
		err := fmt.Errorf("replication link state %s was invalid for replication link %s", link.LinkState, linkName)
		return nil, err
	}

	return link, nil
}

func getSecretForLink(ctx context.Context, linkName string) (*v1.Secret, error) {
	// Get a k8s go client for in-cluster use
	client, err := clientgo.BuildClient()
	if err != nil {
		slog.Error("error", "getting client-go connection", err.Error())
		return nil, err
	}

	namespace := os.Getenv("POD_NAMESPACE")
	slog.Info("env", "POD_NAMESPACE", namespace)
	if namespace == "" {
		slog.Error("env var POD_NAMESPACE was not set, defaulting to infinidat-csi namespace")
		namespace = "infinidat-csi"
	}

	secret, err := client.GetSecretContainsName(ctx, namespace, linkName)
	if err != nil {
		slog.Error("error ", "getting secrets", err.Error())
		return nil, err
	}
	return secret, nil
}
