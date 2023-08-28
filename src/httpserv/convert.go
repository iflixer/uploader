package httpserv

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"uploader/ffmpeg"
)

func (s *Server) convert(w http.ResponseWriter, r *http.Request) {
	log.Println("get convert " + r.RequestURI)
	r.ParseForm()
	videofile := ""
	r.ParseMultipartForm(10 << 20)
	if file, handler, err := r.FormFile("file"); err == nil {
		fmt.Println("Uploading the file")
		defer file.Close()
		fmt.Printf("Uploaded File: %+v\n", handler.Filename)
		fmt.Printf("File Size: %+v\n", handler.Size)
		fmt.Printf("MIME Header: %+v\n", handler.Header)

		videofile = "/files/" + handler.Filename
		dst, err := os.Create(videofile)
		defer dst.Close()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if _, err := io.Copy(dst, file); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		fmt.Fprintf(w, "Successfully Uploaded File\n")
	}

	if videofile == "" && len(r.Form["videofile"]) > 0 {
		videofile = r.Form["videofile"][0]
	}

	if len(r.Form["magnet"]) > 0 {
		for _, url := range r.Form["magnet"] {
			ff := ffmpeg.NewFfmpeg("", "torrent", 0)
			ff.Magnet = url
			if err := s.queue.Add(ff); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(err.Error()))
			} else {
				w.Write([]byte("added magnet to queue"))
			}
		}
		return
	}

	if videofile == "" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("videofile?"))
		return
	}

	videofile = "/files/" + videofile

	if _, err := os.Stat(videofile); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("file not found"))
		return
	}

	ff := ffmpeg.NewFfmpeg(videofile, "", 0)

	// check the probe
	res, err := ff.Probe(false)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(res))
		return
	}

	if err := s.queue.Add(ff); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
	} else {
		w.Write([]byte("added to queue"))
	}
}
