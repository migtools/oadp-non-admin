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
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"github.com/vmware-tanzu/velero/pkg/builder"
	veleroclient "github.com/vmware-tanzu/velero/pkg/client"
	"github.com/vmware-tanzu/velero/pkg/label"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
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
	Scheme        *runtime.Scheme
	OADPNamespace string
}

type reconcileStepFunction func(ctx context.Context, logger logr.Logger, nab *nacv1alpha1.NonAdminBackup) (bool, error)

const (
	veleroReferenceUpdated = "NonAdminBackup - Status Updated with UUID reference"
	statusUpdateExit       = "NonAdminBackup - Exit after Status Update"
	statusUpdateError      = "Failed to update NonAdminBackup Status"
	findSingleVBError      = "Error encountered while retrieving VeleroBackup for NAB during the Delete operation"
	findSingleVDBRError    = "Error encountered while retrieving DeleteBackupRequest for NAB during the Delete operation"
	uuidString             = "UUID"
	nameString             = "name"
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
	nab := &nacv1alpha1.NonAdminBackup{}
	err := r.Get(ctx, req.NamespacedName, nab)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(1).Info(err.Error())
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Unable to fetch NonAdminBackup")
		return ctrl.Result{}, err
	}

	// Determine which path to take
	var reconcileSteps []reconcileStepFunction

	// First switch statement takes precedence over the next one
	switch {
	case nab.Spec.ForceDeleteBackup:
		// Force delete path - immediately removes both VeleroBackup and DeleteBackupRequest
		logger.V(1).Info("Executing force delete path")
		reconcileSteps = []reconcileStepFunction{
			r.setStatusAndConditionForDeletionAndCallDelete,
			r.deleteVeleroBackupAndDeleteBackupRequestObjects,
			r.removeNabFinalizerUponVeleroBackupDeletion,
		}

	case nab.Spec.DeleteBackup:
		// Standard delete path - creates DeleteBackupRequest and waits for VeleroBackup deletion
		logger.V(1).Info("Executing standard delete path")
		reconcileSteps = []reconcileStepFunction{
			r.setStatusAndConditionForDeletionAndCallDelete,
			r.createVeleroDeleteBackupRequest,
			r.removeNabFinalizerUponVeleroBackupDeletion,
		}

	case !nab.ObjectMeta.DeletionTimestamp.IsZero():
		// Direct deletion path - sets status and condition
		// Initializes deletion of the NonAdminBackup object without removing
		// dependent VeleroBackup object
		logger.V(1).Info("Executing direct deletion path")
		reconcileSteps = []reconcileStepFunction{
			r.setStatusForDirectKubernetesAPIDeletion,
		}

	default:
		// Standard creation/update path
		logger.V(1).Info("Executing nab creation/update path")
		reconcileSteps = []reconcileStepFunction{
			r.initNabCreate,
			r.validateSpec,
			r.setBackupUUIDInStatus,
			r.setFinalizerOnNonAdminBackup,
			r.createVeleroBackupAndSyncWithNonAdminBackup,
		}
	}

	// Execute the selected reconciliation steps
	for _, step := range reconcileSteps {
		requeue, err := step(ctx, logger, nab)
		if err != nil {
			return ctrl.Result{}, err
		} else if requeue {
			return ctrl.Result{Requeue: true}, nil
		}
	}

	logger.V(1).Info("NonAdminBackup Reconcile exit")
	return ctrl.Result{}, nil
}

