package helpers

import (
	"context"
	"fmt"
	"strings"

	s3client "github.com/InseeFrLab/s3-operator/internal/s3/client"
	corev1 "k8s.io/api/core/v1"

	s3v1alpha1 "github.com/InseeFrLab/s3-operator/api/v1alpha1"
	s3factory "github.com/InseeFrLab/s3-operator/internal/s3/factory"
	k8sapierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func (s3InstanceHelper *S3InstanceHelper) GetS3ClientForRessource(
	ctx context.Context,
	client client.Client,
	s3factory s3factory.S3Factory,
	ressourceName string,
	ressourceNamespace string,
	ressourceS3InstanceRef string,
) (s3client.S3Client, error) {
	logger := log.FromContext(ctx)
	logger.Info(fmt.Sprintf("Resource refer to s3Instance %s", ressourceS3InstanceRef))
	s3InstanceInfo := s3InstanceHelper.GetS3InstanceRefInfo(ressourceS3InstanceRef, ressourceNamespace)
	s3Instance := &s3v1alpha1.S3Instance{}
	err := client.Get(
		ctx,
		types.NamespacedName{Namespace: s3InstanceInfo.namespace, Name: s3InstanceInfo.name},
		s3Instance,
	)

	if err != nil {
		if k8sapierrors.IsNotFound(err) {
			return nil, fmt.Errorf("S3Instance %s not found", s3InstanceInfo.name)
		}
		return nil, err
	}

	if !s3InstanceHelper.IsAllowedNamespaces(ressourceNamespace, s3Instance) {
		logger.Info(
			fmt.Sprintf(
				"Resource %s try to use s3instance %s in namespace %s but is not allowed",
				ressourceName,
				s3InstanceInfo.name,
				s3InstanceInfo.namespace,
			),
		)
		return nil, fmt.Errorf("S3Instance %s not found", s3InstanceInfo.name)
	}

	if s3Instance.Status.Conditions[0].Reason != s3v1alpha1.Reconciled {
		return nil, fmt.Errorf("S3instance is not in a ready state")
	}

	return s3InstanceHelper.GetS3ClientFromS3Instance(ctx, client, s3factory, s3Instance)
}

func (s3InstanceHelper *S3InstanceHelper) GetS3ClientFromS3Instance(
	ctx context.Context,
	client client.Client,
	s3factory s3factory.S3Factory,
	s3InstanceResource *s3v1alpha1.S3Instance,
) (s3client.S3Client, error) {
	logger := log.FromContext(ctx)

	s3InstanceSecretSecret, err := s3InstanceHelper.getS3InstanceAccessSecret(ctx, client, s3InstanceResource)
	if err != nil {
		logger.Error(
			err,
			"Could not get s3Instance auth secret in namespace",
			"s3InstanceSecretRefName",
			s3InstanceResource.Spec.SecretRef,
			"NamespacedName",
			s3InstanceResource.Namespace,
		)
		return nil, err
	}

	s3InstanceCaCertSecret, err := s3InstanceHelper.getS3InstanceCaCertSecret(ctx, client, s3InstanceResource)
	if err != nil {
		logger.Error(
			err,
			"Could not get s3Instance cert secret in namespace",
			"s3InstanceSecretRefName",
			s3InstanceResource.Spec.SecretRef,
			"NamespacedName",
			s3InstanceResource.Namespace,
		)
		return nil, err
	}

	allowedNamepaces := []string{s3InstanceResource.Namespace}
	if len(s3InstanceResource.Spec.AllowedNamespaces) > 0 {
		allowedNamepaces = s3InstanceResource.Spec.AllowedNamespaces
	}

	s3Config := &s3client.S3Config{
		S3Provider:            s3InstanceResource.Spec.S3Provider,
		AccessKey:             string(s3InstanceSecretSecret.Data["S3_ACCESS_KEY"]),
		SecretKey:             string(s3InstanceSecretSecret.Data["S3_SECRET_KEY"]),
		S3Url:                 s3InstanceResource.Spec.Url,
		Region:                s3InstanceResource.Spec.Region,
		AllowedNamespaces:     allowedNamepaces,
		CaCertificatesBase64:  []string{string(s3InstanceCaCertSecret.Data["ca.crt"])},
		BucketDeletionEnabled: s3InstanceResource.Spec.BucketDeletionEnabled,
		S3UserDeletionEnabled: s3InstanceResource.Spec.S3UserDeletionEnabled,
		PolicyDeletionEnabled: s3InstanceResource.Spec.PolicyDeletionEnabled,
		PathDeletionEnabled:   s3InstanceResource.Spec.PathDeletionEnabled,
	}

	return s3factory.GenerateS3Client(s3Config.S3Provider, s3Config)
}

func (s3InstanceHelper *S3InstanceHelper) getS3InstanceAccessSecret(
	ctx context.Context,
	client client.Client,
	s3InstanceResource *s3v1alpha1.S3Instance,
) (corev1.Secret, error) {
	s3InstanceSecret := &corev1.Secret{}
	err := client.Get(
		ctx,
		types.NamespacedName{
			Namespace: s3InstanceResource.Namespace,
			Name:      s3InstanceResource.Spec.SecretRef,
		},
		s3InstanceSecret,
	)
	if err != nil {
		if k8sapierrors.IsNotFound(err) {
			return *s3InstanceSecret, fmt.Errorf(
				"secret %s not found in namespace %s",
				s3InstanceResource.Spec.SecretRef,
				s3InstanceResource.Namespace,
			)
		}
		return *s3InstanceSecret, err
	}
	return *s3InstanceSecret, nil
}

func (s3InstanceHelper *S3InstanceHelper) getS3InstanceCaCertSecret(
	ctx context.Context,
	client client.Client,
	s3InstanceResource *s3v1alpha1.S3Instance,
) (corev1.Secret, error) {
	logger := log.FromContext(ctx)

	s3InstanceCaCertSecret := &corev1.Secret{}

	if s3InstanceResource.Spec.CaCertSecretRef == "" {
		logger.Info(fmt.Sprintf("No CaCertSecretRef for s3instance %s", s3InstanceResource.Name))
		return *s3InstanceCaCertSecret, nil
	}

	err := client.Get(
		ctx,
		types.NamespacedName{
			Namespace: s3InstanceResource.Namespace,
			Name:      s3InstanceResource.Spec.CaCertSecretRef,
		},
		s3InstanceCaCertSecret,
	)
	if err != nil {
		if k8sapierrors.IsNotFound(err) {
			logger.Info(
				"No Secret %s for s3instance %s",
				s3InstanceResource.Spec.CaCertSecretRef,
				s3InstanceResource.Name,
			)
			return *s3InstanceCaCertSecret, fmt.Errorf(
				"secret %s not found in namespace %s",
				s3InstanceResource.Spec.CaCertSecretRef,
				s3InstanceResource.Namespace,
			)
		}
		return *s3InstanceCaCertSecret, err
	}
	return *s3InstanceCaCertSecret, nil
}

func (s3InstanceHelper *S3InstanceHelper) GetS3InstanceRefInfo(ressourceS3InstanceRef string, ressourceNamespace string) S3InstanceInfo {
	if strings.Contains(ressourceS3InstanceRef, "/") {
		result := strings.Split(ressourceS3InstanceRef, "/")
		return S3InstanceInfo{name: result[1], namespace: result[0]}
	}
	return S3InstanceInfo{name: ressourceS3InstanceRef, namespace: ressourceNamespace}
}

func (s3InstanceHelper *S3InstanceHelper) IsAllowedNamespaces(namespace string, s3Instance *s3v1alpha1.S3Instance) bool {
	if len(s3Instance.Spec.AllowedNamespaces) > 0 {
		for _, allowedNamespace := range s3Instance.Spec.AllowedNamespaces {
			if strings.HasPrefix(allowedNamespace, "*") &&
				strings.HasSuffix(allowedNamespace, "*") && strings.Contains(
				namespace,
				strings.TrimSuffix(strings.TrimPrefix(allowedNamespace, "*"), "*"),
			) {
				return true
			} else if strings.HasPrefix(allowedNamespace, "*") && strings.HasSuffix(namespace, strings.TrimPrefix(allowedNamespace, "*")) {
				return true
			} else if strings.HasSuffix(allowedNamespace, "*") && strings.HasPrefix(namespace, strings.TrimSuffix(allowedNamespace, "*")) {
				return true
			} else if namespace == allowedNamespace {
				return true
			}
		}
		return false
	} else {
		return namespace == s3Instance.Namespace
	}
}

type S3InstanceInfo struct {
	name      string
	namespace string
}

func (s3InstanceInfo S3InstanceInfo) String() string {
	return fmt.Sprintf("%s/%s", s3InstanceInfo.namespace, s3InstanceInfo.name)
}

type S3InstanceHelper struct {
}

func NewS3InstanceHelper() *S3InstanceHelper {
	return &S3InstanceHelper{}
}
