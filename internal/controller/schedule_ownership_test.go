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
	"testing"
	"time"

	"github.com/go-logr/logr"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	nacv1alpha1 "github.com/migtools/oadp-non-admin/api/v1alpha1"
	"github.com/migtools/oadp-non-admin/internal/common/constant"
	"github.com/migtools/oadp-non-admin/internal/common/function"
)

func ownershipTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := nacv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add NAC types to scheme: %v", err)
	}
	if err := velerov1.AddToScheme(scheme); err != nil {
		t.Fatalf("add Velero types to scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add core types to scheme: %v", err)
	}
	return scheme
}

func mergeLabels(base map[string]string, extra map[string]string) map[string]string {
	merged := map[string]string{}
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range extra {
		merged[key] = value
	}
	return merged
}

// TestSynchronizerSkipsScheduleGeneratedBackup proves the periodic synchronizer never creates a
// child NonAdminBackup for a schedule generated backup (the NonAdminSchedule controller owns that),
// while still syncing an equivalent non schedule backup.
func TestSynchronizerSkipsScheduleGeneratedBackup(t *testing.T) {
	ctx := context.Background()
	scheme := ownershipTestScheme(t)
	completion := metav1.NewTime(time.Now())

	defaultBSL := &velerov1.BackupStorageLocation{
		ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "openshift-adp"},
		Spec:       velerov1.BackupStorageLocationSpec{Default: true},
	}
	tenantNamespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "tenant-a"}}

	scheduleBackup := &velerov1.Backup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sched-uuid-20260915020000",
			Namespace: "openshift-adp",
			Labels: mergeLabels(function.GetNonAdminLabels(), map[string]string{
				velerov1.ScheduleNameLabel:     "sched-uuid",
				constant.NasOriginNACUUIDLabel: "sched-uuid",
				constant.NabOriginNACUUIDLabel: "sched-uuid-20260915020000",
			}),
			Annotations: map[string]string{
				constant.NabOriginNamespaceAnnotation: "tenant-a",
				constant.NabOriginNameAnnotation:      "sched-uuid-20260915020000",
			},
		},
		Spec:   velerov1.BackupSpec{StorageLocation: "default"},
		Status: velerov1.BackupStatus{CompletionTimestamp: &completion},
	}
	plainBackup := &velerov1.Backup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "plain-uuid",
			Namespace: "openshift-adp",
			Labels: mergeLabels(function.GetNonAdminLabels(), map[string]string{
				constant.NabOriginNACUUIDLabel: "plain-uuid",
			}),
			Annotations: map[string]string{
				constant.NabOriginNamespaceAnnotation: "tenant-a",
				constant.NabOriginNameAnnotation:      "plain-nab",
			},
		},
		Spec:   velerov1.BackupSpec{StorageLocation: "default"},
		Status: velerov1.BackupStatus{CompletionTimestamp: &completion},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(defaultBSL, tenantNamespace, scheduleBackup, plainBackup).Build()
	reconciler := &NonAdminBackupSynchronizerReconciler{Client: client, Scheme: scheme, OADPNamespace: "openshift-adp"}

	if _, err := reconciler.Reconcile(ctx, ctrlRequest(types.NamespacedName{})); err != nil {
		t.Fatalf("synchronizer reconcile: %v", err)
	}

	err := client.Get(ctx, types.NamespacedName{Name: "sched-uuid-20260915020000", Namespace: "tenant-a"}, &nacv1alpha1.NonAdminBackup{})
	if !apierrors.IsNotFound(err) {
		t.Fatalf("schedule generated backup was synced by the periodic synchronizer: %v", err)
	}
	if err := client.Get(ctx, types.NamespacedName{Name: "plain-nab", Namespace: "tenant-a"}, &nacv1alpha1.NonAdminBackup{}); err != nil {
		t.Fatalf("control non schedule backup was not synced: %v", err)
	}
}

