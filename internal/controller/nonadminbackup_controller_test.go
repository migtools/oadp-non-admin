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
	"k8s.io/apimachinery/pkg/util/wait"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	nacv1alpha1 "github.com/migtools/oadp-non-admin/api/v1alpha1"
	"github.com/migtools/oadp-non-admin/internal/common/constant"
	"github.com/migtools/oadp-non-admin/internal/common/function"
)

type nonAdminBackupSingleReconcileScenario struct {
	resultError                  error
	nonAdminBackupPriorStatus    *nacv1alpha1.NonAdminBackupStatus
	nonAdminBackupSpec           nacv1alpha1.NonAdminBackupSpec
	nonAdminBackupExpectedStatus nacv1alpha1.NonAdminBackupStatus
	result                       reconcile.Result
	createVeleroBackup           bool
	addFinalizer                 bool
	uuidCreatedByReconcile       bool
	uuidFromTestCase             bool
}

type nonAdminBackupFullReconcileScenario struct {
	spec   nacv1alpha1.NonAdminBackupSpec
	status nacv1alpha1.NonAdminBackupStatus
}

func buildTestNonAdminBackup(nonAdminNamespace string, nonAdminName string, spec nacv1alpha1.NonAdminBackupSpec) *nacv1alpha1.NonAdminBackup {
	return &nacv1alpha1.NonAdminBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nonAdminName,
			Namespace: nonAdminNamespace,
		},
		Spec: spec,
	}
}

