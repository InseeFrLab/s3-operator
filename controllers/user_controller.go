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
	"github.com/go-logr/logr"
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
			logger.Info(fmt.Sprintf("S3User CRD %s has been removed. NOOP", req.Name))
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Check if the userResource instance is marked to be deleted, which is
	// indicated by the deletion timestamp being set. The object will be deleted.

	if userResource.GetDeletionTimestamp() != nil {
		logger.Info("userResource have been marked for deletion")
		return handleS3UserDeletion(ctx, userResource, r, logger, err)
	}

	// Add finalizer for this CR
	if !controllerutil.ContainsFinalizer(userResource, userFinalizer) {
		controllerutil.AddFinalizer(userResource, userFinalizer)
		err = r.Update(ctx, userResource)
		if err != nil {
			logger.Error(err, "an error occurred when adding finalizer from user", "user", userResource.Name)
			// return ctrl.Result{}, err
			return r.setS3UserStatusConditionAndUpdate(ctx, userResource, "OperatorFailed", metav1.ConditionFalse, "S3UserFinalizerAddFailed",
				fmt.Sprintf("An error occurred when attempting to add the finalizer from user [%s]", userResource.Name), err)
		}
	}

	// Check user existence on the S3 server
	found, err := r.S3Client.UserExist(userResource.Spec.AccessKey)
	if err != nil {
		logger.Error(err, "an error occurred while checking the existence of a user", "user", userResource.Name)
		return r.setS3UserStatusConditionAndUpdate(ctx, userResource, "OperatorFailed", metav1.ConditionFalse, "S3UserExistenceCheckFailed",
			fmt.Sprintf("Obtaining user[%s] info from S3 instance has failed", userResource.Name), err)
	}

	// If the user does not exist, it is created based on the CR
	if !found {
		// S3User creation
		// The user creation happened without any error
		return handleS3UserCreation(ctx, userResource, r)
	} else {
		// handle user reconciliation with its CR (or NOOP)
		return handleReconcileS3User(ctx, err, r, userResource, logger)
	}
}

func deleteSecret(ctx context.Context, r *S3UserReconciler, secreta corev1.Secret) {
	err := r.Delete(ctx, &secreta)
	if err != nil {
		// Handle error
	}
}

