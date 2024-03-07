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

// Package handler contains all event handlers of the project
package handler

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/migtools/oadp-non-admin/internal/common/constant"
)

// VeleroBackupHandler contains event handlers for Velero Backup objects
type VeleroBackupHandler struct {
	Logger logr.Logger
}

func getVeleroBackupHandlerLogger(ctx context.Context, name, namespace string) logr.Logger {
	return log.FromContext(ctx).WithValues("VeleroBackupHandler", types.NamespacedName{Name: name, Namespace: namespace})
}

// Create event handler
func (*VeleroBackupHandler) Create(ctx context.Context, evt event.CreateEvent, _ workqueue.RateLimitingInterface) {
	nameSpace := evt.Object.GetNamespace()
	name := evt.Object.GetName()
	logger := getVeleroBackupHandlerLogger(ctx, name, nameSpace)
	logger.V(1).Info("Received Create VeleroBackupHandler")
}

// Update event handler
func (*VeleroBackupHandler) Update(ctx context.Context, evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	nameSpace := evt.ObjectNew.GetNamespace()
	name := evt.ObjectNew.GetName()
	logger := getVeleroBackupHandlerLogger(ctx, name, nameSpace)
	logger.V(1).Info("Received Update VeleroBackupHandler")

	annotations := evt.ObjectNew.GetAnnotations()

	if annotations == nil {
		logger.V(1).Info("Backup annotations not found")
		return
	}

	nabOriginNamespace, ok := annotations[constant.NabOriginNamespaceAnnotation]
	if !ok {
		logger.V(1).Info("Backup NonAdminBackup origin namespace annotation not found")
		return
	}

	nabOriginName, ok := annotations[constant.NabOriginNameAnnotation]
	if !ok {
		logger.V(1).Info("Backup NonAdminBackup origin name annotation not found")
		return
	}

	q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
		Name:      nabOriginName,
		Namespace: nabOriginNamespace,
	}})
}

// Delete event handler
func (*VeleroBackupHandler) Delete(_ context.Context, _ event.DeleteEvent, _ workqueue.RateLimitingInterface) {
	// Delete event handler for the Backup object
}

// Generic event handler
func (*VeleroBackupHandler) Generic(_ context.Context, _ event.GenericEvent, _ workqueue.RateLimitingInterface) {
	// Generic event handler for the Backup object
}
