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

// Entrypoint of the project
package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"os"

	// TODO when to update oadp-operator version in go.mod?
	"github.com/openshift/oadp-operator/api/v1alpha1"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	nacv1alpha1 "github.com/migtools/oadp-non-admin/api/v1alpha1"
	"github.com/migtools/oadp-non-admin/internal/common/constant"
	"github.com/migtools/oadp-non-admin/internal/controller"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(nacv1alpha1.AddToScheme(scheme))

	utilruntime.Must(velerov1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

// +kubebuilder:rbac:groups=oadp.openshift.io,resources=dataprotectionapplications,verbs=list

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var secureMetrics bool
	var enableHTTP2 bool
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&secureMetrics, "metrics-secure", false,
		"If set the metrics endpoint is served securely")
	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// if the enable-http2 flag is false (the default), http/2 should be disabled
	// due to its vulnerabilities. More specifically, disabling http/2 will
	// prevent from being vulnerable to the HTTP/2 Stream Cancelation and
	// Rapid Reset CVEs. For more information see:
	// - https://github.com/advisories/GHSA-qppj-fm5r-hxr3
	// - https://github.com/advisories/GHSA-4374-p667-p6c8
	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("disabling http/2")
		c.NextProtos = []string{"http/1.1"}
	}

	tlsOpts := []func(*tls.Config){}
	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	webhookServer := webhook.NewServer(webhook.Options{
		TLSOpts: tlsOpts,
	})

	oadpNamespace := os.Getenv(constant.NamespaceEnvVar)
	if len(oadpNamespace) == 0 {
		setupLog.Error(fmt.Errorf("%v environment variable is empty", constant.NamespaceEnvVar), "environment variable must be set")
		os.Exit(1)
	}

	restConfig := ctrl.GetConfigOrDie()

	dpaConfiguration, err := getDPAConfiguration(restConfig, oadpNamespace)
	if err != nil {
		setupLog.Error(err, "unable to get enforced spec")
		os.Exit(1)
	}

	mgr, err := ctrl.NewManager(restConfig, ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress:   metricsAddr,
			SecureServing: secureMetrics,
			TLSOpts:       tlsOpts,
		},
		WebhookServer:          webhookServer,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "393da43e.openshift.io",
		// LeaderElectionReleaseOnCancel defines if the leader should step down voluntarily
		// when the Manager ends. This requires the binary to immediately end when the
		// Manager is stopped, otherwise, this setting is unsafe. Setting this significantly
		// speeds up voluntary leader transitions as the new leader don't have to wait
		// LeaseDuration time first.
		//
		// In the default scaffold provided, the program ends immediately after
		// the manager stops, so would be fine to enable this option. However,
		// if you are doing or is intended to do any operation such as perform cleanups
		// after the manager stops then its usage might be unsafe.
		// LeaderElectionReleaseOnCancel: true,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = (&controller.NonAdminBackupReconciler{
		Client:             mgr.GetClient(),
		Scheme:             mgr.GetScheme(),
		OADPNamespace:      oadpNamespace,
		EnforcedBackupSpec: dpaConfiguration.EnforceBackupSpec,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to setup NonAdminBackup controller with manager")
		os.Exit(1)
	}
	if err = (&controller.NonAdminRestoreReconciler{
		Client:              mgr.GetClient(),
		Scheme:              mgr.GetScheme(),
		OADPNamespace:       oadpNamespace,
		EnforcedRestoreSpec: dpaConfiguration.EnforceRestoreSpec,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to setup NonAdminRestore controller with manager")
		os.Exit(1)
	}
	if err = (&controller.NonAdminBackupStorageLocationReconciler{
		Client:        mgr.GetClient(),
		Scheme:        mgr.GetScheme(),
		OADPNamespace: oadpNamespace,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to setup NonAdminBackupStorageLocation controller with manager")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder
	if dpaConfiguration.BackupSyncPeriod.Duration > 0 {
		if err = (&controller.NonAdminBackupSynchronizerReconciler{
			Client:        mgr.GetClient(),
			Scheme:        mgr.GetScheme(),
			OADPNamespace: oadpNamespace,
			SyncPeriod:    dpaConfiguration.BackupSyncPeriod.Duration,
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to setup NonAdminBackupSynchronizer controller with manager")
			os.Exit(1)
		}
	}
	if dpaConfiguration.GarbageCollectionPeriod.Duration > 0 {
		if err = (&controller.GarbageCollectorReconciler{
			Client:        mgr.GetClient(),
			Scheme:        mgr.GetScheme(),
			OADPNamespace: oadpNamespace,
			Frequency:     dpaConfiguration.GarbageCollectionPeriod.Duration,
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to setup GarbageCollector controller with manager")
			os.Exit(1)
		}
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func getDPAConfiguration(restConfig *rest.Config, oadpNamespace string) (v1alpha1.NonAdmin, error) {
	dpaConfiguration := v1alpha1.NonAdmin{
		GarbageCollectionPeriod: &metav1.Duration{
			Duration: v1alpha1.DefaultGarbageCollectionPeriod,
		},
		BackupSyncPeriod: &metav1.Duration{
			Duration: v1alpha1.DefaultBackupSyncPeriod,
		},
		EnforceBackupSpec:  &velerov1.BackupSpec{},
		EnforceRestoreSpec: &velerov1.RestoreSpec{},
	}

	dpaClientScheme := runtime.NewScheme()
	utilruntime.Must(v1alpha1.AddToScheme(dpaClientScheme))
	dpaClient, err := client.New(restConfig, client.Options{
		Scheme: dpaClientScheme,
	})
	if err != nil {
		return dpaConfiguration, err
	}
	// TODO we could pass DPA name as env var and do a get call directly. Better?
	dpaList := &v1alpha1.DataProtectionApplicationList{}
	err = dpaClient.List(context.Background(), dpaList, &client.ListOptions{Namespace: oadpNamespace})
	if err != nil {
		return dpaConfiguration, err
	}
	for _, dpa := range dpaList.Items {
		if nonAdmin := dpa.Spec.NonAdmin; nonAdmin != nil {
			if nonAdmin.EnforceBackupSpec != nil {
				dpaConfiguration.EnforceBackupSpec = nonAdmin.EnforceBackupSpec
			}
			if nonAdmin.EnforceRestoreSpec != nil {
				dpaConfiguration.EnforceRestoreSpec = nonAdmin.EnforceRestoreSpec
			}
			if nonAdmin.GarbageCollectionPeriod != nil {
				dpaConfiguration.GarbageCollectionPeriod.Duration = nonAdmin.GarbageCollectionPeriod.Duration
			}
			if nonAdmin.BackupSyncPeriod != nil {
				dpaConfiguration.BackupSyncPeriod.Duration = nonAdmin.BackupSyncPeriod.Duration
			}
			break
		}
	}

	return dpaConfiguration, nil
}
