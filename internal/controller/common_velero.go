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
	"fmt"

	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	nacv1alpha1 "github.com/migtools/oadp-non-admin/api/v1alpha1"
)

const requiredAnnotationError = "backup does not have the required annotation '%s'"

func HasRequiredLabel(backup *velerov1api.Backup) bool {
	labels := backup.GetLabels()
	value, exists := labels[ManagedByLabel]
	return exists && value == ManagedByLabelValue
}

func GetNonAdminFromBackup(ctx context.Context, clientInstance client.Client, backup *velerov1api.Backup) (*nacv1alpha1.NonAdminBackup, error) {
	// Check if the backup has the required annotations to identify the associated NonAdminBackup object
	logger := log.FromContext(ctx)

	annotations := backup.GetAnnotations()

	annotationsStr := fmt.Sprintf("%v", annotations)
	logger.V(1).Info("Velero Backup Annotations", "annotations", annotationsStr)

	if annotations == nil {
		return nil, fmt.Errorf("backup has no annotations")
	}

	nabOriginNamespace, ok := annotations[NabOriginNamespaceAnnotation]
	if !ok {
		return nil, fmt.Errorf(requiredAnnotationError, NabOriginNamespaceAnnotation)
	}

	nabOriginName, ok := annotations[NabOriginNameAnnotation]
	if !ok {
		return nil, fmt.Errorf(requiredAnnotationError, NabOriginNameAnnotation)
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

	nabOriginUUID, ok := annotations[NabOriginUUIDAnnotation]
	if !ok {
		return nil, fmt.Errorf(requiredAnnotationError, NabOriginUUIDAnnotation)
	}
	// Ensure UID matches
	if nonAdminBackup.ObjectMeta.UID != types.UID(nabOriginUUID) {
		return nil, fmt.Errorf("UID from annotation does not match UID of fetched NonAdminBackup object")
	}

	return nonAdminBackup, nil
}
