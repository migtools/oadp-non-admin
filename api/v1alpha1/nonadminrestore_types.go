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
	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NonAdminRestoreSpec defines the desired state of NonAdminRestore
type NonAdminRestoreSpec struct {
	// Specification for a Velero restore.
	// +kubebuilder:validation:Required
	RestoreSpec *velerov1api.RestoreSpec `json:"restoreSpec,omitempty"`
	// TODO add test that NAR can not be created without restoreSpec or restoreSpec.backupName
	// TODO need to investigate restoreSpec.namespaceMapping, depends on how NAC tracks the namespace access per user

	// TODO NonAdminRestore log level, by default TODO.
	// +optional
	// +kubebuilder:validation:Enum=trace;debug;info;warning;error;fatal;panic
	LogLevel string `json:"logLevel,omitempty"`
	// TODO ALSO ADD TEST FOR DIFFERENT LOG LEVELS
}

// NonAdminRestoreStatus defines the observed state of NonAdminRestore
type NonAdminRestoreStatus struct {
	// Related Velero Restore name.
	// +optional
	VeleroRestoreName string `json:"veleroRestoreName,omitempty"`

	// Related Velero Restore status.
	// +optional
	VeleroRestoreStatus *velerov1api.RestoreStatus `json:"veleroRestoreStatus,omitempty"`

	Phase      NonAdminPhase      `json:"phase,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

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
