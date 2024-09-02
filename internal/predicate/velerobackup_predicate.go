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

	"github.com/migtools/oadp-non-admin/internal/common/function"
)

// VeleroBackupPredicate contains event filters for Velero Backup objects
type VeleroBackupPredicate struct {
	// We are watching only Velero Backup objects within
	// namespace where OADP is.
	OadpVeleroNamespace string
	Logger              logr.Logger
}

// TODO try to remove calls to get logger functions, try to initialize it
func getBackupPredicateLogger(ctx context.Context, name, namespace string) logr.Logger {
	return log.FromContext(ctx).WithValues("VeleroBackupPredicate", types.NamespacedName{Name: name, Namespace: namespace})
}

// Create event filter
func (veleroBackupPredicate VeleroBackupPredicate) Create(ctx context.Context, evt event.CreateEvent) bool {
	nameSpace := evt.Object.GetNamespace()
	name := evt.Object.GetName()
	logger := getBackupPredicateLogger(ctx, name, nameSpace)
	logger.V(1).Info("VeleroBackupPredicate: Received Create event")
	// TODO log accepted or not
	if nameSpace != veleroBackupPredicate.OadpVeleroNamespace {
		return false
	}
	return function.CheckVeleroBackupLabels(evt.Object.GetLabels())
	// refactor idea, move all validation to a function, predicate functions would just need to call it and log info
}

// Update event filter
func (veleroBackupPredicate VeleroBackupPredicate) Update(ctx context.Context, evt event.UpdateEvent) bool {
	nameSpace := evt.ObjectNew.GetNamespace()
	name := evt.ObjectNew.GetName()
	logger := getBackupPredicateLogger(ctx, name, nameSpace)
	logger.V(1).Info("VeleroBackupPredicate: Received Update event")
	// TODO log accepted or not
	// should not check labels?
	return nameSpace == veleroBackupPredicate.OadpVeleroNamespace
}

// Delete event filter
func (VeleroBackupPredicate) Delete(_ context.Context, _ event.DeleteEvent) bool {
	// only create function when needed? changing in composite to simply return false
	return false
}

// Generic event filter
func (VeleroBackupPredicate) Generic(_ context.Context, _ event.GenericEvent) bool {
	return false
}
