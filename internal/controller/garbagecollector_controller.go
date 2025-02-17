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

	"github.com/go-logr/logr"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"golang.org/x/sync/errgroup"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	nacv1alpha1 "github.com/migtools/oadp-non-admin/api/v1alpha1"
	"github.com/migtools/oadp-non-admin/internal/common/constant"
	"github.com/migtools/oadp-non-admin/internal/common/function"
	"github.com/migtools/oadp-non-admin/internal/source"
)

// GarbageCollectorReconciler reconciles Velero objects
type GarbageCollectorReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	OADPNamespace string
	Frequency     time.Duration
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *GarbageCollectorReconciler) Reconcile(ctx context.Context, _ ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	labelSelector := client.MatchingLabels{
		constant.OadpLabel:      constant.OadpLabelValue,
		constant.ManagedByLabel: constant.ManagedByLabelValue,
	}

	logger.V(1).Info("Garbage Collector Reconcile start")

	var execution errgroup.Group

	// TODO duplication in delete logic
	execution.Go(func() error {
		secretList := &corev1.SecretList{}
		if err := r.List(ctx, secretList, client.InNamespace(r.OADPNamespace), labelSelector); err != nil {
			logger.Error(err, "Unable to fetch Secret in OADP namespace")
			return err
		}
		for _, secret := range secretList.Items {
			if !function.CheckLabelAnnotationValueIsValid(secret.GetLabels(), constant.NabslOriginNACUUIDLabel) {
				logger.V(1).Info("Secret does not have required label", constant.NameString, secret.Name)
				// TODO delete?
				continue
			}
			annotations := secret.GetAnnotations()
			if !function.CheckVeleroBackupStorageLocationAnnotations(&secret) {
				logger.V(1).Info("Secret does not have required annotations", constant.NameString, secret.Name)
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
					return err
				}
				if err = r.Delete(ctx, &secret); err != nil {
					logger.Error(err, "Failed to delete orphan Secret", constant.NameString, secret.Name)
					return err
				}
				logger.V(1).Info("orphan Secret deleted", constant.NameString, secret.Name)
			}
		}
		return nil
	})

	execution.Go(func() error {
		veleroBackupStorageLocationList := &velerov1.BackupStorageLocationList{}
		if err := r.List(ctx, veleroBackupStorageLocationList, client.InNamespace(r.OADPNamespace), labelSelector); err != nil {
			logger.Error(err, "Unable to fetch BackupStorageLocations in OADP namespace")
			return err
		}
		for _, backupStorageLocation := range veleroBackupStorageLocationList.Items {
			if !function.CheckLabelAnnotationValueIsValid(backupStorageLocation.GetLabels(), constant.NabslOriginNACUUIDLabel) {
				logger.V(1).Info("BackupStorageLocation does not have required label", constant.NameString, backupStorageLocation.Name)
				// TODO delete?
				continue
			}
			annotations := backupStorageLocation.GetAnnotations()
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
					return err
				}
				if err = r.Delete(ctx, &backupStorageLocation); err != nil {
					logger.Error(err, "Failed to delete orphan BackupStorageLocation", constant.NameString, backupStorageLocation.Name)
					return err
				}
				logger.V(1).Info("orphan BackupStorageLocation deleted", constant.NameString, backupStorageLocation.Name)
			}
		}
		return nil
	})

	execution.Go(func() error {
		veleroBackupList := &velerov1.BackupList{}
		if err := r.List(ctx, veleroBackupList, client.InNamespace(r.OADPNamespace), labelSelector); err != nil {
			logger.Error(err, "Unable to fetch Backups in OADP namespace")
			return err
		}
		for _, backup := range veleroBackupList.Items {
			if !function.CheckLabelAnnotationValueIsValid(backup.GetLabels(), constant.NabOriginNACUUIDLabel) {
				logger.V(1).Info("Backup does not have required label", constant.NameString, backup.Name)
				// TODO delete?
				continue
			}
			annotations := backup.GetAnnotations()
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
					return err
				}
				if err = r.Delete(ctx, &backup); err != nil {
					logger.Error(err, "Failed to delete orphan backup", constant.NameString, backup.Name)
					return err
				}
				logger.V(1).Info("orphan Backup deleted", constant.NameString, backup.Name)
			}
		}
		return nil
	})

	execution.Go(func() error {
		veleroRestoreList := &velerov1.RestoreList{}
		if err := r.List(ctx, veleroRestoreList, client.InNamespace(r.OADPNamespace), labelSelector); err != nil {
			logger.Error(err, "Unable to fetch Restores in OADP namespace")
			return err
		}
		for _, restore := range veleroRestoreList.Items {
			if !function.CheckLabelAnnotationValueIsValid(restore.GetLabels(), constant.NarOriginNACUUIDLabel) {
				logger.V(1).Info("Restore does not have required label", constant.NameString, restore.Name)
				// TODO delete?
				continue
			}
			annotations := restore.GetAnnotations()
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
					return err
				}
				if err = r.Delete(ctx, &restore); err != nil {
					logger.Error(err, "Failed to delete orphan Restore", constant.NameString, restore.Name)
					return err
				}
				logger.V(1).Info("orphan Restore deleted", constant.NameString, restore.Name)
			}
		}
		return nil
	})

	err := execution.Wait()

	logger.V(1).Info("Garbage Collector Reconcile end")
	return ctrl.Result{}, err
}

// SetupWithManager sets up the controller with the Manager.
func (r *GarbageCollectorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("nonadmingarbagecollector").
		WithLogConstructor(func(_ *reconcile.Request) logr.Logger {
			return logr.New(ctrl.Log.GetSink().WithValues("controller", "nonadmingarbagecollector"))
		}).
		WatchesRawSource(&source.PeriodicalSource{Frequency: r.Frequency}).
		Complete(r)
}
