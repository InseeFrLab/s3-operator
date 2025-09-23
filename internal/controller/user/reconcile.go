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
	"strings"
	"slices"

	corev1 "k8s.io/api/core/v1"
	k8sapierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	s3v1alpha1 "github.com/InseeFrLab/s3-operator/api/v1alpha1"
)

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *S3UserReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Checking for userResource existence
	userResource := &s3v1alpha1.S3User{}
	err := r.Get(ctx, req.NamespacedName, userResource)
	if err != nil {
		if k8sapierrors.IsNotFound(err) {
			logger.Info(
				fmt.Sprintf(
					"The S3User CR %s (or its owned Secret) has been removed. NOOP",
					req.Name,
				),
			)
			return ctrl.Result{}, nil
		}
		logger.Error(err, "An error occurred when fetching the S3User from Kubernetes")
		return ctrl.Result{}, err
	}

	// Let's just set the status as Unknown when no status are available
	if len(userResource.Status.Conditions) == 0 {
		_, err = r.SetProgressingCondition(ctx,
						   req,
						   userResource,
						   metav1.ConditionUnknown,
						   s3v1alpha1.Reconciling,
						   fmt.Sprintf("Newly discovered resource %s", userResource.Name))
		if err != nil {
			return ctrl.Result{}, err
		}

		// Let's re-fetch the userResource Custom Resource after update the status
		// so that we have the latest state of the resource on the cluster and we will avoid
		// raise the issue "the object has been modified, please apply
		// your changes to the latest version and try again" which would re-trigger the reconciliation
		// if we try to update it again in the following operations
		if err := r.Get(ctx, req.NamespacedName, userResource); err != nil {
			r.SetDegradedCondition(ctx,
					       req,
					       userResource,
					       metav1.ConditionFalse,
					       s3v1alpha1.K8sApiError,
					       fmt.Sprintf("Failed to re-fetch user resource %s", userResource.Name),
					       err)
			return ctrl.Result{}, err
		}
	}

	// Add finalizer for this CR
	if !controllerutil.ContainsFinalizer(userResource, userFinalizer) {
		r.SetProgressingCondition(ctx,
					  req,
					  userResource,
					  metav1.ConditionTrue,
					  s3v1alpha1.Reconciling,
					  fmt.Sprintf("Adding finalizer to user resource %s", userResource.Name))

		if ok := controllerutil.AddFinalizer(userResource, userFinalizer); !ok {
			r.SetDegradedCondition(ctx,
					       req,
					       userResource,
					       metav1.ConditionFalse,
					       s3v1alpha1.InternalError,
					       fmt.Sprintf("Failed to add finalizer to user resource %s", userResource.Name),
					       err)
			return ctrl.Result{Requeue: true}, nil
		}

		if err = r.Update(ctx, userResource); err != nil {
			r.SetDegradedCondition(ctx,
					       req,
					       userResource,
					       metav1.ConditionFalse,
					       s3v1alpha1.K8sApiError,
					       fmt.Sprintf("An error occurred when adding finalizer on user resource %s", userResource.Name),
					       err)
			return ctrl.Result{}, err
		}

		// Let's re-fetch the userResource Custom Resource after adding the finalizer
		// so that we have the latest state of the resource on the cluster and we will avoid
		// raise the issue "the object has been modified, please apply
		// your changes to the latest version and try again" which would re-trigger the reconciliation
		// if we try to update it again in the following operations
		if err := r.Get(ctx, req.NamespacedName, userResource); err != nil {
			r.SetDegradedCondition(ctx,
					       req,
					       userResource,
					       metav1.ConditionFalse,
					       s3v1alpha1.K8sApiError,
					       fmt.Sprintf("Failed to re-fetch user resource %s", userResource.Name),
					       err)
			return ctrl.Result{}, err
		}
	}

	// Check if the userResource instance is marked to be deleted, which is
	// indicated by the deletion timestamp being set. The object will be deleted.
	if userResource.GetDeletionTimestamp() != nil {
		r.SetProgressingCondition(ctx,
					  req,
					  userResource,
					  metav1.ConditionTrue,
					  s3v1alpha1.Reconciling,
					  fmt.Sprintf("User resource have been marked for deletion %s", userResource.Name))
		return r.handleDeletion(ctx, req, userResource)
	}

	r.SetProgressingCondition(ctx, req, userResource, metav1.ConditionTrue, s3v1alpha1.Reconciling, fmt.Sprintf("Starting reconciliation of user %s", userResource.Name))
	return r.handleReconciliation(ctx, req, userResource)

}

