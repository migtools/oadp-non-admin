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
	"os"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	nacv1alpha1 "github.com/migtools/oadp-non-admin/api/v1alpha1"
	"github.com/migtools/oadp-non-admin/internal/common/constant"
)

const testNonAdminBackupName = "test-non-admin-backup"

type nonAdminBackupReconcileScenario struct {
	namespace     string
	oadpNamespace string
	spec          nacv1alpha1.NonAdminBackupSpec
	priorStatus   *nacv1alpha1.NonAdminBackupStatus
	status        nacv1alpha1.NonAdminBackupStatus
	result        reconcile.Result
}

func createTestNonAdminBackup(namespace string, spec nacv1alpha1.NonAdminBackupSpec) *nacv1alpha1.NonAdminBackup {
	return &nacv1alpha1.NonAdminBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testNonAdminBackupName,
			Namespace: namespace,
		},
		Spec: spec,
	}
}

var _ = ginkgo.Describe("Test single reconciles of NonAdminBackup Reconcile function", func() {
	var (
		ctx                 = context.Background()
		currentTestScenario nonAdminBackupReconcileScenario
		updateTestScenario  = func(scenario nonAdminBackupReconcileScenario) {
			currentTestScenario = scenario
		}
	)

	ginkgo.AfterEach(func() {
		gomega.Expect(os.Unsetenv(constant.NamespaceEnvVar)).To(gomega.Succeed())
		if len(currentTestScenario.oadpNamespace) > 0 {
			oadpNamespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: currentTestScenario.oadpNamespace,
				},
			}
			gomega.Expect(k8sClient.Delete(ctx, oadpNamespace)).To(gomega.Succeed())
		}

		nonAdminBackup := &nacv1alpha1.NonAdminBackup{}
		if k8sClient.Get(
			ctx,
			types.NamespacedName{
				Name:      testNonAdminBackupName,
				Namespace: currentTestScenario.namespace,
			},
			nonAdminBackup,
		) == nil {
			gomega.Expect(k8sClient.Delete(ctx, nonAdminBackup)).To(gomega.Succeed())
		}

		namespace := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: currentTestScenario.namespace,
			},
		}
		gomega.Expect(k8sClient.Delete(ctx, namespace)).To(gomega.Succeed())
	})

	ginkgo.DescribeTable("Reconcile should NOT return an error on Delete event",
		func(scenario nonAdminBackupReconcileScenario) {
			updateTestScenario(scenario)

			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: scenario.namespace,
				},
			}
			gomega.Expect(k8sClient.Create(ctx, namespace)).To(gomega.Succeed())

			result, err := (&NonAdminBackupReconciler{
				Client: k8sClient,
				Scheme: testEnv.Scheme,
			}).Reconcile(
				context.Background(),
				reconcile.Request{NamespacedName: types.NamespacedName{
					Namespace: scenario.namespace,
					Name:      testNonAdminBackupName,
				}},
			)

			gomega.Expect(result).To(gomega.Equal(scenario.result))
			gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))
		},
		ginkgo.Entry("Should accept deletion of NonAdminBackup", nonAdminBackupReconcileScenario{
			namespace: "test-nonadminbackup-reconcile-0",
			result:    reconcile.Result{},
		}),
	)

	ginkgo.DescribeTable("Reconcile should NOT return an error on Create and Update events",
		func(scenario nonAdminBackupReconcileScenario) {
			updateTestScenario(scenario)

			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: scenario.namespace,
				},
			}
			gomega.Expect(k8sClient.Create(ctx, namespace)).To(gomega.Succeed())

			nonAdminBackup := createTestNonAdminBackup(scenario.namespace, scenario.spec)
			gomega.Expect(k8sClient.Create(ctx, nonAdminBackup)).To(gomega.Succeed())

			if scenario.priorStatus != nil {
				nonAdminBackup.Status = *scenario.priorStatus
				gomega.Expect(k8sClient.Status().Update(ctx, nonAdminBackup)).To(gomega.Succeed())
			}

			if len(scenario.oadpNamespace) > 0 {
				gomega.Expect(os.Setenv(constant.NamespaceEnvVar, scenario.oadpNamespace)).To(gomega.Succeed())
				oadpNamespace := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: scenario.oadpNamespace,
					},
				}
				gomega.Expect(k8sClient.Create(ctx, oadpNamespace)).To(gomega.Succeed())
			}

			result, err := (&NonAdminBackupReconciler{
				Client: k8sClient,
				Scheme: testEnv.Scheme,
			}).Reconcile(
				context.Background(),
				reconcile.Request{NamespacedName: types.NamespacedName{
					Namespace: scenario.namespace,
					Name:      testNonAdminBackupName,
				}},
			)
			// TODO need to collect logs, so they do not appear in test run
			// also assert them
			gomega.Expect(result).To(gomega.Equal(scenario.result))
			gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))

			// if !scenario.doNotCreateNonAdminBackup {
			// nonAdminBackup := &nacv1alpha1.NonAdminBackup{}
			// gomega.Eventually(func() nacv1alpha1.NonAdminBackupPhase {
			// 	k8sClient.Get(
			// 		ctx,
			// 		types.NamespacedName{
			// 			Name:      testNonAdminBackupName,
			// 			Namespace: currentTestScenario.namespace,
			// 		},
			// 		nonAdminBackup,
			// 	)
			// 	return nonAdminBackup.Status.Phase
			// }, 30*time.Second, 1*time.Second).Should(gomega.Equal(scenario.status.Phase))

			gomega.Expect(k8sClient.Get(
				ctx,
				types.NamespacedName{
					Name:      testNonAdminBackupName,
					Namespace: currentTestScenario.namespace,
				},
				nonAdminBackup,
			)).To(gomega.Succeed())
			gomega.Expect(nonAdminBackup.Status.Phase).To(gomega.Equal(scenario.status.Phase))
			gomega.Expect(nonAdminBackup.Status.VeleroBackupName).To(gomega.Equal(scenario.status.VeleroBackupName))
			gomega.Expect(nonAdminBackup.Status.VeleroBackupNamespace).To(gomega.Equal(scenario.status.VeleroBackupNamespace))
			gomega.Expect(nonAdminBackup.Status.VeleroBackupStatus).To(gomega.Equal(scenario.status.VeleroBackupStatus))

			for index := range nonAdminBackup.Status.Conditions {
				gomega.Expect(nonAdminBackup.Status.Conditions[index].Type).To(gomega.Equal(scenario.status.Conditions[index].Type))
				gomega.Expect(nonAdminBackup.Status.Conditions[index].Status).To(gomega.Equal(scenario.status.Conditions[index].Status))
				gomega.Expect(nonAdminBackup.Status.Conditions[index].Reason).To(gomega.Equal(scenario.status.Conditions[index].Reason))
				gomega.Expect(nonAdminBackup.Status.Conditions[index].Message).To(gomega.Equal(scenario.status.Conditions[index].Message))
			}
		},
		ginkgo.Entry("Should accept creation of NonAdminBackup", nonAdminBackupReconcileScenario{
			namespace: "test-nonadminbackup-reconcile-1",
			// even without providing spec, this does not fail...
			result: reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second},
			status: nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminBackupPhaseNew,
			},
		}),
		ginkgo.Entry("Should accept update of NonAdminBackup phase", nonAdminBackupReconcileScenario{
			namespace: "test-nonadminbackup-reconcile-2",
			spec: nacv1alpha1.NonAdminBackupSpec{
				BackupSpec: &v1.BackupSpec{},
			},
			priorStatus: &nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminBackupPhaseNew,
			},
			result: reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second},
			status: nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminBackupPhaseNew,
				Conditions: []metav1.Condition{
					// Is this a valid Condition???
					{
						Type:    "Accepted",
						Status:  metav1.ConditionTrue,
						Reason:  "Validated",
						Message: "Valid Backup config",
					},
				},
			},
		}),
		ginkgo.Entry("Should accept update of NonAdminBackup Condition", nonAdminBackupReconcileScenario{
			namespace:     "test-nonadminbackup-reconcile-3",
			oadpNamespace: "test-nonadminbackup-reconcile-3-oadp",
			spec: nacv1alpha1.NonAdminBackupSpec{
				BackupSpec: &v1.BackupSpec{},
			},
			priorStatus: &nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminBackupPhaseNew,
				Conditions: []metav1.Condition{
					// Is this a valid Condition???
					{
						Type:               "Accepted",
						Status:             metav1.ConditionTrue,
						Reason:             "Validated",
						Message:            "Valid Backup config",
						LastTransitionTime: metav1.NewTime(time.Now()),
					},
				},
			},
			status: nacv1alpha1.NonAdminBackupStatus{
				// TODO should not have VeleroBackupName and VeleroBackupNamespace?
				Phase: nacv1alpha1.NonAdminBackupPhaseCreated,
				Conditions: []metav1.Condition{
					{
						Type:    "Accepted",
						Status:  metav1.ConditionTrue,
						Reason:  "BackupAccepted",
						Message: "Backup accepted",
					},
					{
						Type:    "Queued",
						Status:  metav1.ConditionTrue,
						Reason:  "BackupScheduled",
						Message: "Created Velero Backup object",
					},
				},
			},
		}),
		ginkgo.Entry("Should NOT accept update of NonAdminBackup phase because of empty backupSpec", nonAdminBackupReconcileScenario{
			namespace: "test-nonadminbackup-reconcile-4",
			spec:      nacv1alpha1.NonAdminBackupSpec{},
			priorStatus: &nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminBackupPhaseNew,
			},
			status: nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminBackupPhaseBackingOff,
			},
		}),
		ginkgo.Entry("Should NOT accept update of NonAdminBackup phase because of includedNamespaces pointing to different namespace", nonAdminBackupReconcileScenario{
			namespace: "test-nonadminbackup-reconcile-5",
			spec: nacv1alpha1.NonAdminBackupSpec{
				BackupSpec: &v1.BackupSpec{
					IncludedNamespaces: []string{"not-valid"},
				},
			},
			priorStatus: &nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminBackupPhaseNew,
			},
			status: nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminBackupPhaseBackingOff,
			},
		}),
	)
})

