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

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/migtools/oadp-non-admin/internal/common/function"
)

// NonAdminBslSecretPredicate contains event filters for Velero BackupStorageLocation objects
type NonAdminBslSecretPredicate struct{}

// Create event filter only accepts NonAdminBackupStorageLocation create events
func (NonAdminBslSecretPredicate) Create(ctx context.Context, evt event.CreateEvent) bool {
	logger := function.GetLogger(ctx, evt.Object, "NonAdminBslSecretPredicate")

	secret, ok := evt.Object.(*corev1.Secret)
	if !ok {
		logger.Error(nil, "Failed to cast event object to Secret")
		return false
	}

	if secret.Type == corev1.SecretTypeOpaque {
		logger.V(1).Info("Accepted Create event")
		return true
	}

	logger.V(1).Info("Rejected Create event")
	return false
}
