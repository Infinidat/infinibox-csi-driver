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

// IboxpromoteSpec defines the desired state of Iboxpromote
type IboxpromoteSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Description     string `json:"description,omitempty"`
	EntityType      string `json:"entity_type,omitempty"`
	EntityName      string `json:"entity_name,omitempty"`
	EntityNamespace string `json:"entity_namespace,omitempty"`
	BaseAction      string `json:"base_action,omitempty"`
}

// IboxpromoteStatus defines the observed state of Iboxpromote
type IboxpromoteStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	State string `json:"state"`
	ID    int    `json:"id,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster

// Iboxpromote is the Schema for the iboxpromotes API
type Iboxpromote struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IboxpromoteSpec   `json:"spec,omitempty"`
	Status IboxpromoteStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// IboxpromoteList contains a list of Iboxpromote
type IboxpromoteList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Iboxpromote `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Iboxpromote{}, &IboxpromoteList{})
}
