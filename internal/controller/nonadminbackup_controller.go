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
	phaseUpdateRequeue           = "NonAdminBackup - Requeue after Phase Update"
	conditionUpdateRequeue       = "NonAdminBackup - Requeue after Condition Update"
	veleroReferenceUpdateRequeue = "NonAdminBackup - Requeue after Status Update with UUID reference"
	statusUpdateExit             = "NonAdminBackup - Exit after Status Update"
	statusUpdateError            = "Failed to update NonAdminBackup Status"
	findSingleVBError            = "Failed to find single VeleroBackup object"
	uuidString                   = "UUID"
	nameString                   = "name"
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

	reconcileSteps := []reconcileStepFunction{
		r.setFinalizerForDeletion,
		r.initVeleroBackupDeletion,
		r.initNabDeletion,
		r.initNabCreate,
		r.validateSpec,
		r.setBackupUUIDInStatus,
		r.createVeleroBackupAndSyncWithNonAdminBackup,
		r.finalizeNabDeletion,
	}
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

// setFinalizerForDeletion ensures that the finalizer is set on the NonAdminBackup object
// if it is marked for deletion by the `deleteBackup` spec field.
//
// Parameters:
//   - ctx: Context for managing request lifetime.
//   - logger: Logger instance for logging messages.
//   - nab: Pointer to the NonAdminBackup object to be updated.
//
// This function checks if the NonAdminBackup object’s `deleteBackup` field is set to true.
// If so, it verifies whether the finalizer is already present. If not, the finalizer is added
// and the function requeues the reconciliation to avoid continuing until the finalizer is set.
// The function also updates the status phase to `Deletion` if necessary and initiates deletion
// of the corresponding VeleroBackup object. It returns a boolean indicating whether to requeue
// and an error if the status update or VeleroBackup deletion fails.
func (r *NonAdminBackupReconciler) setFinalizerForDeletion(ctx context.Context, logger logr.Logger, nab *nacv1alpha1.NonAdminBackup) (bool, error) {
	// Check if the object is marked for deletion by checking the deleteBackup spec
	if !nab.Spec.DeleteBackup {
		return false, nil
	}

	// If the object does not have the finalizer, add it
	if !controllerutil.ContainsFinalizer(nab, constant.NabFinalizerName) {
		controllerutil.AddFinalizer(nab, constant.NabFinalizerName)
		if err := r.Update(ctx, nab); err != nil {
			logger.Error(err, "Failed to add finalizer")
			return false, err
		}
		logger.V(1).Info("Finalizer added to NonAdminBackup", "finalizer", constant.NabFinalizerName)
		return true, nil // Requeue after adding finalizer
	}

	// Update Phase if needed, outside finalizer block to avoid excessive updates
	updated := updateNonAdminPhase(&nab.Status.Phase, nacv1alpha1.NonAdminBackupPhaseDeletion)
	if updated {
		if err := r.Status().Update(ctx, nab); err != nil {
			logger.Error(err, statusUpdateError)
			return false, err
		}
		logger.V(1).Info(phaseUpdateRequeue)
		return true, nil // Requeue after status update
	}

	logger.V(1).Info("NonAdminBackup marked for deletetion")
	return false, nil // Continue so initVeleroBackupDeletion can delete VeleroBackup object
}

// initVeleroBackupDeletion initiates the deletion of the associated VeleroBackup
// if the NonAdminBackup (NAB) object is marked for deletion and contains the finalizer.
//
// Parameters:
//   - ctx: Context to manage request lifetime.
//   - logger: Logger instance for logging messages.
//   - nab: Pointer to the NonAdminBackup object to be managed.
//
// This function checks if the `DeleteBackup` spec field is set to true and verifies the
// presence of the finalizer on the NAB object. If these conditions are met, it attempts
// to locate the associated VeleroBackup object based on the UUID stored in NAB's status.
// If a single VeleroBackup object is found, it initiates deletion of that VeleroBackup
// and requeues the reconcile loop. If VeleroBackup deletion fails or multiple objects
// are found, it logs the error and returns for further handling.
//
// Returns a boolean indicating whether to requeue and an error if deletion or retrieval
// of the VeleroBackup fails.
func (r *NonAdminBackupReconciler) initVeleroBackupDeletion(ctx context.Context, logger logr.Logger, nab *nacv1alpha1.NonAdminBackup) (bool, error) {
	// Check if DeleteBackup is set and finalizer is present
	if !nab.Spec.DeleteBackup || !controllerutil.ContainsFinalizer(nab, constant.NabFinalizerName) {
		return false, nil
	}

	// Initiate VeleroBackup deletion
	veleroBackupNameUUID := nab.Status.VeleroBackup.NameUUID
	veleroBackup, err := function.GetVeleroBackupByLabel(ctx, r.Client, r.OADPNamespace, veleroBackupNameUUID)

	if err != nil {
		// Case in which more then one VeleroBackup is found with the same label UUID
		// TODO: Should we delete all of the objects with such UUID ?
		logger.Error(err, findSingleVBError, uuidString, veleroBackupNameUUID)
		return false, err
	}

	if veleroBackup != nil {
		if err := r.Delete(ctx, veleroBackup); err != nil {
			logger.Error(err, "Failed to delete VeleroBackup", nameString, veleroBackupNameUUID)
			return false, err
		}
		logger.V(1).Info("VeleroBackup deletion initiated", nameString, veleroBackupNameUUID)
		return true, nil // Requeue after deletion of VeleroBackup was initiated
	}

	logger.V(1).Info("VeleroBackup already deleted")
	return false, nil // Continue so initNabDeletion can initialize deletion of an NonAdminBackup object
}

