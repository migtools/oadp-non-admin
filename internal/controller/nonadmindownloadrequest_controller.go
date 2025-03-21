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
	Name          string
}

const statusPatchErr = "unable to patch status condition"

// +kubebuilder:rbac:groups=oadp.openshift.io,resources=nonadmindownloadrequests,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=oadp.openshift.io,resources=nonadmindownloadrequests/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=oadp.openshift.io,resources=nonadmindownloadrequests/finalizers,verbs=update
// +kubebuilder:rbac:groups=velero.io,resources=downloadrequests,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=velero.io,resources=downloadrequests/status,verbs=get

// Reconcile the NonAdminDownloadRequest object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// This reconcile implements ObjectReconciler https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.2/pkg/reconcile#ObjectReconciler
// Each reconciliation event gets the associated object from Kubernetes before passing it to Reconcile
// Notes for test, this function expect req to come in with k8s UID already populated like real cluster
func (r *NonAdminDownloadRequestReconciler) Reconcile(ctx context.Context, req *nacv1alpha1.NonAdminDownloadRequest) (reconcile.Result, error) {
	if req == nil || req.Spec.Target.Kind == constant.EmptyString {
		return ctrl.Result{}, nil
	}
	logger := log.FromContext(ctx)
	logger.Info("Reconciling NonAdminDownloadRequest")
	// defines associated downloadrequest for getting, or deleting
	veleroDR := velerov1.DownloadRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.VeleroDownloadRequestName(),
			Namespace: r.OADPNamespace,
			Labels: func() map[string]string {
				nal := function.GetNonAdminLabels()
				nal[constant.NadrOriginNACUUIDLabel] = string(req.GetUID())
				return nal
			}(),
			Annotations: function.GetNonAdminDownloadRequestAnnotations(req),
		},
		Spec: velerov1.DownloadRequestSpec{
			Target: velerov1.DownloadTarget{
				Kind: req.Spec.Target.Kind,
			},
		},
	}
	// if request has status, it may already be processed or expired
	if req.Status.VeleroDownloadRequest.Status != nil {
		// if request is expired, delete NADR
		if req.Status.VeleroDownloadRequest.Status.Expiration != nil && req.Status.VeleroDownloadRequest.Status.Expiration.Before(&metav1.Time{Time: time.Now()}) {
			return reconcile.Result{}, r.deleteExpiredDownloadRequest(ctx, &veleroDR, req)
		}
		// if request is not expired, and has downloadUrl, requeue when expired to cleanup.
		if req.Status.VeleroDownloadRequest.Status.DownloadURL != constant.EmptyString {
			return reconcile.Result{RequeueAfter: time.Until(req.Status.VeleroDownloadRequest.Status.Expiration.Time)}, nil
		}
	}
	// try get veleroDR if exists, then update status
	if err := r.Get(ctx, types.NamespacedName{Namespace: veleroDR.Namespace, Name: veleroDR.Name}, &veleroDR); err == nil {
		return reconcile.Result{}, r.updateNADRWithURL(ctx, &veleroDR, req)
	} else if !apierrors.IsNotFound(err) {
		// some other errors, requeue to retry get
		return reconcile.Result{}, err
	}
	// veleroDR is not found, so we create one
	return reconcile.Result{}, r.processDownloadRequest(ctx, &veleroDR, req)
}

// Delete expired download request
// find associated velero downloadrequest and delete that first
func (r *NonAdminDownloadRequestReconciler) deleteExpiredDownloadRequest(ctx context.Context, _ *velerov1.DownloadRequest, req *nacv1alpha1.NonAdminDownloadRequest) error {
	logger := log.FromContext(ctx)
	// velero GC handles expired velero download request, so we handle NADR only.
	logger.V(1).Info("Deleting expired NonAdminDownloadRequest")
	if err := r.Delete(ctx, req); err != nil {
		if !apierrors.IsNotFound(err) {
			logger.Error(err, "Failed to delete expired NonAdminDownloadRequest")
			// other errors, requeue to retry delete
			return err
		}
		// not found, stop requeue
	}
	return nil
}

