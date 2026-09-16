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
	"testing"
	"time"

	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	nacv1alpha1 "github.com/migtools/oadp-non-admin/api/v1alpha1"
	"github.com/migtools/oadp-non-admin/internal/common/constant"
)

func TestNonAdminScheduleCreatesBackingVeleroSchedule(t *testing.T) {
	ctx := context.Background()
	scheme := scheduleTestScheme(t)
	nas := &nacv1alpha1.NonAdminSchedule{
		ObjectMeta: metav1.ObjectMeta{Name: "nightly", Namespace: "tenant-a"},
		Spec: nacv1alpha1.NonAdminScheduleSpec{
			Schedule: "0 2 * * *",
			Template: nacv1alpha1.NonAdminScheduleTemplate{
				BackupSpec: &velerov1.BackupSpec{TTL: metav1.Duration{Duration: 24 * time.Hour}},
			},
		},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(nas).WithObjects(nas).Build()
	reconciler := &NonAdminScheduleReconciler{
		Client:             client,
		Scheme:             scheme,
		OADPNamespace:      "openshift-adp",
		EnforcedBackupSpec: &velerov1.BackupSpec{},
	}
	req := types.NamespacedName{Name: nas.Name, Namespace: nas.Namespace}

	for range 3 {
		if _, err := reconciler.Reconcile(ctx, ctrlRequest(req)); err != nil {
			t.Fatalf("reconcile NonAdminSchedule: %v", err)
		}
	}

	actualNAS := &nacv1alpha1.NonAdminSchedule{}
	if err := client.Get(ctx, req, actualNAS); err != nil {
		t.Fatalf("get NonAdminSchedule: %v", err)
	}
	if actualNAS.Status.VeleroSchedule == nil {
		t.Fatal("expected backing Schedule reference")
	}
	veleroSchedule := &velerov1.Schedule{}
	if err := client.Get(ctx, types.NamespacedName{
		Name:      actualNAS.Status.VeleroSchedule.Name,
		Namespace: "openshift-adp",
	}, veleroSchedule); err != nil {
		t.Fatalf("get backing Velero Schedule: %v", err)
	}
	if got := veleroSchedule.Spec.Template.IncludedNamespaces; len(got) != 1 || got[0] != "tenant-a" {
		t.Fatalf("included namespaces = %v, want [tenant-a]", got)
	}
	if veleroSchedule.Labels[constant.NasOriginNACUUIDLabel] != actualNAS.Status.VeleroSchedule.NACUUID {
		t.Fatal("backing Velero Schedule is missing NAS ownership label")
	}
	if veleroSchedule.Annotations[constant.NasOriginNamespaceAnnotation] != "tenant-a" ||
		veleroSchedule.Annotations[constant.NasOriginNameAnnotation] != "nightly" {
		t.Fatal("backing Velero Schedule is missing NAS ownership annotations")
	}
	if _, found := veleroSchedule.Annotations[constant.NasOriginStorageLocationAnnotation]; found {
		t.Fatal("backing Velero Schedule has unexpected tenant storage location annotation")
	}
}

func TestNonAdminScheduleSynchronizesGeneratedBackup(t *testing.T) {
	ctx := context.Background()
	scheme := scheduleTestScheme(t)
	nas := &nacv1alpha1.NonAdminSchedule{
		ObjectMeta: metav1.ObjectMeta{Name: "nightly", Namespace: "tenant-a"},
		Spec: nacv1alpha1.NonAdminScheduleSpec{
			Template: nacv1alpha1.NonAdminScheduleTemplate{BackupSpec: &velerov1.BackupSpec{StorageLocation: "tenant-bsl"}},
		},
		Status: nacv1alpha1.NonAdminScheduleStatus{
			VeleroSchedule: &nacv1alpha1.VeleroSchedule{NACUUID: "schedule-uuid", Name: "schedule-uuid", Namespace: "openshift-adp"},
		},
	}
	backup := &velerov1.Backup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "schedule-uuid-20260915020000",
			Namespace: "openshift-adp",
			Labels: map[string]string{
				velerov1.ScheduleNameLabel:     "schedule-uuid",
				constant.NasOriginNACUUIDLabel: "schedule-uuid",
			},
			Annotations: scheduleAnnotations(nas),
		},
		Spec: velerov1.BackupSpec{StorageLocation: "backing-bsl"},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(nas).WithObjects(nas, backup).Build()
	reconciler := &NonAdminScheduleReconciler{Client: client, Scheme: scheme, OADPNamespace: "openshift-adp"}
	veleroSchedule := &velerov1.Schedule{ObjectMeta: metav1.ObjectMeta{Name: "schedule-uuid", Namespace: "openshift-adp"}}

	if _, err := reconciler.syncGeneratedBackups(ctx, nas, veleroSchedule); err != nil {
		t.Fatalf("sync generated backups: %v", err)
	}

	actualBackup := &velerov1.Backup{}
	backupKey := types.NamespacedName{Name: backup.Name, Namespace: backup.Namespace}
	if err := client.Get(ctx, backupKey, actualBackup); err != nil {
		t.Fatalf("get generated Backup: %v", err)
	}
	if actualBackup.Labels[constant.NabOriginNACUUIDLabel] != backup.Name {
		t.Fatal("generated Backup is missing its per-run NAB identity")
	}
	child := &nacv1alpha1.NonAdminBackup{}
	if err := client.Get(ctx, types.NamespacedName{Name: backup.Name, Namespace: "tenant-a"}, child); err != nil {
		t.Fatalf("get synchronized NonAdminBackup: %v", err)
	}
	if child.Labels[constant.NabSyncLabel] != backup.Name {
		t.Fatal("child NonAdminBackup is not synchronized with the generated Backup")
	}
	if child.Spec.BackupSpec.StorageLocation != "tenant-bsl" {
		t.Fatalf("child storage location = %q, want tenant NABSL name", child.Spec.BackupSpec.StorageLocation)
	}
}