// setStatusAndConditionForDeletionAndCallDelete updates the NonAdminBackup status and conditions
// to reflect that deletion has been initiated, and triggers the actual deletion if needed.
//
// Parameters:
//   - ctx: Context for managing request lifetime.
//   - logger: Logger instance for logging messages.
//   - nab: Pointer to the NonAdminBackup object being processed.
//
// Returns:
//   - bool: true if reconciliation should be requeued, false otherwise
//   - error: any error encountered during the process
func (r *NonAdminBackupReconciler) setStatusAndConditionForDeletionAndCallDelete(ctx context.Context, logger logr.Logger, nab *nacv1alpha1.NonAdminBackup) (bool, error) {
	requeueRequired := false
	updatedPhase := updateNonAdminPhase(&nab.Status.Phase, nacv1alpha1.NonAdminBackupPhaseDeleting)
	updatedCondition := meta.SetStatusCondition(&nab.Status.Conditions,
		metav1.Condition{
			Type:    string(nacv1alpha1.NonAdminConditionDeleting),
			Status:  metav1.ConditionTrue,
			Reason:  "DeletionPending",
			Message: "backup accepted for deletion",
		},
	)
	if updatedPhase || updatedCondition {
		if err := r.Status().Update(ctx, nab); err != nil {
			logger.Error(err, statusUpdateError)
			return false, err
		}
		logger.V(1).Info("NonAdminBackup status marked for deletion")
		requeueRequired = true // Requeue to ensure latest NAB object in the next reconciliation steps
	} else {
		logger.V(1).Info("NonAdminBackup status unchanged during deletion")
	}
	if nab.ObjectMeta.DeletionTimestamp.IsZero() {
		logger.V(1).Info("Marking NonAdminBackup for deletion", nameString, nab.Name)
		if err := r.Delete(ctx, nab); err != nil {
			logger.Error(err, "Failed to call Delete on the NonAdminBackup object")
			return false, err
		}
		requeueRequired = true // Requeue to allow deletion to proceed
	}
	return requeueRequired, nil
}

// setStatusForDirectKubernetesAPIDeletion updates the status and conditions when a NonAdminBackup
// is deleted directly through the Kubernetes API. Only updates status and conditions
// if the NAB finalizer exists.
//
// Parameters:
//   - ctx: Context for managing request lifetime
//   - logger: Logger instance
//   - nab: NonAdminBackup being deleted
//
// Returns:
//   - bool: true if status was updated
//   - error: any error encountered
func (r *NonAdminBackupReconciler) setStatusForDirectKubernetesAPIDeletion(ctx context.Context, logger logr.Logger, nab *nacv1alpha1.NonAdminBackup) (bool, error) {
	// We don't need to check here if the finalizer exists as we already checked if !nab.ObjectMeta.DeletionTimestamp.IsZero()
	// which means that something prevented the NAB object from being deleted
	updatedPhase := updateNonAdminPhase(&nab.Status.Phase, nacv1alpha1.NonAdminBackupPhaseDeleting)
	updatedCondition := meta.SetStatusCondition(&nab.Status.Conditions,
		metav1.Condition{
			Type:    string(nacv1alpha1.NonAdminConditionDeleting),
			Status:  metav1.ConditionTrue,
			Reason:  "DeletionPending",
			Message: "backup deletion requires VeleroBackup deletion or finalizer removal",
		},
	)
	if updatedPhase || updatedCondition {
		if err := r.Status().Update(ctx, nab); err != nil {
			logger.Error(err, statusUpdateError)
			return false, err
		}
		logger.V(1).Info("NonAdminBackup status marked for deletion during direct API deletion")
		// This is final step in the direct API deletion path we do not want to requeue
	} else {
		logger.V(1).Info("NonAdminBackup status unchanged during direct API deletion")
	}
	return false, nil
}

