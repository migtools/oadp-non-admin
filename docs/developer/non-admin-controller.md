# OADP NonAdminBackup Controller

The OADP NonAdminBackup Kubernetes controller manages the lifecycle of custom resources called `NonAdminBackup`. Let's break down the code and understand its functionality.

## Purpose

The primary goal of this controller is to allow users with limited permissions (non-admins) to trigger and manage Velero backups within their namespaces. It acts as an intermediary between the non-admin user and Velero, ensuring that backups are created and managed securely within the defined boundaries.

## Key Components

### NonAdminBackup Custom Resource

* Represents a backup request initiated by a non-admin user.
* Contains a `BackupSpec` field, which mirrors the Velero `BackupSpec` and defines what to back up.
* Tracks the status of the backup process through the `Status` field.

### NonAdminBackupReconciler

* The core controller responsible for processing `NonAdminBackup` resources.
* Watches for changes to `NonAdminBackup` objects and triggers reconciliation loops.

### Reconciliation Loop

* **Init:** Initializes the `NonAdminBackup` status if it's a new object.
* **ValidateSpec:** Ensures the provided `BackupSpec` is valid and adheres to security constraints.
    * Rejects backups targeting namespaces outside the user's allowed scope.
    * Sets appropriate conditions on the `NonAdminBackup` object to indicate success or failure.
* **UpdateSpecStatus:**
    * If the `VeleroBackup` doesn't exist, it creates one based on the validated `BackupSpec`.
    * If the `VeleroBackup` exists, it synchronizes the `NonAdminBackup` status with the `VeleroBackup` status.
    * Updates the `NonAdminBackup` object with relevant information from the `VeleroBackup`.

## Workflow

1. **User Creates NonAdminBackup:** A non-admin user creates a `NonAdminBackup` object, specifying the resources to back up within their namespace.
2. **Controller Detects Creation:** The `NonAdminBackupReconciler` detects the new object and triggers a reconciliation loop.
3. **Validation and Creation:** The controller validates the `BackupSpec` and creates a corresponding `VeleroBackup` object in the designated OADP namespace.
4. **Status Synchronization:** The controller continuously monitors the `VeleroBackup` and updates the `NonAdminBackup` status, reflecting the progress and completion state.

## Security Considerations

* **Namespace Isolation:** The controller ensures that non-admin users can only trigger backups within their own namespaces, preventing unauthorized access to resources in other namespaces.
* **Spec Validation:** The `ValidateSpec` function plays a crucial role in enforcing security policies. It verifies that the requested backup scope aligns with the user's permissions.

## Benefits

* **Self-Service Backups:** Empowers non-admin users to manage their backups without requiring elevated privileges.
* **Improved Security Posture:** Enforces strict access controls and prevents unauthorized backup operations.