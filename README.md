# oadp-nac
// TODO(user): Add simple overview of use/purpose

## Description
// Non Admin controller

## Getting Started

### Prerequisites
- go version v1.21.0+
- docker version 17.03+.
- kubectl version v1.11.3+.
- Access to a Kubernetes v1.11.3+ cluster.

### Creating Non Admin User on the OCP cluster
**Create sample Namespace:**
```sh
$ oc create ns nac-testing
```

**Create sample identity file:**
```sh
# Using user nacuser with sample pass
$ htpasswd -c -B -b ./users_file.htpasswd nacuser Mypassw0rd
```

**Create secret from the previously created htpasswd file in OpenShift**
```sh
$ oc create secret generic htpass-secret --from-file=htpasswd=./users_file.htpasswd -n openshift-config
```

**Create OAuth file**
```sh
$ cat > oauth-nacuser.yaml <<EOF
apiVersion: config.openshift.io/v1
kind: OAuth
metadata:
  name: cluster
spec:
  identityProviders:
  - name: oadp_nac_test_provider 
    mappingMethod: claim 
    type: HTPasswd
    htpasswd:
      fileData:
        name: htpass-secret 
EOF
```
**Apply the OAuth file to the cluster:**
```sh
$ oc apply -f oauth-nacuser.yaml
```

### Assigning NAC permissions to the user

Ensure you have appropriate Cluster Role available in youc cluster:
```shell
$ oc apply -f config/rbac/nonadminbackup_editor_role.yaml
```

**Create Role Binding for our test user within nac-testing namespace:**
**NOTE:** There could be also a ClusterRoleBinding for the nacuser or one of the groups
to which nacuser belongs to easy administrative tasks and allow use of NAC for wider audience. Please see next paragraph. 
```sh
$ cat > nacuser-rolebinding.yaml <<EOF
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: nacuser-nonadminbackup
  namespace: nac-testing
subjects:
- kind: User
  name: nacuser
roleRef:
  kind: ClusterRole
  name: nonadminbackup-editor-role
  apiGroup: rbac.authorization.k8s.io
EOF
```
**Apply the Cluster Role Binding file to the cluster:**
```sh
$ oc apply -f nacuser-rolebinding.yaml
```

**Alternatively Create Cluster Role Binding for our test user within nac-testing namespace:**
```sh
$ cat > nacuser-clusterrolebinding.yaml <<EOF
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: nacuser-nonadminbackup-cluster
subjects:
- kind: User
  name: nacuser
roleRef:
  kind: ClusterRole
  name: nonadminbackup-editor-role
  apiGroup: rbac.authorization.k8s.io
EOF
```
**Apply the Cluster Role Binding file to the cluster:**
```sh
$ oc apply -f nacuser-clusterrolebinding.yaml
```

### To Deploy on the cluster
**Build and push your image to the location specified by `IMG`:**

```sh
# Example to build with podman
export NAC_TAG="v0.0.1"
export IMG_REGISTRY="quay.io/<USER>/oadp-nac"

make docker-build docker-push IMG="${IMG_REGISTRY}:${NAC_REV}" CONTAINER_TOOL=podman
```

**NOTE:** This image ought to be published in the personal registry you specified. 
And it is required to have access to pull the image from the working environment. 
Make sure you have the proper permission to the registry if the above commands don’t work.

**Install the CRDs into the cluster:**

```sh
make install
```

**Deploy the Manager to the cluster with the image specified by `IMG`:**

```sh
export NAC_TAG="v0.0.1"
export IMG_REGISTRY="quay.io/<USER>/oadp-nac"

make deploy IMG="${IMG_REGISTRY}:${NAC_REV}"
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin 
privileges or be logged in as admin.

**Create instances of your solution**
You can apply the samples (examples) from the config/sample:

```sh
kubectl apply -k config/samples/
```

>**NOTE**: Ensure that the samples has default values to test it out.

### To Uninstall
**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Delete the APIs(CRDs) from the cluster:**

```sh
make uninstall
```

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

## Project Distribution

Following are the steps to build the installer and distribute this project to users.

1. Build the installer for the image built and published in the registry:

```sh
make build-installer IMG=<some-registry>/oadp-nac:tag
```

NOTE: The makefile target mentioned above generates an 'install.yaml'
file in the dist directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without
its dependencies.

2. Using the installer

Users can just run kubectl apply -f <URL for YAML BUNDLE> to install the project, i.e.:

```sh
kubectl apply -f https://raw.githubusercontent.com/<org>/oadp-nac/<tag or branch>/dist/install.yaml
```

## Contributing
// TODO(user): Add detailed information on how you would like others to contribute to this project

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## Architecture

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

To upgrade kubebuilder version, create kubebuilder structure using the current kubebuilder version and the upgrade version, using the same commands presented earlier, in two different folders. Then generate a `diff` file from the two folders and apply changes to project code.

## License

Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

