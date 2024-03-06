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

	nacv1alpha1 "github.com/migtools/oadp-non-admin/api/v1alpha1"
	"github.com/migtools/oadp-non-admin/internal/common/constant"
	"github.com/migtools/oadp-non-admin/internal/common/function"
	"github.com/migtools/oadp-non-admin/internal/handler"
	"github.com/migtools/oadp-non-admin/internal/predicate"
)

// NonAdminBackupReconciler reconciles a NonAdminBackup object
type NonAdminBackupReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	Log            logr.Logger
	Context        context.Context
	NamespacedName types.NamespacedName
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
	r.Log = log.FromContext(ctx)
	logger := r.Log.WithValues("NonAdminBackup", req.NamespacedName)
	logger.V(1).Info(">>> Reconcile NonAdminBackup - loop start")

	// Resource version
	// Generation version - metadata - only one is updated...
	r.Context = ctx
	r.NamespacedName = req.NamespacedName

	// Get the Non Admin Backup object
	nab := nacv1alpha1.NonAdminBackup{}
	err := r.Get(ctx, req.NamespacedName, &nab)

	// Bail out when the Non Admin Backup reconcile was triggered, when the NAB got deleted
	// Reconcile loop was triggered when Velero Backup object got updated and NAB isn't there
	if err != nil && apierrors.IsNotFound(err) {
		logger.V(1).Info("Deleted NonAdminBackup CR", nameField, req.Name, constant.NameSpaceString, req.Namespace)
		return ctrl.Result{}, nil
	}

	if err != nil {
		logger.Error(err, "Unable to fetch NonAdminBackup CR", nameField, req.Name, constant.NameSpaceString, req.Namespace)
		return ctrl.Result{}, err
	}

	if nab.Status.Phase == constant.EmptyString {
		// Phase: New
		updatedStatus, errUpdate := function.UpdateNonAdminPhase(ctx, r.Client, logger, &nab, nacv1alpha1.NonAdminBackupPhaseNew)
		if updatedStatus {
			logger.V(1).Info("NonAdminBackup CR - Rqueue after Phase Update")
			return ctrl.Result{Requeue: true, RequeueAfter: requeueTimeSeconds * time.Second}, nil
		}
		if errUpdate != nil {
			logger.Error(errUpdate, "Unable to set NonAdminBackup Phase: New", nameField, req.Name, constant.NameSpaceString, req.Namespace)
			return ctrl.Result{}, errUpdate
		}
	}

	backupSpec, err := function.GetBackupSpecFromNonAdminBackup(&nab)

	if err != nil {
		errMsg := "NonAdminBackup CR does not contain valid BackupSpec"
		logger.Error(err, errMsg)

		// Phase: BackingOff
		// BackupAccepted: False
		// BackupQueued: False   # already set
		updatedStatus, errUpdate := function.UpdateNonAdminPhase(ctx, r.Client, logger, &nab, nacv1alpha1.NonAdminBackupPhaseBackingOff)
		if errUpdate != nil {
			logger.Error(errUpdate, "Unable to set NonAdminBackup Phase: BackingOff", nameField, req.Name, constant.NameSpaceString, req.Namespace)
			return ctrl.Result{}, errUpdate
		} else if updatedStatus {
			// We do not requeue as the State is BackingOff
			return ctrl.Result{}, nil
		}

		updatedStatus, errUpdate = function.UpdateNonAdminBackupCondition(ctx, r.Client, logger, &nab, nacv1alpha1.NonAdminBackupConditionAccepted, metav1.ConditionFalse, "invalid_backupspec", errMsg)
		if updatedStatus {
			return ctrl.Result{}, nil
		}

		if errUpdate != nil {
			logger.Error(errUpdate, "Unable to set BackupAccepted Condition: False", nameField, req.Name, constant.NameSpaceString, req.Namespace)
			return ctrl.Result{}, errUpdate
		}
		return ctrl.Result{}, err
	}

	// Phase: New               # already set
	// BackupAccepted: True
	// BackupQueued: False   # already set
	updatedStatus, errUpdate := function.UpdateNonAdminBackupCondition(ctx, r.Client, logger, &nab, nacv1alpha1.NonAdminBackupConditionAccepted, metav1.ConditionTrue, "backup_accepted", "backup accepted")
	if updatedStatus {
		return ctrl.Result{Requeue: true, RequeueAfter: requeueTimeSeconds * time.Second}, nil
	}

	if errUpdate != nil {
		logger.Error(errUpdate, "Unable to set BackupAccepted Condition: True", nameField, req.Name, constant.NameSpaceString, req.Namespace)
		return ctrl.Result{}, errUpdate
	}

	veleroBackupName := function.GenerateVeleroBackupName(nab.Namespace, nab.Name)

	if veleroBackupName == constant.EmptyString {
		return ctrl.Result{}, errors.New("unable to generate Velero Backup name")
	}

	veleroBackup := velerov1api.Backup{}
	err = r.Get(ctx, client.ObjectKey{Namespace: constant.OadpNamespace, Name: veleroBackupName}, &veleroBackup)

	if err != nil && apierrors.IsNotFound(err) {
		// Create backup
		// Don't update phase nor conditions yet.
		// Those will be updated when then Reconcile loop is triggered by the VeleroBackup object
		logger.Info("No backup found", nameField, veleroBackupName)
		veleroBackup = velerov1api.Backup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      veleroBackupName,
				Namespace: constant.OadpNamespace,
			},
			Spec: *backupSpec,
		}
	} else if err != nil && !apierrors.IsNotFound(err) {
		logger.Error(err, "Unable to fetch VeleroBackup")
		return ctrl.Result{}, err
	} else {
		// TODO: Currently when one updates VeleroBackup spec inside the NonAdminBackup
		//       We do not update already created VeleroBackup object.
		//       Should we update the previously created VeleroBackup object and reflect what was
		//       updated? In the current situation the VeleroBackup within NonAdminBackup will
		//       be reverted back to the previous state - the state which created VeleroBackup
		//       in a first place.
		logger.Info("Backup already exists, updating NonAdminBackup status", nameField, veleroBackupName)
		updatedNab, errBackupUpdate := function.UpdateNonAdminBackupFromVeleroBackup(ctx, r.Client, logger, &nab, &veleroBackup)
		// Regardless if the status was updated or not, we should not
		// requeue here as it was only status update.
		if errBackupUpdate != nil {
			return ctrl.Result{}, errBackupUpdate
		} else if updatedNab {
			logger.V(1).Info("NonAdminBackup CR - Rqueue after Status Update")
			return ctrl.Result{Requeue: true, RequeueAfter: requeueTimeSeconds * time.Second}, nil
		}
		return ctrl.Result{}, nil
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
		return ctrl.Result{}, err
	}
	logger.Info("VeleroBackup successfully created", nameField, veleroBackupName)

	// Phase: Created
	// BackupAccepted: True # do not update
	// BackupQueued: True
	_, errUpdate = function.UpdateNonAdminPhase(ctx, r.Client, logger, &nab, nacv1alpha1.NonAdminBackupPhaseCreated)
	if errUpdate != nil {
		logger.Error(errUpdate, "Unable to set NonAdminBackup Phase: Created", nameField, req.Name, constant.NameSpaceString, req.Namespace)
		return ctrl.Result{}, errUpdate
	}
	_, errUpdate = function.UpdateNonAdminBackupCondition(ctx, r.Client, logger, &nab, nacv1alpha1.NonAdminBackupConditionAccepted, metav1.ConditionTrue, "validated", "Valid Backup config")
	if errUpdate != nil {
		logger.Error(errUpdate, "Unable to set BackupAccepted Condition: True", nameField, req.Name, constant.NameSpaceString, req.Namespace)
		return ctrl.Result{}, errUpdate
	}
	_, errUpdate = function.UpdateNonAdminBackupCondition(ctx, r.Client, logger, &nab, nacv1alpha1.NonAdminBackupConditionQueued, metav1.ConditionTrue, "backup_scheduled", "Created Velero Backup object")
	if errUpdate != nil {
		logger.Error(errUpdate, "Unable to set BackupQueued Condition: True", nameField, req.Name, constant.NameSpaceString, req.Namespace)
		return ctrl.Result{}, errUpdate
	}

	logger.V(1).Info(">>> Reconcile NonAdminBackup - loop end")

	return ctrl.Result{}, nil
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
