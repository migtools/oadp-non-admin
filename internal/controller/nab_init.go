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

package controller

import (
	"context"

	"github.com/go-logr/logr"

	nacv1alpha1 "github.com/migtools/oadp-non-admin/api/v1alpha1"
	"github.com/migtools/oadp-non-admin/internal/common/constant"
	"github.com/migtools/oadp-non-admin/internal/common/function"
)

// InitNonAdminBackup Initsets the New Phase on a NonAdminBackup object if it is not already set.
//
// Parameters:
//
//	ctx: Context for the request.
//	log: Logger instance for logging messages.
//	nab: Pointer to the NonAdminBackup object.
//
// The function checks if the Phase of the NonAdminBackup object is empty.
// If it is empty, it sets the Phase to "New".
// It then returns boolean values indicating whether the reconciliation loop should requeue
// and whether the status was updated.
func (r *NonAdminBackupReconciler) InitNonAdminBackup(ctx context.Context, log logr.Logger, nab *nacv1alpha1.NonAdminBackup) (exitReconcile bool, requeueReconcile bool, errorReconcile error) {
	logger := log.WithValues("NonAdminBackup", nab.Namespace)
	// Set initial Phase
	if nab.Status.Phase == constant.EmptyString {
		// Phase: New
		updatedStatus, errUpdate := function.UpdateNonAdminPhase(ctx, r.Client, logger, nab, nacv1alpha1.NonAdminBackupPhaseNew)
		if updatedStatus {
			logger.V(1).Info("NonAdminBackup CR - Rqueue after Phase Update")
			return false, true, nil
		}
		if errUpdate != nil {
			logger.Error(errUpdate, "Unable to set NonAdminBackup Phase: New", nameField, nab.Name, constant.NameSpaceString, nab.Namespace)
			return true, false, errUpdate
		}
	}

	return false, false, nil
}
