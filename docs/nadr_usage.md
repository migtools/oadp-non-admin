# NonAdminDownloadRequest (NADR) Usage Guide

## Overview

NonAdminDownloadRequest (NADR) allows non-admin users to download logs, backup contents, and other information related to their NonAdminBackup and NonAdminRestore operations. This feature provides secure, time-limited access to download resources through signed URLs.

## Prerequisites

- OADP operator version 1.5+ installed and configured
- NonAdminController (NAC) deployed and running
- Existing NonAdminBackup or NonAdminRestore resources
- NonAdminBackup must use NonAdminBackupStorageLocation (NABSL)

## Supported Download Request Types

### For NonAdminBackup Resources

| **Download Kind** | **Description** | **Content** |
|-------------------|-----------------|-------------|
| `BackupLog` | Backup operation logs | Detailed logs from the backup process |
| `BackupContents` | Backup metadata and manifest | Complete backup metadata and resource manifests |
| `BackupVolumeSnapshots` | Volume snapshot information | Details about volume snapshots created |
| `BackupItemOperations` | Item operation details | Information about backup item operations |
| `BackupResourceList` | List of backed up resources | Complete list of resources included in backup |
| `BackupResults` | Backup execution results | Summary and results of backup operation |
| `CSIBackupVolumeSnapshots` | CSI volume snapshots | CSI-specific volume snapshot information |
| `CSIBackupVolumeSnapshotContents` | CSI snapshot contents | Detailed CSI volume snapshot contents |
| `BackupVolumeInfos` | Volume information | Information about volumes in the backup |

### For NonAdminRestore Resources

| **Download Kind** | **Description** | **Content** |
|-------------------|-----------------|-------------|
| `RestoreLog` | Restore operation logs | Detailed logs from the restore process |
| `RestoreResults` | Restore execution results | Summary and results of restore operation |
| `RestoreResourceList` | List of restored resources | Complete list of resources restored |
| `RestoreItemOperations` | Item operation details | Information about restore item operations |
| `RestoreVolumeInfo` | Volume information | Information about volumes in the restore |

## Creating a NonAdminDownloadRequest

### Basic NADR Structure

```yaml
apiVersion: oadp.openshift.io/v1alpha1
kind: NonAdminDownloadRequest
metadata:
  name: <request-name>
  namespace: <user-namespace>
spec:
  target:
    kind: <download-kind>      # One of the supported kinds above
    name: <backup-or-restore-name>  # Name of the NAB or NAR
```

### Example 1: Download Backup Logs

```yaml
apiVersion: oadp.openshift.io/v1alpha1
kind: NonAdminDownloadRequest
metadata:
  name: backup-logs-download
  namespace: my-app-namespace
spec:
  target:
    kind: BackupLog
    name: my-backup
```

### Example 2: Download Backup Contents

```yaml
apiVersion: oadp.openshift.io/v1alpha1
kind: NonAdminDownloadRequest
metadata:
  name: backup-contents-download
  namespace: my-app-namespace
spec:
  target:
    kind: BackupContents
    name: my-backup
```

### Example 3: Download Restore Logs

```yaml
apiVersion: oadp.openshift.io/v1alpha1
kind: NonAdminDownloadRequest
metadata:
  name: restore-logs-download
  namespace: my-app-namespace
spec:
  target:
    kind: RestoreLog
    name: my-restore
```

## Creating and Managing Download Requests

### Create a Download Request

```bash
# Create from YAML file
oc create -f nadr-example.yaml

# Or create directly
oc create -f - <<EOF
apiVersion: oadp.openshift.io/v1alpha1
kind: NonAdminDownloadRequest
metadata:
  name: backup-logs-download
  namespace: my-namespace
spec:
  target:
    kind: BackupLog
    name: my-backup
EOF
```

### Monitor Download Request Status

```bash
# Check NADR status
oc get nadr backup-logs-download -n my-namespace

# Get detailed status
oc describe nadr backup-logs-download -n my-namespace

# Watch for status changes
oc get nadr backup-logs-download -n my-namespace -w
```

