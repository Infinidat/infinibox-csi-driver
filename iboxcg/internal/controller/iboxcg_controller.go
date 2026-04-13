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
	"errors"
	"fmt"
	"time"

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
	csidriverinfinidatcomv1 "github.com/infinidat/infinibox-csi-driver/iboxcg/api/v1"
)

const (
	IboxcgBaseActionAdd    = "ADD"
	IboxcgBaseActionRemove = "REMOVE"
)

// IboxcgReconciler reconciles a Iboxcg object
type IboxcgReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=csidriver.infinidat.com,resources=iboxcgs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=csidriver.infinidat.com,resources=iboxcgs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=csidriver.infinidat.com,resources=iboxcgs/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Iboxcg object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/reconcile

var logger logr.Logger

func (r *IboxcgReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	// TODO(user): your logic here

	logger = log.FromContext(ctx)

	// TODO(user): your logic here
	// lookup the Iboxcg instance for this reconcile request
	cg := &csidriverinfinidatcomv1.Iboxcg{}
	err := r.Get(ctx, req.NamespacedName, cg)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if err := r.handleFinalizer(ctx, *cg); err != nil {
		logger.Error(err, "failed to update finalizer error")
		return ctrl.Result{}, err
	}

	if !cg.DeletionTimestamp.IsZero() {
		// handle delete and return
		logger.Info("cr was deleted", "cg name", req.Name, "namespace", req.Namespace, "cg ID", cg.Status.ID)
		return ctrl.Result{}, nil
	}

	// handle a new CR
	if cg.Status.ID == 0 {
		err = r.handleNewCR(ctx, cg)
		if err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	if cg.Status.ID != 0 {
		// update the cg state
		err = r.updateIboxcgState(ctx, cg)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	logger.Info("reconcile worked", "cg name", cg.Name, "namespace", cg.Namespace)

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *IboxcgReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&csidriverinfinidatcomv1.Iboxcg{}).
		Complete(r)
}

func (r *IboxcgReconciler) handleFinalizer(ctx context.Context, obj csidriverinfinidatcomv1.Iboxcg) error {
	name := "infinidat.com/iboxcg"
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

func (r *IboxcgReconciler) handleNewCR(ctx context.Context, cr *csidriverinfinidatcomv1.Iboxcg) error {
	logger.Info("handleNewCR called", "volume", cr.Spec.LocalVolumeName, "cg", cr.Spec.LocalCGName)
	// set defaults for optional CR fields
	err := r.Get(ctx, types.NamespacedName{Name: cr.Name, Namespace: cr.Namespace}, cr)
	if err != nil {
		logger.Error(err, "error getting iboxcg")
		cr.Status = csidriverinfinidatcomv1.IboxcgStatus{
			State: err.Error(),
		}
		if e := r.Status().Update(ctx, cr); e != nil {
			logger.Error(e, "unable to update iboxcg state")
		}
		return err
	}
	if cr.Status.State == "completed" {
		logger.Info("handleNewCR called but CR is already completed", "cr", cr.Name, "volume", cr.Spec.LocalVolumeName, "cg", cr.Spec.LocalCGName)
		return nil
	}

	clientsvc, err := getClientService(ctx, cr)
	if err != nil {
		logger.Error(err, "error getting clientService")
		cr.Status = csidriverinfinidatcomv1.IboxcgStatus{
			State: err.Error(),
		}
		if e := r.Status().Update(ctx, cr); e != nil {
			logger.Error(e, "unable to update iboxcg state")
		}
		return err
	}

	// look up the volume by name
	localVolume, err := clientsvc.IboxAPI.GetVolumeByName(ctx, cr.Spec.LocalVolumeName)
	if err != nil {
		logger.Error(err, "error getting volume by ibox snapshot name", "LocalVolumeName", cr.Spec.LocalVolumeName)
		cr.Status = csidriverinfinidatcomv1.IboxcgStatus{
			State: err.Error(),
		}
		if e := r.Status().Update(ctx, cr); e != nil {
			logger.Error(e, "unable to update iboxcg state")
		}
		return err
	}
	logger.Info("validated volume exists", "local volume", localVolume.Name)

	// look up the cg by name
	localCG, err := clientsvc.IboxAPI.GetConsistencyGroupByName(ctx, cr.Spec.LocalCGName)
	if err != nil {
		logger.Error(err, "error getting volume by ibox snapshot name", "LocalVolumeName", cr.Spec.LocalVolumeName)
		cr.Status = csidriverinfinidatcomv1.IboxcgStatus{
			State: err.Error(),
		}
		if e := r.Status().Update(ctx, cr); e != nil {
			logger.Error(e, "unable to update iboxcg state")
		}
		return err
	}
	logger.Info("validated cg exists", "cg.Name", localCG.Name)

	// default to ADD if not set by user
	if cr.Spec.BaseAction == "" {
		cr.Spec.BaseAction = IboxcgBaseActionAdd
	}

	cgMembers, err := clientsvc.IboxAPI.GetMembersByCGID(ctx, localCG.ID)
	if err != nil {
		return err
	}

	switch cr.Spec.BaseAction {
	case IboxcgBaseActionAdd:
		err := r.handleAddMember(ctx, cr, localVolume, localCG, clientsvc, cgMembers)
		if err != nil {
			logger.Error(err, "error in add member logic", "LocalVolumeName", cr.Spec.LocalVolumeName, "LocalCG", cr.Spec.LocalCGName)
			cr.Status = csidriverinfinidatcomv1.IboxcgStatus{
				State: err.Error(),
			}
			if e := r.Status().Update(ctx, cr); e != nil {
				logger.Error(e, "unable to add member iboxcg state")
			}
			return err
		}
	case IboxcgBaseActionRemove:
		err := r.handleRemoveMember(ctx, cr, localVolume, localCG, clientsvc, cgMembers)
		if err != nil {
			logger.Error(err, "error in remove member logic", "LocalVolumeName", cr.Spec.LocalVolumeName, "LocalCG", cr.Spec.LocalCGName)
			cr.Status = csidriverinfinidatcomv1.IboxcgStatus{
				State: err.Error(),
			}
			if e := r.Status().Update(ctx, cr); e != nil {
				logger.Error(e, "unable to remove member iboxcg state")
			}
			return err
		}
	default:
		err := fmt.Errorf("error invalid base action in CR %s", cr.Spec.BaseAction)
		logger.Error(err, fmt.Sprintf("supported values include %s, %s", IboxcgBaseActionAdd, IboxcgBaseActionRemove))
		cr.Status = csidriverinfinidatcomv1.IboxcgStatus{
			State: err.Error(),
		}
		if e := r.Status().Update(ctx, cr); e != nil {
			logger.Error(e, "unable to update iboxcg state")
		}
		return err
	}

	logger.Info("handling cg", "cg.Name", cr.Name)

	err = r.Get(ctx, types.NamespacedName{Name: cr.Name, Namespace: cr.Namespace}, cr)
	if err != nil {
		logger.Error(err, "error getting iboxcg")
		cr.Status = csidriverinfinidatcomv1.IboxcgStatus{
			State: err.Error(),
		}
		if e := r.Status().Update(ctx, cr); e != nil {
			logger.Error(e, "unable to update iboxcg state")
		}
		return err
	}

	// update the cg CR status ID field with the new Promote ID
	cr.Status = csidriverinfinidatcomv1.IboxcgStatus{
		//ID:    response.ID,
		ID:    13,
		State: "created",
	}
	logger.Info("handling iboxcg", "cg successful - status ID", cr.Status.ID, "response ID", 13)
	if err := r.Status().Update(ctx, cr); err != nil {
		logger.Error(err, "unable to update iboxcg status")
		return err
	}
	logger.Info("iboxcg CR was updated", "cg Status ID updated", cr.Status.ID)

	return nil
}

func (r *IboxcgReconciler) updateIboxcgState(ctx context.Context, cg *csidriverinfinidatcomv1.Iboxcg) error {
	err := r.Get(ctx, types.NamespacedName{Name: cg.Name, Namespace: cg.Namespace}, cg)
	if err != nil {
		logger.Error(err, "error getting iboxcg")
		return err
	}

	// update the status of the Iboxcg with the status of the actual cg status on the ibox
	cg.Status.State = "completed"

	if err := r.Status().Update(ctx, cg); err != nil {
		logger.Error(err, "unable to update iboxcg state")
		return err
	}
	logger.Info("iboxcg CR status was updated", "State", cg.Status.State, "ID", cg.Status.ID)

	return nil
}

func getClientService(ctx context.Context, cg *csidriverinfinidatcomv1.Iboxcg) (*api.ClientService, error) {
	// get secret
	secretName := cg.Annotations[common.PVCAnnotationSecretName]
	secretNamespace := cg.Annotations[common.PVCAnnotationSecretNamespace]

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

func (r *IboxcgReconciler) handleAddMember(ctx context.Context, cr *csidriverinfinidatcomv1.Iboxcg, localVolume *iboxapi.Volume, localCG *iboxapi.ConsistencyGroupInfo, clientsvc *api.ClientService, cgMembers []iboxapi.MemberInfo) error {
	for _, m := range cgMembers {
		if m.Name == cr.Spec.LocalVolumeName {
			logger.Info(fmt.Sprintf("member %s already found in CG %s, will do nothing", cr.Spec.LocalVolumeName, cr.Spec.LocalCGName))
			return nil
		}
	}

	var replicaFound bool

	logger.Info("calling GetReplicaForCG", "cr", cr.Name, "local CG", localCG.Name)
	replica, err := clientsvc.IboxAPI.GetReplicaForCG(ctx, localCG.Name)
	if err == nil {
		logger.Info("GetReplicaForCG replica", "replica", replica.ID)
		replicaFound = true
	} else {
		if errors.Is(err, iboxapi.ErrNotFound) {
			replicaFound = false
		} else {
			logger.Error(err, "error getting replica", "cr", cr.Name, "cg", localCG.Name)
			return err
		}
	}
	if replicaFound {
		logger.Info("replica found for cg", "replica", replica.ID, "cr", cr.Name, "cg", localCG.Name, "cg members count", len(cgMembers))
		if len(cgMembers) > 0 {
			err := clientsvc.IboxAPI.SuspendReplica(ctx, replica.ID)
			if err != nil {
				logger.Error(err, "error suspending replica", "replica", replica.ID, "cr", cr.Name, "cg", localCG.Name)
				return err
			}
			for range 20 {
				time.Sleep(1 * time.Second)
				replica, err := clientsvc.IboxAPI.GetReplicaForCG(ctx, localCG.Name)
				if err != nil {
					logger.Error(err, "error getting replica in status check loop", "replica", replica.ID, "cr", cr.Name, "cg", localCG.Name)
				} else {
					logger.Info("replica state", "state", replica.State)
					if replica.State == "SUSPENDED" {
						logger.Info("replica state is now SUSPENDED", "state", replica.State)
						break
					}
				}
			}
		}
	}

	err = clientsvc.IboxAPI.AddMemberToCG(ctx, localVolume.ID, localCG.ID, localVolume.Name)
	if err != nil {
		logger.Error(err, "error adding member to cg", "cr", cr.Name, "volume ID", localVolume.ID, "cg", localCG.Name, "volume name", localVolume.Name)
		return err
	}

	if replicaFound {
		if len(cgMembers) > 0 {
			err = clientsvc.IboxAPI.ResumeReplica(ctx, replica.ID)
			if err != nil {
				logger.Error(err, "error resuming replica", "replica", replica.ID, "cr", cr.Name, "cg", localCG.Name)
			}
		}
	}

	if cr.Spec.RemoteCreatePVC == nil {
		logger.Info("remote_create_pvc was nil, skipping", "iboxcg name", cr.Name, "namespace", cr.Namespace, "volume name", cr.Spec.LocalVolumeName)
		return nil
	}
	if cr.Spec.RemoteCreatePVC != nil {
		logger.Info("remote_create_pvc was not nil", "iboxcg name", cr.Name, "namespace", cr.Namespace, "volume name", cr.Spec.LocalVolumeName, "remote_create_pvc", *cr.Spec.RemoteCreatePVC)
		if *cr.Spec.RemoteCreatePVC {
			// TODO replace this sleep (giving replication time to start) with a proper check
			time.Sleep(time.Second * 3)
			logger.Info("remote_create_pvc was true", "iboxcg name", cr.Name, "namespace", cr.Namespace, "volume name", cr.Spec.LocalVolumeName)
			err = r.createPVC(ctx, cr)
			if err != nil {
				logger.Error(err, "failed to create PVC")
			}
		}
	}
	return nil
}

func (r *IboxcgReconciler) handleRemoveMember(ctx context.Context, cr *csidriverinfinidatcomv1.Iboxcg, localVolume *iboxapi.Volume, localCG *iboxapi.ConsistencyGroupInfo, clientsvc *api.ClientService, cgMembers []iboxapi.MemberInfo) error {
	logger.Info("handleRemove CR called", "volume", cr.Spec.LocalVolumeName, "cg", cr.Spec.LocalCGName)
	memberFound := false
	for _, m := range cgMembers {
		if m.Name == cr.Spec.LocalVolumeName {
			memberFound = true
			logger.Info(fmt.Sprintf("member %s found in CG %s, will attempt removal", cr.Spec.LocalVolumeName, cr.Spec.LocalCGName))
		}
	}
	if !memberFound {
		logger.Info(fmt.Sprintf("member %s not found in CG %s, will do nothing", cr.Spec.LocalVolumeName, cr.Spec.LocalCGName))
		return nil
	}

	var replicaFound bool
	replica, err := clientsvc.IboxAPI.GetReplicaForCG(ctx, localCG.Name)
	if err == nil {
		replicaFound = true
	} else {
		if errors.Is(err, iboxapi.ErrNotFound) {
			replicaFound = false
		} else {
			logger.Error(err, "error getting replica", "cr", cr.Name, "cg", localCG.Name)
			return err
		}
	}

	if replicaFound {
		logger.Info("replica found for cg", "replica", replica.ID, "cr", cr.Name, "cg", localCG.Name, "cg member count", len(cgMembers))
		if len(cgMembers) > 0 {
			err := clientsvc.IboxAPI.SuspendReplica(ctx, replica.ID)
			if err != nil {
				logger.Error(err, "error suspending replica", "replica", replica.ID, "cr", cr.Name, "cg", localCG.Name)
				return err
			}
			for range 20 {
				time.Sleep(1 * time.Second)
				replica, err := clientsvc.IboxAPI.GetReplicaForCG(ctx, localCG.Name)
				if err != nil {
					logger.Error(err, "error getting replica in status check loop", "replica", replica.ID, "cr", cr.Name, "cg", localCG.Name)
				} else {
					logger.Info("replica state", "state", replica.State)
					if replica.State == "SUSPENDED" {
						logger.Info("replica state is now suspended", "state", replica.State)
						break
					}
				}
			}
		}
	}

	err = clientsvc.IboxAPI.RemoveMemberFromCG(ctx, localCG.ID, localVolume.ID)
	if err != nil {
		logger.Error(err, "error removing member from cg", "cr", cr.Name, "cg", localCG.Name, "volume", localVolume.Name)
	}

	if replicaFound {
		err = clientsvc.IboxAPI.ResumeReplica(ctx, replica.ID)
		if err != nil {
			logger.Error(err, "error resuming replica", "replica", replica.ID, "cr", cr.Name, "cg", localCG.Name)
		}
	}
	return nil
}
