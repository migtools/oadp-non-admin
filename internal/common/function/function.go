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
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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

// GetNonAdminRestoreLabels return the required Non Admin restore labels
func GetNonAdminRestoreLabels(uniqueIdentifier string) map[string]string {
	nonAdminLabels := GetNonAdminLabels()
	nonAdminLabels[constant.NarOriginNACUUIDLabel] = uniqueIdentifier
	return nonAdminLabels
}

// GetNonAdminBackupAnnotations return the required Non Admin annotations
func GetNonAdminBackupAnnotations(objectMeta metav1.ObjectMeta) map[string]string {
	return map[string]string{
		constant.NabOriginNamespaceAnnotation: objectMeta.Namespace,
		constant.NabOriginNameAnnotation:      objectMeta.Name,
	}
}

// GetNonAdminRestoreAnnotations return the required Non Admin restore annotations
func GetNonAdminRestoreAnnotations(objectMeta metav1.ObjectMeta) map[string]string {
	return map[string]string{
		constant.NarOriginNamespaceAnnotation: objectMeta.Namespace,
		constant.NarOriginNameAnnotation:      objectMeta.Name,
	}
}

// GetNonAdminBackupStorageLocationAnnotations return the required Non Admin annotations
func GetNonAdminBackupStorageLocationAnnotations(objectMeta metav1.ObjectMeta) map[string]string {
	return map[string]string{
		constant.NabslOriginNamespaceAnnotation: objectMeta.Namespace,
		constant.NabslOriginNameAnnotation:      objectMeta.Name,
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
func ValidateBackupSpec(ctx context.Context, clientInstance client.Client, oadpNamespace string, nonAdminBackup *nacv1alpha1.NonAdminBackup, enforcedBackupSpec *velerov1.BackupSpec) error {
	if nonAdminBackup.Spec.BackupSpec.IncludedNamespaces != nil {
		if !containsOnlyNamespace(nonAdminBackup.Spec.BackupSpec.IncludedNamespaces, nonAdminBackup.Namespace) {
			return fmt.Errorf(constant.NABRestrictedErr+", can not contain namespaces other than: %s", "spec.backupSpec.includedNamespaces", nonAdminBackup.Namespace)
		}
	}

	if nonAdminBackup.Spec.BackupSpec.ExcludedNamespaces != nil {
		return fmt.Errorf(constant.NABRestrictedErr, "spec.backupSpec.excludedNamespaces")
	}

	if nonAdminBackup.Spec.BackupSpec.IncludeClusterResources != nil && *nonAdminBackup.Spec.BackupSpec.IncludeClusterResources {
		return fmt.Errorf(constant.NABRestrictedErr+", can only be set to false", "spec.backupSpec.includeClusterResources")
	}

	if len(nonAdminBackup.Spec.BackupSpec.IncludedClusterScopedResources) > 0 {
		return fmt.Errorf(constant.NABRestrictedErr+", must remain empty", "spec.backupSpec.includedScopedResources")
	}

	if nonAdminBackup.Spec.BackupSpec.StorageLocation != constant.EmptyString {
		nonAdminBsl := &nacv1alpha1.NonAdminBackupStorageLocation{}
		err := clientInstance.Get(ctx, types.NamespacedName{
			Name:      nonAdminBackup.Spec.BackupSpec.StorageLocation,
			Namespace: nonAdminBackup.Namespace,
		}, nonAdminBsl)
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("NonAdminBackupStorageLocation not found in the namespace: %v", err)
		} else if err != nil {
			return fmt.Errorf("NonAdminBackup spec.backupSpec.storageLocation is invalid: %v", err)
		}
		if nonAdminBsl.Status.VeleroBackupStorageLocation == nil || nonAdminBsl.Status.VeleroBackupStorageLocation.NACUUID == constant.EmptyString {
			return fmt.Errorf("unable to get VeleroBackupStorageLocation UUID from NonAdminBackupStorageLocation Status")
		}
		veleroObjectsNACUUID := nonAdminBsl.Status.VeleroBackupStorageLocation.NACUUID
		veleroBackupStorageLocation, veleroBslErr := GetVeleroBackupStorageLocationByLabel(ctx, clientInstance, oadpNamespace, veleroObjectsNACUUID)
		if veleroBslErr != nil {
			return fmt.Errorf("unable to get valid VeleroBackupStorageLocation referenced by the NACUUID %s from NonAdminBackupStorageLocation Status: %v", veleroObjectsNACUUID, veleroBslErr)
		}
		if veleroBackupStorageLocation == nil {
			return fmt.Errorf("VeleroBackupStorageLocation with NACUUID %s not found in the OADP namespace", veleroObjectsNACUUID)
		}
		if veleroBackupStorageLocation.Status.Phase != velerov1.BackupStorageLocationPhaseAvailable {
			return fmt.Errorf("VeleroBackupStorageLocation with NACUUID %s is not in available state and can not be used for the NonAdminBackup", veleroObjectsNACUUID)
		}
	}

	if nonAdminBackup.Spec.BackupSpec.VolumeSnapshotLocations != nil {
		return fmt.Errorf(constant.NABRestrictedErr, "spec.backupSpec.volumeSnapshotLocations")
	}

	enforcedSpec := reflect.ValueOf(enforcedBackupSpec).Elem()
	for index := range enforcedSpec.NumField() {
		enforcedField := enforcedSpec.Field(index)
		enforcedFieldName := enforcedSpec.Type().Field(index).Name
		currentField := reflect.ValueOf(nonAdminBackup.Spec.BackupSpec).Elem().FieldByName(enforcedFieldName)
		if !enforcedField.IsZero() && !currentField.IsZero() && !reflect.DeepEqual(enforcedField.Interface(), currentField.Interface()) {
			field, _ := reflect.TypeOf(nonAdminBackup.Spec.BackupSpec).Elem().FieldByName(enforcedFieldName)
			tagName, _, _ := strings.Cut(field.Tag.Get(constant.JSONTagString), constant.CommaString)
			return fmt.Errorf(
				"the administrator has restricted spec.backupSpec.%v field value to %v",
				tagName,
				reflect.Indirect(enforcedField),
			)
		}
	}

	return nil
}

// ValidateRestoreSpec return nil, if NonAdminRestore is valid; error otherwise
func ValidateRestoreSpec(ctx context.Context, clientInstance client.Client, nonAdminRestore *nacv1alpha1.NonAdminRestore, enforcedRestoreSpec *velerov1.RestoreSpec) error {
	if len(nonAdminRestore.Spec.RestoreSpec.ScheduleName) > 0 {
		return fmt.Errorf(constant.NARRestrictedErr, "nonAdminRestore.spec.restoreSpec.scheduleName")
	}

	if nonAdminRestore.Spec.RestoreSpec.BackupName == constant.EmptyString {
		return fmt.Errorf("NonAdminRestore spec.restoreSpec.backupName is not set")
	}

	nab := &nacv1alpha1.NonAdminBackup{}
	err := clientInstance.Get(ctx, types.NamespacedName{
		Name:      nonAdminRestore.Spec.RestoreSpec.BackupName,
		Namespace: nonAdminRestore.Namespace,
	}, nab)
	if err != nil {
		return fmt.Errorf("NonAdminRestore spec.restoreSpec.backupName is invalid: %v", err)
	}
	// TODO better way to check readiness? simplify and ask user to pass velero backup name? (user has access to this info in nonAdminBackup status)
	if nab.Status.Phase != nacv1alpha1.NonAdminPhaseCreated {
		return fmt.Errorf("NonAdminRestore spec.restoreSpec.backupName is invalid: NonAdminBackup is not ready to be restored")
	}
	// TODO validate that velero backup exists?
	// TODO does velero validate if backup is ready to be restored?
	// Issue link: https://github.com/migtools/oadp-non-admin/issues/225

	if nonAdminRestore.Spec.RestoreSpec.IncludedNamespaces != nil {
		return fmt.Errorf(constant.NARRestrictedErr, "nonAdminRestore.spec.restoreSpec.includedNamespaces")
	}

	if nonAdminRestore.Spec.RestoreSpec.ExcludedNamespaces != nil {
		return fmt.Errorf(constant.NARRestrictedErr, "nonAdminRestore.spec.restoreSpec.excludedNamespaces")
	}

	if nonAdminRestore.Spec.RestoreSpec.NamespaceMapping != nil {
		return fmt.Errorf(constant.NARRestrictedErr, "nonAdminRestore.spec.restoreSpec.namespaceMapping")
	}

	enforcedSpec := reflect.ValueOf(enforcedRestoreSpec).Elem()
	for index := range enforcedSpec.NumField() {
		enforcedField := enforcedSpec.Field(index)
		enforcedFieldName := enforcedSpec.Type().Field(index).Name
		currentField := reflect.ValueOf(nonAdminRestore.Spec.RestoreSpec).Elem().FieldByName(enforcedFieldName)
		if !enforcedField.IsZero() && !currentField.IsZero() && !reflect.DeepEqual(enforcedField.Interface(), currentField.Interface()) {
			field, _ := reflect.TypeOf(nonAdminRestore.Spec.RestoreSpec).Elem().FieldByName(enforcedFieldName)
			tagName, _, _ := strings.Cut(field.Tag.Get(constant.JSONTagString), constant.CommaString)
			return fmt.Errorf(
				"the administrator has restricted spec.restoreSpec.%v field value to %v",
				tagName,
				reflect.Indirect(enforcedField),
			)
		}
	}

	return nil
}

// ValidateBslSpec return nil, if NonAdminBackupStorageLocation is valid; error otherwise
func ValidateBslSpec(ctx context.Context, clientInstance client.Client, nonAdminBsl *nacv1alpha1.NonAdminBackupStorageLocation) error {
	// TODO Introduce validation for NaBSL as described in the
	// https://github.com/migtools/oadp-non-admin/issues/146
	if nonAdminBsl.Spec.BackupStorageLocationSpec.Default {
		return fmt.Errorf("NonAdminBackupStorageLocation cannot be used as a default BSL")
	}
	if nonAdminBsl.Spec.BackupStorageLocationSpec.Credential == nil {
		return fmt.Errorf("NonAdminBackupStorageLocation spec.bslSpec.credential is not set")
	} else if nonAdminBsl.Spec.BackupStorageLocationSpec.Credential.Name == constant.EmptyString || nonAdminBsl.Spec.BackupStorageLocationSpec.Credential.Key == constant.EmptyString {
		return fmt.Errorf("NonAdminBackupStorageLocation spec.bslSpec.credential.name or spec.bslSpec.credential.key is not set")
	}

	// TODO: Enforcement of NaBSL spec fields

	// Check if the secret exists in the same namespace
	secret := &corev1.Secret{}
	if err := clientInstance.Get(ctx, types.NamespacedName{
		Namespace: nonAdminBsl.Namespace,
		Name:      nonAdminBsl.Spec.BackupStorageLocationSpec.Credential.Name,
	}, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("BSL credentials secret not found: %v", err)
		}
		return fmt.Errorf("failed to get BSL credentials secret: %v", err)
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
		return fmt.Errorf("invalid input: namespace=%q, labelKey=%q, labelValue=%q", namespace, labelKey, labelValue)
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

// GetActiveVeleroRestoresByLabel retrieves all VeleroRestore objects based on a specified label within a given namespace.
// It returns a slice of VeleroRestore objects or nil if none are found.
func GetActiveVeleroRestoresByLabel(ctx context.Context, clientInstance client.Client, namespace, labelKey, labelValue string) ([]velerov1.Restore, error) {
	var veleroRestoreList velerov1.RestoreList
	labelSelector := client.MatchingLabels{labelKey: labelValue}

	if err := clientInstance.List(ctx, &veleroRestoreList, client.InNamespace(namespace), labelSelector); err != nil {
		return nil, err
	}

	// Filter out restores with a CompletionTimestamp
	var activeRestores []velerov1.Restore
	for _, restore := range veleroRestoreList.Items {
		if restore.Status.CompletionTimestamp == nil {
			activeRestores = append(activeRestores, restore)
		}
	}

	if len(activeRestores) == 0 {
		return nil, nil
	}

	return activeRestores, nil
}

// GetRestoreQueueInfo determines the queue position of the specified VeleroRestore.
// It calculates how many queued Restores exist in the namespace that were created before this one.
func GetRestoreQueueInfo(ctx context.Context, clientInstance client.Client, namespace string, targetRestore *velerov1.Restore) (nacv1alpha1.QueueInfo, error) {
	var queueInfo nacv1alpha1.QueueInfo

	// If the target restore has no valid CreationTimestamp, it means that it's not yet reconciled by OADP/Velero.
	// In this case, we can't determine its queue position, so we return nil.
	if targetRestore == nil || targetRestore.CreationTimestamp.IsZero() {
		return queueInfo, nil
	}

	// If the target restore has a CompletionTimestamp, it means that it's already served.
	if targetRestore.Status.CompletionTimestamp != nil {
		queueInfo.EstimatedQueuePosition = 0
		return queueInfo, nil
	}

	// List all Restore objects in the namespace
	var restoreList velerov1.RestoreList
	if err := clientInstance.List(ctx, &restoreList, client.InNamespace(namespace)); err != nil {
		return queueInfo, err
	}

	// Extract the target restore's creation timestamp
	targetTimestamp := targetRestore.CreationTimestamp.Time

	// The target restore is always in queue at least in the first position
	// 0 is reserved for the restores that are already served.
	queueInfo.EstimatedQueuePosition = 1

	// Iterate through restores and calculate position
	for i := range restoreList.Items {
		restore := &restoreList.Items[i]

		// Skip restores that have CompletionTimestamp set. This means that the Velero won't be further processing this restore.
		if restore.Status.CompletionTimestamp != nil {
			continue
		}

		// Count restores created earlier than the target restore
		if restore.CreationTimestamp.Time.Before(targetTimestamp) {
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

// GetVeleroRestoreByLabel retrieves a VeleroRestore object based on a specified label within a given namespace.
// It returns the VeleroRestore only when exactly one object is found, throws an error if multiple restores are found,
// or returns nil if no matches are found.
func GetVeleroRestoreByLabel(ctx context.Context, clientInstance client.Client, namespace string, labelValue string) (*velerov1.Restore, error) {
	veleroRestoreList := &velerov1.RestoreList{}
	if err := ListObjectsByLabel(ctx, clientInstance, namespace, constant.NarOriginNACUUIDLabel, labelValue, veleroRestoreList); err != nil {
		return nil, err
	}

	switch len(veleroRestoreList.Items) {
	case 0:
		return nil, nil // No matching VeleroRestores found
	case 1:
		return &veleroRestoreList.Items[0], nil
	default:
		return nil, fmt.Errorf("multiple VeleroRestore objects found with label %s=%s in namespace '%s'", constant.NarOriginNACUUIDLabel, labelValue, namespace)
	}
}

// GetNabslRequestByLabel retrieves a NonAdminBackupStorageLocationRequest object based on a specified label within a given namespace.
// It returns the NonAdminBackupStorageLocationRequest only when exactly one object is found, throws an error if multiple NonAdminBackupStorageLocationRequests are found,
// or returns nil if no matches are found.
func GetNabslRequestByLabel(ctx context.Context, clientInstance client.Client, namespace string, labelValue string) (*nacv1alpha1.NonAdminBackupStorageLocationRequest, error) {
	nabslRequestList := &nacv1alpha1.NonAdminBackupStorageLocationRequestList{}

	// Call the generic ListLabeledObjectsInNamespace function
	if err := ListObjectsByLabel(ctx, clientInstance, namespace, constant.NabslOriginNACUUIDLabel, labelValue, nabslRequestList); err != nil {
		return nil, err
	}

	switch len(nabslRequestList.Items) {
	case 0:
		return nil, nil // No matching NonAdminBackupStorageLocationRequest found
	case 1:
		return &nabslRequestList.Items[0], nil // Found 1 matching NonAdminBackupStorageLocationRequest
	default:
		return nil, fmt.Errorf("multiple NonAdminBackupStorageLocationRequest objects found with label %s=%s in namespace '%s'", constant.NabslOriginNACUUIDLabel, labelValue, namespace)
	}
}

// GetBslSecretByLabel retrieves a Secret object based on a specified label within a given namespace.
// It returns the Secret only when exactly one object is found, throws an error if multiple secrets are found,
// or returns nil if no matches are found.
func GetBslSecretByLabel(ctx context.Context, clientInstance client.Client, namespace string, labelValue string) (*corev1.Secret, error) {
	secretList := &corev1.SecretList{}

	// Call the generic ListLabeledObjectsInNamespace function
	if err := ListObjectsByLabel(ctx, clientInstance, namespace, constant.NabslOriginNACUUIDLabel, labelValue, secretList); err != nil {
		return nil, err
	}

	switch len(secretList.Items) {
	case 0:
		return nil, nil // No matching Secret found
	case 1:
		return &secretList.Items[0], nil // Found 1 matching Secret
	default:
		return nil, fmt.Errorf("multiple Secret objects found with label %s=%s in namespace '%s'", velerov1.StorageLocationLabel, labelValue, namespace)
	}
}

// GetVeleroBackupStorageLocationByLabel retrieves a VeleroBackupStorageLocation object based on a specified label within a given namespace.
// It returns the VeleroBackupStorageLocation only when exactly one object is found, throws an error if multiple VeleroBackupStorageLocation are found,
// or returns nil if no matches are found.
func GetVeleroBackupStorageLocationByLabel(ctx context.Context, clientInstance client.Client, namespace string, labelValue string) (*velerov1.BackupStorageLocation, error) {
	bslList := &velerov1.BackupStorageLocationList{}

	// Call the generic ListLabeledObjectsInNamespace function
	if err := ListObjectsByLabel(ctx, clientInstance, namespace, constant.NabslOriginNACUUIDLabel, labelValue, bslList); err != nil {
		return nil, err
	}

	switch len(bslList.Items) {
	case 0:
		return nil, nil // No matching VeleroBackupStorageLocation found
	case 1:
		return &bslList.Items[0], nil // Found 1 matching VeleroBackupStorageLocation
	default:
		return nil, fmt.Errorf("multiple VeleroBackupStorageLocation objects found with label %s=%s in namespace '%s'", velerov1.StorageLocationLabel, labelValue, namespace)
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

	if !CheckLabelAnnotationValueIsValid(objLabels, constant.NabOriginNACUUIDLabel) {
		return false
	}

	return CheckVeleroBackupAnnotations(obj)
}

// CheckVeleroBackupAnnotations return true if Velero Backup object has required Non Admin annotations, false otherwise
func CheckVeleroBackupAnnotations(obj client.Object) bool {
	annotations := obj.GetAnnotations()
	if !CheckLabelAnnotationValueIsValid(annotations, constant.NabOriginNamespaceAnnotation) {
		return false
	}
	if !CheckLabelAnnotationValueIsValid(annotations, constant.NabOriginNameAnnotation) {
		return false
	}

	return true
}

// CheckVeleroRestoreMetadata return true if Velero Restore object has required Non Admin labels and annotations, false otherwise
func CheckVeleroRestoreMetadata(obj client.Object) bool {
	objLabels := obj.GetLabels()
	if !checkLabelValue(objLabels, constant.OadpLabel, constant.OadpLabelValue) {
		return false
	}
	if !checkLabelValue(objLabels, constant.ManagedByLabel, constant.ManagedByLabelValue) {
		return false
	}

	labelValue, exists := objLabels[constant.NarOriginNACUUIDLabel]
	if !exists || len(labelValue) == 0 {
		return false
	}

	return CheckVeleroRestoreAnnotations(obj)
}

// CheckVeleroRestoreAnnotations return true if Velero Restore object has required Non Admin annotations, false otherwise
func CheckVeleroRestoreAnnotations(obj client.Object) bool {
	annotations := obj.GetAnnotations()
	if !CheckLabelAnnotationValueIsValid(annotations, constant.NarOriginNamespaceAnnotation) {
		return false
	}
	if !CheckLabelAnnotationValueIsValid(annotations, constant.NarOriginNameAnnotation) {
		return false
	}

	return true
}

// CheckVeleroBackupStorageLocationMetadata return true if Velero BackupStorageLocation object has required Non Admin labels and annotations, false otherwise
func CheckVeleroBackupStorageLocationMetadata(obj client.Object) bool {
	objLabels := obj.GetLabels()
	if !checkLabelValue(objLabels, constant.OadpLabel, constant.OadpLabelValue) {
		return false
	}
	if !checkLabelValue(objLabels, constant.ManagedByLabel, constant.ManagedByLabelValue) {
		return false
	}

	if !CheckLabelAnnotationValueIsValid(objLabels, constant.NabslOriginNACUUIDLabel) {
		return false
	}

	return CheckVeleroBackupStorageLocationAnnotations(obj)
}

// CheckVeleroBackupStorageLocationAnnotations return true if Velero BackupStorageLocation object has required Non Admin annotations, false otherwise
func CheckVeleroBackupStorageLocationAnnotations(obj client.Object) bool {
	annotations := obj.GetAnnotations()
	if !CheckLabelAnnotationValueIsValid(annotations, constant.NabslOriginNamespaceAnnotation) {
		return false
	}
	if !CheckLabelAnnotationValueIsValid(annotations, constant.NabslOriginNameAnnotation) {
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

// CheckLabelAnnotationValueIsValid return true if key exists among labels/annotations and has a valid length, false otherwise
func CheckLabelAnnotationValueIsValid(labelsOrAnnotations map[string]string, key string) bool {
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

// ComputePrefixForObjectStorage returns the prefix to be used for the BackupStorageLocation.
// If a custom prefix is provided, it returns "<namespace>/<customPrefix>".
// Otherwise, it returns the namespace name.
func ComputePrefixForObjectStorage(namespace, customPrefix string) string {
	if len(customPrefix) > 0 {
		return fmt.Sprintf("%s/%s", namespace, customPrefix)
	}
	return namespace
}
