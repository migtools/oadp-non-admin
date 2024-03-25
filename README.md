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

### Install

To install OADP operator in your cluster, with OADP NAC from current branch, run
```sh
make deploy-dev
```

The command can be customized by setting the following environment variables
```sh
OADP_FORK=<OADP_operator_user_or_org>
OADP_VERSION=<OADP_operator_branch_or_tag>
OADP_NAMESPACE=<OADP_operator_installation_namespace>
```

### Testing

To test NAC functionality:
- create DPA with non admin feature enabled
- create non admin CRs. For example, run
    ```sh
    oc apply -f config/samples/nac_v1alpha1_nonadminbackup.yaml
    ```

To create a non admin user to test NAC, check [non admin user documentation](docs/non_admin_user.md).

### Uninstall

To uninstall the previously installed OADP operator in your cluster, run
```sh
make undeploy-dev
```

> **NOTE:** make sure there are no running instances of CRDs. Finalizers in those objects can fail `undeploy-dev` command.

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
