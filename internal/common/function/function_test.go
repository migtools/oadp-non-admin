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

package function

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/assert"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	nacv1alpha1 "github.com/migtools/oadp-non-admin/api/v1alpha1"
	"github.com/migtools/oadp-non-admin/internal/common/constant"
)

var _ = ginkgo.Describe("PLACEHOLDER", func() {})

const (
	testNonAdminBackupNamespace   = "non-admin-backup-namespace"
	testNonAdminBackupName        = "non-admin-backup-name"
	testNonAdminSecondBackupName  = "non-admin-second-backup-name"
	testNonAdminBackupUUID        = "12345678-1234-1234-1234-123456789abc"
	testNonAdminRestoreName       = "non-admin-restore-name"
	testNonAdminRestoreUUID       = "12345678-1234-1234-1234-123456789abc"
	testNonAdminRestoreNamespace  = "non-admin-restore-namespace"
	testNonAdminSecondRestoreName = "non-admin-second-restore-name"
	invalidInputEmptyNamespace    = "Invalid input - empty namespace"
	defaultNS                     = "default"
	gcpProvider                   = "gcp"
	awsProvider                   = "aws"
	key                           = "key"
	bucket                        = "bucket"
	region                        = "region"
	test                          = "test"
	expectedIntZero               = 0
	expectedIntOne                = 1
	expectedIntTwo                = 2
)

func TestGetNonAdminLabels(t *testing.T) {
	expected := map[string]string{
		constant.OadpLabel:      constant.OadpLabelValue,
		constant.ManagedByLabel: constant.ManagedByLabelValue,
	}

	result := GetNonAdminLabels()
	assert.Equal(t, expected, result)
}

func TestGetNonAdminBackupAnnotations(t *testing.T) {
	nonAdminBackup := &nacv1alpha1.NonAdminBackup{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNonAdminBackupNamespace,
			Name:      testNonAdminBackupName,
			UID:       types.UID(testNonAdminBackupUUID),
		},
	}

	expected := map[string]string{
		constant.NabOriginNamespaceAnnotation: testNonAdminBackupNamespace,
		constant.NabOriginNameAnnotation:      testNonAdminBackupName,
	}

	result := GetNonAdminBackupAnnotations(nonAdminBackup.ObjectMeta)
	assert.Equal(t, expected, result)
}

func TestValidateBackupSpec(t *testing.T) {
	tests := []struct {
		spec       *velerov1.BackupSpec
		name       string
		errMessage string
	}{
		{
			name: "namespace different than NonAdminBackup namespace",
			spec: &velerov1.BackupSpec{
				IncludedNamespaces: []string{"namespace1", "namespace2", "namespace3"},
			},
			errMessage: fmt.Sprintf(constant.NABRestrictedErr+", can not contain namespaces other than: %s", "spec.backupSpec.includedNamespaces", "non-admin-backup-namespace"),
		},
		{
			name: "valid spec",
			spec: &velerov1.BackupSpec{
				IncludedNamespaces: []string{testNonAdminBackupNamespace},
			},
		},
		{
			name: "invalid spec, excluded ns specified in nab backup spec",
			spec: &velerov1.BackupSpec{
				ExcludedNamespaces: []string{testNonAdminBackupNamespace},
			},
			errMessage: fmt.Sprintf(constant.NABRestrictedErr, "spec.backupSpec.excludedNamespaces"),
		},
		{
			name: "non admin users specify includeClusterResources as true",
			spec: &velerov1.BackupSpec{
				IncludeClusterResources: ptr.To(true),
			},
			errMessage: fmt.Sprintf(constant.NABRestrictedErr+", can only be set to false", "spec.backupSpec.includeClusterResources"),
		},
		{
			name: "non admin users specify includedClusterScopedResources",
			spec: &velerov1.BackupSpec{
				IncludedClusterScopedResources: []string{"foo-something-coz-lint", "bar"},
			},
			errMessage: fmt.Sprintf(constant.NABRestrictedErr+", must remain empty", "spec.backupSpec.includedScopedResources"),
		},
		{
			name: "non admin backupstoragelocation not found in the NonAdminBackup namespace",
			spec: &velerov1.BackupSpec{
				StorageLocation: "user-defined-backup-storage-location",
			},
			errMessage: "NonAdminBackupStorageLocation not found in the namespace: nonadminbackupstoragelocations.oadp.openshift.io \"user-defined-backup-storage-location\" not found",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			nonAdminBackup := &nacv1alpha1.NonAdminBackup{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNonAdminBackupNamespace,
				},
				Spec: nacv1alpha1.NonAdminBackupSpec{
					BackupSpec: test.spec,
				},
			}
			fakeScheme := runtime.NewScheme()
			if err := nacv1alpha1.AddToScheme(fakeScheme); err != nil {
				t.Fatalf("Failed to register NAC type: %v", err)
			}
			fakeClient := fake.NewClientBuilder().WithScheme(fakeScheme).Build()

			err := ValidateBackupSpec(context.Background(), fakeClient, "oadp-namespace", nonAdminBackup, &velerov1.BackupSpec{})
			if len(test.errMessage) == 0 {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Equal(t, test.errMessage, err.Error())
			}
		})
	}
}

