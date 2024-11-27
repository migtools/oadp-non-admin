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
	testNonAdminBackupNamespace  = "non-admin-backup-namespace"
	testNonAdminBackupName       = "non-admin-backup-name"
	testNonAdminSecondBackupName = "non-admin-second-backup-name"
	testNonAdminBackupUUID       = "12345678-1234-1234-1234-123456789abc"
	defaultStr                   = "default"
	expectedIntZero              = 0
	expectedIntOne               = 1
	expectedIntTwo               = 2
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
			name:       "nil spec",
			spec:       nil,
			errMessage: "BackupSpec is not defined",
		},
		{
			name: "namespace different than NonAdminBackup namespace",
			spec: &velerov1.BackupSpec{
				IncludedNamespaces: []string{"namespace1", "namespace2", "namespace3"},
			},
			errMessage: "NonAdminBackup spec.backupSpec.includedNamespaces can not contain namespaces other than: non-admin-backup-namespace",
		},
		{
			name: "valid spec",
			spec: &velerov1.BackupSpec{
				IncludedNamespaces: []string{testNonAdminBackupNamespace},
			},
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
			err := ValidateBackupSpec(nonAdminBackup, &velerov1.BackupSpec{})
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
		enforcedValue any
		overrideValue any
		name          string
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
			name:          "ExcludedNamespaces",
			enforcedValue: []string{"openshift-adp"},
			overrideValue: []string{"cherry"},
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
			name:          "IncludedClusterScopedResources",
			enforcedValue: []string{},
			overrideValue: []string{all},
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
			name:          "IncludeClusterResources",
			enforcedValue: ptr.To(true),
			overrideValue: ptr.To(false),
		},
		{
			name: "Hooks",
			enforcedValue: velerov1.BackupHooks{
				Resources: []velerov1.BackupResourceHookSpec{
					{
						Name: "test",
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
			enforcedValue: "default",
			overrideValue: "lemon",
		},
		{
			name:          "VolumeSnapshotLocations",
			enforcedValue: []string{"aws"},
			overrideValue: []string{"gcp"},
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
				Kind: "test",
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
			err := ValidateBackupSpec(userNonAdminBackup, enforcedSpec)
			if err != nil {
				t.Errorf("not setting backup spec field '%v' test failed: %v", test.name, err)
			}

			reflect.ValueOf(userNonAdminBackup.Spec.BackupSpec).Elem().FieldByName(test.name).Set(reflect.ValueOf(test.enforcedValue))
			err = ValidateBackupSpec(userNonAdminBackup, enforcedSpec)
			if err != nil {
				t.Errorf("setting backup spec field '%v' with value respecting enforcement test failed: %v", test.name, err)
			}

			reflect.ValueOf(userNonAdminBackup.Spec.BackupSpec).Elem().FieldByName(test.name).Set(reflect.ValueOf(test.overrideValue))
			err = ValidateBackupSpec(userNonAdminBackup, enforcedSpec)
			if err == nil {
				t.Errorf("setting backup spec field '%v' with value overriding enforcement test failed: %v", test.name, err)
			}
		})
	}

	t.Run("Ensure all backup spec fields were tested", func(t *testing.T) {
		backupSpecFields := []string{}
		for _, test := range tests {
			backupSpecFields = append(backupSpecFields, test.name)
		}
		backupSpec := reflect.ValueOf(&velerov1.BackupSpec{}).Elem()

		for index := 0; index < backupSpec.NumField(); index++ {
			if !slices.Contains(backupSpecFields, backupSpec.Type().Field(index).Name) {
				t.Errorf("backup spec field '%v' is not tested", backupSpec.Type().Field(index).Name)
			}
		}
		if backupSpec.NumField() != len(tests) {
			t.Errorf("list of tests have different number of elements")
		}
	})
}

