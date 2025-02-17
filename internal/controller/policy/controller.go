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

package policy_controller

import (
	"time"

	"github.com/InseeFrLab/s3-operator/internal/helpers"
	s3factory "github.com/InseeFrLab/s3-operator/pkg/s3/factory"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	s3v1alpha1 "github.com/InseeFrLab/s3-operator/api/v1alpha1"
)

// +kubebuilder:rbac:groups=s3.onyxia.sh,resources=policies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=s3.onyxia.sh,resources=policies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=s3.onyxia.sh,resources=policies/finalizers,verbs=update

// PolicyReconciler reconciles a Policy object
type PolicyReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	ReconcilePeriod  time.Duration
	S3factory        s3factory.S3Factory
	ControllerHelper *helpers.ControllerHelper
	S3Instancehelper *helpers.S3InstanceHelper
}

// SetupWithManager sets up the controller with the Manager.
func (r *PolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&s3v1alpha1.Policy{}).
		// REF : https://sdk.operatorframework.io/docs/building-operators/golang/references/event-filtering/
		WithEventFilter(predicate.Funcs{
			UpdateFunc: func(e event.UpdateEvent) bool {
				// Only reconcile if generation has changed
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
