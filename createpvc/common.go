/*
Copyright 2026 Infinidat
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

package createpvc

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/infinidat/infinibox-csi-driver/api"
	"github.com/infinidat/infinibox-csi-driver/api/clientgo"
	"github.com/infinidat/infinibox-csi-driver/common"
	"github.com/infinidat/infinibox-csi-driver/iboxapi"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type EntityPair struct {
	LocalPV         v1.PersistentVolume
	RemoteEntityID  int
	RemoteIPAddress string
}

func GetIboxCredentials(ctx context.Context, logger logr.Logger, secretName, secretNamespace string) (secret map[string]string, err error) {
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

func GetLocalPV(ctx context.Context, pvName string) (pv *v1.PersistentVolume, err error) {

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

func CreateRemotePVC(ctx context.Context, logger logr.Logger, remotePV *v1.PersistentVolume, namespace, pvcNameToUse, alternateKubeconfigSecretName, alternateKubeconfigSecretNamespace string) error {

	capacity := remotePV.Spec.Capacity

	remotePVC := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcNameToUse,
			Namespace: namespace,
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

	cl, err := clientgo.BuildClient()
	if err != nil {
		return err
	}
	if alternateKubeconfigSecretName != "" {
		logger.Info("using alternate kubeconfig for creating PVC", "secret name", alternateKubeconfigSecretName, "secret namespace", alternateKubeconfigSecretNamespace)
		cl, err = clientgo.BuildClientFromSecret(alternateKubeconfigSecretName, alternateKubeconfigSecretNamespace)
		if err != nil {
			return err
		}
	}

	createdPVC, err := cl.CreatePersistantVolumeClaim(ctx, remotePVC)
	if err != nil {
		return err
	}
	logger.Info("PVC was created", "PVC Name", createdPVC.Name)

	return nil
}

func GetCGMembers(ctx context.Context, logger logr.Logger, clientSvc *api.ClientService, cgName string) (members []iboxapi.MemberInfo, err error) {
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

func CreateRemotePV(ctx context.Context, logger logr.Logger, remoteClientsvc *api.ClientService, localPV *v1.PersistentVolume,
	remoteVolumeID int, remoteIPAddress, remotePoolName, remotePVCNameSuffix, remoteNetworkSpace, remoteIboxCredentialName, remoteIboxCredentialNamespace,
	alternateKubeconfigSecretName, alternateKubeconfigSecretNamespace string) (*v1.PersistentVolume, error) {

	// make changes to the local PV, turning it into what will become the target PV
	remotePV := &v1.PersistentVolume{}
	remotePV.Name = localPV.Name + "-remote"
	if alternateKubeconfigSecretName != "" {
		// we can reuse the same name if the PV is destined for an alternate k8s cluster
		remotePV.Name = localPV.Name
	}
	if remotePVCNameSuffix != "" {
		remotePV.Name = localPV.Name + remotePVCNameSuffix
	}

	remotePV.Spec = v1.PersistentVolumeSpec{
		AccessModes:                   localPV.Spec.AccessModes,
		Capacity:                      localPV.Spec.Capacity,
		MountOptions:                  localPV.Spec.MountOptions,
		PersistentVolumeReclaimPolicy: localPV.Spec.PersistentVolumeReclaimPolicy,
		StorageClassName:              localPV.Spec.StorageClassName,
		VolumeMode:                    localPV.Spec.VolumeMode,
	}

	poolToUse := remotePoolName
	if poolToUse == "" {
		poolToUse = localPV.Spec.CSI.VolumeAttributes[common.StorageClassPoolName]
	}

	// validate the remote pool name
	_, err := remoteClientsvc.IboxAPI.GetPoolByName(ctx, poolToUse)
	if err != nil {
		logger.Error(err, "error getting remote pool", "name", poolToUse)
		return nil, err
	}

	networkSpaceToUse := localPV.Spec.CSI.VolumeAttributes[common.StorageClassNetworkSpace]
	if remoteNetworkSpace != "" {
		networkSpaceToUse = remoteNetworkSpace
	}

	volumeHandle := fmt.Sprintf("%d$$%s", remoteVolumeID, localPV.Spec.CSI.VolumeAttributes[common.StorageClassStorageProtocol])
	logger.Info("calculated remote PV volume handle", "value", volumeHandle, "remote network space", networkSpaceToUse, "remote pool", poolToUse)

	remotePV.Spec.CSI = &v1.CSIPersistentVolumeSource{
		Driver: localPV.Spec.CSI.Driver,
		ControllerExpandSecretRef: &v1.SecretReference{
			Name:      remoteIboxCredentialName,
			Namespace: remoteIboxCredentialNamespace,
		},
		ControllerPublishSecretRef: &v1.SecretReference{
			Name:      remoteIboxCredentialName,
			Namespace: remoteIboxCredentialNamespace,
		},
		NodeExpandSecretRef: &v1.SecretReference{
			Name:      remoteIboxCredentialName,
			Namespace: remoteIboxCredentialNamespace,
		},
		NodePublishSecretRef: &v1.SecretReference{
			Name:      remoteIboxCredentialName,
			Namespace: remoteIboxCredentialNamespace,
		},
		NodeStageSecretRef: &v1.SecretReference{
			Name:      remoteIboxCredentialName,
			Namespace: remoteIboxCredentialNamespace,
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

	if alternateKubeconfigSecretName != "" {
		// the alternate k8s secret has to exist
		_, err := cl.KubeClientInterface.CoreV1().Secrets(alternateKubeconfigSecretNamespace).Get(ctx, alternateKubeconfigSecretName, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		logger.Info("got alternate k8s secret on local k8s", "name", alternateKubeconfigSecretName, "namespace", alternateKubeconfigSecretNamespace)

		localTargetIboxSecret, err := cl.KubeClientInterface.CoreV1().Secrets(remoteIboxCredentialNamespace).Get(ctx, remoteIboxCredentialName, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		logger.Info("got target ibox secret on local k8s", "name", remoteIboxCredentialName, "namespace", remoteIboxCredentialNamespace)

		logger.Info("using alternate kubeconfig for creating PV", "secret name", alternateKubeconfigSecretName, "secret namespace", alternateKubeconfigSecretNamespace)
		cl, err = clientgo.BuildClientFromSecret(alternateKubeconfigSecretName, alternateKubeconfigSecretNamespace)
		if err != nil {
			return nil, err
		}
		// see if the target ibox secret exists on the alternate k8s cluster, if not, create it
		_, err = cl.GetSecret(ctx, remoteIboxCredentialName, remoteIboxCredentialNamespace)
		if err != nil {
			// assume a not found so lets create the secret on the alternate k8s cluster
			logger.Info("creating target ibox secret on alternate k8s", "name", remoteIboxCredentialName, "namespace", remoteIboxCredentialNamespace)
			localTargetIboxSecret.ResourceVersion = ""
			_, err := cl.CreateSecret(ctx, localTargetIboxSecret)
			if err != nil {
				return nil, err
			}
		}
		logger.Info("target ibox secret exists on alternate k8s", "name", remoteIboxCredentialName, "namespace", remoteIboxCredentialNamespace)

	}

	createdPV, err := cl.CreatePersistantVolume(ctx, remotePV)
	if err != nil {
		return nil, err
	}
	logger.Info("PV was created", "PV Name", createdPV.Name)

	return createdPV, nil
}

func GetRemoteEntities(ctx context.Context, logger logr.Logger, localClientsvc, remoteClientsvc *api.ClientService, entityType, localEntityName, remoteNetworkSpace, remoteEntityName string) (remoteEntities []EntityPair, err error) {
	switch entityType {
	case common.ReplicaEntityCG:
		cgMembers, err := GetCGMembers(ctx, logger, localClientsvc, localEntityName)
		if err != nil {
			logger.Error(err, "error getting local cg members", "cg name", localEntityName)
			return remoteEntities, err
		}
		for _, m := range cgMembers {
			localPV, err := GetLocalPV(ctx, m.Name)
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
		localPV, err := GetLocalPV(ctx, localEntityName)
		if err != nil {
			logger.Error(err, "error getting local PV")
			return remoteEntities, err
		}
		networkSpaceToUse := remoteNetworkSpace
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
		fileSystem, err := remoteClientsvc.IboxAPI.GetFileSystemByName(ctx, remoteEntityName)
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
		localPV, err := GetLocalPV(ctx, localEntityName)
		if err != nil {
			logger.Error(err, "error getting local PV")
			return remoteEntities, err
		}
		// lookup remote volume so we can get the volume ID for it
		remoteVolume, err := remoteClientsvc.IboxAPI.GetVolumeByName(ctx, remoteEntityName)
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
		err = fmt.Errorf("error unknown entity type %s", entityType)
		logger.Error(err, "error getting remote entity")
		return remoteEntities, err
	}

	return remoteEntities, nil
}
