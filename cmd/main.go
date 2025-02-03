/*
Copyright 2023.

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

package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	s3v1alpha1 "github.com/InseeFrLab/s3-operator/api/v1alpha1"
	bucketControllers "github.com/InseeFrLab/s3-operator/internal/controller/bucket"
	pathControllers "github.com/InseeFrLab/s3-operator/internal/controller/path"
	policyControllers "github.com/InseeFrLab/s3-operator/internal/controller/policy"
	s3InstanceControllers "github.com/InseeFrLab/s3-operator/internal/controller/s3instance"
	userControllers "github.com/InseeFrLab/s3-operator/internal/controller/user"
	"github.com/InseeFrLab/s3-operator/internal/helpers"
	s3factory "github.com/InseeFrLab/s3-operator/internal/s3/factory/impl"

	"go.uber.org/zap/zapcore"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

// Implementing multi-value flag for custom CAs
// See also : https://stackoverflow.com/a/28323276
type ArrayFlags []string

func (flags *ArrayFlags) String() string {
	return fmt.Sprint(*flags)
}

func (flags *ArrayFlags) Set(value string) error {
	*flags = append(*flags, value)
	return nil
}

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(s3v1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string

	// S3 related variables
	var reconcilePeriod time.Duration

	//K8S related variable
	var overrideExistingSecret bool

	flag.StringVar(
		&metricsAddr,
		"metrics-bind-address",
		":8080",
		"The address the metric endpoint binds to.",
	)
	flag.StringVar(
		&probeAddr,
		"health-probe-bind-address",
		":8081",
		"The address the probe endpoint binds to.",
	)
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.DurationVar(&reconcilePeriod, "reconcile-period", 0,
		"Default reconcile period for controllers. Zero to disable periodic reconciliation")

	// S3 related flags
	flag.BoolVar(
		&overrideExistingSecret,
		"override-existing-secret",
		false,
		"Override existing secret associated to user in case of the secret already exist",
	)

	opts := zap.Options{
		Development: true,
		TimeEncoder: zapcore.ISO8601TimeEncoder,
	}

	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// change due to upgrade version to 0.16.2 of sigs.k8s.io/controller-runtime
	var serverOption = server.Options{
		BindAddress: metricsAddr,
	}

	s3Factory := s3factory.NewS3Factory()
	s3InstanceHelper := helpers.NewS3InstanceHelper()
	controllerHelper := helpers.NewControllerHelper()

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:  scheme,
		Metrics: serverOption,
		// commented because option format change in ctrl.Options
		//Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "1402b7b1.onyxia.sh",
		// To change the sync period, it's possible to use cache.Options()
		// Caveat : the actual period is slightly longer than what configured,
		// possibly because of cache expiration not accounted for ?
		// Cache:                  cache.Options{SyncPeriod: 2 * time.Minute},
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

	if err = (&s3InstanceControllers.S3InstanceReconciler{
		Client:           mgr.GetClient(),
		Scheme:           mgr.GetScheme(),
		ReconcilePeriod:  reconcilePeriod,
		S3factory:        s3Factory,
		ControllerHelper: controllerHelper,
		S3Instancehelper: s3InstanceHelper,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "S3Instance")
		os.Exit(1)
	}
	if err = (&bucketControllers.BucketReconciler{
		Client:           mgr.GetClient(),
		Scheme:           mgr.GetScheme(),
		ReconcilePeriod:  reconcilePeriod,
		S3factory:        s3Factory,
		ControllerHelper: controllerHelper,
		S3Instancehelper: s3InstanceHelper,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Bucket")
		os.Exit(1)
	}
	if err = (&pathControllers.PathReconciler{
		Client:           mgr.GetClient(),
		Scheme:           mgr.GetScheme(),
		ReconcilePeriod:  reconcilePeriod,
		S3factory:        s3Factory,
		ControllerHelper: controllerHelper,
		S3Instancehelper: s3InstanceHelper,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Path")
		os.Exit(1)
	}
	if err = (&policyControllers.PolicyReconciler{
		Client:           mgr.GetClient(),
		Scheme:           mgr.GetScheme(),
		ReconcilePeriod:  reconcilePeriod,
		S3factory:        s3Factory,
		ControllerHelper: controllerHelper,
		S3Instancehelper: s3InstanceHelper,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Policy")
		os.Exit(1)
	}
	if err = (&userControllers.S3UserReconciler{
		Client:                  mgr.GetClient(),
		Scheme:                  mgr.GetScheme(),
		OverrideExistingSecret:  overrideExistingSecret,
		ReconcilePeriod:         reconcilePeriod,
		S3factory:               s3Factory,
		ControllerHelper:        controllerHelper,
		S3Instancehelper:        s3InstanceHelper,
		PasswordGeneratorHelper: helpers.NewPasswordGenerator(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "S3User")
		os.Exit(1)
	}

	//+kubebuilder:scaffold:builder

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
