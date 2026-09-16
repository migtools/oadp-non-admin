/*
Copyright 2026.

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

// NonAdminScheduleTemplate defines the Backup template for a non-admin schedule.
// Metadata is deliberately not exposed. NAC owns the backing Schedule metadata so
// generated Backups always retain their tenant ownership information.
type NonAdminScheduleTemplate struct {
	// BackupSpec defines the specification for every generated Velero Backup.
	BackupSpec *velerov1.BackupSpec `json:"backupSpec"`
}

// NonAdminScheduleSpec defines the desired state of NonAdminSchedule.
type NonAdminScheduleSpec struct {
	// Schedule is a standard cron expression defining when to create a backup.
	Schedule string `json:"schedule"`

	// Paused stops future scheduled backups until it is set to false.
	// +optional
	Paused bool `json:"paused,omitempty"`

	// SkipImmediately skips the next due backup. NAC resets this field after it
	// has submitted the request to the backing Velero Schedule.
	// +optional
	SkipImmediately bool `json:"skipImmediately,omitempty"`

	// Template defines the backup request to run on the schedule.
	Template NonAdminScheduleTemplate `json:"template"`
}

// VeleroSchedule contains information about the backing Velero Schedule object.
type VeleroSchedule struct {
	// Status captures the current status of the backing Velero Schedule.
	// +optional
	Status *velerov1.ScheduleStatus `json:"status,omitempty"`

	// NACUUID identifies the backing Velero Schedule.
	// +optional
	NACUUID string `json:"nacuuid,omitempty"`

	// Name is the backing Velero Schedule name.
	// +optional
	Name string `json:"name,omitempty"`

	// Namespace is the namespace containing the backing Velero Schedule.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// NonAdminScheduledBackup describes one generated backup in schedule status.
type NonAdminScheduledBackup struct {
	// Name is the synchronized NonAdminBackup name for this schedule run.
	Name string `json:"name"`

	// VeleroBackupName is the generated backing Velero Backup name.
	VeleroBackupName string `json:"veleroBackupName"`

	// Phase is the current Velero Backup phase.
	Phase velerov1.BackupPhase `json:"phase,omitempty"`

	// StartTimestamp is when the Backup started.
	// +optional
	StartTimestamp *metav1.Time `json:"startTimestamp,omitempty"`

	// CompletionTimestamp is when the Backup completed.
	// +optional
	CompletionTimestamp *metav1.Time `json:"completionTimestamp,omitempty"`

	// Expiration is when Velero will remove the Backup.
	// +optional
	Expiration *metav1.Time `json:"expiration,omitempty"`
}

// NonAdminScheduleStatus defines the observed state of NonAdminSchedule.
type NonAdminScheduleStatus struct {
	// VeleroSchedule identifies and reports the backing Velero Schedule.
	// +optional
	VeleroSchedule *VeleroSchedule `json:"veleroSchedule,omitempty"`

	// NextRun is calculated from the cron expression and the backing Schedule status.
	// +optional
	NextRun *metav1.Time `json:"nextRun,omitempty"`

	// LastBackup is the newest generated Backup known to NAC.
	// +optional
	LastBackup *NonAdminScheduledBackup `json:"lastBackup,omitempty"`

	// RecentBackups contains at most five generated Backups, newest first.
	// +optional
	RecentBackups []NonAdminScheduledBackup `json:"recentBackups,omitempty"`

	// Phase is a high-level summary of the NAS lifecycle.
	// +optional
	Phase NonAdminPhase `json:"phase,omitempty"`

	// Conditions contains detailed acceptance and lifecycle information.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=nonadminschedules,shortName=nas
// +kubebuilder:printcolumn:name="Request-Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Velero-Phase",type="string",JSONPath=".status.veleroSchedule.status.phase"
// +kubebuilder:printcolumn:name="Next-Run",type="date",JSONPath=".status.nextRun"
// +kubebuilder:printcolumn:name="Paused",type="boolean",JSONPath=".spec.paused"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// NonAdminSchedule is the Schema for non-admin backup schedules.
type NonAdminSchedule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NonAdminScheduleSpec   `json:"spec,omitempty"`
	Status NonAdminScheduleStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NonAdminScheduleList contains a list of NonAdminSchedules.
type NonAdminScheduleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NonAdminSchedule `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NonAdminSchedule{}, &NonAdminScheduleList{})
}
