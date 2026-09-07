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

// VeleroRestorePredicate contains event filters for Velero Restore objects
type VeleroRestorePredicate struct {
	OADPNamespace string
}

// Update event filter only accepts Velero Restore update events from OADP namespace
// and from Velero Restores that have required metadata
func (p VeleroRestorePredicate) Update(ctx context.Context, evt event.UpdateEvent) bool {
	logger := function.GetLogger(ctx, evt.ObjectNew, "VeleroRestorePredicate")

	namespace := evt.ObjectNew.GetNamespace()
	if namespace == p.OADPNamespace {
		if function.CheckVeleroRestoreMetadata(evt.ObjectNew) {
			logger.V(1).Info("Accepted Restore Update event")
			return true
		}
	}

	logger.V(1).Info("Rejected Restore Update event")
	return false
}

// Delete event filter only accepts Velero Restore delete events from OADP namespace
// and from Velero Restores that have required metadata
func (p VeleroRestorePredicate) Delete(ctx context.Context, evt event.DeleteEvent) bool {
	logger := function.GetLogger(ctx, evt.Object, "VeleroRestorePredicate")

	namespace := evt.Object.GetNamespace()
	if namespace == p.OADPNamespace {
		if function.CheckVeleroRestoreMetadata(evt.Object) {
			logger.V(1).Info("Accepted Restore Delete event")
			return true
		}
	}

	logger.V(1).Info("Rejected Restore Delete event")
	return false
}
