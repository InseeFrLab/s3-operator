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

	s3v1alpha1 "github.com/InseeFrLab/s3-operator/api/v1alpha1"
	controllers "github.com/InseeFrLab/s3-operator/controllers"
	s3ClientCache "github.com/InseeFrLab/s3-operator/internal/s3"
	"github.com/InseeFrLab/s3-operator/internal/s3/factory"

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
	var bucketDeletion bool
	var policyDeletion bool
	var pathDeletion bool
	var s3userDeletion bool
	var s3LabelSelector string

	//K8S related variable
	var overrideExistingSecret bool

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
	flag.StringVar(&s3LabelSelector, "s3-label-selector", "", "label selector to filter object managed by this operator if empty all objects are managed")
	flag.Var(&caCertificatesBase64, "s3-ca-certificate-base64", "(Optional) Base64 encoded, PEM format certificate file for a certificate authority, for https requests to S3")
	flag.StringVar(&caCertificatesBundlePath, "s3-ca-certificate-bundle-path", "", "(Optional) Path to a CA certificate file, for https requests to S3")
	flag.StringVar(&region, "region", "us-east-1", "The region to configure for the S3 client")
	flag.BoolVar(&useSsl, "useSsl", true, "Use of SSL/TLS to connect to the S3 endpoint")
	flag.BoolVar(&bucketDeletion, "bucket-deletion", false, "Trigger bucket deletion on the S3 backend upon CR deletion. Will fail if bucket is not empty.")
	flag.BoolVar(&policyDeletion, "policy-deletion", false, "Trigger policy deletion on the S3 backend upon CR deletion")
	flag.BoolVar(&pathDeletion, "path-deletion", false, "Trigger path deletion on the S3 backend upon CR deletion. Limited to deleting the `.keep` files used by the operator.")
	flag.BoolVar(&s3userDeletion, "s3user-deletion", false, "Trigger S3 deletion on the S3 backend upon CR deletion")
	flag.BoolVar(&overrideExistingSecret, "override-existing-secret", false, "Override existing secret associated to user in case of the secret already exist")

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

	s3ClientCache := s3ClientCache.New()

	// Creation of the default S3 client
	s3DefaultClient, err := factory.GenerateDefaultS3Client(s3Provider, s3EndpointUrl, accessKey, secretKey, region, useSsl, caCertificatesBase64, caCertificatesBundlePath)

	if err != nil {
		// setupLog.Log.Error(err, err.Error())
		// fmt.Print(s3Client)
		// fmt.Print(err)
		setupLog.Error(err, "an error occurred while creating the S3 client", "s3Client", s3DefaultClient)
		os.Exit(1)
	}

	if s3DefaultClient != nil {
		s3ClientCache.Set("default", s3DefaultClient)
	}

	if err = (&controllers.BucketReconciler{
		Client:               mgr.GetClient(),
		Scheme:               mgr.GetScheme(),
		S3ClientCache:        s3ClientCache,
		BucketDeletion:       bucketDeletion,
		S3LabelSelectorValue: s3LabelSelector,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Bucket")
		os.Exit(1)
	}
	// if err = (&controllers.PathReconciler{
	// 	Client:               mgr.GetClient(),
	// 	Scheme:               mgr.GetScheme(),
	// 	S3ClientCache:        s3ClientCache,
	// 	PathDeletion:         pathDeletion,
	// 	S3LabelSelectorValue: s3LabelSelector,
	// }).SetupWithManager(mgr); err != nil {
	// 	setupLog.Error(err, "unable to create controller", "controller", "Path")
	// 	os.Exit(1)
	// }
	// if err = (&controllers.PolicyReconciler{
	// 	Client:               mgr.GetClient(),
	// 	Scheme:               mgr.GetScheme(),
	// 	S3ClientCache:        s3ClientCache,
	// 	PolicyDeletion:       policyDeletion,
	// 	S3LabelSelectorValue: s3LabelSelector,
	// }).SetupWithManager(mgr); err != nil {
	// 	setupLog.Error(err, "unable to create controller", "controller", "Policy")
	// 	os.Exit(1)
	// }
	// if err = (&controllers.S3UserReconciler{
	// 	Client:                 mgr.GetClient(),
	// 	Scheme:                 mgr.GetScheme(),
	// 	S3ClientCache:          s3ClientCache,
	// 	S3UserDeletion:         s3userDeletion,
	// 	OverrideExistingSecret: overrideExistingSecret,
	// 	S3LabelSelectorValue:   s3LabelSelector,
	// }).SetupWithManager(mgr); err != nil {
	// 	setupLog.Error(err, "unable to create controller", "controller", "S3User")
	// 	os.Exit(1)
	// }
	if err = (&controllers.S3InstanceReconciler{
		Client:               mgr.GetClient(),
		Scheme:               mgr.GetScheme(),
		S3ClientCache:        s3ClientCache,
		S3LabelSelectorValue: s3LabelSelector,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "S3Instance")
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
