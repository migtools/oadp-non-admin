# Admin users control over non admin objects' spec

## Abstract

Non Admin Controller (NAC) restricts the usage of OADP operator with NonAdminBackups, NonAdminRestores and NonAdminBackupStorageLocations.
Admin users may want to further restrict this by restricting relevant spec fields values.
## Background

The Non Admin Controller (NAC) restricts how non-admin users interact with the OADP operator. It allows them to create backups, restores, and backup storage locations only within their own namespaces using `NonAdminBackup`, `NonAdminRestore`, and `NonAdminBackupStorageLocation`, while enforcing additional restrictions and policies.

The NAC Controller enforces two types of controls: **restrictions** and **enforcements**.

- **Controller Restrictions** define policies that neither users nor cluster administrators can override.
- **Controller Enforcements** define policies that administrators can override if needed.

This design allows system administrators to manage **Controller Enforcements**, enabling them to set custom default values for the `NonAdminBackup`, `NonAdminRestore`, and `NonAdminBackupStorageLocation` spec fields, which non-admin users cannot override.

For example, administrators may want to enforce specific rules on non-admin user operations, such as:

- Enforcing a specific time to live (TTL) for Velero Backups associated with `NonAdminBackup`.
- Limiting the region that users can select within Velero `BackupStorageLocation`.
- Controlling which resource types can be restored in a `NonAdminRestore` request.

## Goals

Enable admin users to
- set custom enforced values for NonAdminBackup spec.backupSpec fields, which can not be overridden
- set custom enforced values for NonAdminRestore spec.restoreSpec fields, which can not be overridden
- set custom enforced values for NonAdminBackupStorageLocation spec.backupStorageLocationSpec fields, which can not be overridden

Also
- Show custom enforced values validation errors in NAC object statuses and in NAC logs
- Each enforced value is associated with a specific spec field. If a spec field is a map or a list, enforcing one key/value pair will not override the whole map/list
- Custom enforced values are overriding the Spec default values
- Non Admin users must define all fields that are within the enforced spec field

## Non Goals

- Prevent non admin users to create NonAdminBackup/NonAdminRestore/NonAdminBackupStorageLocation objects with overridden defaults. During reconciliation, NAC will check if the object has different values than enforced ones and will error if it does.
- Show NonAdminBackup spec.backupSpec fields/NonAdminRestore spec.restoreSpec fields/NonAdminBackupStorageLocation spec.backupStorageLocationSpec fields enforced values to the non admin users
- Check if there are on-going NAC operations prior to recreating NAC Pod
- Allow admin users to enforce falsy values (like empty maps or empty lists) for NonAdminBackup spec.backupSpec fields/NonAdminRestore spec.restoreSpec fields/NonAdminBackupStorageLocation spec.backupStorageLocationSpec fields

## High-Level Design

Additional fields will be added to the OADP DPA object:
- `spec.nonAdmin.enforceBackupSpec`
- `spec.nonAdmin.enforceRestoreSpec`
- `spec.nonAdmin.enforceBackupStorageLocationSpec`

These fields allow admin users to define custom enforced values for the `spec.backupSpec`, `spec.restoreSpec`, and `spec.backupStorageLocationSpec` fields within NonAdminBackup, NonAdminRestore, and NonAdminBackupStorageLocation objects, respectively. The Non Admin Controller (NAC) will ensure compliance with these values, and any NonAdmin object that attempts to override them will fail validation before the creation of the associated Velero object, whether it be a Backup, Restore, or BackupStorageLocation. Furthermore, if an admin user updates any enforced field value, the NAC Pod will be automatically recreated to reflect the latest administrative settings, ensuring that the system consistently enforces the specified policies and maintains operational integrity within the OADP operator.

> **Note:** if there are on-going NAC operations prior to recreating NAC Pod, reconcile progress might get lost for NAC objects.

## Detailed Design

Field `spec.nonAdmin.enforceBackupSpec`, of the same type as the Velero Backup Spec, will be added to OADP DPA object.

With it, admin users will be able to select which NonAdminBackup `spec.backupSpec` fields have custom enforced (new default) values.

To avoid mistakes, not all fields will be able to be enforced, like `IncludedNamespaces` which is within **Controller Restrictions** fields and admin users should not enforce it, which could break NAC usage.

NAC will respect the set values by reading DPA field during startup.If admin user changes any enforced field value, NAC Pod is recreated (and only NAC Pod) to always be up to date with admin user enforcements.

If a NonAdminBackup is created with fields overriding any enforced values, it will fail validation prior to creating an associated Velero Backup. Validation error is shown in NonAdminBackup status and NAC logs.

If NonAdminBackup respects enforcement, the created associated Velero Backup will have the enforced spec field values.

