# Developer Workflow: NonAdminBackup Phase and Conditions within NonAdminBackup Status

## Version

This is `ver_2` of the design document. More complex approach with more states is described in the [ver_1].

## Overview

This document outlines the design around updating NonAdminBackup objects Phase and Conditions within Status.

### Phase

A NonAdminBackup's `status` field has a `phase` field, which is updated by NAC controller.

The `phase` is a simple one high-level summary of the lifecycle of an NonAdminBackup.

It is always a one well defined value, that is intended to be a comprehensive state of a NonAdminBackup.

Those are are the possible values for phase:

| **Value** | **Description**                 |
|-----------|--------------------------------|
| New | *NonAdminBackup resource was accepted by the OpenShift cluster, but it has not yet been processed by the NonAdminController* |
| BackingOff | Velero *Backup* object was not created due to NonAdminBackup error (configuration or similar) |
| Created | Velero *Backup* was created. The Phase will not have additional informations about the |

### Conditions

The `conditions` is also a part of the NonAdminBackup's `status` field. One NAB object may have multiple conditions. It is more granular knowledge of the NonAdminBackup object and represents the array of the conditions through which the NonAdminBackup has or has not passed. Each `NonAdminCondition` has one of the following `type`:

| **Condition** | **Description**                 |
|-----------|--------------------------------|
| BackupAccepted | The Backup object was accepted by the reconcile loop, but the Velero Backup may have not yet been created |
| BackupQueued | The Velero Backup was created succesfully and did not return any known errors. It's in the queue for Backup. At this stage errors may still occur during backup procedure. |

The `condition` data is also accomapied with the following:

| **Field name** | **Description**                 |
|-----------|--------------------------------|
| type | The `Type` of the condition |
| status | represents the state of individual condition. The resulting `phase` value should report `Success` only when all of the conditions are met and the backup succeded. One of `True`, `False` or `Unknown`. |
| lastProbeTime | Timestamp of when the NonAdminBackup condition was last probed. |
| lastTransitionTime | Timestamp for when the NonAdminBackup last transitioned from one status to another. |
| reason | Machine-readable, UpperCamelCase text indicating the reason for the condition's last transition. |
| message | Human-readable message indicating details about the last status transition. |

### BackupStatus

`BackupStatus` which is also part of the `NonAdminBackupStatus` object is a `BackupStatus` that is taken directly from the Velero Backup Status and copied over.

### OadpVeleroBackupName
The `OadpVeleroBackupName` field is a component of the `NonAdminBackupStatus` object. It represents the name of the `VeleroBackup` object, which encompasses the namespace. This `OadpVeleroBackupName` serves as a reference to the Backup responsible for executing the backup task.

The format of the `OadpVeleroBackupName` allows to interact with that Backup using `oc` or `velero` commands as follows:

```shell
# <backup-name>.<namespace>

$ oc describe <backup-name>.<namespace>

$ velero backup describe <backup-name>.<namespace>
```


## Phase Update scenarios

    *** Questions ***

     - BackupQueued stays true at the end,
       should we remove that from conditions?
       I don't think we should set it to false.

```mermaid
graph
AA[Phase: New] -->A
A{BackupAccepted: Unknown\n BackupQueued: False\n}  -- NAC processes --> B[Non Admin Backup Accepted]
A -- NAC hits an error --> E[Phase: BackingOff]
B -- Create Velero Backup --> C[Phase: Created]
B -.-> BA{BackupAccepted: True\nBackupQueued: False\n}
C -.-> BS{BackupAccepted: True\nBackupQueued: True\n}
E -.-> EE{BackupAccepted: False\nBackupQueued: False\n}

classDef conditions fill:#ccc,stroke:#ccc,stroke-width:2px;
class A,BA,BS,EE conditions;
classDef phases fill:#777,stroke:#ccc,stroke-width:2px;
class AA,C,E phases;

```

[ver_1]: ./nab_status_update_ver_1.md
