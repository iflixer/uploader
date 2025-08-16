package storage

import (
	"context"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/minio/minio-go/v7"
)

// UploadWithProgress: заливает файл в R2 с прогрессом и внешними ретраями
// objectKey — путь в бакете, например "inbox/video.mp4" (без ведущего "/")
func (c *Client) UploadWithProgress(ctx context.Context, filePath, objectKey string, on ProgressFunc) error {
	objectKey = strings.TrimLeft(objectKey, "/")

	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat: %w", err)
	}
	total := st.Size()

	ct := mime.TypeByExtension(strings.ToLower(filepath.Ext(filePath)))
	if ct == "" {
		ct = "application/octet-stream"
	}

	opts := minio.PutObjectOptions{
		ContentType: ct,
		// Можно зафиксировать размер части для MPU (помогает стабилизировать скорость):
		// PartSize: 16 * 1024 * 1024, // 16 MiB
		// SendContentMd5: true,       // если нужно строго проверять целостность
	}

	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return err
		}

		tr := &teeCountReader{
			r:          f,
			totalSize:  total,
			lastReport: time.Now(),
			cb: func(done, total int64, mbps float64) {
				if on == nil {
					return
				}
				// Не рисуем «100%» до успешного завершения:
				if done >= total && total > 0 {
					done = total - 1
				}
				on(done, total, mbps)
			},
		}

		_, err := c.cli.PutObject(ctx, c.bucket, objectKey, tr, total, opts)
		if err == nil {
			// финальный 100% только ПОСЛЕ успеха
			if on != nil {
				on(total, total, 0)
			}
			return nil
		}
		lastErr = err
		time.Sleep(time.Duration(attempt) * time.Second) // небольшой backoff
	}
	return fmt.Errorf("upload failed after retries: %w", lastErr)
}

type ProgressFunc func(done, total int64, mbps float64)

type teeCountReader struct {
	r          io.Reader
	totalSize  int64 // общий размер файла (для процента)
	doneTotal  int64 // накопленный прогресс
	winBytes   int64 // байты за окно (для скорости)
	lastReport time.Time
	cb         ProgressFunc
}

func (t *teeCountReader) Read(p []byte) (int, error) {
	n, err := t.r.Read(p)
	if n > 0 {
		atomic.AddInt64(&t.doneTotal, int64(n))
		atomic.AddInt64(&t.winBytes, int64(n))

		// троттлинг отчётов ~5/сек
		now := time.Now()
		if now.Sub(t.lastReport) >= 200*time.Millisecond && t.cb != nil {
			done := atomic.LoadInt64(&t.doneTotal)
			wb := atomic.SwapInt64(&t.winBytes, 0)

			dt := now.Sub(t.lastReport).Seconds()
			mbps := 0.0
			if dt > 0 {
				mbps = (float64(wb) / (1024 * 1024)) / dt
			}

			// не даём вылезать за total, на всякий случай
			if done > t.totalSize && t.totalSize > 0 {
				done = t.totalSize
			}

			t.cb(done, t.totalSize, mbps)
			t.lastReport = now
		}
	}
	return n, err
}
