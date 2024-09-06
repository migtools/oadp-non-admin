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
	"log"
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
	"github.com/migtools/oadp-non-admin/internal/common/function"
)

const testNonAdminBackupName = "test-non-admin-backup"

type nonAdminBackupSingleReconcileScenario struct {
	namespace          string
	oadpNamespace      string
	resultError        error
	priorStatus        *nacv1alpha1.NonAdminBackupStatus
	spec               nacv1alpha1.NonAdminBackupSpec
	status             nacv1alpha1.NonAdminBackupStatus
	result             reconcile.Result
	createVeleroBackup bool
}

type nonAdminBackupFullReconcileScenario struct {
	ctx           context.Context
	cancel        context.CancelFunc
	namespace     string
	oadpNamespace string
	spec          nacv1alpha1.NonAdminBackupSpec
	status        nacv1alpha1.NonAdminBackupStatus
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
		currentTestScenario nonAdminBackupSingleReconcileScenario
		updateTestScenario  = func(scenario nonAdminBackupSingleReconcileScenario) {
			currentTestScenario = scenario
		}
	)

	ginkgo.AfterEach(func() {
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

		if len(currentTestScenario.oadpNamespace) > 0 {
			oadpNamespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: currentTestScenario.oadpNamespace,
				},
			}
			gomega.Expect(k8sClient.Delete(ctx, oadpNamespace)).To(gomega.Succeed())
		}

		namespace := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: currentTestScenario.namespace,
			},
		}
		gomega.Expect(k8sClient.Delete(ctx, namespace)).To(gomega.Succeed())
	})

	ginkgo.DescribeTable("should Reconcile on Delete event",
		func(scenario nonAdminBackupSingleReconcileScenario) {
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
		ginkgo.Entry("Should accept deletion of NonAdminBackup", nonAdminBackupSingleReconcileScenario{
			namespace: "test-nonadminbackup-reconcile-0",
			result:    reconcile.Result{},
		}),
	)

	ginkgo.DescribeTable("should Reconcile on Create and Update events and on Requeue",
		func(scenario nonAdminBackupSingleReconcileScenario) {
			updateTestScenario(scenario)

			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: scenario.namespace,
				},
			}
			gomega.Expect(k8sClient.Create(ctx, namespace)).To(gomega.Succeed())

			if len(scenario.oadpNamespace) > 0 {
				oadpNamespace := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: scenario.oadpNamespace,
					},
				}
				gomega.Expect(k8sClient.Create(ctx, oadpNamespace)).To(gomega.Succeed())
			}

			nonAdminBackup := createTestNonAdminBackup(scenario.namespace, scenario.spec)
			gomega.Expect(k8sClient.Create(ctx, nonAdminBackup)).To(gomega.Succeed())

			if scenario.createVeleroBackup {
				veleroBackup := &v1.Backup{
					ObjectMeta: metav1.ObjectMeta{
						Name:      function.GenerateVeleroBackupName(scenario.namespace, testNonAdminBackupName),
						Namespace: scenario.oadpNamespace,
					},
				}
				gomega.Expect(k8sClient.Create(ctx, veleroBackup)).To(gomega.Succeed())
			}

			if scenario.priorStatus != nil {
				nonAdminBackup.Status = *scenario.priorStatus
				gomega.Expect(k8sClient.Status().Update(ctx, nonAdminBackup)).To(gomega.Succeed())
			}
			// easy hack to test that only one update call happens per reconcile
			// priorResourceVersion, err := strconv.Atoi(nonAdminBackup.ResourceVersion)
			// gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))

			result, err := (&NonAdminBackupReconciler{
				Client:        k8sClient,
				Scheme:        testEnv.Scheme,
				OADPNamespace: scenario.oadpNamespace,
			}).Reconcile(
				context.Background(),
				reconcile.Request{NamespacedName: types.NamespacedName{
					Namespace: scenario.namespace,
					Name:      testNonAdminBackupName,
				}},
			)
			gomega.Expect(result).To(gomega.Equal(scenario.result))
			if scenario.resultError == nil {
				gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))
			} else {
				gomega.Expect(err).To(gomega.HaveOccurred())
				gomega.Expect(err.Error()).To(gomega.Equal(scenario.resultError.Error()))
			}

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

			gomega.Expect(nonAdminBackup.Status.Conditions).To(gomega.HaveLen(len(scenario.status.Conditions)))
			for index := range nonAdminBackup.Status.Conditions {
				gomega.Expect(nonAdminBackup.Status.Conditions[index].Type).To(gomega.Equal(scenario.status.Conditions[index].Type))
				gomega.Expect(nonAdminBackup.Status.Conditions[index].Status).To(gomega.Equal(scenario.status.Conditions[index].Status))
				gomega.Expect(nonAdminBackup.Status.Conditions[index].Reason).To(gomega.Equal(scenario.status.Conditions[index].Reason))
				gomega.Expect(nonAdminBackup.Status.Conditions[index].Message).To(gomega.Equal(scenario.status.Conditions[index].Message))
			}

			// easy hack to test that only one update call happens per reconcile
			// currentResourceVersion, err := strconv.Atoi(nonAdminBackup.ResourceVersion)
			// gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))
			// gomega.Expect(currentResourceVersion - priorResourceVersion).To(gomega.Equal(1))
		},
		ginkgo.Entry("Should accept creation of NonAdminBackup", nonAdminBackupSingleReconcileScenario{
			namespace: "test-nonadminbackup-reconcile-1",
			result:    reconcile.Result{Requeue: true},
			status: nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminBackupPhaseNew,
			},
		}),
		ginkgo.Entry("Should accept update of NonAdminBackup phase to new", nonAdminBackupSingleReconcileScenario{
			namespace: "test-nonadminbackup-reconcile-2",
			spec: nacv1alpha1.NonAdminBackupSpec{
				BackupSpec: &v1.BackupSpec{},
			},
			priorStatus: &nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminBackupPhaseNew,
			},
			result: reconcile.Result{Requeue: true},
			status: nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminBackupPhaseNew,
				Conditions: []metav1.Condition{
					{
						Type:    "Accepted",
						Status:  metav1.ConditionTrue,
						Reason:  "BackupAccepted",
						Message: "Backup accepted",
					},
				},
			},
		}),
		ginkgo.Entry("Should accept update of NonAdminBackup Condition to Accepted True", nonAdminBackupSingleReconcileScenario{
			namespace:     "test-nonadminbackup-reconcile-3",
			oadpNamespace: "test-nonadminbackup-reconcile-3-oadp",
			spec: nacv1alpha1.NonAdminBackupSpec{
				BackupSpec: &v1.BackupSpec{},
			},
			priorStatus: &nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminBackupPhaseNew,
				Conditions: []metav1.Condition{
					{
						Type:               "Accepted",
						Status:             metav1.ConditionTrue,
						Reason:             "BackupAccepted",
						Message:            "Backup accepted",
						LastTransitionTime: metav1.NewTime(time.Now()),
					},
				},
			},
			result: reconcile.Result{Requeue: true},
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
				},
			},
		}),
		ginkgo.Entry("Should accept update of NonAdminBackup phase to created", nonAdminBackupSingleReconcileScenario{
			namespace:     "test-nonadminbackup-reconcile-4",
			oadpNamespace: "test-nonadminbackup-reconcile-4-oadp",
			spec: nacv1alpha1.NonAdminBackupSpec{
				BackupSpec: &v1.BackupSpec{},
			},
			priorStatus: &nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminBackupPhaseCreated,
				Conditions: []metav1.Condition{
					{
						Type:               "Accepted",
						Status:             metav1.ConditionTrue,
						Reason:             "BackupAccepted",
						Message:            "Backup accepted",
						LastTransitionTime: metav1.NewTime(time.Now()),
					},
				},
			},
			createVeleroBackup: true,
			result:             reconcile.Result{Requeue: true},
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
		ginkgo.Entry("Should accept update of NonAdminBackup Condition to Queued True", nonAdminBackupSingleReconcileScenario{
			namespace:     "test-nonadminbackup-reconcile-5",
			oadpNamespace: "test-nonadminbackup-reconcile-5-oadp",
			spec: nacv1alpha1.NonAdminBackupSpec{
				BackupSpec: &v1.BackupSpec{},
			},
			priorStatus: &nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminBackupPhaseCreated,
				Conditions: []metav1.Condition{
					{
						Type:               "Accepted",
						Status:             metav1.ConditionTrue,
						Reason:             "BackupAccepted",
						Message:            "Backup accepted",
						LastTransitionTime: metav1.NewTime(time.Now()),
					},
					{
						Type:               "Queued",
						Status:             metav1.ConditionTrue,
						Reason:             "BackupScheduled",
						Message:            "Created Velero Backup object",
						LastTransitionTime: metav1.NewTime(time.Now()),
					},
				},
			},
			createVeleroBackup: true,
			result:             reconcile.Result{Requeue: true},
			status: nacv1alpha1.NonAdminBackupStatus{
				Phase:                 nacv1alpha1.NonAdminBackupPhaseCreated,
				VeleroBackupName:      "nab-test-nonadminbackup-reconcile-5-c9dd6af01e2e2a",
				VeleroBackupNamespace: "test-nonadminbackup-reconcile-5-oadp",
				VeleroBackupStatus:    &v1.BackupStatus{},
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
		ginkgo.Entry("Should accept update of NonAdminBackup phase to new - invalid spec", nonAdminBackupSingleReconcileScenario{
			namespace: "test-nonadminbackup-reconcile-6",
			spec: nacv1alpha1.NonAdminBackupSpec{
				BackupSpec: &v1.BackupSpec{
					IncludedNamespaces: []string{"not-valid"},
				},
			},
			priorStatus: &nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminBackupPhaseNew,
			},
			result: reconcile.Result{Requeue: true},
			status: nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminBackupPhaseBackingOff,
			},
		}),
		ginkgo.Entry("Should accept update of NonAdminBackup phase to BackingOff", nonAdminBackupSingleReconcileScenario{
			// this validates spec again...
			namespace: "test-nonadminbackup-reconcile-7",
			spec: nacv1alpha1.NonAdminBackupSpec{
				BackupSpec: &v1.BackupSpec{
					IncludedNamespaces: []string{"not-valid"},
				},
			},
			priorStatus: &nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminBackupPhaseBackingOff,
			},
			resultError: reconcile.TerminalError(fmt.Errorf("spec.backupSpec.IncludedNamespaces can not contain namespaces other than: test-nonadminbackup-reconcile-7")),
			status: nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminBackupPhaseBackingOff,
				Conditions: []metav1.Condition{
					{
						Type:    "Accepted",
						Status:  metav1.ConditionFalse,
						Reason:  "InvalidBackupSpec",
						Message: "NonAdminBackup does not contain valid Spec",
					},
				},
			},
		}),
	)
})

