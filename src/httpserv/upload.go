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
)

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

func (s *Server) chunk(in multipart.File, fileNameOut string, chunkNumber, chunksTotal int) (last bool, err error) {
	last = false
	chunkBaseName := fmt.Sprintf("chunk_%s", fileNameOut)

	chunkPath := filepath.Join(s.tmpDir, fmt.Sprintf("%s_%d", chunkBaseName, chunkNumber))
	filePathOut := filepath.Join(s.tmpDir, fileNameOut)

	f, err := os.OpenFile(chunkPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	defer func(f *os.File) { _ = f.Close() }(f)
	if err != nil {
		fmt.Printf("error create chunk:%s", err)
		return
	}

	if _, err = io.Copy(f, in); err != nil {
		fmt.Printf("error copy chunk:%s", err)
		return
	}

	if chunkNumber == chunksTotal-1 { // last chunk
		log.Println("combine file")
		last = true
		fileOut, err1 := os.OpenFile(filePathOut, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		defer func(fileOut multipart.File) { _ = fileOut.Close() }(fileOut)
		if err1 != nil {
			fmt.Printf("error create output file:%s", err1)
			return
		}

		pattern := filepath.Join(s.tmpDir, fmt.Sprintf("chunk_%s_*", fileNameOut))
		log.Printf("chunks pattern: %s\n", pattern)

		files, err1 := filepath.Glob(pattern)
		if err1 != nil {
			fmt.Printf("error read tmp directory for chunks:%s", err1)
			return
		}

		log.Printf("chunks to combine: %d\n", len(files))

		files, err = sortChunks(files)
		if err != nil {
			fmt.Printf("error sorting chunks:%s", err1)
			return
		}
		log.Printf("chunks to combine (sorted): %d\n", len(files))
		for _, file := range files {
			log.Println("combine chunk:", file)
			fileChunk, err := os.Open(file)
			if err != nil {
				log.Printf("error open chunk: %s", err)
			}
			written, err := io.Copy(fileOut, fileChunk)
			if err != nil {
				log.Printf("error copy chunk: %s", err)
			}
			log.Println("written:", written)
			_ = fileChunk.Close()
		}
		return
	}
	return
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
	// log.Println("post upload ")
	// limit the POST body size to 32.5Mb
	r.Body = http.MaxBytesReader(w, r.Body, 32<<20+512)
	// r.ParseForm()
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		fmt.Printf("error ParseMultipartForm:%s", err)
		return
	}
	textresp := ""
	chunkNumber, _ := strconv.Atoi(r.FormValue("chunk"))
	chunksTotal, _ := strconv.Atoi(r.FormValue("chunks"))
	postId := r.FormValue("news_id")
	_ = postId
	fileNameIn := r.FormValue("name")
	// fileExt := filepath.Ext(fileNameIn)
	fileNameOut := fmt.Sprintf("%s_%s", postId, fileNameIn)

	if file, _, err := r.FormFile("qqfile"); err == nil {
		defer func(file multipart.File) { _ = file.Close() }(file)
		log.Printf("chunk (%s) %d of %d\n", fileNameIn, chunkNumber, chunksTotal)
		if last, err := s.chunk(file, fileNameOut, chunkNumber, chunksTotal); !last {
			s.returnResp(w, "added chunk", err)
			return
		}
	} else {
		fmt.Printf("error parse FormFile:%s", err)
	}
	textresp += fmt.Sprintf("Successfully Uploaded File %s as %s .\n", fileNameIn, fileNameOut)

	log.Printf("file %s uploaded as %s\n", fileNameIn, fileNameOut)

	filePathOut := filepath.Join(s.tmpDir, fileNameOut)
	targetPath := filepath.Join("inbox", fileNameOut)
	log.Printf("uploading %s to storage as %s\n", filePathOut, targetPath)

	go s.finalize(filePathOut, targetPath, fileNameOut)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// w.Header().Set("Content-Encoding", "br")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(s.createDleResponse(targetPath)))
}

func (s *Server) finalize(filePathOut, targetPath, fileNameOut string) {
	// upload to storage
	s.uploadResult(filePathOut, targetPath)
	// create task to convert
	s.createConvertTaskAndClean(fileNameOut, filePathOut, targetPath)
}

func (s *Server) uploadResult(filePathOut, targetPath string) {
	log.Printf("uploading %s to storage as %s\n", filePathOut, targetPath)
	err := s.storage.Upload(filePathOut, targetPath)
	if err != nil {
		fmt.Printf("error uploading file to storage:%s", err)
		return
	}
	log.Printf("file %s uploaded to storage as %s\n", filePathOut, targetPath)
}

func (s *Server) createConvertTaskAndClean(fileNameOut, filePathOut, targetPath string) {
	for {
		u := fmt.Sprintf("%s?orig=%s", s.vManagerAddUrl, targetPath)
		log.Printf("sending request to vManager to create task: %s\n", u)
		_, err := getUrl(u, nil, false)
		if err == nil {
			// remove tmp files
			log.Println("removing temporary files...")
			err := os.Remove(filePathOut)
			if err != nil {
				log.Println(err)
			}
			files, err := filepath.Glob(filepath.Join(s.tmpDir, fmt.Sprintf("chunk_%s_*", fileNameOut)))
			log.Printf("chunk files: %+v\n", files)
			if err != nil {
				log.Println(err)
			}
			for _, f := range files {
				if err := os.Remove(f); err != nil {
					log.Println(err)
				}
			}
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