func TestNonAdminSchedulePreservesPerRunStorageLocation(t *testing.T) {
	ctx := context.Background()
	scheme := scheduleTestScheme(t)
	nas := &nacv1alpha1.NonAdminSchedule{
		ObjectMeta: metav1.ObjectMeta{Name: "nightly", Namespace: "tenant-a"},
		Spec: nacv1alpha1.NonAdminScheduleSpec{
			Template: nacv1alpha1.NonAdminScheduleTemplate{BackupSpec: &velerov1.BackupSpec{StorageLocation: "tenant-bsl-b"}},
		},
		Status: nacv1alpha1.NonAdminScheduleStatus{
			VeleroSchedule: &nacv1alpha1.VeleroSchedule{NACUUID: "schedule-uuid", Name: "schedule-uuid", Namespace: "openshift-adp"},
		},
	}
	backup := &velerov1.Backup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "schedule-uuid-20260915020000",
			Namespace: "openshift-adp",
			Labels: map[string]string{
				velerov1.ScheduleNameLabel:     "schedule-uuid",
				constant.NasOriginNACUUIDLabel: "schedule-uuid",
			},
			Annotations: map[string]string{
				constant.NasOriginNamespaceAnnotation:       "tenant-a",
				constant.NasOriginNameAnnotation:            "nightly",
				constant.NasOriginStorageLocationAnnotation: "tenant-bsl-a",
			},
		},
		Spec: velerov1.BackupSpec{StorageLocation: "backing-bsl-a"},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(nas).WithObjects(nas, backup).Build()
	reconciler := &NonAdminScheduleReconciler{Client: client, Scheme: scheme, OADPNamespace: "openshift-adp"}

	if _, err := reconciler.syncGeneratedBackups(ctx, nas, &velerov1.Schedule{ObjectMeta: metav1.ObjectMeta{Name: "schedule-uuid"}}); err != nil {
		t.Fatalf("sync generated backups: %v", err)
	}
	child := &nacv1alpha1.NonAdminBackup{}
	if err := client.Get(ctx, types.NamespacedName{Name: backup.Name, Namespace: "tenant-a"}, child); err != nil {
		t.Fatalf("get synchronized NonAdminBackup: %v", err)
	}
	if child.Spec.BackupSpec.StorageLocation != "tenant-bsl-a" {
		t.Fatalf("child storage location = %q, want original tenant NABSL name", child.Spec.BackupSpec.StorageLocation)
	}
}

