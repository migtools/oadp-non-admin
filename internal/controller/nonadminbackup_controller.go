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

// Package controller contains all controllers of the project
package controller

import (
	"context"
	"errors"
	"time"

	"github.com/go-logr/logr"
	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	nacv1alpha1 "github.com/migtools/oadp-non-admin/api/v1alpha1"
	"github.com/migtools/oadp-non-admin/internal/common/constant"
	"github.com/migtools/oadp-non-admin/internal/common/function"
	"github.com/migtools/oadp-non-admin/internal/handler"
	"github.com/migtools/oadp-non-admin/internal/predicate"
)

// NonAdminBackupReconciler reconciles a NonAdminBackup object
type NonAdminBackupReconciler struct {
	client.Client
	Scheme  *runtime.Scheme
	Context context.Context
}

const (
	nameField          = "Name"
	requeueTimeSeconds = 10
)

// +kubebuilder:rbac:groups=nac.oadp.openshift.io,resources=nonadminbackups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nac.oadp.openshift.io,resources=nonadminbackups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=nac.oadp.openshift.io,resources=nonadminbackups/finalizers,verbs=update

// +kubebuilder:rbac:groups=velero.io,resources=backups,verbs=get;list;watch;create;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the NonAdminBackup object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.0/pkg/reconcile
func (r *NonAdminBackupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	rLog := log.FromContext(ctx)
	logger := rLog.WithValues("NonAdminBackup", req.NamespacedName)
	logger.V(1).Info(">>> Reconcile NonAdminBackup - loop start")

	// Get the NonAdminBackup object
	nab := nacv1alpha1.NonAdminBackup{}
	err := r.Get(ctx, req.NamespacedName, &nab)

	// Bail out when the Non Admin Backup reconcile was triggered, when the NAB got deleted
	// Reconcile loop was triggered when Velero Backup object got updated and NAB isn't there
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(1).Info("Non existing NonAdminBackup CR", nameField, req.Name, constant.NameSpaceString, req.Namespace)
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Unable to fetch NonAdminBackup CR", nameField, req.Name, constant.NameSpaceString, req.Namespace)
		return ctrl.Result{}, err
	}

	reconcileExit, reconcileRequeue, reconcileErr := r.InitNonAdminBackup(ctx, rLog, &nab)
	if reconcileRequeue {
		return ctrl.Result{Requeue: true, RequeueAfter: requeueTimeSeconds * time.Second}, reconcileErr
	} else if reconcileExit || reconcileErr != nil {
		return ctrl.Result{}, reconcileErr
	}

	reconcileExit, reconcileRequeue, reconcileErr = r.ValidateVeleroBackupSpec(ctx, rLog, &nab)
	if reconcileRequeue {
		return ctrl.Result{Requeue: true, RequeueAfter: requeueTimeSeconds * time.Second}, reconcileErr
	} else if reconcileExit || reconcileErr != nil {
		return ctrl.Result{}, reconcileErr
	}

	reconcileExit, reconcileRequeue, reconcileErr = r.CreateVeleroBackupSpec(ctx, rLog, &nab)
	if reconcileRequeue {
		return ctrl.Result{Requeue: true, RequeueAfter: requeueTimeSeconds * time.Second}, reconcileErr
	} else if reconcileExit || reconcileErr != nil {
		return ctrl.Result{}, reconcileErr
	}

	logger.V(1).Info(">>> Reconcile NonAdminBackup - loop end")
	return ctrl.Result{}, nil
}

