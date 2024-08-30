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
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	nacv1alpha1 "github.com/migtools/oadp-non-admin/api/v1alpha1"
	"github.com/migtools/oadp-non-admin/internal/common/constant"
	"github.com/migtools/oadp-non-admin/internal/common/function"
	"github.com/migtools/oadp-non-admin/internal/handler"
	"github.com/migtools/oadp-non-admin/internal/predicate"
)

// NonAdminBackupReconciler reconciles a NonAdminBackup object
type NonAdminBackupReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	// needed???
	Context context.Context
}

// TODO TOO MUCH!!!!!!!!!!!!!!!
const requeueTimeSeconds = 10

// +kubebuilder:rbac:groups=nac.oadp.openshift.io,resources=nonadminbackups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nac.oadp.openshift.io,resources=nonadminbackups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=nac.oadp.openshift.io,resources=nonadminbackups/finalizers,verbs=update

// +kubebuilder:rbac:groups=velero.io,resources=backups,verbs=get;list;watch;create;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the NonAdminBackup to the desired state.
func (r *NonAdminBackupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	rLog := log.FromContext(ctx)
	logger := rLog.WithValues("NonAdminBackup", req.NamespacedName)
	logger.V(1).Info("NonAdminBackup Reconcile start")

	// Get the NonAdminBackup object
	nab := nacv1alpha1.NonAdminBackup{}
	err := r.Get(ctx, req.NamespacedName, &nab)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Delete event triggered this reconcile
			logger.V(1).Info("Non existing NonAdminBackup")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Unable to fetch NonAdminBackup")
		return ctrl.Result{}, err
	}

	// requeue on every change is the correct pattern! document this
	// TODO refactor idea: do not enter on sub functions again
	// TODO refactor idea: sub functions can not exit clean, that should be main func responsibility. Remove reconcileExit return param
	// TODO refactor idea:
	// requeue, err := r.Init(ctx, rLog, &nab)
	// if err != nil {
	// 	// handle err smart way to retry when wanted?
	// 	return ctrl.Result{}, reconcile.TerminalError(err)
	// }
	// if requeue {
	// 	return ctrl.Result{Requeue: true}, nil
	// }
	// SOURCE https://github.com/kubernetes-sigs/controller-runtime/blob/e6c3d139d2b6c286b1dbba6b6a95919159cfe655/pkg/internal/controller/controller.go#L286
	// Alright, after studies, I believe there are only 2 possibilities (DEV eyes):
	// - re trigger reconcile
	// 		AddRateLimited ([requeue and nill error] or [normal error])
	// 			will re trigger reconcile immediately, after 1 second, after 2 seconds, etc
	//	 	AddAfter ([RequeueAfter and nill error])
	// 			will re trigger reconcile after time
	// - will not re trigger reconcile
	// 		Forget (finish process) ([empty result and nill error] or [terminal error])

	reconcileExit, reconcileRequeue, reconcileErr := r.Init(ctx, rLog, &nab)
	if reconcileRequeue {
		// TODO EITHER Requeue or RequeueAfter, both together do not make sense!!!
		return ctrl.Result{Requeue: true, RequeueAfter: requeueTimeSeconds * time.Second}, reconcileErr
	} else if reconcileExit && reconcileErr != nil {
		return ctrl.Result{}, reconcile.TerminalError(reconcileErr)
	} else if reconcileExit {
		return ctrl.Result{}, nil
	}

	// would not be better to validate first?
	reconcileExit, reconcileRequeue, reconcileErr = r.ValidateSpec(ctx, rLog, &nab)
	if reconcileRequeue {
		return ctrl.Result{Requeue: true, RequeueAfter: requeueTimeSeconds * time.Second}, reconcileErr
	} else if reconcileExit && reconcileErr != nil {
		return ctrl.Result{}, reconcile.TerminalError(reconcileErr)
	} else if reconcileExit {
		return ctrl.Result{}, nil
	}

	reconcileExit, reconcileRequeue, reconcileErr = r.UpdateSpecStatus(ctx, rLog, &nab)
	if reconcileRequeue {
		return ctrl.Result{Requeue: true, RequeueAfter: requeueTimeSeconds * time.Second}, reconcileErr
	} else if reconcileExit && reconcileErr != nil {
		return ctrl.Result{}, reconcile.TerminalError(reconcileErr)
	} else if reconcileExit {
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
}

