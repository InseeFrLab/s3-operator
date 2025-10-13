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

package s3clientimpl

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	neturl "net/url"
	"slices"
	"strings"

	s3client "github.com/InseeFrLab/s3-operator/internal/s3/client"
	"github.com/minio/madmin-go/v3"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	ctrl "sigs.k8s.io/controller-runtime"
)

type MinioS3Client struct {
	s3Config    s3client.S3Config
	client      minio.Client
	adminClient madmin.AdminClient
}

func NewMinioS3Client(S3Config *s3client.S3Config) (*MinioS3Client, error) {
	s3Logger := ctrl.Log.WithValues("logger", "s3clientimplminio")
	s3Logger.Info("creating minio clients (regular and admin)")

	minioClient, err := generateMinioClient(
		S3Config.Endpoint,
		S3Config.Secure,
		S3Config.AccessKey,
		S3Config.SecretKey,
		S3Config.Region,
		S3Config.CaCertificatesBase64,
	)
	if err != nil {
		s3Logger.Error(err, "an error occurred while creating a new minio client")
		return nil, err
	}
	adminClient, err := generateAdminMinioClient(
		S3Config.Endpoint,
		S3Config.Secure,
		S3Config.AccessKey,
		S3Config.SecretKey,
		S3Config.CaCertificatesBase64,
	)
	if err != nil {
		s3Logger.Error(err, "an error occurred while creating a new minio admin client")
		return nil, err
	}
	return &MinioS3Client{*S3Config, *minioClient, *adminClient}, nil
}

func generateMinioClient(
	endpoint string,
	isSSL bool,
	accessKey string,
	secretKey string,
	region string,
	caCertificates []string,
) (*minio.Client, error) {
	s3Logger := ctrl.Log.WithValues("logger", "s3clientimplminio")

	minioOptions := &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Region: region,
		Secure: isSSL,
	}

	if len(caCertificates) > 0 {
		addTlsClientConfigToMinioOptions(caCertificates, minioOptions)
	}

	minioClient, err := minio.New(endpoint, minioOptions)
	if err != nil {
		s3Logger.Error(err, "an error occurred while creating a new minio client")
		return nil, err
	}
	return minioClient, nil
}

func generateAdminMinioClient(
	endpoint string,
	isSSL bool,
	accessKey string,
	secretKey string,
	caCertificates []string,
) (*madmin.AdminClient, error) {
	s3Logger := ctrl.Log.WithValues("logger", "s3clientimplminio")

	minioOptions := &madmin.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: isSSL,
	}

	if len(caCertificates) > 0 {
		addTlsClientConfigToMinioAdminOptions(caCertificates, minioOptions)
	}

	minioAdminClient, err := madmin.NewWithOptions(endpoint, minioOptions)
	if err != nil {
		s3Logger.Error(err, "an error occurred while creating a new minio admin client")
		return nil, err
	}

	return minioAdminClient, nil
}

func ConstructEndpointFromURL(url string) (string, bool, error) {
	parsedURL, err := neturl.Parse(url)
	if err != nil {
		return "", false, fmt.Errorf("cannot detect if url use ssl or not")
	}

	var endpoint = parsedURL.Hostname()
	if !((parsedURL.Scheme == "https" && parsedURL.Port() == "443") ||
		(parsedURL.Scheme == "http" && parsedURL.Port() == "80")) {
		endpoint = fmt.Sprintf("%s:%s", endpoint, parsedURL.Port())
	}

	return endpoint, parsedURL.Scheme == "https", nil
}

func addTlsClientConfigToMinioOptions(caCertificates []string, minioOptions *minio.Options) {
	rootCAs, _ := x509.SystemCertPool()
	if rootCAs == nil {
		rootCAs = x509.NewCertPool()
	}

	for _, caCertificate := range caCertificates {
		rootCAs.AppendCertsFromPEM([]byte(caCertificate))
	}

	minioOptions.Transport = &http.Transport{
		TLSClientConfig: &tls.Config{
			RootCAs: rootCAs,
		},
	}
}

