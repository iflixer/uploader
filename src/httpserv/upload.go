package httpserv

import (
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func (s *Server) chunk(in multipart.File, fileNameIn, fileNameOut, contentRange string) (last bool, err error) {
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
}

func (s *Server) upload(w http.ResponseWriter, r *http.Request) {
	log.Println("get upload " + r.RequestURI)
	r.ParseForm()
	textresp := ""
	fileNameIn := ""
	fileNameOut := ""
	r.ParseMultipartForm(10 << 20)
	if file, handler, err := r.FormFile("file"); err == nil {
		fmt.Println("Uploading the file " + handler.Filename)
		defer file.Close()
		contentRange := r.Header.Get("Content-Range")
		fileNameIn = s.stringHash(handler.Filename)
		fileExt := filepath.Ext(fileNameIn)
		fileNameOut = fmt.Sprintf("%s.%s", s.stringHash(fileNameIn), fileExt)
		if contentRange != "" {
			if last, err := s.chunk(file, fileNameIn, fileNameOut, contentRange); !last {
				s.returnResp(w, "add chunk", err)
				return
			}
		} else { // little file?
			fileOutPath := filepath.Join(s.tmpDir, fileNameOut)
			fileOut, err := os.OpenFile(fileOutPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			defer fileOut.Close()
			if err != nil {
				fmt.Errorf("error create out file:%s", err)
				return
			}
			if _, err := io.Copy(fileOut, file); err != nil {
				fmt.Errorf("error copy form file to result file:%s", err)
				return
			}
		}
	}
	textresp += fmt.Sprintf("Successfully Uploaded File %s as %s .\n", fileNameIn, fileNameOut)

	targetPath := filepath.Join("inbox", fileNameOut)
	err := s.storage.Upload(targetPath, filepath.Join(s.tmpDir, fileNameOut))
	if err != nil {
		fmt.Errorf("error uploading file to storage:%s", err)
		return
	}

}
