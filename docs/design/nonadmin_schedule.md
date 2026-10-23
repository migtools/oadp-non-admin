# RFE-9751: NonAdminSchedule

## Summary

Add a namespaced `NonAdminSchedule` API that provides tenant self-service
scheduled backups. The NonAdmin controller validates the schedule template,
creates a backing Velero `Schedule` in the OADP namespace, and exposes every
generated Velero `Backup` as a synchronized `NonAdminBackup` in the tenant
namespace.

Velero remains responsible for cron execution, retaining each generated backup
according to its TTL, pause/resume, and skipping the next run.

## API

`NonAdminSchedule.spec` contains:

- `schedule`: standard Velero cron expression.
- `paused`: stops or resumes future executions.
- `skipImmediately`: skips the next scheduled execution. This is an
  edge-trigger and is reset after Velero accepts it.
- `template.backupSpec`: the complete existing Velero `BackupSpec` surface.

The controller owns backing-object labels, annotations, and owner references.
Tenant-controlled template metadata is intentionally not exposed because
Velero copies it to generated Backups and it could replace NonAdmin ownership
metadata.

The status exposes the backing Velero Schedule state, the next calculated run,
the latest generated backup, and the five newest generated child backups.

## Lifecycle

1. A tenant creates a `NonAdminSchedule` in its namespace.
2. NAC applies the same backup validation, namespace isolation, NABSL lookup,
   exclusions, and DPA backup-spec enforcement used for a `NonAdminBackup`.
3. NAC creates or updates one Velero `Schedule` in the OADP namespace.
4. Velero creates a Backup at each cron run.
5. NAC assigns per-run NonAdmin Backup ownership metadata and creates a
   synchronized child `NonAdminBackup` that refers to the existing Backup.
6. The existing NAB controller mirrors backup state and allows an existing
   `NonAdminRestore` to select any retained run by its child NAB name.

Deleting the schedule deletes the backing Velero Schedule and stops future
runs. It retains already-created child NABs and Velero Backups until their TTL
expires or a tenant deletes a child NAB.

## Validation

Implementation must cover template validation and enforcement, backing Velero
Schedule lifecycle, pause/resume, skip-next-run behavior, generated-backup to
NAB synchronization, status history, NAS deletion, cross-tenant isolation,
and duplicate prevention between the schedule and periodic synchronizers.

Cross-cluster non-admin restore and individual-file restore are not part of
this RFE.
