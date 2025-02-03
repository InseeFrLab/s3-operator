package s3factory

import (
	"fmt"

	s3client "github.com/InseeFrLab/s3-operator/internal/s3/client"
	s3clientImpl "github.com/InseeFrLab/s3-operator/internal/s3/client/impl"
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
