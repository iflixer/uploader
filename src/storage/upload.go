package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

// --- прогресс для multipart: Uploader читает Body через ReadAt, поэтому считаем в ReadAt ---
type progressFile struct {
	*os.File
	total int64
	read  int64
	cb    func(done, total int64)
}

func (p *progressFile) ReadAt(b []byte, off int64) (int, error) {
	n, err := p.File.ReadAt(b, off)
	if n > 0 && p.cb != nil {
		done := atomic.AddInt64(&p.read, int64(n))
		p.cb(done, p.total)
	}
	return n, err
}

// ---- прогресс для single PUT: ReadSeeker ----
type countingReadSeeker struct {
	rs io.ReadSeeker
	n  int64
	cb func(done int64)
}

func (c *countingReadSeeker) Read(p []byte) (int, error) {
	n, err := c.rs.Read(p)
	if n > 0 && c.cb != nil {
		c.n += int64(n)
		c.cb(c.n)
	}
	return n, err
}
func (c *countingReadSeeker) Seek(off int64, whence int) (int64, error) {
	return c.rs.Seek(off, whence)
}

type ProgressFunc func(done, total int64, mbps float64)

func uploadR2WithProgress(ctx context.Context, s3cli *s3.Client, bucket, key, path string, on ProgressFunc) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return err
	}
	total := st.Size()
	if total < 0 {
		return fmt.Errorf("unknown file size for %s", path)
	}

	ct := mime.TypeByExtension(strings.ToLower(filepath.Ext(path)))
	if ct == "" {
		ct = "application/octet-stream"
	}

	// троттлим уведомления прогресса
	var lastDone int64
	var lastTs = time.Now()
	progress := func(done, total int64) {
		now := time.Now()
		dt := now.Sub(lastTs).Seconds()
		if dt < 0.2 {
			return
		}
		delta := done - lastDone
		if delta <= 0 || dt <= 0 {
			return
		}
		mbps := (float64(delta) / (1024 * 1024)) / dt
		lastDone = done
		lastTs = now
		if on != nil {
			on(done, total, mbps)
		}
	}

	const singlePutThreshold = int64(200 * 1024 * 1024) // 200 MiB — всё, что меньше, грузим одним PUT

	oneAttempt := func() error {
		// SINGLE PUT
		if total <= singlePutThreshold {
			if _, err := f.Seek(0, io.SeekStart); err != nil {
				return err
			}
			body := &countingReadSeeker{rs: f, cb: func(n int64) { progress(n, total) }}

			out, err := s3cli.PutObject(ctx, &s3.PutObjectInput{
				Bucket:        aws.String(bucket),
				Key:           aws.String(key),
				Body:          body, // ReadSeeker — важен для ретраев
				ContentType:   aws.String(ct),
				ContentLength: aws.Int64(total), // фиксируем длину — без chunked/CRC проблем
				CacheControl:  aws.String("public, max-age=31536000, immutable"),
			})
			_ = out
			if err == nil && on != nil {
				on(total, total, 0)
			}
			return err
		}

		// MULTIPART
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return err
		}
		pf := &progressFile{File: f, total: total, cb: progress}

		upl := manager.NewUploader(s3cli, func(u *manager.Uploader) {
			u.PartSize = 32 * 1024 * 1024 // 16–32 MiB — стабильнее на R2
			u.Concurrency = 2             // 1–3; 8 часто даёт 5xx
			u.LeavePartsOnError = false
		})

		_, err := upl.Upload(ctx, &s3.PutObjectInput{
			Bucket:       aws.String(bucket),
			Key:          aws.String(key),
			Body:         pf, // ReadAt-счётчик
			ContentType:  aws.String(ct),
			CacheControl: aws.String("public, max-age=31536000, immutable"),
		})
		if err == nil && on != nil {
			on(total, total, 0)
		}
		return err
	}

	// внешние ретраи — только для 5xx/сетевых временных
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		if err := oneAttempt(); err == nil {
			return nil
		} else {
			lastErr = err
			var re *smithyhttp.ResponseError
			var ne net.Error
			if errors.As(err, &re) && re.HTTPStatusCode() >= 500 && attempt < 3 {
				time.Sleep(time.Duration(attempt) * time.Second)
				continue
			}
			if errors.As(err, &ne) && ne.Temporary() && attempt < 3 {
				time.Sleep(time.Duration(attempt) * time.Second)
				continue
			}
			break
		}
	}
	return fmt.Errorf("upload failed after retries: %w", lastErr)
}
