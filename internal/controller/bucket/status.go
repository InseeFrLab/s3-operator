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

package bucket_controller

import (
	"context"

	s3v1alpha1 "github.com/InseeFrLab/s3-operator/api/v1alpha1"

	ctrl "sigs.k8s.io/controller-runtime"
)

func (r *BucketReconciler) SetReconciledCondition(
	ctx context.Context,
	req ctrl.Request,
	bucketResource *s3v1alpha1.Bucket,
	reason string,
	message string,
	err error,
) (ctrl.Result, error) {
	return r.ControllerHelper.SetReconciledCondition(
		ctx,
		r.Status(),
		req,
		bucketResource,
		&bucketResource.Status.Conditions,
		s3v1alpha1.ConditionReconciled,
		reason,
		message,
		err,
		r.ReconcilePeriod,
	)
}