func addTlsClientConfigToMinioAdminOptions(caCertificates []string, minioOptions *madmin.Options) {
	rootCAs, _ := x509.SystemCertPool()
	if rootCAs == nil {
		rootCAs = x509.NewCertPool()
	}

	for _, caCertificate := range caCertificates {
		// caCertificateAsByte := []byte(caCertificate)
		// caCertificateEncoded := base64.StdEncoding.EncodeToString(caCertificateAsByte)
		// rootCAs.AppendCertsFromPEM([]byte(caCertificateEncoded))
		rootCAs.AppendCertsFromPEM([]byte(caCertificate))

	}

	minioOptions.Transport = &http.Transport{
		TLSClientConfig: &tls.Config{
			RootCAs: rootCAs,
		},
	}
}

// //////////////////
// Bucket methods //
// //////////////////
func (minioS3Client *MinioS3Client) BucketExists(name string) (bool, error) {
	s3Logger := ctrl.Log.WithValues("logger", "s3clientimplminio")
	s3Logger.Info("checking bucket existence", "bucket", name)
	return minioS3Client.client.BucketExists(context.Background(), name)
}

func (minioS3Client *MinioS3Client) CreateBucket(name string) error {
	s3Logger := ctrl.Log.WithValues("logger", "s3clientimplminio")
	s3Logger.Info("creating bucket", "bucket", name)
	return minioS3Client.client.MakeBucket(
		context.Background(),
		name,
		minio.MakeBucketOptions{Region: minioS3Client.s3Config.Region},
	)
}

func (minioS3Client *MinioS3Client) ListBuckets() ([]string, error) {
	s3Logger := ctrl.Log.WithValues("logger", "s3clientimplminio")
	s3Logger.Info("listing bucket")
	listBucketsInfo, err := minioS3Client.client.ListBuckets(context.Background())
	bucketsName := []string{}
	if err != nil {
		errAsResponse := minio.ToErrorResponse(err)
		s3Logger.Error(err, "an error occurred while listing buckets", "code", errAsResponse.Code)
		return bucketsName, err
	}
	for _, bucketInfo := range listBucketsInfo {
		bucketsName = append(bucketsName, bucketInfo.Name)
	}
	return bucketsName, nil
}

// Will fail if bucket is not empty
func (minioS3Client *MinioS3Client) DeleteBucket(name string) error {
	s3Logger := ctrl.Log.WithValues("logger", "s3clientimplminio")
	s3Logger.Info("deleting bucket", "bucket", name)
	return minioS3Client.client.RemoveBucket(context.Background(), name)
}

func (minioS3Client *MinioS3Client) CreatePath(bucketname string, path string) error {
	s3Logger := ctrl.Log.WithValues("logger", "s3clientimplminio")
	s3Logger.Info("creating a path on a bucket", "bucket", bucketname, "path", path)
	emptyReader := bytes.NewReader([]byte(""))
	_, err := minioS3Client.client.PutObject(
		context.Background(),
		bucketname,
		"/"+path+"/"+".keep",
		emptyReader,
		0,
		minio.PutObjectOptions{},
	)
	if err != nil {
		s3Logger.Error(
			err,
			"an error occurred during path creation on bucket",
			"bucket",
			bucketname,
			"path",
			path,
		)
		return err
	}
	return nil
}

func (minioS3Client *MinioS3Client) PathExists(bucketname string, path string) (bool, error) {
	s3Logger := ctrl.Log.WithValues("logger", "s3clientimplminio")
	s3Logger.Info("checking path existence on a bucket", "bucket", bucketname, "path", path)
	_, err := minioS3Client.client.
		StatObject(context.Background(),
			bucketname,
			"/"+path+"/"+".keep",
			minio.StatObjectOptions{})

	if err != nil {
		if minio.ToErrorResponse(err).StatusCode == 404 {
			// fmt.Println("The path does not exist")
			s3Logger.Info("the path does not exist", "bucket", bucketname, "path", path)
			return false, nil
		} else {
			s3Logger.Error(err, "an error occurred while checking path existence", "bucket", bucketname, "path", path)
			return false, err
		}
	}

	return true, nil
}

