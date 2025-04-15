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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	nacv1alpha1 "github.com/migtools/oadp-non-admin/api/v1alpha1"
	"github.com/migtools/oadp-non-admin/internal/common/function"
)

// NonAdminBslSecretHandler contains event handlers for NonAdminBackupStorageLocation objects
type NonAdminBslSecretHandler struct {
	Client client.Client
}

// Create event handler
func (h NonAdminBslSecretHandler) Create(ctx context.Context, evt event.CreateEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	// Create event handler for the Secret object
	logger := function.GetLogger(ctx, evt.Object, "NonAdminBslSecretHandler")

	secret, ok := evt.Object.(*corev1.Secret)
	if !ok {
		logger.Error(nil, "Failed to cast event object to Secret")
		return
	}

	var nabslList nacv1alpha1.NonAdminBackupStorageLocationList
	if err := h.Client.List(ctx, &nabslList, client.InNamespace(secret.Namespace)); err != nil {
		logger.Error(err, "Failed to list NonAdminBackupStorageLocation objects")
		return
	}

	for _, nabsl := range nabslList.Items {
		if nabsl.Spec.BackupStorageLocationSpec.Credential.Name == secret.Name {
			logger.V(1).Info("Matching NaBSL found", "NaBSL", nabsl.Name, "Secret", secret.Name)
			q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
				Name:      nabsl.Name,
				Namespace: nabsl.Namespace,
			}})
		}
	}
}

// Update event handler
func (NonAdminBslSecretHandler) Update(_ context.Context, _ event.UpdateEvent, _ workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	// Update event handler for the Secret object
}

// Delete event handler
func (NonAdminBslSecretHandler) Delete(_ context.Context, _ event.DeleteEvent, _ workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	// Delete event handler for the Secret object
}

// Generic event handler
func (NonAdminBslSecretHandler) Generic(_ context.Context, _ event.GenericEvent, _ workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	// Generic event handler for the Secret object
}
