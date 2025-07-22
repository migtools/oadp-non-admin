# OADP NAC

Non Admin Controller

[![Continuous Integration](https://github.com/migtools/oadp-non-admin/actions/workflows/ci.yml/badge.svg)](https://github.com/migtools/oadp-non-admin/actions/workflows/ci.yml)

<!-- TODO add Official documentation link once it is created -->

Documentation in this repository are considered unofficial and for development purposes only.

## Description

This open source controller adds the non admin feature to [OADP operator](https://github.com/openshift/oadp-operator). With it, cluster admins can configure which namespaces non admin users can backup/restore.

## Getting Started

### Prerequisites
- oc
- Access to a OpenShift cluster
- [OADP operator](https://github.com/openshift/oadp-operator) version `1.5+` installed in the cluster

> **NOTE:** Before OADP operator version 1.5.0 is released, you need to [install OADP operator from source](docs/CONTRIBUTING.md#install-from-source) to use NAC.

### Using NAC

To use NAC functionality:
- **as admin user**:
    - create non admin user and its namespace, and apply required permissions to it (to create a non admin user to test NAC, you can check [non admin user documentation](docs/non_admin_user.md))
    - create/update DPA and configure non admin feature as needed, setting it to enabled
- **as non admin user**:
    - create sample application

        For example, use one of the sample applications available in `hack/samples/apps/` folder, by running
        ```sh
        oc process -f ./hack/samples/apps/<name> \
            -p NAMESPACE=<non-admin-user-namespace> \
            | oc create -f -
        ```

        Check the application was successful deployed by accessing its route.

        Create and update items in application UI, to later check if application was successfully restored.
    - create NonAdminBackup

        For example, use one of the sample NonAdminBackup available in `hack/samples/backups/` folder, by running
        ```sh
        oc process -f ./hack/samples/backups/<type> \
            -p NAMESPACE=<non-admin-user-namespace> \
            | oc create -f -
        ```
        <!-- TODO how to track status -->
     - delete sample application

        For example, delete one of the sample applications available in `hack/samples/apps/` folder, by running
        ```sh
        oc process -f ./hack/samples/apps/<name> \
            -p NAMESPACE=<non-admin-user-namespace> \
            | oc delete -f -
        ```

        Check that application was successful deleted by accessing its route.
    - create NonAdminRestore

        For example, use one of the sample NonAdminRestore available in `hack/samples/restores/` folder, by running
        ```sh
        oc process -f ./hack/samples/restores/<type> \
            -p NAMESPACE=<non-admin-user-namespace> \
            -p NAME=<NonAdminBackup-name> \
            | oc create -f -
        ```
        <!-- TODO how to track status -->

        After NonAdminRestore completes, check if the application was successful restored by accessing its route and seeing its items in application UI.

## Notes on Non Admin Permissions and Enforcements
### Cluster Administrator Enforceable Spec Fields
There are several types of cluster scoped objects that non-admin users should not have access to backup or restore.  OADP self-service automatically excludes the following list of cluster scoped resources from being backed up or restored.

* SCCs
* ClusterRoles
* ClusterRoleBindings
* CRDs
* PriorityClasses
* virtualmachineclusterinstancetypes
* virtualmachineclusterpreferences

Cluster administrators may also enforce company or compliance policy by utilizing templated NABSL's, NAB's and NAR's that require fields values to be set and conform to the administrator defined policy.  Admin Enforceable fields are fields that the cluster administrator can enforce non cluster admin users to use. Restricted fields are automatically managed by OADP and cannot be modified by either administrators or users.

#### NABSL
The following NABSL fields are currently supported for template enforcement:

| **NABSL Field**            | **Admin Enforceable** | **Restricted** | **special case** |
|----------------------------|-----------------|----------------|-----------------|
| `backupSyncPeriod`         |                 |                | ⚠️ Must be set lower than the DPA.backupSyncPeriod and lower than the garbage collection period |
| `provider`                 |                 |                |                 |
| `objectStorage`            | ✅ Yes          |                |                 |
| `credential`               | ✅ Yes          |                |                 |
| `config`                   | ✅ Yes          |                |                 |
| `accessMode`               | ✅ Yes          |                |                 |
| `validationFrequency`      | ✅ Yes          |                |                 |
| `default`                  |                 |                | ⚠️ Must be false or empty |

For example if the cluster administrator wanted to mandate that all NABSL's used a particular aws s3 bucket.

```
spec:
  config:
    checksumAlgorithm: ""
    profile: default
    region: us-west-2
  credential:
    key: cloud
    name: cloud-credentials
  objectStorage:
    bucket: my-company-bucket <---
    prefix: velero
  provider: aws
```
The DPA spec must be set in the following way: 

```
nonAdmin:
  enable: true
  enforceBSLSpec:
    config:             <--- entire config must match expected NaBSL config
      checksumAlgorithm: ""
      profile: default
      region: us-west-2
    objectStorage:  <--- all of the objectStorage options must match expected NaBSL options
      bucket: my-company-bucket
      prefix: velero 
    provider: aws
```

#### Restricted NonAdminBackups

In the same sense as the NABSL, cluster administrators can also restrict the NonAdminBackup spec fields to ensure the backup request conforms to the administrator defined policy.  Most of the backup spec fields can be restricted by the cluster administrator, below is a table of reference for the current implementation.


| **Backup Spec Field**                                  | **Admin Enforceable** | **Restricted** | **special case** |
|--------------------------------------------|--------------|--------------------------|-----------------|
| `csiSnapshotTimeout`                       | ✅ Yes       |                          |                 |
| `itemOperationTimeout`                     | ✅ Yes       |                          |                 |
| `resourcePolicy`                           | ✅ Yes       |                          | ⚠️ Non-admin users can specify the config-map that admins created in OADP Operator NS(Admins enforcing this value be a good alternative here), they cannot specify their own configmap as its lifecycle handling is not currently managed by NAC controller |
| `includedNamespaces`                       | ❌ No        | ✅ Yes                   | ⚠️ Admins cannot enforce this because it does not make sense for a cluster wide non-admin backup setting, we have validations in place such that only the NS admins NS in included in the NAB spec.                 |
| `excludedNamespaces`                       | ✅ Yes       | ✅ Yes                   | ⚠️ This spec is restricted for non-admin users and hence not enforceable by admins                |
| `includedResources`                        | ✅ Yes       |                          |                 |
| `excludedResources`                        | ✅ Yes       |                          |                 |
| `orderedResources`                         | ✅ Yes       |                          |                 |
| `includeClusterResources`                  | ✅ Yes       |                          | ⚠️ Non-admin users can only set this spec to false if they want, all other values are restricted, similar rule for admin enforcement regarding this spec value.  |
| `excludedClusterScopedResources`           | ✅ Yes       |                          |                 |
| `includedClusterScopedResources`           | ✅ Yes       |                          | ⚠️ This spec is restricted and non-enforceable, only empty list is acceptable  |
| `excludedNamespaceScopedResources`         | ✅ Yes       |                          |                 |
| `includedNamespaceScopedResources`         | ✅ Yes       |                          |                 |
| `labelSelector`                            | ✅ Yes       |                          |                 |
| `orLabelSelectors`                         | ✅ Yes       |                          |                 |
| `snapshotVolumes`                          | ✅ Yes       |                          |                 |
| `storageLocation`                          |              |                          | ⚠️ Can be empty (implying default BSL usage) or needs to be an existing NABSL |
| `volumeSnapshotLocations`                  |              |                          | ⚠️ Not supported for non-admin users, default will be used if needed |
| `ttl`                                      | ✅ Yes       |                          |                 |
| `defaultVolumesToFsBackup`                 | ✅ Yes       |                          |                 |
| `snapshotMoveData`                         | ✅ Yes       |                          |                 |
| `datamover`                                | ✅ Yes       |                          |                 |
| `uploaderConfig.parallelFilesUpload`       | ✅ Yes       |                          |                 |
| `hooks`                                    |              |                          |                 |

An example enforcement set in the DPA spec to enforce the 
  * ttl to be set to "158h0m0s"
  * snapshotMoveData to be set to true

```
  nonAdmin:
    enable: true
    enforcedBackupSpec.ttl: "158h0m0s"
    enforcedBackupSpec.snapshotMoveData: true
```

#### Restricted NonAdminRestore NAR

NonAdminRestores spec fields can also be restricted by the cluster administrator.  The following NAR spec fields are currently supported for template enforcement:

| **Field**                     | **Admin Enforceable** | **Restricted**    | **special case** |
|-------------------------------|--------------|--------------------|-----------------|
| `backupName`                  | ❌ No        |                    |                 |
| `scheduleName`                | ❌ No        | ✅ Yes             | ⚠️  not supported for non-admin users, we don't have non-admin backup schedule API as of now.  |
| `itemOperationTimeout`        | ✅ Yes       |                    |                 |
| `uploaderConfig`              | ✅ Yes       |                    |                 |
| `includedNamespaces`          | ❌ No        | ✅ Yes             | ⚠️ restricted for non-admin users and hence non-enforceable by admins  |
| `excludedNamespaces`          | ❌ No        | ✅ Yes             | ⚠️  restricted for non-admin users and hence non-enforceable by admins |
| `includedResources`           | ✅ Yes       |                    |                 |
| `excludedResources`           | ✅ Yes       |                |                 |
| `restoreStatus`               | ✅ Yes       |                |                 |
| `includeClusterResources`     | ✅ Yes       |                |                 |
| `labelSelector`               | ✅ Yes       |                |                 |
| `orLabelSelectors`            | ✅ Yes       |                |                 |
| `namespaceMapping`            | ❌ No        | ✅ Yes         | ⚠️ restricted for non-admin users and hence non-enforceable by admins                |
| `restorePVs`                  | ✅ Yes       |                |                 |
| `preserveNodePorts`           | ✅ Yes       |                |                 |
| `existingResourcePolicy`      |              |                |                 |
| `hooks`                       |              |                | ⚠️ special case |
| `resourceModifers`            |              |                | ⚠️ Non-admin users can specify the config-map that admins created in OADP Operator NS(Admins enforcing this value be a good alternative here), they cannot specify their own configmap as its lifecycle handling is not currently managed by NAC controller | 



## Contributing

Please check our [contributing documentation](docs/CONTRIBUTING.md) to propose changes to the repository.

## Architecture

For a better understanding of the project, check our [architecture documentation](docs/architecture.md) and [designs documentation](docs/design/).

## License

This repository is licensed under the terms of [Apache License Version 2.0](LICENSE).
