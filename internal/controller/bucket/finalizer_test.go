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

package bucket_controller_test

import (
	"context"
	"testing"
	"time"

	s3v1alpha1 "github.com/InseeFrLab/s3-operator/api/v1alpha1"
	bucket_controller "github.com/InseeFrLab/s3-operator/internal/controller/bucket"
	TestUtils "github.com/InseeFrLab/s3-operator/test/utils"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/resource"
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

	// Create a fake client with a sample CR
	bucketResource := &s3v1alpha1.Bucket{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "example-bucket",
			Namespace:         "default",
			Generation:        1,
			DeletionTimestamp: &metav1.Time{Time: time.Now()},
			Finalizers:        []string{"s3.onyxia.sh/finalizer"},
		},
		Spec: s3v1alpha1.BucketSpec{
			Name:          "example-bucket",
			S3InstanceRef: "s3-operator/default",
			Quota:         s3v1alpha1.Quota{Default: *resource.NewQuantity(int64(10), resource.BinarySI)},
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

	t.Run("ressource have been deleted", func(t *testing.T) {
		// Call Reconcile function
		req := ctrl.Request{NamespacedName: types.NamespacedName{Name: bucketResource.Name, Namespace: bucketResource.Namespace}}
		_, err := reconciler.Reconcile(context.TODO(), req)
		assert.NoError(t, err)
		policy := &s3v1alpha1.Bucket{}
		err = testUtils.Client.Get(context.TODO(), client.ObjectKey{
			Namespace: "default",
			Name:      "example-bucket",
		}, policy)
		assert.NotNil(t, err)
		assert.ErrorContains(t, err, "buckets.s3.onyxia.sh \"example-bucket\" not found")
	})
}
