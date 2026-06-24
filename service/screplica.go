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

package service

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/infinidat/infinibox-csi-driver/api/clientgo"
	"github.com/infinidat/infinibox-csi-driver/common"
	v1cg "github.com/infinidat/infinibox-csi-driver/iboxcg/api/v1"
	v1 "github.com/infinidat/infinibox-csi-driver/iboxreplica/api/v1"
	storagecommon "github.com/infinidat/infinibox-csi-driver/storage/common"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type RemoteFields struct {
	RemoteIboxCredentialName           string
	RemoteIboxCredentialNamespace      string
	RemoteCreatePVC                    *bool  `json:"remote_create_pvc,omitempty"`
	RemotePVCNameSuffix                string `json:"remote_pvc_name_suffix,omitempty"`
	RemotePVCName                      string `json:"remote_pvc_name,omitempty"`
	RemotePVCKubeconfigSecretName      string `json:"remote_pvc_kubeconfig_secret_name,omitempty"`
	RemotePVCKubeconfigSecretNamespace string `json:"remote_pvc_kubeconfig_secret_namespace,omitempty"`
	RemotePVCNamespace                 string `json:"remote_pvc_namespace,omitempty"`
	RemoteNetworkSpace                 string `json:"remote_network_space,omitempty"`
	RemotePoolName                     string `json:"remote_pool_name,omitempty"`
}

