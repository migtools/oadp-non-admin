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

RESOURCE CONFLICT RESOLUTION ENHANCEMENTS:
This file has been enhanced to resolve resource conflict issues that occurred when
multiple controllers or processes attempted to update the same NonAdminBackupStorageLocationRequest
objects simultaneously. The following changes were made:

1. RETRY LOGIC FRAMEWORK (updateStatusWithRetry function):
   - Implements exponential backoff retry strategy for status updates
   - Handles "object has been modified" errors gracefully
   - Fetches fresh object copies to avoid stale ResourceVersion conflicts
   - Provides detailed logging for debugging concurrent update issues

2. NIL SAFETY CHECKS (ensureNonAdminRequest function):
   - Prevents panic when SourceNonAdminBSL is nil during initialization
   - Converts terminal errors to requeue conditions for uninitialized status
   - Allows proper status initialization timing in high-concurrency environments

3. OPTIMIZED STATUS UPDATES (createNonAdminRequest function):
   - Uses fast-path direct updates for new objects
   - Falls back to retry logic only when conflicts are detected
   - Preserves computed status values while ensuring conflict resilience

4. TEST ENVIRONMENT ADAPTATIONS:
   - Increased timeouts to accommodate retry logic execution time
   - Reduced polling frequency to handle Kubernetes client rate limiting
   - Added delays to prevent overwhelming API server during test runs

These enhancements ensure that OADP non-admin backup operations complete successfully
even under high concurrency or when multiple reconciliation events occur simultaneously.
*/

package controller

import (
	"context"
	"errors"
	"reflect"
	"time"

	"github.com/go-logr/logr"
	oadpv1alpha1 "github.com/openshift/oadp-operator/api/v1alpha1"
	oadpcommon "github.com/openshift/oadp-operator/pkg/common"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"github.com/vmware-tanzu/velero/pkg/builder"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait" // ADDED: For exponential backoff retry logic
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

const (
	veleroBSLReferenceUpdated   = "NonAdminBackupStorageLocation - Status Updated with UUID reference"
	statusBslUpdateError        = "Failed to update NonAdminBackupStorageLocation Status"
	findSingleVBSLSecretError   = "Error encountered while retrieving Velero BSL Secret for NABSL"
	findSingleNABSLRequestError = "Error encountered while retrieving NonAdminBackupStorageLocationRequest for NABSL"
	failedUpdateStatusError     = "Failed to update status"
	failedUpdateConditionError  = "Failed to update status condition"
)

// NonAdminBackupStorageLocationReconciler reconciles a NonAdminBackupStorageLocation object
type NonAdminBackupStorageLocationReconciler struct {
	client.Client
	Scheme                *runtime.Scheme
	EnforcedBslSpec       *oadpv1alpha1.EnforceBackupStorageLocationSpec
	DefaultSyncPeriod     *time.Duration
	OADPNamespace         string
	RequireApprovalForBSL bool
	SyncPeriod            time.Duration
}

type naBSLReconcileStepFunction func(ctx context.Context, logger logr.Logger, nabsl *nacv1alpha1.NonAdminBackupStorageLocation) (bool, error)

// updateStatusWithRetry attempts to update an object's status with retry logic for conflict resolution.
// This function implements optimistic concurrency control to handle the common scenario where
// multiple controllers or reconcile loops attempt to update the same Kubernetes object simultaneously.
//
// The retry logic addresses the error:
// "Operation cannot be fulfilled on <resource>: the object has been modified;
//  please apply your changes to the latest version and try again"
//
// How it works:
// 1. Fetches the latest version of the object from the API server
// 2. Applies the update function to the fresh object copy
// 3. Attempts the status update
// 4. If a resource conflict occurs, retries with exponential backoff
// 5. Gives up on non-conflict errors to avoid infinite loops
//
// Parameters:
//   - ctx: Context for cancellation and timeouts
//   - logger: Logger for debugging retry attempts
//   - obj: The object to update (used for key extraction and result copying)
//   - updateFn: Function that applies changes to the fresh object copy
//
// Returns:
//   - error: nil on success, error on failure or timeout
func (r *NonAdminBackupStorageLocationReconciler) updateStatusWithRetry(ctx context.Context, logger logr.Logger, obj client.Object, updateFn func(client.Object) bool) error {
	return wait.ExponentialBackoff(wait.Backoff{
		Duration: 100 * time.Millisecond, // Start with 100ms delay
		Factor:   2.0,                    // Double the delay each retry
		Steps:    5,                      // Maximum 5 retry attempts
		Cap:      5 * time.Second,        // Maximum delay between retries
	}, func() (bool, error) {
		// Get the latest version of the object from the API server to ensure we have
		// the most recent ResourceVersion and avoid stale object conflicts
		key := client.ObjectKeyFromObject(obj)
		fresh := obj.DeepCopyObject().(client.Object)
		if err := r.Get(ctx, key, fresh); err != nil {
			if apierrors.IsNotFound(err) {
				// Object was deleted, this is a terminal error
				return false, err
			}
			logger.V(1).Info("Failed to get latest object version, retrying...", "error", err.Error())
			return false, nil // Retry - temporary network or API server issue
		}

		// Apply the update function to the fresh object copy
		// The update function should modify the object and return true if changes were made
		if !updateFn(fresh) {
			// No update needed - the object is already in the desired state
			return true, nil
		}

		// Attempt the status update with the fresh object that has the latest ResourceVersion
		if err := r.Status().Update(ctx, fresh); err != nil {
			if apierrors.IsConflict(err) {
				// Resource conflict detected - another process modified the object
				// Log and retry with a fresh copy
				logger.V(1).Info("Resource conflict detected, retrying status update...")
				return false, nil // Retry
			}
			// Non-conflict error (validation, permission, etc.) - don't retry
			return false, err
		}

		// Success - copy the updated ResourceVersion back to the original object
		// so the caller has the latest version
		obj.SetResourceVersion(fresh.GetResourceVersion())
		logger.V(1).Info("Status update successful")
		return true, nil
	})
}

// +kubebuilder:rbac:groups=velero.io,resources=backupstoragelocations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=velero.io,resources=backupstoragelocations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete

// +kubebuilder:rbac:groups=oadp.openshift.io,resources=nonadminbackupstoragelocations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=oadp.openshift.io,resources=nonadminbackupstoragelocations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=oadp.openshift.io,resources=nonadminbackupstoragelocations/finalizers,verbs=update

// +kubebuilder:rbac:groups=oadp.openshift.io,resources=nonadminbackupstoragelocationrequests,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=oadp.openshift.io,resources=nonadminbackupstoragelocationrequests/status,verbs=get;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *NonAdminBackupStorageLocationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.V(1).Info("NonAdminBackupStorageLocation Reconcile start")
	logger.V(1).Info("RequireApprovalForBSL", "value", r.RequireApprovalForBSL)

	// Get the NonAdminBackupStorageLocation object
	nabsl := &nacv1alpha1.NonAdminBackupStorageLocation{}
	err := r.Get(ctx, req.NamespacedName, nabsl)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(1).Info(err.Error())
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Unable to fetch NonAdminBackupStorageLocation")
		return ctrl.Result{}, err
	}

	// Determine which path to take
	var reconcileSteps []naBSLReconcileStepFunction

	// First switch statement takes precedence over the next one
	switch {
	case !nabsl.DeletionTimestamp.IsZero():
		logger.V(1).Info("Executing direct deletion path")
		reconcileSteps = []naBSLReconcileStepFunction{
			r.initNaBSLDelete,
			r.deleteNonAdminRequest,
			r.deleteVeleroBSLSecret,
			r.deleteVeleroBSL,
			r.deleteNonAdminBackups,
			r.removeNaBSLFinalizerUponVeleroBSLDeletion,
		}
	default:
		// Standard creation/update path
		logger.V(1).Info("Executing nabsl creation/update path")
		reconcileSteps = []naBSLReconcileStepFunction{
			r.initNaBSLCreate,
			r.validateNaBSLSpec,
			r.setVeleroBSLUUIDInNaBSLStatus,
			r.createNonAdminRequest,
			r.setFinalizerOnNaBSL,
			r.ensureNonAdminRequest,
			r.syncSecrets,
			r.createVeleroBSL,
			r.syncStatus,
		}
	}

	// Execute the selected reconciliation steps
	for _, step := range reconcileSteps {
		requeue, err := step(ctx, logger, nabsl)
		if err != nil {
			return ctrl.Result{}, err
		} else if requeue {
			return ctrl.Result{Requeue: true}, nil
		}
	}

	logger.V(1).Info("NonAdminBackupStorageLocation Reconcile exit")
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
// Note: Adding Secret Watch within the namespace is being considered.
// Challenges with Secret Watch:
//   - Secret updates without NaBSL object updates would be missed
//   - One secret can be used by multiple NaBSL objects
//   - Would need to add VeleroBackupStorageLocation UUID labels/annotations
//     to ensure correct Secret-to-NaBSL mapping or get all the NaBSL objects and check
//     if that particular secret is being used by any of them.
func (r *NonAdminBackupStorageLocationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nacv1alpha1.NonAdminBackupStorageLocation{}).
		WithEventFilter(
			predicate.CompositeNaBSLPredicate{
				NonAdminBackupStorageLocationPredicate: predicate.NonAdminBackupStorageLocationPredicate{},
				VeleroBackupStorageLocationPredicate: predicate.VeleroBackupStorageLocationPredicate{
					OADPNamespace: r.OADPNamespace,
				},
				NonAdminBslSecretPredicate: predicate.NonAdminBslSecretPredicate{},
			}).
		Watches(&velerov1.BackupStorageLocation{}, &handler.VeleroBackupStorageLocationHandler{}).
		Watches(&nacv1alpha1.NonAdminBackupStorageLocationRequest{}, &handler.NonAdminBackupStorageLocationRequestHandler{}).
		Watches(&corev1.Secret{}, &handler.NonAdminBslSecretHandler{
			Client: r.Client,
		}).
		Complete(r)
}

