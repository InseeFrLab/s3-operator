/*
Copyright 2024 Mathieu Parent <math.parent@gmail.com>.

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

package bucket_controller_test

import (
	"context"
	"testing"

	TestUtils "github.com/InseeFrLab/s3-operator/test/utils"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	s3v1alpha1 "github.com/InseeFrLab/s3-operator/api/v1alpha1"
	bucket_controller "github.com/InseeFrLab/s3-operator/internal/controller/bucket"
	"github.com/stretchr/testify/assert"
)

func TestHandleCreate(t *testing.T) {
	// Set up a logger before running tests
	log.SetLogger(zap.New(zap.UseDevMode(true)))

	// Create a fake client with a sample CR
	bucketResource := &s3v1alpha1.Bucket{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-bucket",
			Namespace:  "default",
			Generation: 1,
		},
		Spec: s3v1alpha1.BucketSpec{
			Name:          "test-bucket",
			S3InstanceRef: "s3-operator/default",
			Quota:         s3v1alpha1.Quota{Default: *resource.NewQuantity(int64(10), resource.BinarySI)},
		},
		TypeMeta: metav1.TypeMeta{
			APIVersion: "s3.onyxia.sh/v1alpha1",
			Kind:       "Bucket",
		},
	}

	// Add mock for s3Factory and client
	testUtils := TestUtils.NewTestUtils()
	testUtils.SetupMockedS3FactoryAndClient()
	s3instanceResource, secretResource := testUtils.GenerateBasicS3InstanceAndSecret()
	testUtils.SetupClient([]client.Object{s3instanceResource, secretResource, bucketResource})

	// Create the reconciler
	reconciler := &bucket_controller.BucketReconciler{
		Client:    testUtils.Client,
		Scheme:    testUtils.Client.Scheme(),
		S3factory: testUtils.S3Factory,
	}

	t.Run("no error", func(t *testing.T) {
		// Call Reconcile function
		req := ctrl.Request{NamespacedName: types.NamespacedName{Name: bucketResource.Name, Namespace: bucketResource.Namespace}}
		_, err := reconciler.Reconcile(context.TODO(), req)
		assert.NoError(t, err)
	})
}

func TestHandleUpdate(t *testing.T) {
	// Set up a logger before running tests
	log.SetLogger(zap.New(zap.UseDevMode(true)))

	// Create a fake client with a sample CR
	bucketResource := &s3v1alpha1.Bucket{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "existing-bucket",
			Namespace:  "default",
			Generation: 1,
			Finalizers: []string{"s3.onyxia.sh/finalizer"},
		},
		Spec: s3v1alpha1.BucketSpec{
			Name:          "existing-bucket",
			Paths:         []string{"example"},
			S3InstanceRef: "s3-operator/default",
			Quota:         s3v1alpha1.Quota{Default: *resource.NewQuantity(int64(10), resource.BinarySI)}},
	}

	// Create a fake client with a sample CR
	bucketInvalidResource := &s3v1alpha1.Bucket{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "existing-invalid-bucket",
			Namespace:  "default",
			Generation: 1,
			Finalizers: []string{"s3.onyxia.sh/finalizer"},
		},
		Spec: s3v1alpha1.BucketSpec{
			Name:          "existing-invalid-bucket",
			Paths:         []string{"example", "non-existing"},
			S3InstanceRef: "s3-operator/default",
			Quota:         s3v1alpha1.Quota{Default: *resource.NewQuantity(int64(100), resource.BinarySI)}},
	}

	// Add mock for s3Factory and client
	testUtils := TestUtils.NewTestUtils()
	testUtils.SetupMockedS3FactoryAndClient()
	s3instanceResource, secretResource := testUtils.GenerateBasicS3InstanceAndSecret()
	testUtils.SetupClient([]client.Object{s3instanceResource, secretResource, bucketResource, bucketInvalidResource})

	// Create the reconciler
	reconciler := &bucket_controller.BucketReconciler{
		Client:    testUtils.Client,
		Scheme:    testUtils.Client.Scheme(),
		S3factory: testUtils.S3Factory,
	}

	t.Run("no error", func(t *testing.T) {
		// Call Reconcile function
		req := ctrl.Request{NamespacedName: types.NamespacedName{Name: bucketResource.Name, Namespace: bucketResource.Namespace}}
		_, err := reconciler.Reconcile(context.TODO(), req)
		assert.NoError(t, err)
	})

	t.Run("no error", func(t *testing.T) {
		// Call Reconcile function
		req := ctrl.Request{NamespacedName: types.NamespacedName{Name: bucketInvalidResource.Name, Namespace: bucketInvalidResource.Namespace}}
		_, err := reconciler.Reconcile(context.TODO(), req)
		assert.NoError(t, err)
	})
}
