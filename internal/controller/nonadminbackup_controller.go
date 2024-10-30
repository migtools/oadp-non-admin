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
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"github.com/vmware-tanzu/velero/pkg/builder"
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
	phaseUpdateRequeue           = "NonAdminBackup - Requeue after Phase Update"
	conditionUpdateRequeue       = "NonAdminBackup - Requeue after Condition Update"
	veleroReferenceUpdateRequeue = "NonAdminBackup - Requeue after Status Update with UUID reference"
	statusUpdateExit             = "NonAdminBackup - Exit after Status Update"
	statusUpdateError            = "Failed to update NonAdminBackup Status"
	findSingleVBError            = "Failed to find single VeleroBackup object"
	findSingleVDBRError          = "Failed to find single DeleteBackupRequest object"
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
		r.setStatusAndConditionForDeletion,
		r.createVeleroDeleteBackupRequestDeletion,
		r.removeVeleroBackupAndDeleteBackupRequestForceDeletion,
		r.initNabDeletion,
		r.finalizeNabDeletion,
		r.initNabCreate,
		r.validateSpec,
		r.setBackupUUIDInStatus,
		r.setFinalizerOnNonAdminBackup,
		r.createVeleroBackupAndSyncWithNonAdminBackup,
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

func (r *NonAdminBackupReconciler) setStatusAndConditionForDeletion(ctx context.Context, logger logr.Logger, nab *nacv1alpha1.NonAdminBackup) (bool, error) {
	// Check if DeleteBackup is set and if it wasn't already marked for deletion
	if nab.Spec.ForceDeleteBackup || nab.Spec.DeleteBackup && controllerutil.ContainsFinalizer(nab, constant.NabFinalizerName) {
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
			return true, nil
		}
	} else if !nab.Spec.DeleteBackup {
		// Check if the delete condition was previously set to True
		if meta.IsStatusConditionTrue(nab.Status.Conditions, string(nacv1alpha1.NonAdminConditionDeleting)) {
			// Set the deletion condition to canceled

			updatedCondition := meta.SetStatusCondition(&nab.Status.Conditions,
				metav1.Condition{
					Type:    string(nacv1alpha1.NonAdminConditionDeleting),
					Status:  metav1.ConditionFalse,
					Reason:  "DeletionCanceled",
					Message: "backup deletion was canceled",
				},
			)
			if updatedCondition {
				if err := r.Status().Update(ctx, nab); err != nil {
					logger.Error(err, statusUpdateError)
					return false, err
				}
				logger.V(1).Info("NonAdminBackup deletion canceled - updating condition")
				return true, nil
			}
		}
	}
	return false, nil
}

