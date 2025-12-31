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

package s3instance_controller

import (
	"context"
	"strings"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	s3v1alpha1 "github.com/InseeFrLab/s3-operator/api/v1alpha1"
)

func (r *S3InstanceReconciler) handleS3InstanceDeletion(
	ctx context.Context,
	req ctrl.Request,
	s3InstanceResource *s3v1alpha1.S3Instance,
) (reconcile.Result, error) {

	if controllerutil.ContainsFinalizer(s3InstanceResource, s3InstanceFinalizer) {
		r.SetProgressingCondition(
			ctx,
			req,
			s3InstanceResource,
			metav1.ConditionTrue,
			s3v1alpha1.Reconciling,
			fmt.Sprintf("Performing Finalizer Operations for S3Instance %s before delete CR", s3InstanceResource.GetName()),
		)

		// Vérifier les références existantes
		if err := r.checkS3InstanceReferences(ctx, s3InstanceResource); err != nil {
			r.SetDegradedCondition(
				ctx,
				req,
				s3InstanceResource,
				metav1.ConditionFalse,
				s3v1alpha1.DeletionBlocked,
				fmt.Sprintf("S3Instance %s is still referenced by other resources", s3InstanceResource.Name),
				err,
			)
			return ctrl.Result{}, err
		}

		//Remove s3InstanceFinalizer. Once all finalizers have been removed, the object will be deleted.
		if ok := controllerutil.RemoveFinalizer(s3InstanceResource, s3InstanceFinalizer); !ok {
			r.SetProgressingCondition(
				ctx,
				req,
				s3InstanceResource,
				metav1.ConditionFalse,
				s3v1alpha1.InternalError,
				fmt.Sprintf("Failed to remove finalizer for S3Instance %s", s3InstanceResource.Name),
			)
			return ctrl.Result{Requeue: true}, nil
		}

		// Let's re-fetch the S3Instance Custom Resource after removing the finalizer
		// so that we have the latest state of the resource on the cluster and we will avoid
		// raise the issue "the object has been modified, please apply
		// your changes to the latest version and try again" which would re-trigger the reconciliation
		// if we try to update it again in the following operations
		if err := r.Update(ctx, s3InstanceResource); err != nil {
			r.SetDegradedCondition(
				ctx,
				req,
				s3InstanceResource,
				metav1.ConditionFalse,
				s3v1alpha1.K8sApiError,
				fmt.Sprintf("An error occured when removing finalizer from S3Instance %s", s3InstanceResource.Name),
				err,
			)
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

// checkS3InstanceReferences make sure the instance is not used anymore
func (r *S3InstanceReconciler) checkS3InstanceReferences(ctx context.Context, s3Instance *s3v1alpha1.S3Instance) error {
	// CR to be checked for existence
	references := map[string]client.ObjectList{
		"Buckets":  &s3v1alpha1.BucketList{},
		"Policies": &s3v1alpha1.PolicyList{},
		"Paths":    &s3v1alpha1.PathList{},
		"S3Users":  &s3v1alpha1.S3UserList{},
	}
	var errors []string
	for name, list := range references {
		if err := r.List(ctx, list); err != nil {
			return fmt.Errorf("Failed to retrieve %s : %w", name, err)
		}

		if found := r.countReferences(list, s3Instance); found > 0 {
			errors = append(errors, fmt.Sprintf("Cannot delete s3Instance as %d %s are used on this instance", found, name))
		}
	}
	if len(errors) >0 {
		return fmt.Errorf(strings.Join(errors, "\n"))
	}
	return nil
}

// countReferences compte les objets faisant référence à un S3Instance
func (r *S3InstanceReconciler) countReferences(list client.ObjectList, s3Instance *s3v1alpha1.S3Instance) int {
	count := 0
	switch objects := list.(type) {
	case *s3v1alpha1.BucketList:
		for _, obj := range objects.Items {
			if r.S3Instancehelper.GetS3InstanceRefInfo(obj.Spec.S3InstanceRef, obj.Namespace).
				String() == fmt.Sprintf("%s/%s", s3Instance.Namespace, s3Instance.Name) {
				count++
			}
		}
	case *s3v1alpha1.PathList:
		for _, obj := range objects.Items {
			if r.S3Instancehelper.GetS3InstanceRefInfo(obj.Spec.S3InstanceRef, obj.Namespace).
				String() == fmt.Sprintf("%s/%s", s3Instance.Namespace, s3Instance.Name) {
				count++
			}
		}
	case *s3v1alpha1.S3UserList:
		for _, obj := range objects.Items {
			if r.S3Instancehelper.GetS3InstanceRefInfo(obj.Spec.S3InstanceRef, obj.Namespace).
				String() == fmt.Sprintf("%s/%s", s3Instance.Namespace, s3Instance.Name) {
				count++
			}
		}
	case *s3v1alpha1.PolicyList:
		for _, obj := range objects.Items {
			if r.S3Instancehelper.GetS3InstanceRefInfo(obj.Spec.S3InstanceRef, obj.Namespace).
				String() == fmt.Sprintf("%s/%s", s3Instance.Namespace, s3Instance.Name) {
				count++
			}
		}
	}

	return count
}