// initNabDeletion initiates the deletion of the NonAdminBackup (NAB) object if
// it is marked for deletion and has the required finalizer.
//
// Parameters:
//   - ctx: Context for managing request lifetime.
//   - logger: Logger instance for logging messages.
//   - nab: Pointer to the NonAdminBackup object to be deleted.
//
// This function checks if the `DeleteBackup` spec field is set to true and if
// the NAB object contains the finalizer. If both conditions are met, and the
// deletion has not previously been initiated, it marks the NAB object for deletion.
// It returns a boolean indicating whether to requeue and an error if any deletion operation fails.
func (r *NonAdminBackupReconciler) initNabDeletion(ctx context.Context, logger logr.Logger, nab *nacv1alpha1.NonAdminBackup) (bool, error) {
	// Check if DeleteBackup is set and finalizer is present
	if !nab.Spec.DeleteBackup || !controllerutil.ContainsFinalizer(nab, constant.NabFinalizerName) {
		return false, nil
	}

	if nab.ObjectMeta.DeletionTimestamp.IsZero() {
		logger.V(1).Info("Marking NonAdminBackup for deletion", nameString, nab.Name)
		if err := r.Delete(ctx, nab); err != nil {
			logger.Error(err, "Failed to mark NonAdminBackup for deletion")
			return false, err
		}
		return true, nil // Requeue to allow deletion to proceed
	}

	logger.V(1).Info("NonAdminBackup already marked for deletion")
	return false, nil // Continue so finalizeNabDeletion can delete NonAdminBackup object
}

