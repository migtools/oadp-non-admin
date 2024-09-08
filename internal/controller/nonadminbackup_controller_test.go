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
	"reflect"
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
	"github.com/migtools/oadp-non-admin/internal/common/function"
)

const testNonAdminBackupName = "test-non-admin-backup"

type nonAdminBackupSingleReconcileScenario struct {
	resultError        error
	priorStatus        *nacv1alpha1.NonAdminBackupStatus
	spec               nacv1alpha1.NonAdminBackupSpec
	status             nacv1alpha1.NonAdminBackupStatus
	result             reconcile.Result
	createVeleroBackup bool
}

type nonAdminBackupFullReconcileScenario struct {
	spec   nacv1alpha1.NonAdminBackupSpec
	status nacv1alpha1.NonAdminBackupStatus
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

func checkTestNonAdminBackupStatus(nonAdminBackup *nacv1alpha1.NonAdminBackup, expectedStatus nacv1alpha1.NonAdminBackupStatus) error {
	if nonAdminBackup.Status.Phase != expectedStatus.Phase {
		return fmt.Errorf("NonAdminBackup Status Phase %v is not equal to expected %v", nonAdminBackup.Status.Phase, expectedStatus.Phase)
	}
	if nonAdminBackup.Status.VeleroBackupName != expectedStatus.VeleroBackupName {
		return fmt.Errorf("NonAdminBackup Status VeleroBackupName %v is not equal to expected %v", nonAdminBackup.Status.VeleroBackupName, expectedStatus.VeleroBackupName)
	}
	if nonAdminBackup.Status.VeleroBackupNamespace != expectedStatus.VeleroBackupNamespace {
		return fmt.Errorf("NonAdminBackup Status VeleroBackupNamespace %v is not equal to expected %v", nonAdminBackup.Status.VeleroBackupNamespace, expectedStatus.VeleroBackupNamespace)
	}
	if !reflect.DeepEqual(nonAdminBackup.Status.VeleroBackupStatus, expectedStatus.VeleroBackupStatus) {
		return fmt.Errorf("NonAdminBackup Status VeleroBackupStatus %v is not equal to expected %v", nonAdminBackup.Status.VeleroBackupStatus, expectedStatus.VeleroBackupStatus)
	}

	if len(nonAdminBackup.Status.Conditions) != len(expectedStatus.Conditions) {
		return fmt.Errorf("NonAdminBackup Status has %v Condition(s), expected to have %v", len(nonAdminBackup.Status.Conditions), len(expectedStatus.Conditions))
	}
	for index := range nonAdminBackup.Status.Conditions {
		if nonAdminBackup.Status.Conditions[index].Type != expectedStatus.Conditions[index].Type {
			return fmt.Errorf("NonAdminBackup Status Conditions [%v] Type %v is not equal to expected %v", index, nonAdminBackup.Status.Conditions[index].Type, expectedStatus.Conditions[index].Type)
		}
		if nonAdminBackup.Status.Conditions[index].Status != expectedStatus.Conditions[index].Status {
			return fmt.Errorf("NonAdminBackup Status Conditions [%v] Status %v is not equal to expected %v", index, nonAdminBackup.Status.Conditions[index].Status, expectedStatus.Conditions[index].Status)
		}
		if nonAdminBackup.Status.Conditions[index].Reason != expectedStatus.Conditions[index].Reason {
			return fmt.Errorf("NonAdminBackup Status Conditions [%v] Reason %v is not equal to expected %v", index, nonAdminBackup.Status.Conditions[index].Reason, expectedStatus.Conditions[index].Reason)
		}
		if nonAdminBackup.Status.Conditions[index].Message != expectedStatus.Conditions[index].Message {
			return fmt.Errorf("NonAdminBackup Status Conditions [%v] Message %v is not equal to expected %v", index, nonAdminBackup.Status.Conditions[index].Message, expectedStatus.Conditions[index].Message)
		}
	}
	return nil
}

func createTestNamespaces(ctx context.Context, nonAdminNamespaceName string, oadpNamespaceName string) error {
	nonAdminNamespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: nonAdminNamespaceName,
		},
	}
	err := k8sClient.Create(ctx, nonAdminNamespace)
	if err != nil {
		return err
	}

	oadpNamespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: oadpNamespaceName,
		},
	}
	return k8sClient.Create(ctx, oadpNamespace)
}

