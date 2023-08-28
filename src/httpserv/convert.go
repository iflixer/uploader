package httpserv

import (
	"log"
	"net/http"
	"os"
	"uploader/ffmpeg"
)

func (s *Server) convert(w http.ResponseWriter, r *http.Request) {
	log.Println("get convert " + r.RequestURI)
	r.ParseForm()
	videofile := ""
	if len(r.Form["videofile"]) > 0 {
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
