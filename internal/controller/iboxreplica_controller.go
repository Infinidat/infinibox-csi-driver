/*
Copyright 2024.

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

package controller

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"infinibox-csi-driver/api"
	"infinibox-csi-driver/api/clientgo"
	csidriverinfinidatcomv1 "infinibox-csi-driver/api/v1"
	"infinibox-csi-driver/common"
	"infinibox-csi-driver/iboxapi"
)

const (
	IBOXREPLICA_SYNC_INTERVAL_DEFAULT        = 240000
	IBOXREPLICA_BASE_ACTION_NEW              = "NEW"
	IBOXREPLICA_RPO_VALUE_DEFAULT            = 300000
	IBOXREPLICA_LINK_STATE_UP                = "UP"
	IBOXREPLICA_LINK_WITNESS_RESILIENCY_MODE = "WITNESS"
)

// IboxreplicaReconciler reconciles a Iboxreplica object
type IboxreplicaReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=csidriver.infinidat.com,resources=iboxreplicas,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=csidriver.infinidat.com,resources=iboxreplicas/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=csidriver.infinidat.com,resources=iboxreplicas/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Iboxreplica object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/reconcile

var thislog logr.Logger

func (r *IboxreplicaReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	// TODO(user): your logic here

	thislog = log.FromContext(ctx)

	// TODO(user): your logic here
	// lookup the Iboxreplica instance for this reconcile request
	replica := &csidriverinfinidatcomv1.Iboxreplica{}
	err := r.Get(ctx, req.NamespacedName, replica)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if err := r.handleFinalizer(ctx, *replica); err != nil {
		thislog.Error(err, "failed to update finalizer error")
		return ctrl.Result{}, err
	}

	if !replica.DeletionTimestamp.IsZero() {
		// handle delete and return
		thislog.Info("cr was deleted", "replica name", req.Name, "namespace", req.Namespace, "replica ID", replica.Status.ID)
		err = r.deleteReplica(replica)
		if err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// handle a new CR
	if replica.Status.ID == 0 {
		err = r.createReplica(replica)
		if err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// fetch the replica from the ibox, create if not found

	if replica.Status.ID != 0 {
		// update the replica state
		err = r.updateIboxreplicaState(replica)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	thislog.Info("reconcile worked", "replica name", replica.Name, "namespace", replica.Namespace)

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *IboxreplicaReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&csidriverinfinidatcomv1.Iboxreplica{}).
		Complete(r)
}

func (r *IboxreplicaReconciler) handleFinalizer(ctx context.Context, obj csidriverinfinidatcomv1.Iboxreplica) error {
	name := "infinidat.com/iboxreplica"
	if obj.DeletionTimestamp.IsZero() {
		// add finalizer in case of create/update
		if !controllerutil.ContainsFinalizer(&obj, name) {
			ok := controllerutil.AddFinalizer(&obj, name)
			thislog.Info("Add Finalizer", "name", name, "ok", ok)
			return r.Update(ctx, &obj)
		}
	} else {
		// remove finalizer in case of deletion
		if controllerutil.ContainsFinalizer(&obj, name) {
			ok := controllerutil.RemoveFinalizer(&obj, name)
			thislog.Info("Remove Finalizer", "name", name, "ok", ok)
			return r.Update(ctx, &obj)
		}
	}
	return nil
}

func (r *IboxreplicaReconciler) createReplica(replica *csidriverinfinidatcomv1.Iboxreplica) error {
	// set defaults for optional CR fields
	// we only support ASYNC and ACTIVE_ACTIVE for replication types
	switch replica.Spec.ReplicationType {
	case common.IBOXREPLICA_REPLICA_TYPE_ACTIVE_ACTIVE:
	case common.IBOXREPLICA_REPLICA_TYPE_ASYNC:
		replica.Spec.IsPreferred = nil
		thislog.Info("creating replica", "setting is_preferred to nil", replica.Name)
	default:
		err := fmt.Errorf("error invalid ReplicationType in CR %s", replica.Spec.ReplicationType)
		thislog.Error(err, fmt.Sprintf("supported values include %s and %s", common.IBOXREPLICA_REPLICA_TYPE_ACTIVE_ACTIVE, common.IBOXREPLICA_REPLICA_TYPE_ASYNC))
		replica.Status = csidriverinfinidatcomv1.IboxreplicaStatus{
			State: err.Error(),
		}
		if e := r.Status().Update(context.Background(), replica); e != nil {
			thislog.Error(e, "unable to update iboxreplica state")
		}
		return err
	}

	// we only support a base action of NEW, default to NEW if not set by user
	if replica.Spec.BaseAction == "" {
		replica.Spec.BaseAction = IBOXREPLICA_BASE_ACTION_NEW
	}

	if replica.Spec.BaseAction != IBOXREPLICA_BASE_ACTION_NEW {
		err := fmt.Errorf("error invalid base action in CR %s", replica.Spec.BaseAction)
		thislog.Error(err, fmt.Sprintf("supported values include %s", IBOXREPLICA_BASE_ACTION_NEW))
		replica.Status = csidriverinfinidatcomv1.IboxreplicaStatus{
			State: err.Error(),
		}
		if e := r.Status().Update(context.Background(), replica); e != nil {
			thislog.Error(e, "unable to update iboxreplica state")
		}
		return err
	}

	// rpo_value and sync_interval are 0 for AA replication type so they should be left as 0 values
	if replica.Spec.ReplicationType == common.IBOXREPLICA_REPLICA_TYPE_ASYNC {
		if replica.Spec.RpoValue == 0 {
			replica.Spec.RpoValue = IBOXREPLICA_RPO_VALUE_DEFAULT
		}
		if replica.Spec.SyncInterval == 0 {
			replica.Spec.SyncInterval = IBOXREPLICA_SYNC_INTERVAL_DEFAULT
		}
	}

	clientsvc, err := getClientService(replica)
	if err != nil {
		thislog.Error(err, "error getting clientService")
		replica.Status = csidriverinfinidatcomv1.IboxreplicaStatus{
			State: err.Error(),
		}
		if e := r.Status().Update(context.Background(), replica); e != nil {
			thislog.Error(e, "unable to update iboxreplica state")
		}
		return err
	}

	thislog.Info("creating replica", "replica.Name", replica.Name)
	var localEntityID int

	switch replica.Spec.EntityType {
	case common.REPLICA_ENTITY_CONSISTENCY_GROUP:
		// look up the CG ID
		consistencyGroup, err := clientsvc.IboxAPI.GetConsistencyGroupByName(replica.Spec.LocalEntityName)
		if err != nil {
			thislog.Error(err, "error getting CG", "localEntityName", replica.Spec.LocalEntityName)
			replica.Status = csidriverinfinidatcomv1.IboxreplicaStatus{
				State: err.Error(),
			}
			if e := r.Status().Update(context.Background(), replica); e != nil {
				thislog.Error(e, "unable to update iboxreplica state")
			}
			return err
		}
		localEntityID = consistencyGroup.ID
	case common.REPLICA_ENTITY_VOLUME:
		// look up the volume ID
		volume, err := clientsvc.IboxAPI.GetVolumeByName(replica.Spec.LocalEntityName)
		if err != nil {
			thislog.Error(err, "error getting Volume", "localEntityName", replica.Spec.LocalEntityName)
			replica.Status = csidriverinfinidatcomv1.IboxreplicaStatus{
				State: err.Error(),
			}
			if e := r.Status().Update(context.Background(), replica); e != nil {
				thislog.Error(e, "unable to update iboxreplica state")
			}
			return err
		}
		localEntityID = volume.ID
	case common.REPLICA_ENTITY_FILESYSTEM:
		// look up the filesystem ID
		fileSystem, err := clientsvc.IboxAPI.GetFileSystemByName(replica.Spec.LocalEntityName)
		if err != nil {
			thislog.Error(err, "error getting file system", "localEntityName", replica.Spec.LocalEntityName)
			replica.Status = csidriverinfinidatcomv1.IboxreplicaStatus{
				State: err.Error(),
			}
			if e := r.Status().Update(context.Background(), replica); e != nil {
				thislog.Error(e, "unable to update iboxreplica state")
			}
			return err
		}
		localEntityID = fileSystem.ID
	default:
		err = fmt.Errorf("error getting local entity type, unknown entity type in the CR %s", replica.Spec.EntityType)
		thislog.Error(err, "error invalid CR entity type")
		replica.Status = csidriverinfinidatcomv1.IboxreplicaStatus{
			State: err.Error(),
		}
		if e := r.Status().Update(context.Background(), replica); e != nil {
			thislog.Error(e, "unable to update iboxreplica state")
		}
		return err
	}

	thislog.Info("creating replica", "entity look up worked", localEntityID)

	// look up the link ID
	links, err := clientsvc.IboxAPI.GetLinks()
	if err != nil {
		thislog.Error(err, "error getting replication links")
		replica.Status = csidriverinfinidatcomv1.IboxreplicaStatus{
			State: err.Error(),
		}
		if e := r.Status().Update(context.Background(), replica); e != nil {
			thislog.Error(e, "unable to update iboxreplica state")
		}
		return err
	}

	var link iboxapi.Link
	var linkFound bool
	for i := range links {
		if links[i].RemoteSystemName == replica.Spec.LinkRemoteSystemName {
			link = links[i]
			linkFound = true
		}
	}
	if !linkFound {
		err := fmt.Errorf("could not find replication link %s", replica.Spec.LinkRemoteSystemName)
		thislog.Error(err, "error could not find replication link", "linkRemoteSystemName", replica.Spec.LinkRemoteSystemName)
		replica.Status = csidriverinfinidatcomv1.IboxreplicaStatus{
			State: err.Error(),
		}
		if e := r.Status().Update(context.Background(), replica); e != nil {
			thislog.Error(e, "unable to update iboxreplica state")
		}
		return err
	}
	if link.ID == 0 {
		err := fmt.Errorf("link ID was 0 for replication link %s", replica.Spec.LinkRemoteSystemName)
		thislog.Error(err, "error link ID == 0", "linkRemoteSystemName", replica.Spec.LinkRemoteSystemName)
		replica.Status = csidriverinfinidatcomv1.IboxreplicaStatus{
			State: err.Error(),
		}
		if e := r.Status().Update(context.Background(), replica); e != nil {
			thislog.Error(e, "unable to update iboxreplica state")
		}
		return err
	}

	// validate the link state as being UP
	if link.LinkState != IBOXREPLICA_LINK_STATE_UP {
		err := fmt.Errorf("replication link state %s was invalid for replication link %s", link.LinkState, replica.Spec.LinkRemoteSystemName)
		thislog.Error(err, "error invalid replication link state", "state", link.LinkState, "linkRemoteSystemName", replica.Spec.LinkRemoteSystemName)
		replica.Status = csidriverinfinidatcomv1.IboxreplicaStatus{
			State: err.Error(),
		}
		if e := r.Status().Update(context.Background(), replica); e != nil {
			thislog.Error(e, "unable to update iboxreplica state")
		}
		return err
	}

	// validate the link supports ACTIVE_ACTIVE
	if link.ResiliencyMode != IBOXREPLICA_LINK_WITNESS_RESILIENCY_MODE {
		err := fmt.Errorf("invalid replication link resiliency mode %s for replication link %s", link.ResiliencyMode, replica.Spec.LinkRemoteSystemName)
		thislog.Error(err, "error invalid replication link resiliency mode", "resiliency mode", link.ResiliencyMode, "required mode", IBOXREPLICA_LINK_WITNESS_RESILIENCY_MODE)
		replica.Status = csidriverinfinidatcomv1.IboxreplicaStatus{
			State: err.Error(),
		}
		if e := r.Status().Update(context.Background(), replica); e != nil {
			thislog.Error(e, "unable to update iboxreplica state")
		}
		return err
	}

	// verify that a replica for this entity doesn't already exist
	replicas, err := clientsvc.IboxAPI.GetReplicas()
	if err != nil {
		thislog.Error(err, "error getting replicas")
		replica.Status = csidriverinfinidatcomv1.IboxreplicaStatus{
			State: err.Error(),
		}
		if e := r.Status().Update(context.Background(), replica); e != nil {
			thislog.Error(e, "unable to update iboxreplica state")
		}
		return err
	}
	for i := range replicas {
		if replicas[i].LocalEntityID == localEntityID {
			thislog.Info("creating replica", "entity already replicated ", localEntityID)
			return nil
		}
	}

	thislog.Info("creating replica", "link look up worked", link.ID)
	request := iboxapi.CreateReplicaRequest{
		IsPreferred:     replica.Spec.IsPreferred,
		SyncInterval:    replica.Spec.SyncInterval,
		Description:     replica.Spec.Description,
		EntityType:      replica.Spec.EntityType,
		LocalEntityID:   localEntityID,
		ReplicationType: replica.Spec.ReplicationType,
		BaseAction:      replica.Spec.BaseAction,
		LinkID:          link.ID,
		RpoValue:        replica.Spec.RpoValue,
		RemotePoolID:    replica.Spec.RemotePoolID,
	}

	response, err := clientsvc.IboxAPI.CreateReplica(request)
	if err != nil {
		thislog.Error(err, "error creating replica")
		replica.Status = csidriverinfinidatcomv1.IboxreplicaStatus{
			State: err.Error(),
		}
		if e := r.Status().Update(context.Background(), replica); e != nil {
			thislog.Error(e, "unable to update iboxreplica state")
		}
		return err
	}

	err = r.Get(context.Background(), types.NamespacedName{Name: replica.Name, Namespace: replica.Namespace}, replica)
	if err != nil {
		thislog.Error(err, "error getting replica")
		replica.Status = csidriverinfinidatcomv1.IboxreplicaStatus{
			State: err.Error(),
		}
		if e := r.Status().Update(context.Background(), replica); e != nil {
			thislog.Error(e, "unable to update iboxreplica state")
		}
		return err
	}

	// update the replica CR status ID field with the new Replica ID
	replica.Status = csidriverinfinidatcomv1.IboxreplicaStatus{
		ID:    response.ID,
		State: "created",
	}
	thislog.Info("creating replica", "create replica worked - status ID", replica.Status.ID, "response ID", response.ID)
	if err := r.Status().Update(context.Background(), replica); err != nil {
		thislog.Error(err, "unable to update replica status")
		return err
	}
	thislog.Info("cr was updated", "replica Status ID updated", replica.Status.ID)

	return nil
}

func (r *IboxreplicaReconciler) deleteReplica(replica *csidriverinfinidatcomv1.Iboxreplica) error {
	clientsvc, err := getClientService(replica)
	if err != nil {
		return err
	}

	err = clientsvc.IboxAPI.DeleteReplica(replica.Status.ID)
	if err != nil {
		thislog.Error(err, "error deleting replica", "ID", replica.Status.ID)
		return err
	}
	thislog.Info("DeleteReplica", "replica ID", replica.Status.ID)

	return nil
}

func (r *IboxreplicaReconciler) updateIboxreplicaState(replica *csidriverinfinidatcomv1.Iboxreplica) error {
	clientsvc, err := getClientService(replica)
	if err != nil {
		thislog.Error(err, "error getting clientService")
		return err
	}

	rep, err := clientsvc.IboxAPI.GetReplica(replica.Status.ID)
	if err != nil {
		thislog.Error(err, "error getting replica")
		return err
	}

	err = r.Get(context.Background(), types.NamespacedName{Name: replica.Name, Namespace: replica.Namespace}, replica)
	if err != nil {
		thislog.Error(err, "error getting iboxreplica")
		return err
	}

	// update the status of the Iboxreplica with the status of the actual replica status on the ibox
	replica.Status.State = rep.State

	if err := r.Status().Update(context.Background(), replica); err != nil {
		thislog.Error(err, "unable to update iboxreplica state")
		return err
	}
	thislog.Info("cr status was updated", "State", replica.Status.State, "ID", replica.Status.ID)

	// TODO updates by a user to the Iboxreplica are not currently supported, only the status is updated
	/**
	err = r.Update(context.Background(), replica)
	if err != nil {
		return err
	}
	*/

	return nil
}

func getClientService(replica *csidriverinfinidatcomv1.Iboxreplica) (*api.ClientService, error) {
	// get secret
	secretName := replica.Annotations[common.PVC_ANNOTATION_SECRET_NAME]
	secretNamespace := replica.Annotations[common.PVC_ANNOTATION_SECRET_NAMESPACE]

	if secretName == "" || secretNamespace == "" {
		return nil, fmt.Errorf("annotations for secret name and namespace are required")
	}

	// Get a k8s go client for in-cluster use
	cl, err := clientgo.BuildClient()
	if err != nil {
		return nil, err
	}

	secret, err := cl.GetSecret(secretName, secretNamespace)
	if err != nil {
		thislog.Error(err, "error getting secret", "secret_name", secretName, "secret_namespace", secretNamespace)
		return nil, err
	}

	x := api.ClientService{
		ConfigMap:  make(map[string]string),
		SecretsMap: secret,
	}

	clientsvc, err := x.NewClient()
	if err != nil {
		thislog.Error(err, "error getting ClientService")
		return nil, err
	}

	return clientsvc, nil
}
