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
	"context"
	"fmt"
	"slices"

	corev1 "k8s.io/api/core/v1"
	k8sapierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
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
		meta.SetStatusCondition(
			&userResource.Status.Conditions,
			metav1.Condition{
				Type:               s3v1alpha1.ConditionReconciled,
				Status:             metav1.ConditionUnknown,
				ObservedGeneration: userResource.Generation,
				Reason:             s3v1alpha1.Reconciling,
				Message:            "Starting reconciliation",
			},
		)
		if err = r.Status().Update(ctx, userResource); err != nil {
			logger.Error(
				err,
				"Failed to update userRessource status",
				"userResourceName",
				userResource.Name,
				"NamespacedName",
				req.NamespacedName.String(),
			)
			return ctrl.Result{}, err
		}

		// Let's re-fetch the userResource Custom Resource after update the status
		// so that we have the latest state of the resource on the cluster and we will avoid
		// raise the issue "the object has been modified, please apply
		// your changes to the latest version and try again" which would re-trigger the reconciliation
		// if we try to update it again in the following operations
		if err := r.Get(ctx, req.NamespacedName, userResource); err != nil {
			logger.Error(
				err,
				"Failed to re-fetch userResource",
				"userResourceName",
				userResource.Name,
				"NamespacedName",
				req.NamespacedName.String(),
			)
			return ctrl.Result{}, err
		}
	}

	// Add finalizer for this CR
	if !controllerutil.ContainsFinalizer(userResource, userFinalizer) {
		logger.Info("Adding finalizer to user resource",
			"userResourceName",
			userResource.Name,
			"NamespacedName",
			req.NamespacedName.String())

		if ok := controllerutil.AddFinalizer(userResource, userFinalizer); !ok {
			logger.Error(
				err,
				"Failed to add finalizer into user resource",
				"userResourceName",
				userResource.Name,
				"NamespacedName",
				req.NamespacedName.String(),
			)
			return ctrl.Result{Requeue: true}, nil
		}

		if err = r.Update(ctx, userResource); err != nil {
			logger.Error(
				err,
				"An error occurred when adding finalizer from user",
				"userResourceName",
				userResource.Name,
				"NamespacedName",
				req.NamespacedName.String(),
			)
			return ctrl.Result{}, err
		}

		// Let's re-fetch the userResource Custom Resource after adding the finalizer
		// so that we have the latest state of the resource on the cluster and we will avoid
		// raise the issue "the object has been modified, please apply
		// your changes to the latest version and try again" which would re-trigger the reconciliation
		// if we try to update it again in the following operations
		if err := r.Get(ctx, req.NamespacedName, userResource); err != nil {
			logger.Error(
				err,
				"Failed to re-fetch userResource",
				"userResourceName",
				userResource.Name,
				"NamespacedName",
				req.NamespacedName.String(),
			)
			return ctrl.Result{}, err
		}
	}

	// Check if the userResource instance is marked to be deleted, which is
	// indicated by the deletion timestamp being set. The object will be deleted.
	if userResource.GetDeletionTimestamp() != nil {
		logger.Info("userResource have been marked for deletion",
			"userResource",
			userResource.Name,
			"NamespacedName",
			req.NamespacedName.String())
		return r.handleDeletion(ctx, req, userResource)
	}

	return r.handleReconciliation(ctx, req, userResource)

}

