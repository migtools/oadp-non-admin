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
	"github.com/migtools/oadp-non-admin/internal/common/function"
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
	"k8s.io/apimachinery/pkg/util/wait"
	ctrl "sigs.k8s.io/controller-runtime"

	nacv1alpha1 "github.com/migtools/oadp-non-admin/api/v1alpha1"
	"github.com/migtools/oadp-non-admin/internal/common/constant"
)

type nonAdminBackupStorageLocationClusterValidationScenario struct {
	spec nacv1alpha1.NonAdminBackupStorageLocationSpec
}

type nonAdminBackupStorageLocationFullReconcileScenario struct {
	nonAdminSecretName   string
	enforcedBslSpec      *velerov1.BackupStorageLocationSpec
	spec                 nacv1alpha1.NonAdminBackupStorageLocationSpec
	expectedStatus       nacv1alpha1.NonAdminBackupStorageLocationStatus
	createNonAdminSecret bool
}

func buildTestNonAdminSecretForBsl(nonAdminBslNamespace, nonAdminBslName, awsAccessKeyID, awsSecretAccessKey string) *corev1.Secret {
	cloudFileContent := fmt.Sprintf(`[default]\naws_access_key_id = %s\naws_secret_access_key = %s\n`, awsAccessKeyID, awsSecretAccessKey)

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nonAdminBslName,
			Namespace: nonAdminBslNamespace,
		},
		Data: map[string][]byte{
			"cloud": []byte(cloudFileContent),
		},
	}
}

func buildTestNonAdminBackupStorageLocation(nonAdminNamespace, nonAdminBslName string, spec nacv1alpha1.NonAdminBackupStorageLocationSpec) *nacv1alpha1.NonAdminBackupStorageLocation {
	return &nacv1alpha1.NonAdminBackupStorageLocation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nonAdminBslName,
			Namespace: nonAdminNamespace,
		},
		Spec: spec,
	}
}

