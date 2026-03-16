package controller

import (
	"context"
	"fmt"

	"github.com/infinidat/infinibox-csi-driver/api"
	"github.com/infinidat/infinibox-csi-driver/api/clientgo"
	"github.com/infinidat/infinibox-csi-driver/common"
	csidriverinfinidatcomv1 "github.com/infinidat/infinibox-csi-driver/iboxcg/api/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	remoteIboxCredential, err := getIboxCredentials(ctx, iboxcg.Spec.RemoteIboxCredentialName, iboxcg.Spec.RemoteIboxCredentialNamespace)
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

	remotePV, err := createRemotePV(ctx, remoteClientsvc, &remoteEntity.LocalPV, iboxcg, remoteEntity.RemoteEntityID, remoteEntity.RemoteIPAddress)
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

	err = createRemotePVC(ctx, remotePV, iboxcg, pvcNameToUse)
	if err != nil {
		logger.Error(err, "error creating remote PVC")
		return err
	}

	logger.Info("PVC was created", "PVC Name", pvcNameToUse)

	return nil
}

func getIboxCredentials(ctx context.Context, secretName, secretNamespace string) (secret map[string]string, err error) {
	// Get a k8s go client for in-cluster use
	cl, err := clientgo.BuildClient()
	if err != nil {
		return secret, err
	}

	secret, err = cl.GetSecret(ctx, secretName, secretNamespace)
	if err != nil {
		logger.Error(err, "error getting secret", "secret_name", secretName, "secret_namespace", secretNamespace)
		return secret, err
	}

	logger.Info("remote ibox credential was found", "credential name", secretName, "namespace", secretNamespace)
	if secret[common.CredentialUsername] == "" {
		logger.Error(err, "error in secret - missing username", "secret_name", secretName, "secret_namespace", secretNamespace)
		return secret, err
	}
	if secret[common.CredentialPassword] == "" {
		logger.Error(err, "error in secret - missing password", "secret_name", secretName, "secret_namespace", secretNamespace)
		return secret, err
	}
	if secret[common.CredentialHostname] == "" {
		logger.Error(err, "error in secret - missing hostname", "secret_name", secretName, "secret_namespace", secretNamespace)
		return secret, err
	}
	return secret, nil
}

func getLocalPV(ctx context.Context, pvName string) (pv *v1.PersistentVolume, err error) {

	cl, err := clientgo.BuildClient()
	if err != nil {
		return nil, err
	}

	pv, err = cl.GetPersistantVolumeByName(ctx, pvName)
	if err != nil {
		return nil, err
	}

	return pv, nil
}

