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
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
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
	Scheme        *runtime.Scheme
	OADPNamespace string
}

type nonAdminRestoreReconcileStepFunction func(ctx context.Context, logger logr.Logger, nar *nacv1alpha1.NonAdminRestore) (bool, error)

//+kubebuilder:rbac:groups=oadp.openshift.io,resources=nonadminrestores,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=oadp.openshift.io,resources=nonadminrestores/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=oadp.openshift.io,resources=nonadminrestores/finalizers,verbs=update

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

	if !nar.DeletionTimestamp.IsZero() {
		updatedPhase := updateNonAdminPhase(&nar.Status.Phase, nacv1alpha1.NonAdminPhaseDeleting)
		updatedCondition := meta.SetStatusCondition(&nar.Status.Conditions,
			metav1.Condition{
				Type:    string(constant.NonAdminConditionDeleting),
				Status:  metav1.ConditionTrue,
				Reason:  "DeletionPending",
				Message: "restore accepted for deletion",
			},
		)
		if updatedPhase || updatedCondition {
			if err := r.Status().Update(ctx, nar); err != nil {
				logger.Error(err, statusUpdateError)
				return ctrl.Result{}, err
			}
			logger.V(1).Info("NonAdminRestore status marked for deletion")
		} else {
			logger.V(1).Info("NonAdminRestore status unchanged during deletion")
		}

		veleroRestore, err := function.GetVeleroRestoreByLabel(ctx, r.Client, r.OADPNamespace, nar.Status.UUID)
		if err != nil {
			if !apierrors.IsNotFound(err) {
				logger.Error(err, "Unable to fetch Velero Restore")
				return ctrl.Result{}, err
			}
		} else {
			// 	TODO when/why Velero Restore has finalizer?
			// 	TODO how to safely remove Velero Restore with finalizer?
			for _, finalizer := range veleroRestore.GetFinalizers() {
				controllerutil.RemoveFinalizer(veleroRestore, finalizer)
			}
			// TODO does this change generation? need to refetch?
			if err := r.Update(ctx, veleroRestore); err != nil {
				return ctrl.Result{}, err
			}
			err = r.Delete(ctx, veleroRestore)
			if err != nil {
				logger.Error(err, "Unable to delete Velero Restore")
				return ctrl.Result{}, err
			}
		}

		controllerutil.RemoveFinalizer(nar, constant.NonAdminRestoreFinalizerName)
		// TODO does this change generation? need to refetch?
		if err := r.Update(ctx, nar); err != nil {
			return ctrl.Result{}, err
		}
		logger.V(1).Info("NonAdminRestore Reconcile exit")
		return ctrl.Result{}, nil
	}

	reconcileSteps := []nonAdminRestoreReconcileStepFunction{
		r.init,
		r.validateSpec,
		r.setUUID,
		r.setFinalizer,
		r.createVeleroRestore,
	}
	for _, step := range reconcileSteps {
		requeue, err := step(ctx, logger, nar)
		if err != nil {
			return ctrl.Result{}, err
		} else if requeue {
			// TODO needed?
			return ctrl.Result{Requeue: true}, nil
		}
	}
	logger.V(1).Info("NonAdminRestore Reconcile exit")
	return ctrl.Result{}, nil
}

