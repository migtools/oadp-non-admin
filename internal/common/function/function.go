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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
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

// GetOADPNamespace get the namespace OADP operator is installed
func GetOADPNamespace() string {
	return os.Getenv(constant.NamespaceEnvVar)
}

// AddNonAdminLabels return a map with both the object labels and with the default Non Admin labels.
// If error occurs, a map with only the default Non Admin labels is returned
func AddNonAdminLabels(labels map[string]string) map[string]string {
	defaultLabels := map[string]string{
		constant.OadpLabel:      "True",
		constant.ManagedByLabel: constant.ManagedByLabelValue,
	}

	mergedLabels, err := mergeMaps(defaultLabels, labels)
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

	mergedAnnotations, err := mergeMaps(defaultAnnotations, existingAnnotations)
	if err != nil {
		// TODO logger
		_, _ = fmt.Println("Error merging annotations:", err)
		// TODO break?
		return defaultAnnotations
	}
	return mergedAnnotations
}

// containsOnlyNamespace checks if the given namespaces slice contains only the specified namespace
func containsOnlyNamespace(namespaces []string, namespace string) bool {
	for _, ns := range namespaces {
		if ns != namespace {
			return false
		}
	}
	return true
}

// GetBackupSpecFromNonAdminBackup return BackupSpec object from NonAdminBackup spec, if no error occurs
func GetBackupSpecFromNonAdminBackup(nonAdminBackup *nacv1alpha1.NonAdminBackup) (*velerov1api.BackupSpec, error) {
	// TODO https://github.com/migtools/oadp-non-admin/issues/60
	if nonAdminBackup.Spec.BackupSpec == nil {
		// this should be Kubernetes API validation
		return nil, fmt.Errorf("BackupSpec is not defined")
	}

	veleroBackupSpec := nonAdminBackup.Spec.BackupSpec.DeepCopy()

	// TODO: Additional validations, before continuing

	if veleroBackupSpec.IncludedNamespaces == nil {
		veleroBackupSpec.IncludedNamespaces = []string{nonAdminBackup.Namespace}
	} else {
		if !containsOnlyNamespace(veleroBackupSpec.IncludedNamespaces, nonAdminBackup.Namespace) {
			return nil, fmt.Errorf("spec.backupSpec.IncludedNamespaces can not contain namespaces other than: %s", nonAdminBackup.Namespace)
		}
	}

	return veleroBackupSpec, nil
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

// UpdateNonAdminBackupFromVeleroBackup update, if necessary, NonAdminBackup object fields related to referenced Velero Backup object, if no error occurs
func UpdateNonAdminBackupFromVeleroBackup(ctx context.Context, r client.Client, logger logr.Logger, nab *nacv1alpha1.NonAdminBackup, veleroBackup *velerov1api.Backup) (bool, error) {
	logger.V(1).Info("NonAdminBackup BackupSpec and VeleroBackupStatus - request to update")

	if reflect.DeepEqual(nab.Status.VeleroBackupStatus, &veleroBackup.Status) && reflect.DeepEqual(nab.Spec.BackupSpec, &veleroBackup.Spec) {
		// No change, no need to update
		logger.V(1).Info("NonAdminBackup BackupSpec and BackupStatus - nothing to update")
		return false, nil
	}

	// Check if BackupStatus needs to be updated
	if !reflect.DeepEqual(nab.Status.VeleroBackupStatus, &veleroBackup.Status) || nab.Status.VeleroBackupName != veleroBackup.Name || nab.Status.VeleroBackupNamespace != veleroBackup.Namespace {
		nab.Status.VeleroBackupStatus = veleroBackup.Status.DeepCopy()
		nab.Status.VeleroBackupName = veleroBackup.Name
		nab.Status.VeleroBackupNamespace = veleroBackup.Namespace
		if err := r.Status().Update(ctx, nab); err != nil {
			logger.Error(err, "NonAdminBackup BackupStatus - Failed to update")
			return false, err
		}
		logger.V(1).Info("NonAdminBackup BackupStatus - updated")
		return true, nil
	}
	logger.V(1).Info("NonAdminBackup BackupStatus - up to date")

	// Check if BackupSpec needs to be updated
	// avoid spec change?
	if !reflect.DeepEqual(nab.Spec.BackupSpec, &veleroBackup.Spec) {
		nab.Spec.BackupSpec = veleroBackup.Spec.DeepCopy()
		if err := r.Update(ctx, nab); err != nil {
			logger.Error(err, "NonAdminBackup BackupSpec - Failed to update")
			return false, err
		}
		logger.V(1).Info("NonAdminBackup BackupSpec - updated")
		return true, nil
	}
	logger.V(1).Info("NonAdminBackup BackupSpec - up to date")

	return false, nil
}

// TODO not used

// CheckVeleroBackupLabels return true if Velero Backup object has required Non Admin labels, false otherwise
func CheckVeleroBackupLabels(labels map[string]string) bool {
	// TODO also need to check for constant.OadpLabel label?
	value, exists := labels[constant.ManagedByLabel]
	return exists && value == constant.ManagedByLabelValue
}

// TODO not used

// GetNonAdminBackupFromVeleroBackup return referenced NonAdminBackup object from Velero Backup object, if no error occurs
func GetNonAdminBackupFromVeleroBackup(ctx context.Context, clientInstance client.Client, backup *velerov1api.Backup) (*nacv1alpha1.NonAdminBackup, error) {
	// Check if the backup has the required annotations to identify the associated NonAdminBackup object
	logger := log.FromContext(ctx)

	// should run CheckVeleroBackupLabels here?

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

// TODO import? Similar to as pkg/common/common.go:AppendUniqueKeyTOfTMaps from github.com/openshift/oadp-operator

// Return map, of the same type as the input maps, that contains all keys/values from all input maps.
// Key/value pairs that are identical in different input maps, are added only once to return map.
// If a key exists in more than one input map, with a different value, an error is returned
func mergeMaps[T comparable](maps ...map[T]T) (map[T]T, error) {
	merge := make(map[T]T)
	for _, m := range maps {
		if m == nil {
			continue
		}
		for k, v := range m {
			existingValue, found := merge[k]
			if found {
				if existingValue != v {
					return nil, fmt.Errorf("conflicting key %v: has both value %v and value %v in input maps", k, v, existingValue)
				}
				continue
			}
			merge[k] = v
		}
	}
	return merge, nil
}
