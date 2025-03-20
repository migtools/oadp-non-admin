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

const nonAdminBackupStorageLocationPredicateKey = "NonAdminBackupStorageLocationPredicate"

// NonAdminBackupStorageLocationPredicate contains event filters for Non Admin Backup Storage Location objects
type NonAdminBackupStorageLocationPredicate struct{}

// Create event filter accepts all NonAdminBackupStorageLocation create events
func (NonAdminBackupStorageLocationPredicate) Create(ctx context.Context, evt event.CreateEvent) bool {
	logger := function.GetLogger(ctx, evt.Object, nonAdminBackupStorageLocationPredicateKey)
	logger.V(1).Info("Accepted Create event")
	return true
}

// Update event filter only accepts NonAdminBackupStorageLocation update events that include spec change
func (NonAdminBackupStorageLocationPredicate) Update(ctx context.Context, evt event.TypedUpdateEvent[client.Object]) bool {
	logger := function.GetLogger(ctx, evt.ObjectNew, nonAdminBackupStorageLocationPredicateKey)

	// spec change
	if evt.ObjectNew.GetGeneration() != evt.ObjectOld.GetGeneration() {
		logger.V(1).Info("Accepted Update event")
		return true
	}

	logger.V(1).Info("Rejected Update event")
	return false
}

// Delete event filter accepts all NonAdminBackupStorageLocation delete events
func (NonAdminBackupStorageLocationPredicate) Delete(ctx context.Context, evt event.DeleteEvent) bool {
	logger := function.GetLogger(ctx, evt.Object, nonAdminBackupStorageLocationPredicateKey)
	logger.V(1).Info("Accepted Delete event")
	return true
}
