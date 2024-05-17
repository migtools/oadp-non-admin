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
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	nacv1alpha1 "github.com/migtools/oadp-non-admin/api/v1alpha1"
	"github.com/migtools/oadp-non-admin/internal/common/function"
)

const (
	testNonAdminBackupName = "test-non-admin-backup"
	placeholder            = "PLACEHOLDER"
)

type nonAdminBackupSingleReconcileScenario struct {
	resultError        error
	priorStatus        *nacv1alpha1.NonAdminBackupStatus
	spec               nacv1alpha1.NonAdminBackupSpec
	ExpectedStatus     nacv1alpha1.NonAdminBackupStatus
	result             reconcile.Result
	createVeleroBackup bool
}

type nonAdminBackupFullReconcileScenario struct {
	spec   nacv1alpha1.NonAdminBackupSpec
	status nacv1alpha1.NonAdminBackupStatus
}

func buildTestNonAdminBackup(namespace string, spec nacv1alpha1.NonAdminBackupSpec) *nacv1alpha1.NonAdminBackup {
	return &nacv1alpha1.NonAdminBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testNonAdminBackupName,
			Namespace: namespace,
		},
		Spec: spec,
	}
}

func checkTestNonAdminBackupStatus(nonAdminBackup *nacv1alpha1.NonAdminBackup, expectedStatus nacv1alpha1.NonAdminBackupStatus, nonAdminNamespaceName string, oadpNamespaceName string) error {
	if nonAdminBackup.Status.Phase != expectedStatus.Phase {
		return fmt.Errorf("NonAdminBackup Status Phase %v is not equal to expected %v", nonAdminBackup.Status.Phase, expectedStatus.Phase)
	}
	if expectedStatus.VeleroBackup != nil {
		veleroBackupName := expectedStatus.VeleroBackup.Name
		if expectedStatus.VeleroBackup.Name == placeholder {
			veleroBackupName = function.GenerateVeleroBackupName(nonAdminNamespaceName, testNonAdminBackupName)
		}
		if nonAdminBackup.Status.VeleroBackup.Name != veleroBackupName {
			return fmt.Errorf("NonAdminBackup Status VeleroBackupName %v is not equal to expected %v", nonAdminBackup.Status.VeleroBackup.Name, veleroBackupName)
		}
		veleroBackupNamespace := expectedStatus.VeleroBackup.Namespace
		if expectedStatus.VeleroBackup.Namespace == placeholder {
			veleroBackupNamespace = oadpNamespaceName
		}
		if nonAdminBackup.Status.VeleroBackup.Namespace != veleroBackupNamespace {
			return fmt.Errorf("NonAdminBackup Status VeleroBackupNamespace %v is not equal to expected %v", nonAdminBackup.Status.VeleroBackup.Namespace, veleroBackupNamespace)
		}
		if !reflect.DeepEqual(nonAdminBackup.Status.VeleroBackup.Status, expectedStatus.VeleroBackup.Status) {
			return fmt.Errorf("NonAdminBackup Status VeleroBackupStatus %v is not equal to expected %v", nonAdminBackup.Status.VeleroBackup.Status, expectedStatus.VeleroBackup.Status)
		}
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
		if !strings.Contains(nonAdminBackup.Status.Conditions[index].Message, expectedStatus.Conditions[index].Message) {
			return fmt.Errorf("NonAdminBackup Status Conditions [%v] Message %v does not contain expected message %v", index, nonAdminBackup.Status.Conditions[index].Message, expectedStatus.Conditions[index].Message)
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

	ginkgo.DescribeTable("Reconcile triggered by NonAdminBackup Delete event",
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

	ginkgo.DescribeTable("Reconcile triggered by NonAdminBackup Create/Update events and by Requeue",
		func(scenario nonAdminBackupSingleReconcileScenario) {
			updateTestScenario()

			gomega.Expect(createTestNamespaces(ctx, nonAdminNamespaceName, oadpNamespaceName)).To(gomega.Succeed())

			nonAdminBackup := buildTestNonAdminBackup(nonAdminNamespaceName, scenario.spec)
			gomega.Expect(k8sClient.Create(ctx, nonAdminBackup)).To(gomega.Succeed())

			if scenario.createVeleroBackup {
				veleroBackup := &velerov1.Backup{
					ObjectMeta: metav1.ObjectMeta{
						Name:        function.GenerateVeleroBackupName(nonAdminNamespaceName, testNonAdminBackupName),
						Namespace:   oadpNamespaceName,
						Labels:      function.AddNonAdminLabels(nil),
						Annotations: function.AddNonAdminBackupAnnotations(nonAdminNamespaceName, testNonAdminBackupName, "", nil),
					},
					Spec: velerov1.BackupSpec{
						IncludedNamespaces: []string{nonAdminNamespaceName},
					},
				}
				gomega.Expect(k8sClient.Create(ctx, veleroBackup)).To(gomega.Succeed())
			}

			if scenario.priorStatus != nil {
				nonAdminBackup.Status = *scenario.priorStatus
				if nonAdminBackup.Status.VeleroBackup != nil {
					if nonAdminBackup.Status.VeleroBackup.Name == placeholder {
						nonAdminBackup.Status.VeleroBackup.Name = function.GenerateVeleroBackupName(nonAdminNamespaceName, testNonAdminBackupName)
					}
					if nonAdminBackup.Status.VeleroBackup.Namespace == placeholder {
						nonAdminBackup.Status.VeleroBackup.Namespace = oadpNamespaceName
					}
				}
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
				gomega.Expect(err.Error()).To(gomega.ContainSubstring(scenario.resultError.Error()))
			}

			gomega.Expect(k8sClient.Get(
				ctx,
				types.NamespacedName{
					Name:      testNonAdminBackupName,
					Namespace: nonAdminNamespaceName,
				},
				nonAdminBackup,
			)).To(gomega.Succeed())

			gomega.Expect(checkTestNonAdminBackupStatus(nonAdminBackup, scenario.ExpectedStatus, nonAdminNamespaceName, oadpNamespaceName)).To(gomega.Succeed())

			// easy hack to test that only one update call happens per reconcile
			// currentResourceVersion, err := strconv.Atoi(nonAdminBackup.ResourceVersion)
			// gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))
			// gomega.Expect(currentResourceVersion - priorResourceVersion).To(gomega.Equal(1))
		},
		ginkgo.Entry("When triggered by NonAdminBackup Create event, should update NonAdminBackup phase to new and Requeue", nonAdminBackupSingleReconcileScenario{
			ExpectedStatus: nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminBackupPhaseNew,
			},
			result: reconcile.Result{Requeue: true},
		}),
		ginkgo.Entry("When triggered by Requeue(NonAdminBackup phase new), should update NonAdminBackup Condition to Accepted True and Requeue", nonAdminBackupSingleReconcileScenario{
			spec: nacv1alpha1.NonAdminBackupSpec{
				BackupSpec: &velerov1.BackupSpec{},
			},
			priorStatus: &nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminBackupPhaseNew,
			},
			ExpectedStatus: nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminBackupPhaseNew,
				Conditions: []metav1.Condition{
					{
						Type:    "Accepted",
						Status:  metav1.ConditionTrue,
						Reason:  "BackupAccepted",
						Message: "backup accepted",
					},
				},
			},
			result: reconcile.Result{Requeue: true},
		}),
		ginkgo.Entry("When triggered by Requeue(NonAdminBackup phase new; Conditions Accepted True), should update NonAdminBackup phase to created and Condition to Queued True and VeleroBackup reference and Exit", nonAdminBackupSingleReconcileScenario{
			spec: nacv1alpha1.NonAdminBackupSpec{
				BackupSpec: &velerov1.BackupSpec{},
			},
			priorStatus: &nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminBackupPhaseNew,
				Conditions: []metav1.Condition{
					{
						Type:               "Accepted",
						Status:             metav1.ConditionTrue,
						Reason:             "BackupAccepted",
						Message:            "backup accepted",
						LastTransitionTime: metav1.NewTime(time.Now()),
					},
				},
			},
			ExpectedStatus: nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminBackupPhaseCreated,
				VeleroBackup: &nacv1alpha1.VeleroBackup{
					Name:      placeholder,
					Namespace: placeholder,
				},
				Conditions: []metav1.Condition{
					{
						Type:    "Accepted",
						Status:  metav1.ConditionTrue,
						Reason:  "BackupAccepted",
						Message: "backup accepted",
					},
					{
						Type:    "Queued",
						Status:  metav1.ConditionTrue,
						Reason:  "BackupScheduled",
						Message: "Created Velero Backup object",
					},
				},
			},
			result: reconcile.Result{},
		}),
		ginkgo.Entry("When triggered by VeleroBackup Update event, should update NonAdminBackup VeleroBackupStatus and Exit", nonAdminBackupSingleReconcileScenario{
			createVeleroBackup: true,
			spec: nacv1alpha1.NonAdminBackupSpec{
				BackupSpec: &velerov1.BackupSpec{},
			},
			priorStatus: &nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminBackupPhaseCreated,
				VeleroBackup: &nacv1alpha1.VeleroBackup{
					Name:      placeholder,
					Namespace: placeholder,
				},
				Conditions: []metav1.Condition{
					{
						Type:               "Accepted",
						Status:             metav1.ConditionTrue,
						Reason:             "BackupAccepted",
						Message:            "backup accepted",
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
			ExpectedStatus: nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminBackupPhaseCreated,
				VeleroBackup: &nacv1alpha1.VeleroBackup{
					Name:      placeholder,
					Namespace: placeholder,
					Status:    &velerov1.BackupStatus{},
				},
				Conditions: []metav1.Condition{
					{
						Type:    "Accepted",
						Status:  metav1.ConditionTrue,
						Reason:  "BackupAccepted",
						Message: "backup accepted",
					},
					{
						Type:    "Queued",
						Status:  metav1.ConditionTrue,
						Reason:  "BackupScheduled",
						Message: "Created Velero Backup object",
					},
				},
			},
			result: reconcile.Result{},
		}),
		ginkgo.Entry("When triggered by Requeue(NonAdminBackup phase new) [invalid spec], should update NonAdminBackup phase to BackingOff and Condition to Accepted False and Exit with terminal error", nonAdminBackupSingleReconcileScenario{
			spec: nacv1alpha1.NonAdminBackupSpec{
				BackupSpec: &velerov1.BackupSpec{
					IncludedNamespaces: []string{"not-valid"},
				},
			},
			priorStatus: &nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminBackupPhaseNew,
			},
			ExpectedStatus: nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminBackupPhaseBackingOff,
				Conditions: []metav1.Condition{
					{
						Type:    "Accepted",
						Status:  metav1.ConditionFalse,
						Reason:  "InvalidBackupSpec",
						Message: "spec.backupSpec.IncludedNamespaces can not contain namespaces other than:",
					},
				},
			},
			resultError: reconcile.TerminalError(fmt.Errorf("spec.backupSpec.IncludedNamespaces can not contain namespaces other than: ")),
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

	ginkgo.DescribeTable("Reconcile triggered by NonAdminBackup Create event",
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
			nonAdminBackup := buildTestNonAdminBackup(nonAdminNamespaceName, scenario.spec)
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
			gomega.Expect(checkTestNonAdminBackupStatus(nonAdminBackup, scenario.status, nonAdminNamespaceName, oadpNamespaceName)).To(gomega.Succeed())

			if scenario.status.VeleroBackup != nil && len(scenario.status.VeleroBackup.Name) > 0 {
				ginkgo.By("Checking if NonAdminBackup Spec was not changed")
				gomega.Expect(reflect.DeepEqual(
					nonAdminBackup.Spec,
					scenario.spec,
				)).To(gomega.BeTrue())

				ginkgo.By("Simulating VeleroBackup update to finished state")
				veleroBackup := &velerov1.Backup{}
				gomega.Expect(k8sClient.Get(
					ctx,
					types.NamespacedName{
						Name:      function.GenerateVeleroBackupName(nonAdminNamespaceName, testNonAdminBackupName),
						Namespace: oadpNamespaceName,
					},
					veleroBackup,
				)).To(gomega.Succeed())
				veleroBackup.Status.Phase = velerov1.BackupPhaseCompleted
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
					if nonAdminBackup.Status.VeleroBackup.Status == nil {
						return false, nil
					}
					return nonAdminBackup.Status.VeleroBackup.Status.Phase == velerov1.BackupPhaseCompleted, nil
				}, 5*time.Second, 1*time.Second).Should(gomega.BeTrue())
			}

			ginkgo.By("Waiting Reconcile of delete event")
			gomega.Expect(k8sClient.Delete(ctx, nonAdminBackup)).To(gomega.Succeed())
			time.Sleep(1 * time.Second)
		},
		ginkgo.Entry("Should update NonAdminBackup until VeleroBackup completes and then delete it", nonAdminBackupFullReconcileScenario{
			spec: nacv1alpha1.NonAdminBackupSpec{
				BackupSpec: &velerov1.BackupSpec{},
			},
			status: nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminBackupPhaseCreated,
				VeleroBackup: &nacv1alpha1.VeleroBackup{
					Name:      placeholder,
					Namespace: placeholder,
					Status:    nil,
				},
				Conditions: []metav1.Condition{
					{
						Type:    "Accepted",
						Status:  metav1.ConditionTrue,
						Reason:  "BackupAccepted",
						Message: "backup accepted",
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
				BackupSpec: &velerov1.BackupSpec{
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
						Message: "spec.backupSpec.IncludedNamespaces can not contain namespaces other than:",
					},
				},
			},
		}),
	)
})
