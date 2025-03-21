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

// VeleroRestoreHandler contains event handlers for Velero Restore objects
type VeleroRestoreHandler struct{}

// Create event handler
func (VeleroRestoreHandler) Create(_ context.Context, _ event.CreateEvent, _ workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	// Create event handler for the Restore object
}

// Update event handler adds Velero Restore's NonAdminRestore to controller queue
func (VeleroRestoreHandler) Update(ctx context.Context, evt event.TypedUpdateEvent[client.Object], q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	logger := function.GetLogger(ctx, evt.ObjectNew, "VeleroRestoreHandler")

	annotations := evt.ObjectNew.GetAnnotations()
	nonAdminRestoreName := annotations[constant.NarOriginNameAnnotation]
	nonAdminRestoreNamespace := annotations[constant.NarOriginNamespaceAnnotation]

	q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
		Name:      nonAdminRestoreName,
		Namespace: nonAdminRestoreNamespace,
	}})
	logger.V(1).Info("Handled Update event")
}

// Delete event handler
func (VeleroRestoreHandler) Delete(ctx context.Context, evt event.DeleteEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	logger := function.GetLogger(ctx, evt.Object, "VeleroRestoreHandler")

	annotations := evt.Object.GetAnnotations()
	nonAdminRestoreName := annotations[constant.NarOriginNameAnnotation]
	nonAdminRestoreNamespace := annotations[constant.NarOriginNamespaceAnnotation]

	q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
		Name:      nonAdminRestoreName,
		Namespace: nonAdminRestoreNamespace,
	}})
	logger.V(1).Info("Handled Delete event")
}

// Generic event handler
func (VeleroRestoreHandler) Generic(_ context.Context, _ event.GenericEvent, _ workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	// Generic event handler for the Restore object
}