func TestValidateBackupSpecEnforcedFields(t *testing.T) {
	all := "*"

	tests := []struct {
		enforcedValue       any
		overrideValue       any
		name                string
		expectErrorEnforced bool
	}{
		{
			name: "Metadata",
			enforcedValue: velerov1.Metadata{
				Labels: map[string]string{
					constant.OadpLabel: constant.TrueString,
				},
			},
			overrideValue: velerov1.Metadata{
				Labels: map[string]string{
					"openshift/banana": "false",
					constant.OadpLabel: constant.TrueString,
				},
			},
		},
		{
			name:          "IncludedNamespaces",
			enforcedValue: []string{"self-service-namespace"},
			overrideValue: []string{"openshift-adp"},
		},
		{
			name:                "ExcludedNamespaces",
			enforcedValue:       []string{"sample-ns"},
			overrideValue:       []string{},
			expectErrorEnforced: true,
		},
		{
			name:                "IncludeClusterResources",
			enforcedValue:       ptr.To(true),
			overrideValue:       ptr.To(true),
			expectErrorEnforced: true,
		},
		{
			name:                "IncludedClusterScopedResources",
			enforcedValue:       []string{"sample.io"},
			overrideValue:       []string{all},
			expectErrorEnforced: true,
		},
		{
			name:          "IncludedResources",
			enforcedValue: []string{"pods"},
			overrideValue: []string{"secrets"},
		},
		{
			name:          "ExcludedResources",
			enforcedValue: []string{"nonadminbackups.nac.oadp.openshift.io"},
			overrideValue: []string{},
		},
		{
			name:          "ExcludedClusterScopedResources",
			enforcedValue: []string{all},
			overrideValue: []string{},
		},
		{
			name:          "IncludedNamespaceScopedResources",
			enforcedValue: []string{},
			overrideValue: []string{all},
		},
		{
			name:          "ExcludedNamespaceScopedResources",
			enforcedValue: []string{all},
			overrideValue: []string{},
		},
		{
			name: "LabelSelector",
			enforcedValue: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"grapes": constant.TrueString,
				},
			},
			overrideValue: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					constant.OadpLabel: constant.TrueString,
				},
			},
		},
		{
			name: "OrLabelSelectors",
			enforcedValue: []*metav1.LabelSelector{
				{
					MatchLabels: map[string]string{
						"kiwi": constant.TrueString,
					},
				},
				{
					MatchLabels: map[string]string{
						"green": "false",
					},
				},
			},
			overrideValue: []*metav1.LabelSelector{
				{
					MatchLabels: map[string]string{
						constant.OadpLabel: constant.TrueString,
					},
				},
			},
		},
		{
			name:          "SnapshotVolumes",
			enforcedValue: ptr.To(false),
			overrideValue: ptr.To(true),
		},
		{
			name:          "TTL",
			enforcedValue: metav1.Duration{Duration: 12 * time.Hour},      //nolint:revive // just test
			overrideValue: metav1.Duration{Duration: 30 * 24 * time.Hour}, //nolint:revive // just test
		},
		{
			name: "Hooks",
			enforcedValue: velerov1.BackupHooks{
				Resources: []velerov1.BackupResourceHookSpec{
					{
						Name: test,
					},
				},
			},
			overrideValue: velerov1.BackupHooks{
				Resources: []velerov1.BackupResourceHookSpec{
					{
						Name: "another",
					},
				},
			},
		},
		{
			name:          "StorageLocation",
			enforcedValue: "enforced-storage-location",
			overrideValue: "lemon",
		},
		{
			name:          "VolumeSnapshotLocations",
			enforcedValue: []string{awsProvider},
			overrideValue: []string{gcpProvider},
		},
		{
			name:          "DefaultVolumesToRestic",
			enforcedValue: ptr.To(true),
			overrideValue: ptr.To(false),
		},
		{
			name:          "DefaultVolumesToFsBackup",
			enforcedValue: ptr.To(false),
			overrideValue: ptr.To(true),
		},
		{
			name: "OrderedResources",
			enforcedValue: map[string]string{
				"pods": "ns1/pod1,ns2/pod2",
			},
			overrideValue: map[string]string{},
		},
		{
			name:          "CSISnapshotTimeout",
			enforcedValue: metav1.Duration{Duration: 3 * time.Minute},  //nolint:revive // just test
			overrideValue: metav1.Duration{Duration: 30 * time.Minute}, //nolint:revive // just test
		},
		{
			name:          "ItemOperationTimeout",
			enforcedValue: metav1.Duration{Duration: 30 * time.Minute}, //nolint:revive // just test
			overrideValue: metav1.Duration{Duration: time.Hour},
		},
		{
			name: "ResourcePolicy",
			enforcedValue: &corev1.TypedLocalObjectReference{
				Kind: test,
				Name: "example",
			},
			overrideValue: &corev1.TypedLocalObjectReference{},
		},
		{
			name:          "SnapshotMoveData",
			enforcedValue: ptr.To(false),
			overrideValue: ptr.To(true),
		},
		{
			name:          "DataMover",
			enforcedValue: "OADP",
			overrideValue: "third-party",
		},
		{
			name: "UploaderConfig",
			enforcedValue: &velerov1.UploaderConfigForBackup{
				ParallelFilesUpload: 2, //nolint:revive // just test
			},
			overrideValue: &velerov1.UploaderConfigForBackup{
				ParallelFilesUpload: 32, //nolint:revive // just test
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			enforcedSpec := &velerov1.BackupSpec{}
			reflect.ValueOf(enforcedSpec).Elem().FieldByName(test.name).Set(reflect.ValueOf(test.enforcedValue))

			userNonAdminBackup := &nacv1alpha1.NonAdminBackup{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "self-service-namespace",
				},
				Spec: nacv1alpha1.NonAdminBackupSpec{
					BackupSpec: &velerov1.BackupSpec{},
				},
			}

			fakeScheme := runtime.NewScheme()
			if err := nacv1alpha1.AddToScheme(fakeScheme); err != nil {
				t.Fatalf("Failed to register NAC type: %v", err)
			}
			if err := velerov1.AddToScheme(fakeScheme); err != nil {
				t.Fatalf("Failed to register Velero type: %v", err)
			}
			fakeClient := fake.NewClientBuilder().WithScheme(fakeScheme).WithObjects(
				&nacv1alpha1.NonAdminBackupStorageLocation{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "enforced-storage-location",
						Namespace: "self-service-namespace",
					},
					Status: nacv1alpha1.NonAdminBackupStorageLocationStatus{
						VeleroBackupStorageLocation: &nacv1alpha1.VeleroBackupStorageLocation{
							NACUUID: "user-defined-backup-storage-location-uuid",
						},
					},
				},
				&velerov1.BackupStorageLocation{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							constant.NabslOriginNACUUIDLabel: "user-defined-backup-storage-location-uuid",
						},
						Name:      "any-name",
						Namespace: "oadp-namespace",
					},
					Status: velerov1.BackupStorageLocationStatus{
						Phase: velerov1.BackupStorageLocationPhaseAvailable,
					},
				},
			).Build()

			err := ValidateBackupSpec(context.Background(), fakeClient, "oadp-namespace", userNonAdminBackup, enforcedSpec)
			if err != nil {
				t.Errorf("not setting backup spec field '%v' test failed: %v", test.name, err)
			}

			reflect.ValueOf(userNonAdminBackup.Spec.BackupSpec).Elem().FieldByName(test.name).Set(reflect.ValueOf(test.enforcedValue))
			err = ValidateBackupSpec(context.Background(), fakeClient, "oadp-namespace", userNonAdminBackup, enforcedSpec)
			if test.expectErrorEnforced {
				if err == nil {
					t.Errorf("expected error when setting field '%v' to enforced value, but got none", test.name)
				}
			} else {
				if err != nil {
					t.Errorf("setting backup spec field '%v' with enforced value test failed: %v", test.name, err)
				}
			}

			reflect.ValueOf(userNonAdminBackup.Spec.BackupSpec).Elem().FieldByName(test.name).Set(reflect.ValueOf(test.overrideValue))
			err = ValidateBackupSpec(context.Background(), fakeClient, "oadp-namespace", userNonAdminBackup, enforcedSpec)
			if err == nil {
				t.Errorf("setting backup spec field '%v' with value overriding enforcement test failed: %v", test.name, err)
			}
			t.Run("Ensure all backup spec fields were tested", func(t *testing.T) {
				backupSpecFields := []string{}
				for _, test := range tests {
					backupSpecFields = append(backupSpecFields, test.name)
				}
				backupSpec := reflect.ValueOf(&velerov1.BackupSpec{}).Elem()

				for index := range backupSpec.NumField() {
					if !slices.Contains(backupSpecFields, backupSpec.Type().Field(index).Name) {
						t.Errorf("backup spec field '%v' is not tested", backupSpec.Type().Field(index).Name)
					}
				}
				if backupSpec.NumField() != len(tests) {
					t.Errorf("list of tests have different number of elements")
				}
			})
		})
	}
}

