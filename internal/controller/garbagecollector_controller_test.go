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
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	nacv1alpha1 "github.com/migtools/oadp-non-admin/api/v1alpha1"
	"github.com/migtools/oadp-non-admin/internal/common/constant"
)

type garbageCollectorFullReconcileScenario struct {
	backups             int
	restores            int
	orphanBackups       int
	orphanRestores      int
	orphanNaBSLRequests int
	errorLogs           int
}

const fakeUUID = "12345678-4321-1234-4321-123456789abc"

func buildTestBackup(namespace string, name string, nonAdminNamespace string) *velerov1.Backup {
	return &velerov1.Backup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				constant.OadpLabel:             constant.OadpLabelValue,
				constant.ManagedByLabel:        constant.ManagedByLabelValue,
				constant.NabOriginNACUUIDLabel: fakeUUID,
			},
			Annotations: map[string]string{
				constant.NabOriginNamespaceAnnotation: nonAdminNamespace,
				constant.NabOriginNameAnnotation:      "non-existent",
			},
		},
		Spec: velerov1.BackupSpec{},
	}
}

func buildTestRestore(namespace string, name string, nonAdminNamespace string) *velerov1.Restore {
	return &velerov1.Restore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				constant.OadpLabel:             constant.OadpLabelValue,
				constant.ManagedByLabel:        constant.ManagedByLabelValue,
				constant.NarOriginNACUUIDLabel: fakeUUID,
			},
			Annotations: map[string]string{
				constant.NarOriginNamespaceAnnotation: nonAdminNamespace,
				constant.NarOriginNameAnnotation:      "non-existent",
			},
		},
		Spec: velerov1.RestoreSpec{},
	}
}