func checkTestNonAdminBackupStorageLocationStatus(nonAdminBsl *nacv1alpha1.NonAdminBackupStorageLocation, expectedStatus nacv1alpha1.NonAdminBackupStorageLocationStatus) error {
	if nonAdminBsl.Status.Phase != expectedStatus.Phase {
		return fmt.Errorf("NonAdminBackupStorageLocation Status Phase %v is not equal to expected %v", nonAdminBsl.Status.Phase, expectedStatus.Phase)
	}

	if nonAdminBsl.Status.VeleroBackupStorageLocation != nil {
		if nonAdminBsl.Status.VeleroBackupStorageLocation.NACUUID == constant.EmptyString {
			return fmt.Errorf("NonAdminBackupStorageLocation Status VeleroBackupStorageLocation NACUUID not set")
		}
		if nonAdminBsl.Status.VeleroBackupStorageLocation.Namespace == constant.EmptyString {
			return fmt.Errorf("NonAdminBackupStorageLocation status.veleroBackupStorageLocation.namespace is not set")
		}
		if nonAdminBsl.Status.VeleroBackupStorageLocation.Name == constant.EmptyString {
			return fmt.Errorf("NonAdminBackupStorageLocation status.veleroBackupStorageLocation.name is not set")
		}
		if expectedStatus.VeleroBackupStorageLocation != nil {
			if expectedStatus.VeleroBackupStorageLocation.Status != nil {
				if !reflect.DeepEqual(nonAdminBsl.Status.VeleroBackupStorageLocation.Status, expectedStatus.VeleroBackupStorageLocation.Status) {
					return fmt.Errorf("NonAdminBackupStorageLocation status.veleroBackupStorageLocation.status %v is not equal to expected %v", nonAdminBsl.Status.VeleroBackupStorageLocation.Status, expectedStatus.VeleroBackupStorageLocation.Status)
				}
			}
		}
	}

	if len(nonAdminBsl.Status.Conditions) != len(expectedStatus.Conditions) {
		var actualConditions []string
		var expectedConditions []string
		for i := range nonAdminBsl.Status.Conditions {
			actualConditions = append(actualConditions, fmt.Sprintf("Type: %v, Status: %v, Reason: %v, Message: %v", nonAdminBsl.Status.Conditions[i].Type, nonAdminBsl.Status.Conditions[i].Status, nonAdminBsl.Status.Conditions[i].Reason, nonAdminBsl.Status.Conditions[i].Message))
		}
		for i := range expectedStatus.Conditions {
			expectedConditions = append(expectedConditions, fmt.Sprintf("Type: %v, Status: %v, Reason: %v, Message: %v", expectedStatus.Conditions[i].Type, expectedStatus.Conditions[i].Status, expectedStatus.Conditions[i].Reason, expectedStatus.Conditions[i].Message))
		}
		return fmt.Errorf("NonAdminBackupStorageLocation Status has %v Condition(s), expected to have %v. Actual conditions: %v. Expected conditions: %v", len(nonAdminBsl.Status.Conditions), len(expectedStatus.Conditions), actualConditions, expectedConditions)
	}

	for index := range nonAdminBsl.Status.Conditions {
		if nonAdminBsl.Status.Conditions[index].Type != expectedStatus.Conditions[index].Type {
			return fmt.Errorf("NonAdminBackupStorageLocation Status Conditions [%v] Type %v is not equal to expected %v", index, nonAdminBsl.Status.Conditions[index].Type, expectedStatus.Conditions[index].Type)
		}
		if nonAdminBsl.Status.Conditions[index].Status != expectedStatus.Conditions[index].Status {
			return fmt.Errorf("NonAdminBackupStorageLocation Status Conditions [%v] Status %v is not equal to expected %v", index, nonAdminBsl.Status.Conditions[index].Status, expectedStatus.Conditions[index].Status)
		}
		if nonAdminBsl.Status.Conditions[index].Reason != expectedStatus.Conditions[index].Reason {
			return fmt.Errorf("NonAdminBackupStorageLocation Status Conditions [%v] Reason %v is not equal to expected %v", index, nonAdminBsl.Status.Conditions[index].Reason, expectedStatus.Conditions[index].Reason)
		}
		if !strings.Contains(nonAdminBsl.Status.Conditions[index].Message, expectedStatus.Conditions[index].Message) {
			return fmt.Errorf("NonAdminBackupStorageLocation Status Conditions [%v] Message %v does not contain expected message %v", index, nonAdminBsl.Status.Conditions[index].Message, expectedStatus.Conditions[index].Message)
		}
	}
	return nil
}

var _ = ginkgo.Describe("Test NonAdminBackupStorageLocation in cluster validation", func() {
	var (
		ctx                  context.Context
		nonAdminBslName      string
		nonAdminBslNamespace string
		counter              int
	)

	ginkgo.BeforeEach(func() {
		ctx = context.Background()
		counter++
		nonAdminBslName = fmt.Sprintf("non-admin-bsl-object-%v", counter)
		nonAdminBslNamespace = fmt.Sprintf("test-non-admin-bsl-cluster-validation-%v", counter)

		nonAdminNamespace := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: nonAdminBslNamespace,
			},
		}
		gomega.Expect(k8sClient.Create(ctx, nonAdminNamespace)).To(gomega.Succeed())
	})

	ginkgo.AfterEach(func() {
		nonAdminNamespace := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: nonAdminBslNamespace,
			},
		}
		gomega.Expect(k8sClient.Delete(ctx, nonAdminNamespace)).To(gomega.Succeed())
	})

	ginkgo.DescribeTable("Validation is false",
		func(scenario nonAdminBackupStorageLocationClusterValidationScenario) {
			nonAdminBsl := buildTestNonAdminBackupStorageLocation(nonAdminBslNamespace, nonAdminBslName, scenario.spec)
			err := k8sClient.Create(ctx, nonAdminBsl)
			gomega.Expect(err).To(gomega.HaveOccurred())
			gomega.Expect(err.Error()).To(gomega.ContainSubstring("Required value"))
		},
		ginkgo.Entry("Should NOT create NonAdminBackupStorageLocation without spec.backupStorageLocationSpec", nonAdminBackupStorageLocationClusterValidationScenario{
			spec: nacv1alpha1.NonAdminBackupStorageLocationSpec{},
		}),
	)
})

