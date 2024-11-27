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
	"fmt"
	"reflect"
	"strings"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	nacv1alpha1 "github.com/migtools/oadp-non-admin/api/v1alpha1"
	"github.com/migtools/oadp-non-admin/internal/common/constant"
)

// GetNonAdminLabels return the required Non Admin labels
func GetNonAdminLabels() map[string]string {
	return map[string]string{
		constant.OadpLabel:      constant.OadpLabelValue,
		constant.ManagedByLabel: constant.ManagedByLabelValue,
	}
}

// GetNonAdminBackupAnnotations return the required Non Admin annotations
func GetNonAdminBackupAnnotations(objectMeta metav1.ObjectMeta) map[string]string {
	return map[string]string{
		constant.NabOriginNamespaceAnnotation: objectMeta.Namespace,
		constant.NabOriginNameAnnotation:      objectMeta.Name,
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
func ValidateBackupSpec(nonAdminBackup *nacv1alpha1.NonAdminBackup, enforcedBackupSpec *velerov1.BackupSpec) error {
	// this should be Kubernetes API validation
	if nonAdminBackup.Spec.BackupSpec == nil {
		return fmt.Errorf("BackupSpec is not defined")
	}

	if nonAdminBackup.Spec.BackupSpec.IncludedNamespaces != nil {
		if !containsOnlyNamespace(nonAdminBackup.Spec.BackupSpec.IncludedNamespaces, nonAdminBackup.Namespace) {
			return fmt.Errorf("NonAdminBackup spec.backupSpec.includedNamespaces can not contain namespaces other than: %s", nonAdminBackup.Namespace)
		}
	}
	if enforcedBackupSpec.IncludedNamespaces != nil {
		if !containsOnlyNamespace(enforcedBackupSpec.IncludedNamespaces, nonAdminBackup.Namespace) {
			return fmt.Errorf("NonAdminBackup spec.backupSpec.includedNamespaces enforced value by admin user violates NAC usage")
		}
	}

	enforcedSpec := reflect.ValueOf(enforcedBackupSpec).Elem()
	for index := 0; index < enforcedSpec.NumField(); index++ {
		enforcedField := enforcedSpec.Field(index)
		enforcedFieldName := enforcedSpec.Type().Field(index).Name
		currentField := reflect.ValueOf(nonAdminBackup.Spec.BackupSpec).Elem().FieldByName(enforcedFieldName)
		if !enforcedField.IsZero() && !currentField.IsZero() && !reflect.DeepEqual(enforcedField.Interface(), currentField.Interface()) {
			field, _ := reflect.TypeOf(nonAdminBackup.Spec.BackupSpec).Elem().FieldByName(enforcedFieldName)
			tagName, _, _ := strings.Cut(field.Tag.Get("json"), ",")
			return fmt.Errorf(
				"NonAdminBackup spec.backupSpec.%v field value is enforced by admin user, can not override it",
				tagName,
			)
		}
	}

	return nil
}

// GenerateNacObjectUUID generates a unique name based on the provided namespace and object origin name.
// It includes a UUID suffix. If the name exceeds the maximum length, it truncates nacName first, then namespace.
func GenerateNacObjectUUID(namespace, nacName string) string {
	// Generate UUID suffix
	uuidSuffix := uuid.New().String()

	// Build the initial name based on the presence of namespace and nacName
	nacObjectName := uuidSuffix
	if len(nacName) > 0 {
		nacObjectName = nacName + constant.NameDelimiter + nacObjectName
	}
	if len(namespace) > 0 {
		nacObjectName = namespace + constant.NameDelimiter + nacObjectName
	}

	if len(nacObjectName) > constant.MaximumNacObjectNameLength {
		// Calculate remaining length after UUID
		remainingLength := constant.MaximumNacObjectNameLength - len(uuidSuffix)

		delimeterLength := len(constant.NameDelimiter)

		// Subtract two delimiter lengths to avoid a corner case where the namespace
		// and delimiters leave no space for any part of nabName
		if len(namespace) > remainingLength-delimeterLength-delimeterLength {
			namespace = namespace[:remainingLength-delimeterLength-delimeterLength]
			nacObjectName = namespace + constant.NameDelimiter + uuidSuffix
		} else {
			remainingLength = remainingLength - len(namespace) - delimeterLength - delimeterLength
			nacName = nacName[:remainingLength]
			nacObjectName = uuidSuffix
			if len(nacName) > 0 {
				nacObjectName = nacName + constant.NameDelimiter + nacObjectName
			}
			if len(namespace) > 0 {
				nacObjectName = namespace + constant.NameDelimiter + nacObjectName
			}
		}
	}

	return nacObjectName
}

// ListObjectsByLabel retrieves a list of Kubernetes objects in a specified namespace
// that match a given label key-value pair.
func ListObjectsByLabel(ctx context.Context, clientInstance client.Client, namespace string, labelKey string, labelValue string, objectList client.ObjectList) error {
	// Validate input parameters
	if namespace == constant.EmptyString || labelKey == constant.EmptyString || labelValue == constant.EmptyString {
		return fmt.Errorf("invalid input: namespace, labelKey, and labelValue must not be empty")
	}

	labelSelector := labels.SelectorFromSet(labels.Set{labelKey: labelValue})

	// Attempt to list objects with the specified label
	if err := clientInstance.List(ctx, objectList, &client.ListOptions{
		LabelSelector: labelSelector,
		Namespace:     namespace,
	}); err != nil {
		return fmt.Errorf("failed to list objects in namespace '%s': %w", namespace, err)
	}

	return nil
}

// GetVeleroBackupByLabel retrieves a VeleroBackup object based on a specified label within a given namespace.
// It returns the VeleroBackup only when exactly one object is found, throws an error if multiple backups are found,
// or returns nil if no matches are found.
func GetVeleroBackupByLabel(ctx context.Context, clientInstance client.Client, namespace string, labelValue string) (*velerov1.Backup, error) {
	veleroBackupList := &velerov1.BackupList{}

	// Call the generic ListLabeledObjectsInNamespace function
	if err := ListObjectsByLabel(ctx, clientInstance, namespace, constant.NabOriginNACUUIDLabel, labelValue, veleroBackupList); err != nil {
		return nil, err
	}

	switch len(veleroBackupList.Items) {
	case 0:
		return nil, nil // No matching VeleroBackup found
	case 1:
		return &veleroBackupList.Items[0], nil // Found 1 matching VeleroBackup
	default:
		return nil, fmt.Errorf("multiple VeleroBackup objects found with label %s=%s in namespace '%s'", constant.NabOriginNACUUIDLabel, labelValue, namespace)
	}
}

// GetActiveVeleroBackupsByLabel retrieves all VeleroBackup objects based on a specified label within a given namespace.
// It returns a slice of VeleroBackup objects or nil if none are found.
func GetActiveVeleroBackupsByLabel(ctx context.Context, clientInstance client.Client, namespace, labelKey, labelValue string) ([]velerov1.Backup, error) {
	var veleroBackupList velerov1.BackupList
	labelSelector := client.MatchingLabels{labelKey: labelValue}

	if err := clientInstance.List(ctx, &veleroBackupList, client.InNamespace(namespace), labelSelector); err != nil {
		return nil, err
	}

	// Filter out backups with a CompletionTimestamp
	var activeBackups []velerov1.Backup
	for _, backup := range veleroBackupList.Items {
		if backup.Status.CompletionTimestamp == nil {
			activeBackups = append(activeBackups, backup)
		}
	}

	if len(activeBackups) == 0 {
		return nil, nil
	}

	return activeBackups, nil
}

// GetBackupQueueInfo determines the queue position of the specified VeleroBackup.
// It calculates how many queued Backups exist in the namespace that were created before this one.
func GetBackupQueueInfo(ctx context.Context, clientInstance client.Client, namespace string, targetBackup *velerov1.Backup) (nacv1alpha1.QueueInfo, error) {
	var queueInfo nacv1alpha1.QueueInfo

	// If the target backup has no valid CreationTimestamp, it means that it's not yet reconciled by OADP/Velero.
	// In this case, we can't determine its queue position, so we return nil.
	if targetBackup == nil || targetBackup.CreationTimestamp.IsZero() {
		return queueInfo, nil
	}

	// If the target backup has a CompletionTimestamp, it means that it's already served.
	if targetBackup.Status.CompletionTimestamp != nil {
		queueInfo.EstimatedQueuePosition = 0
		return queueInfo, nil
	}

	// List all Backup objects in the namespace
	var backupList velerov1.BackupList
	if err := clientInstance.List(ctx, &backupList, client.InNamespace(namespace)); err != nil {
		return queueInfo, err
	}

	// Extract the target backup's creation timestamp
	targetTimestamp := targetBackup.CreationTimestamp.Time

	// The target backup is always in queue at least in the first position
	// 0 is reserved for the backups that are already served.
	queueInfo.EstimatedQueuePosition = 1

	// Iterate through backups and calculate position
	for i := range backupList.Items {
		backup := &backupList.Items[i]

		// Skip backups that have CompletionTimestamp set. This means that the Velero won't be further processing this backup.
		if backup.Status.CompletionTimestamp != nil {
			continue
		}

		// Count backups created earlier than the target backup
		if backup.CreationTimestamp.Time.Before(targetTimestamp) {
			queueInfo.EstimatedQueuePosition++
		}
	}

	return queueInfo, nil
}

// GetVeleroDeleteBackupRequestByLabel retrieves a DeleteBackupRequest object based on a specified label within a given namespace.
// It returns the DeleteBackupRequest only when exactly one object is found, throws an error if multiple backups are found,
// or returns nil if no matches are found.
func GetVeleroDeleteBackupRequestByLabel(ctx context.Context, clientInstance client.Client, namespace string, labelValue string) (*velerov1.DeleteBackupRequest, error) {
	veleroDeleteBackupRequestList := &velerov1.DeleteBackupRequestList{}

	// Call the generic ListLabeledObjectsInNamespace function
	if err := ListObjectsByLabel(ctx, clientInstance, namespace, velerov1.BackupNameLabel, labelValue, veleroDeleteBackupRequestList); err != nil {
		return nil, err
	}

	switch len(veleroDeleteBackupRequestList.Items) {
	case 0:
		return nil, nil // No matching DeleteBackupRequest found
	case 1:
		return &veleroDeleteBackupRequestList.Items[0], nil // Found 1 matching DeleteBackupRequest
	default:
		return nil, fmt.Errorf("multiple DeleteBackupRequest objects found with label %s=%s in namespace '%s'", velerov1.BackupNameLabel, labelValue, namespace)
	}
}

// CheckVeleroBackupMetadata return true if Velero Backup object has required Non Admin labels and annotations, false otherwise
func CheckVeleroBackupMetadata(obj client.Object) bool {
	objLabels := obj.GetLabels()
	if !checkLabelValue(objLabels, constant.OadpLabel, constant.OadpLabelValue) {
		return false
	}
	if !checkLabelValue(objLabels, constant.ManagedByLabel, constant.ManagedByLabelValue) {
		return false
	}

	if !checkLabelAnnotationValueIsValid(objLabels, constant.NabOriginNACUUIDLabel) {
		return false
	}

	annotations := obj.GetAnnotations()
	if !checkLabelAnnotationValueIsValid(annotations, constant.NabOriginNamespaceAnnotation) {
		return false
	}
	if !checkLabelAnnotationValueIsValid(annotations, constant.NabOriginNameAnnotation) {
		return false
	}

	return true
}

func checkLabelValue(objLabels map[string]string, key string, value string) bool {
	got, exists := objLabels[key]
	if !exists {
		return false
	}
	return got == value
}

func checkLabelAnnotationValueIsValid(labelsOrAnnotations map[string]string, key string) bool {
	value, exists := labelsOrAnnotations[key]
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