### Download Request Phases

| **Phase** | **Description** |
|-----------|-----------------|
| `New` | Request created, not yet processed |
| `BackingOff` | Error occurred, controller is backing off |
| `Created` | Download request processed, URL available |

### Download Request Conditions

| **Condition** | **Description** |
|---------------|-----------------|
| `NonAdminBackupStorageLocationNotUsed` | Backup doesn't use NABSL (terminal error) |
| `NonAdminBackupNotAvailable` | Referenced backup not found |
| `NonAdminRestoreNotAvailable` | Referenced restore not found |
| `Processed` | Request successfully processed |

## Downloading Files Using Signed URLs

### Get the Download URL

Once the NADR status shows `Phase: Created`, the signed URL is available in the status:

```bash
# Extract the download URL
oc get nadr backup-logs-download -n my-namespace -o jsonpath='{.status.velero.status.downloadURL}'
```

### Download Using wget

```bash
# Get the URL from the NADR status
DOWNLOAD_URL=$(oc get nadr backup-logs-download -n my-namespace -o jsonpath='{.status.velero.status.downloadURL}')

# Download the file
wget "$DOWNLOAD_URL" -O backup-logs.tar.gz

# Extract if it's a compressed file
tar -xzf backup-logs.tar.gz
```

### Download Using curl

```bash
# Get the URL from the NADR status
DOWNLOAD_URL=$(oc get nadr backup-logs-download -n my-namespace -o jsonpath='{.status.velero.status.downloadURL}')

# Download the file
curl -L "$DOWNLOAD_URL" -o backup-logs.tar.gz

# Extract if it's a compressed file
tar -xzf backup-logs.tar.gz
```

### Complete Download Script Example

```bash
#!/bin/bash

NADR_NAME="backup-logs-download"
NAMESPACE="my-namespace"
OUTPUT_FILE="backup-logs.tar.gz"

# Wait for NADR to be processed
echo "Waiting for download request to be processed..."
oc wait --for=condition=Processed nadr/$NADR_NAME -n $NAMESPACE --timeout=300s

# Check if the request was successful
PHASE=$(oc get nadr $NADR_NAME -n $NAMESPACE -o jsonpath='{.status.phase}')
if [ "$PHASE" != "Created" ]; then
    echo "Download request failed with phase: $PHASE"
    oc describe nadr $NADR_NAME -n $NAMESPACE
    exit 1
fi

# Get the download URL
DOWNLOAD_URL=$(oc get nadr $NADR_NAME -n $NAMESPACE -o jsonpath='{.status.velero.status.downloadURL}')

if [ -z "$DOWNLOAD_URL" ]; then
    echo "No download URL available"
    exit 1
fi

# Check URL expiration
EXPIRATION=$(oc get nadr $NADR_NAME -n $NAMESPACE -o jsonpath='{.status.velero.status.expiration}')
echo "Download URL expires at: $EXPIRATION"

# Download the file
echo "Downloading from: $DOWNLOAD_URL"
wget "$DOWNLOAD_URL" -O "$OUTPUT_FILE"

if [ $? -eq 0 ]; then
    echo "Download completed successfully: $OUTPUT_FILE"
    echo "File size: $(du -h $OUTPUT_FILE | cut -f1)"
else
    echo "Download failed"
    exit 1
fi
```

## URL Expiration and Security

- Download URLs are **time-limited** and will expire
- URLs are **signed** for security
- Check the expiration time in the NADR status: `.status.velero.status.expiration`
- If a URL expires, create a new NADR to get a fresh URL

## Troubleshooting

### Common Issues and Solutions

#### 1. NADR Stuck in "BackingOff" Phase

**Problem**: Download request shows `Phase: BackingOff`

**Solution**:
```bash
# Check the conditions for details
oc describe nadr <nadr-name> -n <namespace>

# Common causes:
# - Backup doesn't use NonAdminBackupStorageLocation
# - Referenced backup/restore doesn't exist
# - Permissions issues
```