func handleReconcileS3User(ctx context.Context, err error, r *S3UserReconciler, userResource *s3v1alpha1.S3User, logger logr.Logger) (reconcile.Result, error) {
	//secret := &corev1.Secret{}
	secretsList := &corev1.SecretList{}
	uiid := userResource.GetUID()
	secretNameFromUser := userResource.Spec.SecretName

	err = r.List(ctx, secretsList, client.InNamespace(userResource.Namespace), client.MatchingLabels{"app.kubernetes.io/created-by": "s3-operator"}) // Use r.Client.List instead of r.List

	if err != nil && (errors.IsNotFound(err) || len(secretsList.Items) == 0) {
		logger.Info("Secret associated to user not found, user will be deleted and recreated", "user", userResource.Name)
		err = r.S3Client.DeleteUser(userResource.Spec.AccessKey)
		if err != nil {
			logger.Error(err, "Could not delete user on S3 server", "user", userResource.Name)
			return r.setS3UserStatusConditionAndUpdate(ctx, userResource, "OperatorFailed", metav1.ConditionFalse, "S3UserDeletionFailed",
				fmt.Sprintf("Deletion of S3user %s on S3 server has failed", userResource.Name), err)
		}
		return handleS3UserCreation(ctx, userResource, r)
	} else if err != nil {
		logger.Error(err, "Could not locate secret", "secret", userResource.Name)
		return r.setS3UserStatusConditionAndUpdate(ctx, userResource, "OperatorFailed", metav1.ConditionFalse, "SecretNotFound",
			fmt.Sprintf("Cannot locate k8s secrets [%s]", userResource.Name), err)
	}

	secretToTest := &corev1.Secret{}
	for _, secret := range secretsList.Items {
		for _, ref := range secret.OwnerReferences {
			if ref.UID == uiid {
				// i do  have a spec.secretName i compar with the secret Name
				if secretNameFromUser != "" {
					if secret.Name != secretNameFromUser {
						logger.Info("the secret named " + secret.Name + " will be deleted")
						deleteSecret(ctx, r, secret)
					} else {
						logger.Info("well done we found a secret named after the userResource.Spec.SecretName " + secret.Name)
						secretToTest = &secret
					}
					// else old case i dont have  a spec.SecretName i compar with the s3user.name
				} else {
					if secret.Name != userResource.Name {
						logger.Info("the secret named " + secret.Name + " will be deleted")
						deleteSecret(ctx, r, secret)
					} else {
						logger.Info("well done we found a secret named after the userResource.Name " + secret.Name)
						secretToTest = &secret
					}
				}

				// deux possibilite soit le s3user a un secretName soit on prend le name du s3User pour comparer les noms
				logger.Info("the Secret found is :", secretToTest.Name)
				// Do something with the secret
			}

		}
	}
	if secretToTest.Name == "" {
		logger.Info("Could not locate any secret ", "secret", userResource.Name)
		logger.Info("Secret associated to user not found, user will be deleted and recreated", "user", userResource.Name)
		err = r.S3Client.DeleteUser(userResource.Spec.AccessKey)
		if err != nil {
			logger.Error(err, "Could not delete user on S3 server", "user", userResource.Name)
			return r.setS3UserStatusConditionAndUpdate(ctx, userResource, "OperatorFailed", metav1.ConditionFalse, "S3UserDeletionFailed",
				fmt.Sprintf("Deletion of S3user %s on S3 server has failed", userResource.Name), err)
		}
		return handleS3UserCreation(ctx, userResource, r)
	}

	secretKeyValid, err := r.S3Client.CheckUserCredentialsValid(userResource.Name, userResource.Spec.AccessKey, string(secretToTest.Data["secretKey"]))
	if err != nil {
		logger.Error(err, "Something went wrong while checking user credential")
	}

	if !secretKeyValid {
		logger.Info("Secret for user is invalid")
		err = r.S3Client.DeleteUser(userResource.Spec.AccessKey)
		if err != nil {
			logger.Error(err, "Could not delete user on S3 server", "user", userResource.Name)
			return r.setS3UserStatusConditionAndUpdate(ctx, userResource, "OperatorFailed", metav1.ConditionFalse, "S3UserDeletionFailed",
				fmt.Sprintf("Deletion of S3user %s on S3 server has failed", userResource.Name), err)
		}
		return handleS3UserCreation(ctx, userResource, r)
	}

	logger.Info("Check user policies is correct")
	userPolicies, err := r.S3Client.GetUserPolicies(userResource.Spec.AccessKey)
	policyToDelete := []string{}
	policyToAdd := []string{}
	for _, policy := range userPolicies {
		policyFound := slices.Contains(userResource.Spec.Policies, policy)
		if !policyFound {
			logger.Info(fmt.Sprintf("S3User policy definition doesn't contains policy %s", policy))
			policyToDelete = append(policyToDelete, policy)
		}
	}

	for _, policy := range userResource.Spec.Policies {
		policyFound := slices.Contains(userPolicies, policy)
		if !policyFound {
			logger.Info(fmt.Sprintf("S3User policy definition must contains policy %s", policy))
			policyToAdd = append(policyToAdd, policy)
		}
	}

	if len(policyToDelete) > 0 {
		r.S3Client.RemovePoliciesFromUser(userResource.Spec.AccessKey, policyToDelete)
	}

	if len(policyToAdd) > 0 {
		r.S3Client.AddPoliciesToUser(userResource.Spec.AccessKey, policyToAdd)
	}

	logger.Info("User was reconcile without error")

	return r.setS3UserStatusConditionAndUpdate(ctx, userResource, "OperatorSucceeded", metav1.ConditionTrue, "S3UserUpdated",
		fmt.Sprintf("The user [%s] was updated according to its matching custom resource", userResource.Name), nil)
}