// createVeleroDeleteBackupRequest initiates deletion of the associated VeleroBackup object
// that is referenced by the NACUUID within the NonAdminBackup (NAB) object.
// This ensures the VeleroBackup is deleted before the NAB object itself is removed.
//
// Parameters:
//   - ctx: Context to manage request lifetime.
//   - logger: Logger instance for logging messages.
//   - nab: Pointer to the NonAdminBackup object to be managed.
//
// This function first checks if the NAB object has the finalizer. If yes, it attempts to locate the associated
// VeleroBackup object by the UUID stored in the NAB status. If a unique VeleroBackup object is found,
// deletion is initiated, and the reconcile loop will be requeued. If multiple VeleroBackup objects
// are found with the same label, the function logs an error and returns.
//
// Returns:
//   - A boolean indicating whether to requeue the reconcile loop,
//   - An error if VeleroBackup deletion or retrieval fails, for example, due to multiple VeleroBackup objects found with the same UUID label.
func (r *NonAdminBackupReconciler) createVeleroDeleteBackupRequest(ctx context.Context, logger logr.Logger, nab *nacv1alpha1.NonAdminBackup) (bool, error) {
	// This function is called just after setStatusAndConditionForDeletionAndCallDelete - standard delete path, which already
	// requeued the reconciliation to get the latest NAB object. There is no need to fetch the latest NAB object here.
	if !controllerutil.ContainsFinalizer(nab, constant.NabFinalizerName) ||
		nab.Status.VeleroBackup == nil ||
		nab.Status.VeleroBackup.NACUUID == constant.EmptyString {
		return false, nil
	}

	// Initiate deletion of the VeleroBackup object only when the finalizer exists.
	// For the ForceDelete we do not create DeleteBackupRequest
	veleroBackupNACUUID := nab.Status.VeleroBackup.NACUUID
	veleroBackup, err := function.GetVeleroBackupByLabel(ctx, r.Client, r.OADPNamespace, veleroBackupNACUUID)

	if err != nil {
		// Log error if multiple VeleroBackup objects are found
		logger.Error(err, findSingleVBError, uuidString, veleroBackupNACUUID)
		return false, err
	}

	if veleroBackup == nil {
		logger.V(1).Info("VeleroBackup already deleted", "UUID", veleroBackupNACUUID)
		return false, nil
	}

	deleteBackupRequest, err := function.GetVeleroDeleteBackupRequestByLabel(ctx, r.Client, r.OADPNamespace, veleroBackupNACUUID)
	if err != nil {
		// Log error if multiple DeleteBackupRequest objects are found
		logger.Error(err, findSingleVDBRError, uuidString, veleroBackupNACUUID)
		return false, err
	}

	if deleteBackupRequest == nil {
		// Build the delete request for VeleroBackup created by NAC
		deleteBackupRequest = builder.ForDeleteBackupRequest(r.OADPNamespace, "").
			BackupName(veleroBackup.Name).
			ObjectMeta(
				builder.WithLabels(
					velerov1.BackupNameLabel, label.GetValidName(veleroBackup.Name),
					velerov1.BackupUIDLabel, string(veleroBackup.UID),
					constant.NabOriginNACUUIDLabel, veleroBackupNACUUID,
				),
				builder.WithLabelsMap(function.GetNonAdminLabels()),
				builder.WithAnnotationsMap(function.GetNonAdminBackupAnnotations(nab.ObjectMeta)),
				builder.WithGenerateName(veleroBackup.Name+"-"),
			).Result()

		// Use CreateRetryGenerateName for retry logic in creating the delete request
		if err := veleroclient.CreateRetryGenerateName(r.Client, ctx, deleteBackupRequest); err != nil {
			logger.Error(err, "Failed to create delete request for VeleroBackup", "VeleroBackup name", veleroBackup.Name, "NonAdminBackup name", nab.Name)
			return false, err
		}
		logger.V(1).Info("Request to delete backup submitted successfully", "VeleroBackup name", veleroBackup.Name, "NonAdminBackup name", nab.Name)
		nab.Status.VeleroDeleteBackupRequest = &nacv1alpha1.VeleroDeleteBackupRequest{
			NACUUID:   veleroBackupNACUUID,
			Namespace: r.OADPNamespace,
			Name:      deleteBackupRequest.Name,
		}
	}
	// Ensure that the NonAdminBackup's NonAdminBackupStatus is in sync
	// with the DeleteBackupRequest. Any required updates to the NonAdminBackup
	// Status will be applied based on the current state of the DeleteBackupRequest.
	updated := updateNonAdminBackupDeleteBackupRequestStatus(&nab.Status, deleteBackupRequest)
	if updated {
		if err := r.Status().Update(ctx, nab); err != nil {
			logger.Error(err, "Failed to update NonAdminBackup Status after DeleteBackupRequest reconciliation")
			return false, err
		}
		logger.V(1).Info("NonAdminBackup DeleteBackupRequest Status updated successfully")
	} else {
		logger.V(1).Info("NonAdminBackup DeleteBackupRequest Status unchanged")
	}

	return false, nil // Continue so initNabDeletion can initialize deletion of a NonAdminBackup object
}

