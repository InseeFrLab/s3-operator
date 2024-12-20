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

package controllers

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	s3v1alpha1 "github.com/InseeFrLab/s3-operator/api/v1alpha1"
	"github.com/InseeFrLab/s3-operator/controllers/s3/factory"
	utils "github.com/InseeFrLab/s3-operator/controllers/utils"
	password "github.com/InseeFrLab/s3-operator/controllers/utils/password"
)

// S3UserReconciler reconciles a S3User object
type S3UserReconciler struct {
	client.Client
	Scheme                 *runtime.Scheme
	S3Client               factory.S3Client
	S3UserDeletion         bool
	OverrideExistingSecret bool
}

const (
	userFinalizer = "s3.onyxia.sh/userFinalizer"
)

//+kubebuilder:rbac:groups=s3.onyxia.sh,resources=S3User,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=s3.onyxia.sh,resources=S3User/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=s3.onyxia.sh,resources=S3User/finalizers,verbs=update

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
		if errors.IsNotFound(err) {
			logger.Info(fmt.Sprintf("The S3User CR %s (or its owned Secret) has been removed. NOOP", req.Name))
			return ctrl.Result{}, nil
		}
		logger.Error(err, "An error occurred when fetching the S3User from Kubernetes")
		return ctrl.Result{}, err
	}

	// Check if the userResource instance is marked to be deleted, which is
	// indicated by the deletion timestamp being set. The object will be deleted.
	if userResource.GetDeletionTimestamp() != nil {
		logger.Info("userResource have been marked for deletion")
		return r.handleS3UserDeletion(ctx, userResource)
	}

	// Add finalizer for this CR
	if !controllerutil.ContainsFinalizer(userResource, userFinalizer) {
		logger.Info("adding finalizer to user")

		controllerutil.AddFinalizer(userResource, userFinalizer)
		err = r.Update(ctx, userResource)
		if err != nil {
			logger.Error(err, "an error occurred when adding finalizer from user", "user", userResource.Name)
			return r.setS3UserStatusConditionAndUpdate(ctx, userResource, "OperatorFailed", metav1.ConditionFalse, "S3UserFinalizerAddFailed",
				fmt.Sprintf("An error occurred when attempting to add the finalizer from user %s", userResource.Name), err)
		}
	}

	// Check user existence on the S3 server
	found, err := r.S3Client.UserExist(userResource.Spec.AccessKey)
	if err != nil {
		logger.Error(err, "an error occurred while checking the existence of a user", "user", userResource.Name)
		return r.setS3UserStatusConditionAndUpdate(ctx, userResource, "OperatorFailed", metav1.ConditionFalse, "S3UserExistenceCheckFailed",
			fmt.Sprintf("The check for user %s's existence on the S3 backend has failed", userResource.Name), err)
	}

	// If the user does not exist, it is created based on the CR
	if !found {
		logger.Info("this user doesn't exist on the S3 backend and will be created", "accessKey", userResource.Spec.AccessKey)
		return r.handleS3NewUser(ctx, userResource)
	}
	logger.Info("this user already exists on the S3 backend and will be reconciled", "accessKey", userResource.Spec.AccessKey)
	return r.handleS3ExistingUser(ctx, userResource)

}

