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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	s3v1alpha1 "github.com/InseeFrLab/s3-operator/api/v1alpha1"
	"github.com/InseeFrLab/s3-operator/controllers/s3/factory"
	"github.com/InseeFrLab/s3-operator/controllers/utils"
)

// PathReconciler reconciles a Path object
type PathReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	S3Client     factory.S3Client
	PathDeletion bool
}

//+kubebuilder:rbac:groups=s3.onyxia.sh,resources=paths,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=s3.onyxia.sh,resources=paths/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=s3.onyxia.sh,resources=paths/finalizers,verbs=update

const pathFinalizer = "s3.onyxia.sh/finalizer"

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *PathReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Checking for path resource existence
	pathResource := &s3v1alpha1.Path{}
	err := r.Get(ctx, req.NamespacedName, pathResource)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("The Path custom resource has been removed ; as such the Path controller is NOOP.", "req.Name", req.Name)
			return ctrl.Result{}, nil
		}
		logger.Error(err, "An error occurred when attempting to read the Path resource from the Kubernetes cluster")
		return ctrl.Result{}, err
	}

	// Managing path deletion with a finalizer
	// REF : https://sdk.operatorframework.io/docs/building-operators/golang/advanced-topics/#external-resources
	isMarkedForDeletion := pathResource.GetDeletionTimestamp() != nil
	if isMarkedForDeletion {
		if controllerutil.ContainsFinalizer(pathResource, pathFinalizer) {
			// Run finalization logic for pathFinalizer. If the
			// finalization logic fails, don't remove the finalizer so
			// that we can retry during the next reconciliation.
			if err := r.finalizePath(pathResource); err != nil {
				// return ctrl.Result{}, err
				logger.Error(err, "an error occurred when attempting to finalize the path", "path", pathResource.Name)
				// return ctrl.Result{}, err
				return r.SetPathStatusConditionAndUpdate(ctx, pathResource, "OperatorFailed", metav1.ConditionFalse, "PathFinalizeFailed",
					fmt.Sprintf("An error occurred when attempting to delete path [%s]", pathResource.Name), err)
			}

			// Remove pathFinalizer. Once all finalizers have been
			// removed, the object will be deleted.
			controllerutil.RemoveFinalizer(pathResource, pathFinalizer)
			err := r.Update(ctx, pathResource)
			if err != nil {
				logger.Error(err, "an error occurred when removing finalizer from path", "path", pathResource.Name)
				// return ctrl.Result{}, err
				return r.SetPathStatusConditionAndUpdate(ctx, pathResource, "OperatorFailed", metav1.ConditionFalse, "PathFinalizerRemovalFailed",
					fmt.Sprintf("An error occurred when attempting to remove the finalizer from path [%s]", pathResource.Name), err)
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer for this CR
	if !controllerutil.ContainsFinalizer(pathResource, pathFinalizer) {
		controllerutil.AddFinalizer(pathResource, pathFinalizer)
		err = r.Update(ctx, pathResource)
		if err != nil {
			logger.Error(err, "an error occurred when adding finalizer from path", "path", pathResource.Name)
			// return ctrl.Result{}, err
			return r.SetPathStatusConditionAndUpdate(ctx, pathResource, "OperatorFailed", metav1.ConditionFalse, "PathFinalizerAddFailed",
				fmt.Sprintf("An error occurred when attempting to add the finalizer from path [%s]", pathResource.Name), err)
		}
	}

	// Path lifecycle management (other than deletion) starts here

	// Check bucket existence on the S3 server
	bucketFound, err := r.S3Client.BucketExists(pathResource.Spec.BucketName)
	if err != nil {
		logger.Error(err, "an error occurred while checking the existence of a bucket", "bucket", pathResource.Spec.BucketName)
		return r.SetPathStatusConditionAndUpdate(ctx, pathResource, "OperatorFailed", metav1.ConditionFalse, "BucketExistenceCheckFailed",
			fmt.Sprintf("Checking existence of bucket [%s] from S3 instance has failed", pathResource.Spec.BucketName), err)
	}

	// If bucket does not exist, the Path CR should be in a failing state
	if !bucketFound {
		errorBucketNotFound := fmt.Errorf("the path CR %s references a non-existing bucket : %s", pathResource.Name, pathResource.Spec.BucketName)
		logger.Error(errorBucketNotFound, errorBucketNotFound.Error())
		return r.SetPathStatusConditionAndUpdate(ctx, pathResource, "OperatorFailed", metav1.ConditionFalse, "ReferencingNonExistingBucket",
			fmt.Sprintf("The Path CR [%s] references a non-existing bucket [%s]", pathResource.Name, pathResource.Spec.BucketName), errorBucketNotFound)
	}

	// If the bucket exists, proceed to create or recreate the referenced paths
	// For every path on the custom resource's spec, we check the path actually
	// exists on the bucket on the S3 server, and create it if it doesn't
	// TODO ? : the way this is naively implemented, it's probably costly. Maybe
	// we can get the "effectiveBucket" (with its quota and paths) once at the beginning,
	// and iterate on this instead of interrogating the S3 server twice for every path.
	// But then again, some buckets will likely be filled with many objects outside the
	// scope of the CR, so getting all of them might be even more costly.
	for _, pathInCr := range pathResource.Spec.Paths {
		pathExists, err := r.S3Client.PathExists(pathResource.Spec.BucketName, pathInCr)
		if err != nil {
			logger.Error(err, "an error occurred while checking a path's existence on a bucket", "bucket", pathResource.Spec.BucketName, "path", pathInCr)
			return r.SetPathStatusConditionAndUpdate(ctx, pathResource, "OperatorFailed", metav1.ConditionFalse, "PathCheckFailed",
				fmt.Sprintf("The check for path [%s] on bucket [%s] has failed", pathInCr, pathResource.Spec.BucketName), err)
		}

		if !pathExists {
			err = r.S3Client.CreatePath(pathResource.Spec.BucketName, pathInCr)
			if err != nil {
				logger.Error(err, "an error occurred while creating a path on a bucket", "bucket", pathResource.Spec.BucketName, "path", pathInCr)
				return r.SetPathStatusConditionAndUpdate(ctx, pathResource, "OperatorFailed", metav1.ConditionFalse, "PathCreationFailed",
					fmt.Sprintf("The creation of path [%s] on bucket [%s] has failed", pathInCr, pathResource.Spec.BucketName), err)
			}
		}
	}

	// The bucket reconciliation with its CR was succesful (or NOOP)
	return r.SetPathStatusConditionAndUpdate(ctx, pathResource, "OperatorSucceeded", metav1.ConditionTrue, "PathsCreated",
		fmt.Sprintf("The paths were created according to the specs of the [%s] CR", pathResource.Name), nil)

}

// SetupWithManager sets up the controller with the Manager.
func (r *PathReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&s3v1alpha1.Path{}).
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

func (r *PathReconciler) finalizePath(pathResource *s3v1alpha1.Path) error {
	logger := log.Log.WithValues("controller", "path")
	if r.PathDeletion {
		var failedPaths []string = make([]string, 0)
		for _, path := range pathResource.Spec.Paths {

			pathExists, err := r.S3Client.PathExists(pathResource.Spec.BucketName, path)
			if err != nil {
				logger.Error(err, "finalize : an error occurred while checking a path's existence on a bucket", "bucket", pathResource.Spec.BucketName, "path", path)
			}

			if pathExists {
				err = r.S3Client.DeletePath(pathResource.Spec.BucketName, path)
				if err != nil {
					failedPaths = append(failedPaths, path)
				}
			}
		}

		if len(failedPaths) > 0 {
			return fmt.Errorf("at least one path couldn't be removed from S3 backend %+q", failedPaths)
		}
	}
	return nil
}

func (r *PathReconciler) SetPathStatusConditionAndUpdate(ctx context.Context, pathResource *s3v1alpha1.Path, conditionType string, status metav1.ConditionStatus, reason string, message string, srcError error) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// We moved away from meta.SetStatusCondition, as the implementation did not allow for updating
	// lastTransitionTime if a Condition (as identified by Reason instead of Type) was previously
	// obtained and updated to again.
	pathResource.Status.Conditions = utils.UpdateConditions(pathResource.Status.Conditions, metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		LastTransitionTime: metav1.NewTime(time.Now()),
		Message:            message,
		ObservedGeneration: pathResource.GetGeneration(),
	})

	err := r.Status().Update(ctx, pathResource)
	if err != nil {
		logger.Error(err, "an error occurred while updating the status of the path resource")
		return ctrl.Result{}, utilerrors.NewAggregate([]error{err, srcError})
	}
	return ctrl.Result{}, srcError
}