// Init initializes the Status.Phase from the NonAdminBackup.
//
// Parameters:
//
//	ctx: Context for the request.
//	logrLogger: Logger instance for logging messages.
//	nab: Pointer to the NonAdminBackup object.
//
// The function checks if the Phase of the NonAdminBackup object is empty.
// If it is empty, it sets the Phase to "New".
// It then returns boolean values indicating whether the reconciliation loop should requeue or exit
// and error value whether the status was updated successfully.
func (r *NonAdminBackupReconciler) Init(ctx context.Context, logrLogger logr.Logger, nab *nacv1alpha1.NonAdminBackup) (exitReconcile bool, requeueReconcile bool, errorReconcile error) {
	logger := logrLogger.WithValues("Init NonAdminBackup", types.NamespacedName{Name: nab.Name, Namespace: nab.Namespace})

	if nab.Status.Phase == constant.EmptyString {
		// // Set initial Phase to New
		updatedStatus, errUpdate := function.UpdateNonAdminPhase(ctx, r.Client, logger, nab, nacv1alpha1.NonAdminBackupPhaseNew)
		if errUpdate != nil {
			logger.Error(errUpdate, "Unable to set NonAdminBackup Phase: New")
			return true, false, errUpdate
		}
		if updatedStatus {
			logger.V(1).Info("NonAdminBackup - Requeue after Phase Update")
			return false, true, nil
		}
	}

	logger.V(1).Info("NonAdminBackup Status.Phase already initialized")
	return false, false, nil
}

// ValidateSpec validates the Spec from the NonAdminBackup.
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
// If the BackupSpec is invalid, the function sets the NonAdminBackup condition to "InvalidBackupSpec". THIS DOES NOT HAPPEN
// If the BackupSpec is valid, the function sets the NonAdminBackup condition to "BackupAccepted". remove?
func (r *NonAdminBackupReconciler) ValidateSpec(ctx context.Context, logrLogger logr.Logger, nab *nacv1alpha1.NonAdminBackup) (exitReconcile bool, requeueReconcile bool, errorReconcile error) {
	logger := logrLogger.WithValues("ValidateSpec NonAdminBackup", types.NamespacedName{Name: nab.Name, Namespace: nab.Namespace})

	// Main Validation point for the VeleroBackup included in NonAdminBackup spec
	_, err := function.GetBackupSpecFromNonAdminBackup(nab)

	if err != nil {
		logger.Error(err, "NonAdminBackup Spec is not valid")

		// this should be one call: update both phase and condition at THE SAME TIME
		// OR do requeue, CONDITION is never set to false
		updatedStatus, errUpdateStatus := function.UpdateNonAdminPhase(ctx, r.Client, logger, nab, nacv1alpha1.NonAdminBackupPhaseBackingOff)
		if errUpdateStatus != nil {
			logger.Error(errUpdateStatus, "Unable to set NonAdminBackup Phase: BackingOff")
			return true, false, errUpdateStatus
		} else if updatedStatus {
			// We do not requeue - the State was set to BackingOff
			return true, false, nil
		}

		// Continue. VeleroBackup looks fine, setting Accepted condition to false
		updatedCondition, errUpdateCondition := function.UpdateNonAdminBackupCondition(ctx, r.Client, logger, nab, nacv1alpha1.NonAdminConditionAccepted, metav1.ConditionFalse, "InvalidBackupSpec", "NonAdminBackup does not contain valid BackupSpec")
		if errUpdateCondition != nil {
			logger.Error(errUpdateCondition, "Unable to set BackupAccepted Condition: False")
			return true, false, errUpdateCondition
		} else if updatedCondition {
			return true, false, nil
		}

		// We do not requeue - this was an error from getting Spec from NAB
		return true, false, err
	}

	// TODO is this needed? from design, does not seem a valid condition
	// this keeps being called...
	// this or UpdateNonAdminBackupCondition(..., "BackupAccepted", "Backup accepted") should be deleted
	updatedStatus, errUpdateStatus := function.UpdateNonAdminBackupCondition(ctx, r.Client, logger, nab, nacv1alpha1.NonAdminConditionAccepted, metav1.ConditionTrue, "Validated", "Valid Backup config")
	if errUpdateStatus != nil {
		logger.Error(errUpdateStatus, "Unable to set BackupAccepted Condition: True")
		return true, false, errUpdateStatus
	} else if updatedStatus {
		// We do requeue - The VeleroBackup got validated and next reconcile loop will continue
		// with further work on the VeleroBackup such as creating it
		return false, true, nil
	}

	logger.V(1).Info("NonAdminBackup Spec already validated")
	return false, false, nil
}

