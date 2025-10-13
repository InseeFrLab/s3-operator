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

package s3client

import (
	"github.com/minio/madmin-go/v3"
)

type S3Config struct {
	S3Provider            string
	S3Url                 string
	Region                string
	AccessKey             string
	SecretKey             string
	CaCertificatesBase64  []string
	AllowedNamespaces     []string
	BucketDeletionEnabled bool
	S3UserDeletionEnabled bool
	PathDeletionEnabled   bool
	PolicyDeletionEnabled bool
	Secure                bool
	Endpoint              string
}

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
	GetConfig() *S3Config
	ListBuckets() ([]string, error)
}