#### 2. "NonAdminBackupStorageLocationNotUsed" Error

**Problem**: Error condition shows backup doesn't use NABSL

**Solution**:
- Ensure your NonAdminBackup uses a NonAdminBackupStorageLocation
- Recreate the NADR after fixing the backup configuration
- This is a terminal error - the NADR will not retry

#### 3. "NonAdminBackupNotAvailable" or "NonAdminRestoreNotAvailable"

**Problem**: Referenced backup or restore not found

**Solution**:
```bash
# Verify the backup/restore exists
oc get nab <backup-name> -n <namespace>
oc get nar <restore-name> -n <namespace>

# Check the name matches exactly in the NADR spec
```

#### 4. Download URL Not Available

**Problem**: NADR status shows `Created` but no URL

**Solution**:
```bash
# Check the underlying Velero DownloadRequest
oc get downloadrequest -n <oadp-namespace>

# Look for events
oc get events -n <namespace> --field-selector involvedObject.name=<nadr-name>
```

#### 5. URL Expired

**Problem**: Download fails with 403 or similar error

**Solution**:
```bash
# Check expiration time
oc get nadr <nadr-name> -n <namespace> -o jsonpath='{.status.velero.status.expiration}'

# Create a new NADR if expired
```

### Debugging Commands

```bash
# Check NADR status and conditions
oc get nadr -n <namespace>
oc describe nadr <nadr-name> -n <namespace>

# Check related Velero resources
oc get downloadrequest -n <oadp-namespace>
oc get backup,restore -n <oadp-namespace>

# Check controller logs
oc logs -n <oadp-namespace> deployment/oadp-non-admin-controller-manager

# Check events
oc get events -n <namespace> --sort-by='.lastTimestamp'
```

## Best Practices

1. **Create NADR close to when you need to download** - URLs have expiration times
2. **Use descriptive names** - Makes it easier to track multiple download requests
3. **Clean up old NADRs** - Remove completed download requests to avoid clutter
4. **Check phases and conditions** - Always verify the NADR is processed before attempting download
5. **Handle URL expiration** - Implement retry logic in automation scripts
6. **Verify backup uses NABSL** - Ensure NonAdminBackups use NonAdminBackupStorageLocation

## Helper Script

For easier usage, a helper script is provided at `hack/nadr-download.sh` that automates the entire process:

```bash
# Download backup logs
./hack/nadr-download.sh -k BackupLog -n my-backup -ns my-namespace

# Download restore logs with verbose output
./hack/nadr-download.sh -k RestoreLog -n my-restore -ns my-namespace -v

# Download backup contents to specific directory
./hack/nadr-download.sh -k BackupContents -n my-backup -ns my-namespace -o /tmp/downloads
```

The script handles:
- Creating the NonAdminDownloadRequest
- Waiting for processing completion
- Downloading the file using the signed URL
- Optional extraction of compressed files
- Cleanup of the download request

## Integration with Automation

### Example: Automated Backup and Download

```bash
#!/bin/bash

NAMESPACE="my-app"
BACKUP_NAME="automated-backup-$(date +%Y%m%d-%H%M%S)"

# Create backup
oc apply -f - <<EOF
apiVersion: oadp.openshift.io/v1alpha1
kind: NonAdminBackup
metadata:
  name: $BACKUP_NAME
  namespace: $NAMESPACE
spec:
  backupSpec:
    includedNamespaces:
    - $NAMESPACE
    storageLocation: my-nabsl
EOF

# Wait for backup completion
oc wait --for=condition=Completed nab/$BACKUP_NAME -n $NAMESPACE --timeout=600s

# Use the helper script to download logs
./hack/nadr-download.sh -k BackupLog -n $BACKUP_NAME -ns $NAMESPACE -o ./backup-logs
```

This guide provides comprehensive information for using the NonAdminDownloadRequest feature effectively and troubleshooting common issues.