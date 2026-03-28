# NonAdminBSL Credential Requirements: Long-Term Credentials Only

_Jira: [OADP-7660](https://redhat.atlassian.net/browse/OADP-7660)_

## Overview

NonAdminBackupStorageLocation (NonAdminBSL) requires non-admin users to provide long-term cloud credentials (access keys, service account JSON keys, client secrets) stored in a Secret in their own namespace.
Short-lived, cloud-native credential mechanisms (AWS STS, GCP Workload Identity Federation, Azure Workload Identity) are **not supported** for NonAdminBSL and **must not be used**.

## How NonAdminBSL Credentials Work Today

The non-admin user creates a Secret in their namespace containing cloud credentials.
The [NonAdminBSL controller](https://github.com/migtools/oadp-non-admin/blob/31a3bfc20310898fae5a438005d0d242240d0fbb/internal/controller/nonadminbackupstoragelocation_controller.go#L736-L841) copies this Secret into the OADP namespace and references the copy in the Velero BSL's `spec.credential` field.
Velero's [`FileStore.Path()`](https://github.com/vmware-tanzu/velero/blob/6e91e72e655568dd6944ca7bb3cf00b6c7fbb3c8/internal/credentials/file_store.go#L66-L88) writes the secret data to a temporary file on disk and passes the file path to the cloud plugin via `config["credentialsFile"]`.

## Why Short-Lived Credentials Do Not Work with NonAdminBSL

### 1. Token File Path Is Not Accessible

Short-lived credential files reference a projected service account token file on the Velero pod's filesystem:

- **AWS STS:** `web_identity_token_file = /var/run/secrets/openshift/serviceaccount/token`
- **GCP WIF:** `"credential_source": {"file": "/var/run/secrets/openshift/serviceaccount/token"}`
- **Azure WI:** `AZURE_FEDERATED_TOKEN_FILE=/var/run/secrets/openshift/serviceaccount/token`

This projected token is bound to the `velero` service account in the OADP namespace.
It is only available inside the Velero pod -- it does not exist in the non-admin user's namespace.

If a non-admin user creates a credential Secret containing one of these formats, the token file path reference is correct only because it points to the Velero pod's existing projected volume.
The credential would use the **admin-provisioned Velero identity** to authenticate to the cloud provider, not a user-scoped identity.

### 2. No Per-Namespace Cloud Identity Scoping

The admin-level STS/WIF/WI credential is configured with a single cloud identity (IAM role, GCP service account, or Azure managed identity) that has broad permissions -- typically access to the entire backup bucket.

If a non-admin user's BSL used this credential:
- **All non-admin users would share the same cloud identity** with the same permissions.
- A non-admin user could access or overwrite **any other user's backup data** within the bucket, limited only by their choice of `objectStorage.prefix`.
- There is no cloud-side enforcement of per-namespace isolation.

### 3. Credential File Copied Without Validation

The NonAdminBSL controller's [`syncSecrets`](https://github.com/migtools/oadp-non-admin/blob/31a3bfc20310898fae5a438005d0d242240d0fbb/internal/controller/nonadminbackupstoragelocation_controller.go#L736-L841) function copies the user's Secret data verbatim:

```go
for k, v := range sourceNaBSLSecret.Data {
    veleroBslSecret.Data[k] = v
}
```

The controller does not inspect or validate the credential format.
If a user provides a WIF/STS/WI credential file, the controller copies it without recognizing that it references the Velero pod's projected token -- effectively granting the user access via the admin's cloud identity.

### 4. Azure Workload Identity Is Broken for Per-BSL Use

Even if per-namespace cloud identities were provisioned, Azure Workload Identity credentials do not work on a per-BSL basis in current Velero.
The [`NewCredential()`](https://github.com/vmware-tanzu/velero/blob/6e91e72e655568dd6944ca7bb3cf00b6c7fbb3c8/pkg/util/azure/credential.go#L48-L54) function reads `AZURE_FEDERATED_TOKEN_FILE`, `AZURE_CLIENT_ID`, and `AZURE_TENANT_ID` exclusively from **environment variables**, ignoring the per-BSL `creds` map entirely.
All Azure BSLs share the same pod-level identity regardless of what is in their credential file.

See upstream issue: [vmware-tanzu/velero#9657](https://github.com/vmware-tanzu/velero/issues/9657)

## Required Credential Formats

Non-admin users must provide **long-term cloud credentials** in a Secret in their namespace.

### AWS

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: my-backup-credentials
  namespace: <user-namespace>
type: Opaque
stringData:
  cloud: |
    [default]
    aws_access_key_id = <ACCESS_KEY_ID>
    aws_secret_access_key = <SECRET_ACCESS_KEY>
```

### GCP

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: my-backup-credentials
  namespace: <user-namespace>
type: Opaque
stringData:
  cloud: |
    {
      "type": "service_account",
      "project_id": "<project-id>",
      "private_key_id": "<key-id>",
      "private_key": "<private-key>",
      "client_email": "<sa-email>",
      "client_id": "<client-id>",
      "auth_uri": "https://accounts.google.com/o/oauth2/auth",
      "token_uri": "https://oauth2.googleapis.com/token"
    }
```

### Azure

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: my-backup-credentials
  namespace: <user-namespace>
type: Opaque
stringData:
  cloud: |
    AZURE_SUBSCRIPTION_ID=<subscription-id>
    AZURE_TENANT_ID=<tenant-id>
    AZURE_CLIENT_ID=<client-id>
    AZURE_CLIENT_SECRET=<client-secret>
    AZURE_RESOURCE_GROUP=<resource-group>
    AZURE_CLOUD_NAME=AzurePublicCloud
```

### NonAdminBSL referencing the Secret

```yaml
apiVersion: nac.oadp.openshift.io/v1alpha1
kind: NonAdminBackupStorageLocation
metadata:
  name: my-bsl
  namespace: <user-namespace>
spec:
  backupStorageLocationSpec:
    provider: aws  # or gcp, azure
    credential:
      name: my-backup-credentials
      key: cloud
    objectStorage:
      bucket: <bucket-name>
      prefix: <prefix>
    config:
      region: <region>
```

## Security Recommendations for Administrators

1. **Scope long-term credentials narrowly.** Each non-admin user's cloud credential should have permissions restricted to a specific bucket/prefix. Do not issue credentials with access to the entire backup bucket.

2. **Rotate credentials regularly.** Since non-admin users must use long-term credentials, establish a rotation policy. Revoke and reissue credentials periodically.

3. **Enable BSL approval.** Set `requireApprovalForBSL: true` in the DPA `nonAdmin` section to review each NonAdminBSL before a Velero BSL is created.

4. **Monitor for credential misuse.** Use cloud provider audit logs (CloudTrail, GCP Audit Logs, Azure Activity Logs) to monitor for unexpected access patterns from non-admin credentials.

5. **Do not share credentials across namespaces.** Each non-admin namespace should have its own unique cloud credential scoped to its own storage path.

## Future Work

Per-namespace short-lived credentials for NonAdminBSL are being designed in [openshift/oadp-operator#2143](https://github.com/openshift/oadp-operator/pull/2143).
This would allow administrators to pre-provision scoped cloud identities per namespace, with the non-admin controller automatically selecting the correct credential based on namespace mapping.
Until that work is complete, **long-term credentials are the only supported option for NonAdminBSL**.
