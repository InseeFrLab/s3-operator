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
	"time"

	k8sapierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	s3v1alpha1 "github.com/InseeFrLab/s3-operator/api/v1alpha1"
	controllerhelpers "github.com/InseeFrLab/s3-operator/internal/controllerhelper"
)

// S3InstanceReconciler reconciles a S3Instance object
type S3InstanceReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	ReconcilePeriod time.Duration
}

const (
	s3InstanceFinalizer = "s3.onyxia.sh/finalizer"
)

//+kubebuilder:rbac:groups=s3.onyxia.sh,resources=s3instances,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=s3.onyxia.sh,resources=s3instances/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=s3.onyxia.sh,resources=s3instances/finalizers,verbs=update

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
		if k8sapierrors.IsNotFound(err) {
			logger.Info(fmt.Sprintf("The S3InstanceResource CR %s has been removed. NOOP", req.Name), "NamespacedName", req.NamespacedName.String())
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get S3InstanceResource", "NamespacedName", req.NamespacedName.String())
		return ctrl.Result{}, err
	}

	// Let's just set the status as Unknown when no status are available
	if len(s3InstanceResource.Status.Conditions) == 0 {
		meta.SetStatusCondition(&s3InstanceResource.Status.Conditions, metav1.Condition{Type: s3v1alpha1.ConditionReconciled, Status: metav1.ConditionUnknown, ObservedGeneration: s3InstanceResource.Generation, Reason: s3v1alpha1.Reconciling, Message: "Starting reconciliation"})
		if err = r.Status().Update(ctx, s3InstanceResource); err != nil {
			logger.Error(err, "Failed to update s3InstanceResource status", "NamespacedName", req.NamespacedName.String())
			return ctrl.Result{}, err
		}

		// Let's re-fetch the s3InstanceResource Custom Resource after update the status
		// so that we have the latest state of the resource on the cluster and we will avoid
		// raise the issue "the object has been modified, please apply
		// your changes to the latest version and try again" which would re-trigger the reconciliation
		// if we try to update it again in the following operations
		if err := r.Get(ctx, req.NamespacedName, s3InstanceResource); err != nil {
			logger.Error(err, "Failed to re-fetch GrafanaInstance", "NamespacedName", req.NamespacedName.String())
			return ctrl.Result{}, err
		}
	}

	// Add finalizer for this CR
	if !controllerutil.ContainsFinalizer(s3InstanceResource, s3InstanceFinalizer) {
		logger.Info("adding finalizer to s3Instance", "NamespacedName", req.NamespacedName.String())
		if ok := controllerutil.AddFinalizer(s3InstanceResource, s3InstanceFinalizer); !ok {
			logger.Error(err, "Failed to add finalizer into the s3Instance", "NamespacedName", req.NamespacedName.String())
			return ctrl.Result{Requeue: true}, nil
		}

		if err = r.Update(ctx, s3InstanceResource); err != nil {
			logger.Error(err, "an error occurred when adding finalizer from s3Instance", "s3Instance", s3InstanceResource.Name)
			return ctrl.Result{}, err
		}

		// Let's re-fetch the S3Instance Custom Resource after adding the finalizer
		// so that we have the latest state of the resource on the cluster and we will avoid
		// raise the issue "the object has been modified, please apply
		// your changes to the latest version and try again" which would re-trigger the reconciliation
		// if we try to update it again in the following operations
		if err := r.Get(ctx, req.NamespacedName, s3InstanceResource); err != nil {
			logger.Error(err, "Failed to re-fetch s3Instance", "NamespacedName", req.NamespacedName.String())
			return ctrl.Result{}, err
		}

	}

	// Check if the s3InstanceResource instance is marked to be deleted, which is
	// indicated by the deletion timestamp being set. The object will be deleted.
	if s3InstanceResource.GetDeletionTimestamp() != nil {
		logger.Info("s3InstanceResource have been marked for deletion")
		return r.handleS3InstanceDeletion(ctx, req, s3InstanceResource)
	}

	// Reconciliation starts here
	return r.handleReconciliation(ctx, req, s3InstanceResource)

}

