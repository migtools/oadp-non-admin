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

// NonAdminPhase is a simple one high-level summary of the lifecycle of a NonAdminBackup or NonAdminRestore.
// +kubebuilder:validation:Enum=New;BackingOff;Created;Deleting
type NonAdminPhase string

const (
	// NonAdminPhaseNew - NonAdmin object was accepted by the OpenShift cluster, but it has not yet been processed by the NonAdminController
	NonAdminPhaseNew NonAdminPhase = "New"
	// NonAdminPhaseBackingOff - Velero object was not created due to NonAdmin object error (configuration or similar)
	NonAdminPhaseBackingOff NonAdminPhase = "BackingOff"
	// NonAdminPhaseCreated - Velero object was created. The Phase will not have additional information about it.
	NonAdminPhaseCreated NonAdminPhase = "Created"
	// NonAdminPhaseDeleting - Velero object is pending deletion. The Phase will not have additional information about it.
	NonAdminPhaseDeleting NonAdminPhase = "Deleting"
)

// QueueInfo represents the position of a specific operation request (Velero Backup or Restore) in the queue.
// It estimates how many operations are pending before the current request is processed.
// The value is approximate and may be inaccurate if the Velero pod is down or restarts after related objects are created.
// It includes only VeleroBackups and VeleroRestores still subject to be handled by OADP/Velero.
type QueueInfo struct {
	// EstimatedQueuePosition is the number of backups or restores ahead in the queue (0 if not queued)
	EstimatedQueuePosition int `json:"estimatedQueuePosition"`
}
