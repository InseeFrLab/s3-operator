package factory

import (
	"github.com/minio/madmin-go/v2"
)

type MockedS3Client struct{}

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

func newMockedS3Client() *MockedS3Client {
	return &MockedS3Client{}
}
