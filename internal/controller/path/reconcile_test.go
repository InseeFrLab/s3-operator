package path_controller_test

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
	path_controller "github.com/InseeFrLab/s3-operator/internal/controller/path"
	"github.com/stretchr/testify/assert"
)

func TestHandleCreate(t *testing.T) {
	// Set up a logger before running tests
	log.SetLogger(zap.New(zap.UseDevMode(true)))

	// Create a fake client with a sample CR
	pathResource := &s3v1alpha1.Path{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "example-path",
			Namespace:  "default",
			Generation: 1,
		},
		Spec: s3v1alpha1.PathSpec{
			BucketName:    "existing-bucket",
			Paths:         []string{"mypath"},
			S3InstanceRef: "s3-operator/default",
		},
	}

	// Add mock for s3Factory and client
	testUtils := TestUtils.NewTestUtils()
	testUtils.SetupMockedS3FactoryAndClient()
	s3instanceResource, secretResource := testUtils.GenerateBasicS3InstanceAndSecret()
	testUtils.SetupClient([]client.Object{s3instanceResource, secretResource, pathResource})

	// Create the reconciler
	reconciler := &path_controller.PathReconciler{
		Client:    testUtils.Client,
		Scheme:    testUtils.Client.Scheme(),
		S3factory: testUtils.S3Factory,
	}

	t.Run("no error", func(t *testing.T) {
		// Call Reconcile function
		req := ctrl.Request{NamespacedName: types.NamespacedName{Name: pathResource.Name, Namespace: pathResource.Namespace}}
		_, err := reconciler.Reconcile(context.TODO(), req)
		assert.NoError(t, err)
	})
}