Enforcement is done dynamically. If new field is added to Velero Backup Spec, it will be presented to user without code changes. If a field changes type/or default value, tests will warn us.

Similar enforcement is done for NonAdminRestore and NonAdminBackupStorageLocation.

It is important to note that the enforcement of one field will not override the whole map/list.

### Example workflows

#### Admin user configures NAC with enforced spec fields

In this example, admin user has configured NAC with the following OADP DPA options
```yaml
...
spec:
  ...
  nonAdmin:
    enable: true
    enforceBackupSpecs:
      snapshotVolumes: false
```

That means, that the following NonAdminBackup will be accepted by NAC validation
```yaml
...
spec:
  backupSpec:
    snapshotVolumes: false
    ttl: 3h
```

> **Note:** the related Velero Backup for this NonAdminBackup will have `spec.snapshotVolumes` set to false, ttl set to 3h and other fields set to their default values.

But this NonAdminBackup will not be accepted by NAC validation
```yaml
...
spec:
  backupSpec:
    snapshotVolumes: true
```
NonAdminBackup status and NAC log will have the following message:
> spec.backupSpec.snapshotVolumes field value is enforced by admin user, can not override it

Also the following NonAdminBackup will be rejected, because user did not define all fields that are within the enforced spec field:
```yaml
...
spec:
  backupSpec:
    ttl: 3h
```

NonAdminBackup status and NAC log will have the following message:
> spec.backupSpec.ttl field value is enforced by admin user, can not override it

#### Admin user configures NAC with enforced spec map/list fields

In this example, admin user has configured NAC with the following OADP DPA options, from which only `config.region` and `objectStorage.prefix` are enforced and the `config` is a map
```yaml
...
spec:
  ...
  nonAdmin:
    enable: true
    enforceBackupStorageLocationSpec:
      config:
        region: eu-central-1
      objectStorage:
        prefix: non-admin-buckets
      provider: aws
```

That means, that the following NonAdminBackupStorageLocation will be accepted by NAC validation
```yaml
...
spec:
  backupStorageLocationSpec:
    config:
      serverSideEncryption: AES256
      region: eu-central-1
    objectStorage:
      prefix: non-admin-buckets
    provider: aws
```

The following NonAdminBackupStorageLocation will be rejected, because user did not define all fields that are within the enforced spec field:
```yaml
...
spec:
  backupStorageLocationSpec:
    config:
      serverSideEncryption: AES256
    objectStorage:
      prefix: non-admin-buckets
    provider: aws
```

NonAdminBackupStorageLocation status and NAC log will have the following message:
> spec.backupStorageLocationSpec.config.region field value is enforced by admin user, can not override it

## Alternatives Considered

Instead of using a DPA field, using a ConfigMap was considered. Since users would not have type assertion when creating those ConfigMap and parsing it would be harder in NAC side, it was discarded.

## Security Considerations

No security considerations.

## Compatibility

No compatibility issues.

## Implementation

Add `EnforceBackupSpec` struct to OADP DPA `NonAdmin` struct
```go
type NonAdmin struct {
	// which bakup spec field values to enforce
	// +optional
	EnforceBackupSpec *velero.BackupSpec `json:"enforceBackupSpec,omitempty"`
}
```

Add `EnforceRestoreSpec` struct to OADP DPA `NonAdmin` struct
```go
type NonAdmin struct {
	// which restore spec field values to enforce
	// +optional
	EnforceBackupSpec *velero.BackupSpec `json:"enforceBackupSpec,omitempty"`
}
```

Add `EnforceBackupStorageLocationSpec` struct to OADP DPA `NonAdmin` struct
```go
type NonAdmin struct {
	// which backupStorageLocation spec field values to enforce
	// +optional
	EnforceBackupStorageLocationSpec *velero.BackupStorageLocationSpec `json:"enforceBackupStorageLocationSpec,omitempty"`
}
```