func handleSCReplica(ctx context.Context, cs storagecommon.Commonservice, volName string, storageProtocol string, params map[string]string) (err error) {
	// 1 - verify user wants to create a replica, they have to specify a replica type
	switch params[common.IboxReplicaTypeParameter] {
	case "":
		slog.Debug("sc replica was not specified, moving on")
		return nil
	case common.IboxreplicaReplicaTypeSYNC, common.IboxreplicaReplicaTypeASYNC, common.IboxreplicaReplicaTypeACTIVE_ACTIVE:
		slog.Debug("sc replica type was specified")
	default:
		return fmt.Errorf("invalid replication type specified in StorageClass")
	}

	// validate other ibox replica required parameters
	var poolID int
	if params[common.IboxReplicaRemotePoolIDParameter] != "" {
		poolID, err = strconv.Atoi(params[common.IboxReplicaRemotePoolIDParameter])
		if err != nil {
			return fmt.Errorf("sc replica parameter [%s] is set but is not an integer, error %s", common.IboxReplicaRemotePoolIDParameter, err.Error())
		}
	}

	if params[common.IboxReplicaRemoteIboxLinkNameParameter] == "" {
		return fmt.Errorf("sc replica parameter [%s] is not set but is required", common.IboxReplicaRemoteIboxLinkNameParameter)
	}

	err = verifyLinkExists(ctx, cs, params[common.IboxReplicaRemoteIboxLinkNameParameter])
	if err != nil {
		return err
	}

	// determine the entity type which is either VOLUME, FILESYSTEM, or CONSISTENCY_GROUP
	entityType := common.ReplicaEntityVolume // default to a VOLUME as being the replica entity type
	entityName := volName                    // either the VOLUME/FILESYSTEM name, or CG name

	cgName := params[common.IboxReplicaCGNameParameter]
	if cgName != "" {
		entityType = common.ReplicaEntityCG
		entityName = cgName
	} else if storageProtocol == common.ProtocolNFS {
		entityType = common.ReplicaEntityFilesystem
	}
	slog.Debug("sc replica entity type", "value", entityType)

	secretName := params[common.IboxReplicaLocalIboxCredNameParameter]
	if secretName == "" {
		return fmt.Errorf("storageclass parameter [%s] is not set but is required", common.IboxReplicaLocalIboxCredNameParameter)
	}

	secretNamespace := params[common.IboxReplicaLocalIboxCredNamespaceParameter]
	if secretNamespace == "" {
		return fmt.Errorf("sc replica parameter [%s] is not set but is required", common.IboxReplicaLocalIboxCredNamespaceParameter)
	}

	// verify the local secret exists
	kubernetesClient, err := clientgo.BuildClient()
	if err != nil {
		return err
	}
	_, err = kubernetesClient.GetSecret(ctx, secretName, secretNamespace)
	if err != nil {
		return err
	}

	tmp := params[common.IboxReplicaCreatePVC] // boolean
	var createPVC bool
	if tmp == "" {
		slog.Debug("create pvc not specified, skipping")
	} else {
		createPVC, err = strconv.ParseBool(params[common.IboxReplicaCreatePVC]) // boolean
		if err != nil {
			slog.Error(err.Error())
			return err
		}
	}

	var createPVCFields *RemoteFields
	if createPVC {
		createPVCFields, err = handleCreatePVC(params)
		if err != nil {
			return err
		}
		if createPVCFields.RemotePVCKubeconfigSecretName != "" {
			_, err = kubernetesClient.GetSecret(ctx, createPVCFields.RemotePVCKubeconfigSecretName, createPVCFields.RemotePVCKubeconfigSecretNamespace)
			if err != nil {
				slog.Error(err.Error())
				return err
			}
			slog.Debug("remote kube secret exists", "name", createPVCFields.RemotePVCKubeconfigSecretName, "namespace", createPVCFields.RemoteIboxCredentialNamespace)
		}
	}

	if cgName == "" {
		// 3 - create iboxreplica
		replicaName := "iboxreplica-" + volName

		// create the iboxreplica CR
		replica := v1.Iboxreplica{
			ObjectMeta: metav1.ObjectMeta{
				Name: replicaName,
				Annotations: map[string]string{
					common.PVCAnnotationSecretName:      secretName,
					common.PVCAnnotationSecretNamespace: secretNamespace,
				},
			},
			Spec: v1.IboxreplicaSpec{
				BaseAction:                    "NEW",
				Description:                   replicaName,
				EntityType:                    entityType,
				LocalEntityName:               entityName,
				RemoteEntityName:              entityName,
				LinkRemoteSystemName:          params[common.IboxReplicaRemoteIboxLinkNameParameter],
				ReplicationType:               params[common.IboxReplicaTypeParameter],
				RemotePoolID:                  poolID,
				RemotePoolName:                params[common.IboxReplicaCreatePVCPoolName],
				RemoteIboxCredentialName:      params[common.IboxReplicaRemoteIboxCredNameParameter],
				RemoteIboxCredentialNamespace: params[common.IboxReplicaRemoteIboxCredNamespaceParameter],
			},
		}

		if createPVC {
			replica.Spec.RemoteIboxCredentialName = createPVCFields.RemoteIboxCredentialName
			replica.Spec.RemoteIboxCredentialNamespace = createPVCFields.RemoteIboxCredentialNamespace
			replica.Spec.RemoteCreatePVC = createPVCFields.RemoteCreatePVC
			replica.Spec.RemotePVCNameSuffix = createPVCFields.RemotePVCNameSuffix
			replica.Spec.RemotePVCName = createPVCFields.RemotePVCName
			replica.Spec.RemotePVCNamespace = createPVCFields.RemotePVCNamespace
			replica.Spec.RemoteNetworkSpace = createPVCFields.RemoteNetworkSpace
			replica.Spec.RemotePoolName = createPVCFields.RemotePoolName
			replica.Spec.RemotePVCKubeconfigSecretName = createPVCFields.RemotePVCKubeconfigSecretName
			replica.Spec.RemotePVCKubeconfigSecretNamespace = createPVCFields.RemotePVCKubeconfigSecretNamespace
		}

		err = kubernetesClient.CreateIboxreplica(ctx, replica)
		if err != nil {
			return err
		}
		slog.Debug("iboxreplica created", "replicaName", replicaName, "volume name", volName)
	} else {
		// if entity type was CONSISTENCY_GROUP, then we need to add the newly created VOLUME into the CG
		err := verifyCGExists(ctx, cs, cgName)
		if err != nil {
			slog.Error(err.Error())
			return err
		}

		iboxcgName := "iboxcg-" + volName

		// create an iboxcg to add the volume into the CG
		// use the iboxreplica name as the iboxcg name to make it easier to debug
		iboxcg := v1cg.Iboxcg{
			ObjectMeta: metav1.ObjectMeta{
				Name: iboxcgName,
				Annotations: map[string]string{
					common.PVCAnnotationSecretName:      secretName,
					common.PVCAnnotationSecretNamespace: secretNamespace,
				},
			},
			Spec: v1cg.IboxcgSpec{
				Description:     volName + "-" + cgName,
				LocalCGName:     cgName,
				LocalVolumeName: volName,
				BaseAction:      "ADD",
			},
		}

		if createPVC {
			iboxcg.Spec.RemoteIboxCredentialName = createPVCFields.RemoteIboxCredentialName
			iboxcg.Spec.RemoteIboxCredentialNamespace = createPVCFields.RemoteIboxCredentialNamespace
			iboxcg.Spec.RemoteCreatePVC = createPVCFields.RemoteCreatePVC
			iboxcg.Spec.RemotePVCNameSuffix = createPVCFields.RemotePVCNameSuffix
			iboxcg.Spec.RemotePVCName = createPVCFields.RemotePVCName
			iboxcg.Spec.RemotePVCNamespace = createPVCFields.RemotePVCNamespace
			iboxcg.Spec.RemotePVCKubeconfigSecretName = createPVCFields.RemotePVCKubeconfigSecretName
			iboxcg.Spec.RemotePVCKubeconfigSecretNamespace = createPVCFields.RemotePVCKubeconfigSecretNamespace
			iboxcg.Spec.RemoteNetworkSpace = createPVCFields.RemoteNetworkSpace
			iboxcg.Spec.RemotePoolName = createPVCFields.RemotePoolName
		}

		err = kubernetesClient.CreateIboxcg(ctx, iboxcg)
		if err != nil {
			return err
		}
		slog.Debug("iboxcg created", "name", iboxcgName, "cg name", cgName, "vol name", volName)

		return nil
	}

	return nil
}

