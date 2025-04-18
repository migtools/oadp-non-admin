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
	velerov2alpha1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v2alpha1"
	"github.com/vmware-tanzu/velero/pkg/label"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	nacv1alpha1 "github.com/migtools/oadp-non-admin/api/v1alpha1"
	"github.com/migtools/oadp-non-admin/internal/common/constant"
	"github.com/migtools/oadp-non-admin/internal/common/function"
)

type nonAdminBackupClusterValidationScenario struct {
	spec nacv1alpha1.NonAdminBackupSpec
}

type nonAdminBackupSingleReconcileScenario struct {
	resultError                         error
	nonAdminBackupPriorStatus           *nacv1alpha1.NonAdminBackupStatus
	nonAdminBackupSpec                  nacv1alpha1.NonAdminBackupSpec
	nonAdminBackupStorageLocationStatus *nacv1alpha1.NonAdminBackupStorageLocationStatus
	nonAdminBackupExpectedStatus        nacv1alpha1.NonAdminBackupStatus
	result                              reconcile.Result
	createVeleroBackup                  bool
	addFinalizer                        bool
	uuidFromTestCase                    bool
	nonAdminBackupExpectedDeleted       bool
	veleroBackupExpectedDeleted         bool
	addNabDeletionTimestamp             bool
	createNonAdminBackupStorageLocation bool
	createVeleroBackupStorageLocation   bool
}

type nonAdminBackupFullReconcileScenario struct {
	enforcedBackupSpec *velerov1.BackupSpec
	spec               nacv1alpha1.NonAdminBackupSpec
	status             nacv1alpha1.NonAdminBackupStatus
	addSyncLabel       bool
	deleteVeleroBackup bool
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
		if nonAdminBackup.Status.VeleroBackup.NACUUID == "" {
			return fmt.Errorf("NonAdminBackup Status VeleroBackupName %v is 0 length string", nonAdminBackup.Status.VeleroBackup.NACUUID)
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
			expectedVeleroBackupSpec := expectedStatus.VeleroBackup.Spec.DeepCopy()
			if expectedVeleroBackupSpec == nil {
				expectedVeleroBackupSpec = &velerov1.BackupSpec{}
			}
			expectedVeleroBackupSpec.IncludedNamespaces = []string{nonAdminBackup.Namespace}
			if !reflect.DeepEqual(nonAdminBackup.Status.VeleroBackup.Spec, expectedVeleroBackupSpec) {
				return fmt.Errorf("NonAdminBackup Status VeleroBackupSpec %v is not equal to expected %v", nonAdminBackup.Status.VeleroBackup.Spec, expectedVeleroBackupSpec)
			}
			if expectedStatus.VeleroBackup.Status != nil {
				if !reflect.DeepEqual(nonAdminBackup.Status.VeleroBackup.Status, expectedStatus.VeleroBackup.Status) {
					return fmt.Errorf("NonAdminBackup Status VeleroBackupStatus %v is not equal to expected %v", nonAdminBackup.Status.VeleroBackup.Status, expectedStatus.VeleroBackup.Status)
				}
			}
		}
	} else {
		if expectedStatus.VeleroBackup != nil && expectedStatus.VeleroBackup.Spec != nil {
			return fmt.Errorf("NonAdminBackup Status VeleroBackup <nil> is not equal to expected VeleroBackupSpec %v", expectedStatus.VeleroBackup.Spec)
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

	if nonAdminBackup.Status.QueueInfo != nil && expectedStatus.QueueInfo != nil {
		if nonAdminBackup.Status.QueueInfo.EstimatedQueuePosition != expectedStatus.QueueInfo.EstimatedQueuePosition {
			return fmt.Errorf("NonAdminBackup Status QueueInfo EstimatedQueuePosition %v is not equal to expected %v", nonAdminBackup.Status.QueueInfo.EstimatedQueuePosition, expectedStatus.QueueInfo.EstimatedQueuePosition)
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

var _ = ginkgo.Describe("Test NonAdminBackup in cluster validation", func() {
	var (
		ctx                     context.Context
		nonAdminBackupName      string
		nonAdminBackupNamespace string
		counter                 int
	)

	ginkgo.BeforeEach(func() {
		ctx = context.Background()
		counter++
		nonAdminBackupName = fmt.Sprintf("non-admin-backup-object-%v", counter)
		nonAdminBackupNamespace = fmt.Sprintf("test-non-admin-backup-cluster-validation-%v", counter)

		nonAdminNamespace := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: nonAdminBackupNamespace,
			},
		}
		gomega.Expect(k8sClient.Create(ctx, nonAdminNamespace)).To(gomega.Succeed())
	})

	ginkgo.AfterEach(func() {
		nonAdminNamespace := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: nonAdminBackupNamespace,
			},
		}
		gomega.Expect(k8sClient.Delete(ctx, nonAdminNamespace)).To(gomega.Succeed())
	})

	ginkgo.DescribeTable("Validation is false",
		func(scenario nonAdminBackupClusterValidationScenario) {
			nonAdminBackup := buildTestNonAdminBackup(nonAdminBackupNamespace, nonAdminBackupName, scenario.spec)
			err := k8sClient.Create(ctx, nonAdminBackup)
			gomega.Expect(err).To(gomega.HaveOccurred())
			gomega.Expect(err.Error()).To(gomega.ContainSubstring("Required value"))
		},
		ginkgo.Entry("Should NOT create NonAdminBackup without spec.backupSpec", nonAdminBackupClusterValidationScenario{
			spec: nacv1alpha1.NonAdminBackupSpec{},
		}),
	)
})

