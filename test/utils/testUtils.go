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

package testUtils

import (
	"fmt"

	s3v1alpha1 "github.com/InseeFrLab/s3-operator/api/v1alpha1"
	s3client "github.com/InseeFrLab/s3-operator/internal/s3/client"
	s3factory "github.com/InseeFrLab/s3-operator/internal/s3/factory"
	"github.com/InseeFrLab/s3-operator/test/mocks"
	"github.com/minio/madmin-go/v3"
	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type TestUtils struct {
	S3Factory s3factory.S3Factory
	Client    client.Client
}

func NewTestUtils() *TestUtils {
	return &TestUtils{}
}

func (t *TestUtils) SetupMockedS3FactoryAndClient() {
	mockedS3Client := mocks.NewMockedS3Client()
	mockedS3Client.On("BucketExists", "test-bucket").Return(false, nil)
	mockedS3Client.On("BucketExists", "existing-bucket").Return(true, nil)
	mockedS3Client.On("CreateBucket", "test-bucket").Return(nil)
	mockedS3Client.On("SetQuota", "test-bucket", int64(10)).Return(nil)
	mockedS3Client.On("ListBuckets").Return([]string{}, nil)
	mockedS3Client.On("GetPolicyInfo", "example-policy").Return(nil, nil)
	existingPolicy := []byte(`{
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
	mockedS3Client.On("GetPolicyInfo", "existing-policy").Return(&madmin.PolicyInfo{PolicyName: "existing-policy", Policy: existingPolicy}, nil)
	mockedS3Client.On("CreateOrUpdatePolicy", "existing-policy", mock.AnythingOfType("string")).Return(nil)
	mockedS3Client.On("CreateOrUpdatePolicy", "example-policy", "").Return(nil)
	mockedS3Client.On("PathExists", "existing-bucket", "mypath").Return(false, nil)
	mockedS3Client.On("CreatePath", "existing-bucket", "mypath").Return(nil)
	mockedS3Client.On("UserExist", "example-user").Return(false, nil)
	mockedS3Client.On("CreateUser", "example-user", mock.AnythingOfType("string")).Return(nil)
	mockedS3Client.On("AddPoliciesToUser", "example-user", mock.AnythingOfType("[]string")).Return(nil)

	mockedS3Client.On("UserExist", "existing-valid-user").Return(true, nil)
	mockedS3Client.On("CreateUser", "existing-valid-user", mock.AnythingOfType("string")).Return(nil)
	mockedS3Client.On("AddPoliciesToUser", "existing-valid-user", mock.AnythingOfType("[]string")).Return(nil)

	mockedS3Client.On("CheckUserCredentialsValid", "existing-valid-user", "existing-valid-user", "invalidSecret").Return(false, nil)
	mockedS3Client.On("CheckUserCredentialsValid", "existing-valid-user", "existing-valid-user", "invalidSecret").Return(false, nil)
	mockedS3Client.On("CheckUserCredentialsValid", "existing-valid-user", "existing-valid-user", "validSecret").Return(true, nil)
	mockedS3Client.On("CheckUserCredentialsValid", "existing-valid-user", "existing-valid-user", mock.AnythingOfType("string")).Return(true, nil)
	mockedS3Client.On("GetQuota", "existing-bucket").Return(10, nil)
	mockedS3Client.On("GetQuota", "existing-invalid-bucket").Return(10, nil)
	mockedS3Client.On("SetQuota", "existing-invalid-bucket", int64(100)).Return(nil)
	mockedS3Client.On("GetUserPolicies", "existing-valid-user").Return([]string{"admin"}, nil)
	mockedS3Client.On("PathExists", "existing-bucket", "example").Return(true, nil)
	mockedS3Client.On("PathExists", "existing-invalid-bucket", "example").Return(true, nil)
	mockedS3Client.On("PathExists", "existing-invalid-bucket", "non-existing").Return(false, nil)
	mockedS3Client.On("BucketExists", "existing-invalid-bucket").Return(true, nil)
	mockedS3Client.On("BucketExists", "non-existing-bucket").Return(false, nil)

	mockedS3Client.On("CreatePath", "existing-invalid-bucket", "non-existing").Return(nil)

	mockedS3Client.On("DeleteUser", "existing-valid-user").Return(nil)

	mockedInvalidS3Client := mocks.NewMockedS3Client()
	mockedInvalidS3Client.On("BucketExists", "test-bucket").Return(false, nil)
	mockedInvalidS3Client.On("CreateBucket", "test-bucket").Return(nil)
	mockedInvalidS3Client.On("SetQuota", "test-bucket", int64(10)).Return(nil)

	mockedInvalidS3Client.On("ListBuckets").Return([]string{}, fmt.Errorf("random error"))

	mockedS3factory := mocks.NewMockedS3ClientFactory()
	mockedS3factory.On("GenerateS3Client", "minio", mock.MatchedBy(func(cfg *s3client.S3Config) bool {
		return cfg.S3Url == "https://minio.example.com"
	})).Return(mockedS3Client, nil)
	mockedS3factory.On("GenerateS3Client", "minio", mock.MatchedBy(func(cfg *s3client.S3Config) bool {
		return cfg.S3Url == "https://minio.invalid.example.com"
	})).Return(mockedInvalidS3Client, nil)

	t.S3Factory = mockedS3factory
}

func (t *TestUtils) SetupDefaultS3instance() *s3v1alpha1.S3Instance {
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
		Status: s3v1alpha1.S3InstanceStatus{Conditions: []metav1.Condition{{Type: s3v1alpha1.ConditionAvailable, Reason: s3v1alpha1.Reconciled}}},
	}

	return s3Instance
}

func (t *TestUtils) GenerateBasicS3InstanceAndSecret() (*s3v1alpha1.S3Instance, *corev1.Secret) {
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
		Status: s3v1alpha1.S3InstanceStatus{Conditions: []metav1.Condition{{Type: s3v1alpha1.ConditionAvailable, Reason: s3v1alpha1.Reconciled}}},
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

	return s3instanceResource, secretResource
}

func (t *TestUtils) SetupClient(objects []client.Object) {
	// Register the custom resource with the scheme	sch := runtime.NewScheme()
	s := scheme.Scheme
	s3v1alpha1.AddToScheme(s)
	corev1.AddToScheme(s)

	client := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(objects...).
		WithStatusSubresource(objects...).
		Build()

	t.Client = client
}
