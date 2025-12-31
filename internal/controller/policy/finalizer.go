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

package policy_controller

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	s3v1alpha1 "github.com/InseeFrLab/s3-operator/api/v1alpha1"
)

func (r *PolicyReconciler) finalizePolicy(
	ctx context.Context,
	req reconcile.Request,
	policyResource *s3v1alpha1.Policy,
) error {
	s3Client, err := r.S3Instancehelper.GetS3ClientForRessource(
		ctx,
		r.Client,
		r.S3factory,
		policyResource.Name,
		policyResource.Namespace,
		policyResource.Spec.S3InstanceRef,
	)
	if err != nil {
		r.SetDegradedCondition(
			ctx,
			req,
			policyResource,
			metav1.ConditionUnknown,
			s3v1alpha1.Unreachable,
			"Failed to generate s3client from instance",
			err,
		)
		return err
	}
	if s3Client.GetConfig().PolicyDeletionEnabled {
		return s3Client.DeletePolicy(policyResource.Spec.Name)
	}
	return nil
}

func (r *PolicyReconciler) handleDeletion(
	ctx context.Context,
	req reconcile.Request,
	policyResource *s3v1alpha1.Policy,
) (reconcile.Result, error) {
	if controllerutil.ContainsFinalizer(policyResource, policyFinalizer) {
		// Run finalization logic for policyFinalizer. If the
		// finalization logic fails, don't remove the finalizer so
		// that we can retry during the next reconciliation.
		if err := r.finalizePolicy(ctx, req, policyResource); err != nil {
			return r.SetDegradedCondition(
				ctx,
				req,
				policyResource,
				metav1.ConditionFalse,
				s3v1alpha1.DeletionFailure,
				fmt.Sprintf("Policy %s deletion has failed", policyResource.Spec.Name),
				err,
			)
		}

		if ok := controllerutil.RemoveFinalizer(policyResource, policyFinalizer); !ok {
			r.SetProgressingCondition(
				ctx,
				req,
				policyResource,
				metav1.ConditionFalse,
				s3v1alpha1.InternalError,
				fmt.Sprintf("Failed to remove finalizer for policy %s", policyResource.Spec.Name),
			)
			return ctrl.Result{Requeue: true}, nil
		}

		if err := r.Update(ctx, policyResource); err != nil {
			r.SetDegradedCondition(
				ctx,
				req,
				policyResource,
				metav1.ConditionFalse,
				s3v1alpha1.K8sApiError,
				fmt.Sprintf("An error occured when removing finalizer from policy %s", policyResource.Spec.Name),
				err,
			)
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}