func (r *S3UserReconciler) handleReconciliation(
	ctx context.Context,
	req reconcile.Request,
	userResource *s3v1alpha1.S3User,
) (reconcile.Result, error) {

	s3Client, err := r.S3Instancehelper.GetS3ClientForRessource(
		ctx,
		r.Client,
		r.S3factory,
		userResource.Name,
		userResource.Namespace,
		userResource.Spec.S3InstanceRef,
	)
	if err != nil {
		return r.SetDegradedCondition(
			ctx,
			req,
			userResource,
			metav1.ConditionUnknown,
			s3v1alpha1.Unreachable,
			"Failed to generate s3client from instance",
			err,
		)
	}

	found, err := s3Client.UserExist(userResource.Spec.AccessKey)
	if err != nil {
		return r.SetDegradedCondition(
			ctx,
			req,
			userResource,
			metav1.ConditionUnknown,
			s3v1alpha1.Unreachable,
			fmt.Sprintf("Error while checking if user %s already exist", userResource.Name),
			err,
		)
	}

	if !found {
		return r.handleCreate(ctx, req, userResource)
	}
	return r.handleUpdate(ctx, req, userResource)
}

func (r *S3UserReconciler) handleUpdate(
	ctx context.Context,
	req reconcile.Request,
	userResource *s3v1alpha1.S3User,
) (reconcile.Result, error) {

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
		return r.SetDegradedCondition(
			ctx,
			req,
			userResource,
			metav1.ConditionUnknown,
			s3v1alpha1.Unreachable,
			"Failed to generate s3client from instance",
			err,
		)
	}
	ownedSecret := true
	userOwnedlinkedSecrets, userUnlinkedSecret, err := r.getUserLinkedSecrets(ctx, userResource)
	if err != nil {
		return r.SetDegradedCondition(
			ctx,
			req,
			userResource,
			metav1.ConditionUnknown,
			s3v1alpha1.Unreachable,
			fmt.Sprintf("Impossible to list the secrets of user %s", userResource.Name),
			err,
		)
	}
	currentUserSecret := corev1.Secret{}
	if len(userOwnedlinkedSecrets) == 0 && userUnlinkedSecret == nil {
		r.SetProgressingCondition(ctx,
					  req,
					  userResource,
					  metav1.ConditionTrue,
					  s3v1alpha1.Reconciling,
					  fmt.Sprintf("No Secret associated to user %s found, user will be deleted from the S3 backend, then recreated with a secret", userResource.Name))

		err = s3Client.DeleteUser(userResource.Spec.AccessKey)
		if err != nil {
			return r.SetDegradedCondition(
				ctx,
				req,
				userResource,
				metav1.ConditionFalse,
				s3v1alpha1.Unreachable,
				fmt.Sprintf(
					"Deletion of S3user %s on S3 server has failed",
					userResource.Name,
				),
				err,
			)
		}
		return r.handleCreate(ctx, req, userResource)
	} else if userUnlinkedSecret != nil {
		currentUserSecret = *userUnlinkedSecret
		ownedSecret = false
	} else {
		foundSecret := false
		for _, linkedsecret := range userOwnedlinkedSecrets {
			if linkedsecret.Name != cmp.Or(userResource.Spec.SecretName, userResource.Name) {
				if err := r.deleteSecret(ctx, &linkedsecret); err != nil {
					return r.SetDegradedCondition(
						ctx,
						req,
						userResource,
						metav1.ConditionFalse,
						s3v1alpha1.Unreachable,
						fmt.Sprintf("Failed to delete old linkedSecret %s for user %s", linkedsecret.Name, userResource.Name),
						err,
					)
				}
			} else {
				foundSecret = true
				currentUserSecret = linkedsecret
			}
		}
		if !foundSecret {
			r.SetProgressingCondition(ctx,
					  req,
					  userResource,
					  metav1.ConditionTrue,
					  s3v1alpha1.Reconciling,
					  "No Secret associated to user found, user will be deleted from the S3 backend, then recreated with a secret")


			err = s3Client.DeleteUser(userResource.Spec.AccessKey)
			if err != nil {
				return r.SetDegradedCondition(
					ctx,
					req,
					userResource,
					metav1.ConditionFalse,
					s3v1alpha1.Unreachable,
					fmt.Sprintf(
						"Deletion of S3user %s on S3 server has failed",
						userResource.Name,
					),
					err,
				)
			}
			return r.handleCreate(ctx, req, userResource)
		}
	}
	r.SetProgressingCondition(ctx,
			  req,
			  userResource,
			  metav1.ConditionTrue,
			  s3v1alpha1.Reconciling,
			  fmt.Sprintf("Checking user %s policies", userResource.Name))


	userPolicies, err := s3Client.GetUserPolicies(userResource.Spec.AccessKey)
	if err != nil {
		return r.SetDegradedCondition(
			ctx,
			req,
			userResource,
			metav1.ConditionUnknown,
			s3v1alpha1.Unreachable,
			fmt.Sprintf("Checking the S3user policies has failed for user %s", userResource.Name),
			err,
		)
	}

	policyToDelete := []string{}
	policyToAdd := []string{}
	for _, policy := range userPolicies {
		policyFound := slices.Contains(userResource.Spec.Policies, policy)
		if !policyFound {
			policyToDelete = append(policyToDelete, policy)
		}
	}

	for _, policy := range userResource.Spec.Policies {
		policyFound := slices.Contains(userPolicies, policy)
		if !policyFound {
			policyToAdd = append(policyToAdd, policy)
		}
	}

	if len(policyToDelete) > 0 || len(policyToAdd) > 0 {
		var message string = ""
		if len(policyToDelete) > 0 {
			message = fmt.Sprintf("S3User %s has unexpected policy not in definition: %s", userResource.Name, strings.Join(policyToDelete, ", "))
		}
		if len(policyToAdd) > 0 {
			message += fmt.Sprintf("S3User %s is missing policy from definition: %s", userResource.Name, strings.Join(policyToAdd, ", "))
		}
		// Update the condition (+ prints a log) before doing anything
		r.SetProgressingCondition(ctx,
			  req,
			  userResource,
			  metav1.ConditionTrue,
			  s3v1alpha1.Reconciling,
			  message)

		if len(policyToDelete) > 0 {
			err = s3Client.RemovePoliciesFromUser(userResource.Spec.AccessKey, policyToDelete)
			if err != nil {
				return r.SetDegradedCondition(
					ctx,
					req,
					userResource,
					metav1.ConditionFalse,
					s3v1alpha1.Unreachable,
					fmt.Sprintf("Error while removing policies of user %s", userResource.Name),
					err,
				)
			}
		}

		if len(policyToAdd) > 0 {
			err := s3Client.AddPoliciesToUser(userResource.Spec.AccessKey, policyToAdd)
			if err != nil {
				return r.SetDegradedCondition(
					ctx,
					req,
					userResource,
					metav1.ConditionFalse,
					s3v1alpha1.Unreachable,
					fmt.Sprintf("Error while adding policies of user %s", userResource.Name),
					err,
				)

			}
		}
	}

	credentialsValid, err := s3Client.CheckUserCredentialsValid(
		userResource.Name,
		string(currentUserSecret.Data[userResource.Spec.SecretFieldNameAccessKey]),
		string(currentUserSecret.Data[userResource.Spec.SecretFieldNameSecretKey]),
	)

	if err != nil {
		return r.SetDegradedCondition(
			ctx,
			req,
			userResource,
			metav1.ConditionFalse,
			s3v1alpha1.Unreachable,
			fmt.Sprintf("Checking credentials on S3 server has failed for user %s", userResource.Name),
			err,
		)
	}

	if !credentialsValid {
		if ownedSecret {
			r.SetProgressingCondition(ctx,
			  req,
			  userResource,
			  metav1.ConditionTrue,
			  s3v1alpha1.Reconciling,
			  fmt.Sprintf("The secret %s containing the credentials will be deleted, and the user %s will be deleted from the S3 backend, then recreated (through another reconcile)", currentUserSecret.Name, userResource.Name))

			err = r.deleteSecret(ctx, &currentUserSecret)
			if err != nil {
				return r.SetDegradedCondition(
					ctx,
					req,
					userResource,
					metav1.ConditionFalse,
					s3v1alpha1.Unreachable,
					fmt.Sprintf("Deletion of secret associated to user %s have failed", userResource.Name),
					err,
				)

			}
		} else {
			r.SetProgressingCondition(ctx,
			  req,
			  userResource,
			  metav1.ConditionTrue,
			  s3v1alpha1.Reconciling,
			  fmt.Sprintf("The user %s will be deleted from the S3 backend, then recreated (through another reconcile), the secret %s will be kept.", userResource.Name, currentUserSecret.Name))

		}

		err = s3Client.DeleteUser(userResource.Spec.AccessKey)
		if err != nil {
			return r.SetDegradedCondition(
				ctx,
				req,
				userResource,
				metav1.ConditionFalse,
				s3v1alpha1.Unreachable,
				fmt.Sprintf(
					"Deletion of S3user %s on S3 server has failed",
					userResource.Name,
				),
				err,
			)

		}
		return r.handleCreate(ctx, req, userResource)
	}

	return r.SetAvailableCondition(
		ctx,
		req,
		userResource,
		s3v1alpha1.Reconciled,
		fmt.Sprintf("User %s reconciled", userResource.Name),
	)
}

