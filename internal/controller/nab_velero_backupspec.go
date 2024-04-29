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
	"errors"

	"github.com/go-logr/logr"
	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	nacv1alpha1 "github.com/migtools/oadp-non-admin/api/v1alpha1"
	"github.com/migtools/oadp-non-admin/internal/common/constant"
	"github.com/migtools/oadp-non-admin/internal/common/function"
)

// CreateVeleroBackupSpec creates or updates a Velero Backup object based on the provided NonAdminBackup object.
//
// Parameters:
//
//	ctx: Context for the request.
//	log: Logger instance for logging messages.
//	nab: Pointer to the NonAdminBackup object.
//
// The function generates a name for the Velero Backup object based on the provided namespace and name.
// It then checks if a Velero Backup object with that name already exists. If it does not exist, it creates a new one.
// The function returns boolean values indicating whether the reconciliation loop should exit or requeue
func (r *NonAdminBackupReconciler) CreateVeleroBackupSpec(ctx context.Context, log logr.Logger, nab *nacv1alpha1.NonAdminBackup) (exitReconcile bool, requeueReconcile bool, errorReconcile error) {
	logger := log.WithValues("NonAdminBackup", nab.Namespace)

	veleroBackupName := function.GenerateVeleroBackupName(nab.Namespace, nab.Name)

	if veleroBackupName == constant.EmptyString {
		return true, false, errors.New("unable to generate Velero Backup name")
	}

	veleroBackup := velerov1api.Backup{}
	err := r.Get(ctx, client.ObjectKey{Namespace: constant.OadpNamespace, Name: veleroBackupName}, &veleroBackup)

	if err != nil && apierrors.IsNotFound(err) {
		// Create VeleroBackup
		// Don't update phase nor conditions yet.
		// Those will be updated when then Reconcile loop is triggered by the VeleroBackup object
		logger.Info("No backup found", nameField, veleroBackupName)

		// We don't validate error here.
		// This was already validated in the ValidateVeleroBackupSpec
		backupSpec, errBackup := function.GetBackupSpecFromNonAdminBackup(nab)

		if errBackup != nil {
			// Should never happen as it was already checked
			return true, false, errBackup
		}

		veleroBackup = velerov1api.Backup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      veleroBackupName,
				Namespace: constant.OadpNamespace,
			},
			Spec: *backupSpec,
		}
	} else if err != nil && !apierrors.IsNotFound(err) {
		logger.Error(err, "Unable to fetch VeleroBackup")
		return true, false, err
	} else {
		// We should not update already created VeleroBackup object.
		// The VeleroBackup within NonAdminBackup will
		// be reverted back to the previous state - the state which created VeleroBackup
		// in a first place, so they will be in sync.
		logger.Info("Backup already exists, updating NonAdminBackup status", nameField, veleroBackupName)
		updatedNab, errBackupUpdate := function.UpdateNonAdminBackupFromVeleroBackup(ctx, r.Client, logger, nab, &veleroBackup)
		// Regardless if the status was updated or not, we should not
		// requeue here as it was only status update.
		if errBackupUpdate != nil {
			return true, false, errBackupUpdate
		} else if updatedNab {
			logger.V(1).Info("NonAdminBackup CR - Rqueue after Status Update")
			return false, true, nil
		}
		return true, false, nil
	}

	// Ensure labels are set for the Backup object
	existingLabels := veleroBackup.Labels
	naManagedLabels := function.AddNonAdminLabels(existingLabels)
	veleroBackup.Labels = naManagedLabels

	// Ensure annotations are set for the Backup object
	existingAnnotations := veleroBackup.Annotations
	ownerUUID := string(nab.ObjectMeta.UID)
	nabManagedAnnotations := function.AddNonAdminBackupAnnotations(nab.Namespace, nab.Name, ownerUUID, existingAnnotations)
	veleroBackup.Annotations = nabManagedAnnotations

	_, err = controllerutil.CreateOrPatch(ctx, r.Client, &veleroBackup, nil)
	if err != nil {
		logger.Error(err, "Failed to create backup", nameField, veleroBackupName)
		return true, false, err
	}
	logger.Info("VeleroBackup successfully created", nameField, veleroBackupName)

	_, errUpdate := function.UpdateNonAdminPhase(ctx, r.Client, logger, nab, nacv1alpha1.NonAdminBackupPhaseCreated)
	if errUpdate != nil {
		logger.Error(errUpdate, "Unable to set NonAdminBackup Phase: Created", nameField, nab.Name, constant.NameSpaceString, nab.Namespace)
		return true, false, errUpdate
	}
	_, errUpdate = function.UpdateNonAdminBackupCondition(ctx, r.Client, logger, nab, nacv1alpha1.NonAdminConditionAccepted, metav1.ConditionTrue, "Validated", "Valid Backup config")
	if errUpdate != nil {
		logger.Error(errUpdate, "Unable to set BackupAccepted Condition: True", nameField, nab.Name, constant.NameSpaceString, nab.Namespace)
		return true, false, errUpdate
	}
	_, errUpdate = function.UpdateNonAdminBackupCondition(ctx, r.Client, logger, nab, nacv1alpha1.NonAdminConditionQueued, metav1.ConditionTrue, "BackupScheduled", "Created Velero Backup object")
	if errUpdate != nil {
		logger.Error(errUpdate, "Unable to set BackupQueued Condition: True", nameField, nab.Name, constant.NameSpaceString, nab.Namespace)
		return true, false, errUpdate
	}

	return false, false, nil
}
