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

package predicate

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/migtools/oadp-non-admin/internal/common/function"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
)

// VeleroBackupQueuePredicate contains event filters for Velero Backup objects
type VeleroBackupQueuePredicate struct {
	OADPNamespace string
}

// Update event filter only accepts Velero Backup update events from the OADP namespace
// and from Velero Backups that have a new CompletionTimestamp. We are not interested in
// checking if the Velero Backup contains NonAdminBackup metadata, because every Velero Backup
// may change the Queue position of the NonAdminBackup object.
func (p VeleroBackupQueuePredicate) Update(ctx context.Context, evt event.UpdateEvent) bool {
	logger := function.GetLogger(ctx, evt.ObjectNew, "VeleroBackupQueuePredicate")

	// Ensure the new and old objects are of the expected type
	newBackup, okNew := evt.ObjectNew.(*velerov1.Backup)
	oldBackup, okOld := evt.ObjectOld.(*velerov1.Backup)

	if !okNew || !okOld {
		logger.V(1).Info("Rejected Update event: invalid object type")
		return false
	}

	namespace := newBackup.GetNamespace()

	if namespace == p.OADPNamespace {
		if oldBackup.Status.CompletionTimestamp == nil && newBackup.Status.CompletionTimestamp != nil {
			logger.V(1).Info("Accepted Update event: new completion timestamp")
			return true
		}
	}

	logger.V(1).Info("Rejected Update event: no changes to the CompletionTimestamp in the VeleroBackup object")
	return false
}