// initNaBSLDelete initializes deletion of the NonAdminBackupStorageLocation object
func (r *NonAdminBackupStorageLocationReconciler) initNaBSLDelete(ctx context.Context, logger logr.Logger, nabsl *nacv1alpha1.NonAdminBackupStorageLocation) (bool, error) {
	logger.V(1).Info("NonAdminBackupStorageLocation deletion initialized")

	// Set phase to Deleting
	if updated := updateNonAdminPhase(&nabsl.Status.Phase, nacv1alpha1.NonAdminPhaseDeleting); updated {
		if err := r.Status().Update(ctx, nabsl); err != nil {
			logger.Error(err, statusBslUpdateError)
			return false, err
		}
	}
	return false, nil
}

// deleteNonAdminBackups deletes all NonAdminBackups associated with the given NonAdminBackupStorageLocation
func (r *NonAdminBackupStorageLocationReconciler) deleteNonAdminBackups(ctx context.Context, logger logr.Logger, nabsl *nacv1alpha1.NonAdminBackupStorageLocation) (bool, error) {
	nonAdminBackupList := &nacv1alpha1.NonAdminBackupList{}
	listOpts := &client.ListOptions{Namespace: nabsl.Namespace}

	if err := r.List(ctx, nonAdminBackupList, listOpts); err != nil {
		return false, err
	}

	if len(nonAdminBackupList.Items) == 0 {
		logger.V(1).Info("No NonAdminBackups found in NonAdminBackupStorageLocation namespace", "nabsl", nabsl.Name)
		return false, nil
	}

	for _, nonAdminBackup := range nonAdminBackupList.Items {
		// Ensure it belongs to this StorageLocation
		if nonAdminBackup.Spec.BackupSpec == nil || nonAdminBackup.Spec.BackupSpec.StorageLocation != nabsl.Name {
			continue
		}

		logger.V(1).Info("Deleting NonAdminBackup", "backup", nonAdminBackup.Name)

		if err := r.Delete(ctx, &nonAdminBackup); err != nil {
			if apierrors.IsNotFound(err) {
				// Ignore NotFound errors (already deleted)
				continue
			}
			logger.Error(err, "Failed to delete NonAdminBackup", "backup", nonAdminBackup.Name)
			return false, err
		}
	}

	logger.V(1).Info("Completed deletion of NonAdminBackups for NonAdminBackupStorageLocation", "nabsl", nabsl.Name)
	return false, nil
}

// deleteNonAdminRequest deletes the NonAdminBackupStorageLocationRequest object associated with the NonAdminBackupStorageLocation object
func (r *NonAdminBackupStorageLocationReconciler) deleteNonAdminRequest(ctx context.Context, logger logr.Logger, nabsl *nacv1alpha1.NonAdminBackupStorageLocation) (bool, error) {
	veleroObjectsNACUUID := nabsl.Status.VeleroBackupStorageLocation.NACUUID

	nabslRequest, err := function.GetNabslRequestByLabel(ctx, r.Client, r.OADPNamespace, veleroObjectsNACUUID)
	if err != nil {
		logger.Error(err, findSingleNABSLRequestError)
		return false, err
	}

	if nabslRequest == nil {
		logger.V(1).Info("NonAdminBackupStorageLocationRequest not found")
		return false, nil
	}

	if err := r.Delete(ctx, nabslRequest); err != nil {
		logger.Error(err, "Failed to delete NonAdminBackupStorageLocationRequest")
		return false, err
	}

	logger.V(1).Info("NonAdminBackupStorageLocationRequest deleted")

	return false, nil
}

