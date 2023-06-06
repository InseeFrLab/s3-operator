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

	s3v1alpha1 "github.com/inseefrlab/s3-operator/api/v1alpha1"
	"github.com/inseefrlab/s3-operator/controllers/s3/factory"
)

// BucketReconciler reconciles a Bucket object
type BucketReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	S3Client factory.S3Client
}

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

	bucketResource := &s3v1alpha1.Bucket{}
	err := r.Get(ctx, req.NamespacedName, bucketResource)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info(fmt.Sprintf("Bucket CRD %s has been removed. NOOP", req.Name))
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Check bucket existence on the S3 server
	found, err := r.S3Client.BucketExists(bucketResource.Spec.Name)
	if err != nil {
		logger.Error(err, "an error occurred while checking the existence of a bucket", "bucket", bucketResource.Spec.Name)
		return r.SetBucketStatusConditionAndUpdate(ctx, bucketResource, "OperatorFailed", metav1.ConditionFalse, "BucketExistenceCheckFailed",
			fmt.Sprintf("Checking existence of bucket [%s] from S3 instance has failed", bucketResource.Spec.Name), err)
	}

	// If the bucket does not exist, it is created based on the CR (with potential quotas and paths)
	if !found {

		// Bucket creation
		err = r.S3Client.CreateBucket(bucketResource.Spec.Name)
		if err != nil {
			logger.Error(err, "an error occurred while creating a bucket", "bucket", bucketResource.Spec.Name)
			return r.SetBucketStatusConditionAndUpdate(ctx, bucketResource, "OperatorFailed", metav1.ConditionFalse, "BucketCreationFailed",
				fmt.Sprintf("Creation of bucket [%s] on S3 instance has failed", bucketResource.Spec.Name), err)
		}

		// Setting quotas
		err = r.S3Client.SetQuota(bucketResource.Spec.Name, bucketResource.Spec.Quota.Default)
		if err != nil {
			logger.Error(err, "an error occurred while setting a quota on a bucket", "bucket", bucketResource.Spec.Name, "quota", bucketResource.Spec.Quota.Default)
			return r.SetBucketStatusConditionAndUpdate(ctx, bucketResource, "OperatorFailed", metav1.ConditionFalse, "SetQuotaOnBucketFailed",
				fmt.Sprintf("Setting a quota of [%v] on bucket [%s] has failed", bucketResource.Spec.Quota.Default, bucketResource.Spec.Name), err)
		}

		// Création des chemins
		for _, v := range bucketResource.Spec.Paths {
			err = r.S3Client.CreatePath(bucketResource.Spec.Name, v)
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

	// If the bucket exists on the S3 server, then we need to compare it to
	// its corresponding custom resource, and update it in case the CR has changed.

	// Checking effectiveQuota existence on the bucket
	effectiveQuota, err := r.S3Client.GetQuota(bucketResource.Spec.Name)
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
		err = r.S3Client.SetQuota(bucketResource.Spec.Name, quotaToResetTo)
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
	// But then again, some buckets will likely be filled with many objets outside the
	// scope of the CR, so getting all of them might be even more costly.
	for _, pathInCr := range bucketResource.Spec.Paths {
		pathExists, err := r.S3Client.PathExists(bucketResource.Spec.Name, pathInCr)
		if err != nil {
			logger.Error(err, "an error occurred while checking a path's existence on a bucket", "bucket", bucketResource.Spec.Name, "path", pathInCr)
			return r.SetBucketStatusConditionAndUpdate(ctx, bucketResource, "OperatorFailed", metav1.ConditionFalse, "BucketPathCheckFailed",
				fmt.Sprintf("The check for path [%s] on bucket [%s] has failed", pathInCr, bucketResource.Spec.Name), err)
		}

		if !pathExists {
			err = r.S3Client.CreatePath(bucketResource.Spec.Name, pathInCr)
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

// SetupWithManager sets up the controller with the Manager.*
func (r *BucketReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&s3v1alpha1.Bucket{}).
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

func (r *BucketReconciler) SetBucketStatusConditionAndUpdate(ctx context.Context, bucketResource *s3v1alpha1.Bucket, conditionType string, status metav1.ConditionStatus, reason string, message string, srcError error) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	meta.SetStatusCondition(&bucketResource.Status.Conditions,
		metav1.Condition{
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
	return ctrl.Result{}, srcError
}
