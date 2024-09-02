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
	"log"
	"strconv"
	// "net/http"
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
)

const testNonAdminBackupName = "test-non-admin-backup"

type nonAdminBackupReconcileScenario struct {
	namespace     string
	oadpNamespace string
	spec          nacv1alpha1.NonAdminBackupSpec
	priorStatus   *nacv1alpha1.NonAdminBackupStatus
	status        nacv1alpha1.NonAdminBackupStatus
	result        reconcile.Result
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
				oadpNamespace := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: scenario.oadpNamespace,
					},
				}
				gomega.Expect(k8sClient.Create(ctx, oadpNamespace)).To(gomega.Succeed())
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
			gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))

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
			result:    reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second},
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
			// TODO WRONG this should be a validator not a code logic
			namespace: "test-nonadminbackup-reconcile-4",
			spec:      nacv1alpha1.NonAdminBackupSpec{},
			priorStatus: &nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminBackupPhaseNew,
			},
			status: nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminBackupPhaseBackingOff,
			},
			// should not return terminal error?
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
			// should not return terminal error?
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
				// TOO MUCH TIME!!!!
			}, 35*time.Second, 1*time.Second).Should(gomega.BeTrue())

			log.Println("Validating NonAdminBackup Status")
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
			veleroBackup.Status.Phase = v1.BackupPhaseNew
			// TODO I can not call .Status().Update() for veleroBackup object: backups.velero.io "name..." not found
			gomega.Expect(k8sClient.Update(currentTestScenario.ctx, veleroBackup)).To(gomega.Succeed())
			// every update produces to reconciles: VeleroBackupPredicate on update -> reconcile start -> update nab status -> requeue -> reconcile start

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
				// TOO MUCH TIME!!!!
			}, 15*time.Second, 1*time.Second).Should(gomega.BeTrue())
			gomega.Expect(nonAdminBackup.Status.VeleroBackupStatus.Phase).To(gomega.Equal(v1.BackupPhaseNew))

			veleroBackup.Status.Phase = v1.BackupPhaseInProgress
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
				currentResourceVersion, err := strconv.Atoi(nonAdminBackup.ResourceVersion)
				if err != nil {
					return false, err
				}
				return currentResourceVersion-originalResourceVersion == scenario.numberOfResourceVersionChanges+4, nil
				// TOO MUCH TIME!!!!
			}, 15*time.Second, 1*time.Second).Should(gomega.BeTrue())
			gomega.Expect(nonAdminBackup.Status.VeleroBackupStatus.Phase).To(gomega.Equal(v1.BackupPhaseInProgress))

			veleroBackup.Status.Phase = v1.BackupPhaseCompleted
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
				currentResourceVersion, err := strconv.Atoi(nonAdminBackup.ResourceVersion)
				if err != nil {
					return false, err
				}
				return currentResourceVersion-originalResourceVersion == scenario.numberOfResourceVersionChanges+6, nil
				// TOO MUCH TIME!!!!
			}, 15*time.Second, 1*time.Second).Should(gomega.BeTrue())
			gomega.Expect(nonAdminBackup.Status.VeleroBackupStatus.Phase).To(gomega.Equal(v1.BackupPhaseCompleted))

			gomega.Expect(k8sClient.Delete(currentTestScenario.ctx, nonAdminBackup)).To(gomega.Succeed())
			// wait reconcile of delete event
			time.Sleep(1 * time.Second)
		},
		// TODO logs for these tests are HUGE!!!!
		// example:
		// DEBUG	NonAdminBackup Reconcile start	{"controller": "nonadminbackup", "controllerGroup": "nac.oadp.openshift.io", "controllerKind": "NonAdminBackup", "NonAdminBackup": {"name":"test-non-admin-backup","namespace":"test-nonadminbackup-reconcile-full-1"}, "namespace": "test-nonadminbackup-reconcile-full-1", "name": "test-non-admin-backup", "reconcileID": "19f8b405-5db8-4bf4-b4a0-24ecdd0ae187", "NonAdminBackup": {"name":"test-non-admin-backup","namespace":"test-nonadminbackup-reconcile-full-1"}}
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
				VeleroBackupStatus: &v1.BackupStatus{
					Version:                       0,
					FormatVersion:                 "",
					Expiration:                    nil,
					Phase:                         "",
					ValidationErrors:              nil,
					StartTimestamp:                nil,
					CompletionTimestamp:           nil,
					VolumeSnapshotsAttempted:      0,
					VolumeSnapshotsCompleted:      0,
					FailureReason:                 "",
					Warnings:                      0,
					Errors:                        0,
					Progress:                      nil,
					CSIVolumeSnapshotsAttempted:   0,
					CSIVolumeSnapshotsCompleted:   0,
					BackupItemOperationsAttempted: 0,
					BackupItemOperationsCompleted: 0,
					BackupItemOperationsFailed:    0,
				},
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
			numberOfResourceVersionChanges: 13, // should be similar to reconcile starts???
		}),
		// PRIOR to mocking velero backup updates
		// events 10: 2 creates (1 nab, 1 velero), 8 update event (all nab, 5 rejected)
		// 6 reconcile starts
		// time: 30s-20s
		// TODO saw this flake!!
		// 2024-09-02T10:58:31-03:00	ERROR	NonAdminBackup Condition - Failed to update	{"controller": "nonadminbackup", "controllerGroup": "nac.oadp.openshift.io", "controllerKind": "NonAdminBackup", "NonAdminBackup": {"name":"test-non-admin-backup","namespace":"test-nonadminbackup-reconcile-full-1"}, "namespace": "test-nonadminbackup-reconcile-full-1", "name": "test-non-admin-backup", "reconcileID": "fd1db7a8-6ed5-40ea-b5f6-03c4b1b88dd1", "ValidateSpec NonAdminBackup": {"name":"test-non-admin-backup","namespace":"test-nonadminbackup-reconcile-full-1"}, "error": "Operation cannot be fulfilled on nonadminbackups.nac.oadp.openshift.io \"test-non-admin-backup\": the object has been modified; please apply your changes to the latest version and try again"}
		// stacktrace...
		// 2024-09-02T10:58:31-03:00	ERROR	Unable to set BackupAccepted Condition: True	{"controller": "nonadminbackup", "controllerGroup": "nac.oadp.openshift.io", "controllerKind": "NonAdminBackup", "NonAdminBackup": {"name":"test-non-admin-backup","namespace":"test-nonadminbackup-reconcile-full-1"}, "namespace": "test-nonadminbackup-reconcile-full-1", "name": "test-non-admin-backup", "reconcileID": "fd1db7a8-6ed5-40ea-b5f6-03c4b1b88dd1", "ValidateSpec NonAdminBackup": {"name":"test-non-admin-backup","namespace":"test-nonadminbackup-reconcile-full-1"}, "error": "Operation cannot be fulfilled on nonadminbackups.nac.oadp.openshift.io \"test-non-admin-backup\": the object has been modified; please apply your changes to the latest version and try again"}
		// stacktrace...
		// 2024-09-02T10:58:31-03:00	ERROR	Reconciler error	{"controller": "nonadminbackup", "controllerGroup": "nac.oadp.openshift.io", "controllerKind": "NonAdminBackup", "NonAdminBackup": {"name":"test-non-admin-backup","namespace":"test-nonadminbackup-reconcile-full-1"}, "namespace": "test-nonadminbackup-reconcile-full-1", "name": "test-non-admin-backup", "reconcileID": "fd1db7a8-6ed5-40ea-b5f6-03c4b1b88dd1", "error": "terminal error: Operation cannot be fulfilled on nonadminbackups.nac.oadp.openshift.io \"test-non-admin-backup\": the object has been modified; please apply your changes to the latest version and try again"}
		// stacktrace...

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
