package httpserv

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"strconv"
	"strings"
	"uploader/ffmpeg"
)

type fileRange struct {
	Start string
	End   string
	Total string
}

func (s *Server) chunk(in multipart.File, filename, contentRange string) (last bool, fileName string) {
	// bytes 0-999/1012602
	fr := fileRange{}
	rangeAndSize := strings.Split(contentRange, " ")
	rangeParts := strings.Split(rangeAndSize[1], "/")
	fr.Total = rangeParts[1]
	curr := strings.Split(rangeParts[0], "-")
	fr.Start = curr[0]
	fr.End = curr[1]

	log.Printf("Chunk: %+v", fr)
	chunkFileName := "chunks_" + s.stringHash(filename)

	f, err := os.OpenFile("/files/"+chunkFileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Errorf("error create file chunks:%s", err)
		return
	}

	if _, err := io.Copy(f, in); err != nil {
		fmt.Errorf("error copy chunk:%s", err)
		return
	}

	e, _ := strconv.Atoi(fr.End)
	t, _ := strconv.Atoi(fr.Total)
	if e == t-1 { // last chunk
		// rename file to its hash
		fname, err := s.fileHashByName("/files/" + chunkFileName)
		if err != nil {
			fmt.Errorf("error opening file for hashing:%s", err)
			return
		}
		fmt.Println("Target filename: ", fname)
		os.Rename("/files/"+chunkFileName, "/files/"+fname)
		fmt.Printf("rename %s to %s\n", "/files/"+chunkFileName, "/files/"+fname)
		/*f, err := os.OpenFile("/files/"+fname, os.O_RDONLY, 0644)
		if err != nil {
			fmt.Errorf("error opening file after rename:%s", err)
			return
		}*/
		return true, fname
	}
	return false, ""
}

func (s *Server) tasks(w http.ResponseWriter, r *http.Request) {
	res, err := s.queue.Tasks()
	if err != nil {
		s.returnResp(w, "error get task list", err)
		return
	}
	wr, _ := json.Marshal(res)
	http.Error(w, string(wr), http.StatusOK)
}
func (s *Server) convert(w http.ResponseWriter, r *http.Request) {
	log.Println("get convert " + r.RequestURI)
	r.ParseForm()
	videofile := ""
	textresp := ""
	origFileName := ""
	r.ParseMultipartForm(10 << 20)
	if file, handler, err := r.FormFile("file"); err == nil {
		fmt.Println("Uploading the file " + handler.Filename)
		defer file.Close()
		if r.Header.Get("Content-Range") != "" {
			if ok, fileName := s.chunk(file, handler.Filename, r.Header.Get("Content-Range")); !ok {
				s.returnResp(w, "added chunk", nil)
				return
			} else {
				videofile = fileName
			}
			origFileName = handler.Filename
			textresp += "Successfully Uploaded chunk for " + handler.Filename + "\n"
		} else {
			fmt.Printf("Uploaded File: %+v\n", handler.Filename)
			fmt.Printf("File Size: %+v\n", handler.Size)
			fmt.Printf("MIME Header: %+v\n", handler.Header)

			videofile, err = s.fileHash(file)
			if err != nil {
				s.returnResp(w, "error hashing file", err)
				return
			}
			fmt.Println("hash file:", videofile)
			dst, err := os.Create("/files/" + videofile)
			defer dst.Close()
			if err != nil {
				s.returnResp(w, "error creating file", err)
				return
			}
			if _, err := io.Copy(dst, file); err != nil {
				s.returnResp(w, "error copy uploaded file", err)
				return
			}
			origFileName = handler.Filename
			textresp += "Successfully Uploaded File " + handler.Filename + " as " + videofile + " .\n"
		}
	}

	if videofile == "" && len(r.Form["videofile"]) > 0 {
		videofile = r.Form["videofile"][0]
	}

	if len(r.Form["magnet"]) > 0 {
		for _, url := range r.Form["magnet"] {
			ff := ffmpeg.NewFfmpeg(url, "", "torrent", 0)
			ff.Magnet = url
			if err := s.queue.Add(ff); err != nil {
				s.returnResp(w, "Error adding magnet to queue", err)
			} else {
				s.returnResp(w, "added magnet to queue", nil)
			}
		}
		return
	}

	if videofile == "" {
		s.returnResp(w, "Error find file", errors.New("videofile parameter is empty"))
		return
	}

	videofilePath := "/files/" + videofile

	if _, err := os.Stat(videofilePath); err != nil {
		s.returnResp(w, "Error find file", errors.New("file not found"))
		return
	}

	ff := ffmpeg.NewFfmpeg(origFileName, videofile, "convert", 0)

	// check the probe
	res, err := ff.Probe(false)
	if err != nil {
		s.returnResp(w, res, errors.New("probe error"))
		return
	}
	textresp += res

	if err := s.queue.Add(ff); err != nil {
		s.returnResp(w, "error adding to queue", err)
		return
	} else {
		s.returnResp(w, textresp, nil)
	}
}
