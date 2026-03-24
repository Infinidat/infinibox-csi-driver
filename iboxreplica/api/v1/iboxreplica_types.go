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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// IboxreplicaSpec defines the desired state of Iboxreplica
type IboxreplicaSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	IsPreferred                        *bool  `json:"is_preferred,omitempty"`
	SyncInterval                       int    `json:"sync_interval,omitempty"`
	Description                        string `json:"description,omitempty"`
	EntityType                         string `json:"entity_type,omitempty"`
	LocalEntityName                    string `json:"local_entity_name,omitempty"`
	RemoteEntityName                   string `json:"remote_entity_name,omitempty"`
	ReplicationType                    string `json:"replication_type,omitempty"`
	BaseAction                         string `json:"base_action,omitempty"`
	LinkRemoteSystemName               string `json:"link_remote_system_name,omitempty"`
	RpoValue                           int    `json:"rpo_value,omitempty"`
	RemotePoolID                       int    `json:"remote_pool_id,omitempty"`
	RemoteIboxCredentialName           string `json:"remote_ibox_credential_name,omitempty"`
	RemoteIboxCredentialNamespace      string `json:"remote_ibox_credential_namespace,omitempty"`
	RemoteCreatePVC                    *bool  `json:"remote_create_pvc,omitempty"`
	RemotePVCNameSuffix                string `json:"remote_pvc_name_suffix,omitempty"`
	RemotePVCName                      string `json:"remote_pvc_name,omitempty"`
	RemotePVCNamespace                 string `json:"remote_pvc_namespace,omitempty"`
	RemoteNetworkSpace                 string `json:"remote_network_space,omitempty"`
	RemotePoolName                     string `json:"remote_pool_name,omitempty"`
	RemotePVCKubeconfigSecretName      string `json:"remote_pvc_kubeconfig_secret_name,omitempty"`
	RemotePVCKubeconfigSecretNamespace string `json:"remote_pvc_kubeconfig_secret_namespace,omitempty"`
}

// IboxreplicaStatus defines the observed state of Iboxreplica
type IboxreplicaStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	State string `json:"state"`
	ID    int    `json:"id,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster

// Iboxreplica is the Schema for the iboxreplicas API
type Iboxreplica struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IboxreplicaSpec   `json:"spec,omitempty"`
	Status IboxreplicaStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// IboxreplicaList contains a list of Iboxreplica
type IboxreplicaList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Iboxreplica `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Iboxreplica{}, &IboxreplicaList{})
}