func (r *S3UserReconciler) handleReconciliation(
	ctx context.Context,
	req reconcile.Request,
	userResource *s3v1alpha1.S3User,
) (reconcile.Result, error) {
	logger := log.FromContext(ctx)

	s3Client, err := r.S3Instancehelper.GetS3ClientForRessource(
		ctx,
		r.Client,
		r.S3factory,
		userResource.Name,
		userResource.Namespace,
		userResource.Spec.S3InstanceRef,
	)
	if err != nil {
		logger.Error(err, "An error occurred while getting s3Client")
		return r.SetReconciledCondition(
			ctx,
			req,
			userResource,
			s3v1alpha1.Unreachable,
			"Failed to generate s3client from instance",
			err,
		)
	}

	found, err := s3Client.UserExist(userResource.Spec.AccessKey)
	if err != nil {
		logger.Error(
			err,
			"An error occurred while checking the existence of a user",
			"userResource",
			userResource.Name,
			"NamespacedName",
			req.NamespacedName.String(),
		)
		return r.SetReconciledCondition(
			ctx,
			req,
			userResource,
			s3v1alpha1.Unreachable,
			"Fail to check existence",
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
		logger.Error(err, "An error occurred while getting s3Client")
		return r.SetReconciledCondition(
			ctx,
			req,
			userResource,
			s3v1alpha1.Unreachable,
			"Failed to generate s3client from instance",
			err,
		)
	}

	userOwnedSecret, err := r.getUserSecret(ctx, userResource)
	if err != nil {
		if err.Error() == "SecretListingFailed" {
			logger.Error(
				err,
				"An error occurred when trying to obtain the user's secret",
				"userResourceName",
				userResource.Name,
				"NamespacedName",
				req.NamespacedName.String(),
			)

			err = r.deleteSecret(ctx, &userOwnedSecret)
			if err != nil {
				logger.Error(
					err,
					"Deletion of secret associated to user have failed",
					"userResource",
					userResource.Name,
					"userResourceName",
					userResource.Name,
					"NamespacedName",
					req.NamespacedName.String(),
				)
				return r.SetReconciledCondition(
					ctx,
					req,
					userResource,
					s3v1alpha1.Unreachable,
					"Deletion of secret associated to user have failed",
					err,
				)

			}
			err = s3Client.DeleteUser(userResource.Spec.AccessKey)
			if err != nil {
				logger.Error(err, "Could not delete user on S3 server", "userResource",
					userResource.Name,
					"userResourceName",
					userResource.Name,
					"NamespacedName",
					req.NamespacedName.String())
				return r.SetReconciledCondition(
					ctx,
					req,
					userResource,
					s3v1alpha1.Unreachable,
					fmt.Sprintf(
						"Deletion of S3user %s on S3 server has failed",
						userResource.Name,
					),
					err,
				)

			}
			return r.handleCreate(ctx, req, userResource)
		} else if err.Error() == "S3UserSecretNameMismatch" {
			logger.Info("A secret with owner reference to the user was found, but its name doesn't match the spec. This is probably due to the S3User's spec changing (specifically spec.secretName being added, changed or removed). The \"old\" secret will be deleted.", "userResource",
				userResource.Name,
				"NamespacedName",
				req.NamespacedName.String())
			err = r.deleteSecret(ctx, &userOwnedSecret)
			if err != nil {
				logger.Error(err, "Deletion of secret associated to user have failed", "userResourceName",
					userResource.Name,
					"NamespacedName",
					req.NamespacedName.String())
				return r.SetReconciledCondition(
					ctx,
					req,
					userResource,
					s3v1alpha1.Unreachable,
					"Deletion of secret associated to user have failed",
					err,
				)

			}
		}
	}

	if userOwnedSecret.Name == "" {
		logger.Info(
			"Secret associated to user not found, user will be deleted from the S3 backend, then recreated with a secret",
			"userResourceName",
			userResource.Name,
			"NamespacedName",
			req.NamespacedName.String(),
		)

		err = s3Client.DeleteUser(userResource.Spec.AccessKey)
		if err != nil {
			logger.Error(err, "Could not delete user on S3 server", "userResource",
				userResource.Name,
				"userResourceName",
				userResource.Name,
				"NamespacedName",
				req.NamespacedName.String())
			return r.SetReconciledCondition(
				ctx,
				req,
				userResource,
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

	credentialsValid, err := s3Client.CheckUserCredentialsValid(
		userResource.Name,
		string(userOwnedSecret.Data["accessKey"]),
		string(userOwnedSecret.Data["secretKey"]),
	)
	if err != nil {
		logger.Error(
			err,
			"An error occurred when checking if user credentials were valid",
			"userResource",
			userResource.Name,
			"NamespacedName",
			req.NamespacedName.String(),
		)
		return r.SetReconciledCondition(
			ctx,
			req,
			userResource,
			s3v1alpha1.Unreachable,
			"Checking credentials on S3 server has failed",
			err,
		)
	}

	if !credentialsValid {
		logger.Info(
			"The secret containing the credentials will be deleted, and the user will be deleted from the S3 backend, then recreated (through another reconcile)",
			"userResource",
			userResource.Name,
			"NamespacedName",
			req.NamespacedName.String(),
		)
		err = r.deleteSecret(ctx, &userOwnedSecret)
		if err != nil {
			logger.Error(err, "Deletion of secret associated to user have failed", "userResource",
				userResource.Name,
				"userResourceName",
				userResource.Name,
				"NamespacedName",
				req.NamespacedName.String())
			return r.SetReconciledCondition(
				ctx,
				req,
				userResource,
				s3v1alpha1.Unreachable,
				"Deletion of secret associated to user have failed",
				err,
			)

		}
		err = s3Client.DeleteUser(userResource.Spec.AccessKey)
		if err != nil {
			logger.Error(err, "Could not delete user on S3 server", "userResource",
				userResource.Name,
				"userResourceName",
				userResource.Name,
				"NamespacedName",
				req.NamespacedName.String())
			return r.SetReconciledCondition(
				ctx,
				req,
				userResource,
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

	// --- End Secret management section

	logger.Info("Checking user policies", "userResource",
		userResource.Name,
		"NamespacedName",
		req.NamespacedName.String())
	userPolicies, err := s3Client.GetUserPolicies(userResource.Spec.AccessKey)
	if err != nil {
		logger.Error(err, "Could not check the user's policies", "userResource",
			userResource.Name,
			"NamespacedName",
			req.NamespacedName.String())
		return r.SetReconciledCondition(
			ctx,
			req,
			userResource,
			s3v1alpha1.Unreachable,
			"Checking the S3user policies has failed",
			err,
		)
	}

	policyToDelete := []string{}
	policyToAdd := []string{}
	for _, policy := range userPolicies {
		policyFound := slices.Contains(userResource.Spec.Policies, policy)
		if !policyFound {
			logger.Info(
				fmt.Sprintf("S3User policy definition doesn't contain policy %s", policy),
				"userResource",
				userResource.Name,
				"NamespacedName",
				req.NamespacedName.String(),
			)
			policyToDelete = append(policyToDelete, policy)
		}
	}

	for _, policy := range userResource.Spec.Policies {
		policyFound := slices.Contains(userPolicies, policy)
		if !policyFound {
			logger.Info(
				fmt.Sprintf("S3User policy definition must contain policy %s", policy),
				"userResource",
				userResource.Name,
				"NamespacedName",
				req.NamespacedName.String(),
			)
			policyToAdd = append(policyToAdd, policy)
		}
	}

	if len(policyToDelete) > 0 {
		err = s3Client.RemovePoliciesFromUser(userResource.Spec.AccessKey, policyToDelete)
		if err != nil {
			logger.Error(
				err,
				"An error occurred while removing policy to user",
				"userResource",
				userResource.Name,
				"NamespacedName",
				req.NamespacedName.String(),
			)
			return r.SetReconciledCondition(
				ctx,
				req,
				userResource,
				s3v1alpha1.Unreachable,
				"Error while updating policies of user",
				err,
			)
		}
	}

	if len(policyToAdd) > 0 {
		err := s3Client.AddPoliciesToUser(userResource.Spec.AccessKey, policyToAdd)
		if err != nil {
			logger.Error(
				err,
				"An error occurred while adding policy to user",
				"userResource",
				userResource.Name,
				"NamespacedName",
				req.NamespacedName.String(),
			)

			return r.SetReconciledCondition(
				ctx,
				req,
				userResource,
				s3v1alpha1.Unreachable,
				"Error while updating policies of user",
				err,
			)

		}
	}

	logger.Info("User was reconciled without error",
		"userResource",
		userResource.Name,
		"NamespacedName",
		req.NamespacedName.String(),
	)

	// Re-fetch the S3User to ensure we have the latest state after updating the secret
	// This is necessary at least when creating a user with secretName targetting a pre-existing secret
	// that has proper form (data.accessKey and data.secretKey) but isn't owned by any other s3user
	if err := r.Get(ctx, req.NamespacedName, userResource); err != nil {
		logger.Error(
			err,
			"Failed to re-fetch userResource",
			"userResourceName",
			userResource.Name,
			"NamespacedName",
			req.NamespacedName.String(),
		)
		return ctrl.Result{}, err
	}

	return r.SetReconciledCondition(
		ctx,
		req,
		userResource,
		s3v1alpha1.Reconciled,
		"user reconciled",
		err,
	)
}

func (r *S3UserReconciler) handleCreate(
	ctx context.Context,
	req reconcile.Request,
	userResource *s3v1alpha1.S3User,
) (reconcile.Result, error) {
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
		logger.Error(err, "An error occurred while getting s3Client")
		return r.SetReconciledCondition(
			ctx,
			req,
			userResource,
			s3v1alpha1.Unreachable,
			"Failed to generate s3client from instance",
			err,
		)
	}

	// Generating a random secret key
	secretKey, err := r.PasswordGeneratorHelper.Generate(20, true, false, true)
	if err != nil {

		logger.Error(err, fmt.Sprintf("Fail to generate password for user %s", userResource.Name),
			"userResource",
			userResource.Name,
			"NamespacedName",
			req.NamespacedName.String(),
		)

		return r.SetReconciledCondition(
			ctx,
			req,
			userResource,
			s3v1alpha1.Unreachable,
			"An error occurred when attempting to generate password for user",
			err,
		)
	}

	// Create a new K8S Secret to hold the user's accessKey and secretKey
	secret, err := r.newSecretForCR(
		ctx,
		userResource,
		map[string][]byte{
			"accessKey": []byte(userResource.Spec.AccessKey),
			"secretKey": []byte(secretKey),
		},
	)
	if err != nil {
		// Error while creating the Kubernetes secret - requeue the request.
		logger.Error(err, "Could not generate Kubernetes secret",
			"userResource",
			userResource.Name,
			"NamespacedName",
			req.NamespacedName.String(),
		)
		return r.SetReconciledCondition(
			ctx,
			req,
			userResource,
			s3v1alpha1.Unreachable,
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
		logger.Info(
			"No secret found ; creating a new Secret",
			"Secret.Namespace",
			secret.Namespace,
			"Secret.Name",
			secret.Name,
		)

		// Creating the user
		err = s3Client.CreateUser(userResource.Spec.AccessKey, secretKey)

		if err != nil {
			logger.Error(
				err,
				"An error occurred while creating user on S3 server",
				"userResource",
				userResource.Name,
				"NamespacedName",
				req.NamespacedName.String(),
			)
			return r.SetReconciledCondition(
				ctx,
				req,
				userResource,
				s3v1alpha1.Unreachable,
				"Creation of user on S3 instance has failed",
				err,
			)
		}

		// Creating the secret
		logger.Info(
			"Creating a new secret to store the user's credentials",
			"secretName",
			secret.Name,
			"userResource",
			userResource.Name,
			"NamespacedName",
			req.NamespacedName.String(),
		)
		err = r.Create(ctx, secret)
		if err != nil {
			logger.Error(err, "Could not create secret for user",
				"secretName",
				secret.Name,
				"userResource",
				userResource.Name,
				"NamespacedName",
				req.NamespacedName.String(),
			)
			return r.SetReconciledCondition(
				ctx,
				req,
				userResource,
				s3v1alpha1.Unreachable,
				"Creation of secret for user has failed",
				err,
			)
		}

		// Add policies
		err = r.addPoliciesToUser(ctx, userResource)
		if err != nil {
			return r.SetReconciledCondition(
				ctx,
				req,
				userResource,
				s3v1alpha1.Unreachable,
				"Error while updating policies of user on S3 instance",
				err,
			)
		}

		return r.SetReconciledCondition(
			ctx,
			req,
			userResource,
			s3v1alpha1.Reconciled,
			"User reconciled",
			err,
		)

	} else if err != nil {
		logger.Error(err, "Couldn't check secret existence",
			"secretName",
			secret.Name,
			"userResource",
			userResource.Name,
			"NamespacedName",
			req.NamespacedName.String(),
		)
		return r.SetReconciledCondition(
			ctx,
			req,
			userResource,
			s3v1alpha1.Unreachable,
			"Fail to check if an existing secret already exist",
			err,
		)
	} else {
		// If a secret already exists, but has a different S3User owner reference, then the creation should
		// fail with no requeue, and use the status to inform that the spec should be changed
		for _, ref := range existingK8sSecret.OwnerReferences {
			if ref.Kind == "S3User" {
				if ref.UID != userResource.UID {
					logger.Error(fmt.Errorf(""), "The secret matching the new S3User's spec is owned by a different S3User.",
						"conflictingUser",
						ref.Name,
						"secretName",
						secret.Name,
						"userResource",
						userResource.Name,
						"NamespacedName",
						req.NamespacedName.String())
					return r.SetReconciledCondition(
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

		if r.OverrideExistingSecret {
			// Case 3.2 : they are not valid, but the operator is configured to overwrite it
			logger.Info(fmt.Sprintf("A secret with the name %s already exists ; it will be overwritten because of operator configuration", secret.Name), "secretName",
				secret.Name,
				"userResource",
				userResource.Name,
				"NamespacedName",
				req.NamespacedName.String())

			// Creating the user
			err = s3Client.CreateUser(userResource.Spec.AccessKey, secretKey)
			if err != nil {
				logger.Error(
					err,
					"An error occurred while creating user on S3 server",
					"userResource",
					userResource.Name,
					"NamespacedName",
					req.NamespacedName.String(),
				)
				return r.SetReconciledCondition(
					ctx,
					req,
					userResource,
					s3v1alpha1.Unreachable,
					"Creation of user on S3 instance has failed",
					err,
				)
			}

			// Updating the secret
			logger.Info("Updating the pre-existing secret with new credentials",
				"secretName",
				secret.Name,
				"userResource",
				userResource.Name,
				"NamespacedName",
				req.NamespacedName.String(),
			)
			err = r.Update(ctx, secret)
			if err != nil {
				logger.Error(err, "Could not update secret", "secretName",
					secret.Name,
					"userResource",
					userResource.Name,
					"NamespacedName",
					req.NamespacedName.String())
				return r.SetReconciledCondition(
					ctx,
					req,
					userResource,
					s3v1alpha1.Unreachable,
					"Update of secret have failed",
					err,
				)
			}

			// Add policies
			err = r.addPoliciesToUser(ctx, userResource)
			if err != nil {
				return r.SetReconciledCondition(
					ctx,
					req,
					userResource,
					s3v1alpha1.Unreachable,
					"Error while updating associated policy",
					err,
				)
			}

			return r.SetReconciledCondition(
				ctx,
				req,
				userResource,
				s3v1alpha1.Reconciled,
				"User Reconciled",
				err,
			)
		}

		// Case 3.3 : they are not valid, and the operator is configured keep the existing secret
		// The user will not be created, with no requeue and with two possible ways out : either toggle
		// OverrideExistingSecret on, or delete the S3User whose credentials are not working anyway.
		logger.Error(fmt.Errorf(""),
			"A secret with the same name already exists ; as the operator is configured to NOT override any pre-existing secrets, this user will not be created on S3 backend until spec change (to target new secret), or until the operator configuration is changed to override existing secrets",
			"secretName",
			secret.Name,
			"userResource",
			userResource.Name,
			"NamespacedName",
			req.NamespacedName.String())
		return r.SetReconciledCondition(
			ctx,
			req,
			userResource,
			s3v1alpha1.CreationFailure,
			"Creation of user on S3 instance has failed necause secret contains invalid credentials. The user's spec should be changed to target a different secret",
			err,
		)
	}
}
