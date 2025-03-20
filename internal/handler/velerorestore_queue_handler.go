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

// VeleroRestoreQueueHandler contains event handlers for Velero Restore objects
type VeleroRestoreQueueHandler struct {
	Client        client.Client
	OADPNamespace string
}

// Create event handler
func (VeleroRestoreQueueHandler) Create(_ context.Context, _ event.CreateEvent, _ workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	// Create event handler for the Restore object
}

// Update event handler adds Velero Restore's NonAdminRestore to controller queue
func (h VeleroRestoreQueueHandler) Update(ctx context.Context, evt event.TypedUpdateEvent[client.Object], q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	// Only update to the first in the queue Velero Restore should trigger changes to the
	// NonAdminRestore objects. Updates to the Velero Restore 2nd and 3rd does not lower the
	// queue. This optimizes the number of times we need to update the NonAdminRestore objects
	// and the number of Velero Restore objects we need to react on.

	logger := function.GetLogger(ctx, evt.ObjectNew, "VeleroBackupQueueHandler")

	// Fetching Velero Restores triggered by NonAdminRestore to optimize our reconcile cycles
	restores, err := function.GetActiveVeleroRestoresByLabel(ctx, h.Client, h.OADPNamespace, constant.ManagedByLabel, constant.ManagedByLabelValue)
	if err != nil {
		logger.Error(err, "Failed to get Velero Restores by label")
		return
	}

	if restores == nil {
		// That should't really be the case as our Update event was triggered by a Velero Restore
		// object that has a new CompletionTimestamp.
		logger.V(1).Info("No pending velero restores found in namespace.", constant.NamespaceString, h.OADPNamespace)
	} else {
		nabEventAnnotations := evt.ObjectNew.GetAnnotations()
		nabEventOriginNamespace := nabEventAnnotations[constant.NabOriginNamespaceAnnotation]
		nabEventOriginName := nabEventAnnotations[constant.NabOriginNameAnnotation]

		for _, restore := range restores {
			annotations := restore.GetAnnotations()
			nabOriginNamespace := annotations[constant.NabOriginNamespaceAnnotation]
			nabOriginName := annotations[constant.NabOriginNameAnnotation]

			// This object is within current queue, so there is no need to trigger changes to it.
			// The VeleroBackupHandler will serve for that.
			if nabOriginNamespace != nabEventOriginNamespace || nabOriginName != nabEventOriginName {
				logger.V(1).Info("Processing Queue update for the NonAdmin Restore referenced by Velero Restore", "Name", restore.Name, constant.NamespaceString, restore.Namespace, "CreatedAt", restore.CreationTimestamp)
				q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
					Name:      nabOriginName,
					Namespace: nabOriginNamespace,
				}})
			} else {
				logger.V(1).Info("Ignoring Queue update for the NonAdmin Restore that triggered this event", "Name", restore.Name, constant.NamespaceString, restore.Namespace, "CreatedAt", restore.CreationTimestamp)
			}
		}
	}
}

// Delete event handler
func (VeleroRestoreQueueHandler) Delete(_ context.Context, _ event.DeleteEvent, _ workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	// Delete event handler for the Restore object
}

// Generic event handler
func (VeleroRestoreQueueHandler) Generic(_ context.Context, _ event.GenericEvent, _ workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	// Generic event handler for the Restore object
}