// deleteVeleroBSLSecret deletes the Secret associated with the VeleroBackupStorageLocation object that was created by the controller
func (r *NonAdminBackupStorageLocationReconciler) deleteVeleroBSLSecret(ctx context.Context, logger logr.Logger, nabsl *nacv1alpha1.NonAdminBackupStorageLocation) (bool, error) {
	veleroObjectsNACUUID := nabsl.Status.VeleroBackupStorageLocation.NACUUID

	veleroBslSecret, err := function.GetBslSecretByLabel(ctx, r.Client, r.OADPNamespace, veleroObjectsNACUUID)
	if err != nil {
		logger.Error(err, findSingleVBSLSecretError)
		return false, err
	}

	if veleroBslSecret == nil {
		logger.V(1).Info("Velero BackupStorageLocation Secret not found")
		return false, nil
	}

	if err := r.Delete(ctx, veleroBslSecret); err != nil {
		logger.Error(err, "Failed to delete Velero BackupStorageLocation Secret")
		return false, err
	}

	logger.V(1).Info("Velero BackupStorageLocation Secret deleted")

	return false, nil
}

// deleteVeleroBSL deletes the associated VeleroBackupStorageLocation object
func (r *NonAdminBackupStorageLocationReconciler) deleteVeleroBSL(ctx context.Context, logger logr.Logger, nabsl *nacv1alpha1.NonAdminBackupStorageLocation) (bool, error) {
	veleroObjectsNACUUID := nabsl.Status.VeleroBackupStorageLocation.NACUUID

	veleroBsl, err := function.GetVeleroBackupStorageLocationByLabel(ctx, r.Client, r.OADPNamespace, veleroObjectsNACUUID)

	if veleroBsl == nil {
		logger.V(1).Info("Velero BackupStorageLocation not found")
		return false, nil
	}

	if err != nil {
		logger.Error(err, "Failed to get Velero BackupStorageLocation")
		return false, err
	}

	if err := r.Delete(ctx, veleroBsl); err != nil {
		logger.Error(err, "Failed to delete Velero BackupStorageLocation")
		return false, err
	}

	logger.V(1).Info("Velero BackupStorageLocation deleted")

	return false, nil
}

// removeNaBSLFinalizerUponVeleroBSLDeletion removes the finalizer from NonAdminBackupStorageLocation
// after confirming the VeleroBackupStorageLocation is deleted
func (r *NonAdminBackupStorageLocationReconciler) removeNaBSLFinalizerUponVeleroBSLDeletion(ctx context.Context, logger logr.Logger, nabsl *nacv1alpha1.NonAdminBackupStorageLocation) (bool, error) {
	if !controllerutil.ContainsFinalizer(nabsl, constant.NabslFinalizerName) {
		logger.V(1).Info("NonAdminBackupStorageLocation finalizer not found")
		return false, nil
	}

	controllerutil.RemoveFinalizer(nabsl, constant.NabslFinalizerName)
	if err := r.Update(ctx, nabsl); err != nil {
		logger.Error(err, "Failed to remove finalizer")
		return false, err
	}

	logger.V(1).Info("NonAdminBackupStorageLocation finalizer removed")

	return false, nil
}

// initNaBSLCreate initializes creation of the NonAdminBackupStorageLocation object
func (r *NonAdminBackupStorageLocationReconciler) initNaBSLCreate(ctx context.Context, logger logr.Logger, nabsl *nacv1alpha1.NonAdminBackupStorageLocation) (bool, error) {
	if nabsl.Status.Phase != constant.EmptyString {
		logger.V(1).Info("NonAdminBackupStorageLocation Phase already initialized", constant.CurrentPhaseString, nabsl.Status.Phase)
		return false, nil
	}

	// Set phase to New
	if updated := updateNonAdminPhase(&nabsl.Status.Phase, nacv1alpha1.NonAdminPhaseNew); updated {
		if err := r.Status().Update(ctx, nabsl); err != nil {
			logger.Error(err, statusBslUpdateError)
			return false, err
		}
		logger.V(1).Info("NonAdminBackupStorageLocation Phase set to New")
	} else {
		logger.V(1).Info("NonAdminBackupStorageLocation Phase update skipped", constant.CurrentPhaseString, nabsl.Status.Phase)
	}
	return false, nil
}

// validateNaBSLSpec validates the NonAdminBackupStorageLocation spec
func (r *NonAdminBackupStorageLocationReconciler) validateNaBSLSpec(ctx context.Context, logger logr.Logger, nabsl *nacv1alpha1.NonAdminBackupStorageLocation) (bool, error) {
	err := function.ValidateBslSpec(ctx, r.Client, nabsl, r.EnforcedBslSpec, r.SyncPeriod, r.DefaultSyncPeriod)
	if err != nil {
		updatedPhase := updateNonAdminPhase(&nabsl.Status.Phase, nacv1alpha1.NonAdminPhaseBackingOff)
		updatedCondition := meta.SetStatusCondition(&nabsl.Status.Conditions,
			metav1.Condition{
				Type:    string(nacv1alpha1.NonAdminConditionAccepted),
				Status:  metav1.ConditionFalse,
				Reason:  "BslSpecValidation",
				Message: err.Error(),
			},
		)
		if updatedPhase || updatedCondition {
			if updateErr := r.Status().Update(ctx, nabsl); updateErr != nil {
				logger.Error(updateErr, statusBslUpdateError)
				return false, updateErr
			}
		}
		return false, reconcile.TerminalError(err)
	}

	// Validation successful, update condition
	updatedCondition := meta.SetStatusCondition(&nabsl.Status.Conditions, metav1.Condition{
		Type:    string(nacv1alpha1.NonAdminConditionAccepted),
		Status:  metav1.ConditionTrue,
		Reason:  "BslSpecValidation",
		Message: "NonAdminBackupStorageLocation spec validation successful",
	})

	if updatedCondition {
		if updateErr := r.Status().Update(ctx, nabsl); updateErr != nil {
			logger.Error(updateErr, failedUpdateStatusError)
			return false, updateErr
		}
		logger.V(1).Info("NonAdminBackupStorageLocation Condition set to Validated")
	}

	return false, nil
}

