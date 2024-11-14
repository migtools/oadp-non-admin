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
	"sigs.k8s.io/controller-runtime/pkg/event"

	nacv1alpha1 "github.com/migtools/oadp-non-admin/api/v1alpha1"
)

// CompositeRestorePredicate is a combination of NonAdminRestore and Velero Restore event filters
type CompositeRestorePredicate struct {
	Context                  context.Context
	NonAdminRestorePredicate NonAdminRestorePredicate
	VeleroRestorePredicate   VeleroRestorePredicate
}

// Create event filter only accepts NonAdminRestore create events
func (p CompositeRestorePredicate) Create(evt event.CreateEvent) bool {
	switch evt.Object.(type) {
	case *nacv1alpha1.NonAdminBackup:
		return p.NonAdminRestorePredicate.Create(p.Context, evt)
	default:
		return false
	}
}

// Update event filter accepts both NonAdminRestore and Velero Restore update events
func (p CompositeRestorePredicate) Update(evt event.UpdateEvent) bool {
	switch evt.ObjectNew.(type) {
	case *nacv1alpha1.NonAdminBackup:
		return p.NonAdminRestorePredicate.Update(p.Context, evt)
	case *velerov1.Backup:
		return p.VeleroRestorePredicate.Update(p.Context, evt)
	default:
		return false
	}
}

// Delete event filter only accepts NonAdminRestore delete events
func (p CompositeRestorePredicate) Delete(evt event.DeleteEvent) bool {
	switch evt.Object.(type) {
	case *nacv1alpha1.NonAdminBackup:
		return p.NonAdminRestorePredicate.Delete(p.Context, evt)
	default:
		return false
	}
}

// Generic event filter does not accept any generic events
func (CompositeRestorePredicate) Generic(_ event.GenericEvent) bool {
	return false
}
