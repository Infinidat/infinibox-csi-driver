package controller

import (
	"context"

	"github.com/infinidat/infinibox-csi-driver/api"
	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/infinidat/infinibox-csi-driver/createpvc"
	csidriverinfinidatcomv1 "github.com/infinidat/infinibox-csi-driver/iboxcg/api/v1"
	v1 "k8s.io/api/core/v1"
)

type EntityPair struct {
	LocalPV         v1.PersistentVolume
	RemoteEntityID  int
	RemoteIPAddress string
}

func (r *IboxcgReconciler) createPVC(ctx context.Context, iboxcg *csidriverinfinidatcomv1.Iboxcg) error {
	clientsvc, err := getClientService(ctx, iboxcg)
	if err != nil {
		logger.Error(err, "error getting clientService")
		iboxcg.Status = csidriverinfinidatcomv1.IboxcgStatus{
			State: err.Error(),
		}
		if e := r.Status().Update(ctx, iboxcg); e != nil {
			logger.Error(e, "unable to update iboxcg state")
		}
		return err
	}

	remoteIboxCredential, err := createpvc.GetIboxCredentials(ctx, logger, iboxcg.Spec.RemoteIboxCredentialName, iboxcg.Spec.RemoteIboxCredentialNamespace)
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

	logger.Info("creating PVC for iboxcg", "iboxcg.Name", iboxcg.Name)
	var localEntityID int

	localEntityID, err = getLocalEntityID(ctx, clientsvc, r, iboxcg)
	if err != nil {
		return err
	}

	logger.Info("creating PVC for iboxcg", "entity look up worked", localEntityID)

	remoteEntity, err := getRemoteEntities(ctx, remoteClientsvc, iboxcg)
	if err != nil {
		logger.Error(err, "error getting remote entity IDs")
		return err
	}

	remotePV, err := createpvc.CreateRemotePV(ctx, logger, remoteClientsvc, &remoteEntity.LocalPV,
		remoteEntity.RemoteEntityID, remoteEntity.RemoteIPAddress, iboxcg.Spec.RemotePoolName, iboxcg.Spec.RemotePVCNameSuffix,
		iboxcg.Spec.RemoteNetworkSpace, iboxcg.Spec.RemoteIboxCredentialName, iboxcg.Spec.RemoteIboxCredentialNamespace,
		iboxcg.Spec.RemotePVCKubeconfigSecretName, iboxcg.Spec.RemotePVCKubeconfigSecretNamespace)
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
		if iboxcg.Spec.RemotePVCNameSuffix != "" {
			pvcNameToUse = localPVCName + iboxcg.Spec.RemotePVCNameSuffix
		}
	}

	// option 3) if there is only a single remote entity (e.g NOT a CG with multiple volumes)
	// see if the user wants to specify the remote PVC name
	if iboxcg.Spec.RemotePVCName != "" {
		pvcNameToUse = iboxcg.Spec.RemotePVCName
	}

	err = createpvc.CreateRemotePVC(ctx, logger, remotePV, iboxcg.Spec.RemotePVCNamespace, pvcNameToUse,
		iboxcg.Spec.RemotePVCKubeconfigSecretName, iboxcg.Spec.RemotePVCKubeconfigSecretNamespace)
	if err != nil {
		logger.Error(err, "error creating remote PVC")
		return err
	}

	logger.Info("PVC was created", "PVC Name", pvcNameToUse)

	return nil
}

func getRemoteEntities(ctx context.Context, remoteClientsvc *api.ClientService, iboxcg *csidriverinfinidatcomv1.Iboxcg) (*EntityPair, error) {
	localPV, err := createpvc.GetLocalPV(ctx, iboxcg.Spec.LocalVolumeName)
	if err != nil {
		logger.Error(err, "error getting local PV")
		return nil, err
	}
	// lookup remote volume so we can get the volume ID for it
	remoteVolume, err := remoteClientsvc.IboxAPI.GetVolumeByName(ctx, iboxcg.Spec.LocalVolumeName)
	if err != nil {
		logger.Error(err, "error getting remote volume")
		return nil, err
	}
	pair := &EntityPair{
		LocalPV:        *localPV,
		RemoteEntityID: remoteVolume.ID,
	}

	return pair, nil
}

func getLocalEntityID(ctx context.Context, clientsvc *api.ClientService, r *IboxcgReconciler, iboxcg *csidriverinfinidatcomv1.Iboxcg) (localEntityID int, err error) {
	// look up the volume ID
	volume, err := clientsvc.IboxAPI.GetVolumeByName(ctx, iboxcg.Spec.LocalVolumeName)
	if err != nil {
		logger.Error(err, "error getting Volume", "localVolumeName", iboxcg.Spec.LocalVolumeName)
		iboxcg.Status = csidriverinfinidatcomv1.IboxcgStatus{
			State: err.Error(),
		}
		if e := r.Status().Update(ctx, iboxcg); e != nil {
			logger.Error(e, "unable to update iboxcg state")
		}
		return 0, err
	}
	return volume.ID, nil
}
