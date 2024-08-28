# Contributing

To contribute to the project, please first check if a issue/pull request does not already exist regarding the changes you are suggesting. If a issue exists, please link it to the pull request you are creating.

If your changes involve controller logic, please test it prior to submitting, by following [install from source section](#install-from-source).

If your changes involve code, please check [code quality and standardization section](#code-quality-and-standardization).

If your changes involve Kubernetes objects (`config/` folder), please follow [Kubernetes objects changes section](#kubernetes-objects-changes).

If you are upgrading project's kubebuilder version, please follow [upgrade kubebuilder version section](#upgrade-kubebuilder-version).

> **NOTE:** Run `make help` for more information on all potential `make` targets

## Prerequisites
- go version v1.22+
- docker version 17.03+
- oc
- Access to a OpenShift cluster

## Install from source

To install OADP operator in your cluster, with OADP NAC from current branch, run
```sh
export DEV_IMG=ttl.sh/oadp-non-admin-$(git rev-parse --short HEAD)-$(echo $RANDOM):1h
export NAC_PATH=$PWD
git clone --depth=1 git@github.com:openshift/oadp-operator.git -b master # or appropriate branch
IMG=$DEV_IMG make docker-build docker-push
cd oadp-operator
NON_ADMIN_CONTROLLER_PATH=$NAC_PATH NON_ADMIN_CONTROLLER_IMG=$DEV_IMG make update-non-admin-manifests deploy-olm
```

To create a non admin user to test NAC, check [non admin user documentation](non_admin_user.md).

To uninstall the previously installed OADP operator in your cluster, run
```sh
cd oadp-operator
make undeploy-olm
```

> **NOTE:** Make sure there are no running instances of CRDs. Finalizers in those objects can fail uninstall command.

## Code quality and standardization

The quality/standardization checks of the project are reproduced by the continuous integration (CI) pipeline of the project. CI configuration in [`.github/workflows/ci.yml`](../.github/workflows/ci.yml) file.

To run all checks locally, run `make ci`.

### Tests

To run unit and integration tests and coverage report, run
```sh
make simulation-test
```

To see the html report, run
```sh
go tool cover -html=cover.out
```

To run just controllers integration tests (which gives more verbose output), run
```sh
ginkgo run -mod=mod internal/controller -- --ginkgo.vv
```

TODO end to end tests

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

## Kubernetes objects changes

If NAC Kubernetes objects are changed, like CRDs, RBACs, etc, follow this workflow:
- create branch in NAC repository and make the necessary changes
- create branch in OADP operator repository and run `make update-non-admin-manifests` command, pointing to previously created NAC branch. Example:
    ```sh
    NON_ADMIN_CONTROLLER_PATH=/home/user/oadp-non-admin make update-non-admin-manifests
    ```
- create pull requests both in NAC and OADP operator repositories (OADP operator repository pull request must be merged first)

[More information](architecture.md#oadp-integration).

## Upgrade kubebuilder version

To upgrade kubebuilder version, create kubebuilder structure using the current kubebuilder version and the upgrade version, using the same commands presented in [kubebuilder architecture documentation](architecture.md#kubebuilder), in two different folders. Then generate a `diff` file from the two folders and apply changes to project code.