func TestValidateRestoreSpec(t *testing.T) {
	tests := []struct {
		name            string
		errorMessage    string
		nonAdminRestore *nacv1alpha1.NonAdminRestore
		objects         []client.Object
	}{
		{
			name: "[invalid] spec.restoreSpec.backupName not set",
			nonAdminRestore: &nacv1alpha1.NonAdminRestore{
				Spec: nacv1alpha1.NonAdminRestoreSpec{
					RestoreSpec: &velerov1.RestoreSpec{},
				},
			},
			errorMessage: "NonAdminRestore spec.restoreSpec.backupName is not set",
		},
		{
			name: "[invalid] spec.restoreSpec.backupName does not exist",
			nonAdminRestore: &nacv1alpha1.NonAdminRestore{
				Spec: nacv1alpha1.NonAdminRestoreSpec{
					RestoreSpec: &velerov1.RestoreSpec{
						BackupName: "john",
					},
				},
			},
			errorMessage: "NonAdminRestore spec.restoreSpec.backupName is invalid: nonadminbackups.oadp.openshift.io \"john\" not found",
		},
		{
			name: "[invalid] spec.restoreSpec.backupName not ready",
			nonAdminRestore: &nacv1alpha1.NonAdminRestore{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: defaultNS,
				},
				Spec: nacv1alpha1.NonAdminRestoreSpec{
					RestoreSpec: &velerov1.RestoreSpec{
						BackupName: "peter",
					},
				},
			},
			objects: []client.Object{
				&nacv1alpha1.NonAdminBackup{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "peter",
						Namespace: defaultNS,
					},
					Status: nacv1alpha1.NonAdminBackupStatus{
						Phase: nacv1alpha1.NonAdminPhaseBackingOff,
					},
				},
			},
			errorMessage: "NonAdminRestore spec.restoreSpec.backupName is invalid: NonAdminBackup is not ready to be restored",
		},
		{
			name: "[valid] spec.restoreSpec.backupName is ready",
			nonAdminRestore: &nacv1alpha1.NonAdminRestore{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: defaultNS,
				},
				Spec: nacv1alpha1.NonAdminRestoreSpec{
					RestoreSpec: &velerov1.RestoreSpec{
						BackupName: "steve",
					},
				},
			},
			objects: []client.Object{
				&nacv1alpha1.NonAdminBackup{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "steve",
						Namespace: defaultNS,
					},
					Status: nacv1alpha1.NonAdminBackupStatus{
						Phase: nacv1alpha1.NonAdminPhaseCreated,
					},
				},
			},
		},
		{
			name: "[invalid] spec.restoreSpec.scheduleName is restricted",
			nonAdminRestore: &nacv1alpha1.NonAdminRestore{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: defaultNS,
				},
				Spec: nacv1alpha1.NonAdminRestoreSpec{
					RestoreSpec: &velerov1.RestoreSpec{
						ScheduleName: "foo-schedule",
					},
				},
			},
			objects: []client.Object{
				&nacv1alpha1.NonAdminBackup{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "foo-schedule",
						Namespace: defaultNS,
					},
					Status: nacv1alpha1.NonAdminBackupStatus{
						Phase: nacv1alpha1.NonAdminPhaseCreated,
					},
				},
			},
			errorMessage: "NonAdminRestore nonAdminRestore.spec.restoreSpec.scheduleName is restricted",
		},
		{
			name: "[invalid] spec.restoreSpec.includedNamespaces is restricted",
			nonAdminRestore: &nacv1alpha1.NonAdminRestore{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: defaultNS,
				},
				Spec: nacv1alpha1.NonAdminRestoreSpec{
					RestoreSpec: &velerov1.RestoreSpec{
						BackupName:         "foo-backup-incl",
						IncludedNamespaces: []string{"foo-bar-ns"},
					},
				},
			},
			objects: []client.Object{
				&nacv1alpha1.NonAdminBackup{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "foo-backup-incl",
						Namespace: defaultNS,
					},
					Status: nacv1alpha1.NonAdminBackupStatus{
						Phase: nacv1alpha1.NonAdminPhaseCreated,
					},
				},
			},
			errorMessage: "NonAdminRestore nonAdminRestore.spec.restoreSpec.includedNamespaces is restricted",
		},
		{
			name: "[invalid] spec.restoreSpec.excludedNamespaces is restricted",
			nonAdminRestore: &nacv1alpha1.NonAdminRestore{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: defaultNS,
				},
				Spec: nacv1alpha1.NonAdminRestoreSpec{
					RestoreSpec: &velerov1.RestoreSpec{
						BackupName:         "foo-backup-ns-ex",
						ExcludedNamespaces: []string{"foo-bar-ns"},
					},
				},
			},
			objects: []client.Object{
				&nacv1alpha1.NonAdminBackup{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "foo-backup-ns-ex",
						Namespace: defaultNS,
					},
					Status: nacv1alpha1.NonAdminBackupStatus{
						Phase: nacv1alpha1.NonAdminPhaseCreated,
					},
				},
			},
			errorMessage: "NonAdminRestore nonAdminRestore.spec.restoreSpec.excludedNamespaces is restricted",
		},
		{
			name: "[invalid] spec.restoreSpec.namespaceMapping is restricted",
			nonAdminRestore: &nacv1alpha1.NonAdminRestore{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: defaultNS,
				},
				Spec: nacv1alpha1.NonAdminRestoreSpec{
					RestoreSpec: &velerov1.RestoreSpec{
						BackupName: "foo-backup-ns-map",
						NamespaceMapping: map[string]string{
							"foo-ns": "bar-ns",
						},
					},
				},
			},
			objects: []client.Object{
				&nacv1alpha1.NonAdminBackup{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "foo-backup-ns-map",
						Namespace: defaultNS,
					},
					Status: nacv1alpha1.NonAdminBackupStatus{
						Phase: nacv1alpha1.NonAdminPhaseCreated,
					},
				},
			},
			errorMessage: "NonAdminRestore nonAdminRestore.spec.restoreSpec.namespaceMapping is restricted",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fakeScheme := runtime.NewScheme()
			if err := nacv1alpha1.AddToScheme(fakeScheme); err != nil {
				t.Fatalf("Failed to register NAC type: %v", err)
			}
			fakeClient := fake.NewClientBuilder().WithScheme(fakeScheme).WithObjects(test.objects...).Build()
			err := ValidateRestoreSpec(context.Background(), fakeClient, test.nonAdminRestore, &velerov1.RestoreSpec{})
			if err != nil {
				if test.errorMessage != err.Error() {
					t.Errorf("test '%s' failed: error messages differ. Expected %v, got %v", test.name, test.errorMessage, err)
				}
				return
			}
			if test.errorMessage != constant.EmptyString {
				t.Errorf("test '%s' failed: expected test to error with '%v'", test.name, err)
			}
		})
	}
}

