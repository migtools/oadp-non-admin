// composite_predicate.go

package controller

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/event"
)

type CompositePredicate struct {
	NonAdminBackupPredicate NonAdminBackupPredicate
	VeleroBackupPredicate   VeleroBackupPredicate
	Context                 context.Context
}

func (p CompositePredicate) Create(evt event.CreateEvent) bool {
	// If NonAdminBackupPredicate returns true, ignore VeleroBackupPredicate
	if p.NonAdminBackupPredicate.Create(p.Context, evt) {
		return true
	}
	// Otherwise, apply VeleroBackupPredicate
	return p.VeleroBackupPredicate.Create(p.Context, evt)
}

func (p CompositePredicate) Update(evt event.UpdateEvent) bool {
	// If NonAdminBackupPredicate returns true, ignore VeleroBackupPredicate
	if p.NonAdminBackupPredicate.Update(p.Context, evt) {
		return true
	}
	// Otherwise, apply VeleroBackupPredicate
	return p.VeleroBackupPredicate.Update(p.Context, evt)
}

func (p CompositePredicate) Delete(evt event.DeleteEvent) bool {
	// If NonAdminBackupPredicate returns true, ignore VeleroBackupPredicate
	if p.NonAdminBackupPredicate.Delete(p.Context, evt) {
		return true
	}
	// Otherwise, apply VeleroBackupPredicate
	return p.VeleroBackupPredicate.Delete(p.Context, evt)
}

func (p CompositePredicate) Generic(evt event.GenericEvent) bool {
	// If NonAdminBackupPredicate returns true, ignore VeleroBackupPredicate
	if p.NonAdminBackupPredicate.Generic(p.Context, evt) {
		return true
	}
	// Otherwise, apply VeleroBackupPredicate
	return p.VeleroBackupPredicate.Generic(p.Context, evt)
}
