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
	"context"
	"fmt"
	"reflect"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
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
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	s3v1alpha1 "github.com/InseeFrLab/s3-operator/api/v1alpha1"
	s3ClientCache "github.com/InseeFrLab/s3-operator/internal/s3"
	s3Factory "github.com/InseeFrLab/s3-operator/internal/s3/factory"

	utils "github.com/InseeFrLab/s3-operator/internal/utils"
)

// S3InstanceReconciler reconciles a S3Instance object
type S3InstanceReconciler struct {
	client.Client
	Scheme               *runtime.Scheme
	S3ClientCache        *s3ClientCache.S3ClientCache
	S3LabelSelectorValue string
}

const (
	s3InstanceFinalizer = "s3.onyxia.sh/s3InstanceFinalizer"
)

//+kubebuilder:rbac:groups=s3.onyxia.sh,resources=S3Instance,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=s3.onyxia.sh,resources=S3Instance/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=s3.onyxia.sh,resources=S3Instance/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *S3InstanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Checking for s3InstanceResource existence
	s3InstanceResource := &s3v1alpha1.S3Instance{}
	err := r.Get(ctx, req.NamespacedName, s3InstanceResource)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info(fmt.Sprintf("The S3InstanceResource CR %s has been removed. NOOP", req.Name))
			return ctrl.Result{}, nil
		}
		logger.Error(err, "An error occurred when fetching the S3InstanceResource from Kubernetes")
		return ctrl.Result{}, err
	}

	// check if this object must be manage by this instance
	if r.S3LabelSelectorValue != "" {
		labelSelectorValue, found := s3InstanceResource.Labels[utils.S3OperatorS3InstanceLabelSelectorKey]
		if !found {
			logger.Info("This s3Instance ressouce will not be manage by this instance because this instance require that s3Instance get labelSelector and label selector not found", "req.Name", req.Name, "Bucket Labels", s3InstanceResource.Labels, "S3OperatorBucketLabelSelectorKey", utils.S3OperatorBucketLabelSelectorKey)
			return ctrl.Result{}, nil
		}
		if labelSelectorValue != r.S3LabelSelectorValue {
			logger.Info("This s3Instance ressouce will not be manage by this instance because this instance require that s3Instance get specific a specific labelSelector value", "req.Name", req.Name, "expected", r.S3LabelSelectorValue, "current", labelSelectorValue)
			return ctrl.Result{}, nil
		}
	}

	// Check if the s3InstanceResource instance is marked to be deleted, which is
	// indicated by the deletion timestamp being set. The object will be deleted.
	if s3InstanceResource.GetDeletionTimestamp() != nil {
		logger.Info("s3InstanceResource have been marked for deletion")
		return r.handleS3InstanceDeletion(ctx, s3InstanceResource)
	}

	// Add finalizer for this CR
	if !controllerutil.ContainsFinalizer(s3InstanceResource, s3InstanceFinalizer) {
		logger.Info("adding finalizer to s3Instance")

		controllerutil.AddFinalizer(s3InstanceResource, s3InstanceFinalizer)
		err = r.Update(ctx, s3InstanceResource)
		if err != nil {
			logger.Error(err, "an error occurred when adding finalizer from s3Instance", "s3Instance", s3InstanceResource.Name)
			return r.setS3InstanceStatusConditionAndUpdate(ctx, s3InstanceResource, "OperatorFailed", metav1.ConditionFalse, "S3InstanceFinalizerAddFailed",
				fmt.Sprintf("An error occurred when attempting to add the finalizer from s3Instance %s", s3InstanceResource.Name), err)
		}
	}

	// Check s3Instance existence
	_, found := r.S3ClientCache.Get(s3InstanceResource.Name)
	// If the s3Instance does not exist, it is created based on the CR
	if !found {
		logger.Info("this S3Instance doesn't exist and will be created")
		return r.handleS3InstanceCreation(ctx, s3InstanceResource)
	}
	logger.Info("this S3Instance already exists and will be reconciled")
	return r.handleS3InstanceUpdate(ctx, s3InstanceResource)

}