// createVeleroDeleteBackupRequestDeletion initiates deletion of the associated VeleroBackup object if
// the NonAdminBackup (NAB) object has DeleteBackup set to true and contains the required finalizer.
// This ensures the VeleroBackup is deleted before the NAB object itself is removed.
//
// Parameters:
//   - ctx: Context to manage request lifetime.
//   - logger: Logger instance for logging messages.
//   - nab: Pointer to the NonAdminBackup object to be managed.
//
// This function first checks if the DeleteBackup field in the NAB spec is set to true and confirms
// that the NAB object has the finalizer. If both conditions are met, it attempts to locate the associated
// VeleroBackup object by the UUID stored in the NAB status. If a unique VeleroBackup object is found,
// deletion is initiated, and the reconcile loop will be requeued. If multiple VeleroBackup objects
// are found with the same label, the function logs an error and returns.
//
// Returns:
//   - A boolean indicating whether to requeue the reconcile loop,
//   - An error if VeleroBackup deletion or retrieval fails, for example, due to multiple VeleroBackup objects found with the same UUID label.
func (r *NonAdminBackupReconciler) createVeleroDeleteBackupRequestDeletion(ctx context.Context, logger logr.Logger, nab *nacv1alpha1.NonAdminBackup) (bool, error) {
	// Initiate deletion of the VeleroBackup object only when DeleteBackup is set and the finalizer exists.
	// For the ForceDelete we do not create DeleteBackupRequest
	if !nab.Spec.ForceDeleteBackup && nab.Spec.DeleteBackup && controllerutil.ContainsFinalizer(nab, constant.NabFinalizerName) {
		veleroBackupNameUUID := nab.Status.VeleroBackup.NameUUID
		if veleroBackupNameUUID != constant.EmptyString {
			veleroBackup, err := function.GetVeleroBackupByLabel(ctx, r.Client, r.OADPNamespace, veleroBackupNameUUID)

			if err != nil {
				// Log error if multiple VeleroBackup objects are found
				logger.Error(err, findSingleVBError, uuidString, veleroBackupNameUUID)
				return false, err
			}

			if veleroBackup != nil {
				deleteBackupRequest, err := function.GetVeleroDeleteBackupRequestByLabel(ctx, r.Client, r.OADPNamespace, veleroBackupNameUUID)
				if err != nil {
					// Log error if multiple DeleteBackupRequest objects are found
					logger.Error(err, findSingleVDBRError, uuidString, veleroBackupNameUUID)
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
								constant.NabOriginNameUUIDLabel, veleroBackupNameUUID,
							),
							builder.WithLabelsMap(function.GetNonAdminLabels()),
							builder.WithAnnotationsMap(function.GetNonAdminBackupAnnotations(nab.ObjectMeta)),
							builder.WithGenerateName(veleroBackup.Name+"-"),
						).Result()

					// Use CreateRetryGenerateName for retry logic in creating the delete request
					if err := function.CreateRetryGenerateName(r.Client, ctx, deleteBackupRequest); err != nil {
						logger.Error(err, "Failed to create delete request for VeleroBackup", "BackupName", veleroBackup.Name)
						return false, err
					}
					logger.V(1).Info("Request to delete backup submitted successfully", "BackupName", veleroBackup.Name)
					nab.Status.VeleroDeleteBackupRequest = &nacv1alpha1.VeleroDeleteBackupRequest{
						NameUUID:  veleroBackupNameUUID,
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
				}
			} else {
				logger.V(1).Info("VeleroBackup already deleted", "UUID", veleroBackupNameUUID)
			}
		}
	}
	return false, nil // Continue so initNabDeletion can initialize deletion of a NonAdminBackup object
}