func TestNonAdminScheduleUpdatesBackingScheduleStorageLocationAnnotation(t *testing.T) {
	ctx := context.Background()
	scheme := scheduleTestScheme(t)
	nas := &nacv1alpha1.NonAdminSchedule{
		ObjectMeta: metav1.ObjectMeta{Name: "nightly", Namespace: "tenant-a"},
		Spec: nacv1alpha1.NonAdminScheduleSpec{
			Schedule: "0 2 * * *",
			Template: nacv1alpha1.NonAdminScheduleTemplate{BackupSpec: &velerov1.BackupSpec{StorageLocation: "tenant-bsl-b"}},
		},
		Status: nacv1alpha1.NonAdminScheduleStatus{
			VeleroSchedule: &nacv1alpha1.VeleroSchedule{NACUUID: "schedule-uuid", Name: "schedule-uuid", Namespace: "openshift-adp"},
		},
	}
	veleroSchedule := &velerov1.Schedule{ObjectMeta: metav1.ObjectMeta{
		Name:        "schedule-uuid",
		Namespace:   "openshift-adp",
		Labels:      map[string]string{constant.NasOriginNACUUIDLabel: "schedule-uuid"},
		Annotations: map[string]string{constant.NasOriginNamespaceAnnotation: "tenant-a", constant.NasOriginNameAnnotation: "nightly", constant.NasOriginStorageLocationAnnotation: "tenant-bsl-a"},
	}}
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(veleroSchedule).Build()
	reconciler := &NonAdminScheduleReconciler{Client: client, Scheme: scheme, OADPNamespace: "openshift-adp"}

	if _, err := reconciler.reconcileVeleroSchedule(ctx, nas, &velerov1.BackupSpec{}); err != nil {
		t.Fatalf("reconcile backing Velero Schedule: %v", err)
	}
	if err := client.Get(ctx, types.NamespacedName{Name: veleroSchedule.Name, Namespace: veleroSchedule.Namespace}, veleroSchedule); err != nil {
		t.Fatalf("get backing Velero Schedule: %v", err)
	}
	if veleroSchedule.Annotations[constant.NasOriginStorageLocationAnnotation] != "tenant-bsl-b" {
		t.Fatalf("storage location annotation = %q, want tenant-bsl-b", veleroSchedule.Annotations[constant.NasOriginStorageLocationAnnotation])
	}
}

func TestNonAdminScheduleDeletesExpiredGeneratedChild(t *testing.T) {
	ctx := context.Background()
	scheme := scheduleTestScheme(t)
	nas := &nacv1alpha1.NonAdminSchedule{
		ObjectMeta: metav1.ObjectMeta{Name: "nightly", Namespace: "tenant-a"},
		Status: nacv1alpha1.NonAdminScheduleStatus{
			VeleroSchedule: &nacv1alpha1.VeleroSchedule{NACUUID: "schedule-uuid", Name: "schedule-uuid", Namespace: "openshift-adp"},
		},
	}
	child := &nacv1alpha1.NonAdminBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "schedule-uuid-20260915020000",
			Namespace: "tenant-a",
			Labels: map[string]string{
				constant.NabSyncLabel:          "schedule-uuid-20260915020000",
				constant.NasOriginNACUUIDLabel: "schedule-uuid",
			},
		},
		Status: nacv1alpha1.NonAdminBackupStatus{
			VeleroBackup: &nacv1alpha1.VeleroBackup{NACUUID: "schedule-uuid-20260915020000"},
		},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(nas, child).WithObjects(nas, child).Build()
	reconciler := &NonAdminScheduleReconciler{Client: client, Scheme: scheme, OADPNamespace: "openshift-adp"}

	if _, err := reconciler.syncGeneratedBackups(ctx, nas, &velerov1.Schedule{ObjectMeta: metav1.ObjectMeta{Name: "schedule-uuid"}}); err != nil {
		t.Fatalf("sync generated backups: %v", err)
	}
	err := client.Get(ctx, types.NamespacedName{Name: child.Name, Namespace: child.Namespace}, &nacv1alpha1.NonAdminBackup{})
	if !apierrors.IsNotFound(err) {
		t.Fatalf("expired child NonAdminBackup was not deleted: %v", err)
	}
}