var _ = ginkgo.Describe("Test full reconciles of NonAdminBackup Reconcile function", func() {
	var (
		ctx, cancel         = context.WithCancel(context.Background())
		currentTestScenario nonAdminBackupReconcileScenario
		updateTestScenario  = func(scenario nonAdminBackupReconcileScenario) {
			currentTestScenario = scenario
		}
	)

	ginkgo.AfterEach(func() {
		gomega.Expect(os.Unsetenv(constant.NamespaceEnvVar)).To(gomega.Succeed())
		if len(currentTestScenario.oadpNamespace) > 0 {
			oadpNamespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: currentTestScenario.oadpNamespace,
				},
			}
			gomega.Expect(k8sClient.Delete(ctx, oadpNamespace)).To(gomega.Succeed())
		}

		// nonAdminBackup := &nacv1alpha1.NonAdminBackup{}
		// if k8sClient.Get(
		// 	ctx,
		// 	types.NamespacedName{
		// 		Name:      testNonAdminBackupName,
		// 		Namespace: currentTestScenario.namespace,
		// 	},
		// 	nonAdminBackup,
		// ) == nil {
		// 	gomega.Expect(k8sClient.Delete(ctx, nonAdminBackup)).To(gomega.Succeed())
		// }

		namespace := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: currentTestScenario.namespace,
			},
		}
		gomega.Expect(k8sClient.Delete(ctx, namespace)).To(gomega.Succeed())
		cancel()
	})

	ginkgo.DescribeTable("Reconcile should NOT return an error",
		func(scenario nonAdminBackupReconcileScenario) {
			updateTestScenario(scenario)

			gomega.Expect(os.Setenv(constant.NamespaceEnvVar, scenario.oadpNamespace)).To(gomega.Succeed())
			oadpNamespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: scenario.oadpNamespace,
				},
			}
			gomega.Expect(k8sClient.Create(ctx, oadpNamespace)).To(gomega.Succeed())

			k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
				Scheme: k8sClient.Scheme(),
			})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			err = (&NonAdminBackupReconciler{
				Client: k8sManager.GetClient(),
				Scheme: k8sManager.GetScheme(),
			}).SetupWithManager(k8sManager)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			// TODO Be CAREFUL about FLAKES with this approach?
			// study ref https://book.kubebuilder.io/cronjob-tutorial/writing-tests
			go func() {
				defer ginkgo.GinkgoRecover()
				err = k8sManager.Start(ctx)
				gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to run manager")
			}()

			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: scenario.namespace,
				},
			}
			gomega.Expect(k8sClient.Create(ctx, namespace)).To(gomega.Succeed())

			nonAdminBackup := createTestNonAdminBackup(scenario.namespace, scenario.spec)
			gomega.Expect(k8sClient.Create(ctx, nonAdminBackup)).To(gomega.Succeed())

			gomega.Eventually(func() (nacv1alpha1.NonAdminBackupPhase, error) {
				err := k8sClient.Get(
					ctx,
					types.NamespacedName{
						Name:      testNonAdminBackupName,
						Namespace: currentTestScenario.namespace,
					},
					nonAdminBackup,
				)
				if err != nil {
					return "", err
				}
				return nonAdminBackup.Status.Phase, nil
				// TOO MUCH TIME!!!!
			}, 30*time.Second, 1*time.Second).Should(gomega.Equal(scenario.status.Phase))

			gomega.Expect(k8sClient.Get(
				ctx,
				types.NamespacedName{
					Name:      testNonAdminBackupName,
					Namespace: currentTestScenario.namespace,
				},
				nonAdminBackup,
			)).To(gomega.Succeed())
			gomega.Expect(nonAdminBackup.Status.Phase).To(gomega.Equal(scenario.status.Phase))
			gomega.Expect(nonAdminBackup.Status.VeleroBackupName).To(gomega.Equal(scenario.status.VeleroBackupName))
			gomega.Expect(nonAdminBackup.Status.VeleroBackupNamespace).To(gomega.Equal(scenario.status.VeleroBackupNamespace))
			gomega.Expect(nonAdminBackup.Status.VeleroBackupStatus).To(gomega.Equal(scenario.status.VeleroBackupStatus))

			for index := range nonAdminBackup.Status.Conditions {
				gomega.Expect(nonAdminBackup.Status.Conditions[index].Type).To(gomega.Equal(scenario.status.Conditions[index].Type))
				gomega.Expect(nonAdminBackup.Status.Conditions[index].Status).To(gomega.Equal(scenario.status.Conditions[index].Status))
				gomega.Expect(nonAdminBackup.Status.Conditions[index].Reason).To(gomega.Equal(scenario.status.Conditions[index].Reason))
				gomega.Expect(nonAdminBackup.Status.Conditions[index].Message).To(gomega.Equal(scenario.status.Conditions[index].Message))
			}
		},
		ginkgo.Entry("Should DO FULL happy path", nonAdminBackupReconcileScenario{
			namespace:     "test-nonadminbackup-reconcile-full-1",
			oadpNamespace: "test-nonadminbackup-reconcile-full-1-oadp",
			spec: nacv1alpha1.NonAdminBackupSpec{
				BackupSpec: &v1.BackupSpec{},
			},
			status: nacv1alpha1.NonAdminBackupStatus{
				// TODO should not have VeleroBackupName and VeleroBackupNamespace?
				Phase: nacv1alpha1.NonAdminBackupPhaseCreated,
				Conditions: []metav1.Condition{
					{
						Type:    "Accepted",
						Status:  metav1.ConditionTrue,
						Reason:  "Validated",
						Message: "Valid Backup config",
					},
					{
						Type:    "Queued",
						Status:  metav1.ConditionTrue,
						Reason:  "BackupScheduled",
						Message: "Created Velero Backup object",
					},
				},
			},
		}),
	)
})
