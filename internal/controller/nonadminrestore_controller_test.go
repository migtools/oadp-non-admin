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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"

	nacv1alpha1 "github.com/migtools/oadp-non-admin/api/v1alpha1"
	"github.com/migtools/oadp-non-admin/internal/common/constant"
)

type nonAdminRestoreClusterValidationScenario struct {
	spec nacv1alpha1.NonAdminRestoreSpec
}

type nonAdminRestoreFullReconcileScenario struct {
	enforcedRestoreSpec *velerov1.RestoreSpec
	spec                nacv1alpha1.NonAdminRestoreSpec
	status              nacv1alpha1.NonAdminRestoreStatus
	backupStatus        nacv1alpha1.NonAdminBackupStatus
	errorLogs           int
}

func buildTestNonAdminRestore(nonAdminNamespace string, nonAdminName string, spec nacv1alpha1.NonAdminRestoreSpec) *nacv1alpha1.NonAdminRestore {
	return &nacv1alpha1.NonAdminRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nonAdminName,
			Namespace: nonAdminNamespace,
		},
		Spec: spec,
	}
}

func checkTestNonAdminRestoreStatus(nonAdminRestore *nacv1alpha1.NonAdminRestore, expectedStatus nacv1alpha1.NonAdminRestoreStatus) error {
	if nonAdminRestore.Status.Phase != expectedStatus.Phase {
		return fmt.Errorf("NonAdminRestore Status Phase %v is not equal to expected %v", nonAdminRestore.Status.Phase, expectedStatus.Phase)
	}

	if nonAdminRestore.Status.VeleroRestore != nil {
		if nonAdminRestore.Status.VeleroRestore.NACUUID == constant.EmptyString {
			return fmt.Errorf("NonAdminRestore Status VeleroRestore NACUUID not set")
		}
		if nonAdminRestore.Status.VeleroRestore.Namespace == constant.EmptyString {
			return fmt.Errorf("NonAdminRestore status.veleroRestore.namespace is not set")
		}
		if nonAdminRestore.Status.VeleroRestore.Name == constant.EmptyString {
			return fmt.Errorf("NonAdminRestore status.veleroRestore.name is not set")
		}
		if expectedStatus.VeleroRestore != nil {
			if expectedStatus.VeleroRestore.Status != nil {
				if !reflect.DeepEqual(nonAdminRestore.Status.VeleroRestore.Status, expectedStatus.VeleroRestore.Status) {
					return fmt.Errorf("NonAdminRestore status.veleroRestore.status %v is not equal to expected %v", nonAdminRestore.Status.VeleroRestore.Status, expectedStatus.VeleroRestore.Status)
				}
			}
		}
	}

	if len(nonAdminRestore.Status.Conditions) != len(expectedStatus.Conditions) {
		return fmt.Errorf("NonAdminRestore Status has %v Condition(s), expected to have %v", len(nonAdminRestore.Status.Conditions), len(expectedStatus.Conditions))
	}
	for index := range nonAdminRestore.Status.Conditions {
		if nonAdminRestore.Status.Conditions[index].Type != expectedStatus.Conditions[index].Type {
			return fmt.Errorf("NonAdminRestore Status Conditions [%v] Type %v is not equal to expected %v", index, nonAdminRestore.Status.Conditions[index].Type, expectedStatus.Conditions[index].Type)
		}
		if nonAdminRestore.Status.Conditions[index].Status != expectedStatus.Conditions[index].Status {
			return fmt.Errorf("NonAdminRestore Status Conditions [%v] Status %v is not equal to expected %v", index, nonAdminRestore.Status.Conditions[index].Status, expectedStatus.Conditions[index].Status)
		}
		if nonAdminRestore.Status.Conditions[index].Reason != expectedStatus.Conditions[index].Reason {
			return fmt.Errorf("NonAdminRestore Status Conditions [%v] Reason %v is not equal to expected %v", index, nonAdminRestore.Status.Conditions[index].Reason, expectedStatus.Conditions[index].Reason)
		}
		if !strings.Contains(nonAdminRestore.Status.Conditions[index].Message, expectedStatus.Conditions[index].Message) {
			return fmt.Errorf("NonAdminRestore Status Conditions [%v] Message %v does not contain expected message %v", index, nonAdminRestore.Status.Conditions[index].Message, expectedStatus.Conditions[index].Message)
		}
	}
	return nil
}