func (r *S3InstanceReconciler) handleS3InstanceUpdate(ctx context.Context, s3InstanceResource *s3v1alpha1.S3Instance) (reconcile.Result, error) {
	logger := log.FromContext(ctx)

	s3Client, found := r.S3ClientCache.Get(s3InstanceResource.Name)
	if !found {
		err := &s3ClientCache.S3ClientCacheError{Reason: fmt.Sprintf("S3InstanceRef: %s,not found in cache", s3InstanceResource.Name)}
		logger.Error(err, "No client was found")
	}
	s3Config := s3Client.GetConfig()

	// Get S3_ACCESS_KEY and S3_SECRET_KEY related to this s3Instance

	s3InstanceSecretSecretExpected, err := r.getS3InstanceSecret(ctx, s3InstanceResource)
	if err != nil {
		logger.Error(err, "Could not get s3InstanceSecret in namespace", "s3InstanceSecretRefName", s3InstanceResource.Spec.SecretName)
		return r.setS3InstanceStatusConditionAndUpdate(ctx, s3InstanceResource, "OperatorFailed", metav1.ConditionFalse, "S3InstanceUpdateFailed",
			fmt.Sprintf("Updating secret of S3Instance %s has failed", s3InstanceResource.Name), err)
	}

	// if s3Provider have change recreate totaly One Differ instance will be deleted and recreated
	if s3Config.S3Provider != s3InstanceResource.Spec.S3Provider || s3Config.S3UrlEndpoint != s3InstanceResource.Spec.UrlEndpoint || s3Config.UseSsl != s3InstanceResource.Spec.UseSSL || s3Config.Region != s3InstanceResource.Spec.Region || !reflect.DeepEqual(s3Config.CaCertificatesBase64, s3InstanceResource.Spec.CaCertificatesBase64) || s3Config.AccessKey != string(s3InstanceSecretSecretExpected.Data["S3_ACCESS_KEY"]) || s3Config.SecretKey != string(s3InstanceSecretSecretExpected.Data["S3_SECRET_KEY"]) {
		logger.Info("Instance in cache not equal to expected , cache will be prune and instance recreate", "s3InstanceSecretRefName", s3InstanceResource.Spec.SecretName)
		r.S3ClientCache.Remove(s3InstanceResource.Name)
		return r.handleS3InstanceCreation(ctx, s3InstanceResource)
	}

	return r.setS3InstanceStatusConditionAndUpdate(ctx, s3InstanceResource, "OperatorSucceeded", metav1.ConditionTrue, "S3InstanceUpdated",
		fmt.Sprintf("The S3Instance %s was updated was reconcile successfully", s3InstanceResource.Name), nil)
}

func (r *S3InstanceReconciler) handleS3InstanceCreation(ctx context.Context, s3InstanceResource *s3v1alpha1.S3Instance) (reconcile.Result, error) {
	logger := log.FromContext(ctx)

	s3InstanceSecretSecret, err := r.getS3InstanceSecret(ctx, s3InstanceResource)
	if err != nil {
		logger.Error(err, "Could not get s3InstanceSecret in namespace", "s3InstanceSecretRefName", s3InstanceResource.Spec.SecretName)
		return r.setS3InstanceStatusConditionAndUpdate(ctx, s3InstanceResource, "OperatorFailed", metav1.ConditionFalse, "S3InstanceCreationFailed",
			fmt.Sprintf("Getting secret of S3s3Instance %s has failed", s3InstanceResource.Name), err)

	}

	s3Config := &s3Factory.S3Config{S3Provider: s3InstanceResource.Spec.S3Provider, AccessKey: string(s3InstanceSecretSecret.Data["S3_ACCESS_KEY"]), SecretKey: string(s3InstanceSecretSecret.Data["S3_SECRET_KEY"]), S3UrlEndpoint: s3InstanceResource.Spec.UrlEndpoint, Region: s3InstanceResource.Spec.Region, UseSsl: s3InstanceResource.Spec.UseSSL, CaCertificatesBase64: s3InstanceResource.Spec.CaCertificatesBase64}

	s3Client, err := s3Factory.GenerateS3Client(s3Config.S3Provider, s3Config)
	if err != nil {
		return r.setS3InstanceStatusConditionAndUpdate(ctx, s3InstanceResource, "OperatorFailed", metav1.ConditionFalse, "S3InstanceCreationFailed",
			fmt.Sprintf("Error while creating s3Instance %s", s3InstanceResource.Name), err)
	}

	r.S3ClientCache.Set(s3InstanceResource.Name, s3Client)

	return r.setS3InstanceStatusConditionAndUpdate(ctx, s3InstanceResource, "OperatorSucceeded", metav1.ConditionTrue, "S3InstanceCreated",
		fmt.Sprintf("The S3Instance %s was created successfully", s3InstanceResource.Name), nil)

}

