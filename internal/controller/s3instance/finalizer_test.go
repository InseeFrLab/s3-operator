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

package s3instance_controller_test

import (
	"context"
	"testing"
	"time"

	s3v1alpha1 "github.com/InseeFrLab/s3-operator/api/v1alpha1"
	s3instance_controller "github.com/InseeFrLab/s3-operator/internal/controller/s3instance"
	TestUtils "github.com/InseeFrLab/s3-operator/test/utils"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestHandleDelete(t *testing.T) {
	// Set up a logger before running tests
	log.SetLogger(zap.New(zap.UseDevMode(true)))

	t.Run("no error", func(t *testing.T) {
		s3instanceResource := &s3v1alpha1.S3Instance{
			Spec: s3v1alpha1.S3InstanceSpec{
				AllowedNamespaces:     []string{"default", "test-*", "*-namespace", "*allowed*"},
				Url:                   "https://minio.example.com",
				S3Provider:            "minio",
				Region:                "us-east-1",
				BucketDeletionEnabled: true,
				S3UserDeletionEnabled: true,
				PathDeletionEnabled:   true,
				PolicyDeletionEnabled: true,
				SecretRef:             "minio-credentials",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:              "default",
				Namespace:         "s3-operator",
				Finalizers:        []string{"s3.onyxia.sh/finalizer"},
				DeletionTimestamp: &metav1.Time{Time: time.Now()},
			},
		}

		secretResource := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "minio-credentials",
				Namespace: "s3-operator",
			},
			StringData: map[string]string{
				"accessKey": "access_key_value",
				"secretKey": "secret_key_value",
			},
		}

		// Add mock for s3Factory and client
		testUtils := TestUtils.NewTestUtils()
		testUtils.SetupMockedS3FactoryAndClient()
		testUtils.SetupClient([]client.Object{s3instanceResource, secretResource})

		// Create the reconciler
		reconciler := &s3instance_controller.S3InstanceReconciler{
			Client:    testUtils.Client,
			Scheme:    testUtils.Client.Scheme(),
			S3factory: testUtils.S3Factory,
		}

		// Call Reconcile function
		req := ctrl.Request{NamespacedName: types.NamespacedName{Name: s3instanceResource.Name, Namespace: s3instanceResource.Namespace}}
		_, err := reconciler.Reconcile(context.TODO(), req)
		assert.NoError(t, err)
	})

	t.Run("error if one bucket ressource still use it", func(t *testing.T) {
		s3instanceResource := &s3v1alpha1.S3Instance{
			Spec: s3v1alpha1.S3InstanceSpec{
				AllowedNamespaces:     []string{"default", "test-*", "*-namespace", "*allowed*"},
				Url:                   "https://minio.example.com",
				S3Provider:            "minio",
				Region:                "us-east-1",
				BucketDeletionEnabled: true,
				S3UserDeletionEnabled: true,
				PathDeletionEnabled:   true,
				PolicyDeletionEnabled: true,
				SecretRef:             "minio-credentials",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:              "default",
				Namespace:         "s3-operator",
				Finalizers:        []string{"s3.onyxia.sh/finalizer"},
				DeletionTimestamp: &metav1.Time{Time: time.Now()},
			},
		}

		secretResource := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "minio-credentials",
				Namespace: "s3-operator",
			},
			StringData: map[string]string{
				"accessKey": "access_key_value",
				"secretKey": "secret_key_value",
			},
		}

		bucketResource := &s3v1alpha1.Bucket{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "bucket",
				Namespace: "default",
			},
			Spec: s3v1alpha1.BucketSpec{
				Name:          "bucket",
				S3InstanceRef: "s3-operator/default",
			},
		}

		// Add mock for s3Factory and client
		testUtils := TestUtils.NewTestUtils()
		testUtils.SetupMockedS3FactoryAndClient()
		testUtils.SetupClient([]client.Object{s3instanceResource, secretResource, bucketResource})

		// Create the reconciler
		reconciler := &s3instance_controller.S3InstanceReconciler{
			Client:    testUtils.Client,
			Scheme:    testUtils.Client.Scheme(),
			S3factory: testUtils.S3Factory,
		}

		// Call Reconcile function
		req := ctrl.Request{NamespacedName: types.NamespacedName{Name: s3instanceResource.Name, Namespace: s3instanceResource.Namespace}}
		_, err := reconciler.Reconcile(context.TODO(), req)
		assert.Error(t, err)
		assert.EqualErrorf(t, err, "Cannot delete s3Instance as 1 Buckets are used on this instance", err.Error())
	})

	t.Run("error if one policy ressource still use it", func(t *testing.T) {
		s3instanceResource := &s3v1alpha1.S3Instance{
			Spec: s3v1alpha1.S3InstanceSpec{
				AllowedNamespaces:     []string{"default", "test-*", "*-namespace", "*allowed*"},
				Url:                   "https://minio.example.com",
				S3Provider:            "minio",
				Region:                "us-east-1",
				BucketDeletionEnabled: true,
				S3UserDeletionEnabled: true,
				PathDeletionEnabled:   true,
				PolicyDeletionEnabled: true,
				SecretRef:             "minio-credentials",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:              "default",
				Namespace:         "s3-operator",
				Finalizers:        []string{"s3.onyxia.sh/finalizer"},
				DeletionTimestamp: &metav1.Time{Time: time.Now()},
			},
		}

		secretResource := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "minio-credentials",
				Namespace: "s3-operator",
			},
			StringData: map[string]string{
				"accessKey": "access_key_value",
				"secretKey": "secret_key_value",
			},
		}

		policyResource := &s3v1alpha1.Policy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "policy",
				Namespace: "default",
			},
			Spec: s3v1alpha1.PolicySpec{
				S3InstanceRef: "s3-operator/default",
			},
		}

		// Add mock for s3Factory and client
		testUtils := TestUtils.NewTestUtils()
		testUtils.SetupMockedS3FactoryAndClient()
		testUtils.SetupClient([]client.Object{s3instanceResource, secretResource, policyResource})

		// Create the reconciler
		reconciler := &s3instance_controller.S3InstanceReconciler{
			Client:    testUtils.Client,
			Scheme:    testUtils.Client.Scheme(),
			S3factory: testUtils.S3Factory,
		}

		// Call Reconcile function
		req := ctrl.Request{NamespacedName: types.NamespacedName{Name: s3instanceResource.Name, Namespace: s3instanceResource.Namespace}}
		_, err := reconciler.Reconcile(context.TODO(), req)
		assert.Error(t, err)
		assert.EqualErrorf(t, err, "Cannot delete s3Instance as 1 Policies are used on this instance", err.Error())
	})

	t.Run("error if one path ressource still use it", func(t *testing.T) {
		s3instanceResource := &s3v1alpha1.S3Instance{
			Spec: s3v1alpha1.S3InstanceSpec{
				AllowedNamespaces:     []string{"default", "test-*", "*-namespace", "*allowed*"},
				Url:                   "https://minio.example.com",
				S3Provider:            "minio",
				Region:                "us-east-1",
				BucketDeletionEnabled: true,
				S3UserDeletionEnabled: true,
				PathDeletionEnabled:   true,
				PolicyDeletionEnabled: true,
				SecretRef:             "minio-credentials",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:              "default",
				Namespace:         "s3-operator",
				Finalizers:        []string{"s3.onyxia.sh/finalizer"},
				DeletionTimestamp: &metav1.Time{Time: time.Now()},
			},
		}

		secretResource := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "minio-credentials",
				Namespace: "s3-operator",
			},
			StringData: map[string]string{
				"accessKey": "access_key_value",
				"secretKey": "secret_key_value",
			},
		}

		pathResource := &s3v1alpha1.Path{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "path",
				Namespace: "default",
			},
			Spec: s3v1alpha1.PathSpec{
				S3InstanceRef: "s3-operator/default",
			},
		}

		// Add mock for s3Factory and client
		testUtils := TestUtils.NewTestUtils()
		testUtils.SetupMockedS3FactoryAndClient()
		testUtils.SetupClient([]client.Object{s3instanceResource, secretResource, pathResource})

		// Create the reconciler
		reconciler := &s3instance_controller.S3InstanceReconciler{
			Client:    testUtils.Client,
			Scheme:    testUtils.Client.Scheme(),
			S3factory: testUtils.S3Factory,
		}

		// Call Reconcile function
		req := ctrl.Request{NamespacedName: types.NamespacedName{Name: s3instanceResource.Name, Namespace: s3instanceResource.Namespace}}
		_, err := reconciler.Reconcile(context.TODO(), req)
		assert.Error(t, err)
		assert.EqualErrorf(t, err, "Cannot delete s3Instance as 1 Paths are used on this instance", err.Error())
	})

	t.Run("error if one user ressource still use it", func(t *testing.T) {
		s3instanceResource := &s3v1alpha1.S3Instance{
			Spec: s3v1alpha1.S3InstanceSpec{
				AllowedNamespaces:     []string{"default", "test-*", "*-namespace", "*allowed*"},
				Url:                   "https://minio.example.com",
				S3Provider:            "minio",
				Region:                "us-east-1",
				BucketDeletionEnabled: true,
				S3UserDeletionEnabled: true,
				PathDeletionEnabled:   true,
				PolicyDeletionEnabled: true,
				SecretRef:             "minio-credentials",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:              "default",
				Namespace:         "s3-operator",
				Finalizers:        []string{"s3.onyxia.sh/finalizer"},
				DeletionTimestamp: &metav1.Time{Time: time.Now()},
			},
		}

		secretResource := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "minio-credentials",
				Namespace: "s3-operator",
			},
			StringData: map[string]string{
				"accessKey": "access_key_value",
				"secretKey": "secret_key_value",
			},
		}

		userResource := &s3v1alpha1.S3User{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "user",
				Namespace: "default",
			},
			Spec: s3v1alpha1.S3UserSpec{
				S3InstanceRef: "s3-operator/default",
			},
		}

		// Add mock for s3Factory and client
		testUtils := TestUtils.NewTestUtils()
		testUtils.SetupMockedS3FactoryAndClient()
		testUtils.SetupClient([]client.Object{s3instanceResource, secretResource, userResource})

		// Create the reconciler
		reconciler := &s3instance_controller.S3InstanceReconciler{
			Client:    testUtils.Client,
			Scheme:    testUtils.Client.Scheme(),
			S3factory: testUtils.S3Factory,
		}

		// Call Reconcile function
		req := ctrl.Request{NamespacedName: types.NamespacedName{Name: s3instanceResource.Name, Namespace: s3instanceResource.Namespace}}
		_, err := reconciler.Reconcile(context.TODO(), req)
		assert.Error(t, err)
		assert.EqualErrorf(t, err, "Cannot delete s3Instance as 1 S3Users are used on this instance", err.Error())
	})
}
