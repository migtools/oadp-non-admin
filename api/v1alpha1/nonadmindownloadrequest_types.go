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
	"fmt"

	"github.com/migtools/oadp-non-admin/internal/common/constant"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// NonAdminDownloadRequestSpec defines the desired state of NonAdminDownloadRequest.
// Mirrors velero DownloadRequestSpec to allow non admins to download information for a non admin backup/restore
type NonAdminDownloadRequestSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Target is what to download (e.g. logs for a backup).
	Target velerov1.DownloadTarget `json:"target"`
}

// VeleroDownloadRequest represents VeleroDownloadRequest
type VeleroDownloadRequest struct {
	// VeleroDownloadRequestStatus represents VeleroDownloadRequestStatus
	// +optional
	Status *velerov1.DownloadRequestStatus `json:"status,omitempty"`
	// name references the Velero delete backup request object by it's name.
	// +optional
	Name string `json:"name,omitempty"`

	// namespace references the Namespace in which Velero delete backup request exists.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// NonAdminDownloadRequestStatus defines the observed state of NonAdminDownloadRequest.
type NonAdminDownloadRequestStatus struct {
	// +optional
	VeleroDownloadRequest VeleroDownloadRequest `json:"velero,omitempty"`
	// phase is a simple one high-level summary of the lifecycle of an NonAdminDownloadRequest
	Phase NonAdminPhase `json:"phase,omitempty"`

	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// NonAdminDownloadRequest is the Schema for the nonadmindownloadrequests API.
type NonAdminDownloadRequest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NonAdminDownloadRequestSpec   `json:"spec,omitempty"`
	Status NonAdminDownloadRequestStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NonAdminDownloadRequestList contains a list of NonAdminDownloadRequest.
type NonAdminDownloadRequestList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NonAdminDownloadRequest `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NonAdminDownloadRequest{}, &NonAdminDownloadRequestList{})
}

// ReadyForProcessing returns if this NonAdminDownloadRequests is in a state ready for processing
// only process NADR with target kind and name populated and phase is not yet completed
// returns true if ready for processing, false if required fields are not populated
func (nadr *NonAdminDownloadRequest) ReadyForProcessing() bool {
	return nadr.Spec.Target.Kind != constant.EmptyString &&
		nadr.Spec.Target.Name != constant.EmptyString &&
		nadr.Status.Phase != NonAdminPhaseCompleted
}

// VeleroDownloadRequestName defines velero download request name for this NonAdminDownloadRequest
func (nadr *NonAdminDownloadRequest) VeleroDownloadRequestName() string {
	return fmt.Sprintf("nadr-%s", string(nadr.GetUID()))
}
