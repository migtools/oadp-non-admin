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

// VeleroDataDownloadHandler contains event handlers for Velero DataDownload objects
type VeleroDataDownloadHandler struct {
	client.Client
	OADPNamespace string
}

// Create event handler
func (VeleroDataDownloadHandler) Create(_ context.Context, _ event.CreateEvent, _ workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	// Create event handler for the DataDownload object
}

// Update event handler adds Velero DataDownload's NonAdminRestore to controller queue
func (h VeleroDataDownloadHandler) Update(ctx context.Context, evt event.TypedUpdateEvent[client.Object], q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	logger := function.GetLogger(ctx, evt.ObjectNew, "VeleroDataDownloadHandler")

	owners := evt.ObjectNew.GetOwnerReferences()
	for _, owner := range owners {
		if owner.Kind == "Restore" && owner.APIVersion == velerov1.SchemeGroupVersion.String() {
			restore := &velerov1.Restore{}
			err := h.Get(ctx, types.NamespacedName{
				Namespace: h.OADPNamespace,
				Name:      owner.Name,
			}, restore)
			if err != nil {
				logger.Error(err, "failed to get DataDownload's Restore while handling Update event")
				return
			}
			narOriginNamespace := restore.Annotations[constant.NarOriginNamespaceAnnotation]
			narOriginName := restore.Annotations[constant.NarOriginNameAnnotation]

			q.Add(reconcile.Request{NamespacedName: types.NamespacedName{
				Name:      narOriginName,
				Namespace: narOriginNamespace,
			}})
			logger.V(1).Info("Handled Update event")
			return
		}
	}
	logger.Error(nil, "failed to handle DataDownload Update event")
}

// Delete event handler
func (VeleroDataDownloadHandler) Delete(_ context.Context, _ event.DeleteEvent, _ workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	// Delete event handler for the DataDownload object
}

// Generic event handler
func (VeleroDataDownloadHandler) Generic(_ context.Context, _ event.GenericEvent, _ workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	// Generic event handler for the DataDownload object
}
