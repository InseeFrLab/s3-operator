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
	"k8s.io/apimachinery/pkg/api/meta"
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
				req.Namespace,
			)
			return ctrl.Result{}, nil
		}
		logger.Error(
			err,
			"Failed to get S3InstanceResource",
			"NamespacedName",
			req.Namespace,
		)
		return ctrl.Result{}, err
	}

	// Let's just set the status as Unknown when no status are available
	if len(s3InstanceResource.Status.Conditions) == 0 {
		meta.SetStatusCondition(
			&s3InstanceResource.Status.Conditions,
			metav1.Condition{
				Type:               s3v1alpha1.ConditionReconciled,
				Status:             metav1.ConditionUnknown,
				ObservedGeneration: s3InstanceResource.Generation,
				Reason:             s3v1alpha1.Reconciling,
				Message:            "Starting reconciliation",
			},
		)
		if err = r.Status().Update(ctx, s3InstanceResource); err != nil {
			logger.Error(
				err,
				"Failed to update s3InstanceResource status",
				"NamespacedName",
				req.Namespace,
			)
			return ctrl.Result{}, err
		}

		// Let's re-fetch the s3InstanceResource Custom Resource after update the status
		// so that we have the latest state of the resource on the cluster and we will avoid
		// raise the issue "the object has been modified, please apply
		// your changes to the latest version and try again" which would re-trigger the reconciliation
		// if we try to update it again in the following operations
		if err := r.Get(ctx, req.NamespacedName, s3InstanceResource); err != nil {
			logger.Error(
				err,
				"Failed to re-fetch s3Instance",
				"NamespacedName",
				req.Namespace,
			)
			return ctrl.Result{}, err
		}
	}

	// Add finalizer for this CR
	if !controllerutil.ContainsFinalizer(s3InstanceResource, s3InstanceFinalizer) {
		logger.Info("Adding finalizer to s3Instance", "NamespacedName", req.Namespace)
		if ok := controllerutil.AddFinalizer(s3InstanceResource, s3InstanceFinalizer); !ok {
			logger.Error(
				err,
				"Failed to add finalizer into the s3Instance",
				"NamespacedName",
				req.Namespace,
			)
			return ctrl.Result{Requeue: true}, nil
		}

		if err = r.Update(ctx, s3InstanceResource); err != nil {
			logger.Error(
				err,
				"an error occurred when adding finalizer on s3Instance",
				"s3Instance",
				s3InstanceResource.Name,
			)
			return ctrl.Result{}, err
		}

		// Let's re-fetch the S3Instance Custom Resource after adding the finalizer
		// so that we have the latest state of the resource on the cluster and we will avoid
		// raise the issue "the object has been modified, please apply
		// your changes to the latest version and try again" which would re-trigger the reconciliation
		// if we try to update it again in the following operations
		if err := r.Get(ctx, req.NamespacedName, s3InstanceResource); err != nil {
			logger.Error(
				err,
				"Failed to re-fetch s3Instance",
				"NamespacedName",
				req.Namespace,
			)
			return ctrl.Result{}, err
		}

	}

	// Check if the s3InstanceResource instance is marked to be deleted, which is
	// indicated by the deletion timestamp being set. The object will be deleted.
	if s3InstanceResource.GetDeletionTimestamp() != nil {
		logger.Info("s3InstanceResource have been marked for deletion")
		return r.handleS3InstanceDeletion(ctx, req, s3InstanceResource)
	}

	// Reconciliation starts here
	return r.handleReconciliation(ctx, req, s3InstanceResource)
}

func (r *S3InstanceReconciler) handleReconciliation(
	ctx context.Context,
	req reconcile.Request,
	s3InstanceResource *s3v1alpha1.S3Instance,
) (reconcile.Result, error) {
	logger := log.FromContext(ctx)

	s3Client, err := r.S3Instancehelper.GetS3ClientFromS3Instance(ctx, r.Client, r.S3factory, s3InstanceResource)
	if err != nil {
		logger.Error(
			err,
			"Could not generate s3Instance",
			"s3InstanceSecretRefName",
			s3InstanceResource.Spec.SecretRef,
			"NamespacedName",
			req.Namespace,
		)
		return r.SetReconciledCondition(ctx, req, s3InstanceResource, s3v1alpha1.Unreachable,
			"Failed to generate S3Instance ", err)
	}

	_, err = s3Client.ListBuckets()
	if err != nil {
		logger.Error(
			err,
			"Could not generate s3Instance",
			"s3InstanceName",
			s3InstanceResource.Name,
			"NamespacedName",
			req.Namespace,
		)
		return r.SetReconciledCondition(ctx, req, s3InstanceResource, s3v1alpha1.CreationFailure,
			"Failed to generate S3Instance ", err)
	}

	return r.SetReconciledCondition(
		ctx,
		req,
		s3InstanceResource,
		s3v1alpha1.Reconciled,
		"S3Instance instance reconciled",
		nil,
	)
}
