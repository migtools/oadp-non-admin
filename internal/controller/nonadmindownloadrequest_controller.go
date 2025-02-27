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
	"errors"
	"time"

	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	ctrlpredicate "sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	nacv1alpha1 "github.com/migtools/oadp-non-admin/api/v1alpha1"
	"github.com/migtools/oadp-non-admin/internal/common/constant"
	"github.com/migtools/oadp-non-admin/internal/common/function"
)

// NonAdminDownloadRequestReconciler reconciles a NonAdminDownloadRequest object
type NonAdminDownloadRequestReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	OADPNamespace string
}

// +kubebuilder:rbac:groups=oadp.openshift.io,resources=nonadmindownloadrequests,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=oadp.openshift.io,resources=nonadmindownloadrequests/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=oadp.openshift.io,resources=nonadmindownloadrequests/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the NonAdminDownloadRequest object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.2/pkg/reconcile
//
// This reconcile implements ObjectReconciler https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.2/pkg/reconcile#ObjectReconciler
// Each reconciliation event gets the associated object from Kubernetes before passing it to Reconcile
func (r *NonAdminDownloadRequestReconciler) Reconcile(ctx context.Context, req *nacv1alpha1.NonAdminDownloadRequest) (reconcile.Result, error) {
	if req == nil || req.Spec.Target.Kind == constant.EmptyString {
		return ctrl.Result{Requeue: false}, nil
	}
	// TODO: delete if UID is always available here.
	// if req.ObjectMeta.UID  == "" {
	// requeue later until uid is populated
	// }
	logger := log.FromContext(ctx)
	logger.Info("Reconciling NonAdminDownloadRequest", "name", req.Name, "namespace", req.Namespace)
	if req.Status.VeleroDownloadRequestStatus.Expiration.Before(&metav1.Time{Time: time.Now()}) {
		// request is expired, so delete NADR after deleting velero DR
		// find associated downloadrequest and delete that first
		logger.V(1).Info("Deleting expired NonAdminDownloadRequest", req.Name, req.Namespace)
		if err := r.Delete(ctx, req); err != nil {
			if apierrors.IsNotFound(err) {
				return reconcile.Result{Requeue: false}, nil
			}
			return reconcile.Result{Requeue: true}, nil
		}
	}
	// veleroObj is backup or restore
	// naObj is nonAdminDownloadRequests
	var veleroBR, veleroDR, naObj client.Object
	switch req.Spec.Target.Kind {
	case velerov1.DownloadTargetKindBackupLog,
		velerov1.DownloadTargetKindBackupContents,
		velerov1.DownloadTargetKindBackupVolumeSnapshots,
		velerov1.DownloadTargetKindBackupItemOperations,
		velerov1.DownloadTargetKindBackupResourceList,
		velerov1.DownloadTargetKindBackupResults,
		velerov1.DownloadTargetKindCSIBackupVolumeSnapshots,
		velerov1.DownloadTargetKindCSIBackupVolumeSnapshotContents,
		velerov1.DownloadTargetKindBackupVolumeInfos:
		// get velero backup name from nab
		var nab nacv1alpha1.NonAdminBackup
		if err := r.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: req.Spec.Target.Name}, &nab); err != nil {
			// TODO:
			_ = ""
		}
		naObj = &nab
		if nab.VeleroBackupName() == constant.EmptyString {
			return reconcile.Result{}, errors.New("unable to get Velero Backup UUID from NonAdminBackup Status")
		}
		var vb = velerov1.Backup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      nab.VeleroBackupName(),
				Namespace: r.OADPNamespace,
			},
		}
		veleroBR = &vb
	case velerov1.DownloadTargetKindRestoreLog,
		velerov1.DownloadTargetKindRestoreResults,
		velerov1.DownloadTargetKindRestoreResourceList,
		velerov1.DownloadTargetKindRestoreItemOperations,
		velerov1.DownloadTargetKindRestoreVolumeInfo:
		veleroBR = &velerov1.Restore{}
	}
	_ = naObj
	_ = veleroDR
	if err := r.Get(ctx, types.NamespacedName{Namespace: veleroBR.GetNamespace(), Name: veleroBR.GetName()}, veleroBR); err != nil {
		logger.Error(err, "Failed to get velero object associated with downloadRequest", veleroBR.GetObjectKind(), veleroBR.GetName())
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
// We are using predicates within For, and Watches itself, instead of defining complicated CompositePredicate that applies to all watches/fors.
// This approach is more readable and less lines of code, and is self contained within each controller.
// Other controllers had to build CompositePredicate because they used EventFilter which applied to all watches.
func (r *NonAdminDownloadRequestReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nacv1alpha1.NonAdminDownloadRequest{}, builder.WithPredicates(ctrlpredicate.Funcs{
			// TODO: DeleteFunc: , delete velero download requests? maybe if we just set ownerReferences
			// Process creates
			CreateFunc: func(_ event.TypedCreateEvent[client.Object]) bool { return true },
			// TODO: UpdateFunc: , potentially don't process updates?
			// TODO: GenericFunc: , what to do with generic events?

		})).
		Named("nonadmindownloadrequest").
		Watches(&velerov1.DownloadRequest{}, handler.Funcs{
			UpdateFunc: func(ctx context.Context, tue event.TypedUpdateEvent[client.Object], rli workqueue.RateLimitingInterface) {
				if dr, ok := tue.ObjectNew.(*velerov1.DownloadRequest); ok &&
					dr.Status.Phase == velerov1.DownloadRequestPhaseProcessed { // only reconcile on updates when downloadrequests is processed
					log := function.GetLogger(ctx, dr, "VeleroDownloadRequestHandler")
					log.V(1).Info("DownloadRequest populated with url")
					// on update, we need to reconcile NonAdminDownloadRequests to update
					rli.Add(reconcile.Request{
						NamespacedName: types.NamespacedName{
							Namespace: constant.NadrOriginNamespaceAnnotation,
							Name:      constant.NadrOriginNameAnnotation,
						},
					})
				}
			},
			// DeleteFunc: , TODO: if velero DownloadRequests gets cleaned up, delete this?
		}, builder.WithPredicates(
			ctrlpredicate.NewPredicateFuncs(func(object client.Object) bool {
				// only watch download requests with our label
				if _, hasUID := object.GetLabels()[constant.NadrOriginNACUUIDLabel]; hasUID {
					return true
				}
				return false
			}),
		),
		).
		Complete(reconcile.AsReconciler(r.Client, r))
}
