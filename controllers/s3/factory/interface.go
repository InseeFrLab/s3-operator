package factory

import (
	"fmt"

	"github.com/minio/madmin-go/v3"

	ctrl "sigs.k8s.io/controller-runtime"
)

var (
	s3Logger = ctrl.Log.WithValues("logger", "s3client")
)

type S3Client interface {
	BucketExists(name string) (bool, error)
	CreateBucket(name string) error
	DeleteBucket(name string) error
	CreatePath(bucketname string, path string) error
	PathExists(bucketname string, path string) (bool, error)
	DeletePath(bucketname string, path string) error
	GetQuota(name string) (int64, error)
	SetQuota(name string, quota int64) error
	// see comment in [minioS3Client.go] regarding the absence of a PolicyExists method
	// PolicyExists(name string) (bool, error)
	PolicyExist(name string) (bool, error)
	DeletePolicy(name string) error
	GetPolicyInfo(name string) (*madmin.PolicyInfo, error)
	CreateOrUpdatePolicy(name string, content string) error
	UserExist(name string) (bool, error)
	CheckUserCredentialsValid(name string, accessKey string, secretKey string) (bool, error)
	AddServiceAccountForUser(name string, accessKey string, secretKey string) error
	CreateUser(accessKey string, secretKey string) error
	DeleteUser(accessKey string) error
	GetUserPolicies(name string) ([]string, error)
	AddPoliciesToUser(accessKey string, policies []string) error
	RemovePoliciesFromUser(accessKey string, policies []string) error
}

type S3Config struct {
	S3Provider           string
	S3UrlEndpoint        string
	Region               string
	AccessKey            string
	SecretKey            string
	UseSsl               bool
	CaCertificatesBase64 []string
	CaBundlePath         string
}

func GetS3Client(s3Provider string, S3Config *S3Config) (S3Client, error) {
	if s3Provider == "mockedS3Provider" {
		return newMockedS3Client(), nil
	}
	if s3Provider == "minio" {
		return newMinioS3Client(S3Config), nil
	}
	return nil, fmt.Errorf("s3 provider " + s3Provider + "not supported")
}
