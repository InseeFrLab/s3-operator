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

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	s3v1alpha1 "github.com/inseefrlab/s3-operator/api/v1alpha1"
	controllers "github.com/inseefrlab/s3-operator/internal/controller"
	"github.com/inseefrlab/s3-operator/internal/controller/s3/factory"
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
	var s3EndpointUrl string
	var accessKey string
	var secretKey string
	var region string
	var s3Provider string
	var useSsl bool
	var caCertificatesBase64 ArrayFlags
	var caCertificatesBundlePath string

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")

	// S3 related flags
	flag.StringVar(&s3Provider, "s3-provider", "minio", "S3 provider (possible values : minio, mockedS3Provider)")
	flag.StringVar(&s3EndpointUrl, "s3-endpoint-url", "localhost:9000", "Hostname (or hostname:port) of the S3 server")
	flag.StringVar(&accessKey, "s3-access-key", "ROOTNAME", "The accessKey of the acount")
	flag.StringVar(&secretKey, "s3-secret-key", "CHANGEME123", "The secretKey of the acount")
	flag.Var(&caCertificatesBase64, "s3-ca-certificate-base64", "(Optional) Base64 encoded, PEM format certificate file for a certificate authority, for https requests to S3")
	flag.StringVar(&caCertificatesBundlePath, "s3-ca-certificate-bundle-path", "", "(Optional) Path to a CA certificate file, for https requests to S3")
	flag.StringVar(&region, "region", "us-east-1", "The region to configure for the S3 client")
	flag.BoolVar(&useSsl, "useSsl", true, "Use of SSL/TLS to connect to the S3 endpoint")

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
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:  scheme,
		Metrics: serverOption,
		// commented because option format change in ctrl.Options
		//Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "1402b7b1.onyxia.sh",
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

	// For S3 access key and secret key, we first try to read the values from environment variables.
	// Only if these are not defined do we use the respective flags.
	var accessKeyFromEnvIfAvailable = os.Getenv("S3_ACCESS_KEY")
	if accessKeyFromEnvIfAvailable == "" {
		accessKeyFromEnvIfAvailable = accessKey
	}
	var secretKeyFromEnvIfAvailable = os.Getenv("S3_SECRET_KEY")
	if secretKeyFromEnvIfAvailable == "" {
		secretKeyFromEnvIfAvailable = secretKey
	}

	// Creation of the S3 client
	s3Config := &factory.S3Config{S3Provider: s3Provider, S3UrlEndpoint: s3EndpointUrl, Region: region, AccessKey: accessKeyFromEnvIfAvailable, SecretKey: secretKeyFromEnvIfAvailable, UseSsl: useSsl, CaCertificatesBase64: caCertificatesBase64, CaBundlePath: caCertificatesBundlePath}
	s3Client, err := factory.GetS3Client(s3Config.S3Provider, s3Config)
	if err != nil {
		// setupLog.Log.Error(err, err.Error())
		// fmt.Print(s3Client)
		// fmt.Print(err)
		setupLog.Error(err, "an error occurred while creating the S3 client", "s3Client", s3Client)
		os.Exit(1)
	}

	if err = (&controllers.BucketReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		S3Client: s3Client,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Bucket")
		os.Exit(1)
	}
	if err = (&controllers.PolicyReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		S3Client: s3Client,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Policy")
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
