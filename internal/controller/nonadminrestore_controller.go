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
	"os"

	"github.com/go-logr/logr"
	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	nacv1alpha1 "github.com/migtools/oadp-non-admin/api/v1alpha1"
	"github.com/migtools/oadp-non-admin/internal/common/constant"
)

// NonAdminRestoreReconciler reconciles a NonAdminRestore object
type NonAdminRestoreReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Logger logr.Logger
}

// +kubebuilder:rbac:groups=nac.oadp.openshift.io,resources=nonadminrestores,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nac.oadp.openshift.io,resources=nonadminrestores/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=nac.oadp.openshift.io,resources=nonadminrestores/finalizers,verbs=update

// +kubebuilder:rbac:groups=velero.io,resources=restores,verbs=get;list;watch;create;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the NonAdminRestore to the desired state.
func (r *NonAdminRestoreReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.Logger = log.FromContext(ctx)
	logger := r.Logger.WithValues("NonAdminRestore", req.NamespacedName)

	logger.Info("TODO")

	nonAdminRestore := nacv1alpha1.NonAdminRestore{}
	err := r.Get(ctx, req.NamespacedName, &nonAdminRestore)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = r.validateSpec(ctx, req, nonAdminRestore.Spec)
	if err != nil {
		return ctrl.Result{}, err
	}

	// TODO try to create Velero Restore

	return ctrl.Result{}, nil
}

// TODO remove functions params
func (r *NonAdminRestoreReconciler) validateSpec(ctx context.Context, req ctrl.Request, objectSpec nacv1alpha1.NonAdminRestoreSpec) error {
	if len(objectSpec.RestoreSpec.ScheduleName) > 0 {
		return fmt.Errorf("spec.restoreSpec.scheduleName field is not allowed in NonAdminRestore")
	}

	// TODO nonAdminBackup respect restricted fields

	nonAdminBackupName := objectSpec.RestoreSpec.BackupName
	nonAdminBackup := &nacv1alpha1.NonAdminBackup{}
	err := r.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: nonAdminBackupName}, nonAdminBackup)
	if err != nil {
		if errors.IsNotFound(err) {
			// TODO add this error message to NonAdminRestore status
			return fmt.Errorf("invalid spec.restoreSpec.backupName: NonAdminBackup '%s' does not exist in namespace %s", nonAdminBackupName, req.Namespace)
		}
		return err
	}
	// TODO nonAdminBackup has necessary labels (NAB controller job :question:)
	// TODO nonAdminBackup is in complete state :question:!!!!

	// TODO create get function in common :question:
	oadpNamespace := os.Getenv(constant.NamespaceEnvVar)

	veleroBackupName := nonAdminBackup.Labels["naoSei"]
	veleroBackup := &velerov1api.Backup{}
	err = r.Get(ctx, types.NamespacedName{Namespace: oadpNamespace, Name: veleroBackupName}, veleroBackup)
	if err != nil {
		// TODO test error messages, THEY MUST BE INFORMATIVE
		return err
	}

	return nil
}

// SetupWithManager sets up the NonAdminRestore controller with the Manager.
func (r *NonAdminRestoreReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nacv1alpha1.NonAdminRestore{}).
		Complete(r)
}
