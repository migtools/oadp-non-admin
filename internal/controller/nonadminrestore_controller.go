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

// NonAdminRestoreReconciler reconciles a NonAdminRestore object
type NonAdminRestoreReconciler struct {
	client.Client
	Scheme              *runtime.Scheme
	EnforcedRestoreSpec *velerov1.RestoreSpec
	OADPNamespace       string
}

type nonAdminRestoreReconcileStepFunction func(ctx context.Context, logger logr.Logger, nar *nacv1alpha1.NonAdminRestore) (bool, error)

const (
	nonAdminRestoreStatusUpdateFailureMessage = "Failed to update NonAdminRestore Status"
	veleroRestoreReferenceUpdated             = "NonAdminRestore - Status Updated with UUID reference"
	findSingleVRError                         = "Error encountered while retrieving VeleroRestore for NAR"
)

// +kubebuilder:rbac:groups=oadp.openshift.io,resources=nonadminrestores,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=oadp.openshift.io,resources=nonadminrestores/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=oadp.openshift.io,resources=nonadminrestores/finalizers,verbs=update

// +kubebuilder:rbac:groups=velero.io,resources=restores,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state,
// defined in NonAdminRestore object Spec.
func (r *NonAdminRestoreReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.V(1).Info("NonAdminRestore Reconcile start")

	nar := &nacv1alpha1.NonAdminRestore{}
	err := r.Get(ctx, req.NamespacedName, nar)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(1).Info(err.Error())
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Unable to fetch NonAdminRestore")
		return ctrl.Result{}, err
	}

	var reconcileSteps []nonAdminRestoreReconcileStepFunction

	switch {
	case !nar.DeletionTimestamp.IsZero():
		logger.V(1).Info("Executing delete path")
		reconcileSteps = []nonAdminRestoreReconcileStepFunction{
			r.setStatusAndConditionForDeletionAndCallDelete,
			r.deleteVeleroRestore,
			r.removeNarFinalizerUponVeleroRestoreDeletion,
		}
	default:
		logger.V(1).Info("Executing creation/update path")
		reconcileSteps = []nonAdminRestoreReconcileStepFunction{
			r.init,
			r.validateSpec,
			r.setUUID,
			r.setFinalizer,
			r.createVeleroRestore,
		}
	}

	// Execute the selected reconciliation steps
	for _, step := range reconcileSteps {
		requeue, err := step(ctx, logger, nar)
		if err != nil {
			return ctrl.Result{}, err
		} else if requeue {
			return ctrl.Result{Requeue: true}, nil
		}
	}

	logger.V(1).Info("NonAdminRestore Reconcile exit")
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
func (r *NonAdminRestoreReconciler) setStatusAndConditionForDeletionAndCallDelete(ctx context.Context, logger logr.Logger, nar *nacv1alpha1.NonAdminRestore) (bool, error) {
	requeueRequired := false
	updatedPhase := updateNonAdminPhase(&nar.Status.Phase, nacv1alpha1.NonAdminPhaseDeleting)
	updatedCondition := meta.SetStatusCondition(&nar.Status.Conditions,
		metav1.Condition{
			Type:    string(nacv1alpha1.NonAdminConditionDeleting),
			Status:  metav1.ConditionTrue,
			Reason:  "DeletionPending",
			Message: "restore accepted for deletion",
		},
	)
	if updatedPhase || updatedCondition {
		if err := r.Status().Update(ctx, nar); err != nil {
			logger.Error(err, statusUpdateError)
			return false, err
		}
		logger.V(1).Info("NonAdminRestore status marked for deletion")
		requeueRequired = true // Requeue to ensure latest NAR object in the next reconciliation steps
	} else {
		logger.V(1).Info("NonAdminRestore status unchanged during deletion")
	}
	if nar.ObjectMeta.DeletionTimestamp.IsZero() {
		logger.V(1).Info("Marking NonAdminRestore for deletion", constant.NameString, nar.Name)
		if err := r.Delete(ctx, nar); err != nil {
			logger.Error(err, "Failed to call Delete on the NonAdminRestore object")
			return false, err
		}
		requeueRequired = true // Requeue to allow deletion to proceed
	}
	return requeueRequired, nil
}

