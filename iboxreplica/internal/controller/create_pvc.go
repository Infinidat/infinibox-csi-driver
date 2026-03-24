package controller

import (
	"context"

	"github.com/infinidat/infinibox-csi-driver/api"
	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/infinidat/infinibox-csi-driver/createpvc"
	csidriverinfinidatcomv1 "github.com/infinidat/infinibox-csi-driver/iboxreplica/api/v1"
)

func (r *IboxreplicaReconciler) createPVC(ctx context.Context, replica *csidriverinfinidatcomv1.Iboxreplica) error {
	clientsvc, err := getClientService(ctx, replica)
	if err != nil {
		logger.Error(err, "error getting clientService")
		replica.Status = csidriverinfinidatcomv1.IboxreplicaStatus{
			State: err.Error(),
		}
		if e := r.Status().Update(ctx, replica); e != nil {
			logger.Error(e, "unable to update iboxreplica state")
		}
		return err
	}

	remoteIboxCredential, err := createpvc.GetIboxCredentials(ctx, logger, replica.Spec.RemoteIboxCredentialName, replica.Spec.RemoteIboxCredentialNamespace)
	if err != nil {
		return err
	}
	logger.Info("found remote ibox credential", "hostname", remoteIboxCredential[common.CredentialHostname])

	x := api.ClientService{
		ConfigMap:  make(map[string]string),
		SecretsMap: remoteIboxCredential,
	}

	remoteClientsvc, err := x.NewClient()
	if err != nil {
		logger.Error(err, "error getting remote ClientService")
		return err
	}

	logger.Info("creating PVC for replica", "replica.Name", replica.Name)
	var localEntityID int

	localEntityID, err = getLocalEntityID(ctx, clientsvc, r, replica)
	if err != nil {
		return err
	}

	logger.Info("creating PVC for replica", "entity look up worked", localEntityID)

	remoteEntities, err := createpvc.GetRemoteEntities(ctx, logger, clientsvc, remoteClientsvc, replica.Spec.EntityType,
		replica.Spec.LocalEntityName, replica.Spec.RemoteNetworkSpace, replica.Spec.RemoteEntityName)
	if err != nil {
		logger.Error(err, "error getting remote entity IDs")
		return err
	}

	for _, remoteEntity := range remoteEntities {
		remotePV, err := createpvc.CreateRemotePV(ctx, logger, remoteClientsvc, &remoteEntity.LocalPV,
			remoteEntity.RemoteEntityID, remoteEntity.RemoteIPAddress, replica.Spec.RemotePoolName, replica.Spec.RemotePVCNameSuffix, replica.Spec.RemoteNetworkSpace, replica.Spec.RemoteIboxCredentialName, replica.Spec.RemoteIboxCredentialNamespace,
			replica.Spec.RemotePVCKubeconfigSecretName, replica.Spec.RemotePVCKubeconfigSecretNamespace)
		if err != nil {
			logger.Error(err, "error creating remote PV")
			return err
		}

		// option 1) default is to use the remote PV name for the PVC name
		pvcNameToUse := remotePV.Name

		// option 2) use the local PVC name if one is found bound to this PV
		localPVCName := remoteEntity.LocalPV.Spec.CSI.VolumeAttributes["csi.storage.k8s.io/pvc/name"]
		if localPVCName != "" {
			pvcNameToUse = localPVCName + "-remote"
			// we can use the same PVC name if this PVC is going to be created on an alternate k8s cluster
			if replica.Spec.RemotePVCKubeconfigSecretName != "" {
				pvcNameToUse = localPVCName
			}
			if replica.Spec.RemotePVCNameSuffix != "" {
				pvcNameToUse = localPVCName + replica.Spec.RemotePVCNameSuffix
			}
		}

		// option 3) if there is only a single remote entity (e.g NOT a CG with multiple volumes)
		// see if the user wants to specify the remote PVC name
		if len(remoteEntities) == 1 && replica.Spec.RemotePVCName != "" {
			pvcNameToUse = replica.Spec.RemotePVCName
		}

		err = createpvc.CreateRemotePVC(ctx, logger, remotePV, replica.Spec.RemotePVCNamespace, pvcNameToUse,
			replica.Spec.RemotePVCKubeconfigSecretName, replica.Spec.RemotePVCKubeconfigSecretNamespace)
		if err != nil {
			logger.Error(err, "error creating remote PVC")
			return err
		}

		logger.Info("PVC was created", "PVC Name", pvcNameToUse)
	}

	return nil
}