func verifyCGExists(ctx context.Context, cs storagecommon.Commonservice, cgName string) error {
	var cgFound bool
	for range 10 {
		time.Sleep(time.Second * 1)
		_, err := cs.IboxAPI.GetConsistencyGroupByName(ctx, cgName)
		if err != nil {
			slog.Error("error getting CG", "cg", cgName, "error", err.Error())
		} else {
			slog.Debug("verified CG exists", "cg", cgName)
			cgFound = true
			break
		}
	}
	if !cgFound {
		err := fmt.Errorf("cg not found after 10 seconds, giving up %s", cgName)
		slog.Error(err.Error())
		return err
	}
	return nil
}

func verifyLinkExists(ctx context.Context, cs storagecommon.Commonservice, linkName string) error {
	links, err := cs.IboxAPI.GetLinks(ctx)
	if err != nil {
		return err
	}

	var linkFound bool
	for i := range links {
		if links[i].RemoteSystemName == linkName {
			linkFound = true
			break
		}
	}
	if !linkFound {
		return fmt.Errorf("link %s not found and is required for creating replicas", linkName)
	}
	return nil
}

func handleCreatePVC(params map[string]string) (*RemoteFields, error) {
	tmp := params[common.IboxReplicaCreatePVC] // boolean
	if tmp == "" {
		slog.Debug("create pvc not specified, skipping")
		return nil, nil
	}
	createPVC, err := strconv.ParseBool(params[common.IboxReplicaCreatePVC]) // boolean
	if err != nil {
		slog.Error(err.Error())
		return nil, err
	}
	if !createPVC {
		slog.Debug("create pvc false, skipping")
		return nil, nil
	}

	pvcSuffix := params[common.IboxReplicaCreatePVCSuffix]
	slog.Debug("handling create PVC - %s is %s", common.IboxReplicaCreatePVCSuffix, pvcSuffix)

	pvcNamespace := params[common.IboxReplicaCreatePVCNamespace]
	if pvcNamespace == "" {
		err := fmt.Errorf("%s parameter is required with %s set to true", common.IboxReplicaCreatePVCNamespace, common.IboxReplicaCreatePVC)
		slog.Error(err.Error())
		return nil, err
	}
	pvcNetworkSpace := params[common.IboxReplicaCreatePVCNetworkSpace]
	if pvcNetworkSpace == "" {
		err := fmt.Errorf("%s parameter is required with %s set to true", common.IboxReplicaCreatePVCNetworkSpace, common.IboxReplicaCreatePVC)
		slog.Error(err.Error())
		return nil, err
	}
	pvcPoolName := params[common.IboxReplicaCreatePVCPoolName]
	if pvcPoolName == "" {
		err := fmt.Errorf("%s parameter is required with %s set to true", common.IboxReplicaCreatePVCPoolName, common.IboxReplicaCreatePVC)
		slog.Error(err.Error())
		return nil, err
	}
	remoteIboxCredName := params[common.IboxReplicaRemoteIboxCredNameParameter]
	if remoteIboxCredName == "" {
		err := fmt.Errorf("%s parameter is required with %s set to true", common.IboxReplicaRemoteIboxCredNameParameter, common.IboxReplicaCreatePVC)
		slog.Error(err.Error())
		return nil, err
	}
	remoteIboxCredNamespace := params[common.IboxReplicaRemoteIboxCredNamespaceParameter]
	if remoteIboxCredNamespace == "" {
		err := fmt.Errorf("%s parameter is required with %s set to true", common.IboxReplicaRemoteIboxCredNamespaceParameter, common.IboxReplicaCreatePVC)
		slog.Error(err.Error())
		return nil, err
	}
	kubeconfigSecretName := params[common.IboxReplicaCreatePVCKubeconfigSecretName]
	kubeconfigSecretNamespace := params[common.IboxReplicaCreatePVCKubeconfigSecretNamespace]
	remoteNetworkSpace := params[common.IboxReplicaCreatePVCNetworkSpace]
	slog.Debug("handling create PVC", common.IboxReplicaCreatePVCKubeconfigSecretNamespace, kubeconfigSecretNamespace, common.IboxReplicaCreatePVCKubeconfigSecretName, kubeconfigSecretName)

	fields := &RemoteFields{
		RemoteCreatePVC:                    &createPVC,
		RemotePVCKubeconfigSecretName:      kubeconfigSecretName,
		RemotePVCKubeconfigSecretNamespace: kubeconfigSecretNamespace,
		RemotePVCNamespace:                 pvcNamespace,
		RemoteNetworkSpace:                 remoteNetworkSpace,
		RemotePVCNameSuffix:                pvcSuffix,
		RemotePoolName:                     pvcPoolName,
		RemoteIboxCredentialName:           remoteIboxCredName,
		RemoteIboxCredentialNamespace:      remoteIboxCredNamespace,
	}
	slog.Debug("remote fields", "values", fields)

	return fields, nil
}
