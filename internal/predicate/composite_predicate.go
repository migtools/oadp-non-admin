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

	"sigs.k8s.io/controller-runtime/pkg/event"
)

// CompositePredicate is a combination of all project event filters
type CompositePredicate struct {
	Context                 context.Context
	NonAdminBackupPredicate NonAdminBackupPredicate
	VeleroBackupPredicate   VeleroBackupPredicate
}

// Create event filter
func (p CompositePredicate) Create(evt event.CreateEvent) bool {
	// If NonAdminBackupPredicate returns true, ignore VeleroBackupPredicate
	if p.NonAdminBackupPredicate.Create(p.Context, evt) {
		return true
	}
	// Otherwise, apply VeleroBackupPredicate
	return p.VeleroBackupPredicate.Create(p.Context, evt)
}

// Update event filter
func (p CompositePredicate) Update(evt event.UpdateEvent) bool {
	// If NonAdminBackupPredicate returns true, ignore VeleroBackupPredicate
	if p.NonAdminBackupPredicate.Update(p.Context, evt) {
		return true
	}
	// Otherwise, apply VeleroBackupPredicate
	return p.VeleroBackupPredicate.Update(p.Context, evt)
}

// Delete event filter
func (p CompositePredicate) Delete(evt event.DeleteEvent) bool {
	// If NonAdminBackupPredicate returns true, ignore VeleroBackupPredicate
	if p.NonAdminBackupPredicate.Delete(p.Context, evt) {
		return true
	}
	// Otherwise, apply VeleroBackupPredicate
	return p.VeleroBackupPredicate.Delete(p.Context, evt)
}

// Generic event filter
func (p CompositePredicate) Generic(evt event.GenericEvent) bool {
	// If NonAdminBackupPredicate returns true, ignore VeleroBackupPredicate
	if p.NonAdminBackupPredicate.Generic(p.Context, evt) {
		return true
	}
	// Otherwise, apply VeleroBackupPredicate
	return p.VeleroBackupPredicate.Generic(p.Context, evt)
}
