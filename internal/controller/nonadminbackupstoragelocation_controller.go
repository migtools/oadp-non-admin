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
	"reflect"

	"github.com/go-logr/logr"
	oadpcommon "github.com/openshift/oadp-operator/pkg/common"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"github.com/vmware-tanzu/velero/pkg/builder"
	corev1 "k8s.io/api/core/v1"
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

const (
	veleroBSLReferenceUpdated  = "NonAdminBackupStorageLocation - Status Updated with UUID reference"
	statusBslUpdateError       = "Failed to update NonAdminBackupStorageLocation Status"
	findSingleVBSLSecretError  = "Error encountered while retrieving Velero BSL Secret for NABSL"
	failedUpdateStatusError    = "Failed to update status"
	failedUpdateConditionError = "Failed to update status condition"
)

// NonAdminBackupStorageLocationReconciler reconciles a NonAdminBackupStorageLocation object
type NonAdminBackupStorageLocationReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	EnforcedBslSpec *velerov1.BackupStorageLocationSpec
	OADPNamespace   string
}

type naBSLReconcileStepFunction func(ctx context.Context, logger logr.Logger, nabsl *nacv1alpha1.NonAdminBackupStorageLocation) (bool, error)

// +kubebuilder:rbac:groups=velero.io,resources=backupstoragelocations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=velero.io,resources=backupstoragelocations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete

// +kubebuilder:rbac:groups=oadp.openshift.io,resources=nonadminbackupstoragelocations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=oadp.openshift.io,resources=nonadminbackupstoragelocations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=oadp.openshift.io,resources=nonadminbackupstoragelocations/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *NonAdminBackupStorageLocationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.V(1).Info("NonAdminBackupStorageLocation Reconcile start")

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
	case !nabsl.ObjectMeta.DeletionTimestamp.IsZero():
		logger.V(1).Info("Executing direct deletion path")
		reconcileSteps = []naBSLReconcileStepFunction{
			r.initNaBSLDelete,
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
			r.setFinalizerOnNaBSL,
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
			}).
		Watches(&velerov1.BackupStorageLocation{}, &handler.VeleroBackupStorageLocationHandler{}).
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

	if err := r.Client.List(ctx, nonAdminBackupList, listOpts); err != nil {
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
	// Skip validation if not in New phase
	if nabsl.Status.Phase != nacv1alpha1.NonAdminPhaseNew {
		logger.V(1).Info("Skipping validation, not in New phase", constant.CurrentPhaseString, nabsl.Status.Phase)
		return false, nil
	}

	err := function.ValidateBslSpec(ctx, r.Client, nabsl)
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

	err = oadpcommon.UpdateBackupStorageLocation(veleroBsl, *nabsl.Spec.BackupStorageLocationSpec)
	if err != nil {
		logger.Error(err, "Failed to update VeleroBackupStorageLocation spec")
		return false, err
	}

	// NaBSL/BSL must have a unique prefix for proper function of the non-admin backup sync controller
	// 1. Check if user has specified the prefix as "foo" in NaBSL creation, then prefix used would be <non-admin-ns>/foo
	// 2. TODO use the value from enforced spec if specified by the admin, if specified as "bar" then prefix used would be <non-admin-ns>/bar
	// 3. If none of the above, then we will use the non-admin user's namespace name as prefix
	prefix := function.ComputePrefixForObjectStorage(nabsl.Namespace, nabsl.Spec.BackupStorageLocationSpec.ObjectStorage.Prefix)

	op, err := controllerutil.CreateOrUpdate(ctx, r.Client, veleroBsl, func() error {
		veleroBsl.Spec = *nabsl.Spec.BackupStorageLocationSpec.DeepCopy()

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
