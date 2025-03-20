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

package bucket_controller

import (
	"context"
	"fmt"

	s3v1alpha1 "github.com/InseeFrLab/s3-operator/api/v1alpha1"

	k8sapierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

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
			logger.Info(
				"The Bucket custom resource has been removed ; as such the Bucket controller is NOOP.",
				"req.Name",
				req.Name,
			)
			return ctrl.Result{}, nil
		}
		logger.Error(
			err,
			"An error occurred when attempting to read the Bucket resource from the Kubernetes cluster",
		)
		return ctrl.Result{}, err
	}

	// Let's just set the status as Unknown when no status are available
	if len(bucketResource.Status.Conditions) == 0 {
		meta.SetStatusCondition(
			&bucketResource.Status.Conditions,
			metav1.Condition{
				Type:               s3v1alpha1.ConditionReconciled,
				Status:             metav1.ConditionUnknown,
				ObservedGeneration: bucketResource.Generation,
				Reason:             s3v1alpha1.Reconciling,
				Message:            "Starting reconciliation",
			},
		)
		if err = r.Status().Update(ctx, bucketResource); err != nil {
			logger.Error(
				err,
				"Failed to update bucketRessource status",
				"bucketName",
				bucketResource.Spec.Name,
				"NamespacedName",
				req.NamespacedName.String(),
			)
			return ctrl.Result{}, err
		}

		// Let's re-fetch the bucketResource Custom Resource after update the status
		// so that we have the latest state of the resource on the cluster and we will avoid
		// raise the issue "the object has been modified, please apply
		// your changes to the latest version and try again" which would re-trigger the reconciliation
		// if we try to update it again in the following operations
		if err := r.Get(ctx, req.NamespacedName, bucketResource); err != nil {
			logger.Error(
				err,
				"Failed to re-fetch bucketResource",
				"bucketName",
				bucketResource.Spec.Name,
				"NamespacedName",
				req.NamespacedName.String(),
			)
			return ctrl.Result{}, err
		}
	}

	// Add finalizer for this CR
	if !controllerutil.ContainsFinalizer(bucketResource, bucketFinalizer) {
		logger.Info("Adding finalizer to bucket resource", "bucketName",
			bucketResource.Spec.Name, "NamespacedName", req.NamespacedName.String())
		if ok := controllerutil.AddFinalizer(bucketResource, bucketFinalizer); !ok {
			logger.Error(
				err,
				"Failed to add finalizer into bucket resource",
				"bucketName",
				bucketResource.Spec.Name,
				"NamespacedName",
				req.NamespacedName.String(),
			)
			return ctrl.Result{Requeue: true}, nil
		}

		if err := r.Update(ctx, bucketResource); err != nil {
			logger.Error(
				err,
				"An error occurred when adding finalizer on bucketResource",
				"bucketName",
				bucketResource.Spec.Name,
				"NamespacedName",
				req.NamespacedName.String(),
			)
			return ctrl.Result{}, err
		}

		if err := r.Get(ctx, req.NamespacedName, bucketResource); err != nil {
			logger.Error(
				err,
				"Failed to re-fetch bucketResource",
				"bucketName",
				bucketResource.Spec.Name,
				"NamespacedName",
				req.NamespacedName.String(),
			)
			return ctrl.Result{}, err
		}
	}

	// // Managing bucket deletion with a finalizer
	// // REF : https://sdk.operatorframework.io/docs/building-operators/golang/advanced-topics/#external-resources
	if bucketResource.GetDeletionTimestamp() != nil {
		logger.Info("bucketResource have been marked for deletion", "bucketName",
			bucketResource.Spec.Name,
			"NamespacedName",
			req.NamespacedName.String())
		return r.handleDeletion(ctx, req, bucketResource)
	}

	return r.handleReconciliation(ctx, req, bucketResource)

}

