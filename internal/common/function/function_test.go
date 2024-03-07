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
	"testing"

	"github.com/stretchr/testify/assert"
	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	nacv1alpha1 "github.com/migtools/oadp-non-admin/api/v1alpha1"
	"github.com/migtools/oadp-non-admin/internal/common/constant"
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
	// Test case: nonAdminBackup is nil
	nonAdminBackup := (*nacv1alpha1.NonAdminBackup)(nil)
	backupSpec, err := GetBackupSpecFromNonAdminBackup(nonAdminBackup)
	assert.Error(t, err)
	assert.Nil(t, backupSpec)
	assert.Equal(t, "nonAdminBackup is nil", err.Error())

	// Test case: BackupSpec is nil
	nonAdminBackup = &nacv1alpha1.NonAdminBackup{
		Spec: nacv1alpha1.NonAdminBackupSpec{
			BackupSpec: nil,
		},
	}
	backupSpec, err = GetBackupSpecFromNonAdminBackup(nonAdminBackup)
	assert.Error(t, err)
	assert.Nil(t, backupSpec)
	assert.Equal(t, "BackupSpec is nil", err.Error())

	// Test case: NonAdminBackup with valid BackupSpec
	backupSpecInput := &velerov1api.BackupSpec{
		IncludedNamespaces:      []string{"namespace1", "namespace2"},
		ExcludedNamespaces:      []string{"namespace3"},
		StorageLocation:         "s3://bucket-name/path/to/backup",
		VolumeSnapshotLocations: []string{"volume-snapshot-location"},
	}

	nonAdminBackup = &nacv1alpha1.NonAdminBackup{
		Spec: nacv1alpha1.NonAdminBackupSpec{
			BackupSpec: backupSpecInput,
		},
	}
	backupSpec, err = GetBackupSpecFromNonAdminBackup(nonAdminBackup)

	assert.NoError(t, err)
	assert.NotNil(t, backupSpec)
	assert.Equal(t, backupSpecInput, backupSpec)
}

func TestGenerateVeleroBackupName(t *testing.T) {
	longString := ""
	const longStringSize = 240
	for i := 0; i < longStringSize; i++ {
		longString += "m"
	}

	truncatedString := ""
	// 253 - len(nameHash) - 4
	// 253 - 14 - 4 = 235
	const truncatedStringSize = 235
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
			expected:  "nab-example-namespace-daf3757ac468f9",
		},
		{
			namespace: longString,
			name:      "example-name",
			expected:  fmt.Sprintf("nab-%s-daf3757ac468f9", truncatedString),
		},
	}

	for _, tc := range testCases {
		actual := GenerateVeleroBackupName(tc.namespace, tc.name)
		if actual != tc.expected {
			t.Errorf("Expected: %s, but got: %s", tc.expected, actual)
		}
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

	backup := &velerov1api.Backup{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-namespace",
			Name:      "test-backup",
			Annotations: map[string]string{
				constant.NabOriginNamespaceAnnotation: "non-admin-backup-namespace",
				constant.NabOriginNameAnnotation:      "non-admin-backup-name",
				constant.NabOriginUUIDAnnotation:      "12345678-1234-1234-1234-123456789abc",
			},
		},
	}

	nonAdminBackup := &nacv1alpha1.NonAdminBackup{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "non-admin-backup-namespace",
			Name:      "non-admin-backup-name",
			UID:       types.UID("12345678-1234-1234-1234-123456789abc"),
		},
	}

	client := fake.NewClientBuilder().WithObjects(nonAdminBackup).Build()

	result, err := GetNonAdminBackupFromVeleroBackup(ctx, client, backup)
	assert.NoError(t, err, "GetNonAdminFromBackup should not return an error")
	assert.NotNil(t, result, "Returned NonAdminBackup should not be nil")
	assert.Equal(t, nonAdminBackup, result, "Returned NonAdminBackup should match expected NonAdminBackup")
}

func TestCheckVeleroBackupLabels(t *testing.T) {
	// Backup has the required label
	backupWithLabel := &velerov1api.Backup{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				constant.ManagedByLabel: constant.ManagedByLabelValue,
			},
		},
	}
	assert.True(t, CheckVeleroBackupLabels(backupWithLabel), "Expected backup to have required label")

	// Backup does not have the required label
	backupWithoutLabel := &velerov1api.Backup{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{},
		},
	}
	assert.False(t, CheckVeleroBackupLabels(backupWithoutLabel), "Expected backup to not have required label")

	// Backup has the required label with incorrect value
	backupWithIncorrectValue := &velerov1api.Backup{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				constant.ManagedByLabel: "incorrect-value",
			},
		},
	}
	assert.False(t, CheckVeleroBackupLabels(backupWithIncorrectValue), "Expected backup to not have required label")
}
