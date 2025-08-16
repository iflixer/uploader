package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
)

// DownloadWithProgress скачивает объект из R2 в файл destPath
// с прогресс‑колбэком и поддержкой дозагрузки (resume).
func (c *Client) DownloadWithProgress(
	ctx context.Context,
	objectKey, destPath string,
	on ProgressFunc,
) error {
	objectKey = strings.TrimLeft(objectKey, "/")

	// Узнаём размер объекта (для процентов)
	st, err := c.cli.StatObject(ctx, c.bucket, objectKey, minio.StatObjectOptions{})
	if err != nil {
		return fmt.Errorf("stat object: %w", err)
	}
	total := st.Size

	// Готовим временный файл *.part (для resume)
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	partPath := destPath + ".part"

	var start int64 = 0
	if fi, err := os.Stat(partPath); err == nil {
		start = fi.Size()
		if start > total {
			// что-то пошло не так — перекачиваем с нуля
			_ = os.Remove(partPath)
			start = 0
		}
	}

	// внешний ретрай целиком
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		// пересчитываем offset (мог измениться после частичной записи)
		if fi, err := os.Stat(partPath); err == nil {
			start = fi.Size()
		} else {
			start = 0
		}

		// Опции с Range для докачки
		getOpts := minio.GetObjectOptions{}
		if start > 0 && start < total {
			if err := getOpts.SetRange(start, 0); err != nil {
				return fmt.Errorf("set range: %w", err)
			}
		}

		obj, err := c.cli.GetObject(ctx, c.bucket, objectKey, getOpts)
		if err != nil {
			lastErr = fmt.Errorf("get object: %w", err)
			time.Sleep(time.Duration(attempt) * time.Second)
			continue
		}
		defer obj.Close()

		// Открываем файл на дозапись
		flag := os.O_CREATE | os.O_WRONLY
		if start > 0 {
			flag |= os.O_APPEND
		} else {
			flag |= os.O_TRUNC
		}
		fout, err := os.OpenFile(partPath, flag, 0o644)
		if err != nil {
			obj.Close()
			return fmt.Errorf("open part: %w", err)
		}

		// будем считать точно: currentWritten
		var currentWritten int64 = 0
		buf := make([]byte, 1<<20) // 1 MiB буфер

		// последний отчёт
		lastReportAt := time.Now()
		for {
			nr, er := obj.Read(buf)
			if nr > 0 {
				nw, ew := fout.Write(buf[:nr])
				if ew != nil {
					_ = fout.Close()
					_ = obj.Close()
					lastErr = fmt.Errorf("write: %w", ew)
					break
				}
				if nw < nr {
					_ = fout.Close()
					_ = obj.Close()
					lastErr = io.ErrShortWrite
					break
				}
				currentWritten += int64(nw)

				// отчёт раз в ~200 мс
				if on != nil {
					now := time.Now()
					if now.Sub(lastReportAt) >= 200*time.Millisecond {
						done := start + currentWritten
						secs := now.Sub(lastReportAt).Seconds()
						var mbps float64
						if secs > 0 {
							mbps = (float64(currentWritten) / (1024 * 1024)) / secs
						}
						on(done, total, mbps)
						lastReportAt = now
						currentWritten = 0
					}
				}
			}
			if er != nil {
				if er == io.EOF {
					// финальный прогресс = 100%
					if on != nil {
						on(start+(st.Size-(total-(start+currentWritten))), total, 0)
						on(total, total, 0)
					}
					lastErr = nil
				} else {
					lastErr = fmt.Errorf("read: %w", er)
				}
				break
			}
		}

		_ = fout.Close()
		_ = obj.Close()

		if lastErr == nil {
			// атомарно переименуем *.part в итоговый файл
			if err := os.Rename(partPath, destPath); err != nil {
				return fmt.Errorf("rename: %w", err)
			}
			return nil
		}

		// небольшой бэк‑офф и повтор
		time.Sleep(time.Duration(attempt) * time.Second)
	}
	return fmt.Errorf("download failed after retries: %w", lastErr)
}
