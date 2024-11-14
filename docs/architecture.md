# Architecture

## OADP integration

Normally, to ship a controller to users, the project would present the file created by `make build-installer` command (which include various Kubernetes objects, like Namespace, ServiceAccount, Deployment, etc), to user to install the controller. But since NAC needs OADP operator to properly work, those Kubernetes objects are shipped within OADP operator (and also Kubernetes objects in `config/samples/` folder). Because of this restriction, generated Kubernetes objects names and labels in `config/` folder, may need to be updated to match OADP operator standards (for example, `oadp-nac` values are changed to `oadp-operator`) and avoid duplications, by changing Kubernetes object names to `non-admin-controller`, or adding it as a prefix.

> **NOTE:** If needed, you can test NAC alone by running `make build-installer` and `oc apply -f ./dist/install.yaml`. You may want to customize namespace (`openshift-adp-system`) and container image (`quay.io/konveyor/oadp-non-admin:latest`) in that file prior to deploying it to your cluster.

NAC objects are included in OADP operator through `make update-non-admin-manifests` command, which is run in OADP operator repository.

> **NOTE:** Manual steps required in OADP operator repository branch prior to implementation of `make update-non-admin-manifests` command:
> - `RELATED_IMAGE_NON_ADMIN_CONTROLLER` must be already set in `config/manager/manager.yaml` file
> - add option to use NAC in OADP CRD
> - write and integrate NAC controller with OADP

> **NOTE:** `make update-non-admin-manifests` command does not work for deletion, i.e., if a file that was previously managed by the command is deleted (or renamed), it needs to be manually deleted.

The continuous integration (CI) pipeline of the project verifies if OADP operator repository branches have up to date NAC objects included.

Velero version and its objects (for tests) are updated in NAC through `make update-velero-manifests` command, which is run in this repository.

> **NOTE:** Manual steps required in this repository prior to implementation of `make update-velero-manifests` command:
> - replace statement with `github.com/openshift/velero` must be already set in `go.mod` file
> - add empty file with expected name in `hack/extra-crds` folder

> **NOTE:** `make update-velero-manifests` command does not work for deletion, i.e., if a file that was previously managed by the command is deleted (or renamed), it needs to be manually deleted.

The continuous integration (CI) pipeline of the project verifies if this repository branches have up to date Velero version and Velero objects.

## Kubebuilder

The project was generated using kubebuilder version `v3.14.0`, running the following commands
```sh
kubebuilder init \
    --plugins go.kubebuilder.io/v4 \
    --project-version 3 \
    --project-name=oadp-nac \
    --repo=github.com/migtools/oadp-non-admin \
    --domain=openshift.io
kubebuilder create api \
    --plugins go.kubebuilder.io/v4 \
    --group oadp \
    --version v1alpha1 \
    --kind NonAdminBackup \
    --resource --controller
kubebuilder create api \
    --plugins go.kubebuilder.io/v4 \
    --group oadp \
    --version v1alpha1 \
    --kind NonAdminRestore \
    --resource --controller
make manifests
```
> **NOTE:** The information about plugin and project version, as well as project name, repo and domain, is stored in [PROJECT](../PROJECT) file

> **NOTE:** More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)
