# Debug content Download URL Exposure Design

<!-- _Note_: The preferred style for design documents is one sentence per line.
*Do not wrap lines*.
This aids in review of the document as changes to a line are not obscured by the reflowing those changes caused and has a side effect of avoiding debate about one or two space after a period. -->

## Abstract

We want to provide a way for Non Admins (NAs) to debug their non admin backup (NAB) and/or non admin restore (NAR).

## Background

A backup/restore operation may not always succeed in part, or in full. Non Admin User would like a way to diagnose their backup or restore based on information available on object store uploaded by Velero during its backup or restore process.

## Goals

- Provide a path or url to debug information on NAB and NAR statuses, allowing credentialed object store users to access debug information.

## Non Goals

- Create a CLI
- Automatically retrieve debug information on behalf of the user

## High-Level Design

When a NAC velero backup or restore is completed, its associated NAB/NAR are updated to completion status.

During this time, NAC will also inject into status a path (ie. S3 url) to a relevant debugging file on the storage location that velero had uploaded.

## Detailed Design

[Currently](https://github.com/vmware-tanzu/velero/blob/2197cab3dbf1038b7afe363284d7b9e18c9300dc/pkg/apis/velero/v1/download_request_types.go#L32-L45) velero exposes following DownloadTargetKind that can be used in a DownloadRequest spec.

```go
 DownloadTargetKindBackupLog                       DownloadTargetKind = "BackupLog"
 DownloadTargetKindBackupContents                  DownloadTargetKind = "BackupContents"
 DownloadTargetKindBackupVolumeSnapshots           DownloadTargetKind = "BackupVolumeSnapshots"
 DownloadTargetKindBackupItemOperations            DownloadTargetKind = "BackupItemOperations"
 DownloadTargetKindBackupResourceList              DownloadTargetKind = "BackupResourceList"
 DownloadTargetKindBackupResults                   DownloadTargetKind = "BackupResults"
 DownloadTargetKindRestoreLog                      DownloadTargetKind = "RestoreLog"
 DownloadTargetKindRestoreResults                  DownloadTargetKind = "RestoreResults"
 DownloadTargetKindRestoreResourceList             DownloadTargetKind = "RestoreResourceList"
 DownloadTargetKindRestoreItemOperations           DownloadTargetKind = "RestoreItemOperations"
 DownloadTargetKindCSIBackupVolumeSnapshots        DownloadTargetKind = "CSIBackupVolumeSnapshots"
 DownloadTargetKindCSIBackupVolumeSnapshotContents DownloadTargetKind = "CSIBackupVolumeSnapshotContents"
 DownloadTargetKindBackupVolumeInfos               DownloadTargetKind = "BackupVolumeInfos"
 DownloadTargetKindRestoreVolumeInfo               DownloadTargetKind = "RestoreVolumeInfo"
```

These are the expected kinds of items we can get from object store for debugging.

The path to these items in an object store is defined in [pkg/persistence/object_store_layout.go](https://github.com/vmware-tanzu/velero/blob/0a280e57863c70495a2dfb65787615a68a0e7b03/pkg/persistence/object_store_layout.go) of velero repo.

We will use this knowledge to populate NAB/NAR status fields with path to log and resourceList.

In the future we may have a CLI that can gather resources available in DownloadTargetKind using the values added to NAB/NAR statuses from this enhancement.

### Usage examples

In a completed NAB, you see in its status.resourceListPath has a value of `s3://velero-6109f5e9711c8c58131acdd2f490f451/velero-e2e-00093375-ecd8-41d8-81fc-8ce9d4983ad6/backups/mongo-blockdevice-e2e-f26ab76c-34d1-11ef-93a5-0a580a834d36/mongo-blockdevice-e2e-f26ab76c-34d1-11ef-93a5-0a580a834d36-resource-list.json.gz`

Given a user have credentials to the object storage, they will be able to run following commands to get resource lists in the backup.
```
❯ aws s3 cp s3://velero-6109f5e9711c8c58131acdd2f490f451/velero-e2e-00093375-ecd8-41d8-81fc-8ce9d4983ad6/backups/mongo-blockdevice-e2e-f26ab76c-34d1-11ef-93a5-0a580a834d36/mongo-blockdevice-e2e-f26ab76c-34d1-11ef-93a5-0a580a834d36-resource-list.json.gz .

❯ gzip --uncompress --to-stdout mongo-blockdevice-e2e-f26ab76c-34d1-11ef-93a5-0a580a834d36-resource-list.json.gz | jq
{
  "apps.openshift.io/v1/DeploymentConfig": [
    "mongo-persistent/todolist"
  ],
  "apps/v1/Deployment": [
    "mongo-persistent/mongo"
  ],
  "apps/v1/ReplicaSet": [
    "mongo-persistent/mongo-7645dc59b7"
  ],
  "authorization.openshift.io/v1/RoleBinding": [
    "mongo-persistent/system:deployers",
    "mongo-persistent/system:image-builders",
    "mongo-persistent/system:image-pullers"
  ],
  "discovery.k8s.io/v1/EndpointSlice": [
    "mongo-persistent/mongo-hf89b",
    "mongo-persistent/todolist-frpft"
  ],
  "image.openshift.io/v1/ImageStream": [
    "mongo-persistent/todolist-mongo-go"
  ],
  "rbac.authorization.k8s.io/v1/RoleBinding": [
    "mongo-persistent/system:deployers",
    "mongo-persistent/system:image-builders",
    "mongo-persistent/system:image-pullers"
  ],
  "route.openshift.io/v1/Route": [
    "mongo-persistent/todolist-route"
  ],
  "security.openshift.io/v1/SecurityContextConstraints": [
    "mongo-persistent-scc"
  ],
  "v1/ConfigMap": [
    "mongo-persistent/kube-root-ca.crt",
    "mongo-persistent/openshift-service-ca.crt"
  ],
  "v1/Endpoints": [
    "mongo-persistent/mongo",
    "mongo-persistent/todolist"
  ],
  "v1/Event": [
    "mongo-persistent/mongo-7645dc59b7-ddnpr.17dcfbf31d2411b3",
    "mongo-persistent/mongo-7645dc59b7-ddnpr.17dcfbf45c7ee237",
    "mongo-persistent/mongo-7645dc59b7-ddnpr.17dcfbf4ecd8fb4f",
    "mongo-persistent/mongo-7645dc59b7-ddnpr.17dcfbf4fc16952d",
    "mongo-persistent/mongo-7645dc59b7-ddnpr.17dcfbf4fc16e3bc",
    "mongo-persistent/mongo-7645dc59b7-ddnpr.17dcfbf518b65eac",
    "mongo-persistent/mongo-7645dc59b7-ddnpr.17dcfbf51a288d26",
    "mongo-persistent/mongo-7645dc59b7-ddnpr.17dcfbf5215c6989",
    "mongo-persistent/mongo-7645dc59b7-ddnpr.17dcfbf526ddd2e3",
    "mongo-persistent/mongo-7645dc59b7-ddnpr.17dcfbf527b6e869",
    "mongo-persistent/mongo-7645dc59b7-ddnpr.17dcfbf5499768e5",
    "mongo-persistent/mongo-7645dc59b7-ddnpr.17dcfbf550e736c1",
    "mongo-persistent/mongo-7645dc59b7-ddnpr.17dcfbf556dbbde4",
    "mongo-persistent/mongo-7645dc59b7-ddnpr.17dcfbf557a59c91",
    "mongo-persistent/mongo-7645dc59b7.17dcfbf31d2357e0",
    "mongo-persistent/mongo.17dcfbf31911596c",
    "mongo-persistent/mongo.17dcfbf32101157f",
    "mongo-persistent/mongo.17dcfbf36dcc9869",
    "mongo-persistent/mongo.17dcfbf36dd6c3b0",
    "mongo-persistent/mongo.17dcfbf439418098",
    "mongo-persistent/todolist-1-deploy.17dcfbf32432b1f8",
    "mongo-persistent/todolist-1-deploy.17dcfbf35206b4ea",
    "mongo-persistent/todolist-1-deploy.17dcfbf353d7fdbf",
    "mongo-persistent/todolist-1-deploy.17dcfbf358a11021",
    "mongo-persistent/todolist-1-deploy.17dcfbf3598d5f3d",
    "mongo-persistent/todolist-1-wqhpb.17dcfbf35f204990",
    "mongo-persistent/todolist-1-wqhpb.17dcfbf38b531ba6",
    "mongo-persistent/todolist-1-wqhpb.17dcfbf38c971e09",
    "mongo-persistent/todolist-1-wqhpb.17dcfbf3919831c9",
    "mongo-persistent/todolist-1-wqhpb.17dcfbf3927a3b4e",
    "mongo-persistent/todolist-1-wqhpb.17dcfbfcfd3a2577",
    "mongo-persistent/todolist-1-wqhpb.17dcfbfd07869b18",
    "mongo-persistent/todolist-1-wqhpb.17dcfbfd0ca71c96",
    "mongo-persistent/todolist-1-wqhpb.17dcfbfd0da3efdc",
    "mongo-persistent/todolist-1.17dcfbf35ebded07",
    "mongo-persistent/todolist.17dcfbf320766931"
  ],
  "v1/Namespace": [
    "mongo-persistent"
  ],
  "v1/PersistentVolume": [
    "pvc-2c2ff5b8-3b06-467c-8b0e-99a40cde4993"
  ],
  "v1/PersistentVolumeClaim": [
    "mongo-persistent/mongo"
  ],
  "v1/Pod": [
    "mongo-persistent/mongo-7645dc59b7-ddnpr",
    "mongo-persistent/todolist-1-deploy",
    "mongo-persistent/todolist-1-wqhpb"
  ],
  "v1/ReplicationController": [
    "mongo-persistent/todolist-1"
  ],
  "v1/Secret": [
    "mongo-persistent/builder-dockercfg-h7p9c",
    "mongo-persistent/builder-token-l79l7",
    "mongo-persistent/default-dockercfg-nnjfj",
    "mongo-persistent/default-token-7d745",
    "mongo-persistent/deployer-dockercfg-dvrn8",
    "mongo-persistent/deployer-token-dl9m2",
    "mongo-persistent/mongo-persistent-sa-dockercfg-p9zfk",
    "mongo-persistent/mongo-persistent-sa-token-55qxx"
  ],
  "v1/Service": [
    "mongo-persistent/mongo",
    "mongo-persistent/todolist"
  ],
  "v1/ServiceAccount": [
    "mongo-persistent/builder",
    "mongo-persistent/default",
    "mongo-persistent/deployer",
    "mongo-persistent/mongo-persistent-sa"
  ]
}
```

## Alternatives Considered

Generating a short lived signed url

- While this approach do not require user to have bucket credentials, the URL retrieved will be short lived such that it will only be practical to use if programmatically used by a CLI, which is out of scope of current design.

## Security Considerations

We will assume that user who have object store credentials are the authorized personnel to access this data path and the object store bucket as a whole.
Other users who share this bucket are also assumed to be similarly privileged and have access to all other shared users data in the object store.

## Compatibility

A new status field(s) will need to be added to NAB/NAR to expose debug info path.
This includes updates to the CRD within oadp-operator project as well.

## Implementation
<!-- A description of the implementation, timelines, and any resources that have agreed to contribute. -->

## Open Issues
<!-- A discussion of issues relating to this proposal for which the author does not know the solution. This section may be omitted if there are none. -->