var _ = ginkgo.Describe("Test full reconcile loop of NonAdminBackupStorageLocation Controller", func() {
	var (
		ctx                  context.Context
		cancel               context.CancelFunc
		nonAdminBslName      string
		nonAdminBslNamespace string
		oadpNamespace        string
		counter              int
	)

	ginkgo.BeforeEach(func() {
		counter++
		nonAdminBslName = fmt.Sprintf("non-admin-bsl-object-%v", counter)
		nonAdminBslNamespace = fmt.Sprintf("test-non-admin-bsl-reconcile-full-%v", counter)
		oadpNamespace = nonAdminBslNamespace + "-oadp"
	})

	ginkgo.AfterEach(func() {
		gomega.Expect(deleteTestNamespaces(ctx, nonAdminBslNamespace, oadpNamespace)).To(gomega.Succeed())

		cancel()
		// wait cancel
		time.Sleep(1 * time.Second)
	})

	ginkgo.DescribeTable("Reconcile triggered by NonAdminBackupStorageLocation Create event",
		func(scenario nonAdminBackupStorageLocationFullReconcileScenario) {
			ctx, cancel = context.WithCancel(context.Background())

			gomega.Expect(createTestNamespaces(ctx, nonAdminBslNamespace, oadpNamespace)).To(gomega.Succeed())

			// Test case(s) to catch error when the corresponding secret is not in the cluster or it's not properly configured
			if scenario.createNonAdminSecret && scenario.nonAdminSecretName != constant.EmptyString {
				nonAdminSecret := buildTestNonAdminSecretForBsl(nonAdminBslNamespace, scenario.nonAdminSecretName, "testAccessKeyId", "testSecretAccessKey")
				gomega.Expect(k8sClient.Create(ctx, nonAdminSecret)).To(gomega.Succeed())
			}

			k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
				Scheme: k8sClient.Scheme(),
			})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			enforcedBslSpec := &velerov1.BackupStorageLocationSpec{}

			if scenario.enforcedBslSpec != nil {
				enforcedBslSpec = scenario.enforcedBslSpec
			}
			err = (&NonAdminBackupStorageLocationReconciler{
				Client:          k8sManager.GetClient(),
				Scheme:          k8sManager.GetScheme(),
				OADPNamespace:   oadpNamespace,
				EnforcedBslSpec: enforcedBslSpec,
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
			nonAdminBsl := buildTestNonAdminBackupStorageLocation(nonAdminBslNamespace, nonAdminBslName, scenario.spec)
			gomega.Expect(k8sClient.Create(ctxTimeout, nonAdminBsl)).To(gomega.Succeed())
			// wait NonAdminBackupStorageLocation reconcile
			time.Sleep(2 * time.Second)

			ginkgo.By("Fetching NonAdminBackupStorageLocation after Reconcile")
			gomega.Expect(k8sClient.Get(
				ctxTimeout,
				types.NamespacedName{
					Name:      nonAdminBslName,
					Namespace: nonAdminBslNamespace,
				},
				nonAdminBsl,
			)).To(gomega.Succeed())

			ginkgo.By("Validating NonAdminBackupStorageLocation Status")

			gomega.Expect(checkTestNonAdminBackupStorageLocationStatus(nonAdminBsl, scenario.expectedStatus)).To(gomega.Succeed())

			veleroBsl := &velerov1.BackupStorageLocation{}
			if scenario.expectedStatus.VeleroBackupStorageLocation != nil && len(nonAdminBsl.Status.VeleroBackupStorageLocation.NACUUID) > 0 {
				ginkgo.By("Checking if NonAdminBackupStorageLocation Spec was not changed")
				gomega.Expect(reflect.DeepEqual(
					nonAdminBsl.Spec,
					scenario.spec,
				)).To(gomega.BeTrue())

				gomega.Expect(k8sClient.Get(
					ctxTimeout,
					types.NamespacedName{
						Name:      nonAdminBsl.Status.VeleroBackupStorageLocation.Name,
						Namespace: oadpNamespace,
					},
					veleroBsl,
				)).To(gomega.Succeed())

				if scenario.enforcedBslSpec != nil {
					ginkgo.By("Validating Velero BackupStorageLocation Spec")
					expectedSpec := scenario.enforcedBslSpec.DeepCopy()
					gomega.Expect(reflect.DeepEqual(veleroBsl.Spec, *expectedSpec)).To(gomega.BeTrue())
				}

				ginkgo.By("Simulating Velero BackupStorageLocation update to finished state")

				veleroBsl.Status = velerov1.BackupStorageLocationStatus{
					Phase: velerov1.BackupStorageLocationPhaseAvailable,
				}
				gomega.Expect(k8sClient.Update(ctxTimeout, veleroBsl)).To(gomega.Succeed())

				ginkgo.By("Velero BackupStorageLocation updated")

				// wait NonAdminBackupStorageLocation reconcile
				gomega.Eventually(func() (bool, error) {
					err := k8sClient.Get(
						ctxTimeout,
						types.NamespacedName{
							Name:      nonAdminBslName,
							Namespace: nonAdminBslNamespace,
						},
						nonAdminBsl,
					)
					if err != nil {
						return false, err
					}
					if nonAdminBsl == nil || nonAdminBsl.Status.VeleroBackupStorageLocation == nil || nonAdminBsl.Status.VeleroBackupStorageLocation.Status == nil {
						return false, nil
					}
					return nonAdminBsl.Status.VeleroBackupStorageLocation.Status.Phase == velerov1.BackupStorageLocationPhaseAvailable, nil
				}, 5*time.Second, 1*time.Second).Should(gomega.BeTrue())
			}

			ginkgo.By("Waiting NonAdminBackupStorageLocation deletion")
			gomega.Expect(k8sClient.Delete(ctxTimeout, nonAdminBsl)).To(gomega.Succeed())
			gomega.Eventually(func() (bool, error) {
				err := k8sClient.Get(
					ctxTimeout,
					types.NamespacedName{
						Name:      nonAdminBslName,
						Namespace: nonAdminBslNamespace,
					},
					nonAdminBsl,
				)
				if apierrors.IsNotFound(err) {
					return true, nil
				}
				return false, err
			}, 10*time.Second, 1*time.Second).Should(gomega.BeTrue())
			if scenario.expectedStatus.VeleroBackupStorageLocation != nil && len(nonAdminBsl.Status.VeleroBackupStorageLocation.NACUUID) > 0 {
				gomega.Eventually(func() (bool, error) {
					err := k8sClient.Get(
						ctxTimeout,
						types.NamespacedName{
							Name:      nonAdminBsl.Status.VeleroBackupStorageLocation.Name,
							Namespace: oadpNamespace,
						},
						veleroBsl,
					)
					if apierrors.IsNotFound(err) {
						return true, nil
					}
					return false, err
				}, 10*time.Second, 1*time.Second).Should(gomega.BeTrue())
			}
		}, ginkgo.Entry("Should fail with NonAdminBackupStorageLocation due to missing credential name", nonAdminBackupStorageLocationFullReconcileScenario{
			createNonAdminSecret: false,
			spec: nacv1alpha1.NonAdminBackupStorageLocationSpec{
				BackupStorageLocationSpec: &velerov1.BackupStorageLocationSpec{
					Credential: &corev1.SecretKeySelector{
						Key: "cloud",
					},
					AccessMode: velerov1.BackupStorageLocationAccessModeReadWrite,
					Provider:   "aws",
					StorageType: velerov1.StorageType{
						ObjectStorage: &velerov1.ObjectStorageLocation{
							Bucket: "test",
						},
					},
				},
			},
			expectedStatus: nacv1alpha1.NonAdminBackupStorageLocationStatus{
				Phase: nacv1alpha1.NonAdminPhaseBackingOff,
				Conditions: []metav1.Condition{
					{
						Type:               "Accepted",
						Status:             metav1.ConditionFalse,
						Reason:             "BslSpecValidation",
						Message:            "NonAdminBackupStorageLocation spec.bslSpec.credential.name or spec.bslSpec.credential.key is not set",
						LastTransitionTime: metav1.NewTime(time.Now()),
					},
				},
			},
		}),
		ginkgo.Entry("Should fail with NonAdminBackupStorageLocation due to missing secret object", nonAdminBackupStorageLocationFullReconcileScenario{
			createNonAdminSecret: false,
			spec: nacv1alpha1.NonAdminBackupStorageLocationSpec{
				BackupStorageLocationSpec: &velerov1.BackupStorageLocationSpec{
					Provider: "aws",
					StorageType: velerov1.StorageType{
						ObjectStorage: &velerov1.ObjectStorageLocation{
							Bucket: "test",
						},
					},
					Config: map[string]string{
						"region": "us-east-1",
					},
					Credential: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "test",
						},
						Key: "cloud",
					},
				},
			},
			expectedStatus: nacv1alpha1.NonAdminBackupStorageLocationStatus{
				Phase: nacv1alpha1.NonAdminPhaseBackingOff,
				Conditions: []metav1.Condition{
					{
						Type:               "Accepted",
						Status:             metav1.ConditionFalse,
						Reason:             "BslSpecValidation",
						Message:            "BSL credentials secret not found: Secret \"test\" not found",
						LastTransitionTime: metav1.NewTime(time.Now()),
					},
				},
			},
		}),
		ginkgo.Entry("Pass with creation of BSL based on the NonAdminBackupStorageLocation spec", nonAdminBackupStorageLocationFullReconcileScenario{
			createNonAdminSecret: true,
			nonAdminSecretName:   "test-secret-name",
			spec: nacv1alpha1.NonAdminBackupStorageLocationSpec{
				BackupStorageLocationSpec: &velerov1.BackupStorageLocationSpec{
					Credential: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "test-secret-name",
						},
						Key: "cloud",
					},
					AccessMode: velerov1.BackupStorageLocationAccessModeReadWrite,
					Provider:   "aws",
					StorageType: velerov1.StorageType{
						ObjectStorage: &velerov1.ObjectStorageLocation{
							Bucket: "test",
						},
					},
				},
			},
			expectedStatus: nacv1alpha1.NonAdminBackupStorageLocationStatus{
				Phase: nacv1alpha1.NonAdminPhaseCreated,
				Conditions: []metav1.Condition{
					{
						Type:               "Accepted",
						Status:             metav1.ConditionTrue,
						Reason:             "BslSpecValidation",
						Message:            "NonAdminBackupStorageLocation spec validation successful",
						LastTransitionTime: metav1.NewTime(time.Now()),
					},
					{
						Type:               "SecretSynced",
						Status:             metav1.ConditionTrue,
						Reason:             "SecretCreated",
						Message:            "Secret successfully created in the OADP namespace",
						LastTransitionTime: metav1.NewTime(time.Now()),
					},
					{
						Type:               "BackupStorageLocationSynced",
						Status:             metav1.ConditionTrue,
						Reason:             "BackupStorageLocationCreated",
						Message:            "BackupStorageLocation successfully created in the OADP namespace",
						LastTransitionTime: metav1.NewTime(time.Now()),
					},
				},
			},
		}),
	)
})

var _ = ginkgo.Describe("ComputePrefixForObjectStorage", func() {
	type prefixTestScenario struct {
		namespace      string
		customPrefix   string
		expectedPrefix string
	}

	ginkgo.DescribeTable("should compute the correct prefix",
		func(sc prefixTestScenario) {
			result := function.ComputePrefixForObjectStorage(sc.namespace, sc.customPrefix)
			gomega.Expect(result).To(gomega.Equal(sc.expectedPrefix))
		},
		ginkgo.Entry("without custom prefix", prefixTestScenario{
			namespace:      "test-nac-ns",
			customPrefix:   "",
			expectedPrefix: "test-nac-ns",
		}),
		ginkgo.Entry("with custom prefix", prefixTestScenario{
			namespace:      "test-nac-ns",
			customPrefix:   "foo",
			expectedPrefix: "test-nac-ns-foo",
		}),
	)
})
