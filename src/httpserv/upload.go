package httpserv

import (
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
	"uploader/telegram"

	"github.com/go-errors/errors"
	"github.com/kennygrant/sanitize"
)

const maxChunkSize = int64(50 << 20) // 50 MiB

func (s *Server) upload(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("upload panic: remote=%s uri=%s err=%v stack=%s", r.RemoteAddr, r.RequestURI, rec, string(debug.Stack()))
			http.Error(w, "internal server error", http.StatusInternalServerError)
		}
	}()

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
		log.Printf("chunk accepted: file=%s chunk=%d/%d elapsed=%s", fileNameOut, chunkNumber, chunksTotal, time.Since(start))
		s.returnResp(w, "chunk accepted", nil)
		return
	}

	// последний чанк → финализируем и запускаем обработку
	finalPath := filepath.Join(s.tmpDir, fileNameOut)
	targetPath := filepath.Join("inbox", fileNameOut)
	log.Printf("last chunk assembled: file=%s finalPath=%s target=%s elapsed=%s", fileNameOut, finalPath, targetPath, time.Since(start))
	go s.finalize(finalPath, targetPath, fileNameOut, postID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if os.Getenv("UPLOAD_RESPONSE_FORMAT") == "dle" {
		res := s.createDleResponse(targetPath)
		n, err := w.Write([]byte(res))
		if err != nil {
			log.Printf("last chunk response write error: file=%s target=%s wrote=%d err=%v", fileNameOut, targetPath, n, err)
			return
		}
		log.Printf("last chunk response sent: file=%s target=%s bytes=%d elapsed=%s", fileNameOut, targetPath, n, time.Since(start))
		return
	}

	log.Printf("last chunk response format mismatch: file=%s UPLOAD_RESPONSE_FORMAT=%q", fileNameOut, os.Getenv("UPLOAD_RESPONSE_FORMAT"))
	s.returnResp(w, "chunk completed", nil)
}

// refinalize takes the file already in the tmpDir and finalizes it
func (s *Server) refinalize(w http.ResponseWriter, r *http.Request) {

	nameIn := r.FormValue("filename")
	if nameIn == "" || filepath.Base(nameIn) != nameIn {
		http.Error(w, "invalid file name", http.StatusBadRequest)
		return
	}
	finalPath := filepath.Join(s.tmpDir, nameIn)
	postID := strings.SplitN(nameIn, "_", 2)[0]
	targetPath := filepath.Join("inbox", nameIn)
	go s.finalize(finalPath, targetPath, nameIn, postID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte("OK, reupload started"))
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
	fileSize, err := s.uploadResult(filePathOut, targetPath)
	if err != nil {
		log.Printf("error uploading file %s -> %s to storage: %s\n", filePathOut, targetPath, err)
		s.telegramService.Send(telegram.ChanVideo, fmt.Sprintf("UPLOAD error: %s", err))
		return
	}

	// create task to convert
	s.createConvertTaskAndClean(fileNameOut, filePathOut, targetPath, postId, fileSize)

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

func (s *Server) uploadResult(filePathOut, targetPath string) (fileSize int64, err error) {
	info, err := os.Stat(filePathOut)
	if err != nil {
		return
	}
	fileSize = info.Size()
	for i := range 3 { // retry 3 times
		log.Printf("uploading %s to storage as %s retry %d\n", filePathOut, targetPath, i+1)
		log.Printf("Размер локального файла %s: %d байт\n", filePathOut, info.Size())

		err = s.storage.Upload(filePathOut, targetPath)
		if err != nil {
			return
		}
		objectSize, err := s.storage.Stat(targetPath)
		if err != nil {
			log.Printf("Ошибка получения размера файла %s из R2: %s\n", targetPath, err)
		} else {
			log.Printf("Размер файла в R2 %s: %d байт\n", targetPath, objectSize)
			if info.Size() != objectSize {
				log.Printf("Размеры не совпадают, ожидается %d, получено %d\n", info.Size(), objectSize)
			} else {
				err1 := os.Remove(filePathOut)
				if err1 != nil {
					log.Printf("ошибка при удалении локального файла %s: %v\n", filePathOut, err1)
				} else {
					log.Printf("локальный файл %s удалён\n", filePathOut)
				}
				log.Printf("file %s uploaded to storage as %s size %d\n", filePathOut, targetPath, objectSize)
				return fileSize, nil
			}
		}
		log.Printf("file %s uploaded to storage as %s\n", filePathOut, targetPath)
	}
	return 0, errors.New("failed to upload file after 3 attempts: " + filePathOut)
}

func (s *Server) createConvertTaskAndClean(fileNameOut, filePathOut, targetPath, postId string, fileSize int64) {
	tryID := rand.Intn(999999) + 100000
	for {
		fileNameOutWoExt := strings.TrimSuffix(fileNameOut, filepath.Ext(fileNameOut))
		u := fmt.Sprintf("%s?orig=%s&post_id=%s&name=%s&size=%d&try_id=%d", s.vManagerAddUrl, targetPath, postId, fileNameOutWoExt, fileSize, tryID)
		log.Printf("sending request to vManager to create task: %s\n", u)
		_, err := getUrl(u, nil, false)
		if err == nil {
			log.Printf("task created for %s\n", fileNameOut)
			s.telegramService.Send(telegram.ChanVideo, fmt.Sprintf("UPLOAD task created: %s", fileNameOut))
			return
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

func postUrl(url string, headers map[string]string, isFollowRedirect bool) (response *http.Response, err error) {
	req, err := http.NewRequest("POST", url, nil)
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
