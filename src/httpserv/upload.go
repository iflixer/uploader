package httpserv

import (
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"uploader/telegram"

	"github.com/kennygrant/sanitize"
)

const maxChunkSize = int64(2 << 20) // 2 MiB

/*func (s *Server) chunk(in multipart.File, fileNameIn, fileNameOut, contentRange string) (last bool, err error) {
	// bytes 0-999/1012602
	rangeAndSize := strings.Split(contentRange, " ")
	rangeParts := strings.Split(rangeAndSize[1], "/")
	fileSize := rangeParts[1]   // 1012602
	chunkRange := rangeParts[0] // 0-999
	curr := strings.Split(chunkRange, "-")
	// chunkStart := curr[0] // 0
	chunkEnd := curr[1] // 999

	chunkBaseName := fmt.Sprintf("chunk_%s", fileNameOut)

	chunkPath := filepath.Join(s.tmpDir, fmt.Sprintf("%s_%s", chunkBaseName, chunkRange))
	log.Printf("chunkPath: %s\n", chunkPath)

	f, err := os.OpenFile(chunkPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	defer f.Close()
	if err != nil {
		fmt.Errorf("error create chunk:%s", err)
		return
	}

	if _, err = io.Copy(f, in); err != nil {
		fmt.Errorf("error copy chunk:%s", err)
		return
	}

	e, _ := strconv.Atoi(chunkEnd)
	t, _ := strconv.Atoi(fileSize)
	if e == t-1 { // last chunk
		last = true
		fileOut, err1 := os.OpenFile(chunkPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		defer fileOut.Close()
		if err1 != nil {
			fmt.Errorf("error create chunk:%s", err1)
			return
		}

		files, err1 := os.ReadDir(s.tmpDir)
		if err1 != nil {
			fmt.Errorf("error read tmp directory:%s", err1)
			return
		}
		for _, file := range files {
			if !file.IsDir() && strings.HasPrefix(file.Name(), chunkBaseName) {
				fileChunk, err1 := os.Open(chunkPath)
				defer fileChunk.Close()
				if err1 != nil {
					fmt.Errorf("error read chunk:%s", err1)
					return
				}
				if _, err2 := io.Copy(fileOut, fileChunk); err2 != nil {
					fmt.Errorf("error copy chunk to result file:%s", err2)
					return
				}
			}
		}
		return
	}
	return
}*/

func (s *Server) chunk(in multipart.File, fileNameOut string, chunkNumber, chunksTotal int) (bool, error) {
	// Безопасное имя
	base := filepath.Base(fileNameOut)
	if base != fileNameOut {
		return false, fmt.Errorf("invalid file name")
	}

	chunkBase := "chunk_" + base
	chunkPath := filepath.Join(s.tmpDir, fmt.Sprintf("%s_%d", chunkBase, chunkNumber))
	filePathOut := filepath.Join(s.tmpDir, base)

	// Чанк: перезаписываем (если номер пришёл повторно)
	f, err := os.OpenFile(chunkPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return false, fmt.Errorf("open chunk: %w", err)
	}
	if _, err = io.Copy(f, in); err != nil {
		_ = f.Close()
		return false, fmt.Errorf("write chunk: %w", err)
	}
	if err = f.Sync(); err != nil { // чтобы следующий этап видел полный чанк
		_ = f.Close()
		return false, fmt.Errorf("fsync chunk: %w", err)
	}
	_ = f.Close()

	// Если это последний номер — пробуем собрать
	if chunkNumber != chunksTotal-1 {
		return false, nil
	}

	// Проверить, что все чанки на месте
	pattern := filepath.Join(s.tmpDir, chunkBase+"_*")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return false, fmt.Errorf("glob: %w", err)
	}
	if len(files) != chunksTotal {
		// Ещё не все чанки долетели — пусть вызывающий повторит позднее
		return false, fmt.Errorf("not all chunks present: have=%d need=%d", len(files), chunksTotal)
	}

	// Отсортировать по номеру
	sort.Slice(files, func(i, j int) bool {
		getIdx := func(p string) int {
			// .../chunk_<name>_<n>
			_, last := filepath.Split(p)
			u := strings.TrimPrefix(last, chunkBase+"_")
			n, _ := strconv.Atoi(u)
			return n
		}
		return getIdx(files[i]) < getIdx(files[j])
	})

	// Собираем во временный файл → атомарный rename
	tmpOut := filePathOut + ".assembling"
	out, err := os.OpenFile(tmpOut, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return false, fmt.Errorf("open out: %w", err)
	}
	buf := make([]byte, 1<<20) // 1MiB буфер
	for _, p := range files {
		inp, err := os.Open(p)
		if err != nil {
			out.Close()
			return false, fmt.Errorf("open chunk %s: %w", p, err)
		}
		if _, err = io.CopyBuffer(out, inp, buf); err != nil {
			inp.Close()
			out.Close()
			return false, fmt.Errorf("copy %s: %w", p, err)
		}
		inp.Close()
	}
	if err = out.Sync(); err != nil {
		out.Close()
		return false, fmt.Errorf("fsync out: %w", err)
	}
	if err = out.Close(); err != nil {
		return false, fmt.Errorf("close out: %w", err)
	}
	if err = os.Rename(tmpOut, filePathOut); err != nil {
		return false, fmt.Errorf("rename: %w", err)
	}

	// (опционально) удалить чанки
	for _, p := range files {
		_ = os.Remove(p)
	}

	return true, nil
}

