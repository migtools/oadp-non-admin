# Creating non admin user on a OpenShift cluster

Check your cloud provider documentation for more detailed information.

## AWS

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

### Assigning NAC permissions to a user

Ensure you have appropriate Cluster Role available in your cluster:
```shell
$ oc apply -f config/rbac/nonadminbackup_editor_role.yaml
```

**Create Role Binding for our test user within oadp-nac-system namespace:**
**NOTE:** There could be also a ClusterRoleBinding for the nacuser or one of the groups
to which nacuser belongs to easy administrative tasks and allow use of NAC for wider audience. Please see next paragraph.
```sh
$ cat > nacuser-rolebinding.yaml <<EOF
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: nacuser-nonadminbackup
  namespace: oadp-nac-system
subjects:
- kind: User
  name: nacuser
roleRef:
  kind: ClusterRole
  name: nonadminbackup-editor-role
  apiGroup: rbac.authorization.k8s.io
EOF
```
**Apply the Role Binding file to the cluster:**
```sh
$ oc apply -f nacuser-rolebinding.yaml
```

**Alternatively Create Cluster Role Binding for our test user:**
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
