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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// PathSpec defines the desired state of Path
type PathSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Name of the bucket
	// +kubebuilder:validation:Required
	BucketName string `json:"bucketName"`

	// Paths (folders) to create inside the bucket
	// +kubebuilder:validation:Optional
	Paths []string `json:"paths,omitempty"`

	// s3InstanceRef where create the Paths
	// +kubebuilder:default=s3-operator/default
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="s3InstanceRef is immutable"
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]{0,61}[a-z0-9])?(/[a-z0-9]([-a-z0-9]{0,61}[a-z0-9])?)?$`
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=127
	S3InstanceRef string `json:"s3InstanceRef,omitempty"`
}

// PathStatus defines the observed state of Path
type PathStatus struct {
	// Status management using Conditions.
	// See also : https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// Path is the Schema for the paths API
type Path struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PathSpec   `json:"spec,omitempty"`
	Status PathStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PathList contains a list of Path
type PathList struct {
	metav1.TypeMeta `       json:",inline"`
	metav1.ListMeta `       json:"metadata,omitempty"`
	Items           []Path `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Path{}, &PathList{})
}
