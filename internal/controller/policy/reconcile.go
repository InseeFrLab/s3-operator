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
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	k8sapierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	s3v1alpha1 "github.com/InseeFrLab/s3-operator/api/v1alpha1"
	"github.com/minio/madmin-go/v3"
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
		_, err = r.SetProgressingCondition(ctx,
						   req,
						   policyResource,
						   metav1.ConditionUnknown,
						   s3v1alpha1.Reconciling,
						   fmt.Sprintf("Newly discovered resource %s", policyResource.Name))
		if err != nil {
			return ctrl.Result{}, err
		}

		// Let's re-fetch the policyResource Custom Resource after update the status
		// so that we have the latest state of the resource on the cluster and we will avoid
		// raise the issue "the object has been modified, please apply
		// your changes to the latest version and try again" which would re-trigger the reconciliation
		// if we try to update it again in the following operations
		if err := r.Get(ctx, req.NamespacedName, policyResource); err != nil {

			r.SetDegradedCondition(ctx,
					       req,
					       policyResource,
					       metav1.ConditionFalse,
					       s3v1alpha1.K8sApiError,
					       fmt.Sprintf("Failed to re-fetch policy resource %s", policyResource.Name),
					       err)
			return ctrl.Result{}, err
		}
	}

	// Add finalizer for this CR
	if !controllerutil.ContainsFinalizer(policyResource, policyFinalizer) {
		r.SetProgressingCondition(ctx,
					  req,
					  policyResource,
					  metav1.ConditionTrue,
					  s3v1alpha1.Reconciling,
					  fmt.Sprintf("Adding finalizer to policy resource %s", policyResource.Name))
		if ok := controllerutil.AddFinalizer(policyResource, policyFinalizer); !ok {
			r.SetDegradedCondition(ctx,
					       req,
					       policyResource,
					       metav1.ConditionFalse,
					       s3v1alpha1.InternalError,
					       fmt.Sprintf("Failed to add finalizer to policy resource %s", policyResource.Name),
					       err)
			return ctrl.Result{Requeue: true}, nil
		}

		if err := r.Update(ctx, policyResource); err != nil {
			r.SetDegradedCondition(ctx,
					       req,
					       policyResource,
					       metav1.ConditionFalse,
					       s3v1alpha1.K8sApiError,
					       fmt.Sprintf("An error occurred when adding finalizer on policy resource %s", policyResource.Name),
					       err)
			return ctrl.Result{}, err
		}
		// Let's re-fetch the policy Custom Resource after adding the finalizer
		// so that we have the latest state of the resource on the cluster and we will avoid
		// raise the issue "the object has been modified, please apply
		// your changes to the latest version and try again" which would re-trigger the reconciliation
		// if we try to update it again in the following operations
		if err := r.Get(ctx, req.NamespacedName, policyResource); err != nil {
			r.SetDegradedCondition(ctx,
					       req,
					       policyResource,
					       metav1.ConditionFalse,
					       s3v1alpha1.K8sApiError,
					       fmt.Sprintf("Failed to re-fetch policy resource %s", policyResource.Name),
					       err)
			return ctrl.Result{}, err
		}
	}

	// Managing policy deletion with a finalizer
	// REF : https://sdk.operatorframework.io/docs/building-operators/golang/advanced-topics/#external-resources
	if policyResource.GetDeletionTimestamp() != nil {
		r.SetProgressingCondition(ctx,
					  req,
					  policyResource,
					  metav1.ConditionTrue,
					  s3v1alpha1.Reconciling,
					  fmt.Sprintf("policy resource have been marked for deletion %s", policyResource.Name))
		return r.handleDeletion(ctx, req, policyResource)
	}

	// Policy lifecycle management (other than deletion) starts here
	r.SetProgressingCondition(ctx, req, policyResource, metav1.ConditionTrue, s3v1alpha1.Reconciling, fmt.Sprintf("Starting reconciliation of policy %s", policyResource.Name))
	return r.handleReconciliation(ctx, req, policyResource)
}

