// velerobackup_predicate.go

package controller

import (
	"context"

	"github.com/go-logr/logr"
	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

func VeleroPredicate(scheme *runtime.Scheme) predicate.Predicate {
	return nil
}

type VeleroBackupPredicate struct {
	// We are watching only Velero Backup objects within
	// namespace where OADP is.
	OadpVeleroNamespace string
	Logger              logr.Logger
}

func getBackupPredicateLogger(ctx context.Context, name, namespace string) logr.Logger {
	return log.FromContext(ctx).WithValues("VeleroBackupPredicate", types.NamespacedName{Name: name, Namespace: namespace})
}

func (veleroBackupPredicate VeleroBackupPredicate) Create(ctx context.Context, evt event.CreateEvent) bool {
	nameSpace := evt.Object.GetNamespace()
	if nameSpace != veleroBackupPredicate.OadpVeleroNamespace {
		return false
	}

	name := evt.Object.GetName()
	log := getBackupPredicateLogger(ctx, name, nameSpace)
	log.V(1).Info("Received Create VeleroBackupPredicate")

	backup, ok := evt.Object.(*velerov1api.Backup)
	if !ok {
		// The event object is not a Backup, ignore it
		return false
	}
	if HasRequiredLabel(backup) {
		return true
	}
	return false
}

func (veleroBackupPredicate VeleroBackupPredicate) Update(ctx context.Context, evt event.UpdateEvent) bool {
	nameSpace := evt.ObjectNew.GetNamespace()
	name := evt.ObjectNew.GetName()
	log := getBackupPredicateLogger(ctx, name, nameSpace)
	log.V(1).Info("Received Update VeleroBackupPredicate")

	return nameSpace == veleroBackupPredicate.OadpVeleroNamespace
}

func (veleroBackupPredicate VeleroBackupPredicate) Delete(ctx context.Context, evt event.DeleteEvent) bool {
	// NonAdminBackup should not care about VeleroBackup delete events
	return false
}

func (veleroBackupPredicate VeleroBackupPredicate) Generic(ctx context.Context, evt event.GenericEvent) bool {
	return false
}
