package model

import (
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	"reflect"
	"slices"
	"strings"
)

const (
	// PrivateBucketAccessPolicy only authenticated users can interact with Bucket
	PrivateBucketAccessPolicy BucketAccessPolicyType = "Private"
	// PublicBucketAccessPolicy all users, included anonymous have access to Bucket
	PublicBucketAccessPolicy BucketAccessPolicyType = "Public"
	// CustomBucketAccessPolicy custom policy
	CustomBucketAccessPolicy BucketAccessPolicyType = "Custom"
)

var (
	// ErrUnknownBucketAccessPolicyType is raised when the given policy type doesn't exist or is not implemented
	ErrUnknownBucketAccessPolicyType = "unknown bucket access policy type"
	// ErrInvalidBucketAccessPolicy is raised when the given policy can't be parsed
	ErrInvalidBucketAccessPolicy = "invalid bucket access policy"
	// ErrLoadingBucketAccessPolicy is raised when the load of the current policy failed
	ErrLoadingBucketAccessPolicy = "failed to load the current policy"
)

// BucketAccessPolicyType represents the type
// of the BucketAccessPolicy
type BucketAccessPolicyType string

// BucketAccessPolicy represents the BucketAccessPolicy
type BucketAccessPolicy struct {
	Type    BucketAccessPolicyType
	Content BucketAccessPolicyContent
}

// BucketAccessPolicyContent wrap an IAM object represents
// the policy content
type BucketAccessPolicyContent struct {
	IAM
}

// NewBucketAccessPolicy return an already defined policy for PRIVATE and PUBLIC type and a CUSTOM for custom policy
func NewBucketAccessPolicy(bucketName string, policyType BucketAccessPolicyType, policy string) (*BucketAccessPolicy, error) {
	var (
		accessPolicy *BucketAccessPolicy
		err          error
	)

	switch policyType {
	case PrivateBucketAccessPolicy:
		accessPolicy = privateBucketAccessPolicy()
	case PublicBucketAccessPolicy:
		accessPolicy = publicBucketAccessPolicy(bucketName)
	case CustomBucketAccessPolicy:
		if policy == "" {
			return nil, fmt.Errorf(ErrInvalidBucketAccessPolicy)
		}

		accessPolicy, err = customBucketAccessPolicy(policy)
		if err != nil {
			return accessPolicy, nil
		}
	default:
		return accessPolicy, errors.New(ErrUnknownBucketAccessPolicyType)
	}

	return accessPolicy, nil
}

// LoadBucketAccessPolicy construct the BucketAccessPolicy object from policy given as string
func LoadBucketAccessPolicy(bucketName string, policy string) (*BucketAccessPolicy, error) {
	accessPolicy := &BucketAccessPolicy{}

	// when a bucket is created is by default Private
	// without any policy set
	if policy == "" {
		accessPolicy.Type = PrivateBucketAccessPolicy
		return accessPolicy, nil
	}

	err := json.Unmarshal([]byte(policy), &accessPolicy.Content)
	if err != nil {
		return nil, errors.Wrap(err, ErrLoadingBucketAccessPolicy)
	}

	accessPolicy.Type = findAccessBucketPolicyTypeFromContent(bucketName, accessPolicy.Content)

	return accessPolicy, nil
}

// JSON convert BucketAccessPolicyContent to JSON
func (b BucketAccessPolicyContent) JSON() ([]byte, error) {
	return json.Marshal(b)
}

func (b *BucketAccessPolicy) Sort() {
	for _, stmt := range b.Content.Statement {
		for _, principal := range stmt.Principal {
			slices.Sort(principal)
		}

		slices.Sort(stmt.Resource)
		slices.Sort(stmt.Action)
	}
}

func (b *BucketAccessPolicy) Equal(compare *BucketAccessPolicy) bool {
	if b == nil && compare == nil {
		return true
	}

	if b == nil || compare == nil {
		return false
	}

	// check type
	if b.Type != compare.Type {
		return false
	}

	b.Sort()
	compare.Sort()

	return reflect.DeepEqual(compare.Content, b.Content)
}

func (b BucketAccessPolicyContent) isPublic(bucket string) bool {
	for _, stmt := range b.Statement {
		for _, principal := range stmt.Principal {
			slices.Sort(principal)
		}

		slices.Sort(stmt.Resource)
		slices.Sort(stmt.Action)
	}

	return reflect.DeepEqual(b, publicBucketAccessPolicy(bucket).Content)
}

func (b BucketAccessPolicyContent) isPrivate() bool {
	return len(b.Statement) == 0
}

func parseCustomPolicy(policy string) (BucketAccessPolicyContent, error) {
	var accessPolicyContent BucketAccessPolicyContent

	decoder := json.NewDecoder(strings.NewReader(policy))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&accessPolicyContent); err != nil {
		return accessPolicyContent, errors.Wrap(err, ErrInvalidBucketAccessPolicy)
	}
	return accessPolicyContent, nil
}

func findAccessBucketPolicyTypeFromContent(bucket string, policyContent BucketAccessPolicyContent) BucketAccessPolicyType {
	if policyContent.isPrivate() {
		return PrivateBucketAccessPolicy
	}

	if policyContent.isPublic(bucket) {
		return PublicBucketAccessPolicy
	}

	return CustomBucketAccessPolicy
}

func customBucketAccessPolicy(policy string) (*BucketAccessPolicy, error) {
	bucketAccessPolicy := BucketAccessPolicy{
		Type: CustomBucketAccessPolicy,
	}

	decoder := json.NewDecoder(strings.NewReader(policy))
	decoder.DisallowUnknownFields()
	bucketAcessPolicyContent, err := parseCustomPolicy(policy)
	if err != nil {
		return nil, err
	}

	bucketAccessPolicy.Content = bucketAcessPolicyContent
	return &bucketAccessPolicy, nil
}

func publicBucketAccessPolicy(bucket string) *BucketAccessPolicy {
	return &BucketAccessPolicy{
		Type: PublicBucketAccessPolicy,
		Content: BucketAccessPolicyContent{
			IAM: IAM{
				Version: "2012-10-17",
				Statement: []IAMStatement{
					{
						Effect: "Allow",
						Principal: map[string][]string{
							"AWS": []string{"*"},
						},
						Action: []string{
							"s3:GetBucketLocation",
							"s3:ListBucket",
							"s3:ListBucketMultipartUploads",
						},
						Resource: []string{
							fmt.Sprintf("arn:aws:s3:::%s", bucket),
						},
					},
					{
						Effect: "Allow",
						Principal: map[string][]string{
							"AWS": []string{"*"},
						},
						Action: []string{
							"s3:AbortMultipartUpload",
							"s3:DeleteObject",
							"s3:GetObject",
							"s3:ListMultipartUploadParts",
							"s3:PutObject",
						},
						Resource: []string{
							fmt.Sprintf("arn:aws:s3:::%s/*", bucket),
						},
					},
				},
			},
		},
	}
}

func privateBucketAccessPolicy() *BucketAccessPolicy {
	return &BucketAccessPolicy{
		Type: PrivateBucketAccessPolicy,
		Content: BucketAccessPolicyContent{
			IAM: IAM{
				Version:   "2012-10-17",
				Statement: []IAMStatement{},
			},
		},
	}
}
