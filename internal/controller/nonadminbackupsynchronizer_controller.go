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
	"slices"
	"time"

	"github.com/go-logr/logr"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

// NonAdminBackupSynchronizerReconciler reconciles BackupStorageLocation objects
type NonAdminBackupSynchronizerReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	OADPNamespace string
	SyncPeriod    time.Duration
}

// +kubebuilder:rbac:groups=core,resources=namespaces,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *NonAdminBackupSynchronizerReconciler) Reconcile(ctx context.Context, _ ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	labelSelector := client.MatchingLabels(function.GetNonAdminLabels())

	logger.V(1).Info("NonAdminBackup Synchronization start")

	veleroBackupStorageLocationList := &velerov1.BackupStorageLocationList{}
	if err := r.List(ctx, veleroBackupStorageLocationList, client.InNamespace(r.OADPNamespace)); err != nil {
		return ctrl.Result{}, err
	}

	var watchedBackupStorageLocations []string
	relatedNonAdminBackupStorageLocations := map[string]string{}
	for _, backupStorageLocation := range veleroBackupStorageLocationList.Items {
		if backupStorageLocation.Spec.Default {
			watchedBackupStorageLocations = append(watchedBackupStorageLocations, backupStorageLocation.Name)
		}
		if function.CheckVeleroBackupStorageLocationMetadata(&backupStorageLocation) {
			err := r.Get(ctx, types.NamespacedName{
				Name:      backupStorageLocation.Annotations[constant.NabslOriginNameAnnotation],
				Namespace: backupStorageLocation.Annotations[constant.NabslOriginNamespaceAnnotation],
			}, &nacv1alpha1.NonAdminBackupStorageLocation{})
			if err != nil {
				if apierrors.IsNotFound(err) {
					continue
				}
				logger.Error(err, "Unable to fetch NonAdminBackupStorageLocation")
				return ctrl.Result{}, err
			}
			watchedBackupStorageLocations = append(watchedBackupStorageLocations, backupStorageLocation.Name)
			relatedNonAdminBackupStorageLocations[backupStorageLocation.Name] = backupStorageLocation.Annotations[constant.NabslOriginNameAnnotation]
		}
	}

	veleroBackupList := &velerov1.BackupList{}
	if err := r.List(ctx, veleroBackupList, client.InNamespace(r.OADPNamespace), labelSelector); err != nil {
		return ctrl.Result{}, err
	}

	var backupsToSync []velerov1.Backup
	var possibleBackupsToSync []velerov1.Backup
	for _, backup := range veleroBackupList.Items {
		if function.CheckVeleroBackupAnnotations(&backup) &&
			function.CheckLabelAnnotationValueIsValid(backup.GetLabels(), constant.NabOriginNACUUIDLabel) &&
			backup.Status.CompletionTimestamp != nil &&
			slices.Contains(watchedBackupStorageLocations, backup.Spec.StorageLocation) {
			possibleBackupsToSync = append(possibleBackupsToSync, backup)
		}
	}

	logger.V(1).Info(fmt.Sprintf("%v possible Backup(s) to be synced to NonAdmin namespaces", len(possibleBackupsToSync)))
	for _, backup := range possibleBackupsToSync {
		err := r.Get(ctx, types.NamespacedName{
			Name: backup.Annotations[constant.NabOriginNamespaceAnnotation],
		}, &corev1.Namespace{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			logger.Error(err, "Unable to fetch Namespace")
			return ctrl.Result{}, err
		}

		err = r.Get(ctx, types.NamespacedName{
			Namespace: backup.Annotations[constant.NabOriginNamespaceAnnotation],
			Name:      backup.Annotations[constant.NabOriginNameAnnotation],
		}, &nacv1alpha1.NonAdminBackup{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				backupsToSync = append(backupsToSync, backup)
				continue
			}
			logger.Error(err, "Unable to fetch NonAdminBackup")
			return ctrl.Result{}, err
		}
	}

	logger.V(1).Info(fmt.Sprintf("%v Backup(s) to sync to NonAdmin namespaces", len(backupsToSync)))
	for _, backup := range backupsToSync {
		nab := &nacv1alpha1.NonAdminBackup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      backup.Annotations[constant.NabOriginNameAnnotation],
				Namespace: backup.Annotations[constant.NabOriginNamespaceAnnotation],
				// TODO sync operation does not preserve labels
				Labels: map[string]string{
					constant.NabSyncLabel: backup.Labels[constant.NabOriginNACUUIDLabel],
				},
			},
			Spec: nacv1alpha1.NonAdminBackupSpec{
				BackupSpec: &backup.Spec,
			},
		}
		value, exist := relatedNonAdminBackupStorageLocations[nab.Spec.BackupSpec.StorageLocation]
		if exist {
			nab.Spec.BackupSpec.StorageLocation = value
		} else {
			nab.Spec.BackupSpec.StorageLocation = constant.EmptyString
		}
		err := r.Create(ctx, nab)
		if err != nil {
			logger.Error(err, "Failed to create NonAdminBackup")
			return ctrl.Result{}, err
		}
	}

	logger.V(1).Info("NonAdminBackup Synchronization exit")
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NonAdminBackupSynchronizerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("nonadminbackupsynchronizer").
		WithLogConstructor(func(_ *reconcile.Request) logr.Logger {
			return logr.New(ctrl.Log.GetSink().WithValues("controller", "nonadminbackupsynchronizer"))
		}).
		WatchesRawSource(&source.PeriodicalSource{Frequency: r.SyncPeriod}).
		Complete(r)
}
