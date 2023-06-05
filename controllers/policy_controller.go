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

package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/minio/madmin-go/v2"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	s3v1alpha1 "github.com/inseefrlab/s3-operator/api/v1alpha1"
	"github.com/inseefrlab/s3-operator/controllers/s3/factory"
)

// PolicyReconciler reconciles a Policy object
type PolicyReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	S3Client factory.S3Client
}

//+kubebuilder:rbac:groups=s3.onyxia.sh,resources=policies,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=s3.onyxia.sh,resources=policies/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=s3.onyxia.sh,resources=policies/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *PolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	policyResource := &s3v1alpha1.Policy{}
	err := r.Get(ctx, req.NamespacedName, policyResource)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("The Policy CRD has been removed ; as such the Policy controller is NOOP.", "req.Name", req.Name)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Check policy existence on the S3 server
	effectivePolicy, err := r.S3Client.GetPolicyInfo(policyResource.Spec.Name)

	// If the policy does not exist on S3...
	if err != nil {
		logger.Error(err, "an error occurred while checking the existence of a policy", "policy", policyResource.Spec.Name)
		// TODO ? : logging in this way gets the error from S3Client, but not the one from r.Status().Update()
		// (this applies to every occurrence of SetBucketStatusCondition down below)
		SetPolicyStatusCondition(policyResource, "OperatorFailed", metav1.ConditionFalse, "PolicyInfoFailed",
			fmt.Sprintf("Obtaining policy[%s] info from S3 instance has failed", policyResource.Spec.Name))
		return ctrl.Result{}, utilerrors.NewAggregate([]error{err, r.Status().Update(ctx, policyResource)})
	}

	if effectivePolicy == nil {

		// Policy creation using info from the CR
		err = r.S3Client.CreateOrUpdatePolicy(policyResource.Spec.Name, policyResource.Spec.PolicyContent)
		if err != nil {
			logger.Error(err, "an error occurred while creating the policy", "policy", policyResource.Spec.Name)
			SetPolicyStatusCondition(policyResource, "OperatorFailed", metav1.ConditionFalse, "PolicyCreationFailed",
				fmt.Sprintf("The creation of policy [%s] has failed", policyResource.Spec.Name))
			return ctrl.Result{}, utilerrors.NewAggregate([]error{err, r.Status().Update(ctx, policyResource)})
		}

		// Update status to reflect policy creation
		SetPolicyStatusCondition(policyResource, "OperatorSucceeded", metav1.ConditionTrue, "PolicyCreated",
			fmt.Sprintf("The creation of policy [%s] has succeeded", policyResource.Spec.Name))
		if err := r.Status().Update(ctx, policyResource); err != nil {
			logger.Error(err, "an error occurred while updating the status after creating the policy", "policy", policyResource.Spec.Name)
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil

	}

	// If the policy exists on S3, we compare its state to the custom resource that spawned it on K8S
	matching, err := IsPolicyMatchingWithCustomResource(policyResource, effectivePolicy)
	if err != nil {
		logger.Error(err, "an error occurred while comparing actual and expected configuration for the policy", "policy", policyResource.Spec.Name)
		SetPolicyStatusCondition(policyResource, "OperatorFailed", metav1.ConditionFalse, "PolicyComparisonFailed",
			fmt.Sprintf("The comparison between the effective policy [%s] on S3 and its corresponding custom resource on K8S has failed", policyResource.Spec.Name))
		return ctrl.Result{}, utilerrors.NewAggregate([]error{err, r.Status().Update(ctx, policyResource)})
	}
	// If the two match, no reconciliation is needed, but we still need to update
	// the status, in case the generation changed (eg : rollback to previous state after a problematic change)
	if matching {
		SetPolicyStatusCondition(policyResource, "OperatorSucceeded", metav1.ConditionTrue, "PolicyUnchanged",
			fmt.Sprintf("The policy [%s] matches its corresponding custom resource", policyResource.Spec.Name))
		if err := r.Status().Update(ctx, policyResource); err != nil {
			logger.Error(err, "an error occurred while updating the status after comparing the actual and expected configuration for the policy", "policy", policyResource.Spec.Name)
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}
	// If not we update the policy to match the CR
	err = r.S3Client.CreateOrUpdatePolicy(policyResource.Spec.Name, policyResource.Spec.PolicyContent)
	if err != nil {
		logger.Error(err, "an error occurred while updating the policy", "policy", policyResource.Spec.Name)
		SetPolicyStatusCondition(policyResource, "OperatorFailed", metav1.ConditionFalse, "PolicyUpdateFailed",
			fmt.Sprintf("The update of effective policy [%s] on S3 to match its corresponding custom resource on K8S has failed", policyResource.Spec.Name))
		return ctrl.Result{}, utilerrors.NewAggregate([]error{err, r.Status().Update(ctx, policyResource)})
	}

	// Update status to reflect policy update
	SetPolicyStatusCondition(policyResource, "OperatorSucceeded", metav1.ConditionTrue, "PolicyUpdated",
		fmt.Sprintf("The policy [%s] was updated according to its matching custom resource", policyResource.Spec.Name))
	if err := r.Status().Update(ctx, policyResource); err != nil {
		logger.Error(err, "an error occurred while updating the status after updating the policy", "policy", policyResource.Spec.Name)
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&s3v1alpha1.Policy{}).
		// TODO : implement a real strategy for event filtering ; for now just using the example from OpSDK doc
		// (https://sdk.operatorframework.io/docs/building-operators/golang/references/event-filtering/)
		WithEventFilter(predicate.Funcs{
			UpdateFunc: func(e event.UpdateEvent) bool {
				// Ignore updates to CR status in which case metadata.Generation does not change
				return e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration()
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				// Evaluates to false if the object has been confirmed deleted.
				return !e.DeleteStateUnknown
			},
		}).
		WithOptions(controller.Options{MaxConcurrentReconciles: 10}).
		Complete(r)
}

func IsPolicyMatchingWithCustomResource(policyResource *s3v1alpha1.Policy, effectivePolicy *madmin.PolicyInfo) (bool, error) {
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

func SetPolicyStatusCondition(policyResource *s3v1alpha1.Policy, conditionType string, status metav1.ConditionStatus, reason string, message string) {
	meta.SetStatusCondition(&policyResource.Status.Conditions,
		metav1.Condition{
			Type:               conditionType,
			Status:             status,
			Reason:             reason,
			LastTransitionTime: metav1.NewTime(time.Now()),
			Message:            message,
			ObservedGeneration: policyResource.GetGeneration(),
		})
}
