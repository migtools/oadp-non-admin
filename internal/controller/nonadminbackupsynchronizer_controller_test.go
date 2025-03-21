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
	"strconv"
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	nacv1alpha1 "github.com/migtools/oadp-non-admin/api/v1alpha1"
	"github.com/migtools/oadp-non-admin/internal/common/constant"
	"github.com/migtools/oadp-non-admin/internal/common/function"
)

type nonAdminBackupSynchronizerFullReconcileScenario struct {
	backupsToCreate []backupToCreate
	errorLogs       int
}

type backupToCreate struct {
	nonAdminBSL                   bool
	namespaceExist                bool
	withNonAdminLabelsAnnotations bool
	finished                      bool
}

var _ = ginkgo.Describe("Test full reconcile loop of NonAdminBackup Synchronizer Controller", func() {
	var (
		ctx               context.Context
		cancel            context.CancelFunc
		nonAdminNamespace string
		oadpNamespace     string
		counter           int
	)

	ginkgo.BeforeEach(func() {
		counter++
		nonAdminNamespace = fmt.Sprintf("test-non-admin-backup-synchronizer-reconcile-full-%v", counter)
		oadpNamespace = nonAdminNamespace + "-oadp"
	})

	ginkgo.AfterEach(func() {
		gomega.Expect(deleteTestNamespaces(ctx, nonAdminNamespace, oadpNamespace)).To(gomega.Succeed())

		cancel()

		// wait manager shutdown
		gomega.Eventually(func() (bool, error) {
			logOutput := ginkgo.CurrentSpecReport().CapturedGinkgoWriterOutput
			shutdownlog := "INFO	Wait completed, proceeding to shutdown the manager"
			return strings.Contains(logOutput, shutdownlog) && strings.Count(logOutput, shutdownlog) == 1, nil
		}, 5*time.Second, 1*time.Second).Should(gomega.BeTrue())
	})

	ginkgo.DescribeTable("Reconcile triggered by NAC Pod start up",
		func(scenario nonAdminBackupSynchronizerFullReconcileScenario) {
			ctx, cancel = context.WithCancel(context.Background())

			gomega.Expect(createTestNamespaces(ctx, nonAdminNamespace, oadpNamespace)).To(gomega.Succeed())

			defaultBSL := &velerov1.BackupStorageLocation{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-default-bsl",
					Namespace: oadpNamespace,
				},
				Spec: velerov1.BackupStorageLocationSpec{
					StorageType: velerov1.StorageType{
						ObjectStorage: &velerov1.ObjectStorageLocation{
							Bucket: "example-bucket",
						},
					},
					Default: true,
				},
			}
			gomega.Expect(k8sClient.Create(ctx, defaultBSL)).To(gomega.Succeed())

			nonAdminBSL := buildTestNonAdminBackupStorageLocation(nonAdminNamespace, "test-non-admin-bsl", nacv1alpha1.NonAdminBackupStorageLocationSpec{
				BackupStorageLocationSpec: &velerov1.BackupStorageLocationSpec{
					StorageType: velerov1.StorageType{
						ObjectStorage: &velerov1.ObjectStorageLocation{
							Bucket: "another-bucket",
						},
					},
				},
			})
			gomega.Expect(k8sClient.Create(ctx, nonAdminBSL)).To(gomega.Succeed())

			bslForNonAdminBSL := &velerov1.BackupStorageLocation{
				ObjectMeta: metav1.ObjectMeta{
					Name:        fakeUUID,
					Namespace:   oadpNamespace,
					Labels:      function.GetNonAdminLabels(),
					Annotations: function.GetNonAdminBackupStorageLocationAnnotations(nonAdminBSL.ObjectMeta),
				},
				Spec: velerov1.BackupStorageLocationSpec{
					StorageType: velerov1.StorageType{
						ObjectStorage: &velerov1.ObjectStorageLocation{
							Bucket: "another-bucket",
						},
					},
				},
			}
			bslForNonAdminBSL.Labels[constant.NabslOriginNACUUIDLabel] = fakeUUID
			gomega.Expect(k8sClient.Create(ctx, bslForNonAdminBSL)).To(gomega.Succeed())

			for index, create := range scenario.backupsToCreate {
				backup := buildTestBackup(oadpNamespace, fmt.Sprintf("test-backup-%v", index), nonAdminNamespace)
				backup.Annotations[constant.NabOriginNameAnnotation] = fmt.Sprintf("test-non-admin-backup-%v", index)
				if create.nonAdminBSL {
					backup.Spec.StorageLocation = bslForNonAdminBSL.Name
				} else {
					backup.Spec.StorageLocation = defaultBSL.Name
				}
				if !create.namespaceExist {
					backup.Annotations[constant.NabOriginNamespaceAnnotation] = "non-existent"
				}
				if !create.withNonAdminLabelsAnnotations {
					backup.Labels = map[string]string{}
					backup.Annotations = map[string]string{}
				}
				gomega.Expect(k8sClient.Create(ctx, backup)).To(gomega.Succeed())

				if create.finished {
					backup.Status = velerov1.BackupStatus{
						Phase:               velerov1.BackupPhaseCompleted,
						CompletionTimestamp: &metav1.Time{Time: time.Now()},
					}
					// can not call .Status().Update() for veleroBackup object https://github.com/vmware-tanzu/velero/issues/8285
					gomega.Expect(k8sClient.Update(ctx, backup)).To(gomega.Succeed())
				}
			}

			nonAdminBackupsInNonAminNamespace := &nacv1alpha1.NonAdminBackupList{}
			gomega.Expect(k8sClient.List(ctx, nonAdminBackupsInNonAminNamespace, client.InNamespace(nonAdminNamespace))).To(gomega.Succeed())
			gomega.Expect(nonAdminBackupsInNonAminNamespace.Items).To(gomega.BeEmpty())

			k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
				Scheme: k8sClient.Scheme(),
				Cache: cache.Options{
					DefaultNamespaces: map[string]cache.Config{
						nonAdminNamespace: {},
						oadpNamespace:     {},
						"non-existent":    {},
					},
				},
			})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			name := "nabsync-test-reconciler-" + strconv.Itoa(counter)
			err = (&NonAdminBackupSynchronizerReconciler{
				Client:        k8sManager.GetClient(),
				Scheme:        k8sManager.GetScheme(),
				OADPNamespace: oadpNamespace,
				SyncPeriod:    2 * time.Second,
				Name:          name,
			}).SetupWithManager(k8sManager)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			go func() {
				defer ginkgo.GinkgoRecover()
				err = k8sManager.Start(ctx)
				gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to run manager")
			}()
			// wait manager start
			gomega.Eventually(func() (bool, error) {
				logOutput := ginkgo.CurrentSpecReport().CapturedGinkgoWriterOutput
				startUpLog := `INFO	Starting workers	{"controller": "` + name + `", "worker count": 1}`
				return strings.Contains(logOutput, startUpLog) &&
					strings.Count(logOutput, startUpLog) == 1, nil
			}, 5*time.Second, 1*time.Second).Should(gomega.BeTrue())

			time.Sleep(8 * time.Second)
			gomega.Expect(strings.Count(ginkgo.CurrentSpecReport().CapturedGinkgoWriterOutput, "NonAdminBackup Synchronization start")).Should(gomega.Equal(5))
			gomega.Expect(strings.Count(ginkgo.CurrentSpecReport().CapturedGinkgoWriterOutput, "4 possible Backup(s) to be synced to NonAdmin namespaces")).Should(gomega.Equal(5))
			gomega.Expect(strings.Count(ginkgo.CurrentSpecReport().CapturedGinkgoWriterOutput, "2 Backup(s) to sync to NonAdmin namespaces")).Should(gomega.Equal(1))
			gomega.Expect(strings.Count(ginkgo.CurrentSpecReport().CapturedGinkgoWriterOutput, "0 Backup(s) to sync to NonAdmin namespaces")).Should(gomega.Equal(4))
			gomega.Expect(strings.Count(ginkgo.CurrentSpecReport().CapturedGinkgoWriterOutput, "ERROR")).Should(gomega.Equal(scenario.errorLogs))

			gomega.Expect(k8sClient.List(ctx, nonAdminBackupsInNonAminNamespace, client.InNamespace(nonAdminNamespace))).To(gomega.Succeed())
			gomega.Expect(nonAdminBackupsInNonAminNamespace.Items).To(gomega.HaveLen(2))
		},
		ginkgo.Entry("Should sync NonAdminBackups to non admin namespace", nonAdminBackupSynchronizerFullReconcileScenario{
			backupsToCreate: []backupToCreate{
				{
					nonAdminBSL:                   false,
					namespaceExist:                true,
					withNonAdminLabelsAnnotations: true,
					finished:                      true,
				},
				{
					nonAdminBSL:                   false,
					namespaceExist:                false,
					withNonAdminLabelsAnnotations: true,
					finished:                      true,
				},
				{
					nonAdminBSL:                   false,
					namespaceExist:                true,
					withNonAdminLabelsAnnotations: false,
					finished:                      true,
				},
				{
					nonAdminBSL:                   false,
					namespaceExist:                true,
					withNonAdminLabelsAnnotations: true,
					finished:                      false,
				},
				{
					nonAdminBSL:                   true,
					namespaceExist:                true,
					withNonAdminLabelsAnnotations: true,
					finished:                      true,
				},
				{
					nonAdminBSL:                   true,
					namespaceExist:                false,
					withNonAdminLabelsAnnotations: true,
					finished:                      true,
				},
				{
					nonAdminBSL:                   true,
					namespaceExist:                true,
					withNonAdminLabelsAnnotations: false,
					finished:                      true,
				},
				{
					nonAdminBSL:                   true,
					namespaceExist:                true,
					withNonAdminLabelsAnnotations: true,
					finished:                      false,
				},
			},
		}),
	)
})
