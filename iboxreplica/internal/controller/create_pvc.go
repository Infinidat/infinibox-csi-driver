package controller

import (
	"context"
	"fmt"

	"github.com/infinidat/infinibox-csi-driver/api"
	"github.com/infinidat/infinibox-csi-driver/api/clientgo"
	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/infinidat/infinibox-csi-driver/iboxapi"
	csidriverinfinidatcomv1 "github.com/infinidat/infinibox-csi-driver/iboxreplica/api/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type EntityPair struct {
	LocalPV         v1.PersistentVolume
	RemoteEntityID  int
	RemoteIPAddress string
}

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

	remoteIboxCredential, err := getIboxCredentials(ctx, replica.Spec.RemoteIboxCredentialName, replica.Spec.RemoteIboxCredentialNamespace)
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

	remoteEntities, err := getRemoteEntities(ctx, clientsvc, remoteClientsvc, replica)
	if err != nil {
		logger.Error(err, "error getting remote entity IDs")
		return err
	}

	for _, remoteEntity := range remoteEntities {
		remotePV, err := createRemotePV(ctx, remoteClientsvc, &remoteEntity.LocalPV, replica, remoteEntity.RemoteEntityID, remoteEntity.RemoteIPAddress)
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
			if replica.Spec.RemotePVCNameSuffix != "" {
				pvcNameToUse = localPVCName + replica.Spec.RemotePVCNameSuffix
			}
		}

		// option 3) if there is only a single remote entity (e.g NOT a CG with multiple volumes)
		// see if the user wants to specify the remote PVC name
		if len(remoteEntities) == 1 && replica.Spec.RemotePVCName != "" {
			pvcNameToUse = replica.Spec.RemotePVCName
		}

		err = createRemotePVC(ctx, remotePV, replica, pvcNameToUse)
		if err != nil {
			logger.Error(err, "error creating remote PVC")
			return err
		}

		logger.Info("PVC was created", "PVC Name", pvcNameToUse)
	}

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