func handleS3UserDeletion(ctx context.Context, userResource *s3v1alpha1.S3User, r *S3UserReconciler, logger logr.Logger, err error) (reconcile.Result, error) {
	if controllerutil.ContainsFinalizer(userResource, userFinalizer) {
		// Run finalization logic for S3UserFinalizer. If the finalization logic fails, don't remove the finalizer so that we can retry during the next reconciliation.
		if err := r.finalizeS3User(userResource); err != nil {
			logger.Error(err, "an error occurred when attempting to finalize the user", "user", userResource.Name)
			return r.setS3UserStatusConditionAndUpdate(ctx, userResource, "OperatorFailed", metav1.ConditionFalse, "S3UserFinalizeFailed",
				fmt.Sprintf("An error occurred when attempting to delete user [%s]", userResource.Name), err)
		}

		//Remove userFinalizer. Once all finalizers have been removed, the object will be deleted.
		controllerutil.RemoveFinalizer(userResource, userFinalizer)
		err = r.Update(ctx, userResource)
		if err != nil {
			logger.Error(err, "Failed to remove finalizer.")
			return r.setS3UserStatusConditionAndUpdate(ctx, userResource, "OperatorFailed", metav1.ConditionFalse, "S3UserFinalizerRemovalFailed",
				fmt.Sprintf("An error occurred when attempting to remove the finalizer from user [%s]", userResource.Name), err)
		}
	}
	return ctrl.Result{}, nil
}

