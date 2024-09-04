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
	// "net/http"
	"fmt"
	"log"
	"strconv"
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

type nonAdminBackupReconcileScenario struct {
	namespace          string
	oadpNamespace      string
	spec               nacv1alpha1.NonAdminBackupSpec
	priorStatus        *nacv1alpha1.NonAdminBackupStatus
	status             nacv1alpha1.NonAdminBackupStatus
	result             reconcile.Result
	resultError        error
	createVeleroBackup bool
	// TODO create a struct for each test case!
	ctx                            context.Context
	cancel                         context.CancelFunc
	numberOfResourceVersionChanges int
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

			if scenario.priorStatus != nil {
				nonAdminBackup.Status = *scenario.priorStatus
				gomega.Expect(k8sClient.Status().Update(ctx, nonAdminBackup)).To(gomega.Succeed())
			}
			priorResourceVersion, err := strconv.Atoi(nonAdminBackup.ResourceVersion)
			gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))

			if scenario.createVeleroBackup {
				veleroBackup := &v1.Backup{
					ObjectMeta: metav1.ObjectMeta{
						Name:      function.GenerateVeleroBackupName(scenario.namespace, testNonAdminBackupName),
						Namespace: scenario.oadpNamespace,
					},
				}
				gomega.Expect(k8sClient.Create(ctx, veleroBackup)).To(gomega.Succeed())
			}

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
			// TODO need to collect logs, so they do not appear in test run
			// also assert them
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

			currentResourceVersion, err := strconv.Atoi(nonAdminBackup.ResourceVersion)
			gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))
			gomega.Expect(currentResourceVersion - priorResourceVersion).To(gomega.Equal(1))
		},
		ginkgo.Entry("Should accept creation of NonAdminBackup", nonAdminBackupReconcileScenario{
			namespace: "test-nonadminbackup-reconcile-1",
			result:    reconcile.Result{Requeue: true},
			status: nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminBackupPhaseNew,
			},
		}),
		ginkgo.Entry("Should accept update of NonAdminBackup phase to new", nonAdminBackupReconcileScenario{
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
		ginkgo.Entry("Should accept update of NonAdminBackup Condition to Accepted True", nonAdminBackupReconcileScenario{
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
		ginkgo.Entry("Should accept update of NonAdminBackup phase to created", nonAdminBackupReconcileScenario{
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
		ginkgo.Entry("Should accept update of NonAdminBackup Condition to Queued True", nonAdminBackupReconcileScenario{
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
		ginkgo.Entry("Should accept update of NonAdminBackup phase to new - invalid spec", nonAdminBackupReconcileScenario{
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
		ginkgo.Entry("Should accept update of NonAdminBackup phase to BackingOff", nonAdminBackupReconcileScenario{
			// this validates spec again... WRONG!!!
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
						Message: "NonAdminBackup does not contain valid BackupSpec",
					},
				},
			},
		}),
	)
})

var _ = ginkgo.Describe("Test full reconcile loop of NonAdminBackup Controller", func() {
	var (
		currentTestScenario nonAdminBackupReconcileScenario
		updateTestScenario  = func(scenario nonAdminBackupReconcileScenario) {
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
		// https://github.com/kubernetes-sigs/controller-runtime/issues/1280
		// clientTransport := &http.Transport{}
		// clientTransport.CloseIdleConnections()
		// gomega.Eventually(func() error {
		// 	ret := ctx.Done()
		// 	if ret != nil {
		// 		return fmt.Errorf("not ready :(")
		// 	}
		// 	close(ret)
		// 	return nil
		// }, 5*time.Second, 1*time.Millisecond).Should(gomega.BeNil())
		// TODO HOW to wait process finish?
		// this is still being finished in next step
		time.Sleep(1 * time.Second)
	})

	ginkgo.DescribeTable("Reconcile loop should succeed",
		func(scenario nonAdminBackupReconcileScenario) {
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

			// I am seeing test overlap...
			// TODO Be CAREFUL about FLAKES with this approach?
			// study ref https://book.kubebuilder.io/cronjob-tutorial/writing-tests
			go func() {
				defer ginkgo.GinkgoRecover()
				err = k8sManager.Start(currentTestScenario.ctx)
				gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to run manager")
			}()

			nonAdminBackup := createTestNonAdminBackup(scenario.namespace, scenario.spec)
			gomega.Expect(k8sClient.Create(currentTestScenario.ctx, nonAdminBackup)).To(gomega.Succeed())
			originalResourceVersion, err := strconv.Atoi(nonAdminBackup.DeepCopy().ResourceVersion)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			// TODO how to know reconcile finished???
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
				currentResourceVersion, err := strconv.Atoi(nonAdminBackup.ResourceVersion)
				if err != nil {
					return false, err
				}
				return currentResourceVersion-originalResourceVersion == scenario.numberOfResourceVersionChanges, nil
			}, 5*time.Second, 1*time.Second).Should(gomega.BeTrue())

			log.Println("Validating NonAdminBackup Status")
			gomega.Expect(nonAdminBackup.Status.Phase).To(gomega.Equal(scenario.status.Phase))
			gomega.Expect(nonAdminBackup.Status.VeleroBackupName).To(gomega.Equal(scenario.status.VeleroBackupName))
			gomega.Expect(nonAdminBackup.Status.VeleroBackupNamespace).To(gomega.Equal(scenario.status.VeleroBackupNamespace))
			gomega.Expect(nonAdminBackup.Status.VeleroBackupStatus.Phase).To(gomega.Equal(v1.BackupPhase(constant.EmptyString)))

			for index := range nonAdminBackup.Status.Conditions {
				gomega.Expect(nonAdminBackup.Status.Conditions[index].Type).To(gomega.Equal(scenario.status.Conditions[index].Type))
				gomega.Expect(nonAdminBackup.Status.Conditions[index].Status).To(gomega.Equal(scenario.status.Conditions[index].Status))
				gomega.Expect(nonAdminBackup.Status.Conditions[index].Reason).To(gomega.Equal(scenario.status.Conditions[index].Reason))
				gomega.Expect(nonAdminBackup.Status.Conditions[index].Message).To(gomega.Equal(scenario.status.Conditions[index].Message))
			}
			log.Println("Validation of NonAdminBackup Status completed successfully")

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
			// every update produces 2 reconciles: VeleroBackupPredicate on update -> reconcile start -> update nab status -> requeue -> reconcile start

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
				currentResourceVersion, err := strconv.Atoi(nonAdminBackup.ResourceVersion)
				if err != nil {
					return false, err
				}
				// why 2 ResourceVersion upgrades per veleroBackup update?
				return currentResourceVersion-originalResourceVersion == scenario.numberOfResourceVersionChanges+2, nil
			}, 5*time.Second, 1*time.Second).Should(gomega.BeTrue())
			gomega.Expect(nonAdminBackup.Status.VeleroBackupStatus.Phase).To(gomega.Equal(v1.BackupPhaseCompleted))

			gomega.Expect(k8sClient.Delete(currentTestScenario.ctx, nonAdminBackup)).To(gomega.Succeed())
			// wait reconcile of delete event
			time.Sleep(1 * time.Second)
		},
		ginkgo.Entry("Should create, update and delete NonAdminBackup", nonAdminBackupReconcileScenario{
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
			numberOfResourceVersionChanges: 7, // should be similar to reconcile starts???
		}),

		// ginkgo.Entry("Should DO FULL sad path", nonAdminBackupReconcileScenario{
		// 	namespace:     "test-nonadminbackup-reconcile-full-2",
		// 	oadpNamespace: "test-nonadminbackup-reconcile-full-2-oadp",
		// 	spec:          nacv1alpha1.NonAdminBackupSpec{},
		// 	priorStatus: &nacv1alpha1.NonAdminBackupStatus{
		// 		Phase: nacv1alpha1.NonAdminBackupPhaseNew,
		// 	},
		// 	status: nacv1alpha1.NonAdminBackupStatus{
		// 		Phase: nacv1alpha1.NonAdminBackupPhaseBackingOff,
		// 	},
		// 	numberOfResourceVersionChanges: 2,
		// }),
		// events 3: 1 create, 2 update (1 rejected)
		// 2 reconcile starts
	)
})
