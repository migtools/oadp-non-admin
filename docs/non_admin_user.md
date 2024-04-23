# Creating non admin user on a OpenShift cluster

Check your cloud provider documentation for more detailed information.

## AWS

### Authentication

Choose one of the authentication method sections to follow.

#### OAuth

- Create sample identity file
  ```sh
  htpasswd -c -B -b ./non_admin_user.htpasswd <non-admin-user> <password>
  ```
- Create secret from the previously created identity file in your cluster
  ```sh
  oc create secret generic non-admin-user --from-file=htpasswd=./non_admin_user.htpasswd -n openshift-config
  ```
- Add new entry to `spec.identityProviders` field from OAuth cluster (`oc get OAuth cluster`)
  ```yaml
  ...
  spec:
    identityProviders:
    - name: # non-admin-user
      mappingMethod: claim
      type: HTPasswd
      htpasswd:
        fileData:
          name: # non-admin-user
  ```
- [Apply permissions to your non admin user](#permissions)

## Permissions

- Create non admin user namespace
  ```sh
  oc create namespace <non-admin-user-namespace>
  ```
- Ensure non admin user have appropriate permissions in its namespace, i.e., non admin user have editor roles for the following objects
  - `nonadminbackups.nac.oadp.openshift.io`

  For example
  ```yaml
    # config/rbac/nonadminbackup_editor_role.yaml
    - apiGroups:
        - nac.oadp.openshift.io
      resources:
        - nonadminbackups
      verbs:
        - create
        - delete
        - get
        - list
        - patch
        - update
        - watch
    - apiGroups:
        - nac.oadp.openshift.io
      resources:
        - nonadminbackups/status
      verbs:
        - get
  ```
  For example, make non admin user have `admin` ClusterRole permissions on its namespace
  ```sh
  oc create rolebinding <non-admin-user>-namespace-admin --clusterrole=admin --user=<non-admin-user> --namespace=<non-admin-user-namespace>
  ```
  <!-- TODO  check what restrictions non admin user permissions must have, for example can not create project or velero/oadp objects -->
