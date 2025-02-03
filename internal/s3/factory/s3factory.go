package s3factory

import (
	s3client "github.com/InseeFrLab/s3-operator/internal/s3/client"
)

type S3Factory interface {
	GenerateS3Client(s3Provider string, s3Config *s3client.S3Config) (s3client.S3Client, error)
}
