package httpserv

import (
	"log"
	"net/http"
	"os"
	"uploader/ffmpeg"
)

func (s *Server) check(w http.ResponseWriter, r *http.Request) {
	log.Println("get check " + r.RequestURI)
	videofile := r.URL.Query().Get("videofile")

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

	res, err := ff.Probe(false)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
	}
	w.Write([]byte(res))
}
