/*
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
*/

package controller

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	oadpv1alpha1 "github.com/migtools/oadp-non-admin/api/v1alpha1"
	"github.com/migtools/oadp-non-admin/internal/common/constant"
)

var _ = ginkgo.Describe("NonAdminRestore Controller", func() {
	ginkgo.Context("When reconciling a resource", func() {
		const (
			resourceName      = "test-restore-resource-name"
			resourceNamespace = "test-restore-resource-namespace"

			nonAdminBackupResourceName = "test-non-admin-backup-resource-name"

			TestUUID = "test-123456789"

			oadpNamespace = "test-restore-oadp-namespace"
		)

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: resourceNamespace,
		}
		nonadminrestore := &oadpv1alpha1.NonAdminRestore{}

		ginkgo.BeforeEach(func() {
			ginkgo.By("creating namespaces")
			err := createTestNamespaces(ctx, resourceNamespace, oadpNamespace)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("creating the custom resource for the Kind NonAdminBackup")
			nonAdminBackupResource := &oadpv1alpha1.NonAdminBackup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      nonAdminBackupResourceName,
					Namespace: resourceNamespace,
				},
				Spec: oadpv1alpha1.NonAdminBackupSpec{
					BackupSpec: &velerov1.BackupSpec{
						IncludedNamespaces: []string{resourceNamespace},
					},
				},
			}
			gomega.Expect(k8sClient.Create(ctx, nonAdminBackupResource)).To(gomega.Succeed())
			nonAdminBackupResource.Status.Phase = oadpv1alpha1.NonAdminPhaseCreated
			nonAdminBackupResource.Status.VeleroBackup = &oadpv1alpha1.VeleroBackup{}
			nonAdminBackupResource.Status.VeleroBackup.NACUUID = TestUUID
			gomega.Expect(k8sClient.Status().Update(ctx, nonAdminBackupResource)).To(gomega.Succeed())

			ginkgo.By("creating the custom resource for the Kind Velero Backup")
			backupResource := &velerov1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "does-not-matter",
					Namespace: oadpNamespace,
					Labels: map[string]string{
						constant.NabOriginNACUUIDLabel: TestUUID,
					},
				},
			}
			gomega.Expect(k8sClient.Create(ctx, backupResource)).To(gomega.Succeed())

			ginkgo.By("creating the custom resource for the Kind NonAdminRestore")
			err = k8sClient.Get(ctx, typeNamespacedName, nonadminrestore)
			if err != nil && errors.IsNotFound(err) {
				resource := &oadpv1alpha1.NonAdminRestore{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: resourceNamespace,
					},
					Spec: oadpv1alpha1.NonAdminRestoreSpec{
						RestoreSpec: &velerov1.RestoreSpec{
							BackupName: nonAdminBackupResourceName,
						},
					},
				}
				gomega.Expect(k8sClient.Create(ctx, resource)).To(gomega.Succeed())

				p, _ := json.MarshalIndent(resource, "", "\t")
				fmt.Printf("%s\n", p)
			}
		})

		ginkgo.AfterEach(func() {
			resource := &oadpv1alpha1.NonAdminRestore{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			p, _ := json.MarshalIndent(resource, "", "\t")
			fmt.Printf("%s\n", p)

			ginkgo.By("Cleanup the specific resource instance NonAdminRestore")
			gomega.Expect(k8sClient.Delete(ctx, resource)).To(gomega.Succeed())

			ginkgo.By("Finalizer should not allow NonAdminRestore deletion")
			err = k8sClient.Get(ctx, typeNamespacedName, resource)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			controllerReconciler := &NonAdminRestoreReconciler{
				Client:        k8sClient,
				Scheme:        k8sClient.Scheme(),
				OADPNamespace: oadpNamespace,
			}
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("After reconcile should allow NonAdminRestore deletion")
			err = k8sClient.Get(ctx, typeNamespacedName, resource)
			gomega.Expect(apierrors.IsNotFound(err)).To(gomega.BeTrue())
		})
		ginkgo.It("should successfully reconcile the resource", func() {
			ginkgo.By("Reconciling the created resource")
			controllerReconciler := &NonAdminRestoreReconciler{
				Client:        k8sClient,
				Scheme:        k8sClient.Scheme(),
				OADPNamespace: oadpNamespace,
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
			// Example: If you expect a certain status condition after reconciliation, verify it here.
		})
	})
})