// TestGarbageCollectorSkipsScheduleGeneratedBackup proves the garbage collector never deletes a
// schedule generated backup as an orphan, while still deleting an equivalent non schedule orphan.
func TestGarbageCollectorSkipsScheduleGeneratedBackup(t *testing.T) {
	ctx := context.Background()
	scheme := ownershipTestScheme(t)

	scheduleBackup := &velerov1.Backup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sched-uuid-20260915020000",
			Namespace: "openshift-adp",
			Labels: mergeLabels(function.GetNonAdminLabels(), map[string]string{
				velerov1.ScheduleNameLabel:     "sched-uuid",
				constant.NasOriginNACUUIDLabel: "sched-uuid",
				constant.NabOriginNACUUIDLabel: "sched-uuid-20260915020000",
			}),
			Annotations: map[string]string{
				constant.NabOriginNamespaceAnnotation: "tenant-a",
				constant.NabOriginNameAnnotation:      "sched-uuid-20260915020000",
			},
		},
	}
	orphanBackup := &velerov1.Backup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "plain-orphan",
			Namespace: "openshift-adp",
			Labels: mergeLabels(function.GetNonAdminLabels(), map[string]string{
				constant.NabOriginNACUUIDLabel: "plain-orphan",
			}),
			Annotations: map[string]string{
				constant.NabOriginNamespaceAnnotation: "tenant-a",
				constant.NabOriginNameAnnotation:      "plain-nab",
			},
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(scheduleBackup, orphanBackup).Build()
	reconciler := &GarbageCollectorReconciler{Client: client, Scheme: scheme, OADPNamespace: "openshift-adp"}

	if _, err := reconciler.Reconcile(ctx, ctrlRequest(types.NamespacedName{})); err != nil {
		t.Fatalf("garbage collector reconcile: %v", err)
	}

	if err := client.Get(ctx, types.NamespacedName{Name: "sched-uuid-20260915020000", Namespace: "openshift-adp"}, &velerov1.Backup{}); err != nil {
		t.Fatalf("schedule generated backup was deleted by the garbage collector: %v", err)
	}
	err := client.Get(ctx, types.NamespacedName{Name: "plain-orphan", Namespace: "openshift-adp"}, &velerov1.Backup{})
	if !apierrors.IsNotFound(err) {
		t.Fatalf("control non schedule orphan backup was not deleted: %v", err)
	}
}

// TestScheduleChildReapedWhenBackingBackupGone proves a schedule generated child NonAdminBackup
// is deleted once its backing Velero Backup no longer exists, independent of the NonAdminSchedule.
func TestScheduleChildReapedWhenBackingBackupGone(t *testing.T) {
	ctx := context.Background()
	scheme := ownershipTestScheme(t)
	child := &nacv1alpha1.NonAdminBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "sched-uuid-20260915020000",
			Namespace:  "tenant-a",
			Finalizers: []string{constant.NabFinalizerName},
			Labels: map[string]string{
				constant.NabSyncLabel:          "sched-uuid-20260915020000",
				constant.NasOriginNACUUIDLabel: "sched-uuid",
			},
		},
		Status: nacv1alpha1.NonAdminBackupStatus{
			Phase:        nacv1alpha1.NonAdminPhaseCreated,
			VeleroBackup: &nacv1alpha1.VeleroBackup{NACUUID: "sched-uuid-20260915020000"},
		},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(child).WithObjects(child).Build()
	reconciler := &NonAdminBackupReconciler{Client: client, Scheme: scheme, OADPNamespace: "openshift-adp"}

	requeue, err := reconciler.createVeleroBackupAndSyncWithNonAdminBackup(ctx, logr.Discard(), child.DeepCopy())
	if err != nil {
		t.Fatalf("reconcile child: %v", err)
	}
	if !requeue {
		t.Fatal("expected requeue after initiating child deletion")
	}
	actual := &nacv1alpha1.NonAdminBackup{}
	if err := client.Get(ctx, types.NamespacedName{Name: child.Name, Namespace: child.Namespace}, actual); err != nil {
		t.Fatalf("get child: %v", err)
	}
	if actual.DeletionTimestamp.IsZero() {
		t.Fatal("expected schedule child NonAdminBackup to be marked for deletion")
	}
}

// TestNonScheduleSyncedNABNotReapedWhenBackupGone proves the reap is scoped to schedule children:
// a regular synced NonAdminBackup (no nas-origin-nacuuid label) is not deleted when its backup is gone.
func TestNonScheduleSyncedNABNotReapedWhenBackupGone(t *testing.T) {
	ctx := context.Background()
	scheme := ownershipTestScheme(t)
	nab := &nacv1alpha1.NonAdminBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "plain-synced",
			Namespace:  "tenant-a",
			Finalizers: []string{constant.NabFinalizerName},
			Labels: map[string]string{
				constant.NabSyncLabel: "plain-uuid",
			},
		},
		Status: nacv1alpha1.NonAdminBackupStatus{
			Phase:        nacv1alpha1.NonAdminPhaseCreated,
			VeleroBackup: &nacv1alpha1.VeleroBackup{NACUUID: "plain-uuid"},
		},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(nab).WithObjects(nab).Build()
	reconciler := &NonAdminBackupReconciler{Client: client, Scheme: scheme, OADPNamespace: "openshift-adp"}

	_, err := reconciler.createVeleroBackupAndSyncWithNonAdminBackup(ctx, logr.Discard(), nab.DeepCopy())
	if err == nil {
		t.Fatal("expected a terminal error for a synced NAB whose backup is missing")
	}
	actual := &nacv1alpha1.NonAdminBackup{}
	if err := client.Get(ctx, types.NamespacedName{Name: nab.Name, Namespace: nab.Namespace}, actual); err != nil {
		t.Fatalf("get nab: %v", err)
	}
	if !actual.DeletionTimestamp.IsZero() {
		t.Fatal("regular synced NonAdminBackup must not be reaped when its backup is missing")
	}
}
