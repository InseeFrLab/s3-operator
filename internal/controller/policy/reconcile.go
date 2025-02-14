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
func (r *PolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Checking for policy resource existence
	policyResource := &s3v1alpha1.Policy{}
	err := r.Get(ctx, req.NamespacedName, policyResource)
	if err != nil {
		if k8sapierrors.IsNotFound(err) {
			logger.Info(
				"The Policy custom resource has been removed ; as such the Policy controller is NOOP.",
				"req.Name",
				req.Name,
			)
			return ctrl.Result{}, nil
		}
		logger.Error(
			err,
			"An error occurred when attempting to read the Policy resource from the Kubernetes cluster",
		)
		return ctrl.Result{}, err
	}

	// Let's just set the status as Unknown when no status are available
	if len(policyResource.Status.Conditions) == 0 {
		meta.SetStatusCondition(
			&policyResource.Status.Conditions,
			metav1.Condition{
				Type:               s3v1alpha1.ConditionReconciled,
				Status:             metav1.ConditionUnknown,
				ObservedGeneration: policyResource.Generation,
				Reason:             s3v1alpha1.Reconciling,
				Message:            "Starting reconciliation",
			},
		)
		if err = r.Status().Update(ctx, policyResource); err != nil {
			logger.Error(
				err,
				"Failed to update bucketRessource status",
				"bucketName",
				policyResource.Spec.Name,
				"NamespacedName",
				req.NamespacedName.String(),
			)
			return ctrl.Result{}, err
		}

		// Let's re-fetch the policyResource Custom Resource after update the status
		// so that we have the latest state of the resource on the cluster and we will avoid
		// raise the issue "the object has been modified, please apply
		// your changes to the latest version and try again" which would re-trigger the reconciliation
		// if we try to update it again in the following operations
		if err := r.Get(ctx, req.NamespacedName, policyResource); err != nil {
			logger.Error(
				err,
				"Failed to re-fetch policyResource",
				"policyName",
				policyResource.Spec.Name,
				"NamespacedName",
				req.NamespacedName.String(),
			)
			return ctrl.Result{}, err
		}
	}

	// Add finalizer for this CR
	if !controllerutil.ContainsFinalizer(policyResource, policyFinalizer) {
		logger.Info("Adding finalizer to policy resource", "PolicyName",
			policyResource.Spec.Name, "NamespacedName", req.NamespacedName.String())
		if ok := controllerutil.AddFinalizer(policyResource, policyFinalizer); !ok {
			logger.Error(
				err,
				"Failed to add finalizer into policy resource",
				"policyName",
				policyResource.Spec.Name,
				"NamespacedName",
				req.NamespacedName.String(),
			)
			return ctrl.Result{Requeue: true}, nil
		}

		err = r.Update(ctx, policyResource)
		if err != nil {
			logger.Error(
				err,
				"An error occurred when adding finalizer from policyResource",
				"policyResource",
				policyResource.Spec.Name,
				"NamespacedName",
				req.NamespacedName.String(),
			)
			return ctrl.Result{}, err
		}

		// Let's re-fetch the policy Custom Resource after adding the finalizer
		// so that we have the latest state of the resource on the cluster and we will avoid
		// raise the issue "the object has been modified, please apply
		// your changes to the latest version and try again" which would re-trigger the reconciliation
		// if we try to update it again in the following operations
		if err := r.Get(ctx, req.NamespacedName, policyResource); err != nil {
			logger.Error(
				err,
				"Failed to re-fetch policyResource",
				"policyName",
				policyResource.Spec.Name,
				"NamespacedName",
				req.NamespacedName.String(),
			)
			return ctrl.Result{}, err
		}
	}

	// Managing policy deletion with a finalizer
	// REF : https://sdk.operatorframework.io/docs/building-operators/golang/advanced-topics/#external-resources
	if policyResource.GetDeletionTimestamp() != nil {
		logger.Info("policyResource have been marked for deletion", "policyName",
			policyResource.Spec.Name,
			"NamespacedName",
			req.NamespacedName.String())
		return r.handleDeletion(ctx, req, policyResource)
	}

	// Policy lifecycle management (other than deletion) starts here
	return r.handleReconciliation(ctx, req, policyResource)

}

func (r *PolicyReconciler) handleReconciliation(
	ctx context.Context,
	req reconcile.Request,
	policyResource *s3v1alpha1.Policy,
) (reconcile.Result, error) {
	logger := log.FromContext(ctx)

	s3Client, err := r.S3Instancehelper.GetS3ClientForRessource(
		ctx,
		r.Client,
		r.S3factory,
		policyResource.Name,
		policyResource.Namespace,
		policyResource.Spec.S3InstanceRef,
	)
	if err != nil {
		logger.Error(err, "an error occurred while getting s3Client")
		return r.SetReconciledCondition(
			ctx,
			req,
			policyResource,
			s3v1alpha1.Unreachable,
			"Failed to generate s3client from instance",
			err,
		)
	}

	// Check policy existence on the S3 server
	effectivePolicy, err := s3Client.GetPolicyInfo(policyResource.Spec.Name)
	if err != nil {
		logger.Error(
			err,
			"An error occurred while checking the existence of a policy",
			"policyName",
			policyResource.Spec.Name,
			"NamespacedName",
			req.NamespacedName.String(),
		)
		return r.SetReconciledCondition(
			ctx,
			req,
			policyResource,
			s3v1alpha1.Unreachable,
			"Error while checking if policy already exist",
			err,
		)
	}

	if effectivePolicy == nil {
		return r.handleCreation(ctx, req, policyResource)
	}

	// If the policy exists on S3, we compare its state to the custom resource that spawned it on K8S
	return r.handleUpdate(ctx, req, policyResource)
}

