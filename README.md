# OADP NAC

Non Admin Controller

[![Continuos Integration](https://github.com/migtools/oadp-non-admin/actions/workflows/ci.yml/badge.svg)](https://github.com/migtools/oadp-non-admin/actions/workflows/ci.yml)
[![OADP version compatibility check](https://github.com/migtools/oadp-non-admin/actions/workflows/oadp-compatibility-check.yml/badge.svg)](https://github.com/migtools/oadp-non-admin/actions/workflows/oadp-compatibility-check.yml)

<!-- TODO add Official documentation link once it is created -->

Documentation in this repository are considered unofficial and for development purposes only.

## Description

This open source controller adds the non admin feature to [OADP operator](https://github.com/openshift/oadp-operator). With it, cluster admins can configure which namespaces non admin users can backup/restore.

## Getting Started

### Prerequisites
- go version v1.21.0+
- oc
- Access to a OpenShift cluster
- [OADP operator](https://github.com/openshift/oadp-operator) installed in the cluster

### Using NAC

To use NAC functionality:
- as admin user, create non admin user (to create a non admin user to test NAC, check [non admin user documentation](docs/non_admin_user.md)) and its namespace
- as admin user, create/update DPA and set non admin feature to enabled
- as non admin user, create non admin CRs

## Contributing

Please check our [contributing documentation](docs/CONTRIBUTING.md) to propose changes to the repository.

## Architecture

For a better understanding of the project, check our [architecture documentation](docs/architecture.md).

## OADP version compatibility

OADP NAC needs OADP operator to work. The relationship between compatible branches is presented below.

| NAC branch | OADP branch |
|------------|-------------|
| master     | master      |

## License

This repository is licensed under the terms of [Apache License Version 2.0](LICENSE).
