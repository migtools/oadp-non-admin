# Admin users control over non admin objects' spec

## Abstract

Non Admin Controller (NAC) restricts the usage of OADP operator with NonAdminBackups and NonAdminRestores.
Admin users may want to further restrict this by restricting NonAdminBackup/NonAdminRestore spec fields values.

## Background

Non Admin Controller (NAC) adds the ability to admin users restrict the use of OADP operator for non admin users, by only allowing them to create backup/restores from their namespaces with NonAdminBackups/NonAdminRestores.
Admin users may want to further restrict non admin users operations, like forcing a specific time to live (TTL) for NonAdminBackups associated Velero Backups.
This design enables admin users to set custom default values for NonAdminBackup/NonAdminRestore spec fields, which can not be overridden by non-admin users.

## Goals

Enable admin users to
- set custom default values for NonAdminBackup spec.backupSpec fields, which can not be overridden
- set custom default values for NonAdminRestore spec.restoreSpec fields, which can not be overridden

Also
- Show custom default values validation errors in NAC object statuses and in NAC logs

## Non Goals

- Show NonAdminBackup spec.backupSpec fields/NonAdminRestore spec.restoreSpec fields custom default values to non admin users
- Prevent non admin users to create NonAdminBackup/NonAdminRestore with overridden defaults
- Allow admin users to set second level defaults (for example, NonAdminBackup `spec.backupSpec.labelSelector` can have a custom default value, but not just `spec.backupSpec.labelSelector.matchLabels`)
- Check if there are on-going NAC operations prior to recreating NAC Pod

## High-Level Design

A field will be added to OADP DPA object. With it, admin users will be able to select which NonAdminBackup `spec.backupSpec` fields have custom default (and enforced) values. NAC will respect the set values. If a NonAdminBackup is created with fields overriding any enforced values, it will fail validation prior to creating an associated Velero Backup.

Another field will be added to OADP DPA object. With it, admin users will be able to select which NonAdminRestore `spec.restoreSpec` fields have custom default (and enforced) values. NAC will respect the set values. If a NonAdminRestore is created with fields overriding any enforced values, it will fail validation prior to creating an associated Velero Restore.

If admin user changes any enforced field value, NAC Pod is recreated to always be up to date with admin user enforcements.

> **Note:** if there are on-going NAC operations prior to recreating NAC Pod, reconcile progress might get lost for NAC objects.

## Detailed Design

Field `spec.nonAdmin.enforceBackupSpec`, of the same type as the Velero Backup Spec, will be added to OADP DPA object.

With it, admin users will be able to select which NonAdminBackup `spec.backupSpec` fields have custom default (and enforced) values.

To avoid mistakes, not all fields will be able to be enforced, like `IncludedNamespaces`, that could break NAC usage.

NAC will respect the set values by reading DPA field during startup.If admin user changes any enforced field value, NAC Pod is recreated (and only NAC Pod) to always be up to date with admin user enforcements.

If a NonAdminBackup is created with fields overriding any enforced values, it will fail validation prior to creating an associated Velero Backup. Validation error is shown in NonAdminBackup status and NAC logs.

If NonAdminBackup respects enforcement, the created associated Velero Backup will have the enforced spec field values.

Enforcement is done dynamically. If new field is added to Velero Backup Spec, it will be presented to user without code changes. If a field changes type/or default value, tests will warn us.

### Example workflows

In this example, admin user has configured NAC with the following OADP DPA options
```yaml
...
spec:
  ...
  nonAdmin:
    enable: true
    enforceBackupSpecs:
      snapshotVolumes: false
  unsupportedOverrides:
    tech-preview-ack: 'true'
```

That means, that the 2 following NonAdminBackup will be accepted by NAC validation
```yaml
...
spec:
  backupSpec:
    snapshotVolumes: false
```

```yaml
...
spec:
  backupSpec:
    ttl: 3h
```
> **Note:** the related Velero Backup for this NonAdminBackup will have `spec.snapshotVolumes` set to false

But this NonAdminBackup will not be accepted by NAC validation
```yaml
...
spec:
  backupSpec:
    snapshotVolumes: true
```
NonAdminBackup status and NAC log will have the following message:
> spec.backupSpec.snapshotVolumes field value is enforced by admin user, can not override it

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

