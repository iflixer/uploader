package storage

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"io"
	"os"
)

func (s *Service) test() (err error) {
	fmt.Println("testing storage connection:")
	fmt.Println("accessKeyId:", s.accessKeyId)
	fmt.Println("bucketName:", s.bucketName)
	fmt.Println("endPoint:", s.endPoint)
	fmt.Println("accessKeySecret(first 3 chars):", s.accessKeySecret[:3])
	fmt.Println("head test.txt...")
	_, err = s.Head("inbox/test.txt")
	if err == nil {
		fmt.Println("OK")
	} else {
		fmt.Println("error", err)
	}
	fmt.Println("upload test.txt as inbox/test.txt...")
	err = s.Upload("test.txt", "inbox/test.txt")
	if err == nil {
		fmt.Println("OK")
	} else {
		fmt.Println("error", err)
	}
	fmt.Println("download inbox/test.mp4...")
	written, err := s.Download("inbox/test.txt", "test.txt")
	if err == nil {
		fmt.Println("OK, bytes:", written)
	} else {
		fmt.Println("error", err)
	}
	fmt.Println("list inbox/test.txt...")
	l, err := s.List("inbox/test.txt")
	if err == nil {
		fmt.Printf(" %+v \n", l)
		fmt.Println("OK")
	} else {
		fmt.Println("error", err)
	}
	fmt.Println("testing storage connection done")
	return
}

func (s *Service) head(objectName string) (length int64, err error) {
	input := &s3.HeadObjectInput{
		Bucket: &s.bucketName,
		Key:    &objectName,
	}
	resp, err := s.s3Client.HeadObject(context.TODO(), input)
	if err != nil {
		return 0, err
	}
	return resp.ContentLength, nil
}

func (s *Service) list(path string) (res []string, err error) {
	input := &s3.ListObjectsInput{
		Bucket: &s.bucketName,
		Prefix: &path,
	}

	resp, err := s.s3Client.ListObjects(context.TODO(), input)
	if err != nil {
		return nil, err
	}

	result := []string{}

	for _, res := range resp.Contents {
		result = append(result, *res.Key)
	}

	return result, nil
}

func (s *Service) upload(localFile, remoteFile string) error {
	f, err := os.OpenFile(localFile, os.O_RDONLY, 0644)
	if err != nil {
		return err
	}
	object := s3.PutObjectInput{
		Bucket: aws.String(s.bucketName), // The path to the directory you want to upload the object to, starting with your Space name.
		Key:    aws.String(remoteFile),   // Object key, referenced whenever you want to access this file later.
		Body:   f,                        // The object's contents.
		/*		ACL:    aws.String("public-read"),                  // Defines Access-control List (ACL) permissions, such as private or public.
				Metadata: map[string]*string{ // Required. Defines metadata tags.
					"x-amz-meta-my-key": aws.String("your-value"),
				},*/
	}

	_, err = s.s3Client.PutObject(context.TODO(), &object)
	if err != nil {
		return err
	}
	return nil
}

func (s *Service) download(remoteFile, localFile string) (written int64, err error) {
	// file = strings.Replace(file, "/"+bucketName+"/", "", 1)
	// log.Println("s3.download (" + s.bucketName + "):" + remoteFile)
	object := s3.GetObjectInput{
		Bucket: aws.String(s.bucketName),
		Key:    aws.String(remoteFile),
	}

	result, err := s.s3Client.GetObject(context.TODO(), &object)
	if err != nil {
		return
	}

	out, err := os.Create(localFile)
	defer func() { _ = out.Close() }()
	if err != nil {
		return
	}

	written, err = io.Copy(out, result.Body)
	return
}
