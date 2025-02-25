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

// CompositeNaBSLPredicate is a combination of NonAdminBackupStorageLocation and Velero BackupStorageLocation event filters
type CompositeNaBSLPredicate struct {
	Context                                       context.Context
	NonAdminBackupStorageLocationPredicate        NonAdminBackupStorageLocationPredicate
	NonAdminBackupStorageLocationRequestPredicate NonAdminBackupStorageLocationRequestPredicate
	VeleroBackupStorageLocationPredicate          VeleroBackupStorageLocationPredicate
}

// Create event filter only accepts NonAdminBackupStorageLocation create events
func (p CompositeNaBSLPredicate) Create(evt event.CreateEvent) bool {
	switch evt.Object.(type) {
	case *nacv1alpha1.NonAdminBackupStorageLocation:
		return p.NonAdminBackupStorageLocationPredicate.Create(p.Context, evt)
	default:
		return false
	}
}

// Update event filter accepts both NonAdminBackupStorageLocation and Velero BackupStorageLocation update events
func (p CompositeNaBSLPredicate) Update(evt event.UpdateEvent) bool {
	switch evt.ObjectNew.(type) {
	case *nacv1alpha1.NonAdminBackupStorageLocation:
		return p.NonAdminBackupStorageLocationPredicate.Update(p.Context, evt)
	case *velerov1.BackupStorageLocation:
		return p.VeleroBackupStorageLocationPredicate.Update(p.Context, evt)
	case *nacv1alpha1.NonAdminBackupStorageLocationRequest:
		return p.NonAdminBackupStorageLocationRequestPredicate.Update(p.Context, evt)
	default:
		return false
	}
}

// Delete event filter only accepts NonAdminBackupStorageLocation delete events
func (p CompositeNaBSLPredicate) Delete(evt event.DeleteEvent) bool {
	switch evt.Object.(type) {
	case *nacv1alpha1.NonAdminBackupStorageLocation:
		return p.NonAdminBackupStorageLocationPredicate.Delete(p.Context, evt)
	case *nacv1alpha1.NonAdminBackupStorageLocationRequest:
		return p.NonAdminBackupStorageLocationRequestPredicate.Delete(p.Context, evt)
	default:
		return false
	}
}

// Generic event filter does not accept any generic events
func (CompositeNaBSLPredicate) Generic(_ event.GenericEvent) bool {
	return false
}