var _ = ginkgo.Describe("Test single reconciles of NonAdminBackup Reconcile function", func() {
	var (
		ctx                               = context.Background()
		nonAdminObjectName                string
		nonAdminObjectNamespace           string
		nonAdminBackupStorageLocationName string
		veleroBSLName                     string
		oadpNamespace                     string
		veleroBackupNACUUID               string
		veleroBSLUUID                     string
		counter                           = 0
	)
	ginkgo.BeforeEach(func() {
		counter++
		nonAdminObjectName = fmt.Sprintf("nab-object-%v", counter)
		nonAdminBackupStorageLocationName = fmt.Sprintf("nab-storage-location-%v", counter)
		veleroBSLName = fmt.Sprintf("velero-bsl-%v", counter)

		nonAdminObjectNamespace = fmt.Sprintf("test-nab-reconcile-%v", counter)
		oadpNamespace = nonAdminObjectNamespace + "-oadp"
		veleroBackupNACUUID = function.GenerateNacObjectUUID(nonAdminObjectNamespace, nonAdminObjectName)
		veleroBSLUUID = function.GenerateNacObjectUUID(oadpNamespace, veleroBSLName)

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
		nonAdminBackupStorageLocation := &nacv1alpha1.NonAdminBackupStorageLocation{}
		if k8sClient.Get(
			ctx,
			types.NamespacedName{
				Name:      nonAdminBackupStorageLocationName,
				Namespace: nonAdminObjectNamespace,
			},
			nonAdminBackupStorageLocation,
		) == nil {
			gomega.Expect(k8sClient.Delete(ctx, nonAdminBackupStorageLocation)).To(gomega.Succeed())
		}
		veleroBackupStorageLocation := &velerov1.BackupStorageLocation{}
		if k8sClient.Get(
			ctx,
			types.NamespacedName{
				Name:      veleroBSLName,
				Namespace: oadpNamespace,
			},
			veleroBackupStorageLocation,
		) == nil {
			gomega.Expect(k8sClient.Delete(ctx, veleroBackupStorageLocation)).To(gomega.Succeed())
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

			if scenario.createNonAdminBackupStorageLocation {
				// Define a default BSL spec
				bslSpec := &velerov1.BackupStorageLocationSpec{
					Credential: &corev1.SecretKeySelector{
						Key: "cloud",
					},
					AccessMode: velerov1.BackupStorageLocationAccessModeReadWrite,
					Provider:   "aws",
					StorageType: velerov1.StorageType{
						ObjectStorage: &velerov1.ObjectStorageLocation{
							Bucket: "test",
							Prefix: "test",
						},
					},
				}
				nonAdminBackupStorageLocation := &nacv1alpha1.NonAdminBackupStorageLocation{
					ObjectMeta: metav1.ObjectMeta{
						Name:      nonAdminBackupStorageLocationName,
						Namespace: nonAdminObjectNamespace,
					},
					Spec: nacv1alpha1.NonAdminBackupStorageLocationSpec{
						BackupStorageLocationSpec: bslSpec,
					},
				}
				gomega.Expect(k8sClient.Create(ctx, nonAdminBackupStorageLocation)).To(gomega.Succeed())

				nonAdminBackupStorageLocation.Status = *scenario.nonAdminBackupStorageLocationStatus
				gomega.Expect(k8sClient.Status().Update(ctx, nonAdminBackupStorageLocation)).To(gomega.Succeed())

				nonAdminBackup.Spec.BackupSpec.StorageLocation = nonAdminBackupStorageLocationName

				// Ensure that the NABSL object is created
				gomega.Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      nonAdminBackupStorageLocationName,
					Namespace: nonAdminObjectNamespace,
				}, nonAdminBackupStorageLocation)).To(gomega.Succeed())

				if scenario.createVeleroBackupStorageLocation {
					veleroBackupStorageLocation := &velerov1.BackupStorageLocation{
						ObjectMeta: metav1.ObjectMeta{
							Name:      veleroBSLName,
							Namespace: oadpNamespace,
							Labels: map[string]string{
								constant.NabslOriginNACUUIDLabel: veleroBSLUUID,
							},
						},
						Spec: *bslSpec,
					}
					gomega.Expect(k8sClient.Create(ctx, veleroBackupStorageLocation)).To(gomega.Succeed())

					veleroBackupStorageLocation.Status = velerov1.BackupStorageLocationStatus{
						Phase: velerov1.BackupStorageLocationPhaseAvailable,
					}

					gomega.Expect(k8sClient.Update(ctx, veleroBackupStorageLocation)).To(gomega.Succeed())

					nonAdminBackupStorageLocation.Status.VeleroBackupStorageLocation = &nacv1alpha1.VeleroBackupStorageLocation{
						NACUUID: veleroBSLUUID,
					}
					gomega.Expect(k8sClient.Status().Update(ctx, nonAdminBackupStorageLocation)).To(gomega.Succeed())

					// Ensure that the Velero BSL object is created
					gomega.Expect(k8sClient.Get(ctx, types.NamespacedName{
						Name:      veleroBSLName,
						Namespace: oadpNamespace,
					}, veleroBackupStorageLocation)).To(gomega.Succeed())
				}
			}

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
						NACUUID:   veleroBackupNACUUID,
						Namespace: oadpNamespace,
					}
				}
				gomega.Expect(k8sClient.Status().Update(ctx, nonAdminBackupAfterCreate)).To(gomega.Succeed())
			}
			if scenario.createVeleroBackup {
				veleroBackup := &velerov1.Backup{
					ObjectMeta: metav1.ObjectMeta{
						Name:      nonAdminBackupAfterCreate.Status.VeleroBackup.NACUUID,
						Namespace: oadpNamespace,
						Labels: map[string]string{
							constant.OadpLabel:             constant.OadpLabelValue,
							constant.ManagedByLabel:        constant.ManagedByLabelValue,
							constant.NabOriginNACUUIDLabel: nonAdminBackupAfterCreate.Status.VeleroBackup.NACUUID,
						},
						Annotations: function.GetNonAdminBackupAnnotations(nonAdminBackup.ObjectMeta),
					},
					Spec: velerov1.BackupSpec{
						IncludedNamespaces: []string{nonAdminObjectNamespace},
					},
				}
				gomega.Expect(k8sClient.Create(ctx, veleroBackup)).To(gomega.Succeed())
			}

			// DeletionTimestamp is immutable and can only be set by the API server
			// We need to use Delete() instead of trying to set it directly
			if scenario.addNabDeletionTimestamp {
				gomega.Expect(k8sClient.Delete(ctx, nonAdminBackupAfterCreate)).To(gomega.Succeed())
			}

			// easy hack to test that only one update call happens per reconcile
			// priorResourceVersion, err := strconv.Atoi(nonAdminBackup.ResourceVersion)
			// gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))
			result, err := (&NonAdminBackupReconciler{
				Client:             k8sClient,
				Scheme:             testEnv.Scheme,
				OADPNamespace:      oadpNamespace,
				EnforcedBackupSpec: &velerov1.BackupSpec{},
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
			nonAdminBackupErr := k8sClient.Get(
				ctx,
				types.NamespacedName{
					Name:      nonAdminObjectName,
					Namespace: nonAdminObjectNamespace,
				},
				nonAdminBackupAfterReconcile,
			)
			veleroBackup := &velerov1.Backup{}
			if nonAdminBackupAfterReconcile.Status.VeleroBackup != nil && len(nonAdminBackupAfterReconcile.Status.VeleroBackup.NACUUID) > 0 {
				veleroBackupNACUUID = nonAdminBackupAfterReconcile.Status.VeleroBackup.NACUUID
			}
			veleroBackupErr := k8sClient.Get(ctx, types.NamespacedName{
				Name:      veleroBackupNACUUID,
				Namespace: oadpNamespace,
			}, veleroBackup)

			if !scenario.nonAdminBackupExpectedDeleted {
				gomega.Expect(nonAdminBackupErr).To(gomega.Not(gomega.HaveOccurred()))
				gomega.Expect(checkTestNonAdminBackupStatus(nonAdminBackupAfterReconcile, scenario.nonAdminBackupExpectedStatus, oadpNamespace)).To(gomega.Succeed())
				// TODO: Include the following check in the checkTestNonAdminBackupStatus. Note that there is a challenge where variables are used in the scenario
				//       data within nonAdminBackupExpectedStatus. Currently the data there needs to be static.
				if nonAdminBackupAfterReconcile.Status.VeleroBackup != nil {
					gomega.Expect(nonAdminBackupAfterReconcile.Status.VeleroBackup.NACUUID).To(gomega.ContainSubstring(nonAdminObjectNamespace))
					gomega.Expect(nonAdminBackupAfterReconcile.Status.VeleroBackup.Namespace).To(gomega.Equal(oadpNamespace))
				}
			} else {
				gomega.Expect(errors.IsNotFound(nonAdminBackupErr)).To(gomega.BeTrue())
			}

			if !scenario.veleroBackupExpectedDeleted {
				gomega.Expect(veleroBackupErr).To(gomega.Not(gomega.HaveOccurred()))
			} else {
				gomega.Expect(errors.IsNotFound(veleroBackupErr)).To(gomega.BeTrue(), "Expected VeleroBackup to be deleted")
			}

			// easy hack to test that only one update call happens per reconcile
			// currentResourceVersion, err := strconv.Atoi(nonAdminBackup.ResourceVersion)
			// gomega.Expect(err).To(gomega.Not(gomega.HaveOccurred()))
			// gomega.Expect(currentResourceVersion - priorResourceVersion).To(gomega.Equal(1))
		},
		ginkgo.Entry("When triggered by NonAdminBackup Create event with invalid BackupSpec, should update NonAdminBackup phase to BackingOff and exit with terminal error", nonAdminBackupSingleReconcileScenario{
			nonAdminBackupSpec: nacv1alpha1.NonAdminBackupSpec{
				BackupSpec: &velerov1.BackupSpec{
					IncludedNamespaces: []string{"wrong", "wrong-again"},
				},
			},
			nonAdminBackupExpectedStatus: nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminPhaseBackingOff,
				Conditions: []metav1.Condition{
					{
						Type:    string(nacv1alpha1.NonAdminConditionAccepted),
						Status:  metav1.ConditionFalse,
						Reason:  "InvalidBackupSpec",
						Message: fmt.Sprintf(constant.NABRestrictedErr+", can not contain namespaces other than: %s", "spec.backupSpec.includedNamespaces", ""),
					},
				},
			},
			veleroBackupExpectedDeleted: true,
			resultError:                 reconcile.TerminalError(fmt.Errorf(constant.NABRestrictedErr+", can not contain namespaces other than: %s", "spec.backupSpec.includedNamespaces", "")),
		}),
		ginkgo.Entry("When triggered by NonAdminBackup Create event with not existing NonAdminBackupStorageLocation, should update NonAdminBackup phase to BackingOff and exit with terminal error", nonAdminBackupSingleReconcileScenario{
			createNonAdminBackupStorageLocation: false,
			nonAdminBackupSpec: nacv1alpha1.NonAdminBackupSpec{
				BackupSpec: &velerov1.BackupSpec{
					StorageLocation: "wrong",
				},
			},
			nonAdminBackupExpectedStatus: nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminPhaseBackingOff,
				Conditions: []metav1.Condition{
					{
						Type:    string(nacv1alpha1.NonAdminConditionAccepted),
						Status:  metav1.ConditionFalse,
						Reason:  "InvalidBackupSpec",
						Message: "NonAdminBackupStorageLocation not found in the namespace: nonadminbackupstoragelocations.oadp.openshift.io \"wrong\" not found",
					},
				},
			},
			veleroBackupExpectedDeleted: true,
			resultError:                 reconcile.TerminalError(fmt.Errorf("NonAdminBackupStorageLocation not found in the namespace: nonadminbackupstoragelocations.oadp.openshift.io \"wrong\" not found")),
		}),
		ginkgo.Entry("When triggered by NonAdminBackup Create event with NonAdminBackupStorageLocation that does not have corresponding VeleroBackupStorageLocation UUID, should update NonAdminBackup phase to BackingOff", nonAdminBackupSingleReconcileScenario{
			createNonAdminBackupStorageLocation: true,
			createVeleroBackupStorageLocation:   false,
			nonAdminBackupSpec: nacv1alpha1.NonAdminBackupSpec{
				BackupSpec: &velerov1.BackupSpec{},
			},
			nonAdminBackupStorageLocationStatus: &nacv1alpha1.NonAdminBackupStorageLocationStatus{
				Phase:      nacv1alpha1.NonAdminPhaseCreated,
				Conditions: []metav1.Condition{},
			},
			nonAdminBackupExpectedStatus: nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminPhaseBackingOff,
				Conditions: []metav1.Condition{
					{
						Type:    string(nacv1alpha1.NonAdminConditionAccepted),
						Status:  metav1.ConditionFalse,
						Reason:  "InvalidBackupSpec",
						Message: "unable to get VeleroBackupStorageLocation UUID from NonAdminBackupStorageLocation Status",
					},
				},
			},
			veleroBackupExpectedDeleted: true,
			resultError:                 fmt.Errorf("unable to get VeleroBackupStorageLocation UUID from NonAdminBackupStorageLocation Status"),
		}),
		ginkgo.Entry("When triggered by NonAdminBackup Create event with valid NonAdminBackupStorageLocation, should update NonAdminBackup phase to Accepted", nonAdminBackupSingleReconcileScenario{
			createNonAdminBackupStorageLocation: true,
			createVeleroBackupStorageLocation:   true,
			nonAdminBackupSpec: nacv1alpha1.NonAdminBackupSpec{
				BackupSpec: &velerov1.BackupSpec{},
			},
			nonAdminBackupStorageLocationStatus: &nacv1alpha1.NonAdminBackupStorageLocationStatus{
				Phase: nacv1alpha1.NonAdminPhaseCreated,
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
				Phase: nacv1alpha1.NonAdminPhaseCreated,
				Conditions: []metav1.Condition{
					{
						Type:    string(nacv1alpha1.NonAdminConditionAccepted),
						Status:  metav1.ConditionTrue,
						Reason:  "BackupAccepted",
						Message: "backup accepted",
					},
					{
						Type:    string(nacv1alpha1.NonAdminConditionQueued),
						Status:  metav1.ConditionTrue,
						Reason:  "BackupScheduled",
						Message: "Created Velero Backup object",
					},
				},
			},
			result: reconcile.Result{},
		}),
		ginkgo.Entry("When triggered by NonAdminBackup Create event with NonAdminBackupStorageLocation phase different then created, should fail to create Velero Backup object", nonAdminBackupSingleReconcileScenario{
			createNonAdminBackupStorageLocation: true,
			createVeleroBackupStorageLocation:   true,
			nonAdminBackupSpec: nacv1alpha1.NonAdminBackupSpec{
				BackupSpec: &velerov1.BackupSpec{},
			},
			nonAdminBackupStorageLocationStatus: &nacv1alpha1.NonAdminBackupStorageLocationStatus{
				Phase: nacv1alpha1.NonAdminPhaseBackingOff,
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
				Phase: nacv1alpha1.NonAdminPhaseBackingOff,
				Conditions: []metav1.Condition{
					{
						Type:    string(nacv1alpha1.NonAdminConditionAccepted),
						Status:  metav1.ConditionFalse,
						Reason:  "InvalidBackupSpec",
						Message: "NonAdminBackupStorageLocation is not in created state and can not be used for the NonAdminBackup",
					},
				},
			},
			veleroBackupExpectedDeleted: true,
			resultError:                 fmt.Errorf("NonAdminBackupStorageLocation is not in created state and can not be used for the NonAdminBackup"),
		}),
		ginkgo.Entry("When triggered by NonAdminBackup Create event with NonAdminBackupStorageLocation phase BackingOff due to one of the conditions False, should fail to create Velero Backup object", nonAdminBackupSingleReconcileScenario{
			createNonAdminBackupStorageLocation: true,
			createVeleroBackupStorageLocation:   true,
			nonAdminBackupSpec: nacv1alpha1.NonAdminBackupSpec{
				BackupSpec: &velerov1.BackupSpec{},
			},
			nonAdminBackupStorageLocationStatus: &nacv1alpha1.NonAdminBackupStorageLocationStatus{
				Phase: nacv1alpha1.NonAdminPhaseBackingOff,
				Conditions: []metav1.Condition{
					{
						Type:               string(nacv1alpha1.NonAdminConditionAccepted),
						Status:             metav1.ConditionTrue,
						Reason:             "BackupAccepted",
						Message:            "backup accepted",
						LastTransitionTime: metav1.NewTime(time.Now()),
					},
					{
						Type:               string(nacv1alpha1.NonAdminBSLConditionSecretSynced),
						Status:             metav1.ConditionFalse,
						Reason:             "SecretSyncFailed",
						Message:            "secret sync failed",
						LastTransitionTime: metav1.NewTime(time.Now()),
					},
				},
			},
			nonAdminBackupExpectedStatus: nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminPhaseBackingOff,
				Conditions: []metav1.Condition{
					{
						Type:               string(nacv1alpha1.NonAdminConditionAccepted),
						Status:             metav1.ConditionFalse,
						Reason:             "InvalidBackupSpec",
						Message:            "NonAdminBackupStorageLocation is not in created state and can not be used for the NonAdminBackup",
						LastTransitionTime: metav1.NewTime(time.Now()),
					},
				},
			},
			veleroBackupExpectedDeleted: true,
			resultError:                 fmt.Errorf("NonAdminBackupStorageLocation is not in created state and can not be used for the NonAdminBackup"),
		}),
		ginkgo.Entry("When triggered by NonAdminBackup deleteNonAdmin spec field when BackupSpec is invalid, should delete NonAdminBackup without error", nonAdminBackupSingleReconcileScenario{
			nonAdminBackupSpec: nacv1alpha1.NonAdminBackupSpec{
				BackupSpec: &velerov1.BackupSpec{
					IncludedNamespaces: []string{"wrong", "wrong-again"},
				},
				DeleteBackup: true,
			},
			nonAdminBackupPriorStatus: &nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminPhaseBackingOff,
				Conditions: []metav1.Condition{
					{
						Type:               string(nacv1alpha1.NonAdminConditionAccepted),
						Status:             metav1.ConditionFalse,
						Reason:             "InvalidBackupSpec",
						Message:            "BackupSpec is not defined",
						LastTransitionTime: metav1.NewTime(time.Now()),
					},
				},
			},
			nonAdminBackupExpectedDeleted: true,
			veleroBackupExpectedDeleted:   true,
			result:                        reconcile.Result{Requeue: true},
		}),
		ginkgo.Entry("When triggered by NonAdminBackup deleteNonAdmin spec field with Finalizer set, should not delete NonAdminBackup as it's waiting for finalizer to be removed", nonAdminBackupSingleReconcileScenario{
			addFinalizer:       true,
			createVeleroBackup: true,
			uuidFromTestCase:   true,
			nonAdminBackupSpec: nacv1alpha1.NonAdminBackupSpec{
				BackupSpec:   &velerov1.BackupSpec{},
				DeleteBackup: true,
			},
			nonAdminBackupPriorStatus: &nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminPhaseCreated,
				Conditions: []metav1.Condition{
					{
						Type:               string(nacv1alpha1.NonAdminConditionAccepted),
						Status:             metav1.ConditionTrue,
						Reason:             "BackupAccepted",
						Message:            "backup accepted",
						LastTransitionTime: metav1.NewTime(time.Now()),
					},
					{
						Type:               string(nacv1alpha1.NonAdminConditionQueued),
						Status:             metav1.ConditionTrue,
						Reason:             "BackupScheduled",
						Message:            "Created Velero Backup object",
						LastTransitionTime: metav1.NewTime(time.Now()),
					},
				},
			},
			nonAdminBackupExpectedStatus: nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminPhaseDeleting,
				Conditions: []metav1.Condition{
					{
						Type:    string(nacv1alpha1.NonAdminConditionAccepted),
						Status:  metav1.ConditionTrue,
						Reason:  "BackupAccepted",
						Message: "backup accepted",
					},
					{
						Type:    string(nacv1alpha1.NonAdminConditionQueued),
						Status:  metav1.ConditionTrue,
						Reason:  "BackupScheduled",
						Message: "Created Velero Backup object",
					},
					{
						Type:    string(nacv1alpha1.NonAdminConditionDeleting),
						Status:  metav1.ConditionTrue,
						Reason:  "DeletionPending",
						Message: "backup accepted for deletion",
					},
				},
			},
			nonAdminBackupExpectedDeleted: false,
			result:                        reconcile.Result{Requeue: true},
		}),
		ginkgo.Entry("When triggered by NonAdminBackup deleteNonAdmin spec field with Finalizer set, should not delete NonAdminBackup as it's waiting for Velero Backup object to be deleted", nonAdminBackupSingleReconcileScenario{
			createVeleroBackup:      true,
			addFinalizer:            true,
			addNabDeletionTimestamp: true,
			nonAdminBackupSpec: nacv1alpha1.NonAdminBackupSpec{
				BackupSpec:   &velerov1.BackupSpec{},
				DeleteBackup: true,
			},
			nonAdminBackupPriorStatus: &nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminPhaseDeleting,
				Conditions: []metav1.Condition{
					{
						Type:               string(nacv1alpha1.NonAdminConditionAccepted),
						Status:             metav1.ConditionTrue,
						Reason:             "BackupAccepted",
						Message:            "backup accepted",
						LastTransitionTime: metav1.NewTime(time.Now()),
					},
					{
						Type:               string(nacv1alpha1.NonAdminConditionQueued),
						Status:             metav1.ConditionTrue,
						Reason:             "BackupScheduled",
						Message:            "Created Velero Backup object",
						LastTransitionTime: metav1.NewTime(time.Now()),
					},
					{
						Type:               string(nacv1alpha1.NonAdminConditionDeleting),
						Status:             metav1.ConditionTrue,
						Reason:             "DeletionPending",
						Message:            "backup accepted for deletion",
						LastTransitionTime: metav1.NewTime(time.Now()),
					},
				},
			},
			nonAdminBackupExpectedStatus: nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminPhaseDeleting,
				Conditions: []metav1.Condition{
					{
						Type:    string(nacv1alpha1.NonAdminConditionAccepted),
						Status:  metav1.ConditionTrue,
						Reason:  "BackupAccepted",
						Message: "backup accepted",
					},
					{
						Type:    string(nacv1alpha1.NonAdminConditionQueued),
						Status:  metav1.ConditionTrue,
						Reason:  "BackupScheduled",
						Message: "Created Velero Backup object",
					},
					{
						Type:    string(nacv1alpha1.NonAdminConditionDeleting),
						Status:  metav1.ConditionTrue,
						Reason:  "DeletionPending",
						Message: "backup accepted for deletion",
					},
				},
			},
			uuidFromTestCase:              true,
			nonAdminBackupExpectedDeleted: false,
			result:                        reconcile.Result{Requeue: false},
		}),
		ginkgo.Entry("When triggered by NonAdminBackup deleteNonAdmin spec field with Finalizer unset, should delete NonAdminBackup", nonAdminBackupSingleReconcileScenario{
			createVeleroBackup: true,
			uuidFromTestCase:   true,
			nonAdminBackupSpec: nacv1alpha1.NonAdminBackupSpec{
				BackupSpec:   &velerov1.BackupSpec{},
				DeleteBackup: true,
			},
			nonAdminBackupPriorStatus: &nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminPhaseCreated,
				Conditions: []metav1.Condition{
					{
						Type:               string(nacv1alpha1.NonAdminConditionAccepted),
						Status:             metav1.ConditionTrue,
						Reason:             "BackupAccepted",
						Message:            "backup accepted",
						LastTransitionTime: metav1.NewTime(time.Now()),
					},
					{
						Type:               string(nacv1alpha1.NonAdminConditionQueued),
						Status:             metav1.ConditionTrue,
						Reason:             "BackupScheduled",
						Message:            "Created Velero Backup object",
						LastTransitionTime: metav1.NewTime(time.Now()),
					},
				},
			},
			nonAdminBackupExpectedDeleted: true,
			result:                        reconcile.Result{Requeue: true},
		}),
		ginkgo.Entry("When triggered by VeleroBackup Delete event, should delete NonAdminBackup and exit", nonAdminBackupSingleReconcileScenario{
			addFinalizer:            true,
			addNabDeletionTimestamp: true,
			nonAdminBackupSpec: nacv1alpha1.NonAdminBackupSpec{
				BackupSpec: &velerov1.BackupSpec{},
			},
			nonAdminBackupPriorStatus: &nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminPhaseDeleting,
				Conditions: []metav1.Condition{
					{
						Type:               string(nacv1alpha1.NonAdminConditionAccepted),
						Status:             metav1.ConditionTrue,
						Reason:             "BackupAccepted",
						Message:            "backup accepted",
						LastTransitionTime: metav1.NewTime(time.Now()),
					},
					{
						Type:               string(nacv1alpha1.NonAdminConditionQueued),
						Status:             metav1.ConditionTrue,
						Reason:             "BackupScheduled",
						Message:            "Created Velero Backup object",
						LastTransitionTime: metav1.NewTime(time.Now()),
					},
					{
						Type:               string(nacv1alpha1.NonAdminConditionDeleting),
						Status:             metav1.ConditionTrue,
						Reason:             "DeletionPending",
						Message:            "permanent backup deletion requires setting spec.deleteBackup to true",
						LastTransitionTime: metav1.NewTime(time.Now()),
					},
				},
			},
			uuidFromTestCase:              true,
			nonAdminBackupExpectedDeleted: true,
			veleroBackupExpectedDeleted:   true,
			result:                        reconcile.Result{Requeue: false},
		}),
		ginkgo.Entry("When triggered by NonAdminBackup delete event, should delete associated Velero Backup and Exit", nonAdminBackupSingleReconcileScenario{
			addFinalizer:            true,
			addNabDeletionTimestamp: true,
			nonAdminBackupSpec: nacv1alpha1.NonAdminBackupSpec{
				BackupSpec: &velerov1.BackupSpec{},
			},
			nonAdminBackupPriorStatus: &nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminPhaseCreated,
				Conditions: []metav1.Condition{
					{
						Type:               string(nacv1alpha1.NonAdminConditionAccepted),
						Status:             metav1.ConditionTrue,
						Reason:             "BackupAccepted",
						Message:            "backup accepted",
						LastTransitionTime: metav1.NewTime(time.Now()),
					},
					{
						Type:               string(nacv1alpha1.NonAdminConditionQueued),
						Status:             metav1.ConditionTrue,
						Reason:             "BackupScheduled",
						Message:            "Created Velero Backup object",
						LastTransitionTime: metav1.NewTime(time.Now()),
					},
				},
			},
			nonAdminBackupExpectedStatus: nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminPhaseDeleting,
				Conditions: []metav1.Condition{
					{
						Type:    string(nacv1alpha1.NonAdminConditionAccepted),
						Status:  metav1.ConditionTrue,
						Reason:  "BackupAccepted",
						Message: "backup accepted",
					},
					{
						Type:    string(nacv1alpha1.NonAdminConditionQueued),
						Status:  metav1.ConditionTrue,
						Reason:  "BackupScheduled",
						Message: "Created Velero Backup object",
					},
					{
						Type:    string(nacv1alpha1.NonAdminConditionDeleting),
						Status:  metav1.ConditionTrue,
						Reason:  "DeletionPending",
						Message: "permanent backup deletion requires setting spec.deleteBackup to true",
					},
				},
			},
			createVeleroBackup:          true,
			veleroBackupExpectedDeleted: true,
			uuidFromTestCase:            true,
			result:                      reconcile.Result{Requeue: false},
		}),
		ginkgo.Entry("When triggered by Requeue(NonAdminBackup phase new), should update NonAdminBackup Phase to Created and Condition to Accepted True and NOT Requeue", nonAdminBackupSingleReconcileScenario{
			nonAdminBackupSpec: nacv1alpha1.NonAdminBackupSpec{
				BackupSpec: &velerov1.BackupSpec{},
			},
			nonAdminBackupPriorStatus: &nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminPhaseNew,
			},
			nonAdminBackupExpectedStatus: nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminPhaseCreated,
				Conditions: []metav1.Condition{
					{
						Type:    string(nacv1alpha1.NonAdminConditionAccepted),
						Status:  metav1.ConditionTrue,
						Reason:  "BackupAccepted",
						Message: "backup accepted",
					},
					{
						Type:    string(nacv1alpha1.NonAdminConditionQueued),
						Status:  metav1.ConditionTrue,
						Reason:  "BackupScheduled",
						Message: "Created Velero Backup object",
					},
				},
			},
			result: reconcile.Result{Requeue: false},
		}),
		ginkgo.Entry("When triggered by Requeue(NonAdminBackup phase new; Conditions Accepted True), should update NonAdminBackup Status generated UUID for VeleroBackup and NOT Requeue", nonAdminBackupSingleReconcileScenario{
			nonAdminBackupSpec: nacv1alpha1.NonAdminBackupSpec{
				BackupSpec: &velerov1.BackupSpec{},
			},
			nonAdminBackupPriorStatus: &nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminPhaseNew,
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
				Phase: nacv1alpha1.NonAdminPhaseCreated,
				Conditions: []metav1.Condition{
					{
						Type:    string(nacv1alpha1.NonAdminConditionAccepted),
						Status:  metav1.ConditionTrue,
						Reason:  "BackupAccepted",
						Message: "backup accepted",
					},
					{
						Type:    string(nacv1alpha1.NonAdminConditionQueued),
						Status:  metav1.ConditionTrue,
						Reason:  "BackupScheduled",
						Message: "Created Velero Backup object",
					},
				},
			},
			result: reconcile.Result{Requeue: false},
		}),
		ginkgo.Entry("When triggered by Requeue(NonAdminBackup phase new; Conditions Accepted True; NonAdminBackup Status NACUUID set), should update NonAdminBackup phase to created and Condition to Queued True and Exit, 1 position in queue", nonAdminBackupSingleReconcileScenario{
			addFinalizer: true,
			nonAdminBackupSpec: nacv1alpha1.NonAdminBackupSpec{
				BackupSpec: &velerov1.BackupSpec{},
			},
			nonAdminBackupPriorStatus: &nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminPhaseNew,
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
				QueueInfo: &nacv1alpha1.QueueInfo{
					EstimatedQueuePosition: 1,
				},
				Phase: nacv1alpha1.NonAdminPhaseCreated,
				Conditions: []metav1.Condition{
					{
						Type:    string(nacv1alpha1.NonAdminConditionAccepted),
						Status:  metav1.ConditionTrue,
						Reason:  "BackupAccepted",
						Message: "backup accepted",
					},
					{
						Type:    string(nacv1alpha1.NonAdminConditionQueued),
						Status:  metav1.ConditionTrue,
						Reason:  "BackupScheduled",
						Message: "Created Velero Backup object",
					},
				},
			},
			result: reconcile.Result{},
		}),
		ginkgo.Entry("When triggered by VeleroBackup Update event, should update NonAdminBackup VeleroBackupStatus and Exit, 1st position in queue", nonAdminBackupSingleReconcileScenario{
			createVeleroBackup: true,
			addFinalizer:       true,
			nonAdminBackupSpec: nacv1alpha1.NonAdminBackupSpec{
				BackupSpec: &velerov1.BackupSpec{},
			},
			nonAdminBackupPriorStatus: &nacv1alpha1.NonAdminBackupStatus{
				QueueInfo: &nacv1alpha1.QueueInfo{
					EstimatedQueuePosition: 5,
				},
				Phase:        nacv1alpha1.NonAdminPhaseCreated,
				VeleroBackup: &nacv1alpha1.VeleroBackup{},
				Conditions: []metav1.Condition{
					{
						Type:               string(nacv1alpha1.NonAdminConditionAccepted),
						Status:             metav1.ConditionTrue,
						Reason:             "BackupAccepted",
						Message:            "backup accepted",
						LastTransitionTime: metav1.NewTime(time.Now()),
					},
					{
						Type:               string(nacv1alpha1.NonAdminConditionQueued),
						Status:             metav1.ConditionTrue,
						Reason:             "BackupScheduled",
						Message:            "Created Velero Backup object",
						LastTransitionTime: metav1.NewTime(time.Now()),
					},
				},
			},
			nonAdminBackupExpectedStatus: nacv1alpha1.NonAdminBackupStatus{
				QueueInfo: &nacv1alpha1.QueueInfo{
					EstimatedQueuePosition: 1,
				},
				Phase: nacv1alpha1.NonAdminPhaseCreated,
				VeleroBackup: &nacv1alpha1.VeleroBackup{
					Status: &velerov1.BackupStatus{},
				},
				Conditions: []metav1.Condition{
					{
						Type:    string(nacv1alpha1.NonAdminConditionAccepted),
						Status:  metav1.ConditionTrue,
						Reason:  "BackupAccepted",
						Message: "backup accepted",
					},
					{
						Type:    string(nacv1alpha1.NonAdminConditionQueued),
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
				Phase: nacv1alpha1.NonAdminPhaseNew,
			},
			nonAdminBackupExpectedStatus: nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminPhaseBackingOff,
				Conditions: []metav1.Condition{
					{
						Type:    string(nacv1alpha1.NonAdminConditionAccepted),
						Status:  metav1.ConditionFalse,
						Reason:  "InvalidBackupSpec",
						Message: fmt.Sprintf(constant.NABRestrictedErr+", can not contain namespaces other than: %s", "spec.backupSpec.includedNamespaces", ""),
					},
				},
			},
			veleroBackupExpectedDeleted: true,
			resultError:                 reconcile.TerminalError(fmt.Errorf(constant.NABRestrictedErr+", can not contain namespaces other than: %s", "spec.backupSpec.includedNamespaces", "")),
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

		// wait manager shutdown
		gomega.Eventually(func() (bool, error) {
			logOutput := ginkgo.CurrentSpecReport().CapturedGinkgoWriterOutput
			shutdownlog := "INFO	Wait completed, proceeding to shutdown the manager"
			return strings.Contains(logOutput, shutdownlog) && strings.Count(logOutput, shutdownlog) == 1, nil
		}, 5*time.Second, 1*time.Second).Should(gomega.BeTrue())
	})

	ginkgo.DescribeTable("Reconcile triggered by NonAdminBackup Create event",
		func(scenario nonAdminBackupFullReconcileScenario) {
			ctx, cancel = context.WithCancel(context.Background())

			gomega.Expect(createTestNamespaces(ctx, nonAdminObjectNamespace, oadpNamespace)).To(gomega.Succeed())

			k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
				Controller: config.Controller{
					SkipNameValidation: ptr.To(true),
				},
				Scheme: k8sClient.Scheme(),
				Cache: cache.Options{
					DefaultNamespaces: map[string]cache.Config{
						nonAdminObjectNamespace: {},
						oadpNamespace:           {},
					},
				},
			})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			enforcedBackupSpec := &velerov1.BackupSpec{}
			if scenario.enforcedBackupSpec != nil {
				enforcedBackupSpec = scenario.enforcedBackupSpec
			}
			err = (&NonAdminBackupReconciler{
				Client:             k8sManager.GetClient(),
				Scheme:             k8sManager.GetScheme(),
				OADPNamespace:      oadpNamespace,
				EnforcedBackupSpec: enforcedBackupSpec,
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
			ctxTimeout, cancel2 := context.WithTimeout(ctx, managerStartTimeout)
			defer cancel2()

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
			if scenario.addSyncLabel {
				nonAdminBackup.Labels = map[string]string{
					constant.NabSyncLabel: "malicious-injection",
				}
			}
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

			veleroBackup := &velerov1.Backup{}
			veleroPodVolumeBackup := &velerov1.PodVolumeBackup{}
			veleroDataUpload := &velerov2alpha1.DataUpload{}
			if scenario.status.VeleroBackup != nil {
				gomega.Expect(k8sClient.Get(
					ctxTimeout,
					types.NamespacedName{
						Name:      nonAdminBackup.Status.VeleroBackup.NACUUID,
						Namespace: oadpNamespace,
					},
					veleroBackup,
				)).To(gomega.Succeed())
				if scenario.deleteVeleroBackup {
					ginkgo.By("Simulating VeleroBackup deletion")
					gomega.Expect(k8sClient.Delete(ctxTimeout, veleroBackup)).To(gomega.Succeed())

					gomega.Eventually(func() (bool, error) {
						err := k8sClient.Get(
							ctxTimeout,
							types.NamespacedName{
								Name:      nonAdminBackup.Status.VeleroBackup.Name,
								Namespace: oadpNamespace,
							},
							veleroBackup,
						)
						if errors.IsNotFound(err) {
							return true, nil
						}
						return false, err
					}, 10*time.Second, 1*time.Second).Should(gomega.BeTrue())

					// wait NAB reconcile
					time.Sleep(2 * time.Second)

					gomega.Expect(k8sClient.Get(
						ctxTimeout,
						types.NamespacedName{
							Name:      nonAdminObjectName,
							Namespace: nonAdminObjectNamespace,
						},
						nonAdminBackup,
					)).To(gomega.Succeed())
				} else {
					veleroPodVolumeBackup = &velerov1.PodVolumeBackup{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test",
							Namespace: oadpNamespace,
							OwnerReferences: []metav1.OwnerReference{
								{
									Kind:       "Backup",
									APIVersion: velerov1.SchemeGroupVersion.String(),
									Name:       veleroBackup.Name,
									UID:        veleroBackup.UID,
								},
							},
							Labels: map[string]string{
								velerov1.BackupNameLabel: label.GetValidName(veleroBackup.Name),
							},
						},
					}
					gomega.Expect(k8sClient.Create(ctxTimeout, veleroPodVolumeBackup)).To(gomega.Succeed())

					veleroDataUpload = &velerov2alpha1.DataUpload{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test",
							Namespace: oadpNamespace,
							OwnerReferences: []metav1.OwnerReference{
								{
									Kind:       "Backup",
									APIVersion: velerov1.SchemeGroupVersion.String(),
									Name:       veleroBackup.Name,
									UID:        veleroBackup.UID,
								},
							},
							Labels: map[string]string{
								velerov1.BackupNameLabel: label.GetValidName(veleroBackup.Name),
							},
						},
					}
					gomega.Expect(k8sClient.Create(ctxTimeout, veleroDataUpload)).To(gomega.Succeed())
				}
			}

			ginkgo.By("Validating NonAdminBackup Status")

			gomega.Expect(checkTestNonAdminBackupStatus(nonAdminBackup, scenario.status, oadpNamespace)).To(gomega.Succeed())

			if scenario.status.VeleroBackup != nil && len(nonAdminBackup.Status.VeleroBackup.NACUUID) > 0 {
				ginkgo.By("Checking if NonAdminBackup Spec was not changed")
				gomega.Expect(reflect.DeepEqual(
					nonAdminBackup.Spec,
					scenario.spec,
				)).To(gomega.BeTrue())

				if !scenario.deleteVeleroBackup {
					if scenario.enforcedBackupSpec != nil {
						ginkgo.By("Validating Velero Backup Spec")
						expectedSpec := scenario.enforcedBackupSpec.DeepCopy()
						expectedSpec.IncludedNamespaces = []string{nonAdminObjectNamespace}
						expectedSpec.ExcludedResources = []string{
							nacv1alpha1.NonAdminBackups,
							nacv1alpha1.NonAdminRestores,
							nacv1alpha1.NonAdminBackupStorageLocations,
							"securitycontextconstraints",
							"clusterroles",
							"clusterrolebindings",
							"priorityclasses",
							"customresourcedefinitions",
							"virtualmachineclusterinstancetypes",
							"virtualmachineclusterpreferences",
						}
						gomega.Expect(reflect.DeepEqual(veleroBackup.Spec, *expectedSpec)).To(gomega.BeTrue())
					}

					ginkgo.By("Simulating VeleroBackup update to finished state")

					veleroBackup.Status = velerov1.BackupStatus{
						Phase:               velerov1.BackupPhaseCompleted,
						CompletionTimestamp: &metav1.Time{Time: time.Now()},
					}
					veleroPodVolumeBackup.Status = velerov1.PodVolumeBackupStatus{
						Phase: velerov1.PodVolumeBackupPhaseCompleted,
					}
					veleroDataUpload.Status = velerov2alpha1.DataUploadStatus{
						Phase: velerov2alpha1.DataUploadPhaseCompleted,
					}

					// can not call .Status().Update() for veleroBackup object https://github.com/vmware-tanzu/velero/issues/8285
					gomega.Expect(k8sClient.Update(ctxTimeout, veleroBackup)).To(gomega.Succeed())
					gomega.Expect(k8sClient.Update(ctxTimeout, veleroPodVolumeBackup)).To(gomega.Succeed())
					gomega.Expect(k8sClient.Update(ctxTimeout, veleroDataUpload)).To(gomega.Succeed())

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
						if nonAdminBackup == nil ||
							nonAdminBackup.Status.VeleroBackup == nil ||
							nonAdminBackup.Status.VeleroBackup.Status == nil ||
							nonAdminBackup.Status.FileSystemPodVolumeBackups == nil ||
							nonAdminBackup.Status.DataMoverDataUploads == nil {
							return false, nil
						}
						return nonAdminBackup.Status.VeleroBackup.Status.Phase == velerov1.BackupPhaseCompleted &&
							nonAdminBackup.Status.FileSystemPodVolumeBackups.Completed == 1 &&
							nonAdminBackup.Status.DataMoverDataUploads.Completed == 1, nil
					}, 5*time.Second, 1*time.Second).Should(gomega.BeTrue())
				}
			}

			ginkgo.By("Waiting Reconcile of delete event")
			gomega.Expect(k8sClient.Delete(ctxTimeout, nonAdminBackup)).To(gomega.Succeed())
			gomega.Eventually(func() (bool, error) {
				err := k8sClient.Get(
					ctxTimeout,
					types.NamespacedName{
						Name:      nonAdminObjectName,
						Namespace: nonAdminObjectNamespace,
					},
					nonAdminBackup,
				)
				if errors.IsNotFound(err) {
					return true, nil
				}
				return false, err
			}, 10*time.Second, 1*time.Second).Should(gomega.BeTrue())
			if scenario.status.VeleroBackup != nil && len(nonAdminBackup.Status.VeleroBackup.NACUUID) > 0 {
				gomega.Eventually(func() (bool, error) {
					err := k8sClient.Get(
						ctxTimeout,
						types.NamespacedName{
							Name:      nonAdminBackup.Status.VeleroBackup.Name,
							Namespace: oadpNamespace,
						},
						veleroBackup,
					)
					if errors.IsNotFound(err) {
						return true, nil
					}
					return false, err
				}, 10*time.Second, 1*time.Second).Should(gomega.BeTrue())
			}
		},
		ginkgo.Entry("Should update NonAdminBackup until VeleroBackup completes and then delete it", nonAdminBackupFullReconcileScenario{
			spec: nacv1alpha1.NonAdminBackupSpec{
				BackupSpec: &velerov1.BackupSpec{},
			},
			status: nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminPhaseCreated,
				VeleroBackup: &nacv1alpha1.VeleroBackup{
					Namespace: oadpNamespace,
					Status:    nil,
					Spec: &velerov1.BackupSpec{
						ExcludedResources: []string{
							"nonadminbackups",
							"nonadminrestores",
							"nonadminbackupstoragelocations",
							"securitycontextconstraints",
							"clusterroles",
							"clusterrolebindings",
							"priorityclasses",
							"customresourcedefinitions",
							"virtualmachineclusterinstancetypes",
							"virtualmachineclusterpreferences",
						},
						SnapshotVolumes: ptr.To(false),
						TTL: metav1.Duration{
							Duration: 36 * time.Hour,
						},
						DefaultVolumesToFsBackup: ptr.To(true),
					},
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
			enforcedBackupSpec: &velerov1.BackupSpec{
				SnapshotVolumes: ptr.To(false),
				TTL: metav1.Duration{
					Duration: 36 * time.Hour,
				},
				DefaultVolumesToFsBackup: ptr.To(true),
			},
		}),
		ginkgo.Entry("Should update NonAdminBackup when VeleroBackup is deleted then delete it", nonAdminBackupFullReconcileScenario{
			deleteVeleroBackup: true,
			spec: nacv1alpha1.NonAdminBackupSpec{
				BackupSpec: &velerov1.BackupSpec{},
			},
			status: nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminPhaseBackingOff,
				VeleroBackup: &nacv1alpha1.VeleroBackup{
					Namespace: oadpNamespace,
					Status:    nil,
					Spec: &velerov1.BackupSpec{
						ExcludedResources: []string{
							"nonadminbackups",
							"nonadminrestores",
							"nonadminbackupstoragelocations",
							"securitycontextconstraints",
							"clusterroles",
							"clusterrolebindings",
							"priorityclasses",
							"customresourcedefinitions",
							"virtualmachineclusterinstancetypes",
							"virtualmachineclusterpreferences",
						},
					},
				},
				Conditions: []metav1.Condition{
					{
						Type:    "Accepted",
						Status:  metav1.ConditionFalse,
						Reason:  "VeleroBackupNotFound",
						Message: "Velero Backup has been removed",
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
					SnapshotVolumes: ptr.To(true),
				},
			},
			status: nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminPhaseBackingOff,
				Conditions: []metav1.Condition{
					{
						Type:    "Accepted",
						Status:  metav1.ConditionFalse,
						Reason:  "InvalidBackupSpec",
						Message: "the administrator has restricted spec.backupSpec.snapshotVolumes field value to false",
					},
				},
			},
			enforcedBackupSpec: &velerov1.BackupSpec{
				SnapshotVolumes: ptr.To(false),
			},
		}),
		ginkgo.Entry("Should update malicious NonAdminBackup until it invalidates and then delete it", nonAdminBackupFullReconcileScenario{
			spec: nacv1alpha1.NonAdminBackupSpec{
				BackupSpec: &velerov1.BackupSpec{
					IncludedNamespaces: []string{"openshift-adp", "openshift"},
					SnapshotVolumes:    ptr.To(true),
				},
			},
			status: nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminPhaseBackingOff,
				Conditions: []metav1.Condition{
					{
						Type:    "Accepted",
						Status:  metav1.ConditionFalse,
						Reason:  "VeleroBackupNotFound",
						Message: "related Velero Backup to be synced from does not exist",
					},
				},
			},
			enforcedBackupSpec: &velerov1.BackupSpec{
				SnapshotVolumes: ptr.To(false),
			},
			addSyncLabel: true,
		}),
	)

	ginkgo.DescribeTable("Reconcile triggered by NonAdminBackup sync event",
		func(scenario nonAdminBackupFullReconcileScenario) {
			ctx, cancel = context.WithCancel(context.Background())

			gomega.Expect(createTestNamespaces(ctx, nonAdminObjectNamespace, oadpNamespace)).To(gomega.Succeed())

			veleroBackup := &velerov1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      scenario.status.VeleroBackup.Name,
					Namespace: oadpNamespace,
					Labels: map[string]string{
						constant.OadpLabel:             constant.OadpLabelValue,
						constant.ManagedByLabel:        constant.ManagedByLabelValue,
						constant.NabOriginNACUUIDLabel: scenario.status.VeleroBackup.NACUUID,
					},
					Annotations: map[string]string{
						constant.NabOriginNamespaceAnnotation: nonAdminObjectNamespace,
						constant.NabOriginNameAnnotation:      nonAdminObjectName,
					},
				},
				Spec: *scenario.spec.BackupSpec,
			}
			veleroBackup.Spec.IncludedNamespaces = []string{nonAdminObjectNamespace}
			gomega.Expect(k8sClient.Create(ctx, veleroBackup)).To(gomega.Succeed())

			veleroBackup.Status = velerov1.BackupStatus{
				Phase:               velerov1.BackupPhaseCompleted,
				CompletionTimestamp: scenario.status.VeleroBackup.Status.CompletionTimestamp,
			}
			// can not call .Status().Update() for veleroBackup object https://github.com/vmware-tanzu/velero/issues/8285
			gomega.Expect(k8sClient.Update(ctx, veleroBackup)).To(gomega.Succeed())

			k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
				Controller: config.Controller{
					SkipNameValidation: ptr.To(true),
				},
				Scheme: k8sClient.Scheme(),
				Cache: cache.Options{
					DefaultNamespaces: map[string]cache.Config{
						nonAdminObjectNamespace: {},
						oadpNamespace:           {},
					},
				},
			})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			enforcedBackupSpec := &velerov1.BackupSpec{}
			if scenario.enforcedBackupSpec != nil {
				enforcedBackupSpec = scenario.enforcedBackupSpec
			}
			err = (&NonAdminBackupReconciler{
				Client:             k8sManager.GetClient(),
				Scheme:             k8sManager.GetScheme(),
				OADPNamespace:      oadpNamespace,
				EnforcedBackupSpec: enforcedBackupSpec,
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
				startUpLog := `INFO	Starting workers	{"controller": "nonadminbackup", "controllerGroup": "oadp.openshift.io", "controllerKind": "NonAdminBackup", "worker count": 1}`
				return strings.Contains(logOutput, startUpLog) &&
					strings.Count(logOutput, startUpLog) == 1, nil
			}, 5*time.Second, 1*time.Second).Should(gomega.BeTrue())

			ginkgo.By("Waiting Reconcile of sync event")
			nonAdminBackup := buildTestNonAdminBackup(nonAdminObjectNamespace, nonAdminObjectName, scenario.spec)
			nonAdminBackup.Labels = map[string]string{
				constant.NabSyncLabel: scenario.status.VeleroBackup.NACUUID,
			}
			gomega.Expect(k8sClient.Create(ctx, nonAdminBackup)).To(gomega.Succeed())

			gomega.Eventually(func() (bool, error) {
				err := k8sClient.Get(
					ctx,
					types.NamespacedName{
						Name:      nonAdminObjectName,
						Namespace: nonAdminObjectNamespace,
					},
					nonAdminBackup,
				)
				if err != nil {
					return false, err
				}
				err = checkTestNonAdminBackupStatus(nonAdminBackup, scenario.status, oadpNamespace)
				return err == nil, err
			}, 5*time.Second, 1*time.Second).Should(gomega.BeTrue())

			if scenario.status.VeleroBackup != nil && len(nonAdminBackup.Status.VeleroBackup.NACUUID) > 0 {
				ginkgo.By("Checking if NonAdminBackup Spec was not changed")
				gomega.Expect(reflect.DeepEqual(
					nonAdminBackup.Spec,
					scenario.spec,
				)).To(gomega.BeTrue())
			}

			ginkgo.By("Waiting Reconcile of delete event")
			gomega.Expect(k8sClient.Delete(ctx, nonAdminBackup)).To(gomega.Succeed())
			gomega.Eventually(func() (bool, error) {
				err := k8sClient.Get(
					ctx,
					types.NamespacedName{
						Name:      scenario.status.VeleroBackup.Name,
						Namespace: oadpNamespace,
					},
					veleroBackup,
				)
				if errors.IsNotFound(err) {
					return true, nil
				}
				return false, err
			}, 5*time.Second, 1*time.Second).Should(gomega.BeTrue())
			gomega.Eventually(func() (bool, error) {
				err := k8sClient.Get(
					ctx,
					types.NamespacedName{
						Name:      nonAdminObjectName,
						Namespace: nonAdminObjectNamespace,
					},
					nonAdminBackup,
				)
				if errors.IsNotFound(err) {
					return true, nil
				}
				return false, err
			}, 5*time.Second, 1*time.Second).Should(gomega.BeTrue())
			gomega.Eventually(func() (bool, error) {
				logOutput := ginkgo.CurrentSpecReport().CapturedGinkgoWriterOutput
				deletelog := "DEBUG	Accepted NAB Delete event"
				return strings.Contains(logOutput, deletelog) && strings.Count(logOutput, deletelog) == 1, nil
			}, 5*time.Second, 1*time.Second).Should(gomega.BeTrue())
		},
		ginkgo.Entry("Should re create NonAdminBackup and then delete it", nonAdminBackupFullReconcileScenario{
			spec: nacv1alpha1.NonAdminBackupSpec{
				BackupSpec: &velerov1.BackupSpec{
					TTL: metav1.Duration{
						Duration: 20 * time.Hour,
					},
				},
			},
			status: nacv1alpha1.NonAdminBackupStatus{
				Phase: nacv1alpha1.NonAdminPhaseCreated,
				VeleroBackup: &nacv1alpha1.VeleroBackup{
					Namespace: oadpNamespace,
					Spec: &velerov1.BackupSpec{
						TTL: metav1.Duration{
							Duration: 20 * time.Hour,
						},
					},
					Status: &velerov1.BackupStatus{
						Phase:               velerov1.BackupPhaseCompleted,
						CompletionTimestamp: &metav1.Time{Time: time.Date(2025, 2, 10, 12, 12, 12, 0, time.Local)},
					},
					Name:    "test",
					NACUUID: "test",
				},
				QueueInfo: &nacv1alpha1.QueueInfo{
					EstimatedQueuePosition: 0,
				},
				Conditions: []metav1.Condition{
					{
						Type:    "Queued",
						Status:  metav1.ConditionTrue,
						Reason:  "BackupScheduled",
						Message: "Created Velero Backup object",
					},
				},
			},
			enforcedBackupSpec: &velerov1.BackupSpec{
				TTL: metav1.Duration{
					Duration: 10 * time.Hour,
				},
			},
		}),
	)
})