func (r *S3InstanceReconciler) handleS3InstanceDeletion(ctx context.Context, s3InstanceResource *s3v1alpha1.S3Instance) (reconcile.Result, error) {
	logger := log.FromContext(ctx)

	if controllerutil.ContainsFinalizer(s3InstanceResource, s3InstanceFinalizer) {
		// Run finalization logic for S3InstanceFinalizer. If the finalization logic fails, don't remove the finalizer so that we can retry during the next reconciliation.
		if err := r.finalizeS3Instance(ctx, s3InstanceResource); err != nil {
			logger.Error(err, "an error occurred when attempting to finalize the s3Instance", "s3Instance", s3InstanceResource.Name)
			return r.setS3InstanceStatusConditionAndUpdate(ctx, s3InstanceResource, "OperatorFailed", metav1.ConditionFalse, "S3InstanceFinalizeFailed",
				fmt.Sprintf("An error occurred when attempting to delete s3Instance %s", s3InstanceResource.Name), err)
		}

		//Remove s3InstanceFinalizer. Once all finalizers have been removed, the object will be deleted.
		controllerutil.RemoveFinalizer(s3InstanceResource, s3InstanceFinalizer)
		// Unsure why the behavior is different to that of bucket/policy/path controllers, but it appears
		// calling r.Update() for adding/removal of finalizer is not necessary (an update event is generated
		// with the call to AddFinalizer/RemoveFinalizer), and worse, causes "freshness" problem (with the
		// "the object has been modified; please apply your changes to the latest version and try again" error)
		err := r.Update(ctx, s3InstanceResource)
		if err != nil {
			logger.Error(err, "Failed to remove finalizer.")
			return r.setS3InstanceStatusConditionAndUpdate(ctx, s3InstanceResource, "OperatorFailed", metav1.ConditionFalse, "S3InstanceFinalizerRemovalFailed",
				fmt.Sprintf("An error occurred when attempting to remove the finalizer from s3Instance %s", s3InstanceResource.Name), err)
		}
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.*
func (r *S3InstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// filterLogger := ctrl.Log.WithName("filterEvt")
	return ctrl.NewControllerManagedBy(mgr).
		For(&s3v1alpha1.S3Instance{}).
		// The "secret owning" implies the reconcile loop will be called whenever a Secret owned
		// by a S3Instance is created/updated/deleted. In other words, even when creating a single S3Instance,
		// there is going to be several iterations.
		Owns(&corev1.Secret{}).
		// See : https://sdk.operatorframework.io/docs/building-operators/golang/references/event-filtering/
		WithEventFilter(predicate.Funcs{

			// Ignore updates to CR status in which case metadata.Generation does not change,
			// unless it is a change to the underlying Secret
			UpdateFunc: func(e event.UpdateEvent) bool {

				// To check if the update event is tied to a change on secret,
				// we try to cast e.ObjectNew to a secret (only if it's not a S3Instance, which
				// should prevent any TypeAssertionError based panic).
				secretUpdate := false
				newUser, _ := e.ObjectNew.(*s3v1alpha1.S3Instance)
				if newUser == nil {
					secretUpdate = (e.ObjectNew.(*corev1.Secret) != nil)
				}

				return e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration() || secretUpdate
			},
			// Ignore create events caused by the underlying secret's creation
			CreateFunc: func(e event.CreateEvent) bool {
				s3Instance, _ := e.Object.(*s3v1alpha1.S3Instance)
				return s3Instance != nil
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				// Evaluates to false if the object has been confirmed deleted.
				return !e.DeleteStateUnknown
			},
		}).
		WithOptions(controller.Options{MaxConcurrentReconciles: 10}).
		Complete(r)
}

func (r *S3InstanceReconciler) setS3InstanceStatusConditionAndUpdate(ctx context.Context, s3InstanceResource *s3v1alpha1.S3Instance, conditionType string, status metav1.ConditionStatus, reason string, message string, srcError error) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// We moved away from meta.SetStatusCondition, as the implementation did not allow for updating
	// lastTransitionTime if a Condition (as identified by Reason instead of Type) was previously
	// obtained and updated to again.
	s3InstanceResource.Status.Conditions = utils.UpdateConditions(s3InstanceResource.Status.Conditions, metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		LastTransitionTime: metav1.NewTime(time.Now()),
		Message:            message,
		ObservedGeneration: s3InstanceResource.GetGeneration(),
	})

	err := r.Status().Update(ctx, s3InstanceResource)
	if err != nil {
		logger.Error(err, "an error occurred while updating the status of the S3Instance resource")
		return ctrl.Result{}, utilerrors.NewAggregate([]error{err, srcError})
	}
	return ctrl.Result{}, srcError
}

