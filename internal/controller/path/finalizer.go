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

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	s3v1alpha1 "github.com/InseeFrLab/s3-operator/api/v1alpha1"
)

func (r *PathReconciler) handleDeletion(
	ctx context.Context,
	req reconcile.Request,
	pathResource *s3v1alpha1.Path,
) (reconcile.Result, error) {
	logger := log.FromContext(ctx)

	if controllerutil.ContainsFinalizer(pathResource, pathFinalizer) {
		if err := r.finalizePath(ctx, pathResource); err != nil {
			logger.Error(
				err,
				"An error occurred when attempting to finalize the path",
				"path",
				pathResource.Name,
				"NamespacedName",
				req.Namespace,
			)
			return r.SetReconciledCondition(
				ctx,
				req,
				pathResource,
				s3v1alpha1.DeletionFailure,
				"Path deletion has failed",
				err,
			)
		}

		// Remove pathFinalizer. Once all finalizers have been
		// removed, the object will be deleted.

		if ok := controllerutil.RemoveFinalizer(pathResource, pathFinalizer); !ok {
			logger.Info(
				"Failed to remove finalizer for pathResource",
				"pathResource",
				pathResource.Name,
				"NamespacedName",
				req.Namespace,
			)
			return ctrl.Result{Requeue: true}, nil
		}

		// Let's re-fetch the S3Instance Custom Resource after removing the finalizer
		// so that we have the latest state of the resource on the cluster and we will avoid
		// raise the issue "the object has been modified, please apply
		// your changes to the latest version and try again" which would re-trigger the reconciliation
		// if we try to update it again in the following operations
		if err := r.Update(ctx, pathResource); err != nil {
			logger.Error(
				err,
				"An error occurred when removing finalizer from pathResource",
				"pathResource",
				pathResource.Name,
				"NamespacedName",
				req.Namespace,
			)
			return ctrl.Result{}, err
		}

	}
	return ctrl.Result{}, nil
}

func (r *PathReconciler) finalizePath(ctx context.Context, pathResource *s3v1alpha1.Path) error {
	logger := log.FromContext(ctx)

	s3Client, err := r.S3Instancehelper.GetS3ClientForRessource(
		ctx,
		r.Client,
		r.S3factory,
		pathResource.Name,
		pathResource.Namespace,
		pathResource.Spec.S3InstanceRef,
	)
	if err != nil {
		logger.Error(err, "An error occurred while getting s3Client")
		return err
	}

	if s3Client.GetConfig().PathDeletionEnabled {
		var failedPaths = make([]string, 0)
		for _, path := range pathResource.Spec.Paths {

			pathExists, err := s3Client.PathExists(pathResource.Spec.BucketName, path)
			if err != nil {
				logger.Error(
					err,
					"finalize : an error occurred while checking a path's existence on a bucket",
					"bucket",
					pathResource.Spec.BucketName,
					"path",
					path,
				)
			}

			if pathExists {
				err = s3Client.DeletePath(pathResource.Spec.BucketName, path)
				if err != nil {
					failedPaths = append(failedPaths, path)
				}
			}
		}

		if len(failedPaths) > 0 {
			return fmt.Errorf(
				"at least one path couldn't be removed from S3 backend %+q",
				failedPaths,
			)
		}
	}
	return nil
}
