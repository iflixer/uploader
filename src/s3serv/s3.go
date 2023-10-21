package s3serv

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"uploader/ffmpeg"
)

// this service should watch the new tasks in DB and process them

type S3serv struct {
	client          *s3.Client
	bucketName      string
	endPoint        string
	accessKeyId     string
	accessKeySecret string
}

func (s *S3serv) Upload(ff *ffmpeg.Ffmpeg) (err error) {
	if err = s.UploadObject("/files/"+ff.FileNameResult, "tmp", ff.FileNameResult); err != nil {
		return
	}
	if err = s.UploadObject("/files/"+ff.FileNameLog, "tmp", ff.FileNameLog); err != nil {
		return
	}
	if err = s.UploadObject("/files/"+ff.FileName, "tmp", ff.FileName); err != nil {
		return
	}
	return nil
}

func (s *S3serv) Head(path string) (length int64, err error) {
	return s.HeadObject(path)
}

func (s *S3serv) List(path string) (list []string, err error) {
	return s.ListStorage(path)
}

func NewS3serv(bucketName, endPoint, accessKeyId, accessKeySecret string) (*S3serv, error) {
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

	s3Client := s3.NewFromConfig(cfg)
	return &S3serv{
		client:          s3Client,
		bucketName:      bucketName,
		endPoint:        endPoint,
		accessKeyId:     accessKeyId,
		accessKeySecret: accessKeySecret,
	}, nil
}
