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
)

const nonAdminBackupPredicateKey = "NonAdminBackupPredicate"

// NonAdminBackupPredicate contains event filters for Non Admin Backup objects
type NonAdminBackupPredicate struct{}

// Create event filter accepts all NonAdminBackup create events
func (NonAdminBackupPredicate) Create(ctx context.Context, evt event.CreateEvent) bool {
	logger := function.GetLogger(ctx, evt.Object, nonAdminBackupPredicateKey)
	logger.V(1).Info("Accepted NAB Create event")
	return true
}

// Update event filter only accepts NonAdminBackup update events that include spec change
func (NonAdminBackupPredicate) Update(ctx context.Context, evt event.UpdateEvent) bool {
	logger := function.GetLogger(ctx, evt.ObjectNew, nonAdminBackupPredicateKey)

	if evt.ObjectNew.GetGeneration() != evt.ObjectOld.GetGeneration() {
		logger.V(1).Info("Accepted NAB Update event")
		return true
	}

	logger.V(1).Info("Rejected NAB Update event")
	return false
}

// Delete event filter accepts all NonAdminBackup delete events
func (NonAdminBackupPredicate) Delete(ctx context.Context, evt event.DeleteEvent) bool {
	logger := function.GetLogger(ctx, evt.Object, nonAdminBackupPredicateKey)
	logger.V(1).Info("Accepted NAB Delete event")
	return true
}
