// velerobackup_handler.go

package controller

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

// Handler for VeleroBackup events
type VeleroBackupHandler struct {
	Logger logr.Logger
}

func getVeleroBackupHandlerLogger(ctx context.Context, name, namespace string) logr.Logger {
	return log.FromContext(ctx).WithValues("VeleroBackupHandler", types.NamespacedName{Name: name, Namespace: namespace})
}

func (h *VeleroBackupHandler) Create(ctx context.Context, evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	nameSpace := evt.Object.GetNamespace()
	name := evt.Object.GetName()
	log := getVeleroBackupHandlerLogger(ctx, name, nameSpace)
	log.V(1).Info("Received Create VeleroBackupHandler")
}

func (h *VeleroBackupHandler) Update(ctx context.Context, evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	nameSpace := evt.ObjectNew.GetNamespace()
	name := evt.ObjectNew.GetName()
	log := getVeleroBackupHandlerLogger(ctx, name, nameSpace)
	log.V(1).Info("Received Update VeleroBackupHandler")

	annotations := evt.ObjectNew.GetAnnotations()

	if annotations == nil {
		log.V(1).Info("Backup annotations not found")
		return
	}

	nabOriginNamespace, ok := annotations[NabOriginNamespaceAnnotation]
	if !ok {
		log.V(1).Info("Backup NonAdminBackup origin namespace annotation not found")
		return
	}

	nabOriginName, ok := annotations[NabOriginNameAnnotation]
	if !ok {
		log.V(1).Info("Backup NonAdminBackup origin name annotation not found")
		return
	}

	q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
		Name:      nabOriginName,
		Namespace: nabOriginNamespace,
	}})
}

func (h *VeleroBackupHandler) Delete(ctx context.Context, evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	// Delete event handler for the Backup object. We should ignore it.
}

func (h *VeleroBackupHandler) Generic(ctx context.Context, evt event.GenericEvent, q workqueue.RateLimitingInterface) {
	// Generic event handler for the Backup object. We should ignore it.
}
