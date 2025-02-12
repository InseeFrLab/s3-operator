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

package helpers

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type ControllerHelper struct {
}

func NewControllerHelper() *ControllerHelper {
	return &ControllerHelper{}
}

// SetReconciledCondition is a generic helper to update the reconciled condition for any Kubernetes resource.
func (c *ControllerHelper) SetReconciledCondition(
	ctx context.Context,
	statusWriter client.StatusWriter, // Allows updating status for any reconciler
	req reconcile.Request,
	resource client.Object, // Accepts any Kubernetes object with conditions
	conditions *[]metav1.Condition, // Conditions field reference (must be a pointer)
	conditionType string, // The type of condition to set
	reason string,
	message string,
	err error,
	requeueAfter time.Duration, // Requeue period for reconciliation
) (reconcile.Result, error) {
	logger := log.FromContext(ctx)

	var changed bool

	if err != nil {
		logger.Error(err, message, "NamespacedName", req.NamespacedName.String())
		changed = meta.SetStatusCondition(
			conditions,
			metav1.Condition{
				Type:               conditionType,
				Status:             metav1.ConditionFalse,
				ObservedGeneration: resource.GetGeneration(),
				Reason:             reason,
				Message:            fmt.Sprintf("%s: %s", message, err),
			},
		)
	} else {
		logger.Info(message, "NamespacedName", req.NamespacedName.String())
		changed = meta.SetStatusCondition(
			conditions,
			metav1.Condition{
				Type:               conditionType,
				Status:             metav1.ConditionTrue,
				ObservedGeneration: resource.GetGeneration(),
				Reason:             reason,
				Message:            message,
			},
		)
	}

	if changed {
		if errStatusUpdate := statusWriter.Update(ctx, resource); errStatusUpdate != nil {
			logger.Error(errStatusUpdate, "Failed to update resource status", "ObjectKind", resource.GetObjectKind(), "NamespacedName", req.NamespacedName.String())
			return reconcile.Result{}, errStatusUpdate
		}
	}

	return reconcile.Result{RequeueAfter: requeueAfter}, err
}
