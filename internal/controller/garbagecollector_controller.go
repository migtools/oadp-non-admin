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
	"time"

	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	nacv1alpha1 "github.com/migtools/oadp-non-admin/api/v1alpha1"
	"github.com/migtools/oadp-non-admin/internal/common/constant"
	"github.com/migtools/oadp-non-admin/internal/common/function"
	"github.com/migtools/oadp-non-admin/internal/predicate"
)

// GarbageCollectorReconciler reconciles Velero objects
type GarbageCollectorReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	OADPNamespace string
	Frequency     time.Duration
}

var previousGarbageCollectorRun *time.Time

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *GarbageCollectorReconciler) Reconcile(ctx context.Context, _ ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	labelSelector := client.MatchingLabels{
		constant.OadpLabel:      constant.OadpLabelValue,
		constant.ManagedByLabel: constant.ManagedByLabelValue,
	}

	if previousGarbageCollectorRun == nil || time.Now().After(previousGarbageCollectorRun.Add(r.Frequency)) {
		previousGarbageCollectorRun = ptr.To(time.Now())
		logger.V(1).Info("Garbage Collector Reconcile start")
		// TODO duplication in delete logic
		// TODO do deletion in parallel?

		// TODO delete secret as well?
		veleroBackupStorageLocationList := &velerov1.BackupStorageLocationList{}
		if err := r.List(ctx, veleroBackupStorageLocationList, client.InNamespace(r.OADPNamespace), labelSelector); err != nil {
			logger.Error(err, "Unable to fetch BackupStorageLocations in OADP namespace")
			return ctrl.Result{}, err
		}
		for _, backupStorageLocation := range veleroBackupStorageLocationList.Items {
			annotations := backupStorageLocation.GetAnnotations()
			// TODO check UUID as well?
			if !function.CheckVeleroBackupStorageLocationAnnotations(&backupStorageLocation) {
				logger.V(1).Info("BackupStorageLocation does not have required annotations", constant.NameString, backupStorageLocation.Name)
				// TODO delete?
				continue
			}
			nabsl := &nacv1alpha1.NonAdminBackupStorageLocation{}
			err := r.Get(ctx, types.NamespacedName{
				Name:      annotations[constant.NabslOriginNameAnnotation],
				Namespace: annotations[constant.NabslOriginNamespaceAnnotation],
			}, nabsl)
			if err != nil {
				if !apierrors.IsNotFound(err) {
					logger.Error(err, "Unable to fetch NonAdminBackupStorageLocation")
					return ctrl.Result{}, err
				}
				if err = r.Delete(ctx, &backupStorageLocation); err != nil {
					logger.Error(err, "Failed to delete orphan BackupStorageLocation", constant.NameString, backupStorageLocation.Name)
					return ctrl.Result{}, err
				}
				logger.V(1).Info("orphan BackupStorageLocation deleted", constant.NameString, backupStorageLocation.Name)
			}
		}

		veleroBackupList := &velerov1.BackupList{}
		if err := r.List(ctx, veleroBackupList, client.InNamespace(r.OADPNamespace), labelSelector); err != nil {
			logger.Error(err, "Unable to fetch Backups in OADP namespace")
			return ctrl.Result{}, err
		}
		for _, backup := range veleroBackupList.Items {
			annotations := backup.GetAnnotations()
			// TODO check UUID as well?
			if !function.CheckVeleroBackupAnnotations(&backup) {
				logger.V(1).Info("Backup does not have required annotations", constant.NameString, backup.Name)
				// TODO delete?
				continue
			}
			nab := &nacv1alpha1.NonAdminBackup{}
			err := r.Get(ctx, types.NamespacedName{
				Name:      annotations[constant.NabOriginNameAnnotation],
				Namespace: annotations[constant.NabOriginNamespaceAnnotation],
			}, nab)
			if err != nil {
				if !apierrors.IsNotFound(err) {
					logger.Error(err, "Unable to fetch NonAdminBackup")
					return ctrl.Result{}, err
				}
				if err = r.Delete(ctx, &backup); err != nil {
					logger.Error(err, "Failed to delete orphan backup", constant.NameString, backup.Name)
					return ctrl.Result{}, err
				}
				logger.V(1).Info("orphan Backup deleted", constant.NameString, backup.Name)
			}
		}

		veleroRestoreList := &velerov1.RestoreList{}
		if err := r.List(ctx, veleroRestoreList, client.InNamespace(r.OADPNamespace), labelSelector); err != nil {
			logger.Error(err, "Unable to fetch Restores in OADP namespace")
			return ctrl.Result{}, err
		}
		for _, restore := range veleroRestoreList.Items {
			annotations := restore.GetAnnotations()
			// TODO check UUID as well?
			if !function.CheckVeleroRestoreAnnotations(&restore) {
				logger.V(1).Info("Restore does not have required annotations", constant.NameString, restore.Name)
				// TODO delete?
				continue
			}
			nar := &nacv1alpha1.NonAdminRestore{}
			err := r.Get(ctx, types.NamespacedName{
				Name:      annotations[constant.NarOriginNameAnnotation],
				Namespace: annotations[constant.NarOriginNamespaceAnnotation],
			}, nar)
			if err != nil {
				if !apierrors.IsNotFound(err) {
					logger.Error(err, "Unable to fetch NonAdminRestore")
					return ctrl.Result{}, err
				}
				if err = r.Delete(ctx, &restore); err != nil {
					logger.Error(err, "Failed to delete orphan Restore", constant.NameString, restore.Name)
					return ctrl.Result{}, err
				}
				logger.V(1).Info("orphan Restore deleted", constant.NameString, restore.Name)
			}
		}

		logger.V(1).Info("Garbage Collector Reconcile end")
		return ctrl.Result{RequeueAfter: r.Frequency}, nil
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GarbageCollectorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&velerov1.BackupStorageLocation{}).
		WithEventFilter(predicate.GarbageCollectorPredicate{}).
		Complete(r)
}