var _ = ginkgo.Describe("Test full reconcile loop of NonAdminBackup Controller", func() {
	var (
		currentTestScenario nonAdminBackupFullReconcileScenario
		updateTestScenario  = func(scenario nonAdminBackupFullReconcileScenario) {
			ctx, cancel := context.WithCancel(context.Background())
			scenario.ctx = ctx
			scenario.cancel = cancel
			currentTestScenario = scenario
		}
	)

	ginkgo.AfterEach(func() {
		oadpNamespace := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: currentTestScenario.oadpNamespace,
			},
		}
		gomega.Expect(k8sClient.Delete(currentTestScenario.ctx, oadpNamespace)).To(gomega.Succeed())

		namespace := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: currentTestScenario.namespace,
			},
		}
		gomega.Expect(k8sClient.Delete(currentTestScenario.ctx, namespace)).To(gomega.Succeed())

		currentTestScenario.cancel()
		// wait cancel
		time.Sleep(1 * time.Second)
	})

	ginkgo.DescribeTable("full reconcile loop",
		func(scenario nonAdminBackupFullReconcileScenario) {
			updateTestScenario(scenario)

			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: scenario.namespace,
				},
			}
			gomega.Expect(k8sClient.Create(currentTestScenario.ctx, namespace)).To(gomega.Succeed())

			oadpNamespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: scenario.oadpNamespace,
				},
			}
			gomega.Expect(k8sClient.Create(currentTestScenario.ctx, oadpNamespace)).To(gomega.Succeed())

			k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
				Scheme: k8sClient.Scheme(),
			})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			err = (&NonAdminBackupReconciler{
				Client:        k8sManager.GetClient(),
				Scheme:        k8sManager.GetScheme(),
				OADPNamespace: scenario.oadpNamespace,
			}).SetupWithManager(k8sManager)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			go func() {
				defer ginkgo.GinkgoRecover()
				err = k8sManager.Start(currentTestScenario.ctx)
				gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to run manager")
			}()

			// wait manager start
			time.Sleep(1 * time.Second)
			nonAdminBackup := createTestNonAdminBackup(scenario.namespace, scenario.spec)
			gomega.Expect(k8sClient.Create(currentTestScenario.ctx, nonAdminBackup)).To(gomega.Succeed())

			// wait NAB reconcile
			time.Sleep(1 * time.Second)
			gomega.Expect(k8sClient.Get(
				currentTestScenario.ctx,
				types.NamespacedName{
					Name:      testNonAdminBackupName,
					Namespace: scenario.namespace,
				},
				nonAdminBackup,
			)).To(gomega.Succeed())

			log.Println("Validating NonAdminBackup Status")
			gomega.Expect(nonAdminBackup.Status.Phase).To(gomega.Equal(scenario.status.Phase))
			gomega.Expect(nonAdminBackup.Status.VeleroBackupName).To(gomega.Equal(scenario.status.VeleroBackupName))
			gomega.Expect(nonAdminBackup.Status.VeleroBackupNamespace).To(gomega.Equal(scenario.status.VeleroBackupNamespace))
			if len(scenario.status.VeleroBackupName) > 0 {
				gomega.Expect(nonAdminBackup.Status.VeleroBackupStatus.Phase).To(gomega.Equal(v1.BackupPhase(constant.EmptyString)))
			}

			gomega.Expect(nonAdminBackup.Status.Conditions).To(gomega.HaveLen(len(scenario.status.Conditions)))
			for index := range nonAdminBackup.Status.Conditions {
				gomega.Expect(nonAdminBackup.Status.Conditions[index].Type).To(gomega.Equal(scenario.status.Conditions[index].Type))
				gomega.Expect(nonAdminBackup.Status.Conditions[index].Status).To(gomega.Equal(scenario.status.Conditions[index].Status))
				gomega.Expect(nonAdminBackup.Status.Conditions[index].Reason).To(gomega.Equal(scenario.status.Conditions[index].Reason))
				gomega.Expect(nonAdminBackup.Status.Conditions[index].Message).To(gomega.Equal(scenario.status.Conditions[index].Message))
			}
			log.Println("Validation of NonAdminBackup Status completed successfully")

			if len(scenario.status.VeleroBackupName) > 0 {
				log.Println("Mocking VeleroBackup update to finished state")
				veleroBackup := &v1.Backup{}
				gomega.Expect(k8sClient.Get(
					currentTestScenario.ctx,
					types.NamespacedName{
						Name:      scenario.status.VeleroBackupName,
						Namespace: scenario.oadpNamespace,
					},
					veleroBackup,
				)).To(gomega.Succeed())
				veleroBackup.Status.Phase = v1.BackupPhaseCompleted
				// TODO I can not call .Status().Update() for veleroBackup object: backups.velero.io "name..." not found
				gomega.Expect(k8sClient.Update(currentTestScenario.ctx, veleroBackup)).To(gomega.Succeed())

				gomega.Eventually(func() (bool, error) {
					err := k8sClient.Get(
						currentTestScenario.ctx,
						types.NamespacedName{
							Name:      testNonAdminBackupName,
							Namespace: scenario.namespace,
						},
						nonAdminBackup,
					)
					if err != nil {
						return false, err
					}
					return nonAdminBackup.Status.VeleroBackupStatus.Phase == v1.BackupPhaseCompleted, nil
				}, 5*time.Second, 1*time.Second).Should(gomega.BeTrue())
			}

			gomega.Expect(k8sClient.Delete(currentTestScenario.ctx, nonAdminBackup)).To(gomega.Succeed())
			// wait reconcile of delete event
			time.Sleep(1 * time.Second)
		},
		ginkgo.Entry("Should update NonAdminBackup until VeleroBackup completes and than delete it", nonAdminBackupFullReconcileScenario{
			namespace:     "test-nonadminbackup-reconcile-full-1",
			oadpNamespace: "test-nonadminbackup-reconcile-full-1-oadp",
			spec: nacv1alpha1.NonAdminBackupSpec{
				BackupSpec: &v1.BackupSpec{},
			},
			status: nacv1alpha1.NonAdminBackupStatus{
				Phase:                 nacv1alpha1.NonAdminBackupPhaseCreated,
				VeleroBackupName:      "nab-test-nonadminbackup-reconcile-full-1-c9dd6af01e2e2a",
				VeleroBackupNamespace: "test-nonadminbackup-reconcile-full-1-oadp",
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
		ginkgo.Entry("Should update NonAdminBackup until it invalidates and than delete it", nonAdminBackupFullReconcileScenario{
			namespace:     "test-nonadminbackup-reconcile-full-2",
			oadpNamespace: "test-nonadminbackup-reconcile-full-2-oadp",
			spec: nacv1alpha1.NonAdminBackupSpec{
				BackupSpec: &v1.BackupSpec{
					IncludedNamespaces: []string{"not-valid"},
				},
			},
			status: nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminBackupPhaseBackingOff,
				Conditions: []metav1.Condition{
					{
						Type:    "Accepted",
						Status:  metav1.ConditionFalse,
						Reason:  "InvalidBackupSpec",
						Message: "NonAdminBackup does not contain valid Spec",
					},
				},
			},
		}),
	)
})
