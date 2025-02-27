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

package predicate

import (
	"context"

	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/migtools/oadp-non-admin/internal/common/function"
)

// VeleroDataUploadPredicate contains event filters for Velero DataUpload objects
type VeleroDataUploadPredicate struct {
	client.Client
	OADPNamespace string
}

// Update event filter only accepts Velero DataUpload update events from OADP namespace
// and from Velero DataUpload that have required metadata
func (p VeleroDataUploadPredicate) Update(ctx context.Context, evt event.UpdateEvent) bool {
	logger := function.GetLogger(ctx, evt.ObjectNew, "VeleroDataUploadPredicate")

	namespace := evt.ObjectNew.GetNamespace()
	if namespace == p.OADPNamespace {
		owners := evt.ObjectNew.GetOwnerReferences()
		for _, owner := range owners {
			if owner.Kind == "Backup" && owner.APIVersion == velerov1.SchemeGroupVersion.String() {
				backup := &velerov1.Backup{}
				err := p.Get(ctx, types.NamespacedName{
					Namespace: p.OADPNamespace,
					Name:      owner.Name,
				}, backup)
				if err != nil {
					logger.Error(err, "failed to get DataUpload's Backup in Update event")
					return false
				}
				if function.CheckVeleroBackupMetadata(backup) {
					logger.V(1).Info("Accepted DataUpload Update event")
					return true
				}
			}
		}
	}

	logger.V(1).Info("Rejected DataUpload Update event")
	return false
}
