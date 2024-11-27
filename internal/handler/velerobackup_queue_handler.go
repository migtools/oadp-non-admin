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

// VeleroBackupQueueHandler contains event handlers for Velero Backup objects
type VeleroBackupQueueHandler struct {
	Client        client.Client
	OADPNamespace string
}

// Create event handler
func (VeleroBackupQueueHandler) Create(_ context.Context, _ event.CreateEvent, _ workqueue.RateLimitingInterface) {
	// Create event handler for the Backup object
}

// Update event handler adds Velero Backup's NonAdminBackup to controller queue
func (h VeleroBackupQueueHandler) Update(ctx context.Context, evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	// Only update to the first in the queue Velero Backup should trigger changes to the
	// NonAdminBackup objects. Updates to the Velero Backup 2nd and 3rd does not lower the
	// queue. This optimizes the number of times we need to update the NonAdminBackup objects
	// and the number of Velero Backup objects we need to react on.

	logger := function.GetLogger(ctx, evt.ObjectNew, "VeleroBackupQueueHandler")

	// Fetching Velero Backups triggered by NonAdminBackup to optimize our reconcile cycles
	backups, err := function.GetActiveVeleroBackupsByLabel(ctx, h.Client, h.OADPNamespace, constant.ManagedByLabel, constant.ManagedByLabelValue)
	if err != nil {
		logger.Error(err, "Failed to get Velero Backups by label")
		return
	}

	if backups == nil {
		// That should't really be the case as our Update event was triggered by a Velero Backup
		// object that has a new CompletionTimestamp.
		logger.V(1).Info("No pending velero backups found in namespace.", constant.NamespaceString, h.OADPNamespace)
	} else {
		nabEventAnnotations := evt.ObjectNew.GetAnnotations()
		nabEventOriginNamespace := nabEventAnnotations[constant.NabOriginNamespaceAnnotation]
		nabEventOriginName := nabEventAnnotations[constant.NabOriginNameAnnotation]

		for _, backup := range backups {
			annotations := backup.GetAnnotations()
			nabOriginNamespace := annotations[constant.NabOriginNamespaceAnnotation]
			nabOriginName := annotations[constant.NabOriginNameAnnotation]

			// This object is within current queue, so there is no need to trigger changes to it.
			// The VeleroBackupHandler will serve for that.
			if nabOriginNamespace != nabEventOriginNamespace || nabOriginName != nabEventOriginName {
				logger.V(1).Info("Processing Queue update for the NonAdmin Backup referenced by Velero Backup", "Name", backup.Name, constant.NamespaceString, backup.Namespace, "CreatedAt", backup.CreationTimestamp)
				q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
					Name:      nabOriginName,
					Namespace: nabOriginNamespace,
				}})
			} else {
				logger.V(1).Info("Ignoring Queue update for the NonAdmin Backup that triggered this event", "Name", backup.Name, constant.NamespaceString, backup.Namespace, "CreatedAt", backup.CreationTimestamp)
			}
		}
	}
}

// Delete event handler
func (VeleroBackupQueueHandler) Delete(_ context.Context, _ event.DeleteEvent, _ workqueue.RateLimitingInterface) {
	// Delete event handler for the Backup object
}

// Generic event handler
func (VeleroBackupQueueHandler) Generic(_ context.Context, _ event.GenericEvent, _ workqueue.RateLimitingInterface) {
	// Generic event handler for the Backup object
}
