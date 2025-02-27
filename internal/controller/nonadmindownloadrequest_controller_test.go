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

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	nacv1alpha1 "github.com/migtools/oadp-non-admin/api/v1alpha1"
)

var _ = ginkgo.Describe("NonAdminDownloadRequest Controller", func() {
	ginkgo.Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		nonadmindownloadrequest := &nacv1alpha1.NonAdminDownloadRequest{}

		ginkgo.BeforeEach(func() {
			ginkgo.By("creating the custom resource for the Kind NonAdminDownloadRequest")
			err := k8sClient.Get(ctx, typeNamespacedName, nonadmindownloadrequest)
			if err != nil && errors.IsNotFound(err) {
				resource := &nacv1alpha1.NonAdminDownloadRequest{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					// TODO(user): Specify other spec details if needed.
				}
				gomega.Expect(k8sClient.Create(ctx, resource)).To(gomega.Succeed())
			}
		})

		ginkgo.AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &nacv1alpha1.NonAdminDownloadRequest{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			ginkgo.By("Cleanup the specific resource instance NonAdminDownloadRequest")
			gomega.Expect(k8sClient.Delete(ctx, resource)).To(gomega.Succeed())
		})
		ginkgo.It("should successfully reconcile the resource", func() {
			ginkgo.By("Reconciling the created resource")
			controllerReconciler := &NonAdminDownloadRequestReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			// TODO: Create NABSL in user1-ns
			// TODO: Create backup in user1-ns
			_, err := controllerReconciler.Reconcile(ctx, &nacv1alpha1.NonAdminDownloadRequest{
				// TODO: test various specs here
				ObjectMeta: metav1.ObjectMeta{
					Name:      "user1-nadr1",
					Namespace: "user1-ns",
				},
				Spec: nacv1alpha1.NonAdminDownloadRequestSpec{
					Target: velerov1.DownloadTarget{
						Kind: velerov1.DownloadTargetKindBackupResults,
						Name: "user1-nab-name",
					},
				},
			})
			// TODO: err might occur if backup is not yet completed but we can passthrough velero behavior
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
			// Example: If you expect a certain status condition after reconciliation, verify it here.
		})
	})
})
