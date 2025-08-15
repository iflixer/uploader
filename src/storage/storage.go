package storage

import (
	"context"
	"fmt"

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
	// r2Resolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
	// 	return aws.Endpoint{
	// 		// URL:               fmt.Sprintf("https://%s.r2.cloudflarestorage.com", accountId),
	// 		URL:               endPoint,
	// 		HostnameImmutable: true,
	// 	}, nil
	// })

	// cfg, err := config.LoadDefaultConfig(context.TODO(),
	// 	config.WithEndpointResolverWithOptions(r2Resolver),
	// 	config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKeyId, accessKeySecret, "")),
	// )
	// if err != nil {
	// 	return nil, err
	// }
	// cfg.Region = "auto"

	// 1. базовый конфиг
	cfg, err := config.LoadDefaultConfig(
		context.TODO(),
		config.WithRegion("auto"), // для R2 всегда auto
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(accessKeyId, accessKeySecret, ""),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	// 2. endpoint Cloudflare R2
	//endpoint := "https://" + accountID + ".r2.cloudflarestorage.com"

	// 3. создаём клиента S3
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		// современный вариант (если твоя версия поддерживает):
		o.BaseEndpoint = aws.String(endPoint)

		// если выдаст "unknown field BaseEndpoint",
		// замени на:
		// o.EndpointResolver = s3.EndpointResolverFromURL(endpoint)

		o.UsePathStyle = false // R2 работает в virtual-hosted-style
	})
	s := &Service{
		s3Client:        client,
		bucketName:      bucketName,
		endPoint:        endPoint,
		accessKeyId:     accessKeyId,
		accessKeySecret: accessKeySecret,
	}

	return s, s.test()
}
