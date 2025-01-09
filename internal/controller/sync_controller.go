/*
TODO example implementation
*/

package controller

import (
	"context"
	"fmt"
	"time"

	// "github.com/sirupsen/logrus"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	// "github.com/vmware-tanzu/velero/pkg/persistence"
	// "github.com/vmware-tanzu/velero/pkg/plugin/clientmgmt"
	// "github.com/vmware-tanzu/velero/pkg/plugin/clientmgmt/process"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	// "k8s.io/apimachinery/pkg/util/sets"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	nacv1alpha1 "github.com/migtools/oadp-non-admin/api/v1alpha1"
	"github.com/migtools/oadp-non-admin/internal/common/constant"
	"github.com/migtools/oadp-non-admin/internal/common/function"
)

// NonAdminBackupSyncReconciler reconciles a NonAdminBackupStorageLocation object
type NonAdminBackupSyncReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	OADPNamespace string
	SyncPeriod    time.Duration
}

type nacCredentialsFileStore struct{}

func (f *nacCredentialsFileStore) Path(*corev1.SecretKeySelector) (string, error) {
	return "/tmp/credentials/secret-file", nil
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state,
// defined in NonAdminBackupStorageLocation object Spec.
func (r *NonAdminBackupSyncReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	// on NonAdminBackupStorageLocation create/update and every configured timestamp
	logger.V(1).Info("NonAdminBackup Synchronization start")

	nonAdminBackupStorageLocation := &nacv1alpha1.NonAdminBackupStorageLocation{}
	err := r.Get(ctx, req.NamespacedName, nonAdminBackupStorageLocation)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(1).Info(err.Error())
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Unable to fetch NonAdminBackupStorageLocation")
		return ctrl.Result{}, err
	}

	// Get related Velero BSL
	if nonAdminBackupStorageLocation.Status.VeleroBackupStorageLocation == nil {
		err = fmt.Errorf("VeleroBackupStorageLocation not yet processed")
		logger.Error(err, "VeleroBackupStorageLocation not yet processed")
		return ctrl.Result{}, err
	}
	backupStorageLocation, err := function.GetVeleroBackupStorageLocationByLabel(
		ctx, r.Client, r.OADPNamespace,
		nonAdminBackupStorageLocation.Status.VeleroBackupStorageLocation.NACUUID,
	)
	if err != nil {
		logger.Error(err, "Failed to get VeleroBackupStorageLocation")
		return ctrl.Result{}, err
	}
	if backupStorageLocation == nil {
		err = fmt.Errorf("VeleroBackupStorageLocation not yet created")
		logger.Error(err, "VeleroBackupStorageLocation not yet created")
		return ctrl.Result{}, err
	}

	// // OPTION 1: copy how Velero does
	// // PROBLEM: NAC Pod will not have Velero Pod files
	// pluginRegistry := process.NewRegistry("/plugins", logger, logger.Level)
	// if err := pluginRegistry.DiscoverPlugins(); err != nil {
	// 	return ctrl.Result{}, err
	// }
	// newPluginManager := func(logger logrus.FieldLogger) clientmgmt.Manager {
	// 	return clientmgmt.NewManager(logger, logLevel, pluginRegistry)
	// }
	// objectStorageClient := persistence.NewObjectBackupStoreGetter(&nacCredentialsFileStore{})
	// objectStorage, err := objectStorageClient.Get(backupStorageLocation, newPluginManager(logger), logger)
	// if err != nil {
	// 	return ctrl.Result{}, err
	// }
	// objectStorageBackupList, err := objectStorage.ListBackups()
	// if err != nil {
	// 	return ctrl.Result{}, nil
	// }

	// // get all Velero backups that
	// // NAC owns
	// // are finished
	// // were created from same namespace as NABSL
	// var veleroBackupList velerov1.BackupList
	// labelSelector := client.MatchingLabels(function.GetNonAdminLabels())

	// if err := r.List(ctx, &veleroBackupList, client.InNamespace(r.OADPNamespace), labelSelector); err != nil {
	// 	return ctrl.Result{}, err
	// }
	// logger.V(1).Info(fmt.Sprintf("%v Backups owned by NAC controller in OADP namespace", len(veleroBackupList.Items)))

	// var backupsToSync []velerov1.Backup
	// // var possibleBackupsToSync []velerov1.Backup
	// var possibleBackupsToSync []string
	// for _, backup := range veleroBackupList.Items {
	// 	if backup.Status.CompletionTimestamp != nil &&
	// 		backup.Annotations[constant.NabOriginNamespaceAnnotation] == nonAdminBackupStorageLocation.Namespace {
	// 		possibleBackupsToSync = append(possibleBackupsToSync, backup.Name)
	// 	}
	// }

	// objectStorageBackupSet := sets.New(objectStorageBackupList...)
	// inClusterBackupSet := sets.New(possibleBackupsToSync...)
	// backupsToSyncSet := objectStorageBackupSet.Difference(inClusterBackupSet)

	// for backupName := range backupsToSyncSet {
	// 	veleroBackup := velerov1.Backup{}
	// 	err := r.Get(ctx, types.NamespacedName{
	// 		Namespace: r.OADPNamespace,
	// 		Name:      backupName,
	// 	}, &veleroBackup)
	// 	if err != nil {
	// 		if apierrors.IsNotFound(err) {
	// 			// not yet synced by velero
	// 			continue
	// 		}
	// 		logger.Error(err, "Unable to fetch Velero Backup")
	// 		return ctrl.Result{}, err
	// 	}
	// 	backupsToSync = append(backupsToSync, veleroBackup)
	// }

	// logger.V(1).Info(fmt.Sprintf("%v Backup(s) to sync to NonAdminBackupStorageLocation namespace", len(backupsToSync)))
	// for _, backup := range backupsToSync {
	// 	nab := &nacv1alpha1.NonAdminBackup{
	// 		ObjectMeta: v1.ObjectMeta{
	// 			Name:      backup.Annotations[constant.NabOriginNameAnnotation],
	// 			Namespace: backup.Annotations[constant.NabOriginNamespaceAnnotation],
	// 			// TODO sync operation does not preserve labels
	// 			Labels: map[string]string{
	// 				"openshift.io/oadp-nab-synced-from-nacuuid": backup.Labels[constant.NabOriginNACUUIDLabel],
	// 			},
	// 		},
	// 		Spec: nacv1alpha1.NonAdminBackupSpec{
	// 			BackupSpec: &backup.Spec,
	// 		},
	// 	}
	// 	nab.Spec.BackupSpec.StorageLocation = nonAdminBackupStorageLocation.Name
	// 	r.Create(ctx, nab)
	// }

	// ------------------------------------------------------------------------

	// OPTION 2: compare BSLs specs
	// PROBLEM: if user deletes NABSL, and them recreates it, velero backup will point to a BSL that does not exist

	// get all Velero backups that
	// NAC owns
	// are finished
	// were created from same namespace as NABSL
	// were created from this NABSL
	var veleroBackupList velerov1.BackupList
	labelSelector := client.MatchingLabels(function.GetNonAdminLabels())

	if err := r.List(ctx, &veleroBackupList, client.InNamespace(r.OADPNamespace), labelSelector); err != nil {
		return ctrl.Result{}, err
	}

	var backupsToSync []velerov1.Backup
	var possibleBackupsToSync []velerov1.Backup
	for _, backup := range veleroBackupList.Items {
		if backup.Status.CompletionTimestamp != nil &&
			backup.Annotations[constant.NabOriginNamespaceAnnotation] == nonAdminBackupStorageLocation.Namespace &&
			backup.Spec.StorageLocation == backupStorageLocation.Name {
			possibleBackupsToSync = append(possibleBackupsToSync, backup)
		}
	}
	logger.V(1).Info(fmt.Sprintf("%v possible Backup(s) to be synced to NonAdminBackupStorageLocation namespace", len(possibleBackupsToSync)))

	for _, backup := range possibleBackupsToSync {
		nab := &nacv1alpha1.NonAdminBackup{}
		err := r.Get(ctx, types.NamespacedName{
			Namespace: backup.Annotations[constant.NabOriginNamespaceAnnotation],
			Name:      backup.Annotations[constant.NabOriginNameAnnotation],
		}, nab)
		if err != nil {
			if apierrors.IsNotFound(err) {
				backupsToSync = append(backupsToSync, backup)
				continue
			}
			logger.Error(err, "Unable to fetch NonAdminBackup")
			return ctrl.Result{}, err
		}
	}
	logger.V(1).Info(fmt.Sprintf("%v Backup(s) to sync to NonAdminBackupStorageLocation namespace", len(backupsToSync)))
	for _, backup := range backupsToSync {
		nab := &nacv1alpha1.NonAdminBackup{
			ObjectMeta: v1.ObjectMeta{
				Name:      backup.Annotations[constant.NabOriginNameAnnotation],
				Namespace: backup.Annotations[constant.NabOriginNamespaceAnnotation],
				// TODO sync operation does not preserve labels
				Labels: map[string]string{
					"openshift.io/oadp-nab-synced-from-nacuuid": backup.Labels[constant.NabOriginNACUUIDLabel],
				},
			},
			Spec: nacv1alpha1.NonAdminBackupSpec{
				BackupSpec: &backup.Spec,
			},
		}
		nab.Spec.BackupSpec.StorageLocation = nonAdminBackupStorageLocation.Name
		r.Create(ctx, nab)
	}

	logger.V(1).Info("NonAdminBackup Synchronization exit")
	return ctrl.Result{RequeueAfter: r.SyncPeriod}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NonAdminBackupSyncReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nacv1alpha1.NonAdminBackupStorageLocation{}).
		Complete(r)
}
