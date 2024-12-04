# NonAdminBackupStorageLocation Controller Design

## Overview
The `NonAdminBackupStorageLocation` controller is responsible for managing backup storage locations requested by non-admin users in a multi-tenant Kubernetes environment. It ensures that users can only access and manage backup storage locations within their authorized namespaces while maintaining security boundaries.

## Architecture

```mermaid
%%{init: {'theme':'neutral'}}%%
flowchart TD
    title[Non-Admin BSL Controller Workflow]
    style title font-size:24px,font-weight:bold,fill:#e6f3ff,stroke:#666,stroke-width:2px,stroke-dasharray: 0

    %% Start
    START[**Start NaBSL Reconciliation**] --> OPERATION[**Determine Operation Type**]

    %% Create/Update Flow
    OPERATION -->|**Create/Update**| VALIDATE_CONFIG{Validate Non-Admin BSL Config}
    VALIDATE_CONFIG -->|Invalid| INVALID_CONFIG[Set Phase: Invalid]
    VALIDATE_CONFIG -->|Valid| GENERATE_UUID[Generate NaBSL UUID and Store in Status]

    GENERATE_UUID --> CREATE_OR_UPDATE_SECRET[Create/Update Secret in OADP Namespace]
    CREATE_OR_UPDATE_SECRET --> CREATE_OR_UPDATE_BSL[Create/Update Velero BSL Resource in OADP Namespace]
    CREATE_OR_UPDATE_BSL --> UPDATE_STATUS[Update NaBSL Status with Velero BSL Info]

    %% Delete Flow
    OPERATION -->|**Delete**| CHECK_SECRET_EXISTS{Check if Secret Exists}
    CHECK_SECRET_EXISTS -->|Yes| DELETE_SECRET[Delete Secret in OADP Namespace]
    CHECK_SECRET_EXISTS -->|No| CHECK_BSL_EXISTS{Check if Velero BSL Exists}

    DELETE_SECRET --> CHECK_BSL_EXISTS
    CHECK_BSL_EXISTS -->|Yes| DELETE_BSL[Delete Velero BSL Resource in OADP Namespace]
    CHECK_BSL_EXISTS -->|No| REMOVE_FINALIZER[Remove Finalizer from NaBSL Resource]

    DELETE_BSL --> REMOVE_FINALIZER

    %% Endpoints
    INVALID_CONFIG --> END[End Reconciliation]
    UPDATE_STATUS --> END
    REMOVE_FINALIZER --> END

    %% Subgraphs
    subgraph "Validation"
    VALIDATE_CONFIG
    end

    subgraph "Create/Update Operations"
    GENERATE_UUID
    CREATE_OR_UPDATE_SECRET
    CREATE_OR_UPDATE_BSL
    UPDATE_STATUS
    end

    subgraph "Delete Operations"
    CHECK_SECRET_EXISTS
    DELETE_SECRET
    CHECK_BSL_EXISTS
    DELETE_BSL
    REMOVE_FINALIZER
    end

    %% Styling
    classDef phase fill:#ffcc99,stroke:#333,stroke-width:2px
    classDef process fill:#b3d9ff,stroke:#333,stroke-width:2px
    classDef decision fill:#ffeb99,stroke:#333,stroke-width:2px
    classDef endpoint fill:#d9f2d9,stroke:#333,stroke-width:2px

    %% Apply styles
    class START,END endpoint
    class OPERATION,VALIDATE_CONFIG,CHECK_SECRET_EXISTS,CHECK_BSL_EXISTS decision
    class GENERATE_UUID,CREATE_OR_UPDATE_SECRET,CREATE_OR_UPDATE_BSL,DELETE_SECRET,DELETE_BSL,REMOVE_FINALIZER process
    class INVALID_CONFIG,UPDATE_STATUS phase
```

## Components

### 1. Controller Structure
- **Name**: NonAdminBackupStorageLocation
- **Type**: Kubernetes Custom Resource Controller
- **Scope**: Namespace-scoped
- **Watch Resources**: BackupStorageLocation CRD

### 2. Key Responsibilities
- Validate user permissions for Non-Admin BSL
- Manage Velero BSL lifecycle (create, update, delete)
- Manage Velero BSL Secret lifecycle (create, update, delete)
- Ensure namespace isolation
- Validate Non-Admin BSL configurations
- Update Non-Admin BSL status
- Generate and store Non-Admin BSL UUID in the NaBSL Status
- Use the UUID to create or update relevant resources

### 3. Security Considerations
- Prevention of cross-namespace access by ensuring that user can only point to the namespace Secret and the resulting Velero BSL resource will point to the secret in the OADP namespace

## Workflow

### Non-Admin BSL Creation Flow
1. User submits a Non-Admin BSL creation request.
2. Controller verifies the Non-Admin BSL configuration including existance of the secret in user's namespace.
3. Controller generates Non-Admin BSL UUID and stores it in the NaBSL Status.
4. Controller creates or updates a Secret in the OADP namespace based on the Non-Admin BSL UUID.
5. Controller creates a Velero BSL resource in the OADP namespace pointing to the Secret from the OADP namespace.
6. Controller updates the NaBSL Status with the information from the created Velero BSL resource.

### Non-Admin BSL Update Flow
1. User submits a Non-Admin BSL update request.
2. Controller validates changes
3. Controller updates the Secret and/or Velero BSL resource in the OADP namespace based on the Non-Admin BSL UUID.
4. Controller updates the NaBSL Status with the information from the updated Velero BSL resource.

### Deletion Flow
1. User deletes the Non-Admin BSL resource.
2. Controller deletes the Secret from the OADP namespace based on the Non-Admin BSL UUID.
3. Controller deletes the Velero BSL resource from the OADP namespace based on the Non-Admin BSL UUID.
4. Controller removes the finalizer from the Non-Admin BSL resource.