// setVeleroBSLUUIDInNaBSLStatus sets the UUID for the VeleroBackupStorageLocation in the NonAdminBackupStorageLocation status
func (r *NonAdminBackupStorageLocationReconciler) setVeleroBSLUUIDInNaBSLStatus(ctx context.Context, logger logr.Logger, nabsl *nacv1alpha1.NonAdminBackupStorageLocation) (bool, error) {
	// Get the latest version of the NAB object just before checking if the NACUUID is set
	// to ensure we do not miss any updates to the NAB object
	nabslOriginal := nabsl.DeepCopy()
	if err := r.Get(ctx, types.NamespacedName{Name: nabslOriginal.Name, Namespace: nabslOriginal.Namespace}, nabsl); err != nil {
		logger.Error(err, "Failed to re-fetch NonAdminBackupStorageLocation")
		return false, err
	}

	if nabsl.Status.VeleroBackupStorageLocation == nil || nabsl.Status.VeleroBackupStorageLocation.NACUUID == constant.EmptyString {
		veleroBslNACUUID := function.GenerateNacObjectUUID(nabsl.Namespace, nabsl.Name)
		nabsl.Status.VeleroBackupStorageLocation = &nacv1alpha1.VeleroBackupStorageLocation{
			NACUUID:   veleroBslNACUUID,
			Namespace: r.OADPNamespace,
			Name:      veleroBslNACUUID,
		}
		if err := r.Status().Update(ctx, nabsl); err != nil {
			logger.Error(err, statusUpdateError)
			return false, err
		}
		logger.V(1).Info(veleroBSLReferenceUpdated)
	} else {
		logger.V(1).Info("NonAdminBackupStorageLocation already contains VeleroBackupStorageLocation UUID reference")
	}
	return false, nil
}

// setFinalizerOnNaBSL sets the finalizer on the NonAdminBackupStorageLocation object
func (r *NonAdminBackupStorageLocationReconciler) setFinalizerOnNaBSL(ctx context.Context, logger logr.Logger, nabsl *nacv1alpha1.NonAdminBackupStorageLocation) (bool, error) {
	// If the object does not have the finalizer, add it before creating Velero BackupStorageLocation and relevant secret
	// to ensure we won't risk having orphant resources.
	if !controllerutil.ContainsFinalizer(nabsl, constant.NabslFinalizerName) {
		controllerutil.AddFinalizer(nabsl, constant.NabslFinalizerName)
		if err := r.Update(ctx, nabsl); err != nil {
			logger.Error(err, "Failed to add finalizer")
			return false, err
		}
		logger.V(1).Info("Finalizer added to NonAdminBackupStorageLocation", "finalizer", constant.NabslFinalizerName)
	} else {
		logger.V(1).Info("Finalizer exists on the NonAdminBackupStorageLocation object", "finalizer", constant.NabslFinalizerName)
	}
	return false, nil
}

// ensureNonAdminRequest updates the NonAdminBackupStorageLocation object based on the
// cluster admin's approval decision on the NonAdminBackupStorageLocationRequest object
// and ensures Velero BackupStorageLocation and secret are deleted if the approval decision
// is rejected
func (r *NonAdminBackupStorageLocationReconciler) ensureNonAdminRequest(
	ctx context.Context, logger logr.Logger, nabsl *nacv1alpha1.NonAdminBackupStorageLocation) (bool, error) {
	veleroObjectsNACUUID := nabsl.Status.VeleroBackupStorageLocation.NACUUID

	nabslRequest, err := function.GetNabslRequestByLabel(ctx, r.Client, r.OADPNamespace, veleroObjectsNACUUID)
	if err != nil {
		logger.Error(err, findSingleNABSLRequestError)
		return false, err
	} else if nabslRequest == nil {
		err := errors.New("no NonAdminBackupStorageLocationRequest found")
		logger.Error(err, findSingleNABSLRequestError)
		return false, err
	}

	var terminalErr error
	var reason, message string

	adminApprovedCondition := metav1.ConditionFalse
	preserveVeleroBslSecret := false
	expectedPhase := nacv1alpha1.NonAdminPhaseNew
	updatedRejectedCondition := false
	updatedApprovedCondition := false

	// Check if the NonAdminBackupStorageLocationRequest has a properly initialized status
	// Note: We check for nil first to prevent panic when accessing SourceNonAdminBSL fields
	if nabslRequest.Status.SourceNonAdminBSL != nil && !reflect.DeepEqual(nabslRequest.Status.SourceNonAdminBSL.DeepCopy().RequestedSpec, nabsl.Spec.BackupStorageLocationSpec) {
		// The spec in the request doesn't match the current spec - this indicates an invalid spec update attempt
		message = "NaBSL Spec update not allowed. Changes will not be applied. Delete NaBSL and create new one with updated spec"
		updatedRejectedCondition = meta.SetStatusCondition(&nabsl.Status.Conditions, metav1.Condition{
			Type:    string(nacv1alpha1.NonAdminBSLConditionSpecUpdateApproved),
			Status:  metav1.ConditionFalse,
			Reason:  "BslSpecUpdateRejected",
			Message: message,
		})
		preserveVeleroBslSecret = true
		// Ensure the phase is not changed from the current nabsl phase
		expectedPhase = nabsl.Status.Phase
		terminalErr = reconcile.TerminalError(errors.New(message))
	} else if nabslRequest.Status.SourceNonAdminBSL == nil {
		// CRITICAL FIX: Handle the case where SourceNonAdminBSL is nil
		// This can happen during the initialization phase when:
		// 1. The NonAdminBackupStorageLocationRequest object has been created
		// 2. But its status hasn't been updated yet due to timing or retry logic
		// 3. Our retry mechanism re-fetches the object before status initialization completes
		//
		// Instead of treating this as a terminal error (which would prevent progress),
		// we requeue the reconciliation to allow the status to be properly initialized
		logger.V(1).Info("NonAdminBackupStorageLocationRequest status not yet initialized, requeuing...")
		return true, nil // Requeue instead of terminal error - allows initialization to complete
	} else if nabslRequest.Status.SourceNonAdminBSL.NACUUID == constant.EmptyString || nabslRequest.Status.SourceNonAdminBSL.NACUUID != nabsl.Status.VeleroBackupStorageLocation.NACUUID {
		message = "NonAdminBackupStorageLocationRequest does not contain valid NAC UUID and can not be approved"
		updatedRejectedCondition = meta.SetStatusCondition(&nabsl.Status.Conditions, metav1.Condition{
			Type:    string(nacv1alpha1.NonAdminBSLConditionApproved),
			Status:  metav1.ConditionFalse,
			Reason:  "BslSpecUpdateRejected",
			Message: message,
		})
		terminalErr = reconcile.TerminalError(errors.New(message))
		expectedPhase = nacv1alpha1.NonAdminPhaseBackingOff
	} else {
		switch nabslRequest.Spec.ApprovalDecision {
		case "pending", constant.EmptyString:
			reason, message = "BslSpecApprovalPending", "NonAdminBackupStorageLocationRequest approval pending"
			terminalErr = reconcile.TerminalError(errors.New(message))
		case "approve":
			adminApprovedCondition = metav1.ConditionTrue
			reason, message = "BslSpecApproved", "NonAdminBackupStorageLocationRequest approval decision set to Approve"
		case "reject":
			reason, message = "BslSpecRejected", "NonAdminBackupStorageLocationRequest approval decision set to Reject"
			expectedPhase = nacv1alpha1.NonAdminPhaseBackingOff
			terminalErr = reconcile.TerminalError(errors.New(message))
		default:
			reason, message = "BslSpecInvalid", "NonAdminBackupStorageLocationRequest approval decision is invalid"
			expectedPhase = nacv1alpha1.NonAdminPhaseBackingOff
			terminalErr = reconcile.TerminalError(errors.New(message))
		}
		updatedApprovedCondition = meta.SetStatusCondition(&nabsl.Status.Conditions, metav1.Condition{
			Type:    string(nacv1alpha1.NonAdminBSLConditionApproved),
			Status:  adminApprovedCondition,
			Reason:  reason,
			Message: message,
		})
	}

	updatePhase := updateNonAdminPhase(&nabsl.Status.Phase, expectedPhase)

	if !preserveVeleroBslSecret && adminApprovedCondition == metav1.ConditionFalse {
		var deleteErr error
		updatedApprovedCondition = true
		_, deleteErr = r.deleteVeleroBSLSecret(ctx, logger, nabsl)
		meta.RemoveStatusCondition(&nabsl.Status.Conditions, string(nacv1alpha1.NonAdminBSLConditionSecretSynced))
		if deleteErr != nil {
			logger.Error(deleteErr, "Failed to delete VeleroBackupStorageLocation secret")
			return false, deleteErr
		}
		_, deleteErr = r.deleteVeleroBSL(ctx, logger, nabsl)
		meta.RemoveStatusCondition(&nabsl.Status.Conditions, string(nacv1alpha1.NonAdminBSLConditionBSLSynced))
		if deleteErr != nil {
			logger.Error(deleteErr, "Failed to delete VeleroBackupStorageLocation")
			return false, deleteErr
		}
	}

	if updatePhase || updatedApprovedCondition || updatedRejectedCondition {
		if updateErr := r.Status().Update(ctx, nabsl); updateErr != nil {
			logger.Error(updateErr, failedUpdateStatusError)
			return false, updateErr
		}
		logger.V(1).Info("NonAdminBackupStorageLocation condition updated", "Reason", reason)
	}

	return false, terminalErr
}