func (r *S3UserReconciler) handleS3ExistingUser(ctx context.Context, userResource *s3v1alpha1.S3User) (reconcile.Result, error) {
	logger := log.FromContext(ctx)

	// --- Begin Secret management section

	userOwnedSecret, err := r.getUserSecret(ctx, userResource)
	if err != nil {
		if err.Error() == "SecretListingFailed" {
			logger.Error(err, "An error occurred when trying to obtain the user's secret. The user will be deleted from S3 backend and recreated with a secret.")

			r.deleteSecret(ctx, &userOwnedSecret)
			err = r.S3Client.DeleteUser(userResource.Spec.AccessKey)
			if err != nil {
				logger.Error(err, "Could not delete user on S3 server", "user", userResource.Name)
				return r.setS3UserStatusConditionAndUpdate(ctx, userResource, "OperatorFailed", metav1.ConditionFalse, "S3UserDeletionFailed",
					fmt.Sprintf("Deletion of S3user %s on S3 server has failed", userResource.Name), err)
			}
			return r.handleS3NewUser(ctx, userResource)
		} else if err.Error() == "S3UserSecretNameMismatch" {
			logger.Info("A secret with owner reference to the user was found, but its name doesn't match the spec. This is probably due to the S3User's spec changing (specifically spec.secretName being added, changed or removed). The \"old\" secret will be deleted.")
			r.deleteSecret(ctx, &userOwnedSecret)
		}
	}

	if userOwnedSecret.Name == "" {
		logger.Info("Secret associated to user not found, user will be deleted from the S3 backend, then recreated with a secret")
		err = r.S3Client.DeleteUser(userResource.Spec.AccessKey)
		if err != nil {
			logger.Error(err, "Could not delete user on S3 server", "user", userResource.Name)
			return r.setS3UserStatusConditionAndUpdate(ctx, userResource, "OperatorFailed", metav1.ConditionFalse, "S3UserDeletionFailed",
				fmt.Sprintf("Deletion of S3User %s on S3 server has failed", userResource.Name), err)
		}
		return r.handleS3NewUser(ctx, userResource)
	}

	// --- End Secret management section

	logger.Info("Checking user policies")
	userPolicies, err := r.S3Client.GetUserPolicies(userResource.Spec.AccessKey)
	if err != nil {
		logger.Error(err, "Could not check the user's policies")
		return r.setS3UserStatusConditionAndUpdate(ctx, userResource, "OperatorFailed", metav1.ConditionFalse, "S3UserPolicyCheckFailed",
			fmt.Sprintf("Checking the S3user %s's policies has failed", userResource.Name), err)
	}

	policyToDelete := []string{}
	policyToAdd := []string{}
	for _, policy := range userPolicies {
		policyFound := slices.Contains(userResource.Spec.Policies, policy)
		if !policyFound {
			logger.Info(fmt.Sprintf("S3User policy definition doesn't contain policy %s", policy))
			policyToDelete = append(policyToDelete, policy)
		}
	}

	for _, policy := range userResource.Spec.Policies {
		policyFound := slices.Contains(userPolicies, policy)
		if !policyFound {
			logger.Info(fmt.Sprintf("S3User policy definition must contain policy %s", policy))
			policyToAdd = append(policyToAdd, policy)
		}
	}

	if len(policyToDelete) > 0 {
		err = r.S3Client.RemovePoliciesFromUser(userResource.Spec.AccessKey, policyToDelete)
		if err != nil {
			logger.Error(err, "an error occurred while removing policy to user", "user", userResource.Name)
			return r.setS3UserStatusConditionAndUpdate(ctx, userResource, "OperatorFailed", metav1.ConditionFalse, "S3UserPolicyAppendFailed",
				fmt.Sprintf("Error while updating policies of user %s on S3 backend has failed", userResource.Name), err)
		}
	}

	if len(policyToAdd) > 0 {
		err := r.S3Client.AddPoliciesToUser(userResource.Spec.AccessKey, policyToAdd)
		if err != nil {
			logger.Error(err, "an error occurred while adding policy to user", "user", userResource.Name)
			return r.setS3UserStatusConditionAndUpdate(ctx, userResource, "OperatorFailed", metav1.ConditionFalse, "S3UserPolicyAppendFailed",
				fmt.Sprintf("Error while updating policies of user %s on S3 backend has failed", userResource.Name), err)
		}
	}

        // If a matching secret is found, then we check if it is still valid, as in : do the credentials it
        // contains still allow authenticating the S3User on the backend ? If not, the user is deleted and recreated.
        // credentialsValid, err := r.S3Client.CheckUserCredentialsValid(userResource.Name, userResource.Spec.AccessKey, string(userOwnedSecret.Data["secretKey"]))
        credentialsValid, err := r.S3Client.CheckUserCredentialsValid(userResource.Name, string(userOwnedSecret.Data["accessKey"]), string(userOwnedSecret.Data["secretKey"]))
        if err != nil {
                logger.Error(err, "An error occurred when checking if user credentials were valid", "user", userResource.Name)
                return r.setS3UserStatusConditionAndUpdate(ctx, userResource, "OperatorFailed", metav1.ConditionFalse, "S3UserCredentialsCheckFailed",
                        fmt.Sprintf("Checking the S3User %s's credentials on S3 server has failed", userResource.Name), err)
        }

        if !credentialsValid {
                logger.Info("The secret containing the credentials will be deleted, and the user will be deleted from the S3 backend, then recreated (through another reconcile)")
                r.deleteSecret(ctx, &userOwnedSecret)
                err = r.S3Client.DeleteUser(userResource.Spec.AccessKey)
                if err != nil {
                        logger.Error(err, "Could not delete user on S3 server", "user", userResource.Name)
                        return r.setS3UserStatusConditionAndUpdate(ctx, userResource, "OperatorFailed", metav1.ConditionFalse, "S3UserDeletionFailed",
                                fmt.Sprintf("Deletion of S3user %s on S3 server has failed", userResource.Name), err)
                }

                return r.handleS3NewUser(ctx, userResource)

        }

	logger.Info("User was reconciled without error")

	// Re-fetch the S3User to ensure we have the latest state after updating the secret
	// This is necessary at least when creating a user with secretName targetting a pre-existing secret
	// that has proper form (data.accessKey and data.secretKey) but isn't owned by any other s3user
	if err := r.Get(ctx, types.NamespacedName{Name: userResource.Name, Namespace: userResource.Namespace}, userResource); err != nil {
		logger.Error(err, "Failed to re-fetch S3User")
		return ctrl.Result{}, err
	}

	return r.setS3UserStatusConditionAndUpdate(ctx, userResource, "OperatorSucceeded", metav1.ConditionTrue, "S3UserUpdated",
		fmt.Sprintf("The user %s was updated according to its matching custom resource", userResource.Name), nil)
}

