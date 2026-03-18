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
	RemoteIboxCredentialName      string
	RemoteIboxCredentialNamespace string
	RemoteCreatePVC               *bool  `json:"remote_create_pvc,omitempty"`
	RemotePVCNameSuffix           string `json:"remote_pvc_name_suffix,omitempty"`
	RemotePVCName                 string `json:"remote_pvc_name,omitempty"`
	RemotePVCNamespace            string `json:"remote_pvc_namespace,omitempty"`
	RemoteNetworkSpace            string `json:"remote_network_space,omitempty"`
	RemotePoolName                string `json:"remote_pool_name,omitempty"`
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
	if params[common.IboxReplicaRemotePoolIDParameter] == "" {
		return fmt.Errorf("sc replica parameter [%s] is not set but is required", common.IboxReplicaRemotePoolIDParameter)
	}
	poolID, err := strconv.Atoi(params[common.IboxReplicaRemotePoolIDParameter])
	if err != nil {
		return fmt.Errorf("sc replica parameter [%s] is set but is not an integer, error %s", common.IboxReplicaRemotePoolIDParameter, err.Error())
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
				Description:          replicaName,
				EntityType:           entityType,
				LocalEntityName:      entityName,
				RemoteEntityName:     entityName,
				LinkRemoteSystemName: params[common.IboxReplicaRemoteIboxLinkNameParameter],
				ReplicationType:      params[common.IboxReplicaTypeParameter],
				RemotePoolID:         poolID,
			},
		}

		fields, err := handleCreatePVC(params)
		if err != nil {
			return err
		}
		replica.Spec.RemoteIboxCredentialName = fields.RemoteIboxCredentialName
		replica.Spec.RemoteIboxCredentialNamespace = fields.RemoteIboxCredentialNamespace
		replica.Spec.RemoteCreatePVC = fields.RemoteCreatePVC
		replica.Spec.RemotePVCNameSuffix = fields.RemotePVCNameSuffix
		replica.Spec.RemotePVCName = fields.RemotePVCName
		replica.Spec.RemotePVCNamespace = fields.RemotePVCNamespace
		replica.Spec.RemoteNetworkSpace = fields.RemoteNetworkSpace
		replica.Spec.RemotePoolName = fields.RemotePoolName

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

		fields, err := handleCreatePVC(params)
		if err != nil {
			return err
		}
		iboxcg.Spec.RemoteIboxCredentialName = fields.RemoteIboxCredentialName
		iboxcg.Spec.RemoteIboxCredentialNamespace = fields.RemoteIboxCredentialNamespace
		iboxcg.Spec.RemoteCreatePVC = fields.RemoteCreatePVC
		iboxcg.Spec.RemotePVCNameSuffix = fields.RemotePVCNameSuffix
		iboxcg.Spec.RemotePVCName = fields.RemotePVCName
		iboxcg.Spec.RemotePVCNamespace = fields.RemotePVCNamespace
		iboxcg.Spec.RemoteNetworkSpace = fields.RemoteNetworkSpace
		iboxcg.Spec.RemotePoolName = fields.RemotePoolName

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

	fields := &RemoteFields{
		RemoteCreatePVC:               &createPVC,
		RemotePVCNamespace:            pvcNamespace,
		RemotePVCNameSuffix:           pvcSuffix,
		RemotePoolName:                pvcPoolName,
		RemoteIboxCredentialName:      remoteIboxCredName,
		RemoteIboxCredentialNamespace: remoteIboxCredNamespace,
	}

	return fields, nil
}
