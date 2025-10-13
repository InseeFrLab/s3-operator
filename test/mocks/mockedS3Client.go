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

package mocks

import (
	s3client "github.com/InseeFrLab/s3-operator/internal/s3/client"
	"github.com/minio/madmin-go/v3"
	"github.com/stretchr/testify/mock"
	ctrl "sigs.k8s.io/controller-runtime"
)

type MockedS3Client struct {
	s3Config s3client.S3Config
	mock.Mock
}

func (mockedS3Provider *MockedS3Client) BucketExists(name string) (bool, error) {
	s3Logger := ctrl.Log.WithValues("logger", "mockedS3Client")
	s3Logger.Info("checking bucket existence", "bucket", name)
	args := mockedS3Provider.Called(name)
	return args.Bool(0), args.Error(1)
}

func (mockedS3Provider *MockedS3Client) CreateBucket(name string) error {
	s3Logger := ctrl.Log.WithValues("logger", "mockedS3Client")
	s3Logger.Info("checking a bucket", "bucket", name)
	args := mockedS3Provider.Called(name)
	return args.Error(0)
}

func (mockedS3Provider *MockedS3Client) DeleteBucket(name string) error {
	s3Logger := ctrl.Log.WithValues("logger", "mockedS3Client")
	s3Logger.Info("deleting a bucket", "bucket", name)
	args := mockedS3Provider.Called(name)
	return args.Error(0)
}

func (mockedS3Provider *MockedS3Client) CreatePath(bucketname string, path string) error {
	s3Logger := ctrl.Log.WithValues("logger", "mockedS3Client")
	s3Logger.Info("creating a path on a bucket", "bucket", bucketname, "path", path)
	args := mockedS3Provider.Called(bucketname, path)
	return args.Error(0)
}

func (mockedS3Provider *MockedS3Client) PathExists(bucketname string, path string) (bool, error) {
	s3Logger := ctrl.Log.WithValues("logger", "mockedS3Client")
	s3Logger.Info("checking path existence on a bucket", "bucket", bucketname, "path", path)
	args := mockedS3Provider.Called(bucketname, path)
	return args.Bool(0), args.Error(1)
}

func (mockedS3Provider *MockedS3Client) DeletePath(bucketname string, path string) error {
	s3Logger := ctrl.Log.WithValues("logger", "mockedS3Client")
	s3Logger.Info("deleting a path on a bucket", "bucket", bucketname, "path", path)
	args := mockedS3Provider.Called(bucketname, path)
	return args.Error(0)
}

func (mockedS3Provider *MockedS3Client) GetQuota(name string) (int64, error) {
	s3Logger := ctrl.Log.WithValues("logger", "mockedS3Client")
	s3Logger.Info("getting quota on bucket", "bucket", name)
	args := mockedS3Provider.Called(name)
	return int64(args.Int(0)), args.Error(1)
}

func (mockedS3Provider *MockedS3Client) SetQuota(name string, quota int64) error {
	s3Logger := ctrl.Log.WithValues("logger", "mockedS3Client")
	s3Logger.Info("setting quota on bucket", "bucket", name, "quotaToSet", quota)
	args := mockedS3Provider.Called(name, quota)
	return args.Error(0)
}

func (mockedS3Provider *MockedS3Client) GetPolicyInfo(name string) (*madmin.PolicyInfo, error) {
	s3Logger := ctrl.Log.WithValues("logger", "mockedS3Client")
	s3Logger.Info("retrieving policy info", "policy", name)
	args := mockedS3Provider.Called(name)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*madmin.PolicyInfo), args.Error(1)

}

func (mockedS3Provider *MockedS3Client) CreateOrUpdatePolicy(name string, content string) error {
	s3Logger := ctrl.Log.WithValues("logger", "mockedS3Client")
	s3Logger.Info("create or update policy", "policy", name, "policyContent", content)
	args := mockedS3Provider.Called(name, content)
	return args.Error(0)
}

