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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	nacv1alpha1 "github.com/migtools/oadp-non-admin/api/v1alpha1"
	"github.com/migtools/oadp-non-admin/internal/common/constant"
)

// GetVeleroBackupSpecFromNonAdminBackup gets the BackupSpec from the NonAdminBackup object
func GetVeleroBackupSpecFromNonAdminBackup(nonAdminBackup *nacv1alpha1.NonAdminBackup) (*velerov1api.BackupSpec, error) {
	if nonAdminBackup == nil {
		return nil, fmt.Errorf("nonAdminBackup is nil")
	}

	if nonAdminBackup.Spec.BackupSpec == nil {
		return nil, fmt.Errorf("BackupSpec is nil")
	}

	// TODO: Additional validations, before continuing
	veleroBackup := nonAdminBackup.Spec.BackupSpec.DeepCopy()

	return veleroBackup, nil
}

// GenerateVeleroBackupName generates a Velero backup name based on the provided namespace and NonAdminBackup name.
// It calculates a hash of the NonAdminBackup name and combines it with the namespace and a prefix to create the Velero backup name.
// If the resulting name exceeds the maximum Kubernetes name length, it truncates the namespace to fit within the limit.
func GenerateVeleroBackupName(namespace, nabName string) string {
	// Calculate a hash of the name
	const hashLength = 14
	prefixLength := len(constant.VeleroBackupNamePrefix) + len("--") // Account for two "-"

	hasher := sha256.New()
	_, err := hasher.Write([]byte(nabName))
	if err != nil {
		return ""
	}

	nameHash := hex.EncodeToString(hasher.Sum(nil))[:hashLength] // Take first 14 chars

	// Generate the Velero backup name created from NAB
	veleroBackupName := fmt.Sprintf("%s-%s-%s", constant.VeleroBackupNamePrefix, namespace, nameHash)

	// Ensure the name is within the character limit
	if len(veleroBackupName) > constant.MaxKubernetesNameLength {
		// Truncate the namespace if necessary
		maxNamespaceLength := constant.MaxKubernetesNameLength - len(nameHash) - prefixLength
		if len(namespace) > maxNamespaceLength {
			namespace = namespace[:maxNamespaceLength]
		}
		veleroBackupName = fmt.Sprintf("%s-%s-%s", constant.VeleroBackupNamePrefix, namespace, nameHash)
	}

	return veleroBackupName
}

// UpdateNonAdminBackupFromVeleroBackup takes the Velero Backup and Velero Status and updates the corresponding
// NonAdminBackup with exact same copies of that information.
func UpdateNonAdminBackupFromVeleroBackup(ctx context.Context, r client.Client, log logr.Logger, nab *nacv1alpha1.NonAdminBackup, veleroBackup *velerov1api.Backup) error {
	// Make a copy of the current status for comparison
	oldStatus := nab.Status.BackupStatus.DeepCopy()
	oldSpec := nab.Spec.BackupSpec.DeepCopy()

	// Update the status & spec
	// Copy the status from veleroBackup.Status to nab.Status
	nab.Status.BackupStatus = veleroBackup.Status.DeepCopy()

	nab.Spec.BackupSpec = veleroBackup.Spec.DeepCopy()

	// Check if the spec has been updated
	if !reflect.DeepEqual(oldSpec, nab.Spec.BackupSpec) {
		if err := r.Update(ctx, nab); err != nil {
			log.Error(err, "Failed to update NonAdminBackup Spec")
			return err
		}
		log.V(1).Info("NonAdminBackup spec was updated")
	} else {
		log.V(1).Info("NonAdminBackup spec is already up to date")
	}

	// Check if the status has been updated
	if !reflect.DeepEqual(oldStatus, nab.Status.BackupStatus) {
		if err := r.Status().Update(ctx, nab); err != nil {
			log.Error(err, "Failed to update NonAdminBackup Status")
			return err
		}
		log.V(1).Info("NonAdminBackup status was updated")
	} else {
		log.V(1).Info("NonAdminBackup status is already up to date")
	}

	return nil
}
