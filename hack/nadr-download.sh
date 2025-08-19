#!/bin/bash

# NADR Download Helper Script
# This script automates the process of creating a NonAdminDownloadRequest 
# and downloading the resulting file using the signed URL.

set -e

# Default values
TIMEOUT=300
OUTPUT_DIR="./downloads"
VERBOSE=false

# Function to display usage
usage() {
    cat << EOF
Usage: $0 [OPTIONS] -k <kind> -n <name> -ns <namespace>

Download logs and other information from NonAdminBackup or NonAdminRestore resources.

Required arguments:
  -k, --kind        Kind of download (e.g., BackupLog, RestoreLog, BackupContents, etc.)
  -n, --name        Name of the NonAdminBackup or NonAdminRestore
  -ns, --namespace  Namespace containing the backup/restore

Optional arguments:
  -r, --request-name  Name for the download request (default: auto-generated)
  -o, --output-dir    Output directory for downloaded files (default: ./downloads)
  -t, --timeout       Timeout in seconds to wait for processing (default: 300)
  -v, --verbose       Enable verbose output
  -h, --help          Display this help message

Supported download kinds:
  Backup downloads:    BackupLog, BackupContents, BackupVolumeSnapshots, 
                       BackupItemOperations, BackupResourceList, BackupResults,
                       CSIBackupVolumeSnapshots, CSIBackupVolumeSnapshotContents,
                       BackupVolumeInfos
  
  Restore downloads:   RestoreLog, RestoreResults, RestoreResourceList,
                       RestoreItemOperations, RestoreVolumeInfo

Examples:
  # Download backup logs
  $0 -k BackupLog -n my-backup -ns my-app

  # Download restore logs with custom output directory
  $0 -k RestoreLog -n my-restore -ns my-app -o /tmp/restore-logs

  # Download backup contents with verbose output
  $0 -k BackupContents -n my-backup -ns my-app -v
EOF
}

# Function for verbose output
log() {
    if [ "$VERBOSE" = true ]; then
        echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*"
    fi
}

# Function to generate output filename
generate_filename() {
    local kind="$1"
    local name="$2"
    local timestamp=$(date +%Y%m%d-%H%M%S)
    echo "${name}-${kind,,}-${timestamp}.tar.gz"
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -k|--kind)
            KIND="$2"
            shift 2
            ;;
        -n|--name)
            NAME="$2"
            shift 2
            ;;
        -ns|--namespace)
            NAMESPACE="$2"
            shift 2
            ;;
        -r|--request-name)
            REQUEST_NAME="$2"
            shift 2
            ;;
        -o|--output-dir)
            OUTPUT_DIR="$2"
            shift 2
            ;;
        -t|--timeout)
            TIMEOUT="$2"
            shift 2
            ;;
        -v|--verbose)
            VERBOSE=true
            shift
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            usage
            exit 1
            ;;
    esac
done

# Validate required arguments
if [[ -z "$KIND" || -z "$NAME" || -z "$NAMESPACE" ]]; then
    echo "Error: Missing required arguments"
    usage
    exit 1
fi

# Set default request name if not provided
if [[ -z "$REQUEST_NAME" ]]; then
    REQUEST_NAME="${NAME,,}-${KIND,,}-download-$(date +%s)"
fi

# Create output directory if it doesn't exist
mkdir -p "$OUTPUT_DIR"

log "Starting download process for $KIND from $NAME in namespace $NAMESPACE"

# Check if the referenced backup/restore exists
if [[ "$KIND" == *"Backup"* || "$KIND" == *"backup"* ]]; then
    log "Checking if NonAdminBackup '$NAME' exists in namespace '$NAMESPACE'"
    if ! oc get nab "$NAME" -n "$NAMESPACE" &> /dev/null; then
        echo "Error: NonAdminBackup '$NAME' not found in namespace '$NAMESPACE'"
        exit 1
    fi
