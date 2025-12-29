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
	"time"

	"github.com/InseeFrLab/s3-operator/internal/helpers"
	s3factory "github.com/InseeFrLab/s3-operator/internal/s3/factory"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	s3v1alpha1 "github.com/InseeFrLab/s3-operator/api/v1alpha1"
)

// +kubebuilder:rbac:groups=s3.onyxia.sh,resources=s3users,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=s3.onyxia.sh,resources=s3users/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=s3.onyxia.sh,resources=s3users/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups="",resources=secrets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=secrets/finalizers,verbs=update

// S3UserReconciler reconciles a S3User object
type S3UserReconciler struct {
	client.Client
	Scheme                  *runtime.Scheme
	OverrideExistingSecret  bool
	ReadExistingSecret      bool
	ReconcilePeriod         time.Duration
	S3factory               s3factory.S3Factory
	ControllerHelper        *helpers.ControllerHelper
	S3Instancehelper        *helpers.S3InstanceHelper
	PasswordGeneratorHelper *helpers.PasswordGenerator
}

// SetupWithManager sets up the controller with the Manager.*
func (r *S3UserReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// filterLogger := ctrl.Log.WithName("filterEvt")
	return ctrl.NewControllerManagedBy(mgr).
		For(&s3v1alpha1.S3User{}).
		Named("s3User").
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
