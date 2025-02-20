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

// NonAdminBSLApprovalRequest controls the approval of the NonAdminBackupStorageLocation
// +kubebuilder:validation:Enum=approve;reject;pending
type NonAdminBSLApprovalRequest string

// Predefined NonAdminBSLApprovalRequestConditions
const (
	NonAdminBSLApprovalRequestApproved NonAdminBSLApprovalRequest = "approve"
	NonAdminBSLApprovalRequestRejected NonAdminBSLApprovalRequest = "reject"
	NonAdminBSLApprovalRequestPending  NonAdminBSLApprovalRequest = "pending"
)

// NonAdminBackupStorageLocationApprovalRequestSpec defines the desired state of NonAdminBackupStorageLocationApprovalRequest
type NonAdminBackupStorageLocationApprovalRequestSpec struct {
	// pendingApprovalDecision is the decision of the cluster admin on the currently pending NonAdminBackupStorageLocation creation or update.
	// This applies only to the pending NonAdminBackupStorageLocation Spec waiting for the approval.
	// If set to "reject", the update request will not be approved, but an already approved spec remains unless explicitly revoked.
	// +optional
	PendingApprovalDecision NonAdminBSLApprovalRequest `json:"pendingApprovalDecision,omitempty"`

	// revokeApprovedSpec indicates whether the previously approved NonAdminBackupStorageLocation Spec should be revoked.
	// if set to true, the entire BSL will be removed regardless of the PendingApprovalDecision and the PendingApprovalDecision will become "pending".
	// The AprovedSpec from the Status will be updated to the PendingSpec.
	// +optional
	RevokeApprovedSpec bool `json:"revokeApprovedSpec,omitempty"`
}

// VeleroBackupStorageLocationApprovalRequest contains information of the related Velero backup object.
type VeleroBackupStorageLocationApprovalRequest struct {
	// pendingApprovalSpec contains the pending spec request for the NonAdminBackupStorageLocation
	// +optionl
	PendingSpec *velerov1.BackupStorageLocationSpec `json:"pendingApprovalSpec"`

	// approvedSpec contains the approved spec request for the NonAdminBackupStorageLocation
	// +optionl
	ApprovedSpec *velerov1.BackupStorageLocationSpec `json:"approvedSpec"`

	// nacuuid references the NonAdminBackupStorageLocation object by it's label containing same NACUUID.
	// +optional
	NACUUID string `json:"nacuuid,omitempty"`

	// name references the NonAdminBackupStorageLocation object by it's name.
	// +optional
	Name string `json:"name,omitempty"`

	// namespace references the Namespace in which NonAdminBackupStorageLocation exists.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// NonAdminBackupStorageLocationApprovalRequestStatus defines the observed state of NonAdminBackupStorageLocationApprovalRequest
type NonAdminBackupStorageLocationApprovalRequestStatus struct {
	// +optional
	VeleroBackupStorageLocationApprovalRequest *VeleroBackupStorageLocationApprovalRequest `json:"veleroBackupStorageLocationApprovalRequest,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=nonadminbackupstoragelocationapprovalrequests,shortName=nabslapprovalrequest

// NonAdminBackupStorageLocationApprovalRequest is the Schema for the nonadminbackupstoragelocationapprovalrequests API
type NonAdminBackupStorageLocationApprovalRequest struct {
	Status            NonAdminBackupStorageLocationApprovalRequestStatus `json:"status,omitempty"`
	Spec              NonAdminBackupStorageLocationApprovalRequestSpec   `json:"spec,omitempty"`
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
}

// +kubebuilder:object:root=true

// NonAdminBackupStorageLocationApprovalRequestList contains a list of NonAdminBackupStorageLocationApprovalRequest
type NonAdminBackupStorageLocationApprovalRequestList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NonAdminBackupStorageLocationApprovalRequest `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NonAdminBackupStorageLocationApprovalRequest{}, &NonAdminBackupStorageLocationApprovalRequestList{})
}
