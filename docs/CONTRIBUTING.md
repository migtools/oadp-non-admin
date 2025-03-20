# Contributing

To contribute to the project, please first check if a issue/pull request does not already exist regarding the changes you are suggesting. If a issue exists, please link it to the pull request you are creating.

If your changes involve controller logic, please test it prior to submitting, by following [install from source section](#install-from-source).

If your changes involve code, please check [code quality and standardization section](#code-quality-and-standardization).

If your changes involve Kubernetes objects (`config/` folder), please follow [Kubernetes objects changes section](#kubernetes-objects-changes).

If your changes involve Velero version or its objects, please follow [Velero objects changes section](#velero-objects-changes).

If you are upgrading project's OADP version, please follow [upgrade OADP version section](#upgrade-oadp-version).

If you are upgrading project's kubebuilder version, please follow [upgrade kubebuilder version section](#upgrade-kubebuilder-version).

> **NOTE:** Run `make help` for more information on all potential `make` targets

## Prerequisites
- go version v1.23+
- docker version 17.03+
- oc
- Access to a OpenShift cluster

## Install from source

To install OADP operator from default or a release branch in your cluster, with related OADP NAC from default or same release branch, run
```sh
git clone --depth=1 git@github.com:openshift/oadp-operator.git -b master # or appropriate branch
cd oadp-operator
make deploy-olm
```

To install OADP operator from a branch in your cluster, with OADP NAC from current development branch (a PR branch, for example), run
```sh
export NAC_PATH=$PWD # or appropriate NAC repository path, already with current branch pointing to development branch
export DEV_IMG=ttl.sh/oadp-non-admin-$(git rev-parse --short HEAD)-$(echo $RANDOM):1h
IMG=$DEV_IMG make docker-build docker-push
git clone --depth=1 git@github.com:openshift/oadp-operator.git -b master # or appropriate branch
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

### Go dependencies

To check if project's Go dependencies are ok, run
```sh
make check-go-dependencies
```

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

## Velero objects changes

If Velero version or its objects needs changes, follow this workflow:
- create branch in this repository and run `make update-non-admin-manifests` command, pointing to related created OADP operator repository branch. Example:
    ```sh
    OADP_OPERATOR_PATH=./home/user/oadp-operator make update-velero-manifests
    ```
- create pull requests in NAC

[More information](architecture.md#oadp-integration).

## Upgrade OADP version

To upgrade OADP version, run
```sh
go get github.com/openshift/oadp-operator@master # or appropriate branch
```

TODO when to update oadp-operator version in go.mod?

## Upgrade kubebuilder version

To upgrade kubebuilder version, create kubebuilder structure using the current kubebuilder version and the upgrade version (get kubebuilder executables in https://github.com/kubernetes-sigs/kubebuilder/releases), using the same commands presented in [kubebuilder architecture documentation](architecture.md#kubebuilder), in two different folders. Then generate a `diff` file from the two folders and apply changes to project code.

Example
```sh
mkdir current
mkdir new
cd current
# Run kubebuilder commands pointing to kubebuilder executable with the current version
cd ..
cd new
# Run kubebuilder commands pointing to kubebuilder executable with the new version
cd ..
diff -ruN current new > kubebuilder-upgrade.diff
patch -p1 --verbose -d ./ -i kubebuilder-upgrade.diff
# Resolve possible conflicts
```
