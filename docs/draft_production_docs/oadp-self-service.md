# OADP Self Service Documentation

## Overview

OADP Self Service enables non-administrator users to perform backup and restore operations in their authorized namespaces without requiring cluster-wide administrator privileges. This feature provides secure self-service data protection capabilities while maintaining proper access controls.

### Key Benefits

- Allows namespace-scoped backup and restore operations
- Provides secure access to backup logs and status information
- Enables users to create dedicated backup storage locations
- Maintains cluster administrator control over non-administrator operations through templates and policies

### Components

The self-service functionality is implemented through several custom resources:

- NonAdminBackup (NAB) - Manages namespace-scoped backup operations
- NonAdminRestore (NAR) - Handles namespace-scoped restore operations  
- NonAdminBackupStorageLocation (NABSL) - Defines user-specific backup storage locations
- NonAdminController (NAC) - Controls and orchestrates the self-service operations

## OADP Self Service

Previously, the OADP (OpenShift API for Data Protection) Operator required cluster administrator privileges to perform any backup and restore operations. With OADP self-service, regular users can now safely perform backup and restore operations within namespaces where they have appropriate permissions.
OADP self-service enables regular OpenShift users to perform backup and restore operations within their authorized namespaces. This is achieved through custom resources that securely manage these operations while maintaining proper access controls and visibility. Users can:

- Create and manage backups of their authorized namespaces
- Restore data to their authorized namespaces 
- Monitor backup and restore status
- Access relevant logs and descriptions
- Configure their own backup storage locations

The self-service functionality is implemented in a way that ensures users can only operate within their assigned namespaces and permissions, while cluster administrators maintain overall control through templates and policies.
 
## Glossary of terms
* NAB - Non Admin Backup
* NAR - Non Admin Restore
* NAC - Non Admin Controller
* NABSL - Non Admin Backup Storage Location
* NADR - Non Admin Download Request


## Cluster Administrator Setup

Install and configure the OADP operator according to the documentation and your requirements.

To enable OADP Self-Service the DPA spec must the spec.nonAdmin.enable field to true.

```
  nonAdmin:
    enable: true
```

Once the OADP DPA is reconciled the cluster administrator should see the non-admin-controller running in the openshift-adp namespace.  The Openshift users without cluster admin rights will now be able to create NAB or NAR objects in their namespace to create a backup or restore.

## OpenShift User Instructions

Prior to OpenShift users taking advantage of OADP self-service feature the OpenShift cluster administrator must have completed the following prerequisite steps:

* The OADP DPA has been configured to support self-service
* The cluster administrator has created the users 
  * account 
  * namespace
  * namespace privileges, e.g. namespace admin.

Non Cluster Administrators can utilize OADP self-service by creating NonAdminBackup (NAB) and NonAdminRestore (NAR) objects in the namespace to be backed up or restored.  A NonAdminBackup is an OpenShift custom resource that securely facilitates the creation, status and lifecycle of a Velero Backup custom resource.  

```mermaid
sequenceDiagram
    participant User
    participant NAB as NonAdminBackup
    participant NAC as NonAdminController
    participant VB as Velero Backup

    User->>NAB: Creates NonAdminBackup CR
    NAB->>NAC: Detected by controller
    NAC-->>NAB: Validates backup request
    NAC->>VB: Creates Velero Backup CR
    VB-->>NAB: Status updates
    NAB-->>User: View backup status
```