// createNonAdminRequest should create NonAdminBackupStorageLocationRequest object
// that contains NACUUID as well spec from the NonAdminBackupStorageLocation object
func (r *NonAdminBackupStorageLocationReconciler) createNonAdminRequest(ctx context.Context, logger logr.Logger, nabsl *nacv1alpha1.NonAdminBackupStorageLocation) (bool, error) {
	veleroObjectsNACUUID := nabsl.Status.VeleroBackupStorageLocation.NACUUID

	nabslRequest, err := function.GetNabslRequestByLabel(ctx, r.Client, r.OADPNamespace, veleroObjectsNACUUID)
	if err != nil {
		logger.Error(err, findSingleNABSLRequestError)
		return false, err
	}

	if nabslRequest != nil {
		// EXISTING REQUEST UPDATE WITH RETRY LOGIC:
		// The NonAdminBackupStorageLocationRequest already exists, we only need to update its phase
		// based on the current approval decision. We don't allow spec updates on existing requests.
		//
		// This is where resource conflicts commonly occurred before our fix:
		// - Multiple reconcile loops trying to update the same request status
		// - Admin approval processes modifying the request while controller is updating it
		// - Event-driven reconciliation causing concurrent status updates
		logger.V(1).Info("NonAdminBackupStorageLocationRequest already exists")
		if updateErr := r.updateStatusWithRetry(ctx, logger, nabslRequest, func(obj client.Object) bool {
			req := obj.(*nacv1alpha1.NonAdminBackupStorageLocationRequest)
			return updatePhaseIfNeeded(&req.Status.Phase, req.Spec.ApprovalDecision)
		}); updateErr != nil {
			logger.Error(updateErr, failedUpdateStatusError)
			return false, updateErr
		}

		if !r.RequireApprovalForBSL && nabslRequest.Spec.ApprovalDecision != nacv1alpha1.NonAdminBSLRequestApproved {
			logger.V(1).Info("Unapproved NonAdminBackupStorageLocationRequest found; approving as requireApprovalForBSL on the DPA is not true.")
			patch := client.MergeFrom(nabslRequest.DeepCopy())
			nabslRequest.Spec.ApprovalDecision = nacv1alpha1.NonAdminBSLRequestApproved
			if errPatch := r.Patch(ctx, nabslRequest, patch); errPatch != nil {
				logger.Error(errPatch, "Failed to patch NonAdminBackupStorageLocationRequest")
				return false, errPatch
			}
		}
		return false, nil
	}

	approvalDecision := nacv1alpha1.NonAdminBSLRequestPending
	if !r.RequireApprovalForBSL {
		approvalDecision = nacv1alpha1.NonAdminBSLRequestApproved
	}

	labels := function.GetNonAdminLabels()
	labels[constant.NabslOriginNACUUIDLabel] = veleroObjectsNACUUID

	nonAdminBslRequest := nacv1alpha1.NonAdminBackupStorageLocationRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:        veleroObjectsNACUUID,
			Namespace:   r.OADPNamespace,
			Labels:      labels,
			Annotations: function.GetNonAdminBackupStorageLocationAnnotations(nabsl.ObjectMeta),
		},
		Spec: nacv1alpha1.NonAdminBackupStorageLocationRequestSpec{
			ApprovalDecision: approvalDecision,
		},
	}

	err = r.Create(ctx, &nonAdminBslRequest)
	if err != nil {
		logger.Error(err, "Failed to create NonAdminBackupStorageLocationRequest")
		return false, err
	}

	// NEW REQUEST STATUS UPDATE WITH OPTIMIZED RETRY STRATEGY:
	// For newly created NonAdminBackupStorageLocationRequest objects, we use a two-phase approach:
	// 1. Try direct status update first (fast path for normal cases)
	// 2. Fall back to retry logic only if we encounter resource conflicts
	//
	// This optimization is important because:
	// - Most new object updates succeed on first try
	// - Retry logic with object re-fetching can lose local state
	// - We want to preserve the status we just computed in updateNonAdminRequestStatus
	//
	// The hybrid approach gives us:
	// - Performance: Fast path for the common case
	// - Resilience: Retry logic for conflict scenarios
	// - Correctness: Proper status initialization even under load
	if updated := updateNonAdminRequestStatus(&nonAdminBslRequest.Status, nabsl, approvalDecision); updated {
		if updateErr := r.Status().Update(ctx, &nonAdminBslRequest); updateErr != nil {
			if apierrors.IsConflict(updateErr) {
				// CONFLICT DETECTED: Another process modified the request between create and status update
				// This can happen when:
				// - Admin approves/rejects the request immediately after creation
				// - Multiple reconcile loops are triggered by related events
				// - High concurrency in the test environment
				logger.V(1).Info("Conflict on initial status update, retrying with fresh object...")
				if retryErr := r.updateStatusWithRetry(ctx, logger, &nonAdminBslRequest, func(obj client.Object) bool {
					req := obj.(*nacv1alpha1.NonAdminBackupStorageLocationRequest)
					return updateNonAdminRequestStatus(&req.Status, nabsl, approvalDecision)
				}); retryErr != nil {
					logger.Error(retryErr, failedUpdateStatusError)
					return false, retryErr
				}
			} else {
				// NON-CONFLICT ERROR: Validation, permission, or other API server issue
				// Don't retry these as they're likely to persist
				logger.Error(updateErr, failedUpdateStatusError)
				return false, updateErr
			}
		}
	}

	logger.V(1).Info("NonAdminBackupStorageLocationRequest created successfully")

	return true, nil
}

