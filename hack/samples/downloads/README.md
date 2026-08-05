# NonAdminDownloadRequest (NADR) Samples

This directory contains sample templates for creating NonAdminDownloadRequest resources to download various types of backup and restore information.

## Available Samples

### Backup-Related Downloads

- **backup-logs.yaml** - Download backup operation logs
- **backup-contents.yaml** - Download backup metadata and manifest files  
- **backup-resource-list.yaml** - Download list of resources included in backup

### Restore-Related Downloads

- **restore-logs.yaml** - Download restore operation logs

## Usage

All samples are OpenShift templates that can be processed with parameters:

### Example: Download Backup Logs

```bash
oc process -f backup-logs.yaml \
  -p NAMESPACE=my-app-namespace \
  -p BACKUP_NAME=my-backup \
  -p REQUEST_NAME=my-backup-logs \
  | oc apply -f -
```

### Example: Download Backup Contents

```bash
oc process -f backup-contents.yaml \
  -p NAMESPACE=my-app-namespace \
  -p BACKUP_NAME=my-backup \
  | oc apply -f -
```

### Example: Download Restore Logs

```bash
oc process -f restore-logs.yaml \
  -p NAMESPACE=my-app-namespace \
  -p RESTORE_NAME=my-restore \
  | oc apply -f -
```

## Parameters

All templates support these parameters:

- **NAMESPACE** (required) - Namespace containing the backup/restore
- **BACKUP_NAME/RESTORE_NAME** (required) - Name of the backup or restore
- **REQUEST_NAME** (optional) - Name for the download request (has defaults)

## After Creation

1. Wait for the download request to be processed:
   ```bash
   oc wait --for=condition=Processed nadr/<request-name> -n <namespace> --timeout=300s
   ```

2. Get the download URL and download the file:
   ```bash
   DOWNLOAD_URL=$(oc get nadr <request-name> -n <namespace> -o jsonpath='{.status.velero.status.downloadURL}')
   wget "$DOWNLOAD_URL" -O downloaded-file.tar.gz
   ```

For complete documentation, see [NADR Usage Guide](../../docs/nadr_usage.md).