# Contributing

To contribute to the project, please first check if a issue/pull request does not already exist regarding the changes you are suggesting. If a issue exists, please link it to the pull request you are creating.

If your changes involve controller logic, please test it prior to submitting, by following [install from source section](#install-from-source).

If your changes involve code, please check [code quality and standardization section](#code-quality-and-standardization).

If you are upgrading project's kubebuilder version, please follow [upgrade kubebuilder version section](#upgrade-kubebuilder-version).

> **NOTE:** Run `make help` for more information on all potential `make` targets

## Prerequisites
- go version v1.21.0+
- docker version 17.03+
- oc
- Access to a OpenShift cluster
- [OADP operator](https://github.com/openshift/oadp-operator) installed in the cluster

## Install from source

To install OADP NAC from current branch in `oadp-nac-system` namespace in your cluster, run
```sh
make deploy-dev
```

> **NOTE:** You can use a different development image by running `export DEV_IMG=quay.io/user/image-name:tag` prior to `deploy-dev` command.

To create a non admin user to test NAC, check [non admin user documentation](non_admin_user.md).

To uninstall the previously installed OADP NAC in your cluster, run
```sh
make undeploy-dev
```

## Code quality and standardization

The quality/standardization checks of the project are reproduced by the continuos integration (CI) pipeline of the project. CI configuration in [`.github/workflows/ci.yml`](../.github/workflows/ci.yml) file.

To run all checks locally, run `make ci`.

### Tests

To run unit and integration tests, run
```sh
make simulation-test
```

TODO report, coverage and tests information

### Linters and code formatters

To run Go linters and check Go code format, run
```sh
make lint
```

To fix Go linters issues and format Go code, run
```sh
make lint-fix
```

Go linters and Go code formatters configuration in [`.golangci.yml`](../.golangci.yml) file.

To check all files format, run
```sh
make ec
```

Files format configuration in [`.editorconfig`](../.editorconfig) file.

### Container file linter

To run container file linter, run
```sh
make hadolint
```

### Code generation

To check if project code was generated, run
```sh
make check-generate
make check-manifests
```

## Upgrade kubebuilder version

To upgrade kubebuilder version, create kubebuilder structure using the current kubebuilder version and the upgrade version, using the same commands presented in [kubebuilder architecture documentation](architecture.md#kubebuilder), in two different folders. Then generate a `diff` file from the two folders and apply changes to project code.
