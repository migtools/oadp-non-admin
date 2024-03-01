package controller

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"

	nacv1alpha1 "github.com/migtools/oadp-non-admin/api/v1alpha1"
)

func TestGetVeleroBackupSpecFromNonAdminBackup(t *testing.T) {
	// Test case: nonAdminBackup is nil
	nonAdminBackup := (*nacv1alpha1.NonAdminBackup)(nil)
	backupSpec, err := GetVeleroBackupSpecFromNonAdminBackup(nonAdminBackup)
	assert.Error(t, err)
	assert.Nil(t, backupSpec)
	assert.Equal(t, "nonAdminBackup is nil", err.Error())

	// Test case: BackupSpec is nil
	nonAdminBackup = &nacv1alpha1.NonAdminBackup{
		Spec: nacv1alpha1.NonAdminBackupSpec{
			BackupSpec: nil,
		},
	}
	backupSpec, err = GetVeleroBackupSpecFromNonAdminBackup(nonAdminBackup)
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
	backupSpec, err = GetVeleroBackupSpecFromNonAdminBackup(nonAdminBackup)

	assert.NoError(t, err)
	assert.NotNil(t, backupSpec)
	assert.Equal(t, backupSpecInput, backupSpec)
}

func TestGenerateVeleroBackupName(t *testing.T) {
	longString := ""
	for i := 0; i < 240; i++ {
		longString += "a"
	}

	truncatedString := ""
	// 253 - len(nameHash) - 4
	// 253 - 14 - 4 = 235
	for i := 0; i < 235; i++ {
		truncatedString += "a"
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
