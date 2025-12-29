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

	corev1 "k8s.io/api/core/v1"

	TestUtils "github.com/InseeFrLab/s3-operator/test/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	s3v1alpha1 "github.com/InseeFrLab/s3-operator/api/v1alpha1"
	s3instance_controller "github.com/InseeFrLab/s3-operator/internal/controller/s3instance"
	"github.com/stretchr/testify/assert"
)

func TestHandleCreate(t *testing.T) {
	// Set up a logger before running tests
	log.SetLogger(zap.New(zap.UseDevMode(true)))

	s3instanceResourceInvalid := &s3v1alpha1.S3Instance{
		Spec: s3v1alpha1.S3InstanceSpec{
			AllowedNamespaces:     []string{"default", "test-*", "*-namespace", "*allowed*"},
			Url:                   "https://minio.invalid.example.com",
			S3Provider:            "minio",
			Region:                "us-east-1",
			BucketDeletionEnabled: true,
			S3UserDeletionEnabled: true,
			PathDeletionEnabled:   true,
			PolicyDeletionEnabled: true,
			SecretRef:             "minio-credentials",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "invalid-instance",
			Namespace: "s3-operator",
		},
	}

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
			Name:      "default",
			Namespace: "s3-operator",
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
	testUtils.SetupClient([]client.Object{s3instanceResource, secretResource, s3instanceResourceInvalid})

	// Create the reconciler
	reconciler := &s3instance_controller.S3InstanceReconciler{
		Client:    testUtils.Client,
		Scheme:    testUtils.Client.Scheme(),
		S3factory: testUtils.S3Factory,
	}

	t.Run("no error", func(t *testing.T) {
		// Call Reconcile function
		req := ctrl.Request{NamespacedName: types.NamespacedName{Name: s3instanceResource.Name, Namespace: s3instanceResource.Namespace}}
		_, err := reconciler.Reconcile(context.TODO(), req)
		assert.NoError(t, err)
	})

	t.Run("finalizer is added", func(t *testing.T) {
		// Call Reconcile function
		req := ctrl.Request{NamespacedName: types.NamespacedName{Name: s3instanceResource.Name, Namespace: s3instanceResource.Namespace}}
		_, err := reconciler.Reconcile(context.TODO(), req)
		assert.NoError(t, err)

		// FetchReconciledInstance
		reconciledInstance := &s3v1alpha1.S3Instance{}
		_ = testUtils.Client.Get(context.TODO(), client.ObjectKey{
			Namespace: "s3-operator",
			Name:      "default",
		}, reconciledInstance)

		assert.Equal(t, "s3.onyxia.sh/finalizer", reconciledInstance.Finalizers[0])
	})

	t.Run("status is reconciled", func(t *testing.T) {
		// Call Reconcile function
		req := ctrl.Request{NamespacedName: types.NamespacedName{Name: s3instanceResource.Name, Namespace: s3instanceResource.Namespace}}
		_, err := reconciler.Reconcile(context.TODO(), req)
		assert.NoError(t, err)

		// FetchReconciledInstance
		reconciledInstance := &s3v1alpha1.S3Instance{}
		_ = testUtils.Client.Get(context.TODO(), client.ObjectKey{
			Namespace: "s3-operator",
			Name:      "default",
		}, reconciledInstance)

		assert.Equal(t, "Reconciled", reconciledInstance.Status.Conditions[0].Reason)
	})

	t.Run("reason is creation failure because of invalid client", func(t *testing.T) {
		// Call Reconcile function
		req := ctrl.Request{NamespacedName: types.NamespacedName{Name: s3instanceResourceInvalid.Name, Namespace: s3instanceResourceInvalid.Namespace}}
		_, err := reconciler.Reconcile(context.TODO(), req)
		assert.NotNil(t, err)

		// 4️⃣ FetchReconciledInstance
		reconciledInstance := &s3v1alpha1.S3Instance{}
		_ = testUtils.Client.Get(context.TODO(), client.ObjectKey{
			Namespace: "s3-operator",
			Name:      "invalid-instance",
		}, reconciledInstance)

		assert.Equal(t, "CreationFailure", reconciledInstance.Status.Conditions[0].Reason)
	})
}
