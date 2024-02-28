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

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
)

// VeleroBackupReconciler reconciles a VeleroBackup object
type VeleroBackupReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Log    logr.Logger
}

//+kubebuilder:rbac:groups=nac.oadp.openshift.io,resources=nonadminbackups,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=nac.oadp.openshift.io,resources=nonadminbackups/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=nac.oadp.openshift.io,resources=nonadminbackups/finalizers,verbs=update

//+kubebuilder:rbac:groups=velero.io,resources=backups,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the VeleroBackup object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.0/pkg/reconcile
func (r *VeleroBackupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.Log = log.FromContext(ctx)
	log := r.Log.WithValues("VeleroBackup", req.NamespacedName)

	// Check if the reconciliation request is for the oadp namespace
	if req.Namespace != OadpNamespace {
		log.Info("Ignoring reconciliation request for namespace", "Namespace", req.Namespace)
		return ctrl.Result{}, nil
	}

	// TODO(user): your logic here

	backup := &velerov1api.Backup{}
	err := r.Get(ctx, req.NamespacedName, backup)
	if err != nil {
		log.Error(err, "Unable to fetch VeleroBackup CR", "Name", req.Name, "Namespace", req.Namespace)
		return ctrl.Result{}, err
	}

	if !HasRequiredLabel(backup) {
		log.Info("Ignoring VeleroBackup without the required label", "Name", backup.Name, "Namespace", backup.Namespace)
		return ctrl.Result{}, nil
	}

	log.Info("Velero Backup Reconcile loop")
	nonAdminBackup, err := GetNonAdminFromBackup(ctx, r.Client, backup)
	if err != nil {
		log.V(1).Info("Could not find matching Velero Non Admin Backup", "error", err)
		// We don't report error to reconcile
	} else {
		log.V(1).Info("Got Velero Non Admin Backup:", "nab", nonAdminBackup)
		log.V(1).Info("Velero Backup status:", "Status", backup.Status)
		nonAdminBackup.Spec.BackupStatus = &backup.Status
		if err := r.Client.Update(ctx, nonAdminBackup); err != nil {
			log.Error(err, "Failed to update NonAdminBackup status")
		} else {
			log.V(1).Info("Updated NonAdminBackup status:", "Status", backup.Status)
		}
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *VeleroBackupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&velerov1api.Backup{}).
		Complete(r)
}