func (r *S3UserReconciler) handleS3NewUser(ctx context.Context, userResource *s3v1alpha1.S3User) (reconcile.Result, error) {
	logger := log.FromContext(ctx)

	// Generating a random secret key
	secretKey, err := password.Generate(20, true, false, true)
	if err != nil {
		logger.Error(err, fmt.Sprintf("Fail to generate password for user %s", userResource.Name))
		return r.setS3UserStatusConditionAndUpdate(ctx, userResource, "OperatorFailed", metav1.ConditionFalse, "S3UserGeneratePasswordFailed",
			fmt.Sprintf("An error occurred when attempting to generate password for user %s", userResource.Name), err)
	}

	// Create a new K8S Secret to hold the user's accessKey and secretKey
	secret, err := r.newSecretForCR(ctx, userResource, map[string][]byte{"accessKey": []byte(userResource.Spec.AccessKey), "secretKey": []byte(secretKey)})
	if err != nil {
		// Error while creating the Kubernetes secret - requeue the request.
		logger.Error(err, "Could not generate Kubernetes secret")
		return r.setS3UserStatusConditionAndUpdate(ctx, userResource, "OperatorFailed", metav1.ConditionFalse, "SecretGenerationFailed",
			fmt.Sprintf("The generation of the k8s Secret %s has failed", userResource.Name), err)
	}

	// For managing user creation, we first check if a Secret matching
	// the user's spec (not matching the owner reference) exists
	existingK8sSecret := &corev1.Secret{}
	err = r.Get(ctx, types.NamespacedName{Name: secret.Name, Namespace: secret.Namespace}, existingK8sSecret)

	// If none exist : we create the user, then the secret
	if err != nil && errors.IsNotFound(err) {
		logger.Info("No secret found ; creating a new Secret", "Secret.Namespace", secret.Namespace, "Secret.Name", secret.Name)

		// Creating the user
		err = r.S3Client.CreateUser(userResource.Spec.AccessKey, secretKey)

		if err != nil {
			logger.Error(err, "an error occurred while creating user on S3 server", "user", userResource.Name)
			return r.setS3UserStatusConditionAndUpdate(ctx, userResource, "OperatorFailed", metav1.ConditionFalse, "S3UserCreationFailed",
				fmt.Sprintf("Creation of user %s on S3 instance has failed", userResource.Name), err)
		}

		// Creating the secret
		logger.Info("Creating a new secret to store the user's credentials", "secretName", secret.Name)
		err = r.Create(ctx, secret)
		if err != nil {
			logger.Error(err, "Could not create secret")
			return r.setS3UserStatusConditionAndUpdate(ctx, userResource, "OperatorFailed", metav1.ConditionFalse, "S3UserSecretCreationFailed",
				fmt.Sprintf("The creation of the k8s Secret %s has failed", secret.Name), err)
		}

		// Add policies
		err = r.addPoliciesToUser(ctx, userResource)
		if err != nil {
			return r.setS3UserStatusConditionAndUpdate(ctx, userResource, "OperatorFailed", metav1.ConditionFalse, "S3UserPolicyAppendFailed",
				fmt.Sprintf("Error while updating policies of user %s on S3 instance has failed", userResource.Name), err)
		}

		return r.setS3UserStatusConditionAndUpdate(ctx, userResource, "OperatorSucceeded", metav1.ConditionTrue, "S3UserCreatedWithNewSecret",
			fmt.Sprintf("The S3User %s and the Secret %s were created successfully", userResource.Name, secret.Name), nil)

	} else if err != nil {
		logger.Error(err, "Couldn't check secret existence")
		return r.setS3UserStatusConditionAndUpdate(ctx, userResource, "OperatorFailed", metav1.ConditionFalse, "SecretExistenceCheckFailedDuringS3UserCreation",
			fmt.Sprintf("The check for an existing secret %s during the creation of the S3User %s has failed", secret.Name, userResource.Name), err)

	} else {
		// If a secret already exists, but has a different S3User owner reference, then the creation should
		// fail with no requeue, and use the status to inform that the spec should be changed
		for _, ref := range existingK8sSecret.OwnerReferences {
			if ref.Kind == "S3User" {
				if ref.UID != userResource.UID {
					logger.Error(fmt.Errorf(""), "The secret matching the new S3User's spec is owned by a different S3User.", "conflictingUser", ref.Name)
					return r.setS3UserStatusConditionAndUpdate(ctx, userResource, "OperatorFailed", metav1.ConditionFalse, "S3UserCreationFailedAsSecretIsOwnedByAnotherS3User",
						fmt.Sprintf("The secret matching the new S3User's spec is owned by a different, pre-existing S3User (%s). The S3User being created now (%s) won't be created on the S3 backend until its spec changes to target a different secret", ref.Name, userResource.Name), nil)
				}
			}
		}

		if r.OverrideExistingSecret {
			// Case 3.2 : they are not valid, but the operator is configured to overwrite it
			logger.Info(fmt.Sprintf("A secret with the name %s already exists ; it will be overwritten as per operator configuration", secret.Name))

			// Creating the user
			err = r.S3Client.CreateUser(userResource.Spec.AccessKey, secretKey)

			if err != nil {
				logger.Error(err, "an error occurred while creating user on S3 server", "user", userResource.Name)
				return r.setS3UserStatusConditionAndUpdate(ctx, userResource, "OperatorFailed", metav1.ConditionFalse, "S3UserCreationFailed",
					fmt.Sprintf("Creation of user %s on S3 instance has failed", userResource.Name), err)
			}

			// Updating the secret
			logger.Info("Updating the pre-existing secret with new credentials", "secretName", secret.Name)
			err = r.Update(ctx, secret)
			if err != nil {
				logger.Error(err, "Could not update secret")
				return r.setS3UserStatusConditionAndUpdate(ctx, userResource, "OperatorFailed", metav1.ConditionFalse, "SecretUpdateFailed",
					fmt.Sprintf("The update of the k8s Secret %s has failed", secret.Name), err)
			}

			// Add policies
			err = r.addPoliciesToUser(ctx, userResource)
			if err != nil {
				return r.setS3UserStatusConditionAndUpdate(ctx, userResource, "OperatorFailed", metav1.ConditionFalse, "S3UserPolicyAppendFailed",
					fmt.Sprintf("Error while updating policies of user %s on S3 instance has failed", userResource.Name), err)
			}

			return r.setS3UserStatusConditionAndUpdate(ctx, userResource, "OperatorSucceeded", metav1.ConditionTrue, "S3UserCreatedWithWithOverridenSecret",
				fmt.Sprintf("The S3User %s was created and the Secret %s was updated successfully", userResource.Name, secret.Name), nil)
		}

		// Case 3.3 : they are not valid, and the operator is configured keep the existing secret
		// The user will not be created, with no requeue and with two possible ways out : either toggle
		// OverrideExistingSecret on, or delete the S3User whose credentials are not working anyway.
		logger.Error(nil, fmt.Sprintf("A secret with the name %s already exists ; as the operator is configured to NOT override any pre-existing secrets, this user (%s) not be created on S3 backend until spec change (to target new secret), or until the operator configuration is changed to override existing secrets", secret.Name, userResource.Name))
		return r.setS3UserStatusConditionAndUpdate(ctx, userResource, "OperatorSucceeded", metav1.ConditionTrue, "S3UserCreationFailedAsSecretCannotBeOverwritten",
			fmt.Sprintf("The S3User %s wasn't created, as its spec targets a secret (%s) containing invalid credentials. The user's spec should be changed to target a different secret.", userResource.Name, secret.Name), nil)

	}
}

