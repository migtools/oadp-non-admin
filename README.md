# OADP NAC

Non Admin Controller

[![Continuos Integration](https://github.com/migtools/oadp-non-admin/actions/workflows/ci.yml/badge.svg)](https://github.com/migtools/oadp-non-admin/actions/workflows/ci.yml)

<!-- TODO add Official documentation link once it is created -->

Documentation in this repository are considered unofficial and for development purposes only.

## Description

This open source controller adds the non admin feature to [OADP operator](https://github.com/openshift/oadp-operator). With it, cluster admins can configure which namespaces non admin users can backup/restore.

## Getting Started

### Prerequisites
- oc
- Access to a OpenShift cluster
- [OADP operator](https://github.com/openshift/oadp-operator) version `1.4+` installed in the cluster

> **NOTE:** Before OADP operator version 1.4.0 is released, you need to [install OADP operator from source](docs/CONTRIBUTING.md#install-from-source) to use NAC.

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
    - create NonAdminBackup

        For example, use one of the sample NonAdminBackup available in `hack/samples/backups/` folder, by running
        ```sh
        oc process -f ./hack/samples/backups/<type> \
            -p NAMESPACE=<non-admin-user-namespace> \
            | oc create -f -
        ```
        <!-- TODO how to track status -->
    - TODO NonAdminRestore

## Contributing

Please check our [contributing documentation](docs/CONTRIBUTING.md) to propose changes to the repository.

## Architecture

For a better understanding of the project, check our [architecture documentation](docs/architecture.md).

## License

This repository is licensed under the terms of [Apache License Version 2.0](LICENSE).
