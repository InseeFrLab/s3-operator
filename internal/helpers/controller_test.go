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

package helpers_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	s3v1alpha1 "github.com/InseeFrLab/s3-operator/api/v1alpha1"
	"github.com/InseeFrLab/s3-operator/internal/helpers"
	testUtils "github.com/InseeFrLab/s3-operator/test/utils"
	"github.com/stretchr/testify/assert"
)

func TestSetReconciledCondition(t *testing.T) {

	log.SetLogger(zap.New(zap.UseDevMode(true)))

	s3instanceResource := &s3v1alpha1.S3Instance{
		Spec: s3v1alpha1.S3InstanceSpec{
			AllowedNamespaces:     []string{"default", "test-*", "*-namespace", "*allowed*"},
			Url:                   "https://minio.example.com",
			S3Provider:            "minio",
			Region:                "us-east-1",
			BucketDeletionEnabled: true,
			S3UserDeletionEnabled: true,
			PathDeletionEnabled:   true,
			PolicyDeletionEnabled: true,
			SecretRef:             "minio-credentials",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              "default",
			Namespace:         "s3-operator",
			Finalizers:        []string{"s3.onyxia.sh/finalizer"},
			DeletionTimestamp: &metav1.Time{Time: time.Now()},
		},
	}
	testUtils := testUtils.NewTestUtils()
	testUtils.SetupClient([]client.Object{s3instanceResource})
	controllerHelper := helpers.NewControllerHelper()

	t.Run("no error", func(t *testing.T) {
		_, err := controllerHelper.SetReconciledCondition(
			context.TODO(),
			testUtils.Client.Status(),
			ctrl.Request{NamespacedName: types.NamespacedName{Name: s3instanceResource.Name, Namespace: s3instanceResource.Namespace}},
			s3instanceResource,
			&s3instanceResource.Status.Conditions,
			s3v1alpha1.Reconciled,
			"s3Instance reconciled",
			"s3Instance reconciled",
			nil, time.Duration(10),
		)
		assert.NoError(t, err)
	})

	t.Run("ressource status have changed", func(t *testing.T) {
		s3instanceResourceUpdated := &s3v1alpha1.S3Instance{}
		err := testUtils.Client.Get(context.TODO(), client.ObjectKey{
			Namespace: "s3-operator",
			Name:      "default",
		}, s3instanceResourceUpdated)
		assert.NoError(t, err)
		assert.Equal(t, s3v1alpha1.Reconciled, s3instanceResourceUpdated.Status.Conditions[0].Type)
		assert.Equal(t, "s3Instance reconciled", s3instanceResourceUpdated.Status.Conditions[0].Message)
	})

	t.Run("with error", func(t *testing.T) {
		_, err := controllerHelper.SetReconciledCondition(
			context.TODO(),
			testUtils.Client.Status(),
			ctrl.Request{NamespacedName: types.NamespacedName{Name: s3instanceResource.Name, Namespace: s3instanceResource.Namespace}},
			s3instanceResource,
			&s3instanceResource.Status.Conditions,
			s3v1alpha1.CreationFailure,
			"s3Instance reconciled",
			"s3Instance reconciled",
			fmt.Errorf("Something wrong have happened"), time.Duration(10),
		)

		assert.NotNil(t, err)

	})

	t.Run("ressource status have changed", func(t *testing.T) {
		s3instanceResourceUpdated := &s3v1alpha1.S3Instance{}
		err := testUtils.Client.Get(context.TODO(), client.ObjectKey{
			Namespace: "s3-operator",
			Name:      "default",
		}, s3instanceResourceUpdated)
		assert.NoError(t, err)
		assert.Equal(t, s3v1alpha1.CreationFailure, s3instanceResourceUpdated.Status.Conditions[1].Type)
		assert.Contains(t, s3instanceResourceUpdated.Status.Conditions[1].Message, "Something wrong have happened")
	})
}
