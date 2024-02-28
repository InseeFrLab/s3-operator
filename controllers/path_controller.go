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

	s3v1alpha1 "github.com/InseeFrLab/s3-operator/api/v1alpha1"
	"github.com/InseeFrLab/s3-operator/controllers/s3/factory"
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

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *PathReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	pathResource := &s3v1alpha1.Path{}
	err := r.Get(ctx, req.NamespacedName, pathResource)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info(fmt.Sprintf("Path CRD %s has been removed. NOOP", req.Name))
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Check bucket existence on the S3 server
	bucketFound, err := r.S3Client.BucketExists(pathResource.Spec.BucketName)
	if err != nil {
		logger.Error(err, "an error occurred while checking the existence of a bucket", "bucket", pathResource.Spec.BucketName)
		return r.SetPathStatusConditionAndUpdate(ctx, pathResource, "OperatorFailed", metav1.ConditionFalse, "BucketExistenceCheckFailed",
			fmt.Sprintf("Checking existence of bucket [%s] from S3 instance has failed", pathResource.Spec.BucketName), err)
	}

	// If bucket does not exist, the Path CR should be in a failing state
	if !bucketFound {
		logger.Error(err, "the path CR references a non-existing bucket", "pathCr", pathResource.Name, "bucket", pathResource.Spec.BucketName)
		return r.SetPathStatusConditionAndUpdate(ctx, pathResource, "OperatorFailed", metav1.ConditionFalse, "ReferencingNonExistingBucket",
			fmt.Sprintf("The Path CR [%s] references a non-existing bucket [%s]", pathResource.Name, pathResource.Spec.BucketName), err)
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
	logger := ctrl.Log.WithName("eventFilter")
	return ctrl.NewControllerManagedBy(mgr).
		For(&s3v1alpha1.Path{}).
		WithEventFilter(predicate.Funcs{
			UpdateFunc: func(e event.UpdateEvent) bool {
				// Only reconcile if :
				// - Generation has changed
				//   or
				// - Of all Conditions matching the last generation, none is in status "True"
				// There is an implicit assumption that in such a case, the resource was once failing, but then transitioned
				// to a functional state. We use this ersatz because lastTransitionTime appears to not work properly - see also
				// comment in SetPathStatusConditionAndUpdate() below.
				newPath, _ := e.ObjectNew.(*s3v1alpha1.Path)

				// 1 - Identifying the most recent generation
				var maxGeneration int64 = 0
				for _, condition := range newPath.Status.Conditions {
					if condition.ObservedGeneration > maxGeneration {
						maxGeneration = condition.ObservedGeneration
					}
				}
				// 2 - Checking one of the conditions in most recent generation is True
				conditionTrueInLastGeneration := false
				for _, condition := range newPath.Status.Conditions {
					if condition.ObservedGeneration == maxGeneration && condition.Status == metav1.ConditionTrue {
						conditionTrueInLastGeneration = true
					}
				}
				predicate := e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration() || !conditionTrueInLastGeneration
				if !predicate {
					logger.Info("reconcile update event is filtered out", "resource", e.ObjectNew.GetName())
				}
				return predicate
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				// Evaluates to false if the object has been confirmed deleted.
				logger.Info("reconcile delete event is filtered out", "resource", e.Object.GetName())
				return !e.DeleteStateUnknown
			},
		}).
		WithOptions(controller.Options{MaxConcurrentReconciles: 10}).
		Complete(r)
}

func (r *PathReconciler) SetPathStatusConditionAndUpdate(ctx context.Context, pathResource *s3v1alpha1.Path, conditionType string, status metav1.ConditionStatus, reason string, message string, srcError error) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// It would seem LastTransitionTime does not work as intended (our understanding of the intent coming from this :
	// https://pkg.go.dev/k8s.io/apimachinery@v0.28.3/pkg/api/meta#SetStatusCondition). Whether we set the
	// date manually or leave it out to have default behavior, the lastTransitionTime is NOT updated if the CR
	// had that condition at least once in the past.
	// For instance, with the following updates to a CR :
	//	- gen 1 : condition type = A
	//	- gen 2 : condition type = B
	//	- gen 3 : condition type = A again
	// Then the condition with type A in CR Status will still have the lastTransitionTime dating back to gen 1.
	// Because of this, lastTransitionTime cannot be reliably used to determine current state, which in turn had
	// us turn to a less than ideal event filter (see above in SetupWithManager())
	meta.SetStatusCondition(&pathResource.Status.Conditions,
		metav1.Condition{
			Type:   conditionType,
			Status: status,
			Reason: reason,
			// LastTransitionTime: metav1.NewTime(time.Now()),
			Message:            message,
			ObservedGeneration: pathResource.GetGeneration(),
		})

	err := r.Status().Update(ctx, pathResource)
	if err != nil {
		logger.Error(err, "an error occurred while updating the status of the bucket resource")
		return ctrl.Result{}, utilerrors.NewAggregate([]error{err, srcError})
	}
	return ctrl.Result{}, srcError
}
