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
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/assert"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
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
)

func TestMergeMaps(t *testing.T) {
	const (
		d     = "d"
		delta = "delta"
	)
	tests := []struct {
		name    string
		want    map[string]string
		args    []map[string]string
		wantErr bool
	}{
		{
			name: "append unique labels together",
			args: []map[string]string{
				{"a": "alpha"},
				{"b": "beta"},
			},
			want: map[string]string{
				"a": "alpha",
				"b": "beta",
			},
		},
		{
			name: "append unique labels together, with valid duplicates",
			args: []map[string]string{
				{"c": "gamma"},
				{d: delta},
				{d: delta},
			},
			want: map[string]string{
				"c": "gamma",
				d:   delta,
			},
		},
		{
			name: "append unique labels together - nil sandwich",
			args: []map[string]string{
				{"x": "chi"},
				nil,
				{"y": "psi"},
			},
			want: map[string]string{
				"x": "chi",
				"y": "psi",
			},
		},
		{
			name: "should error when append duplicate label keys with different value together",
			args: []map[string]string{
				{"key": "value-1"},
				{"key": "value-2"},
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := mergeMaps(tt.args...)
			if (err != nil) != tt.wantErr {
				t.Errorf("mergeMaps() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("mergeMaps() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAddNonAdminLabels(t *testing.T) {
	// Additional labels provided
	additionalLabels := map[string]string{
		"additionalLabel1": "value1",
		"additionalLabel2": "value2",
	}

	expectedLabels := map[string]string{
		constant.OadpLabel:      "True",
		constant.ManagedByLabel: constant.ManagedByLabelValue,
		"additionalLabel1":      "value1",
		"additionalLabel2":      "value2",
	}

	mergedLabels := AddNonAdminLabels(additionalLabels)
	assert.Equal(t, expectedLabels, mergedLabels, "Merged labels should match expected labels")
}

func TestAddNonAdminBackupAnnotations(t *testing.T) {
	// Merging annotations without conflicts
	existingAnnotations := map[string]string{
		"existingKey1": "existingValue1",
		"existingKey2": "existingValue2",
	}

	ownerName := "testOwner"
	ownerNamespace := "testNamespace"
	ownerUUID := "f2c4d2c3-58d3-46ec-bf03-5940f567f7f8"

	expectedAnnotations := map[string]string{
		constant.NabOriginNamespaceAnnotation: ownerNamespace,
		constant.NabOriginNameAnnotation:      ownerName,
		constant.NabOriginUUIDAnnotation:      ownerUUID,
		"existingKey1":                        "existingValue1",
		"existingKey2":                        "existingValue2",
	}

	mergedAnnotations := AddNonAdminBackupAnnotations(ownerNamespace, ownerName, ownerUUID, existingAnnotations)
	assert.Equal(t, expectedAnnotations, mergedAnnotations, "Merged annotations should match expected annotations")

	// Merging annotations with conflicts
	existingAnnotationsWithConflict := map[string]string{
		constant.NabOriginNameAnnotation: "conflictingValue",
	}

	expectedAnnotationsWithConflict := map[string]string{
		constant.NabOriginNameAnnotation:      ownerName,
		constant.NabOriginNamespaceAnnotation: ownerNamespace,
		constant.NabOriginUUIDAnnotation:      ownerUUID,
	}

	mergedAnnotationsWithConflict := AddNonAdminBackupAnnotations(ownerNamespace, ownerName, ownerUUID, existingAnnotationsWithConflict)
	assert.Equal(t, expectedAnnotationsWithConflict, mergedAnnotationsWithConflict, "Merged annotations should match expected annotations with conflict")
}

func TestGetBackupSpecFromNonAdminBackup(t *testing.T) {
	// Test case: BackupSpec is nil
	nonAdminBackup := &nacv1alpha1.NonAdminBackup{
		Spec: nacv1alpha1.NonAdminBackupSpec{
			BackupSpec: nil,
		},
	}
	backupSpec, err := GetBackupSpecFromNonAdminBackup(nonAdminBackup)
	assert.Error(t, err)
	assert.Nil(t, backupSpec)
	assert.Equal(t, "BackupSpec is not defined", err.Error())

	// Test case: NonAdminBackup with valid BackupSpec
	backupSpecInput := &velerov1.BackupSpec{
		IncludedNamespaces:      []string{"namespace1"},
		StorageLocation:         "s3://bucket-name/path/to/backup",
		VolumeSnapshotLocations: []string{"volume-snapshot-location"},
	}

	nonAdminBackup = &nacv1alpha1.NonAdminBackup{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "namespace1", // Set the namespace of NonAdminBackup
		},
		Spec: nacv1alpha1.NonAdminBackupSpec{
			BackupSpec: backupSpecInput,
		},
	}
	backupSpec, err = GetBackupSpecFromNonAdminBackup(nonAdminBackup)

	assert.NoError(t, err)
	assert.NotNil(t, backupSpec)
	assert.Equal(t, []string{"namespace1"}, backupSpec.IncludedNamespaces) // Check that only the namespace from NonAdminBackup is included
	assert.Equal(t, backupSpecInput.ExcludedNamespaces, backupSpec.ExcludedNamespaces)
	assert.Equal(t, backupSpecInput.StorageLocation, backupSpec.StorageLocation)
	assert.Equal(t, backupSpecInput.VolumeSnapshotLocations, backupSpec.VolumeSnapshotLocations)

	backupSpecInput = &velerov1.BackupSpec{
		IncludedNamespaces: []string{"namespace2", "namespace3"},
	}

	nonAdminBackup = &nacv1alpha1.NonAdminBackup{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "namespace2", // Set the namespace of NonAdminBackup
		},
		Spec: nacv1alpha1.NonAdminBackupSpec{
			BackupSpec: backupSpecInput,
		},
	}
	backupSpec, err = GetBackupSpecFromNonAdminBackup(nonAdminBackup)

	assert.Error(t, err)
	assert.Nil(t, backupSpec)
	assert.Equal(t, "spec.backupSpec.IncludedNamespaces can not contain namespaces other than: namespace2", err.Error())

	backupSpecInput = &velerov1.BackupSpec{
		IncludedNamespaces: []string{"namespace3"},
	}

	nonAdminBackup = &nacv1alpha1.NonAdminBackup{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "namespace4", // Set the namespace of NonAdminBackup
		},
		Spec: nacv1alpha1.NonAdminBackupSpec{
			BackupSpec: backupSpecInput,
		},
	}
	backupSpec, err = GetBackupSpecFromNonAdminBackup(nonAdminBackup)

	assert.Error(t, err)
	assert.Nil(t, backupSpec)
	assert.Equal(t, "spec.backupSpec.IncludedNamespaces can not contain namespaces other than: namespace4", err.Error())
}

func TestGenerateVeleroBackupName(t *testing.T) {
	longString := ""
	const longStringSize = 240
	for i := 0; i < longStringSize; i++ {
		longString += "m"
	}

	truncatedString := ""
	// constant.MaxKubernetesNameLength = 253
	// deduct -3: constant.VeleroBackupNamePrefix = "nab"
	// deduct -2: there are two additional separators "-" in the name
	// 253 - len(nameHash) - 3 - 2
	// 253 - 14 - 3 - 2 = 234
	const truncatedStringSize = 234
	for i := 0; i < truncatedStringSize; i++ {
		truncatedString += "m"
	}

	testCases := []struct {
		namespace string
		name      string
		expected  string
	}{
		{
			namespace: "example-namespace",
			name:      "example-name",
			expected:  "nab-example-namespace-1cdadb729eaac1",
		},
		{
			namespace: longString,
			name:      "example-name",
			expected:  fmt.Sprintf("nab-%s-1cdadb729eaac1", truncatedString),
		},
	}

	for _, tc := range testCases {
		actual := GenerateVeleroBackupName(tc.namespace, tc.name)
		if actual != tc.expected {
			t.Errorf("Expected: %s, but got: %s", tc.expected, actual)
		}
	}
}

func TestGenerateVeleroBackupNameWithUUID(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		nabName   string
	}{
		{
			name:      "Valid names without truncation",
			namespace: "default",
			nabName:   "my-backup",
		},
		{
			name:      "Truncate nabName due to length",
			namespace: "some",
			nabName:   strings.Repeat("q", validation.DNS1123SubdomainMaxLength+10), // too long for DNS limit
		},
		{
			name:      "Truncate very long namespace and very long name",
			namespace: strings.Repeat("w", validation.DNS1123SubdomainMaxLength+10),
			nabName:   strings.Repeat("e", validation.DNS1123SubdomainMaxLength+10),
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
			nabName:   strings.Repeat("r", validation.DNS1123SubdomainMaxLength+10),
		},
		{
			name:      "very long namespace and name empty",
			namespace: strings.Repeat("t", validation.DNS1123SubdomainMaxLength+10),
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
			result := GenerateVeleroBackupNameWithUUID(tt.namespace, tt.nabName)

			// Check length
			if len(result) > validation.DNS1123SubdomainMaxLength {
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

func TestGetNonAdminBackupFromVeleroBackup(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	ctx := context.Background()
	ctx = ctrl.LoggerInto(ctx, log)

	// Register NonAdminBackup type with the scheme
	if err := nacv1alpha1.AddToScheme(clientgoscheme.Scheme); err != nil {
		t.Fatalf("Failed to register NonAdminBackup type: %v", err)
	}

	backup := &velerov1.Backup{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-namespace",
			Name:      "test-backup",
			Annotations: map[string]string{
				constant.NabOriginNamespaceAnnotation: testNonAdminBackupNamespace,
				constant.NabOriginNameAnnotation:      testNonAdminBackupName,
				constant.NabOriginUUIDAnnotation:      testNonAdminBackupUUID,
			},
		},
	}

	nonAdminBackup := &nacv1alpha1.NonAdminBackup{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testNonAdminBackupNamespace,
			Name:      testNonAdminBackupName,
			UID:       types.UID(testNonAdminBackupUUID),
		},
	}

	client := fake.NewClientBuilder().WithObjects(nonAdminBackup).Build()

	result, err := GetNonAdminBackupFromVeleroBackup(ctx, client, backup)
	assert.NoError(t, err, "GetNonAdminFromBackup should not return an error")
	assert.NotNil(t, result, "Returned NonAdminBackup should not be nil")
	assert.Equal(t, nonAdminBackup, result, "Returned NonAdminBackup should match expected NonAdminBackup")
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
						constant.NabOriginUUIDAnnotation:      testNonAdminBackupUUID,
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
						constant.OadpLabel:      constant.OadpLabelValue,
						constant.ManagedByLabel: constant.ManagedByLabelValue,
					},
					Annotations: map[string]string{
						constant.NabOriginNamespaceAnnotation: constant.EmptyString,
						constant.NabOriginNameAnnotation:      testNonAdminBackupName,
						constant.NabOriginUUIDAnnotation:      testNonAdminBackupUUID,
					},
				},
			},
			expected: false,
		},
		{
			name: "Velero Backup with wrong required non admin annotation [long]",
			backup: &velerov1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						constant.OadpLabel:      constant.OadpLabelValue,
						constant.ManagedByLabel: constant.ManagedByLabelValue,
					},
					Annotations: map[string]string{
						constant.NabOriginNamespaceAnnotation: testNonAdminBackupNamespace,
						constant.NabOriginNameAnnotation:      strings.Repeat("nn", validation.DNS1123SubdomainMaxLength),
						constant.NabOriginUUIDAnnotation:      testNonAdminBackupUUID,
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
						constant.OadpLabel:      constant.OadpLabelValue,
						constant.ManagedByLabel: constant.ManagedByLabelValue,
					},
					Annotations: map[string]string{
						constant.NabOriginNamespaceAnnotation: testNonAdminBackupNamespace,
						constant.NabOriginNameAnnotation:      testNonAdminBackupName,
						constant.NabOriginUUIDAnnotation:      testNonAdminBackupUUID,
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