func TestGenerateNacObjectNameWithUUID(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		nabName   string
	}{
		{
			name:      "Valid names without truncation",
			namespace: defaultStr,
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
			namespace:  defaultStr,
			labelValue: testAppStr,
			mockBackups: []velerov1.Backup{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: defaultStr,
						Name:      "backup1",
						Labels:    map[string]string{constant.NabOriginNACUUIDLabel: testAppStr},
					},
				},
			},
			expected:      &velerov1.Backup{ObjectMeta: metav1.ObjectMeta{Namespace: defaultStr, Name: "backup1", Labels: map[string]string{constant.NabOriginNACUUIDLabel: testAppStr}}},
			expectedError: nil,
		},
		{
			name:          "No VeleroBackups found",
			namespace:     defaultStr,
			labelValue:    testAppStr,
			mockBackups:   []velerov1.Backup{},
			expected:      nil,
			expectedError: nil,
		},
		{
			name:       "Multiple VeleroBackups found",
			namespace:  defaultStr,
			labelValue: testAppStr,
			mockBackups: []velerov1.Backup{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: defaultStr,
						Name:      "backup2",
						Labels:    map[string]string{constant.NabOriginNACUUIDLabel: testAppStr},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: defaultStr,
						Name:      "backup3",
						Labels:    map[string]string{constant.NabOriginNACUUIDLabel: testAppStr},
					},
				},
			},
			expected:      nil,
			expectedError: errors.New("multiple VeleroBackup objects found with label openshift.io/oadp-nab-origin-nacuuid=test-app in namespace 'default'"),
		},
		{
			name:          "Invalid input - empty namespace",
			namespace:     "",
			labelValue:    testAppStr,
			mockBackups:   []velerov1.Backup{},
			expected:      nil,
			expectedError: errors.New("invalid input: namespace, labelKey, and labelValue must not be empty"),
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
			namespace:  defaultStr,
			labelValue: testAppStr,
			mockRequests: []velerov1.DeleteBackupRequest{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: defaultStr,
						Name:      "delete-request-1",
						Labels:    map[string]string{constant.NabOriginNACUUIDLabel: testAppStr},
					},
				},
			},
			expected:      &velerov1.DeleteBackupRequest{ObjectMeta: metav1.ObjectMeta{Namespace: defaultStr, Name: "delete-request-1", Labels: map[string]string{constant.NabOriginNACUUIDLabel: testAppStr}}},
			expectedError: nil,
		},
		{
			name:          "No DeleteBackupRequests found",
			namespace:     defaultStr,
			labelValue:    testAppStr,
			mockRequests:  []velerov1.DeleteBackupRequest{},
			expected:      nil,
			expectedError: nil,
		},
		{
			name:       "Multiple DeleteBackupRequests found",
			namespace:  defaultStr,
			labelValue: testAppStr,
			mockRequests: []velerov1.DeleteBackupRequest{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: defaultStr,
						Name:      "delete-request-2",
						Labels:    map[string]string{constant.NabOriginNACUUIDLabel: testAppStr},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: defaultStr,
						Name:      "delete-request-3",
						Labels:    map[string]string{constant.NabOriginNACUUIDLabel: testAppStr},
					},
				},
			},
			expected: &velerov1.DeleteBackupRequest{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: defaultStr,
					Name:      "delete-request-2",
					Labels:    map[string]string{constant.NabOriginNACUUIDLabel: testAppStr},
				},
			},
			expectedError: nil,
		},
		{
			name:          "Invalid input - empty namespace",
			namespace:     "",
			labelValue:    testAppStr,
			mockRequests:  []velerov1.DeleteBackupRequest{},
			expected:      nil,
			expectedError: errors.New("invalid input: namespace, labelKey, and labelValue must not be empty"),
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
			namespace:  defaultStr,
			labelKey:   constant.NabOriginNACUUIDLabel,
			labelValue: testNonAdminBackupUUID,
			mockBackups: []velerov1.Backup{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: defaultStr,
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
			namespace:  defaultStr,
			labelKey:   constant.NabOriginNACUUIDLabel,
			labelValue: testNonAdminBackupUUID,
			mockBackups: []velerov1.Backup{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: defaultStr,
						Name:      testNonAdminBackupName,
						Labels:    map[string]string{constant.NabOriginNACUUIDLabel: testNonAdminBackupUUID},
					},
				},
			},
			expectedCount: expectedIntOne,
		},
		{
			name:       "Multiple active backups",
			namespace:  defaultStr,
			labelKey:   constant.NabOriginNACUUIDLabel,
			labelValue: testNonAdminBackupUUID,
			mockBackups: []velerov1.Backup{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: defaultStr,
						Name:      testNonAdminBackupName,
						Labels:    map[string]string{constant.NabOriginNACUUIDLabel: testNonAdminBackupUUID},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: defaultStr,
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
			namespace: defaultStr,
			targetBackup: &velerov1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:         defaultStr,
					Name:              testNonAdminBackupName,
					CreationTimestamp: metav1.Time{Time: time.Now()},
				},
			},
			mockBackups:   []velerov1.Backup{},
			expectedQueue: expectedIntOne,
		},
		{
			name:      "One backup ahead in queue",
			namespace: defaultStr,
			targetBackup: &velerov1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:         defaultStr,
					Name:              testNonAdminSecondBackupName,
					CreationTimestamp: metav1.Time{Time: time.Now().Add(1 * time.Hour)},
				},
			},
			mockBackups: []velerov1.Backup{
				{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:         defaultStr,
						Name:              testNonAdminBackupName,
						CreationTimestamp: metav1.Time{Time: time.Now()},
					},
				},
			},
			expectedQueue: expectedIntTwo,
		},
		{
			name:      "Target backup already completed",
			namespace: defaultStr,
			targetBackup: &velerov1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:         defaultStr,
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
