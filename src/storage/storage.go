package storage

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type Service struct {
	s3Client        *s3.Client
	bucketName      string
	endPoint        string
	accessKeyId     string
	accessKeySecret string
}

func (s *Service) Upload(localFile, remoteFile string) (err error) {
	return s.upload(localFile, remoteFile)
}

func (s *Service) Download(remoteFile, localFile string) (written int64, err error) {
	return s.download(remoteFile, localFile)
}

func (s *Service) Head(path string) (length int64, err error) {
	return s.head(path)
}

func (s *Service) List(path string) (list []string, err error) {
	return s.list(path)
}

func NewService(bucketName, endPoint, accessKeyId, accessKeySecret string) (*Service, error) {
	r2Resolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		return aws.Endpoint{
			// URL:               fmt.Sprintf("https://%s.r2.cloudflarestorage.com", accountId),
			URL:               endPoint,
			HostnameImmutable: true,
		}, nil
	})

	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithEndpointResolverWithOptions(r2Resolver),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKeyId, accessKeySecret, "")),
	)
	if err != nil {
		return nil, err
	}
	cfg.Region = "auto"

	s3Client := s3.NewFromConfig(cfg)
	s := &Service{
		s3Client:        s3Client,
		bucketName:      bucketName,
		endPoint:        endPoint,
		accessKeyId:     accessKeyId,
		accessKeySecret: accessKeySecret,
	}

	return s, s.test()
}
