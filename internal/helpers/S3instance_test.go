package helpers_test

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	s3v1alpha1 "github.com/InseeFrLab/s3-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	helpers "github.com/InseeFrLab/s3-operator/internal/helpers"
	TestUtils "github.com/InseeFrLab/s3-operator/test/utils"
	"github.com/stretchr/testify/assert"
)

func TestGetS3ClientForRessource(t *testing.T) {

	// Set up a logger before running tests
	log.SetLogger(zap.New(zap.UseDevMode(true)))

	// Register the custom resource with the scheme	sch := runtime.NewScheme()
	s := scheme.Scheme
	s3v1alpha1.AddToScheme(s)
	corev1.AddToScheme(s)

	s3Instance := &s3v1alpha1.S3Instance{
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
		Status: s3v1alpha1.S3InstanceStatus{Conditions: []metav1.Condition{{Reason: s3v1alpha1.Reconciled}}},
	}

	s3Instance_not_ready := &s3v1alpha1.S3Instance{
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
			Name:      "not-ready",
			Namespace: "s3-operator",
		},
		Status: s3v1alpha1.S3InstanceStatus{Conditions: []metav1.Condition{{Reason: s3v1alpha1.CreationFailure}}},
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "minio-credentials",
			Namespace: "s3-operator",
		},
		StringData: map[string]string{
			"accessKey": "access_key_value",
			"secretKey": "secret_key_value",
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(s3Instance, s3Instance_not_ready, secret).
		WithStatusSubresource(s3Instance, s3Instance_not_ready, secret).
		Build()

	// Add mock for s3Factory and client
	testUtils := TestUtils.NewTestUtils()
	testUtils.SetupMockedS3FactoryAndClient()

	t.Run("no error", func(t *testing.T) {
		s3instanceHelper := helpers.NewS3InstanceHelper()
		_, err := s3instanceHelper.GetS3ClientForRessource(context.TODO(), client, testUtils.S3Factory, "bucket-example", "default", "s3-operator/default")
		assert.NoError(t, err)
	})

	t.Run("error because instance not ready", func(t *testing.T) {
		s3instanceHelper := helpers.NewS3InstanceHelper()
		_, err := s3instanceHelper.GetS3ClientForRessource(context.TODO(), client, testUtils.S3Factory, "bucket-example", "default", "s3-operator/not-ready")
		assert.Equal(t, "S3instance is not in a ready state", err.Error())
	})
}

func TestGetS3ClientFromS3instance(t *testing.T) {
	// Set up a logger before running tests
	log.SetLogger(zap.New(zap.UseDevMode(true)))

	// Register the custom resource with the scheme	sch := runtime.NewScheme()
	s := scheme.Scheme
	s3v1alpha1.AddToScheme(s)
	corev1.AddToScheme(s)

	s3Instance := &s3v1alpha1.S3Instance{
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

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "minio-credentials",
			Namespace: "s3-operator",
		},
		StringData: map[string]string{
			"accessKey": "access_key_value",
			"secretKey": "secret_key_value",
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(s3Instance, secret).
		WithStatusSubresource(s3Instance, secret).
		Build()

	// Add mock for s3Factory and client
	testUtils := TestUtils.NewTestUtils()
	testUtils.SetupMockedS3FactoryAndClient()

	t.Run("no error", func(t *testing.T) {
		s3instanceHelper := helpers.NewS3InstanceHelper()
		_, err := s3instanceHelper.GetS3ClientFromS3Instance(context.TODO(), client, testUtils.S3Factory, s3Instance)
		assert.NoError(t, err)
	})
}

func TestIsAllowedNamespaces(t *testing.T) {
	s3Instance := &s3v1alpha1.S3Instance{
		Spec: s3v1alpha1.S3InstanceSpec{
			AllowedNamespaces: []string{"default", "test-*", "*-namespace", "*allowed*"},
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default",
			Namespace: "s3-operator",
		},
	}

	t.Run("Exact match", func(t *testing.T) {
		s3instanceHelper := helpers.NewS3InstanceHelper()
		result := s3instanceHelper.IsAllowedNamespaces("default", s3Instance)
		assert.Equal(t, true, result)
	})

	t.Run("Wildcard prefix", func(t *testing.T) {
		s3instanceHelper := helpers.NewS3InstanceHelper()
		result := s3instanceHelper.IsAllowedNamespaces("test-namespace", s3Instance)
		assert.Equal(t, true, result)
	})

	t.Run("Wildcard suffix", func(t *testing.T) {
		s3instanceHelper := helpers.NewS3InstanceHelper()
		result := s3instanceHelper.IsAllowedNamespaces("my-namespace", s3Instance)
		assert.Equal(t, true, result)
	})

	t.Run("Wildcard contains", func(t *testing.T) {
		s3instanceHelper := helpers.NewS3InstanceHelper()
		result := s3instanceHelper.IsAllowedNamespaces("this-is-allowed-namespace", s3Instance)
		assert.Equal(t, true, result)
	})

	t.Run("Not allowed", func(t *testing.T) {
		s3instanceHelper := helpers.NewS3InstanceHelper()
		result := s3instanceHelper.IsAllowedNamespaces("random", s3Instance)
		assert.Equal(t, false, result)
	})
}
