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

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/migtools/oadp-non-admin/internal/common/function"
)

// VeleroBackupStorageLocationPredicate contains event filters for Velero BackupStorageLocation objects
type VeleroBackupStorageLocationPredicate struct {
	OADPNamespace string
}

// Update event filter only accepts Velero Backup update events from OADP namespace
// and from Velero Backups that have required metadata
func (p VeleroBackupStorageLocationPredicate) Update(ctx context.Context, evt event.TypedUpdateEvent[client.Object]) bool {
	logger := function.GetLogger(ctx, evt.ObjectNew, "VeleroBackupStorageLocationPredicate")

	namespace := evt.ObjectNew.GetNamespace()
	if namespace == p.OADPNamespace {
		if function.CheckVeleroBackupStorageLocationMetadata(evt.ObjectNew) {
			logger.V(1).Info("Accepted BackupStorageLocation Update event")
			return true
		}
	}

	logger.V(1).Info("Rejected Update event")
	return false
}
