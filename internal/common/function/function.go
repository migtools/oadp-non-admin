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

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
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
		constant.OadpLabel:      constant.OadpLabelValue,
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
func GetBackupSpecFromNonAdminBackup(nonAdminBackup *nacv1alpha1.NonAdminBackup) (*velerov1.BackupSpec, error) {
	// TODO https://github.com/migtools/oadp-non-admin/issues/60
	// this should be Kubernetes API validation
	if nonAdminBackup.Spec.BackupSpec == nil {
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
	if len(veleroBackupName) > validation.DNS1123SubdomainMaxLength {
		// Truncate the namespace if necessary
		maxNamespaceLength := validation.DNS1123SubdomainMaxLength - len(nameHash) - prefixLength
		if len(namespace) > maxNamespaceLength {
			namespace = namespace[:maxNamespaceLength]
		}
		veleroBackupName = fmt.Sprintf("%s-%s-%s", constant.VeleroBackupNamePrefix, namespace, nameHash)
	}

	return veleroBackupName
}

// GenerateVeleroBackupNameWithUUID generates a Velero backup name based on the provided namespace and NonAdminBackup name.
// It includes a UUID suffix. If the name exceeds the maximum length, it truncates nabName first, then namespace.
func GenerateVeleroBackupNameWithUUID(namespace, nabName string) string {
	// Generate UUID suffix
	uuidSuffix := uuid.New().String()

	// Build the initial backup name based on the presence of namespace and nabName
	veleroBackupName := uuidSuffix
	if len(nabName) > 0 {
		veleroBackupName = nabName + constant.BackupNameDelimiter + veleroBackupName
	}
	if len(namespace) > 0 {
		veleroBackupName = namespace + constant.BackupNameDelimiter + veleroBackupName
	}

	// Ensure the name is within the character limit
	maxLength := validation.DNS1123SubdomainMaxLength

	if len(veleroBackupName) > maxLength {
		// Calculate remaining length after UUID
		remainingLength := maxLength - len(uuidSuffix)

		delimeterLength := len(constant.BackupNameDelimiter)

		// Subtract two delimiter lengths to avoid a corner case where the namespace
		// and delimiters leave no space for any part of nabName
		if len(namespace) > remainingLength-delimeterLength-delimeterLength {
			namespace = namespace[:remainingLength-delimeterLength-delimeterLength]
			veleroBackupName = namespace + constant.BackupNameDelimiter + uuidSuffix
		} else {
			remainingLength = remainingLength - len(namespace) - delimeterLength - delimeterLength
			nabName = nabName[:remainingLength]
			veleroBackupName = uuidSuffix
			if len(nabName) > 0 {
				veleroBackupName = nabName + constant.BackupNameDelimiter + veleroBackupName
			}
			if len(namespace) > 0 {
				veleroBackupName = namespace + constant.BackupNameDelimiter + veleroBackupName
			}
		}
	}

	return veleroBackupName
}

// CheckVeleroBackupMetadata return true if Velero Backup object has required Non Admin labels and annotations, false otherwise
func CheckVeleroBackupMetadata(obj client.Object) bool {
	labels := obj.GetLabels()
	if !checkLabelValue(labels, constant.OadpLabel, constant.OadpLabelValue) {
		return false
	}
	if !checkLabelValue(labels, constant.ManagedByLabel, constant.ManagedByLabelValue) {
		return false
	}

	annotations := obj.GetAnnotations()
	if !checkAnnotationValueIsValid(annotations, constant.NabOriginNamespaceAnnotation) {
		return false
	}
	if !checkAnnotationValueIsValid(annotations, constant.NabOriginNameAnnotation) {
		return false
	}
	// TODO what is a valid uuid?
	if !checkAnnotationValueIsValid(annotations, constant.NabOriginUUIDAnnotation) {
		return false
	}

	return true
}

func checkLabelValue(labels map[string]string, key string, value string) bool {
	got, exists := labels[key]
	if !exists {
		return false
	}
	return got == value
}

func checkAnnotationValueIsValid(annotations map[string]string, key string) bool {
	value, exists := annotations[key]
	if !exists {
		return false
	}
	length := len(value)
	return length > 0 && length < validation.DNS1123SubdomainMaxLength
}

// TODO not used

// GetNonAdminBackupFromVeleroBackup return referenced NonAdminBackup object from Velero Backup object, if no error occurs
func GetNonAdminBackupFromVeleroBackup(ctx context.Context, clientInstance client.Client, backup *velerov1.Backup) (*nacv1alpha1.NonAdminBackup, error) {
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

// GetLogger return a logger from input ctx, with additional key/value pairs being
// input key and input obj name and namespace
func GetLogger(ctx context.Context, obj client.Object, key string) logr.Logger {
	return log.FromContext(ctx).WithValues(key, types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()})
}