func (r *PolicyReconciler) handleUpdate(
	ctx context.Context,
	req reconcile.Request,
	policyResource *s3v1alpha1.Policy,
) (reconcile.Result, error) {
	logger := log.FromContext(ctx)

	s3Client, err := r.S3Instancehelper.GetS3ClientForRessource(
		ctx,
		r.Client,
		r.S3factory,
		policyResource.Name,
		policyResource.Namespace,
		policyResource.Spec.S3InstanceRef,
	)

	if err != nil {
		logger.Error(err, "an error occurred while getting s3Client")
		return r.SetReconciledCondition(
			ctx,
			req,
			policyResource,
			s3v1alpha1.Unreachable,
			"Failed to generate s3client from instance",
			err,
		)
	}

	// Check policy existence on the S3 server
	effectivePolicy, err := s3Client.GetPolicyInfo(policyResource.Spec.Name)
	if err != nil {
		logger.Error(
			err,
			"An error occurred while checking the existence of a policy",
			"policyName",
			policyResource.Spec.Name,
			"NamespacedName",
			req.NamespacedName.String(),
		)
		return r.SetReconciledCondition(
			ctx,
			req,
			policyResource,
			s3v1alpha1.Unreachable,
			"Error while checking if policy already exist",
			err,
		)
	}

	matching, err := r.isPolicyMatchingWithCustomResource(policyResource, effectivePolicy)
	if err != nil {
		logger.Error(
			err,
			"An error occurred while comparing actual and expected configuration for the policy",
			"policyName",
			policyResource.Spec.Name,
			"NamespacedName",
			req.NamespacedName.String(),
		)
		return r.SetReconciledCondition(
			ctx,
			req,
			policyResource,
			s3v1alpha1.Unreachable,
			fmt.Sprintf(
				"The comparison between the effective policy [%s] on S3 and its corresponding custom resource on K8S has failed",
				policyResource.Spec.Name,
			),
			err,
		)
	}

	if !matching {
		// If not we update the policy to match the CR
		err = s3Client.CreateOrUpdatePolicy(
			policyResource.Spec.Name,
			policyResource.Spec.PolicyContent,
		)
		if err != nil {
			logger.Error(
				err,
				"An error occurred while updating the policy",
				"policyName",
				policyResource.Spec.Name,
				"NamespacedName",
				req.NamespacedName.String(),
			)
			r.SetReconciledCondition(
				ctx,
				req,
				policyResource,
				s3v1alpha1.Unreachable,
				fmt.Sprintf(
					"The comparison between the effective policy [%s] on S3 and its corresponding custom resource on K8S has failed",
					policyResource.Spec.Name,
				),
				err,
			)
		}
	}

	return r.SetReconciledCondition(
		ctx,
		req,
		policyResource,
		s3v1alpha1.Reconciled,
		"Policy reconciled",
		nil,
	)
}

func (r *PolicyReconciler) handleCreation(ctx context.Context, req reconcile.Request,
	policyResource *s3v1alpha1.Policy) (reconcile.Result, error) {
	logger := log.FromContext(ctx)

	s3Client, err := r.S3Instancehelper.GetS3ClientForRessource(
		ctx,
		r.Client,
		r.S3factory,
		policyResource.Name,
		policyResource.Namespace,
		policyResource.Spec.S3InstanceRef,
	)

	if err != nil {
		logger.Error(err, "An error occurred while getting s3Client")
		return r.SetReconciledCondition(
			ctx,
			req,
			policyResource,
			s3v1alpha1.Unreachable,
			"Failed to generate s3client from instance",
			err,
		)
	}

	err = s3Client.CreateOrUpdatePolicy(
		policyResource.Spec.Name,
		policyResource.Spec.PolicyContent,
	)

	if err != nil {
		logger.Error(
			err,
			"An error occurred while creating the policy",
			"policyName",
			policyResource.Spec.Name,
			"NamespacedName",
			req.NamespacedName.String(),
		)
		return r.SetReconciledCondition(
			ctx,
			req,
			policyResource,
			s3v1alpha1.Unreachable,
			"Error while creating policy",
			err,
		)
	}

	return r.SetReconciledCondition(
		ctx,
		req,
		policyResource,
		s3v1alpha1.Reconciled,
		"Policy reconciled",
		err,
	)
}