func (r *PolicyReconciler) handleReconciliation(
	ctx context.Context,
	req reconcile.Request,
	policyResource *s3v1alpha1.Policy,
) (reconcile.Result, error) {


	// Create S3Client
	s3Client, err := r.S3Instancehelper.GetS3ClientForRessource(
		ctx,
		r.Client,
		r.S3factory,
		policyResource.Name,
		policyResource.Namespace,
		policyResource.Spec.S3InstanceRef,
	)
	if err != nil {
		return r.SetDegradedCondition(
			ctx,
			req,
			policyResource,
			metav1.ConditionUnknown,
			s3v1alpha1.Unreachable,
			"Failed to generate s3client from instance",
			err,
		)
	}

	// check policy existence on the s3 server
	effectivePolicy, err := s3Client.GetPolicyInfo(policyResource.Spec.Name)
	if err != nil {
		return r.SetDegradedCondition(
			ctx,
			req,
			policyResource,
			metav1.ConditionUnknown,
			s3v1alpha1.Unreachable,
			fmt.Sprintf("Error while checking if policy %s already exist", policyResource.Spec.Name),
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

	s3Client, err := r.S3Instancehelper.GetS3ClientForRessource(
		ctx,
		r.Client,
		r.S3factory,
		policyResource.Name,
		policyResource.Namespace,
		policyResource.Spec.S3InstanceRef,
	)

	if err != nil {
		return r.SetDegradedCondition(
			ctx,
			req,
			policyResource,
			metav1.ConditionUnknown,
			s3v1alpha1.Unreachable,
			"Failed to generate s3client from instance",
			err,
		)
	}

	// Check policy existence on the S3 server
	effectivePolicy, err := s3Client.GetPolicyInfo(policyResource.Spec.Name)
	if err != nil {
		return r.SetDegradedCondition(
			ctx,
			req,
			policyResource,
			metav1.ConditionUnknown,
			s3v1alpha1.Unreachable,
			fmt.Sprintf("Error while checking if policy %s already exist", policyResource.Spec.Name),
			err,
		)
	}

	matching, err := r.isPolicyMatchingWithCustomResource(policyResource, effectivePolicy)
	if err != nil {
		return r.SetDegradedCondition(
			ctx,
			req,
			policyResource,
			metav1.ConditionFalse,
			s3v1alpha1.Unreachable,
			fmt.Sprintf(
				"The comparison between the effective policy [%s] on S3 and its corresponding custom resource %s on K8S has failed",
				policyResource.Spec.Name,
				policyResource.Name,
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
			r.SetDegradedCondition(
				ctx,
				req,
				policyResource,
				metav1.ConditionFalse,
				s3v1alpha1.Unreachable,
				fmt.Sprintf(
					"The comparison between the effective policy [%s] on S3 and its corresponding custom resource %s on K8S has failed",
					policyResource.Spec.Name,
					policyResource.Name,
				),
				err,
			)
		}
	}

	return r.SetAvailableCondition(
		ctx,
		req,
		policyResource,
		s3v1alpha1.Reconciled,
		"Policy reconciled",
	)
}

func (r *PolicyReconciler) handleCreation(ctx context.Context, req reconcile.Request,
	policyResource *s3v1alpha1.Policy) (reconcile.Result, error) {

	s3Client, err := r.S3Instancehelper.GetS3ClientForRessource(
		ctx,
		r.Client,
		r.S3factory,
		policyResource.Name,
		policyResource.Namespace,
		policyResource.Spec.S3InstanceRef,
	)

	if err != nil {
		return r.SetRejectedCondition(
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

		return r.SetRejectedCondition(
			ctx,
			req,
			policyResource,
			s3v1alpha1.Unreachable,
			"Error while creating policy",
			err,
		)
	}

	return r.SetAvailableCondition(
		ctx,
		req,
		policyResource,
		s3v1alpha1.Reconciled,
		"Policy reconciled",
	)
}

func (r *PolicyReconciler) isPolicyMatchingWithCustomResource(
	policyResource *s3v1alpha1.Policy,
	effectivePolicy *madmin.PolicyInfo,
) (bool, error) {
	// The policy content visible in the custom resource usually contains indentations and newlines
	// while the one we get from S3 is compacted. In order to compare them, we compact the former.

	policyResourceAsByteSlice := []byte(policyResource.Spec.PolicyContent)
	buffer := new(bytes.Buffer)
	err := json.Compact(buffer, policyResourceAsByteSlice)
	if err != nil {
		return false, err
	}

	// Another gotcha is that the effective policy comes up as a json.RawContent,
	// which needs marshalling in order to be properly compared to the []byte we get from the CR.
	marshalled, err := json.Marshal(effectivePolicy.Policy)
	if err != nil {
		return false, err
	}

	return bytes.Equal(buffer.Bytes(), marshalled), nil
}