var _ = ginkgo.Describe("Test NonAdminRestore in cluster validation", func() {
	var (
		ctx                      context.Context
		nonAdminRestoreName      string
		nonAdminRestoreNamespace string
		counter                  int
	)

	ginkgo.BeforeEach(func() {
		ctx = context.Background()
		counter++
		nonAdminRestoreName = fmt.Sprintf("non-admin-restore-object-%v", counter)
		nonAdminRestoreNamespace = fmt.Sprintf("test-non-admin-restore-cluster-validation-%v", counter)

		nonAdminNamespace := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: nonAdminRestoreNamespace,
			},
		}
		gomega.Expect(k8sClient.Create(ctx, nonAdminNamespace)).To(gomega.Succeed())
	})

	ginkgo.AfterEach(func() {
		nonAdminNamespace := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: nonAdminRestoreNamespace,
			},
		}
		gomega.Expect(k8sClient.Delete(ctx, nonAdminNamespace)).To(gomega.Succeed())
	})

	ginkgo.DescribeTable("Validation is false",
		func(scenario nonAdminRestoreClusterValidationScenario) {
			nonAdminRestore := buildTestNonAdminRestore(nonAdminRestoreNamespace, nonAdminRestoreName, scenario.spec)
			err := k8sClient.Create(ctx, nonAdminRestore)
			gomega.Expect(err).To(gomega.HaveOccurred())
			gomega.Expect(err.Error()).To(gomega.ContainSubstring("Required value"))
		},
		ginkgo.Entry("Should NOT create NonAdminRestore without spec.restoreSpec", nonAdminRestoreClusterValidationScenario{
			spec: nacv1alpha1.NonAdminRestoreSpec{},
		}),
	)
})