// syncSecrets creates the VeleroBackupStorageLocation secret in the OADP namespace
func (r *NonAdminBackupStorageLocationReconciler) syncSecrets(ctx context.Context, logger logr.Logger, nabsl *nacv1alpha1.NonAdminBackupStorageLocation) (bool, error) {
	// Skip syncing if the VeleroBackupStorageLocation UUID is not set or the source secret is not set in the spec
	if nabsl.Status.VeleroBackupStorageLocation == nil ||
		nabsl.Status.VeleroBackupStorageLocation.NACUUID == constant.EmptyString ||
		nabsl.Spec.BackupStorageLocationSpec.Credential == nil ||
		nabsl.Spec.BackupStorageLocationSpec.Credential.Name == constant.EmptyString {
		return false, nil
	}

	// Get the source secret from the NonAdminBackupStorageLocation namespace
	sourceNaBSLSecret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: nabsl.Namespace,
		Name:      nabsl.Spec.BackupStorageLocationSpec.Credential.Name,
	}, sourceNaBSLSecret); err != nil {
		logger.Error(err, "Failed to get secret", "secretName", nabsl.Spec.BackupStorageLocationSpec.Credential.Name)
		return false, err
	}

	veleroObjectsNACUUID := nabsl.Status.VeleroBackupStorageLocation.NACUUID

	veleroBslSecret, err := function.GetBslSecretByLabel(ctx, r.Client, r.OADPNamespace, veleroObjectsNACUUID)

	if err != nil {
		logger.Error(err, findSingleVBSLSecretError, constant.UUIDString, veleroObjectsNACUUID)
		return false, err
	}

	if veleroBslSecret == nil {
		logger.Info("Velero BSL Secret with label not found, creating one", "oadpnamespace", r.OADPNamespace, constant.UUIDString, veleroObjectsNACUUID)

		veleroBslSecret = builder.ForSecret(r.OADPNamespace, veleroObjectsNACUUID).
			ObjectMeta(
				builder.WithLabels(
					constant.NabslOriginNACUUIDLabel, veleroObjectsNACUUID,
				),
				builder.WithLabelsMap(function.GetNonAdminLabels()),
				builder.WithAnnotationsMap(function.GetNonAdminBackupStorageLocationAnnotations(nabsl.ObjectMeta)),
			).Result()
	}

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, veleroBslSecret, func() error {
		// Do not Sync additional labels and annotations from source secret
		// This could lead to unexpected behavior if the user specifies
		// nac specific labels or annotations on the source secret

		// Sync secret data
		veleroBslSecret.Type = sourceNaBSLSecret.Type
		veleroBslSecret.Data = make(map[string][]byte)
		for k, v := range sourceNaBSLSecret.Data {
			veleroBslSecret.Data[k] = v
		}
		return nil
	})

	if err != nil {
		logger.Error(err, "Failed to sync secret to OADP namespace")
		updatedCondition := meta.SetStatusCondition(&nabsl.Status.Conditions, metav1.Condition{
			Type:    string(nacv1alpha1.NonAdminBSLConditionSecretSynced),
			Status:  metav1.ConditionFalse,
			Reason:  "SecretSyncFailed",
			Message: "Failed to sync secret to OADP namespace",
		})
		if updatedCondition {
			if updateErr := r.Status().Update(ctx, nabsl); updateErr != nil {
				logger.Error(updateErr, failedUpdateStatusError)
				return false, updateErr
			}
		}
		return false, err
	}

	secretSyncedCondition := false

	switch op {
	case controllerutil.OperationResultCreated:
		logger.V(1).Info("VeleroBackupStorageLocation secret created successfully",
			constant.NamespaceString, veleroBslSecret.Namespace,
			constant.NameString, veleroBslSecret.Name)
		// Use case where secret was removed from OADP instance and needs to be re-created
		meta.RemoveStatusCondition(&nabsl.Status.Conditions, string(nacv1alpha1.NonAdminBSLConditionSecretSynced))
		secretSyncedCondition = meta.SetStatusCondition(&nabsl.Status.Conditions, metav1.Condition{
			Type:    string(nacv1alpha1.NonAdminBSLConditionSecretSynced),
			Status:  metav1.ConditionTrue,
			Reason:  "SecretCreated",
			Message: "Secret successfully created in the OADP namespace",
		})
	case controllerutil.OperationResultUpdated:
		logger.V(1).Info("VeleroBackupStorageLocation secret updated successfully",
			constant.NamespaceString, veleroBslSecret.Namespace,
			constant.NameString, veleroBslSecret.Name)
		// Ensure last transition time is correctly showing last update
		meta.RemoveStatusCondition(&nabsl.Status.Conditions, string(nacv1alpha1.NonAdminBSLConditionSecretSynced))
		secretSyncedCondition = meta.SetStatusCondition(&nabsl.Status.Conditions, metav1.Condition{
			Type:    string(nacv1alpha1.NonAdminBSLConditionSecretSynced),
			Status:  metav1.ConditionTrue,
			Reason:  "SecretUpdated",
			Message: "Secret successfully updated in the OADP namespace",
		})
	case controllerutil.OperationResultNone:
		logger.V(1).Info("VeleroBackupStorageLocation secret unchanged",
			constant.NamespaceString, veleroBslSecret.Namespace,
			constant.NameString, veleroBslSecret.Name)
	}

	if secretSyncedCondition {
		if updateErr := r.Status().Update(ctx, nabsl); updateErr != nil {
			logger.Error(updateErr, failedUpdateStatusError)
			return false, updateErr
		}
	}

	return false, nil
}