func TestNonAdminScheduleDoesNotDeleteUnverifiedChild(t *testing.T) {
	ctx := context.Background()
	scheme := scheduleTestScheme(t)
	nas := &nacv1alpha1.NonAdminSchedule{
		ObjectMeta: metav1.ObjectMeta{Name: "nightly", Namespace: "tenant-a"},
		Status: nacv1alpha1.NonAdminScheduleStatus{
			VeleroSchedule: &nacv1alpha1.VeleroSchedule{NACUUID: "schedule-uuid", Name: "schedule-uuid", Namespace: "openshift-adp"},
		},
	}
	child := &nacv1alpha1.NonAdminBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tenant-created",
			Namespace: "tenant-a",
			Labels: map[string]string{
				constant.NabSyncLabel:          "tenant-created",
				constant.NasOriginNACUUIDLabel: "schedule-uuid",
			},
		},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(nas, child).WithObjects(nas, child).Build()
	reconciler := &NonAdminScheduleReconciler{Client: client, Scheme: scheme, OADPNamespace: "openshift-adp"}

	if _, err := reconciler.syncGeneratedBackups(ctx, nas, &velerov1.Schedule{ObjectMeta: metav1.ObjectMeta{Name: "schedule-uuid"}}); err != nil {
		t.Fatalf("sync generated backups: %v", err)
	}
	if err := client.Get(ctx, types.NamespacedName{Name: child.Name, Namespace: child.Namespace}, &nacv1alpha1.NonAdminBackup{}); err != nil {
		t.Fatalf("unverified child NonAdminBackup was unexpectedly deleted: %v", err)
	}
}

func TestNonAdminScheduleRejectsForeignBackupNamespace(t *testing.T) {
	ctx := context.Background()
	scheme := scheduleTestScheme(t)
	nas := &nacv1alpha1.NonAdminSchedule{
		ObjectMeta: metav1.ObjectMeta{Name: "nightly", Namespace: "tenant-a"},
		Spec: nacv1alpha1.NonAdminScheduleSpec{
			Schedule: "0 2 * * *",
			Template: nacv1alpha1.NonAdminScheduleTemplate{
				BackupSpec: &velerov1.BackupSpec{IncludedNamespaces: []string{"tenant-b"}},
			},
		},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	reconciler := &NonAdminScheduleReconciler{Client: client, Scheme: scheme, EnforcedBackupSpec: &velerov1.BackupSpec{}}

	if _, err := reconciler.scheduleTemplate(ctx, nas); err == nil {
		t.Fatal("expected a foreign namespace in the template to be rejected")
	}
}

func TestNonAdminScheduleDoesNotSyncForeignTenantBackup(t *testing.T) {
	ctx := context.Background()
	scheme := scheduleTestScheme(t)
	nas := &nacv1alpha1.NonAdminSchedule{
		ObjectMeta: metav1.ObjectMeta{Name: "nightly", Namespace: "tenant-a"},
		Spec: nacv1alpha1.NonAdminScheduleSpec{
			Template: nacv1alpha1.NonAdminScheduleTemplate{BackupSpec: &velerov1.BackupSpec{}},
		},
		Status: nacv1alpha1.NonAdminScheduleStatus{
			VeleroSchedule: &nacv1alpha1.VeleroSchedule{NACUUID: "schedule-uuid", Name: "schedule-uuid"},
		},
	}
	foreignNAS := nas.DeepCopy()
	foreignNAS.Namespace = "tenant-b"
	backup := &velerov1.Backup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "schedule-uuid-20260915020000",
			Namespace: "openshift-adp",
			Labels: map[string]string{
				velerov1.ScheduleNameLabel:     "schedule-uuid",
				constant.NasOriginNACUUIDLabel: "schedule-uuid",
			},
			Annotations: scheduleAnnotations(foreignNAS),
		},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(nas, backup).Build()
	reconciler := &NonAdminScheduleReconciler{Client: client, Scheme: scheme, OADPNamespace: "openshift-adp"}

	if _, err := reconciler.syncGeneratedBackups(ctx, nas, &velerov1.Schedule{ObjectMeta: metav1.ObjectMeta{Name: "schedule-uuid"}}); err != nil {
		t.Fatalf("sync generated backups: %v", err)
	}
	child := &nacv1alpha1.NonAdminBackup{}
	err := client.Get(ctx, types.NamespacedName{Name: backup.Name, Namespace: "tenant-a"}, child)
	if !apierrors.IsNotFound(err) {
		t.Fatalf("foreign Backup was unexpectedly synchronized into tenant-a: %v", err)
	}
}