else
    log "Checking if NonAdminRestore '$NAME' exists in namespace '$NAMESPACE'"
    if ! oc get nar "$NAME" -n "$NAMESPACE" &> /dev/null; then
        echo "Error: NonAdminRestore '$NAME' not found in namespace '$NAMESPACE'"
        exit 1
    fi
fi

# Create the NonAdminDownloadRequest
log "Creating NonAdminDownloadRequest '$REQUEST_NAME'"
cat << EOF | oc apply -f -
apiVersion: oadp.openshift.io/v1alpha1
kind: NonAdminDownloadRequest
metadata:
  name: $REQUEST_NAME
  namespace: $NAMESPACE
  labels:
    app.kubernetes.io/name: oadp-nac
    app.kubernetes.io/component: download-request
    created-by: nadr-download-script
spec:
  target:
    kind: $KIND
    name: $NAME
EOF

echo "NonAdminDownloadRequest '$REQUEST_NAME' created successfully"

# Wait for the request to be processed
echo "Waiting for download request to be processed (timeout: ${TIMEOUT}s)..."
if ! oc wait --for=condition=Processed nadr/"$REQUEST_NAME" -n "$NAMESPACE" --timeout="${TIMEOUT}s"; then
    echo "Error: Download request timed out or failed"
    echo "Current status:"
    oc describe nadr "$REQUEST_NAME" -n "$NAMESPACE"
    exit 1
fi

# Check if the request was successful
PHASE=$(oc get nadr "$REQUEST_NAME" -n "$NAMESPACE" -o jsonpath='{.status.phase}')
if [ "$PHASE" != "Created" ]; then
    echo "Error: Download request failed with phase: $PHASE"
    echo "Details:"
    oc describe nadr "$REQUEST_NAME" -n "$NAMESPACE"
    exit 1
fi

# Get the download URL
log "Extracting download URL from NADR status"
DOWNLOAD_URL=$(oc get nadr "$REQUEST_NAME" -n "$NAMESPACE" -o jsonpath='{.status.velero.status.downloadURL}')

if [ -z "$DOWNLOAD_URL" ]; then
    echo "Error: No download URL available"
    oc describe nadr "$REQUEST_NAME" -n "$NAMESPACE"
    exit 1
fi

# Check URL expiration
EXPIRATION=$(oc get nadr "$REQUEST_NAME" -n "$NAMESPACE" -o jsonpath='{.status.velero.status.expiration}')
echo "Download URL expires at: $EXPIRATION"

# Generate output filename
OUTPUT_FILE="$OUTPUT_DIR/$(generate_filename "$KIND" "$NAME")"

# Download the file
echo "Downloading to: $OUTPUT_FILE"
log "Download URL: $DOWNLOAD_URL"

if command -v wget &> /dev/null; then
    wget "$DOWNLOAD_URL" -O "$OUTPUT_FILE" --progress=bar:force
elif command -v curl &> /dev/null; then
    curl -L "$DOWNLOAD_URL" -o "$OUTPUT_FILE" --progress-bar
else
    echo "Error: Neither wget nor curl is available"
    exit 1
fi

if [ $? -eq 0 ]; then
    echo "✓ Download completed successfully!"
    echo "  File: $OUTPUT_FILE"
    echo "  Size: $(du -h "$OUTPUT_FILE" | cut -f1)"
    
    # Offer to extract if it's a tar.gz file
    if [[ "$OUTPUT_FILE" == *.tar.gz ]]; then
        echo ""
        read -p "Extract the downloaded file? (y/N): " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            EXTRACT_DIR="${OUTPUT_FILE%.tar.gz}"
            mkdir -p "$EXTRACT_DIR"
            tar -xzf "$OUTPUT_FILE" -C "$EXTRACT_DIR"
            echo "✓ Extracted to: $EXTRACT_DIR"
        fi
    fi
    
    # Offer to cleanup the NADR
    echo ""
    read -p "Delete the download request '$REQUEST_NAME'? (y/N): " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        oc delete nadr "$REQUEST_NAME" -n "$NAMESPACE"
        echo "✓ Download request deleted"
    fi
    
else
    echo "✗ Download failed"
    exit 1
fi