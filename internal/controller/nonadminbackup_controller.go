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
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	nacv1alpha1 "github.com/openshift/oadp-non-admin/api/v1alpha1"
	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NonAdminBackupReconciler reconciles a NonAdminBackup object
type NonAdminBackupReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	Log            logr.Logger
	Context        context.Context
	NamespacedName types.NamespacedName
}

//+kubebuilder:rbac:groups=nac.oadp.openshift.io,resources=nonadminbackups,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=nac.oadp.openshift.io,resources=nonadminbackups/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=nac.oadp.openshift.io,resources=nonadminbackups/finalizers,verbs=update

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
	log := r.Log.WithValues("NonAdminBackup", req.NamespacedName)

	r.Context = ctx
	r.NamespacedName = req.NamespacedName

	nab := nacv1alpha1.NonAdminBackup{}
	err := r.Get(ctx, req.NamespacedName, &nab)

	if err != nil && errors.IsNotFound(err) {
		log.V(1).Info("Deleted NonAdminBackup CR", "Name", req.Name, "Namespace", req.Namespace)
		return ctrl.Result{}, nil
	}

	if err != nil {
		log.Error(err, "Unable to fetch NonAdminBackup CR", "Name", req.Name, "Namespace", req.Namespace)
		return ctrl.Result{}, err
	}

	veleroBackupSpec, err := GetVeleroBackupSpecFromNonAdminBackup(&nab)

	if veleroBackupSpec == nil {
		log.Error(err, "unable to fetch VeleroBackupSpec from NonAdminBackup")
		return ctrl.Result{}, nil
	}

	if err != nil {
		log.Error(err, "Error while performing NonAdminBackup reconcile")
		return ctrl.Result{}, err
	}

	log.Info("NonAdminBackup Reconcile loop")

	veleroBackupName := GenerateVeleroBackupName(nab.Namespace, nab.Name)

	veleroBackup := velerov1api.Backup{}
	err = r.Get(ctx, client.ObjectKey{Namespace: OadpNamespace, Name: veleroBackupName}, &veleroBackup)

	if err != nil && errors.IsNotFound(err) {
		// Create backup
		log.Info("No backup found", "Name", veleroBackupName)
		veleroBackup = velerov1api.Backup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      veleroBackupName,
				Namespace: OadpNamespace,
			},
			Spec: *veleroBackupSpec,
		}
	} else if err != nil && !errors.IsNotFound(err) {
		log.Error(err, "unable to fetch VeleroBackup")
		return ctrl.Result{}, err
	} else {
		log.Info("Backup already exists", "Name", veleroBackupName)
		return ctrl.Result{}, nil
	}

	// Ensure labels for the BackupSpec are merged
	// with the existing NAB labels
	existingLabels := veleroBackup.Labels
	nacManagedLabels := CreateLabelsForNac(existingLabels)
	veleroBackup.Labels = nacManagedLabels

	// Ensure annotations are set for the Backup object
	existingAnnotations := veleroBackup.Annotations
	ownerUUID := string(nab.ObjectMeta.UID)
	nacManagedAnnotations := CreateAnnotationsForNac(nab.Namespace, nab.Name, ownerUUID, existingAnnotations)
	veleroBackup.Annotations = nacManagedAnnotations

	_, err = controllerutil.CreateOrPatch(ctx, r.Client, &veleroBackup, nil)
	if err != nil {
		log.Error(err, "Failed to create backup", "Name", veleroBackupName)
		return ctrl.Result{}, err
	} else {
		log.Info("Backup successfully created", "Name", veleroBackupName)
	}

	log.Info("NonAdminBackup Reconcile loop end")

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NonAdminBackupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nacv1alpha1.NonAdminBackup{}).
		Complete(r)
}
