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

// NonAdminBackupSpec defines the desired state of NonAdminBackup
type NonAdminBackupSpec struct {
	// https://github.com/vmware-tanzu/velero/blob/main/pkg/apis/velero/v1/backup_types.go

	// BackupSpec defines the specification for a Velero backup.
	BackupSpec *velerov1api.BackupSpec `json:"backupSpec,omitempty"`

	// BackupStatus captures the current status of a Velero backup.
	BackupStatus *velerov1api.BackupStatus `json:"backupStatus,omitempty"`

	// NonAdminBackup log level (use debug for the most logging, leave unset for default)
	// +optional
	// +kubebuilder:validation:Enum=trace;debug;info;warning;error;fatal;panic
	LogLevel string `json:"logLevel,omitempty"`
}

// NonAdminBackupStatus defines the observed state of NonAdminBackup
type NonAdminBackupStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// NonAdminBackup is the Schema for the nonadminbackups API
type NonAdminBackup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NonAdminBackupSpec   `json:"spec,omitempty"`
	Status NonAdminBackupStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NonAdminBackupList contains a list of NonAdminBackup
type NonAdminBackupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NonAdminBackup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NonAdminBackup{}, &NonAdminBackupList{})
}