// UpdateSpecStatus updates the Spec and Status from the NonAdminBackup.
//
// Parameters:
//
//	ctx: Context for the request.
//	log: Logger instance for logging messages.
//	nab: Pointer to the NonAdminBackup object.
//
// The function generates the name for the Velero Backup object based on the provided namespace and name.
// It then checks if a Velero Backup object with that name already exists. If it does not exist, it creates a new one
// and updates NonAdminBackup Status. Otherwise, updates NonAdminBackup VeleroBackup Status based on Velero Backup object Status.
// The function returns boolean values indicating whether the reconciliation loop should exit or requeue
func (r *NonAdminBackupReconciler) UpdateSpecStatus(ctx context.Context, logrLogger logr.Logger, nab *nacv1alpha1.NonAdminBackup) (exitReconcile bool, requeueReconcile bool, errorReconcile error) {
	logger := logrLogger.WithValues("UpdateSpecStatus NonAdminBackup", types.NamespacedName{Name: nab.Name, Namespace: nab.Namespace})

	veleroBackupName := function.GenerateVeleroBackupName(nab.Namespace, nab.Name)

	if veleroBackupName == constant.EmptyString {
		return true, false, errors.New("unable to generate Velero Backup name")
	}

	oadpNamespace := function.GetOADPNamespace()
	veleroBackup := velerov1api.Backup{}
	veleroBackupLogger := logger.WithValues("VeleroBackup", types.NamespacedName{Name: veleroBackupName, Namespace: oadpNamespace})
	err := r.Get(ctx, client.ObjectKey{Namespace: oadpNamespace, Name: veleroBackupName}, &veleroBackup)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			veleroBackupLogger.Error(err, "Unable to fetch VeleroBackup")
			return true, false, err
		}
		// Create VeleroBackup
		// Don't update phase nor conditions yet.
		// Those will be updated when then Reconcile loop is triggered by the VeleroBackup object
		veleroBackupLogger.Info("VeleroBackup not found")

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
				Namespace: oadpNamespace,
			},
			Spec: *backupSpec,
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
			veleroBackupLogger.Error(err, "Failed to create VeleroBackup")
			return true, false, err
		}
		veleroBackupLogger.Info("VeleroBackup successfully created")

		_, errUpdate := function.UpdateNonAdminPhase(ctx, r.Client, logger, nab, nacv1alpha1.NonAdminBackupPhaseCreated)
		if errUpdate != nil {
			logger.Error(errUpdate, "Unable to set NonAdminBackup Phase: Created")
			return true, false, errUpdate
		}
		_, errUpdate = function.UpdateNonAdminBackupCondition(ctx, r.Client, logger, nab, nacv1alpha1.NonAdminConditionAccepted, metav1.ConditionTrue, "BackupAccepted", "Backup accepted")
		if errUpdate != nil {
			logger.Error(errUpdate, "Unable to set BackupAccepted Condition: True")
			return true, false, errUpdate
		}
		_, errUpdate = function.UpdateNonAdminBackupCondition(ctx, r.Client, logger, nab, nacv1alpha1.NonAdminConditionQueued, metav1.ConditionTrue, "BackupScheduled", "Created Velero Backup object")
		if errUpdate != nil {
			logger.Error(errUpdate, "Unable to set BackupQueued Condition: True")
			return true, false, errUpdate
		}

		return false, false, nil
	}
	// We should not update already created VeleroBackup object.
	// The VeleroBackup within NonAdminBackup will
	// be reverted back to the previous state - the state which created VeleroBackup
	// in a first place, so they will be in sync.
	veleroBackupLogger.Info("VeleroBackup already exists, updating NonAdminBackup status")
	updatedNab, errBackupUpdate := function.UpdateNonAdminBackupFromVeleroBackup(ctx, r.Client, logger, nab, &veleroBackup)
	// Regardless if the status was updated or not, we should not
	// requeue here as it was only status update.
	if errBackupUpdate != nil {
		return true, false, errBackupUpdate
	} else if updatedNab {
		logger.V(1).Info("NonAdminBackup - Requeue after Status Update")
		return false, true, nil
	}
	return true, false, nil
}

// TODO refactor idea: break in smaller functions: CreateVeleroBackup, UpdateStatusAfterVeleroBackupCreation and UpdateSpecStatus

// SetupWithManager sets up the controller with the Manager.
func (r *NonAdminBackupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nacv1alpha1.NonAdminBackup{}).
		Watches(&velerov1api.Backup{}, &handler.VeleroBackupHandler{}).
		WithEventFilter(predicate.CompositePredicate{
			NonAdminBackupPredicate: predicate.NonAdminBackupPredicate{},
			VeleroBackupPredicate: predicate.VeleroBackupPredicate{
				OadpVeleroNamespace: function.GetOADPNamespace(),
			},
			Context: r.Context,
		}).
		Complete(r)
}
