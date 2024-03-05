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
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	s3v1alpha1 "github.com/phlg/s3-operator-downgrade/api/v1alpha1"
	"github.com/phlg/s3-operator-downgrade/controllers/s3/factory"

	utils "github.com/phlg/s3-operator-downgrade/controllers/utils"
)

// UserReconciler reconciles a User object
type UserReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	S3Client factory.S3Client
}

//+kubebuilder:rbac:groups=s3.onyxia.sh,resources=user,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=s3.onyxia.sh,resources=user/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=s3.onyxia.sh,resources=user/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *UserReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	userResource := &s3v1alpha1.User{}
	err := r.Get(ctx, req.NamespacedName, userResource)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info(fmt.Sprintf("User CRD %s has been removed. NOOP", req.Name))
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Check user existence on the S3 server
	found, err := r.S3Client.UserExist(userResource.Spec.Name)
	if err != nil {
		logger.Error(err, "an error occurred while checking the existence of a user", "user", userResource.Spec.Name)
		return r.SetUserStatusConditionAndUpdate(ctx, userResource, "OperatorFailed", metav1.ConditionFalse, "UserExistenceCheckFailed",
			fmt.Sprintf("Checking existence of user [%s] from S3 instance has failed", userResource.Spec.Name), err)
	}

	// If the user does not exist, it is created based on the CR
	if !found {
		// User creation
		username := userResource.Spec.Name
		password := userResource.Spec.Password
		if (password) == "" {
			password = utils.GeneratePassword(10, true, true, true)
		}
		err = r.S3Client.CreateUser(username, password)
		if err != nil {
			logger.Error(err, "an error occurred while creating a user", "user", userResource.Spec.Name)
			return r.SetUserStatusConditionAndUpdate(ctx, userResource, "OperatorFailed", metav1.ConditionFalse, "UserCreationFailed",
				fmt.Sprintf("Creation of user [%s] on S3 instance has failed", userResource.Spec.Name), err)
		}
		policies := userResource.Spec.Policies
		if policies != nil {
			r.S3Client.AddPoliciesToUser(username, policies)
		}
		groups := userResource.Spec.Groups
		if groups != nil {
			r.S3Client.AddGroupsToUser(username, groups)
		}

		// The user creation happened without any error
		return r.SetUserStatusConditionAndUpdate(ctx, userResource, "OperatorSucceeded", metav1.ConditionTrue, "UserCreated",
			fmt.Sprintf("The user [%s] was created", userResource.Spec.Name), nil)
	}

	// The user reconciliation with its CR was succesful (or NOOP)
	return r.SetUserStatusConditionAndUpdate(ctx, userResource, "OperatorSucceeded", metav1.ConditionTrue, "UserUpdated",
		fmt.Sprintf("The user [%s] was updated according to its matching custom resource", userResource.Spec.Name), nil)

}

// SetupWithManager sets up the controller with the Manager.*
func (r *UserReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&s3v1alpha1.User{}).
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

func (r *UserReconciler) SetUserStatusConditionAndUpdate(ctx context.Context, userResource *s3v1alpha1.User, conditionType string, status metav1.ConditionStatus, reason string, message string, srcError error) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	meta.SetStatusCondition(&userResource.Status.Conditions,
		metav1.Condition{
			Type:               conditionType,
			Status:             status,
			Reason:             reason,
			LastTransitionTime: metav1.NewTime(time.Now()),
			Message:            message,
			ObservedGeneration: userResource.GetGeneration(),
		})

	err := r.Status().Update(ctx, userResource)
	if err != nil {
		logger.Error(err, "an error occurred while updating the status of the user resource")
		return ctrl.Result{}, utilerrors.NewAggregate([]error{err, srcError})
	}
	return ctrl.Result{}, srcError
}
