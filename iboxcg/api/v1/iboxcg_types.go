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

// IboxcgSpec defines the desired state of Iboxcg
type IboxcgSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Description     string `json:"description,omitempty"`
	LocalCGName     string `json:"local_cg_name,omitempty"`
	LocalVolumeName string `json:"local_volume_name,omitempty"`
	BaseAction      string `json:"base_action,omitempty"`
}

// IboxcgStatus defines the observed state of Iboxcg
type IboxcgStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	State string `json:"state"`
	ID    int    `json:"id,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster

// Iboxcg is the Schema for the iboxcgs API
type Iboxcg struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IboxcgSpec   `json:"spec,omitempty"`
	Status IboxcgStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// IboxcgList contains a list of Iboxcg
type IboxcgList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Iboxcg `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Iboxcg{}, &IboxcgList{})
}