func (r *NonAdminRestoreReconciler) deleteVeleroRestore(ctx context.Context, logger logr.Logger, nar *nacv1alpha1.NonAdminRestore) (bool, error) {
	if nar.Status.VeleroRestore == nil || nar.Status.VeleroRestore.NACUUID == constant.EmptyString {
		return false, nil
	}

	veleroRestoreNACUUID := nar.Status.VeleroRestore.NACUUID

	veleroRestore, err := function.GetVeleroRestoreByLabel(ctx, r.Client, r.OADPNamespace, veleroRestoreNACUUID)

	if err != nil {
		// Case in which more then one VeleroRestore is found with the same label NACUUID
		logger.Error(err, findSingleVRError, constant.UUIDString, veleroRestoreNACUUID)
		return false, err
	}

	if veleroRestore != nil {
		// All the data within VeleroRestore is stored in object storage, so veleroRestore deletion is not blocking
		// and it will get removed by the Velero cleanup process when the restore object gets deleted
		// https://github.com/vmware-tanzu/velero/blob/074f26539d3eb06c7b1a6af9b4975254e61b956c/pkg/cmd/cli/restore/delete.go#L122
		if err = r.Delete(ctx, veleroRestore); err != nil {
			logger.Error(err, "Failed to delete VeleroRestore", constant.NameString, veleroRestore.Name)
			return false, err
		}
		logger.V(1).Info("VeleroRestore deletion initiated", constant.NameString, veleroRestore.Name)
	} else {
		logger.V(1).Info("VeleroRestore already deleted")
	}
	return false, nil // Continue
}

func (r *NonAdminRestoreReconciler) removeNarFinalizerUponVeleroRestoreDeletion(ctx context.Context, logger logr.Logger, nar *nacv1alpha1.NonAdminRestore) (bool, error) {
	if !nar.ObjectMeta.DeletionTimestamp.IsZero() {
		if nar.Status.VeleroRestore != nil && nar.Status.VeleroRestore.NACUUID != constant.EmptyString {
			veleroRestoreNACUUID := nar.Status.VeleroRestore.NACUUID

			veleroRestore, err := function.GetVeleroRestoreByLabel(ctx, r.Client, r.OADPNamespace, veleroRestoreNACUUID)
			if err != nil {
				// Case in which more then one VeleroRestore is found with the same label UUID
				logger.Error(err, findSingleVRError, constant.UUIDString, veleroRestoreNACUUID)
				return false, err
			}

			if veleroRestore != nil {
				logger.V(1).Info("Waiting for VeleroRestore to be deleted", constant.NameString, veleroRestoreNACUUID)
				return true, nil // Requeue
			}
		}
		// VeleroRestore is deleted, proceed with deleting the NonAdminRestore
		logger.V(1).Info("VeleroRestore deleted, removing NonAdminRestore finalizer")

		controllerutil.RemoveFinalizer(nar, constant.NarFinalizerName)

		if err := r.Update(ctx, nar); err != nil {
			logger.Error(err, "Failed to remove finalizer from NonAdminRestore")
			return false, err
		}

		logger.V(1).Info("NonAdminRestore finalizer removed and object deleted")
	}
	return false, nil
}

func (r *NonAdminRestoreReconciler) init(ctx context.Context, logger logr.Logger, nar *nacv1alpha1.NonAdminRestore) (bool, error) {
	if nar.Status.Phase == constant.EmptyString {
		if updated := updateNonAdminPhase(&nar.Status.Phase, nacv1alpha1.NonAdminPhaseNew); updated {
			if err := r.Status().Update(ctx, nar); err != nil {
				logger.Error(err, nonAdminRestoreStatusUpdateFailureMessage)
				return false, err
			}
			logger.V(1).Info("NonAdminRestore Phase set to New")
		}
	} else {
		logger.V(1).Info("NonAdminRestore Phase already initialized")
	}
	return false, nil
}