func TestValidateRestoreSpecEnforcedFields(t *testing.T) {
	tests := []struct {
		enforcedValue       any
		overrideValue       any
		name                string
		expectErrorEnforced bool
	}{
		{
			name:                "IncludedNamespaces",
			enforcedValue:       []string{"self-service-namespace"},
			overrideValue:       []string{"openshift-monitor"},
			expectErrorEnforced: true,
		},
		{
			name:                "ExcludedNamespaces",
			enforcedValue:       []string{"openshift-monitor"},
			overrideValue:       []string{"cherry"},
			expectErrorEnforced: true,
		},
		{
			name:          "BackupName",
			enforcedValue: "self-service-backup",
			overrideValue: "another",
		},
		{
			name:                "ScheduleName",
			enforcedValue:       "allowed",
			overrideValue:       "not-alllowed",
			expectErrorEnforced: true,
		},
		{
			name:          "IncludedResources",
			enforcedValue: []string{"deployments"},
			overrideValue: []string{"secrets"},
		},
		{
			name:          "ExcludedResources",
			enforcedValue: []string{"foobar.io"},
			overrideValue: []string{},
		},
		{
			name:          "IncludeClusterResources",
			enforcedValue: ptr.To(true),
			overrideValue: ptr.To(false),
		},
		{
			name: "Hooks",
			enforcedValue: velerov1.RestoreHooks{
				Resources: []velerov1.RestoreResourceHookSpec{
					{
						Name: "computer",
					},
				},
			},
			overrideValue: velerov1.RestoreHooks{
				Resources: []velerov1.RestoreResourceHookSpec{
					{
						Name: "microwave",
					},
				},
			},
		},
		{
			name: "LabelSelector",
			enforcedValue: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"grapes": constant.TrueString,
				},
			},
			overrideValue: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					constant.OadpLabel: constant.TrueString,
				},
			},
		},
		{
			name: "OrLabelSelectors",
			enforcedValue: []*metav1.LabelSelector{
				{
					MatchLabels: map[string]string{
						"kiwi": constant.TrueString,
					},
				},
				{
					MatchLabels: map[string]string{
						"green": "no",
					},
				},
			},
			overrideValue: []*metav1.LabelSelector{
				{
					MatchLabels: map[string]string{
						constant.OadpLabel: constant.TrueString,
					},
				},
			},
		},
		{
			name:          "ItemOperationTimeout",
			enforcedValue: metav1.Duration{Duration: 30 * time.Minute}, //nolint:revive // just test
			overrideValue: metav1.Duration{Duration: time.Hour},
		},
		{
			name: "UploaderConfig",
			enforcedValue: &velerov1.UploaderConfigForRestore{
				ParallelFilesDownload: 2, //nolint:revive // just test
			},
			overrideValue: &velerov1.UploaderConfigForRestore{
				ParallelFilesDownload: 32, //nolint:revive // just test
			},
		},
		{
			name:          "RestorePVs",
			enforcedValue: ptr.To(true),
			overrideValue: ptr.To(false),
		},
		{
			name:          "PreserveNodePorts",
			enforcedValue: ptr.To(true),
			overrideValue: ptr.To(false),
		},
		{
			name: "NamespaceMapping",
			enforcedValue: map[string]string{
				"video": "game",
			},
			overrideValue: map[string]string{
				"movie": "star",
			},
			expectErrorEnforced: true,
		},
		{
			name: "RestoreStatus",
			enforcedValue: &velerov1.RestoreStatusSpec{
				IncludedResources: []string{"conditions"},
			},
			overrideValue: &velerov1.RestoreStatusSpec{
				IncludedResources: []string{"phase"},
			},
		},
		{
			name:          "ExistingResourcePolicy",
			enforcedValue: velerov1.PolicyTypeNone,
			overrideValue: velerov1.PolicyTypeUpdate,
		},
		{
			name: "ResourceModifier",
			enforcedValue: &corev1.TypedLocalObjectReference{
				Name: "banana",
			},
			overrideValue: &corev1.TypedLocalObjectReference{
				Name: "melon",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			enforcedSpec := &velerov1.RestoreSpec{}
			reflect.ValueOf(enforcedSpec).Elem().FieldByName(test.name).Set(reflect.ValueOf(test.enforcedValue))
			userNonAdminRestore := &nacv1alpha1.NonAdminRestore{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "self-service-namespace",
				},
				Spec: nacv1alpha1.NonAdminRestoreSpec{
					RestoreSpec: &velerov1.RestoreSpec{
						BackupName: "self-service-backup",
					},
				},
			}

			fakeScheme := runtime.NewScheme()
			if err := nacv1alpha1.AddToScheme(fakeScheme); err != nil {
				t.Fatalf("Failed to register NAC type: %v", err)
			}
			fakeClient := fake.NewClientBuilder().WithScheme(fakeScheme).WithObjects([]client.Object{
				&nacv1alpha1.NonAdminBackup{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "self-service-backup",
						Namespace: "self-service-namespace",
					},
					Status: nacv1alpha1.NonAdminBackupStatus{
						Phase: nacv1alpha1.NonAdminPhaseCreated,
					},
				},
			}...).Build()

			err := ValidateRestoreSpec(context.Background(), fakeClient, userNonAdminRestore, enforcedSpec)
			if err != nil {
				t.Errorf("not setting restore spec field '%v' test failed: %v", test.name, err)
			}

			reflect.ValueOf(userNonAdminRestore.Spec.RestoreSpec).Elem().FieldByName(test.name).Set(reflect.ValueOf(test.enforcedValue))
			err = ValidateRestoreSpec(context.Background(), fakeClient, userNonAdminRestore, enforcedSpec)
			if test.expectErrorEnforced {
				if err == nil {
					t.Errorf("expected error when setting field '%v' to enforced value, but got none", test.name)
				}
			} else {
				if err != nil {
					t.Errorf("setting backup spec field '%v' with enforced value test failed: %v", test.name, err)
				}
			}
			reflect.ValueOf(userNonAdminRestore.Spec.RestoreSpec).Elem().FieldByName(test.name).Set(reflect.ValueOf(test.overrideValue))
			err = ValidateRestoreSpec(context.Background(), fakeClient, userNonAdminRestore, enforcedSpec)
			if err == nil {
				t.Errorf("setting restore spec field '%v' with value overriding enforcement test failed: %v", test.name, err)
			}
		})
	}
	t.Run("Ensure all restore spec fields were tested", func(t *testing.T) {
		restoreSpecFields := []string{}
		for _, test := range tests {
			restoreSpecFields = append(restoreSpecFields, test.name)
		}
		restoreSpec := reflect.ValueOf(&velerov1.RestoreSpec{}).Elem()
		for index := range restoreSpec.NumField() {
			if !slices.Contains(restoreSpecFields, restoreSpec.Type().Field(index).Name) {
				t.Errorf("restore spec field '%v' is not tested", restoreSpec.Type().Field(index).Name)
			}
		}
		if restoreSpec.NumField() != len(tests) {
			t.Errorf("list of tests have different number of elements")
		}
	})
}

