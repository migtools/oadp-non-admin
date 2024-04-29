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

type clusterScenario struct {
	namespace       string
	nonAdminRestore string
}

type nonAdminRestoreReconcileScenario struct {
	restoreSpec     *v1.RestoreSpec
	namespace       string
	nonAdminRestore string
	errMessage      string
}

func createTestNonAdminRestore(name string, namespace string, restoreSpec v1.RestoreSpec) *nacv1alpha1.NonAdminRestore {
	return &nacv1alpha1.NonAdminRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: nacv1alpha1.NonAdminRestoreSpec{
			RestoreSpec: &restoreSpec,
		},
	}
}

// TODO this does not work with envtest :question:
var _ = ginkgo.Describe("Test NonAdminRestore in cluster validation", func() {
	var (
		ctx                 = context.Background()
		currentTestScenario clusterScenario
		updateTestScenario  = func(scenario clusterScenario) {
			currentTestScenario = scenario
		}
	)

	ginkgo.AfterEach(func() {
		nonAdminRestore := &nacv1alpha1.NonAdminRestore{}
		if k8sClient.Get(
			ctx,
			types.NamespacedName{
				Name:      currentTestScenario.nonAdminRestore,
				Namespace: currentTestScenario.namespace,
			},
			nonAdminRestore,
		) == nil {
			gomega.Expect(k8sClient.Delete(ctx, nonAdminRestore)).To(gomega.Succeed())
		}

		namespace := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: currentTestScenario.namespace,
			},
		}
		gomega.Expect(k8sClient.Delete(ctx, namespace)).To(gomega.Succeed())
	})

	ginkgo.DescribeTable("Validation is false",
		func(scenario clusterScenario) {
			updateTestScenario(scenario)

			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: scenario.namespace,
				},
			}
			gomega.Expect(k8sClient.Create(ctx, namespace)).To(gomega.Succeed())

			nonAdminRestore := &nacv1alpha1.NonAdminRestore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      scenario.nonAdminRestore,
					Namespace: scenario.namespace,
				},
				// Spec: nacv1alpha1.NonAdminRestoreSpec{},
			}
			gomega.Expect(k8sClient.Create(ctx, nonAdminRestore)).To(gomega.Not(gomega.Succeed()))
		},
		ginkgo.Entry("Should NOT create NonAdminRestore without spec.restoreSpec", clusterScenario{
			namespace:       "test-nonadminrestore-cluster-1",
			nonAdminRestore: "test-nonadminrestore-cluster-1-cr",
		}),
		// TODO Should NOT create NonAdminRestore without spec.restoreSpec.backupName
	)
})

var _ = ginkgo.Describe("Test NonAdminRestore Reconcile function", func() {
	var (
		ctx                 = context.Background()
		currentTestScenario nonAdminRestoreReconcileScenario
		updateTestScenario  = func(scenario nonAdminRestoreReconcileScenario) {
			currentTestScenario = scenario
		}
	)

	ginkgo.AfterEach(func() {
		gomega.Expect(os.Unsetenv(constant.NamespaceEnvVar)).To(gomega.Succeed())

		nonAdminRestore := &nacv1alpha1.NonAdminRestore{}
		if k8sClient.Get(
			ctx,
			types.NamespacedName{
				Name:      currentTestScenario.nonAdminRestore,
				Namespace: currentTestScenario.namespace,
			},
			nonAdminRestore,
		) == nil {
			gomega.Expect(k8sClient.Delete(ctx, nonAdminRestore)).To(gomega.Succeed())
		}

		namespace := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: currentTestScenario.namespace,
			},
		}
		gomega.Expect(k8sClient.Delete(ctx, namespace)).To(gomega.Succeed())
	})

	ginkgo.DescribeTable("Reconcile is false",
		func(scenario nonAdminRestoreReconcileScenario) {
			updateTestScenario(scenario)

			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: scenario.namespace,
				},
			}
			gomega.Expect(k8sClient.Create(ctx, namespace)).To(gomega.Succeed())

			nonAdminRestore := createTestNonAdminRestore(scenario.nonAdminRestore, scenario.namespace, *scenario.restoreSpec)
			gomega.Expect(k8sClient.Create(ctx, nonAdminRestore)).To(gomega.Succeed())

			gomega.Expect(os.Setenv(constant.NamespaceEnvVar, "envVarValue")).To(gomega.Succeed())
			r := &NonAdminRestoreReconciler{
				Client: k8sClient,
				Scheme: testEnv.Scheme,
			}
			result, err := r.Reconcile(
				context.Background(),
				reconcile.Request{NamespacedName: types.NamespacedName{
					Namespace: scenario.namespace,
					Name:      scenario.nonAdminRestore,
				}},
			)

			if len(scenario.errMessage) == 0 {
				gomega.Expect(result).To(gomega.Equal(reconcile.Result{Requeue: false, RequeueAfter: 0}))
				gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))
			} else {
				gomega.Expect(result).To(gomega.Equal(reconcile.Result{Requeue: false, RequeueAfter: 0}))
				gomega.Expect(err).To(gomega.HaveOccurred())
				gomega.Expect(err.Error()).To(gomega.ContainSubstring(scenario.errMessage))
			}
		},
		ginkgo.Entry("Should NOT accept scheduleName", nonAdminRestoreReconcileScenario{
			namespace:       "test-nonadminrestore-reconcile-1",
			nonAdminRestore: "test-nonadminrestore-reconcile-1-cr",
			errMessage:      "scheduleName",
			restoreSpec: &v1.RestoreSpec{
				ScheduleName: "wrong",
			},
		}),
		ginkgo.Entry("Should NOT accept non existing NonAdminBackup", nonAdminRestoreReconcileScenario{
			namespace:       "test-nonadminrestore-reconcile-2",
			nonAdminRestore: "test-nonadminrestore-reconcile-2-cr",
			errMessage:      "backupName",
			restoreSpec: &v1.RestoreSpec{
				BackupName: "do-not-exist",
			},
		}),
		// TODO Should NOT accept NonAdminBackup that is not in complete state :question:
		// TODO Should NOT accept non existing related Velero Backup
	)
})
