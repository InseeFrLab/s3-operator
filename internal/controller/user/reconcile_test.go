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

func TestHandleCreate(t *testing.T) {
	// Set up a logger before running tests
	log.SetLogger(zap.New(zap.UseDevMode(true)))

	// Create a fake client with a sample CR
	s3UserResource := &s3v1alpha1.S3User{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "example-user",
			Namespace:  "default",
			Generation: 1,
		},
		Spec: s3v1alpha1.S3UserSpec{
			S3InstanceRef: "s3-operator/default",
			AccessKey:     "example-user",
			SecretName:    "example-user-secret",
			Policies:      []string{"admin"},
		},
	}

	// Create a fake client with a sample CR
	s3UserUsingNotAllowedS3Instance := &s3v1alpha1.S3User{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "example-user",
			Namespace:  "unauthorized",
			Generation: 1,
		},
		Spec: s3v1alpha1.S3UserSpec{
			S3InstanceRef: "s3-operator/default",
			AccessKey:     "example-user",
			SecretName:    "example-user-secret",
			Policies:      []string{"admin"},
		},
	}

	// Add mock for s3Factory and client
	testUtils := TestUtils.NewTestUtils()
	testUtils.SetupMockedS3FactoryAndClient()
	s3instanceResource, secretResource := testUtils.GenerateBasicS3InstanceAndSecret()
	testUtils.SetupClient([]client.Object{s3instanceResource, secretResource, s3UserResource, s3UserUsingNotAllowedS3Instance})

	// Create the reconciler
	reconciler := &user_controller.S3UserReconciler{
		Client:    testUtils.Client,
		Scheme:    testUtils.Client.Scheme(),
		S3factory: testUtils.S3Factory,
	}

	t.Run("no error", func(t *testing.T) {
		// Call Reconcile function
		req := ctrl.Request{NamespacedName: types.NamespacedName{Name: s3UserResource.Name, Namespace: s3UserResource.Namespace}}
		_, err := reconciler.Reconcile(context.TODO(), req)
		assert.NoError(t, err)
	})

	t.Run("error if using invalidS3Instance", func(t *testing.T) {
		// Call Reconcile function
		req := ctrl.Request{NamespacedName: types.NamespacedName{Name: s3UserUsingNotAllowedS3Instance.Name, Namespace: s3UserUsingNotAllowedS3Instance.Namespace}}
		_, err := reconciler.Reconcile(context.TODO(), req)
		assert.NotNil(t, err)
	})

	t.Run("secret is created", func(t *testing.T) {
		// Call Reconcile function
		req := ctrl.Request{NamespacedName: types.NamespacedName{Name: s3UserResource.Name, Namespace: s3UserResource.Namespace}}
		reconciler.Reconcile(context.TODO(), req)

		secretCreated := &corev1.Secret{}
		err := testUtils.Client.Get(context.TODO(), client.ObjectKey{
			Namespace: "default",
			Name:      "example-user-secret",
		}, secretCreated)
		assert.NoError(t, err)
		assert.Equal(t, "example-user", string(secretCreated.Data["accessKey"]))
		assert.GreaterOrEqual(t, len(string(secretCreated.Data["secretKey"])), 20)

	})
}

