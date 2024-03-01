package controller

import (
	"context"

	"fmt"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	nacv1alpha1 "github.com/migtools/oadp-non-admin/api/v1alpha1"
	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
)

func HasRequiredLabel(backup *velerov1api.Backup) bool {
	labels := backup.GetLabels()
	value, exists := labels[ManagedByLabel]
	return exists && value == ManagedByLabelValue
}

func GetNonAdminFromBackup(ctx context.Context, client client.Client, backup *velerov1api.Backup) (*nacv1alpha1.NonAdminBackup, error) {
	// Check if the backup has the required annotations to identify the associated NonAdminBackup object
	log := log.FromContext(ctx)

	annotations := backup.GetAnnotations()

	annotationsStr := fmt.Sprintf("%v", annotations)
	log.V(1).Info("Velero Backup Annotations", "annotations", annotationsStr)

	if annotations == nil {
		return nil, fmt.Errorf("backup has no annotations")
	}

	nabOriginNamespace, ok := annotations[NabOriginNamespaceAnnotation]
	if !ok {
		return nil, fmt.Errorf("backup does not have the required annotation '%s'", NabOriginNamespaceAnnotation)
	}

	nabOriginName, ok := annotations[NabOriginNameAnnotation]
	if !ok {
		return nil, fmt.Errorf("backup does not have the required annotation '%s'", NabOriginNameAnnotation)
	}

	nonAdminBackupKey := types.NamespacedName{
		Namespace: nabOriginNamespace,
		Name:      nabOriginName,
	}

	nonAdminBackup := &nacv1alpha1.NonAdminBackup{}
	err := client.Get(ctx, nonAdminBackupKey, nonAdminBackup)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch NonAdminBackup object: %v", err)
	}

	nabOriginUuid, ok := annotations[NabOriginUuidAnnotation]
	if !ok {
		return nil, fmt.Errorf("backup does not have the required annotation '%s'", NabOriginUuidAnnotation)
	}
	// Ensure UID matches
	if nonAdminBackup.ObjectMeta.UID != types.UID(nabOriginUuid) {
		return nil, fmt.Errorf("UID from annotation does not match UID of fetched NonAdminBackup object")
	}

	return nonAdminBackup, nil
}
