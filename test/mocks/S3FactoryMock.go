package mocks

import (
	"github.com/stretchr/testify/mock"

	s3client "github.com/InseeFrLab/s3-operator/internal/s3/client"
)

// Mocked Factory
type MockedS3ClientFactory struct {
	mock.Mock
}

func NewMockedS3ClientFactory() *MockedS3ClientFactory {
	return &MockedS3ClientFactory{}
}

func (m *MockedS3ClientFactory) GenerateS3Client(s3Provider string, s3Config *s3client.S3Config) (s3client.S3Client, error) {
	args := m.Called(s3Provider, s3Config)
	return args.Get(0).(s3client.S3Client), args.Error(1)
}