func createRemotePV(ctx context.Context, remoteClientsvc *api.ClientService, localPV *v1.PersistentVolume, iboxcg *csidriverinfinidatcomv1.Iboxcg, remoteVolumeID int, remoteIPAddress string) (*v1.PersistentVolume, error) {

	// make changes to the local PV, turning it into what will become the remote PV
	remotePV := &v1.PersistentVolume{}
	remotePV.Name = localPV.Name + "-remote"
	if iboxcg.Spec.RemotePVCNameSuffix != "" {
		remotePV.Name = localPV.Name + iboxcg.Spec.RemotePVCNameSuffix
	}

	remotePV.Spec = v1.PersistentVolumeSpec{
		AccessModes:                   localPV.Spec.AccessModes,
		Capacity:                      localPV.Spec.Capacity,
		MountOptions:                  localPV.Spec.MountOptions,
		PersistentVolumeReclaimPolicy: localPV.Spec.PersistentVolumeReclaimPolicy,
		StorageClassName:              localPV.Spec.StorageClassName,
		VolumeMode:                    localPV.Spec.VolumeMode,
	}

	poolToUse := iboxcg.Spec.RemotePoolName
	if poolToUse == "" {
		poolToUse = localPV.Spec.CSI.VolumeAttributes[common.StorageClassPoolName]
	}

	// validate the remote pool name
	_, err := remoteClientsvc.IboxAPI.GetPoolByName(ctx, poolToUse)
	if err != nil {
		logger.Error(err, "error getting remote pool", "name", poolToUse)
		return nil, err
	}

	networkSpaceToUse := iboxcg.Spec.RemoteNetworkSpace
	if networkSpaceToUse == "" {
		networkSpaceToUse = localPV.Spec.CSI.VolumeAttributes[common.StorageClassNetworkSpace]
	}

	volumeHandle := fmt.Sprintf("%d$$%s", remoteVolumeID, localPV.Spec.CSI.VolumeAttributes[common.StorageClassStorageProtocol])
	logger.Info("calculated remote PV volume handle", "value", volumeHandle, "remote network space", networkSpaceToUse, "remote pool", poolToUse)

	remotePV.Spec.CSI = &v1.CSIPersistentVolumeSource{
		Driver: localPV.Spec.CSI.Driver,
		ControllerExpandSecretRef: &v1.SecretReference{
			Name:      iboxcg.Spec.RemoteIboxCredentialName,
			Namespace: iboxcg.Spec.RemoteIboxCredentialNamespace,
		},
		ControllerPublishSecretRef: &v1.SecretReference{
			Name:      iboxcg.Spec.RemoteIboxCredentialName,
			Namespace: iboxcg.Spec.RemoteIboxCredentialNamespace,
		},
		NodeExpandSecretRef: &v1.SecretReference{
			Name:      iboxcg.Spec.RemoteIboxCredentialName,
			Namespace: iboxcg.Spec.RemoteIboxCredentialNamespace,
		},
		NodePublishSecretRef: &v1.SecretReference{
			Name:      iboxcg.Spec.RemoteIboxCredentialName,
			Namespace: iboxcg.Spec.RemoteIboxCredentialNamespace,
		},
		NodeStageSecretRef: &v1.SecretReference{
			Name:      iboxcg.Spec.RemoteIboxCredentialName,
			Namespace: iboxcg.Spec.RemoteIboxCredentialNamespace,
		},
		VolumeAttributes: map[string]string{
			common.StorageClassUNIXPermissions:             localPV.Spec.CSI.VolumeAttributes[common.StorageClassUNIXPermissions],
			common.StorageClassGID:                         localPV.Spec.CSI.VolumeAttributes[common.StorageClassGID],
			common.StorageClassUID:                         localPV.Spec.CSI.VolumeAttributes[common.StorageClassUID],
			common.StorageClassStorageProtocol:             localPV.Spec.CSI.VolumeAttributes[common.StorageClassStorageProtocol],
			common.PVCAnnotationPoolName:                   localPV.Spec.CSI.VolumeAttributes[common.PVCAnnotationPoolName],
			common.PVCAnnotationNetworkSpace:               localPV.Spec.CSI.VolumeAttributes[common.PVCAnnotationNetworkSpace],
			common.PVCAnnotationIBOXSecret:                 localPV.Spec.CSI.VolumeAttributes[common.PVCAnnotationIBOXSecret],
			common.PVCAnnotationVolumeMetadata:             localPV.Spec.CSI.VolumeAttributes[common.PVCAnnotationVolumeMetadata],
			common.StorageClassNetworkSpace:                networkSpaceToUse,
			common.StorageClassNFSExportPermissions:        localPV.Spec.CSI.VolumeAttributes[common.StorageClassNFSExportPermissions],
			common.StorageClassPoolName:                    poolToUse,
			common.StorageClassSnapDirVisible:              localPV.Spec.CSI.VolumeAttributes[common.StorageClassSnapDirVisible],
			"storage.kubernetes.io/csiProvisionerIdentity": localPV.Spec.CSI.VolumeAttributes["storage.kubernetes.io/csiProvisionerIdentity"],
			"volPathd": localPV.Spec.CSI.VolumeAttributes["volPathd"],
		},
		VolumeHandle: volumeHandle,
	}

	if remoteIPAddress != "" {
		remotePV.Spec.CSI.VolumeAttributes["ipAddress"] = remoteIPAddress
	}

	cl, err := clientgo.BuildClient()
	if err != nil {
		return nil, err
	}

	createdPV, err := cl.CreatePersistantVolume(ctx, remotePV)
	if err != nil {
		return nil, err
	}
	logger.Info("PV was created", "PV Name", createdPV.Name)

	return createdPV, nil
}

func createRemotePVC(ctx context.Context, remotePV *v1.PersistentVolume, iboxcg *csidriverinfinidatcomv1.Iboxcg, pvcNameToUse string) error {

	cl, err := clientgo.BuildClient()
	if err != nil {
		return err
	}

	capacity := remotePV.Spec.Capacity

	remotePVC := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcNameToUse,
			Namespace: iboxcg.Spec.RemotePVCNamespace,
		},
		Spec: v1.PersistentVolumeClaimSpec{
			AccessModes: remotePV.Spec.AccessModes,
			Resources: v1.VolumeResourceRequirements{
				Requests: v1.ResourceList{
					"storage": *capacity.Storage(),
				},
			},
			StorageClassName: &remotePV.Spec.StorageClassName,
			VolumeMode:       remotePV.Spec.VolumeMode,
			VolumeName:       remotePV.Name,
		},
	}

	createdPVC, err := cl.CreatePersistantVolumeClaim(ctx, remotePVC)
	if err != nil {
		return err
	}
	logger.Info("PVC was created", "PVC Name", createdPVC.Name)

	return nil
}

func getRemoteEntities(ctx context.Context, remoteClientsvc *api.ClientService, iboxcg *csidriverinfinidatcomv1.Iboxcg) (*EntityPair, error) {
	localPV, err := getLocalPV(ctx, iboxcg.Spec.LocalVolumeName)
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
