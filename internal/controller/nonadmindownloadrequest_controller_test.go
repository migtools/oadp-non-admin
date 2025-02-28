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
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	nacv1alpha1 "github.com/migtools/oadp-non-admin/api/v1alpha1"
	"github.com/migtools/oadp-non-admin/internal/common/constant"
	"github.com/migtools/oadp-non-admin/internal/common/function"
)

var _ = ginkgo.Describe("NonAdminDownloadRequest Controller", func() {
	var (
		ctx                  context.Context
		reconciler           NonAdminDownloadRequestReconciler
		nadr                 *nacv1alpha1.NonAdminDownloadRequest
		nabsl                *nacv1alpha1.NonAdminBackupStorageLocation
		nab                  *nacv1alpha1.NonAdminBackup
		nabDefaultBsl        *nacv1alpha1.NonAdminBackup
		request              reconcile.Request
		fakeClient           client.Client
		expectedDownloadName string
	)
	const (
		testNamespace = "test-namespace"
	)
	ginkgo.BeforeEach(func() {
		ctx = context.Background()
		// nabsl need to exist first
		nabsl = &nacv1alpha1.NonAdminBackupStorageLocation{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "nabsl",
				Namespace: testNamespace,
			},
		}
		// nab need to use nabsl
		nab = &nacv1alpha1.NonAdminBackup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "nab-yo",
				Namespace: testNamespace,
			},
			Spec: nacv1alpha1.NonAdminBackupSpec{
				BackupSpec: &velerov1.BackupSpec{
					StorageLocation: nabsl.Name,
				},
			},
			Status: nacv1alpha1.NonAdminBackupStatus{
				VeleroBackup: &nacv1alpha1.VeleroBackup{
					Name: "its-velero-backup-yo",
				},
			},
		}
		nabDefaultBsl = &nacv1alpha1.NonAdminBackup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "nab-adminbsl",
				Namespace: testNamespace,
			},
			// No NonAdminBackupStorageLocation specified in Spec
			Spec: nacv1alpha1.NonAdminBackupSpec{
				BackupSpec: &velerov1.BackupSpec{},
			},
		}
		nadr = &nacv1alpha1.NonAdminDownloadRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-nadr",
				Namespace: testNamespace,
				UID:       "test-uid", // Set a UID for the test
			},
			Spec: nacv1alpha1.NonAdminDownloadRequestSpec{
				Target: velerov1.DownloadTarget{
					Kind: velerov1.DownloadTargetKindBackupLog,
					Name: nab.Name,
				},
			},
		}
		expectedDownloadName = nadr.VeleroDownloadRequestName()

		// Create a fake client with the NonAdminDownloadRequest object
		fakeClient = fake.NewClientBuilder().WithScheme(scheme.Scheme).
			WithObjects(
				nabsl, nab, nabDefaultBsl, nadr,
			).WithStatusSubresource(nabsl, nab, nabDefaultBsl, nadr).Build()

		// Initialize the reconciler with the fake client
		reconciler = NonAdminDownloadRequestReconciler{
			Client:        fakeClient,
			Scheme:        scheme.Scheme,
			OADPNamespace: testNamespace, // Set a test OADP namespace
		}
		request = reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      nadr.Name,
				Namespace: nadr.Namespace,
			},
		}
		err := nacv1alpha1.AddToScheme(scheme.Scheme)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		err = velerov1.AddToScheme(scheme.Scheme)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	})

	ginkgo.Context("When reconciling a resource", func() {
		ginkgo.It("Should not requeue if the NonAdminDownloadRequest is not found", func() {
			request.NamespacedName.Name = "non-existent-nadr"
			result, err := reconciler.Reconcile(ctx, &nacv1alpha1.NonAdminDownloadRequest{}) // pass object
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(result.Requeue).To(gomega.BeFalse())
		})

		ginkgo.It("Should create a DownloadRequest when a NonAdminDownloadRequest is created", func() {
			result, err := reconciler.Reconcile(ctx, nadr)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(result.Requeue).To(gomega.BeFalse())

			// Check if the DownloadRequest was created
			downloadRequest := &velerov1.DownloadRequest{}
			err = fakeClient.Get(ctx, types.NamespacedName{Name: expectedDownloadName, Namespace: testNamespace}, downloadRequest)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(downloadRequest.Name).To(gomega.Equal(expectedDownloadName))
			gomega.Expect(downloadRequest.Spec.Target.Kind).To(gomega.Equal(nadr.Spec.Target.Kind))
			gomega.Expect(downloadRequest.Spec.Target.Name).To(gomega.Equal(nab.VeleroBackupName()))

			// test labels
			gomega.Expect(downloadRequest.Labels[constant.NadrOriginNACUUIDLabel]).To(gomega.Equal(string(nadr.UID)))
		})

		ginkgo.It("Should update the status when the DownloadRequest is processed", func() {
			// Create the associated DownloadRequest
			downloadRequest := &velerov1.DownloadRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      expectedDownloadName,
					Namespace: testNamespace,
					Labels: func() map[string]string {
						nal := function.GetNonAdminLabels()
						nal[constant.NadrOriginNACUUIDLabel] = string(nadr.GetUID())
						return nal
					}(),
					Annotations:     function.GetNonAdminDownloadRequestAnnotations(nadr),
					ResourceVersion: constant.EmptyString,
				},
				Spec: velerov1.DownloadRequestSpec{
					Target: velerov1.DownloadTarget{
						Kind: nadr.Spec.Target.Kind,
						Name: nadr.Spec.Target.Name,
					},
				},
			}

			// Rebuild the fake client to include the DownloadRequest and its status.
			fakeClient = fake.NewClientBuilder().
				WithScheme(scheme.Scheme).
				WithObjects(nabsl, nab, nadr, downloadRequest).
				WithStatusSubresource(nabsl, nab, nadr, downloadRequest). // Include status subresource for DownloadRequest
				Build()

			reconciler.Client = fakeClient // Update the reconciler's client

			// Simulate Velero processing the DownloadRequest
			downloadRequest.Status.Phase = velerov1.DownloadRequestPhaseProcessed
			downloadRequest.Status.DownloadURL = "http://example.com/download"
			downloadRequest.Status.Expiration = &metav1.Time{Time: time.Now().Add(time.Hour)}
			err := fakeClient.Status().Update(ctx, downloadRequest)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			result, err := reconciler.Reconcile(ctx, nadr)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(result.Requeue).To(gomega.BeFalse()) // no requeue, requeue after is handled separately

			// Check if the NonAdminDownloadRequest status was updated
			updatedNadr := &nacv1alpha1.NonAdminDownloadRequest{}
			err = fakeClient.Get(ctx, types.NamespacedName{Name: nadr.Name, Namespace: nadr.Namespace}, updatedNadr)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(updatedNadr.Status.Phase).To(gomega.Equal(nacv1alpha1.NonAdminPhaseCreated))
			gomega.Expect(updatedNadr.Status.Conditions[0].Type).To(gomega.Equal(string(nacv1alpha1.ConditionNonAdminProcessed)))
			gomega.Expect(updatedNadr.Status.VeleroDownloadRequest.Status.DownloadURL).To(gomega.Equal("http://example.com/download"))
		})

		ginkgo.It("Should delete the NonAdminDownloadRequest and DownloadRequest when expired", func() {
			// Create the associated DownloadRequest
			downloadRequest := &velerov1.DownloadRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      expectedDownloadName,
					Namespace: testNamespace,
					Labels: func() map[string]string {
						nal := function.GetNonAdminLabels()
						nal[constant.NadrOriginNACUUIDLabel] = string(nadr.GetUID())
						return nal
					}(),
					Annotations: function.GetNonAdminDownloadRequestAnnotations(nadr),
				},
				Spec: velerov1.DownloadRequestSpec{
					Target: velerov1.DownloadTarget{
						Kind: nadr.Spec.Target.Kind,
						Name: nadr.Spec.Target.Name,
					},
				},
			}
			err := fakeClient.Create(ctx, downloadRequest)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			// Set the NonAdminDownloadRequest status to expired
			nadr.Status.VeleroDownloadRequest.Status = &velerov1.DownloadRequestStatus{
				Expiration: &metav1.Time{Time: time.Now().Add(-time.Hour)},
			}
			err = fakeClient.Status().Update(ctx, nadr) // Use Status().Update()
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			result, err := reconciler.Reconcile(ctx, nadr)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(result.Requeue).To(gomega.BeFalse())

			// Check if the NonAdminDownloadRequest was deleted
			deletedNadr := &nacv1alpha1.NonAdminDownloadRequest{}
			err = fakeClient.Get(ctx, types.NamespacedName{Name: nadr.Name, Namespace: nadr.Namespace}, deletedNadr)
			gomega.Expect(apierrors.IsNotFound(err)).To(gomega.BeTrue())

			// check if DR was deleted
			deletedDR := &velerov1.DownloadRequest{}
			err = fakeClient.Get(ctx, types.NamespacedName{Name: expectedDownloadName, Namespace: testNamespace}, deletedDR)
			gomega.Expect(apierrors.IsNotFound(err)).To(gomega.BeTrue())
		})

		ginkgo.It("Should requeue when download URL is available and not expired", func() {
			// Set the NonAdminDownloadRequest status with download URL and not expired
			expirationTime := time.Now().Add(time.Hour)
			nadr.Status.VeleroDownloadRequest.Status = &velerov1.DownloadRequestStatus{
				DownloadURL: "http://example.com/download",
				Expiration:  &metav1.Time{Time: expirationTime},
			}
			err := fakeClient.Status().Update(ctx, nadr) // Use Status().Update()
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			result, err := reconciler.Reconcile(ctx, nadr)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(result.RequeueAfter).To(gomega.BeNumerically("~", time.Until(expirationTime), time.Second)) // Allow for some timing differences
		})
		ginkgo.It("Should create DownloadRequest with correct backup name for restore target kinds", func() {
			// Setup for restore target kinds
			restoreName := "test-restore"
			backupName := "test-backup-from-restore"

			// Create a NonAdminRestore referencing the backup
			nar := &nacv1alpha1.NonAdminRestore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      restoreName,
					Namespace: nadr.Namespace,
				},
				Spec: nacv1alpha1.NonAdminRestoreSpec{
					RestoreSpec: &velerov1.RestoreSpec{BackupName: backupName}, // Assuming this field links to NonAdminBackup
				},
			}
			err := fakeClient.Create(ctx, nar)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			// Create a NonAdminBackup
			nab := &nacv1alpha1.NonAdminBackup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      backupName,
					Namespace: nadr.Namespace,
				},
				Spec: nacv1alpha1.NonAdminBackupSpec{
					BackupSpec: &velerov1.BackupSpec{
						StorageLocation: "default",
					},
				},
				Status: nacv1alpha1.NonAdminBackupStatus{
					VeleroBackup: &nacv1alpha1.VeleroBackup{
						Spec: &velerov1.BackupSpec{
							StorageLocation: "default",
						},
					},
				},
			}
			err = fakeClient.Create(ctx, nab)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			// Update NonAdminDownloadRequest for RestoreLog target
			nadr.Spec.Target.Kind = velerov1.DownloadTargetKindRestoreLog
			nadr.Spec.Target.Name = restoreName
			err = fakeClient.Update(ctx, nadr)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			result, err := reconciler.Reconcile(ctx, nadr)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(result.Requeue).To(gomega.BeFalse())

			// Verify DownloadRequest is created and has the correct restore name
			dr := &velerov1.DownloadRequest{}
			err = fakeClient.Get(ctx, types.NamespacedName{Name: expectedDownloadName, Namespace: reconciler.OADPNamespace}, dr)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(dr.Spec.Target.Name).To(gomega.Equal(nar.VeleroRestoreName()))
		})
		ginkgo.It("Should return an error and set status when NonAdminBackupStorageLocation is not used", func() {
			nadr.Spec.Target.Name = nabDefaultBsl.Name
			_, err := reconciler.Reconcile(ctx, nadr)
			gomega.Expect(err).NotTo(gomega.HaveOccurred()) // expect no error from reconcile, the reconcile completes with phase Created

			updatedNadr := &nacv1alpha1.NonAdminDownloadRequest{}
			err = fakeClient.Get(ctx, client.ObjectKeyFromObject(nadr), updatedNadr)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(updatedNadr.Status.Conditions).NotTo(gomega.BeEmpty())
			gomega.Expect(updatedNadr.Status.Conditions[0].Type).To(gomega.Equal(string(nacv1alpha1.ConditionNonAdminBackupStorageLocationNotUsed)))
			gomega.Expect(updatedNadr.Status.Conditions[0].Status).To(gomega.Equal(metav1.ConditionTrue))
			gomega.Expect(updatedNadr.Status.Phase).To(gomega.Equal(nacv1alpha1.NonAdminPhaseCreated))
		})
		ginkgo.It("should error when referring a non-existent NonAdminBackup", func() {
			nadr.Spec.Target.Kind = velerov1.DownloadTargetKindBackupLog
			nadr.Spec.Target.Name = "non-existent-backup"
			nadr.Status.Conditions = []metav1.Condition{} // clear conditions to avoid delete logic.
			err := fakeClient.Update(ctx, nadr)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			_, err = reconciler.Reconcile(ctx, nadr)
			gomega.Expect(err).To(gomega.HaveOccurred())
			gomega.Expect(nadr.Status.Conditions[0].Type).To(gomega.Equal(string(nacv1alpha1.ConditionNonAdminBackupNotAvailable)))
		})
		ginkgo.It("should error when referring a non-existent NonAdminRestore", func() {
			nadr.Spec.Target.Kind = velerov1.DownloadTargetKindRestoreLog
			nadr.Spec.Target.Name = "non-existent-restore"
			nadr.Status.Conditions = []metav1.Condition{} // clear conditions to avoid delete logic.
			err := fakeClient.Update(ctx, nadr)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			_, err = reconciler.Reconcile(ctx, nadr)
			gomega.Expect(err).To(gomega.HaveOccurred())
			gomega.Expect(nadr.Status.Conditions[0].Type).To(gomega.Equal(string(nacv1alpha1.ConditionNonAdminRestoreNotAvailable)))
		})
	})
	ginkgo.Context("Testing SetupWithManager", func() {
		ginkgo.It("Should set up the controller with the manager", func() {
			mgr, err := ctrl.NewManager(cfg, ctrl.Options{
				Scheme: scheme.Scheme,
			})
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			err = reconciler.SetupWithManager(mgr)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		})
	})
})
