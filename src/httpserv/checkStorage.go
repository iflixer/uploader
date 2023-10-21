package httpserv

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
)

func (s *Server) checkStorage(w http.ResponseWriter, r *http.Request) {
	log.Println("get check storage " + r.RequestURI)
	path := r.URL.Query().Get("path")
	// ?path=/series/p4/ladybug/5/dub/temp5ep18.mp4

	if path == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("?path=..."))
		return
	}

	path = strings.Trim(path, "/")

	l, err := s.s3.Head(path)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(fmt.Sprintf("%d", l)))
}

func (s *Server) listStorage(w http.ResponseWriter, r *http.Request) {
	log.Println("get list storage " + r.RequestURI)
	path := r.URL.Query().Get("path")
	// ?path=/series/p4/ladybug/5/dub/temp5ep18.mp4

	if path == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("?path=..."))
		return
	}

	path = strings.Trim(path, "/")

	l, err := s.s3.List(path)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusOK)
	wr, _ := json.Marshal(l)
	_, _ = w.Write(wr)
}
