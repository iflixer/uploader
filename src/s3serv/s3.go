package s3serv

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"os"
	"time"
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

func (s *S3serv) Add(ff *ffmpeg.Ffmpeg) (err error) {
	if err = s.upload("/files/"+ff.FileNameResult, "tmp", ff.FileNameResult); err != nil {
		return
	}
	if err = s.upload("/files/"+ff.FileNameLog, "tmp", ff.FileNameLog); err != nil {
		return
	}
	if err = s.upload("/files/"+ff.FileName, "tmp", ff.FileName); err != nil {
		return
	}
	return nil
}

func (s *S3serv) upload(localFilePath string, s3folder, s3filename string) error {
	targetPath := s3folder + "/" + s3filename
	fmt.Println("Upload " + localFilePath + " to " + targetPath + "...")
	start := time.Now()
	f, err := os.OpenFile(localFilePath, os.O_RDONLY, 0644)
	if err != nil {
		return err
	}
	object := s3.PutObjectInput{
		Bucket: aws.String(s.bucketName), // The path to the directory you want to upload the object to, starting with your Space name.
		Key:    aws.String(targetPath),   // Object key, referenced whenever you want to access this file later.
		Body:   f,                        // The object's contents.
		/*		ACL:    aws.String("public-read"),                  // Defines Access-control List (ACL) permissions, such as private or public.
				Metadata: map[string]*string{ // Required. Defines metadata tags.
					"x-amz-meta-my-key": aws.String("your-value"),
				},*/
	}

	_, err = s.client.PutObject(context.TODO(), &object)
	if err != nil {
		return err
	}

	fmt.Printf("upload to s3 done in %s\n", time.Since(start))
	return nil
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
