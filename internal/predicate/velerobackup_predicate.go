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
	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
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

func getBackupPredicateLogger(ctx context.Context, name, namespace string) logr.Logger {
	return log.FromContext(ctx).WithValues("VeleroBackupPredicate", types.NamespacedName{Name: name, Namespace: namespace})
}

// Create event filter
func (veleroBackupPredicate VeleroBackupPredicate) Create(ctx context.Context, evt event.CreateEvent) bool {
	if backup, ok := evt.Object.(*velerov1api.Backup); ok {
		nameSpace := evt.Object.GetNamespace()
		if nameSpace != veleroBackupPredicate.OadpVeleroNamespace {
			return false
		}

		name := evt.Object.GetName()
		logger := getBackupPredicateLogger(ctx, name, nameSpace)
		logger.V(1).Info("VeleroBackupPredicate: Received Create event")

		return function.CheckVeleroBackupLabels(backup)
	}
	return false
}

// Update event filter
func (veleroBackupPredicate VeleroBackupPredicate) Update(ctx context.Context, evt event.UpdateEvent) bool {
	nameSpace := evt.ObjectNew.GetNamespace()
	name := evt.ObjectNew.GetName()
	logger := getBackupPredicateLogger(ctx, name, nameSpace)
	logger.V(1).Info("VeleroBackupPredicate: Received Update event")
	return nameSpace == veleroBackupPredicate.OadpVeleroNamespace
}

// Delete event filter
func (VeleroBackupPredicate) Delete(_ context.Context, _ event.DeleteEvent) bool {
	return false
}

// Generic event filter
func (VeleroBackupPredicate) Generic(_ context.Context, _ event.GenericEvent) bool {
	return false
}
