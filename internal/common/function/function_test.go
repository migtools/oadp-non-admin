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
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/assert"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	nacv1alpha1 "github.com/migtools/oadp-non-admin/api/v1alpha1"
	"github.com/migtools/oadp-non-admin/internal/common/constant"
)

var _ = ginkgo.Describe("PLACEHOLDER", func() {})

const (
	testNonAdminBackupNamespace = "non-admin-backup-namespace"
	testNonAdminBackupName      = "non-admin-backup-name"
	testNonAdminBackupUUID      = "12345678-1234-1234-1234-123456789abc"
	defaultStr                  = "default"
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
			errMessage: "spec.backupSpec.IncludedNamespaces can not contain namespaces other than: non-admin-backup-namespace",
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
			err := ValidateBackupSpec(nonAdminBackup)
			if len(test.errMessage) == 0 {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Equal(t, test.errMessage, err.Error())
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

	// Register VeleroBackup type with the scheme
	if err := velerov1.AddToScheme(scheme); err != nil {
		t.Fatalf("Failed to register VeleroBackup type: %v", err)
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