func (minioS3Client *MinioS3Client) DeletePath(bucketname string, path string) error {
	s3Logger := ctrl.Log.WithValues("logger", "s3clientimplminio")
	s3Logger.Info("deleting a path on a bucket", "bucket", bucketname, "path", path)
	err := minioS3Client.client.RemoveObject(
		context.Background(),
		bucketname,
		"/"+path+"/.keep",
		minio.RemoveObjectOptions{},
	)
	if err != nil {
		s3Logger.Error(
			err,
			"an error occurred during path deletion on bucket",
			"bucket",
			bucketname,
			"path",
			path,
		)
		return err
	}
	return nil
}

// /////////////////
// Quota methods //
// /////////////////
func (minioS3Client *MinioS3Client) GetQuota(name string) (int64, error) {
	s3Logger := ctrl.Log.WithValues("logger", "s3clientimplminio")
	s3Logger.Info("getting quota on bucket", "bucket", name)
	bucketQuota, err := minioS3Client.adminClient.GetBucketQuota(context.Background(), name)
	if err != nil {
		s3Logger.Error(err, "error while getting quota on bucket", "bucket", name)
	}
	return int64(bucketQuota.Quota), err
}

func (minioS3Client *MinioS3Client) SetQuota(name string, quota int64) error {
	s3Logger := ctrl.Log.WithValues("logger", "s3clientimplminio")
	s3Logger.Info("setting quota on bucket", "bucket", name, "quotaToSet", quota)
	minioS3Client.adminClient.SetBucketQuota(
		context.Background(),
		name,
		&madmin.BucketQuota{Quota: uint64(quota), Type: madmin.HardQuota},
	)
	return nil
}

////////////////////
// Policy methods //
////////////////////

// Note regarding the implementation of policy existence check

// No method exposed by the madmin client is truly satisfying to test the existence of a policy
//   - InfoCannedPolicyV2 returns an error if the policy does not exist (as opposed to BucketExists,
//     for instance, see https://github.com/minio/minio-go/blob/v7.0.52/api-stat.go#L43-L45)
//   - ListCannedPolicyV2 is extremely slow to run when the minio instance holds a large number of policies
//     ( ~10000 => ~50s execution time on a modest staging minio cluster)
//
// For lack of a better solution, we use InfoCannedPolicyV2 and test the error code to identify the
// case of a missing policy (vs a technical, non-recoverable error in contacting the S3 server for instance)
// A consequence is that we do things a little differently compared to buckets - instead of just testing for
// existence, we get the whole policy info, and the controller uses it down the line.
func (minioS3Client *MinioS3Client) GetPolicyInfo(name string) (*madmin.PolicyInfo, error) {
	s3Logger := ctrl.Log.WithValues("logger", "s3clientimplminio")
	s3Logger.Info("retrieving policy info", "policy", name)

	policy, err := minioS3Client.adminClient.InfoCannedPolicyV2(context.Background(), name)

	if err != nil {
		// Not ideal (breaks if error nomenclature changes), but still
		// better than testing the error message as we did before
		// if err.Error() == "The canned policy does not exist. (Specified canned policy does not exist)" {
		if madmin.ToErrorResponse(err).Code == "XMinioAdminNoSuchPolicy" {
			s3Logger.Info("the policy does not exist", "policy", name)
			return nil, nil
		} else {
			s3Logger.Error(err, "an error occurred while checking policy existence", "policy", name)
			return nil, err
		}
	}

	return policy, nil
}