![nab-backup-workflow](https://hackmd.io/_uploads/BJz4bEbKyx.jpg)

For the most part one can think of a NonAdminBackup and a Velero Backup in very much the same way.  Both objects specify a velero backup and how the backup should be executed.  There are a few differences to keep in mind when creating a NonAdminBackup.

1. The NonAdminBackup creates the Velero Backup CR instance in a secure way that limits the users access.
2. A user cannot specify the namespace that will be backed up.  The namespace from which the NAB object is created is the defined namespace to be backed up.
3. In addition to the creation of the Velero Backup the NonAdminBackup object's main purpose is to track the status of the Velero Backup in a secure and clear way.


### NAB / NAR Status

#### Phase
The phase field is a simple one high-level summary of the lifecycle of the objects, that only moves forward. Once a phase changes, it can not return to the previous value.

| **Value** | **Description** |
|-----------|-----------------|
| New | *NonAdminBackup/NonAdminRestore* resource was accepted by the NAB/NAR Controller, but it has not yet been validated by the NAB/NAR Controller |
| BackingOff | *NonAdminBackup/NonAdminRestore* resource was invalidated by the NAB/NAR Controller, due to invalid Spec. NAB/NAR Controller will not reconcile the object further, until user updates it |
| Created | *NonAdminBackup/NonAdminRestore* resource was validated by the NAB/NAR Controller and Velero *Backup/restore* was created. The Phase will not have additional information about the *Backup/Restore* run |
| Deletion | *NonAdminBackup/NonAdminRestore* resource has been marked for deletion. The NAB/NAR Controller will delete the corresponding Velero *Backup/Restore* if it exists. Once this deletion completes, the *NonAdminBackup/NonAdminRestore* object itself will also be removed |


## Advanced Cluster Administrator Features

### Restricted NonAdminBackupStorageLocation 

Cluster administrators can gain efficiencies by delegating backup and restore operations to OpenShift users. It is recommended that cluster administrators carefully manage the NABSL to conform to any company policies, compliance requirements, etc.  There are two ways cluster administrators can manage the NABSL's.

Cluster administrators can optionally set an approval policy for any NABSL.  This policy will require that any NABSL be approved by the cluster administrator before it can be used.

```
  nonAdmin:
    enable: true
    requireAdminApprovalForBSL: true
```

```
apiVersion: oadp.openshift.io/v1alpha1
kind: NABSLApprovalRequest
metadata:
  name: nabsl-hash-name
  namespace: openshift-adp<Operator NS, this is the key here>
spec:
  nabslName: nabsl-name
  nabslNamespace: nac-user-ns
  creationApproved: false  # Tracks approval for creation
  updateApproved: false    # Tracks approval for updates
  lastApprovedSpec: {}  # Stores last approved NABSL spec
```
  This ensures the cluster administrator has reviewed the NABSL to ensure the correct object storage location options are used.

### Cluster Administrator Enforceable Spec Fields

Cluster administrators may also enforce templated NABSL's, NAB's and NAR's that require fields values to be set and conform to the administrator defined policy.  Admin Enforceable fields are fields that the cluster administrator can enforce non cluster admin users to use. Restricted fields are automatically managed by OADP and cannot be modified by either administrators or users.

#### NABSL
The following NABSL fields are currently supported for template enforcement:

| **NABSL Field**            | **Admin Enforceable** | **Restricted** |
|----------------------------|-----------------|----------------|
| `backupSyncPeriod`         | ⚠️ special case |                |
| `provider`                 | ⚠️ special case |                |
| `objectStorage`            | ✅ Yes          |                |
| `credential`               | ✅ Yes          |                |
| `config`                   | ✅ Yes          |                |
| `accessMode`               | ✅ Yes          |                |
| `validationFrequency`      | ✅ Yes          |                |


TODO need example NABSL enforcement

#### Restricted NonAdminBackups

In the same sense as the NABSL, cluster administrators can also restrict the NonAdminBackup spec fields to ensure the backup request conforms to the administrator defined policy.  Most of the backup spec fields can be restricted by the cluster administrator, below is a table of reference for the current implementation.


| **Backup Spec Field**                                  | **Admin Enforceable** | **Restricted** |
|--------------------------------------------|--------------|--------------------------|
| `csiSnapshotTimeout`                       | ✅ Yes       |                          |
| `itemOperationTimeout`                     | ✅ Yes       |                          |
| `resourcePolicy`                           | ✅ Yes       | ⚠️ special case           |
| `includedNamespaces`                       | ❌ No        | ✅ Yes                   |
| `excludedNamespaces`                       | ✅ Yes       |        ✅ Yes             |
| `includedResources`                        | ✅ Yes       |                          |
| `excludedResources`                        | ✅ Yes       |                          |
| `orderedResources`                         | ✅ Yes       |                          |
| `includeClusterResources`                  | ✅ Yes       |             ⚠️ special case               |
| `excludedClusterScopedResources`           | ✅ Yes       |                          |
| `includedClusterScopedResources`           | ✅ Yes       |        ⚠️ special case                    |
| `excludedNamespaceScopedResources`         | ✅ Yes       |                          |
| `includedNamespaceScopedResources`         | ✅ Yes       |                          |
| `labelSelector`                            | ✅ Yes       |                          |
| `orLabelSelectors`                         | ✅ Yes       |                          |
| `snapshotVolumes`                          | ✅ Yes       |                          |
| `storageLocation`                          | ⚠️ special case |                          |
| `volumeSnapshotLocations`                  | ⚠️ special case |                          |
| `ttl`                                      | ✅ Yes       |                          |
| `defaultVolumesToFsBackup`                 | ✅ Yes       |                          |
| `snapshotMoveData`                         | ✅ Yes       |                          |
| `datamover`                                | ✅ Yes       |                          |
| `uploaderConfig.parallelFilesUpload`       | ✅ Yes       |                          |
| `hooks`                                    | ⚠️ special case |                          |



#### Restricted NonAdminRestore NAR

NonAdminRestores spec fields can also be restricted by the cluster administrator.  The following NAR spec fields are currently supported for template enforcement:

| **Field**                     | **Admin Enforceable** | **Restricted**    |
|-------------------------------|--------------|--------------------|
| `backupName`                  | ❌ No        |                    |
| `scheduleName`                | ❌ No        | ✅ Yes         |
| `itemOperationTimeout`        | ✅ Yes       |                |
| `uploaderConfig`              | ✅ Yes       |                |
| `includedNamespaces`          | ❌ No        | ✅ Yes         |
| `excludedNamespaces`          | ❌ No        | ✅ Yes         |
| `includedResources`           | ✅ Yes       |                |
| `excludedResources`           | ✅ Yes       |                |
| `restoreStatus`               | ✅ Yes       |                |
| `includeClusterResources`     | ✅ Yes       |                |
| `labelSelector`               | ✅ Yes       |                |
| `orLabelSelectors`            | ✅ Yes       |                |
| `namespaceMapping`            | ❌ No        | ✅ Yes         |
| `restorePVs`                  | ✅ Yes       |                |
| `preserveNodePorts`           | ✅ Yes       |                |
| `existingResourcePolicy`      | ✅ Yes       |                |
| `resourceModifier`            | ⚠️ special case |                |
| `hooks`                       | ⚠️ special case |                |



## TODO
* add a section that describes which backup spec fields can be restricted by the cluster administrator https://github.com/migtools/oadp-non-admin/issues/151
* Document limited non-admin console use - via 
  * administrator -> Home -> API Explorer -> Filter on NonAdminBackup or NonAdminRestore -> Instances -> Create NonAdminBackup or NonAdminRestore
  