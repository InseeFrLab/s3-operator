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
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	s3v1alpha1 "github.com/InseeFrLab/s3-operator/api/v1alpha1"
	"github.com/InseeFrLab/s3-operator/controllers/s3/factory"
)

// BucketReconciler reconciles a Bucket object
type BucketReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	S3Client       factory.S3Client
	BucketDeletion bool
}

//+kubebuilder:rbac:groups=s3.onyxia.sh,resources=buckets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=s3.onyxia.sh,resources=buckets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=s3.onyxia.sh,resources=buckets/finalizers,verbs=update

const bucketFinalizer = "s3.onyxia.sh/finalizer"

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *BucketReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	errorLogger := log.FromContext(ctx)
	logger := ctrl.Log.WithName("bucketReconcile")

	// Checking for bucket resource existence
	bucketResource := &s3v1alpha1.Bucket{}
	err := r.Get(ctx, req.NamespacedName, bucketResource)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("The Bucket custom resource has been removed ; as such the Bucket controller is NOOP.", "req.Name", req.Name)
			return ctrl.Result{}, nil
		}
		errorLogger.Error(err, "An error occurred when attempting to read the Bucket resource from the Kubernetes cluster")
		return ctrl.Result{}, err
	}

	// Managing bucket deletion with a finalizer
	// REF : https://sdk.operatorframework.io/docs/building-operators/golang/advanced-topics/#external-resources
	isMarkedForDeletion := bucketResource.GetDeletionTimestamp() != nil
	if isMarkedForDeletion {
		if controllerutil.ContainsFinalizer(bucketResource, bucketFinalizer) {
			// Run finalization logic for bucketFinalizer. If the
			// finalization logic fails, don't remove the finalizer so
			// that we can retry during the next reconciliation.
			if err := r.finalizeBucket(bucketResource); err != nil {
				// return ctrl.Result{}, err
				errorLogger.Error(err, "an error occurred when attempting to finalize the bucket", "bucket", bucketResource.Spec.Name)
				// return ctrl.Result{}, err
				return r.SetBucketStatusConditionAndUpdate(ctx, bucketResource, "OperatorFailed", metav1.ConditionFalse, "BucketFinalizeFailed",
					fmt.Sprintf("An error occurred when attempting to delete bucket [%s]", bucketResource.Spec.Name), err)
			}

			// Remove bucketFinalizer. Once all finalizers have been
			// removed, the object will be deleted.
			controllerutil.RemoveFinalizer(bucketResource, bucketFinalizer)
			err := r.Update(ctx, bucketResource)
			if err != nil {
				errorLogger.Error(err, "an error occurred when removing finalizer from bucket", "bucket", bucketResource.Spec.Name)
				// return ctrl.Result{}, err
				return r.SetBucketStatusConditionAndUpdate(ctx, bucketResource, "OperatorFailed", metav1.ConditionFalse, "BucketFinalizerRemovalFailed",
					fmt.Sprintf("An error occurred when attempting to remove the finalizer from bucket [%s]", bucketResource.Spec.Name), err)
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer for this CR
	if !controllerutil.ContainsFinalizer(bucketResource, bucketFinalizer) {
		controllerutil.AddFinalizer(bucketResource, bucketFinalizer)
		err = r.Update(ctx, bucketResource)
		if err != nil {
			errorLogger.Error(err, "an error occurred when adding finalizer from bucket", "bucket", bucketResource.Spec.Name)
			return r.SetBucketStatusConditionAndUpdate(ctx, bucketResource, "OperatorFailed", metav1.ConditionFalse, "BucketFinalizerAddFailed",
				fmt.Sprintf("An error occurred when attempting to add the finalizer from bucket [%s]", bucketResource.Spec.Name), err)
		}
	}

	// Bucket lifecycle management (other than deletion) starts here

	// Check bucket existence on the S3 server
	found, err := r.S3Client.BucketExists(bucketResource.Spec.Name)
	if err != nil {
		errorLogger.Error(err, "an error occurred while checking the existence of a bucket", "bucket", bucketResource.Spec.Name)
		return r.SetBucketStatusConditionAndUpdate(ctx, bucketResource, "OperatorFailed", metav1.ConditionFalse, "BucketExistenceCheckFailed",
			fmt.Sprintf("Checking existence of bucket [%s] from S3 instance has failed", bucketResource.Spec.Name), err)
	}

	// If the bucket does not exist, it is created based on the CR (with potential quotas and paths)
	if !found {

		// Bucket creation
		err = r.S3Client.CreateBucket(bucketResource.Spec.Name)
		if err != nil {
			errorLogger.Error(err, "an error occurred while creating a bucket", "bucket", bucketResource.Spec.Name)
			return r.SetBucketStatusConditionAndUpdate(ctx, bucketResource, "OperatorFailed", metav1.ConditionFalse, "BucketCreationFailed",
				fmt.Sprintf("Creation of bucket [%s] on S3 instance has failed", bucketResource.Spec.Name), err)
		}

		// Setting quotas
		err = r.S3Client.SetQuota(bucketResource.Spec.Name, bucketResource.Spec.Quota.Default)
		if err != nil {
			errorLogger.Error(err, "an error occurred while setting a quota on a bucket", "bucket", bucketResource.Spec.Name, "quota", bucketResource.Spec.Quota.Default)
			return r.SetBucketStatusConditionAndUpdate(ctx, bucketResource, "OperatorFailed", metav1.ConditionFalse, "SetQuotaOnBucketFailed",
				fmt.Sprintf("Setting a quota of [%v] on bucket [%s] has failed", bucketResource.Spec.Quota.Default, bucketResource.Spec.Name), err)
		}

		// Path creation
		for _, v := range bucketResource.Spec.Paths {
			err = r.S3Client.CreatePath(bucketResource.Spec.Name, v)
			if err != nil {
				errorLogger.Error(err, "an error occurred while creating a path on a bucket", "bucket", bucketResource.Spec.Name, "path", v)
				return r.SetBucketStatusConditionAndUpdate(ctx, bucketResource, "OperatorFailed", metav1.ConditionFalse, "CreatingPathOnBucketFailed",
					fmt.Sprintf("Creating the path [%s] on bucket [%s] has failed", v, bucketResource.Spec.Name), err)
			}
		}

		// The bucket creation, quota setting and path creation happened without any error
		return r.SetBucketStatusConditionAndUpdate(ctx, bucketResource, "OperatorSucceeded", metav1.ConditionTrue, "BucketCreated",
			fmt.Sprintf("The bucket [%s] was created with its quota and paths", bucketResource.Spec.Name), nil)
	}

	// If the bucket exists on the S3 server, then we need to compare it to
	// its corresponding custom resource, and update it in case the CR has changed.

	// Checking effectiveQuota existence on the bucket
	effectiveQuota, err := r.S3Client.GetQuota(bucketResource.Spec.Name)
	if err != nil {
		errorLogger.Error(err, "an error occurred while getting the quota for a bucket", "bucket", bucketResource.Spec.Name)
		return r.SetBucketStatusConditionAndUpdate(ctx, bucketResource, "OperatorFailed", metav1.ConditionFalse, "BucketQuotaCheckFailed",
			fmt.Sprintf("The check for a quota on bucket [%s] has failed", bucketResource.Spec.Name), err)
	}

	// If a quota exists, we check it versus the spec of the CR. In case they don't match,
	// we reset the quota using the value from CR ("override" is present, "default" if not)

	// Choosing between override / default
	quotaToResetTo := bucketResource.Spec.Quota.Override
	if quotaToResetTo == 0 {
		quotaToResetTo = bucketResource.Spec.Quota.Default
	}

	if effectiveQuota != quotaToResetTo {
		err = r.S3Client.SetQuota(bucketResource.Spec.Name, quotaToResetTo)
		if err != nil {
			errorLogger.Error(err, "an error occurred while resetting the quota for a bucket", "bucket", bucketResource.Spec.Name, "quotaToResetTo", quotaToResetTo)
			return r.SetBucketStatusConditionAndUpdate(ctx, bucketResource, "OperatorFailed", metav1.ConditionFalse, "BucketQuotaUpdateFailed",
				fmt.Sprintf("The quota update (%v => %v) on bucket [%s] has failed", effectiveQuota, quotaToResetTo, bucketResource.Spec.Name), err)
		}
	}

	// For every path on the custom resource's spec, we check the path actually
	// exists on the bucket on the S3 server, and create it if it doesn't
	// TODO ? : the way this is naively implemented, it's probably costly. Maybe
	// we can get the "effectiveBucket" (with its quota and paths) once at the beginning,
	// and iterate on this instead of interrogating the S3 server twice for every path.
	// But then again, some buckets will likely be filled with many objects outside the
	// scope of the CR, so getting all of them might be even more costly.
	for _, pathInCr := range bucketResource.Spec.Paths {
		pathExists, err := r.S3Client.PathExists(bucketResource.Spec.Name, pathInCr)
		if err != nil {
			errorLogger.Error(err, "an error occurred while checking a path's existence on a bucket", "bucket", bucketResource.Spec.Name, "path", pathInCr)
			return r.SetBucketStatusConditionAndUpdate(ctx, bucketResource, "OperatorFailed", metav1.ConditionFalse, "BucketPathCheckFailed",
				fmt.Sprintf("The check for path [%s] on bucket [%s] has failed", pathInCr, bucketResource.Spec.Name), err)
		}

		if !pathExists {
			err = r.S3Client.CreatePath(bucketResource.Spec.Name, pathInCr)
			if err != nil {
				errorLogger.Error(err, "an error occurred while creating a path on a bucket", "bucket", bucketResource.Spec.Name, "path", pathInCr)
				return r.SetBucketStatusConditionAndUpdate(ctx, bucketResource, "OperatorFailed", metav1.ConditionFalse, "BucketPathCreationFailed",
					fmt.Sprintf("The creation of path [%s] on bucket [%s] has failed", pathInCr, bucketResource.Spec.Name), err)
			}
		}
	}

	// The bucket reconciliation with its CR was succesful (or NOOP)
	return r.SetBucketStatusConditionAndUpdate(ctx, bucketResource, "OperatorSucceeded", metav1.ConditionTrue, "BucketUpdated",
		fmt.Sprintf("The bucket [%s] was updated according to its matching custom resource", bucketResource.Spec.Name), nil)

}

// SetupWithManager sets up the controller with the Manager.*
func (r *BucketReconciler) SetupWithManager(mgr ctrl.Manager) error {
	logger := ctrl.Log.WithName("bucketEventFilter")
	return ctrl.NewControllerManagedBy(mgr).
		For(&s3v1alpha1.Bucket{}).
		// REF : https://sdk.operatorframework.io/docs/building-operators/golang/references/event-filtering/
		WithEventFilter(predicate.Funcs{
			UpdateFunc: func(e event.UpdateEvent) bool {
				// Only reconcile if :
				// - Generation has changed
				//   or
				// - Of all Conditions matching the last generation, none is in status "True"
				// There is an implicit assumption that in such a case, the resource was once failing, but then transitioned
				// to a functional state. We use this ersatz because lastTransitionTime appears to not work properly - see also
				// comment in SetBucketStatusConditionAndUpdate() below.
				newBucket, _ := e.ObjectNew.(*s3v1alpha1.Bucket)

				// 1 - Identifying the most recent generation
				var maxGeneration int64 = 0
				for _, condition := range newBucket.Status.Conditions {
					if condition.ObservedGeneration > maxGeneration {
						maxGeneration = condition.ObservedGeneration
					}
				}
				// 2 - Checking one of the conditions in most recent generation is True
				conditionTrueInLastGeneration := false
				for _, condition := range newBucket.Status.Conditions {
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

func (r *BucketReconciler) finalizeBucket(bucketResource *s3v1alpha1.Bucket) error {
	if r.BucketDeletion {
		return r.S3Client.DeleteBucket(bucketResource.Spec.Name)
	}
	return nil
}

func (r *BucketReconciler) SetBucketStatusConditionAndUpdate(ctx context.Context, bucketResource *s3v1alpha1.Bucket, conditionType string, status metav1.ConditionStatus, reason string, message string, srcError error) (ctrl.Result, error) {
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
	meta.SetStatusCondition(&bucketResource.Status.Conditions,
		metav1.Condition{
			Type:   conditionType,
			Status: status,
			Reason: reason,
			// LastTransitionTime: metav1.NewTime(time.Now()),
			Message:            message,
			ObservedGeneration: bucketResource.GetGeneration(),
		})

	err := r.Status().Update(ctx, bucketResource)
	if err != nil {
		logger.Error(err, "an error occurred while updating the status of the bucket resource")
		return ctrl.Result{}, utilerrors.NewAggregate([]error{err, srcError})
	}
	return ctrl.Result{}, srcError
}
