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
	if n > 0 {
		newDone := atomic.AddInt64(&p.read, int64(n))
		if p.cb != nil {
			p.cb(newDone, p.total)
		}
	}
	return n, err
}

// ---- высокоуровневая загрузка с прогрессом и обработкой smithyhttp.ResponseError ----

type ProgressFunc func(done, total int64, mbps float64)

func uploadR2WithProgress(ctx context.Context, s3cli *s3.Client, bucket, key, path string, on ProgressFunc) error {
	// Открываем файл
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

	// MIME по расширению
	ct := mime.TypeByExtension(strings.ToLower(filepath.Ext(path)))
	if ct == "" {
		ct = "application/octet-stream"
	}

	// Решаем: single PUT или multipart
	const singlePutThreshold = int64(200 * 1024 * 1024) // < 200 MiB — обычный PutObject надёжнее

	// Колбэк прогресса: троттлим до ~5/сек
	var lastDone int64
	var lastTs = time.Now()
	progress := func(done, total int64) {
		now := time.Now()
		dt := now.Sub(lastTs).Seconds()
		if dt < 0.2 {
			return
		}
		delta := done - lastDone
		mbps := 0.0
		if dt > 0 && delta > 0 {
			mbps = (float64(delta) / (1024 * 1024)) / dt
		}
		lastDone = done
		lastTs = now
		if on != nil {
			on(done, total, mbps)
		}
	}

	// Один прогон (для внешнего ретрая)
	putOnce := func() error {
		if total > 0 && total <= singlePutThreshold {
			// Single PUT: вешаем прогресс через TeeReader
			if _, err := f.Seek(0, io.SeekStart); err != nil {
				return err
			}
			pr := &teeCountReader{r: f, cb: func(n int64) { progress(n, total) }}
			_, err := s3cli.PutObject(ctx, &s3.PutObjectInput{
				Bucket:       aws.String(bucket),
				Key:          aws.String(key),
				Body:         pr,
				ContentType:  aws.String(ct),
				CacheControl: aws.String("public, max-age=31536000, immutable"),
			})
			if on != nil {
				on(total, total, 0)
			}
			return err
		}

		// Multipart
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return err
		}
		pf := &progressFile{File: f, total: total, cb: progress}

		upl := manager.NewUploader(s3cli, func(u *manager.Uploader) {
			u.PartSize = 32 * 1024 * 1024 // 16–64 MiB — подбирайте под канал
			u.Concurrency = 2             // 1–3: устойчиво для R2
			u.LeavePartsOnError = false
		})
		_, err := upl.Upload(ctx, &s3.PutObjectInput{
			Bucket:       aws.String(bucket),
			Key:          aws.String(key),
			Body:         pf, // ReadAt-счётчик
			ContentType:  aws.String(ct),
			CacheControl: aws.String("public, max-age=31536000, immutable"),
		})
		if on != nil {
			on(total, total, 0)
		}
		return err
	}

	// Внешний ретрай: повторяем весь upload для 5xx/временных
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		if err := putOnce(); err == nil {
			return nil
		} else {
			lastErr = err
			// Разбираем smithyhttp.ResponseError
			var respErr *smithyhttp.ResponseError
			if errors.As(err, &respErr) {
				code := respErr.HTTPStatusCode()
				// Ретраим только 5xx и сетевые "временные"
				if code >= 500 && code < 600 && attempt < 3 {
					time.Sleep(time.Duration(attempt) * time.Second) // небольшой backoff
					continue
				}
			}
			// Можно добавить специальные кейсы (context deadline, временный net.Error)
			var nerr net.Error
			if errors.As(err, &nerr) && nerr.Temporary() && attempt < 3 {
				time.Sleep(time.Duration(attempt) * time.Second)
				continue
			}
			break
		}
	}
	return fmt.Errorf("upload failed after retries: %w", lastErr)
}

// teeCountReader — простой прогресс для single PUT (читает линейно).
type teeCountReader struct {
	r  io.Reader
	n  int64
	cb func(n int64)
}

func (t *teeCountReader) Read(p []byte) (int, error) {
	n, err := t.r.Read(p)
	if n > 0 {
		t.n += int64(n)
		if t.cb != nil {
			t.cb(t.n)
		}
	}
	return n, err
}
