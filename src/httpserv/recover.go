package httpserv

import (
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// recoverPending runs on startup, before the HTTP server accepts requests.
// Nothing can be running at this point, so every *.lock file in tmpDir is stale
// (left by a killed container). It cleans them up and returns the assembled files
// that still have to be uploaded to the storage.
func (s *Server) recoverPending() (names []string) {
	entries, err := os.ReadDir(s.tmpDir)
	if err != nil {
		log.Printf("recover: read tmp dir %s: %v", s.tmpDir, err)
		return nil
	}

	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".assemble.lock") {
			continue
		}
		// Сборка была прервана — финальный файл неполный. Удаляем его вместе с lock,
		// чтобы повтор последнего чанка собрал файл заново из .part.
		partial := strings.TrimSuffix(name, ".assemble.lock")
		log.Printf("recover: interrupted assembly, removing partial file %s", partial)
		_ = os.Remove(filepath.Join(s.tmpDir, partial))
		_ = os.Remove(filepath.Join(s.tmpDir, name))
	}

	entries, err = os.ReadDir(s.tmpDir)
	if err != nil {
		log.Printf("recover: read tmp dir %s: %v", s.tmpDir, err)
		return nil
	}

	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			continue
		}
		if strings.HasSuffix(name, ".lock") {
			log.Printf("recover: removing stale lock %s", name)
			_ = os.Remove(filepath.Join(s.tmpDir, name))
			continue
		}
		if strings.HasSuffix(name, ".part") {
			continue
		}
		// Имя собранного файла: <postID>_<name>
		if id, _, ok := strings.Cut(name, "_"); !ok || id == "" {
			continue
		}
		names = append(names, name)
	}
	return names
}

// recoverConcurrency: сколько файлов заливать одновременно (env RECOVER_CONCURRENCY, по умолчанию 3).
func recoverConcurrency() int {
	if n, err := strconv.Atoi(os.Getenv("RECOVER_CONCURRENCY")); err == nil && n > 0 {
		return n
	}
	return 3
}

// finalizePending uploads the files found by recoverPending, a few at a time.
func (s *Server) finalizePending(names []string) {
	var wg sync.WaitGroup
	sem := make(chan struct{}, recoverConcurrency())
	for _, name := range names {
		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			log.Printf("recover: resuming upload of %s", name)
			postID, _, _ := strings.Cut(name, "_")
			s.finalize(filepath.Join(s.tmpDir, name), filepath.Join("inbox", name), name, postID)
		}()
	}
	wg.Wait()
	if len(names) > 0 {
		log.Printf("recover: finished processing %d pending file(s)", len(names))
	}
}