var _ = ginkgo.Describe("Test full reconcile loop of GarbageCollector Controller", func() {
	var (
		ctx               context.Context
		cancel            context.CancelFunc
		nonAdminNamespace string
		oadpNamespace     string
		counter           int
	)

	ginkgo.BeforeEach(func() {
		counter++
		nonAdminNamespace = fmt.Sprintf("test-garbage-collector-reconcile-full-%v", counter)
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
		func(scenario garbageCollectorFullReconcileScenario) {
			ctx, cancel = context.WithCancel(context.Background())

			gomega.Expect(createTestNamespaces(ctx, nonAdminNamespace, oadpNamespace)).To(gomega.Succeed())

			for index := range scenario.backups {
				backup := buildTestBackup(oadpNamespace, fmt.Sprintf("test-backup-%v", index), nonAdminNamespace)
				backup.Labels = map[string]string{}
				backup.Annotations = map[string]string{}
				gomega.Expect(k8sClient.Create(ctx, backup)).To(gomega.Succeed())
			}
			for index := range scenario.restores {
				restore := buildTestRestore(oadpNamespace, fmt.Sprintf("test-restore-%v", index), nonAdminNamespace)
				restore.Labels = map[string]string{}
				restore.Annotations = map[string]string{}
				gomega.Expect(k8sClient.Create(ctx, restore)).To(gomega.Succeed())
			}
			for index := range scenario.orphanBackups {
				gomega.Expect(k8sClient.Create(ctx, buildTestBackup(oadpNamespace, fmt.Sprintf("test-garbage-collector-backup-%v", index), nonAdminNamespace))).To(gomega.Succeed())
			}
			for index := range scenario.orphanRestores {
				gomega.Expect(k8sClient.Create(ctx, buildTestRestore(oadpNamespace, fmt.Sprintf("test-garbage-collector-restore-%v", index), nonAdminNamespace))).To(gomega.Succeed())
			}

			backupsInOADPNamespace := &velerov1.BackupList{}
			gomega.Expect(k8sClient.List(ctx, backupsInOADPNamespace, client.InNamespace(oadpNamespace))).To(gomega.Succeed())
			gomega.Expect(backupsInOADPNamespace.Items).To(gomega.HaveLen(scenario.backups + scenario.orphanBackups))

			restoresInOADPNamespace := &velerov1.RestoreList{}
			gomega.Expect(k8sClient.List(ctx, restoresInOADPNamespace, client.InNamespace(oadpNamespace))).To(gomega.Succeed())
			gomega.Expect(restoresInOADPNamespace.Items).To(gomega.HaveLen(scenario.restores + scenario.orphanRestores))

			k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
				Scheme: k8sClient.Scheme(),
				Cache: cache.Options{
					DefaultNamespaces: map[string]cache.Config{
						nonAdminNamespace: {},
						oadpNamespace:     {},
					},
				},
			})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			err = (&GarbageCollectorReconciler{
				Client:        k8sManager.GetClient(),
				Scheme:        k8sManager.GetScheme(),
				OADPNamespace: oadpNamespace,
				Frequency:     2 * time.Second,
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
				startUpLog := `INFO	Starting workers	{"controller": "nonadmingarbagecollector", "worker count": 1}`
				return strings.Contains(logOutput, startUpLog) &&
					strings.Count(logOutput, startUpLog) == 1, nil
			}, 5*time.Second, 1*time.Second).Should(gomega.BeTrue())

			go func() {
				defer ginkgo.GinkgoRecover()
				for index := range 5 {
					bsl := &velerov1.BackupStorageLocation{
						ObjectMeta: metav1.ObjectMeta{
							Name:      fmt.Sprintf("test-garbage-collector-bsl-%v", index),
							Namespace: oadpNamespace,
							Labels: map[string]string{
								constant.OadpLabel:               constant.OadpLabelValue,
								constant.ManagedByLabel:          constant.ManagedByLabelValue,
								constant.NabslOriginNACUUIDLabel: fakeUUID,
							},
							Annotations: map[string]string{
								constant.NabslOriginNamespaceAnnotation: nonAdminNamespace,
								constant.NabslOriginNameAnnotation:      "non-existent",
							},
						},
						Spec: velerov1.BackupStorageLocationSpec{
							StorageType: velerov1.StorageType{
								ObjectStorage: &velerov1.ObjectStorageLocation{
									Bucket: "example-bucket",
									Prefix: "test",
								},
							},
						},
					}
					gomega.Expect(k8sClient.Create(ctx, bsl)).To(gomega.Succeed())
					time.Sleep(1 * time.Second)
				}
			}()

			go func() {
				defer ginkgo.GinkgoRecover()
				for index := range 2 {
					nabslRequest := &nacv1alpha1.NonAdminBackupStorageLocationRequest{
						ObjectMeta: metav1.ObjectMeta{
							Name:      fmt.Sprintf("test-garbage-collector-nabslrequest-%v", index),
							Namespace: oadpNamespace,
							Labels: map[string]string{
								constant.OadpLabel:               constant.OadpLabelValue,
								constant.ManagedByLabel:          constant.ManagedByLabelValue,
								constant.NabslOriginNACUUIDLabel: fakeUUID,
							},
							Annotations: map[string]string{
								constant.NabslOriginNamespaceAnnotation: nonAdminNamespace,
								constant.NabslOriginNameAnnotation:      "non-existent",
							},
						},
						Spec: nacv1alpha1.NonAdminBackupStorageLocationRequestSpec{
							ApprovalDecision: nacv1alpha1.NonAdminBSLRequestApproved,
						},
						Status: nacv1alpha1.NonAdminBackupStorageLocationRequestStatus{
							VeleroBackupStorageLocationRequest: &nacv1alpha1.VeleroBackupStorageLocationRequest{},
						},
					}
					gomega.Expect(k8sClient.Create(ctx, nabslRequest)).To(gomega.Succeed())
					time.Sleep(1 * time.Second)
				}
			}()

			go func() {
				defer ginkgo.GinkgoRecover()
				for index := range 2 {
					nabslRequest := &nacv1alpha1.NonAdminBackupStorageLocationRequest{
						ObjectMeta: metav1.ObjectMeta{
							Name:      fmt.Sprintf("test-garbage-collector-nabslrequest-with-status-%v", index),
							Namespace: oadpNamespace,
							Labels: map[string]string{
								constant.OadpLabel:               constant.OadpLabelValue,
								constant.ManagedByLabel:          constant.ManagedByLabelValue,
								constant.NabslOriginNACUUIDLabel: fakeUUID,
							},
							Annotations: map[string]string{
								constant.NabslOriginNamespaceAnnotation: nonAdminNamespace,
								constant.NabslOriginNameAnnotation:      "non-existent",
							},
						},
						Spec: nacv1alpha1.NonAdminBackupStorageLocationRequestSpec{
							ApprovalDecision: nacv1alpha1.NonAdminBSLRequestApproved,
						},
						Status: nacv1alpha1.NonAdminBackupStorageLocationRequestStatus{
							VeleroBackupStorageLocationRequest: &nacv1alpha1.VeleroBackupStorageLocationRequest{
								Namespace: oadpNamespace,
								Name:      fmt.Sprintf("test-garbage-collector-nabsl-%v", index),
								RequestedSpec: &velerov1.BackupStorageLocationSpec{
									StorageType: velerov1.StorageType{
										ObjectStorage: &velerov1.ObjectStorageLocation{
											Bucket: "example-bucket",
											Prefix: "test",
										},
									},
								},
							},
						},
					}
					gomega.Expect(k8sClient.Create(ctx, nabslRequest)).To(gomega.Succeed())
					time.Sleep(1 * time.Second)
				}
			}()

			go func() {
				defer ginkgo.GinkgoRecover()
				for index := range 3 {
					secret := &corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      fmt.Sprintf("test-garbage-collector-secret-%v", index),
							Namespace: oadpNamespace,
							Labels: map[string]string{
								constant.OadpLabel:               constant.OadpLabelValue,
								constant.ManagedByLabel:          constant.ManagedByLabelValue,
								constant.NabslOriginNACUUIDLabel: fakeUUID,
							},
							Annotations: map[string]string{
								constant.NabslOriginNamespaceAnnotation: nonAdminNamespace,
								constant.NabslOriginNameAnnotation:      "non-existent",
							},
						},
					}
					gomega.Expect(k8sClient.Create(ctx, secret)).To(gomega.Succeed())
					time.Sleep(1 * time.Second)
				}
			}()

			time.Sleep(8 * time.Second)
			gomega.Expect(strings.Count(ginkgo.CurrentSpecReport().CapturedGinkgoWriterOutput, "orphan Secret deleted")).Should(gomega.Equal(3))
			gomega.Expect(strings.Count(ginkgo.CurrentSpecReport().CapturedGinkgoWriterOutput, "orphan BackupStorageLocation deleted")).Should(gomega.Equal(5))
			gomega.Expect(strings.Count(ginkgo.CurrentSpecReport().CapturedGinkgoWriterOutput, "orphan NonAdminBackupStorageLocationRequest deleted")).Should(gomega.Equal(scenario.orphanNaBSLRequests))
			gomega.Expect(strings.Count(ginkgo.CurrentSpecReport().CapturedGinkgoWriterOutput, "orphan Backup deleted")).Should(gomega.Equal(scenario.orphanBackups))
			gomega.Expect(strings.Count(ginkgo.CurrentSpecReport().CapturedGinkgoWriterOutput, "orphan Restore deleted")).Should(gomega.Equal(scenario.orphanRestores))
			gomega.Expect(strings.Count(ginkgo.CurrentSpecReport().CapturedGinkgoWriterOutput, "Garbage Collector Reconcile start")).Should(gomega.Equal(5))
			gomega.Expect(strings.Count(ginkgo.CurrentSpecReport().CapturedGinkgoWriterOutput, "ERROR")).Should(gomega.Equal(scenario.errorLogs))

			gomega.Expect(k8sClient.List(ctx, backupsInOADPNamespace, client.InNamespace(oadpNamespace))).To(gomega.Succeed())
			gomega.Expect(backupsInOADPNamespace.Items).To(gomega.HaveLen(scenario.backups))

			gomega.Expect(k8sClient.List(ctx, restoresInOADPNamespace, client.InNamespace(oadpNamespace))).To(gomega.Succeed())
			gomega.Expect(restoresInOADPNamespace.Items).To(gomega.HaveLen(scenario.restores))
		},
		ginkgo.Entry("Should delete orphaned Velero resources and then watch them periodically", garbageCollectorFullReconcileScenario{
			backups:             2,
			restores:            3,
			orphanBackups:       4,
			orphanRestores:      1,
			orphanNaBSLRequests: 4,
		}),
	)
})
