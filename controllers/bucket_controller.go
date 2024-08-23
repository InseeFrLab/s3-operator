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

	s3v1alpha1 "github.com/InseeFrLab/s3-operator/api/v1alpha1"
	controllerhelpers "github.com/InseeFrLab/s3-operator/internal/controllerhelper"

	utils "github.com/InseeFrLab/s3-operator/internal/utils"
	k8sapierrors "k8s.io/apimachinery/pkg/api/errors"
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
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// BucketReconciler reconciles a Bucket object
type BucketReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	ReconcilePeriod time.Duration
}

const bucketFinalizer = "s3.onyxia.sh/finalizer"

//+kubebuilder:rbac:groups=s3.onyxia.sh,resources=buckets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=s3.onyxia.sh,resources=buckets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=s3.onyxia.sh,resources=buckets/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *BucketReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Checking for bucket resource existence
	bucketResource := &s3v1alpha1.Bucket{}
	err := r.Get(ctx, req.NamespacedName, bucketResource)
	if err != nil {
		if k8sapierrors.IsNotFound(err) {
			logger.Info("The Bucket custom resource has been removed ; as such the Bucket controller is NOOP.", "req.Name", req.Name)
			return ctrl.Result{}, nil
		}
		logger.Error(err, "An error occurred when attempting to read the Bucket resource from the Kubernetes cluster")
		return ctrl.Result{}, err
	}

	// Add finalizer for this CR
	if !controllerutil.ContainsFinalizer(bucketResource, bucketFinalizer) {
		logger.Info("Adding finalizer on ressource")
		controllerutil.AddFinalizer(bucketResource, bucketFinalizer)
		err = r.Update(ctx, bucketResource)
		if err != nil {
			logger.Error(err, "an error occurred when adding finalizer from bucket", "bucket", bucketResource.Spec.Name)
			return r.SetBucketStatusConditionAndUpdate(ctx, bucketResource, "OperatorFailed", metav1.ConditionFalse, "BucketFinalizerAddFailed",
				fmt.Sprintf("An error occurred when attempting to add the finalizer from bucket [%s]", bucketResource.Spec.Name), err)
		}

		// Let's re-fetch the S3Instance Custom Resource after adding the finalizer
		// so that we have the latest state of the resource on the cluster and we will avoid
		// raise the issue "the object has been modified, please apply
		// your changes to the latest version and try again" which would re-trigger the reconciliation
		// if we try to update it again in the following operations
		if err := r.Get(ctx, req.NamespacedName, bucketResource); err != nil {
			logger.Error(err, "Failed to re-fetch bucketResource", "NamespacedName", req.NamespacedName.String())
			return ctrl.Result{}, err
		}
	}

	// // Managing bucket deletion with a finalizer
	// // REF : https://sdk.operatorframework.io/docs/building-operators/golang/advanced-topics/#external-resources
	if bucketResource.GetDeletionTimestamp() != nil {
		logger.Info("bucketResource have been marked for deletion")
		return r.handleDeletion(ctx, req, bucketResource)
	}

	return r.handleReconciliation(ctx, bucketResource)

}

func (r *BucketReconciler) handleReconciliation(ctx context.Context, bucketResource *s3v1alpha1.Bucket) (reconcile.Result, error) {
	logger := log.FromContext(ctx)

	s3Client, err := controllerhelpers.GetS3ClientForRessource(ctx, r.Client, bucketResource.Name, bucketResource.Namespace, bucketResource.Spec.S3InstanceRef)
	if err != nil {
		logger.Error(err, "an error occurred while getting s3Client")
		return r.SetBucketStatusConditionAndUpdate(ctx, bucketResource, "OperatorFailed", metav1.ConditionFalse, "FailedS3Client",
			"Unknown error occured while getting bucket", err)
	}

	// Bucket lifecycle management (other than deletion) starts here
	// Check bucket existence on the S3 server
	found, err := s3Client.BucketExists(bucketResource.Spec.Name)
	if err != nil {
		logger.Error(err, "an error occurred while checking the existence of a bucket", "bucket", bucketResource.Spec.Name)
		return r.SetBucketStatusConditionAndUpdate(ctx, bucketResource, "OperatorFailed", metav1.ConditionFalse, "BucketExistenceCheckFailed",
			fmt.Sprintf("Checking existence of bucket [%s] from S3 instance has failed", bucketResource.Spec.Name), err)
	}

	// If the bucket does not exist, it is created based on the CR (with potential quotas and paths)
	if !found {
		return r.handleBucketCreation(ctx, bucketResource)
	}

	logger.Info("this bucket already exists and will be reconciled")
	return r.handleBucketUpdate(ctx, bucketResource)

}

