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

const garbageCollectorPredicateKey = "GarbageCollectorPredicate"

// GarbageCollectorPredicate contains event filters for GarbageCollector objects
type GarbageCollectorPredicate struct {
	Context context.Context
}

// Create event filter accepts all Velero BackupStorageLocation create events
func (g GarbageCollectorPredicate) Create(evt event.CreateEvent) bool {
	logger := function.GetLogger(g.Context, evt.Object, garbageCollectorPredicateKey)
	logger.V(1).Info("Accepted GarbageCollector Create event")
	return true
}

// Update event filter rejects all Velero BackupStorageLocation create events
func (GarbageCollectorPredicate) Update(_ event.UpdateEvent) bool {
	return false
}

// Delete event filter rejects all Velero BackupStorageLocation delete events
func (GarbageCollectorPredicate) Delete(_ event.DeleteEvent) bool {
	return false
}

// Generic event filter rejects all Velero BackupStorageLocation create events
func (GarbageCollectorPredicate) Generic(_ event.GenericEvent) bool {
	return false
}
