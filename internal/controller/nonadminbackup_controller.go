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
	"reflect"

	"github.com/go-logr/logr"
	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	Scheme        *runtime.Scheme
	OADPNamespace string
}

const (
	phaseUpdateRequeue     = "NonAdminBackup - Requeue after Phase Update"
	conditionUpdateRequeue = "NonAdminBackup - Requeue after Condition Update"
	phaseUpdateError       = "Failed to update NonAdminBackup Phase"
	conditionUpdateError   = "Failed to update NonAdminBackup Condition"
)

// +kubebuilder:rbac:groups=nac.oadp.openshift.io,resources=nonadminbackups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nac.oadp.openshift.io,resources=nonadminbackups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=nac.oadp.openshift.io,resources=nonadminbackups/finalizers,verbs=update

// +kubebuilder:rbac:groups=velero.io,resources=backups,verbs=get;list;watch;create;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state,
// defined in NonAdminBackup object Spec.
func (r *NonAdminBackupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.V(1).Info("NonAdminBackup Reconcile start")

	// Get the NonAdminBackup object
	nab := nacv1alpha1.NonAdminBackup{}
	err := r.Get(ctx, req.NamespacedName, &nab)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(1).Info("NonAdminBackup was deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Unable to fetch NonAdminBackup")
		return ctrl.Result{}, err
	}

	reconcileExit, reconcileRequeue, reconcileErr := r.Init(ctx, logger, &nab)
	if reconcileRequeue {
		return ctrl.Result{Requeue: true}, reconcileErr
	} else if reconcileExit && reconcileErr != nil {
		return ctrl.Result{}, reconcileErr
	} else if reconcileExit {
		return ctrl.Result{}, nil
	}

	reconcileExit, reconcileRequeue, reconcileErr = r.ValidateSpec(ctx, logger, &nab)
	if reconcileRequeue {
		return ctrl.Result{Requeue: true}, reconcileErr
	} else if reconcileExit && reconcileErr != nil {
		return ctrl.Result{}, reconcileErr
	} else if reconcileExit {
		return ctrl.Result{}, nil
	}

	reconcileExit, reconcileRequeue, reconcileErr = r.UpdateSpecStatus(ctx, logger, &nab)
	if reconcileRequeue {
		return ctrl.Result{Requeue: true}, reconcileErr
	} else if reconcileExit && reconcileErr != nil {
		return ctrl.Result{}, reconcileErr
	} else if reconcileExit {
		return ctrl.Result{}, nil
	}

	logger.V(1).Info("NonAdminBackup Reconcile exit")
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
	logger := logrLogger

	if nab.Status.Phase == constant.EmptyString {
		updated := updateNonAdminPhase(&nab.Status.Phase, nacv1alpha1.NonAdminBackupPhaseNew)
		if updated {
			if err := r.Status().Update(ctx, nab); err != nil {
				logger.Error(err, phaseUpdateError)
				return true, false, err
			}

			logger.V(1).Info(phaseUpdateRequeue)
			return false, true, nil
		}
	}

	logger.V(1).Info("NonAdminBackup Phase already initialized")
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
// The function validates the Spec from the NonAdminBackup object.
// If the BackupSpec is invalid, the function sets the NonAdminBackup phase to "BackingOff".
// If the BackupSpec is invalid, the function sets the NonAdminBackup condition Accepted to "False".
// If the BackupSpec is valid, the function sets the NonAdminBackup condition Accepted to "True".
func (r *NonAdminBackupReconciler) ValidateSpec(ctx context.Context, logrLogger logr.Logger, nab *nacv1alpha1.NonAdminBackup) (exitReconcile bool, requeueReconcile bool, errorReconcile error) {
	logger := logrLogger

	// Main Validation point for the VeleroBackup included in NonAdminBackup spec
	_, err := function.GetBackupSpecFromNonAdminBackup(nab)
	if err != nil {
		updated := updateNonAdminPhase(&nab.Status.Phase, nacv1alpha1.NonAdminBackupPhaseBackingOff)
		if updated {
			if updateErr := r.Status().Update(ctx, nab); updateErr != nil {
				logger.Error(updateErr, phaseUpdateError)
				return true, false, updateErr
			}

			logger.V(1).Info(phaseUpdateRequeue)
			return false, true, nil
		}

		updated = meta.SetStatusCondition(&nab.Status.Conditions,
			metav1.Condition{
				Type:    string(nacv1alpha1.NonAdminConditionAccepted),
				Status:  metav1.ConditionFalse,
				Reason:  "InvalidBackupSpec",
				Message: err.Error(),
			},
		)
		if updated {
			if updateErr := r.Status().Update(ctx, nab); updateErr != nil {
				logger.Error(updateErr, conditionUpdateError)
				return true, false, updateErr
			}
		}

		logger.Error(err, "NonAdminBackup Spec is not valid")
		return true, false, reconcile.TerminalError(err)
	}

	updated := meta.SetStatusCondition(&nab.Status.Conditions,
		metav1.Condition{
			Type:    string(nacv1alpha1.NonAdminConditionAccepted),
			Status:  metav1.ConditionTrue,
			Reason:  "BackupAccepted",
			Message: "backup accepted",
		},
	)
	if updated {
		if err := r.Status().Update(ctx, nab); err != nil {
			logger.Error(err, conditionUpdateError)
			return true, false, err
		}

		logger.V(1).Info(conditionUpdateRequeue)
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
// and updates NonAdminBackup Status. Otherwise, updates NonAdminBackup VeleroBackup Spec and Status based on Velero Backup object Spec and Status.
// The function returns boolean values indicating whether the reconciliation loop should exit or requeue
func (r *NonAdminBackupReconciler) UpdateSpecStatus(ctx context.Context, logrLogger logr.Logger, nab *nacv1alpha1.NonAdminBackup) (exitReconcile bool, requeueReconcile bool, errorReconcile error) {
	logger := logrLogger

	veleroBackupName := function.GenerateVeleroBackupName(nab.Namespace, nab.Name)
	if veleroBackupName == constant.EmptyString {
		return true, false, errors.New("unable to generate Velero Backup name")
	}

	veleroBackup := velerov1api.Backup{}
	veleroBackupLogger := logger.WithValues("VeleroBackup", types.NamespacedName{Name: veleroBackupName, Namespace: r.OADPNamespace})
	err := r.Get(ctx, client.ObjectKey{Namespace: r.OADPNamespace, Name: veleroBackupName}, &veleroBackup)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			veleroBackupLogger.Error(err, "Unable to fetch VeleroBackup")
			return true, false, err
		}
		// Create VeleroBackup
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
				Namespace: r.OADPNamespace,
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

		err = r.Create(ctx, &veleroBackup)
		if err != nil {
			veleroBackupLogger.Error(err, "Failed to create VeleroBackup")
			return true, false, err
		}
		veleroBackupLogger.Info("VeleroBackup successfully created")
	}

	updated := updateNonAdminPhase(&nab.Status.Phase, nacv1alpha1.NonAdminBackupPhaseCreated)
	if updated {
		if err := r.Status().Update(ctx, nab); err != nil {
			logger.Error(err, phaseUpdateError)
			return true, false, err
		}

		logger.V(1).Info(phaseUpdateRequeue)
		return false, true, nil
	}

	updated = meta.SetStatusCondition(&nab.Status.Conditions,
		metav1.Condition{
			Type:    string(nacv1alpha1.NonAdminConditionQueued),
			Status:  metav1.ConditionTrue,
			Reason:  "BackupScheduled",
			Message: "Created Velero Backup object",
		},
	)
	if updated {
		if err := r.Status().Update(ctx, nab); err != nil {
			logger.Error(err, conditionUpdateError)
			return true, false, err
		}

		logger.V(1).Info(conditionUpdateRequeue)
		return false, true, nil
	}

	// We should not update already created VeleroBackup object.
	// The VeleroBackup within NonAdminBackup will
	// be reverted back to the previous state - the state which created VeleroBackup
	// in a first place, so they will be in sync.
	veleroBackupLogger.Info("VeleroBackup already exists, checking if NonAdminBackup VeleroBackupSpec and VeleroBackupStatus needs update")
	updated = updateNonAdminBackupVeleroBackupStatus(&nab.Status, &veleroBackup)
	if updated {
		if err := r.Status().Update(ctx, nab); err != nil {
			veleroBackupLogger.Error(err, "NonAdminBackup BackupStatus - Failed to update")
			return true, false, err
		}

		logger.V(1).Info("NonAdminBackup - Requeue after Status Update")
		return false, true, nil
	}
	updated = updateNonAdminBackupVeleroBackupSpec(&nab.Spec, &veleroBackup)
	if updated {
		if err := r.Update(ctx, nab); err != nil {
			veleroBackupLogger.Error(err, "NonAdminBackup BackupSpec - Failed to update")
			return true, false, err
		}

		logger.V(1).Info("NonAdminBackup - Requeue after Spec Update")
		return false, true, nil
	}

	logger.V(1).Info("NonAdminBackup VeleroBackupSpec and VeleroBackupStatus already up to date")
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
				OadpVeleroNamespace: r.OADPNamespace,
			},
		}).
		Complete(r)
}