func TestValidateBslSpec(t *testing.T) {
	fakeScheme := runtime.NewScheme()
	if err := corev1.AddToScheme(fakeScheme); err != nil {
		t.Fatalf("Failed to register corev1 type: %v", err)
	}

	tests := []struct {
		name         string
		errorMessage string
		nonAdminBsl  *nacv1alpha1.NonAdminBackupStorageLocation
		objects      []client.Object
	}{
		{
			name: "[invalid] spec.bslSpec.credential not set",
			nonAdminBsl: &nacv1alpha1.NonAdminBackupStorageLocation{
				Spec: nacv1alpha1.NonAdminBackupStorageLocationSpec{
					BackupStorageLocationSpec: &velerov1.BackupStorageLocationSpec{
						Credential: nil,
					},
				},
			},
			errorMessage: "NonAdminBackupStorageLocation spec.bslSpec.credential is not set",
		},
		{
			name: "[invalid] spec.bslSpec.credential Name or Key not set",
			nonAdminBsl: &nacv1alpha1.NonAdminBackupStorageLocation{
				Spec: nacv1alpha1.NonAdminBackupStorageLocationSpec{
					BackupStorageLocationSpec: &velerov1.BackupStorageLocationSpec{
						Credential: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: constant.EmptyString,
							},
							Key: "non-empty-key",
						},
					},
				},
			},
			errorMessage: "NonAdminBackupStorageLocation spec.bslSpec.credential.name or spec.bslSpec.credential.key is not set",
		},
		{
			name: "[invalid] secret not found",
			nonAdminBsl: &nacv1alpha1.NonAdminBackupStorageLocation{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-namespace-1",
				},
				Spec: nacv1alpha1.NonAdminBackupStorageLocationSpec{
					BackupStorageLocationSpec: &velerov1.BackupStorageLocationSpec{
						Credential: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "test-secret-1",
							},
							Key: key,
						},
					},
				},
			},
			errorMessage: "BSL credentials secret not found: secrets \"test-secret-1\" not found",
			objects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: "different-secret", Namespace: "test-namespace-1"},
				},
			},
		},
		{
			name: "[valid] secret found",
			nonAdminBsl: &nacv1alpha1.NonAdminBackupStorageLocation{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-namespace-2",
				},
				Spec: nacv1alpha1.NonAdminBackupStorageLocationSpec{
					BackupStorageLocationSpec: &velerov1.BackupStorageLocationSpec{
						Credential: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "test-secret-2",
							},
							Key: key,
						},
					},
				},
			},
			errorMessage: constant.EmptyString,
			objects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: "test-secret-2", Namespace: "test-namespace-2"},
				},
			},
		},
		{
			name: "[invalid] spec.bslSpec.default is set to true",
			nonAdminBsl: &nacv1alpha1.NonAdminBackupStorageLocation{
				Spec: nacv1alpha1.NonAdminBackupStorageLocationSpec{
					BackupStorageLocationSpec: &velerov1.BackupStorageLocationSpec{
						Default: true,
					},
				},
			},
			errorMessage: "NonAdminBackupStorageLocation cannot be used as a default BSL",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(fakeScheme).WithObjects(test.objects...).Build()

			err := ValidateBslSpec(context.Background(), fakeClient, test.nonAdminBsl)
			if err != nil {
				if test.errorMessage != err.Error() {
					t.Errorf("test '%s' failed: error messages differ. Expected '%v', got '%v'", test.name, test.errorMessage, err)
				}
				return
			}
			if test.errorMessage != "" {
				t.Errorf("test '%s' failed: expected error '%v' but got none", test.name, test.errorMessage)
			}
		})
	}
}

func TestGenerateNacObjectNameWithUUID(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		nabName   string
	}{
		{
			name:      "Valid names without truncation",
			namespace: defaultNS,
			nabName:   "my-backup",
		},
		{
			name:      "Truncate nabName due to length",
			namespace: "some",
			nabName:   strings.Repeat("q", constant.MaximumNacObjectNameLength+10), // too long for DNS limit
		},
		{
			name:      "Truncate very long namespace and very long name",
			namespace: strings.Repeat("w", constant.MaximumNacObjectNameLength+10),
			nabName:   strings.Repeat("e", constant.MaximumNacObjectNameLength+10),
		},
		{
			name:      "nabName empty",
			namespace: "example",
			nabName:   constant.EmptyString,
		},
		{
			name:      "namespace empty",
			namespace: constant.EmptyString,
			nabName:   "my-backup",
		},
		{
			name:      "very long name and namespace empty",
			namespace: constant.EmptyString,
			nabName:   strings.Repeat("r", constant.MaximumNacObjectNameLength+10),
		},
		{
			name:      "very long namespace and name empty",
			namespace: strings.Repeat("t", constant.MaximumNacObjectNameLength+10),
			nabName:   constant.EmptyString,
		},
		{
			name:      "empty namespace and empty name",
			namespace: constant.EmptyString,
			nabName:   constant.EmptyString,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateNacObjectUUID(tt.namespace, tt.nabName)

			// Check length, don't use constant.MaximumNacObjectNameLength here
			// so if constant is changed test needs to be changed as well
			if len(result) > validation.DNS1123LabelMaxLength {
				t.Errorf("Generated name is too long: %s", result)
			}

			// Extract the last 36 characters, which should be the UUID
			if len(result) < 36 {
				t.Errorf("Generated name is too short to contain a valid UUID: %s", result)
			}
			uuidPart := result[len(result)-36:] // The UUID is always the last 36 characters

			// Attempt to parse the UUID part
			if _, err := uuid.Parse(uuidPart); err != nil {
				t.Errorf("Last part is not a valid UUID: %s", uuidPart)
			}

			// Check if no double hyphens are present
			if strings.Contains(result, "--") {
				t.Errorf("Generated name contains double hyphens: %s", result)
			}
		})
	}
}

func TestGetVeleroBackupByLabel(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	ctx := context.Background()
	ctx = ctrl.LoggerInto(ctx, log)
	scheme := runtime.NewScheme()
	const testAppStr = "test-app"

	if err := velerov1.AddToScheme(scheme); err != nil {
		t.Fatalf("Failed to register VeleroBackup type in TestGetVeleroBackupByLabel: %v", err)
	}

	tests := []struct {
		name          string
		namespace     string
		labelValue    string
		expected      *velerov1.Backup
		expectedError error
		mockBackups   []velerov1.Backup
	}{
		{
			name:       "Single VeleroBackup found",
			namespace:  defaultNS,
			labelValue: testAppStr,
			mockBackups: []velerov1.Backup{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: defaultNS,
						Name:      "backup1",
						Labels:    map[string]string{constant.NabOriginNACUUIDLabel: testAppStr},
					},
				},
			},
			expected:      &velerov1.Backup{ObjectMeta: metav1.ObjectMeta{Namespace: defaultNS, Name: "backup1", Labels: map[string]string{constant.NabOriginNACUUIDLabel: testAppStr}}},
			expectedError: nil,
		},
		{
			name:          "No VeleroBackups found",
			namespace:     defaultNS,
			labelValue:    testAppStr,
			mockBackups:   []velerov1.Backup{},
			expected:      nil,
			expectedError: nil,
		},
		{
			name:       "Multiple VeleroBackups found",
			namespace:  defaultNS,
			labelValue: testAppStr,
			mockBackups: []velerov1.Backup{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: defaultNS,
						Name:      "backup2",
						Labels:    map[string]string{constant.NabOriginNACUUIDLabel: testAppStr},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: defaultNS,
						Name:      "backup3",
						Labels:    map[string]string{constant.NabOriginNACUUIDLabel: testAppStr},
					},
				},
			},
			expected:      nil,
			expectedError: errors.New("multiple VeleroBackup objects found with label openshift.io/oadp-nab-origin-nacuuid=test-app in namespace 'default'"),
		},
		{
			name:          invalidInputEmptyNamespace,
			namespace:     constant.EmptyString,
			labelValue:    testAppStr,
			mockBackups:   []velerov1.Backup{},
			expected:      nil,
			expectedError: errors.New("invalid input: namespace=\"\", labelKey=\"openshift.io/oadp-nab-origin-nacuuid\", labelValue=\"test-app\""),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var objects []client.Object
			for _, backup := range tt.mockBackups {
				backupCopy := backup // Create a copy to avoid memory aliasing
				objects = append(objects, &backupCopy)
			}
			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()

			result, err := GetVeleroBackupByLabel(ctx, client, tt.namespace, tt.labelValue)

			if tt.expectedError != nil {
				assert.EqualError(t, err, tt.expectedError.Error())
			} else {
				assert.NoError(t, err)
				if tt.expected != nil && result != nil {
					assert.Equal(t, tt.expected.Name, result.Name, "VeleroBackup Name should match")
					assert.Equal(t, tt.expected.Namespace, result.Namespace, "VeleroBackup Namespace should match")
					assert.Equal(t, tt.expected.Labels, result.Labels, "VeleroBackup Labels should match")
				} else {
					assert.Nil(t, result, "Expected result should be nil")
				}
			}
		})
	}
}

