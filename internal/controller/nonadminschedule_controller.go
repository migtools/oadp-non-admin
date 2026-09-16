/*
Copyright 2026.

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
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sort"

	"github.com/robfig/cron/v3"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	ctrlhandler "sigs.k8s.io/controller-runtime/pkg/handler"
	ctrlpredicate "sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	nacv1alpha1 "github.com/migtools/oadp-non-admin/api/v1alpha1"
	"github.com/migtools/oadp-non-admin/internal/common/constant"
	"github.com/migtools/oadp-non-admin/internal/common/function"
)

const nonAdminScheduleHistoryLimit = 5

// NonAdminScheduleReconciler reconciles a NonAdminSchedule with a backing
// Velero Schedule in the OADP namespace.
type NonAdminScheduleReconciler struct {
	client.Client
	Scheme             *runtime.Scheme
	EnforcedBackupSpec *velerov1.BackupSpec
	OADPNamespace      string
}

// +kubebuilder:rbac:groups=oadp.openshift.io,resources=nonadminschedules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=oadp.openshift.io,resources=nonadminschedules/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=oadp.openshift.io,resources=nonadminschedules/finalizers,verbs=update
// +kubebuilder:rbac:groups=velero.io,resources=schedules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=velero.io,resources=schedules/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=velero.io,resources=backups,verbs=get;list;watch;update;patch

// Reconcile creates one backing Velero Schedule and makes its generated
// Backups available through synchronized NonAdminBackup objects.
func (r *NonAdminScheduleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	nas := &nacv1alpha1.NonAdminSchedule{}
	if err := r.Get(ctx, req.NamespacedName, nas); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if !nas.DeletionTimestamp.IsZero() {
		return r.reconcileDeletion(ctx, nas)
	}
	if !controllerutil.ContainsFinalizer(nas, constant.NasFinalizerName) {
		controllerutil.AddFinalizer(nas, constant.NasFinalizerName)
		if err := r.Update(ctx, nas); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	if nas.Status.VeleroSchedule == nil || nas.Status.VeleroSchedule.NACUUID == constant.EmptyString {
		nas.Status.VeleroSchedule = &nacv1alpha1.VeleroSchedule{
			NACUUID:   function.GenerateNacObjectUUID(nas.Namespace, nas.Name),
			Namespace: r.OADPNamespace,
		}
		nas.Status.VeleroSchedule.Name = nas.Status.VeleroSchedule.NACUUID
		if err := r.Status().Update(ctx, nas); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	template, err := r.scheduleTemplate(ctx, nas)
	if err != nil {
		return ctrl.Result{}, r.setRejected(ctx, nas, err)
	}

	veleroSchedule, err := r.reconcileVeleroSchedule(ctx, nas, template)
	if err != nil {
		return ctrl.Result{}, err
	}

	if nas.Spec.SkipImmediately {
		// Velero resets its copy after processing this edge-trigger. Reset the
		// tenant object at submission time so subsequent reconciliations do not
		// repeatedly request skipped runs.
		nas.Spec.SkipImmediately = false
		if err := r.Update(ctx, nas); err != nil {
			return ctrl.Result{}, err
		}
	}

	backups, err := r.syncGeneratedBackups(ctx, nas, veleroSchedule)
	if err != nil {
		return ctrl.Result{}, err
	}
	if err := r.updateStatus(ctx, nas, veleroSchedule, backups); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *NonAdminScheduleReconciler) reconcileDeletion(ctx context.Context, nas *nacv1alpha1.NonAdminSchedule) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(nas, constant.NasFinalizerName) {
		return ctrl.Result{}, nil
	}

	if nas.Status.VeleroSchedule != nil && nas.Status.VeleroSchedule.Name != constant.EmptyString {
		veleroSchedule := &velerov1.Schedule{}
		err := r.Get(ctx, types.NamespacedName{Namespace: r.OADPNamespace, Name: nas.Status.VeleroSchedule.Name}, veleroSchedule)
		if err == nil {
			if err := r.Delete(ctx, veleroSchedule); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{Requeue: true}, nil
		}
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
	}

	// The backing Schedule does not use owner references for generated Backups.
	// Removing it therefore stops future runs while retaining existing backups.
	controllerutil.RemoveFinalizer(nas, constant.NasFinalizerName)
	return ctrl.Result{}, r.Update(ctx, nas)
}

func (r *NonAdminScheduleReconciler) scheduleTemplate(ctx context.Context, nas *nacv1alpha1.NonAdminSchedule) (*velerov1.BackupSpec, error) {
	if nas.Spec.Template.BackupSpec == nil {
		return nil, errors.New("NonAdminSchedule spec.template.backupSpec is not set")
	}
	if _, err := cron.ParseStandard(nas.Spec.Schedule); err != nil {
		return nil, fmt.Errorf("NonAdminSchedule spec.schedule is invalid: %w", err)
	}

	validationNAB := &nacv1alpha1.NonAdminBackup{
		ObjectMeta: metav1.ObjectMeta{Namespace: nas.Namespace},
		Spec: nacv1alpha1.NonAdminBackupSpec{
			BackupSpec: nas.Spec.Template.BackupSpec,
		},
	}
	enforced := r.EnforcedBackupSpec
	if enforced == nil {
		enforced = &velerov1.BackupSpec{}
	}
	if err := function.ValidateBackupSpec(ctx, r.Client, r.OADPNamespace, validationNAB, enforced); err != nil {
		return nil, err
	}

	template := nas.Spec.Template.BackupSpec.DeepCopy()
	enforcedValue := reflect.ValueOf(enforced).Elem()
	templateValue := reflect.ValueOf(template).Elem()
	for index := range enforcedValue.NumField() {
		enforcedField := enforcedValue.Field(index)
		templateField := templateValue.Field(index)
		if !enforcedField.IsZero() && templateField.IsZero() {
			templateField.Set(enforcedField)
		}
	}

	template.IncludedNamespaces = []string{nas.Namespace}
	if template.StorageLocation != constant.EmptyString {
		nabsl := &nacv1alpha1.NonAdminBackupStorageLocation{}
		if err := r.Get(ctx, types.NamespacedName{Namespace: nas.Namespace, Name: template.StorageLocation}, nabsl); err != nil {
			return nil, err
		}
		template.StorageLocation = nabsl.Status.VeleroBackupStorageLocation.Name
	}

	hasNewResourceFilters := len(template.IncludedClusterScopedResources) > 0 ||
		len(template.ExcludedClusterScopedResources) > 0 ||
		len(template.IncludedNamespaceScopedResources) > 0 ||
		len(template.ExcludedNamespaceScopedResources) > 0
	if hasNewResourceFilters {
		if !slices.Contains(template.ExcludedNamespaceScopedResources, constant.WildcardString) {
			template.ExcludedNamespaceScopedResources = append(template.ExcludedNamespaceScopedResources, alwaysExcludedNamespacedResources...)
		}
		if !slices.Contains(template.ExcludedClusterScopedResources, constant.WildcardString) {
			template.ExcludedClusterScopedResources = append(template.ExcludedClusterScopedResources, alwaysExcludedClusterResources...)
		}
	} else if !slices.Contains(template.ExcludedResources, constant.WildcardString) {
		template.ExcludedResources = append(template.ExcludedResources, alwaysExcludedNamespacedResources...)
		template.ExcludedResources = append(template.ExcludedResources, alwaysExcludedClusterResources...)
	}

	return template, nil
}

func (r *NonAdminScheduleReconciler) reconcileVeleroSchedule(ctx context.Context, nas *nacv1alpha1.NonAdminSchedule, template *velerov1.BackupSpec) (*velerov1.Schedule, error) {
	ref := nas.Status.VeleroSchedule
	veleroSchedule := &velerov1.Schedule{}
	err := r.Get(ctx, types.NamespacedName{Namespace: r.OADPNamespace, Name: ref.Name}, veleroSchedule)
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	}

	desiredSpec := velerov1.ScheduleSpec{
		Schedule: nas.Spec.Schedule,
		Template: *template,
		Paused:   nas.Spec.Paused,
	}
	if nas.Spec.SkipImmediately {
		// Preserve an already submitted request until Velero clears it. If NAS
		// immediately wrote false after submission, it could cancel the skip
		// before the Velero Schedule controller observed it.
		desiredSpec.SkipImmediately = &nas.Spec.SkipImmediately
	} else if !apierrors.IsNotFound(err) {
		desiredSpec.SkipImmediately = veleroSchedule.Spec.SkipImmediately
	}
	if apierrors.IsNotFound(err) {
		veleroSchedule = &velerov1.Schedule{
			ObjectMeta: metav1.ObjectMeta{
				Name:        ref.Name,
				Namespace:   r.OADPNamespace,
				Labels:      function.GetNonAdminLabels(),
				Annotations: scheduleAnnotations(nas),
			},
			Spec: desiredSpec,
		}
		veleroSchedule.Labels[constant.NasOriginNACUUIDLabel] = ref.NACUUID
		if err := r.Create(ctx, veleroSchedule); err != nil {
			return nil, err
		}
		return veleroSchedule, nil
	}

	if veleroSchedule.Labels[constant.NasOriginNACUUIDLabel] != ref.NACUUID ||
		veleroSchedule.Annotations[constant.NasOriginNamespaceAnnotation] != nas.Namespace ||
		veleroSchedule.Annotations[constant.NasOriginNameAnnotation] != nas.Name {
		return nil, errors.New("related Velero Schedule does not point to NonAdminSchedule")
	}
	updated := false
	if !reflect.DeepEqual(veleroSchedule.Spec, desiredSpec) {
		veleroSchedule.Spec = desiredSpec
		updated = true
	}
	storageLocation := nas.Spec.Template.BackupSpec.StorageLocation
	if storageLocation == constant.EmptyString {
		if _, found := veleroSchedule.Annotations[constant.NasOriginStorageLocationAnnotation]; found {
			delete(veleroSchedule.Annotations, constant.NasOriginStorageLocationAnnotation)
			updated = true
		}
	} else if veleroSchedule.Annotations[constant.NasOriginStorageLocationAnnotation] != storageLocation {
		veleroSchedule.Annotations[constant.NasOriginStorageLocationAnnotation] = storageLocation
		updated = true
	}
	if updated {
		if err := r.Update(ctx, veleroSchedule); err != nil {
			return nil, err
		}
	}
	return veleroSchedule, nil
}

func (r *NonAdminScheduleReconciler) syncGeneratedBackups(ctx context.Context, nas *nacv1alpha1.NonAdminSchedule, veleroSchedule *velerov1.Schedule) ([]velerov1.Backup, error) {
	backupList := &velerov1.BackupList{}
	if err := r.List(ctx, backupList, client.InNamespace(r.OADPNamespace), client.MatchingLabels{
		velerov1.ScheduleNameLabel:     veleroSchedule.Name,
		constant.NasOriginNACUUIDLabel: refNACUUID(nas),
	}); err != nil {
		return nil, err
	}

	for index := range backupList.Items {
		backup := &backupList.Items[index]
		if backup.Annotations[constant.NasOriginNamespaceAnnotation] != nas.Namespace ||
			backup.Annotations[constant.NasOriginNameAnnotation] != nas.Name {
			continue
		}
		if err := r.syncGeneratedBackup(ctx, nas, backup); err != nil {
			return nil, err
		}
	}
	if err := r.deleteExpiredGeneratedBackups(ctx, nas, backupList.Items); err != nil {
		return nil, err
	}
	return backupList.Items, nil
}

// deleteExpiredGeneratedBackups removes only child NABs that NAC previously
// synchronized and whose backing Velero Backup is no longer retained. The
// status UUID is controller-written and prevents tenant-created NABs with a
// matching label from being deleted by this cleanup path.
func (r *NonAdminScheduleReconciler) deleteExpiredGeneratedBackups(ctx context.Context, nas *nacv1alpha1.NonAdminSchedule, backups []velerov1.Backup) error {
	retainedBackups := make(map[string]struct{}, len(backups))
	for _, backup := range backups {
		retainedBackups[backup.Name] = struct{}{}
	}

	children := &nacv1alpha1.NonAdminBackupList{}
	if err := r.List(ctx, children, client.InNamespace(nas.Namespace), client.MatchingLabels{
		constant.NasOriginNACUUIDLabel: refNACUUID(nas),
	}); err != nil {
		return err
	}
	for index := range children.Items {
		child := &children.Items[index]
		backupName := child.Labels[constant.NabSyncLabel]
		if _, retained := retainedBackups[backupName]; retained || !isGeneratedScheduleChild(child, backupName) {
			continue
		}
		if child.DeletionTimestamp.IsZero() {
			if err := r.Delete(ctx, child); err != nil {
				return err
			}
		}
	}
	return nil
}

func isGeneratedScheduleChild(child *nacv1alpha1.NonAdminBackup, backupName string) bool {
	return backupName != constant.EmptyString &&
		child.Name == backupName &&
		child.Status.VeleroBackup != nil &&
		child.Status.VeleroBackup.NACUUID == backupName
}

func (r *NonAdminScheduleReconciler) syncGeneratedBackup(ctx context.Context, nas *nacv1alpha1.NonAdminSchedule, backup *velerov1.Backup) error {
	original := backup.DeepCopy()
	if backup.Labels == nil {
		backup.Labels = map[string]string{}
	}
	if value, exists := backup.Labels[constant.NabOriginNACUUIDLabel]; exists && value != backup.Name {
		return fmt.Errorf("generated Velero Backup %s has an unexpected NACUUID", backup.Name)
	}
	backup.Labels[constant.NabOriginNACUUIDLabel] = backup.Name
	if backup.Annotations == nil {
		backup.Annotations = map[string]string{}
	}
	backup.Annotations[constant.NabOriginNamespaceAnnotation] = nas.Namespace
	backup.Annotations[constant.NabOriginNameAnnotation] = backup.Name
	if !reflect.DeepEqual(original.Labels, backup.Labels) || !reflect.DeepEqual(original.Annotations, backup.Annotations) {
		if err := r.Patch(ctx, backup, client.MergeFrom(original)); err != nil {
			return err
		}
	}

	child := &nacv1alpha1.NonAdminBackup{}
	err := r.Get(ctx, types.NamespacedName{Namespace: nas.Namespace, Name: backup.Name}, child)
	if err == nil {
		if child.Labels[constant.NabSyncLabel] != backup.Name {
			return fmt.Errorf("NonAdminBackup %s/%s conflicts with generated backup", nas.Namespace, backup.Name)
		}
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return err
	}

	childSpec := backup.Spec.DeepCopy()
	// Preserve the tenant-facing NABSL name so existing NAB consumers, such as
	// NonAdminDownloadRequest, continue to resolve the storage location.
	storageLocation, found := backup.Annotations[constant.NasOriginStorageLocationAnnotation]
	if !found {
		return fmt.Errorf("generated Velero Backup %s is missing its tenant storage location", backup.Name)
	}
	childSpec.StorageLocation = storageLocation
	child = &nacv1alpha1.NonAdminBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      backup.Name,
			Namespace: nas.Namespace,
			Labels: map[string]string{
				constant.NabSyncLabel:          backup.Name,
				constant.NasOriginNACUUIDLabel: refNACUUID(nas),
			},
		},
		Spec: nacv1alpha1.NonAdminBackupSpec{BackupSpec: childSpec},
	}
	return r.Create(ctx, child)
}

func (r *NonAdminScheduleReconciler) updateStatus(ctx context.Context, nas *nacv1alpha1.NonAdminSchedule, veleroSchedule *velerov1.Schedule, backups []velerov1.Backup) error {
	previous := nas.DeepCopy()
	if nas.Status.VeleroSchedule == nil {
		nas.Status.VeleroSchedule = &nacv1alpha1.VeleroSchedule{}
	}
	nas.Status.VeleroSchedule.Name = veleroSchedule.Name
	nas.Status.VeleroSchedule.Namespace = veleroSchedule.Namespace
	nas.Status.VeleroSchedule.Status = veleroSchedule.Status.DeepCopy()
	nas.Status.Phase = nacv1alpha1.NonAdminPhaseCreated
	meta.SetStatusCondition(&nas.Status.Conditions, metav1.Condition{
		Type:    string(nacv1alpha1.NonAdminConditionAccepted),
		Status:  metav1.ConditionTrue,
		Reason:  "ScheduleAccepted",
		Message: "schedule accepted",
	})

	sort.Slice(backups, func(i, j int) bool {
		return backups[i].CreationTimestamp.After(backups[j].CreationTimestamp.Time)
	})
	nas.Status.RecentBackups = nil
	for index, backup := range backups {
		if index == nonAdminScheduleHistoryLimit {
			break
		}
		nas.Status.RecentBackups = append(nas.Status.RecentBackups, scheduledBackupStatus(&backup))
	}
	if len(nas.Status.RecentBackups) > 0 {
		last := nas.Status.RecentBackups[0]
		nas.Status.LastBackup = &last
	} else {
		nas.Status.LastBackup = nil
	}
	nas.Status.NextRun = nextRun(veleroSchedule)

	if reflect.DeepEqual(previous.Status, nas.Status) {
		return nil
	}
	return r.Status().Update(ctx, nas)
}

func (r *NonAdminScheduleReconciler) setRejected(ctx context.Context, nas *nacv1alpha1.NonAdminSchedule, cause error) error {
	previous := nas.DeepCopy()
	nas.Status.Phase = nacv1alpha1.NonAdminPhaseBackingOff
	meta.SetStatusCondition(&nas.Status.Conditions, metav1.Condition{
		Type:    string(nacv1alpha1.NonAdminConditionAccepted),
		Status:  metav1.ConditionFalse,
		Reason:  "InvalidScheduleSpec",
		Message: cause.Error(),
	})
	if !reflect.DeepEqual(previous.Status, nas.Status) {
		if err := r.Status().Update(ctx, nas); err != nil {
			return err
		}
	}
	return reconcile.TerminalError(cause)
}

func scheduleAnnotations(nas *nacv1alpha1.NonAdminSchedule) map[string]string {
	annotations := map[string]string{
		constant.NasOriginNamespaceAnnotation: nas.Namespace,
		constant.NasOriginNameAnnotation:      nas.Name,
	}
	if nas.Spec.Template.BackupSpec.StorageLocation != constant.EmptyString {
		annotations[constant.NasOriginStorageLocationAnnotation] = nas.Spec.Template.BackupSpec.StorageLocation
	}
	return annotations
}

func refNACUUID(nas *nacv1alpha1.NonAdminSchedule) string {
	if nas.Status.VeleroSchedule == nil {
		return constant.EmptyString
	}
	return nas.Status.VeleroSchedule.NACUUID
}

func scheduledBackupStatus(backup *velerov1.Backup) nacv1alpha1.NonAdminScheduledBackup {
	return nacv1alpha1.NonAdminScheduledBackup{
		Name:                backup.Name,
		VeleroBackupName:    backup.Name,
		Phase:               backup.Status.Phase,
		StartTimestamp:      backup.Status.StartTimestamp,
		CompletionTimestamp: backup.Status.CompletionTimestamp,
		Expiration:          backup.Status.Expiration,
	}
}

func nextRun(veleroSchedule *velerov1.Schedule) *metav1.Time {
	cronSchedule, err := cron.ParseStandard(veleroSchedule.Spec.Schedule)
	if err != nil {
		return nil
	}
	lastRun := veleroSchedule.CreationTimestamp.Time
	if veleroSchedule.Status.LastBackup != nil {
		lastRun = veleroSchedule.Status.LastBackup.Time
	}
	if veleroSchedule.Status.LastSkipped != nil && veleroSchedule.Status.LastSkipped.After(lastRun) {
		lastRun = veleroSchedule.Status.LastSkipped.Time
	}
	return &metav1.Time{Time: cronSchedule.Next(lastRun)}
}

// SetupWithManager registers the NAS controller and maps backing Velero object
// events to the tenant-owned NAS identified by NAC annotations.
func (r *NonAdminScheduleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nacv1alpha1.NonAdminSchedule{}, builder.WithPredicates(ctrlpredicate.GenerationChangedPredicate{})).
		Watches(&velerov1.Schedule{}, ctrlhandler.EnqueueRequestsFromMapFunc(nonAdminScheduleRequest), builder.WithPredicates(ctrlpredicate.ResourceVersionChangedPredicate{})).
		Watches(&velerov1.Backup{}, ctrlhandler.EnqueueRequestsFromMapFunc(nonAdminScheduleRequest), builder.WithPredicates(ctrlpredicate.ResourceVersionChangedPredicate{})).
		Complete(r)
}

func nonAdminScheduleRequest(_ context.Context, object client.Object) []reconcile.Request {
	annotations := object.GetAnnotations()
	name := annotations[constant.NasOriginNameAnnotation]
	namespace := annotations[constant.NasOriginNamespaceAnnotation]
	if name == constant.EmptyString || namespace == constant.EmptyString {
		return nil
	}
	return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: name, Namespace: namespace}}}
}
