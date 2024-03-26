# Architecture

## OADP integration

Normally, to ship a controller to users, the project would present the file created by `make build-installer` command (which contains Namespace, ServiceAccount, Deployment, etc, objects), to user to install the controller. But since NAC needs OADP operator to properly work, those Kubernetes objects are shipped within OADP operator.

Because of this restriction, generated Kubernetes object names and labels in `config` folder, may need to be updated to match OADP operator standards.

NAC objects are included in OADP operator through `make update-non-admin-manifests` command, which is run in OADP operator repository. To run the command:
- switch to OADP operator repository branch you want to update
- clone NAC compatible branch with the OADP operator repository branch you are (to check branches compatibility, see [OADP version compatibility](../README.md#oadp-version-compatibility)). Example:
    ```sh
    git clone --depth=1 git@github.com:migtools/oadp-non-admin -b oadp-1.4
    ```
- run `make update-non-admin-manifests` command, pointing to previously cloned NAC compatible branch. Example:
    ```sh
    NON_ADMIN_CONTROLLER_PATH=/home/user/oadp-non-admin make update-non-admin-manifests
    ```
- create pull request targeting OADP operator repository branch you want to update

The continuos integration (CI) pipeline of the project verifies if OADP operator repository compatible branches have up to date NAC objects included. OADP version compatibility check configuration in [`.github/workflows/oadp-compatibility-check.yml`](../.github/workflows/oadp-compatibility-check.yml) file.

## Kubebuilder

The project was generated using kubebuilder version `v3.14.0`, running the following commands
```sh
kubebuilder init \
    --plugins go.kubebuilder.io/v4 \
    --project-version 3 \
    --project-name=oadp-nac \
    --repo=github.com/migtools/oadp-non-admin \
    --domain=oadp.openshift.io
kubebuilder create api \
    --plugins go.kubebuilder.io/v4 \
    --group nac \
    --version v1alpha1 \
    --kind NonAdminBackup \
    --resource --controller
make manifests
```
> **NOTE:** The information about plugin and project version, as well as project name, repo and domain, is stored in [PROJECT](../PROJECT) file

> **NOTE:** More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)