func TestGetVeleroRestoreByLabel(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	ctx := context.Background()
	ctx = ctrl.LoggerInto(ctx, log)
	scheme := runtime.NewScheme()
	const testAppStr = "test-app"

	if err := velerov1.AddToScheme(scheme); err != nil {
		t.Fatalf("Failed to register VeleroRestore type in TestGetVeleroRestoreByLabel: %v", err)
	}

	tests := []struct {
		name          string
		namespace     string
		labelValue    string
		expected      *velerov1.Restore
		expectedError error
		mockRestores  []velerov1.Restore
	}{
		{
			name:       "Single VeleroBackup found",
			namespace:  defaultNS,
			labelValue: testAppStr,
			mockRestores: []velerov1.Restore{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: defaultNS,
						Name:      "restore1",
						Labels:    map[string]string{constant.NarOriginNACUUIDLabel: testAppStr},
					},
				},
			},
			expected:      &velerov1.Restore{ObjectMeta: metav1.ObjectMeta{Namespace: defaultNS, Name: "restore1", Labels: map[string]string{constant.NarOriginNACUUIDLabel: testAppStr}}},
			expectedError: nil,
		},
		{
			name:          "No VeleroRestores found",
			namespace:     defaultNS,
			labelValue:    testAppStr,
			mockRestores:  []velerov1.Restore{},
			expected:      nil,
			expectedError: nil,
		},
		{
			name:       "Multiple VeleroRestores found",
			namespace:  defaultNS,
			labelValue: testAppStr,
			mockRestores: []velerov1.Restore{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: defaultNS,
						Name:      "restore2",
						Labels:    map[string]string{constant.NarOriginNACUUIDLabel: testAppStr},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: defaultNS,
						Name:      "restore3",
						Labels:    map[string]string{constant.NarOriginNACUUIDLabel: testAppStr},
					},
				},
			},
			expected:      nil,
			expectedError: errors.New("multiple VeleroRestore objects found with label openshift.io/oadp-nar-origin-nacuuid=test-app in namespace 'default'"),
		},
		{
			name:          invalidInputEmptyNamespace,
			namespace:     constant.EmptyString,
			labelValue:    testAppStr,
			mockRestores:  []velerov1.Restore{},
			expected:      nil,
			expectedError: errors.New("invalid input: namespace=\"\", labelKey=\"openshift.io/oadp-nar-origin-nacuuid\", labelValue=\"test-app\""),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var objects []client.Object
			for _, restore := range tt.mockRestores {
				restoreCopy := restore // Create a copy to avoid memory aliasing
				objects = append(objects, &restoreCopy)
			}
			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()

			result, err := GetVeleroRestoreByLabel(ctx, client, tt.namespace, tt.labelValue)

			if tt.expectedError != nil {
				assert.EqualError(t, err, tt.expectedError.Error())
			} else {
				assert.NoError(t, err)
				if tt.expected != nil && result != nil {
					assert.Equal(t, tt.expected.Name, result.Name, "VeleroRestore Name should match")
					assert.Equal(t, tt.expected.Namespace, result.Namespace, "VeleroRestore Namespace should match")
					assert.Equal(t, tt.expected.Labels, result.Labels, "VeleroRestore Labels should match")
				} else {
					assert.Nil(t, result, "Expected result should be nil")
				}
			}
		})
	}
}

func TestCheckVeleroBackupMetadata(t *testing.T) {
	tests := []struct {
		backup   *velerov1.Backup
		name     string
		expected bool
	}{
		{
			name:     "Velero Backup without required non admin labels and annotations",
			backup:   &velerov1.Backup{},
			expected: false,
		},
		{
			name: "Velero Backup without required non admin annotations",
			backup: &velerov1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						constant.OadpLabel:      constant.OadpLabelValue,
						constant.ManagedByLabel: constant.ManagedByLabelValue,
					},
				},
			},
			expected: false,
		},
		{
			name: "Velero Backup with wrong required non admin label",
			backup: &velerov1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						constant.OadpLabel:      constant.OadpLabelValue,
						constant.ManagedByLabel: "foo",
					},
				},
			},
			expected: false,
		},
		{
			name: "Velero Backup without required non admin labels",
			backup: &velerov1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constant.NabOriginNamespaceAnnotation: testNonAdminBackupNamespace,
						constant.NabOriginNameAnnotation:      testNonAdminBackupName,
					},
				},
			},
			expected: false,
		},
		{
			name: "Velero Backup with wrong required non admin annotation [empty]",
			backup: &velerov1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						constant.OadpLabel:             constant.OadpLabelValue,
						constant.ManagedByLabel:        constant.ManagedByLabelValue,
						constant.NabOriginNACUUIDLabel: testNonAdminBackupUUID,
					},
					Annotations: map[string]string{
						constant.NabOriginNamespaceAnnotation: constant.EmptyString,
						constant.NabOriginNameAnnotation:      testNonAdminBackupName,
					},
				},
			},
			expected: false,
		},
		{
			name: "Velero Backup with wrong required non admin label [long]",
			backup: &velerov1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						constant.OadpLabel:             constant.OadpLabelValue,
						constant.ManagedByLabel:        strings.Repeat("ll", validation.DNS1123SubdomainMaxLength),
						constant.NabOriginNACUUIDLabel: testNonAdminBackupUUID,
					},
					Annotations: map[string]string{
						constant.NabOriginNamespaceAnnotation: testNonAdminBackupNamespace,
						constant.NabOriginNameAnnotation:      testNonAdminBackupName,
					},
				},
			},
			expected: false,
		},
		{
			name: "Velero Backup with wrong required non admin name annotation [long]",
			backup: &velerov1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						constant.OadpLabel:             constant.OadpLabelValue,
						constant.ManagedByLabel:        constant.ManagedByLabelValue,
						constant.NabOriginNACUUIDLabel: testNonAdminBackupUUID,
					},
					Annotations: map[string]string{
						constant.NabOriginNamespaceAnnotation: testNonAdminBackupNamespace,
						constant.NabOriginNameAnnotation:      strings.Repeat("nm", validation.DNS1123SubdomainMaxLength),
					},
				},
			},
			expected: false,
		},
		{
			name: "Velero Backup with wrong required non admin name annotation [long]",
			backup: &velerov1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						constant.OadpLabel:             constant.OadpLabelValue,
						constant.ManagedByLabel:        constant.ManagedByLabelValue,
						constant.NabOriginNACUUIDLabel: testNonAdminBackupUUID,
					},
					Annotations: map[string]string{
						constant.NabOriginNamespaceAnnotation: strings.Repeat("ns", validation.DNS1123SubdomainMaxLength),
						constant.NabOriginNameAnnotation:      testNonAdminBackupName,
					},
				},
			},
			expected: false,
		},
		{
			name: "Velero Backup with required non admin labels and annotations",
			backup: &velerov1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						constant.OadpLabel:             constant.OadpLabelValue,
						constant.ManagedByLabel:        constant.ManagedByLabelValue,
						constant.NabOriginNACUUIDLabel: testNonAdminBackupUUID,
					},
					Annotations: map[string]string{
						constant.NabOriginNamespaceAnnotation: testNonAdminBackupNamespace,
						constant.NabOriginNameAnnotation:      testNonAdminBackupName,
					},
				},
			},
			expected: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := CheckVeleroBackupMetadata(test.backup)
			assert.Equal(t, test.expected, result)
		})
	}
}

