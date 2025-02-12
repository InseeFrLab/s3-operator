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


package user_controller_test

import (
	"context"
	"testing"

	s3v1alpha1 "github.com/InseeFrLab/s3-operator/api/v1alpha1"
	user_controller "github.com/InseeFrLab/s3-operator/internal/controller/user"
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

	// Create a fake client with a sample CR
	s3UserResource := &s3v1alpha1.S3User{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "s3.onyxia.sh/v1alpha1",
			Kind:       "S3User",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:       "existing-valid-user",
			Namespace:  "default",
			Generation: 1,
			Finalizers: []string{"s3.onyxia.sh/userFinalizer"},
			UID:        "6c8dceca-f7df-469d-80a5-1afed9e4d710",
		},
		Spec: s3v1alpha1.S3UserSpec{
			AccessKey:                "existing-valid-user",
			Policies:                 []string{"admin"},
			SecretName:               "existing-valid-user-credentials",
			S3InstanceRef:            "s3-operator/default",
			SecretFieldNameAccessKey: "accessKey",
			SecretFieldNameSecretKey: "secretKey",
		},
	}

	blockOwnerDeletion := true
	controller := true
	s3UserSecretResource := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "existing-valid-user-credentials",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         s3UserResource.APIVersion,
					Kind:               s3UserResource.Kind,
					Name:               s3UserResource.Name,
					BlockOwnerDeletion: &blockOwnerDeletion,
					Controller:         &controller,
					UID:                s3UserResource.UID,
				},
			},
		},
		Data: map[string][]byte{
			"accessKey": []byte("existing-valid-user"),
			"secretKey": []byte("validSecret"),
		},
	}

	// Add mock for s3Factory and client
	testUtils := TestUtils.NewTestUtils()
	testUtils.SetupMockedS3FactoryAndClient()
	s3instanceResource, secretResource := testUtils.GenerateBasicS3InstanceAndSecret()
	testUtils.SetupClient([]client.Object{s3instanceResource, secretResource, s3UserResource, s3UserSecretResource})

	// Create the reconciler
	reconciler := &user_controller.S3UserReconciler{
		Client:    testUtils.Client,
		Scheme:    testUtils.Client.Scheme(),
		S3factory: testUtils.S3Factory,
	}
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: s3UserResource.Name, Namespace: s3UserResource.Namespace}}
	reconciler.Reconcile(context.TODO(), req)
	testUtils.Client.Delete(context.TODO(), s3UserResource)

	t.Run("no error", func(t *testing.T) {
		// Call Reconcile function
		req := ctrl.Request{NamespacedName: types.NamespacedName{Name: s3UserResource.Name, Namespace: s3UserResource.Namespace}}
		_, err := reconciler.Reconcile(context.TODO(), req)
		assert.NoError(t, err)
	})

	t.Run("ressource have been deleted", func(t *testing.T) {
		// Call Reconcile function
		req := ctrl.Request{NamespacedName: types.NamespacedName{Name: s3UserResource.Name, Namespace: s3UserResource.Namespace}}
		reconciler.Reconcile(context.TODO(), req)
		s3UserResource := &s3v1alpha1.S3User{}
		err := testUtils.Client.Get(context.TODO(), client.ObjectKey{
			Namespace: "default",
			Name:      "example-user",
		}, s3UserResource)
		assert.NotNil(t, err)
		assert.ErrorContains(t, err, "s3users.s3.onyxia.sh \"example-user\" not found")

		s3UserSecret := &corev1.Secret{}
		err = testUtils.Client.Get(context.TODO(), client.ObjectKey{
			Namespace: "default",
			Name:      "existing-valid-user-credentials",
		}, s3UserSecret)
		assert.NotNil(t, err)
		assert.ErrorContains(t, err, "secrets \"existing-valid-user-credentials\" not found")

	})

}
