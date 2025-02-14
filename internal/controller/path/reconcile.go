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

package path_controller

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
func (r *PathReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Checking for path resource existence
	pathResource := &s3v1alpha1.Path{}
	err := r.Get(ctx, req.NamespacedName, pathResource)
	if err != nil {
		if k8sapierrors.IsNotFound(err) {
			logger.Info(
				"The Path custom resource has been removed ; as such the Path controller is NOOP.",
				"req.Name",
				req.Name,
			)
			return ctrl.Result{}, nil
		}
		logger.Error(
			err,
			"An error occurred when attempting to read the Path resource from the Kubernetes cluster",
		)
		return ctrl.Result{}, err
	}

	// Let's just set the status as Unknown when no status are available
	if len(pathResource.Status.Conditions) == 0 {
		meta.SetStatusCondition(
			&pathResource.Status.Conditions,
			metav1.Condition{
				Type:               s3v1alpha1.ConditionReconciled,
				Status:             metav1.ConditionUnknown,
				ObservedGeneration: pathResource.Generation,
				Reason:             s3v1alpha1.Reconciling,
				Message:            "Starting reconciliation",
			},
		)
		if err = r.Status().Update(ctx, pathResource); err != nil {
			logger.Error(
				err,
				"Failed to update pathResource status",
				"pathResourceName",
				pathResource.Name,
				"NamespacedName",
				req.NamespacedName.String(),
			)
			return ctrl.Result{}, err
		}

		// Let's re-fetch the s3InstanceResource Custom Resource after update the status
		// so that we have the latest state of the resource on the cluster and we will avoid
		// raise the issue "the object has been modified, please apply
		// your changes to the latest version and try again" which would re-trigger the reconciliation
		// if we try to update it again in the following operations
		if err := r.Get(ctx, req.NamespacedName, pathResource); err != nil {
			logger.Error(
				err,
				"Failed to re-fetch pathResource",
				"pathResourceName",
				pathResource.Name,
				"NamespacedName",
				req.NamespacedName.String(),
			)
			return ctrl.Result{}, err
		}
	}

	// Add finalizer for this CR
	if !controllerutil.ContainsFinalizer(pathResource, pathFinalizer) {
		logger.Info("Adding finalizer to pathResource", "pathResourceName",
			pathResource.Name, "NamespacedName", req.NamespacedName.String())
		if ok := controllerutil.AddFinalizer(pathResource, pathFinalizer); !ok {
			logger.Error(
				err,
				"Failed to add finalizer into pathResource",
				"pathResourceName",
				pathResource.Name, "NamespacedName", req.NamespacedName.String())
			return ctrl.Result{Requeue: true}, nil
		}

		if err := r.Update(ctx, pathResource); err != nil {
			logger.Error(
				err,
				"an error occurred when adding finalizer on pathResource",
				"pathResourceName",
				pathResource.Name, "NamespacedName", req.NamespacedName.String(),
			)
			return ctrl.Result{}, err
		}

		if err := r.Get(ctx, req.NamespacedName, pathResource); err != nil {
			logger.Error(
				err,
				"Failed to re-fetch pathResource",
				"pathResourceName",
				pathResource.Name, "NamespacedName", req.NamespacedName.String(),
			)
			return ctrl.Result{}, err
		}
	}

	// Managing path deletion with a finalizer
	// REF : https://sdk.operatorframework.io/docs/building-operators/golang/advanced-topics/#external-resources
	if pathResource.GetDeletionTimestamp() != nil {
		logger.Info("pathResource have been marked for deletion", "pathResourceName",
			pathResource.Name,
			"NamespacedName",
			req.NamespacedName.String())
		return r.handleDeletion(ctx, req, pathResource)
	}

	return r.handleReconciliation(ctx, req, pathResource)

}

func (r *PathReconciler) handleReconciliation(
	ctx context.Context,
	req reconcile.Request,
	pathResource *s3v1alpha1.Path,
) (reconcile.Result, error) {

	logger := log.FromContext(ctx)

	// Create S3Client
	s3Client, err := r.S3Instancehelper.GetS3ClientForRessource(
		ctx,
		r.Client,
		r.S3factory,
		pathResource.Name,
		pathResource.Namespace,
		pathResource.Spec.S3InstanceRef,
	)
	if err != nil {
		logger.Error(err, "an error occurred while getting s3Client")
		return r.SetReconciledCondition(
			ctx,
			req,
			pathResource,
			s3v1alpha1.Unreachable,
			"Failed to generate s3client from instance",
			err,
		)
	}

	// Path lifecycle management (other than deletion) starts here

	// Check bucket existence on the S3 server
	bucketFound, err := s3Client.BucketExists(pathResource.Spec.BucketName)
	if err != nil {
		logger.Error(
			err,
			"an error occurred while checking the existence of a bucket",
			"bucketName",
			pathResource.Spec.BucketName,
			"NamespacedName",
			req.NamespacedName.String(),
		)
		return r.SetReconciledCondition(
			ctx,
			req,
			pathResource,
			s3v1alpha1.Unreachable,
			"Error while checking if bucket already exist",
			err,
		)
	}

	// If bucket does not exist, the Path CR should be in a failing state
	if !bucketFound {
		errorBucketNotFound := fmt.Errorf(
			"the path CR %s references a non-existing bucket : %s",
			pathResource.Name,
			pathResource.Spec.BucketName,
		)
		logger.Error(errorBucketNotFound, errorBucketNotFound.Error(), "pathResourceName",
			pathResource.Name,
			"NamespacedName",
			req.NamespacedName.String())
		return r.SetReconciledCondition(
			ctx,
			req,
			pathResource,
			s3v1alpha1.CreationFailure,
			fmt.Sprintf(
				"The Path CR [%s] references a non-existing bucket [%s]",
				pathResource.Name,
				pathResource.Spec.BucketName,
			),
			err,
		)
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
		pathExists, err := s3Client.PathExists(pathResource.Spec.BucketName, pathInCr)
		if err != nil {
			logger.Error(
				err,
				"An error occurred while checking a path's existence for bucket ressource",
				"path",
				pathInCr,
				"bucketName",
				pathResource.Spec.BucketName,
				"NamespacedName",
				req.NamespacedName.String(),
			)
			return r.SetReconciledCondition(
				ctx,
				req,
				pathResource,
				s3v1alpha1.Unreachable,
				fmt.Sprintf("The check for path [%s] in bucket has failed", pathInCr),
				err,
			)
		}

		if !pathExists {
			err = s3Client.CreatePath(pathResource.Spec.BucketName, pathInCr)
			if err != nil {
				logger.Error(
					err,
					"An error occurred while creating a path for bucket ressource",
					"path",
					pathInCr,
					"bucketName",
					pathResource.Spec.BucketName,
					"NamespacedName",
					req.NamespacedName.String(),
				)
				return r.SetReconciledCondition(
					ctx,
					req,
					pathResource,
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
		pathResource,
		s3v1alpha1.Reconciled,
		"Path reconciled",
		nil,
	)
}