Store previous `EnforceBackupSpec` and `EnforceRestoreSpec` value, so when admin user changes it, Deployment is also changed to trigger a Pod recreation
```go
const (
	enforcedBackupSpecKey = "enforced-backup-spec"
    enforcedRestoreSpecKey = "enforced-restore-spec"
)

var (
	previousEnforcedBackupSpec    *velero.BackupSpec  = nil
	dpaBackupSpecResourceVersion                      = ""
	previousEnforcedRestoreSpec   *velero.RestoreSpec = nil
	dpaRestoreSpecResourceVersion                     = ""
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
	enforcedSpecAnnotation := map[string]string{
		enforcedBackupSpecKey: dpaBackupSpecResourceVersion,
        enforcedRestoreSpecKey: dpaRestoreSpecResourceVersion,
	}

	templateObjectAnnotations := deploymentObject.Spec.Template.GetAnnotations()
	if templateObjectAnnotations == nil {
		deploymentObject.Spec.Template.SetAnnotations(enforcedSpecAnnotation)
	} else {
		templateObjectAnnotations[enforcedBackupSpecKey] = enforcedSpecAnnotation[enforcedBackupSpecKey]
		templateObjectAnnotations[enforcedRestoreSpecKey] = enforcedSpecAnnotation[enforcedRestoreSpecKey]
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

Modify ValidateBackupSpec function to use `EnforceBackupSpec` and apply that to non admin users' NonAdminBackup request
```go
func ValidateBackupSpec(nonAdminBackup *nacv1alpha1.NonAdminBackup, enforcedBackupSpec *velerov1.BackupSpec) error {
	enforcedSpec := reflect.ValueOf(enforcedBackupSpec).Elem()
	for index := range enforcedSpec.NumField() {
		enforcedField := enforcedSpec.Field(index)
		enforcedFieldName := enforcedSpec.Type().Field(index).Name
		currentField := reflect.ValueOf(nonAdminBackup.Spec.BackupSpec).Elem().FieldByName(enforcedFieldName)
		if !enforcedField.IsZero() && !currentField.IsZero() && !reflect.DeepEqual(enforcedField.Interface(), currentField.Interface()) {
			field, _ := reflect.TypeOf(nonAdminBackup.Spec.BackupSpec).Elem().FieldByName(enforcedFieldName)
			tagName, _, _ := strings.Cut(field.Tag.Get("json"), ",")
			return fmt.Errorf(
				"NonAdminBackup spec.backupSpec.%v field value is enforced by admin user, can not override it",
				tagName,
			)
		}
	}
}
```

Before creating NonAdminBackup's related Velero Backup, apply any missing fields to it that admin user has enforced
```go
		enforcedSpec := reflect.ValueOf(r.EnforcedBackupSpec).Elem()
		for index := range enforcedSpec.NumField() {
			enforcedField := enforcedSpec.Field(index)
			enforcedFieldName := enforcedSpec.Type().Field(index).Name
			currentField := reflect.ValueOf(backupSpec).Elem().FieldByName(enforcedFieldName)
			if !enforcedField.IsZero() && currentField.IsZero() {
				currentField.Set(enforcedField)
			}
		}
```

Modify ValidateRestoreSpec function to use `EnforceRestoreSpec` and apply that to non admin users' NonAdminBackup request
```go
	enforcedSpec := reflect.ValueOf(enforcedRestoreSpec).Elem()
	for index := range enforcedSpec.NumField() {
		enforcedField := enforcedSpec.Field(index)
		enforcedFieldName := enforcedSpec.Type().Field(index).Name
		currentField := reflect.ValueOf(nonAdminRestore.Spec.RestoreSpec).Elem().FieldByName(enforcedFieldName)
		if !enforcedField.IsZero() && !currentField.IsZero() && !reflect.DeepEqual(enforcedField.Interface(), currentField.Interface()) {
			field, _ := reflect.TypeOf(nonAdminRestore.Spec.RestoreSpec).Elem().FieldByName(enforcedFieldName)
			tagName, _, _ := strings.Cut(field.Tag.Get("json"), ",")
			return fmt.Errorf(
				"NonAdminRestore spec.restoreSpec.%v field value is enforced by admin user, can not override it",
				tagName,
			)
		}
	}
```

Before creating NonAdminRestore's related Velero Restore, apply any missing fields to it that admin user has enforced
```go
		enforcedSpec := reflect.ValueOf(r.EnforcedRestoreSpec).Elem()
		for index := range enforcedSpec.NumField() {
			enforcedField := enforcedSpec.Field(index)
			enforcedFieldName := enforcedSpec.Type().Field(index).Name
			currentField := reflect.ValueOf(restoreSpec).Elem().FieldByName(enforcedFieldName)
			if !enforcedField.IsZero() && currentField.IsZero() {
				currentField.Set(enforcedField)
			}
		}
```

For more details, check https://github.com/openshift/oadp-operator/pull/1584, https://github.com/migtools/oadp-non-admin/pull/110, https://github.com/openshift/oadp-operator/pull/1600 and .

## Open Issues

- Show NonAdminBackup spec.backupSpec fields/NonAdminRestore spec.restoreSpec fields custom default values to non admin users https://github.com/migtools/oadp-non-admin/issues/111