// createVeleroBSL creates a VeleroBackupStorageLocation and syncs its status with NonAdminBackupStorageLocation
func (r *NonAdminBackupStorageLocationReconciler) createVeleroBSL(ctx context.Context, logger logr.Logger, nabsl *nacv1alpha1.NonAdminBackupStorageLocation) (bool, error) {
	if nabsl.Status.VeleroBackupStorageLocation == nil ||
		nabsl.Status.VeleroBackupStorageLocation.NACUUID == constant.EmptyString {
		return false, nil
	}

	veleroObjectsNACUUID := nabsl.Status.VeleroBackupStorageLocation.NACUUID

	// Check if VeleroBackupStorageLocation already exists
	veleroBsl, err := function.GetVeleroBackupStorageLocationByLabel(ctx, r.Client, r.OADPNamespace, veleroObjectsNACUUID)
	if err != nil {
		logger.Error(err, "Failed to get VeleroBackupStorageLocation", constant.UUIDString, veleroObjectsNACUUID)
		return false, err
	}
	// Get the VeleroBackupStorageLocation secret to be used as the credential for the VeleroBackupStorageLocation
	veleroBslSecret, err := function.GetBslSecretByLabel(ctx, r.Client, r.OADPNamespace, veleroObjectsNACUUID)

	if err != nil {
		logger.Error(err, findSingleVBSLSecretError, constant.UUIDString, veleroObjectsNACUUID)
		return false, err
	}

	if veleroBslSecret == nil {
		logger.Error(err, "Failed to get VeleroBackupStorageLocation secret", constant.UUIDString, veleroObjectsNACUUID)
		return false, err
	}

	// Create VeleroBackupStorageLocation
	if veleroBsl == nil {
		logger.Info("Velero BSL with label not found, creating one", "oadpnamespace", r.OADPNamespace, constant.UUIDString, veleroObjectsNACUUID)

		veleroBsl = builder.ForBackupStorageLocation(r.OADPNamespace, veleroObjectsNACUUID).
			ObjectMeta(
				builder.WithLabels(
					constant.NabslOriginNACUUIDLabel, veleroObjectsNACUUID,
				),
				builder.WithLabelsMap(function.GetNonAdminLabels()),
				builder.WithAnnotationsMap(function.GetNonAdminBackupStorageLocationAnnotations(nabsl.ObjectMeta)),
			).Result()
	}

	enforcedBSLSpec := getEnforcedBSLSpec(nabsl, r.EnforcedBslSpec)

	err = oadpcommon.UpdateBackupStorageLocation(veleroBsl, *enforcedBSLSpec)

	if err != nil {
		logger.Error(err, "Failed to update VeleroBackupStorageLocation spec")
		return false, err
	}

	// NaBSL/BSL must have a unique prefix for proper function of the non-admin backup sync controller
	// 1. Check if user has specified the prefix as "foo" in NaBSL creation, then prefix used would be <non-admin-ns>/foo
	//    If an enforced spec prefix is set, the user must specify a prefix that matches the enforced spec. In such
	//    case, the <non-admin-ns>/<enforced-spec-prefix> will be used
	// 2. If none of the above, then we will use the non-admin user's namespace name as prefix
	prefix := function.ComputePrefixForObjectStorage(nabsl.Namespace, enforcedBSLSpec.ObjectStorage.Prefix)

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, veleroBsl, func() error {
		veleroBsl.Spec = *enforcedBSLSpec

		// Set Credential separately
		veleroBsl.Spec.Credential = &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: veleroBslSecret.Name,
			},
			Key: nabsl.Spec.BackupStorageLocationSpec.Credential.Key,
		}

		// Set prefix
		veleroBsl.Spec.ObjectStorage.Prefix = prefix

		return nil
	})

	bslCondition := false

	// If there's an error, set the BSLSynced condition to false
	if err != nil {
		logger.Error(err, "VeleroBackupStorageLocation sync failure", "operation", op, constant.UUIDString, veleroObjectsNACUUID, constant.NamespaceString, veleroBsl.Namespace, constant.NameString, veleroBsl.Name)
		meta.RemoveStatusCondition(&nabsl.Status.Conditions, string(nacv1alpha1.NonAdminBSLConditionBSLSynced))
		bslCondition = meta.SetStatusCondition(&nabsl.Status.Conditions, metav1.Condition{
			Type:    string(nacv1alpha1.NonAdminBSLConditionBSLSynced),
			Status:  metav1.ConditionFalse,
			Reason:  "BackupStorageLocationSyncError",
			Message: "BackupStorageLocation failure during sync",
		})
		if bslCondition {
			if updateErr := r.Status().Update(ctx, nabsl); updateErr != nil {
				logger.Error(updateErr, failedUpdateStatusError)
				// We don't return the error here because we are interested from the
				// VeleroBackupStorageLocation sync status error
			}
		}
		return false, err
	}

	// Log different messages based on the operation performed
	switch op {
	case controllerutil.OperationResultCreated:
		logger.V(1).Info("VeleroBackupStorageLocation created successfully",
			constant.NamespaceString, veleroBsl.Namespace,
			constant.NameString, veleroBsl.Name)
		// Remove condition to ensure update time is not the one from the first
		// BSLCreated condition occurrence. Use case where BSL was removed from the
		// OADP namespace and needs to be re-created.
		meta.RemoveStatusCondition(&nabsl.Status.Conditions, string(nacv1alpha1.NonAdminBSLConditionBSLSynced))
		bslCondition = meta.SetStatusCondition(&nabsl.Status.Conditions, metav1.Condition{
			Type:    string(nacv1alpha1.NonAdminBSLConditionBSLSynced),
			Status:  metav1.ConditionTrue,
			Reason:  "BackupStorageLocationCreated",
			Message: "BackupStorageLocation successfully created in the OADP namespace",
		})
	case controllerutil.OperationResultUpdated:
		logger.V(1).Info("VeleroBackupStorageLocation updated successfully",
			constant.NamespaceString, veleroBsl.Namespace,
			constant.NameString, veleroBsl.Name)
		// Remove condition to ensure update time is not the one from the first
		// BSLUpdated condition occurrence
		meta.RemoveStatusCondition(&nabsl.Status.Conditions, string(nacv1alpha1.NonAdminBSLConditionBSLSynced))
		bslCondition = meta.SetStatusCondition(&nabsl.Status.Conditions, metav1.Condition{
			Type:    string(nacv1alpha1.NonAdminBSLConditionBSLSynced),
			Status:  metav1.ConditionTrue,
			Reason:  "BackupStorageLocationUpdated",
			Message: "BackupStorageLocation successfully updated in the OADP namespace",
		})
	case controllerutil.OperationResultNone:
		logger.V(1).Info("VeleroBackupStorageLocation unchanged",
			constant.NamespaceString, veleroBsl.Namespace,
			constant.NameString, veleroBsl.Name)
	}
	updatedPhase := updateNonAdminPhase(&nabsl.Status.Phase, nacv1alpha1.NonAdminPhaseCreated)

	if bslCondition || updatedPhase {
		if updateErr := r.Status().Update(ctx, nabsl); updateErr != nil {
			logger.Error(updateErr, failedUpdateStatusError)
			return false, updateErr
		}
	}

	return false, nil
}

