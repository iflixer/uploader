package httpserv

import (
	"log"
	"net/http"
	"os"
	"uploader/ffmpeg"
)

func (s *Server) probe(w http.ResponseWriter, r *http.Request) {
	log.Println("get probe " + r.RequestURI)
	videofile := r.URL.Query().Get("videofile")

	if videofile == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("videofile?"))
		return
	}

	videofile = "/files/" + videofile

	if _, err := os.Stat(videofile); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("file not found"))
		return
	}

	ff := ffmpeg.NewFfmpeg("", videofile, "", 0)

	res, err := ff.Probe(true)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
	}
	_, _ = w.Write([]byte(res))

}
