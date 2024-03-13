# Non-Admin Backup/Restore Design

## Background
OADP (Openshift API for Data Protection) Operator currently requires cluster admin access for performing Backup and Restore operations of applications deployed on the OpenShift platform. This design intends to enable the ability to perform Backup and Restore operations of their own application namespace for namespace owners aka non-admin users.

## Goals
- Enable non-admin backup operation
- Enable non-admin restore operation
- Enable non-admin configuration of BackupStorageLocation

## Non-Goals
- Performance improvements of Backup and Restore Operations
- Parallel Backup and Restore Operations

## Use-Cases

### Backup Operation
- As a non-admin user/namespace owner with administrative priviledges for a particular namespace, the user should be able to:
    - Create a Backup/Schedule of the namespace
    - Update the Backup/Schedule spec of the namespace
    - View the status of the Backup/Schedule created for the particular namespace
    - Delete the Backup/Schedule of the namespace

### Restore Operation
- As a non-admin user/namespace owner with administrative priviledges for a particular namespace, the user should be able to:
    - Create a Restore of the namespace
    - Update the Restore spec of the namespace
    - View the status of the Restore created for the particular namespace
    - Delete the Restore of the namespace

### BackupStorageLocation(BSL) Configuration
- As a non-admin user/namespace owner, the user should be able to 
    - Create a backup storage location with their own credentials
    - Update the BSL spec of the already configured BSL
    - View the status of the BSL created 
    - Delete the BSL created


## Installation

- The Non-Admin Controller (NAC) will be installed via OADP Operator. 
- The Data Protection Application (DPA) CR will consist of a root level spec flag called `enableNonAdminMode`
- If the `enableNonAdminMode` flag is set to `true`, the OADP Operator will install the NAC in OADP Operator's install namespace

## Pre-requisites
- Non-admin user with administrative priviledges for particular application namespace 
- Create/Update/View/Delete verb support of all the Non-admin CRDs introduced in this feature design

## High-Level design

### Components
- OADP Operator: OADP is the OpenShift API for Data Protection operator. This open source operator sets up and installs Velero on the OpenShift platform, allowing users to backup and restore applications.
- Controllers: The Non-Admin controller will pack the following controllers as part of it:
    - Non-Admin Backup (NAB) Controller: The responsibilities of the NAB controller are:
        - Validate whether the non-admin user has appropriate administrative namepsace access 
        - Validate Wehther the non-admin user has appropriate access to create/view/update /delete Non-Admin Backup CR
        - Listen to requests pertaining to Non-Admin Backup CRD across all the namespaces 
        - Process requests pertaining to Non-Admin Backup CRD across all the namespaces 
        - Update Non-Admin CR status with the status/events from Velero Backup CR
        - Cascade Any actions performed on Non-Admin Backup CR to corresponding Velero backup CR
    - Non-Admin Restore (NAR) Controller
    - Non-Admin BackupStorageLocation (NABSL) Controller
- CRDs: The following CRDs will be provided to Non-Admin users:
    - Non-Admin Backup (NAB) CRD: This iCRD will encapsulate the whole Velero Backup CRD and some additional spec felds that will be needed for non-admin feature.
    - Non-Admin Restore (NAR) CRD
    - Non-Admin BSL (NABSL) CRD


### Implementation details
- Backup Workflow
    - Non-Admin user creates a Non-Admin backup CR
    - NAB controller reconiles on this NAB CR 
    - NAB controller validates the NAB CR and then creates a corresponding Velero Backup CR
    - NAB controller cascades the status backup from Velero CR to NAB CR

![NAB-Backup Workflow Diagram](nab-backup-workflow.jpg)

- Restore Workflow


## Open Questions and Know Limitations
- Velero command and pod logs
- Multiple instance of OADP Operator