func (r *BucketReconciler) handleReconciliation(
	ctx context.Context,
	req reconcile.Request,
	bucketResource *s3v1alpha1.Bucket,
) (reconcile.Result, error) {
	logger := log.FromContext(ctx)

	s3Client, err := r.S3Instancehelper.GetS3ClientForRessource(
		ctx,
		r.Client,
		r.S3factory,
		bucketResource.Name,
		bucketResource.Namespace,
		bucketResource.Spec.S3InstanceRef,
	)
	if err != nil {
		logger.Error(err, "an error occurred while getting s3Client")
		return r.SetReconciledCondition(
			ctx,
			req,
			bucketResource,
			s3v1alpha1.Unreachable,
			"Failed to generate s3client from instance",
			err,
		)
	}

	// Bucket lifecycle management (other than deletion) starts here
	// Check bucket existence on the S3 server
	found, err := s3Client.BucketExists(bucketResource.Spec.Name)
	if err != nil {
		logger.Error(
			err,
			"An error occurred while checking the existence of a bucket",
			"bucketName",
			bucketResource.Spec.Name,
			"NamespacedName",
			req.NamespacedName.String(),
		)
		return r.SetReconciledCondition(
			ctx,
			req,
			bucketResource,
			s3v1alpha1.Unreachable,
			"Error while checking if bucket already exist",
			err,
		)

	}

	// If the bucket does not exist, it is created based on the CR (with potential quotas and paths)
	if !found {
		return r.handleCreation(ctx, req, bucketResource)
	}

	return r.handleUpdate(ctx, req, bucketResource)

}

func (r *BucketReconciler) handleUpdate(
	ctx context.Context,
	req reconcile.Request,
	bucketResource *s3v1alpha1.Bucket,
) (reconcile.Result, error) {
	logger := log.FromContext(ctx)

	s3Client, err := r.S3Instancehelper.GetS3ClientForRessource(
		ctx,
		r.Client,
		r.S3factory,
		bucketResource.Name,
		bucketResource.Namespace,
		bucketResource.Spec.S3InstanceRef,
	)
	if err != nil {
		logger.Error(
			err,
			"An error occurred while getting s3Client for bucket ressource",
			"bucketName",
			bucketResource.Spec.Name,
			"NamespacedName",
			req.NamespacedName.String(),
		)
		return r.SetReconciledCondition(
			ctx,
			req,
			bucketResource,
			s3v1alpha1.Unreachable,
			"Failed to generate s3client from instance",
			err,
		)
	}

	// If the bucket exists on the S3 server, then we need to compare it to
	// its corresponding custom resource, and update it in case the CR has changed.

	// Checking effectiveQuota existence on the bucket
	effectiveQuota, err := s3Client.GetQuota(bucketResource.Spec.Name)
	if err != nil {
		logger.Error(
			err,
			"An error occurred while checking the quota for bucket ressource",
			"bucketName",
			bucketResource.Spec.Name,
			"NamespacedName",
			req.NamespacedName.String(),
		)
		return r.SetReconciledCondition(
			ctx,
			req,
			bucketResource,
			s3v1alpha1.Unreachable,
			"Checking quota has failed",
			err,
		)
	}

	// If a quota exists, we check it versus the spec of the CR. In case they don't match,
	// we reset the quota using the value from CR ("override" is present, "default" if not)

	// Choosing between override / default
	quotaToResetTo, convertionSucceed := bucketResource.Spec.Quota.Override.AsInt64()
	if !convertionSucceed {
		logger.Error(
			err,
			"An error occurred while getting quotas override as int64 for ressource",
			"bucketName",
			bucketResource.Spec.Name,
			"NamespacedName",
			req.NamespacedName.String(),
		)
		return r.SetReconciledCondition(
			ctx,
			req,
			bucketResource,
			s3v1alpha1.Unreachable,
			"An error occurred while creating bucket",
			err,
		)
	}
	if quotaToResetTo == 0 {
		quotaToResetTo, convertionSucceed = bucketResource.Spec.Quota.Default.AsInt64()
		if !convertionSucceed {
			logger.Error(
				err,
				"An error occurred while getting default quotas as int64 for ressource",
				"bucketName",
				bucketResource.Spec.Name,
				"NamespacedName",
				req.NamespacedName.String(),
			)
			return r.SetReconciledCondition(
				ctx,
				req,
				bucketResource,
				s3v1alpha1.Unreachable,
				"An error occurred while creating bucket",
				err,
			)
		}
	}

	if effectiveQuota != quotaToResetTo {
		err = s3Client.SetQuota(bucketResource.Spec.Name, quotaToResetTo)
		if err != nil {
			logger.Error(
				err,
				"An error occurred while resetting the quota for bucket ressource",
				"bucketName",
				bucketResource.Spec.Name,
				"NamespacedName",
				req.NamespacedName.String(),
			)
			return r.SetReconciledCondition(
				ctx,
				req,
				bucketResource,
				s3v1alpha1.Unreachable,
				fmt.Sprintf(
					"The quota update (%v => %v) has failed",
					effectiveQuota,
					quotaToResetTo,
				),
				err,
			)
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
			logger.Error(
				err,
				"An error occurred while checking a path's existence for bucket ressource",
				"path",
				pathInCr,
				"bucketName",
				bucketResource.Spec.Name,
				"NamespacedName",
				req.NamespacedName.String(),
			)
			return r.SetReconciledCondition(
				ctx,
				req,
				bucketResource,
				s3v1alpha1.Unreachable,
				fmt.Sprintf("The check for path [%s] in bucket has failed", pathInCr),
				err,
			)
		}

		if !pathExists {
			err = s3Client.CreatePath(bucketResource.Spec.Name, pathInCr)
			if err != nil {
				logger.Error(
					err,
					"An error occurred while creating a path for bucket ressource",
					"path",
					pathInCr,
					"bucketName",
					bucketResource.Spec.Name,
					"NamespacedName",
					req.NamespacedName.String(),
				)
				return r.SetReconciledCondition(
					ctx,
					req,
					bucketResource,
					s3v1alpha1.Unreachable,
					fmt.Sprintf("The creation of path [%s] in bucket has failed", pathInCr),
					err,
				)
			}
		}
	}

	return r.SetReconciledCondition(
		ctx,
		req,
		bucketResource,
		s3v1alpha1.Reconciled,
		"Bucket reconciled",
		nil,
	)
}

