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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	nacv1alpha1 "github.com/migtools/oadp-non-admin/api/v1alpha1"
	"github.com/migtools/oadp-non-admin/internal/common/constant"
	"github.com/migtools/oadp-non-admin/internal/common/function"
)

// ValidateVeleroBackupSpec validates the VeleroBackup Spec from the NonAdminBackup.
//
// Parameters:
//
//	ctx: Context for the request.
//	log: Logger instance for logging messages.
//	nab: Pointer to the NonAdminBackup object.
//
// The function attempts to get the BackupSpec from the NonAdminBackup object.
// If an error occurs during this process, the function sets the NonAdminBackup status to "BackingOff"
// and updates the corresponding condition accordingly.
// If the BackupSpec is invalid, the function sets the NonAdminBackup condition to "InvalidBackupSpec".
// If the BackupSpec is valid, the function sets the NonAdminBackup condition to "BackupAccepted".
func (r *NonAdminBackupReconciler) ValidateVeleroBackupSpec(ctx context.Context, log logr.Logger, nab *nacv1alpha1.NonAdminBackup) (exitReconcile bool, requeueReconcile bool, errorReconcile error) {
	logger := log.WithValues("NonAdminBackup", nab.Namespace)

	// Main Validation point for the VeleroBackup included in NonAdminBackup spec
	_, err := function.GetBackupSpecFromNonAdminBackup(nab)

	if err != nil {
		errMsg := "NonAdminBackup CR does not contain valid BackupSpec"
		logger.Error(err, errMsg)

		updatedStatus, errUpdateStatus := function.UpdateNonAdminPhase(ctx, r.Client, logger, nab, nacv1alpha1.NonAdminBackupPhaseBackingOff)
		if errUpdateStatus != nil {
			logger.Error(errUpdateStatus, "Unable to set NonAdminBackup Phase: BackingOff", nameField, nab.Name, constant.NameSpaceString, nab.Namespace)
			return true, false, errUpdateStatus
		} else if updatedStatus {
			// We do not requeue - the State was set to BackingOff
			return true, false, nil
		}

		// Continue. VeleroBackup looks fine, setting Accepted condition
		updatedCondition, errUpdateCondition := function.UpdateNonAdminBackupCondition(ctx, r.Client, logger, nab, nacv1alpha1.NonAdminConditionAccepted, metav1.ConditionFalse, "InvalidBackupSpec", errMsg)
		if updatedCondition {
			// We do not requeue - this was only Condition update
			return true, false, nil
		}

		if errUpdateCondition != nil {
			logger.Error(errUpdateCondition, "Unable to set BackupAccepted Condition: False", nameField, nab.Name, constant.NameSpaceString, nab.Namespace)
			return true, false, errUpdateCondition
		}
		// We do not requeue - this was error from getting Spec from NAB
		return true, false, err
	}

	updatedStatus, errUpdateStatus := function.UpdateNonAdminBackupCondition(ctx, r.Client, logger, nab, nacv1alpha1.NonAdminConditionAccepted, metav1.ConditionTrue, "BackupAccepted", "backup accepted")
	if updatedStatus {
		// We do requeue - The VeleroBackup got accepted and next reconcile loop will continue
		// with further work on the VeleroBackup such as creating it
		return false, true, nil
	}

	if errUpdateStatus != nil {
		logger.Error(errUpdateStatus, "Unable to set BackupAccepted Condition: True", nameField, nab.Name, constant.NameSpaceString, nab.Namespace)
		return true, false, errUpdateStatus
	}

	return false, false, nil
}
