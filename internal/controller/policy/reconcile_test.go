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

package policy_controller_test

import (
	"context"
	"testing"

	TestUtils "github.com/InseeFrLab/s3-operator/test/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	s3v1alpha1 "github.com/InseeFrLab/s3-operator/api/v1alpha1"
	policy_controller "github.com/InseeFrLab/s3-operator/internal/controller/policy"
	"github.com/stretchr/testify/assert"
)

func TestHandleCreate(t *testing.T) {
	// Set up a logger before running tests
	log.SetLogger(zap.New(zap.UseDevMode(true)))

	// Create a fake client with a sample CR
	policyResource := &s3v1alpha1.Policy{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "example-policy",
			Namespace:  "default",
			Generation: 1,
		},
		Spec: s3v1alpha1.PolicySpec{
			Name:          "example-policy",
			S3InstanceRef: "s3-operator/default",
			PolicyContent: "",
		},
	}

	// Add mock for s3Factory and client
	testUtils := TestUtils.NewTestUtils()
	testUtils.SetupMockedS3FactoryAndClient()
	s3instanceResource, secretResource := testUtils.GenerateBasicS3InstanceAndSecret()
	testUtils.SetupClient([]client.Object{s3instanceResource, secretResource, policyResource})

	// Create the reconciler
	reconciler := &policy_controller.PolicyReconciler{
		Client:    testUtils.Client,
		Scheme:    testUtils.Client.Scheme(),
		S3factory: testUtils.S3Factory,
	}

	t.Run("no error", func(t *testing.T) {
		// Call Reconcile function
		req := ctrl.Request{NamespacedName: types.NamespacedName{Name: policyResource.Name, Namespace: policyResource.Namespace}}
		_, err := reconciler.Reconcile(context.TODO(), req)
		assert.NoError(t, err)
	})
}

func TestHandleUpdate(t *testing.T) {
	// Set up a logger before running tests
	log.SetLogger(zap.New(zap.UseDevMode(true)))

	existingValidPolicy := []byte(`{
"Version": "2012-10-17",
"Statement": [
{
	"Effect": "Allow",
	"Principal": {
	"AWS": "*"
	},
	"Action": [
	"s3:GetObject",
	"s3:PutObject",
	"s3:DeleteObject"
	],
	"Resource": "arn:aws:s3:::my-bucket/*"
}
]
}`)

	existingInvalidPolicy := []byte(`{
	"Version": "2012-10-17",
	"Statement": [
	{
		"Effect": "Allow",
		"Principal": {
		"AWS": "*"
		},
		"Action": [
		"s3:GetObject",
		"s3:PutObject",
		"s3:DeleteObject"
		],
		"Resource": "arn:aws:s3:::my-bucket2/*"
	}
	]
	}`)

	// Create a fake client with a sample CR
	policyResource := &s3v1alpha1.Policy{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "existing-policy",
			Namespace:  "default",
			Generation: 1,
		},
		Spec: s3v1alpha1.PolicySpec{
			Name:          "existing-policy",
			S3InstanceRef: "s3-operator/default",
			PolicyContent: string(existingValidPolicy),
		},
	}

	// Create a fake client with a sample CR
	policyInvalidResource := &s3v1alpha1.Policy{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "existing-invalid-policy",
			Namespace:  "default",
			Generation: 1,
		},
		Spec: s3v1alpha1.PolicySpec{
			Name:          "existing-policy",
			S3InstanceRef: "s3-operator/default",
			PolicyContent: string(existingInvalidPolicy),
		},
	}

	// Add mock for s3Factory and client
	testUtils := TestUtils.NewTestUtils()
	testUtils.SetupMockedS3FactoryAndClient()
	s3instanceResource, secretResource := testUtils.GenerateBasicS3InstanceAndSecret()
	testUtils.SetupClient([]client.Object{s3instanceResource, secretResource, policyResource, policyInvalidResource})

	// Create the reconciler
	reconciler := &policy_controller.PolicyReconciler{
		Client:    testUtils.Client,
		Scheme:    testUtils.Client.Scheme(),
		S3factory: testUtils.S3Factory,
	}

	t.Run("no error", func(t *testing.T) {
		// Call Reconcile function
		req := ctrl.Request{NamespacedName: types.NamespacedName{Name: policyResource.Name, Namespace: policyResource.Namespace}}
		_, err := reconciler.Reconcile(context.TODO(), req)
		assert.NoError(t, err)
	})

	t.Run("no error", func(t *testing.T) {
		// Call Reconcile function
		req := ctrl.Request{NamespacedName: types.NamespacedName{Name: policyInvalidResource.Name, Namespace: policyInvalidResource.Namespace}}
		_, err := reconciler.Reconcile(context.TODO(), req)
		assert.NoError(t, err)
	})
}