// InitNonAdminBackup sets the New Phase on a NonAdminBackup object if it is not already set.
//
// Parameters:
//
//	ctx: Context for the request.
//	logrLogger: Logger instance for logging messages.
//	nab: Pointer to the NonAdminBackup object.
//
// The function checks if the Phase of the NonAdminBackup object is empty.
// If it is empty, it sets the Phase to "New".
// It then returns boolean values indicating whether the reconciliation loop should requeue
// and whether the status was updated.
func (r *NonAdminBackupReconciler) InitNonAdminBackup(ctx context.Context, logrLogger logr.Logger, nab *nacv1alpha1.NonAdminBackup) (exitReconcile bool, requeueReconcile bool, errorReconcile error) {
	logger := logrLogger.WithValues("InitNonAdminBackup", nab.Namespace)
	// Set initial Phase
	if nab.Status.Phase == constant.EmptyString {
		// Phase: New
		updatedStatus, errUpdate := function.UpdateNonAdminPhase(ctx, r.Client, logger, nab, nacv1alpha1.NonAdminBackupPhaseNew)

		if errUpdate != nil {
			logger.Error(errUpdate, "Unable to set NonAdminBackup Phase: New", nameField, nab.Name, constant.NameSpaceString, nab.Namespace)
			return true, false, errUpdate
		}

		if updatedStatus {
			logger.V(1).Info("NonAdminBackup CR - Requeue after Phase Update")
			return false, true, nil
		}
	}

	return false, false, nil
}

// ValidateVeleroBackupSpec validates the VeleroBackup Spec from the NonAdminBackup.
//
// Parameters:
//
//	ctx: Context for the request.
//	logrLogger: Logger instance for logging messages.
//	nab: Pointer to the NonAdminBackup object.
//
// The function attempts to get the BackupSpec from the NonAdminBackup object.
// If an error occurs during this process, the function sets the NonAdminBackup status to "BackingOff"
// and updates the corresponding condition accordingly.
// If the BackupSpec is invalid, the function sets the NonAdminBackup condition to "InvalidBackupSpec".
// If the BackupSpec is valid, the function sets the NonAdminBackup condition to "BackupAccepted".
func (r *NonAdminBackupReconciler) ValidateVeleroBackupSpec(ctx context.Context, logrLogger logr.Logger, nab *nacv1alpha1.NonAdminBackup) (exitReconcile bool, requeueReconcile bool, errorReconcile error) {
	logger := logrLogger.WithValues("ValidateVeleroBackupSpec", nab.Namespace)

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

		if errUpdateCondition != nil {
			logger.Error(errUpdateCondition, "Unable to set BackupAccepted Condition: False", nameField, nab.Name, constant.NameSpaceString, nab.Namespace)
			return true, false, errUpdateCondition
		} else if updatedCondition {
			return true, false, nil
		}

		// We do not requeue - this was an error from getting Spec from NAB
		return true, false, err
	}

	updatedStatus, errUpdateStatus := function.UpdateNonAdminBackupCondition(ctx, r.Client, logger, nab, nacv1alpha1.NonAdminConditionAccepted, metav1.ConditionTrue, "BackupAccepted", "backup accepted")
	if errUpdateStatus != nil {
		logger.Error(errUpdateStatus, "Unable to set BackupAccepted Condition: True", nameField, nab.Name, constant.NameSpaceString, nab.Namespace)
		return true, false, errUpdateStatus
	} else if updatedStatus {
		// We do requeue - The VeleroBackup got accepted and next reconcile loop will continue
		// with further work on the VeleroBackup such as creating it
		return false, true, nil
	}

	return false, false, nil
}

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
func (r *NonAdminBackupReconciler) CreateVeleroBackupSpec(ctx context.Context, logrLogger logr.Logger, nab *nacv1alpha1.NonAdminBackup) (exitReconcile bool, requeueReconcile bool, errorReconcile error) {
	logger := logrLogger.WithValues("CreateVeleroBackupSpec", nab.Namespace)

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

// SetupWithManager sets up the controller with the Manager.
func (r *NonAdminBackupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nacv1alpha1.NonAdminBackup{}).
		Watches(&velerov1api.Backup{}, &handler.VeleroBackupHandler{}).
		WithEventFilter(predicate.CompositePredicate{
			NonAdminBackupPredicate: predicate.NonAdminBackupPredicate{},
			VeleroBackupPredicate: predicate.VeleroBackupPredicate{
				OadpVeleroNamespace: constant.OadpNamespace,
			},
			Context: r.Context,
		}).
		Complete(r)
}
