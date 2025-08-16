package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

func (s *Service) test() (err error) {
	fmt.Println("testing storage connection:")
	fmt.Println("accessKeyId:", s.accessKeyId)
	fmt.Println("bucketName:", s.bucketName)
	fmt.Println("accountID:", s.accountID)
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
	// err = s.Upload("/downloads/10690_o.jardim.de.isabel.2024.1080p.web-dl.dual.2.0.mkv", "inbox/tmp.test")
	if err == nil {
		fmt.Println("OK")
	} else {
		fmt.Println("error", err)
	}

	// os.Exit(0) // exit after upload test

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
	return *resp.ContentLength, nil
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

func (s *Service) UploadR2WithProgress(localFile, remoteKey string) (err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Hour)
	defer cancel()

	log.Println("Uploading to R2:", localFile, remoteKey)

	err = uploadR2WithProgress(ctx, s.s3Client, s.bucketName, remoteKey, localFile, func(done, total int64, mbps float64) {
		pct := float64(done) / float64(total) * 100
		log.Printf("Uploading %s: %6.2f%%  %8.2f MiB/s\n", remoteKey, pct, mbps)
	})
	return
}

func (s *Service) upload(localFile, remoteKey string) error {
	remoteKey = strings.TrimLeft(remoteKey, "/")

	f, err := os.Open(localFile)
	if err != nil {
		return err
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return err
	}
	size := st.Size()

	// разумный общий таймаут (под себя)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Hour)
	defer cancel()

	ct := mime.TypeByExtension(strings.ToLower(filepath.Ext(localFile)))
	if ct == "" {
		ct = "application/octet-stream"
	}

	// Порог: мелкие файлы — обычный PutObject (надёжнее, без MPU)
	const singlePutThreshold = int64(200 * 1024 * 1024) // 200 MiB, подстрой под себя

	// функция одного прогона (для ретрая выше)
	putOnce := func() error {
		if size > 0 && size <= singlePutThreshold {
			// single PUT (без multipart)
			// Сбросим курсор, если был ретрай
			if _, err := f.Seek(0, io.SeekStart); err != nil {
				return err
			}
			_, err := s.s3Client.PutObject(ctx, &s3.PutObjectInput{
				Bucket:       aws.String(s.bucketName),
				Key:          aws.String(remoteKey),
				Body:         f,
				ContentType:  aws.String(ct),
				CacheControl: aws.String("public, max-age=31536000, immutable"),
			})
			return err
		}

		// Multipart для больших
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return err
		}

		upl := manager.NewUploader(s.s3Client, func(u *manager.Uploader) {
			u.Concurrency = 2             // 1–3 для R2 стабильно
			u.PartSize = 32 * 1024 * 1024 // 32 MiB (16–32 MiB обычно ок)
			u.LeavePartsOnError = false
		})

		_, err := upl.Upload(ctx, &s3.PutObjectInput{
			Bucket:       aws.String(s.bucketName),
			Key:          aws.String(remoteKey),
			Body:         f, // *os.File реализует ReaderAt — ок
			ContentType:  aws.String(ct),
			CacheControl: aws.String("public, max-age=31536000, immutable"),
		})
		return err
	}

	// Внешний ретрай для 5xx/временных сетевых сбоев
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		if err := putOnce(); err == nil {
			return nil
		} else {
			lastErr = err
			var respErr *smithyhttp.ResponseError
			temporary := errors.As(err, &respErr) && respErr.HTTPStatusCode() >= 500
			if !temporary || attempt == 3 {
				break
			}
			time.Sleep(time.Duration(attempt) * time.Second) // небольшой backoff
		}
	}
	return fmt.Errorf("upload failed after retries: %w", lastErr)
}

// func (s *Service) upload(localFile, remoteFile string) error {
// 	f, err := os.OpenFile(localFile, os.O_RDONLY, 0644)
// 	if err != nil {
// 		return err
// 	}
// 	object := s3.PutObjectInput{
// 		Bucket: aws.String(s.bucketName), // The path to the directory you want to upload the object to, starting with your Space name.
// 		Key:    aws.String(remoteFile),   // Object key, referenced whenever you want to access this file later.
// 		Body:   f,                        // The object's contents.
// 		/*		ACL:    aws.String("public-read"),                  // Defines Access-control List (ACL) permissions, such as private or public.
// 				Metadata: map[string]*string{ // Required. Defines metadata tags.
// 					"x-amz-meta-my-key": aws.String("your-value"),
// 				},*/
// 	}

// 	_, err = s.s3Client.PutObject(context.TODO(), &object)
// 	if err != nil {
// 		return err
// 	}
// 	return nil
// }

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