func createRemotePV(ctx context.Context, remoteClientsvc *api.ClientService, localPV *v1.PersistentVolume, replica *csidriverinfinidatcomv1.Iboxreplica, remoteVolumeID int, remoteIPAddress string) (*v1.PersistentVolume, error) {

	// make changes to the local PV, turning it into what will become the remote PV
	remotePV := &v1.PersistentVolume{}
	remotePV.Name = localPV.Name + "-remote"
	if replica.Spec.RemotePVCNameSuffix != "" {
		remotePV.Name = localPV.Name + replica.Spec.RemotePVCNameSuffix
	}

	remotePV.Spec = v1.PersistentVolumeSpec{
		AccessModes:                   localPV.Spec.AccessModes,
		Capacity:                      localPV.Spec.Capacity,
		MountOptions:                  localPV.Spec.MountOptions,
		PersistentVolumeReclaimPolicy: localPV.Spec.PersistentVolumeReclaimPolicy,
		StorageClassName:              localPV.Spec.StorageClassName,
		VolumeMode:                    localPV.Spec.VolumeMode,
	}

	poolToUse := replica.Spec.RemotePoolName
	if poolToUse == "" {
		poolToUse = localPV.Spec.CSI.VolumeAttributes[common.StorageClassPoolName]
	}

	// validate the remote pool name
	_, err := remoteClientsvc.IboxAPI.GetPoolByName(ctx, poolToUse)
	if err != nil {
		logger.Error(err, "error getting remote pool", "name", poolToUse)
		return nil, err
	}

	networkSpaceToUse := replica.Spec.RemoteNetworkSpace
	if networkSpaceToUse == "" {
		networkSpaceToUse = localPV.Spec.CSI.VolumeAttributes[common.StorageClassNetworkSpace]
	}

	volumeHandle := fmt.Sprintf("%d$$%s", remoteVolumeID, localPV.Spec.CSI.VolumeAttributes[common.StorageClassStorageProtocol])
	logger.Info("calculated remote PV volume handle", "value", volumeHandle, "remote network space", networkSpaceToUse, "remote pool", poolToUse)

	remotePV.Spec.CSI = &v1.CSIPersistentVolumeSource{
		Driver: localPV.Spec.CSI.Driver,
		ControllerExpandSecretRef: &v1.SecretReference{
			Name:      replica.Spec.RemoteIboxCredentialName,
			Namespace: replica.Spec.RemoteIboxCredentialNamespace,
		},
		ControllerPublishSecretRef: &v1.SecretReference{
			Name:      replica.Spec.RemoteIboxCredentialName,
			Namespace: replica.Spec.RemoteIboxCredentialNamespace,
		},
		NodeExpandSecretRef: &v1.SecretReference{
			Name:      replica.Spec.RemoteIboxCredentialName,
			Namespace: replica.Spec.RemoteIboxCredentialNamespace,
		},
		NodePublishSecretRef: &v1.SecretReference{
			Name:      replica.Spec.RemoteIboxCredentialName,
			Namespace: replica.Spec.RemoteIboxCredentialNamespace,
		},
		NodeStageSecretRef: &v1.SecretReference{
			Name:      replica.Spec.RemoteIboxCredentialName,
			Namespace: replica.Spec.RemoteIboxCredentialNamespace,
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

func createRemotePVC(ctx context.Context, remotePV *v1.PersistentVolume, replica *csidriverinfinidatcomv1.Iboxreplica, pvcNameToUse string) error {

	cl, err := clientgo.BuildClient()
	if err != nil {
		return err
	}

	capacity := remotePV.Spec.Capacity

	remotePVC := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcNameToUse,
			Namespace: replica.Spec.RemotePVCNamespace,
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

func getCGMembers(ctx context.Context, clientSvc *api.ClientService, cgName string) (members []iboxapi.MemberInfo, err error) {
	cg, err := clientSvc.IboxAPI.GetConsistencyGroupByName(ctx, cgName)
	if err != nil {
		logger.Error(err, "error getting CG")
		return members, err
	}

	members, err = clientSvc.IboxAPI.GetMembersByCGID(ctx, cg.ID)
	if err != nil {
		logger.Error(err, "error getting CG")
		return members, err
	}

	return members, nil
}

func getRemoteEntities(ctx context.Context, localClientsvc, remoteClientsvc *api.ClientService, replica *csidriverinfinidatcomv1.Iboxreplica) (remoteEntities []EntityPair, err error) {
	switch replica.Spec.EntityType {
	case common.ReplicaEntityCG:
		cgMembers, err := getCGMembers(ctx, localClientsvc, replica.Spec.LocalEntityName)
		if err != nil {
			logger.Error(err, "error getting local cg members", "cg name", replica.Spec.LocalEntityName)
			return remoteEntities, err
		}
		for _, m := range cgMembers {
			localPV, err := getLocalPV(ctx, m.Name)
			if err != nil {
				logger.Error(err, "error getting local PV for CG", "member ID", m.ID, "member name", m.Name)
				return remoteEntities, err
			}
			pair := EntityPair{
				RemoteEntityID: m.ID,
				LocalPV:        *localPV,
			}
			remoteEntities = append(remoteEntities, pair)
		}
		logger.Info("cg has members", "count", len(remoteEntities))
	case common.ReplicaEntityFilesystem:
		localPV, err := getLocalPV(ctx, replica.Spec.LocalEntityName)
		if err != nil {
			logger.Error(err, "error getting local PV")
			return remoteEntities, err
		}
		networkSpaceToUse := replica.Spec.RemoteNetworkSpace
		if networkSpaceToUse == "" {
			networkSpaceToUse = localPV.Spec.CSI.VolumeAttributes[common.StorageClassNetworkSpace]
		}
		netSpace, err := remoteClientsvc.IboxAPI.GetNetworkSpaceByName(ctx, networkSpaceToUse)
		if err != nil {
			logger.Error(err, "error getting remote network space", common.StorageClassNetworkSpace, networkSpaceToUse)
			return remoteEntities, err
		}
		if len(netSpace.Portals) == 0 {
			logger.Error(err, "error remote network space has zero portals")
			return remoteEntities, err
		}
		remoteIPAddress := netSpace.Portals[0].IPAddress
		fileSystem, err := remoteClientsvc.IboxAPI.GetFileSystemByName(ctx, replica.Spec.RemoteEntityName)
		if err != nil {
			logger.Error(err, "error getting remote filesystem")
			return remoteEntities, err
		}
		pair := EntityPair{
			LocalPV:         *localPV,
			RemoteEntityID:  fileSystem.ID,
			RemoteIPAddress: remoteIPAddress,
		}
		remoteEntities = append(remoteEntities, pair)
	case common.ReplicaEntityVolume:
		localPV, err := getLocalPV(ctx, replica.Spec.LocalEntityName)
		if err != nil {
			logger.Error(err, "error getting local PV")
			return remoteEntities, err
		}
		// lookup remote volume so we can get the volume ID for it
		remoteVolume, err := remoteClientsvc.IboxAPI.GetVolumeByName(ctx, replica.Spec.RemoteEntityName)
		if err != nil {
			logger.Error(err, "error getting remote volume")
			return remoteEntities, err
		}
		pair := EntityPair{
			LocalPV:        *localPV,
			RemoteEntityID: remoteVolume.ID,
		}
		remoteEntities = append(remoteEntities, pair)
	default:
		err = fmt.Errorf("error unknown entity type %s", replica.Spec.EntityType)
		logger.Error(err, "error getting remote entity")
		return remoteEntities, err
	}

	return remoteEntities, nil
}
