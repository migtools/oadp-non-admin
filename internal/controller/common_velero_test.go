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

	"github.com/stretchr/testify/assert"
	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	nacv1alpha1 "github.com/migtools/oadp-non-admin/api/v1alpha1"
)

func TestGetNonAdminFromBackup(t *testing.T) {
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
				NabOriginNamespaceAnnotation: "non-admin-backup-namespace",
				NabOriginNameAnnotation:      "non-admin-backup-name",
				NabOriginUUIDAnnotation:      "12345678-1234-1234-1234-123456789abc",
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

	result, err := GetNonAdminFromBackup(ctx, client, backup)
	assert.NoError(t, err, "GetNonAdminFromBackup should not return an error")
	assert.NotNil(t, result, "Returned NonAdminBackup should not be nil")
	assert.Equal(t, nonAdminBackup, result, "Returned NonAdminBackup should match expected NonAdminBackup")
}

func TestHasRequiredLabel(t *testing.T) {
	// Backup has the required label
	backupWithLabel := &velerov1api.Backup{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				ManagedByLabel: ManagedByLabelValue,
			},
		},
	}
	assert.True(t, HasRequiredLabel(backupWithLabel), "Expected backup to have required label")

	// Backup does not have the required label
	backupWithoutLabel := &velerov1api.Backup{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{},
		},
	}
	assert.False(t, HasRequiredLabel(backupWithoutLabel), "Expected backup to not have required label")

	// Backup has the required label with incorrect value
	backupWithIncorrectValue := &velerov1api.Backup{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				ManagedByLabel: "incorrect-value",
			},
		},
	}
	assert.False(t, HasRequiredLabel(backupWithIncorrectValue), "Expected backup to not have required label")
}