// finalizeNabDeletion handles the deletion of the NonAdminBackup (NAB) object by ensuring
// the associated VeleroBackup is deleted and the NAB finalizer is removed once cleanup is complete.
//
// Parameters:
//   - ctx: Context for managing request lifetime.
//   - logger: Logger instance for logging messages.
//   - nab: Pointer to the NonAdminBackup object to be cleaned up.
//
// The function first checks if the `deleteBackup` spec field is set to true and if the NAB object
// contains the finalizer. If these conditions are met, it attempts to locate the VeleroBackup
// object associated with the NAB and if it is found it ensures it's deleted.
// Once the VeleroBackup object is confirmed as deleted, the finalizer
// is removed from the NAB, allowing the object itself to be deleted by Kubernetes garbage collection.
// Returns a boolean indicating whether to requeue and an error if any update or deletion operation fails.
func (r *NonAdminBackupReconciler) finalizeNabDeletion(ctx context.Context, logger logr.Logger, nab *nacv1alpha1.NonAdminBackup) (bool, error) {
	// Check if DeleteBackup is set and finalizer is present
	if !nab.Spec.DeleteBackup || !controllerutil.ContainsFinalizer(nab, constant.NabFinalizerName) {
		return false, nil
	}

	veleroBackupNameUUID := nab.Status.VeleroBackup.NameUUID
	veleroBackup, err := function.GetVeleroBackupByLabel(ctx, r.Client, r.OADPNamespace, veleroBackupNameUUID)

	if err != nil {
		// Case in which more then one VeleroBackup is found with the same label UUID
		// TODO: Should we delete all of the objects with such UUID ?
		logger.Error(err, findSingleVBError, uuidString, veleroBackupNameUUID)
		return false, err
	}

	if veleroBackup != nil {
		logger.V(1).Info("Waiting for VeleroBackup to be deleted", nameString, veleroBackupNameUUID)
		return true, nil // Requeue
	}

	// VeleroBackup is deleted, proceed with deleting the NonAdminBackup
	logger.V(1).Info("VeleroBackup deleted, removing NonAdminBackup finalizer")
	controllerutil.RemoveFinalizer(nab, constant.NabFinalizerName)

	if err := r.Update(ctx, nab); err != nil {
		logger.Error(err, "Failed to remove finalizer from NonAdminBackup")
		return false, err
	}

	logger.V(1).Info("NonAdminBackup finalizer removed and object deleted")
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
	// Skip the init, due to finalizer being set
	if controllerutil.ContainsFinalizer(nab, constant.NabFinalizerName) {
		return false, nil
	}

	if nab.Status.Phase == constant.EmptyString {
		updated := updateNonAdminPhase(&nab.Status.Phase, nacv1alpha1.NonAdminBackupPhaseNew)
		if updated {
			if err := r.Status().Update(ctx, nab); err != nil {
				logger.Error(err, statusUpdateError)
				return false, err
			}

			logger.V(1).Info(phaseUpdateRequeue)
			return true, nil
		}
	}

	logger.V(1).Info("NonAdminBackup Phase already initialized")
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
	// Skip the validateSpec, due to finalizer being set
	if controllerutil.ContainsFinalizer(nab, constant.NabFinalizerName) {
		return false, nil
	}

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
		}

		logger.Error(err, "NonAdminBackup Spec is not valid")
		return false, reconcile.TerminalError(err)
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
			logger.Error(err, statusUpdateError)
			return false, err
		}

		logger.V(1).Info(conditionUpdateRequeue)
		return true, nil
	}

	logger.V(1).Info("NonAdminBackup Spec already validated")
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
	// Skip the setBackupUUIDInStatus, due to finalizer being set
	if controllerutil.ContainsFinalizer(nab, constant.NabFinalizerName) {
		return false, nil
	}

	if nab.Status.VeleroBackup == nil || nab.Status.VeleroBackup.NameUUID == constant.EmptyString {
		veleroBackupNameUUID := function.GenerateNacObjectNameWithUUID(nab.Namespace, nab.Name)
		nab.Status.VeleroBackup = &nacv1alpha1.VeleroBackup{
			NameUUID:  veleroBackupNameUUID,
			Namespace: r.OADPNamespace,
		}
		if err := r.Status().Update(ctx, nab); err != nil {
			logger.Error(err, statusUpdateError)
			return false, err
		}
		logger.V(1).Info(veleroReferenceUpdateRequeue)
		return true, nil
	}
	logger.V(1).Info("NonAdminBackup already contains VeleroBackup UUID reference")
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
	// Skip the createVeleroBackupAndSyncWithNonAdminBackup, due to finalizer being set
	if controllerutil.ContainsFinalizer(nab, constant.NabFinalizerName) {
		return false, nil
	}

	if nab.Status.VeleroBackup == nil || nab.Status.VeleroBackup.NameUUID == constant.EmptyString {
		return false, errors.New("unable to get Velero Backup UUID from NonAdminBackup Status")
	}

	veleroBackupNameUUID := nab.Status.VeleroBackup.NameUUID

	veleroBackup, err := function.GetVeleroBackupByLabel(ctx, r.Client, r.OADPNamespace, veleroBackupNameUUID)

	if err != nil {
		// Case in which more then one VeleroBackup is found with the same label UUID
		logger.Error(err, findSingleVBError, uuidString, veleroBackupNameUUID)
		return false, err
	}

	if veleroBackup == nil {
		logger.Info("VeleroBackup with label not found, creating one", uuidString, veleroBackupNameUUID)

		backupSpec := nab.Spec.BackupSpec.DeepCopy()
		backupSpec.IncludedNamespaces = []string{nab.Namespace}

		veleroBackup := velerov1.Backup{
			ObjectMeta: metav1.ObjectMeta{
				Name:        veleroBackupNameUUID,
				Namespace:   r.OADPNamespace,
				Labels:      function.GetNonAdminLabels(),
				Annotations: function.GetNonAdminBackupAnnotations(nab.ObjectMeta),
			},
			Spec: *backupSpec,
		}

		// Add NonAdminBackup's veleroBackupNameUUID as the label to the VeleroBackup object
		// We don't add this as an argument of GetNonAdminLabels(), because there may be
		// situations where NAC object do not require NabOriginUUIDLabel
		veleroBackup.Labels[constant.NabOriginNameUUIDLabel] = veleroBackupNameUUID

		err = r.Create(ctx, &veleroBackup)

		if err != nil {
			// We do not retry here as the veleroBackupNameUUID
			// should be guaranteed to be unique
			logger.Error(err, "Failed to create VeleroBackup")
			return false, err
		}
		logger.Info("VeleroBackup successfully created")
	}

	veleroBackupLogger := logger.WithValues("VeleroBackup", types.NamespacedName{Name: veleroBackupNameUUID, Namespace: r.OADPNamespace})

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
		return false, nil
	}

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
