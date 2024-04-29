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
	"time"

	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	nacv1alpha1 "github.com/migtools/oadp-non-admin/api/v1alpha1"
	"github.com/migtools/oadp-non-admin/internal/common/constant"
	"github.com/migtools/oadp-non-admin/internal/handler"
	"github.com/migtools/oadp-non-admin/internal/predicate"
)

// NonAdminBackupReconciler reconciles a NonAdminBackup object
type NonAdminBackupReconciler struct {
	client.Client
	Scheme  *runtime.Scheme
	Context context.Context
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
	rLog := log.FromContext(ctx)
	logger := rLog.WithValues("NonAdminBackup", req.NamespacedName)
	logger.V(1).Info(">>> Reconcile NonAdminBackup - loop start")

	// Get the NonAdminBackup object
	nab := nacv1alpha1.NonAdminBackup{}
	err := r.Get(ctx, req.NamespacedName, &nab)

	// Bail out when the Non Admin Backup reconcile was triggered, when the NAB got deleted
	// Reconcile loop was triggered when Velero Backup object got updated and NAB isn't there
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(1).Info("Non existing NonAdminBackup CR", nameField, req.Name, constant.NameSpaceString, req.Namespace)
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Unable to fetch NonAdminBackup CR", nameField, req.Name, constant.NameSpaceString, req.Namespace)
		return ctrl.Result{}, err
	}

	// Run Reconcile Batch as we have the NonAdminBackup object
	reconcileExit, reconcileRequeue, reconcileErr := ReconcileBatch(
		ReconcileFunc(func(...any) (bool, bool, error) {
			return r.InitNonAdminBackup(ctx, rLog, &nab) // Phase: New
		}),
		ReconcileFunc(func(...any) (bool, bool, error) {
			return r.ValidateVeleroBackupSpec(ctx, rLog, &nab) // Phase: New
		}),
		ReconcileFunc(func(...any) (bool, bool, error) {
			return r.CreateVeleroBackupSpec(ctx, rLog, &nab) // Phase: New
		}),
	)

	// Return vars from the ReconcileBatch
	if reconcileExit {
		return ctrl.Result{}, reconcileErr
	} else if reconcileRequeue {
		return ctrl.Result{Requeue: true, RequeueAfter: requeueTimeSeconds * time.Second}, reconcileErr
	} else if reconcileErr != nil {
		return ctrl.Result{}, reconcileErr
	} // No error and both reconcileExit and reconcileRequeue false, continue...

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