func (r *S3UserReconciler) addPoliciesToUser(ctx context.Context, userResource *s3v1alpha1.S3User) error {
	logger := log.FromContext(ctx)
	policies := userResource.Spec.Policies
	if policies != nil {
		err := r.S3Client.AddPoliciesToUser(userResource.Spec.AccessKey, policies)
		if err != nil {
			logger.Error(err, "an error occurred while adding policy to user", "user", userResource.Name)
			return err
		}
	}
	return nil
}

func (r *S3UserReconciler) handleS3UserDeletion(ctx context.Context, userResource *s3v1alpha1.S3User) (reconcile.Result, error) {
	logger := log.FromContext(ctx)

	if controllerutil.ContainsFinalizer(userResource, userFinalizer) {
		// Run finalization logic for S3UserFinalizer. If the finalization logic fails, don't remove the finalizer so that we can retry during the next reconciliation.
		if err := r.finalizeS3User(userResource); err != nil {
			logger.Error(err, "an error occurred when attempting to finalize the user", "user", userResource.Name)
			return r.setS3UserStatusConditionAndUpdate(ctx, userResource, "OperatorFailed", metav1.ConditionFalse, "S3UserFinalizeFailed",
				fmt.Sprintf("An error occurred when attempting to delete user %s", userResource.Name), err)
		}

		//Remove userFinalizer. Once all finalizers have been removed, the object will be deleted.
		controllerutil.RemoveFinalizer(userResource, userFinalizer)
		// Unsure why the behavior is different to that of bucket/policy/path controllers, but it appears
		// calling r.Update() for adding/removal of finalizer is not necessary (an update event is generated
		// with the call to AddFinalizer/RemoveFinalizer), and worse, causes "freshness" problem (with the
		// "the object has been modified; please apply your changes to the latest version and try again" error)
		err := r.Update(ctx, userResource)
		if err != nil {
			logger.Error(err, "Failed to remove finalizer.")
			return r.setS3UserStatusConditionAndUpdate(ctx, userResource, "OperatorFailed", metav1.ConditionFalse, "S3UserFinalizerRemovalFailed",
				fmt.Sprintf("An error occurred when attempting to remove the finalizer from user %s", userResource.Name), err)
		}
	}
	return ctrl.Result{}, nil
}