func (r *BucketReconciler) handleCreation(
	ctx context.Context,
	req reconcile.Request,
	bucketResource *s3v1alpha1.Bucket,
) (reconcile.Result, error) {
	logger := log.FromContext(ctx)

	s3Client, err := r.S3Instancehelper.GetS3ClientForRessource(
		ctx,
		r.Client,
		r.S3factory,
		bucketResource.Name,
		bucketResource.Namespace,
		bucketResource.Spec.S3InstanceRef,
	)
	if err != nil {
		logger.Error(
			err,
			"An error occurred while getting s3Client for bucket ressource",
			"bucketName",
			bucketResource.Spec.Name,
			"NamespacedName",
			req.NamespacedName.String(),
		)
		return r.SetReconciledCondition(
			ctx,
			req,
			bucketResource,
			s3v1alpha1.Unreachable,
			"Failed to generate s3client from instance",
			err,
		)
	}

	// Bucket creation
	err = s3Client.CreateBucket(bucketResource.Spec.Name)
	if err != nil {
		logger.Error(
			err,
			"An error occurred while creating bucket ressource",
			"bucketName",
			bucketResource.Spec.Name,
			"NamespacedName",
			req.NamespacedName.String(),
		)
		return r.SetReconciledCondition(
			ctx,
			req,
			bucketResource,
			s3v1alpha1.CreationFailure,
			"An error occurred while creating bucket",
			err,
		)
	}

	// Setting quotas
	quotas, convertionSucceed := bucketResource.Spec.Quota.Default.AsInt64()
	if !convertionSucceed {
		logger.Error(
			err,
			"An error occurred while getting quotas as int64 for ressource",
			"bucketName",
			bucketResource.Spec.Name,
			"NamespacedName",
			req.NamespacedName.String(),
		)
		return r.SetReconciledCondition(
			ctx,
			req,
			bucketResource,
			s3v1alpha1.CreationFailure,
			"An error occurred while creating bucket",
			err,
		)
	}
	err = s3Client.SetQuota(bucketResource.Spec.Name, quotas)
	if err != nil {
		logger.Error(
			err,
			"An error occurred while setting quota for bucket ressource",
			"bucketName",
			bucketResource.Spec.Name,
			"NamespacedName",
			req.NamespacedName.String(),
		)
		return r.SetReconciledCondition(
			ctx,
			req,
			bucketResource,
			s3v1alpha1.Unreachable,
			fmt.Sprintf(
				"Setting a quota of [%v] on bucket [%s] has failed",
				bucketResource.Spec.Quota.Default,
				bucketResource.Spec.Name,
			),
			err,
		)
	}

	// Path creation
	for _, pathInCr := range bucketResource.Spec.Paths {
		err = s3Client.CreatePath(bucketResource.Spec.Name, pathInCr)
		if err != nil {
			logger.Error(
				err,
				"An error occurred while creating path for bucket ressource",
				"path",
				pathInCr,
				"bucketName",
				bucketResource.Spec.Name,
				"NamespacedName",
				req.NamespacedName.String(),
			)
			return r.SetReconciledCondition(
				ctx,
				req,
				bucketResource,
				s3v1alpha1.Unreachable,
				fmt.Sprintf("Creation for path [%s] in bucket has failed", pathInCr),
				err,
			)
		}
	}

	return r.SetReconciledCondition(
		ctx,
		req,
		bucketResource,
		s3v1alpha1.Reconciled,
		"Bucket reconciled",
		nil,
	)
}
