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

// Package predicate contains all event filters of the project
package predicate

import (
	"context"

	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"

	nacv1alpha1 "github.com/migtools/oadp-non-admin/api/v1alpha1"
)

// CompositePredicate is a combination of all project event filters
type CompositePredicate struct {
	Context                 context.Context
	NonAdminBackupPredicate NonAdminBackupPredicate
	VeleroBackupPredicate   VeleroBackupPredicate
}

// Create event filter
func (p CompositePredicate) Create(evt event.CreateEvent) bool {
	switch evt.Object.(type) {
	case *nacv1alpha1.NonAdminBackup:
		// Apply NonAdminBackupPredicate
		return p.NonAdminBackupPredicate.Create(p.Context, evt)
	case *velerov1api.Backup:
		// Apply VeleroBackupPredicate
		return p.VeleroBackupPredicate.Create(p.Context, evt)
	default:
		// Unknown object type, return false
		return false
	}
}

// Update event filter
func (p CompositePredicate) Update(evt event.UpdateEvent) bool {
	switch evt.ObjectNew.(type) {
	case *nacv1alpha1.NonAdminBackup:
		return p.NonAdminBackupPredicate.Update(p.Context, evt)
	case *velerov1api.Backup:
		return p.VeleroBackupPredicate.Update(p.Context, evt)
	default:
		return false
	}
}

// Delete event filter
func (p CompositePredicate) Delete(evt event.DeleteEvent) bool {
	switch evt.Object.(type) {
	case *nacv1alpha1.NonAdminBackup:
		return p.NonAdminBackupPredicate.Delete(p.Context, evt)
	case *velerov1api.Backup:
		return p.VeleroBackupPredicate.Delete(p.Context, evt)
	default:
		return false
	}
}

// Generic event filter
func (p CompositePredicate) Generic(evt event.GenericEvent) bool {
	switch evt.Object.(type) {
	case *nacv1alpha1.NonAdminBackup:
		return p.NonAdminBackupPredicate.Generic(p.Context, evt)
	case *velerov1api.Backup:
		return p.VeleroBackupPredicate.Generic(p.Context, evt)
	default:
		return false
	}
}