// deleteVeleroBackupAndDeleteBackupRequestObjects deletes both the VeleroBackup and any associated
// DeleteBackupRequest objects for a given NonAdminBackup when force deletion is requested.
//
// Parameters:
//   - ctx: Context for managing request lifetime
//   - logger: Logger instance
//   - nab: NonAdminBackup object
//
// Returns:
//   - bool: whether to requeue (always false)
//   - error: any error encountered during deletion
func (r *NonAdminBackupReconciler) deleteVeleroBackupAndDeleteBackupRequestObjects(ctx context.Context, logger logr.Logger, nab *nacv1alpha1.NonAdminBackup) (bool, error) {
	// This function is called just after setStatusAndConditionForDeletionAndCallDelete - force delete path, which already
	// requeued the reconciliation to get the latest NAB object. There is no need to fetch the latest NAB object here.
	if nab.Status.VeleroBackup == nil || nab.Status.VeleroBackup.NACUUID == constant.EmptyString {
		return false, nil
	}

	veleroBackupNACUUID := nab.Status.VeleroBackup.NACUUID
	veleroBackup, err := function.GetVeleroBackupByLabel(ctx, r.Client, r.OADPNamespace, veleroBackupNACUUID)

	if err != nil {
		// Case where more than one VeleroBackup is found with the same label UUID
		// TODO (migi): Determine if all objects with this UUID should be deleted
		logger.Error(err, findSingleVBError, uuidString, veleroBackupNACUUID)
		return false, err
	}

	if veleroBackup != nil {
		if err = r.Delete(ctx, veleroBackup); err != nil {
			logger.Error(err, "Failed to delete VeleroBackup", nameString, veleroBackup.Name)
			return false, err
		}
		logger.V(1).Info("VeleroBackup deletion initiated", nameString, veleroBackup.Name)
	} else {
		logger.V(1).Info("VeleroBackup already deleted")
	}

	deleteBackupRequest, err := function.GetVeleroDeleteBackupRequestByLabel(ctx, r.Client, r.OADPNamespace, veleroBackupNACUUID)
	if err != nil {
		// Log error if multiple DeleteBackupRequest objects are found
		logger.Error(err, findSingleVDBRError, uuidString, veleroBackupNACUUID)
		return false, err
	}
	if deleteBackupRequest != nil {
		if err = r.Delete(ctx, deleteBackupRequest); err != nil {
			logger.Error(err, "Failed to delete VeleroDeleteBackupRequest", nameString, deleteBackupRequest.Name)
			return false, err
		}
		logger.V(1).Info("VeleroDeleteBackupRequest deletion initiated", nameString, deleteBackupRequest.Name)
	}
	return false, nil // Continue so initNabDeletion can initialize deletion of an NonAdminBackup object
}