func (r *S3UserReconciler) getUserSecret(ctx context.Context, userResource *s3v1alpha1.S3User) (corev1.Secret, error) {
	logger := log.FromContext(ctx)

	// Listing every secrets in the S3User's namespace, as a first step
	// to get the actual secret matching the S3User proper.
	// TODO : proper label matching ?
	secretsList := &corev1.SecretList{}
	userSecret := corev1.Secret{}

	err := r.List(ctx, secretsList, client.InNamespace(userResource.Namespace))
	if err != nil {
		logger.Error(err, "An error occurred while listing the secrets in user's namespace")
		return userSecret, fmt.Errorf("SecretListingFailed")
	}

	if len(secretsList.Items) == 0 {
		logger.Info("The user's namespace doesn't appear to contain any secret")
		return userSecret, nil
	}
	// In all the secrets inside the S3User's namespace, one should have an owner reference
	// pointing to the S3User. For that specific secret, we check if its name matches the one from
	// the S3User, whether explicit (userResource.Spec.SecretName) or implicit (userResource.Name)
	// In case of mismatch, that secret is deleted (and will be recreated) ; if there is a match,
	// it will be used for state comparison.
	uid := userResource.GetUID()

	// cmp.Or takes the first non "zero" value, see https://pkg.go.dev/cmp#Or
	effectiveS3UserSecretName := cmp.Or(userResource.Spec.SecretName, userResource.Name)
	for _, secret := range secretsList.Items {
		for _, ref := range secret.OwnerReferences {
			if ref.UID == uid {
				if secret.Name != effectiveS3UserSecretName {
					return secret, fmt.Errorf("S3UserSecretNameMismatch")
				} else {
					userSecret = secret
					break
				}
			}
		}
	}

	return userSecret, nil
}

