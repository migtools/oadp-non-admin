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

	// TODO(user): your logic here
	log.Info("Reconcile loop")

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *VeleroBackupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&velerov1api.Backup{}).
		Complete(r)
}
