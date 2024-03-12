# Architecture

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
> **NOTE:** The information about plugin and project version, as well as project name, repo and domain, is stored in [PROJECT](PROJECT) file

> **NOTE:** More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)