// The AddCannedPolicy of the madmin client actually does both creation and update (so does the CLI, as both
// are wired to the same endpoint on Minio API server).
func (minioS3Client *MinioS3Client) CreateOrUpdatePolicy(name string, content string) error {
	s3Logger := ctrl.Log.WithValues("logger", "s3clientimplminio")
	s3Logger.Info("create or update policy", "policy", name)
	return minioS3Client.adminClient.AddCannedPolicy(context.Background(), name, []byte(content))
}

func (minioS3Client *MinioS3Client) PolicyExist(name string) (bool, error) {
	s3Logger := ctrl.Log.WithValues("logger", "s3clientimplminio")
	s3Logger.Info("checking policy existence", "policy", name)
	policies, err := minioS3Client.adminClient.ListPolicies(context.Background(), name)
	if err != nil {
		return false, err
	}
	filteredPolicies := []string{}
	for i := 0; i < len(policies); i++ {
		if policies[i].Name == name {
			filteredPolicies = append(filteredPolicies, name)
		}
	}
	return len(filteredPolicies) > 0, nil
}

func (minioS3Client *MinioS3Client) DeletePolicy(name string) error {
	s3Logger := ctrl.Log.WithValues("logger", "s3clientimplminio")
	s3Logger.Info("delete policy", "policy", name)
	return minioS3Client.adminClient.RemoveCannedPolicy(context.Background(), name)
}

////////////////////
// USER   methods //
////////////////////

func (minioS3Client *MinioS3Client) CreateUser(accessKey string, secretKey string) error {
	s3Logger := ctrl.Log.WithValues("logger", "s3clientimplminio")
	s3Logger.Info("Creating user", "accessKey", accessKey)
	err := minioS3Client.adminClient.AddUser(context.Background(), accessKey, secretKey)
	if err != nil {
		s3Logger.Error(err, "Error while creating user", "user", accessKey)
		return err
	}
	return nil
}

func (minioS3Client *MinioS3Client) AddServiceAccountForUser(
	name string,
	accessKey string,
	secretKey string,
) error {
	s3Logger := ctrl.Log.WithValues("logger", "s3clientimplminio")
	s3Logger.Info("Adding service account for user", "user", name, "accessKey", accessKey)

	opts := madmin.AddServiceAccountReq{
		AccessKey:   accessKey,
		SecretKey:   secretKey,
		Name:        accessKey,
		Description: "",
		TargetUser:  name,
	}

	_, err := minioS3Client.adminClient.AddServiceAccount(context.Background(), opts)
	if err != nil {
		s3Logger.Error(err, "Error while creating service account for user", "user", name)
		return err
	}

	return nil

}

func (minioS3Client *MinioS3Client) UserExist(accessKey string) (bool, error) {
	s3Logger := ctrl.Log.WithValues("logger", "s3clientimplminio")
	s3Logger.Info("checking user existence", "accessKey", accessKey)
	_, _err := minioS3Client.adminClient.GetUserInfo(context.Background(), accessKey)
	if _err != nil {
		if madmin.ToErrorResponse(_err).Code == "XMinioAdminNoSuchUser" {
			return false, nil
		}
		s3Logger.Error(_err, "an error occurred when checking user's existence")
		return false, _err
	}

	return true, nil
}

func (minioS3Client *MinioS3Client) DeleteUser(accessKey string) error {
	s3Logger := ctrl.Log.WithValues("logger", "s3clientimplminio")
	s3Logger.Info("delete user with accessKey", "accessKey", accessKey)
	err := minioS3Client.adminClient.RemoveUser(context.Background(), accessKey)
	if err != nil {
		if madmin.ToErrorResponse(err).Code == "XMinioAdminNoSuchUser" {
			s3Logger.Info("the user was already deleted from s3 backend")
			return nil
		}
		s3Logger.Error(err, "an error occurred when attempting to delete the user")
		return err
	}
	return nil
}

