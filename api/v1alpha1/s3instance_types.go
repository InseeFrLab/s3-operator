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

// S3InstanceSpec defines the desired state of S3Instance
type S3InstanceSpec struct {

	// type of the S3Instance
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="S3Provider is immutable"
	// +kubebuilder:default=minio
	// +kubebuilder:validation:Enum=minio;mockedS3Provider
	S3Provider string `json:"s3Provider,omitempty"`

	// url of the S3Instance
	// +kubebuilder:validation:Required
	Url string `json:"url"`

	// Ref to Secret associated to the S3Instance containing accessKey and secretKey
	// +kubebuilder:validation:Required
	SecretRef string `json:"secretRef"`

	// region associated to the S3Instance
	// +kubebuilder:validation:Optional
	Region string `json:"region,omitempty"`

	// Secret containing key ca.crt with the certificate associated to the S3InstanceUrl
	// +kubebuilder:validation:Optional
	CaCertSecretRef string `json:"caCertSecretRef,omitempty"`

	// AllowedNamespaces to use this S3InstanceUrl if empty only the namespace of this instance url is allowed to use it
	// +kubebuilder:validation:Optional
	AllowedNamespaces []string `json:"allowedNamespaces,omitempty"`

	// BucketDeletionEnabled Trigger bucket deletion on the S3 backend upon CR deletion. Will fail if bucket is not empty.
	// +kubebuilder:default=false
	BucketDeletionEnabled bool `json:"bucketDeletionEnabled,omitempty"`

	// PolicyDeletionEnabled Trigger policy deletion on the S3 backend upon CR deletion.
	// +kubebuilder:default=false
	PolicyDeletionEnabled bool `json:"policyDeletionEnabled,omitempty"`

	// PathDeletionEnabled Trigger path deletion on the S3 backend upon CR deletion. Limited to deleting the `.keep` files used by the operator.
	// +kubebuilder:default=false
	PathDeletionEnabled bool `json:"pathDeletionEnabled,omitempty"`

	// S3UserDeletionEnabled Trigger S3 deletion on the S3 backend upon CR deletion.
	// +kubebuilder:default=false
	S3UserDeletionEnabled bool `json:"s3UserDeletionEnabled,omitempty"`
}

// S3InstanceStatus defines the observed state of S3Instance
type S3InstanceStatus struct {
	// Status management using Conditions.
	// See also : https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// S3Instance is the Schema for the S3Instances API
type S3Instance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   S3InstanceSpec   `json:"spec,omitempty"`
	Status S3InstanceStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// S3InstanceList contains a list of S3Instance
type S3InstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []S3Instance `json:"items"`
}

func init() {
	SchemeBuilder.Register(&S3Instance{}, &S3InstanceList{})
}