// removeNabFinalizerUponVeleroBackupDeletion ensures the associated VeleroBackup object is deleted
// and removes the finalizer from the NonAdminBackup (NAB) object to complete its cleanup process.
//
// Parameters:
//   - ctx: Context for managing request lifetime.
//   - logger: Logger instance for logging messages.
//   - nab: Pointer to the NonAdminBackup object undergoing cleanup.
//
// This function first checks if the `DeleteBackup` field in the NAB spec is set to true or if
// the NAB has been marked for deletion by Kubernetes. If either condition is met, it verifies
// the existence of an associated VeleroBackup object by consulting the UUID stored in the NAB’s status.
// If the VeleroBackup is found, the function waits for it to be deleted before proceeding further.
// After confirming VeleroBackup deletion, it removes the finalizer from the NAB, allowing Kubernetes
// to complete the garbage collection process for the NAB object itself.
//
// Returns:
//   - A boolean indicating whether to requeue the reconcile loop (true if waiting for VeleroBackup deletion).
//   - An error if any update operation or deletion check fails.
func (r *NonAdminBackupReconciler) removeNabFinalizerUponVeleroBackupDeletion(ctx context.Context, logger logr.Logger, nab *nacv1alpha1.NonAdminBackup) (bool, error) {
	// We do not need to gather latest NAB object here:
	//  1. the NAB object was already requeued by setStatusAndConditionForDeletionAndCallDelete
	//  2. removal of NAB finalizer is the last step in the delete paths that will requeue the reconciliation if the
	//      corresponding VeleroBackup object is found based on the NACUUID stored in the NAB status for which we already
	//      have the latest NAB object (point 1 above).
	if !nab.ObjectMeta.DeletionTimestamp.IsZero() {
		if !nab.Spec.ForceDeleteBackup && nab.Status.VeleroBackup != nil && nab.Status.VeleroBackup.NACUUID != constant.EmptyString {
			veleroBackupNACUUID := nab.Status.VeleroBackup.NACUUID

			veleroBackup, err := function.GetVeleroBackupByLabel(ctx, r.Client, r.OADPNamespace, veleroBackupNACUUID)
			if err != nil {
				// Case in which more then one VeleroBackup is found with the same label UUID
				// TODO (migi): Should we delete all of the objects with such UUID ?
				logger.Error(err, findSingleVBError, uuidString, veleroBackupNACUUID)
				return false, err
			}

			if veleroBackup != nil {
				logger.V(1).Info("Waiting for VeleroBackup to be deleted", nameString, veleroBackupNACUUID)
				return true, nil // Requeue
			}
		}
		// VeleroBackup is deleted, proceed with deleting the NonAdminBackup
		logger.V(1).Info("VeleroBackup deleted, removing NonAdminBackup finalizer")

		controllerutil.RemoveFinalizer(nab, constant.NabFinalizerName)

		if err := r.Update(ctx, nab); err != nil {
			logger.Error(err, "Failed to remove finalizer from NonAdminBackup")
			return false, err
		}

		logger.V(1).Info("NonAdminBackup finalizer removed and object deleted")
	}
	return false, nil
}

// initNabCreate initializes the Status.Phase from the NonAdminBackup.
//
// Parameters:
//
//	ctx: Context for the request.
//	logger: Logger instance for logging messages.
//	nab: Pointer to the NonAdminBackup object.
//
// The function checks if the Phase of the NonAdminBackup object is empty.
// If it is empty, it sets the Phase to "New".
// It then returns boolean values indicating whether the reconciliation loop should requeue or exit
// and error value whether the status was updated successfully.
func (r *NonAdminBackupReconciler) initNabCreate(ctx context.Context, logger logr.Logger, nab *nacv1alpha1.NonAdminBackup) (bool, error) {
	// If phase is already set, nothing to do
	if nab.Status.Phase != constant.EmptyString {
		logger.V(1).Info("NonAdminBackup Phase already initialized", "currentPhase", nab.Status.Phase)
		return false, nil
	}

	// Set phase to New
	if updated := updateNonAdminPhase(&nab.Status.Phase, nacv1alpha1.NonAdminBackupPhaseNew); updated {
		if err := r.Status().Update(ctx, nab); err != nil {
			logger.Error(err, statusUpdateError)
			return false, err
		}
		logger.V(1).Info("NonAdminBackup Phase set to New")
	} else {
		logger.V(1).Info("NonAdminBackup Phase update skipped", "currentPhase", nab.Status.Phase)
	}

	return false, nil
}