func (r *NonAdminRestoreReconciler) validateSpec(ctx context.Context, logger logr.Logger, nar *nacv1alpha1.NonAdminRestore) (bool, error) {
	err := function.ValidateRestoreSpec(ctx, r.Client, nar, r.EnforcedRestoreSpec)
	if err != nil {
		updatedPhase := updateNonAdminPhase(&nar.Status.Phase, nacv1alpha1.NonAdminPhaseBackingOff)
		updatedCondition := meta.SetStatusCondition(&nar.Status.Conditions,
			metav1.Condition{
				Type:    string(nacv1alpha1.NonAdminConditionAccepted),
				Status:  metav1.ConditionFalse,
				Reason:  "InvalidRestoreSpec",
				Message: err.Error(),
			},
		)
		if updatedPhase || updatedCondition {
			if updateErr := r.Status().Update(ctx, nar); updateErr != nil {
				logger.Error(updateErr, nonAdminRestoreStatusUpdateFailureMessage)
				return false, updateErr
			}
		}
		return false, reconcile.TerminalError(err)
	}
	logger.V(1).Info("NonAdminRestore Spec validated")

	updated := meta.SetStatusCondition(&nar.Status.Conditions,
		metav1.Condition{
			Type:    string(nacv1alpha1.NonAdminConditionAccepted),
			Status:  metav1.ConditionTrue,
			Reason:  "RestoreAccepted",
			Message: "restore accepted",
		},
	)
	if updated {
		if err := r.Status().Update(ctx, nar); err != nil {
			logger.Error(err, nonAdminRestoreStatusUpdateFailureMessage)
			return false, err
		}
		logger.V(1).Info("NonAdminRestore condition set to Accepted")
	}
	return false, nil
}

func (r *NonAdminRestoreReconciler) setUUID(ctx context.Context, logger logr.Logger, nar *nacv1alpha1.NonAdminRestore) (bool, error) {
	// Get the latest version of the NAR object just before checking if the NACUUID is set
	// to ensure we do not miss any updates to the NAR object
	narOriginal := nar.DeepCopy()
	if err := r.Get(ctx, types.NamespacedName{Name: narOriginal.Name, Namespace: narOriginal.Namespace}, nar); err != nil {
		logger.Error(err, "Failed to re-fetch NonAdminRestore")
		return false, err
	}

	if nar.Status.VeleroRestore == nil || nar.Status.VeleroRestore.NACUUID == constant.EmptyString {
		veleroRestoreNACUUID := function.GenerateNacObjectUUID(nar.Namespace, nar.Name)
		nar.Status.VeleroRestore = &nacv1alpha1.VeleroRestore{
			NACUUID:   veleroRestoreNACUUID,
			Namespace: r.OADPNamespace,
			Name:      veleroRestoreNACUUID,
		}
		if err := r.Status().Update(ctx, nar); err != nil {
			logger.Error(err, nonAdminRestoreStatusUpdateFailureMessage)
			return false, err
		}
		logger.V(1).Info(veleroRestoreReferenceUpdated)
	} else {
		logger.V(1).Info("NonAdminRestore already contains VeleroRestore UUID reference")
	}
	return false, nil
}

func (r *NonAdminRestoreReconciler) setFinalizer(ctx context.Context, logger logr.Logger, nar *nacv1alpha1.NonAdminRestore) (bool, error) {
	added := controllerutil.AddFinalizer(nar, constant.NarFinalizerName)
	if added {
		if err := r.Update(ctx, nar); err != nil {
			logger.Error(err, "Failed to add finalizer")
			return false, err
		}
		logger.V(1).Info("Finalizer added to NonAdminRestore")
	} else {
		logger.V(1).Info("Finalizer already exists on NonAdminRestore")
	}
	return false, nil
}

