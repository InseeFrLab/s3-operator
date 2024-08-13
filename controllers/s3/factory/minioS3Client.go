package factory

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"net/http"
	"os"
	"strings"

	"github.com/minio/madmin-go/v3"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type MinioS3Client struct {
	s3Config    S3Config
	client      minio.Client
	adminClient madmin.AdminClient
}

func newMinioS3Client(S3Config *S3Config) *MinioS3Client {
	s3Logger.Info("creating minio clients (regular and admin)")

	minioOptions := &minio.Options{
		Creds:  credentials.NewStaticV4(S3Config.AccessKey, S3Config.SecretKey, ""),
		Region: S3Config.Region,
		Secure: S3Config.UseSsl,
	}

	// Preparing the tlsConfig to support custom CA if configured
	// See also :
	// - https://pkg.go.dev/github.com/minio/minio-go/v7@v7.0.52#Options
	// - https://pkg.go.dev/net/http#RoundTripper
	// - https://youngkin.github.io/post/gohttpsclientserver/#create-the-client
	// - https://forfuncsake.github.io/post/2017/08/trust-extra-ca-cert-in-go-app/
	// Appending content directly, from a base64-encoded, PEM format CA certificate
	// Variant : if S3Config.CaBundlePath was a string[]
	// for _, caCertificateFilePath := range S3Config.S3Config.CaBundlePaths {
	// 	caCert, err := os.ReadFile(caCertificateFilePath)
	// 	if err != nil {
	// 		log.Fatalf("Error opening CA cert file %s, Error: %s", caCertificateFilePath, err)
	// 	}
	// 	rootCAs.AppendCertsFromPEM([]byte(caCert))
	// }
	addTransportOptions(S3Config, minioOptions)

	minioClient, err := minio.New(S3Config.S3UrlEndpoint, minioOptions)
	if err != nil {
		s3Logger.Error(err, "an error occurred while creating a new minio client")
	}

	adminClient, err := madmin.New(S3Config.S3UrlEndpoint, S3Config.AccessKey, S3Config.SecretKey, S3Config.UseSsl)
	if err != nil {
		s3Logger.Error(err, "an error occurred while creating a new minio admin client")
	}
	// Getting the custom root CA (if any) from the "regular" client's Transport
	adminClient.SetCustomTransport(minioOptions.Transport)

	return &MinioS3Client{*S3Config, *minioClient, *adminClient}
}

func addTransportOptions(S3Config *S3Config, minioOptions *minio.Options) {
	if len(S3Config.CaCertificatesBase64) > 0 {

		rootCAs, _ := x509.SystemCertPool()
		if rootCAs == nil {
			rootCAs = x509.NewCertPool()
		}

		for _, caCertificateBase64 := range S3Config.CaCertificatesBase64 {
			decodedCaCertificate, err := base64.StdEncoding.DecodeString(caCertificateBase64)
			if err != nil {
				s3Logger.Error(err, "an error occurred while parsing a base64-encoded CA certificate")
			}

			rootCAs.AppendCertsFromPEM(decodedCaCertificate)
		}

		minioOptions.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: rootCAs,
			},
		}
	} else if len(S3Config.CaBundlePath) > 0 {

		rootCAs, _ := x509.SystemCertPool()
		if rootCAs == nil {
			rootCAs = x509.NewCertPool()
		}

		caCert, err := os.ReadFile(S3Config.CaBundlePath)
		if err != nil {
			s3Logger.Error(err, "an error occurred while reading a CA certificates bundle file")
		}
		rootCAs.AppendCertsFromPEM([]byte(caCert))

		minioOptions.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: rootCAs,
			},
		}
	}
}

// //////////////////
// Bucket methods //
// //////////////////
func (minioS3Client *MinioS3Client) BucketExists(name string) (bool, error) {
	s3Logger.Info("checking bucket existence", "bucket", name)
	return minioS3Client.client.BucketExists(context.Background(), name)
}

func (minioS3Client *MinioS3Client) CreateBucket(name string) error {
	s3Logger.Info("creating bucket", "bucket", name)
	return minioS3Client.client.MakeBucket(context.Background(), name, minio.MakeBucketOptions{Region: minioS3Client.s3Config.Region})
}

// Will fail if bucket is not empty
func (minioS3Client *MinioS3Client) DeleteBucket(name string) error {
	s3Logger.Info("deleting bucket", "bucket", name)
	return minioS3Client.client.RemoveBucket(context.Background(), name)
}

func (minioS3Client *MinioS3Client) CreatePath(bucketname string, path string) error {
	s3Logger.Info("creating a path on a bucket", "bucket", bucketname, "path", path)
	emptyReader := bytes.NewReader([]byte(""))
	_, err := minioS3Client.client.PutObject(context.Background(), bucketname, "/"+path+"/"+".keep", emptyReader, 0, minio.PutObjectOptions{})
	if err != nil {
		s3Logger.Error(err, "an error occurred during path creation on bucket", "bucket", bucketname, "path", path)
		return err
	}
	return nil
}