func (r *NonAdminRestoreReconciler) init(ctx context.Context, logger logr.Logger, nar *nacv1alpha1.NonAdminRestore) (bool, error) {
	if nar.Status.Phase == constant.EmptyString {
		if updated := updateNonAdminPhase(&nar.Status.Phase, nacv1alpha1.NonAdminPhaseNew); updated {
			if err := r.Status().Update(ctx, nar); err != nil {
				logger.Error(err, statusUpdateError)
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
	err := function.ValidateRestoreSpec(nar)
	if err != nil {
		updatedPhase := updateNonAdminPhase(&nar.Status.Phase, nacv1alpha1.NonAdminPhaseBackingOff)
		updatedCondition := meta.SetStatusCondition(&nar.Status.Conditions,
			metav1.Condition{
				Type:    string(constant.NonAdminConditionAccepted),
				Status:  metav1.ConditionFalse,
				Reason:  "InvalidRestoreSpec",
				Message: err.Error(),
			},
		)
		if updatedPhase || updatedCondition {
			if updateErr := r.Status().Update(ctx, nar); updateErr != nil {
				logger.Error(updateErr, statusUpdateError)
				return false, updateErr
			}
		}
		return false, reconcile.TerminalError(err)
	}
	logger.V(1).Info("NonAdminRestore Spec validated")

	updated := meta.SetStatusCondition(&nar.Status.Conditions,
		metav1.Condition{
			Type:    string(constant.NonAdminConditionAccepted),
			Status:  metav1.ConditionTrue,
			Reason:  "RestoreAccepted",
			Message: "restore accepted",
		},
	)
	if updated {
		if err := r.Status().Update(ctx, nar); err != nil {
			logger.Error(err, statusUpdateError)
			return false, err
		}
		logger.V(1).Info("NonAdminRestore condition set to Accepted")
	}
	return false, nil
}

func (r *NonAdminRestoreReconciler) setUUID(ctx context.Context, logger logr.Logger, nar *nacv1alpha1.NonAdminRestore) (bool, error) {
	if nar.Status.UUID == constant.EmptyString {
		// TODO handle panic
		nar.Status.UUID = uuid.New().String()
		if err := r.Status().Update(ctx, nar); err != nil {
			logger.Error(err, statusUpdateError)
			return false, err
		}
		logger.V(1).Info(veleroReferenceUpdated)
	} else {
		logger.V(1).Info("NonAdminRestore already contains NAC UUID")
	}
	return false, nil
}

func (r *NonAdminRestoreReconciler) setFinalizer(ctx context.Context, logger logr.Logger, nar *nacv1alpha1.NonAdminRestore) (bool, error) {
	added := controllerutil.AddFinalizer(nar, constant.NonAdminRestoreFinalizerName)
	if added {
		if err := r.Update(ctx, nar); err != nil {
			logger.Error(err, "Failed to add finalizer")
			return false, err
		}
		// TODO does this change generation? need to refetch?
		logger.V(1).Info("Finalizer added to NonAdminRestore", "finalizer", constant.NonAdminRestoreFinalizerName)
	} else {
		logger.V(1).Info("Finalizer already exists on NonAdminRestore", "finalizer", constant.NonAdminRestoreFinalizerName)
	}
	return false, nil
}

func (r *NonAdminRestoreReconciler) createVeleroRestore(ctx context.Context, logger logr.Logger, nar *nacv1alpha1.NonAdminRestore) (bool, error) {
	nacUUID := nar.Status.UUID
	veleroRestore, err := function.GetVeleroRestoreByLabel(ctx, r.Client, r.OADPNamespace, nacUUID)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return false, err
		}

		// TODO how to add this to validation without doing GET call for NAB/Velero backup twice? add field in status?
		restoreSpec := nar.Spec.RestoreSpec.DeepCopy()
		nab := &nacv1alpha1.NonAdminBackup{}
		err = r.Get(ctx, types.NamespacedName{Namespace: nar.Namespace, Name: nar.Spec.RestoreSpec.BackupName}, nab)
		if err != nil {
			return false, err
		}
		// TODO better way to check readiness? simplify and ask user to pass velero backup name? (user has access to this info in nonAdminBackup status)
		if nab.Status.Phase != nacv1alpha1.NonAdminPhaseCreated {
			return false, fmt.Errorf("NonAdminBackup is not ready to use for restore")
		}
		veleroBackup, err := function.GetVeleroBackupByLabel(ctx, r.Client, r.OADPNamespace, nab.Status.VeleroBackup.NACUUID)
		if err != nil || veleroBackup == nil {
			return false, fmt.Errorf("velero Backup does not exist or multiple exist")
		}
		// TODO does velero validate if backup is ready to be restored?
		restoreSpec.BackupName = veleroBackup.Name

		veleroRestore = &velerov1.Restore{
			ObjectMeta: metav1.ObjectMeta{
				// TODO even though Kubernetes object name can be up to 253 char length
				// using generate name, object name will be 63 char length
				// it will add a random 5 char length to GenerateName prefix
				// if prefix is more than 58 char length, it is truncated
				GenerateName: function.GetGenerateNamePrefix(nar.Namespace, nar.Name),
				Namespace:    r.OADPNamespace,
				Labels:       function.GetNonAdminRestoreLabels(nar.Status.UUID),
				Annotations:  function.GetNonAdminRestoreAnnotations(nab.ObjectMeta),
			},
			Spec: *restoreSpec,
		}

		err = r.Create(ctx, veleroRestore)
		if err != nil {
			logger.Error(err, "Failed to create Velero Restore")
			return false, err
		}
		logger.Info("Velero Restore successfully created")
	}

	updatedPhase := updateNonAdminPhase(&nar.Status.Phase, nacv1alpha1.NonAdminPhaseCreated)
	updatedCondition := meta.SetStatusCondition(&nar.Status.Conditions,
		metav1.Condition{
			Type:    string(constant.NonAdminConditionQueued),
			Status:  metav1.ConditionTrue,
			Reason:  "RestoreScheduled", // TODO can this confuse user? scheduled -> queued?
			Message: "Created Velero Restore object",
		},
	)
	// TODO need to refetch velero restore because of generate name?
	updatedVeleroStatus := updateVeleroRestoreStatus(&nar.Status, veleroRestore)
	if updatedPhase || updatedCondition || updatedVeleroStatus {
		if err := r.Status().Update(ctx, nar); err != nil {
			logger.Error(err, statusUpdateError)
			return false, err
		}
		logger.V(1).Info("NonAdminRestore Status updated successfully")
	} else {
		logger.V(1).Info("NonAdminRestore Status unchanged during Velero Restore reconciliation")
	}

	return false, nil
}

func updateVeleroRestoreStatus(status *nacv1alpha1.NonAdminRestoreStatus, veleroRestore *velerov1.Restore) bool {
	if status.VeleroRestore == nil {
		status.VeleroRestore = &nacv1alpha1.VeleroRestore{
			Name:      veleroRestore.Name,
			Namespace: veleroRestore.Namespace,
			Status:    &veleroRestore.Status,
		}
		return true
	} else if status.VeleroRestore.Name != veleroRestore.Name ||
		status.VeleroRestore.Namespace != veleroRestore.Namespace ||
		!reflect.DeepEqual(status.VeleroRestore.Status, &veleroRestore.Status) {
		status.VeleroRestore.Name = veleroRestore.Name
		status.VeleroRestore.Namespace = veleroRestore.Namespace
		status.VeleroRestore.Status = &veleroRestore.Status
		return true
	}
	return false
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