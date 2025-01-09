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
	"fmt"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	ctrl "sigs.k8s.io/controller-runtime"

	nacv1alpha1 "github.com/migtools/oadp-non-admin/api/v1alpha1"
)

type nonAdminBackupStorageLocationFullReconcileScenario struct {
	spec nacv1alpha1.NonAdminBackupStorageLocationSpec
}

func buildTestNonAdminBackupStorageLocation(nonAdminNamespace string, nonAdminName string, spec nacv1alpha1.NonAdminBackupStorageLocationSpec) *nacv1alpha1.NonAdminBackupStorageLocation {
	return &nacv1alpha1.NonAdminBackupStorageLocation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nonAdminName,
			Namespace: nonAdminNamespace,
		},
		Spec: spec,
	}
}

var _ = ginkgo.Describe("Test full reconcile loop of NonAdminBackupSyncReconciler Controller", func() {
	var (
		ctx                                    context.Context
		cancel                                 context.CancelFunc
		nonAdminBackupStorageLocationName      string
		nonAdminBackupStorageLocationNamespace string
		oadpNamespace                          string
		counter                                int
	)

	ginkgo.BeforeEach(func() {
		counter++
		nonAdminBackupStorageLocationName = fmt.Sprintf("non-admin-bsl-object-%v", counter)
		nonAdminBackupStorageLocationNamespace = fmt.Sprintf("test-non-admin-bsl-reconcile-full-%v", counter)
		oadpNamespace = nonAdminBackupStorageLocationNamespace + "-oadp"
	})

	ginkgo.AfterEach(func() {
		gomega.Expect(deleteTestNamespaces(ctx, nonAdminBackupStorageLocationNamespace, oadpNamespace)).To(gomega.Succeed())

		cancel()
		// wait cancel
		time.Sleep(1 * time.Second)
	})

	ginkgo.FDescribeTable("Reconcile triggered by NonAdminBackupStorageLocation Create event",
		func(scenario nonAdminBackupStorageLocationFullReconcileScenario) {
			ctx, cancel = context.WithCancel(context.Background())

			gomega.Expect(createTestNamespaces(ctx, nonAdminBackupStorageLocationNamespace, oadpNamespace)).To(gomega.Succeed())

			k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
				Scheme: k8sClient.Scheme(),
			})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			err = (&NonAdminBackupSyncReconciler{
				Client:        k8sManager.GetClient(),
				Scheme:        k8sManager.GetScheme(),
				OADPNamespace: oadpNamespace,
				SyncPeriod:    3 * time.Second,
			}).SetupWithManager(k8sManager)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			go func() {
				defer ginkgo.GinkgoRecover()
				err = k8sManager.Start(ctx)
				gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to run manager")
			}()
			// wait manager start
			managerStartTimeout := 30 * time.Second
			pollInterval := 100 * time.Millisecond
			ctxTimeout, cancel := context.WithTimeout(ctx, managerStartTimeout)
			defer cancel()

			// TODO read logs
			err = wait.PollUntilContextTimeout(ctxTimeout, pollInterval, managerStartTimeout, true, func(ctx context.Context) (done bool, err error) {
				select {
				case <-ctx.Done():
					return false, ctx.Err()
				default:
					// Check if the manager has started by verifying if the client is initialized
					return k8sManager.GetClient() != nil, nil
				}
			})
			// Check if the context timeout or another error occurred
			gomega.Expect(err).ToNot(gomega.HaveOccurred(), "Manager failed to start within the timeout period")

			ginkgo.By("Waiting Reconcile of create event")
			nonAdminBackupStorageLocation := buildTestNonAdminBackupStorageLocation(nonAdminBackupStorageLocationNamespace, nonAdminBackupStorageLocationName, scenario.spec)
			gomega.Expect(k8sClient.Create(ctxTimeout, nonAdminBackupStorageLocation)).To(gomega.Succeed())
			// wait NonAdminRestore reconcile
			time.Sleep(13 * time.Second)

			// TODO check synced NonAdminBackups specs are expected

			ginkgo.By("Waiting NonAdminBackupStorageLocation deletion")
			gomega.Expect(k8sClient.Delete(ctxTimeout, nonAdminBackupStorageLocation)).To(gomega.Succeed())
			gomega.Eventually(func() (bool, error) {
				err := k8sClient.Get(
					ctxTimeout,
					types.NamespacedName{
						Name:      nonAdminBackupStorageLocationName,
						Namespace: nonAdminBackupStorageLocationNamespace,
					},
					nonAdminBackupStorageLocation,
				)
				if apierrors.IsNotFound(err) {
					return true, nil
				}
				return false, err
			}, 10*time.Second, 1*time.Second).Should(gomega.BeTrue())
		},
		ginkgo.Entry("Should sync NonAdminBackup 5 times, then delete NonAdminBackupStorageLocation", nonAdminBackupStorageLocationFullReconcileScenario{
			spec: nacv1alpha1.NonAdminBackupStorageLocationSpec{
				BackupStorageLocationSpec: velerov1.BackupStorageLocationSpec{
					Provider: "aws",
					StorageType: velerov1.StorageType{
						ObjectStorage: &velerov1.ObjectStorageLocation{
							Bucket: "my-bucket-name",
							Prefix: "velero",
						},
					},
					Config: map[string]string{
						"bucket": "my-bucket-name",
						"prefix": "velero",
					},
					Credential: &v1.SecretKeySelector{
						LocalObjectReference: v1.LocalObjectReference{
							Name: "cloud-credentials",
						},
						Key: "cloud",
					},
				},
			},
		}),
		// ginkgo.Entry("Should check that NonAdminBackupStorageLocation update changes next sync time, then delete NonAdminBackupStorageLocation", nonAdminBackupStorageLocationFullReconcileScenario{}),
		// ginkgo.Entry("Should sync only finished NonAdminBackups, then delete NonAdminBackupStorageLocation", nonAdminBackupStorageLocationFullReconcileScenario{}),
		// ginkgo.Entry("Should sync only related NonAdminBackupStorageLocation NonAdminBackups, then delete NonAdminBackupStorageLocation", nonAdminBackupStorageLocationFullReconcileScenario{}),
	)
})
