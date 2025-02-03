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

	corev1 "k8s.io/api/core/v1"
	k8sapierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"

	s3v1alpha1 "github.com/InseeFrLab/s3-operator/api/v1alpha1"
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

func (r *S3UserReconciler) getUserSecret(
	ctx context.Context,
	userResource *s3v1alpha1.S3User,
) (corev1.Secret, error) {
	userSecret := &corev1.Secret{}
	secretName := userResource.Spec.SecretName
	if secretName == "" {
		secretName = userResource.Name
	}
	err := r.Get(
		ctx,
		types.NamespacedName{Namespace: userResource.Namespace, Name: secretName},
		userSecret,
	)
	if err != nil {
		if k8sapierrors.IsNotFound(err) {
			return *userSecret, fmt.Errorf(
				"secret %s not found in namespace %s",
				userResource.Spec.SecretName,
				userResource.Namespace,
			)
		}
		return *userSecret, err
	}

	for _, ref := range userSecret.OwnerReferences {
		if ref.UID == userResource.GetUID() {
			return *userSecret, nil
		}
	}

	return *userSecret, err
}

func (r *S3UserReconciler) deleteSecret(ctx context.Context, secret *corev1.Secret) error {
	logger := log.FromContext(ctx)
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