func (r *NonAdminRestoreReconciler) createVeleroRestore(ctx context.Context, logger logr.Logger, nar *nacv1alpha1.NonAdminRestore) (bool, error) {
	if nar.Status.VeleroRestore == nil || nar.Status.VeleroRestore.NACUUID == constant.EmptyString {
		return false, errors.New("unable to get Velero Restore UUID from NonAdminRestore Status")
	}

	veleroRestoreNACUUID := nar.Status.VeleroRestore.NACUUID

	veleroRestore, err := function.GetVeleroRestoreByLabel(ctx, r.Client, r.OADPNamespace, veleroRestoreNACUUID)

	if err != nil {
		// Case in which more then one VeleroBackup is found with the same label UUID
		logger.Error(err, findSingleVRError, constant.UUIDString, veleroRestoreNACUUID)
		return false, err
	}

	if veleroRestore == nil {
		logger.Info("VeleroRestore with label not found, creating one", constant.UUIDString, veleroRestoreNACUUID)
		nab := &nacv1alpha1.NonAdminBackup{}
		err = r.Get(ctx, types.NamespacedName{Name: nar.Spec.RestoreSpec.BackupName, Namespace: nar.Namespace}, nab)
		if err != nil {
			logger.Error(err, "Failed to get NonAdminBackup referenced by NonAdminRestore")
			return false, err
		}

		restoreSpec := nar.Spec.RestoreSpec.DeepCopy()
		restoreSpec.BackupName = nab.Status.VeleroBackup.Name
		restoreSpec.IncludedNamespaces = []string{nar.Namespace}

		enforcedSpec := reflect.ValueOf(r.EnforcedRestoreSpec).Elem()
		for index := range enforcedSpec.NumField() {
			enforcedField := enforcedSpec.Field(index)
			enforcedFieldName := enforcedSpec.Type().Field(index).Name
			currentField := reflect.ValueOf(restoreSpec).Elem().FieldByName(enforcedFieldName)
			if !enforcedField.IsZero() && currentField.IsZero() {
				currentField.Set(enforcedField)
			}
		}

		restoreSpec.ExcludedResources = append(restoreSpec.ExcludedResources,
			"volumesnapshotclasses")

		veleroRestore := velerov1.Restore{
			ObjectMeta: metav1.ObjectMeta{
				Name:        veleroRestoreNACUUID,
				Namespace:   r.OADPNamespace,
				Labels:      function.GetNonAdminRestoreLabels(veleroRestoreNACUUID),
				Annotations: function.GetNonAdminRestoreAnnotations(nar.ObjectMeta),
			},
			Spec: *restoreSpec,
		}

		err = r.Create(ctx, &veleroRestore)

		if err != nil {
			// We do not retry here as the veleroRestoreNACUUID
			// should be guaranteed to be unique
			logger.Error(err, "Failed to create VeleroRestore")
			return false, err
		}
		logger.Info("VeleroRestore successfully created")
	}

	updatedQueueInfo := false

	// Determine how many Restores are scheduled before the given VeleroRestore in the OADP namespace.
	queueInfo, err := function.GetRestoreQueueInfo(ctx, r.Client, r.OADPNamespace, veleroRestore)
	if err != nil {
		// Log error and continue with the reconciliation, this is not critical error as it's just
		// about the Velero Restore queue position information
		logger.Error(err, "Failed to get the queue position for the VeleroRestore")
	} else {
		nar.Status.QueueInfo = &queueInfo
		updatedQueueInfo = true
	}

	updatedPhase := updateNonAdminPhase(&nar.Status.Phase, nacv1alpha1.NonAdminPhaseCreated)

	updatedCondition := meta.SetStatusCondition(&nar.Status.Conditions,
		metav1.Condition{
			Type:    string(nacv1alpha1.NonAdminConditionQueued),
			Status:  metav1.ConditionTrue,
			Reason:  "RestoreScheduled", // TODO can this confuse user? scheduled -> queued?
			Message: "Created Velero Restore object",
		},
	)

	updatedVeleroStatus := updateVeleroRestoreStatus(&nar.Status, veleroRestore)

	if updatedPhase || updatedCondition || updatedVeleroStatus || updatedQueueInfo {
		if err := r.Status().Update(ctx, nar); err != nil {
			logger.Error(err, nonAdminRestoreStatusUpdateFailureMessage)
			return false, err
		}
		logger.V(1).Info("NonAdminRestore Status updated successfully")
	} else {
		logger.V(1).Info("NonAdminRestore Status unchanged during Velero Restore reconciliation")
	}

	return false, nil
}

// updateVeleroRestoreStatus sets the VeleroRestore status field in NonAdminRestore object status and returns true
// if the VeleroRestore fields are changed by this call.
func updateVeleroRestoreStatus(status *nacv1alpha1.NonAdminRestoreStatus, veleroRestore *velerov1.Restore) bool {
	if status == nil || veleroRestore == nil {
		return false
	}

	if status.VeleroRestore == nil {
		status.VeleroRestore = &nacv1alpha1.VeleroRestore{}
	}

	if status.VeleroRestore.Status == nil {
		status.VeleroRestore.Status = &velerov1.RestoreStatus{}
	}

	if reflect.DeepEqual(*status.VeleroRestore.Status, veleroRestore.Status) {
		return false
	}

	status.VeleroRestore.Status = veleroRestore.Status.DeepCopy()
	return true
}

// SetupWithManager sets up the controller with the Manager.
func (r *NonAdminRestoreReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nacv1alpha1.NonAdminRestore{}).
		WithEventFilter(predicate.CompositeRestorePredicate{
			NonAdminRestorePredicate: predicate.NonAdminRestorePredicate{},
			VeleroRestorePredicate: predicate.VeleroRestorePredicate{
				OADPNamespace: r.OADPNamespace,
			},
		}).
		// handler runs after predicate
		Watches(&velerov1.Restore{}, &handler.VeleroRestoreHandler{}).
		Complete(r)
}
