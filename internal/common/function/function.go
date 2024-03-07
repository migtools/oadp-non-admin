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

// Package function contains all common functions used in the project
package function

import (
	"context"
	"crypto/sha1" //nolint:gosec // TODO remove
	"encoding/hex"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	nacv1alpha1 "github.com/migtools/oadp-non-admin/api/v1alpha1"
	"github.com/migtools/oadp-non-admin/internal/common/constant"
)

const requiredAnnotationError = "backup does not have the required annotation '%s'"

// AddNonAdminLabels return a map with both the object labels and with the default Non Admin labels.
// If error occurs, a map with only the default Non Admin labels is returned
func AddNonAdminLabels(labels map[string]string) map[string]string {
	defaultLabels := map[string]string{
		constant.OadpLabel:      "True",
		constant.ManagedByLabel: constant.ManagedByLabelValue,
	}

	mergedLabels, err := mergeUniqueKeyTOfTMaps(defaultLabels, labels)
	if err != nil {
		// TODO logger
		_, _ = fmt.Println("Error merging labels:", err)
		// TODO break?
		return defaultLabels
	}
	return mergedLabels
}

// AddNonAdminBackupAnnotations return a map with both the object annotations and with the default NonAdminBackup annotations.
// If error occurs, a map with only the default NonAdminBackup annotations is returned
func AddNonAdminBackupAnnotations(ownerNamespace string, ownerName string, ownerUUID string, existingAnnotations map[string]string) map[string]string {
	// TODO could not receive object meta and get info from there?
	defaultAnnotations := map[string]string{
		constant.NabOriginNamespaceAnnotation: ownerNamespace,
		constant.NabOriginNameAnnotation:      ownerName,
		constant.NabOriginUUIDAnnotation:      ownerUUID,
	}

	mergedAnnotations, err := mergeUniqueKeyTOfTMaps(defaultAnnotations, existingAnnotations)
	if err != nil {
		// TODO logger
		_, _ = fmt.Println("Error merging annotations:", err)
		// TODO break?
		return defaultAnnotations
	}
	return mergedAnnotations
}

// GetBackupSpecFromNonAdminBackup return BackupSpec object from NonAdminBackup spec, if no error occurs
func GetBackupSpecFromNonAdminBackup(nonAdminBackup *nacv1alpha1.NonAdminBackup) (*velerov1api.BackupSpec, error) {
	if nonAdminBackup == nil {
		return nil, fmt.Errorf("nonAdminBackup is nil")
	}

	// TODO check spec?

	if nonAdminBackup.Spec.BackupSpec == nil {
		return nil, fmt.Errorf("BackupSpec is nil")
	}

	// TODO: Additional validations, before continuing

	return nonAdminBackup.Spec.BackupSpec.DeepCopy(), nil
}

// GenerateVeleroBackupName return generated name for Velero Backup object created from NonAdminBackup
func GenerateVeleroBackupName(namespace, nabName string) string {
	// Calculate a hash of the name
	hasher := sha1.New() //nolint:gosec // TODO use another tool
	_, _ = hasher.Write([]byte(nabName))
	const nameLength = 14
	nameHash := hex.EncodeToString(hasher.Sum(nil))[:nameLength] // Take first 14 chars

	// Generate the Velero backup name created from NAB
	veleroBackupName := fmt.Sprintf("nab-%s-%s", namespace, nameHash)

	const characterLimit = 253
	const occupiedSize = 4
	// Ensure the name is within the character limit
	if len(veleroBackupName) > characterLimit {
		// Truncate the namespace if necessary
		maxNamespaceLength := characterLimit - len(nameHash) - occupiedSize // Account for "nab-" and "-" TODO should not be 5?
		if len(namespace) > maxNamespaceLength {
			namespace = namespace[:maxNamespaceLength]
		}
		veleroBackupName = fmt.Sprintf("nab-%s-%s", namespace, nameHash)
	}

	return veleroBackupName
}