func TestHandleUpdate(t *testing.T) {
	// Set up a logger before running tests
	log.SetLogger(zap.New(zap.UseDevMode(true)))

	t.Run("valid user", func(t *testing.T) {
		// Create a fake client with a sample CR
		s3UserResource := &s3v1alpha1.S3User{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "existing-valid-user",
				Namespace:  "default",
				Generation: 1,
				Finalizers: []string{"s3.onyxia.sh/userFinalizer"},
			},
			Spec: s3v1alpha1.S3UserSpec{
				S3InstanceRef: "s3-operator/default",
				AccessKey:     "existing-valid-user",
				SecretName:    "existing-valid-user-secret",
				Policies:      []string{"admin"},
			},
		}

		secretS3UserResource := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "existing-valid-user-secret",
				Namespace: "default",
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
		testUtils.SetupClient([]client.Object{s3instanceResource, secretResource, s3UserResource, secretS3UserResource})

		// Create the reconciler
		reconciler := &user_controller.S3UserReconciler{
			Client:    testUtils.Client,
			Scheme:    testUtils.Client.Scheme(),
			S3factory: testUtils.S3Factory,
		}

		t.Run("no error", func(t *testing.T) {
			// Call Reconcile function
			req := ctrl.Request{NamespacedName: types.NamespacedName{Name: s3UserResource.Name, Namespace: s3UserResource.Namespace}}
			_, err := reconciler.Reconcile(context.TODO(), req)
			assert.NoError(t, err)
		})
	})

	t.Run("invalid user password", func(t *testing.T) {
		// Create a fake client with a sample CR
		s3UserResource := &s3v1alpha1.S3User{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "existing-valid-user",
				Namespace:  "default",
				Generation: 1,
				Finalizers: []string{"s3.onyxia.sh/userFinalizer"},
			},
			Spec: s3v1alpha1.S3UserSpec{
				S3InstanceRef: "s3-operator/default",
				AccessKey:     "existing-valid-user",
				SecretName:    "existing-valid-user-secret",
				Policies:      []string{"admin"},
			},
		}

		secretS3UserResource := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "existing-valid-user-secret",
				Namespace: "default",
			},
			Data: map[string][]byte{
				"accessKey": []byte("existing-valid-user"),
				"secretKey": []byte("invalidSecret"),
			},
		}

		// Add mock for s3Factory and client
		testUtils := TestUtils.NewTestUtils()
		testUtils.SetupMockedS3FactoryAndClient()
		s3instanceResource, secretResource := testUtils.GenerateBasicS3InstanceAndSecret()
		testUtils.SetupClient([]client.Object{s3instanceResource, secretResource, s3UserResource, secretS3UserResource})

		// Create the reconciler
		reconciler := &user_controller.S3UserReconciler{
			Client:    testUtils.Client,
			Scheme:    testUtils.Client.Scheme(),
			S3factory: testUtils.S3Factory,
		}

		t.Run("no error", func(t *testing.T) {
			// Call Reconcile function
			req := ctrl.Request{NamespacedName: types.NamespacedName{Name: s3UserResource.Name, Namespace: s3UserResource.Namespace}}
			_, err := reconciler.Reconcile(context.TODO(), req)
			assert.NoError(t, err)
		})

		t.Run("secret have changed", func(t *testing.T) {
			// Call Reconcile function
			req := ctrl.Request{NamespacedName: types.NamespacedName{Name: s3UserResource.Name, Namespace: s3UserResource.Namespace}}
			reconciler.Reconcile(context.TODO(), req)

			secretCreated := &corev1.Secret{}
			err := testUtils.Client.Get(context.TODO(), client.ObjectKey{
				Namespace: "default",
				Name:      "existing-valid-user-secret",
			}, secretCreated)
			assert.NoError(t, err)
			assert.Equal(t, "existing-valid-user", string(secretCreated.Data["accessKey"]))
			assert.NotEqualValues(t, string(secretCreated.Data["secretKey"]), "invalidSecret")
		})
	})

	t.Run("invalid user policy", func(t *testing.T) {
		// Create a fake client with a sample CR
		s3UserResource := &s3v1alpha1.S3User{
			ObjectMeta: metav1.ObjectMeta{
				Name:       "existing-valid-user",
				Namespace:  "default",
				Generation: 1,
				Finalizers: []string{"s3.onyxia.sh/userFinalizer"},
			},
			Spec: s3v1alpha1.S3UserSpec{
				S3InstanceRef: "s3-operator/default",
				AccessKey:     "existing-valid-user",
				SecretName:    "existing-valid-user",
				Policies:      []string{"admin", "missing-policy"},
			},
		}

		secretS3UserResource := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "existing-valid-user-secret",
				Namespace: "default",
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
		testUtils.SetupClient([]client.Object{s3instanceResource, secretResource, s3UserResource, secretS3UserResource})

		// Create the reconciler
		reconciler := &user_controller.S3UserReconciler{
			Client:    testUtils.Client,
			Scheme:    testUtils.Client.Scheme(),
			S3factory: testUtils.S3Factory,
		}

		t.Run("no error", func(t *testing.T) {
			// Call Reconcile function
			req := ctrl.Request{NamespacedName: types.NamespacedName{Name: s3UserResource.Name, Namespace: s3UserResource.Namespace}}
			_, err := reconciler.Reconcile(context.TODO(), req)
			assert.NoError(t, err)
		})
	})

}
