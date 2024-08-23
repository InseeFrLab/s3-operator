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

// S3UserSpec defines the desired state of S3User
type S3UserSpec struct {

	// Name of the S3User
	// +kubebuilder:validation:Required
	AccessKey string `json:"accessKey"`

	// Policies associated to the S3User
	// +kubebuilder:validation:Optional
	Policies []string `json:"policies,omitempty"`

	// SecretName associated to the S3User
	// +kubebuilder:validation:Optional
	SecretName string `json:"secretName"`

	// s3InstanceRef where create the user
	// +kubebuilder:default=s3-operator/default
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="s3InstanceRef is immutable"
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]{0,61}[a-z0-9])?(/[a-z0-9]([-a-z0-9]{0,61}[a-z0-9])?)?$`
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=127
	S3InstanceRef string `json:"s3InstanceRef,omitempty"`
}

// S3UserStatus defines the observed state of S3User
type S3UserStatus struct {
	// Status management using Conditions.
	// See also : https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// S3User is the Schema for the S3Users API
type S3User struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   S3UserSpec   `json:"spec,omitempty"`
	Status S3UserStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// S3UserList contains a list of S3User
type S3UserList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []S3User `json:"items"`
}

func init() {
	SchemeBuilder.Register(&S3User{}, &S3UserList{})
}