// if url is available and not expired, then set status to completed
func (r *NonAdminDownloadRequestReconciler) updateNADRWithURL(ctx context.Context, veleroDR *velerov1.DownloadRequest, req *nacv1alpha1.NonAdminDownloadRequest) error {
	logger := log.FromContext(ctx)
	if veleroDR.Status.DownloadURL != constant.EmptyString &&
		veleroDR.Status.Phase == velerov1.DownloadRequestPhaseProcessed &&
		!veleroDR.Status.Expiration.Before(&metav1.Time{Time: time.Now()}) {
		// copy req, prepare to patch
		prePatch := req.DeepCopy()
		// copy status to NADR from VDR
		req.Status.VeleroDownloadRequest.Status = &veleroDR.Status
		// veleroDR is processed, update NADR status
		if patchErr := r.Status().Patch(ctx, req, client.MergeFrom(prePatch)); patchErr != nil {
			logger.Error(patchErr, "unable to patch status")
			return patchErr
		}
		if patchErr := r.patchAddStatusTrueConditionPhase(ctx, req, nacv1alpha1.NonAdminPhaseCreated, nacv1alpha1.ConditionNonAdminProcessed, constant.EmptyString, "Success"); patchErr != nil {
			logger.Error(patchErr, "unable to patch status")
			return patchErr
		}
	}
	return nil
}

// processDownloadRequest creates download requests after performing NaBSL checks on NAB/NAR from the downloadRequest
func (r *NonAdminDownloadRequestReconciler) processDownloadRequest(ctx context.Context, veleroDR *velerov1.DownloadRequest, req *nacv1alpha1.NonAdminDownloadRequest) error {
	logger := log.FromContext(ctx)
	var nab nacv1alpha1.NonAdminBackup // holds nonadminbackup
	var nabName string                 // holds nonadminbackup name
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
		nabName = req.Spec.Target.Name
	case velerov1.DownloadTargetKindRestoreLog,
		velerov1.DownloadTargetKindRestoreResults,
		velerov1.DownloadTargetKindRestoreResourceList,
		velerov1.DownloadTargetKindRestoreItemOperations,
		velerov1.DownloadTargetKindRestoreVolumeInfo:
		var nar nacv1alpha1.NonAdminRestore
		if err := r.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: req.Spec.Target.Name}, &nar); err != nil {
			patchErr := r.patchAddErrorStatusTrueConditionBackoff(ctx, req, nacv1alpha1.ConditionNonAdminRestoreNotAvailable, "cannot get nonadminrestore")
			logger.Error(patchErr, statusPatchErr)
			logger.Error(err, "unable to get nar", nar.Name, nar.Namespace)
			return err
		}
		// check nar for nabsl
		nabName = nar.NonAdminBackupName()
		veleroDR.Spec.Target.Name = nar.VeleroRestoreName()
	}
	if err := r.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: nabName}, &nab); err != nil {
		patchErr := r.patchAddErrorStatusTrueConditionBackoff(ctx, req, nacv1alpha1.ConditionNonAdminBackupNotAvailable, "cannot get nonadminbackup")
		logger.Error(patchErr, statusPatchErr)
		logger.Error(err, "unable to get nab", nab.Name, nab.Namespace)
		return err
	}
	// error if nab does not use nabsl
	if !nab.UsesNaBSL() {
		patchErr := r.patchAddErrorStatusTrueConditionBackoff(ctx, req, nacv1alpha1.ConditionNonAdminBackupStorageLocationNotUsed, "backup does not use nonadminbackupstoragelocation, recreate nadr to process")
		logger.Error(patchErr, statusPatchErr)
		// patch status to completed to stop processing this NADR
		// because it is not using NonAdminBackupStorageLocation, user is expected to recreate NADR
		// after they have a NAB using NABSL
		prePatch := req.DeepCopy()
		req.Status.Phase = nacv1alpha1.NonAdminPhaseBackingOff // not using nabsl is terminal
		if patchErr := r.Status().Patch(ctx, req, client.MergeFrom(prePatch)); patchErr != nil {
			logger.Error(patchErr, statusPatchErr)
			return patchErr
		}
		return nil
	}
	// if VDR has target name, it is populated by restore case
	// if VDR do not have target name, this is a backup case
	// so set veleroDR.Spec.Target.Name to nab.VeleroBackupName()
	if veleroDR.Spec.Target.Name == constant.EmptyString {
		veleroDR.Spec.Target.Name = nab.VeleroBackupName()
	}
	// veleroDR is now ready to be created
	if veleroDR.ResourceVersion == constant.EmptyString {
		if err := r.Create(ctx, veleroDR); err != nil {
			// requeue so Get can update nadr status (if exists) or recreate
			return err
		}
	}
	//  veleroDR is created, when veleroDR status is updated, the watch will trigger reconcile
	return nil
}

