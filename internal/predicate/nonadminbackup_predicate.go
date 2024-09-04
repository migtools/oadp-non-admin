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

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"

	nacv1alpha1 "github.com/migtools/oadp-non-admin/api/v1alpha1"
	"github.com/migtools/oadp-non-admin/internal/common/constant"
)

// NonAdminBackupPredicate contains event filters for Non Admin Backup objects
type NonAdminBackupPredicate struct{}

func getNonAdminBackupPredicateLogger(ctx context.Context, name, namespace string) logr.Logger {
	return log.FromContext(ctx).WithValues("NonAdminBackupPredicate", types.NamespacedName{Name: name, Namespace: namespace})
}

// Create event filter
func (NonAdminBackupPredicate) Create(ctx context.Context, evt event.CreateEvent) bool {
	nameSpace := evt.Object.GetNamespace()
	name := evt.Object.GetName()
	logger := getNonAdminBackupPredicateLogger(ctx, name, nameSpace)
	logger.V(1).Info("NonAdminBackupPredicate: Accepted Create event")
	return true
}

// Update event filter
func (NonAdminBackupPredicate) Update(ctx context.Context, evt event.UpdateEvent) bool {
	nameSpace := evt.ObjectNew.GetNamespace()
	name := evt.ObjectNew.GetName()
	logger := getNonAdminBackupPredicateLogger(ctx, name, nameSpace)

	if evt.ObjectNew.GetGeneration() != evt.ObjectOld.GetGeneration() {
		logger.V(1).Info("NonAdminBackupPredicate: Accepted Update event - generation change")
		return true
	}

	if nonAdminBackupOld, ok := evt.ObjectOld.(*nacv1alpha1.NonAdminBackup); ok {
		if nonAdminBackupNew, ok := evt.ObjectNew.(*nacv1alpha1.NonAdminBackup); ok {
			oldPhase := nonAdminBackupOld.Status.Phase
			newPhase := nonAdminBackupNew.Status.Phase

			// New phase set, reconcile
			if oldPhase == constant.EmptyString && newPhase != constant.EmptyString {
				logger.V(1).Info("NonAdminBackupPredicate: Accepted Update event - phase change")
				return true
			} else if oldPhase == nacv1alpha1.NonAdminBackupPhaseNew && newPhase == nacv1alpha1.NonAdminBackupPhaseCreated {
				// This is HARD to understand and TEST
				// even though reconcile will reach Reconcile loop end
				// this will trigger a new reconcile
				logger.V(1).Info("NonAdminBackupPredicate: Accepted Update event - phase created")
				return true
			}
		}
	}
	logger.V(1).Info("NonAdminBackupPredicate: Rejecting Update event")
	return false
}

// Delete event filter
func (NonAdminBackupPredicate) Delete(ctx context.Context, evt event.DeleteEvent) bool {
	nameSpace := evt.Object.GetNamespace()
	name := evt.Object.GetName()
	logger := getNonAdminBackupPredicateLogger(ctx, name, nameSpace)
	logger.V(1).Info("NonAdminBackupPredicate: Accepted Delete event")
	return true
}

// Generic event filter
func (NonAdminBackupPredicate) Generic(ctx context.Context, evt event.GenericEvent) bool {
	nameSpace := evt.Object.GetNamespace()
	name := evt.Object.GetName()
	logger := getNonAdminBackupPredicateLogger(ctx, name, nameSpace)
	logger.V(1).Info("NonAdminBackupPredicate: Accepted Generic event")
	// refactor: all functions start the same way, move this initialization to a separate function
	return true
}
