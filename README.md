# OADP NAC

Non Admin Controller

[![Continuos Integration](https://github.com/migtools/oadp-non-admin/actions/workflows/ci.yml/badge.svg)](https://github.com/migtools/oadp-non-admin/actions/workflows/ci.yml)

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

<!-- TODO update in future, probably will need to use unsupported override in DPA -->

### Install

To install latest OADP NAC in `oadp-nac-system` namespace in your cluster, run
```sh
make deploy
```

To check the deployment, run
```sh
oc get all -n oadp-nac-system
```

you should have an output similar to this:
```
NAME                                               READY   STATUS    RESTARTS   AGE
pod/oadp-nac-controller-manager-74bbf4577b-nssw4   2/2     Running   0          3m7s

NAME                                                  TYPE        CLUSTER-IP       EXTERNAL-IP   PORT(S)    AGE
service/oadp-nac-controller-manager-metrics-service   ClusterIP   172.30.201.185   <none>        8443/TCP   3m8s

NAME                                          READY   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/oadp-nac-controller-manager   1/1     1            1           3m7s

NAME                                                     DESIRED   CURRENT   READY   AGE
replicaset.apps/oadp-nac-controller-manager-74bbf4577b   1         1         1       3m7s
```

### Testing

To test NAC functionality, create non admin CRs. For example, run
```sh
oc apply -f config/samples/nac_v1alpha1_nonadminbackup.yaml
```

To create a non admin user to test NAC, check [non admin user documentation](docs/non_admin_user.md).

### Uninstall

To uninstall the previously installed OADP NAC in your cluster, run
```sh
make undeploy
```

> **NOTE:** make sure there are no running instances of CRDs. Finalizers in those objects can fail the `undeploy` command.

## Contributing

Please check our [contributing documentation](docs/CONTRIBUTING.md) to propose changes to the repository.

## Architecture

For a better understanding of the project, check our [architecture documentation](docs/architecture.md).

## OADP version compatibility

OADP NAC needs OADP operator to work. The relationship between compatible versions is presented below.

| NAC version | OADP version |
|-------------|--------------|
| master      | master       |
| v0.1.0      | v1.4.0       |

## License

This repository is licensed under the terms of [Apache License Version 2.0](LICENSE).
