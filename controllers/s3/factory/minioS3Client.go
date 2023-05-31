package factory

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/minio/madmin-go/v2"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type MinioS3Client struct {
	s3Config    S3Config
	client      minio.Client
	adminClient madmin.AdminClient
}

func newMinioS3Client(S3Config *S3Config) *MinioS3Client {
	log.Println("create minio clients")

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
	if len(S3Config.CaCertificatesBase64) > 0 {

		rootCAs, _ := x509.SystemCertPool()
		if rootCAs == nil {
			rootCAs = x509.NewCertPool()
		}

		// Appending content directly, from a base64-encoded, PEM format CA certificate
		for _, caCertificateBase64 := range S3Config.CaCertificatesBase64 {
			decodedCaCertificate, err := base64.StdEncoding.DecodeString(caCertificateBase64)
			if err != nil {
				log.Fatal("error:", err)
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
			log.Fatalf("Error opening CA cert file %s, Error: %s", S3Config.CaBundlePath, err)
		}
		rootCAs.AppendCertsFromPEM([]byte(caCert))

		// Variant : if S3Config.CaBundlePath was a string[]
		// for _, caCertificateFilePath := range S3Config.S3Config.CaBundlePaths {
		// 	caCert, err := os.ReadFile(caCertificateFilePath)
		// 	if err != nil {
		// 		log.Fatalf("Error opening CA cert file %s, Error: %s", caCertificateFilePath, err)
		// 	}
		// 	rootCAs.AppendCertsFromPEM([]byte(caCert))
		// }

		minioOptions.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: rootCAs,
			},
		}
	}

	minioClient, err := minio.New(S3Config.S3UrlEndpoint, minioOptions)
	if err != nil {
		log.Fatalln(err)
	}

	adminClient, err := madmin.New(S3Config.S3UrlEndpoint, S3Config.AccessKey, S3Config.SecretKey, S3Config.UseSsl)
	if err != nil {
		log.Fatalln(err)
	}
	// Getting the custom root CA (if any) from the "regular" client's Transport
	adminClient.SetCustomTransport(minioOptions.Transport)

	return &MinioS3Client{*S3Config, *minioClient, *adminClient}
}

// //////////////////
// Bucket methods //
// //////////////////
func (minioS3Client *MinioS3Client) BucketExists(name string) (bool, error) {
	log.Println("check if bucket " + name + " exists")
	return minioS3Client.client.BucketExists(context.Background(), name)
}

func (minioS3Client *MinioS3Client) CreateBucket(name string) error {
	log.Println("create bucket " + name)
	return minioS3Client.client.MakeBucket(context.Background(), name, minio.MakeBucketOptions{Region: minioS3Client.s3Config.Region})
}

func (minioS3Client *MinioS3Client) DeleteBucket(name string) error {
	log.Println("delete bucket " + name)
	return minioS3Client.client.RemoveBucket(context.Background(), name)
}

func (minioS3Client *MinioS3Client) CreatePath(bucketname string, name string) error {
	log.Println("create path " + name + " in bucket" + bucketname)
	emptyReader := bytes.NewReader([]byte(""))
	_, err := minioS3Client.client.PutObject(context.Background(), bucketname, "/"+name+"/"+".keep", emptyReader, 0, minio.PutObjectOptions{})
	if err != nil {
		fmt.Println(err)
		return fmt.Errorf("error on path creation" + bucketname + " " + name)
	}
	return nil
}

func (minioS3Client *MinioS3Client) PathExists(bucketname string, name string) (bool, error) {
	log.Println("check if path " + name + " exists in bucket " + bucketname)
	_, err := minioS3Client.client.
		StatObject(context.Background(),
			bucketname,
			"/"+name+"/"+".keep",
			minio.StatObjectOptions{})

	if err != nil {
		if minio.ToErrorResponse(err).StatusCode == 404 {
			fmt.Println("The path does not exist")
			return false, nil
		} else {
			// There was an error.
			return false, err
		}
	}

	return true, nil
}

// /////////////////
// Quota methods //
// /////////////////
func (minioS3Client *MinioS3Client) GetQuota(name string) (int64, error) {
	log.Println("bucket " + name + " get quota")
	bucketQuota, err := minioS3Client.adminClient.GetBucketQuota(context.Background(), name)
	if err != nil {
		log.Fatalln(err)
	}
	return int64(bucketQuota.Quota), err
}

func (minioS3Client *MinioS3Client) SetQuota(name string, quota int64) error {
	log.Println("set quota " + fmt.Sprint(quota) + " on bucket " + name)
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
	log.Println("check if policy " + name + " exists")

	policy, err := minioS3Client.adminClient.InfoCannedPolicyV2(context.Background(), name)

	if err != nil {
		// Not ideal (breaks if error nomenclature changes), but still
		// better than testing the error message as we did before
		// if err.Error() == "The canned policy does not exist. (Specified canned policy does not exist)" {
		if madmin.ToErrorResponse(err).Code == "XMinioAdminNoSuchPolicy" {
			log.Println("policy " + name + " does not exist")
			return nil, nil
		} else {
			log.Println("an error has occurred while checking for policy existence")
			return nil, err
		}
	}

	return policy, nil
}

// The AddCannedPolicy of the madmin client actually does both creation and update (so does the CLI, as both
// are wired to the same endpoint on Minio API server).
func (minioS3Client *MinioS3Client) CreateOrUpdatePolicy(name string, content string) error {
	log.Println("create or update policy " + name)
	return minioS3Client.adminClient.AddCannedPolicy(context.Background(), name, []byte(content))
}
