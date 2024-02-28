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

	"github.com/minio/madmin-go/v3"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	s3v1alpha1 "github.com/InseeFrLab/s3-operator/api/v1alpha1"
	"github.com/InseeFrLab/s3-operator/controllers/s3/factory"
)

// PolicyReconciler reconciles a Policy object
type PolicyReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	S3Client       factory.S3Client
	PolicyDeletion bool
}

//+kubebuilder:rbac:groups=s3.onyxia.sh,resources=policies,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=s3.onyxia.sh,resources=policies/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=s3.onyxia.sh,resources=policies/finalizers,verbs=update

const policyFinalizer = "s3.onyxia.sh/finalizer"

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *PolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	errorLogger := log.FromContext(ctx)
	logger := ctrl.Log.WithName("policyReconcile")

	// Checking for policy resource existence
	policyResource := &s3v1alpha1.Policy{}
	err := r.Get(ctx, req.NamespacedName, policyResource)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("The Policy custom resource has been removed ; as such the Policy controller is NOOP.", "req.Name", req.Name)
			return ctrl.Result{}, nil
		}
		errorLogger.Error(err, "An error occurred when attempting to read the Policy resource from the Kubernetes cluster")
		return ctrl.Result{}, err
	}

	// Managing policy deletion with a finalizer
	// REF : https://sdk.operatorframework.io/docs/building-operators/golang/advanced-topics/#external-resources
	isMarkedForDeletion := policyResource.GetDeletionTimestamp() != nil
	if isMarkedForDeletion {
		if controllerutil.ContainsFinalizer(policyResource, policyFinalizer) {
			// Run finalization logic for policyFinalizer. If the
			// finalization logic fails, don't remove the finalizer so
			// that we can retry during the next reconciliation.
			if err := r.finalizePolicy(policyResource); err != nil {
				// return ctrl.Result{}, err
				errorLogger.Error(err, "an error occurred when attempting to finalize the policy", "policy", policyResource.Spec.Name)
				// return ctrl.Result{}, err
				return r.SetPolicyStatusConditionAndUpdate(ctx, policyResource, "OperatorFailed", metav1.ConditionFalse, "PolicyFinalizeFailed",
					fmt.Sprintf("An error occurred when attempting to delete policy [%s]", policyResource.Spec.Name), err)
			}

			// Remove policyFinalizer. Once all finalizers have been
			// removed, the object will be deleted.
			controllerutil.RemoveFinalizer(policyResource, policyFinalizer)
			err := r.Update(ctx, policyResource)
			if err != nil {
				errorLogger.Error(err, "an error occurred when removing finalizer from policy", "policy", policyResource.Spec.Name)
				// return ctrl.Result{}, err
				return r.SetPolicyStatusConditionAndUpdate(ctx, policyResource, "OperatorFailed", metav1.ConditionFalse, "PolicyFinalizerRemovalFailed",
					fmt.Sprintf("An error occurred when attempting to remove the finalizer from policy [%s]", policyResource.Spec.Name), err)
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer for this CR
	if !controllerutil.ContainsFinalizer(policyResource, policyFinalizer) {
		controllerutil.AddFinalizer(policyResource, policyFinalizer)
		err = r.Update(ctx, policyResource)
		if err != nil {
			errorLogger.Error(err, "an error occurred when adding finalizer from policy", "policy", policyResource.Spec.Name)
			// return ctrl.Result{}, err
			return r.SetPolicyStatusConditionAndUpdate(ctx, policyResource, "OperatorFailed", metav1.ConditionFalse, "PolicyFinalizerAddFailed",
				fmt.Sprintf("An error occurred when attempting to add the finalizer from policy [%s]", policyResource.Spec.Name), err)
		}
	}

	// Policy lifecycle management (other than deletion) starts here

	// Check policy existence on the S3 server
	effectivePolicy, err := r.S3Client.GetPolicyInfo(policyResource.Spec.Name)

	// If the policy does not exist on S3...
	if err != nil {
		errorLogger.Error(err, "an error occurred while checking the existence of a policy", "policy", policyResource.Spec.Name)
		return r.SetPolicyStatusConditionAndUpdate(ctx, policyResource, "OperatorFailed", metav1.ConditionFalse, "PolicyInfoFailed",
			fmt.Sprintf("Obtaining policy[%s] info from S3 instance has failed", policyResource.Spec.Name), err)
	}

	if effectivePolicy == nil {

		// Policy creation using info from the CR
		err = r.S3Client.CreateOrUpdatePolicy(policyResource.Spec.Name, policyResource.Spec.PolicyContent)
		if err != nil {
			errorLogger.Error(err, "an error occurred while creating the policy", "policy", policyResource.Spec.Name)
			return r.SetPolicyStatusConditionAndUpdate(ctx, policyResource, "OperatorFailed", metav1.ConditionFalse, "PolicyCreationFailed",
				fmt.Sprintf("The creation of policy [%s] has failed", policyResource.Spec.Name), err)
		}

		// Update status to reflect policy creation
		return r.SetPolicyStatusConditionAndUpdate(ctx, policyResource, "OperatorSucceeded", metav1.ConditionTrue, "PolicyCreated",
			fmt.Sprintf("The creation of policy [%s] has succeeded", policyResource.Spec.Name), nil)

	}

	// If the policy exists on S3, we compare its state to the custom resource that spawned it on K8S
	matching, err := IsPolicyMatchingWithCustomResource(policyResource, effectivePolicy)
	if err != nil {
		errorLogger.Error(err, "an error occurred while comparing actual and expected configuration for the policy", "policy", policyResource.Spec.Name)
		return r.SetPolicyStatusConditionAndUpdate(ctx, policyResource, "OperatorFailed", metav1.ConditionFalse, "PolicyComparisonFailed",
			fmt.Sprintf("The comparison between the effective policy [%s] on S3 and its corresponding custom resource on K8S has failed", policyResource.Spec.Name), err)
	}
	// If the two match, no reconciliation is needed, but we still need to update
	// the status, in case the generation changed (eg : rollback to previous state after a problematic change)
	if matching {
		return r.SetPolicyStatusConditionAndUpdate(ctx, policyResource, "OperatorSucceeded", metav1.ConditionTrue, "PolicyUnchanged",
			fmt.Sprintf("The policy [%s] matches its corresponding custom resource", policyResource.Spec.Name), nil)
	}

	// If not we update the policy to match the CR
	err = r.S3Client.CreateOrUpdatePolicy(policyResource.Spec.Name, policyResource.Spec.PolicyContent)
	if err != nil {
		errorLogger.Error(err, "an error occurred while updating the policy", "policy", policyResource.Spec.Name)
		return r.SetPolicyStatusConditionAndUpdate(ctx, policyResource, "OperatorFailed", metav1.ConditionFalse, "PolicyUpdateFailed",
			fmt.Sprintf("The update of effective policy [%s] on S3 to match its corresponding custom resource on K8S has failed", policyResource.Spec.Name), err)
	}

	// Update status to reflect policy update
	return r.SetPolicyStatusConditionAndUpdate(ctx, policyResource, "OperatorSucceeded", metav1.ConditionTrue, "PolicyUpdated",
		fmt.Sprintf("The policy [%s] was updated according to its matching custom resource", policyResource.Spec.Name), nil)
}

// SetupWithManager sets up the controller with the Manager.
func (r *PolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	logger := ctrl.Log.WithName("policyEventFilter")
	return ctrl.NewControllerManagedBy(mgr).
		For(&s3v1alpha1.Policy{}).
		// REF : https://sdk.operatorframework.io/docs/building-operators/golang/references/event-filtering/
		WithEventFilter(predicate.Funcs{
			UpdateFunc: func(e event.UpdateEvent) bool {
				// Only reconcile if :
				// - Generation has changed
				//   or
				// - Of all Conditions matching the last generation, none is in status "True"
				// There is an implicit assumption that in such a case, the resource was once failing, but then transitioned
				// to a functional state. We use this ersatz because lastTransitionTime appears to not work properly - see also
				// comment in SetPolicyStatusConditionAndUpdate() below.
				newPolicy, _ := e.ObjectNew.(*s3v1alpha1.Policy)

				// 1 - Identifying the most recent generation
				var maxGeneration int64 = 0
				for _, condition := range newPolicy.Status.Conditions {
					if condition.ObservedGeneration > maxGeneration {
						maxGeneration = condition.ObservedGeneration
					}
				}
				// 2 - Checking one of the conditions in most recent generation is True
				conditionTrueInLastGeneration := false
				for _, condition := range newPolicy.Status.Conditions {
					if condition.ObservedGeneration == maxGeneration && condition.Status == metav1.ConditionTrue {
						conditionTrueInLastGeneration = true
					}
				}
				predicate := e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration() || !conditionTrueInLastGeneration
				if !predicate {
					logger.Info("reconcile update event is filtered out", "resource", e.ObjectNew.GetName())
				}
				return predicate
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				// Evaluates to false if the object has been confirmed deleted.
				logger.Info("reconcile delete event is filtered out", "resource", e.Object.GetName())
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

func (r *PolicyReconciler) finalizePolicy(policyResource *s3v1alpha1.Policy) error {
	if r.PolicyDeletion {
		return r.S3Client.DeletePolicy(policyResource.Spec.Name)
	}
	return nil
}

func (r *PolicyReconciler) SetPolicyStatusConditionAndUpdate(ctx context.Context, policyResource *s3v1alpha1.Policy, conditionType string, status metav1.ConditionStatus, reason string, message string, srcError error) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// It would seem LastTransitionTime does not work as intended (our understanding of the intent coming from this :
	// https://pkg.go.dev/k8s.io/apimachinery@v0.28.3/pkg/api/meta#SetStatusCondition). Whether we set the
	// date manually or leave it out to have default behavior, the lastTransitionTime is NOT updated if the CR
	// had that condition at least once in the past.
	// For instance, with the following updates to a CR :
	//	- gen 1 : condition type = A
	//	- gen 2 : condition type = B
	//	- gen 3 : condition type = A again
	// Then the condition with type A in CR Status will still have the lastTransitionTime dating back to gen 1.
	// Because of this, lastTransitionTime cannot be reliably used to determine current state, which in turn had
	// us turn to a less than ideal event filter (see above in SetupWithManager())
	meta.SetStatusCondition(&policyResource.Status.Conditions,
		metav1.Condition{
			Type:   conditionType,
			Status: status,
			Reason: reason,
			// LastTransitionTime: metav1.NewTime(time.Now()),
			Message:            message,
			ObservedGeneration: policyResource.GetGeneration(),
		})

	err := r.Status().Update(ctx, policyResource)
	if err != nil {
		logger.Error(err, "an error occurred while updating the status of the policy resource")
		return ctrl.Result{}, utilerrors.NewAggregate([]error{err, srcError})
	}
	return ctrl.Result{}, srcError
}
