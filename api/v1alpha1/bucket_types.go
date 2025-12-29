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

package v1alpha1

import (
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// BucketSpec defines the desired state of Bucket
type BucketSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Name of the bucket
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Paths (folders) to create inside the bucket
	// +kubebuilder:validation:Optional
	Paths []string `json:"paths,omitempty"`

	// s3InstanceRef where create the bucket
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]{0,61}[a-z0-9])?(/[a-z0-9]([-a-z0-9]{0,61}[a-z0-9])?)?$`
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=127
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="s3InstanceRef is immutable"
	// +kubebuilder:default=s3-operator/default
	S3InstanceRef string `json:"s3InstanceRef"`

	// Quota to apply to the bucket
	// +kubebuilder:validation:Required
	Quota Quota `json:"quota"`
}

// BucketStatus defines the observed state of Bucket
type BucketStatus struct {
	// Status management using Conditions.
	// See also : https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Bucket is the Schema for the buckets API
type Bucket struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BucketSpec   `json:"spec,omitempty"`
	Status BucketStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BucketList contains a list of Bucket
type BucketList struct {
	metav1.TypeMeta `         json:",inline"`
	metav1.ListMeta `         json:"metadata,omitempty"`
	Items           []Bucket `json:"items"`
}

type Quota struct {
	// Default quota to apply, mandatory
	// +kubebuilder:validation:Required
	Default resource.Quantity `json:"default"`

	// Optional override quota, to be used by cluster admin.
	// +kubebuilder:validation:Optional
	Override resource.Quantity `json:"override,omitempty"`
}

func init() {
	SchemeBuilder.Register(&Bucket{}, &BucketList{})
}
