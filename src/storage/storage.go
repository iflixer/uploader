package storage

import (
	"context"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type Service struct {
	s3Client        *s3.Client
	bucketName      string
	accountID       string
	accessKeyId     string
	accessKeySecret string
}

func (s *Service) Upload(localFile, remoteFile string) (err error) {
	return s.UploadR2WithProgress(localFile, remoteFile)
	// return s.upload(localFile, remoteFile)
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

func NewService(bucketName, accountID, accessKeyId, accessKeySecret string) (*Service, error) {
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

	// tr := &http.Transport{
	// 	MaxIdleConns:        256,
	// 	MaxIdleConnsPerHost: 128,
	// 	IdleConnTimeout:     90 * time.Second,
	// 	TLSHandshakeTimeout: 10 * time.Second,
	// 	DisableCompression:  true,
	// 	ForceAttemptHTTP2:   false,
	// }
	// httpClient := &http.Client{Transport: tr, Timeout: 0}

	// // 1. базовый конфиг
	// cfg, err := config.LoadDefaultConfig(
	// 	context.TODO(),
	// 	config.WithHTTPClient(httpClient),
	// 	config.WithRegion("auto"), // для R2 всегда auto
	// 	config.WithCredentialsProvider(
	// 		credentials.NewStaticCredentialsProvider(accessKeyId, accessKeySecret, ""),
	// 	),
	// 	config.WithRetryer(func() aws.Retryer {
	// 		return retry.NewStandard(func(o *retry.StandardOptions) { o.MaxAttempts = 8 })
	// 	}),
	// )
	// if err != nil {
	// 	return nil, fmt.Errorf("load config: %w", err)
	// }

	// // 2. endpoint Cloudflare R2
	// //endpoint := "https://" + accountID + ".r2.cloudflarestorage.com"

	// // 3. создаём клиента S3
	// client := s3.NewFromConfig(cfg, func(o *s3.Options) {
	// 	// современный вариант (если твоя версия поддерживает):
	// 	o.BaseEndpoint = aws.String(endPoint)

	// 	// если выдаст "unknown field BaseEndpoint",
	// 	// замени на:
	// 	// o.EndpointResolver = s3.EndpointResolverFromURL(endpoint)

	// 	o.UsePathStyle = false // R2 работает в virtual-hosted-style
	// })

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Hour)
	defer cancel()

	r2Client, err := newR2Client(ctx, accountID, accessKeyId, accessKeySecret)
	if err != nil {
		log.Printf("error creating R2 client: %s\n", err)
		return nil, err
	}

	s := &Service{
		s3Client:        r2Client,
		bucketName:      bucketName,
		accountID:       accountID,
		accessKeyId:     accessKeyId,
		accessKeySecret: accessKeySecret,
	}

	return s, s.test()
}

func newR2Client(ctx context.Context, accountID, accessKey, secretKey string) (*s3.Client, error) {
	// Надёжный транспорт для длинных PUT’ов: без общего Timeout, keep-alive, можно отключить h2
	tr := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:        256,
		MaxIdleConnsPerHost: 128,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
		DisableCompression:  true,
		ForceAttemptHTTP2:   false, // при желании можно true; если видите 5xx волнами — попробуйте false
	}
	httpClient := &http.Client{Transport: tr, Timeout: 0}

	cfg, err := config.LoadDefaultConfig(
		ctx,
		config.WithHTTPClient(httpClient),
		config.WithRegion("auto"), // для R2 всегда "auto"
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
		config.WithRetryer(func() aws.Retryer {
			return retry.NewStandard(func(o *retry.StandardOptions) {
				o.MaxAttempts = 8 // больше попыток на 5xx
			})
		}),
	)
	if err != nil {
		return nil, err
	}

	endpoint := "https://" + accountID + ".r2.cloudflarestorage.com"
	log.Println("R2 endpoint:", endpoint)

	// Без deprecated-хуков: фиксируем endpoint на уровне клиента
	return s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint) // если поле недоступно в вашей версии — замените на EndpointResolverFromURL
		// o.EndpointResolver = s3.EndpointResolverFromURL(endpoint)
		o.UsePathStyle = false // R2 = виртуальный хостинг
	}), nil
}