// validateSpec validates the Spec from the NonAdminBackup.
//
// Parameters:
//
//	ctx: Context for the request.
//	logger: Logger instance for logging messages.
//	nab: Pointer to the NonAdminBackup object.
//
// The function validates the Spec from the NonAdminBackup object.
// If the BackupSpec is invalid, the function sets the NonAdminBackup phase to "BackingOff".
// If the BackupSpec is invalid, the function sets the NonAdminBackup condition Accepted to "False".
// If the BackupSpec is valid, the function sets the NonAdminBackup condition Accepted to "True".
func (r *NonAdminBackupReconciler) validateSpec(ctx context.Context, logger logr.Logger, nab *nacv1alpha1.NonAdminBackup) (bool, error) {
	err := function.ValidateBackupSpec(nab)
	if err != nil {
		updatedPhase := updateNonAdminPhase(&nab.Status.Phase, nacv1alpha1.NonAdminBackupPhaseBackingOff)
		updatedCondition := meta.SetStatusCondition(&nab.Status.Conditions,
			metav1.Condition{
				Type:    string(nacv1alpha1.NonAdminConditionAccepted),
				Status:  metav1.ConditionFalse,
				Reason:  "InvalidBackupSpec",
				Message: err.Error(),
			},
		)
		if updatedPhase || updatedCondition {
			if updateErr := r.Status().Update(ctx, nab); updateErr != nil {
				logger.Error(updateErr, statusUpdateError)
				return false, updateErr
			}
			logger.V(1).Info("NonAdminBackup Phase set to BackingOff")
			logger.V(1).Info("NonAdminBackup condition set to InvalidBackupSpec")
		}
		return false, reconcile.TerminalError(err)
	}

	logger.V(1).Info("NonAdminBackup Spec is valid")

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
			logger.Error(err, statusUpdateError)
			return false, err
		}
		logger.V(1).Info("NonAdminBackup condition set to Accepted")
	} else {
		logger.V(1).Info("NonAdminBackup already has Accepted condition")
	}
	return false, nil
}

// setBackupUUIDInStatus generates a UUID for VeleroBackup and stores it in the NonAdminBackup status.
//
// Parameters:
//
//	ctx: Context for the request.
//	logger: Logger instance for logging messages.
//	nab: Pointer to the NonAdminBackup object.
//
// This function generates a UUID and stores it in the VeleroBackup status field of NonAdminBackup.
func (r *NonAdminBackupReconciler) setBackupUUIDInStatus(ctx context.Context, logger logr.Logger, nab *nacv1alpha1.NonAdminBackup) (bool, error) {
	// Get the latest version of the NAB object just before checking if the NACUUID is set
	// to ensure we do not miss any updates to the NAB object
	nabOriginal := nab.DeepCopy()
	if err := r.Get(ctx, types.NamespacedName{Name: nabOriginal.Name, Namespace: nabOriginal.Namespace}, nab); err != nil {
		logger.Error(err, "Failed to re-fetch NonAdminBackup")
		return false, err
	}

	if nab.Status.VeleroBackup == nil || nab.Status.VeleroBackup.NACUUID == constant.EmptyString {
		veleroBackupNACUUID := function.GenerateNacObjectUUID(nab.Namespace, nab.Name)
		nab.Status.VeleroBackup = &nacv1alpha1.VeleroBackup{
			NACUUID:   veleroBackupNACUUID,
			Namespace: r.OADPNamespace,
			Name:      veleroBackupNACUUID,
		}
		if err := r.Status().Update(ctx, nab); err != nil {
			logger.Error(err, statusUpdateError)
			return false, err
		}
		logger.V(1).Info(veleroReferenceUpdated)
	} else {
		logger.V(1).Info("NonAdminBackup already contains VeleroBackup UUID reference")
	}
	return false, nil
}

