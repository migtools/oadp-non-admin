// velerobackup_predicate.go

package controller

import (
	"context"

	"github.com/go-logr/logr"
	nacv1alpha1 "github.com/migtools/oadp-non-admin/api/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type NonAdminBackupPredicate struct {
	Logger logr.Logger
}

func getNonAdminBackupPredicateLogger(ctx context.Context, name, namespace string) logr.Logger {
	return log.FromContext(ctx).WithValues("NonAdminBackupPredicate", types.NamespacedName{Name: name, Namespace: namespace})
}

func (predicate NonAdminBackupPredicate) Create(ctx context.Context, evt event.CreateEvent) bool {

	if _, ok := evt.Object.(*nacv1alpha1.NonAdminBackup); ok {
		nameSpace := evt.Object.GetNamespace()
		name := evt.Object.GetName()
		log := getNonAdminBackupPredicateLogger(ctx, name, nameSpace)
		log.V(1).Info("Received Create NonAdminBackupPredicate")
		return true
	}

	return false
}

func (predicate NonAdminBackupPredicate) Update(ctx context.Context, evt event.UpdateEvent) bool {
	if _, ok := evt.ObjectNew.(*nacv1alpha1.NonAdminBackup); ok {
		nameSpace := evt.ObjectNew.GetNamespace()
		name := evt.ObjectNew.GetName()
		log := getNonAdminBackupPredicateLogger(ctx, name, nameSpace)
		log.V(1).Info("Received Update NonAdminBackupPredicate")
		return true
	}
	return false
}

func (predicate NonAdminBackupPredicate) Delete(ctx context.Context, evt event.DeleteEvent) bool {
	if _, ok := evt.Object.(*nacv1alpha1.NonAdminBackup); ok {
		nameSpace := evt.Object.GetNamespace()
		name := evt.Object.GetName()
		log := getNonAdminBackupPredicateLogger(ctx, name, nameSpace)
		log.V(1).Info("Received Delete NonAdminBackupPredicate")
		return true
	}
	return false
}

func (predicate NonAdminBackupPredicate) Generic(ctx context.Context, evt event.GenericEvent) bool {
	if _, ok := evt.Object.(*nacv1alpha1.NonAdminBackup); ok {
		nameSpace := evt.Object.GetNamespace()
		name := evt.Object.GetName()
		log := getNonAdminBackupPredicateLogger(ctx, name, nameSpace)
		log.V(1).Info("Received Generic NonAdminBackupPredicate")
		return true
	}
	return false
}
