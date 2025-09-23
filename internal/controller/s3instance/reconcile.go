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

package s3instance_controller

import (
	"context"
	"fmt"

	k8sapierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	s3v1alpha1 "github.com/InseeFrLab/s3-operator/api/v1alpha1"
)

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *S3InstanceReconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Checking for s3InstanceResource existence
	s3InstanceResource := &s3v1alpha1.S3Instance{}
	err := r.Get(ctx, req.NamespacedName, s3InstanceResource)
	if err != nil {
		if k8sapierrors.IsNotFound(err) {
			logger.Info(
				fmt.Sprintf("The S3InstanceResource CR %s has been removed. NOOP", req.Name),
				"NamespacedName",
				req.NamespacedName.String(),
			)
			return ctrl.Result{}, nil
		}
		logger.Error(
			err,
			"Failed to get S3InstanceResource",
			"NamespacedName",
			req.NamespacedName.String(),
		)
		return ctrl.Result{}, err
	}

	// Add finalizer for this CR
	if !controllerutil.ContainsFinalizer(s3InstanceResource, s3InstanceFinalizer) {
		r.SetProgressingCondition(ctx,
					  req,
					  s3InstanceResource,
					  metav1.ConditionTrue,
					  s3v1alpha1.Reconciling,
					  fmt.Sprintf("Adding finalizer to s3Instance resource %s", s3InstanceResource.Name))
		if ok := controllerutil.AddFinalizer(s3InstanceResource, s3InstanceFinalizer); !ok {
			r.SetDegradedCondition(ctx,
					       req,
					       s3InstanceResource,
					       metav1.ConditionFalse,
					       s3v1alpha1.InternalError,
					       fmt.Sprintf("Failed to add finalizer to s3Instance resource %s", s3InstanceResource.Name),
					       err)
			return ctrl.Result{Requeue: true}, nil
		}

		if err := r.Update(ctx, s3InstanceResource); err != nil {
			r.SetDegradedCondition(ctx,
					       req,
					       s3InstanceResource,
					       metav1.ConditionFalse,
					       s3v1alpha1.K8sApiError,
					       fmt.Sprintf("An error occurred when adding finalizer on s3Instance resource %s", s3InstanceResource.Name),
					       err)
			return ctrl.Result{}, err
		}

		if err := r.Get(ctx, req.NamespacedName, s3InstanceResource); err != nil {
			r.SetDegradedCondition(ctx,
					       req,
					       s3InstanceResource,
					       metav1.ConditionFalse,
					       s3v1alpha1.K8sApiError,
					       fmt.Sprintf("Failed to re-fetch s3Instance resource %s", s3InstanceResource.Name),
					       err)
			return ctrl.Result{}, err
		}
	}
	// Check if the s3InstanceResource instance is marked to be deleted, which is
	// indicated by the deletion timestamp being set. The object will be deleted.
	if s3InstanceResource.GetDeletionTimestamp() != nil {
		r.SetProgressingCondition(ctx,
					  req,
					  s3InstanceResource,
					  metav1.ConditionTrue,
					  s3v1alpha1.Reconciling,
					  fmt.Sprintf("S3Instance resource have been marked for deletion %s", s3InstanceResource.Name))
		return r.handleS3InstanceDeletion(ctx, req, s3InstanceResource)
	}

	// Reconciliation starts here
	r.SetProgressingCondition(ctx, req, s3InstanceResource, metav1.ConditionTrue, s3v1alpha1.Reconciling, fmt.Sprintf("Starting reconciliation of s3Instance %s", s3InstanceResource.Name))
	return r.handleReconciliation(ctx, req, s3InstanceResource)
}

func (r *S3InstanceReconciler) handleReconciliation(
	ctx context.Context,
	req reconcile.Request,
	s3InstanceResource *s3v1alpha1.S3Instance,
) (reconcile.Result, error) {

	s3Client, err := r.S3Instancehelper.GetS3ClientFromS3Instance(ctx, r.Client, r.S3factory, s3InstanceResource)

	if err != nil {
		return r.SetDegradedCondition(ctx, req, s3InstanceResource, metav1.ConditionFalse, s3v1alpha1.Unreachable,
			fmt.Sprintf("Failed to generate S3Instance %s using secret %s", s3InstanceResource.Name, s3InstanceResource.Spec.SecretRef), err)
	}

	_, err = s3Client.ListBuckets()
	if err != nil {
		return r.SetDegradedCondition(ctx, req, s3InstanceResource, metav1.ConditionFalse, s3v1alpha1.CreationFailure,
			fmt.Sprintf("Failed to generate S3Instance %s, listing buckets failed, using secret %s", s3InstanceResource.Name, s3InstanceResource.Spec.SecretRef), err)
	}

	return r.SetAvailableCondition(
		ctx,
		req,
		s3InstanceResource,
		s3v1alpha1.Reconciled,
		"S3Instance instance reconciled",
	)

}