// UpdateNonAdminBackupFromVeleroBackup update, if necessary, NonAdminBackup object fields related to referenced Velero Backup object, if no error occurs
func UpdateNonAdminBackupFromVeleroBackup(ctx context.Context, r client.Client, logger logr.Logger, nab *nacv1alpha1.NonAdminBackup, veleroBackup *velerov1api.Backup) error {
	// Make a copy of the current status for comparison
	oldStatus := nab.Spec.BackupStatus.DeepCopy()
	oldSpec := nab.Spec.BackupSpec.DeepCopy()

	// Update the status & spec
	nab.Spec.BackupStatus = &veleroBackup.Status
	nab.Spec.BackupSpec = &veleroBackup.Spec

	if reflect.DeepEqual(oldStatus, nab.Spec.BackupStatus) && reflect.DeepEqual(oldSpec, nab.Spec.BackupSpec) {
		// No change, no need to update
		logger.V(1).Info("NonAdminBackup status and spec is already up to date")
		return nil
	}

	if err := r.Update(ctx, nab); err != nil {
		logger.Error(err, "Failed to update NonAdminBackup")
		return err
	}

	return nil
}

// CheckVeleroBackupLabels return true if Velero Backup object has required Non Admin labels, false otherwise
func CheckVeleroBackupLabels(backup *velerov1api.Backup) bool {
	// TODO also need to check for constant.OadpLabel label?
	labels := backup.GetLabels()
	value, exists := labels[constant.ManagedByLabel]
	return exists && value == constant.ManagedByLabelValue
}

// TODO not used

// GetNonAdminBackupFromVeleroBackup return referenced NonAdminBackup object from Velero Backup object, if no error occurs
func GetNonAdminBackupFromVeleroBackup(ctx context.Context, clientInstance client.Client, backup *velerov1api.Backup) (*nacv1alpha1.NonAdminBackup, error) {
	// Check if the backup has the required annotations to identify the associated NonAdminBackup object
	logger := log.FromContext(ctx)

	annotations := backup.GetAnnotations()

	annotationsStr := fmt.Sprintf("%v", annotations)
	logger.V(1).Info("Velero Backup Annotations", "annotations", annotationsStr)

	if annotations == nil {
		return nil, fmt.Errorf("backup has no annotations")
	}

	nabOriginNamespace, ok := annotations[constant.NabOriginNamespaceAnnotation]
	if !ok {
		return nil, fmt.Errorf(requiredAnnotationError, constant.NabOriginNamespaceAnnotation)
	}

	nabOriginName, ok := annotations[constant.NabOriginNameAnnotation]
	if !ok {
		return nil, fmt.Errorf(requiredAnnotationError, constant.NabOriginNameAnnotation)
	}

	nonAdminBackupKey := types.NamespacedName{
		Namespace: nabOriginNamespace,
		Name:      nabOriginName,
	}

	nonAdminBackup := &nacv1alpha1.NonAdminBackup{}
	err := clientInstance.Get(ctx, nonAdminBackupKey, nonAdminBackup)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch NonAdminBackup object: %v", err)
	}

	nabOriginUUID, ok := annotations[constant.NabOriginUUIDAnnotation]
	if !ok {
		return nil, fmt.Errorf(requiredAnnotationError, constant.NabOriginUUIDAnnotation)
	}
	// Ensure UID matches
	if nonAdminBackup.ObjectMeta.UID != types.UID(nabOriginUUID) {
		return nil, fmt.Errorf("UID from annotation does not match UID of fetched NonAdminBackup object")
	}

	return nonAdminBackup, nil
}

// TODO import?
// Similar to as pkg/common/common.go:AppendUniqueKeyTOfTMaps from github.com/openshift/oadp-operator
func mergeUniqueKeyTOfTMaps[T comparable](userMap ...map[T]T) (map[T]T, error) {
	var base map[T]T
	for i, mapElements := range userMap {
		if mapElements == nil {
			continue
		}
		if base == nil {
			base = make(map[T]T)
		}
		for k, v := range mapElements {
			existingValue, found := base[k]
			if found {
				if existingValue != v {
					return nil, fmt.Errorf("conflicting key %v with value %v in map %d may not override %v", k, v, i, existingValue)
				}
			} else {
				base[k] = v
			}
		}
	}
	return base, nil
}
