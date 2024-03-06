package controller

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	nacv1alpha1 "github.com/migtools/oadp-non-admin/api/v1alpha1"
)

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

func GenerateVeleroBackupName(namespace, nabName string) string {
	// Calculate a hash of the name
	hasher := sha1.New()
	hasher.Write([]byte(nabName))
	nameHash := hex.EncodeToString(hasher.Sum(nil))[:14] // Take first 14 chars

	// Generate the Velero backup name created from NAB
	veleroBackupName := fmt.Sprintf("nab-%s-%s", namespace, nameHash)

	// Ensure the name is within the character limit
	if len(veleroBackupName) > 253 {
		// Truncate the namespace if necessary
		maxNamespaceLength := 253 - len(nameHash) - 4 // Account for "-nab-" and "-"
		if len(namespace) > maxNamespaceLength {
			namespace = namespace[:maxNamespaceLength]
		}
		veleroBackupName = fmt.Sprintf("nab-%s-%s", namespace, nameHash)
	}

	return veleroBackupName
}

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