func (r *NonAdminBackupReconciler) setFinalizerOnNonAdminBackup(ctx context.Context, logger logr.Logger, nab *nacv1alpha1.NonAdminBackup) (bool, error) {
	// If the object does not have the finalizer, add it before creating Velero Backup
	// to ensure we won't risk having orphant Velero Backup resource, due to an unexpected error
	// while adding finalizer after creatign Velero Backup
	if !controllerutil.ContainsFinalizer(nab, constant.NabFinalizerName) {
		controllerutil.AddFinalizer(nab, constant.NabFinalizerName)
		if err := r.Update(ctx, nab); err != nil {
			logger.Error(err, "Failed to add finalizer")
			return false, err
		}
		logger.V(1).Info("Finalizer added to NonAdminBackup", "finalizer", constant.NabFinalizerName)
	} else {
		logger.V(1).Info("Finalizer exists on the NonAdminBackup object", "finalizer", constant.NabFinalizerName)
	}
	return false, nil
}

// createVeleroBackupAndSyncWithNonAdminBackup ensures the VeleroBackup associated with the given NonAdminBackup resource
// is created, if it does not exist.
// The function also updates the status and conditions of the NonAdminBackup resource to reflect the state
// of the VeleroBackup.
//
// Parameters:
//
//	ctx: Context for the request.
//	logger: Logger instance for logging messages.
//	nab: Pointer to the NonAdminBackup object.
func (r *NonAdminBackupReconciler) createVeleroBackupAndSyncWithNonAdminBackup(ctx context.Context, logger logr.Logger, nab *nacv1alpha1.NonAdminBackup) (bool, error) {
	if nab.Status.VeleroBackup == nil || nab.Status.VeleroBackup.NACUUID == constant.EmptyString {
		return false, errors.New("unable to get Velero Backup UUID from NonAdminBackup Status")
	}

	veleroBackupNACUUID := nab.Status.VeleroBackup.NACUUID

	veleroBackup, err := function.GetVeleroBackupByLabel(ctx, r.Client, r.OADPNamespace, veleroBackupNACUUID)

	if err != nil {
		// Case in which more then one VeleroBackup is found with the same label UUID
		logger.Error(err, findSingleVBError, uuidString, veleroBackupNACUUID)
		return false, err
	}

	if veleroBackup == nil {
		logger.Info("VeleroBackup with label not found, creating one", uuidString, veleroBackupNACUUID)

		backupSpec := nab.Spec.BackupSpec.DeepCopy()
		backupSpec.IncludedNamespaces = []string{nab.Namespace}

		veleroBackup := velerov1.Backup{
			ObjectMeta: metav1.ObjectMeta{
				Name:        veleroBackupNACUUID,
				Namespace:   r.OADPNamespace,
				Labels:      function.GetNonAdminLabels(),
				Annotations: function.GetNonAdminBackupAnnotations(nab.ObjectMeta),
			},
			Spec: *backupSpec,
		}

		// Add NonAdminBackup's veleroBackupNACUUID as the label to the VeleroBackup object
		// We don't add this as an argument of GetNonAdminLabels(), because there may be
		// situations where NAC object do not require NabOriginUUIDLabel
		veleroBackup.Labels[constant.NabOriginNACUUIDLabel] = veleroBackupNACUUID

		err = r.Create(ctx, &veleroBackup)

		if err != nil {
			// We do not retry here as the veleroBackupNACUUID
			// should be guaranteed to be unique
			logger.Error(err, "Failed to create VeleroBackup")
			return false, err
		}
		logger.Info("VeleroBackup successfully created")
	}

	veleroBackupLogger := logger.WithValues("VeleroBackup", types.NamespacedName{Name: veleroBackupNACUUID, Namespace: r.OADPNamespace})

	updatedPhase := updateNonAdminPhase(&nab.Status.Phase, nacv1alpha1.NonAdminBackupPhaseCreated)

	updatedCondition := meta.SetStatusCondition(&nab.Status.Conditions,
		metav1.Condition{
			Type:    string(nacv1alpha1.NonAdminConditionQueued),
			Status:  metav1.ConditionTrue,
			Reason:  "BackupScheduled",
			Message: "Created Velero Backup object",
		},
	)

	if updatedPhase || updatedCondition {
		if err := r.Status().Update(ctx, nab); err != nil {
			logger.Error(err, statusUpdateError)
			return false, err
		}
		logger.V(1).Info(statusUpdateExit)
		return false, nil // TODO (migi): We probably can safely continue with the reconciliation here
	}

	logger.V(1).Info("NonAdminBackup status unchanged during VeleroBackup reconciliation")

	// Ensure that the NonAdminBackup's NonAdminBackupStatus is in sync
	// with the VeleroBackup. Any required updates to the NonAdminBackup
	// Status will be applied based on the current state of the VeleroBackup.
	updated := updateNonAdminBackupVeleroBackupStatus(&nab.Status, veleroBackup)
	if updated {
		if err := r.Status().Update(ctx, nab); err != nil {
			veleroBackupLogger.Error(err, "Failed to update NonAdminBackup Status after VeleroBackup reconciliation")
			return false, err
		}
		logger.V(1).Info("NonAdminBackup Status updated successfully")
	} else {
		logger.V(1).Info("NonAdminBackup Status unchanged during VeleroBackup reconciliation")
	}

	return false, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NonAdminBackupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nacv1alpha1.NonAdminBackup{}).
		WithEventFilter(predicate.CompositePredicate{
			NonAdminBackupPredicate: predicate.NonAdminBackupPredicate{},
			VeleroBackupPredicate: predicate.VeleroBackupPredicate{
				OADPNamespace: r.OADPNamespace,
			},
		}).
		// handler runs after predicate
		Watches(&velerov1.Backup{}, &handler.VeleroBackupHandler{}).
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

