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

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/migtools/oadp-non-admin/internal/common/constant"
	"github.com/migtools/oadp-non-admin/internal/common/function"
)

// NonAdminBackupStorageLocationRequestHandler contains event handlers for NonAdminBackupStorageLocationRequest objects
type NonAdminBackupStorageLocationRequestHandler struct{}

// Create event handler
func (NonAdminBackupStorageLocationRequestHandler) Create(_ context.Context, _ event.CreateEvent, _ workqueue.RateLimitingInterface) {
	// Create event handler for the NonAdminBackupStorageLocationRequest object
}

// Update event handler adds NonAdminBackupStorageLocationRequest's NonAdminBackupStorageLocation to controller queue
func (NonAdminBackupStorageLocationRequestHandler) Update(ctx context.Context, evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	logger := function.GetLogger(ctx, evt.ObjectNew, "NonAdminBackupStorageLocationRequestHandler")

	annotations := evt.ObjectNew.GetAnnotations()
	nabslOriginNamespace := annotations[constant.NabslOriginNamespaceAnnotation]
	nabslOriginName := annotations[constant.NabslOriginNameAnnotation]

	q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
		Name:      nabslOriginName,
		Namespace: nabslOriginNamespace,
	}})
	logger.V(1).Info("Handled Update event")
}

// Delete event handler
func (NonAdminBackupStorageLocationRequestHandler) Delete(ctx context.Context, evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	logger := function.GetLogger(ctx, evt.Object, "NonAdminBackupStorageLocationRequestHandler")

	annotations := evt.Object.GetAnnotations()
	nabslOriginNamespace := annotations[constant.NabslOriginNamespaceAnnotation]
	nabslOriginName := annotations[constant.NabslOriginNameAnnotation]

	q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
		Name:      nabslOriginName,
		Namespace: nabslOriginNamespace,
	}})
	logger.V(1).Info("Handled Delete event")
}

// Generic event handler
func (NonAdminBackupStorageLocationRequestHandler) Generic(_ context.Context, _ event.GenericEvent, _ workqueue.RateLimitingInterface) {
	// Generic event handler for the NonAdminBackupStorageLocationRequest object
}