var _ = ginkgo.Describe("Test full reconcile loop of NonAdminRestore Controller", func() {
	var (
		ctx                      context.Context
		cancel                   context.CancelFunc
		nonAdminRestoreName      string
		nonAdminRestoreNamespace string
		oadpNamespace            string
		counter                  int
	)

	ginkgo.BeforeEach(func() {
		counter++
		nonAdminRestoreName = fmt.Sprintf("non-admin-restore-object-%v", counter)
		nonAdminRestoreNamespace = fmt.Sprintf("test-non-admin-restore-reconcile-full-%v", counter)
		oadpNamespace = nonAdminRestoreNamespace + "-oadp"
	})

	ginkgo.AfterEach(func() {
		gomega.Expect(deleteTestNamespaces(ctx, nonAdminRestoreNamespace, oadpNamespace)).To(gomega.Succeed())

		cancel()
		// wait manager shutdown
		gomega.Eventually(func() (bool, error) {
			logOutput := ginkgo.CurrentSpecReport().CapturedGinkgoWriterOutput
			shutdownlog := "INFO	Wait completed, proceeding to shutdown the manager"
			return strings.Contains(logOutput, shutdownlog) && strings.Count(logOutput, shutdownlog) == 1, nil
		}, 5*time.Second, 1*time.Second).Should(gomega.BeTrue())
	})

	ginkgo.DescribeTable("Reconcile triggered by NonAdminRestore Create event",
		func(scenario nonAdminRestoreFullReconcileScenario) {
			ctx, cancel = context.WithCancel(context.Background())

			gomega.Expect(createTestNamespaces(ctx, nonAdminRestoreNamespace, oadpNamespace)).To(gomega.Succeed())

			nonAdminBackup := buildTestNonAdminBackup(nonAdminRestoreNamespace, scenario.spec.RestoreSpec.BackupName, nacv1alpha1.NonAdminBackupSpec{BackupSpec: &velerov1.BackupSpec{}})
			gomega.Expect(k8sClient.Create(ctx, nonAdminBackup)).To(gomega.Succeed())
			nonAdminBackup.Status = scenario.backupStatus
			gomega.Expect(k8sClient.Status().Update(ctx, nonAdminBackup)).To(gomega.Succeed())

			k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
				Scheme: k8sClient.Scheme(),
				Cache: cache.Options{
					DefaultNamespaces: map[string]cache.Config{
						nonAdminRestoreNamespace: {},
						oadpNamespace:            {},
					},
				},
			})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			enforcedRestoreSpec := &velerov1.RestoreSpec{}
			if scenario.enforcedRestoreSpec != nil {
				enforcedRestoreSpec = scenario.enforcedRestoreSpec
			}
			err = (&NonAdminRestoreReconciler{
				Client:              k8sManager.GetClient(),
				Scheme:              k8sManager.GetScheme(),
				OADPNamespace:       oadpNamespace,
				EnforcedRestoreSpec: enforcedRestoreSpec,
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
				startUplog := `INFO	Starting workers	{"controller": "nonadminrestore", "controllerGroup": "oadp.openshift.io", "controllerKind": "NonAdminRestore", "worker count": 1}`
				return strings.Contains(logOutput, startUplog) &&
					strings.Count(logOutput, startUplog) == 1 &&
					!strings.Contains(logOutput, "DEBUG	Accepted Create event"), nil
			}, 5*time.Second, 1*time.Second).Should(gomega.BeTrue())

			ginkgo.By("Waiting Reconcile of create event")
			nonAdminRestore := buildTestNonAdminRestore(nonAdminRestoreNamespace, nonAdminRestoreName, scenario.spec)
			gomega.Expect(k8sClient.Create(ctx, nonAdminRestore)).To(gomega.Succeed())
			// wait NonAdminRestore reconcile
			gomega.Eventually(func() (bool, error) {
				err := k8sClient.Get(
					ctx,
					types.NamespacedName{
						Name:      nonAdminRestoreName,
						Namespace: nonAdminRestoreNamespace,
					},
					nonAdminRestore,
				)
				if err != nil {
					return false, err
				}
				err = checkTestNonAdminRestoreStatus(nonAdminRestore, scenario.status)
				return err == nil, err
			}, 5*time.Second, 1*time.Second).Should(gomega.BeTrue())

			ginkgo.By("Checking if NonAdminRestore Spec was not changed")
			gomega.Expect(reflect.DeepEqual(
				nonAdminRestore.Spec,
				scenario.spec,
			)).To(gomega.BeTrue())

			veleroRestore := &velerov1.Restore{}
			if scenario.status.VeleroRestore != nil && len(nonAdminRestore.Status.VeleroRestore.NACUUID) > 0 {
				gomega.Expect(k8sClient.Get(
					ctx,
					types.NamespacedName{
						Name:      nonAdminRestore.Status.VeleroRestore.Name,
						Namespace: oadpNamespace,
					},
					veleroRestore,
				)).To(gomega.Succeed())

				if scenario.enforcedRestoreSpec != nil {
					ginkgo.By("Validating Velero Restore Spec")
					expectedSpec := scenario.enforcedRestoreSpec.DeepCopy()
					expectedSpec.IncludedNamespaces = []string{nonAdminRestoreNamespace}
					gomega.Expect(reflect.DeepEqual(veleroRestore.Spec, *expectedSpec)).To(gomega.BeTrue())
				}

				ginkgo.By("Simulating Velero Restore update to finished state")
				veleroRestore.Status = velerov1.RestoreStatus{
					Phase: velerov1.RestorePhaseCompleted,
				}
				// can not call .Status().Update() for veleroRestore object https://github.com/vmware-tanzu/velero/issues/8285
				gomega.Expect(k8sClient.Update(ctx, veleroRestore)).To(gomega.Succeed())

				// wait NonAdminRestore reconcile
				gomega.Eventually(func() (bool, error) {
					err := k8sClient.Get(
						ctx,
						types.NamespacedName{
							Name:      nonAdminRestoreName,
							Namespace: nonAdminRestoreNamespace,
						},
						nonAdminRestore,
					)
					if err != nil {
						return false, err
					}
					if nonAdminRestore == nil || nonAdminRestore.Status.VeleroRestore == nil || nonAdminRestore.Status.VeleroRestore.Status == nil {
						return false, nil
					}
					return nonAdminRestore.Status.VeleroRestore.Status.Phase == velerov1.RestorePhaseCompleted, nil
				}, 5*time.Second, 1*time.Second).Should(gomega.BeTrue())
			}

			ginkgo.By("Waiting NonAdminRestore deletion")
			gomega.Expect(k8sClient.Delete(ctx, nonAdminRestore)).To(gomega.Succeed())
			if scenario.status.VeleroRestore != nil && len(nonAdminRestore.Status.VeleroRestore.NACUUID) > 0 {
				gomega.Eventually(func() (bool, error) {
					err := k8sClient.Get(
						ctx,
						types.NamespacedName{
							Name:      nonAdminRestore.Status.VeleroRestore.Name,
							Namespace: oadpNamespace,
						},
						veleroRestore,
					)
					if apierrors.IsNotFound(err) {
						return true, nil
					}
					return false, err
				}, 10*time.Second, 1*time.Second).Should(gomega.BeTrue())
			}
			gomega.Eventually(func() (bool, error) {
				err := k8sClient.Get(
					ctx,
					types.NamespacedName{
						Name:      nonAdminRestoreName,
						Namespace: nonAdminRestoreNamespace,
					},
					nonAdminRestore,
				)
				if apierrors.IsNotFound(err) {
					return true, nil
				}
				return false, err
			}, 10*time.Second, 1*time.Second).Should(gomega.BeTrue())
			gomega.Eventually(func() (bool, error) {
				logOutput := ginkgo.CurrentSpecReport().CapturedGinkgoWriterOutput
				deletelog := `DEBUG	Accepted Delete event	{"NonAdminRestorePredicate"`
				return strings.Contains(logOutput, deletelog) && strings.Count(logOutput, deletelog) == 1, nil
			}, 5*time.Second, 1*time.Second).Should(gomega.BeTrue())
			gomega.Expect(strings.Count(ginkgo.CurrentSpecReport().CapturedGinkgoWriterOutput, "ERROR")).Should(gomega.Equal(scenario.errorLogs))
		},
		ginkgo.Entry("Should update NonAdminRestore until Velero Restore completes and then delete it", nonAdminRestoreFullReconcileScenario{
			spec: nacv1alpha1.NonAdminRestoreSpec{
				RestoreSpec: &velerov1.RestoreSpec{
					BackupName: "test",
				},
			},
			backupStatus: nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminPhaseCreated,
				VeleroBackup: &nacv1alpha1.VeleroBackup{
					Status: nil,
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
			status: nacv1alpha1.NonAdminRestoreStatus{
				Phase: nacv1alpha1.NonAdminPhaseCreated,
				VeleroRestore: &nacv1alpha1.VeleroRestore{
					Status: nil,
				},
				Conditions: []metav1.Condition{
					{
						Type:    "Accepted",
						Status:  metav1.ConditionTrue,
						Reason:  "RestoreAccepted",
						Message: "restore accepted",
					},
					{
						Type:    "Queued",
						Status:  metav1.ConditionTrue,
						Reason:  "RestoreScheduled",
						Message: "Created Velero Restore object",
					},
				},
			},
			enforcedRestoreSpec: &velerov1.RestoreSpec{
				RestorePVs: ptr.To(false),
				ItemOperationTimeout: metav1.Duration{
					Duration: 7 * time.Hour,
				},
				UploaderConfig: &velerov1.UploaderConfigForRestore{
					WriteSparseFiles: ptr.To(true),
				},
			},
		}),
		ginkgo.Entry("Should update NonAdminRestore until it invalidates and then delete it", nonAdminRestoreFullReconcileScenario{
			spec: nacv1alpha1.NonAdminRestoreSpec{
				RestoreSpec: &velerov1.RestoreSpec{
					BackupName: "non-admin-backup-with-phase-backing-off",
				},
			},
			backupStatus: nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminPhaseBackingOff,
				Conditions: []metav1.Condition{
					{
						Type:               "Accepted",
						Status:             metav1.ConditionFalse,
						Reason:             "InvalidBackupSpec",
						Message:            "spec.backupSpec.IncludedNamespaces can not contain namespaces other than:",
						LastTransitionTime: metav1.NewTime(time.Now()),
					},
				},
			},
			status: nacv1alpha1.NonAdminRestoreStatus{
				Phase: nacv1alpha1.NonAdminPhaseBackingOff,
				Conditions: []metav1.Condition{
					{
						Type:    "Accepted",
						Status:  metav1.ConditionFalse,
						Reason:  "InvalidRestoreSpec",
						Message: "NonAdminRestore spec.restoreSpec.backupName is invalid: ",
					},
				},
			},
			errorLogs: 1,
		}),
	)
})
