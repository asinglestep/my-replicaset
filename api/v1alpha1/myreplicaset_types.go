/*
Copyright 2023.

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

package v1alpha1

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// MyReplicasetSpec defines the desired state of MyReplicaset
type MyReplicasetSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Replicas *int `json:"replicas,omitempty"`

	Selector *metav1.LabelSelector `json:"selector"`

	Template v1.PodTemplateSpec `json:"template,omitempty"`
}

// MyReplicasetStatus defines the observed state of MyReplicaset
type MyReplicasetStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Replicas *int `json:"replicas"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// MyReplicaset is the Schema for the myreplicasets API
type MyReplicaset struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MyReplicasetSpec   `json:"spec,omitempty"`
	Status MyReplicasetStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// MyReplicasetList contains a list of MyReplicaset
type MyReplicasetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MyReplicaset `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MyReplicaset{}, &MyReplicasetList{})
}
