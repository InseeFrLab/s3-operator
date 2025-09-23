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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	s3v1alpha1 "github.com/InseeFrLab/s3-operator/api/v1alpha1"
)

func (r *PolicyReconciler) SetProgressingCondition(
	ctx context.Context,
	req ctrl.Request,
	policyResource *s3v1alpha1.Policy,
	status metav1.ConditionStatus,
	reason string,
	message string,
) (ctrl.Result, error) {
	return r.ControllerHelper.SetReconciledCondition(
		ctx,
		r.Status(),
		req,
		policyResource,
		&policyResource.Status.Conditions,
		s3v1alpha1.ConditionProgressing,
		status,
		reason,
		message,
		nil,
		r.ReconcilePeriod,
	)
}
func (r *PolicyReconciler) SetAvailableCondition(
	ctx context.Context,
	req ctrl.Request,
	policyResource *s3v1alpha1.Policy,
	reason string,
	message string,
) (ctrl.Result, error) {
	return r.ControllerHelper.SetReconciledCondition(
		ctx,
		r.Status(),
		req,
		policyResource,
		&policyResource.Status.Conditions,
		s3v1alpha1.ConditionAvailable,
		metav1.ConditionTrue,
		reason,
		message,
		nil,
		r.ReconcilePeriod,
	)
}
func (r *PolicyReconciler) SetDegradedCondition(
	ctx context.Context,
	req ctrl.Request,
	policyResource *s3v1alpha1.Policy,
	status metav1.ConditionStatus,
	reason string,
	message string,
	err error,
) (ctrl.Result, error) {
	return r.ControllerHelper.SetReconciledCondition(
		ctx,
		r.Status(),
		req,
		policyResource,
		&policyResource.Status.Conditions,
		s3v1alpha1.ConditionDegraded,
		status,
		reason,
		message,
		err,
		r.ReconcilePeriod,
	)
}
func (r *PolicyReconciler) SetRejectedCondition(
	ctx context.Context,
	req ctrl.Request,
	policyResource *s3v1alpha1.Policy,
	reason string,
	message string,
	err error,
) (ctrl.Result, error) {
	return r.ControllerHelper.SetReconciledCondition(
		ctx,
		r.Status(),
		req,
		policyResource,
		&policyResource.Status.Conditions,
		s3v1alpha1.ConditionRejected,
		metav1.ConditionFalse,
		reason,
		message,
		err,
		r.ReconcilePeriod,
	)
}