func deleteTestNamespaces(ctx context.Context, nonAdminNamespaceName string, oadpNamespaceName string) error {
	oadpNamespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: oadpNamespaceName,
		},
	}
	err := k8sClient.Delete(ctx, oadpNamespace)
	if err != nil {
		return err
	}

	nonAdminNamespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: nonAdminNamespaceName,
		},
	}
	return k8sClient.Delete(ctx, nonAdminNamespace)
}

var _ = ginkgo.Describe("Test single reconciles of NonAdminBackup Reconcile function", func() {
	var (
		ctx                   = context.Background()
		nonAdminNamespaceName = ""
		oadpNamespaceName     = ""
		counter               = 0
		updateTestScenario    = func() {
			counter++
			nonAdminNamespaceName = fmt.Sprintf("test-nonadminbackup-reconcile-%v", counter)
			oadpNamespaceName = nonAdminNamespaceName + "-oadp"
		}
	)

	ginkgo.AfterEach(func() {
		nonAdminBackup := &nacv1alpha1.NonAdminBackup{}
		if k8sClient.Get(
			ctx,
			types.NamespacedName{
				Name:      testNonAdminBackupName,
				Namespace: nonAdminNamespaceName,
			},
			nonAdminBackup,
		) == nil {
			gomega.Expect(k8sClient.Delete(ctx, nonAdminBackup)).To(gomega.Succeed())
		}

		gomega.Expect(deleteTestNamespaces(ctx, nonAdminNamespaceName, oadpNamespaceName)).To(gomega.Succeed())
	})

	ginkgo.DescribeTable("Reconcile called by NonAdminBackup Delete event",
		func(scenario nonAdminBackupSingleReconcileScenario) {
			updateTestScenario()

			gomega.Expect(createTestNamespaces(ctx, nonAdminNamespaceName, oadpNamespaceName)).To(gomega.Succeed())

			result, err := (&NonAdminBackupReconciler{
				Client: k8sClient,
				Scheme: testEnv.Scheme,
			}).Reconcile(
				context.Background(),
				reconcile.Request{NamespacedName: types.NamespacedName{
					Namespace: nonAdminNamespaceName,
					Name:      testNonAdminBackupName,
				}},
			)

			gomega.Expect(result).To(gomega.Equal(scenario.result))
			gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))
		},
		ginkgo.Entry("Should exit", nonAdminBackupSingleReconcileScenario{
			result: reconcile.Result{},
		}),
	)

	ginkgo.DescribeTable("Reconcile called by NonAdminBackup Create/Update events and by Requeue",
		func(scenario nonAdminBackupSingleReconcileScenario) {
			updateTestScenario()

			gomega.Expect(createTestNamespaces(ctx, nonAdminNamespaceName, oadpNamespaceName)).To(gomega.Succeed())

			nonAdminBackup := createTestNonAdminBackup(nonAdminNamespaceName, scenario.spec)
			gomega.Expect(k8sClient.Create(ctx, nonAdminBackup)).To(gomega.Succeed())

			if scenario.createVeleroBackup {
				veleroBackup := &v1.Backup{
					ObjectMeta: metav1.ObjectMeta{
						Name:      function.GenerateVeleroBackupName(nonAdminNamespaceName, testNonAdminBackupName),
						Namespace: oadpNamespaceName,
					},
					Spec: v1.BackupSpec{
						IncludedNamespaces: []string{nonAdminNamespaceName},
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
				OADPNamespace: oadpNamespaceName,
			}).Reconcile(
				context.Background(),
				reconcile.Request{NamespacedName: types.NamespacedName{
					Namespace: nonAdminNamespaceName,
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
					Namespace: nonAdminNamespaceName,
				},
				nonAdminBackup,
			)).To(gomega.Succeed())

			gomega.Expect(checkTestNonAdminBackupStatus(nonAdminBackup, scenario.status)).To(gomega.Succeed())
			if scenario.priorStatus != nil {
				if len(scenario.priorStatus.VeleroBackupName) > 0 {
					gomega.Expect(reflect.DeepEqual(
						nonAdminBackup.Spec.BackupSpec,
						&v1.BackupSpec{
							IncludedNamespaces: []string{
								nonAdminNamespaceName,
							},
						},
					)).To(gomega.BeTrue())
				}
			}

			// easy hack to test that only one update call happens per reconcile
			// currentResourceVersion, err := strconv.Atoi(nonAdminBackup.ResourceVersion)
			// gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))
			// gomega.Expect(currentResourceVersion - priorResourceVersion).To(gomega.Equal(1))
		},
		ginkgo.Entry("When called by NonAdminBackup Create event, should update NonAdminBackup phase to new and Requeue", nonAdminBackupSingleReconcileScenario{
			status: nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminBackupPhaseNew,
			},
			result: reconcile.Result{Requeue: true},
		}),
		ginkgo.Entry("When called by Requeue(update NonAdminBackup phase to new), should update NonAdminBackup Condition to Accepted True and Requeue", nonAdminBackupSingleReconcileScenario{
			spec: nacv1alpha1.NonAdminBackupSpec{
				BackupSpec: &v1.BackupSpec{},
			},
			priorStatus: &nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminBackupPhaseNew,
			},
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
			result: reconcile.Result{Requeue: true},
		}),
		ginkgo.Entry("When called by Requeue(update NonAdminBackup Condition to Accepted True), should update NonAdminBackup phase to created and Requeue", nonAdminBackupSingleReconcileScenario{
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
			result: reconcile.Result{Requeue: true},
		}),
		ginkgo.Entry("When called by Requeue(update NonAdminBackup phase to created), should update NonAdminBackup Condition to Queued True and Requeue", nonAdminBackupSingleReconcileScenario{
			createVeleroBackup: true,
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
			result: reconcile.Result{Requeue: true},
		}),
		ginkgo.Entry("When called by Requeue(update NonAdminBackup Condition to Queued True), should update NonAdminBackup VeleroBackupStatus and Requeue", nonAdminBackupSingleReconcileScenario{
			createVeleroBackup: true,
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
			status: nacv1alpha1.NonAdminBackupStatus{
				Phase:                 nacv1alpha1.NonAdminBackupPhaseCreated,
				VeleroBackupName:      "nab-test-nonadminbackup-reconcile-6-c9dd6af01e2e2a",
				VeleroBackupNamespace: "test-nonadminbackup-reconcile-6-oadp",
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
			result: reconcile.Result{Requeue: true},
		}),
		ginkgo.Entry("When called by Requeue(update NonAdminBackup VeleroBackupStatus), should update NonAdminBackup spec BackupSpec and Requeue", nonAdminBackupSingleReconcileScenario{
			createVeleroBackup: true,
			spec: nacv1alpha1.NonAdminBackupSpec{
				BackupSpec: &v1.BackupSpec{},
			},
			priorStatus: &nacv1alpha1.NonAdminBackupStatus{
				Phase:                 nacv1alpha1.NonAdminBackupPhaseCreated,
				VeleroBackupName:      "nab-test-nonadminbackup-reconcile-7-c9dd6af01e2e2a",
				VeleroBackupNamespace: "test-nonadminbackup-reconcile-7-oadp",
				VeleroBackupStatus:    &v1.BackupStatus{},
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
			status: nacv1alpha1.NonAdminBackupStatus{
				Phase:                 nacv1alpha1.NonAdminBackupPhaseCreated,
				VeleroBackupName:      "nab-test-nonadminbackup-reconcile-7-c9dd6af01e2e2a",
				VeleroBackupNamespace: "test-nonadminbackup-reconcile-7-oadp",
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
			// TODO should not exit?
			result: reconcile.Result{Requeue: true},
		}),
		ginkgo.Entry("When called by Requeue(update NonAdminBackup phase to new - invalid spec), should update NonAdminBackup phase to BackingOff and Requeue", nonAdminBackupSingleReconcileScenario{
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
			result: reconcile.Result{Requeue: true},
		}),
		ginkgo.Entry("When called by Requeue(update NonAdminBackup phase to BackingOff), should update NonAdminBackup Condition to Accepted False and stop with terminal error", nonAdminBackupSingleReconcileScenario{
			// TODO this validates spec again...
			spec: nacv1alpha1.NonAdminBackupSpec{
				BackupSpec: &v1.BackupSpec{
					IncludedNamespaces: []string{"not-valid"},
				},
			},
			priorStatus: &nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminBackupPhaseBackingOff,
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
			resultError: reconcile.TerminalError(fmt.Errorf("spec.backupSpec.IncludedNamespaces can not contain namespaces other than: test-nonadminbackup-reconcile-9")),
		}),
	)
})

