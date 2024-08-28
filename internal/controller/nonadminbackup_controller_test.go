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

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	nacv1alpha1 "github.com/migtools/oadp-non-admin/api/v1alpha1"
	"github.com/migtools/oadp-non-admin/internal/common/constant"
)

type nonAdminBackupReconcileScenario struct {
	namespace                 string
	nonAdminBackup            string
	oadpNamespace             string
	spec                      nacv1alpha1.NonAdminBackupSpec
	status                    nacv1alpha1.NonAdminBackupStatus
	doNotCreateNonAdminBackup bool
}

func createTestNonAdminBackup(name string, namespace string, spec nacv1alpha1.NonAdminBackupSpec) *nacv1alpha1.NonAdminBackup {
	return &nacv1alpha1.NonAdminBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: spec,
	}
}

func ruNonAdminBackupReconcilerUntilExit(r *NonAdminBackupReconciler, scenario nonAdminBackupReconcileScenario) (reconcile.Result, error) {
	result, err := r.Reconcile(
		context.Background(),
		reconcile.Request{NamespacedName: types.NamespacedName{
			Namespace: scenario.namespace,
			Name:      scenario.nonAdminBackup,
		}},
	)
	if err == nil && result.Requeue {
		return ruNonAdminBackupReconcilerUntilExit(r, scenario)
	}
	return result, err
}

var _ = ginkgo.Describe("Test NonAdminBackup Reconcile function", func() {
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
				Name:      currentTestScenario.nonAdminBackup,
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

	// TODO need to test more reconcile cases...
	ginkgo.DescribeTable("Reconcile without error",
		func(scenario nonAdminBackupReconcileScenario) {
			updateTestScenario(scenario)

			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: scenario.namespace,
				},
			}
			gomega.Expect(k8sClient.Create(ctx, namespace)).To(gomega.Succeed())

			if !scenario.doNotCreateNonAdminBackup {
				nonAdminBackup := createTestNonAdminBackup(scenario.nonAdminBackup, scenario.namespace, scenario.spec)
				gomega.Expect(k8sClient.Create(ctx, nonAdminBackup)).To(gomega.Succeed())
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

			r := &NonAdminBackupReconciler{
				Client: k8sClient,
				Scheme: testEnv.Scheme,
			}

			result, err := ruNonAdminBackupReconcilerUntilExit(r, scenario)
			// TODO need to collect logs, so they do not appear in test run
			// also assert them

			gomega.Expect(result).To(gomega.Equal(reconcile.Result{Requeue: false, RequeueAfter: 0}))
			gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))

			if !scenario.doNotCreateNonAdminBackup {
				nonAdminBackup := &nacv1alpha1.NonAdminBackup{}
				gomega.Expect(k8sClient.Get(
					ctx,
					types.NamespacedName{
						Name:      currentTestScenario.nonAdminBackup,
						Namespace: currentTestScenario.namespace,
					},
					nonAdminBackup,
				)).To(gomega.Succeed())
				gomega.Expect(nonAdminBackup.Status.Phase).To(gomega.Equal(scenario.status.Phase))
				for index := range nonAdminBackup.Status.Conditions {
					gomega.Expect(nonAdminBackup.Status.Conditions[index].Type).To(gomega.Equal(scenario.status.Conditions[index].Type))
					gomega.Expect(nonAdminBackup.Status.Conditions[index].Status).To(gomega.Equal(scenario.status.Conditions[index].Status))
					gomega.Expect(nonAdminBackup.Status.Conditions[index].Reason).To(gomega.Equal(scenario.status.Conditions[index].Reason))
					gomega.Expect(nonAdminBackup.Status.Conditions[index].Message).To(gomega.Equal(scenario.status.Conditions[index].Message))
				}
			}
		},
		ginkgo.Entry("Should NOT accept non existing nonAdminBackup", nonAdminBackupReconcileScenario{
			namespace:                 "test-nonadminbackup-reconcile-1",
			nonAdminBackup:            "test-nonadminbackup-reconcile-1-cr",
			doNotCreateNonAdminBackup: true,
			// TODO should have loop end in logs
			// TODO unnecessary duplication in logs
			//      {"NonAdminBackup": {"name":"test-nonadminbackup-reconcile-1-cr","namespace":"test-nonadminbackup-reconcile-1"},
			//                         "Name": "test-nonadminbackup-reconcile-1-cr", "Namespace": "test-nonadminbackup-reconcile-1"}
		}),
		ginkgo.Entry("Should NOT accept NonAdminBackup with empty backupSpec", nonAdminBackupReconcileScenario{
			namespace:      "test-nonadminbackup-reconcile-2",
			nonAdminBackup: "test-nonadminbackup-reconcile-2-cr",
			spec:           nacv1alpha1.NonAdminBackupSpec{},
			status: nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminBackupPhaseBackingOff,
			},
		}),
		// TODO should not have loop start again in logs
		// TODO error message duplication
		// TODO should have loop end in logs
		ginkgo.Entry("Should NOT accept NonAdminBackup with includedNamespaces pointing to different namespace", nonAdminBackupReconcileScenario{
			namespace:      "test-nonadminbackup-reconcile-3",
			nonAdminBackup: "test-nonadminbackup-reconcile-3-cr",
			spec: nacv1alpha1.NonAdminBackupSpec{
				BackupSpec: &v1.BackupSpec{
					IncludedNamespaces: []string{"not-valid"},
				},
			},
			status: nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminBackupPhaseBackingOff,
			},
		}),
		// TODO should not have loop start again in logs
		// TODO error message duplication
		// TODO should have loop end in logs
		ginkgo.Entry("Should accept NonAdminBackup and create Velero Backup", nonAdminBackupReconcileScenario{
			namespace:      "test-nonadminbackup-reconcile-4",
			nonAdminBackup: "test-nonadminbackup-reconcile-4-cr",
			oadpNamespace:  "test-nonadminbackup-reconcile-4-oadp",
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
		// TODO should not have loop start again and again in logs
		// TODO 3 condition logs, only 2 in CR status?

		// TODO create tests for single reconciles, so we can test https://github.com/migtools/oadp-non-admin/blob/master/docs/design/nab_status_update.md
	)
})
