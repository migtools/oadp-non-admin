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
	"fmt"
	"strings"
	"testing"

	"github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/assert"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"

	nacv1alpha1 "github.com/migtools/oadp-non-admin/api/v1alpha1"
	"github.com/migtools/oadp-non-admin/internal/common/constant"
)

var _ = ginkgo.Describe("PLACEHOLDER", func() {})

const (
	testNonAdminBackupNamespace = "non-admin-backup-namespace"
	testNonAdminBackupName      = "non-admin-backup-name"
	testNonAdminBackupUUID      = "12345678-1234-1234-1234-123456789abc"
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
		constant.NabOriginUUIDAnnotation:      testNonAdminBackupUUID,
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

func TestGenerateVeleroBackupName(t *testing.T) {
	// Max Kubernetes name length := 253
	// hashLength := 14
	// deduct -3: constant.VeleroBackupNamePrefix = "nab"
	// deduct -2: two "-" in the name
	// 253 - 14 - 3 - 2 = 234
	const truncatedStringSize = 234

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
			namespace: strings.Repeat("m", validation.DNS1123SubdomainMaxLength),
			name:      "example-name",
			expected:  fmt.Sprintf("nab-%s-1cdadb729eaac1", strings.Repeat("m", truncatedStringSize)),
		},
	}

	for _, tc := range testCases {
		actual := GenerateVeleroBackupName(tc.namespace, tc.name)
		if actual != tc.expected {
			t.Errorf("Expected: %s, but got: %s", tc.expected, actual)
		}
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
