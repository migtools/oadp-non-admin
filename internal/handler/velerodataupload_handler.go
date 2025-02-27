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

	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/migtools/oadp-non-admin/internal/common/constant"
	"github.com/migtools/oadp-non-admin/internal/common/function"
)

// VeleroDataUploadHandler contains event handlers for Velero DataUpload objects
type VeleroDataUploadHandler struct {
	client.Client
	OADPNamespace string
}

// Create event handler
func (VeleroDataUploadHandler) Create(_ context.Context, _ event.CreateEvent, _ workqueue.RateLimitingInterface) {
	// Create event handler for the DataUpload object
}

// Update event handler adds Velero DataUpload's NonAdminBackup to controller queue
func (h VeleroDataUploadHandler) Update(ctx context.Context, evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	logger := function.GetLogger(ctx, evt.ObjectNew, "VeleroDataUploadHandler")

	owners := evt.ObjectNew.GetOwnerReferences()
	for _, owner := range owners {
		if owner.Kind == "Backup" && owner.APIVersion == velerov1.SchemeGroupVersion.String() {
			backup := &velerov1.Backup{}
			err := h.Get(ctx, types.NamespacedName{
				Namespace: h.OADPNamespace,
				Name:      owner.Name,
			}, backup)
			if err != nil {
				logger.Error(err, "failed to get DataUpload's Backup while handling Update event")
				return
			}
			nabOriginNamespace := backup.Annotations[constant.NabOriginNamespaceAnnotation]
			nabOriginName := backup.Annotations[constant.NabOriginNameAnnotation]

			q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
				Name:      nabOriginName,
				Namespace: nabOriginNamespace,
			}})
			logger.V(1).Info("Handled Update event")
			return
		}
	}
	logger.Error(nil, "failed to handle DataUpload Update event")
}

// Delete event handler
func (VeleroDataUploadHandler) Delete(_ context.Context, _ event.DeleteEvent, _ workqueue.RateLimitingInterface) {
	// Delete event handler for the DataUpload object
}

// Generic event handler
func (VeleroDataUploadHandler) Generic(_ context.Context, _ event.GenericEvent, _ workqueue.RateLimitingInterface) {
	// Generic event handler for the DataUpload object
}