func (r *S3UserReconciler) deleteSecret(ctx context.Context, secret *corev1.Secret) {
	logger := log.FromContext(ctx)
	err := r.Delete(ctx, secret)
	if err != nil {
		logger.Error(err, "an error occurred while deleting a secret")
	}
}

// SetupWithManager sets up the controller with the Manager.*
func (r *S3UserReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// filterLogger := ctrl.Log.WithName("filterEvt")
	return ctrl.NewControllerManagedBy(mgr).
		For(&s3v1alpha1.S3User{}).
		// The "secret owning" implies the reconcile loop will be called whenever a Secret owned
		// by a S3User is created/updated/deleted. In other words, even when creating a single S3User,
		// there is going to be several iterations.
		Owns(&corev1.Secret{}).
		// See : https://sdk.operatorframework.io/docs/building-operators/golang/references/event-filtering/
		WithEventFilter(predicate.Funcs{

			// Ignore updates to CR status in which case metadata.Generation does not change,
			// unless it is a change to the underlying Secret
			UpdateFunc: func(e event.UpdateEvent) bool {

				// To check if the update event is tied to a change on secret,
				// we try to cast e.ObjectNew to a secret (only if it's not a S3User, which
				// should prevent any TypeAssertionError based panic).
				secretUpdate := false
				newUser, _ := e.ObjectNew.(*s3v1alpha1.S3User)
				if newUser == nil {
					secretUpdate = (e.ObjectNew.(*corev1.Secret) != nil)
				}

				return e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration() || secretUpdate
			},
			// Ignore create events caused by the underlying secret's creation
			CreateFunc: func(e event.CreateEvent) bool {
				user, _ := e.Object.(*s3v1alpha1.S3User)
				return user != nil
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				// Evaluates to false if the object has been confirmed deleted.
				return !e.DeleteStateUnknown
			},
		}).
		WithOptions(controller.Options{MaxConcurrentReconciles: 10}).
		Complete(r)
}

func (r *S3UserReconciler) setS3UserStatusConditionAndUpdate(ctx context.Context, userResource *s3v1alpha1.S3User, conditionType string, status metav1.ConditionStatus, reason string, message string, srcError error) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// We moved away from meta.SetStatusCondition, as the implementation did not allow for updating
	// lastTransitionTime if a Condition (as identified by Reason instead of Type) was previously
	// obtained and updated to again.
	userResource.Status.Conditions = utils.UpdateConditions(userResource.Status.Conditions, metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		LastTransitionTime: metav1.NewTime(time.Now()),
		Message:            message,
		ObservedGeneration: userResource.GetGeneration(),
	})

	err := r.Status().Update(ctx, userResource)
	if err != nil {
		logger.Error(err, "an error occurred while updating the status of the S3User resource")
		return ctrl.Result{}, utilerrors.NewAggregate([]error{err, srcError})
	}
	return ctrl.Result{}, srcError
}

func (r *S3UserReconciler) finalizeS3User(userResource *s3v1alpha1.S3User) error {
	if r.S3UserDeletion {
		return r.S3Client.DeleteUser(userResource.Spec.AccessKey)
	}
	return nil
}

// newSecretForCR returns a secret with the same name/namespace as the CR.
// The secret will include all labels and annotations from the CR.
func (r *S3UserReconciler) newSecretForCR(ctx context.Context, userResource *s3v1alpha1.S3User, data map[string][]byte) (*corev1.Secret, error) {
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