func (mockedS3Provider *MockedS3Client) CreateUser(name string, password string) error {
	s3Logger := ctrl.Log.WithValues("logger", "mockedS3Client")
	s3Logger.Info("create or update user", "user", name)
	args := mockedS3Provider.Called(name, password)
	return args.Error(0)
}

func (mockedS3Provider *MockedS3Client) UserExist(name string) (bool, error) {
	s3Logger := ctrl.Log.WithValues("logger", "mockedS3Client")
	s3Logger.Info("checking user existence", "user", name)
	args := mockedS3Provider.Called(name)
	return args.Bool(0), args.Error(1)
}

func (mockedS3Provider *MockedS3Client) AddServiceAccountForUser(
	name string,
	accessKey string,
	secretKey string,
) error {
	s3Logger := ctrl.Log.WithValues("logger", "mockedS3Client")
	s3Logger.Info("Adding service account for user", "user", name)
	args := mockedS3Provider.Called(name)
	return args.Error(0)
}

func (mockedS3Provider *MockedS3Client) PolicyExist(name string) (bool, error) {
	s3Logger := ctrl.Log.WithValues("logger", "mockedS3Client")
	s3Logger.Info("checking policy existence", "policy", name)
	args := mockedS3Provider.Called(name)
	return args.Bool(0), args.Error(1)
}

func (mockedS3Provider *MockedS3Client) AddPoliciesToUser(
	username string,
	policies []string,
) error {
	s3Logger := ctrl.Log.WithValues("logger", "mockedS3Client")
	s3Logger.Info("Adding policies to user", "user", username, "policies", policies)
	args := mockedS3Provider.Called(username, policies)
	return args.Error(0)
}

func (mockedS3Provider *MockedS3Client) DeletePolicy(name string) error {
	s3Logger := ctrl.Log.WithValues("logger", "mockedS3Client")
	s3Logger.Info("delete policy", "policy", name)
	args := mockedS3Provider.Called(name)
	return args.Error(0)
}

func (mockedS3Provider *MockedS3Client) DeleteUser(name string) error {
	s3Logger := ctrl.Log.WithValues("logger", "mockedS3Client")
	s3Logger.Info("delete user", "user", name)
	args := mockedS3Provider.Called(name)
	return args.Error(0)
}

func (mockedS3Provider *MockedS3Client) CheckUserCredentialsValid(
	name string,
	accessKey string,
	secretKey string,
) (bool, error) {
	s3Logger := ctrl.Log.WithValues("logger", "mockedS3Client")
	s3Logger.Info("checking credential for user", "user", name)
	args := mockedS3Provider.Called(name, accessKey, secretKey)
	return args.Bool(0), args.Error(1)
}

func (mockedS3Provider *MockedS3Client) GetUserPolicies(name string) ([]string, error) {
	s3Logger := ctrl.Log.WithValues("logger", "mockedS3Client")
	s3Logger.Info("Getting user policies for user", "user", name)
	args := mockedS3Provider.Called(name)
	return args.Get(0).([]string), args.Error(1)
}

func (mockedS3Provider *MockedS3Client) RemovePoliciesFromUser(
	username string,
	policies []string,
) error {
	s3Logger := ctrl.Log.WithValues("logger", "mockedS3Client")
	s3Logger.Info("Removing policies from user", "user", username)
	args := mockedS3Provider.Called(username, policies)
	return args.Error(0)
}

func (mockedS3Provider *MockedS3Client) ListBuckets() ([]string, error) {
	s3Logger := ctrl.Log.WithValues("logger", "mockedS3Client")
	s3Logger.Info("Listing bucket")
	args := mockedS3Provider.Called()
	return args.Get(0).([]string), args.Error(1)
}

func (mockedS3Provider *MockedS3Client) GetConfig() *s3client.S3Config {
	return &mockedS3Provider.s3Config
}

func NewMockedS3Client(s3Config s3client.S3Config) *MockedS3Client {
	return &MockedS3Client{s3Config: s3Config}
}