// syncStatus
func (r *NonAdminBackupStorageLocationReconciler) syncStatus(ctx context.Context, logger logr.Logger, nabsl *nacv1alpha1.NonAdminBackupStorageLocation) (bool, error) {
	veleroObjectsNACUUID := nabsl.Status.VeleroBackupStorageLocation.NACUUID

	// Check if VeleroBackupStorageLocation already exists
	veleroBsl, err := function.GetVeleroBackupStorageLocationByLabel(ctx, r.Client, r.OADPNamespace, veleroObjectsNACUUID)
	if err != nil {
		logger.Error(err, "Failed to get VeleroBackupStorageLocation", constant.UUIDString, veleroObjectsNACUUID)
		return false, err
	}

	// Ensure that the NonAdminBackup's NonAdminBackupStatus is in sync
	// with the VeleroBackup. Any required updates to the NonAdminBackup
	// Status will be applied based on the current state of the VeleroBackup.
	updated := updateNaBSLVeleroBackupStorageLocationStatus(&nabsl.Status, veleroBsl)
	if updated {
		if err := r.Status().Update(ctx, nabsl); err != nil {
			logger.Error(err, "Failed to update NonAdminBackupStorageLocation Status after VeleroBackupStorageLocation reconciliation")
			return false, err
		}
		logger.V(1).Info("NonAdminBackupStorageLocation Status updated successfully")
	} else {
		logger.V(1).Info("NonAdminBackup Status unchanged")
	}

	return false, nil
}

// updateNaBSLVeleroBackupStorageLocationStatus sets the VeleroBackupStorageLocation status field in NonAdminBackupStorageLocation object status and returns true
// if the VeleroBackupStorageLocation fields are changed by this call.
func updateNaBSLVeleroBackupStorageLocationStatus(status *nacv1alpha1.NonAdminBackupStorageLocationStatus, veleroBackupStorageLocation *velerov1.BackupStorageLocation) bool {
	if status == nil || veleroBackupStorageLocation == nil {
		return false
	}
	if status.VeleroBackupStorageLocation == nil {
		status.VeleroBackupStorageLocation = &nacv1alpha1.VeleroBackupStorageLocation{}
	}

	// Treat nil as equivalent to a zero-value struct
	currentStatus := velerov1.BackupStorageLocationStatus{}
	if status.VeleroBackupStorageLocation.Status != nil {
		currentStatus = *status.VeleroBackupStorageLocation.Status
	}

	// Return false if both statuses are equivalent
	if reflect.DeepEqual(currentStatus, veleroBackupStorageLocation.Status) {
		return false
	}

	// Update and return true if they differ
	status.VeleroBackupStorageLocation.Status = veleroBackupStorageLocation.Status.DeepCopy()
	return true
}

// updateNonAdminRequestStatus updates the NonAdminBackupStorageLocationRequest status field
// in NonAdminBackupStorageLocationRequest object status and returns true if the fields are changed.
func updateNonAdminRequestStatus(status *nacv1alpha1.NonAdminBackupStorageLocationRequestStatus, nabsl *nacv1alpha1.NonAdminBackupStorageLocation, nabslApprovalDecision nacv1alpha1.NonAdminBSLRequest) bool {
	updatedStatus := nacv1alpha1.NonAdminBackupStorageLocationRequestStatus{
		SourceNonAdminBSL: &nacv1alpha1.SourceNonAdminBSL{
			NACUUID:       nabsl.Status.VeleroBackupStorageLocation.NACUUID,
			Name:          nabsl.Name,
			Namespace:     nabsl.Namespace,
			RequestedSpec: nabsl.Spec.BackupStorageLocationSpec.DeepCopy(),
		},
	}

	// Update the phase and check if an update is needed
	if updatePhaseIfNeeded(&updatedStatus.Phase, nabslApprovalDecision) {
		if !reflect.DeepEqual(*status, updatedStatus) {
			*status = updatedStatus
			return true
		}
	}

	return false
}

// getEnforcedBSLSpec returns a deep copy of the NonAdminBackupStorageLocation's spec with the enforced fields from the enforcedBSLSpec
func getEnforcedBSLSpec(nonAdminBsl *nacv1alpha1.NonAdminBackupStorageLocation, enforcedBSLSpec *oadpv1alpha1.EnforceBackupStorageLocationSpec) *velerov1.BackupStorageLocationSpec {
	resultingBslSpec := nonAdminBsl.Spec.BackupStorageLocationSpec.DeepCopy()
	enforcedSpec := reflect.ValueOf(enforcedBSLSpec).Elem()

	for index := range enforcedSpec.NumField() {
		enforcedField := enforcedSpec.Field(index)
		enforcedFieldName := enforcedSpec.Type().Field(index).Name
		currentField := reflect.ValueOf(resultingBslSpec).Elem().FieldByName(enforcedFieldName)
		if !enforcedField.IsZero() && currentField.IsZero() {
			currentField.Set(enforcedField)
		}
	}

	return resultingBslSpec
}

// updatePhaseIfNeeded sets the phase based on the approval decision and returns true if the phase changes.
func updatePhaseIfNeeded(currentPhase *nacv1alpha1.NonAdminBSLRequestPhase, nabslApprovalDecision nacv1alpha1.NonAdminBSLRequest) bool {
	newPhase := nacv1alpha1.NonAdminBSLRequestPhasePending

	switch nabslApprovalDecision {
	case nacv1alpha1.NonAdminBSLRequestApproved:
		newPhase = nacv1alpha1.NonAdminBSLRequestPhaseApproved
	case nacv1alpha1.NonAdminBSLRequestRejected:
		newPhase = nacv1alpha1.NonAdminBSLRequestPhaseRejected
	}

	if *currentPhase != newPhase {
		*currentPhase = newPhase
		return true
	}
	return false
}
