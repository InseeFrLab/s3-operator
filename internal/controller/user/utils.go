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

package user_controller

import (
	"cmp"
	"context"
	"fmt"

	s3v1alpha1 "github.com/InseeFrLab/s3-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func (r *S3UserReconciler) addPoliciesToUser(
	ctx context.Context,
	userResource *s3v1alpha1.S3User,
) error {
	logger := log.FromContext(ctx)
	// Create S3Client
	s3Client, err := r.S3Instancehelper.GetS3ClientForRessource(
		ctx,
		r.Client,
		r.S3factory,
		userResource.Name,
		userResource.Namespace,
		userResource.Spec.S3InstanceRef,
	)
	if err != nil {
		return err
	}
	policies := userResource.Spec.Policies
	if policies != nil {
		err := s3Client.AddPoliciesToUser(userResource.Spec.AccessKey, policies)
		if err != nil {
			logger.Error(
				err,
				"An error occurred while adding policy to user",
				"user",
				userResource.Name,
			)
			return err
		}
	}
	return nil
}

func (r *S3UserReconciler) getUserLinkedSecrets(
	ctx context.Context,
	userResource *s3v1alpha1.S3User,
) ([]corev1.Secret, *corev1.Secret, error) {
	logger := log.FromContext(ctx)

	// Listing every secrets in the S3User's namespace, as a first step
	// to get the actual secret matching the S3User proper.
	// TODO : proper label matching ?
	secretsList := &corev1.SecretList{}

	userOwnedSecretList := []corev1.Secret{}

	err := r.List(ctx, secretsList, client.InNamespace(userResource.Namespace))
	if err != nil {
		logger.Error(err, "An error occurred while listing the secrets in user's namespace")
		return userOwnedSecretList, nil, fmt.Errorf("SecretListingFailed")
	}

	if len(secretsList.Items) == 0 {
		logger.Info("The user's namespace doesn't appear to contain any secret")
		return userOwnedSecretList, nil, nil
	}
	// In all the secrets inside the S3User's namespace, one should have an owner reference
	// pointing to the S3User. For that specific secret, we check if its name matches the one from
	// the S3User, whether explicit (userResource.Spec.SecretName) or implicit (userResource.Name)
	// In case of mismatch, that secret is deleted (and will be recreated) ; if there is a match,
	// it will be used for state comparison.
	// We also check for secret not owned by the resource but with a name matching the configured
	// or default one. If such a secret is found it will be returned separately as it is to be
	// handled differently.
	uid := userResource.GetUID()

	var secretConfiguredName string = userResource.Spec.SecretName
	var secretDefaultName string = userResource.Name
	var notOwnedConfiguredSecret *corev1.Secret
	// cmp.Or takes the first non "zero" value, see https://pkg.go.dev/cmp#Or
	for _, secret := range secretsList.Items {
		for _, ref := range secret.OwnerReferences {
			if ref.UID == uid {
				userOwnedSecretList = append(userOwnedSecretList, secret)
			} else if secret.Name == secretConfiguredName {
				notOwnedConfiguredSecret = &secret
			} else if secret.Name == secretDefaultName && notOwnedConfiguredSecret == nil {
				notOwnedConfiguredSecret = &secret
			}
		}
	}

	return userOwnedSecretList, notOwnedConfiguredSecret, nil
}

func (r *S3UserReconciler) deleteSecret(ctx context.Context, secret *corev1.Secret) error {
	logger := log.FromContext(ctx)
	logger.Info("the secret named " + secret.Name + " will be deleted")
	err := r.Delete(ctx, secret)
	if err != nil {
		logger.Error(err, "An error occurred while deleting a secret")
		return err
	}
	return nil
}

// newSecretForCR returns a secret with the same name/namespace as the CR.
// The secret will include all labels and annotations from the CR.
func (r *S3UserReconciler) newSecretForCR(
	ctx context.Context,
	userResource *s3v1alpha1.S3User,
	data map[string][]byte,
) (*corev1.Secret, error) {
	logger := log.FromContext(ctx)

	// Reusing the S3User's labels and annotations
	labels := map[string]string{}
	labels["app.kubernetes.io/created-by"] = "s3-operator"
	for k, v := range userResource.ObjectMeta.Labels {
		labels[k] = v
	}

	annotations := map[string]string{}
	for k, v := range userResource.ObjectMeta.Annotations {
		annotations[k] = v
	}

	secretName := cmp.Or(userResource.Spec.SecretName, userResource.Name)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        secretName,
			Namespace:   userResource.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Data: data,
		Type: "Opaque",
	}

	// Set S3User instance as the owner and controller
	err := ctrl.SetControllerReference(userResource, secret, r.Scheme)
	if err != nil {
		logger.Error(err, "Could not set owner of kubernetes secret")
		return nil, err
	}

	return secret, nil

}
