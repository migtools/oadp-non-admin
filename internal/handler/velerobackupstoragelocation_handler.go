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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/migtools/oadp-non-admin/internal/common/constant"
	"github.com/migtools/oadp-non-admin/internal/common/function"
)

// VeleroBackupStorageLocationHandler contains event handlers for Velero BackupStorageLocation objects
type VeleroBackupStorageLocationHandler struct{}

// Create event handler
func (VeleroBackupStorageLocationHandler) Create(_ context.Context, _ event.CreateEvent, _ workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	// Create event handler for the BackupStorageLocation object
}

// Update event handler adds Velero BackupStorageLocation's NonAdminBackupStorageLocation to controller queue
func (VeleroBackupStorageLocationHandler) Update(ctx context.Context, evt event.TypedUpdateEvent[client.Object], q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	logger := function.GetLogger(ctx, evt.ObjectNew, "VeleroBackupStorageLocationHandler")

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
func (VeleroBackupStorageLocationHandler) Delete(_ context.Context, _ event.DeleteEvent, _ workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	// Delete event handler for the BackupStorageLocation object
}

// Generic event handler
func (VeleroBackupStorageLocationHandler) Generic(_ context.Context, _ event.GenericEvent, _ workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	// Generic event handler for the BackupStorageLocation object
}
