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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	nacv1alpha1 "github.com/migtools/oadp-non-admin/api/v1alpha1"
	"github.com/migtools/oadp-non-admin/internal/common/constant"
)

// AddNonAdminLabels return the required Non Admin labels
func AddNonAdminLabels() map[string]string {
	return map[string]string{
		constant.OadpLabel:      constant.OadpLabelValue,
		constant.ManagedByLabel: constant.ManagedByLabelValue,
	}
}

// AddNonAdminBackupAnnotations return the required Non Admin annotations
func AddNonAdminBackupAnnotations(objectMeta metav1.ObjectMeta) map[string]string {
	return map[string]string{
		constant.NabOriginNamespaceAnnotation: objectMeta.Namespace,
		constant.NabOriginNameAnnotation:      objectMeta.Name,
		constant.NabOriginUUIDAnnotation:      string(objectMeta.UID),
	}
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

// ValidateBackupSpec return nil, if NonAdminBackup is valid; error otherwise
func ValidateBackupSpec(nonAdminBackup *nacv1alpha1.NonAdminBackup) error {
	// this should be Kubernetes API validation
	if nonAdminBackup.Spec.BackupSpec == nil {
		return fmt.Errorf("BackupSpec is not defined")
	}

	if nonAdminBackup.Spec.BackupSpec.IncludedNamespaces != nil {
		if !containsOnlyNamespace(nonAdminBackup.Spec.BackupSpec.IncludedNamespaces, nonAdminBackup.Namespace) {
			return fmt.Errorf("spec.backupSpec.IncludedNamespaces can not contain namespaces other than: %s", nonAdminBackup.Namespace)
		}
	}

	return nil
}

// GenerateVeleroBackupName generates a Velero backup name based on the provided namespace and NonAdminBackup name.
// It calculates a hash of the NonAdminBackup name and combines it with the namespace and a prefix to create the Velero backup name.
// If the resulting name exceeds the maximum Kubernetes name length, it truncates the namespace to fit within the limit.
func GenerateVeleroBackupName(namespace, nabName string) string {
	// Calculate a hash of the name
	hasher := sha256.New()
	_, err := hasher.Write([]byte(nabName))
	if err != nil {
		return ""
	}

	const hashLength = 14
	nameHash := hex.EncodeToString(hasher.Sum(nil))[:hashLength] // Take first 14 chars

	usedLength := hashLength + len(constant.VeleroBackupNamePrefix) + len("--")
	maxNamespaceLength := validation.DNS1123SubdomainMaxLength - usedLength
	// Ensure the name is within the character limit
	if len(namespace) > maxNamespaceLength {
		// Truncate the namespace if necessary
		return fmt.Sprintf("%s-%s-%s", constant.VeleroBackupNamePrefix, namespace[:maxNamespaceLength], nameHash)
	}

	return fmt.Sprintf("%s-%s-%s", constant.VeleroBackupNamePrefix, namespace, nameHash)
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

// GetLogger return a logger from input ctx, with additional key/value pairs being
// input key and input obj name and namespace
func GetLogger(ctx context.Context, obj client.Object, key string) logr.Logger {
	return log.FromContext(ctx).WithValues(key, types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()})
}
