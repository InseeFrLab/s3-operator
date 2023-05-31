package factory

import (
	"fmt"
	"log"

	"github.com/minio/madmin-go/v2"
)

type MockedS3Client struct{}

func (mockedS3Provider *MockedS3Client) BucketExists(name string) (bool, error) {
	log.Println("check if bucket " + name + "exists")
	return false, nil
}

func (mockedS3Provider *MockedS3Client) CreateBucket(name string) error {
	log.Println("create bucket " + name + "exists")
	return nil
}

func (mockedS3Provider *MockedS3Client) CreatePath(bucketname string, name string) error {
	log.Println("create path " + name + "exists")
	return nil
}

func (mockedS3Provider *MockedS3Client) PathExists(bucketname string, name string) (bool, error) {
	log.Println("check if  path " + name + "exists")
	return true, nil
}

func (mockedS3Provider *MockedS3Client) DeleteBucket(name string) error {
	log.Println("delete bucket " + name + "exists")
	return nil
}

func (mockedS3Provider *MockedS3Client) GetQuota(name string) (int64, error) {
	log.Println("bucket " + name + " get quota")
	return 1, nil
}

func (mockedS3Provider *MockedS3Client) SetQuota(name string, quota int64) error {
	log.Println("set quota " + fmt.Sprint(quota) + "on bucket " + name + "exists")
	return nil
}

func (mockedS3Provider *MockedS3Client) GetPolicyInfo(name string) (*madmin.PolicyInfo, error) {
	log.Println("create policy " + name)
	return nil, nil
}

func (mockedS3Provider *MockedS3Client) CreateOrUpdatePolicy(name string, content string) error {
	log.Println("create policy " + name)
	return nil
}

func newMockedS3Client() *MockedS3Client {
	return &MockedS3Client{}
}
