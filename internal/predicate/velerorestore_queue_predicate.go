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

	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/migtools/oadp-non-admin/internal/common/function"
)

// VeleroRestoreQueuePredicate contains event filters for Velero Restore objects
type VeleroRestoreQueuePredicate struct {
	OADPNamespace string
}

// Update event filter only accepts Velero Restore update events from the OADP namespace
// and from Velero Restores that have a new CompletionTimestamp. We are not interested in
// checking if the Velero Restore contains NonAdminRestore metadata, because every Velero Restore
// may change the Queue position of the NonAdminRestore object.
func (p VeleroRestoreQueuePredicate) Update(ctx context.Context, evt event.TypedUpdateEvent[client.Object]) bool {
	logger := function.GetLogger(ctx, evt.ObjectNew, "VeleroRestoreQueuePredicate")

	// Ensure the new and old objects are of the expected type
	newRestore, okNew := evt.ObjectNew.(*velerov1.Restore)
	oldRestore, okOld := evt.ObjectOld.(*velerov1.Restore)

	if !okNew || !okOld {
		logger.V(1).Info("Rejected Restore Update event: invalid object type")
		return false
	}

	namespace := newRestore.GetNamespace()

	if namespace == p.OADPNamespace {
		if oldRestore.Status.CompletionTimestamp == nil && newRestore.Status.CompletionTimestamp != nil {
			logger.V(1).Info("Accepted Restore Update event: new completion timestamp")
			return true
		}
	}

	logger.V(1).Info("Rejected Restore Update event: no changes to the CompletionTimestamp in the VeleroRestore object")
	return false
}