func (minioS3Client *MinioS3Client) GetUserPolicies(accessKey string) ([]string, error) {
	s3Logger := ctrl.Log.WithValues("logger", "s3clientimplminio")
	s3Logger.Info("Get user policies", "accessKey", accessKey)
	userInfo, err := minioS3Client.adminClient.GetUserInfo(context.Background(), accessKey)
	if err != nil {
		s3Logger.Error(err, "Error when getting userInfo")

		return []string{}, err
	}
	userPolicies := strings.Split(strings.TrimSpace(userInfo.PolicyName), ",")
	if len(userPolicies) == 1 && slices.Contains(userPolicies, "") {
		return []string{}, nil
	}
	return userPolicies, nil
}

func (minioS3Client *MinioS3Client) CheckUserCredentialsValid(
	name string,
	accessKey string,
	secretKey string,
) (bool, error) {
	s3Logger := ctrl.Log.WithValues("logger", "s3clientimplminio")
	s3Logger.Info("Check credentials for user", "user", name, "accessKey", accessKey)
	minioTestClient, err := generateMinioClient(
		minioS3Client.s3Config.Endpoint,
		minioS3Client.s3Config.Secure,
		accessKey,
		secretKey,
		minioS3Client.s3Config.Region,
		minioS3Client.s3Config.CaCertificatesBase64,
	)
	if err != nil {
		s3Logger.Error(err, "An error occurred while creating a new Minio test client")
		return false, err
	}
	_, err = minioTestClient.ListBuckets(context.Background())
	if err != nil {
		errAsResponse := minio.ToErrorResponse(err)
		switch errAsResponse.Code {
		case "SignatureDoesNotMatch":
			s3Logger.Info(
				"the user credentials appear to be invalid",
				"accessKey",
				accessKey,
				"s3BackendError",
				errAsResponse,
			)
			return false, nil
		case "InvalidAccessKeyId":
			s3Logger.Info("this accessKey does not exist on the s3 backend", "accessKey", accessKey, "s3BackendError", errAsResponse)
			return false, nil
		default:
			s3Logger.Error(err, "an error occurred while checking if the S3 user's credentials were valid", "accessKey", accessKey, "code", errAsResponse.Code)
			return false, err
		}
	}
	return true, nil
}

func (minioS3Client *MinioS3Client) RemovePoliciesFromUser(
	accessKey string,
	policies []string,
) error {
	s3Logger := ctrl.Log.WithValues("logger", "s3clientimplminio")
	s3Logger.Info("Removing policies from user", "user", accessKey, "policies", policies)

	opts := madmin.PolicyAssociationReq{
		Policies: policies,
		User:     accessKey,
	}

	_, err := minioS3Client.adminClient.DetachPolicy(context.Background(), opts)

	if err != nil {
		errAsResp := madmin.ToErrorResponse(err)
		if errAsResp.Code == "XMinioAdminPolicyChangeAlreadyApplied" {
			s3Logger.Info("The policy change has no net effect")
			return nil
		}
		s3Logger.Error(
			err,
			"an error occurred when detaching a policy to the user",
			"code",
			errAsResp.Code,
		)
		return err
	}

	return nil
}

func (minioS3Client *MinioS3Client) AddPoliciesToUser(accessKey string, policies []string) error {
	s3Logger := ctrl.Log.WithValues("logger", "s3clientimplminio")
	s3Logger.Info("Adding policies to user", "user", accessKey, "policies", policies)
	opts := madmin.PolicyAssociationReq{
		User:     accessKey,
		Policies: policies,
	}
	_, err := minioS3Client.adminClient.AttachPolicy(context.Background(), opts)
	if err != nil {
		errAsResp := madmin.ToErrorResponse(err)
		if errAsResp.Code == "XMinioAdminPolicyChangeAlreadyApplied" {
			s3Logger.Info("The policy change has no net effect")
			return nil
		}
		s3Logger.Error(
			err,
			"an error occurred when attaching a policy to the user",
			"code",
			errAsResp.Code,
		)
		return err
	}
	return nil
}

func (minioS3Client *MinioS3Client) GetConfig() *s3client.S3Config {
	return &minioS3Client.s3Config
}