func sortChunks(data []string) ([]string, error) {
	var lastErr error
	sort.Slice(data, func(i, j int) bool {
		aArr := strings.Split(data[i], "_")
		a, err := strconv.Atoi(aArr[len(aArr)-1])
		if err != nil {
			lastErr = err
			return false
		}
		bArr := strings.Split(data[j], "_")
		b, err := strconv.Atoi(bArr[len(bArr)-1])
		if err != nil {
			lastErr = err
			return false
		}
		return a < b
	})
	return data, lastErr
}

func (s *Server) upload(w http.ResponseWriter, r *http.Request) {
	// Ограничиваем тело
	r.Body = http.MaxBytesReader(w, r.Body, maxChunkSize)

	// ВАЖНО: либо этот путь, либо MultipartReader — но не оба
	if err := r.ParseMultipartForm(maxChunkSize); err != nil {
		http.Error(w, "invalid multipart: "+err.Error(), http.StatusBadRequest)
		return
	}

	chunkNumber, _ := strconv.Atoi(r.FormValue("chunk"))
	chunksTotal, _ := strconv.Atoi(r.FormValue("chunks"))
	postID := r.FormValue("news_id")

	nameIn := sanitize.Path(r.FormValue("name"))
	if nameIn == "" || filepath.Base(nameIn) != nameIn {
		http.Error(w, "invalid file name", http.StatusBadRequest)
		return
	}
	fileNameOut := fmt.Sprintf("%s_%s", postID, nameIn)

	file, hdr, err := r.FormFile("qqfile")
	if err != nil {
		http.Error(w, "file part not found: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	log.Printf("chunk %d/%d size=%d name=%s", chunkNumber, chunksTotal, hdr.Size, nameIn)

	// последовательно пишем в .part (как обсуждали ранее)
	if err := s.appendChunk(file, fileNameOut, chunkNumber, chunksTotal); err != nil {
		http.Error(w, "write chunk: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if chunkNumber != chunksTotal-1 {
		s.returnResp(w, "chunk accepted", nil)
		return
	}

	// последний чанк → финализируем и запускаем обработку
	finalPath := filepath.Join(s.tmpDir, fileNameOut)
	targetPath := filepath.Join("inbox", fileNameOut)
	go s.finalize(finalPath, targetPath, fileNameOut, postID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(s.createDleResponse(targetPath)))
}

// appendChunk пишет чанк в отдельный файл, а на последнем — объединяет.
func (s *Server) appendChunk(r io.Reader, fileNameOut string, chunkNumber, chunksTotal int) error {
	// путь к чанку
	chunkPath := filepath.Join(s.tmpDir, fmt.Sprintf("%s_%d.part", fileNameOut, chunkNumber))
	out, err := os.OpenFile(chunkPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("open chunk: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, r); err != nil {
		return fmt.Errorf("write chunk: %w", err)
	}

	// если это последний чанк → склеиваем
	if chunkNumber == chunksTotal-1 {
		finalPath := filepath.Join(s.tmpDir, fileNameOut)
		fout, err := os.OpenFile(finalPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			return fmt.Errorf("create final file: %w", err)
		}
		defer fout.Close()

		for i := range chunksTotal {
			partPath := filepath.Join(s.tmpDir, fmt.Sprintf("%s_%d.part", fileNameOut, i))
			fin, err := os.Open(partPath)
			if err != nil {
				return fmt.Errorf("open part %d: %w", i, err)
			}
			// io.Copy двигает указатель, поэтому не нужен O_APPEND
			if _, err := io.Copy(fout, fin); err != nil {
				fin.Close()
				return fmt.Errorf("copy part %d: %w", i, err)
			}
			fin.Close()
			// после сборки можно удалить
			_ = os.Remove(partPath)
		}
	}

	return nil
}

func (s *Server) finalize(filePathOut, targetPath, fileNameOut, postId string) {
	// upload to storage
	err := s.uploadResult(filePathOut, targetPath)
	if err != nil {
		log.Printf("error uploading file to storage: %s\n", err)
		s.telegramService.Send(telegram.ChanVideo, fmt.Sprintf("UPLOAD error: %s", err))
		return
	}

	// create task to convert
	s.createConvertTaskAndClean(fileNameOut, filePathOut, targetPath, postId)

	// check if there are no tasks in progress and send notification
	files, err := filepath.Glob(filepath.Join(s.tmpDir, "*"))
	if err != nil {
		log.Println("error check tmp dir: ", err)
	}
	log.Printf("files qty after clean: %d", len(files))
	if len(files) == 0 {
		s.telegramService.Send(telegram.ChanVideo, fmt.Sprintf("UPLOAD done: %s", targetPath))
	}
}

func (s *Server) uploadResult(filePathOut, targetPath string) (err error) {
	log.Printf("uploading %s to storage as %s\n", filePathOut, targetPath)
	err = s.storage.Upload(filePathOut, targetPath)
	if err != nil {
		// log.Printf("error uploading file to storage:%s\n", err)
		return
	}
	log.Printf("file %s uploaded to storage as %s\n", filePathOut, targetPath)
	return nil
}

func (s *Server) createConvertTaskAndClean(fileNameOut, filePathOut, targetPath, postId string) {
	for {
		fileNameOutWoExt := strings.TrimSuffix(fileNameOut, filepath.Ext(fileNameOut))
		u := fmt.Sprintf("%s?orig=%s&post_id=%s&name=%s", s.vManagerAddUrl, targetPath, postId, fileNameOutWoExt)
		log.Printf("sending request to vManager to create task: %s\n", u)
		_, err := getUrl(u, nil, false)
		if err == nil {
			log.Printf("task created for %s\n", fileNameOut)
			s.telegramService.Send(telegram.ChanVideo, fmt.Sprintf("UPLOAD task created: %s", fileNameOut))
			return
			// remove tmp files
			// log.Println("removing temporary files...")
			// err := os.Remove(filePathOut)
			// if err != nil {
			// 	log.Println(err)
			// }
			// files, err := filepath.Glob(filepath.Join(s.tmpDir, fmt.Sprintf("chunk_%s_*", fileNameOut)))
			// // log.Printf("chunk files: %+v\n", files)
			// if err != nil {
			// 	log.Println(err)
			// }
			// for _, f := range files {
			// 	if err := os.Remove(f); err != nil {
			// 		log.Println(err)
			// 	}
			// }
			// return
		} else {
			log.Println("task add error, will try again in 10 seconds:", err)
			time.Sleep(time.Second * 10)
		}
	}
}

func (s *Server) createDleResponse(targetFilePath string) (res string) {
	success := "false"
	if targetFilePath != "" {
		success = "true"
	}
	res = `{"success":%s,"returnbox":"
<div class=\"file-preview-card\" data-type=\"file\" data-area=\"files\" data-deleteid=\"16\" data-url=\"https://alukard.bio/a1/%s\" data-path=\"16:XXX.mp4\" data-play=\"video\" data-public=\"1\">
    \n\t
    <div class=\"active-ribbon\">
        <span>
            <i class=\"mediaupload-icon mediaupload-icon-ok\"></i>
        </span>
    </div>
    \n\t
    <div class=\"file-content\">
        \n\t\t<img src=\"https://testme.cloud/engine/skins/images/video_file.png\" class=\"file-preview-image\">\n\t
    </div>
    \n\t
    <div class=\"file-footer\">
        \n\t\t
        <div class=\"file-footer-caption\">
            \n\t\t\t<div class=\"file-caption-info\" rel=\"tooltip\">%s</div>
            \n\t\t\t<div class=\"file-size-info\">(0 b)</div>
            \n\t\t
        </div>
        \n\t\t
        <div class=\"file-footer-bottom\">
            \n\t\t\t
            <div class=\"file-preview\">
                <a class=\"clipboard-copy-link\" href=\"#\" rel=\"tooltip\" title=\"\">
                    <i class=\"mediaupload-icon mediaupload-icon-copy\"></i>
                </a>
            </div>
            \n\t\t\t
            <div class=\"file-delete\">
                <a class=\"file-delete-link\" href=\"#\">
                    <i class=\"mediaupload-icon mediaupload-icon-trash\"></i>
                </a>
            </div>
            \n\t\t
        </div>
        \n\t
    </div>
    \n
</div>
","uploaded_filename":"%s","xfvalue":"","link":false,"flink":false,"remote_error":null,"tinypng_error":false}
`
	res = strings.ReplaceAll(res, "\n", "")
	res = strings.ReplaceAll(res, "\r", "")
	res = fmt.Sprintf(res, success, targetFilePath, targetFilePath, targetFilePath)
	return
}

func getUrl(url string, headers map[string]string, isFollowRedirect bool) (response *http.Response, err error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Println(err)
		return
	}
	req.Header.Set("DevSecret", "jdhhfUUEUo938HHqppxcbdGa8")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	client := http.Client{
		Timeout: 5 * time.Second,
	}
	if isFollowRedirect {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			// log.Printf("redirect %s to %s\n", via[0].URL, req.URL)
			return nil // allow redirect
			// return errors.NewService("Redirect")
		}
	}
	return client.Do(req)
}