func checkTestNonAdminBackupStatus(nonAdminBackup *nacv1alpha1.NonAdminBackup, expectedStatus nacv1alpha1.NonAdminBackupStatus, oadpNamespaceName string) error {
	if nonAdminBackup.Status.Phase != expectedStatus.Phase {
		return fmt.Errorf("NonAdminBackup Status Phase %v is not equal to expected %v", nonAdminBackup.Status.Phase, expectedStatus.Phase)
	}

	if nonAdminBackup.Status.VeleroBackup != nil {
		if nonAdminBackup.Status.VeleroBackup.NameUUID == "" {
			return fmt.Errorf("NonAdminBackup Status VeleroBackupName %v is 0 length string", nonAdminBackup.Status.VeleroBackup.NameUUID)
		}

		if expectedStatus.VeleroBackup != nil {
			// When there is no VeleroBackup expected Namespace provided, use one that should be result of reconcile loop
			veleroBackupNamespace := expectedStatus.VeleroBackup.Namespace
			if veleroBackupNamespace == "" {
				veleroBackupNamespace = oadpNamespaceName
			}
			if nonAdminBackup.Status.VeleroBackup.Namespace != veleroBackupNamespace {
				return fmt.Errorf("NonAdminBackup Status VeleroBackupNamespace %v is not equal to expected %v", nonAdminBackup.Status.VeleroBackup.Namespace, veleroBackupNamespace)
			}
			if expectedStatus.VeleroBackup.Status != nil {
				if !reflect.DeepEqual(nonAdminBackup.Status.VeleroBackup.Status, expectedStatus.VeleroBackup.Status) {
					return fmt.Errorf("NonAdminBackup Status VeleroBackupStatus %v is not equal to expected %v", nonAdminBackup.Status.VeleroBackup.Status, expectedStatus.VeleroBackup.Status)
				}
			}
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
		ctx                     = context.Background()
		nonAdminObjectName      string
		nonAdminObjectNamespace string
		oadpNamespace           string
		veleroBackupNameUUID    string
		counter                 = 0
	)
	ginkgo.BeforeEach(func() {
		counter++
		nonAdminObjectName = fmt.Sprintf("nab-object-%v", counter)
		nonAdminObjectNamespace = fmt.Sprintf("test-nab-reconcile-%v", counter)
		oadpNamespace = nonAdminObjectNamespace + "-oadp"
		veleroBackupNameUUID = function.GenerateNacObjectNameWithUUID(nonAdminObjectNamespace, nonAdminObjectName)
		gomega.Expect(createTestNamespaces(ctx, nonAdminObjectNamespace, oadpNamespace)).To(gomega.Succeed())
	})
	ginkgo.AfterEach(func() {
		nonAdminBackup := &nacv1alpha1.NonAdminBackup{}
		if k8sClient.Get(
			ctx,
			types.NamespacedName{
				Name:      nonAdminObjectName,
				Namespace: nonAdminObjectNamespace,
			},
			nonAdminBackup,
		) == nil {
			gomega.Expect(k8sClient.Delete(ctx, nonAdminBackup)).To(gomega.Succeed())
		}
		gomega.Expect(deleteTestNamespaces(ctx, nonAdminObjectNamespace, oadpNamespace)).To(gomega.Succeed())
	})
	ginkgo.DescribeTable("Reconcile triggered by NonAdminBackup Delete event",
		func(scenario nonAdminBackupSingleReconcileScenario) {
			result, err := (&NonAdminBackupReconciler{
				Client: k8sClient,
				Scheme: testEnv.Scheme,
			}).Reconcile(
				context.Background(),
				reconcile.Request{NamespacedName: types.NamespacedName{
					Name:      nonAdminObjectName,
					Namespace: nonAdminObjectNamespace,
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
			nonAdminBackup := buildTestNonAdminBackup(nonAdminObjectNamespace, nonAdminObjectName, scenario.nonAdminBackupSpec)
			gomega.Expect(k8sClient.Create(ctx, nonAdminBackup.DeepCopy())).To(gomega.Succeed())
			nonAdminBackupAfterCreate := &nacv1alpha1.NonAdminBackup{}
			gomega.Expect(k8sClient.Get(
				ctx,
				types.NamespacedName{
					Name:      nonAdminObjectName,
					Namespace: nonAdminObjectNamespace,
				},
				nonAdminBackupAfterCreate,
			)).To(gomega.Succeed())
			if scenario.addFinalizer {
				if !controllerutil.ContainsFinalizer(nonAdminBackupAfterCreate, constant.NabFinalizerName) {
					controllerutil.AddFinalizer(nonAdminBackupAfterCreate, constant.NabFinalizerName)
					gomega.Expect(k8sClient.Update(ctx, nonAdminBackupAfterCreate)).To(gomega.Succeed())
				}
			}
			if scenario.nonAdminBackupPriorStatus != nil {
				nonAdminBackupAfterCreate.Status = *scenario.nonAdminBackupPriorStatus

				if scenario.uuidFromTestCase {
					nonAdminBackupAfterCreate.Status.VeleroBackup = &nacv1alpha1.VeleroBackup{
						NameUUID:  veleroBackupNameUUID,
						Namespace: oadpNamespace,
					}
				}
				gomega.Expect(k8sClient.Status().Update(ctx, nonAdminBackupAfterCreate)).To(gomega.Succeed())
			}
			if scenario.createVeleroBackup {
				veleroBackup := &velerov1.Backup{
					ObjectMeta: metav1.ObjectMeta{
						Name:      nonAdminBackupAfterCreate.Status.VeleroBackup.NameUUID,
						Namespace: oadpNamespace,
						Labels: map[string]string{
							constant.OadpLabel:              constant.OadpLabelValue,
							constant.ManagedByLabel:         constant.ManagedByLabelValue,
							constant.NabOriginNameUUIDLabel: nonAdminBackupAfterCreate.Status.VeleroBackup.NameUUID,
						},
						Annotations: function.GetNonAdminBackupAnnotations(nonAdminBackup.ObjectMeta),
					},
					Spec: velerov1.BackupSpec{
						IncludedNamespaces: []string{nonAdminObjectNamespace},
					},
				}
				gomega.Expect(k8sClient.Create(ctx, veleroBackup)).To(gomega.Succeed())
			}
			// easy hack to test that only one update call happens per reconcile
			// priorResourceVersion, err := strconv.Atoi(nonAdminBackup.ResourceVersion)
			// gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))
			result, err := (&NonAdminBackupReconciler{
				Client:        k8sClient,
				Scheme:        testEnv.Scheme,
				OADPNamespace: oadpNamespace,
			}).Reconcile(
				context.Background(),
				reconcile.Request{NamespacedName: types.NamespacedName{
					Name:      nonAdminObjectName,
					Namespace: nonAdminObjectNamespace,
				}},
			)
			gomega.Expect(result).To(gomega.Equal(scenario.result))
			if scenario.resultError == nil {
				gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))
			} else {
				gomega.Expect(err).To(gomega.HaveOccurred())
				gomega.Expect(err.Error()).To(gomega.ContainSubstring(scenario.resultError.Error()))
			}
			nonAdminBackupAfterReconcile := &nacv1alpha1.NonAdminBackup{}
			gomega.Expect(k8sClient.Get(
				ctx,
				types.NamespacedName{
					Name:      nonAdminObjectName,
					Namespace: nonAdminObjectNamespace,
				},
				nonAdminBackupAfterReconcile,
			)).To(gomega.Succeed())
			gomega.Expect(checkTestNonAdminBackupStatus(nonAdminBackupAfterReconcile, scenario.nonAdminBackupExpectedStatus, oadpNamespace)).To(gomega.Succeed())
			// TODO: Include the following check in the checkTestNonAdminBackupStatus. Note that there is a challenge where variables are used in the scenario
			//       data within nonAdminBackupExpectedStatus. Currently the data there needs to be static.
			if scenario.uuidCreatedByReconcile {
				gomega.Expect(nonAdminBackupAfterReconcile.Status.VeleroBackup.NameUUID).To(gomega.ContainSubstring(nonAdminObjectNamespace))
				gomega.Expect(nonAdminBackupAfterReconcile.Status.VeleroBackup.Namespace).To(gomega.Equal(oadpNamespace))
			}
			// easy hack to test that only one update call happens per reconcile
			// currentResourceVersion, err := strconv.Atoi(nonAdminBackup.ResourceVersion)
			// gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))
			// gomega.Expect(currentResourceVersion - priorResourceVersion).To(gomega.Equal(1))
		},
		ginkgo.Entry("When triggered by NonAdminBackup Create event, should update NonAdminBackup phase to new and Requeue", nonAdminBackupSingleReconcileScenario{
			nonAdminBackupExpectedStatus: nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminBackupPhaseNew,
			},
			result: reconcile.Result{Requeue: true},
		}),
		ginkgo.Entry("When triggered by Requeue(NonAdminBackup phase new), should update NonAdminBackup Condition to Accepted True and Requeue", nonAdminBackupSingleReconcileScenario{
			nonAdminBackupSpec: nacv1alpha1.NonAdminBackupSpec{
				BackupSpec: &velerov1.BackupSpec{},
			},
			nonAdminBackupPriorStatus: &nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminBackupPhaseNew,
			},
			nonAdminBackupExpectedStatus: nacv1alpha1.NonAdminBackupStatus{
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
		ginkgo.Entry("When triggered by Requeue(NonAdminBackup phase new; Conditions Accepted True), should update NonAdminBackup Status generated UUID for VeleroBackup and Requeue", nonAdminBackupSingleReconcileScenario{
			nonAdminBackupSpec: nacv1alpha1.NonAdminBackupSpec{
				BackupSpec: &velerov1.BackupSpec{},
			},
			nonAdminBackupPriorStatus: &nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminBackupPhaseNew,
				Conditions: []metav1.Condition{
					{
						Type:               string(nacv1alpha1.NonAdminConditionAccepted),
						Status:             metav1.ConditionTrue,
						Reason:             "BackupAccepted",
						Message:            "backup accepted",
						LastTransitionTime: metav1.NewTime(time.Now()),
					},
				},
			},
			nonAdminBackupExpectedStatus: nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminBackupPhaseNew,
				Conditions: []metav1.Condition{
					{
						Type:    string(nacv1alpha1.NonAdminConditionAccepted),
						Status:  metav1.ConditionTrue,
						Reason:  "BackupAccepted",
						Message: "backup accepted",
					},
				},
			},
			uuidCreatedByReconcile: true,
			result:                 reconcile.Result{Requeue: true},
		}),
		ginkgo.Entry("When triggered by Requeue(NonAdminBackup phase new; Conditions Accepted True; NonAdminBackup Status NameUUID set), should update NonAdminBackup phase to created and Condition to Queued True and Exit", nonAdminBackupSingleReconcileScenario{
			addFinalizer: true,
			nonAdminBackupSpec: nacv1alpha1.NonAdminBackupSpec{
				BackupSpec: &velerov1.BackupSpec{},
			},
			nonAdminBackupPriorStatus: &nacv1alpha1.NonAdminBackupStatus{
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
			nonAdminBackupExpectedStatus: nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminBackupPhaseCreated,
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
			uuidFromTestCase: true,
			result:           reconcile.Result{},
		}),
		ginkgo.Entry("When triggered by VeleroBackup Update event, should update NonAdminBackup VeleroBackupStatus and Exit", nonAdminBackupSingleReconcileScenario{
			createVeleroBackup: true,
			addFinalizer:       true,
			nonAdminBackupSpec: nacv1alpha1.NonAdminBackupSpec{
				BackupSpec: &velerov1.BackupSpec{},
			},
			nonAdminBackupPriorStatus: &nacv1alpha1.NonAdminBackupStatus{
				Phase:        nacv1alpha1.NonAdminBackupPhaseCreated,
				VeleroBackup: &nacv1alpha1.VeleroBackup{},
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
			nonAdminBackupExpectedStatus: nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminBackupPhaseCreated,
				VeleroBackup: &nacv1alpha1.VeleroBackup{
					Status: &velerov1.BackupStatus{},
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
			uuidFromTestCase: true,
			result:           reconcile.Result{},
		}),
		ginkgo.Entry("When triggered by Requeue(NonAdminBackup phase new) [invalid spec], should update NonAdminBackup phase to BackingOff and Condition to Accepted False and Exit with terminal error", nonAdminBackupSingleReconcileScenario{
			nonAdminBackupSpec: nacv1alpha1.NonAdminBackupSpec{
				BackupSpec: &velerov1.BackupSpec{
					IncludedNamespaces: []string{"not-valid"},
				},
			},
			nonAdminBackupPriorStatus: &nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminBackupPhaseNew,
			},
			nonAdminBackupExpectedStatus: nacv1alpha1.NonAdminBackupStatus{
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
		}))
})

var _ = ginkgo.Describe("Test full reconcile loop of NonAdminBackup Controller", func() {
	var (
		ctx                     context.Context
		cancel                  context.CancelFunc
		nonAdminObjectName      = ""
		nonAdminObjectNamespace = ""
		oadpNamespace           = ""
		counter                 = 0
	)

	ginkgo.BeforeEach(func() {
		counter++
		nonAdminObjectName = fmt.Sprintf("nab-object-%v", counter)
		nonAdminObjectNamespace = fmt.Sprintf("test-nab-reconcile-full-%v", counter)
		oadpNamespace = nonAdminObjectNamespace + "-oadp"
	})

	ginkgo.AfterEach(func() {
		gomega.Expect(deleteTestNamespaces(ctx, nonAdminObjectNamespace, oadpNamespace)).To(gomega.Succeed())

		cancel()
		// wait cancel
		time.Sleep(1 * time.Second)
	})

	ginkgo.DescribeTable("Reconcile triggered by NonAdminBackup Create event",
		func(scenario nonAdminBackupFullReconcileScenario) {
			ctx, cancel = context.WithCancel(context.Background())

			gomega.Expect(createTestNamespaces(ctx, nonAdminObjectNamespace, oadpNamespace)).To(gomega.Succeed())

			k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
				Scheme: k8sClient.Scheme(),
			})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			err = (&NonAdminBackupReconciler{
				Client:        k8sManager.GetClient(),
				Scheme:        k8sManager.GetScheme(),
				OADPNamespace: oadpNamespace,
			}).SetupWithManager(k8sManager)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			go func() {
				defer ginkgo.GinkgoRecover()
				err = k8sManager.Start(ctx)
				gomega.Expect(err).ToNot(gomega.HaveOccurred(), "failed to run manager")
			}()
			// wait manager start
			managerStartTimeout := 10 * time.Second
			pollInterval := 100 * time.Millisecond
			ctxTimeout, cancel := context.WithTimeout(ctx, managerStartTimeout)
			defer cancel()

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
			nonAdminBackup := buildTestNonAdminBackup(nonAdminObjectNamespace, nonAdminObjectName, scenario.spec)
			gomega.Expect(k8sClient.Create(ctxTimeout, nonAdminBackup)).To(gomega.Succeed())
			// wait NAB reconcile
			time.Sleep(2 * time.Second)

			ginkgo.By("Fetching NonAdminBackup after Reconcile")
			gomega.Expect(k8sClient.Get(
				ctxTimeout,
				types.NamespacedName{
					Name:      nonAdminObjectName,
					Namespace: nonAdminObjectNamespace,
				},
				nonAdminBackup,
			)).To(gomega.Succeed())

			ginkgo.By("Validating NonAdminBackup Status")

			gomega.Expect(checkTestNonAdminBackupStatus(nonAdminBackup, scenario.status, oadpNamespace)).To(gomega.Succeed())

			if scenario.status.VeleroBackup != nil && len(nonAdminBackup.Status.VeleroBackup.NameUUID) > 0 {
				ginkgo.By("Checking if NonAdminBackup Spec was not changed")
				gomega.Expect(reflect.DeepEqual(
					nonAdminBackup.Spec,
					scenario.spec,
				)).To(gomega.BeTrue())

				ginkgo.By("Simulating VeleroBackup update to finished state")

				veleroBackup := &velerov1.Backup{}
				gomega.Expect(k8sClient.Get(
					ctxTimeout,
					types.NamespacedName{
						Name:      nonAdminBackup.Status.VeleroBackup.NameUUID,
						Namespace: oadpNamespace,
					},
					veleroBackup,
				)).To(gomega.Succeed())

				// can not call .Status().Update() for veleroBackup object https://github.com/vmware-tanzu/velero/issues/8285
				veleroBackup.Status = velerov1.BackupStatus{
					Phase: velerov1.BackupPhaseCompleted,
				}

				gomega.Expect(k8sClient.Update(ctxTimeout, veleroBackup)).To(gomega.Succeed())

				ginkgo.By("VeleroBackup updated")

				// wait NAB reconcile

				gomega.Eventually(func() (bool, error) {
					err := k8sClient.Get(
						ctxTimeout,
						types.NamespacedName{
							Name:      nonAdminObjectName,
							Namespace: nonAdminObjectNamespace,
						},
						nonAdminBackup,
					)
					if err != nil {
						return false, err
					}
					if nonAdminBackup == nil || nonAdminBackup.Status.VeleroBackup == nil || nonAdminBackup.Status.VeleroBackup.Status == nil {
						return false, nil
					}
					return nonAdminBackup.Status.VeleroBackup.Status.Phase == velerov1.BackupPhaseCompleted, nil
				}, 5*time.Second, 1*time.Second).Should(gomega.BeTrue())
			}

			ginkgo.By("Waiting Reconcile of delete event")
			gomega.Expect(k8sClient.Delete(ctxTimeout, nonAdminBackup)).To(gomega.Succeed())
			time.Sleep(1 * time.Second)
		},
		ginkgo.Entry("Should update NonAdminBackup until VeleroBackup completes and then delete it", nonAdminBackupFullReconcileScenario{
			spec: nacv1alpha1.NonAdminBackupSpec{
				BackupSpec: &velerov1.BackupSpec{},
			},
			status: nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminBackupPhaseCreated,
				VeleroBackup: &nacv1alpha1.VeleroBackup{
					Namespace: oadpNamespace,
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
