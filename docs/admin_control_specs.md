# Admin users control over non admin object specs

## Background

Non Admin Controller (NAC) adds the ability to admin users restrict the use of OADP operator for non admin users, by only allowing them to create backup/restores from their namespaces with NonAdminBackup/NonAdminRestore. Admin users may want to further restrict non admin users operations, like forcing a specific NonAdminBackup type. This design enables admin users to restrict which NonAdminBackup/NonAdminRestore specs will be open for non admin users and set custom default values for these specs.

## Goals

- Enable admin users to restrict which NonAdminBackup/NonAdminRestore specs non admin users can use, and set custom default values to these specs

## Non-Goals

- Remove restricted specs or show their custom default values in NonAdminBackup/NonAdminRestore (non admin users can still create NonAdminBackup/NonAdminRestore with restricted specs, but they will be simply not reconciled by NAC, with an error explaining why)
- Allow admin users to restrict second level specs (for example, `labelSelector` can be restricted, but not `labelSelector.matchLabels`)

## Use-Cases

- Admin user wants to enforce that only file system backups are done

    This can be done by setting file system field as not editable to non admin users and setting it custom default value to true. Also, admin user could also make other backup type fields not editable

- Admin user wants to enforce that NonAdminBackup have time to live (TTL) of 2 days.

    This can be done by setting TTL field as not editable to non admin users and setting it custom default value to 2 days

## High-Level design

TODO choose one

- Admin user would create a ConfigMap in OADP operator namespace, specifying which specs are restrict to non admin users and which specs would have a custom default value.

- Admin user would configure which specs are restrict to non admin users and which specs would have a custom default value, in OADP operator DPA.

## Implementation details

TODO choose one

### ConfigMap

Admin user would create a ConfigMap in OADP operator namespace, called `nac-specs-control`. TODO explain more

### DPA

Admin user would configure which specs are restricted to non admin users and what are their custom default values, in OADP operator DPA. These specs would be in `spec.features.nonAdmin.specsControl` of DPA, and will be the same type as Velero objects.

## Open Questions and Know Limitations

TODO

ConfigMap
- no field name or type checking (worst user experience)