func handleS3UserCreation(ctx context.Context, userResource *s3v1alpha1.S3User, r *S3UserReconciler) (reconcile.Result, error) {
	logger := log.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(userResource, userFinalizer) {
		controllerutil.AddFinalizer(userResource, userFinalizer)
		err := r.Update(ctx, userResource)
		if err != nil {
			logger.Error(err, "Failed to add finalizer.")
			return r.setS3UserStatusConditionAndUpdate(ctx, userResource, "OperatorFailed", metav1.ConditionFalse, "S3UserResourceAddFinalizerFailed",
				fmt.Sprintf("Failed to add finalizer on userResource [%s] has failed", userResource.Name), err)
		}
	}

	secretKey, err := password.Generate(20, true, false, true)
	if err != nil {
		logger.Error(err, fmt.Sprintf("Fail to generate password for user %s", userResource.Name))
		return r.setS3UserStatusConditionAndUpdate(ctx, userResource, "OperatorFailed", metav1.ConditionFalse, "S3UserGeneratePasswordFailed",
			fmt.Sprintf("An error occurred when attempting to generate password for user [%s]", userResource.Name), err)
	}

	err = r.S3Client.CreateUser(userResource.Spec.AccessKey, secretKey)

	if err != nil {
		logger.Error(err, "an error occurred while creating user on S3 server", "user", userResource.Name)
		return r.setS3UserStatusConditionAndUpdate(ctx, userResource, "OperatorFailed", metav1.ConditionFalse, "S3UserCreationFailed",
			fmt.Sprintf("Creation of user %s on S3 instance has failed", userResource.Name), err)
	}

	policies := userResource.Spec.Policies
	if policies != nil {
		err := r.S3Client.AddPoliciesToUser(userResource.Spec.AccessKey, policies)
		if err != nil {
			logger.Error(err, "an error occurred while adding policy to user", "user", userResource.Name)
			return r.setS3UserStatusConditionAndUpdate(ctx, userResource, "OperatorFailed", metav1.ConditionFalse, "S3UserCreationFailed",
				fmt.Sprintf("Error while updating policies of user [%s] on S3 instance has failed", userResource.Name), err)
		}

	}

	// Define a new K8S Secrets
	secret, err := r.newSecretForCR(ctx, userResource, map[string][]byte{"accessKey": []byte(userResource.Spec.AccessKey), "secretKey": []byte(secretKey)})
	if err != nil {
		// Error while creating the Kubernetes secret - requeue the request.
		logger.Error(err, "Could not generate Kubernetes secret")
		return r.setS3UserStatusConditionAndUpdate(ctx, userResource, "OperatorFailed", metav1.ConditionFalse, "SecretGenerationFailed",
			fmt.Sprintf("Generation of k8s secrets [%s] has failed", userResource.Name), err)
	}

	// Check if this Secret already exists
	err = r.Get(ctx, types.NamespacedName{Name: secret.Name, Namespace: secret.Namespace}, &corev1.Secret{})
	if err != nil && errors.IsNotFound(err) {
		logger.Info("Creating a new Secret", "Secret.Namespace", secret.Namespace, "Secret.Name", secret.Name)
		err = r.Create(ctx, secret)
		if err != nil {
			logger.Error(err, "Could not create secret")
			return r.setS3UserStatusConditionAndUpdate(ctx, userResource, "OperatorFailed", metav1.ConditionFalse, "SecretCreationFailed",
				fmt.Sprintf("Creation of k8s secrets [%s] has failed", secret.Name), err)
		}
		return r.setS3UserStatusConditionAndUpdate(ctx, userResource, "OperatorSucceeded", metav1.ConditionTrue, "S3UserCreationSuccess",
			fmt.Sprintf("The secret [%s] was created according to its matching custom resource", secret.Name), nil)
	} else if err != nil {
		logger.Error(err, "Could not create secret")
		return r.setS3UserStatusConditionAndUpdate(ctx, userResource, "OperatorFailed", metav1.ConditionFalse, "SecretCreationFailed",
			fmt.Sprintf("Creation of k8s secrets [%s] has failed", secret.Name), err)
	} else {
		if r.OverrideExistingSecret {
			logger.Info(fmt.Sprintf("A secret with the name %s already exist. You choose to update existing secret.", userResource.Name))
			err = r.Update(ctx, secret)
			if err != nil {
				logger.Error(err, "Could not update secret")
				return r.setS3UserStatusConditionAndUpdate(ctx, userResource, "OperatorFailed", metav1.ConditionFalse, "SecretUpdateFailed",
					fmt.Sprintf("Update of k8s secrets [%s] has failed", secret.Name), err)
			}

			return r.setS3UserStatusConditionAndUpdate(ctx, userResource, "OperatorSucceeded", metav1.ConditionTrue, "S3UserCreationSuccess",
				fmt.Sprintf("The secret [%s] was created according to its matching custom resource", secret.Name), nil)
		} else {
			return r.setS3UserStatusConditionAndUpdate(ctx, userResource, "OperatorFailed", metav1.ConditionFalse, "SecretCreationFailed",
				fmt.Sprintf("A secret with the name %s already exist NOOP", secret.Name), nil)
		}
	}
}

// SetupWithManager sets up the controller with the Manager.*
func (r *S3UserReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&s3v1alpha1.S3User{}).
		Owns(&corev1.Secret{}).
		// TODO : implement a real strategy for event filtering ; for now just using the example from OpSDK doc
		// (https://sdk.operatorframework.io/docs/building-operators/golang/references/event-filtering/)
		WithEventFilter(predicate.Funcs{
			UpdateFunc: func(e event.UpdateEvent) bool {
				// Ignore updates to CR status in which case metadata.Generation does not change
				return e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration()
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

// newSecretForCR returns a secret with the same name/namespace as the CR. The secret will include all labels and
// annotations from the CR.
func (r *S3UserReconciler) newSecretForCR(ctx context.Context, userResource *s3v1alpha1.S3User, data map[string][]byte) (*corev1.Secret, error) {
	logger := log.FromContext(ctx)

	labels := map[string]string{}
	for k, v := range userResource.ObjectMeta.Labels {
		labels[k] = v
	}

	annotations := map[string]string{}
	for k, v := range userResource.ObjectMeta.Annotations {
		annotations[k] = v
	}

	secretName := userResource.Name
	if userResource.Spec.SecretName != "" {
		secretName = userResource.Spec.SecretName
	}
	logger.Info(" Name of the secret wanted " + secretName)
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
