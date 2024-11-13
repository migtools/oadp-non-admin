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
// +kubebuilder:validation:Enum=New;BackingOff;Created;Deleting
type NonAdminBackupPhase string

const (
	// NonAdminBackupPhaseNew - NonAdminBackup resource was accepted by the OpenShift cluster, but it has not yet been processed by the NonAdminController
	NonAdminBackupPhaseNew NonAdminBackupPhase = "New"
	// NonAdminBackupPhaseBackingOff - Velero Backup object was not created due to NonAdminBackup error (configuration or similar)
	NonAdminBackupPhaseBackingOff NonAdminBackupPhase = "BackingOff"
	// NonAdminBackupPhaseCreated - Velero Backup was created. The Phase will not have additional informations about the Backup.
	NonAdminBackupPhaseCreated NonAdminBackupPhase = "Created"
	// NonAdminBackupPhaseDeleting - Velero Backup is pending deletion. The Phase will not have additional informations about the Backup.
	NonAdminBackupPhaseDeleting NonAdminBackupPhase = "Deleting"
)

// NonAdminBackupSpec defines the desired state of NonAdminBackup
type NonAdminBackupSpec struct {
	// BackupSpec defines the specification for a Velero backup.
	BackupSpec *velerov1.BackupSpec `json:"backupSpec,omitempty"`

	// NonAdminBackup log level (use debug for the most logging, leave unset for default)
	// +optional
	// +kubebuilder:validation:Enum=trace;debug;info;warning;error;fatal;panic
	LogLevel string `json:"logLevel,omitempty"`

	// DeleteBackup removes the NonAdminBackup and its associated VeleroBackup from the cluster,
	// as well as the corresponding object storage
	// +optional
	DeleteBackup bool `json:"deleteBackup,omitempty"`

	// ForceDeleteBackup removes the NonAdminBackup and its associated VeleroBackup from the cluster,
	// regardless of whether deletion from object storage succeeds or fails
	// +optional
	ForceDeleteBackup bool `json:"forceDeleteBackup,omitempty"`
}

// VeleroBackup contains information of the related Velero backup object.
type VeleroBackup struct {
	// status captures the current status of the Velero backup.
	// +optional
	Status *velerov1.BackupStatus `json:"status,omitempty"`

	// nacuuid references the Velero Backup object by it's label containing same NACUUID.
	// +optional
	NACUUID string `json:"nacuuid,omitempty"`

	// references the Velero Backup object by it's name.
	// +optional
	Name string `json:"name,omitempty"`

	// namespace references the Namespace in which Velero backup exists.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// VeleroDeleteBackupRequest contains information of the related Velero delete backup request object.
type VeleroDeleteBackupRequest struct {
	// status captures the current status of the Velero delete backup request.
	// +optional
	Status *velerov1.DeleteBackupRequestStatus `json:"status,omitempty"`

	// nacuuid references the Velero delete backup request object by it's label containing same NACUUID.
	// +optional
	NACUUID string `json:"nacuuid,omitempty"`

	// name references the Velero delete backup request object by it's name.
	// +optional
	Name string `json:"name,omitempty"`

	// namespace references the Namespace in which Velero delete backup request exists.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// NonAdminBackupStatus defines the observed state of NonAdminBackup
type NonAdminBackupStatus struct {
	// +optional
	VeleroBackup *VeleroBackup `json:"veleroBackup,omitempty"`

	// +optional
	VeleroDeleteBackupRequest *VeleroDeleteBackupRequest `json:"veleroDeleteBackupRequest,omitempty"`

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