Store previous `EnforceBackupSpec`, `EnforceRestoreSpec` and `EnforceBackupStorageLocationSpec` value, so when admin user changes it, Deployment is also changed to trigger a Pod recreation
```go
const (
	enforcedBackupSpecKey = "enforced-backup-spec"
    enforcedRestoreSpecKey = "enforced-restore-spec"
	enforcedBackupStorageLocationSpecKey = "enforced-backup-storage-location-spec"
)

var (
	previousEnforcedBackupSpec    *velero.BackupSpec  = nil
	dpaBackupSpecResourceVersion                      = ""
	previousEnforcedRestoreSpec   *velero.RestoreSpec = nil
	dpaRestoreSpecResourceVersion                     = ""
	previousEnforcedBackupStorageLocationSpec *velero.BackupStorageLocationSpec = nil
	dpaBackupStorageLocationSpecResourceVersion = ""
)

func ensureRequiredSpecs(deploymentObject *appsv1.Deployment, dpa *oadpv1alpha1.DataProtectionApplication, image string, imagePullPolicy corev1.PullPolicy) error {
	if len(dpaBackupSpecResourceVersion) == 0 || !reflect.DeepEqual(dpa.Spec.NonAdmin.EnforceBackupSpec, previousEnforcedBackupSpec) {
		dpaBackupSpecResourceVersion = dpa.GetResourceVersion()
	}
	previousEnforcedBackupSpec = dpa.Spec.NonAdmin.EnforceBackupSpec
	if len(dpaRestoreSpecResourceVersion) == 0 || !reflect.DeepEqual(dpa.Spec.NonAdmin.EnforceRestoreSpec, previousEnforcedRestoreSpec) {
		dpaRestoreSpecResourceVersion = dpa.GetResourceVersion()
	}
	previousEnforcedRestoreSpec = dpa.Spec.NonAdmin.EnforceRestoreSpec
	if len(dpaBackupStorageLocationSpecResourceVersion) == 0 || !reflect.DeepEqual(dpa.Spec.NonAdmin.EnforceBackupStorageLocationSpec, previousEnforcedBackupStorageLocationSpec) {
		dpaBackupStorageLocationSpecResourceVersion = dpa.GetResourceVersion()
	}
	previousEnforcedBackupStorageLocationSpec = dpa.Spec.NonAdmin.EnforceBackupStorageLocationSpec
	enforcedSpecAnnotation := map[string]string{
		enforcedBackupSpecKey: dpaBackupSpecResourceVersion,
        enforcedRestoreSpecKey: dpaRestoreSpecResourceVersion,
		enforcedBackupStorageLocationSpecKey: dpaBackupStorageLocationSpecResourceVersion,
	}

	templateObjectAnnotations := deploymentObject.Spec.Template.GetAnnotations()
	if templateObjectAnnotations == nil {
		deploymentObject.Spec.Template.SetAnnotations(enforcedSpecAnnotation)
	} else {
		templateObjectAnnotations[enforcedBackupSpecKey] = enforcedSpecAnnotation[enforcedBackupSpecKey]
		templateObjectAnnotations[enforcedRestoreSpecKey] = enforcedSpecAnnotation[enforcedRestoreSpecKey]
		templateObjectAnnotations[enforcedBackupStorageLocationSpecKey] = enforcedSpecAnnotation[enforcedBackupStorageLocationSpecKey]
		deploymentObject.Spec.Template.SetAnnotations(templateObjectAnnotations)
	}
}
```

During NAC startup, read OADP DPA, to be able to apply admin user enforcement
```go
	restConfig := ctrl.GetConfigOrDie()

	dpaClientScheme := runtime.NewScheme()
	utilruntime.Must(v1alpha1.AddToScheme(dpaClientScheme))
	dpaClient, err := client.New(restConfig, client.Options{
		Scheme: dpaClientScheme,
	})
	if err != nil {
		setupLog.Error(err, "unable to create Kubernetes client")
		os.Exit(1)
	}
	dpaList := &v1alpha1.DataProtectionApplicationList{}
	err = dpaClient.List(context.Background(), dpaList)
	if err != nil {
		setupLog.Error(err, "unable to list DPAs")
		os.Exit(1)
	}
```

Implement `EnforceNacSpec` function to validate NonAdminBackup/NonAdminRestore/NonAdminBackupStorageLocation spec from Validation functions:
```go

func EnforceNacSpec(currentSpec, enforcedSpec any) error {
	// Implementation not part of the design
}

// Validate NonAdminBackup/NonAdminRestore/NonAdminBackupStorageLocation spec from ValidateBackupSpec/ValidateRestoreSpec/ValidateBackupStorageLocationSpec functions. Example on the RestoreSpec validation:
err = EnforceNacSpec(nonAdminRestore.Spec.RestoreSpec, enforcedRestoreSpec)
if err != nil {
	return err
}

return nil
```

For more details, check [Issue 151](https://github.com/migtools/oadp-non-admin/issues/151) and [PR 1584](https://github.com/openshift/oadp-operator/pull/1584), [PR 110](https://github.com/migtools/oadp-non-admin/pull/110), [PR 1600](https://github.com/openshift/oadp-operator/pull/1600) and [PR 122](https://github.com/migtools/oadp-non-admin/pull/122).

## Open Issues

- Show NonAdminBackup spec.backupSpec fields/NonAdminRestore spec.restoreSpec fields custom default values to non admin users https://github.com/migtools/oadp-non-admin/issues/111