// updateNonAdminPhase sets the phase in NonAdminBackup object status and returns true
// if the phase is changed by this call.
func updateNonAdminPhase(phase *nacv1alpha1.NonAdminBackupPhase, newPhase nacv1alpha1.NonAdminBackupPhase) bool {
	// Ensure phase is valid
	if newPhase == constant.EmptyString {
		return false
	}

	if *phase == newPhase {
		return false
	}

	*phase = newPhase
	return true
}

// updateNonAdminBackupVeleroBackupStatus sets the VeleroBackup fields in NonAdminBackup object status and returns true
// if the VeleroBackup fields are changed by this call.
func updateNonAdminBackupVeleroBackupStatus(status *nacv1alpha1.NonAdminBackupStatus, veleroBackup *velerov1api.Backup) bool {
	if !reflect.DeepEqual(status.VeleroBackupStatus, &veleroBackup.Status) || status.VeleroBackupName != veleroBackup.Name || status.VeleroBackupNamespace != veleroBackup.Namespace {
		status.VeleroBackupStatus = veleroBackup.Status.DeepCopy()
		status.VeleroBackupName = veleroBackup.Name
		status.VeleroBackupNamespace = veleroBackup.Namespace
		return true
	}
	return false
}

// updateNonAdminBackupVeleroBackupSpec sets the BackupSpec in NonAdminBackup object spec and returns true
// if the BackupSpec is changed by this call.
func updateNonAdminBackupVeleroBackupSpec(spec *nacv1alpha1.NonAdminBackupSpec, veleroBackup *velerov1api.Backup) bool {
	if !reflect.DeepEqual(spec.BackupSpec, &veleroBackup.Spec) {
		spec.BackupSpec = veleroBackup.Spec.DeepCopy()
		return true
	}
	return false
}