func TestCheckVeleroRestoreMetadata(t *testing.T) {
	tests := []struct {
		restore  *velerov1.Restore
		name     string
		expected bool
	}{
		{
			name:     "Velero Restore without required non admin labels and annotations",
			restore:  &velerov1.Restore{},
			expected: false,
		},
		{
			name: "Velero Restore without required non admin annotations",
			restore: &velerov1.Restore{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						constant.OadpLabel:      constant.OadpLabelValue,
						constant.ManagedByLabel: constant.ManagedByLabelValue,
					},
				},
			},
			expected: false,
		},
		{
			name: "Velero Restore with wrong required non admin label",
			restore: &velerov1.Restore{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						constant.OadpLabel:      constant.OadpLabelValue,
						constant.ManagedByLabel: "foo",
					},
				},
			},
			expected: false,
		},
		{
			name: "Velero Restore without required non admin labels",
			restore: &velerov1.Restore{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						constant.NarOriginNamespaceAnnotation: testNonAdminRestoreNamespace,
						constant.NarOriginNameAnnotation:      testNonAdminRestoreName,
					},
				},
			},
			expected: false,
		},
		{
			name: "Velero Restore with wrong required non admin annotation [empty]",
			restore: &velerov1.Restore{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						constant.OadpLabel:             constant.OadpLabelValue,
						constant.ManagedByLabel:        constant.ManagedByLabelValue,
						constant.NarOriginNACUUIDLabel: testNonAdminRestoreUUID,
					},
					Annotations: map[string]string{
						constant.NarOriginNamespaceAnnotation: constant.EmptyString,
						constant.NarOriginNameAnnotation:      testNonAdminRestoreName,
					},
				},
			},
			expected: false,
		},
		{
			name: "Velero Restore with required non admin labels and annotations",
			restore: &velerov1.Restore{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						constant.OadpLabel:             constant.OadpLabelValue,
						constant.ManagedByLabel:        constant.ManagedByLabelValue,
						constant.NarOriginNACUUIDLabel: testNonAdminRestoreUUID,
					},
					Annotations: map[string]string{
						constant.NarOriginNamespaceAnnotation: testNonAdminRestoreNamespace,
						constant.NarOriginNameAnnotation:      testNonAdminRestoreName,
					},
				},
			},
			expected: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := CheckVeleroRestoreMetadata(test.restore)
			assert.Equal(t, test.expected, result)
		})
	}
}

func TestGetVeleroDeleteBackupRequestByLabel(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	ctx := context.Background()
	ctx = ctrl.LoggerInto(ctx, log)
	scheme := runtime.NewScheme()
	const testAppStr = "test-app"

	if err := velerov1.AddToScheme(scheme); err != nil {
		t.Fatalf("Failed to register DeleteBackupRequest type in TestGetVeleroDeleteBackupRequestByLabel: %v", err)
	}

	tests := []struct {
		name          string
		namespace     string
		labelValue    string
		expected      *velerov1.DeleteBackupRequest
		expectedError error
		mockRequests  []velerov1.DeleteBackupRequest
	}{
		{
			name:       "Single DeleteBackupRequest found",
			namespace:  defaultNS,
			labelValue: testAppStr,
			mockRequests: []velerov1.DeleteBackupRequest{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: defaultNS,
						Name:      "delete-request-1",
						Labels:    map[string]string{constant.NabOriginNACUUIDLabel: testAppStr},
					},
				},
			},
			expected:      &velerov1.DeleteBackupRequest{ObjectMeta: metav1.ObjectMeta{Namespace: defaultNS, Name: "delete-request-1", Labels: map[string]string{constant.NabOriginNACUUIDLabel: testAppStr}}},
			expectedError: nil,
		},
		{
			name:          "No DeleteBackupRequests found",
			namespace:     defaultNS,
			labelValue:    testAppStr,
			mockRequests:  []velerov1.DeleteBackupRequest{},
			expected:      nil,
			expectedError: nil,
		},
		{
			name:       "Multiple DeleteBackupRequests found",
			namespace:  defaultNS,
			labelValue: testAppStr,
			mockRequests: []velerov1.DeleteBackupRequest{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: defaultNS,
						Name:      "delete-request-2",
						Labels:    map[string]string{constant.NabOriginNACUUIDLabel: testAppStr},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: defaultNS,
						Name:      "delete-request-3",
						Labels:    map[string]string{constant.NabOriginNACUUIDLabel: testAppStr},
					},
				},
			},
			expected: &velerov1.DeleteBackupRequest{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: defaultNS,
					Name:      "delete-request-2",
					Labels:    map[string]string{constant.NabOriginNACUUIDLabel: testAppStr},
				},
			},
			expectedError: nil,
		},
		{
			name:          invalidInputEmptyNamespace,
			namespace:     constant.EmptyString,
			labelValue:    testAppStr,
			mockRequests:  []velerov1.DeleteBackupRequest{},
			expected:      nil,
			expectedError: errors.New("invalid input: namespace=\"\", labelKey=\"velero.io/backup-name\", labelValue=\"test-app\""),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var objects []client.Object
			for _, request := range tt.mockRequests {
				requestCopy := request // Create a copy to avoid memory aliasing
				objects = append(objects, &requestCopy)
			}
			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()

			result, err := GetVeleroDeleteBackupRequestByLabel(ctx, client, tt.namespace, tt.labelValue)

			if tt.expectedError != nil {
				assert.EqualError(t, err, tt.expectedError.Error())
			} else {
				assert.NoError(t, err)
				if tt.expected != nil && result != nil {
					assert.Equal(t, tt.expected.Name, result.Name, "DeleteBackupRequest Name should match")
					assert.Equal(t, tt.expected.Namespace, result.Namespace, "DeleteBackupRequest Namespace should match")
					assert.Equal(t, tt.expected.Labels, result.Labels, "DeleteBackupRequest Labels should match")
				} else {
					assert.Nil(t, result, "Expected result should be nil")
				}
			}
		})
	}
}