func TestNonAdminScheduleStatusKeepsFiveNewestBackups(t *testing.T) {
	ctx := context.Background()
	scheme := scheduleTestScheme(t)
	nas := &nacv1alpha1.NonAdminSchedule{
		ObjectMeta: metav1.ObjectMeta{Name: "nightly", Namespace: "tenant-a"},
		Status: nacv1alpha1.NonAdminScheduleStatus{
			VeleroSchedule: &nacv1alpha1.VeleroSchedule{NACUUID: "schedule-uuid"},
		},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(nas).WithObjects(nas).Build()
	reconciler := &NonAdminScheduleReconciler{Client: client, Scheme: scheme}
	base := time.Date(2026, 9, 15, 2, 0, 0, 0, time.UTC)
	backups := make([]velerov1.Backup, 6)
	for index := range backups {
		backups[index] = velerov1.Backup{ObjectMeta: metav1.ObjectMeta{
			Name:              "run-" + string(rune('0'+index)),
			CreationTimestamp: metav1.NewTime(base.Add(time.Duration(index) * time.Hour)),
		}}
	}
	veleroSchedule := &velerov1.Schedule{
		ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(base)},
		Spec:       velerov1.ScheduleSpec{Schedule: "0 2 * * *"},
	}
	if err := reconciler.updateStatus(ctx, nas, veleroSchedule, backups); err != nil {
		t.Fatalf("update NAS status: %v", err)
	}
	if len(nas.Status.RecentBackups) != nonAdminScheduleHistoryLimit {
		t.Fatalf("history length = %d, want %d", len(nas.Status.RecentBackups), nonAdminScheduleHistoryLimit)
	}
	if nas.Status.LastBackup == nil || nas.Status.LastBackup.Name != "run-5" {
		t.Fatalf("last backup = %#v, want run-5", nas.Status.LastBackup)
	}
}

func TestNextRunUsesLastSkippedTime(t *testing.T) {
	lastBackup := metav1.NewTime(time.Date(2026, 9, 15, 2, 0, 0, 0, time.UTC))
	lastSkipped := metav1.NewTime(time.Date(2026, 9, 16, 2, 0, 0, 0, time.UTC))
	actual := nextRun(&velerov1.Schedule{
		Spec: velerov1.ScheduleSpec{Schedule: "0 2 * * *"},
		Status: velerov1.ScheduleStatus{
			LastBackup:  &lastBackup,
			LastSkipped: &lastSkipped,
		},
	})
	want := time.Date(2026, 9, 17, 2, 0, 0, 0, time.UTC)
	if actual == nil || !actual.Time.Equal(want) {
		t.Fatalf("next run = %v, want %v", actual, want)
	}
}

func scheduleTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := nacv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add NAC types to scheme: %v", err)
	}
	if err := velerov1.AddToScheme(scheme); err != nil {
		t.Fatalf("add Velero types to scheme: %v", err)
	}
	return scheme
}

func ctrlRequest(key types.NamespacedName) ctrl.Request {
	return ctrl.Request{NamespacedName: key}
}
