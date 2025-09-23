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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	s3v1alpha1 "github.com/InseeFrLab/s3-operator/api/v1alpha1"
)

func (r *PathReconciler) handleDeletion(
	ctx context.Context,
	req reconcile.Request,
	pathResource *s3v1alpha1.Path,
) (reconcile.Result, error) {

	if controllerutil.ContainsFinalizer(pathResource, pathFinalizer) {
		if err := r.finalizePath(ctx, req, pathResource); err != nil {
			return r.SetDegradedCondition(
				ctx,
				req,
				pathResource,
				metav1.ConditionFalse,
				s3v1alpha1.DeletionFailure,
				fmt.Sprintf("Path %s deletion has failed", pathResource.Name),
				err,
			)
		}

		// Remove pathFinalizer. Once all finalizers have been
		// removed, the object will be deleted.

		if ok := controllerutil.RemoveFinalizer(pathResource, pathFinalizer); !ok {
			r.SetProgressingCondition(
				ctx,
				req,
				pathResource,
				metav1.ConditionFalse,
				s3v1alpha1.InternalError,
				fmt.Sprintf("Failed to remove finalizer for path %s", pathResource.Name),
			)
			return ctrl.Result{Requeue: true}, nil
		}

		// Let's re-fetch the S3Instance Custom Resource after removing the finalizer
		// so that we have the latest state of the resource on the cluster and we will avoid
		// raise the issue "the object has been modified, please apply
		// your changes to the latest version and try again" which would re-trigger the reconciliation
		// if we try to update it again in the following operations
		if err := r.Update(ctx, pathResource); err != nil {
			r.SetDegradedCondition(
				ctx,
				req,
				pathResource,
				metav1.ConditionFalse,
				s3v1alpha1.K8sApiError,
				fmt.Sprintf("An error occured when removing finalizer from path %s", pathResource.Name),
				err,
			)

			return ctrl.Result{}, err
		}

	}
	return ctrl.Result{}, nil
}

func (r *PathReconciler) finalizePath(ctx context.Context, req reconcile.Request, pathResource *s3v1alpha1.Path) error {

	s3Client, err := r.S3Instancehelper.GetS3ClientForRessource(
		ctx,
		r.Client,
		r.S3factory,
		pathResource.Name,
		pathResource.Namespace,
		pathResource.Spec.S3InstanceRef,
	)

	if err != nil {
		r.SetDegradedCondition(
			ctx,
			req,
			pathResource,
			metav1.ConditionUnknown,
			s3v1alpha1.Unreachable,
			"Failed to generate s3client from instance",
			err,
		)
		return err
	}

	if s3Client.GetConfig().PathDeletionEnabled {
		var failedPaths []string = make([]string, 0)
		for _, path := range pathResource.Spec.Paths {

			pathExists, err := s3Client.PathExists(pathResource.Spec.BucketName, path)
			if err != nil {
				r.SetDegradedCondition(
					ctx,
					req,
					pathResource,
					metav1.ConditionFalse,
					s3v1alpha1.DeletionFailure,
					fmt.Sprintf("An error occured while finalizing path %s, failed to check path's existence on bucket %s", pathResource.Name, pathResource.Spec.BucketName),
					err,
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
