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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func (r *BucketReconciler) handleDeletion(
	ctx context.Context,
	req reconcile.Request,
	bucketResource *s3v1alpha1.Bucket,
) (reconcile.Result, error) {

	if controllerutil.ContainsFinalizer(bucketResource, bucketFinalizer) {

		if err := r.finalizeBucket(ctx, req, bucketResource); err != nil {
			return r.SetDegradedCondition(
				ctx,
				req,
				bucketResource,
				metav1.ConditionFalse,
				s3v1alpha1.DeletionFailure,
				fmt.Sprintf("Bucket %s deletion has failed", bucketResource.Spec.Name),
				err,
			)
		}

		if ok := controllerutil.RemoveFinalizer(bucketResource, bucketFinalizer); !ok {
			r.SetProgressingCondition(
				ctx,
				req,
				bucketResource,
				metav1.ConditionFalse,
				s3v1alpha1.InternalError,
				fmt.Sprintf("Failed to remove finalizer for bucket %s", bucketResource.Spec.Name),
			)
			return ctrl.Result{Requeue: true}, nil
		}

		// Let's re-fetch the S3Instance Custom Resource after removing the finalizer
		// so that we have the latest state of the resource on the cluster and we will avoid
		// raise the issue "the object has been modified, please apply
		// your changes to the latest version and try again" which would re-trigger the reconciliation
		// if we try to update it again in the following operations
		if err := r.Update(ctx, bucketResource); err != nil {
			r.SetDegradedCondition(
				ctx,
				req,
				bucketResource,
				metav1.ConditionFalse,
				s3v1alpha1.K8sApiError,
				fmt.Sprintf("An error occured when removing finalizer from bucket %s", bucketResource.Spec.Name),
				err,
			)

			return ctrl.Result{}, err
		}

	}
	return ctrl.Result{}, nil
}

func (r *BucketReconciler) finalizeBucket(
	ctx context.Context,
	req reconcile.Request,
	bucketResource *s3v1alpha1.Bucket,
) error {

	s3Client, err := r.S3Instancehelper.GetS3ClientForRessource(
		ctx,
		r.Client,
		r.S3factory,
		bucketResource.Name,
		bucketResource.Namespace,
		bucketResource.Spec.S3InstanceRef,
	)
	if err != nil {
		r.SetDegradedCondition(
			ctx,
			req,
			bucketResource,
			metav1.ConditionUnknown,
			s3v1alpha1.Unreachable,
			"Failed to generate s3client from instance",
			err,
		)
		return err
	}
	if s3Client.GetConfig().BucketDeletionEnabled {
		return s3Client.DeleteBucket(bucketResource.Spec.Name)
	}
	return nil
}