func (r *S3UserReconciler) handleCreate(
	ctx context.Context,
	req reconcile.Request,
	userResource *s3v1alpha1.S3User,
) (reconcile.Result, error) {

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
		return r.SetDegradedCondition(
			ctx,
			req,
			userResource,
			metav1.ConditionUnknown,
			s3v1alpha1.InternalError,
			"Failed to generate s3client from instance",
			err,
		)
	}

	// Generating a random secret key
	secretKey, err := r.PasswordGeneratorHelper.Generate(20, true, false, true)
	if err != nil {

		return r.SetRejectedCondition(
			ctx,
			req,
			userResource,
			s3v1alpha1.InternalError,
			fmt.Sprintf("Fail to generate password for user %s", userResource.Name),
			err,
		)
	}

	// Create a new K8S Secret to hold the user's accessKey and secretKey
	secret, err := r.newSecretForCR(
		ctx,
		userResource,
		map[string][]byte{
			userResource.Spec.SecretFieldNameAccessKey: []byte(userResource.Spec.AccessKey),
			userResource.Spec.SecretFieldNameSecretKey: []byte(secretKey)},
	)
	if err != nil {
		// Error while creating the Kubernetes secret - requeue the request.
		return r.SetRejectedCondition(
			ctx,
			req,
			userResource,
			s3v1alpha1.K8sApiError,
			"Generation of associated k8s Secret has failed",
			err,
		)
	}

	// For managing user creation, we first check if a Secret matching
	// the user's spec (not matching the owner reference) exists
	existingK8sSecret := &corev1.Secret{}
	err = r.Get(
		ctx,
		types.NamespacedName{Name: secret.Name, Namespace: secret.Namespace},
		existingK8sSecret,
	)

	// If none exist : we create the user, then the secret
	if err != nil && k8sapierrors.IsNotFound(err) {
		r.SetProgressingCondition(ctx,
			  req,
			  userResource,
			  metav1.ConditionTrue,
			  s3v1alpha1.Reconciling,
			  fmt.Sprintf("No secret found for user %s creating a new Secret", userResource.Name))

		// Creating the user
		err = s3Client.CreateUser(userResource.Spec.AccessKey, secretKey)

		if err != nil {

			return r.SetRejectedCondition(
				ctx,
				req,
				userResource,
				s3v1alpha1.Unreachable,
				fmt.Sprintf("Creation of user %s on S3 instance has failed", userResource.Name),
				err,
			)
		}

		// Creating the secret
		r.SetProgressingCondition(ctx,
			  req,
			  userResource,
			  metav1.ConditionTrue,
			  s3v1alpha1.Reconciling,
			  fmt.Sprintf("Creating a new secret (%s) to store the credentials of user %s", secret.Name, userResource.Name))

		err = r.Create(ctx, secret)
		if err != nil {
			return r.SetDegradedCondition(
				ctx,
				req,
				userResource,
				metav1.ConditionFalse,
				s3v1alpha1.Unreachable,
				fmt.Sprintf("Creation of secret %s for user %s has failed", secret.Name, userResource.Name),
				err,
			)
		}

		// Add policies
		err = r.addPoliciesToUser(ctx, userResource)
		if err != nil {
			return r.SetDegradedCondition(
				ctx,
				req,
				userResource,
				metav1.ConditionFalse,
				s3v1alpha1.Unreachable,
				"Error while updating policies of user on S3 instance",
				err,
			)
		}

		return r.SetAvailableCondition(
			ctx,
			req,
			userResource,
			s3v1alpha1.Reconciled,
			fmt.Sprintf("User %s reconciled", userResource.Name),
		)

	} else if err != nil {
		return r.SetRejectedCondition(
			ctx,
			req,
			userResource,
			s3v1alpha1.K8sApiError,
			fmt.Sprintf("Fail to check if an existing secret %s already exist for user %s", secret.Name, userResource.Name),
			err,
		)
	} else {
		// Case 3.1 : If a secret already exists, but has a different S3User owner reference, then the creation should
		// fail with no requeue, and use the status to inform that the spec should be changed
		for _, ref := range existingK8sSecret.OwnerReferences {
			if ref.Kind == "S3User" {
				if ref.UID != userResource.UID {
					err = fmt.Errorf("The secret matching the new S3User's spec is owned by a different S3User.")
					return r.SetRejectedCondition(
						ctx,
						req,
						userResource,
						s3v1alpha1.CreationFailure,
						fmt.Sprintf("The secret matching the new S3User's spec is owned by a different, pre-existing S3User (%s). The S3User being created now (%s) won't be created on the S3 backend until its spec changes to target a different secret", ref.Name, userResource.Name),
						err,
					)
				}
			}
		}

		if r.OverrideExistingSecret || r.ReadExistingSecret {
			if r.ReadExistingSecret {
				// Case 3.2a : read existing secret instead of updating it
				r.SetProgressingCondition(ctx,
						  req,
						  userResource,
						  metav1.ConditionTrue,
						  s3v1alpha1.Reconciling,
						  fmt.Sprintf("The secret key of user %s will be retrieved from the secret named %s.", userResource.Name, secret.Name))

				var cpData = *&existingK8sSecret.Data
				for k, v := range cpData {
					if k == userResource.Spec.SecretFieldNameSecretKey {
						secretKey = string(v)
					}
				}
			} else {
				// Case 3.2b : they are not valid, but the operator is configured to overwrite it
				r.SetProgressingCondition(ctx,
						  req,
						  userResource,
						  metav1.ConditionTrue,
						  s3v1alpha1.Reconciling,
						  fmt.Sprintf("A secret with the name %s already exists ; it will be overwritten for user %s because of operator configuration", secret.Name, userResource.Name))
			}
			// Creating the user
			err = s3Client.CreateUser(userResource.Spec.AccessKey, secretKey)
			if err != nil {
				return r.SetRejectedCondition(
					ctx,
					req,
					userResource,
					s3v1alpha1.Unreachable,
					fmt.Sprintf("Creation of user %s on S3 instance has failed", userResource.Name),
					err,
				)
			}
			if r.OverrideExistingSecret {
				// Updating the secret
				r.SetProgressingCondition(ctx,
						  req,
						  userResource,
						  metav1.ConditionTrue,
						  s3v1alpha1.Reconciling,
						  fmt.Sprintf("Updating the pre-existing secret %s with new credentials for user %s", secret.Name, userResource.Name))

				err = r.Update(ctx, secret)
				if err != nil {
					return r.SetDegradedCondition(
						ctx,
						req,
						userResource,
						metav1.ConditionFalse,
						s3v1alpha1.Unreachable,
						fmt.Sprintf("Update of secret %s have failed for user %s", secret.Name, userResource.Name),
						err,
					)

				}
			}

			// Add policies
			err = r.addPoliciesToUser(ctx, userResource)
			if err != nil {
				return r.SetDegradedCondition(
					ctx,
					req,
					userResource,
					metav1.ConditionFalse,
					s3v1alpha1.Unreachable,
					"Error while updating associated policy",
					err,
				)
			}

			return r.SetAvailableCondition(
				ctx,
				req,
				userResource,
				s3v1alpha1.Reconciled,
				fmt.Sprintf("User %s reconciled", userResource.Name),
			)
		}

		// Case 3.3 : they are not valid, and the operator is configured keep the existing secret
		// The user will not be created, with no requeue and with two possible ways out : either toggle
		// OverrideExistingSecret on, or delete the S3User whose credentials are not working anyway.
		err = fmt.Errorf("A secret with the same name already exists ; as the operator is configured to NOT override nor read any pre-existing secrets, this user will not be created on S3 backend until spec change (to target new secret), or until the operator configuration is changed to override existing secrets")

		return r.SetRejectedCondition(
			ctx,
			req,
			userResource,
			s3v1alpha1.CreationFailure,
			fmt.Sprintf("Creation of user %s on S3 instance has failed because a secret named %s already exists. The user's spec should be changed to target a different secret", userResource.Name, secret.Name),
			err,
		)
	}
}
