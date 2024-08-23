package s3factory

import (
	"github.com/minio/madmin-go/v3"
)

type MockedS3Client struct {
	s3Config S3Config
}

func (mockedS3Provider *MockedS3Client) BucketExists(name string) (bool, error) {
	s3Logger.Info("checking bucket existence", "bucket", name)
	return false, nil
}

func (mockedS3Provider *MockedS3Client) CreateBucket(name string) error {
	s3Logger.Info("checking a bucket", "bucket", name)
	return nil
}

func (mockedS3Provider *MockedS3Client) DeleteBucket(name string) error {
	s3Logger.Info("deleting a bucket", "bucket", name)
	return nil
}

func (mockedS3Provider *MockedS3Client) CreatePath(bucketname string, path string) error {
	s3Logger.Info("creating a path on a bucket", "bucket", bucketname, "path", path)
	return nil
}

func (mockedS3Provider *MockedS3Client) PathExists(bucketname string, path string) (bool, error) {
	s3Logger.Info("checking path existence on a bucket", "bucket", bucketname, "path", path)
	return true, nil
}

func (mockedS3Provider *MockedS3Client) DeletePath(bucketname string, path string) error {
	s3Logger.Info("deleting a path on a bucket", "bucket", bucketname, "path", path)
	return nil
}

func (mockedS3Provider *MockedS3Client) GetQuota(name string) (int64, error) {
	s3Logger.Info("getting quota on bucket", "bucket", name)
	return 1, nil
}

func (mockedS3Provider *MockedS3Client) SetQuota(name string, quota int64) error {
	s3Logger.Info("setting quota on bucket", "bucket", name, "quotaToSet", quota)
	return nil
}

func (mockedS3Provider *MockedS3Client) GetPolicyInfo(name string) (*madmin.PolicyInfo, error) {
	s3Logger.Info("retrieving policy info", "policy", name)
	return nil, nil
}

func (mockedS3Provider *MockedS3Client) CreateOrUpdatePolicy(name string, content string) error {
	s3Logger.Info("create or update policy", "policy", name, "policyContent", content)
	return nil
}

func (mockedS3Provider *MockedS3Client) CreateUser(name string, password string) error {
	s3Logger.Info("create or update user", "user", name)
	return nil
}

func (mockedS3Provider *MockedS3Client) UserExist(name string) (bool, error) {
	s3Logger.Info("checking user existence", "user", name)
	return true, nil
}

func (mockedS3Provider *MockedS3Client) AddServiceAccountForUser(name string, accessKey string, secretKey string) error {
	s3Logger.Info("Adding service account for user", "user", name)
	return nil
}

func (mockedS3Provider *MockedS3Client) PolicyExist(name string) (bool, error) {
	s3Logger.Info("checking policy existence", "policy", name)
	return true, nil
}

func (mockedS3Provider *MockedS3Client) AddPoliciesToUser(username string, policies []string) error {
	s3Logger.Info("Adding policies to user", "user", username, "policies", policies)
	return nil
}

func (mockedS3Provider *MockedS3Client) DeletePolicy(name string) error {
	s3Logger.Info("delete policy", "policy", name)
	return nil
}

func (mockedS3Provider *MockedS3Client) DeleteUser(name string) error {
	s3Logger.Info("delete user", "user", name)
	return nil
}

func (mockedS3Provider *MockedS3Client) CheckUserCredentialsValid(name string, accessKey string, secretKey string) (bool, error) {
	s3Logger.Info("checking credential for user", "user", name)
	return true, nil
}

func (mockedS3Provider *MockedS3Client) GetUserPolicies(name string) ([]string, error) {
	s3Logger.Info("Getting user policies for user", "user", name)
	return []string{}, nil
}

func (mockedS3Provider *MockedS3Client) RemovePoliciesFromUser(username string, policies []string) error {
	s3Logger.Info("Removing policies from user", "user", username)
	return nil
}

func (mockedS3Provider *MockedS3Client) ListBuckets() ([]string, error) {
	return []string{}, nil
}

func (mockedS3Provider *MockedS3Client) GetConfig() *S3Config {
	return &mockedS3Provider.s3Config
}

func newMockedS3Client() *MockedS3Client {
	return &MockedS3Client{s3Config: S3Config{}}
}
