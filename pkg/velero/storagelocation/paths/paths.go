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

// Package paths  helps NAC controllers with getting relevant object store paths of interests
// generally as defined in https://github.com/vmware-tanzu/velero/blob/0a280e57863c70495a2dfb65787615a68a0e7b03/pkg/persistence/object_store_layout.go
package paths

import (
	"context"
	"errors"
	"fmt"
	"path"

	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// BasePathForLocation returns base path for a given storage location
// <protocol>://<bucket>/<prefix>
func BasePathForLocation(storageLocation *velerov1.BackupStorageLocation) (basepath string, err error) {
	var protocol string
	switch storageLocation.Spec.Provider {
	case "aws":
		protocol = "s3://"
	// TODO: implement azure
	// azure format would be source-uri, like https://srcaccount.file.core.windows.net/myshare/mydir... //nolint:dupword // ehh
	// see: https://learn.microsoft.com/en-us/cli/azure/storage/file/copy?view=azure-cli-latest#az-storage-file-copy-start
	// TODO: implement gcp
	//nolint:dupword // gcp format would be gs://my-bucket, see: https://cloud.google.com/sdk/gcloud/reference/storage/cp
	default:
		return "", errors.New("unimplemented provider")
	}
	return path.Join(protocol, storageLocation.Spec.ObjectStorage.Bucket, storageLocation.Spec.ObjectStorage.Prefix), nil
}

// BasePathForBackup returns base path for a given backup
func BasePathForBackup(c client.Client, backup *velerov1.Backup) (basepath string, err error) {
	// get storage location
	storageLocationName := backup.Spec.StorageLocation
	storageLocation := velerov1.BackupStorageLocation{}
	err = c.Get(context.Background(), client.ObjectKey{Name: storageLocationName, Namespace: backup.Namespace}, &storageLocation)
	if err != nil {
		return "", errors.Join(errors.New("unable to get base path for backup"), err)
	}
	return BasePathForLocation(&storageLocation)
}

// BasePathForRestore returns base path for a given restore
func BasePathForRestore(c client.Client, restore *velerov1.Restore) (basepath string, err error) {
	// get backup name
	backupName := restore.Spec.BackupName
	backup := velerov1.Backup{}
	err = c.Get(context.Background(), client.ObjectKey{Name: backupName, Namespace: restore.Namespace}, &backup)
	if err != nil {
		return "", errors.Join(errors.New("unable to get base path for restore"), err) //nolint:revive // empty return
	}
	return BasePathForBackup(c, &backup)
}

// BackupLogs returns path to backup logs
// <basepath>/backups/<backup-name>/<backup-name>-logs.gz
func BackupLogs(c client.Client, backup *velerov1.Backup) (string, error) {
	basepath, err := BasePathForBackup(c, backup)
	if err != nil {
		return "", err
	}
	return path.Join(basepath, "backups", backup.Name, fmt.Sprintf("%s-logs.gz", backup.Name)), nil
}

// RestoreLogs returns path to restore logs
func RestoreLogs(c client.Client, restore *velerov1.Restore) (string, error) {
	basepath, err := BasePathForRestore(c, restore)
	if err != nil {
		return "", err
	}
	return path.Join(basepath, "restores", restore.Name, fmt.Sprintf("restore-%s-logs.gz", restore.Name)), nil
}

// BackupResourceList returns path to backup resource list
// <basepath>/backups/<backup-name>/<backup-name>-resource-list.json.gz
func BackupResourceList(c client.Client, backup *velerov1.Backup) (string, error) {
	basepath, err := BasePathForBackup(c, backup)
	if err != nil {
		return "", err
	}
	return path.Join(basepath, "backups", backup.Name, fmt.Sprintf("%s-resource-list.json.gz", backup.Name)), nil
}

// RestoreResourceList returns path to restore resource list
func RestoreResourceList(c client.Client, restore *velerov1.Restore) (string, error) {
	basepath, err := BasePathForRestore(c, restore)
	if err != nil {
		return "", err
	}
	return path.Join(basepath, "restores", restore.Name, fmt.Sprintf("restore-%s-resource-list.json.gz", restore.Name)), nil
}
