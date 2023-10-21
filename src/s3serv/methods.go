package s3serv

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"os"
	"time"
)

func (s *S3serv) HeadObject(objectName string) (length int64, err error) {
	fmt.Printf("S3serv.HeadObject: %+v\n", objectName)
	input := &s3.HeadObjectInput{
		Bucket: &s.bucketName,
		Key:    &objectName,
	}
	resp, err := s.client.HeadObject(context.TODO(), input)
	if err != nil {
		fmt.Printf("HeadObject %s err: %s\n", objectName, err)
		return 0, err
	}
	return resp.ContentLength, nil
}

func (s *S3serv) ListStorage(path string) (res []string, err error) {
	fmt.Printf("S3serv.ListStorage: %+v\n", path)
	input := &s3.ListObjectsInput{
		Bucket: &s.bucketName,
		Prefix: &path,
	}
	resp, err := s.client.ListObjects(context.TODO(), input)
	if err != nil {
		fmt.Printf("err: %s\n", err)
		return nil, err
	}

	result := []string{}

	for _, res := range resp.Contents {
		result = append(result, *res.Key)
	}

	return result, nil
}

func (s *S3serv) UploadObject(localFilePath string, s3folder, s3filename string) error {
	targetPath := s3folder + "/" + s3filename
	fmt.Println("S3serv.UploadObject: " + localFilePath + " to " + targetPath + "...")
	start := time.Now()
	f, err := os.OpenFile(localFilePath, os.O_RDONLY, 0644)
	if err != nil {
		return err
	}
	object := s3.PutObjectInput{
		Bucket: aws.String(s.bucketName), // The path to the directory you want to UploadObject the object to, starting with your Space name.
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

	fmt.Printf("UploadObject to s3 done in %s\n", time.Since(start))
	return nil
}
