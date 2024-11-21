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

	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"

	nacv1alpha1 "github.com/migtools/oadp-non-admin/api/v1alpha1"
)

// CompositePredicate is a combination of NonAdminBackup and Velero Backup event filters
type CompositePredicate struct {
	Context                    context.Context
	NonAdminBackupPredicate    NonAdminBackupPredicate
	VeleroBackupPredicate      VeleroBackupPredicate
	VeleroBackupQueuePredicate VeleroBackupQueuePredicate
}

// Create event filter only accepts NonAdminBackup create events
func (p CompositePredicate) Create(evt event.CreateEvent) bool {
	switch evt.Object.(type) {
	case *nacv1alpha1.NonAdminBackup:
		return p.NonAdminBackupPredicate.Create(p.Context, evt)
	default:
		return false
	}
}

// Update event filter accepts both NonAdminBackup and Velero Backup update events
func (p CompositePredicate) Update(evt event.UpdateEvent) bool {
	switch evt.ObjectNew.(type) {
	case *nacv1alpha1.NonAdminBackup:
		return p.NonAdminBackupPredicate.Update(p.Context, evt)
	case *velerov1.Backup:
		return p.VeleroBackupQueuePredicate.Update(p.Context, evt) || p.VeleroBackupPredicate.Update(p.Context, evt)
	default:
		return false
	}
}

// Delete event filter only accepts NonAdminBackup delete events
func (p CompositePredicate) Delete(evt event.DeleteEvent) bool {
	switch evt.Object.(type) {
	case *nacv1alpha1.NonAdminBackup:
		return p.NonAdminBackupPredicate.Delete(p.Context, evt)
	default:
		return false
	}
}

// Generic event filter does not accept any generic events
func (CompositePredicate) Generic(_ event.GenericEvent) bool {
	return false
}
