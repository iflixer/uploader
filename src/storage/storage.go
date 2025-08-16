package storage

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Client struct {
	cli       *minio.Client
	bucket    string
	userAgent string
}

type Service struct {
	Client          *Client
	bucketName      string
	accountID       string
	accessKeyId     string
	accessKeySecret string
}

func (s *Service) Upload(localFile, remoteFile string) (err error) {
	ctx := context.Background()
	err = s.Client.UploadWithProgress(ctx,
		localFile,
		remoteFile,
		func(done, total int64, mbps float64) {
			pct := 100 * float64(done) / float64(total)
			if pct > 99.9 {
				pct = 99.9
			} // 100% покажем по факту успеха
			fmt.Printf("Uploading %s: %6.2f%%  %6.2f MiB/s\n", remoteFile, pct, mbps)
		},
	)
	if err != nil {
		log.Println("ERR Upload for:", remoteFile, err)
	} else {
		log.Println("Upload successful for", remoteFile)
	}
	return
}

func (s *Service) Download(remoteFile, localFile string) (written int64, err error) {
	ctx := context.Background()
	err = s.Client.DownloadWithProgress(ctx,
		remoteFile,
		localFile,
		func(done, total int64, mbps float64) {
			written = total
			pct := 100 * float64(done) / float64(total)
			if pct > 100 {
				pct = 100
			}
			log.Printf("Downloading: %6.2f%%  %6.2f MiB/s", pct, mbps)
		},
	)
	if err != nil {
		log.Println("ERR:", err)
	} else {
		log.Println("OK")
	}
	return
}

func NewService(bucketName, accountID, accessKeyId, accessKeySecret string) (*Service, error) {

	endpoint := accountID + ".r2.cloudflarestorage.com"

	r2Client, err := newR2(endpoint, accessKeyId, accessKeySecret, bucketName)
	if err != nil {
		log.Printf("error creating R2 client: %s\n", err)
		return nil, err
	}

	s := &Service{
		Client:          r2Client,
		bucketName:      bucketName,
		accountID:       accountID,
		accessKeyId:     accessKeyId,
		accessKeySecret: accessKeySecret,
	}

	return s, s.test()
}

func newR2(endpoint, accessKey, secretKey, bucket string) (*Client, error) {
	// Транспорт с хорошими дефолтами и опцией переключить h2/1.1 при желании
	tr := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:        512,
		MaxIdleConnsPerHost: 256,
		IdleConnTimeout:     120 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
		DisableCompression:  true,
		// ForceAttemptHTTP2: false, // если заметишь, что h2 даёт хуже — раскомментируй
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
	}
	httpClient := &http.Client{Transport: tr}

	cli, err := minio.New(endpoint, &minio.Options{
		Creds:     credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure:    true,
		Transport: httpClient.Transport,
	})
	if err != nil {
		return nil, err
	}
	return &Client{cli: cli, bucket: bucket, userAgent: "r2-uploader/1.0"}, nil
}
