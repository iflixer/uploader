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
		_, _ = w.Write([]byte("videofile?"))
		return
	}

	videofilePath := "/files/" + videofile

	if _, err := os.Stat(videofilePath); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("file not found"))
		return
	}

	ff := ffmpeg.NewFfmpeg("", videofile, "", 0)

	res, err := ff.Probe(false)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
	}
	_, _ = w.Write([]byte(res))
}