func TestGetActiveVeleroBackupsByLabel(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	ctx := context.Background()
	ctx = ctrl.LoggerInto(ctx, log)
	scheme := runtime.NewScheme()

	if err := velerov1.AddToScheme(scheme); err != nil {
		t.Fatalf("Failed to register VeleroBackup type in TestGetActiveVeleroBackupsByLabel: %v", err)
	}

	tests := []struct {
		name          string
		namespace     string
		labelKey      string
		labelValue    string
		mockBackups   []velerov1.Backup
		expectedCount int
	}{
		{
			name:       "No active backups",
			namespace:  defaultNS,
			labelKey:   constant.NabOriginNACUUIDLabel,
			labelValue: testNonAdminBackupUUID,
			mockBackups: []velerov1.Backup{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: defaultNS,
						Name:      testNonAdminBackupName,
						Labels:    map[string]string{constant.NabOriginNACUUIDLabel: testNonAdminBackupUUID},
					},
					Status: velerov1.BackupStatus{
						CompletionTimestamp: &metav1.Time{Time: time.Now()},
					},
				},
			},
			expectedCount: expectedIntZero,
		},
		{
			name:       "One active backup",
			namespace:  defaultNS,
			labelKey:   constant.NabOriginNACUUIDLabel,
			labelValue: testNonAdminBackupUUID,
			mockBackups: []velerov1.Backup{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: defaultNS,
						Name:      testNonAdminBackupName,
						Labels:    map[string]string{constant.NabOriginNACUUIDLabel: testNonAdminBackupUUID},
					},
				},
			},
			expectedCount: expectedIntOne,
		},
		{
			name:       "Multiple active backups",
			namespace:  defaultNS,
			labelKey:   constant.NabOriginNACUUIDLabel,
			labelValue: testNonAdminBackupUUID,
			mockBackups: []velerov1.Backup{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: defaultNS,
						Name:      testNonAdminBackupName,
						Labels:    map[string]string{constant.NabOriginNACUUIDLabel: testNonAdminBackupUUID},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: defaultNS,
						Name:      testNonAdminSecondBackupName,
						Labels:    map[string]string{constant.NabOriginNACUUIDLabel: testNonAdminBackupUUID},
					},
				},
			},
			expectedCount: expectedIntTwo,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var objects []client.Object
			for _, backup := range tt.mockBackups {
				backupCopy := backup // Create a copy to avoid memory aliasing
				objects = append(objects, &backupCopy)
			}
			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()

			result, err := GetActiveVeleroBackupsByLabel(ctx, client, tt.namespace, tt.labelKey, tt.labelValue)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedCount, len(result))
		})
	}
}

func TestGetBackupQueueInfo(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	ctx := context.Background()
	ctx = ctrl.LoggerInto(ctx, log)
	scheme := runtime.NewScheme()

	if err := velerov1.AddToScheme(scheme); err != nil {
		t.Fatalf("Failed to register VeleroBackup type in TestGetBackupQueueInfo: %v", err)
	}

	tests := []struct {
		name          string
		namespace     string
		targetBackup  *velerov1.Backup
		mockBackups   []velerov1.Backup
		expectedQueue int
	}{
		{
			name:      "No backups in queue",
			namespace: defaultNS,
			targetBackup: &velerov1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:         defaultNS,
					Name:              testNonAdminBackupName,
					CreationTimestamp: metav1.Time{Time: time.Now()},
				},
			},
			mockBackups:   []velerov1.Backup{},
			expectedQueue: expectedIntOne,
		},
		{
			name:      "One backup ahead in queue",
			namespace: defaultNS,
			targetBackup: &velerov1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:         defaultNS,
					Name:              testNonAdminSecondBackupName,
					CreationTimestamp: metav1.Time{Time: time.Now().Add(1 * time.Hour)},
				},
			},
			mockBackups: []velerov1.Backup{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         defaultNS,
						Name:              testNonAdminBackupName,
						CreationTimestamp: metav1.Time{Time: time.Now()},
					},
				},
			},
			expectedQueue: expectedIntTwo,
		},
		{
			name:      "Target backup already completed",
			namespace: defaultNS,
			targetBackup: &velerov1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:         defaultNS,
					Name:              testNonAdminBackupName,
					CreationTimestamp: metav1.Time{Time: time.Now()},
				},
				Status: velerov1.BackupStatus{
					CompletionTimestamp: &metav1.Time{Time: time.Now()},
				},
			},
			mockBackups:   []velerov1.Backup{},
			expectedQueue: expectedIntZero,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var objects []client.Object
			for _, backup := range tt.mockBackups {
				backupCopy := backup
				objects = append(objects, &backupCopy)
			}
			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()

			queueInfo, err := GetBackupQueueInfo(ctx, client, tt.namespace, tt.targetBackup)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedQueue, queueInfo.EstimatedQueuePosition)
		})
	}
}

func TestGetRestoreQueueInfo(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	ctx := context.Background()
	ctx = ctrl.LoggerInto(ctx, log)
	scheme := runtime.NewScheme()

	if err := velerov1.AddToScheme(scheme); err != nil {
		t.Fatalf("Failed to register VeleroRestore type in TestGetRestoreQueueInfo: %v", err)
	}

	tests := []struct {
		name          string
		namespace     string
		targetRestore *velerov1.Restore
		mockRestores  []velerov1.Restore
		expectedQueue int
	}{
		{
			name:      "No restores in queue",
			namespace: defaultNS,
			targetRestore: &velerov1.Restore{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:         defaultNS,
					Name:              testNonAdminRestoreName,
					CreationTimestamp: metav1.Time{Time: time.Now()},
				},
			},
			mockRestores:  []velerov1.Restore{},
			expectedQueue: expectedIntOne,
		},
		{
			name:      "One restore ahead in queue",
			namespace: defaultNS,
			targetRestore: &velerov1.Restore{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:         defaultNS,
					Name:              testNonAdminSecondRestoreName,
					CreationTimestamp: metav1.Time{Time: time.Now().Add(1 * time.Hour)},
				},
			},
			mockRestores: []velerov1.Restore{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         defaultNS,
						Name:              testNonAdminRestoreName,
						CreationTimestamp: metav1.Time{Time: time.Now()},
					},
				},
			},
			expectedQueue: expectedIntTwo,
		},
		{
			name:      "Target restore already completed",
			namespace: defaultNS,
			targetRestore: &velerov1.Restore{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:         defaultNS,
					Name:              testNonAdminRestoreName,
					CreationTimestamp: metav1.Time{Time: time.Now()},
				},
				Status: velerov1.RestoreStatus{
					CompletionTimestamp: &metav1.Time{Time: time.Now()},
				},
			},
			mockRestores:  []velerov1.Restore{},
			expectedQueue: expectedIntZero,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var objects []client.Object
			for _, restore := range tt.mockRestores {
				restoreCopy := restore
				objects = append(objects, &restoreCopy)
			}
			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()

			queueInfo, err := GetRestoreQueueInfo(ctx, client, tt.namespace, tt.targetRestore)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedQueue, queueInfo.EstimatedQueuePosition)
		})
	}
}
