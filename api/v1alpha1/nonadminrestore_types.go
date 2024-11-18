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

package v1alpha1

import (
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NonAdminRestoreSpec defines the desired state of NonAdminRestore
type NonAdminRestoreSpec struct {
	// restoreSpec defines the specification for a Velero restore.
	RestoreSpec *velerov1.RestoreSpec `json:"restoreSpec"`
}

// VeleroRestore contains information of the related Velero restore object.
type VeleroRestore struct {
	// status captures the current status of the Velero restore.
	// +optional
	Status *velerov1.RestoreStatus `json:"status,omitempty"`

	// references the Velero Restore object by it's name.
	// +optional
	Name string `json:"name,omitempty"`

	// namespace references the Namespace in which Velero Restore exists.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// NonAdminRestoreStatus defines the observed state of NonAdminRestore
type NonAdminRestoreStatus struct {
	// +optional
	VeleroRestore *VeleroRestore `json:"veleroRestore,omitempty"`

	// +optional
	UUID string `json:"uuid,omitempty"`

	// phase is a simple one high-level summary of the lifecycle of an NonAdminRestore.
	Phase NonAdminPhase `json:"phase,omitempty"`

	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=nonadminrestores,shortName=nar

// NonAdminRestore is the Schema for the nonadminrestores API
type NonAdminRestore struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NonAdminRestoreSpec   `json:"spec,omitempty"`
	Status NonAdminRestoreStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NonAdminRestoreList contains a list of NonAdminRestore
type NonAdminRestoreList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NonAdminRestore `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NonAdminRestore{}, &NonAdminRestoreList{})
}