func (r *S3InstanceReconciler) finalizeS3Instance(ctx context.Context, s3InstanceResource *s3v1alpha1.S3Instance) error {
	logger := log.FromContext(ctx)
	// Create S3Client
	logger.Info(fmt.Sprintf("Search S3Instance %s to delete in cache , search instance in cache", s3InstanceResource.Name))
	_, found := r.S3ClientCache.Get(s3InstanceResource.Name)
	if !found {
		err := &s3ClientCache.S3ClientCacheError{Reason: fmt.Sprintf("S3InstanceRef: %s,not found in cache cannot finalize", s3InstanceResource.Name)}
		logger.Error(err, "No client was found")
		return err
	}
	r.S3ClientCache.Remove(s3InstanceResource.Name)
	return nil
}

func (r *S3InstanceReconciler) getS3InstanceSecret(ctx context.Context, s3InstanceResource *s3v1alpha1.S3Instance) (corev1.Secret, error) {
	logger := log.FromContext(ctx)

	secretsList := &corev1.SecretList{}
	s3InstanceSecret := corev1.Secret{}

	err := r.List(ctx, secretsList, client.InNamespace(s3InstanceResource.Namespace))
	if err != nil {
		logger.Error(err, "An error occurred while listing the secrets in s3instance's namespace")
		return s3InstanceSecret, fmt.Errorf("SecretListingFailed")
	}

	if len(secretsList.Items) == 0 {
		logger.Info("The s3instance's namespace doesn't appear to contain any secret")
		return s3InstanceSecret, nil
	}
	// In all the secrets inside the s3instance's namespace, one should have a name equal to
	// the S3InstanceSecretRefName field.
	s3InstanceSecretName := s3InstanceResource.Spec.SecretName

	// cmp.Or takes the first non "zero" value, see https://pkg.go.dev/cmp#Or
	for _, secret := range secretsList.Items {
		if secret.Name == s3InstanceSecretName {
			s3InstanceSecret = secret
			break
		}
	}

	return s3InstanceSecret, nil
}
