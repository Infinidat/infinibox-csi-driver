/*
Copyright 2025.

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

	"github.com/infinidat/infinibox-csi-driver/api"
	"github.com/infinidat/infinibox-csi-driver/api/clientgo"
	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/infinidat/infinibox-csi-driver/iboxapi"
	csidriverinfinidatcomv1 "github.com/infinidat/infinibox-csi-driver/iboxpromote/api/v1"
)

const (
	IboxpromoteBaseActionNew = "NEW"
)

// IboxpromoteReconciler reconciles a Iboxpromote object
type IboxpromoteReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=csidriver.infinidat.com,resources=iboxpromotes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=csidriver.infinidat.com,resources=iboxpromotes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=csidriver.infinidat.com,resources=iboxpromotes/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Iboxpromote object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/reconcile

var logger logr.Logger

func (r *IboxpromoteReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	// TODO(user): your logic here

	logger = log.FromContext(ctx)

	// TODO(user): your logic here
	// lookup the Iboxpromote instance for this reconcile request
	promote := &csidriverinfinidatcomv1.Iboxpromote{}
	err := r.Get(ctx, req.NamespacedName, promote)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if err := r.handleFinalizer(ctx, *promote); err != nil {
		logger.Error(err, "failed to update finalizer error")
		return ctrl.Result{}, err
	}

	if !promote.DeletionTimestamp.IsZero() {
		// handle delete and return
		logger.Info("cr was deleted", "promote name", req.Name, "namespace", req.Namespace, "promote ID", promote.Status.ID)
		return ctrl.Result{}, nil
	}

	// handle a new CR
	if promote.Status.ID == 0 {
		err = r.createPromote(ctx, promote)
		if err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// fetch the promote from the ibox, create if not found

	if promote.Status.ID != 0 {
		// update the promote state
		err = r.updateIboxpromoteState(ctx, promote)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	logger.Info("reconcile worked", "promote name", promote.Name, "namespace", promote.Namespace)

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *IboxpromoteReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&csidriverinfinidatcomv1.Iboxpromote{}).
		Complete(r)
}

func (r *IboxpromoteReconciler) handleFinalizer(ctx context.Context, obj csidriverinfinidatcomv1.Iboxpromote) error {
	name := "infinidat.com/iboxpromote"
	if obj.DeletionTimestamp.IsZero() {
		// add finalizer in case of create/update
		if !controllerutil.ContainsFinalizer(&obj, name) {
			ok := controllerutil.AddFinalizer(&obj, name)
			logger.Info("Add Finalizer", "name", name, "ok", ok)
			return r.Update(ctx, &obj)
		}
	} else {
		// remove finalizer in case of deletion
		if controllerutil.ContainsFinalizer(&obj, name) {
			ok := controllerutil.RemoveFinalizer(&obj, name)
			logger.Info("Remove Finalizer", "name", name, "ok", ok)
			return r.Update(ctx, &obj)
		}
	}
	return nil
}

func (r *IboxpromoteReconciler) createPromote(ctx context.Context, promote *csidriverinfinidatcomv1.Iboxpromote) error {
	// set defaults for optional CR fields

	// we only support a base action of NEW, default to NEW if not set by user
	if promote.Spec.BaseAction == "" {
		promote.Spec.BaseAction = IboxpromoteBaseActionNew
	}

	if promote.Spec.BaseAction != IboxpromoteBaseActionNew {
		err := fmt.Errorf("error invalid base action in CR %s", promote.Spec.BaseAction)
		logger.Error(err, fmt.Sprintf("supported values include %s", IboxpromoteBaseActionNew))
		promote.Status = csidriverinfinidatcomv1.IboxpromoteStatus{
			State: err.Error(),
		}
		if e := r.Status().Update(ctx, promote); e != nil {
			logger.Error(e, "unable to update iboxpromote state")
		}
		return err
	}

	clientsvc, err := getClientService(ctx, promote)
	if err != nil {
		logger.Error(err, "error getting clientService")
		promote.Status = csidriverinfinidatcomv1.IboxpromoteStatus{
			State: err.Error(),
		}
		if e := r.Status().Update(ctx, promote); e != nil {
			logger.Error(e, "unable to update iboxpromote state")
		}
		return err
	}

	logger.Info("handling promote", "promote.Name", promote.Name)
	var entityID int

	var volume *iboxapi.Volume

	switch promote.Spec.EntityType {
	case "SNAPSHOT":
		// look up the snapshot by name
		volume, err = clientsvc.IboxAPI.GetVolumeByName(ctx, promote.Spec.EntityName)
		if err != nil {
			logger.Error(err, "error getting volume by ibox snapshot name", "EntityName", promote.Spec.EntityName)
			promote.Status = csidriverinfinidatcomv1.IboxpromoteStatus{
				State: err.Error(),
			}
			if e := r.Status().Update(ctx, promote); e != nil {
				logger.Error(e, "unable to update iboxpromote state")
			}
			return err
		}
		entityID = volume.ID
	default:
		err = fmt.Errorf("error getting entity type, unknown entity type in the CR %s", promote.Spec.EntityType)
		logger.Error(err, "error invalid CR entity type")
		promote.Status = csidriverinfinidatcomv1.IboxpromoteStatus{
			State: err.Error(),
		}
		if e := r.Status().Update(ctx, promote); e != nil {
			logger.Error(e, "unable to update iboxpromote state")
		}
		return err
	}

	logger.Info("handling promote", "entity look up worked", entityID)
	if volume.Type == "MASTER" {
		logger.Info("info", "volume", promote.Spec.EntityName, "already a MASTER, will not promote")
		return nil
	}

	response, err := clientsvc.IboxAPI.PromoteSnapshot(ctx, entityID)
	if err != nil {
		logger.Error(err, "error promoting snapshot")
		promote.Status = csidriverinfinidatcomv1.IboxpromoteStatus{
			State: err.Error(),
		}
		if e := r.Status().Update(ctx, promote); e != nil {
			logger.Error(e, "unable to update iboxpromote state")
		}
		return err
	}

	err = r.Get(ctx, types.NamespacedName{Name: promote.Name, Namespace: promote.Namespace}, promote)
	if err != nil {
		logger.Error(err, "error getting iboxpromote")
		promote.Status = csidriverinfinidatcomv1.IboxpromoteStatus{
			State: err.Error(),
		}
		if e := r.Status().Update(ctx, promote); e != nil {
			logger.Error(e, "unable to update iboxpromote state")
		}
		return err
	}

	// update the promote CR status ID field with the new Promote ID
	promote.Status = csidriverinfinidatcomv1.IboxpromoteStatus{
		ID:    response.ID,
		State: "created",
	}
	logger.Info("handling iboxpromote", "promote successful - status ID", promote.Status.ID, "response ID", response.ID)
	if err := r.Status().Update(ctx, promote); err != nil {
		logger.Error(err, "unable to update iboxpromote status")
		return err
	}
	logger.Info("iboxpromote CR was updated", "promote Status ID updated", promote.Status.ID)

	return nil
}

func (r *IboxpromoteReconciler) updateIboxpromoteState(ctx context.Context, promote *csidriverinfinidatcomv1.Iboxpromote) error {
	err := r.Get(ctx, types.NamespacedName{Name: promote.Name, Namespace: promote.Namespace}, promote)
	if err != nil {
		logger.Error(err, "error getting iboxpromote")
		return err
	}

	// update the status of the Iboxpromote with the status of the actual promote status on the ibox
	promote.Status.State = "completed"

	if err := r.Status().Update(ctx, promote); err != nil {
		logger.Error(err, "unable to update iboxpromote state")
		return err
	}
	logger.Info("iboxpromote CR status was updated", "State", promote.Status.State, "ID", promote.Status.ID)

	return nil
}

func getClientService(ctx context.Context, promote *csidriverinfinidatcomv1.Iboxpromote) (*api.ClientService, error) {
	// get secret
	secretName := promote.Annotations[common.PVCAnnotationSecretName]
	secretNamespace := promote.Annotations[common.PVCAnnotationSecretNamespace]

	if secretName == "" || secretNamespace == "" {
		return nil, fmt.Errorf("annotations for secret name and namespace are required")
	}

	// Get a k8s go client for in-cluster use
	cl, err := clientgo.BuildClient()
	if err != nil {
		return nil, err
	}

	secret, err := cl.GetSecret(ctx, secretName, secretNamespace)
	if err != nil {
		logger.Error(err, "error getting secret", "secret_name", secretName, "secret_namespace", secretNamespace)
		return nil, err
	}

	x := api.ClientService{
		ConfigMap:  make(map[string]string),
		SecretsMap: secret,
	}

	clientsvc, err := x.NewClient()
	if err != nil {
		logger.Error(err, "error getting ClientService")
		return nil, err
	}

	return clientsvc, nil
}