func (minioS3Client *MinioS3Client) PathExists(bucketname string, path string) (bool, error) {
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
	s3Logger.Info("deleting a path on a bucket", "bucket", bucketname, "path", path)
	err := minioS3Client.client.RemoveObject(context.Background(), bucketname, "/"+path+"/.keep", minio.RemoveObjectOptions{})
	if err != nil {
		s3Logger.Error(err, "an error occurred during path deletion on bucket", "bucket", bucketname, "path", path)
		return err
	}
	return nil
}

// /////////////////
// Quota methods //
// /////////////////
func (minioS3Client *MinioS3Client) GetQuota(name string) (int64, error) {
	s3Logger.Info("getting quota on bucket", "bucket", name)
	bucketQuota, err := minioS3Client.adminClient.GetBucketQuota(context.Background(), name)
	if err != nil {
		s3Logger.Error(err, "error while getting quota on bucket", "bucket", name)
	}
	return int64(bucketQuota.Quota), err
}

func (minioS3Client *MinioS3Client) SetQuota(name string, quota int64) error {
	s3Logger.Info("setting quota on bucket", "bucket", name, "quotaToSet", quota)
	minioS3Client.adminClient.SetBucketQuota(context.Background(), name, &madmin.BucketQuota{Quota: uint64(quota), Type: madmin.HardQuota})
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
	s3Logger.Info("create or update policy", "policy", name)
	return minioS3Client.adminClient.AddCannedPolicy(context.Background(), name, []byte(content))
}

func (minioS3Client *MinioS3Client) PolicyExist(name string) (bool, error) {
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
	s3Logger.Info("delete policy", "policy", name)
	return minioS3Client.adminClient.RemoveCannedPolicy(context.Background(), name)
}

////////////////////
// USER   methods //
////////////////////

func (minioS3Client *MinioS3Client) CreateUser(accessKey string, secretKey string) error {
	s3Logger.Info("Creating user", "accessKey", accessKey)
	err := minioS3Client.adminClient.AddUser(context.Background(), accessKey, secretKey)
	if err != nil {
		s3Logger.Error(err, "Error while creating user", "user", accessKey)
		return err
	}
	return nil
}

func (minioS3Client *MinioS3Client) AddServiceAccountForUser(name string, accessKey string, secretKey string) error {
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
	s3Logger.Info("Get user policies", "accessKey", accessKey)
	userInfo, err := minioS3Client.adminClient.GetUserInfo(context.Background(), accessKey)
	if err != nil {
		s3Logger.Error(err, "Error when getting userInfo")

		return []string{}, err
	}
	return strings.Split(userInfo.PolicyName, ","), nil
}

func (minioS3Client *MinioS3Client) CheckUserCredentialsValid(name string, accessKey string, secretKey string) (bool, error) {
	s3Logger.Info("Check credentials for user", "user", name, "accessKey", accessKey)
	minioTestClientOptions := &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Region: minioS3Client.s3Config.Region,
		Secure: minioS3Client.s3Config.UseSsl,
	}
	addTransportOptions(&minioS3Client.s3Config, minioTestClientOptions)
	minioTestClient, err := minio.New(minioS3Client.s3Config.S3UrlEndpoint, minioTestClientOptions)
	if err != nil {
		s3Logger.Error(err, "An error occurred while creating a new Minio test client")
	}

	_, err = minioTestClient.ListBuckets(context.Background())
	if err != nil {
		errAsResponse := minio.ToErrorResponse(err)
		if errAsResponse.Code == "SignatureDoesNotMatch" {
			s3Logger.Info("the user credentials appear to be invalid", "accessKey", accessKey, "s3BackendError", errAsResponse)
			return false, nil
		} else if errAsResponse.Code == "InvalidAccessKeyId" {
			s3Logger.Info("this accessKey does not exist on the s3 backend", "accessKey", accessKey, "s3BackendError", errAsResponse)
			return false, nil
		} else {
			s3Logger.Error(err, "an error occurred while checking if the S3 user's credentials were valid", "accessKey", accessKey, "code", errAsResponse.Code)
			return false, err
		}
	}
	return true, nil
}

func (minioS3Client *MinioS3Client) RemovePoliciesFromUser(accessKey string, policies []string) error {
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
		s3Logger.Error(err, "an error occurred when attaching a policy to the user", "code", errAsResp.Code)
		return err
	}

	return nil
}

func (minioS3Client *MinioS3Client) AddPoliciesToUser(accessKey string, policies []string) error {
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
		s3Logger.Error(err, "an error occurred when attaching a policy to the user", "code", errAsResp.Code)
		return err
	}
	return nil
}
