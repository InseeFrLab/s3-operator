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

package s3factory

import (
	"fmt"

	s3client "github.com/InseeFrLab/s3-operator/pkg/s3/client"
	s3clientImpl "github.com/InseeFrLab/s3-operator/pkg/s3/client/impl"
)

type S3Factory struct {
}

func NewS3Factory() *S3Factory {
	return &S3Factory{}
}

func (mockedS3Provider *S3Factory) GenerateS3Client(s3Provider string, s3Config *s3client.S3Config) (s3client.S3Client, error) {
	if s3Provider == "mockedS3Provider" {
		return s3clientImpl.NewMockedS3Client(), nil
	}
	if s3Provider == "minio" {
		return s3clientImpl.NewMinioS3Client(s3Config)
	}
	return nil, fmt.Errorf("s3 provider %s not supported", s3Provider)
}
