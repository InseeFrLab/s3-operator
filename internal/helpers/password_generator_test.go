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
	"testing"

	helpers "github.com/InseeFrLab/s3-operator/internal/helpers"
	"github.com/stretchr/testify/assert"
)

func TestGenerate(t *testing.T) {
	t.Run("Exact match", func(t *testing.T) {
		passwordGenerator := helpers.NewPasswordGenerator()
		password, _ := passwordGenerator.Generate(20, true, true, true)
		assert.Len(t, password, 20)
	})
}
