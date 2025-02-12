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

package user_controller

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	s3v1alpha1 "github.com/InseeFrLab/s3-operator/api/v1alpha1"
)

func (r *S3UserReconciler) finalizeS3User(
	ctx context.Context,
	userResource *s3v1alpha1.S3User,
) error {
	logger := log.FromContext(ctx)
	s3Client, err := r.S3Instancehelper.GetS3ClientForRessource(
		ctx,
		r.Client,
		r.S3factory,
		userResource.Name,
		userResource.Namespace,
		userResource.Spec.S3InstanceRef,
	)
	if err != nil {
		logger.Error(err, "An error occurred while getting s3Client")
		return err
	}
	if s3Client.GetConfig().S3UserDeletionEnabled {
		return s3Client.DeleteUser(userResource.Spec.AccessKey)
	}
	return nil
}

func (r *S3UserReconciler) handleDeletion(
	ctx context.Context,
	req reconcile.Request,
	userResource *s3v1alpha1.S3User,
) (reconcile.Result, error) {
	logger := log.FromContext(ctx)

	if controllerutil.ContainsFinalizer(userResource, userFinalizer) {
		// Run finalization logic for S3UserFinalizer. If the finalization logic fails, don't remove the finalizer so that we can retry during the next reconciliation.
		if err := r.finalizeS3User(ctx, userResource); err != nil {
			logger.Error(
				err,
				"An error occurred when attempting to finalize the user",
				"userResource",
				userResource.Name,
				"NamespacedName",
				req.NamespacedName.String(),
			)
			return r.SetReconciledCondition(
				ctx,
				req,
				userResource,
				s3v1alpha1.DeletionFailure,
				"user deletion has failed",
				err,
			)
		}

		err := r.deleteOldLinkedSecret(ctx, userResource)
		if err != nil {
			logger.Error(
				err,
				"An error occurred when trying to clean old secret linked to user",
				"userResourceName",
				userResource.Name,
				"NamespacedName",
				req.NamespacedName.String(),
			)
			return r.SetReconciledCondition(
				ctx,
				req,
				userResource,
				s3v1alpha1.DeletionFailure,
				"Deletion of old secret associated to user have failed",
				err,
			)
		}

		userOwnedSecret, _ := r.getUserSecret(ctx, userResource)
		if err := r.deleteSecret(ctx, &userOwnedSecret); err != nil {
			logger.Error(
				err,
				"An error occurred when trying to clean secret linked to user",
				"userResourceName",
				userResource.Name,
				"NamespacedName",
				req.NamespacedName.String(),
			)
			return r.SetReconciledCondition(
				ctx,
				req,
				userResource,
				s3v1alpha1.DeletionFailure,
				"Deletion of secret associated to user have failed",
				err,
			)
		}

		//Remove userFinalizer. Once all finalizers have been removed, the object will be deleted.
		if ok := controllerutil.RemoveFinalizer(userResource, userFinalizer); !ok {
			logger.Info(
				"Failed to remove finalizer for user resource",
				"userResource",
				userResource.Name,
				"NamespacedName",
				req.NamespacedName.String(),
			)
			return ctrl.Result{Requeue: true}, nil
		}

		// Unsure why the behavior is different to that of bucket/policy/path controllers, but it appears
		// calling r.Update() for adding/removal of finalizer is not necessary (an update event is generated
		// with the call to AddFinalizer/RemoveFinalizer), and worse, causes "freshness" problem (with the
		// "the object has been modified; please apply your changes to the latest version and try again" error)
		err = r.Update(ctx, userResource)
		if err != nil {
			logger.Error(
				err,
				"An error occurred when removing finalizer from policy",
				"userResource",
				userResource.Name,
				"NamespacedName",
				req.NamespacedName.String(),
			)
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}