func (r *NonAdminBackupReconciler) removeVeleroBackupAndDeleteBackupRequestForceDeletion(ctx context.Context, logger logr.Logger, nab *nacv1alpha1.NonAdminBackup) (bool, error) {
	if nab.Spec.ForceDeleteBackup {
		if nab.Status.VeleroBackup != nil {
			veleroBackupNameUUID := nab.Status.VeleroBackup.NameUUID
			if veleroBackupNameUUID != constant.EmptyString {
				veleroBackup, err := function.GetVeleroBackupByLabel(ctx, r.Client, r.OADPNamespace, veleroBackupNameUUID)

				if err != nil {
					// Case where more than one VeleroBackup is found with the same label UUID
					// TODO: Determine if all objects with this UUID should be deleted
					logger.Error(err, findSingleVBError, uuidString, veleroBackupNameUUID)
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

				deleteBackupRequest, err := function.GetVeleroDeleteBackupRequestByLabel(ctx, r.Client, r.OADPNamespace, veleroBackupNameUUID)
				if err != nil {
					// Log error if multiple DeleteBackupRequest objects are found
					logger.Error(err, findSingleVDBRError, uuidString, veleroBackupNameUUID)
					return false, err
				}
				if deleteBackupRequest != nil {
					if err = r.Delete(ctx, deleteBackupRequest); err != nil {
						logger.Error(err, "Failed to delete VeleroDeleteBackupRequest", nameString, deleteBackupRequest.Name)
						return false, err
					}
					logger.V(1).Info("VeleroDeleteBackupRequest deletion initiated", nameString, deleteBackupRequest.Name)
				}
			}
		}
	}
	return false, nil // Continue so initNabDeletion can initialize deletion of an NonAdminBackup object
}

// initNabDeletion initiates the deletion of the NonAdminBackup (NAB) object if it has been marked
// with DeleteBackup in its spec.
//
// Parameters:
//   - ctx: Context for managing request lifetime.
//   - logger: Logger instance for logging messages.
//   - nab: Pointer to the NonAdminBackup object to be deleted.
//
// This function first checks if DeleteBackup is set to true in the NAB spec and if the object does not already
// have a DeletionTimestamp, indicating that deletion has not yet been marked. If these conditions are met,
// it attempts to delete the NAB object and requeues the reconcile loop to allow finalizers to proceed with cleanup.
// If the NAB object already has a DeletionTimestamp, the function logs that the object is already marked for deletion,
// avoiding redundant delete operations.
//
// Returns:
//   - A boolean indicating whether to requeue the reconcile loop (true if deletion is initiated, false otherwise).
//   - An error if deletion of the NAB object fails.
func (r *NonAdminBackupReconciler) initNabDeletion(ctx context.Context, logger logr.Logger, nab *nacv1alpha1.NonAdminBackup) (bool, error) {
	// Check if DeleteBackup is set and if it wasn't already marked for deletion
	if (nab.Spec.ForceDeleteBackup || nab.Spec.DeleteBackup) && nab.ObjectMeta.DeletionTimestamp.IsZero() {
		logger.V(1).Info("Marking NonAdminBackup for deletion", nameString, nab.Name)
		if err := r.Delete(ctx, nab); err != nil {
			logger.Error(err, "Failed to mark NonAdminBackup for deletion")
			return false, err
		}
		return true, nil // Requeue to allow deletion to proceed
	} else if !nab.ObjectMeta.DeletionTimestamp.IsZero() {
		logger.V(1).Info("NonAdminBackup already marked for deletion")
	}
	return false, nil // Continue so finalizeNabDeletion can delete NonAdminBackup object
}

// finalizeNabDeletion ensures the associated VeleroBackup object is deleted and removes
// the finalizer from the NonAdminBackup (NAB) object to complete its cleanup process.
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
func (r *NonAdminBackupReconciler) finalizeNabDeletion(ctx context.Context, logger logr.Logger, nab *nacv1alpha1.NonAdminBackup) (bool, error) {
	// Check if DeleteBackup is set or NAB was deleted
	if !nab.ObjectMeta.DeletionTimestamp.IsZero() {
		if !nab.Spec.ForceDeleteBackup && nab.Status.VeleroBackup != nil && nab.Status.VeleroBackup.NameUUID != constant.EmptyString {
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
	// If the NonAdminBackup is marked for deletion (either by DeleteBackup in the spec or with a DeletionTimestamp), skip initNabCreate
	if nab.Spec.DeleteBackup || !nab.ObjectMeta.DeletionTimestamp.IsZero() {
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
	// If the NonAdminBackup is marked for deletion (either by DeleteBackup in the spec or with a DeletionTimestamp), skip validateSpec
	if nab.Spec.DeleteBackup || !nab.ObjectMeta.DeletionTimestamp.IsZero() {
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
	// If the NonAdminBackup is marked for deletion (either by DeleteBackup in the spec or with a DeletionTimestamp), skip setBackupUUIDInStatus
	if nab.Spec.DeleteBackup || !nab.ObjectMeta.DeletionTimestamp.IsZero() {
		return false, nil
	}

	if nab.Status.VeleroBackup == nil || nab.Status.VeleroBackup.NameUUID == constant.EmptyString {
		veleroBackupNameUUID := function.GenerateNacObjectNameWithUUID(nab.Namespace, nab.Name)
		nab.Status.VeleroBackup = &nacv1alpha1.VeleroBackup{
			NameUUID:  veleroBackupNameUUID,
			Namespace: r.OADPNamespace,
			Name:      veleroBackupNameUUID,
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

func (r *NonAdminBackupReconciler) setFinalizerOnNonAdminBackup(ctx context.Context, logger logr.Logger, nab *nacv1alpha1.NonAdminBackup) (bool, error) {
	// If the NonAdminBackup is marked for deletion (either by DeleteBackup in the spec or with a DeletionTimestamp)
	// skip setFinalizerOnNonAdminBackup as we don't want to add finalizer again
	if nab.Spec.DeleteBackup || !nab.ObjectMeta.DeletionTimestamp.IsZero() {
		return false, nil
	}

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
		return true, nil // Requeue after adding finalizer
	}
	logger.V(1).Info("Finalizer exists on the NonAdminBackup object", "finalizer", constant.NabFinalizerName)
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
	// If the NonAdminBackup is marked for deletion (either by DeleteBackup in the spec or with a DeletionTimestamp), skip createVeleroBackupAndSyncWithNonAdminBackup
	if nab.Spec.DeleteBackup || !nab.ObjectMeta.DeletionTimestamp.IsZero() {
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

		// Skip the createVeleroBackupAndSyncWithNonAdminBackup, due to finalizer missing
		if !controllerutil.ContainsFinalizer(nab, constant.NabFinalizerName) {
			return false, fmt.Errorf("NonAdminBackup object is missing finalizer, can not continue with VeleroBackup creation")
		}

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