var _ = ginkgo.Describe("Test full reconcile loop of NonAdminBackup Controller", func() {
	var (
		ctx                   context.Context
		cancel                context.CancelFunc
		nonAdminNamespaceName = ""
		oadpNamespaceName     = ""
		counter               = 0
		updateTestScenario    = func() {
			ctx, cancel = context.WithCancel(context.Background())
			counter++
			nonAdminNamespaceName = fmt.Sprintf("test-nonadminbackup-reconcile-full-%v", counter)
			oadpNamespaceName = nonAdminNamespaceName + "-oadp"
		}
	)

	ginkgo.AfterEach(func() {
		gomega.Expect(deleteTestNamespaces(ctx, nonAdminNamespaceName, oadpNamespaceName)).To(gomega.Succeed())

		cancel()
		// wait cancel
		time.Sleep(1 * time.Second)
	})

	ginkgo.DescribeTable("full reconcile loop",
		func(scenario nonAdminBackupFullReconcileScenario) {
			updateTestScenario()

			gomega.Expect(createTestNamespaces(ctx, nonAdminNamespaceName, oadpNamespaceName)).To(gomega.Succeed())

			k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
				Scheme: k8sClient.Scheme(),
			})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			err = (&NonAdminBackupReconciler{
				Client:        k8sManager.GetClient(),
				Scheme:        k8sManager.GetScheme(),
				OADPNamespace: oadpNamespaceName,
			}).SetupWithManager(k8sManager)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			go func() {
				defer ginkgo.GinkgoRecover()
				err = k8sManager.Start(ctx)
				gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to run manager")
			}()
			// wait manager start
			time.Sleep(1 * time.Second)

			ginkgo.By("Waiting Reconcile of create event")
			nonAdminBackup := createTestNonAdminBackup(nonAdminNamespaceName, scenario.spec)
			gomega.Expect(k8sClient.Create(ctx, nonAdminBackup)).To(gomega.Succeed())
			// wait NAB reconcile
			time.Sleep(1 * time.Second)

			ginkgo.By("Fetching NonAdminBackup after Reconcile")
			gomega.Expect(k8sClient.Get(
				ctx,
				types.NamespacedName{
					Name:      testNonAdminBackupName,
					Namespace: nonAdminNamespaceName,
				},
				nonAdminBackup,
			)).To(gomega.Succeed())

			ginkgo.By("Validating NonAdminBackup Status")
			gomega.Expect(checkTestNonAdminBackupStatus(nonAdminBackup, scenario.status)).To(gomega.Succeed())

			if len(scenario.status.VeleroBackupName) > 0 {
				ginkgo.By("Validating NonAdminBackup Spec")
				gomega.Expect(reflect.DeepEqual(
					nonAdminBackup.Spec.BackupSpec,
					&v1.BackupSpec{
						IncludedNamespaces: []string{
							nonAdminNamespaceName,
						},
					},
				)).To(gomega.BeTrue())

				ginkgo.By("Simulating VeleroBackup update to finished state")
				veleroBackup := &v1.Backup{}
				gomega.Expect(k8sClient.Get(
					ctx,
					types.NamespacedName{
						Name:      scenario.status.VeleroBackupName,
						Namespace: oadpNamespaceName,
					},
					veleroBackup,
				)).To(gomega.Succeed())
				veleroBackup.Status.Phase = v1.BackupPhaseCompleted
				// TODO can not call .Status().Update() for veleroBackup object: backups.velero.io "name..." not found error
				gomega.Expect(k8sClient.Update(ctx, veleroBackup)).To(gomega.Succeed())

				gomega.Eventually(func() (bool, error) {
					err := k8sClient.Get(
						ctx,
						types.NamespacedName{
							Name:      testNonAdminBackupName,
							Namespace: nonAdminNamespaceName,
						},
						nonAdminBackup,
					)
					if err != nil {
						return false, err
					}
					return nonAdminBackup.Status.VeleroBackupStatus.Phase == v1.BackupPhaseCompleted, nil
				}, 5*time.Second, 1*time.Second).Should(gomega.BeTrue())
			}

			ginkgo.By("Waiting Reconcile of delete event")
			gomega.Expect(k8sClient.Delete(ctx, nonAdminBackup)).To(gomega.Succeed())
			time.Sleep(1 * time.Second)
		},
		ginkgo.Entry("Should update NonAdminBackup until VeleroBackup completes and then delete it", nonAdminBackupFullReconcileScenario{
			spec: nacv1alpha1.NonAdminBackupSpec{
				BackupSpec: &v1.BackupSpec{},
			},
			status: nacv1alpha1.NonAdminBackupStatus{
				Phase:                 nacv1alpha1.NonAdminBackupPhaseCreated,
				VeleroBackupName:      "nab-test-nonadminbackup-reconcile-full-1-c9dd6af01e2e2a",
				VeleroBackupNamespace: "test-nonadminbackup-reconcile-full-1-oadp",
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
		ginkgo.Entry("Should update NonAdminBackup until it invalidates and then delete it", nonAdminBackupFullReconcileScenario{
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