func (r *BucketReconciler) handleBucketUpdate(ctx context.Context, bucketResource *s3v1alpha1.Bucket) (reconcile.Result, error) {
	logger := log.FromContext(ctx)

	s3Client, err := controllerhelpers.GetS3ClientForRessource(ctx, r.Client, bucketResource.Name, bucketResource.Namespace, bucketResource.Spec.S3InstanceRef)
	if err != nil {
		logger.Error(err, "an error occurred while getting s3Client")
		return r.SetBucketStatusConditionAndUpdate(ctx, bucketResource, "OperatorFailed", metav1.ConditionFalse, "FailedS3Client",
			"Unknown error occured while getting bucket", err)
	}

	// If the bucket exists on the S3 server, then we need to compare it to
	// its corresponding custom resource, and update it in case the CR has changed.

	// Checking effectiveQuota existence on the bucket
	effectiveQuota, err := s3Client.GetQuota(bucketResource.Spec.Name)
	if err != nil {
		logger.Error(err, "an error occurred while getting the quota for a bucket", "bucket", bucketResource.Spec.Name)
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
		err = s3Client.SetQuota(bucketResource.Spec.Name, quotaToResetTo)
		if err != nil {
			logger.Error(err, "an error occurred while resetting the quota for a bucket", "bucket", bucketResource.Spec.Name, "quotaToResetTo", quotaToResetTo)
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
		pathExists, err := s3Client.PathExists(bucketResource.Spec.Name, pathInCr)
		if err != nil {
			logger.Error(err, "an error occurred while checking a path's existence on a bucket", "bucket", bucketResource.Spec.Name, "path", pathInCr)
			return r.SetBucketStatusConditionAndUpdate(ctx, bucketResource, "OperatorFailed", metav1.ConditionFalse, "BucketPathCheckFailed",
				fmt.Sprintf("The check for path [%s] on bucket [%s] has failed", pathInCr, bucketResource.Spec.Name), err)
		}

		if !pathExists {
			err = s3Client.CreatePath(bucketResource.Spec.Name, pathInCr)
			if err != nil {
				logger.Error(err, "an error occurred while creating a path on a bucket", "bucket", bucketResource.Spec.Name, "path", pathInCr)
				return r.SetBucketStatusConditionAndUpdate(ctx, bucketResource, "OperatorFailed", metav1.ConditionFalse, "BucketPathCreationFailed",
					fmt.Sprintf("The creation of path [%s] on bucket [%s] has failed", pathInCr, bucketResource.Spec.Name), err)
			}
		}
	}

	// The bucket reconciliation with its CR was succesful (or NOOP)
	return r.SetBucketStatusConditionAndUpdate(ctx, bucketResource, "OperatorSucceeded", metav1.ConditionTrue, "BucketUpdated",
		fmt.Sprintf("The bucket [%s] was updated according to its matching custom resource", bucketResource.Spec.Name), nil)
}

func (r *BucketReconciler) handleBucketCreation(ctx context.Context, bucketResource *s3v1alpha1.Bucket) (reconcile.Result, error) {
	logger := log.FromContext(ctx)

	s3Client, err := controllerhelpers.GetS3ClientForRessource(ctx, r.Client, bucketResource.Name, bucketResource.Namespace, bucketResource.Spec.S3InstanceRef)
	if err != nil {
		logger.Error(err, "an error occurred while getting s3Client")
		return r.SetBucketStatusConditionAndUpdate(ctx, bucketResource, "OperatorFailed", metav1.ConditionFalse, "FailedS3Client",
			"Unknown error occured while getting bucket", err)
	}

	// Bucket creation
	err = s3Client.CreateBucket(bucketResource.Spec.Name)
	if err != nil {
		logger.Error(err, "an error occurred while creating a bucket", "bucket", bucketResource.Spec.Name)
		return r.SetBucketStatusConditionAndUpdate(ctx, bucketResource, "OperatorFailed", metav1.ConditionFalse, "BucketCreationFailed",
			fmt.Sprintf("Creation of bucket [%s] on S3 instance has failed", bucketResource.Spec.Name), err)
	}

	// Setting quotas
	err = s3Client.SetQuota(bucketResource.Spec.Name, bucketResource.Spec.Quota.Default)
	if err != nil {
		logger.Error(err, "an error occurred while setting a quota on a bucket", "bucket", bucketResource.Spec.Name, "quota", bucketResource.Spec.Quota.Default)
		return r.SetBucketStatusConditionAndUpdate(ctx, bucketResource, "OperatorFailed", metav1.ConditionFalse, "SetQuotaOnBucketFailed",
			fmt.Sprintf("Setting a quota of [%v] on bucket [%s] has failed", bucketResource.Spec.Quota.Default, bucketResource.Spec.Name), err)
	}

	// Path creation
	for _, v := range bucketResource.Spec.Paths {
		err = s3Client.CreatePath(bucketResource.Spec.Name, v)
		if err != nil {
			logger.Error(err, "an error occurred while creating a path on a bucket", "bucket", bucketResource.Spec.Name, "path", v)
			return r.SetBucketStatusConditionAndUpdate(ctx, bucketResource, "OperatorFailed", metav1.ConditionFalse, "CreatingPathOnBucketFailed",
				fmt.Sprintf("Creating the path [%s] on bucket [%s] has failed", v, bucketResource.Spec.Name), err)
		}
	}

	// The bucket creation, quota setting and path creation happened without any error
	return r.SetBucketStatusConditionAndUpdate(ctx, bucketResource, "OperatorSucceeded", metav1.ConditionTrue, "BucketCreated",
		fmt.Sprintf("The bucket [%s] was created with its quota and paths", bucketResource.Spec.Name), nil)
}

func (r *BucketReconciler) handleDeletion(ctx context.Context, req reconcile.Request, bucketResource *s3v1alpha1.Bucket) (reconcile.Result, error) {
	logger := log.FromContext(ctx)

	if controllerutil.ContainsFinalizer(bucketResource, bucketFinalizer) {

		if err := r.finalizeBucket(ctx, bucketResource); err != nil {

			logger.Error(err, "an error occurred when attempting to finalize the bucket", "bucket", bucketResource.Spec.Name)

			return r.SetBucketStatusConditionAndUpdate(ctx, bucketResource, "OperatorFailed", metav1.ConditionFalse, "BucketFinalizeFailed",
				fmt.Sprintf("An error occurred when attempting to delete bucket [%s]", bucketResource.Spec.Name), err)
		}

		if ok := controllerutil.RemoveFinalizer(bucketResource, bucketFinalizer); !ok {
			logger.Info("Failed to remove finalizer for bucketResource", "NamespacedName", req.NamespacedName.String())
			return ctrl.Result{Requeue: true}, nil
		}

		// Let's re-fetch the S3Instance Custom Resource after removing the finalizer
		// so that we have the latest state of the resource on the cluster and we will avoid
		// raise the issue "the object has been modified, please apply
		// your changes to the latest version and try again" which would re-trigger the reconciliation
		// if we try to update it again in the following operations
		if err := r.Update(ctx, bucketResource); err != nil {
			logger.Error(err, "Failed to remove finalizer for bucketResource", "NamespacedName", req.NamespacedName.String())
			return ctrl.Result{}, err
		}

	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.*
func (r *BucketReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&s3v1alpha1.Bucket{}).
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

func (r *BucketReconciler) finalizeBucket(ctx context.Context, bucketResource *s3v1alpha1.Bucket) error {
	logger := log.FromContext(ctx)

	s3Client, err := controllerhelpers.GetS3ClientForRessource(ctx, r.Client, bucketResource.Name, bucketResource.Namespace, bucketResource.Spec.S3InstanceRef)
	if err != nil {
		logger.Error(err, "an error occurred while getting s3Client")
		return err
	}
	if s3Client.GetConfig().BucketDeletionEnabled {
		return s3Client.DeleteBucket(bucketResource.Spec.Name)
	}
	return nil
}

func (r *BucketReconciler) SetBucketStatusConditionAndUpdate(ctx context.Context, bucketResource *s3v1alpha1.Bucket, conditionType string, status metav1.ConditionStatus, reason string, message string, srcError error) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// We moved away from meta.SetStatusCondition, as the implementation did not allow for updating
	// lastTransitionTime if a Condition (as identified by Reason instead of Type) was previously
	// obtained and updated to again.
	bucketResource.Status.Conditions = utils.UpdateConditions(bucketResource.Status.Conditions, metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		LastTransitionTime: metav1.NewTime(time.Now()),
		Message:            message,
		ObservedGeneration: bucketResource.GetGeneration(),
	})

	err := r.Status().Update(ctx, bucketResource)
	if err != nil {
		logger.Error(err, "an error occurred while updating the status of the bucket resource")
		return ctrl.Result{}, utilerrors.NewAggregate([]error{err, srcError})
	}
	return ctrl.Result{RequeueAfter: r.ReconcilePeriod}, srcError
}
