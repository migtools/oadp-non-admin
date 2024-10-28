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

// NonAdminBackupPhase is a simple one high-level summary of the lifecycle of an NonAdminBackup.
// +kubebuilder:validation:Enum=New;BackingOff;Created;Deletion
type NonAdminBackupPhase string

const (
	// NonAdminBackupPhaseNew - NonAdminBackup resource was accepted by the OpenShift cluster, but it has not yet been processed by the NonAdminController
	NonAdminBackupPhaseNew NonAdminBackupPhase = "New"
	// NonAdminBackupPhaseBackingOff - Velero Backup object was not created due to NonAdminBackup error (configuration or similar)
	NonAdminBackupPhaseBackingOff NonAdminBackupPhase = "BackingOff"
	// NonAdminBackupPhaseCreated - Velero Backup was created. The Phase will not have additional informations about the Backup.
	NonAdminBackupPhaseCreated NonAdminBackupPhase = "Created"
	// NonAdminBackupPhaseDeletion - Velero Backup is pending deletion. The Phase will not have additional informations about the Backup.
	NonAdminBackupPhaseDeletion NonAdminBackupPhase = "Deletion"
)

// NonAdminBackupSpec defines the desired state of NonAdminBackup
type NonAdminBackupSpec struct {
	// BackupSpec defines the specification for a Velero backup.
	BackupSpec *velerov1.BackupSpec `json:"backupSpec,omitempty"`

	// NonAdminBackup log level (use debug for the most logging, leave unset for default)
	// +optional
	// +kubebuilder:validation:Enum=trace;debug;info;warning;error;fatal;panic
	LogLevel string `json:"logLevel,omitempty"`

	// DeleteBackup tells the controller to remove created Velero Backup.
	// +optional
	DeleteBackup bool `json:"deleteBackup,omitempty"`
}

// VeleroBackup contains information of the related Velero backup object.
type VeleroBackup struct {
	// status captures the current status of the Velero backup.
	// +optional
	Status *velerov1.BackupStatus `json:"status,omitempty"`

	// nameuuid references the Velero Backup object by it's label containing same NameUUID.
	// +optional
	NameUUID string `json:"nameuuid,omitempty"`

	// namespace references the Namespace in which Velero backup exists.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// NonAdminBackupStatus defines the observed state of NonAdminBackup
type NonAdminBackupStatus struct {
	// +optional
	VeleroBackup *VeleroBackup `json:"veleroBackup,omitempty"`

	Phase      NonAdminBackupPhase `json:"phase,omitempty"`
	Conditions []metav1.Condition  `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=nonadminbackups,shortName=nab

// NonAdminBackup is the Schema for the nonadminbackups API
type NonAdminBackup struct {
	Spec   NonAdminBackupSpec   `json:"spec,omitempty"`
	Status NonAdminBackupStatus `json:"status,omitempty"`

	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
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