// SetupWithManager sets up the controller with the Manager.
// We are using predicates within For, and Watches itself, instead of defining complicated CompositePredicate that applies to all watches/fors.
// This approach is more readable and less lines of code, and is self contained within each controller.
// Other controllers had to build CompositePredicate because they used EventFilter which applied to all watches.
func (r *NonAdminDownloadRequestReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nacv1alpha1.NonAdminDownloadRequest{}, builder.WithPredicates(ctrlpredicate.Funcs{
			CreateFunc: func(_ event.TypedCreateEvent[client.Object]) bool {
				return true // required fields are set via velero validation markers
			},
			UpdateFunc: func(tue event.TypedUpdateEvent[client.Object]) bool {
				// only process update on spec change
				if tue.ObjectNew.GetGeneration() == tue.ObjectOld.GetGeneration() {
					return false
				}
				if nadr, ok := tue.ObjectNew.(*nacv1alpha1.NonAdminDownloadRequest); ok {
					return nadr.ReadyForProcessing()
				}
				return false
			},
			DeleteFunc: func(_ event.TypedDeleteEvent[client.Object]) bool {
				return true
			}, // we process delete events by deleting corresponding velero download requests if found
			GenericFunc: func(_ event.TypedGenericEvent[client.Object]) bool {
				return false
			},
		})).
		Named(r.Name).
		Watches(&velerov1.DownloadRequest{}, handler.Funcs{
			UpdateFunc: func(ctx context.Context, tue event.TypedUpdateEvent[client.Object], rli workqueue.TypedRateLimitingInterface[reconcile.Request]) {
				if dr, ok := tue.ObjectNew.(*velerov1.DownloadRequest); ok &&
					dr.Status.Phase == velerov1.DownloadRequestPhaseProcessed { // only reconcile on updates when downloadrequests is processed
					log := function.GetLogger(ctx, dr, "VeleroDownloadRequestHandler")
					log.V(1).Info("DownloadRequest populated with url")
					// on update, we need to reconcile NonAdminDownloadRequests to update
					rli.Add(reconcile.Request{
						NamespacedName: types.NamespacedName{
							Namespace: dr.Annotations[constant.NadrOriginNamespaceAnnotation],
							Name:      dr.Annotations[constant.NadrOriginNameAnnotation],
						},
					})
				}
			},
			// DeleteFunc: we won't delete on VDR deletion because eventually this will NADR will expire and get deleted anyway. GC controller will handle any leftovers
		}, builder.WithPredicates(
			ctrlpredicate.NewPredicateFuncs(func(object client.Object) bool {
				// only watch OADP NS
				if object.GetNamespace() != r.OADPNamespace {
					return false
				}
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

// patchAddErrorStatusTrueConditionBackoff adds backoff phase and sets condition on NADR to notify users of potential issues
func (r *NonAdminDownloadRequestReconciler) patchAddErrorStatusTrueConditionBackoff(ctx context.Context, req *nacv1alpha1.NonAdminDownloadRequest, conditionType nacv1alpha1.NonAdminDownloadRequestConditionType, message string) error {
	return r.patchAddStatusTrueConditionPhase(ctx, req, nacv1alpha1.NonAdminPhaseBackingOff, conditionType, message, "Error")
}

func (r *NonAdminDownloadRequestReconciler) patchAddStatusTrueConditionPhase(ctx context.Context, req *nacv1alpha1.NonAdminDownloadRequest, phase nacv1alpha1.NonAdminPhase, conditionType nacv1alpha1.NonAdminDownloadRequestConditionType, message, reason string) error {
	prePatch := req.DeepCopy()
	req.Status.Phase = phase
	req.Status.Conditions = []metav1.Condition{
		{
			Type:               string(conditionType),
			Status:             metav1.ConditionTrue,
			Reason:             reason,
			Message:            message,
			LastTransitionTime: metav1.Time{Time: time.Now()},
		},
	}
	return r.Status().Patch(ctx, req, client.MergeFrom(prePatch))
}