// updateNonAdminBackupVeleroBackupStatus sets the VeleroBackup status field in NonAdminBackup object status and returns true
// if the VeleroBackup fields are changed by this call.
func updateNonAdminBackupVeleroBackupStatus(status *nacv1alpha1.NonAdminBackupStatus, veleroBackup *velerov1.Backup) bool {
	if status == nil || veleroBackup == nil {
		return false
	}
	if status.VeleroBackup == nil {
		status.VeleroBackup = &nacv1alpha1.VeleroBackup{}
	}
	if status.VeleroBackup.Status == nil || !reflect.DeepEqual(status.VeleroBackup.Status, veleroBackup.Status) {
		status.VeleroBackup.Status = veleroBackup.Status.DeepCopy()
		return true
	}
	return false
}

// updateNonAdminBackupDeleteBackupRequestStatus sets the VeleroDeleteBackupRequest status field in NonAdminBackup object status and returns true
// if the VeleroDeleteBackupRequest fields are changed by this call.
func updateNonAdminBackupDeleteBackupRequestStatus(status *nacv1alpha1.NonAdminBackupStatus, veleroDeleteBackupRequest *velerov1.DeleteBackupRequest) bool {
	if status == nil || veleroDeleteBackupRequest == nil {
		return false
	}
	if status.VeleroDeleteBackupRequest == nil {
		status.VeleroDeleteBackupRequest = &nacv1alpha1.VeleroDeleteBackupRequest{}
	}
	if status.VeleroDeleteBackupRequest.Status == nil || !reflect.DeepEqual(status.VeleroDeleteBackupRequest.Status, veleroDeleteBackupRequest.Status) {
		status.VeleroDeleteBackupRequest.Status = veleroDeleteBackupRequest.Status.DeepCopy()
		return true
	}
	return false
}