func (r *S3InstanceReconciler) handleReconciliation(ctx context.Context, req reconcile.Request, s3InstanceResource *s3v1alpha1.S3Instance) (reconcile.Result, error) {
	logger := log.FromContext(ctx)

	s3Client, err := controllerhelpers.GetS3ClientFromS3Instance(ctx, r.Client, s3InstanceResource)

	if err != nil {
		logger.Error(err, "Could not generate s3Instance", "s3InstanceSecretRefName", s3InstanceResource.Spec.SecretRef, "NamespacedName", req.NamespacedName.String())
		return r.SetReconciledCondition(ctx, req, s3InstanceResource, s3v1alpha1.CreationFailure,
			"Failed to generate S3Instance ", err)
	}

	_, err = s3Client.ListBuckets()
	if err != nil {
		logger.Error(err, "Could not generate s3Instance", "s3InstanceName", s3InstanceResource.Name, "NamespacedName", req.NamespacedName.String())
		return r.SetReconciledCondition(ctx, req, s3InstanceResource, s3v1alpha1.CreationFailure,
			"Failed to generate S3Instance ", err)
	}

	return r.SetReconciledCondition(ctx, req, s3InstanceResource, s3v1alpha1.Reconciled, "S3Instance instance reconciled", nil)

}

func (r *S3InstanceReconciler) handleS3InstanceDeletion(ctx context.Context, req ctrl.Request, s3InstanceResource *s3v1alpha1.S3Instance) (reconcile.Result, error) {
	logger := log.FromContext(ctx)

	if controllerutil.ContainsFinalizer(s3InstanceResource, s3InstanceFinalizer) {
		logger.Info("Performing Finalizer Operations for S3Instance before delete CR", "Namespace", s3InstanceResource.GetNamespace(), "Name", s3InstanceResource.GetName())

		ctrlResult, err := r.checkS3InstanceReferencesInBucket(ctx, req, s3InstanceResource)
		if err != nil {
			return ctrlResult, err
		}

		ctrlResult, err = r.checkS3InstanceReferencesInPolicy(ctx, req, s3InstanceResource)
		if err != nil {
			return ctrlResult, err
		}

		ctrlResult, err = r.checkS3InstanceReferencesInPath(ctx, req, s3InstanceResource)
		if err != nil {
			return ctrlResult, err
		}

		ctrlResult, err = r.checkS3InstanceReferencesInS3User(ctx, req, s3InstanceResource)
		if err != nil {
			return ctrlResult, err
		}

		//Remove s3InstanceFinalizer. Once all finalizers have been removed, the object will be deleted.
		if ok := controllerutil.RemoveFinalizer(s3InstanceResource, s3InstanceFinalizer); !ok {
			logger.Info("Failed to remove finalizer for S3Instance", "NamespacedName", req.NamespacedName.String())
			return ctrl.Result{Requeue: true}, nil
		}

		// Let's re-fetch the S3Instance Custom Resource after removing the finalizer
		// so that we have the latest state of the resource on the cluster and we will avoid
		// raise the issue "the object has been modified, please apply
		// your changes to the latest version and try again" which would re-trigger the reconciliation
		// if we try to update it again in the following operations
		if err := r.Update(ctx, s3InstanceResource); err != nil {
			logger.Error(err, "Failed to remove finalizer for S3Instance", "NamespacedName", req.NamespacedName.String())
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.*
func (r *S3InstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// filterLogger := ctrl.Log.WithName("filterEvt")
	return ctrl.NewControllerManagedBy(mgr).
		For(&s3v1alpha1.S3Instance{}).
		// See : https://sdk.operatorframework.io/docs/building-operators/golang/references/event-filtering/
		WithEventFilter(predicate.Funcs{
			// Ignore updates to CR status in which case metadata.Generation does not change,
			// unless it is a change to the underlying Secret
			UpdateFunc: func(e event.UpdateEvent) bool {
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

func (r *S3InstanceReconciler) SetReconciledCondition(ctx context.Context, req ctrl.Request, s3InstanceResource *s3v1alpha1.S3Instance, reason string, message string, err error) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var changed bool

	if err != nil {
		logger.Error(err, message, "NamespacedName", req.NamespacedName.String())
		changed = meta.SetStatusCondition(&s3InstanceResource.Status.Conditions, metav1.Condition{Type: s3v1alpha1.ConditionReconciled,
			Status: metav1.ConditionFalse, ObservedGeneration: s3InstanceResource.Generation, Reason: reason,
			Message: fmt.Sprintf("%s: %s", message, err)})

	} else {
		logger.Info(message, "NamespacedName", req.NamespacedName.String())
		changed = meta.SetStatusCondition(&s3InstanceResource.Status.Conditions, metav1.Condition{Type: s3v1alpha1.ConditionReconciled,
			Status: metav1.ConditionTrue, ObservedGeneration: s3InstanceResource.Generation, Reason: reason,
			Message: message})
	}

	if changed {
		if errStatusUpdate := r.Status().Update(ctx, s3InstanceResource); errStatusUpdate != nil {
			logger.Error(errStatusUpdate, "Failed to update s3Instance status", "NamespacedName", req.NamespacedName.String())
			return ctrl.Result{}, errStatusUpdate
		}
	}

	return ctrl.Result{RequeueAfter: r.ReconcilePeriod}, err
}

func (r *S3InstanceReconciler) checkS3InstanceReferencesInBucket(ctx context.Context, req ctrl.Request, s3InstanceResource *s3v1alpha1.S3Instance) (ctrl.Result, error) {
	bucketLists := s3v1alpha1.BucketList{}
	err := r.List(ctx, &bucketLists)
	found := 0
	if err != nil {
		return r.SetReconciledCondition(ctx, req, s3InstanceResource,
			s3v1alpha1.DeletionFailure, "Failed retrieve Buckets", err)
	}

	for _, bucket := range bucketLists.Items {
		if controllerhelpers.GetS3InstanceRefInfo(bucket.Spec.S3InstanceRef, bucket.Namespace).String() == s3InstanceResource.Namespace+"/"+s3InstanceResource.Name {
			found++
		}
	}

	if found > 0 {
		return r.SetReconciledCondition(ctx, req, s3InstanceResource,
			s3v1alpha1.DeletionFailure, "Unable to delete s3Instance", fmt.Errorf("found %d bucket which use this s3Instance", found))
	}
	return ctrl.Result{}, nil
}

func (r *S3InstanceReconciler) checkS3InstanceReferencesInPolicy(ctx context.Context, req ctrl.Request, s3InstanceResource *s3v1alpha1.S3Instance) (ctrl.Result, error) {
	policyLists := s3v1alpha1.PolicyList{}
	err := r.List(ctx, &policyLists)
	found := 0
	if err != nil {
		return r.SetReconciledCondition(ctx, req, s3InstanceResource,
			s3v1alpha1.DeletionFailure, "Failed retrieve Policies", err)
	}

	for _, policy := range policyLists.Items {
		if controllerhelpers.GetS3InstanceRefInfo(policy.Spec.S3InstanceRef, policy.Namespace).String() == s3InstanceResource.Namespace+"/"+s3InstanceResource.Name {
			found++
		}
	}

	if found > 0 {
		return r.SetReconciledCondition(ctx, req, s3InstanceResource,
			s3v1alpha1.DeletionFailure, "Unable to delete s3Instance", fmt.Errorf("found %d policy which use this s3Instance", found))
	}
	return ctrl.Result{}, nil
}

func (r *S3InstanceReconciler) checkS3InstanceReferencesInS3User(ctx context.Context, req ctrl.Request, s3InstanceResource *s3v1alpha1.S3Instance) (ctrl.Result, error) {
	s3UserList := s3v1alpha1.S3UserList{}
	err := r.List(ctx, &s3UserList)
	found := 0
	if err != nil {
		return r.SetReconciledCondition(ctx, req, s3InstanceResource,
			s3v1alpha1.DeletionFailure, "Failed retrieve s3Users", err)
	}

	for _, s3User := range s3UserList.Items {
		if controllerhelpers.GetS3InstanceRefInfo(s3User.Spec.S3InstanceRef, s3User.Namespace).String() == s3InstanceResource.Namespace+"/"+s3InstanceResource.Name {
			found++
		}

	}

	if found > 0 {
		return r.SetReconciledCondition(ctx, req, s3InstanceResource,
			s3v1alpha1.DeletionFailure, "Unable to delete s3Instance", fmt.Errorf("found %d s3User which use this s3Instance", found))
	}
	return ctrl.Result{}, nil
}

func (r *S3InstanceReconciler) checkS3InstanceReferencesInPath(ctx context.Context, req ctrl.Request, s3InstanceResource *s3v1alpha1.S3Instance) (ctrl.Result, error) {
	pathList := s3v1alpha1.PathList{}
	err := r.List(ctx, &pathList)
	found := 0
	if err != nil {
		return r.SetReconciledCondition(ctx, req, s3InstanceResource,
			s3v1alpha1.DeletionFailure, "Failed retrieve paths", err)
	}

	for _, path := range pathList.Items {
		if controllerhelpers.GetS3InstanceRefInfo(path.Spec.S3InstanceRef, path.Namespace).String() == s3InstanceResource.Namespace+"/"+s3InstanceResource.Name {
			found++
		}
	}

	if found > 0 {
		return r.SetReconciledCondition(ctx, req, s3InstanceResource,
			s3v1alpha1.DeletionFailure, "Unable to delete s3Instance", fmt.Errorf("found %d path which use this s3Instance", found))
	}
	return ctrl.Result{}, nil
}
